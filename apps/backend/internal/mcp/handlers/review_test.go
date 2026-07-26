package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/review"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// requireWSSuccess asserts the handler returned a non-error response.
func requireWSSuccess(t *testing.T, resp *ws.Message) {
	t.Helper()
	require.NotNil(t, resp)
	if resp.Type == ws.MessageTypeError {
		t.Fatalf("expected success, got error payload: %s", string(resp.Payload))
	}
}

// wsErrorDetails returns the machine-readable details map from an error response.
func wsErrorDetails(t *testing.T, resp *ws.Message) map[string]interface{} {
	t.Helper()
	require.NotNil(t, resp)
	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &ep))
	return ep.Details
}

// stubReviewRunner records the launch request and returns a canned outcome.
type stubReviewRunner struct {
	run       *models.TaskReviewRun
	err       error
	launched  []review.RunRequest
	cancelled []string
	cancelRun *models.TaskReviewRun
	cancelErr error
}

func (s *stubReviewRunner) Launch(_ context.Context, req review.RunRequest) (*models.TaskReviewRun, error) {
	s.launched = append(s.launched, req)
	return s.run, s.err
}

func (s *stubReviewRunner) Cancel(_ context.Context, runID string) (*models.TaskReviewRun, error) {
	s.cancelled = append(s.cancelled, runID)
	if s.cancelErr != nil {
		return nil, s.cancelErr
	}
	// Mirror the real runner, which delegates to the service and therefore
	// surfaces not-found for a run it does not know.
	if s.cancelRun == nil {
		return nil, models.ErrTaskReviewRunNotFound
	}
	return s.cancelRun, nil
}

func newReviewHandlers(t *testing.T) (*Handlers, *service.ReviewService, *stubReviewRunner) {
	t.Helper()
	svc, repo := newTestTaskService(t)
	reviewSvc := service.NewReviewService(repo, nil, testLogger(t))
	runner := &stubReviewRunner{}
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetReviewService(reviewSvc)
	h.SetReviewRunner(runner)
	return h, reviewSvc, runner
}

func seedReviewHandlerTask(t *testing.T, h *Handlers, taskID string) {
	t.Helper()
	ctx := context.Background()
	repo := h.taskRepo.(interface {
		CreateWorkspace(context.Context, *models.Workspace) error
		CreateWorkflow(context.Context, *models.Workflow) error
		CreateTask(context.Context, *models.Task) error
	})
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-r", Name: "R"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-r", WorkspaceID: "ws-r", Name: "WF"})
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws-r", WorkflowID: "wf-r",
		Title: "t", State: "BACKLOG", Priority: "medium",
	}))
}

func validFindingPayload() map[string]interface{} {
	return map[string]interface{}{
		"file":     "apps/web/a.ts",
		"line":     12,
		"severity": "blocker",
		"category": "correctness",
		"title":    "Nil dereference",
		"body":     "x can be nil",
	}
}

func TestHandlePublishReviewFindings_StoresValidBatch(t *testing.T) {
	h, reviewSvc, _ := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-pub")

	msg := makeWSMessage(t, ws.ActionMCPPublishReviewFindings, map[string]interface{}{
		"task_id": "task-pub",
		"summary": "two issues",
		"findings": []interface{}{
			validFindingPayload(),
			map[string]interface{}{
				"file": "apps/web/b.ts", "line": 3, "severity": "nit",
				"category": "style", "title": "Naming", "body": "prefer camelCase",
			},
		},
	})
	resp, err := h.handlePublishReviewFindings(context.Background(), msg)
	require.NoError(t, err)
	requireWSSuccess(t, resp)

	stored, err := reviewSvc.GetTaskReview(context.Background(), "task-pub")
	require.NoError(t, err)
	require.Len(t, stored.Findings, 2)
	require.Len(t, stored.Runs, 1)
	require.Equal(t, models.ReviewTriggerAgent, stored.Runs[0].Trigger)
	require.Equal(t, models.ReviewRunCompleted, stored.Runs[0].Status)
	// An agent-published finding has no reviewed-diff hash, so it is never
	// reported stale and never relocated.
	require.Empty(t, stored.Findings[0].FileDiffHash)
	require.Empty(t, stored.Findings[0].AnchorText)
}

func TestHandlePublishReviewFindings_RejectsWholeBatchOnMalformedEntry(t *testing.T) {
	h, reviewSvc, _ := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-bad")

	bad := validFindingPayload()
	delete(bad, "file")
	msg := makeWSMessage(t, ws.ActionMCPPublishReviewFindings, map[string]interface{}{
		"task_id":  "task-bad",
		"findings": []interface{}{validFindingPayload(), bad},
	})
	resp, err := h.handlePublishReviewFindings(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)

	stored, err := reviewSvc.GetTaskReview(context.Background(), "task-bad")
	require.NoError(t, err)
	require.Empty(t, stored.Findings, "a rejected batch must store nothing")
	require.Empty(t, stored.Runs, "a rejected batch must not create a run")
}

