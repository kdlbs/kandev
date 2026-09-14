package messagequeue

import (
	"context"
	"errors"
	"testing"
)

type autoMergeOverrideRepository interface {
	GetAutoMergeOverride(context.Context, QueueSessionIdentity) (*AutoMergeOverride, error)
	SetAutoMergeOverride(context.Context, QueueSessionIdentity, bool) (AutoMergeOverride, error)
}

// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.2
// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.3
func TestMemoryAutoMergeOverridePersistsTriStateAndRejectsTextualReuse(t *testing.T) {
	repo := NewMemoryRepository()
	policies, ok := repo.(autoMergeOverrideRepository)
	if !ok {
		t.Fatal("memory repository does not implement session Auto-merge overrides")
	}
	ctx := context.Background()
	first := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation-1"}
	replacement := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation-2"}
	seedQueueSessionIdentity(t, repo, first)

	inherited, err := policies.GetAutoMergeOverride(ctx, first)
	if err != nil {
		t.Fatalf("get inherited override: %v", err)
	}
	if inherited != nil {
		t.Fatalf("inherited override = %+v, want nil", inherited)
	}
	written, err := policies.SetAutoMergeOverride(ctx, first, false)
	if err != nil {
		t.Fatalf("write explicit OFF: %v", err)
	}
	if written.Enabled || written.Revision != 1 {
		t.Fatalf("explicit OFF = %+v, want false revision 1", written)
	}
	written, err = policies.SetAutoMergeOverride(ctx, first, true)
	if err != nil {
		t.Fatalf("write explicit ON: %v", err)
	}
	if !written.Enabled || written.Revision != 2 {
		t.Fatalf("explicit ON = %+v, want true revision 2", written)
	}

	inherited, err = policies.GetAutoMergeOverride(ctx, replacement)
	if !errors.Is(err, ErrSessionIdentityMismatch) {
		t.Fatalf("replacement identity error = %v, want ErrSessionIdentityMismatch", err)
	}
	if inherited != nil {
		t.Fatalf("replacement override = %+v, want nil", inherited)
	}
}

func TestSQLiteAutoMergeOverrideReadRejectsReplacedSessionIdentity(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()
	first := QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation-1",
	}
	replacement := QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation-2",
	}
	seedQueueSessionIdentity(t, repo, first)
	if _, err := repo.SetAutoMergeOverride(ctx, first, true); err != nil {
		t.Fatalf("set first override: %v", err)
	}
	seedQueueSessionIdentity(t, repo, replacement)

	override, err := repo.GetAutoMergeOverride(ctx, first)
	if !errors.Is(err, ErrSessionIdentityMismatch) {
		t.Fatalf("stale identity error = %v, want ErrSessionIdentityMismatch", err)
	}
	if override != nil {
		t.Fatalf("stale identity override = %+v, want nil", override)
	}
}

func TestSQLiteAutoMergeOverrideClaimsExistingUnboundSessionState(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()
	const sessionID = "session"
	if err := repo.SetAutoRun(ctx, sessionID, false); err != nil {
		t.Fatalf("create unbound queue session state: %v", err)
	}
	identity := QueueSessionIdentity{
		TaskID: "task", SessionID: sessionID, SessionIncarnationID: "incarnation",
	}
	seedQueueSessionIdentity(t, repo, identity)
	written, err := repo.SetAutoMergeOverride(ctx, identity, true)
	if err != nil {
		t.Fatalf("claim queue session state for override: %v", err)
	}
	if !written.Enabled || written.Revision != 1 {
		t.Fatalf("written override = %+v, want ON revision 1", written)
	}
}
