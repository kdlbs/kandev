package messagequeue

import (
	"context"
	"errors"
	"testing"
)

func TestSendNowClaimIsExactAtomicAndRestorable(t *testing.T) {
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
			ctx := context.Background()
			first := insertTestEntry(t, repo, "session-1", "task-1", "first", QueuedByUser, nil, nil)
			durable := insertTestEntry(t, repo, "session-1", "task-1", "durable", QueuedByWorkflow, nil,
				map[string]interface{}{MetadataLifecycleDurable: true})
			third := insertTestEntry(t, repo, "session-1", "task-1", "third", QueuedByUser, nil, nil)

			claim, err := repo.ClaimSendNow(ctx, "session-1", []string{third.ID, durable.ID, first.ID})
			if err != nil {
				t.Fatalf("claim: %v", err)
			}
			if got := []string{claim.Sources[0].ID, claim.Sources[1].ID, claim.Sources[2].ID}; got[0] != first.ID || got[1] != durable.ID || got[2] != third.ID {
				t.Fatalf("claim source order = %v", got)
			}
			if claim.Dispatch.Content != "first\n\ndurable\n\nthird" {
				t.Fatalf("dispatch content = %q", claim.Dispatch.Content)
			}

			pending, err := repo.ListBySession(ctx, "session-1")
			if err != nil {
				t.Fatalf("list after claim: %v", err)
			}
			if len(pending) != 1 || pending[0].ID != durable.ID || !pending[0].IsReservedInFlight() {
				t.Fatalf("pending after claim = %#v, want only reserved durable source", pending)
			}

			if err := repo.RestoreSendNowClaim(ctx, claim); err != nil {
				t.Fatalf("restore: %v", err)
			}
			restored, err := repo.ListBySession(ctx, "session-1")
			if err != nil {
				t.Fatalf("list after restore: %v", err)
			}
			if len(restored) != 3 || restored[0].ID != first.ID || restored[1].ID != durable.ID || restored[2].ID != third.ID {
				t.Fatalf("restored entries = %#v", restored)
			}
			if restored[1].IsReservedInFlight() {
				t.Fatal("restore left durable source reserved")
			}

			claim, err = repo.ClaimSendNow(ctx, "session-1", []string{first.ID, durable.ID, third.ID})
			if err != nil {
				t.Fatalf("claim before acknowledge: %v", err)
			}
			if err := repo.AcknowledgeSendNowClaim(ctx, claim); err != nil {
				t.Fatalf("acknowledge: %v", err)
			}
			remaining, err := repo.ListBySession(ctx, "session-1")
			if err != nil {
				t.Fatalf("list after acknowledge: %v", err)
			}
			if len(remaining) != 0 {
				t.Fatalf("remaining after acknowledge = %#v", remaining)
			}
		})
	}
}

func TestSendNowClaimRejectsMissingOrReservedSelectionWithoutMutation(t *testing.T) {
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
			ctx := context.Background()
			first := insertTestEntry(t, repo, "session-1", "task-1", "first", QueuedByUser, nil, nil)
			second := insertTestEntry(t, repo, "session-1", "task-1", "second", QueuedByUser, nil, nil)

			if _, err := repo.ClaimSendNow(ctx, "session-1", []string{first.ID, "missing"}); !errors.Is(err, ErrSendNowClaimChanged) {
				t.Fatalf("missing claim error = %v, want ErrSendNowClaimChanged", err)
			}
			entries, err := repo.ListBySession(ctx, "session-1")
			if err != nil {
				t.Fatalf("list after missing claim: %v", err)
			}
			if len(entries) != 2 || entries[0].ID != first.ID || entries[1].ID != second.ID {
				t.Fatalf("entries changed after missing claim = %#v", entries)
			}

			reserved := insertTestEntry(t, repo, "session-1", "task-1", "reserved", QueuedByWorkflow, nil,
				map[string]interface{}{MetadataLifecycleDurable: true})
			if _, err := repo.TakeHead(ctx, "session-1"); err != nil {
				t.Fatalf("take ordinary head: %v", err)
			}
			if _, err := repo.TakeHead(ctx, "session-1"); err != nil {
				t.Fatalf("take second ordinary head: %v", err)
			}
			after := insertTestEntry(t, repo, "session-1", "task-1", "after", QueuedByUser, nil, nil)
			if _, err := repo.ReserveHead(ctx, "session-1"); err != nil {
				t.Fatalf("reserve head: %v", err)
			}
			if _, err := repo.ClaimSendNow(ctx, "session-1", []string{after.ID, reserved.ID}); !errors.Is(err, ErrSendNowReservationConflict) {
				t.Fatalf("reserved claim error = %v, want ErrSendNowReservationConflict", err)
			}
			entries, err = repo.ListBySession(ctx, "session-1")
			if err != nil {
				t.Fatalf("list after reserved claim: %v", err)
			}
			if len(entries) != 2 || entries[0].ID != reserved.ID || entries[1].ID != after.ID || !entries[0].IsReservedInFlight() {
				t.Fatalf("entries changed after reserved claim = %#v", entries)
			}
		})
	}
}
