package health

import (
	"context"
	"fmt"
	"testing"
)

// --- GitHubChecker tests ---

type mockGitHubProvider struct {
	authenticated bool
	authMethod    string
}

func (m *mockGitHubProvider) IsAuthenticated() bool { return m.authenticated }
func (m *mockGitHubProvider) AuthMethod() string    { return m.authMethod }

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
}

func TestGitHubChecker_Authenticated(t *testing.T) {
	checker := NewGitHubChecker(&mockGitHubProvider{authenticated: true, authMethod: "gh_cli"})
	issues := checker.Check(context.Background())
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
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
