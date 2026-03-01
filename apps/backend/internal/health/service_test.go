package health

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "json",
		OutputPath: "stderr",
	})
	return log
}

func TestRunChecks_NoCheckers(t *testing.T) {
	svc := NewService(newTestLogger())
	result := svc.RunChecks(context.Background())
	if !result.Healthy {
		t.Error("expected healthy = true with no checkers")
	}
	if len(result.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(result.Issues))
	}
}

func TestRunChecks_AllHealthy(t *testing.T) {
	svc := NewService(newTestLogger(),
		NewGitHubChecker(&mockGitHubProvider{authenticated: true}),
		NewAgentChecker(&mockAgentProvider{available: true}),
	)
	result := svc.RunChecks(context.Background())
	if !result.Healthy {
		t.Error("expected healthy = true")
	}
	if len(result.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(result.Issues))
	}
}

func TestRunChecks_WithWarnings(t *testing.T) {
	svc := NewService(newTestLogger(),
		NewGitHubChecker(nil), // will produce a warning
		NewAgentChecker(&mockAgentProvider{available: true}),
	)
	result := svc.RunChecks(context.Background())
	if result.Healthy {
		t.Error("expected healthy = false with warnings")
	}
	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(result.Issues))
	}
	if result.Issues[0].ID != "github_unavailable" {
		t.Errorf("issue ID = %q, want %q", result.Issues[0].ID, "github_unavailable")
	}
}

func TestRunChecks_MultipleIssues(t *testing.T) {
	svc := NewService(newTestLogger(),
		NewGitHubChecker(nil), // warning
		NewAgentChecker(&mockAgentProvider{available: false}), // warning
	)
	result := svc.RunChecks(context.Background())
	if result.Healthy {
		t.Error("expected healthy = false")
	}
	if len(result.Issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(result.Issues))
	}
}

func TestRunChecks_InfoOnlyIsHealthy(t *testing.T) {
	// A checker that returns only info-severity issues
	infoChecker := &staticChecker{issues: []Issue{{
		ID:       "info_only",
		Category: "system",
		Title:    "Info message",
		Severity: SeverityInfo,
	}}}
	svc := NewService(newTestLogger(), infoChecker)
	result := svc.RunChecks(context.Background())
	if !result.Healthy {
		t.Error("expected healthy = true with info-only issues")
	}
	if len(result.Issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(result.Issues))
	}
}

// staticChecker is a test helper that returns fixed issues.
type staticChecker struct {
	issues []Issue
}

func (c *staticChecker) Check(_ context.Context) []Issue {
	return c.issues
}