func TestHandlePublishReviewFindings_ValidationErrors(t *testing.T) {
	h, _, _ := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-val")

	cases := map[string]map[string]interface{}{
		"missing task_id":  {"findings": []interface{}{validFindingPayload()}},
		"empty findings":   {"task_id": "task-val", "findings": []interface{}{}},
		"missing findings": {"task_id": "task-val"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			msg := makeWSMessage(t, ws.ActionMCPPublishReviewFindings, payload)
			resp, err := h.handlePublishReviewFindings(context.Background(), msg)
			require.NoError(t, err)
			assertWSError(t, resp, ws.ErrorCodeValidation)
		})
	}
}

func TestHandlePublishReviewFindings_MultiRepoKeepsRepositoryName(t *testing.T) {
	h, reviewSvc, _ := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-multi")

	payload := validFindingPayload()
	payload["repo"] = "backend"
	msg := makeWSMessage(t, ws.ActionMCPPublishReviewFindings, map[string]interface{}{
		"task_id":  "task-multi",
		"findings": []interface{}{payload},
	})
	resp, err := h.handlePublishReviewFindings(context.Background(), msg)
	require.NoError(t, err)
	requireWSSuccess(t, resp)

	stored, err := reviewSvc.GetTaskReview(context.Background(), "task-multi")
	require.NoError(t, err)
	require.Len(t, stored.Findings, 1)
	require.Equal(t, "backend", stored.Findings[0].RepositoryName)
}

func TestHandleRunTaskReview_ReturnsPendingRun(t *testing.T) {
	h, _, runner := newReviewHandlers(t)
	runner.run = &models.TaskReviewRun{ID: "run-1", TaskID: "task-run", Status: models.ReviewRunPending}

	msg := makeWSMessage(t, ws.ActionTaskReviewRun, map[string]interface{}{
		"task_id":          "task-run",
		"agent_profile_id": "profile-9",
	})
	resp, err := h.handleRunTaskReview(context.Background(), msg)
	require.NoError(t, err)
	requireWSSuccess(t, resp)
	require.Len(t, runner.launched, 1)
	require.Equal(t, "profile-9", runner.launched[0].AgentProfileID)
	require.Equal(t, models.ReviewTriggerManual, runner.launched[0].Trigger)
}

func TestHandleRunTaskReview_SurfacesMachineReadableCodes(t *testing.T) {
	cases := map[string]struct {
		err  error
		code string
	}{
		"no capable agent":      {review.ErrAgentUnavailable, review.CodeAgentUnavailable},
		"nothing to review":     {review.ErrNoChanges, review.CodeNoChanges},
		"workspace unavailable": {review.ErrWorkspaceUnavailable, review.CodeWorkspaceUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h, _, runner := newReviewHandlers(t)
			runner.err = tc.err

			msg := makeWSMessage(t, ws.ActionTaskReviewRun, map[string]interface{}{"task_id": "task-x"})
			resp, err := h.handleRunTaskReview(context.Background(), msg)
			require.NoError(t, err)
			assertWSError(t, resp, ws.ErrorCodeValidation)
			require.Equal(t, tc.code, wsErrorDetails(t, resp)["code"],
				"the UI branches on this code to show an actionable message")
		})
	}
}

func TestHandleRunTaskReview_UnexpectedErrorIsInternal(t *testing.T) {
	h, _, runner := newReviewHandlers(t)
	runner.err = errors.New("boom")

	msg := makeWSMessage(t, ws.ActionTaskReviewRun, map[string]interface{}{"task_id": "task-x"})
	resp, err := h.handleRunTaskReview(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
}

func TestHandleRunTaskReview_RequiresTaskID(t *testing.T) {
	h, _, _ := newReviewHandlers(t)
	msg := makeWSMessage(t, ws.ActionTaskReviewRun, map[string]interface{}{})
	resp, err := h.handleRunTaskReview(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleUpdateReviewFinding(t *testing.T) {
	h, reviewSvc, _ := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-upd")
	_, findings, err := reviewSvc.PublishFindings(context.Background(), service.PublishFindingsRequest{
		TaskID: "task-upd",
		Findings: []service.ReviewFindingInput{{
			FilePath: "a.go", StartLine: 1, EndLine: 1, Severity: "minor",
			Category: "c", Title: "t", Body: "b",
		}},
	})
	require.NoError(t, err)

	msg := makeWSMessage(t, ws.ActionTaskReviewFindingUpdate, map[string]interface{}{
		"finding_id": findings[0].ID,
		"status":     "resolved",
	})
	resp, err := h.handleUpdateReviewFinding(context.Background(), msg)
	require.NoError(t, err)
	requireWSSuccess(t, resp)

	t.Run("unknown status is rejected", func(t *testing.T) {
		bad := makeWSMessage(t, ws.ActionTaskReviewFindingUpdate, map[string]interface{}{
			"finding_id": findings[0].ID, "status": "archived",
		})
		resp, err := h.handleUpdateReviewFinding(context.Background(), bad)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeValidation)
	})

	t.Run("unknown finding is not found", func(t *testing.T) {
		missing := makeWSMessage(t, ws.ActionTaskReviewFindingUpdate, map[string]interface{}{
			"finding_id": "nope", "status": "open",
		})
		resp, err := h.handleUpdateReviewFinding(context.Background(), missing)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeNotFound)
	})

	t.Run("missing finding_id is rejected", func(t *testing.T) {
		blank := makeWSMessage(t, ws.ActionTaskReviewFindingUpdate, map[string]interface{}{"status": "open"})
		resp, err := h.handleUpdateReviewFinding(context.Background(), blank)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeValidation)
	})
}

