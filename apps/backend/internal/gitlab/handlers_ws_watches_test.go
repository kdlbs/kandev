package gitlab

import (
	"context"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// newWatchWSFixture builds a service whose watch-dependency validators accept
// the "ws-1" fixture identifiers, so the WS create/update handlers exercise the
// real validation path rather than a stub that waves everything through.
func newWatchWSFixture(t *testing.T) (*Service, *Store) {
	t.Helper()
	svc, store := newDependencyTestService(t)
	seedWorkspace(t, store, "ws-1")
	seedWorkspace(t, store, "ws-2")
	return svc, store
}

// --- Review watches ---

func TestWSListReviewWatchesScopesToWorkspaceWhenGiven(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()
	mine := validReviewWatchRow()
	other := validReviewWatchRow()
	other.WorkspaceID = "ws-2"
	for _, w := range []*ReviewWatch{mine, other} {
		if err := store.CreateReviewWatch(ctx, w); err != nil {
			t.Fatalf("seed review watch: %v", err)
		}
	}

	resp, err := wsListReviewWatches(svc, nil)(ctx,
		wsRequest(t, ws.ActionGitLabReviewWatchesList, map[string]string{"workspace_id": "ws-1"}))

	var body struct {
		Watches []*ReviewWatch `json:"watches"`
	}
	wsOK(t, resp, err, ws.ActionGitLabReviewWatchesList, &body)
	if len(body.Watches) != 1 {
		t.Fatalf("watches = %d, want only the ws-1 watch", len(body.Watches))
	}
	if body.Watches[0].WorkspaceID != "ws-1" {
		t.Errorf("watch workspace = %q, want ws-1", body.Watches[0].WorkspaceID)
	}
}

// TestWSListReviewWatchesWithoutWorkspaceListsAll pins the deliberate fallback
// in wsListReviewWatches: an absent workspace_id lists every workspace's
// watches. If that ever needs to become a rejection, this test is the one to
// change.
func TestWSListReviewWatchesWithoutWorkspaceListsAll(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()
	other := validReviewWatchRow()
	other.WorkspaceID = "ws-2"
	for _, w := range []*ReviewWatch{validReviewWatchRow(), other} {
		if err := store.CreateReviewWatch(ctx, w); err != nil {
			t.Fatalf("seed review watch: %v", err)
		}
	}

	resp, err := wsListReviewWatches(svc, nil)(ctx,
		wsRequest(t, ws.ActionGitLabReviewWatchesList, map[string]string{}))

	var body struct {
		Watches []*ReviewWatch `json:"watches"`
	}
	wsOK(t, resp, err, ws.ActionGitLabReviewWatchesList, &body)
	if len(body.Watches) != 2 {
		t.Fatalf("watches = %d, want both workspaces' watches", len(body.Watches))
	}
}

func TestWSListReviewWatchesRejectsMalformedPayload(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsListReviewWatches(svc, nil)(context.Background(),
		wsRawRequest(ws.ActionGitLabReviewWatchesList, `{"workspace_id":7}`))

	wsErr(t, resp, err, ws.ActionGitLabReviewWatchesList, ws.ErrorCodeBadRequest, errMsgInvalidPayload)
}

func TestWSCreateReviewWatchPersistsAndReturnsRow(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()

	resp, err := wsCreateReviewWatch(svc, nil)(ctx,
		wsRequest(t, ws.ActionGitLabReviewWatchCreate, validReviewWatchRequest()))

	var created ReviewWatch
	wsOK(t, resp, err, ws.ActionGitLabReviewWatchCreate, &created)
	if created.ID == "" {
		t.Fatal("created watch has no id")
	}
	if created.WorkspaceID != "ws-1" || created.RepositoryID != "repo-valid" {
		t.Errorf("created watch = %+v, want the requested workspace/repository", created)
	}
	stored, err := store.GetReviewWatch(ctx, created.ID)
	if err != nil || stored == nil {
		t.Fatalf("watch was not persisted: row=%v err=%v", stored, err)
	}
}

func TestWSCreateReviewWatchRejectsInvalidDependencies(t *testing.T) {
	svc, _ := newWatchWSFixture(t)
	req := validReviewWatchRequest()
	req.RepositoryID = "repo-other" // belongs to ws-2

	resp, err := wsCreateReviewWatch(svc, nil)(context.Background(),
		wsRequest(t, ws.ActionGitLabReviewWatchCreate, req))

	wsErr(t, resp, err, ws.ActionGitLabReviewWatchCreate, ws.ErrorCodeInternalError, "")
}

func TestWSCreateReviewWatchRejectsMalformedPayload(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsCreateReviewWatch(svc, nil)(context.Background(),
		wsRawRequest(ws.ActionGitLabReviewWatchCreate, `{"workspace_id":[]}`))

	wsErr(t, resp, err, ws.ActionGitLabReviewWatchCreate, ws.ErrorCodeBadRequest, errMsgInvalidPayload)
}

func TestWSUpdateReviewWatchAppliesPatch(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()
	watch := validReviewWatchRow()
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("seed review watch: %v", err)
	}

	resp, err := wsUpdateReviewWatch(svc, nil)(ctx,
		wsRequest(t, ws.ActionGitLabReviewWatchUpdate, map[string]interface{}{
			"id":      watch.ID,
			"enabled": false,
		}))

	var body map[string]bool
	wsOK(t, resp, err, ws.ActionGitLabReviewWatchUpdate, &body)
	if !body["updated"] {
		t.Errorf("payload = %v, want {\"updated\":true}", body)
	}
	stored, err := store.GetReviewWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("reload watch: %v", err)
	}
	if stored.Enabled {
		t.Error("watch is still enabled; the patch did not reach the store")
	}
}

