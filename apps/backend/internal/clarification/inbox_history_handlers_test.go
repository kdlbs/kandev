package clarification

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// fakeInboxBundleStoreWithHistory adds the History read's two entry points on
// top of the existing fakeInboxBundleStore double, so the operational-inbox
// fixtures stay untouched.
type fakeInboxBundleStoreWithHistory struct {
	*fakeInboxBundleStore

	historyPage     *taskmodels.ClarificationHistoryPage
	historyListErr  error
	historyLastOpts taskmodels.ListClarificationHistoryOptions

	historyTotal    int
	historyTotalErr error
}

func (f *fakeInboxBundleStoreWithHistory) ListInboxHistoryBundles(
	_ context.Context, opts taskmodels.ListClarificationHistoryOptions,
) (*taskmodels.ClarificationHistoryPage, error) {
	f.historyLastOpts = opts
	if f.historyListErr != nil {
		return nil, f.historyListErr
	}
	if f.historyPage == nil {
		return &taskmodels.ClarificationHistoryPage{}, nil
	}
	return f.historyPage, nil
}

func (f *fakeInboxBundleStoreWithHistory) CountInboxHistoryBundles(
	context.Context, taskmodels.ListClarificationHistoryOptions,
) (int, error) {
	if f.historyTotalErr != nil {
		return 0, f.historyTotalErr
	}
	return f.historyTotal, nil
}

// fakeInboxTasksWithWorkflow adds GetWorkflowStep on top of the existing
// fakeInboxTasks double.
type fakeInboxTasksWithWorkflow struct {
	*fakeInboxTasks

	steps   map[string]*wfmodels.WorkflowStep
	stepErr error
}

func (f *fakeInboxTasksWithWorkflow) GetWorkflowStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	if f.stepErr != nil {
		return nil, f.stepErr
	}
	return f.steps[stepID], nil
}

func newInboxHistoryTestHandler(
	t *testing.T,
	msgs map[string][]*taskmodels.Message,
	tasks *fakeInboxTasksWithWorkflow,
	bundles *fakeInboxBundleStoreWithHistory,
) *Handlers {
	t.Helper()
	gin.SetMode(gin.TestMode)
	bundles.stubMessageStore = stubMessageStore{messages: msgs}
	h := NewHandlers(NewStore(time.Minute), nil, &stubMessageCreator{}, &stubMessageStore{messages: msgs},
		&stubEventBus{}, nil, logger.Default(), tasks, bundles)
	h.now = func() time.Time { return testInboxNow }
	return h
}

func newHistoryQuestionMessage(pendingID, taskID, sessionID, questionID string, index int, created time.Time) *taskmodels.Message {
	return &taskmodels.Message{
		ID:            pendingID + "-msg",
		TaskSessionID: sessionID,
		TaskID:        taskID,
		Type:          taskmodels.MessageTypeClarificationRequest,
		Content:       "what would you like?",
		CreatedAt:     created,
		Metadata: map[string]any{
			"question_id":    questionID,
			"question_index": float64(index),
		},
	}
}

