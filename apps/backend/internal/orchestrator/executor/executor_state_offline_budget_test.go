package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.2
func TestOfflineBudgetProfileResolution(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"empty means default 15", "", 15},
		{"minimum", "1", 1},
		{"maximum", "1440", 1440},
		{"typical value", "30", 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveOfflineBudgetMinutes(tc.raw)
			if err != nil {
				t.Fatalf("ResolveOfflineBudgetMinutes(%q) returned error: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("ResolveOfflineBudgetMinutes(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

// @covers AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.2
func TestOfflineBudgetProfileValidation(t *testing.T) {
	invalid := []string{"0", "1441", "-5", "abc", "3.5", "15m"}
	for _, raw := range invalid {
		t.Run(raw, func(t *testing.T) {
			if _, err := ResolveOfflineBudgetMinutes(raw); !errors.Is(err, ErrInvalidOfflineBudget) {
				t.Fatalf("ResolveOfflineBudgetMinutes(%q) error = %v, want ErrInvalidOfflineBudget", raw, err)
			}
		})
	}
}

// @covers AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.2
func TestResolveExecutorConfig_OfflineBudgetProfileOverridesTaskMetadata(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	repo.executors["exec-1"] = &models.Executor{
		ID:   "exec-1",
		Type: models.ExecutorTypeLocalDocker,
	}
	repo.executorProfiles["prof-1"] = &models.ExecutorProfile{
		ID:     "prof-1",
		Config: map[string]string{lifecycle.MetadataKeyOfflineBudgetMinutes: "45"},
	}

	cfg := exec.resolveExecutorConfig(context.Background(), "exec-1", "ws-1",
		map[string]interface{}{
			"executor_profile_id":                     "prof-1",
			lifecycle.MetadataKeyOfflineBudgetMinutes: "999",
		})

	if got := cfg.Metadata[lifecycle.MetadataKeyOfflineBudgetMinutes]; got != "45" {
		t.Fatalf("offline_budget_minutes = %v, want profile value 45 (authoritative over task-supplied 999)", got)
	}
}

// @covers AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.2
func TestValidateOfflineBudgetMetadata(t *testing.T) {
	if err := validateOfflineBudgetMetadata(map[string]interface{}{
		lifecycle.MetadataKeyOfflineBudgetMinutes: "30",
	}); err != nil {
		t.Fatalf("unexpected error for valid value: %v", err)
	}
	if err := validateOfflineBudgetMetadata(map[string]interface{}{}); err != nil {
		t.Fatalf("unexpected error for absent value: %v", err)
	}
	err := validateOfflineBudgetMetadata(map[string]interface{}{
		lifecycle.MetadataKeyOfflineBudgetMinutes: "not-a-number",
	})
	if !errors.Is(err, ErrInvalidOfflineBudget) {
		t.Fatalf("error = %v, want ErrInvalidOfflineBudget", err)
	}
}