func TestWSUpdateReviewWatchRequiresID(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsUpdateReviewWatch(svc, nil)(context.Background(),
		wsRequest(t, ws.ActionGitLabReviewWatchUpdate, map[string]interface{}{"enabled": false}))

	wsErr(t, resp, err, ws.ActionGitLabReviewWatchUpdate, ws.ErrorCodeBadRequest, errMsgIDRequired)
}

func TestWSUpdateReviewWatchReportsUnknownIDAsInternalError(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsUpdateReviewWatch(svc, nil)(context.Background(),
		wsRequest(t, ws.ActionGitLabReviewWatchUpdate, map[string]interface{}{
			"id": "does-not-exist", "enabled": false,
		}))

	wsErr(t, resp, err, ws.ActionGitLabReviewWatchUpdate, ws.ErrorCodeInternalError, "")
}

func TestWSDeleteReviewWatchRemovesRow(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()
	watch := validReviewWatchRow()
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("seed review watch: %v", err)
	}

	resp, err := wsDeleteReviewWatch(svc, nil)(ctx,
		wsRequest(t, ws.ActionGitLabReviewWatchDelete, map[string]string{"id": watch.ID}))

	var body map[string]bool
	wsOK(t, resp, err, ws.ActionGitLabReviewWatchDelete, &body)
	if !body[respKeyDeleted] {
		t.Errorf("payload = %v, want {\"deleted\":true}", body)
	}
	remaining, err := store.ListReviewWatches(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list review watches: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("watches remaining = %d, want 0", len(remaining))
	}
}

func TestWSDeleteReviewWatchRequiresID(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsDeleteReviewWatch(svc, nil)(context.Background(),
		wsRequest(t, ws.ActionGitLabReviewWatchDelete, map[string]string{}))

	wsErr(t, resp, err, ws.ActionGitLabReviewWatchDelete, ws.ErrorCodeBadRequest, errMsgIDRequired)
}

func TestWSTriggerReviewWatchRequiresID(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsTriggerReviewWatch(svc, nil)(context.Background(),
		wsRequest(t, ws.ActionGitLabReviewTrigger, map[string]string{}))

	wsErr(t, resp, err, ws.ActionGitLabReviewTrigger, ws.ErrorCodeBadRequest, errMsgIDRequired)
}