func TestHttpListInboxHistory_ReturnsHydratedBundles(t *testing.T) {
	created := testInboxNow.Add(-time.Hour)
	msgs := map[string][]*taskmodels.Message{
		"pend-1": {newHistoryQuestionMessage("pend-1", "task-1", "sess-1", "q-1", 0, created)},
	}
	tasks := &fakeInboxTasksWithWorkflow{
		fakeInboxTasks: &fakeInboxTasks{
			tasks: map[string]*taskmodels.Task{
				"task-1": {ID: "task-1", Title: "Do the thing", WorkflowStepID: "step-1"},
			},
		},
		steps: map[string]*wfmodels.WorkflowStep{
			"step-1": {ID: "step-1"},
		},
	}
	bundles := &fakeInboxBundleStoreWithHistory{
		fakeInboxBundleStore: &fakeInboxBundleStore{},
		historyPage: &taskmodels.ClarificationHistoryPage{
			Bundles: []taskmodels.ClarificationHistoryBundleSummary{
				{
					PendingID:    "pend-1",
					TaskID:       "task-1",
					SessionID:    "sess-1",
					CreatedAt:    created,
					Reason:       taskmodels.ClarificationHistoryReasonSessionEnded,
					AskingTurnID: "turn-1",
				},
			},
		},
		historyTotal: 1,
	}
	h := newInboxHistoryTestHandler(t, msgs, tasks, bundles)

	rec := runInboxGet(h, "/api/v1/clarification-inbox/history?workspace_id=ws-1", h.httpListInboxHistory)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp inboxHistoryListResponse
	decodeInboxBody(t, rec, &resp)
	if len(resp.Bundles) != 1 {
		t.Fatalf("expected 1 bundle, got %d", len(resp.Bundles))
	}
	got := resp.Bundles[0]
	if got.PendingID != "pend-1" || got.Reason != "session_ended" || got.Kind != "clarification" {
		t.Fatalf("unexpected bundle view: %+v", got)
	}
	if got.AskingTurnID != "turn-1" {
		t.Fatalf("expected asking_turn_id turn-1, got %q", got.AskingTurnID)
	}
	if got.SupersedingTurnID != nil {
		t.Fatalf("expected no superseding turn id, got %v", *got.SupersedingTurnID)
	}
	if got.StepStartsNoAgent == nil || *got.StepStartsNoAgent != true {
		t.Fatalf("expected step_starts_no_agent=true, got %+v", got.StepStartsNoAgent)
	}
	if got.TaskTitle != "Do the thing" {
		t.Fatalf("expected task title hydrated, got %q", got.TaskTitle)
	}
	if resp.Total != 1 || resp.Count != 1 {
		t.Fatalf("expected total=1 count=1, got total=%d count=%d", resp.Total, resp.Count)
	}
}

func TestHttpListInboxHistory_SupersedingTurnIDOmittedWhenEmpty(t *testing.T) {
	created := testInboxNow.Add(-time.Hour)
	msgs := map[string][]*taskmodels.Message{
		"pend-1": {newHistoryQuestionMessage("pend-1", "task-1", "sess-1", "q-1", 0, created)},
	}
	tasks := &fakeInboxTasksWithWorkflow{fakeInboxTasks: &fakeInboxTasks{tasks: map[string]*taskmodels.Task{
		"task-1": {ID: "task-1", Title: "T"},
	}}}
	bundles := &fakeInboxBundleStoreWithHistory{
		fakeInboxBundleStore: &fakeInboxBundleStore{},
		historyPage: &taskmodels.ClarificationHistoryPage{
			Bundles: []taskmodels.ClarificationHistoryBundleSummary{
				{
					PendingID:         "pend-1",
					TaskID:            "task-1",
					SessionID:         "sess-1",
					CreatedAt:         created,
					Reason:            taskmodels.ClarificationHistoryReasonSuperseded,
					AskingTurnID:      "turn-1",
					SupersedingTurnID: "turn-2",
				},
			},
		},
	}
	h := newInboxHistoryTestHandler(t, msgs, tasks, bundles)

	rec := runInboxGet(h, "/api/v1/clarification-inbox/history?workspace_id=ws-1", h.httpListInboxHistory)

	var resp inboxHistoryListResponse
	decodeInboxBody(t, rec, &resp)
	if resp.Bundles[0].SupersedingTurnID == nil || *resp.Bundles[0].SupersedingTurnID != "turn-2" {
		t.Fatalf("expected superseding_turn_id turn-2, got %+v", resp.Bundles[0].SupersedingTurnID)
	}

	// A different bundle, same-turn permission supersession, carries no
	// separate superseding turn (AC .10): the field stays nil/omitted.
	bundles.historyPage.Bundles[0].SupersedingTurnID = ""
	rec2 := runInboxGet(h, "/api/v1/clarification-inbox/history?workspace_id=ws-1", h.httpListInboxHistory)
	var resp2 inboxHistoryListResponse
	decodeInboxBody(t, rec2, &resp2)
	if resp2.Bundles[0].SupersedingTurnID != nil {
		t.Fatalf("expected omitted superseding_turn_id, got %v", *resp2.Bundles[0].SupersedingTurnID)
	}
}

