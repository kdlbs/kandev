package messagequeue

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func censusTestService(t *testing.T, repo Repository) *Service {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	svc := NewService(repo, DefaultMaxPerSession, log)
	svc.SetAutoMergeEnabled(false)
	return svc
}

func TestQueueCensusIsFIFOAndOmitsMessageBodies(t *testing.T) {
	svc := censusTestService(t, NewMemoryRepository())
	ctx := context.Background()
	first, err := svc.QueueMessageWithMetadata(ctx, "session-1", "task-1", "first secret body", "", QueuedByAgent, false, nil,
		map[string]interface{}{"origin": "inter-task", MetadataSenderTaskID: "sender-1"})
	if err != nil {
		t.Fatalf("queue first: %v", err)
	}
	second, err := svc.QueueMessageWithMetadata(ctx, "session-1", "task-1", "second secret body", "", QueuedByWorkflow, false, nil,
		map[string]interface{}{"origin": "workflow"})
	if err != nil {
		t.Fatalf("queue second: %v", err)
	}

	census, err := svc.Census(ctx, "session-1")
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	if census.BeforeCount != 2 || census.Max != DefaultMaxPerSession {
		t.Fatalf("counts = before:%d max:%d", census.BeforeCount, census.Max)
	}
	if len(census.Entries) != 2 || census.Entries[0].ID != first.ID || census.Entries[1].ID != second.ID {
		t.Fatalf("FIFO census = %#v", census.Entries)
	}
	if census.Entries[0].Claim == "" || census.Entries[0].ContentSHA256 == "" || census.Entries[0].ContentBytes != len("first secret body") {
		t.Fatalf("first safe descriptor = %#v", census.Entries[0])
	}
	if census.Entries[0].Origin != "inter-task" || census.Entries[0].SenderTaskID != "sender-1" {
		t.Fatalf("first provenance = %#v", census.Entries[0])
	}
}

func TestExactDispositionIsIdempotentAndPreservesChangedAndNewRows(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
		{name: "postgres", new: newTestPostgresRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			svc := censusTestService(t, repo)
			ctx := context.Background()
			first, err := svc.QueueMessage(ctx, "session-1", "task-1", "first", "", QueuedByUser, false, nil)
			if err != nil {
				t.Fatalf("queue first: %v", err)
			}
			second, err := svc.QueueMessage(ctx, "session-1", "task-1", "second", "", QueuedByUser, false, nil)
			if err != nil {
				t.Fatalf("queue second: %v", err)
			}
			census, err := svc.Census(ctx, "session-1")
			if err != nil {
				t.Fatalf("census: %v", err)
			}

			if err := repo.UpdateContent(ctx, "session-1", second.ID, "changed after census", nil, QueuedByUser); err != nil {
				t.Fatalf("change second: %v", err)
			}
			newArrival, err := svc.QueueMessage(ctx, "session-1", "task-1", "new arrival", "", QueuedByUser, false, nil)
			if err != nil {
				t.Fatalf("queue new arrival: %v", err)
			}

			result, err := svc.DisposeExact(ctx, "session-1", []QueueEntryClaim{
				{ID: first.ID, Claim: census.Entries[0].Claim},
				{ID: second.ID, Claim: census.Entries[1].Claim},
			})
			if err != nil {
				t.Fatalf("dispose: %v", err)
			}
			if result.BeforeCount != 3 || result.AfterCount != 2 {
				t.Fatalf("counts = before:%d after:%d", result.BeforeCount, result.AfterCount)
			}
			if len(result.Outcomes) != 2 || result.Outcomes[0].Status != QueueDispositionRemoved || result.Outcomes[1].Status != QueueDispositionChanged {
				t.Fatalf("outcomes = %#v", result.Outcomes)
			}

			retry, err := svc.DisposeExact(ctx, "session-1", []QueueEntryClaim{{ID: first.ID, Claim: census.Entries[0].Claim}})
			if err != nil {
				t.Fatalf("retry: %v", err)
			}
			if retry.BeforeCount != 2 || retry.AfterCount != 2 || retry.Outcomes[0].Status != QueueDispositionNotFound {
				t.Fatalf("retry = %#v", retry)
			}

			remaining, err := repo.ListBySession(ctx, "session-1")
			if err != nil {
				t.Fatalf("list remaining: %v", err)
			}
			if len(remaining) != 2 || remaining[0].ID != second.ID || remaining[1].ID != newArrival.ID {
				t.Fatalf("remaining = %#v", remaining)
			}
		})
	}
}

func TestConcurrentExactDispositionRemovesEntryOnce(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
		{name: "postgres", new: newTestPostgresRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := censusTestService(t, tt.new(t))
			ctx := context.Background()
			entry, err := svc.QueueMessage(ctx, "session-1", "task-1", "remove once", "", QueuedByAgent, false, nil)
			if err != nil {
				t.Fatalf("queue: %v", err)
			}
			census, err := svc.Census(ctx, "session-1")
			if err != nil {
				t.Fatalf("census: %v", err)
			}
			claim := QueueEntryClaim{ID: entry.ID, Claim: census.Entries[0].Claim}

			start := make(chan struct{})
			results := make(chan QueueDispositionStatus, 2)
			errs := make(chan error, 2)
			var wg sync.WaitGroup
			for range 2 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					result, disposeErr := svc.DisposeExact(ctx, "session-1", []QueueEntryClaim{claim})
					if disposeErr != nil {
						errs <- disposeErr
						return
					}
					results <- result.Outcomes[0].Status
				}()
			}
			close(start)
			wg.Wait()
			close(results)
			close(errs)
			for disposeErr := range errs {
				t.Fatalf("concurrent dispose: %v", disposeErr)
			}
			counts := map[QueueDispositionStatus]int{}
			for status := range results {
				counts[status]++
			}
			if counts[QueueDispositionRemoved] != 1 || counts[QueueDispositionNotFound] != 1 {
				t.Fatalf("concurrent outcomes = %#v", counts)
			}
		})
	}
}

