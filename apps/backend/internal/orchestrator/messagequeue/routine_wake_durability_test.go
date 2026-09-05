package messagequeue

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestRoutineWakeClaimPreservesOneSuccessorAndReceipt(t *testing.T) {
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
			svc.SetMaxPerSession(1)
			ctx := context.Background()
			metadata := canonicalRoutineMetadata("fence-7", "dirty-1")

			first, replaced, err := svc.QueueMessageWithCoalesceKey(
				ctx, "session-1", "task-1", "WAKE:CYCLE", "", QueuedByAgent,
				false, nil, metadata, "routine-wake:key-1", true,
			)
			if err != nil || replaced {
				t.Fatalf("queue first = (%#v, %t, %v)", first, replaced, err)
			}
			firstReceipt := RoutineWakeReceiptFromMessage(first)
			if firstReceipt == nil || firstReceipt.CanonicalEntryID != first.ID || firstReceipt.AbsorbedCount != 0 {
				t.Fatalf("first receipt = %#v", firstReceipt)
			}

			claimed, exists := svc.ReserveQueued(ctx, "session-1")
			if !exists || claimed == nil || !claimed.IsReservedRoutineWakeDelivery() {
				t.Fatalf("claimed routine = %#v, exists=%t", claimed, exists)
			}
			if got := svc.GetStatus(ctx, "session-1").Count; got != 0 {
				t.Fatalf("visible count while claimed = %d, want 0", got)
			}

			second, replaced, err := svc.QueueMessageWithCoalesceKey(
				ctx, "session-1", "task-1", "WAKE:CYCLE", "", QueuedByAgent,
				false, nil, canonicalRoutineMetadata("fence-7", "dirty-2"),
				"routine-wake:key-1", true,
			)
			if err != nil || replaced {
				t.Fatalf("queue successor at capacity = (%#v, %t, %v)", second, replaced, err)
			}
			if second.ID == first.ID {
				t.Fatal("claimed canonical entry was overwritten instead of preserving a successor")
			}

			third, replaced, err := svc.QueueMessageWithCoalesceKey(
				ctx, "session-1", "task-1", "WAKE:CYCLE", "", QueuedByAgent,
				false, nil, canonicalRoutineMetadata("fence-7", "dirty-3"),
				"routine-wake:key-1", true,
			)
			if err != nil || !replaced {
				t.Fatalf("coalesce successor = (%#v, %t, %v)", third, replaced, err)
			}
			if third.ID != second.ID {
				t.Fatalf("successor identity changed: %s -> %s", second.ID, third.ID)
			}
			receipt := RoutineWakeReceiptFromMessage(third)
			if receipt == nil || receipt.AbsorbedCount != 1 || len(receipt.AbsorbedSources) != 1 {
				t.Fatalf("successor receipt = %#v", receipt)
			}
			if receipt.AbsorbedSources[0].ID == "" || receipt.AbsorbedSources[0].QueuedAt == "" {
				t.Fatalf("absorbed source receipt leaks or omits identity: %#v", receipt.AbsorbedSources[0])
			}
			if receipt.LeaderFencingToken != "fence-7" || receipt.DirtyGeneration != "dirty-3" || !receipt.PostRunRequeue {
				t.Fatalf("successor execution receipt = %#v", receipt)
			}

			if err := svc.AcknowledgeQueued(ctx, "session-1", claimed.ID); err != nil {
				t.Fatalf("acknowledge claimed wake: %v", err)
			}
			status := svc.GetStatus(ctx, "session-1")
			if status.Count != 1 || status.Entries[0].ID != second.ID {
				t.Fatalf("successor after acknowledgement = %#v", status.Entries)
			}
		})
	}
}

func TestRoutineWakeTransferPreservesCanonicalCoalescing(t *testing.T) {
	for _, tt := range []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
		{name: "postgres", new: newTestPostgresRepo},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := censusTestService(t, tt.new(t))
			ctx := context.Background()
			first, _, err := svc.QueueMessageWithCoalesceKey(
				ctx, "old-session", "task-1", "WAKE:CYCLE", "", QueuedByAgent,
				false, nil, canonicalRoutineMetadata("fence-1", "dirty-1"),
				"routine-wake:key-1", true,
			)
			if err != nil {
				t.Fatalf("queue first: %v", err)
			}
			if err := svc.TransferSession(ctx, "old-session", "new-session"); err != nil {
				t.Fatalf("transfer: %v", err)
			}
			after, replaced, err := svc.QueueMessageWithCoalesceKey(
				ctx, "new-session", "task-1", "WAKE:CYCLE", "", QueuedByAgent,
				false, nil, canonicalRoutineMetadata("fence-1", "dirty-2"),
				"routine-wake:key-1", true,
			)
			if err != nil || !replaced || after.ID != first.ID {
				t.Fatalf("coalesce after transfer = (%#v, %t, %v), first=%s", after, replaced, err, first.ID)
			}
		})
	}
}

