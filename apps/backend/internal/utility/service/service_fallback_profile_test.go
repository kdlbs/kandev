package service

import (
	"context"
	"errors"
	"testing"

	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/template"
)

// recordingProfileResolver resolves known profile IDs and records each request.
type recordingProfileResolver struct {
	known     map[string]*agentsettingsmodels.AgentProfile
	requested []string
}

func (r *recordingProfileResolver) Resolve(_ context.Context, id string) (*agentsettingsmodels.AgentProfile, error) {
	r.requested = append(r.requested, id)
	if profile, ok := r.known[id]; ok {
		return profile, nil
	}
	return nil, errors.New("profile not eligible")
}

func (r *recordingProfileResolver) MatchLegacy(context.Context, string, string) (*agentsettingsmodels.AgentProfile, error) {
	return nil, errors.New("not used")
}

func fallbackTestService(binding string, boundProfileID string) (*Service, *recordingProfileResolver) {
	resolver := &recordingProfileResolver{known: map[string]*agentsettingsmodels.AgentProfile{
		"bound":   {ID: "bound", AgentID: "claude-acp", Model: "haiku"},
		"default": {ID: "default", AgentID: "codex-acp", Model: "gpt-5"},
		"session": {ID: "session", AgentID: "opencode-acp", Model: "sol"},
	}}
	svc := NewService(&fakeRepository{agents: map[string]*models.UtilityAgent{
		"builtin": {
			ID:                  "builtin",
			Prompt:              "Predict {{ConversationHistory}}",
			Builtin:             true,
			ProfileBindingState: binding,
			AgentProfileID:      boundProfileID,
		},
	}})
	svc.SetProfileResolver(resolver)
	return svc, resolver
}

func prepareWithDefaults(svc *Service, defaults *DefaultUtilitySettings) (*PromptRequest, error) {
	return svc.PreparePromptRequest(context.Background(), "builtin",
		&template.Context{ConversationHistory: "User: hi"}, defaults, true)
}

// TestPreparePromptRequest_FallbackProfileOrder verifies the fallback profile is
// used only when the binding inherits and no default utility profile exists.
func TestPreparePromptRequest_FallbackProfileOrder(t *testing.T) {
	cases := []struct {
		name     string
		binding  string
		bound    string
		defaults *DefaultUtilitySettings
		want     string
	}{
		{name: "no binding and no default uses the session profile", binding: models.ProfileBindingInherit,
			defaults: &DefaultUtilitySettings{FallbackProfileID: "session"}, want: "session"},
		{name: "default wins over the session profile", binding: models.ProfileBindingInherit,
			defaults: &DefaultUtilitySettings{ProfileID: "default", FallbackProfileID: "session"}, want: "default"},
		{name: "explicit binding wins over the session profile", binding: models.ProfileBindingExplicit, bound: "bound",
			defaults: &DefaultUtilitySettings{FallbackProfileID: "session"}, want: "bound"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, resolver := fallbackTestService(tc.binding, tc.bound)
			req, err := prepareWithDefaults(svc, tc.defaults)
			if err != nil {
				t.Fatalf("PreparePromptRequest() error = %v", err)
			}
			if req.AgentProfileID != tc.want {
				t.Fatalf("AgentProfileID = %q, want %q", req.AgentProfileID, tc.want)
			}
			if len(resolver.requested) != 1 || resolver.requested[0] != tc.want {
				t.Fatalf("resolver requests = %v, want only %q", resolver.requested, tc.want)
			}
		})
	}
}

// TestPreparePromptRequest_FallbackProfileFailsClosed verifies an ineligible
// fallback profile and a missing one keep the existing fail-closed behavior.
func TestPreparePromptRequest_FallbackProfileFailsClosed(t *testing.T) {
	svc, _ := fallbackTestService(models.ProfileBindingInherit, "")
	if _, err := prepareWithDefaults(svc, &DefaultUtilitySettings{FallbackProfileID: "passthrough"}); err == nil {
		t.Fatal("ineligible fallback profile resolved, want an error")
	}
	if _, err := prepareWithDefaults(svc, &DefaultUtilitySettings{}); !errors.Is(err, ErrProfileRequired) {
		t.Fatalf("no fallback error = %v, want %v", err, ErrProfileRequired)
	}
}