func TestExactDispositionPreservesReservedLifecycleRows(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
		{name: "postgres", new: newTestPostgresRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			svc := censusTestService(t, repo)
			ctx := context.Background()
			entry, err := svc.QueueMessageWithMetadata(
				ctx, "session-1", "task-1", "durable lifecycle", "", QueuedByWorkflow, false, nil,
				map[string]interface{}{MetadataLifecycleDurable: true},
			)
			if err != nil {
				t.Fatalf("queue durable entry: %v", err)
			}
			census, err := svc.Census(ctx, "session-1")
			if err != nil {
				t.Fatalf("census: %v", err)
			}
			if _, err := repo.ReserveHead(ctx, "session-1"); err != nil {
				t.Fatalf("reserve durable entry: %v", err)
			}

			result, err := svc.DisposeExact(ctx, "session-1", []QueueEntryClaim{{ID: entry.ID, Claim: census.Entries[0].Claim}})
			if err != nil {
				t.Fatalf("dispose reserved entry: %v", err)
			}
			if result.BeforeCount != 0 || result.AfterCount != 0 || result.Outcomes[0].Status != QueueDispositionChanged {
				t.Fatalf("reserved disposition = %#v", result)
			}
			remaining, err := repo.ListBySession(ctx, "session-1")
			if err != nil {
				t.Fatalf("list reserved entry: %v", err)
			}
			if len(remaining) != 1 || remaining[0].ID != entry.ID || !remaining[0].IsReservedInFlight() {
				t.Fatalf("reserved row was not preserved: %#v", remaining)
			}
		})
	}
}

func TestIdenticalRoutineWakeCoalescingIsConcurrentRestartDurableAndCapacitySafe(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
		{name: "postgres", new: newTestPostgresRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			svc := censusTestService(t, repo)
			svc.SetMaxPerSession(3)
			ctx := context.Background()
			const routineKey = "routine-wake:automation-1:trigger-1:payload-a"
			metadata := map[string]interface{}{
				MetadataRoutineWake:     true,
				MetadataRoutineIdentity: "automation-1:trigger-1",
			}

			start := make(chan struct{})
			ids := make(chan string, 8)
			errs := make(chan error, 8)
			var wg sync.WaitGroup
			for range 8 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					entry, _, err := svc.QueueMessageWithCoalesceKey(
						ctx, "session-1", "task-1", "routine payload a", "", QueuedByAgent, false, nil,
						metadata, routineKey, true,
					)
					if err != nil {
						errs <- err
						return
					}
					ids <- entry.ID
				}()
			}
			close(start)
			wg.Wait()
			close(ids)
			close(errs)
			for err := range errs {
				t.Fatalf("queue identical routine wake: %v", err)
			}
			var retainedID string
			for id := range ids {
				if retainedID == "" {
					retainedID = id
				}
				if id != retainedID {
					t.Fatalf("identical routine wake IDs = %q and %q", retainedID, id)
				}
			}
			if status := svc.GetStatus(ctx, "session-1"); status.Count != 1 || status.Entries[0].ID != retainedID {
				t.Fatalf("coalesced status = %#v", status)
			}

			if _, err := svc.QueueMessage(ctx, "session-1", "task-1", "distinct peer", "", QueuedByAgent, false, nil); err != nil {
				t.Fatalf("queue peer: %v", err)
			}
			if _, _, err := svc.QueueMessageWithCoalesceKey(
				ctx, "session-1", "task-1", "routine payload b", "", QueuedByAgent, false, nil,
				metadata, "routine-wake:automation-1:trigger-1:payload-b", true,
			); err != nil {
				t.Fatalf("queue distinct routine: %v", err)
			}
			if status := svc.GetStatus(ctx, "session-1"); status.Count != 3 {
				t.Fatalf("distinct queue count = %d", status.Count)
			}

			restarted := censusTestService(t, repo)
			restarted.SetMaxPerSession(3)
			entry, replaced, err := restarted.QueueMessageWithCoalesceKey(
				ctx, "session-1", "task-1", "routine payload a", "", QueuedByAgent, false, nil,
				metadata, routineKey, true,
			)
			if err != nil || !replaced || entry.ID != retainedID {
				t.Fatalf("post-restart coalesce = entry:%#v replaced:%v err:%v", entry, replaced, err)
			}
			if _, err := restarted.QueueMessage(ctx, "session-1", "task-1", "capacity overflow", "", QueuedByAgent, false, nil); !errors.Is(err, ErrQueueFull) {
				t.Fatalf("capacity overflow error = %v, want ErrQueueFull", err)
			}
		})
	}
}
