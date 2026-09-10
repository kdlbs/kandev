package messagequeue

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionGenerationAdvancesForDestructiveQueueChanges(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			ctx := context.Background()
			if got, err := repo.SessionGeneration(ctx, "session-1"); err != nil || got != 0 {
				t.Fatalf("initial session generation = %d err=%v, want 0", got, err)
			}
			insertTestEntry(t, repo, "session-1", "task-1", "pending", QueuedByUser, nil, nil)
			if _, err := repo.DeleteAllBySession(ctx, "session-1"); err != nil {
				t.Fatalf("delete session queue: %v", err)
			}
			if got, err := repo.SessionGeneration(ctx, "session-1"); err != nil || got != 1 {
				t.Fatalf("session generation after delete = %d err=%v, want 1", got, err)
			}
			if _, err := repo.PurgeSession(ctx, "session-1"); err != nil {
				t.Fatalf("purge session queue: %v", err)
			}
			if got, err := repo.SessionGeneration(ctx, "session-1"); err != nil || got != 2 {
				t.Fatalf("session generation after purge = %d err=%v, want 2", got, err)
			}
		})
	}
}

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
			clickSnapshot, err := repo.ListBySession(ctx, "session-1")
			if err != nil {
				t.Fatalf("list click-time snapshot: %v", err)
			}
			if len(clickSnapshot) != 3 {
				t.Fatalf("click-time snapshot = %#v, want three entries", clickSnapshot)
			}

			claim, err := repo.ClaimSendNow(ctx, "session-1", clickSnapshot)
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

			claim, err = repo.ClaimSendNow(ctx, "session-1", clickSnapshot)
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

func TestIdentityBoundSendNowClaimRejectsRecreatedSession(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			ctx := context.Background()
			first := QueueSessionIdentity{
				TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "incarnation-1",
			}
			replacement := QueueSessionIdentity{
				TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "incarnation-2",
			}
			seedQueueSessionIdentity(t, repo, first)
			source := &QueuedMessage{
				SessionID: first.SessionID, TaskID: first.TaskID, Content: "old prompt", QueuedBy: QueuedByUser,
			}
			if err := repo.InsertForSession(ctx, first, source, 0); err != nil {
				t.Fatalf("insert old source: %v", err)
			}
			claim, err := repo.ClaimSendNowForSession(ctx, first, []QueuedMessage{*source})
			if err != nil {
				t.Fatalf("claim old source: %v", err)
			}
			seedQueueSessionIdentity(t, repo, replacement)

			if err := repo.RestoreSendNowClaim(ctx, claim); !errors.Is(err, ErrSessionIdentityMismatch) {
				t.Fatalf("restore old claim error = %v, want ErrSessionIdentityMismatch", err)
			}
			if err := repo.AcknowledgeSendNowClaim(ctx, claim); !errors.Is(err, ErrSessionIdentityMismatch) {
				t.Fatalf("acknowledge old claim error = %v, want ErrSessionIdentityMismatch", err)
			}
			entries, err := repo.ListBySession(ctx, replacement.SessionID)
			if err != nil {
				t.Fatalf("list replacement queue: %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("old claim mutated replacement queue: %+v", entries)
			}
		})
	}
}
func TestIdentityBoundSendNowClaimPersistsDurableReservationIncarnation(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			ctx := context.Background()
			identity := QueueSessionIdentity{
				TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "incarnation-1",
			}
			seedQueueSessionIdentity(t, repo, identity)
			source := &QueuedMessage{
				SessionID: identity.SessionID,
				TaskID:    identity.TaskID,
				Content:   "durable",
				QueuedBy:  QueuedByWorkflow,
				Metadata:  map[string]interface{}{MetadataLifecycleDurable: true},
			}
			if err := repo.InsertForSession(ctx, identity, source, 0); err != nil {
				t.Fatalf("insert durable source: %v", err)
			}
			if _, err := repo.ClaimSendNowForSession(ctx, identity, []QueuedMessage{*source}); err != nil {
				t.Fatalf("claim durable source: %v", err)
			}
			entries, err := repo.ListBySession(ctx, identity.SessionID)
			if err != nil {
				t.Fatalf("list reserved source: %v", err)
			}
			if len(entries) != 1 {
				t.Fatalf("reserved entries = %d, want 1", len(entries))
			}
			if got := lifecycleReservationIncarnation(entries[0].Metadata); got != identity.SessionIncarnationID {
				t.Fatalf("reservation incarnation = %q, want %q", got, identity.SessionIncarnationID)
			}
		})
	}
}

