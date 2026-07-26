package review

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeProfiles struct {
	profile ReviewerProfile
	found   bool
	err     error
}

func (f *fakeProfiles) ReviewerProfile(context.Context, string) (ReviewerProfile, bool, error) {
	return f.profile, f.found, f.err
}

type fakeUtility struct {
	agentID string
	model   string
	enabled bool
	found   bool
	err     error
}

func (f *fakeUtility) CodeReviewAgent(context.Context) (string, string, bool, bool, error) {
	return f.agentID, f.model, f.enabled, f.found, f.err
}

type fakeDefaults struct {
	agentID string
	model   string
	err     error
}

func (f *fakeDefaults) DefaultUtilitySettings(context.Context) (string, string, error) {
	return f.agentID, f.model, f.err
}

func TestResolve_PrefersExplicitAgentProfile(t *testing.T) {
	r := NewResolver(
		&fakeProfiles{found: true, profile: ReviewerProfile{AgentID: "codex-acp", Model: "gpt-5.3", Mode: "review", Name: "Reviewer"}},
		&fakeUtility{found: true, enabled: true, agentID: "claude-acp", model: "claude-haiku-4-5"},
		&fakeDefaults{agentID: "claude-acp", model: "claude-haiku-4-5"},
	)

	got, err := r.Resolve(context.Background(), "profile-1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.AgentID != "codex-acp" || got.Model != "gpt-5.3" || got.Mode != "review" {
		t.Fatalf("expected the step profile to win, got %+v", got)
	}
	if got.Source != SourceAgentProfile {
		t.Fatalf("expected agent_profile source, got %q", got.Source)
	}
}

func TestResolve_RejectsPassthroughProfile(t *testing.T) {
	r := NewResolver(
		&fakeProfiles{found: true, profile: ReviewerProfile{AgentID: "claude", CLIPassthrough: true, Name: "TUI profile"}},
		nil, nil,
	)

	_, err := r.Resolve(context.Background(), "profile-1")
	if !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("expected ErrAgentUnavailable, got %v", err)
	}
	if !strings.Contains(err.Error(), "TUI profile") || !strings.Contains(err.Error(), "passthrough") {
		t.Fatalf("error should name the profile and the reason, got %v", err)
	}
}

func TestResolve_ProfileFailures(t *testing.T) {
	cases := map[string]*Resolver{
		"unknown profile":   NewResolver(&fakeProfiles{found: false}, nil, nil),
		"lookup error":      NewResolver(&fakeProfiles{err: errors.New("db down")}, nil, nil),
		"no profiles wired": NewResolver(nil, nil, nil),
		"profile without agent": NewResolver(
			&fakeProfiles{found: true, profile: ReviewerProfile{Model: "m"}}, nil, nil),
		"profile without model": NewResolver(
			&fakeProfiles{found: true, profile: ReviewerProfile{AgentID: "a"}}, nil, nil),
	}
	for name, r := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := r.Resolve(context.Background(), "profile-1"); !errors.Is(err, ErrAgentUnavailable) {
				t.Fatalf("expected ErrAgentUnavailable, got %v", err)
			}
		})
	}
}

func TestResolve_ProfileBorrowsDefaultModelOnlyForSameAgent(t *testing.T) {
	sameAgent := NewResolver(
		&fakeProfiles{found: true, profile: ReviewerProfile{AgentID: "claude-acp"}},
		nil,
		&fakeDefaults{agentID: "claude-acp", model: "claude-haiku-4-5"},
	)
	got, err := sameAgent.Resolve(context.Background(), "p")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Model != "claude-haiku-4-5" {
		t.Fatalf("expected the same-agent default model, got %+v", got)
	}

	// A default model belonging to another provider must never be crossed over.
	crossAgent := NewResolver(
		&fakeProfiles{found: true, profile: ReviewerProfile{AgentID: "codex-acp"}},
		nil,
		&fakeDefaults{agentID: "claude-acp", model: "claude-haiku-4-5"},
	)
	if _, err := crossAgent.Resolve(context.Background(), "p"); !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("expected ErrAgentUnavailable rather than a cross-provider model, got %v", err)
	}
}

