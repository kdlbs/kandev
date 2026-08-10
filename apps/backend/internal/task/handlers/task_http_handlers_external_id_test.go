package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// recordingOrchestrator is a non-nil OrchestratorStarter that records
// whether it was ever invoked, and by which intent. newIdempotencyTestHandlers
// otherwise leaves h.orchestrator nil, under which session preparation
// no-ops on its own nil check regardless of whether the caller should have
// reached it at all — which cannot distinguish "correctly skipped" from
// "incorrectly attempted, but swallowed by the nil guard". Wiring this in
// makes a missing no-side-effects guard observable: if a session-prepare or
// -dispatch path is reached, LaunchSession fires for real and launched
// flips true. intents distinguishes step 6 (IntentPrepare, synchronous —
// expected to run even for a CreatedIdentityLost outcome, since it runs
// before settlement discovers that outcome) from step 8
// (IntentStartCreated, asynchronous dispatch — must never run for any
// Found or CreatedIdentityLost outcome).
type recordingOrchestrator struct {
	mu       sync.Mutex
	launched bool
	intents  []orchestrator.SessionIntent
}

func (o *recordingOrchestrator) LaunchSession(_ context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	o.mu.Lock()
	o.launched = true
	o.intents = append(o.intents, req.Intent)
	o.mu.Unlock()
	return &orchestrator.LaunchSessionResponse{Success: true, SessionID: "should-not-have-launched"}, nil
}

func (o *recordingOrchestrator) EnsureSession(context.Context, string, ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
	o.mu.Lock()
	o.launched = true
	o.mu.Unlock()
	return &orchestrator.EnsureSessionResponse{Success: true, SessionID: "should-not-have-launched"}, nil
}

func (o *recordingOrchestrator) wasLaunched() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.launched
}

// wasAsyncDispatched reports whether the asynchronous start intent
// (create-sequence step 8) ever fired — the guarantee that actually matters
// for "no side effects on a found/lost outcome", independent of whether
// synchronous prepare (step 6) ran.
func (o *recordingOrchestrator) wasAsyncDispatched() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, intent := range o.intents {
		if intent == orchestrator.IntentStartCreated {
			return true
		}
	}
	return false
}

// idempotentCreateTaskRepo is a minimal in-memory TaskRepository fake that
// supports the external-id create-idempotency contract end to end (create,
// lookup, settle, release), so handler-level tests can exercise real
// Found/CreatedIdentityLost outcomes without a real database. It embeds
// mockRepository for every unrelated method's no-op stub.
type idempotentCreateTaskRepo struct {
	mockRepository
	mu               sync.Mutex
	tasks            map[string]*models.Task
	forceNextSettle0 bool // force the next SettleTaskExternalID call to report zero rows, once
}

func newIdempotentCreateTaskRepo() *idempotentCreateTaskRepo {
	return &idempotentCreateTaskRepo{tasks: map[string]*models.Task{}}
}

func (r *idempotentCreateTaskRepo) CreateTask(_ context.Context, task *models.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if task.ID == "" {
		task.ID = "task-" + task.Title
	}
	if task.ExternalID != "" {
		for _, existing := range r.tasks {
			if existing.WorkspaceID == task.WorkspaceID && existing.ExternalID == task.ExternalID {
				return taskrepo.ErrExternalIDConflict
			}
		}
	}
	stored := *task
	r.tasks[task.ID] = &stored
	return nil
}

func (r *idempotentCreateTaskRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok {
		return nil, taskrepo.ErrTaskNotFound
	}
	copied := *task
	return &copied, nil
}

func (r *idempotentCreateTaskRepo) UpdateTask(_ context.Context, task *models.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tasks[task.ID]; !ok {
		return taskrepo.ErrTaskNotFound
	}
	stored := *task
	r.tasks[task.ID] = &stored
	return nil
}

func (r *idempotentCreateTaskRepo) GetTaskByExternalID(_ context.Context, workspaceID, externalID string) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, task := range r.tasks {
		if task.WorkspaceID == workspaceID && task.ExternalID == externalID {
			copied := *task
			return &copied, nil
		}
	}
	return nil, taskrepo.ErrTaskNotFound
}

