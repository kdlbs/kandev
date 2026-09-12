package service_test

import (
	"context"
	"testing"

	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// TestQueueRun_WakeCarryingRequest_NotCoalescedIntoNonWaveRow isolates the
// REQUEST side of AC-OFFICE-WAKE-WAVE-IDENTITY-002.14 specifically: the
// existing QUEUED row for the same agent+reason carries NO wave identity,
// so CoalesceRun's own row-side guard (`AND wake_wave_key = ”`) would
// happily accept it as a merge candidate — only shouldCoalesceRun's request
// side ("a wave-carrying request is never coalesced") can prevent a merge
// here. TestQueueRun_WakeCarryingRequest_NotCoalesced cannot prove this: both
// of its requests carry a wave key, so its outcome is fully explained by the
// row-side guard alone regardless of what the request-side check does.
func TestQueueRun_WakeCarryingRequest_NotCoalescedIntoNonWaveRow(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	first, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		Reason:  "task_children_completed",
		Payload: agentInPayload("a1"),
	})
	if err != nil {
		t.Fatalf("queue first run: %v", err)
	}
	if first != runsservice.QueueOutcomeQueued {
		t.Fatalf("first outcome = %q, want %q", first, runsservice.QueueOutcomeQueued)
	}

	const waveKey = "task_children_completed:parent-2:bbbb"
	second, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		Reason:         "task_children_completed",
		WakeWaveKey:    waveKey,
		WakeWaveString: "parent-2|child-2",
		Payload:        agentInPayload("a1"),
	})
	if err != nil {
		t.Fatalf("queue second run: %v", err)
	}
	if second != runsservice.QueueOutcomeQueued {
		t.Fatalf("second outcome = %q, want %q (a wave-carrying request must not coalesce into a non-wave-carrying row)",
			second, runsservice.QueueOutcomeQueued)
	}

	var count int
	if err := repo.Reader().GetContext(ctx, &count,
		`SELECT COUNT(*) FROM runs WHERE agent_profile_id = 'a1' AND reason = 'task_children_completed'`,
	); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if count != 2 {
		t.Fatalf("runs for a1 = %d, want 2 (the wave-carrying request must persist as its own row)", count)
	}

	var persistedWaveKey string
	if err := repo.Reader().GetContext(ctx, &persistedWaveKey,
		`SELECT wake_wave_key FROM runs WHERE agent_profile_id = 'a1' AND reason = 'task_children_completed' AND wake_wave_key <> ''`,
	); err != nil {
		t.Fatalf("get persisted wave key: %v", err)
	}
	if persistedWaveKey != waveKey {
		t.Fatalf("persisted wake_wave_key = %q, want %q (second row's wave identity must survive uncoalesced)", persistedWaveKey, waveKey)
	}
}
