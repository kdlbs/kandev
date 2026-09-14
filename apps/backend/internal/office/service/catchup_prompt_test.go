package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/routines"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
)

// routineCatchUpReasons are the three reasons AC-OFFICE-ROUTINE-CATCHUP-002.5
// requires the gap to render under. Event is included specifically because
// PromoteRunAndCoalesceWakeupIfQueued can rewrite a cron-created run's
// reason to it after the gap was already measured and stored — a
// Cron-only gate would silently drop that gap.
var routineCatchUpReasons = []string{
	shared.RunReasonRoutineDispatchCron,
	shared.RunReasonRoutineDispatchEvent,
	shared.RunReasonRoutineDispatch,
}

// TestBuildPrompt_RoutineGap_RendersUnderEachRoutineReason covers
// AC-OFFICE-ROUTINE-CATCHUP-002.5: a run whose ContextSnapshot carries a
// measured gap renders a wake-context line under all three routine-dispatch
// reasons, including the count and the first-missed timestamp, and states
// the count is a lower bound when truncated.
func TestBuildPrompt_RoutineGap_RendersUnderEachRoutineReason(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	firstMissed := time.Date(2026, 5, 10, 11, 55, 0, 0, time.UTC)
	snapshot, err := routines.MarshalRoutinePayloadForTest("r-1", nil, 5, firstMissed, true)
	if err != nil {
		t.Fatalf("marshal routine payload: %v", err)
	}

	for _, reason := range routineCatchUpReasons {
		t.Run(reason, func(t *testing.T) {
			pc := service.BuildPromptContextWithSnapshotForTest(svc, ctx, reason, "{}", snapshot)
			if pc.MissedTicks != 5 {
				t.Fatalf("MissedTicks = %d, want 5", pc.MissedTicks)
			}
			if pc.MissedSince != "2026-05-10T11:55:00Z" {
				t.Errorf("MissedSince = %q, want 2026-05-10T11:55:00Z", pc.MissedSince)
			}
			if !pc.MissedTruncated {
				t.Error("MissedTruncated = false, want true")
			}

			prompt := service.BuildPrompt(pc)
			if !strings.Contains(prompt, "missed 5") {
				t.Errorf("prompt = %q, want it to state the missed-tick count", prompt)
			}
			if !strings.Contains(prompt, "tick") {
				t.Errorf("prompt = %q, want it to describe missed ticks, not runs (a tick never becomes a run)", prompt)
			}
			if strings.Contains(prompt, "run") {
				t.Errorf("prompt = %q, want no mention of a missed run — only one run is ever dispatched", prompt)
			}
			if !strings.Contains(prompt, "2026-05-10T11:55:00Z") {
				t.Errorf("prompt = %q, want it to state the first-missed timestamp", prompt)
			}
			if !strings.Contains(prompt, "lower bound") {
				t.Errorf("prompt = %q, want it to state the count is a lower bound (truncated)", prompt)
			}
		})
	}
}

// TestBuildPrompt_RoutineGap_NoSummaryRendersNoStatement covers AC-002.3:
// when the ContextSnapshot carries no gap (a merely-late tick, or a policy
// that opts out), the run's reason still gates on being a routine reason,
// but no missed-tick statement is rendered at all.
func TestBuildPrompt_RoutineGap_NoSummaryRendersNoStatement(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	pc := service.BuildPromptContextWithSnapshotForTest(
		svc, ctx, shared.RunReasonRoutineDispatchCron, "{}", `{"routine_id":"r-1"}`,
	)
	if pc.MissedTicks != 0 {
		t.Fatalf("MissedTicks = %d, want 0", pc.MissedTicks)
	}
	prompt := service.BuildPrompt(pc)
	if strings.Contains(prompt, "missed") {
		t.Errorf("prompt = %q, want no missed-tick statement", prompt)
	}
}

// TestBuildPrompt_RoutineGap_LegacyPayloadWithoutMissedSinceIsIgnored covers
// the upgrade path: a wakeup request queued by a pre-catch-up build wrote
// only {"routine_id":...,"missed_ticks":N} — missed_since did not exist yet
// — and can sit undispatched across a deploy. Decoding that legacy shape
// must not populate the catch-up fields from an incomplete payload, or the
// rendered prompt states a tick count with no timestamp ("since .").
func TestBuildPrompt_RoutineGap_LegacyPayloadWithoutMissedSinceIsIgnored(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	legacySnapshot := `{"routine_id":"r1","missed_ticks":5}`
	pc := service.BuildPromptContextWithSnapshotForTest(svc, ctx, shared.RunReasonRoutineDispatchCron, "{}", legacySnapshot)
	if pc.MissedTicks != 0 {
		t.Fatalf("MissedTicks = %d, want 0 (a legacy payload with no missed_since must not populate the gap fields)", pc.MissedTicks)
	}
	if pc.MissedSince != "" {
		t.Errorf("MissedSince = %q, want empty", pc.MissedSince)
	}

	prompt := service.BuildPrompt(pc)
	if strings.Contains(prompt, "missed") {
		t.Errorf("prompt = %q, want no missed-tick statement for a legacy payload missing missed_since", prompt)
	}
	if strings.Contains(prompt, "since .") {
		t.Errorf("prompt = %q, must never render a malformed empty timestamp", prompt)
	}
}

// TestBuildPrompt_RoutineGap_NonRoutineReasonIgnoresSnapshot proves the
// decode is gated on the run's reason: a non-routine reason with the exact
// same context_snapshot renders nothing, keeping every other run's prompt
// byte-identical to before this parameter existed.
func TestBuildPrompt_RoutineGap_NonRoutineReasonIgnoresSnapshot(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	snapshot, err := routines.MarshalRoutinePayloadForTest(
		"r-1", nil, 5, time.Date(2026, 5, 10, 11, 55, 0, 0, time.UTC), true,
	)
	if err != nil {
		t.Fatalf("marshal routine payload: %v", err)
	}
	pc := service.BuildPromptContextWithSnapshotForTest(svc, ctx, service.RunReasonHeartbeat, "{}", snapshot)
	if pc.MissedTicks != 0 {
		t.Fatalf("MissedTicks = %d, want 0 (non-routine reason must not decode the snapshot)", pc.MissedTicks)
	}
	prompt := service.BuildPrompt(pc)
	if strings.Contains(prompt, "missed 5") {
		t.Errorf("prompt = %q, want no missed-tick statement for a non-routine reason", prompt)
	}
}