func (r *idempotentCreateTaskRepo) SettleTaskExternalID(_ context.Context, taskID, externalID string, settledAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.forceNextSettle0 {
		r.forceNextSettle0 = false
		// Simulate another actor releasing the identity concurrently: the
		// task survives but the columns are cleared, same as
		// ReleaseTaskExternalID's real effect.
		if task, ok := r.tasks[taskID]; ok {
			task.ExternalID = ""
			task.ExternalIDSettledAt = nil
		}
		return false, nil
	}
	task, ok := r.tasks[taskID]
	if !ok || task.ExternalID != externalID || task.ExternalIDSettledAt != nil {
		return false, nil
	}
	task.ExternalIDSettledAt = &settledAt
	return true, nil
}

func (r *idempotentCreateTaskRepo) ReleaseTaskExternalID(_ context.Context, workspaceID, externalID string) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, task := range r.tasks {
		if task.WorkspaceID == workspaceID && task.ExternalID == externalID {
			task.ExternalID = ""
			task.ExternalIDSettledAt = nil
			task.UpdatedAt = time.Now().UTC()
			copied := *task
			return &copied, nil
		}
	}
	return nil, nil
}

func (r *idempotentCreateTaskRepo) GetWorkflow(_ context.Context, id string) (*models.Workflow, error) {
	return &models.Workflow{ID: id, WorkspaceID: "ws-1"}, nil
}

func (r *idempotentCreateTaskRepo) GetStep(_ context.Context, id string) (*wfmodels.WorkflowStep, error) {
	return &wfmodels.WorkflowStep{ID: id, WorkflowID: "wf-1"}, nil
}

func (r *idempotentCreateTaskRepo) GetNextStepByPosition(context.Context, string, int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

func newIdempotencyTestHandlers(t *testing.T, repo *idempotentCreateTaskRepo) *TaskHandlers {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	svc.SetWorkflowStepGetter(repo)
	return &TaskHandlers{service: svc, logger: log}
}

func doCreateTask(h *TaskHandlers, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.httpCreateTask(c)
	return rec
}

// TestHTTPCreateTask_ExternalIDGoldenPath covers the golden path: a create
// carrying a fresh external_id reports deduplicated:false, creation_complete
// :true, and echoes external_id.
func TestHTTPCreateTask_ExternalIDGoldenPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	rec := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Task","external_id":"ext-1"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ext-1", resp["external_id"])
	assert.Equal(t, false, resp["deduplicated"])
	assert.Equal(t, true, resp["creation_complete"])

	// creation_complete:true above is a literal in httpCreateTask's Created
	// branch, not read back from the row — it would read true even if the
	// handler's own h.service.SettleExternalID call were deleted entirely.
	// Assert the row itself to pin that the handler's settlement call
	// actually ran and stamped it.
	repo.mu.Lock()
	stored, ok := repo.tasks[resp["id"].(string)]
	repo.mu.Unlock()
	require.True(t, ok, "created task must exist in the repo")
	assert.NotNil(t, stored.ExternalIDSettledAt, "the handler's own settlement call must have stamped external_id_settled_at")
}