func TestIdentityBoundPostClaimMutationsRejectRecreatedSession(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			ctx := context.Background()
			first := QueueSessionIdentity{SessionID: "session-1", TaskID: "task-1", SessionIncarnationID: "incarnation-1"}
			replacement := first
			replacement.SessionIncarnationID = "incarnation-2"
			seedQueueSessionIdentity(t, repo, first)

			reserved := &QueuedMessage{
				SessionID: first.SessionID,
				TaskID:    first.TaskID,
				Content:   "old lifecycle prompt",
				QueuedBy:  QueuedByWorkflow,
				Metadata: map[string]interface{}{
					MetadataLifecycleDurable:    true,
					MetadataLifecycleGeneration: int64(0),
					MetadataCoalesceKey:         "old-lifecycle",
				},
			}
			if err := repo.InsertForSession(ctx, first, reserved, 0); err != nil {
				t.Fatalf("insert reserved source: %v", err)
			}
			claimed, enabled, err := repo.ReserveHeadIfAutoRunForSession(ctx, first)
			if err != nil {
				t.Fatalf("reserve source: %v", err)
			}
			if !enabled {
				t.Fatal("expected Auto-run enabled")
			}
			if claimed == nil {
				t.Fatal("expected reserved source")
			}
			seedQueueSessionIdentity(t, repo, replacement)

			if err := repo.AcknowledgeByIDForSession(ctx, first, claimed.ID); !errors.Is(err, ErrSessionIdentityMismatch) {
				t.Fatalf("acknowledge error = %v, want ErrSessionIdentityMismatch", err)
			}
			ordinary := &QueuedMessage{
				ID:        "restored-old-entry",
				SessionID: first.SessionID,
				TaskID:    first.TaskID,
				Content:   "old ordinary prompt",
				QueuedBy:  QueuedByUser,
			}
			if err := repo.RestoreForSession(ctx, first, ordinary, 0); !errors.Is(err, ErrSessionIdentityMismatch) {
				t.Fatalf("restore error = %v, want ErrSessionIdentityMismatch", err)
			}
			if err := repo.RequeuePreservingFIFOForSession(ctx, first, ordinary); !errors.Is(err, ErrSessionIdentityMismatch) {
				t.Fatalf("FIFO requeue error = %v, want ErrSessionIdentityMismatch", err)
			}
			retry := &QueuedMessage{
				SessionID: first.SessionID,
				TaskID:    first.TaskID,
				Content:   "old lifecycle retry",
				QueuedBy:  QueuedByWorkflow,
				Metadata: map[string]interface{}{
					MetadataLifecycleGeneration: int64(0),
				},
			}
			if _, _, err := repo.InsertOrReplaceLifecycleByCoalesceKeyForSession(
				ctx, first, retry, "old-lifecycle", 0, true,
			); !errors.Is(err, ErrSessionIdentityMismatch) {
				t.Fatalf("lifecycle requeue error = %v, want ErrSessionIdentityMismatch", err)
			}

			entries, err := repo.ListBySession(ctx, replacement.SessionID)
			if err != nil {
				t.Fatalf("list replacement queue: %v", err)
			}
			if len(entries) != 1 || entries[0].ID != claimed.ID || !entries[0].IsReservedInFlight() {
				t.Fatalf("post-claim mutation changed replacement queue: %+v", entries)
			}
			if got := lifecycleReservationIncarnation(entries[0].Metadata); got != first.SessionIncarnationID {
				t.Fatalf("reservation incarnation = %q, want %q", got, first.SessionIncarnationID)
			}
		})
	}
}