func TestWSTriggerReviewWatchReportsUnknownWatch(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsTriggerReviewWatch(svc, nil)(context.Background(),
		wsRequest(t, ws.ActionGitLabReviewTrigger, map[string]string{"id": "nope"}))

	wsErr(t, resp, err, ws.ActionGitLabReviewTrigger, ws.ErrorCodeInternalError, "")
}

func TestWSTriggerAllReviewChecksReturnsCount(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()
	disabled := validReviewWatchRow()
	disabled.Enabled = false
	if err := store.CreateReviewWatch(ctx, disabled); err != nil {
		t.Fatalf("seed review watch: %v", err)
	}

	resp, err := wsTriggerAllReviewChecks(svc, nil)(ctx,
		wsRequest(t, ws.ActionGitLabReviewTriggerAll, nil))

	var body map[string]int
	wsOK(t, resp, err, ws.ActionGitLabReviewTriggerAll, &body)
	if body["count"] != 0 {
		t.Errorf("count = %d, want 0 — a disabled watch must not be checked", body["count"])
	}
}

// --- MR watches ---

func TestWSListMRWatchesSelectsBranchBySessionThenTaskThenAll(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()
	seedTask(t, store, "task-a", "ws-1")
	seedTask(t, store, "task-b", "ws-1")
	if _, err := svc.CreateMRWatch(ctx, "sess-a", "task-a", "repo-1", "group/a", 1, "feat/a"); err != nil {
		t.Fatalf("seed watch a: %v", err)
	}
	if _, err := svc.CreateMRWatch(ctx, "sess-b", "task-b", "repo-1", "group/b", 2, "feat/b"); err != nil {
		t.Fatalf("seed watch b: %v", err)
	}

	tests := []struct {
		name    string
		payload map[string]string
		want    []string
	}{
		{"by session", map[string]string{"session_id": "sess-a"}, []string{"group/a"}},
		{"by task", map[string]string{"task_id": "task-b"}, []string{"group/b"}},
		{"all active", map[string]string{}, []string{"group/a", "group/b"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := wsListMRWatches(svc, nil)(ctx,
				wsRequest(t, ws.ActionGitLabMRWatchesList, test.payload))

			var body struct {
				Watches []*MRWatch `json:"watches"`
			}
			wsOK(t, resp, err, ws.ActionGitLabMRWatchesList, &body)
			if len(body.Watches) != len(test.want) {
				t.Fatalf("watches = %d, want %d (%v)", len(body.Watches), len(test.want), body.Watches)
			}
			got := map[string]bool{}
			for _, w := range body.Watches {
				got[w.ProjectPath] = true
			}
			for _, project := range test.want {
				if !got[project] {
					t.Errorf("watch for %q missing from %v", project, got)
				}
			}
		})
	}
}

func TestWSListMRWatchesRejectsMalformedPayload(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsListMRWatches(svc, nil)(context.Background(),
		wsRawRequest(ws.ActionGitLabMRWatchesList, `{"session_id":{}}`))

	wsErr(t, resp, err, ws.ActionGitLabMRWatchesList, ws.ErrorCodeBadRequest, errMsgInvalidPayload)
}

func TestWSDeleteMRWatchRemovesRow(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()
	seedTask(t, store, "task-a", "ws-1")
	watch, err := svc.CreateMRWatch(ctx, "sess-a", "task-a", "repo-1", "group/a", 1, "feat/a")
	if err != nil {
		t.Fatalf("seed watch: %v", err)
	}

	resp, err := wsDeleteMRWatch(svc, nil)(ctx,
		wsRequest(t, ws.ActionGitLabMRWatchDelete, map[string]string{"id": watch.ID}))

	var body map[string]bool
	wsOK(t, resp, err, ws.ActionGitLabMRWatchDelete, &body)
	if !body[respKeyDeleted] {
		t.Errorf("payload = %v, want {\"deleted\":true}", body)
	}
	remaining, err := svc.ListActiveMRWatches(ctx)
	if err != nil {
		t.Fatalf("list active watches: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("watches remaining = %d, want 0", len(remaining))
	}
}

func TestWSDeleteMRWatchRequiresID(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsDeleteMRWatch(svc, nil)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRWatchDelete, map[string]string{}))

	wsErr(t, resp, err, ws.ActionGitLabMRWatchDelete, ws.ErrorCodeBadRequest, errMsgIDRequired)
}