// TestHTTPCreateTask_FoundSettledSkipsPostCreateWork covers the dedupe hit:
// the retry must not create a new session/attachment/branch side effect and
// must report deduplicated:true, creation_complete:true, echoing the
// existing task even though the retry payload differs (title, start_agent).
func TestHTTPCreateTask_FoundSettledSkipsPostCreateWork(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	first := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Original","external_id":"ext-1"}`)
	require.Equal(t, http.StatusOK, first.Code, "body: %s", first.Body.String())
	var firstResp map[string]interface{}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstResp))
	firstID := firstResp["id"].(string)

	// Settle it directly (simulating the create's own settlement having
	// already run), then retry with a very different payload.
	_, err := repo.SettleTaskExternalID(context.Background(), firstID, "ext-1", time.Now().UTC())
	require.NoError(t, err)

	// A non-nil recording orchestrator: with h.orchestrator left nil (as
	// newIdempotencyTestHandlers otherwise does), handlePostCreateTaskSession
	// no-ops on its own nil check regardless of whether the Found-outcome
	// guard above it even ran, so an empty session_id alone cannot tell
	// "correctly skipped" from "incorrectly attempted, but swallowed by the
	// nil guard". This makes a missing guard observable.
	rec := &recordingOrchestrator{}
	h.orchestrator = rec

	retry := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Changed","external_id":"ext-1","start_agent":true,"agent_profile_id":"agent-1"}`)
	require.Equal(t, http.StatusOK, retry.Code, "body: %s", retry.Body.String())

	var retryResp map[string]interface{}
	require.NoError(t, json.Unmarshal(retry.Body.Bytes(), &retryResp))
	assert.Equal(t, firstID, retryResp["id"])
	assert.Equal(t, "Original", retryResp["title"], "the existing task must be returned unchanged")
	assert.Equal(t, true, retryResp["deduplicated"])
	assert.Equal(t, true, retryResp["creation_complete"])
	assert.Nil(t, retryResp["session_id"], "no session should be prepared/started for a Found outcome")
	assert.False(t, rec.wasLaunched(), "the orchestrator must never be reached for a Found outcome, even with start_agent:true")

	repo.mu.Lock()
	count := 0
	for _, tk := range repo.tasks {
		if tk.WorkspaceID == "ws-1" && tk.ExternalID == "ext-1" {
			count++
		}
	}
	repo.mu.Unlock()
	assert.Equal(t, 1, count, "no duplicate task should exist")
}

// TestHTTPCreateTask_FoundOutcomeIgnoresNonexistentRepositoryInRetry pins
// the spec's documented answer to "what happens when a Found-outcome retry
// names a repository that no longer exists": convertCreateTaskRepositories
// (the handler-level pre-service conversion) only checks that an
// identifying field is present, never that the repository actually exists —
// existence validation lives in Service.createTaskRepositories, which a
// Found outcome's early return never reaches. So the retry payload's
// repositories are never even looked at; the existing task is returned as
// normal, not a validation failure.
func TestHTTPCreateTask_FoundOutcomeIgnoresNonexistentRepositoryInRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	first := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Original","external_id":"ext-1"}`)
	require.Equal(t, http.StatusOK, first.Code, "body: %s", first.Body.String())
	var firstResp map[string]interface{}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstResp))
	firstID := firstResp["id"].(string)

	_, err := repo.SettleTaskExternalID(context.Background(), firstID, "ext-1", time.Now().UTC())
	require.NoError(t, err)

	retry := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Retry","external_id":"ext-1","repositories":[{"repository_id":"repo-does-not-exist"}]}`)
	require.Equal(t, http.StatusOK, retry.Code, "body: %s", retry.Body.String())

	var retryResp map[string]interface{}
	require.NoError(t, json.Unmarshal(retry.Body.Bytes(), &retryResp))
	assert.Equal(t, firstID, retryResp["id"])
	assert.Equal(t, true, retryResp["deduplicated"])
	assert.Equal(t, true, retryResp["creation_complete"])
}

// TestHTTPCreateTask_FoundUnsettledReportsIncomplete covers the diagnostic
// tuple: deduplicated:true + creation_complete:false for an in-flight
// unsettled create.
func TestHTTPCreateTask_FoundUnsettledReportsIncomplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	// Seed a task holding ext-1 directly, bypassing the handler's own
	// create+settle flow, to represent a create whose required synchronous
	// work has not finished yet (e.g. it crashed before settling, or is
	// still genuinely running) — the scenario this outcome exists to report.
	repo.mu.Lock()
	repo.tasks["task-inflight"] = &models.Task{
		ID: "task-inflight", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Title: "In flight", ExternalID: "ext-1",
	}
	repo.mu.Unlock()

	retry := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Task","external_id":"ext-1"}`)
	require.Equal(t, http.StatusOK, retry.Code, "body: %s", retry.Body.String())

	var retryResp map[string]interface{}
	require.NoError(t, json.Unmarshal(retry.Body.Bytes(), &retryResp))
	assert.Equal(t, true, retryResp["deduplicated"])
	assert.Equal(t, false, retryResp["creation_complete"])
}