func TestSendNowClaimGenerationRejectsSupersededRestore(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			ctx := context.Background()
			identity := QueueSessionIdentity{
				TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "incarnation-1",
			}
			seedQueueSessionIdentity(t, repo, identity)
			source := &QueuedMessage{
				SessionID: identity.SessionID, TaskID: identity.TaskID, Content: "prompt", QueuedBy: QueuedByUser,
			}
			if err := repo.InsertForSession(ctx, identity, source, 0); err != nil {
				t.Fatalf("insert source: %v", err)
			}
			first, err := repo.ClaimSendNowForSession(ctx, identity, []QueuedMessage{*source})
			if err != nil {
				t.Fatalf("first claim: %v", err)
			}
			if err := repo.RestoreSendNowClaim(ctx, first); err != nil {
				t.Fatalf("restore first claim: %v", err)
			}
			second, err := repo.ClaimSendNowForSession(ctx, identity, []QueuedMessage{*source})
			if err != nil {
				t.Fatalf("second claim: %v", err)
			}

			if err := repo.RestoreSendNowClaim(ctx, first); !errors.Is(err, ErrSendNowClaimChanged) {
				t.Fatalf("superseded restore error = %v, want ErrSendNowClaimChanged", err)
			}
			if err := repo.RestoreSendNowClaim(ctx, second); err != nil {
				t.Fatalf("restore current claim: %v", err)
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

			if _, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*first, {ID: "missing"}}); !errors.Is(err, ErrSendNowClaimChanged) {
				t.Fatalf("missing claim error = %v, want ErrSendNowClaimChanged", err)
			}
			entries, err := repo.ListBySession(ctx, "session-1")
			if err != nil {
				t.Fatalf("list after missing claim: %v", err)
			}
			if len(entries) != 2 || entries[0].ID != first.ID || entries[1].ID != second.ID {
				t.Fatalf("entries changed after missing claim = %#v", entries)
			}

			snapshot := *first
			if err := repo.UpdateContentAndMetadata(ctx, "session-1", first.ID, "edited", nil,
				map[string]interface{}{"snapshot": "edited"}, QueuedByUser); err != nil {
				t.Fatalf("edit queued snapshot: %v", err)
			}
			if _, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{snapshot}); !errors.Is(err, ErrSendNowClaimChanged) {
				t.Fatalf("changed snapshot error = %v, want ErrSendNowClaimChanged", err)
			}
			entries, err = repo.ListBySession(ctx, "session-1")
			if err != nil {
				t.Fatalf("list after changed snapshot: %v", err)
			}
			if len(entries) != 2 || entries[0].ID != first.ID || entries[0].Content != "edited" || entries[1].ID != second.ID {
				t.Fatalf("entries changed after changed snapshot = %#v", entries)
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
			if _, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*reserved, *after}); !errors.Is(err, ErrSendNowReservationConflict) {
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

func TestSameQueuedMessageContentIncludesStoredSnapshot(t *testing.T) {
	base := &QueuedMessage{
		ID:        "entry-1",
		SessionID: "session-1",
		TaskID:    "task-1",
		Position:  1,
		Content:   "original",
		Model:     "model-a",
		PlanMode:  true,
		Metadata:  map[string]interface{}{"source": "original"},
		QueuedBy:  QueuedByUser,
	}

	changedMetadata := *base
	changedMetadata.Metadata = map[string]interface{}{"source": "edited"}
	if sameQueuedMessageContent(base, &changedMetadata) {
		t.Fatal("metadata edit matched the click-time queue snapshot")
	}

	changedModel := *base
	changedModel.Model = "model-b"
	if sameQueuedMessageContent(base, &changedModel) {
		t.Fatal("model edit matched the click-time queue snapshot")
	}
}

func TestSendNowRestoreDiscardsSourcesFromPurgedTaskGeneration(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			ctx := context.Background()
			entry := insertTestEntry(t, repo, "session-1", "task-1", "discard after purge", QueuedByUser, nil, nil)
			claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*entry})
			if err != nil {
				t.Fatalf("claim: %v", err)
			}
			if got := claim.SourceGenerations["task-1"]; got != 0 {
				t.Fatalf("claim generation = %d, want initial generation 0", got)
			}

			if _, err := repo.PurgeTask(ctx, "task-1"); err != nil {
				t.Fatalf("purge task: %v", err)
			}
			err = repo.RestoreSendNowClaim(ctx, claim)
			if err != nil {
				t.Fatalf("restore after purge: %v", err)
			}
			entries, err := repo.ListBySession(ctx, "session-1")
			if err != nil {
				t.Fatalf("list after restore: %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("restored entries after task purge = %#v, want none", entries)
			}
		})
	}
}
func TestSendNowRestoreDoesNotResurrectSourcesFromPurgedSession(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			ctx := context.Background()
			entry := insertTestEntry(t, repo, "session-1", "task-1", "discard after session purge", QueuedByUser, nil, nil)
			claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*entry})
			if err != nil {
				t.Fatalf("claim: %v", err)
			}
			if _, err := repo.PurgeSession(ctx, "session-1"); err != nil {
				t.Fatalf("purge session: %v", err)
			}
			if err := repo.RestoreSendNowClaim(ctx, claim); !errors.Is(err, ErrSendNowClaimChanged) {
				t.Fatalf("restore after session purge error = %v, want ErrSendNowClaimChanged", err)
			}
			entries, err := repo.ListBySession(ctx, "session-1")
			if err != nil {
				t.Fatalf("list after restore: %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("restored entries after session purge = %#v, want none", entries)
			}
		})
	}
}