// --- Issue watches ---

func TestWSListIssueWatchesScopesToWorkspaceWhenGiven(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()
	other := validIssueWatchRow()
	other.WorkspaceID = "ws-2"
	for _, w := range []*IssueWatch{validIssueWatchRow(), other} {
		if err := store.CreateIssueWatch(ctx, w); err != nil {
			t.Fatalf("seed issue watch: %v", err)
		}
	}

	resp, err := wsListIssueWatches(svc, nil)(ctx,
		wsRequest(t, ws.ActionGitLabIssueWatchesList, map[string]string{"workspace_id": "ws-2"}))

	var body struct {
		Watches []*IssueWatch `json:"watches"`
	}
	wsOK(t, resp, err, ws.ActionGitLabIssueWatchesList, &body)
	if len(body.Watches) != 1 || body.Watches[0].WorkspaceID != "ws-2" {
		t.Fatalf("watches = %+v, want only the ws-2 watch", body.Watches)
	}
}

func TestWSListIssueWatchesWithoutWorkspaceListsAll(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()
	other := validIssueWatchRow()
	other.WorkspaceID = "ws-2"
	for _, w := range []*IssueWatch{validIssueWatchRow(), other} {
		if err := store.CreateIssueWatch(ctx, w); err != nil {
			t.Fatalf("seed issue watch: %v", err)
		}
	}

	resp, err := wsListIssueWatches(svc, nil)(ctx,
		wsRequest(t, ws.ActionGitLabIssueWatchesList, map[string]string{}))

	var body struct {
		Watches []*IssueWatch `json:"watches"`
	}
	wsOK(t, resp, err, ws.ActionGitLabIssueWatchesList, &body)
	if len(body.Watches) != 2 {
		t.Fatalf("watches = %d, want both workspaces' watches", len(body.Watches))
	}
}

func TestWSListIssueWatchesRejectsMalformedPayload(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsListIssueWatches(svc, nil)(context.Background(),
		wsRawRequest(ws.ActionGitLabIssueWatchesList, `{"workspace_id":false}`))

	wsErr(t, resp, err, ws.ActionGitLabIssueWatchesList, ws.ErrorCodeBadRequest, errMsgInvalidPayload)
}

func TestWSCreateIssueWatchPersistsAndReturnsRow(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()

	resp, err := wsCreateIssueWatch(svc, nil)(ctx,
		wsRequest(t, ws.ActionGitLabIssueWatchCreate, validIssueWatchRequest()))

	var created IssueWatch
	wsOK(t, resp, err, ws.ActionGitLabIssueWatchCreate, &created)
	if created.ID == "" {
		t.Fatal("created watch has no id")
	}
	stored, err := store.GetIssueWatch(ctx, created.ID)
	if err != nil || stored == nil {
		t.Fatalf("watch was not persisted: row=%v err=%v", stored, err)
	}
}

func TestWSCreateIssueWatchRejectsInvalidDependencies(t *testing.T) {
	svc, _ := newWatchWSFixture(t)
	req := validIssueWatchRequest()
	req.AgentProfileID = "agent-missing"

	resp, err := wsCreateIssueWatch(svc, nil)(context.Background(),
		wsRequest(t, ws.ActionGitLabIssueWatchCreate, req))

	wsErr(t, resp, err, ws.ActionGitLabIssueWatchCreate, ws.ErrorCodeInternalError, "")
}

func TestWSCreateIssueWatchRejectsMalformedPayload(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsCreateIssueWatch(svc, nil)(context.Background(),
		wsRawRequest(ws.ActionGitLabIssueWatchCreate, `"a string, not an object"`))

	wsErr(t, resp, err, ws.ActionGitLabIssueWatchCreate, ws.ErrorCodeBadRequest, errMsgInvalidPayload)
}