func TestResolve_UsesCodeReviewUtilityAgent(t *testing.T) {
	r := NewResolver(nil,
		&fakeUtility{found: true, enabled: true, agentID: "claude-acp", model: "claude-haiku-4-5"},
		&fakeDefaults{agentID: "other", model: "other-model"},
	)

	got, err := r.Resolve(context.Background(), "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.AgentID != "claude-acp" || got.Model != "claude-haiku-4-5" {
		t.Fatalf("unexpected identity: %+v", got)
	}
	if got.Source != SourceUtilityAgent {
		t.Fatalf("expected utility_agent source, got %q", got.Source)
	}
}

func TestResolve_DisabledUtilityAgentFallsBackToDefaults(t *testing.T) {
	r := NewResolver(nil,
		&fakeUtility{found: true, enabled: false},
		&fakeDefaults{agentID: "claude-acp", model: "claude-haiku-4-5"},
	)

	got, err := r.Resolve(context.Background(), "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Source != SourceUserDefault {
		t.Fatalf("expected the user default to be used, got %+v", got)
	}
}

func TestResolve_UtilityAgentWithoutModelTakesDefaultPair(t *testing.T) {
	r := NewResolver(nil,
		&fakeUtility{found: true, enabled: true, agentID: "claude-acp", model: ""},
		&fakeDefaults{agentID: "claude-acp", model: "claude-haiku-4-5"},
	)

	got, err := r.Resolve(context.Background(), "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.AgentID != "claude-acp" || got.Model != "claude-haiku-4-5" {
		t.Fatalf("expected the default pair, got %+v", got)
	}
}

func TestResolve_UtilityAgentModelFromDifferentAgentIsRejected(t *testing.T) {
	// A blank model on the code-review agent means "use my defaults", but the
	// default belongs to a different provider — crossing them would send one
	// provider's model to another's ACP config.
	r := NewResolver(nil,
		&fakeUtility{found: true, enabled: true, agentID: "codex-acp", model: ""},
		&fakeDefaults{agentID: "claude-acp", model: "claude-haiku-4-5"},
	)

	if _, err := r.Resolve(context.Background(), ""); !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("expected ErrAgentUnavailable, got %v", err)
	}
}

func TestResolve_NothingConfiguredFailsClosedWithSettingsHint(t *testing.T) {
	for name, r := range map[string]*Resolver{
		"no lookups":        NewResolver(nil, nil, nil),
		"missing utility":   NewResolver(nil, &fakeUtility{found: false}, &fakeDefaults{}),
		"empty defaults":    NewResolver(nil, nil, &fakeDefaults{}),
		"defaults error":    NewResolver(nil, nil, &fakeDefaults{err: errors.New("boom")}),
		"utility error":     NewResolver(nil, &fakeUtility{err: errors.New("boom")}, nil),
		"model but noagent": NewResolver(nil, &fakeUtility{found: true, enabled: true, model: "m"}, nil),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := r.Resolve(context.Background(), ""); !errors.Is(err, ErrAgentUnavailable) {
				t.Fatalf("expected ErrAgentUnavailable, got %v", err)
			}
		})
	}

	_, err := NewResolver(nil, nil, &fakeDefaults{}).Resolve(context.Background(), "")
	if !strings.Contains(err.Error(), "Settings > Utility Agents") {
		t.Fatalf("the error must point the user at the settings page, got %v", err)
	}
}

func TestResolve_TrimsProfileID(t *testing.T) {
	r := NewResolver(
		&fakeProfiles{found: true, profile: ReviewerProfile{AgentID: "a", Model: "m"}},
		&fakeUtility{found: true, enabled: true, agentID: "u", model: "um"}, nil,
	)
	// A blank-but-whitespace profile id must fall through to the utility agent,
	// not be treated as an explicit (and unknown) profile.
	got, err := r.Resolve(context.Background(), "   ")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Source != SourceUtilityAgent {
		t.Fatalf("expected the whitespace id to be ignored, got %+v", got)
	}
}

func TestCodeFor(t *testing.T) {
	cases := map[error]string{
		nil:                     "",
		ErrAgentUnavailable:     CodeAgentUnavailable,
		ErrWorkspaceUnavailable: CodeWorkspaceUnavailable,
		ErrNoChanges:            CodeNoChanges,
		ErrUnparseableResponse:  CodeUnparseableResponse,
		errors.New("other"):     CodeExecutionFailed,
	}
	for err, want := range cases {
		if got := CodeFor(err); got != want {
			t.Fatalf("CodeFor(%v) = %q, want %q", err, got, want)
		}
	}
}