func TestSendNowRestoreFencesSessionTransferAndReplacement(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("transfer", func(t *testing.T) {
				repo := tt.new(t)
				ctx := context.Background()
				entry := insertTestEntry(t, repo, "session-1", "task-1", "transferred", QueuedByUser, nil, nil)
				claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*entry})
				if err != nil {
					t.Fatalf("claim: %v", err)
				}
				if err := repo.TransferSession(ctx, "session-1", "session-2"); err != nil {
					t.Fatalf("transfer: %v", err)
				}
				if err := repo.RestoreSendNowClaim(ctx, claim); !errors.Is(err, ErrSendNowClaimChanged) {
					t.Fatalf("restore after transfer error = %v, want ErrSendNowClaimChanged", err)
				}
				entries, err := repo.ListBySession(ctx, "session-2")
				if err != nil {
					t.Fatalf("list transferred entries: %v", err)
				}
				if len(entries) != 0 {
					t.Fatalf("transferred entries = %#v, want none", entries)
				}
			})

			t.Run("replacement", func(t *testing.T) {
				repo := tt.new(t)
				ctx := context.Background()
				entry := insertTestEntry(t, repo, "session-1", "task-1", "replaced", QueuedByUser, nil, nil)
				claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*entry})
				if err != nil {
					t.Fatalf("claim: %v", err)
				}
				if err := repo.ReplaceSession(ctx, "session-1", nil, nil); err != nil {
					t.Fatalf("replace: %v", err)
				}
				if err := repo.RestoreSendNowClaim(ctx, claim); !errors.Is(err, ErrSendNowClaimChanged) {
					t.Fatalf("restore after replacement error = %v, want ErrSendNowClaimChanged", err)
				}
				entries, err := repo.ListBySession(ctx, "session-1")
				if err != nil {
					t.Fatalf("list replaced entries: %v", err)
				}
				if len(entries) != 0 {
					t.Fatalf("replaced entries = %#v, want none", entries)
				}
			})
		})
	}
}

func TestSendNowAcknowledgeFencesSessionChanges(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, change := range []struct {
				name string
				run  func(*testing.T, Repository, *SendNowClaim)
			}{
				{
					name: "transfer",
					run: func(t *testing.T, repo Repository, claim *SendNowClaim) {
						ctx := context.Background()
						if err := repo.TransferSession(ctx, "session-1", "session-2"); err != nil {
							t.Fatalf("transfer: %v", err)
						}
						if err := repo.AcknowledgeSendNowClaim(ctx, claim); !errors.Is(err, ErrSendNowClaimChanged) {
							t.Fatalf("acknowledge after transfer error = %v, want ErrSendNowClaimChanged", err)
						}
						entries, err := repo.ListBySession(ctx, "session-2")
						if err != nil {
							t.Fatalf("list transferred entries: %v", err)
						}
						if len(entries) != 1 || !entries[0].IsReservedInFlight() {
							t.Fatalf("transferred entries = %#v, want reserved source preserved", entries)
						}
					},
				},
				{
					name: "replacement",
					run: func(t *testing.T, repo Repository, claim *SendNowClaim) {
						ctx := context.Background()
						replacement, err := repo.ListBySession(ctx, "session-1")
						if err != nil {
							t.Fatalf("list reserved replacement: %v", err)
						}
						if err := repo.ReplaceSession(ctx, "session-1", replacement, nil); err != nil {
							t.Fatalf("replace: %v", err)
						}
						if err := repo.AcknowledgeSendNowClaim(ctx, claim); !errors.Is(err, ErrSendNowClaimChanged) {
							t.Fatalf("acknowledge after replacement error = %v, want ErrSendNowClaimChanged", err)
						}
						entries, err := repo.ListBySession(ctx, "session-1")
						if err != nil {
							t.Fatalf("list replaced entries: %v", err)
						}
						if len(entries) != 1 || !entries[0].IsReservedInFlight() {
							t.Fatalf("replaced entries = %#v, want reserved replacement preserved", entries)
						}
					},
				},
				{
					name: "purge",
					run: func(t *testing.T, repo Repository, claim *SendNowClaim) {
						ctx := context.Background()
						if _, err := repo.PurgeSession(ctx, "session-1"); err != nil {
							t.Fatalf("purge session: %v", err)
						}
						if err := repo.AcknowledgeSendNowClaim(ctx, claim); !errors.Is(err, ErrSendNowClaimChanged) {
							t.Fatalf("acknowledge after purge error = %v, want ErrSendNowClaimChanged", err)
						}
					},
				},
			} {
				t.Run(change.name, func(t *testing.T) {
					repo := tt.new(t)
					ctx := context.Background()
					entry := insertTestEntry(t, repo, "session-1", "task-1", "durable source", QueuedByWorkflow, nil,
						map[string]interface{}{MetadataLifecycleDurable: true})
					claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*entry})
					if err != nil {
						t.Fatalf("claim: %v", err)
					}
					change.run(t, repo, claim)
				})
			}
		})
	}
}