func TestHttpListInboxHistory_MissingWorkspaceIDRejectedWith400(t *testing.T) {
	h := newInboxHistoryTestHandler(t, nil,
		&fakeInboxTasksWithWorkflow{fakeInboxTasks: &fakeInboxTasks{}},
		&fakeInboxBundleStoreWithHistory{fakeInboxBundleStore: &fakeInboxBundleStore{}})

	rec := runInboxGet(h, "/api/v1/clarification-inbox/history", h.httpListInboxHistory)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing workspace_id, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpListInboxHistory_StepReadFailureOmitsLabel(t *testing.T) {
	created := testInboxNow.Add(-time.Hour)
	msgs := map[string][]*taskmodels.Message{
		"pend-1": {newHistoryQuestionMessage("pend-1", "task-1", "sess-1", "q-1", 0, created)},
	}
	tasks := &fakeInboxTasksWithWorkflow{
		fakeInboxTasks: &fakeInboxTasks{tasks: map[string]*taskmodels.Task{
			"task-1": {ID: "task-1", Title: "T", WorkflowStepID: "step-missing"},
		}},
		stepErr: context.DeadlineExceeded,
	}
	bundles := &fakeInboxBundleStoreWithHistory{
		fakeInboxBundleStore: &fakeInboxBundleStore{},
		historyPage: &taskmodels.ClarificationHistoryPage{
			Bundles: []taskmodels.ClarificationHistoryBundleSummary{
				{PendingID: "pend-1", TaskID: "task-1", SessionID: "sess-1", CreatedAt: created, Reason: taskmodels.ClarificationHistoryReasonUnreadable, AskingTurnID: "turn-1"},
			},
		},
	}
	h := newInboxHistoryTestHandler(t, msgs, tasks, bundles)

	rec := runInboxGet(h, "/api/v1/clarification-inbox/history?workspace_id=ws-1", h.httpListInboxHistory)

	var resp inboxHistoryListResponse
	decodeInboxBody(t, rec, &resp)
	if resp.Bundles[0].StepStartsNoAgent != nil {
		t.Fatalf("expected step_starts_no_agent omitted on read failure, got %v", *resp.Bundles[0].StepStartsNoAgent)
	}
}

func TestHttpListInboxHistory_ListErrorReturns500(t *testing.T) {
	h := newInboxHistoryTestHandler(t, nil,
		&fakeInboxTasksWithWorkflow{fakeInboxTasks: &fakeInboxTasks{}},
		&fakeInboxBundleStoreWithHistory{fakeInboxBundleStore: &fakeInboxBundleStore{}, historyListErr: context.DeadlineExceeded})

	rec := runInboxGet(h, "/api/v1/clarification-inbox/history?workspace_id=ws-1", h.httpListInboxHistory)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHttpListInboxHistory_CountErrorReturns500(t *testing.T) {
	h := newInboxHistoryTestHandler(t, nil,
		&fakeInboxTasksWithWorkflow{fakeInboxTasks: &fakeInboxTasks{}},
		&fakeInboxBundleStoreWithHistory{fakeInboxBundleStore: &fakeInboxBundleStore{}, historyTotalErr: context.DeadlineExceeded})

	rec := runInboxGet(h, "/api/v1/clarification-inbox/history?workspace_id=ws-1", h.httpListInboxHistory)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestOrderInboxHistoryMessages_ThreeKeySort(t *testing.T) {
	a := &taskmodels.Message{ID: "a", Metadata: map[string]any{"question_id": "z", "question_index": float64(0)}}
	b := &taskmodels.Message{ID: "b", Metadata: map[string]any{"question_id": "y", "question_index": float64(0)}}
	c := &taskmodels.Message{ID: "c", Metadata: map[string]any{"question_index": float64(1)}}

	got := orderInboxHistoryMessages([]*taskmodels.Message{c, a, b})

	if got[0].ID != "b" || got[1].ID != "a" || got[2].ID != "c" {
		t.Fatalf("unexpected order: %v, %v, %v", got[0].ID, got[1].ID, got[2].ID)
	}
}

// TestBuildInboxHistoryBundleViews_FiltersReusedPendingIDMessagesByPermissionGroupKey
// pins that FindMessagesByPendingIDs returning both an old (unrelated,
// already-resolved) and a new permission message under one reused pending_id
// does not leak the old request's content into the new request's bundle:
// filterMessagesByPermissionGroupKey must narrow hydration down to the
// bundle's own PermissionGroupKey before rendering.
func TestBuildInboxHistoryBundleViews_FiltersReusedPendingIDMessagesByPermissionGroupKey(t *testing.T) {
	created := testInboxNow.Add(-time.Hour)
	oldMsg := &taskmodels.Message{
		ID: "perm-old-msg", TaskSessionID: "sess-1", TaskID: "task-1",
		Type: taskmodels.MessageTypePermissionRequest, Content: "old: delete /tmp?",
		CreatedAt: created,
		Metadata:  map[string]any{"request_id": "req-old"},
	}
	newMsg := &taskmodels.Message{
		ID: "perm-new-msg", TaskSessionID: "sess-1", TaskID: "task-1",
		Type: taskmodels.MessageTypePermissionRequest, Content: "new: run tests?",
		CreatedAt: created.Add(time.Minute),
		Metadata:  map[string]any{"request_id": "req-new"},
	}
	// FindMessagesByPendingIDs returns every message sharing the raw
	// pending_id, including the old, unrelated request's once a provider
	// reuses it.
	msgs := map[string][]*taskmodels.Message{
		"pend-reused": {oldMsg, newMsg},
	}
	tasks := &fakeInboxTasksWithWorkflow{fakeInboxTasks: &fakeInboxTasks{
		tasks: map[string]*taskmodels.Task{"task-1": {ID: "task-1", Title: "Do the thing"}},
	}}
	bundles := &fakeInboxBundleStoreWithHistory{fakeInboxBundleStore: &fakeInboxBundleStore{}}
	h := newInboxHistoryTestHandler(t, msgs, tasks, bundles)

	views, err := h.buildInboxHistoryBundleViews(context.Background(), []taskmodels.ClarificationHistoryBundleSummary{
		{
			PendingID:          "pend-reused",
			TaskID:             "task-1",
			SessionID:          "sess-1",
			CreatedAt:          created.Add(time.Minute),
			Reason:             taskmodels.ClarificationHistoryReasonSuperseded,
			AskingTurnID:       "turn-b",
			PermissionGroupKey: "req-new",
		},
	})
	if err != nil {
		t.Fatalf("buildInboxHistoryBundleViews: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("expected 1 view, got %d: %+v", len(views), views)
	}
	got := views[0]
	if len(got.Messages) != 1 {
		t.Fatalf("expected the reused pending_id's hydration to keep only the bundle's own request's message, got %d: %+v", len(got.Messages), got.Messages)
	}
	if got.Messages[0].ID != "perm-new-msg" {
		t.Fatalf("expected only the new request's message to render, got %q (old, unrelated request's content leaked through)", got.Messages[0].ID)
	}
}

func TestInboxHistoryBundleKind_PermissionVsClarification(t *testing.T) {
	perm := []*taskmodels.Message{{Type: taskmodels.MessageTypePermissionRequest}}
	if got := inboxHistoryBundleKind(perm); got != "permission" {
		t.Fatalf("expected permission, got %q", got)
	}
	clar := []*taskmodels.Message{{Type: taskmodels.MessageTypeClarificationRequest}}
	if got := inboxHistoryBundleKind(clar); got != "clarification" {
		t.Fatalf("expected clarification, got %q", got)
	}
	if got := inboxHistoryBundleKind(nil); got != "" {
		t.Fatalf("expected empty for no messages, got %q", got)
	}
}
