package lifecycle

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/synctest"
	"time"
	"unicode/utf8"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.15
func TestStallDiagnosticBoundsAndExcludesPrivatePayloads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log, observed := observedLogger(t)
		sm := NewSessionManager(log, make(chan struct{}))
		const private = "private-prompt-and-token-sentinel"
		execution := &AgentExecution{
			ID:     strings.Repeat("x", 127) + "é",
			TaskID: string([]byte{0xff}), SessionID: "session",
			AgentID: "codex", AgentCommand: private, AgentArgs: []string{private},
			DeliveryMode:  DurableDeliveryMode(strings.Repeat(private, 200)),
			WorkspacePath: private,
			metadata:      map[string]interface{}{"prompt": private, "cli_version": private},
			promptDoneCh:  make(chan PromptCompletionSignal, 1),
		}
		tracker := &streamProjectionTracker{}
		progress := &streamProjectionProgress{ownerID: 1, tracker: tracker}
		tracker.install(progress.ownerID)
		for i := 0; i < 128; i++ {
			progress.enqueued()
		}
		progress.completed(64)
		execution.installStreamProgress(progress)
		generation := beginExecutionPrompt(execution)
		sm.markPromptDispatched(execution, generation)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			_, _ = sm.waitForPromptDone(ctx, execution, generation)
			close(done)
		}()
		defer func() { cancel(); <-done }()
		time.Sleep(7 * time.Minute)
		synctest.Wait()

		entries := observed.FilterMessage("agent stall detected: no events received").All()
		if len(entries) != 1 {
			t.Fatalf("stall records = %d, want 1", len(entries))
		}
		raw, err := json.Marshal(entries[0].ContextMap())
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > 4096 || strings.Contains(string(raw), private) {
			t.Fatal("stall diagnostic exceeds its bound or contains private payloads")
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		var snapshot map[string]interface{}
		if err := json.Unmarshal(record["stall"], &snapshot); err != nil {
			t.Fatalf("missing bounded stall snapshot: %v", err)
		}
		if snapshot["prompt_accepted"] != true || snapshot["output_observed"] != false ||
			snapshot["effect_observed"] != false {
			t.Fatalf("incorrect acceptance/output/effect snapshot: %#v", snapshot)
		}
		id, _ := snapshot["execution_id"].(string)
		if id != strings.Repeat("x", 127) || !utf8.ValidString(id) || len(id) > 128 {
			t.Fatalf("execution ID was not truncated at a rune boundary: %q", id)
		}
		if _, exists := snapshot["task_id"]; exists {
			t.Fatal("malformed UTF-8 must be omitted")
		}
	})
}

func TestStreamStageCountsAreUnknownWithoutFreshHeartbeat(t *testing.T) {
	stages := captureStreamStages(&AgentExecution{})
	if stages.AdapterSeenCount != -1 || stages.WriterSentCount != -1 {
		t.Fatalf("counts without a heartbeat = %d/%d, want -1/-1", stages.AdapterSeenCount, stages.WriterSentCount)
	}
	fresh := client.UpdateStreamStageSnapshot{HeartbeatAgeMillis: 10}
	stale := client.UpdateStreamStageSnapshot{HeartbeatAgeMillis: 30_001}
	if got := upstreamStageCount(fresh, 7); got != 7 {
		t.Fatalf("fresh heartbeat count = %d, want 7", got)
	}
	if got := upstreamStageCount(stale, 7); got != -1 {
		t.Fatalf("stale heartbeat count = %d, want -1", got)
	}
	if got := upstreamStageAge(stale, 10); got != -1 {
		t.Fatalf("stale heartbeat age = %d, want -1", got)
	}
	boundary := client.UpdateStreamStageSnapshot{HeartbeatAgeMillis: 30_000}
	if got := upstreamStageAge(boundary, 10); got != 30_010 {
		t.Fatalf("fresh heartbeat age = %d, want 30010", got)
	}
}

func TestCaptureStreamStagesUsesProjectionAdmissionCount(t *testing.T) {
	execution := &AgentExecution{}
	tracker := &streamProjectionTracker{}
	progress := &streamProjectionProgress{ownerID: 1, tracker: tracker}
	tracker.install(progress.ownerID)
	progress.enqueued()
	progress.enqueued()
	progress.enqueued()
	execution.streamProgress.Store(progress)

	got := captureStreamStages(execution)
	if got.EnqueueCount != 3 {
		t.Fatalf("projection enqueue count = %d, want successful queue admissions 3", got.EnqueueCount)
	}
}

func TestCaptureStreamStagesKeepsPredecessorWorkUnknown(t *testing.T) {
	execution := &AgentExecution{}
	tracker := &streamProjectionTracker{}
	predecessor := &streamProjectionProgress{ownerID: 1, tracker: tracker}
	execution.installStreamProgress(predecessor)
	predecessor.enqueued()
	predecessor.enqueued()
	execution.installStreamProgress(&streamProjectionProgress{ownerID: 2, tracker: tracker})

	got := captureStreamStages(execution)
	if got.ProjectionAvailable {
		t.Fatalf("projection state available with unresolved predecessor work: %+v", got)
	}
	predecessor.completed(2)
	got = captureStreamStages(execution)
	if !got.ProjectionAvailable || got.ProjectionPendingCount != 0 {
		t.Fatalf("projection state after predecessor settles = %+v, want available with no current pending work", got)
	}
}

func TestNewStreamEventProcessorDoesNotClaimConnectionOrderBeforeDial(t *testing.T) {
	processor := newStreamEventProcessor(nil, context.Background(), &AgentExecution{}, nil, nil, 0, nil)
	defer func() { _ = processor.close() }()
	if processor.progress.ownerID != 0 {
		t.Fatalf("pre-dial stream owner = %d, want no connection owner yet", processor.progress.ownerID)
	}
}

func TestInstallStreamProgressUsesAcceptedConnectionOrder(t *testing.T) {
	execution := &AgentExecution{}
	tracker := &streamProjectionTracker{}
	firstCreatedLastAccepted := &streamProjectionProgress{ownerID: 2, tracker: tracker}
	lastCreatedFirstAccepted := &streamProjectionProgress{ownerID: 1, tracker: tracker}

	execution.installStreamProgress(lastCreatedFirstAccepted)
	execution.installStreamProgress(firstCreatedLastAccepted)

	if got := execution.streamProgress.Load(); got != firstCreatedLastAccepted {
		t.Fatalf("installed progress owner = %+v, want the most recently accepted connection", got)
	}
}
