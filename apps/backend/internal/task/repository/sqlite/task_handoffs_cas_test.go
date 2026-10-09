package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestGetTaskHandoffsRawReturnsEmptyWhenAbsent(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{"other_key": "keep me"})

	raw, err := repo.GetTaskHandoffsRaw(context.Background(), casTaskID)
	if err != nil {
		t.Fatalf("GetTaskHandoffsRaw: %v", err)
	}
	if raw != "" {
		t.Fatalf("raw = %q, want empty when handoffs key is absent", raw)
	}
}

func TestGetTaskHandoffsRawUnknownTaskReturnsSentinel(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)

	_, err := repo.GetTaskHandoffsRaw(context.Background(), "does-not-exist")
	if err != repoerrors.ErrTaskNotFound {
		t.Fatalf("err = %v, want repoerrors.ErrTaskNotFound", err)
	}
}

func TestSetTaskHandoffsIfUnchangedWritesOnMatch(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{"other_key": "keep me"})
	ctx := context.Background()

	stored, current, err := repo.SetTaskHandoffsIfUnchanged(ctx, casTaskID, "", `[{"task_id":"t1"}]`)
	if err != nil {
		t.Fatalf("SetTaskHandoffsIfUnchanged: %v", err)
	}
	if !stored {
		t.Fatal("expected the write to succeed against an empty expected value")
	}
	if current != `[{"task_id":"t1"}]` {
		t.Fatalf("current = %q, want the newly written value", current)
	}

	raw, err := repo.GetTaskHandoffsRaw(ctx, casTaskID)
	if err != nil {
		t.Fatalf("GetTaskHandoffsRaw: %v", err)
	}
	if raw != `[{"task_id":"t1"}]` {
		t.Fatalf("raw = %q, want the stored array", raw)
	}
	if _, untouched := metadataValue(t, repo, "other_key"); !untouched {
		t.Fatal("the handoffs write must not disturb neighbouring metadata keys")
	}
}

func TestSetTaskHandoffsIfUnchangedRefusesOnMismatch(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{"other_key": "keep me"})
	ctx := context.Background()

	stored, _, err := repo.SetTaskHandoffsIfUnchanged(ctx, casTaskID, "", `[{"task_id":"t1"}]`)
	if err != nil || !stored {
		t.Fatalf("seed write failed: stored=%v err=%v", stored, err)
	}

	stored, current, err := repo.SetTaskHandoffsIfUnchanged(ctx, casTaskID, "", `[{"task_id":"t2"}]`)
	if err != nil {
		t.Fatalf("SetTaskHandoffsIfUnchanged: %v", err)
	}
	if stored {
		t.Fatal("expected the stale-expected write to be refused")
	}
	if current != `[{"task_id":"t1"}]` {
		t.Fatalf("current = %q, want the actual stored value for the caller to retry against", current)
	}

	raw, err := repo.GetTaskHandoffsRaw(ctx, casTaskID)
	if err != nil {
		t.Fatalf("GetTaskHandoffsRaw: %v", err)
	}
	if raw != `[{"task_id":"t1"}]` {
		t.Fatalf("raw = %q, want the first write preserved", raw)
	}
}

func TestSetTaskHandoffsIfUnchangedUnknownTaskReturnsSentinel(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)

	_, _, err := repo.SetTaskHandoffsIfUnchanged(context.Background(), "does-not-exist", "", `[{"task_id":"t1"}]`)
	if err != repoerrors.ErrTaskNotFound {
		t.Fatalf("err = %v, want repoerrors.ErrTaskNotFound", err)
	}
}