func TestHandleGetAndClearTaskReview(t *testing.T) {
	h, reviewSvc, _ := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-get")
	_, _, err := reviewSvc.PublishFindings(context.Background(), service.PublishFindingsRequest{
		TaskID: "task-get",
		Findings: []service.ReviewFindingInput{{
			FilePath: "a.go", StartLine: 1, EndLine: 1, Severity: "minor",
			Category: "c", Title: "t", Body: "b",
		}},
	})
	require.NoError(t, err)

	getMsg := makeWSMessage(t, ws.ActionTaskReviewGet, map[string]interface{}{"task_id": "task-get"})
	resp, err := h.handleGetTaskReview(context.Background(), getMsg)
	require.NoError(t, err)
	requireWSSuccess(t, resp)

	clearMsg := makeWSMessage(t, ws.ActionTaskReviewClear, map[string]interface{}{"task_id": "task-get"})
	resp, err = h.handleClearTaskReview(context.Background(), clearMsg)
	require.NoError(t, err)
	requireWSSuccess(t, resp)

	after, err := reviewSvc.GetTaskReview(context.Background(), "task-get")
	require.NoError(t, err)
	require.Empty(t, after.Findings)
	require.Empty(t, after.Runs)
}

func TestHandleCancelTaskReview(t *testing.T) {
	// Exercises the service-backed semantics directly. Runner routing (which also
	// stops live inference) has its own test below.
	h, reviewSvc, _ := newReviewHandlers(t)
	h.SetReviewRunner(nil)
	seedReviewHandlerTask(t, h, "task-cancel")
	run, err := reviewSvc.CreateRun(context.Background(), service.CreateRunRequest{TaskID: "task-cancel"})
	require.NoError(t, err)

	msg := makeWSMessage(t, ws.ActionTaskReviewCancel, map[string]interface{}{"run_id": run.ID})
	resp, err := h.handleCancelTaskReview(context.Background(), msg)
	require.NoError(t, err)
	requireWSSuccess(t, resp)

	t.Run("missing run_id", func(t *testing.T) {
		blank := makeWSMessage(t, ws.ActionTaskReviewCancel, map[string]interface{}{})
		resp, err := h.handleCancelTaskReview(context.Background(), blank)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeValidation)
	})

	t.Run("unknown run", func(t *testing.T) {
		missing := makeWSMessage(t, ws.ActionTaskReviewCancel, map[string]interface{}{"run_id": "nope"})
		resp, err := h.handleCancelTaskReview(context.Background(), missing)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeNotFound)
	})
}

// TestHandleCancelTaskReview_RoutesThroughRunner pins the fix for cancel only
// marking the DB row: the handler must go through the runner, which also cancels
// the live inference context, otherwise a finishing pass overwrites the status.
func TestHandleCancelTaskReview_RoutesThroughRunner(t *testing.T) {
	h, reviewSvc, runner := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-runner-cancel")
	run, err := reviewSvc.CreateRun(context.Background(), service.CreateRunRequest{TaskID: "task-runner-cancel"})
	require.NoError(t, err)
	runner.cancelRun = run

	msg := makeWSMessage(t, ws.ActionTaskReviewCancel, map[string]interface{}{"run_id": run.ID})
	resp, err := h.handleCancelTaskReview(context.Background(), msg)
	require.NoError(t, err)
	requireWSSuccess(t, resp)
	require.Equal(t, []string{run.ID}, runner.cancelled,
		"cancel must reach the runner so the live pass stops, not just the DB row")
}

func TestHandleCancelTaskReview_FallsBackToServiceWithoutRunner(t *testing.T) {
	h, reviewSvc, _ := newReviewHandlers(t)
	h.SetReviewRunner(nil)
	seedReviewHandlerTask(t, h, "task-no-runner")
	run, err := reviewSvc.CreateRun(context.Background(), service.CreateRunRequest{TaskID: "task-no-runner"})
	require.NoError(t, err)

	msg := makeWSMessage(t, ws.ActionTaskReviewCancel, map[string]interface{}{"run_id": run.ID})
	resp, err := h.handleCancelTaskReview(context.Background(), msg)
	require.NoError(t, err)
	requireWSSuccess(t, resp)
}
