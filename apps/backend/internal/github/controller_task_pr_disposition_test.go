package github

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func seedDispositionEndpointTaskPR(t *testing.T, store *Store, tp *TaskPR) *TaskPR {
	t.Helper()
	if tp.CreatedAt.IsZero() {
		tp.CreatedAt = time.Now().UTC()
	}
	if err := store.CreateTaskPR(context.Background(), tp); err != nil {
		t.Fatalf("seed task PR: %v", err)
	}
	return tp
}

func dispositionPatch(t *testing.T, router http.Handler, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPatch, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

// TestHttpSetTaskPRDisposition_HappyPathPersistsAllThreeAndReturnsRow covers
// AC-20, AC-21: a valid body persists disposition, superseded_by_url, and
// disposition_recorded_at, and returns the updated association.
func TestHttpSetTaskPRDisposition_HappyPathPersistsAllThreeAndReturnsRow(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	tp := seedDispositionEndpointTaskPR(t, store, &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "kdlbs", Repo: "kandev",
		PRNumber: 100, PRURL: "https://github.com/kdlbs/kandev/pull/100",
		PRTitle: "closed unmerged", State: "closed",
	})

	resp := dispositionPatch(t, router, "/api/v1/github/task-prs/"+tp.ID+"/disposition?workspace_id=ws-1", map[string]any{
		"disposition":       "superseded",
		"superseded_by_url": "https://github.com/kdlbs/kandev/pull/101",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var got TaskPR
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Disposition == nil || *got.Disposition != "superseded" {
		t.Errorf("Disposition = %v, want superseded", got.Disposition)
	}
	if got.DispositionSupersededByURL == nil || *got.DispositionSupersededByURL != "https://github.com/kdlbs/kandev/pull/101" {
		t.Errorf("DispositionSupersededByURL = %v", got.DispositionSupersededByURL)
	}
	if got.DispositionRecordedAt == nil {
		t.Error("DispositionRecordedAt = nil, want set")
	}

	fromStore, err := store.GetTaskPRByID(context.Background(), tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if fromStore.Disposition == nil || *fromStore.Disposition != "superseded" {
		t.Fatalf("persisted Disposition = %v, want superseded", fromStore.Disposition)
	}
}

// TestHttpSetTaskPRDisposition_NullClearsAllThreeColumns covers AC-22.
func TestHttpSetTaskPRDisposition_NullClearsAllThreeColumns(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	disposition := "duplicate"
	now := time.Now().UTC()
	tp := seedDispositionEndpointTaskPR(t, store, &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "kdlbs", Repo: "kandev",
		PRNumber: 100, PRURL: "https://github.com/kdlbs/kandev/pull/100",
		PRTitle: "t", State: "closed",
		Disposition: &disposition, DispositionRecordedAt: &now,
	})

	resp := dispositionPatch(t, router, "/api/v1/github/task-prs/"+tp.ID+"/disposition?workspace_id=ws-1", map[string]any{
		"disposition": nil,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	fromStore, err := store.GetTaskPRByID(context.Background(), tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if fromStore.Disposition != nil || fromStore.DispositionSupersededByURL != nil || fromStore.DispositionRecordedAt != nil {
		t.Fatalf("expected all three cleared, got %+v", fromStore)
	}
}

// TestHttpSetTaskPRDisposition_RejectsUnknownEnumValue covers AC-23.
func TestHttpSetTaskPRDisposition_RejectsUnknownEnumValue(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	tp := seedDispositionEndpointTaskPR(t, store, &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "kdlbs", Repo: "kandev",
		PRNumber: 100, PRURL: "https://github.com/kdlbs/kandev/pull/100", PRTitle: "t", State: "closed",
	})
	resp := dispositionPatch(t, router, "/api/v1/github/task-prs/"+tp.ID+"/disposition?workspace_id=ws-1", map[string]any{
		"disposition": "abandoned",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", resp.Code, resp.Body.String())
	}
	fromStore, err := store.GetTaskPRByID(context.Background(), tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if fromStore.Disposition != nil {
		t.Fatalf("nothing should have been written, got Disposition = %v", *fromStore.Disposition)
	}
}

// TestHttpSetTaskPRDisposition_RejectsURLWithoutSupersededDisposition covers
// AC-24, including the clear-plus-URL body.
func TestHttpSetTaskPRDisposition_RejectsURLWithoutSupersededDisposition(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	tp := seedDispositionEndpointTaskPR(t, store, &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "kdlbs", Repo: "kandev",
		PRNumber: 100, PRURL: "https://github.com/kdlbs/kandev/pull/100", PRTitle: "t", State: "closed",
	})

	t.Run("non-superseded disposition with URL", func(t *testing.T) {
		resp := dispositionPatch(t, router, "/api/v1/github/task-prs/"+tp.ID+"/disposition?workspace_id=ws-1", map[string]any{
			"disposition":       "duplicate",
			"superseded_by_url": "https://github.com/kdlbs/kandev/pull/101",
		})
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body = %s", resp.Code, resp.Body.String())
		}
	})
	t.Run("clear plus URL", func(t *testing.T) {
		resp := dispositionPatch(t, router, "/api/v1/github/task-prs/"+tp.ID+"/disposition?workspace_id=ws-1", map[string]any{
			"superseded_by_url": "https://github.com/kdlbs/kandev/pull/101",
		})
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body = %s", resp.Code, resp.Body.String())
		}
	})
}

// TestHttpSetTaskPRDisposition_EmptySupersededURLTreatedAsAbsent covers the
// spec's "Nil, empty, and error" rule: an empty superseded_by_url string is
// absent, not an invalid URL, so it must not trip AC-24 (URL without
// superseded disposition) or AC-25 (unparseable URL).
func TestHttpSetTaskPRDisposition_EmptySupersededURLTreatedAsAbsent(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	tp := seedDispositionEndpointTaskPR(t, store, &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "kdlbs", Repo: "kandev",
		PRNumber: 100, PRURL: "https://github.com/kdlbs/kandev/pull/100", PRTitle: "t", State: "closed",
	})
	resp := dispositionPatch(t, router, "/api/v1/github/task-prs/"+tp.ID+"/disposition?workspace_id=ws-1", map[string]any{
		"disposition":       "duplicate",
		"superseded_by_url": "",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", resp.Code, resp.Body.String())
	}
	fromStore, err := store.GetTaskPRByID(context.Background(), tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if fromStore.Disposition == nil || *fromStore.Disposition != "duplicate" {
		t.Fatalf("Disposition = %v, want duplicate", fromStore.Disposition)
	}
	if fromStore.DispositionSupersededByURL != nil {
		t.Fatalf("DispositionSupersededByURL = %v, want nil (empty string treated as absent)", *fromStore.DispositionSupersededByURL)
	}
}

// TestHttpSetTaskPRDisposition_RejectsUnparseableURL covers AC-25.
func TestHttpSetTaskPRDisposition_RejectsUnparseableURL(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	tp := seedDispositionEndpointTaskPR(t, store, &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "kdlbs", Repo: "kandev",
		PRNumber: 100, PRURL: "https://github.com/kdlbs/kandev/pull/100", PRTitle: "t", State: "closed",
	})
	resp := dispositionPatch(t, router, "/api/v1/github/task-prs/"+tp.ID+"/disposition?workspace_id=ws-1", map[string]any{
		"disposition":       "superseded",
		"superseded_by_url": "not-a-url",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", resp.Code, resp.Body.String())
	}
}

// TestHttpSetTaskPRDisposition_RejectsSelfSupersession covers AC-26.
func TestHttpSetTaskPRDisposition_RejectsSelfSupersession(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	tp := seedDispositionEndpointTaskPR(t, store, &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "kdlbs", Repo: "kandev",
		PRNumber: 100, PRURL: "https://github.com/kdlbs/kandev/pull/100", PRTitle: "t", State: "closed",
	})
	resp := dispositionPatch(t, router, "/api/v1/github/task-prs/"+tp.ID+"/disposition?workspace_id=ws-1", map[string]any{
		"disposition":       "superseded",
		"superseded_by_url": "https://github.com/KDLBS/Kandev/pull/100",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", resp.Code, resp.Body.String())
	}
}

// TestHttpSetTaskPRDisposition_MissingOrCrossWorkspaceReturns404 covers
// AC-28: no distinction between missing and cross-workspace.
func TestHttpSetTaskPRDisposition_MissingOrCrossWorkspaceReturns404(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	tp := seedDispositionEndpointTaskPR(t, store, &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "kdlbs", Repo: "kandev",
		PRNumber: 100, PRURL: "https://github.com/kdlbs/kandev/pull/100", PRTitle: "t", State: "closed",
	})

	t.Run("missing association", func(t *testing.T) {
		resp := dispositionPatch(t, router, "/api/v1/github/task-prs/does-not-exist/disposition?workspace_id=ws-1", map[string]any{
			"disposition": "unknown",
		})
		if resp.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404, body = %s", resp.Code, resp.Body.String())
		}
	})
	t.Run("cross-workspace association", func(t *testing.T) {
		resp := dispositionPatch(t, router, "/api/v1/github/task-prs/"+tp.ID+"/disposition?workspace_id=ws-other", map[string]any{
			"disposition": "unknown",
		})
		if resp.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404, body = %s", resp.Code, resp.Body.String())
		}
	})
}

// TestHttpSetTaskPRDisposition_AcceptedOnDetachedAssociation covers AC-27.
func TestHttpSetTaskPRDisposition_AcceptedOnDetachedAssociation(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	tp := seedDispositionEndpointTaskPR(t, store, &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "kdlbs", Repo: "kandev",
		PRNumber: 100, PRURL: "https://github.com/kdlbs/kandev/pull/100", PRTitle: "t", State: "closed",
	})
	if _, _, err := store.DetachTaskPR(context.Background(), tp.ID); err != nil {
		t.Fatalf("DetachTaskPR: %v", err)
	}

	resp := dispositionPatch(t, router, "/api/v1/github/task-prs/"+tp.ID+"/disposition?workspace_id=ws-1", map[string]any{
		"disposition": "withdrawn",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", resp.Code, resp.Body.String())
	}
}

// TestHttpSetTaskPRDisposition_AcceptedRegardlessOfState covers AC-29b: the
// endpoint accepts a write on an open (not closed-unmerged) association.
func TestHttpSetTaskPRDisposition_AcceptedRegardlessOfState(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	tp := seedDispositionEndpointTaskPR(t, store, &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "kdlbs", Repo: "kandev",
		PRNumber: 100, PRURL: "https://github.com/kdlbs/kandev/pull/100", PRTitle: "t", State: "open",
	})
	resp := dispositionPatch(t, router, "/api/v1/github/task-prs/"+tp.ID+"/disposition?workspace_id=ws-1", map[string]any{
		"disposition": "exploratory",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", resp.Code, resp.Body.String())
	}
}

// TestHttpSetTaskPRDisposition_IdenticalRePatchDoesNotAdvanceRecordedAt
// covers AC-29's idempotency clause: a repeated PATCH with an identical body
// leaves disposition_recorded_at byte-identical and publishes nothing.
func TestHttpSetTaskPRDisposition_IdenticalRePatchDoesNotAdvanceRecordedAt(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	tp := seedDispositionEndpointTaskPR(t, store, &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "kdlbs", Repo: "kandev",
		PRNumber: 100, PRURL: "https://github.com/kdlbs/kandev/pull/100", PRTitle: "t", State: "closed",
	})
	body := map[string]any{"disposition": "unknown"}

	first := dispositionPatch(t, router, "/api/v1/github/task-prs/"+tp.ID+"/disposition?workspace_id=ws-1", body)
	if first.Code != http.StatusOK {
		t.Fatalf("first PATCH status = %d, body = %s", first.Code, first.Body.String())
	}
	afterFirst, err := store.GetTaskPRByID(context.Background(), tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if afterFirst.DispositionRecordedAt == nil {
		t.Fatal("DispositionRecordedAt = nil after first PATCH, want set")
	}
	firstRecordedAt := *afterFirst.DispositionRecordedAt

	time.Sleep(2 * time.Millisecond)
	second := dispositionPatch(t, router, "/api/v1/github/task-prs/"+tp.ID+"/disposition?workspace_id=ws-1", body)
	if second.Code != http.StatusOK {
		t.Fatalf("second PATCH status = %d, body = %s", second.Code, second.Body.String())
	}
	afterSecond, err := store.GetTaskPRByID(context.Background(), tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID after second PATCH: %v", err)
	}
	if afterSecond.DispositionRecordedAt == nil || !afterSecond.DispositionRecordedAt.Equal(firstRecordedAt) {
		t.Fatalf("DispositionRecordedAt after identical re-PATCH = %v, want unchanged %v", afterSecond.DispositionRecordedAt, firstRecordedAt)
	}
}