func TestWSUpdateIssueWatchAppliesPatch(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()
	watch := validIssueWatchRow()
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("seed issue watch: %v", err)
	}

	resp, err := wsUpdateIssueWatch(svc, nil)(ctx,
		wsRequest(t, ws.ActionGitLabIssueWatchUpdate, map[string]interface{}{
			"id": watch.ID, "enabled": false,
		}))

	var body map[string]bool
	wsOK(t, resp, err, ws.ActionGitLabIssueWatchUpdate, &body)
	if !body["updated"] {
		t.Errorf("payload = %v, want {\"updated\":true}", body)
	}
	stored, err := store.GetIssueWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("reload watch: %v", err)
	}
	if stored.Enabled {
		t.Error("watch is still enabled; the patch did not reach the store")
	}
}

func TestWSUpdateIssueWatchRequiresID(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsUpdateIssueWatch(svc, nil)(context.Background(),
		wsRequest(t, ws.ActionGitLabIssueWatchUpdate, map[string]interface{}{"enabled": true}))

	wsErr(t, resp, err, ws.ActionGitLabIssueWatchUpdate, ws.ErrorCodeBadRequest, errMsgIDRequired)
}

func TestWSDeleteIssueWatchRemovesRow(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()
	watch := validIssueWatchRow()
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("seed issue watch: %v", err)
	}

	resp, err := wsDeleteIssueWatch(svc, nil)(ctx,
		wsRequest(t, ws.ActionGitLabIssueWatchDelete, map[string]string{"id": watch.ID}))

	var body map[string]bool
	wsOK(t, resp, err, ws.ActionGitLabIssueWatchDelete, &body)
	if !body[respKeyDeleted] {
		t.Errorf("payload = %v, want {\"deleted\":true}", body)
	}
	remaining, err := store.ListIssueWatches(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list issue watches: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("watches remaining = %d, want 0", len(remaining))
	}
}

func TestWSDeleteIssueWatchRequiresID(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsDeleteIssueWatch(svc, nil)(context.Background(),
		wsRequest(t, ws.ActionGitLabIssueWatchDelete, map[string]string{}))

	wsErr(t, resp, err, ws.ActionGitLabIssueWatchDelete, ws.ErrorCodeBadRequest, errMsgIDRequired)
}

func TestWSTriggerIssueWatchRequiresID(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsTriggerIssueWatch(svc, nil)(context.Background(),
		wsRequest(t, ws.ActionGitLabIssueTrigger, map[string]string{}))

	wsErr(t, resp, err, ws.ActionGitLabIssueTrigger, ws.ErrorCodeBadRequest, errMsgIDRequired)
}

func TestWSTriggerIssueWatchReportsUnknownWatch(t *testing.T) {
	svc, _ := newWatchWSFixture(t)

	resp, err := wsTriggerIssueWatch(svc, nil)(context.Background(),
		wsRequest(t, ws.ActionGitLabIssueTrigger, map[string]string{"id": "nope"}))

	wsErr(t, resp, err, ws.ActionGitLabIssueTrigger, ws.ErrorCodeInternalError, "")
}

func TestWSTriggerAllIssueChecksReturnsCount(t *testing.T) {
	svc, store := newWatchWSFixture(t)
	ctx := context.Background()
	disabled := validIssueWatchRow()
	disabled.Enabled = false
	if err := store.CreateIssueWatch(ctx, disabled); err != nil {
		t.Fatalf("seed issue watch: %v", err)
	}

	resp, err := wsTriggerAllIssueChecks(svc, nil)(ctx,
		wsRequest(t, ws.ActionGitLabIssueTriggerAll, nil))

	var body map[string]int
	wsOK(t, resp, err, ws.ActionGitLabIssueTriggerAll, &body)
	if body["count"] != 0 {
		t.Errorf("count = %d, want 0 — a disabled watch must not be checked", body["count"])
	}
}
