package health

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// --- GitHubChecker tests ---

type mockGitHubProvider struct {
	authenticated bool
	authMethod    string
}

func (m *mockGitHubProvider) IsAuthenticated() bool { return m.authenticated }
func (m *mockGitHubProvider) AuthMethod() string    { return m.authMethod }

// expectedGitHubFixURL is the live route in apps/web/app/settings/integrations/github.
// Kept here so a typo or accidental rename breaks a single test instead of
// shipping a 404 to users.
const expectedGitHubFixURL = "/settings/integrations/github"

func TestGitHubChecker_NilProvider(t *testing.T) {
	checker := NewGitHubChecker(nil)
	issues := checker.Check(context.Background())
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].ID != "github_unavailable" {
		t.Errorf("issue ID = %q, want %q", issues[0].ID, "github_unavailable")
	}
	if issues[0].Severity != SeverityWarning {
		t.Errorf("severity = %q, want %q", issues[0].Severity, SeverityWarning)
	}
	if issues[0].Category != "github" {
		t.Errorf("category = %q, want %q", issues[0].Category, "github")
	}
	if issues[0].FixURL != expectedGitHubFixURL {
		t.Errorf("FixURL = %q, want %q", issues[0].FixURL, expectedGitHubFixURL)
	}
}

func TestGitHubChecker_NotAuthenticated(t *testing.T) {
	checker := NewGitHubChecker(&mockGitHubProvider{authenticated: false, authMethod: "gh_cli"})
	issues := checker.Check(context.Background())
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].ID != "github_not_authenticated" {
		t.Errorf("issue ID = %q, want %q", issues[0].ID, "github_not_authenticated")
	}
	if issues[0].Severity != SeverityWarning {
		t.Errorf("severity = %q, want %q", issues[0].Severity, SeverityWarning)
	}
	if issues[0].FixURL != expectedGitHubFixURL {
		t.Errorf("FixURL = %q, want %q", issues[0].FixURL, expectedGitHubFixURL)
	}
}

func TestGitHubChecker_Authenticated(t *testing.T) {
	checker := NewGitHubChecker(&mockGitHubProvider{authenticated: true, authMethod: "gh_cli"})
	issues := checker.Check(context.Background())
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

type stubRateLimitProvider struct {
	exhausted []GitHubRateLimitStatus
}

func (s *stubRateLimitProvider) ExhaustedRateLimits() []GitHubRateLimitStatus {
	return s.exhausted
}

func TestGitHubChecker_AuthenticatedButRateLimited(t *testing.T) {
	rate := &stubRateLimitProvider{exhausted: []GitHubRateLimitStatus{
		{Resource: "graphql", ResetAt: time.Now().Add(15 * time.Minute)},
	}}
	checker := NewGitHubChecker(&mockGitHubProvider{authenticated: true, authMethod: "gh_cli"})
	checker.WithRateLimitProvider(rate)
	issues := checker.Check(context.Background())
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].ID != "github_rate_limit_graphql" {
		t.Errorf("ID = %q, want github_rate_limit_graphql", issues[0].ID)
	}
	if !strings.Contains(issues[0].Message, "resets in") {
		t.Errorf("message should include reset countdown, got %q", issues[0].Message)
	}
	if issues[0].FixURL != expectedGitHubFixURL {
		t.Errorf("FixURL = %q, want %q", issues[0].FixURL, expectedGitHubFixURL)
	}
}

func TestGitHubChecker_AuthenticatedRateProviderEmpty(t *testing.T) {
	rate := &stubRateLimitProvider{}
	checker := NewGitHubChecker(&mockGitHubProvider{authenticated: true})
	checker.WithRateLimitProvider(rate)
	if got := checker.Check(context.Background()); len(got) != 0 {
		t.Errorf("expected 0 issues when rate provider empty, got %d", len(got))
	}
}

// --- AgentChecker tests ---

type mockAgentProvider struct {
	available bool
	err       error
}

func (m *mockAgentProvider) HasAvailableAgents(_ context.Context) (bool, error) {
	return m.available, m.err
}

func TestAgentChecker_Available(t *testing.T) {
	checker := NewAgentChecker(&mockAgentProvider{available: true})
	issues := checker.Check(context.Background())
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestAgentChecker_NotAvailable(t *testing.T) {
	checker := NewAgentChecker(&mockAgentProvider{available: false})
	issues := checker.Check(context.Background())
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].ID != "no_agents" {
		t.Errorf("issue ID = %q, want %q", issues[0].ID, "no_agents")
	}
	if issues[0].Category != "agents" {
		t.Errorf("category = %q, want %q", issues[0].Category, "agents")
	}
	if issues[0].Severity != SeverityWarning {
		t.Errorf("severity = %q, want %q", issues[0].Severity, SeverityWarning)
	}
}

func TestAgentChecker_Error(t *testing.T) {
	checker := NewAgentChecker(&mockAgentProvider{err: fmt.Errorf("discovery failed")})
	issues := checker.Check(context.Background())
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue on error, got %d", len(issues))
	}
	if issues[0].ID != "agent_detection_failed" {
		t.Errorf("issue ID = %q, want %q", issues[0].ID, "agent_detection_failed")
	}
	if issues[0].Severity != SeverityWarning {
		t.Errorf("severity = %q, want %q", issues[0].Severity, SeverityWarning)
	}
}

func TestAgentChecker_NilProvider(t *testing.T) {
	checker := NewAgentChecker(nil)
	issues := checker.Check(context.Background())
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for nil provider, got %d", len(issues))
	}
}
