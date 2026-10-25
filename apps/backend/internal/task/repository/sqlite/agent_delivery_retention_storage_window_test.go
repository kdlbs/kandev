package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionStorageRecentWindow(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, events := seedRetentionBoundsStream(t, engine, "recent-window", 1, 0)
			event := *events[0]
			now := event.ReceivedAt
			repo.clockNow = func() time.Time { return now }
			assertRetentionStorageToolPolicy(t, repo)
			prepareRetentionWindowSubmission(t, repo, &event)
			const count = 10_000
			seedRetentionStorageRows(t, repo, &event, count, 1024)
			before := retentionCanonicalContent(t, repo, event.SessionID)
			now = event.ReceivedAt.Add(30 * time.Minute)
			sweepRetentionStorageWindow(t, repo, count)
			raw := measureRetentionStorage(t, repo)
			if raw.InboxRows != count || raw.CompactRows != 0 || raw.RawPayloadBytes == 0 {
				t.Fatalf("raw grace: %+v", raw)
			}
			now = event.ReceivedAt.Add(2 * time.Hour)
			sweepRetentionStorageWindow(t, repo, count)
			compact := measureRetentionStorage(t, repo)
			if compact.InboxRows != count || compact.CompactRows != count || compact.RawPayloadBytes != 0 || compact.GenericEffects != count {
				t.Fatalf("recent compact window: %+v", compact)
			}
			if got := retentionCanonicalContent(t, repo, event.SessionID); got != before {
				t.Fatal("active prompt compaction changed saved output")
			}
			prompt, err := repo.GetAgentDeliverySubmission(context.Background(), event.SubmissionID)
			if err != nil || len(prompt.Payload) == 0 || prompt.FullySettledAt != nil {
				t.Fatalf("active prompt lost protected payload: %+v %v", prompt, err)
			}
			event.Sequence = count + 1
			event.ContentHash, event.HashVersion, event.PayloadBytes = "", 0, 0
			event.ProjectedAt, event.PayloadPrunedAt = nil, nil
			event.ReceivedAt = now
			event.Payload, _ = json.Marshal(streams.AgentEvent{Type: event.EventType, Text: "next saved output"})
			if inserted, err := repo.ReceiveAgentDeliveryEvent(context.Background(), &event, event.Sequence); err != nil || !inserted {
				t.Fatalf("next event: %t %v", inserted, err)
			}
			if _, err := repo.ProjectCanonicalAgentDeliveryEvent(context.Background(), &event, completedRetentionEventEffect(&event)); err != nil {
				t.Fatal(err)
			}
			sweepRetentionStorageWindow(t, repo, count)
			cursor, err := repo.GetAgentDeliveryCursor(context.Background(), event.StreamID)
			if err != nil || cursor.RetiredSequence != 1 {
				t.Fatalf("recent count eviction: %+v %v", cursor, err)
			}
			final := measureRetentionStorage(t, repo)
			if final.InboxRows != count || final.CompactRows != count-1 || final.RawPayloadBytes == 0 {
				t.Fatalf("recent window after new output: %+v", final)
			}
			t.Logf("engine=%s raw_grace=%+v compact_window=%+v after_new_output=%+v", engine, raw, compact, final)
		})
	}
}

func sweepRetentionStorageWindow(t *testing.T, repo *Repository, count int) {
	t.Helper()
	for batch := 0; batch < count/100+2; batch++ {
		runRetentionStorageBatch(t, repo, agentDeliveryRetentionCandidateLimit)
	}
}

func prepareRetentionWindowSubmission(t *testing.T, repo *Repository, event *models.AgentDeliveryEvent) {
	t.Helper()
	ctx := context.Background()
	if err := repo.db.QueryRow(repo.db.Rebind(`SELECT task_id FROM task_sessions WHERE id=?`), event.SessionID).Scan(&event.TaskID); err != nil {
		t.Fatal(err)
	}
	event.ExecutionID = "retention-window-execution"
	event.OwnerGeneration = 1
	event.ExpectedStreamID = event.StreamID
	event.SubmissionID = "prompt:retention-window"
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "retention-window-running", SessionID: event.SessionID, TaskID: event.TaskID, ExecutorID: "retention-window-executor", AgentExecutionID: event.ExecutionID, Status: "running"}); err != nil {
		t.Fatal(err)
	}
	now := repo.nowUTC()
	if stored, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: event.SessionID, IncarnationID: event.IncarnationID, Generation: 1, NativeSessionID: "native-retention-window", CreationReason: "initial", CreatedAt: now, CommittedAt: now}, 0); err != nil || !stored {
		t.Fatalf("window generation: %t %v", stored, err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: event.SubmissionID, SessionID: event.SessionID, IncarnationID: event.IncarnationID, HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "window-prompt-hash", Payload: []byte("active window prompt"), State: models.DeliverySubmissionDispatching}); err != nil {
		t.Fatal(err)
	}
}
