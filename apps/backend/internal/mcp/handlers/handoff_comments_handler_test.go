package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCommentReader is a minimal service.CommentReader double, mirroring the
// one in internal/task/service/handoff_comments_test.go (unexported there,
// so this package needs its own).
type fakeCommentReader struct {
	byTask map[string][]service.CommentRecord
}

func (f *fakeCommentReader) ListTaskCommentsWindow(_ context.Context, taskID string, limit int) ([]service.CommentRecord, int, error) {
	all := f.byTask[taskID]
	total := len(all)
	if limit <= 0 || limit > len(all) {
		limit = len(all)
	}
	return all[:limit], total, nil
}

func newCommentsHandoffFixture(t *testing.T) (*Handlers, *service.HandoffService, string, string) {
	t.Helper()
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-comments", Name: "Comments WS", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-comments", WorkspaceID: "ws-comments", Name: "Comments Workflow", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "parent-c", WorkspaceID: "ws-comments", WorkflowID: "wf-comments",
		Title: "Parent", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "child-c", WorkspaceID: "ws-comments", WorkflowID: "wf-comments",
		Title: "Child", ParentID: "parent-c", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "unrelated-c", WorkspaceID: "ws-comments", WorkflowID: "wf-comments",
		Title: "Unrelated", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))

	handoffSvc := service.NewHandoffService(repo, nil, nil, nil, nil, testLogger(t))
	handoffSvc.SetCommentReader(&fakeCommentReader{byTask: map[string][]service.CommentRecord{
		"child-c": {{
			ID: "c1", TaskID: "child-c", AuthorType: "agent", AuthorID: "worker-1",
			Source: "run", Body: "stage deliverable", CreatedAt: now,
		}},
	}})

	h := &Handlers{taskSvc: svc, handoffSvc: handoffSvc, logger: testLogger(t).WithFields()}
	return h, handoffSvc, "parent-c", "child-c"
}

func TestHandleListTaskComments_RequiresHandoffService(t *testing.T) {
	h := &Handlers{logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPListTaskComments, map[string]any{"task_id": "task-A"})

	resp, err := h.handleListTaskComments(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
}

// AC-001.3/AC-002: a parent reading its child's comments gets the projected
// window, including author fields.
func TestHandleListTaskComments_DispatchesToServiceAndProjects(t *testing.T) {
	h, _, parentID, childID := newCommentsHandoffFixture(t)

	msg := makeWSMessage(t, ws.ActionMCPListTaskComments, map[string]any{
		"task_id": childID, "caller_task_id": parentID, "limit": 20,
	})
	resp, err := h.handleListTaskComments(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	var window service.CommentWindow
	require.NoError(t, json.Unmarshal(resp.Payload, &window))
	require.Len(t, window.Comments, 1)
	assert.Equal(t, "stage deliverable", window.Comments[0].Body)
	assert.Equal(t, "agent", window.Comments[0].AuthorType)
	assert.Equal(t, 1, window.Total)
	assert.Equal(t, 1, window.Returned)
	assert.False(t, window.HasMore)
}

// AC-005.4/005.5: an empty task_id must reach the service layer unresolved so
// its self-fallback (and F13's caller-required validation) applies — this
// handler must NOT reject on an empty task_id the way the document handlers
// do.
func TestHandleListTaskComments_ForwardsEmptyTaskIDToService(t *testing.T) {
	h, _, _, childID := newCommentsHandoffFixture(t)

	msg := makeWSMessage(t, ws.ActionMCPListTaskComments, map[string]any{
		"task_id": "", "caller_task_id": childID,
	})
	resp, err := h.handleListTaskComments(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "empty task_id with a caller must resolve to the caller, not error")

	var window service.CommentWindow
	require.NoError(t, json.Unmarshal(resp.Payload, &window))
	assert.Equal(t, 1, window.Total)
}

func TestHandleListTaskComments_MapsAccessDeniedToForbidden(t *testing.T) {
	h, _, _, childID := newCommentsHandoffFixture(t)

	msg := makeWSMessage(t, ws.ActionMCPListTaskComments, map[string]any{
		"task_id": childID, "caller_task_id": "unrelated-task",
	})
	resp, err := h.handleListTaskComments(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
}

func TestHandleListTaskCommentsUsesTrustedPrincipalCaller(t *testing.T) {
	h, _, parentID, childID := newCommentsHandoffFixture(t)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		CallerTaskID:    "unrelated-c",
		CallerSessionID: "unrelated-session",
		Surface:         mcpprofile.SurfaceOfficeTask,
	})

	msg := makeWSMessage(t, ws.ActionMCPListTaskComments, map[string]any{
		"task_id": childID, "caller_task_id": parentID, "limit": 20,
	})
	resp, err := h.handleListTaskComments(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
}

func TestHandleListTaskComments_MapsMissingTaskIDToValidation(t *testing.T) {
	h, _, _, _ := newCommentsHandoffFixture(t)

	msg := makeWSMessage(t, ws.ActionMCPListTaskComments, map[string]any{
		"task_id": "", "caller_task_id": "",
	})
	resp, err := h.handleListTaskComments(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)

	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &ep))
	assert.Equal(t, service.ErrDocumentTaskRequired.Error(), ep.Message)
}

func TestHandleListTaskComments_RejectsBadPayload(t *testing.T) {
	h, _, _, _ := newCommentsHandoffFixture(t)
	msg := &ws.Message{ID: "1", Action: ws.ActionMCPListTaskComments, Payload: json.RawMessage(`{"limit":`)}

	resp, err := h.handleListTaskComments(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}
