package clarification

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type fakeInboxRecoveryStore struct {
	fakeInboxBundleStore
	blocks        []taskmodels.WorkspaceRecoveryBlockSummary
	hasMore       bool
	err           error
	lastWorkspace string
	lastLimit     int
}

func (f *fakeInboxRecoveryStore) ListOpenWorkspaceRecoveryBlocks(
	_ context.Context, workspaceID string, limit int,
) ([]taskmodels.WorkspaceRecoveryBlockSummary, bool, error) {
	f.lastWorkspace, f.lastLimit = workspaceID, limit
	return f.blocks, f.hasMore, f.err
}

func TestHttpListInboxRecoveryBlocks_ListsBlocks(t *testing.T) {
	at := testInboxNow.Add(-30 * time.Minute)
	store := &fakeInboxRecoveryStore{
		blocks: []taskmodels.WorkspaceRecoveryBlockSummary{{
			BlockID: "b1", SessionID: "s1", SessionState: "WAITING_FOR_INPUT", TaskID: "t1",
			TaskTitle: "Blocked task", Reason: "native_resume_failed", CreatedAt: at, UpdatedAt: at,
		}},
		hasMore: true,
	}
	h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, &store.fakeInboxBundleStore)
	h.inboxBundles = store

	rec := runInboxGet(h, "/api/v1/clarification-inbox/recovery-blocks?workspace_id=w1&limit=5", h.httpListInboxRecoveryBlocks)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if store.lastWorkspace != "w1" || store.lastLimit != 5 {
		t.Fatalf("reader called with %q/%d, want w1/5", store.lastWorkspace, store.lastLimit)
	}
	var body inboxRecoveryListResponse
	decodeInboxBody(t, rec, &body)
	if body.Count != 1 || !body.Truncated || body.Blocks[0].BlockID != "b1" || body.Blocks[0].TaskTitle != "Blocked task" ||
		body.Blocks[0].CreatedAt != at.Format(time.RFC3339) {
		t.Fatalf("body = %+v, want b1 truncated", body)
	}
}

func TestHttpListInboxRecoveryBlocks_Errors(t *testing.T) {
	t.Run("forbidden", func(t *testing.T) {
		store := &fakeInboxRecoveryStore{}
		h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, &fakeInboxTasks{authzErr: repoerrors.ErrWorkspaceNotFound}, &store.fakeInboxBundleStore)
		h.inboxBundles = store
		rec := runInboxGet(h, "/api/v1/clarification-inbox/recovery-blocks?workspace_id=w1", h.httpListInboxRecoveryBlocks)
		if rec.Code != http.StatusNotFound || store.lastWorkspace != "" {
			t.Fatalf("status = %d (reader ws %q), want 404 without a read", rec.Code, store.lastWorkspace)
		}
	})
	t.Run("store failure", func(t *testing.T) {
		store := &fakeInboxRecoveryStore{err: errors.New("boom")}
		h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, &store.fakeInboxBundleStore)
		h.inboxBundles = store
		rec := runInboxGet(h, "/api/v1/clarification-inbox/recovery-blocks?workspace_id=w1", h.httpListInboxRecoveryBlocks)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
	})
	t.Run("unsupported store", func(t *testing.T) {
		h := newInboxTestHandler(t, nil, alwaysAllowAuthorizer{}, &fakeInboxTasks{}, &fakeInboxBundleStore{})
		rec := runInboxGet(h, "/api/v1/clarification-inbox/recovery-blocks?workspace_id=w1", h.httpListInboxRecoveryBlocks)
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501", rec.Code)
		}
	})
}
