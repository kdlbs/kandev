package managedruntime

import (
	"context"
	"errors"
	"testing"
	"time"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.1
func TestAutomaticRuntimeConsentPersistsAndDoesNotCrossRuntimeIdentity(t *testing.T) {
	settings := &memorySettings{}
	ctx := context.Background()
	store := NewAutoUpdateStore(settings)
	policy, err := store.Get(ctx, "claude-acp", "npm:claude")
	if err != nil || policy.Enabled {
		t.Fatalf("default policy: %+v, %v", policy, err)
	}
	policy = AutoUpdatePolicy{RuntimeID: "npm:claude", Enabled: true, AttemptedVersion: "1.2.0", Outcome: &UpdateOutcome{ID: "attempt-1", Status: "failed", PreviousVersion: "1.1.0", TargetVersion: "1.2.0", FinishedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}}
	if err := store.Save(ctx, "claude-acp", policy); err != nil {
		t.Fatal(err)
	}
	restarted := NewAutoUpdateStore(settings)
	got, err := restarted.Get(ctx, "claude-acp", "npm:claude")
	if err != nil || !got.Enabled || got.AttemptedVersion != "1.2.0" || got.Outcome == nil || got.Outcome.PreviousVersion != "1.1.0" {
		t.Fatalf("restart policy: %+v, %v", got, err)
	}
	for _, id := range []string{"npm:changed-package", "native:claude"} {
		got, err := restarted.Get(ctx, "claude-acp", id)
		if err != nil || got.Enabled || got.Outcome != nil {
			t.Fatalf("cross-runtime consent: %+v, %v", got, err)
		}
	}
	got, err = restarted.Get(ctx, "codex-acp", "npm:claude")
	if err != nil || got.Enabled {
		t.Fatal("consent crossed agent identity")
	}
}

func TestAutomaticRuntimePolicyFailsClosedOnPersistenceFailure(t *testing.T) {
	settings := &memorySettings{err: errors.New("store offline")}
	store := NewAutoUpdateStore(settings)
	if policy, err := store.Get(context.Background(), "agent", "npm:package"); err == nil || policy.Enabled {
		t.Fatal("failed read authorized automation")
	}
	if err := store.Save(context.Background(), "agent", AutoUpdatePolicy{RuntimeID: "npm:package", Enabled: true}); err == nil {
		t.Fatal("failed save reported success")
	}
}
