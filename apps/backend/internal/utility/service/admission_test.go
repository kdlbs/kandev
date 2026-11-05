package service

import (
	"context"
	"errors"
	"testing"

	settings "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/template"
)

type admissionProfileResolver struct{ calls int }

func (r *admissionProfileResolver) Resolve(context.Context, string) (*settings.AgentProfile, error) {
	r.calls++
	return &settings.AgentProfile{ID: "profile", AgentID: "codex-acp"}, nil
}
func (r *admissionProfileResolver) MatchLegacy(context.Context, string, string) (*settings.AgentProfile, error) {
	return nil, nil
}

type admissionRouteResolver struct{ calls int }

func (r *admissionRouteResolver) ResolveExecution(context.Context, string) (*settings.AgentProfile, string, error) {
	r.calls++
	return &settings.AgentProfile{ID: "profile", AgentID: "codex-acp"}, "profile", nil
}

// @covers AC-AGENTS-UTILITY-AGENT-PROFILES-001.9
func TestPreparePromptRequestDisabledCustomAdmission(t *testing.T) {
	agent := &models.UtilityAgent{ID: "custom", Prompt: "Hello {{UserPrompt}}", AgentProfileID: "profile", ProfileBindingState: models.ProfileBindingExplicit}
	repo := &fakeRepository{agents: map[string]*models.UtilityAgent{"custom": agent}}
	svc := NewService(repo)
	profiles := &admissionProfileResolver{}
	routes := &admissionRouteResolver{}
	svc.SetProfileResolver(profiles)
	svc.SetExecutionProfileResolver(routes)
	for _, sessionless := range []bool{false, true} {
		_, err := svc.PreparePromptRequest(context.Background(), "custom", &template.Context{}, nil, sessionless)
		if !errors.Is(err, ErrAgentDisabled) {
			t.Errorf("disabled custom sessionless=%v: error = %v", sessionless, err)
		}
	}
	if profiles.calls != 0 || routes.calls != 0 {
		t.Errorf("disabled custom resolved %d profiles and %d routes", profiles.calls, routes.calls)
	}
	agent.Enabled = true
	prepared, err := svc.PreparePromptRequest(context.Background(), "custom", &template.Context{UserPrompt: "world"}, nil, false)
	if err != nil || prepared == nil || prepared.ResolvedPrompt != "Hello world" || profiles.calls != 1 || routes.calls != 1 {
		t.Fatalf("enabled: prepared=%+v err=%v profile=%d route=%d", prepared, err, profiles.calls, routes.calls)
	}
	agent.Enabled = false
	if prepared.ResolvedPrompt != "Hello world" {
		t.Fatal("admitted request changed after disable")
	}
	agent.Builtin = true
	agent.ProfileBindingState = models.ProfileBindingInherit
	agent.AgentProfileID = ""
	_, err = svc.PreparePromptRequest(context.Background(), "custom", &template.Context{}, &DefaultUtilitySettings{ProfileID: "profile"}, false)
	if err != nil {
		t.Fatalf("disabled built-in inheritance: %v", err)
	}
}