func TestRoutineWakeCoalescingRecordsSuppressedScanMetric(t *testing.T) {
	svc := censusTestService(t, NewMemoryRepository())
	ctx := context.Background()
	before := routineWakeFullBoardScansSuppressed.Value()
	for _, dirty := range []string{"dirty-1", "dirty-2"} {
		_, _, err := svc.QueueMessageWithCoalesceKey(
			ctx, "session-1", "task-1", "WAKE:CYCLE", "", QueuedByAgent,
			false, nil, canonicalRoutineMetadata("fence-1", dirty),
			"routine-wake:key-1", true,
		)
		if err != nil {
			t.Fatalf("queue %s: %v", dirty, err)
		}
	}
	if got := routineWakeFullBoardScansSuppressed.Value(); got != before+1 {
		t.Fatalf("suppressed scan metric = %d, want %d", got, before+1)
	}
}

func TestRoutineWakeBurstRetainsOneCanonicalEntry(t *testing.T) {
	for _, tt := range []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
		{name: "postgres", new: newTestPostgresRepo},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := censusTestService(t, tt.new(t))
			ctx := context.Background()
			const arrivals = 24
			var wg sync.WaitGroup
			errs := make(chan error, arrivals)
			for i := 0; i < arrivals; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, _, err := svc.QueueMessageWithCoalesceKey(
						ctx, "session-1", "task-1", "WAKE:CYCLE", "", QueuedByAgent,
						false, nil, canonicalRoutineMetadata("fence-1", "dirty-burst"),
						"routine-wake:key-1", true,
					)
					errs <- err
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatalf("burst admission: %v", err)
				}
			}
			status := svc.GetStatus(ctx, "session-1")
			if status.Count != 1 {
				t.Fatalf("burst queue count = %d, want 1", status.Count)
			}
			receipt := RoutineWakeReceiptFromMessage(&status.Entries[0])
			if receipt == nil || receipt.AbsorbedCount != arrivals-1 {
				t.Fatalf("burst receipt = %#v", receipt)
			}
		})
	}
}

func TestRoutineWakeReservationSurvivesSQLiteRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "queue.db")
	open := func() (*sqlx.DB, Repository) {
		raw, err := sql.Open("sqlite3", path)
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		raw.SetMaxOpenConns(1)
		raw.SetMaxIdleConns(1)
		db := sqlx.NewDb(raw, "sqlite3")
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS task_sessions (id TEXT PRIMARY KEY)`); err != nil {
			t.Fatalf("create task sessions: %v", err)
		}
		repo, err := NewSQLiteRepository(db, db)
		if err != nil {
			t.Fatalf("new repository: %v", err)
		}
		return db, repo
	}

	db1, repo1 := open()
	svc1 := censusTestService(t, repo1)
	first, _, err := svc1.QueueMessageWithCoalesceKey(
		ctx, "session-1", "task-1", "WAKE:CYCLE", "", QueuedByAgent,
		false, nil, canonicalRoutineMetadata("fence-1", "dirty-1"),
		"routine-wake:key-1", true,
	)
	if err != nil {
		t.Fatalf("queue first: %v", err)
	}
	claimed, exists := svc1.ReserveQueued(ctx, "session-1")
	if !exists || claimed == nil || claimed.ID != first.ID {
		t.Fatalf("reserve before restart = %#v, exists=%t", claimed, exists)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close first database: %v", err)
	}

	db2, repo2 := open()
	defer func() { _ = db2.Close() }()
	svc2 := censusTestService(t, repo2)
	svc2.SetMaxPerSession(1)
	entries, err := repo2.ListBySession(ctx, "session-1")
	if err != nil || len(entries) != 1 || !entries[0].IsReservedInFlight() {
		t.Fatalf("reserved row after restart = %#v, err=%v", entries, err)
	}
	successor, replaced, err := svc2.QueueMessageWithCoalesceKey(
		ctx, "session-1", "task-1", "WAKE:CYCLE", "", QueuedByAgent,
		false, nil, canonicalRoutineMetadata("fence-1", "dirty-2"),
		"routine-wake:key-1", true,
	)
	if err != nil || replaced || successor.ID == first.ID {
		t.Fatalf("successor after restart = (%#v, %t, %v)", successor, replaced, err)
	}
	if err := svc2.AcknowledgeQueued(ctx, "session-1", first.ID); err != nil {
		t.Fatalf("acknowledge recovered reservation: %v", err)
	}
	status := svc2.GetStatus(ctx, "session-1")
	if status.Count != 1 || status.Entries[0].ID != successor.ID {
		t.Fatalf("restart successor = %#v", status.Entries)
	}
}

func canonicalRoutineMetadata(fence, dirty string) map[string]interface{} {
	return map[string]interface{}{
		MetadataRoutineWake:               true,
		MetadataRoutineIdentity:           "routine:key-1",
		MetadataRoutineWorkspaceID:        "workspace-1",
		MetadataRoutineType:               "cycle",
		MetadataRoutineName:               "heartbeat",
		MetadataRoutinePolicyGeneration:   "policy-v7",
		MetadataRoutineScopeGeneration:    "board-v3",
		MetadataRoutineLeaderFencingToken: fence,
		MetadataRoutineDirtyGeneration:    dirty,
	}
}