func TestSendNowAcknowledgeIgnoresPurgedTaskGeneration(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			ctx := context.Background()
			entry := insertTestEntry(t, repo, "session-1", "task-1", "durable source", QueuedByWorkflow, nil,
				map[string]interface{}{MetadataLifecycleDurable: true})
			claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*entry})
			if err != nil {
				t.Fatalf("claim: %v", err)
			}
			if _, err := repo.PurgeTask(ctx, "task-1"); err != nil {
				t.Fatalf("purge task: %v", err)
			}

			replacement := claim.Sources[0]
			replacement.Metadata = map[string]interface{}{
				MetadataLifecycleDurable:  true,
				MetadataLifecycleReserved: true,
			}
			replacement.reservedLifecycleDelivery = true
			replacement.SessionID = "session-1"
			if err := repo.ReplaceSession(ctx, "session-1", []QueuedMessage{replacement}, nil); err != nil {
				t.Fatalf("replace with successor: %v", err)
			}
			// The replacement changes the session generation, so isolate the
			// task-generation fence to the purged task generation itself.
			claim.SessionGeneration++
			err = repo.AcknowledgeSendNowClaim(ctx, claim)
			if tt.name == "sqlite" {
				if !errors.Is(err, ErrSendNowClaimChanged) {
					t.Fatalf("acknowledge stale task claim error = %v, want %v", err, ErrSendNowClaimChanged)
				}
			} else if err != nil {
				t.Fatalf("acknowledge stale task claim: %v", err)
			}
			entries, err := repo.ListBySession(ctx, "session-1")
			if err != nil {
				t.Fatalf("list successor: %v", err)
			}
			if len(entries) != 1 || !entries[0].IsReservedInFlight() {
				t.Fatalf("successor after stale acknowledge = %#v, want reserved successor preserved", entries)
			}
		})
	}
}
func TestSendNowRestoreKeepsCurrentDurableMetadataAndRecordedMarker(t *testing.T) {
	repo := NewMemoryRepository().(*memoryRepository)
	ctx := context.Background()
	entry := insertTestEntry(t, repo, "session-1", "task-1", "durable", QueuedByWorkflow, nil,
		map[string]interface{}{
			MetadataLifecycleDurable: true,
			"origin":                 "github_pr_automation",
			"custom":                 "before-claim",
		})
	claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*entry})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Simulate a metadata update made while the durable row was reserved. The
	// restore must not overwrite it with the click-time snapshot.
	stored := repo.entries["session-1"][0]
	stored.Metadata["custom"] = "edited-while-reserved"
	claim.Sources[0].Metadata[metadataUserMessageRecorded] = true

	if err := repo.RestoreSendNowClaim(ctx, claim); err != nil {
		t.Fatalf("restore: %v", err)
	}
	entries, err := repo.ListBySession(ctx, "session-1")
	if err != nil {
		t.Fatalf("list after restore: %v", err)
	}
	if got := entries[0].Metadata["custom"]; got != "edited-while-reserved" {
		t.Fatalf("restored current metadata = %#v, want edited value", got)
	}
	if recorded, _ := entries[0].Metadata[metadataUserMessageRecorded].(bool); !recorded {
		t.Fatalf("restored metadata lost recorded marker: %#v", entries[0].Metadata)
	}
}
func TestRestoreKeepsPositionsUniqueAfterQueueWasEmptied(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			ctx := context.Background()
			first := insertTestEntry(t, repo, "session-restore-order", "task", "first", QueuedByUser, nil, nil)
			second := insertTestEntry(t, repo, "session-restore-order", "task", "second", QueuedByUser, nil, nil)
			sources := []QueuedMessage{*first, *second}
			claim, err := repo.ClaimSendNow(ctx, "session-restore-order", sources)
			require.NoError(t, err)
			later := insertTestEntry(t, repo, "session-restore-order", "task", "later", QueuedByUser, nil, nil)
			require.NoError(t, repo.RestoreSendNowClaim(ctx, claim))

			entries, err := repo.ListBySession(ctx, "session-restore-order")
			require.NoError(t, err)
			require.Len(t, entries, 3)
			require.Equal(t, []string{first.ID, second.ID, later.ID}, []string{entries[0].ID, entries[1].ID, entries[2].ID})
			require.Less(t, entries[0].Position, entries[1].Position)
			require.Less(t, entries[1].Position, entries[2].Position)
		})
	}
}
func TestOrdinaryRestoreKeepsPositionAheadOfLaterArrival(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			ctx := context.Background()
			first := insertTestEntry(t, repo, "session-ordinary-restore", "task", "first", QueuedByUser, nil, nil)
			later := insertTestEntry(t, repo, "session-ordinary-restore", "task", "later", QueuedByUser, nil, nil)
			taken, err := repo.TakeHead(ctx, "session-ordinary-restore")
			require.NoError(t, err)
			require.Equal(t, first.ID, taken.ID)
			third := insertTestEntry(t, repo, "session-ordinary-restore", "task", "third", QueuedByUser, nil, nil)
			require.NoError(t, repo.Restore(ctx, taken, 0))

			entries, err := repo.ListBySession(ctx, "session-ordinary-restore")
			require.NoError(t, err)
			require.Len(t, entries, 3)
			require.Equal(t, []string{first.ID, later.ID, third.ID}, []string{entries[0].ID, entries[1].ID, entries[2].ID})
			require.Less(t, entries[0].Position, entries[1].Position)
			require.Less(t, entries[1].Position, entries[2].Position)
		})
	}
}
func TestTransferRestoreKeepsDestinationPositionHighWater(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			ctx := context.Background()
			var transferredSource *QueuedMessage
			for i := range 7 {
				entry := insertTestEntry(t, repo, "transfer-source", "task", "source", QueuedByUser, nil, nil)
				if i == 6 {
					transferredSource = entry
					continue
				}
				taken, err := repo.TakeHead(ctx, "transfer-source")
				require.NoError(t, err)
				require.Equal(t, entry.ID, taken.ID)
			}
			require.NotNil(t, transferredSource)

			require.NoError(t, repo.TransferSession(ctx, "transfer-source", "transfer-destination"))
			transferred, err := repo.TakeHead(ctx, "transfer-destination")
			require.NoError(t, err)
			require.Equal(t, transferredSource.ID, transferred.ID)
			later := insertTestEntry(t, repo, "transfer-destination", "task", "later", QueuedByUser, nil, nil)

			require.NoError(t, repo.Restore(ctx, transferred, 0))
			entries, err := repo.ListBySession(ctx, "transfer-destination")
			require.NoError(t, err)
			require.Equal(t, []string{transferred.ID, later.ID}, []string{entries[0].ID, entries[1].ID})
			require.Less(t, entries[0].Position, entries[1].Position)
		})
	}
}
func TestOrdinaryRestoreKeepsPositionAfterDeleteEmptiesQueue(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			ctx := context.Background()
			first := insertTestEntry(t, repo, "session-delete-restore", "task", "first", QueuedByUser, nil, nil)
			second := insertTestEntry(t, repo, "session-delete-restore", "task", "second", QueuedByUser, nil, nil)
			taken, err := repo.TakeHead(ctx, "session-delete-restore")
			require.NoError(t, err)
			require.Equal(t, first.ID, taken.ID)
			require.NoError(t, repo.DeleteByID(ctx, "session-delete-restore", second.ID))

			later := insertTestEntry(t, repo, "session-delete-restore", "task", "later", QueuedByUser, nil, nil)
			require.NoError(t, repo.Restore(ctx, taken, 0))

			entries, err := repo.ListBySession(ctx, "session-delete-restore")
			require.NoError(t, err)
			require.Equal(t, []string{first.ID, later.ID}, []string{entries[0].ID, entries[1].ID})
			require.Less(t, entries[0].Position, entries[1].Position)
		})
	}
}