// TestHTTPCreateTask_InvalidExternalIDReturns400 covers validation failing
// before any task is created.
func TestHTTPCreateTask_InvalidExternalIDReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	rec := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Task","external_id":"ext-1\n"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	assert.Empty(t, repo.tasks, "no task should be created when external_id validation fails")
}

// TestHTTPCreateTask_CreatedIdentityLostOmitsExternalID covers the fourth
// outcome: settlement affecting zero rows because the identity was released
// mid-create. The task survives, no external_id in the body, no
// asynchronous work dispatched.
//
// Synchronous session prepare (create-sequence step 6) DOES run here, and
// must: it happens before settlement (step 7) discovers the CreatedIdentityLost
// outcome, per the settlement-ordering fix (docs/specs/tasks/external-id-idempotency/spec.md,
// "Settlement call site (normative, per surface)"). Only the asynchronous
// start dispatch (step 8) is guaranteed to never fire — that is the actual
// "no side effects" guarantee the spec makes for this outcome ("no
// asynchronous work (session start, PR association) is dispatched for it").
func TestHTTPCreateTask_CreatedIdentityLostOmitsExternalID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	// Force settlement to report zero rows for the task this create makes,
	// simulating another actor releasing the identity mid-create.
	repo.mu.Lock()
	repo.forceNextSettle0 = true
	repo.mu.Unlock()

	orch := &recordingOrchestrator{}
	h.orchestrator = orch

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(
		`{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Task","external_id":"ext-1","start_agent":true,"agent_profile_id":"agent-1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.httpCreateTask(c)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	_, hasExternalID := resp["external_id"]
	assert.False(t, hasExternalID, "external_id must be absent from a CreatedIdentityLost response")
	assert.Equal(t, false, resp["deduplicated"])
	assert.Equal(t, true, resp["creation_complete"])
	assert.Nil(t, resp["session_id"], "no session should be reported in a CreatedIdentityLost response, even though one may have been prepared")
	assert.False(t, orch.wasAsyncDispatched(), "the asynchronous start intent must never fire once settlement reports CreatedIdentityLost")
}

func doGetTaskByExternalID(h *TaskHandlers, workspaceID, externalID string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/workspaces/"+workspaceID+"/tasks/by-external-id?external_id="+externalID, nil)
	c.Params = gin.Params{{Key: "id", Value: workspaceID}}
	h.httpGetTaskByExternalID(c)
	return rec
}

func doReleaseTaskByExternalID(h *TaskHandlers, workspaceID, externalID string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodDelete, "/workspaces/"+workspaceID+"/tasks/by-external-id?external_id="+externalID, nil)
	c.Params = gin.Params{{Key: "id", Value: workspaceID}}
	h.httpReleaseTaskExternalID(c)
	// A bare gin.CreateTestContext (unlike a full router.ServeHTTP cycle)
	// never flushes a body-less status via c.Status(...) on its own —
	// WriteHeaderNow runs at end-of-request in the real pipeline.
	c.Writer.WriteHeaderNow()
	return rec
}

// TestHTTPGetTaskByExternalID_Found covers the lookup route's success path,
// including the unsettled case, and confirms the lookup is side-effect-free
// (no task created).
func TestHTTPGetTaskByExternalID_Found(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)
	repo.mu.Lock()
	repo.tasks["task-1"] = &models.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "T", ExternalID: "ext-1"}
	repo.mu.Unlock()

	rec := doGetTaskByExternalID(h, "ws-1", "ext-1")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "task-1", resp["id"])
	assert.Equal(t, false, resp["creation_complete"], "unsettled task must report creation_complete:false")
	_, hasDeduplicated := resp["deduplicated"]
	assert.False(t, hasDeduplicated, "lookup response carries no deduplicated field")
}

// TestHTTPGetTaskByExternalID_NotFound covers the miss path.
func TestHTTPGetTaskByExternalID_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	rec := doGetTaskByExternalID(h, "ws-1", "ext-nope")
	assert.Equal(t, http.StatusNotFound, rec.Code, "body: %s", rec.Body.String())
}

// TestHTTPGetTaskByExternalID_MissingParam covers the 400 validation path.
func TestHTTPGetTaskByExternalID_MissingParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	rec := doGetTaskByExternalID(h, "ws-1", "")
	assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
}

// TestHTTPReleaseTaskExternalID_Success covers release freeing the identity
// without deleting the task.
func TestHTTPReleaseTaskExternalID_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)
	repo.mu.Lock()
	repo.tasks["task-1"] = &models.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "T", ExternalID: "ext-1"}
	repo.mu.Unlock()

	rec := doReleaseTaskByExternalID(h, "ws-1", "ext-1")
	assert.Equal(t, http.StatusNoContent, rec.Code, "body: %s", rec.Body.String())

	repo.mu.Lock()
	task := repo.tasks["task-1"]
	repo.mu.Unlock()
	require.NotNil(t, task, "release must not delete the task")
	assert.Empty(t, task.ExternalID)

	// The lookup route must now report not found.
	lookupRec := doGetTaskByExternalID(h, "ws-1", "ext-1")
	assert.Equal(t, http.StatusNotFound, lookupRec.Code)
}

// TestHTTPReleaseTaskExternalID_NotFound covers releasing an identity
// nothing holds.
func TestHTTPReleaseTaskExternalID_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	rec := doReleaseTaskByExternalID(h, "ws-1", "ext-nope")
	assert.Equal(t, http.StatusNotFound, rec.Code, "body: %s", rec.Body.String())
}

// orderRecordingOrchestrator records whether the task was already settled
// (external_id_settled_at set) at the moment synchronous session prepare
// (orchestrator.IntentPrepare) was invoked — a direct, observable proof of
// create-sequence ordering (docs/specs/tasks/external-id-idempotency/spec.md,
// "Settlement call site (normative, per surface)"): settlement (step 7) must
// never precede synchronous session prepare (part of step 6). Only the
// asynchronous dispatch (step 8, the actual agent-start goroutine) may run
// after settlement.
type orderRecordingOrchestrator struct {
	repo              *idempotentCreateTaskRepo
	externalID        string
	mu                sync.Mutex
	prepareCalled     bool
	prepareSawSettled bool
}

func (o *orderRecordingOrchestrator) LaunchSession(_ context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	if req.Intent == orchestrator.IntentPrepare {
		o.mu.Lock()
		o.prepareCalled = true
		o.repo.mu.Lock()
		for _, tk := range o.repo.tasks {
			if tk.ExternalID == o.externalID {
				o.prepareSawSettled = tk.ExternalIDSettledAt != nil
				break
			}
		}
		o.repo.mu.Unlock()
		o.mu.Unlock()
	}
	return &orchestrator.LaunchSessionResponse{Success: true, SessionID: "sess-order-check"}, nil
}

func (o *orderRecordingOrchestrator) EnsureSession(context.Context, string, ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
	return &orchestrator.EnsureSessionResponse{Success: true, SessionID: "sess-order-check"}, nil
}

// TestHTTPCreateTask_SettlesAfterSynchronousSessionPrepareNotBefore is the
// human-review finding (carlosflorencio, PR #2440): settlement previously ran
// before handlePostCreateTaskSession was even called, not between its
// synchronous prepare and asynchronous dispatch halves as the spec requires.
// A crash in the gap between settle succeeding and prepare running would
// leave a task reporting creation_complete:true with no session ever
// prepared, and a retry would never attempt session-prep again since Found
// outcomes skip all post-create work by design.
func TestHTTPCreateTask_SettlesAfterSynchronousSessionPrepareNotBefore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)
	orch := &orderRecordingOrchestrator{repo: repo, externalID: "ext-order"}
	h.orchestrator = orch

	rec := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Task","external_id":"ext-order","start_agent":true,"agent_profile_id":"agent-1"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	orch.mu.Lock()
	prepareCalled, prepareSawSettled := orch.prepareCalled, orch.prepareSawSettled
	orch.mu.Unlock()
	require.True(t, prepareCalled, "synchronous session prepare must run for a start_agent create")
	assert.False(t, prepareSawSettled, "settlement must not have run before synchronous session prepare was invoked — settlement must sit between prepare and async dispatch, not before prepare")
}

// failingPrepareOrchestrator fails every LaunchSession call. Used to prove
// that a genuine prepare *failure* (as opposed to a process crash) does not
// block settlement — the create-sequence's session-prepare step is allowed
// to fail gracefully, matching the existing behavior for a create with no
// start_agent/prepare_session at all. Only a crash before prepare returns
// leaves the row unsettled; a returned error does not.
type failingPrepareOrchestrator struct {
	mu       sync.Mutex
	launched bool
}

func (o *failingPrepareOrchestrator) LaunchSession(context.Context, *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	o.mu.Lock()
	o.launched = true
	o.mu.Unlock()
	return nil, errPrepareFailed
}

func (o *failingPrepareOrchestrator) EnsureSession(context.Context, string, ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
	return nil, errPrepareFailed
}

var errPrepareFailed = errors.New("session prepare failed")

// TestHTTPCreateTask_SessionPrepareFailureStillSettles covers the
// intentionally narrow scope of the settlement-ordering fix: session
// prepare running *before* settlement means prepare is allowed to observe
// (and fail against) an unsettled row, but a graceful prepare failure must
// not block settlement — only a crash before prepare returns does, and that
// is covered by TestHTTPCreateTask_SettlesAfterSynchronousSessionPrepareNotBefore
// observing the row's state at the moment of the call. This test pins the
// other half: the create still succeeds and settles even though its agent
// session never got prepared, exactly like a plain create with no
// start_agent at all.
func TestHTTPCreateTask_SessionPrepareFailureStillSettles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)
	orch := &failingPrepareOrchestrator{}
	h.orchestrator = orch

	rec := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Task","external_id":"ext-prep-fail","start_agent":true,"agent_profile_id":"agent-1"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["creation_complete"], "a graceful session-prepare failure must not prevent settlement")
	assert.Nil(t, resp["session_id"], "no session_id should be reported when prepare failed")

	repo.mu.Lock()
	stored, ok := repo.tasks[resp["id"].(string)]
	repo.mu.Unlock()
	require.True(t, ok)
	assert.NotNil(t, stored.ExternalIDSettledAt, "settlement must still run after a graceful (non-crash) prepare failure")

	orch.mu.Lock()
	defer orch.mu.Unlock()
	assert.True(t, orch.launched, "prepare must have actually been attempted")
}

// TestHTTPUpdateTask_IgnoresExternalIDField pins the spec's Lifecycle
// requirement that "No API changes an external ID on an existing task;
// PATCH /tasks/:id ... do[es] not accept the field." httpUpdateTaskRequest
// has no ExternalID field at all, so a PATCH body carrying one is silently
// dropped by JSON unmarshaling — this test proves that holds end to end,
// not just by inspecting the struct.
func TestHTTPUpdateTask_IgnoresExternalIDField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newIdempotentCreateTaskRepo()
	h := newIdempotencyTestHandlers(t, repo)

	created := doCreateTask(h, `{"workspace_id":"ws-1","workflow_id":"wf-1","workflow_step_id":"step-1","title":"Task","external_id":"ext-1"}`)
	require.Equal(t, http.StatusOK, created.Code, "body: %s", created.Body.String())
	var createResp map[string]interface{}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &createResp))
	taskID := createResp["id"].(string)
	require.Equal(t, "ext-1", createResp["external_id"])

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: taskID}}
	c.Request = httptest.NewRequest(http.MethodPatch, "/tasks/"+taskID,
		strings.NewReader(`{"title":"Renamed","external_id":"ext-should-be-ignored"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.httpUpdateTask(c)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var updateResp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updateResp))
	assert.Equal(t, "Renamed", updateResp["title"], "the title field the request actually supports must still apply")
	assert.Equal(t, "ext-1", updateResp["external_id"], "external_id in a PATCH body must be silently ignored, not applied")

	repo.mu.Lock()
	stored := repo.tasks[taskID]
	repo.mu.Unlock()
	require.NotNil(t, stored)
	assert.Equal(t, "ext-1", stored.ExternalID, "the stored task's identity must be unchanged by the PATCH")
}
