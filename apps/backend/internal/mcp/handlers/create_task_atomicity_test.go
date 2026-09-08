package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// unknownReferenceUUID is the id from the incident report: syntactically valid,
// but a task_repositories join-row id rather than a repositories.id, so it
// resolves to nothing.
const unknownReferenceUUID = "1f63b0c4-892d-44ea-a30c-f2a0300298bb"

type atomicityFixture struct {
	svc         *service.Service
	blockers    *memBlockerRepo
	repo        *sqliterepo.Repository
	handlers    *Handlers
	workspaceID string
	workflowID  string
}

type noOpContributionDestinationPreparer struct{}

func (*noOpContributionDestinationPreparer) PrepareContributionDestination(
	context.Context,
	*service.CreateTaskRequest,
	*models.Workflow,
	[]*models.Repository,
) error {
	return nil
}

func newAtomicityFixture(t *testing.T) *atomicityFixture {
	t.Helper()
	svc, repo := newTestTaskService(t)
	blockers := &memBlockerRepo{}
	svc.SetBlockerRepository(blockers)
	ctx := context.Background()
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	workflows, err := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	return &atomicityFixture{
		svc:         svc,
		blockers:    blockers,
		repo:        repo,
		handlers:    NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t)),
		workspaceID: workspaces[0].ID,
		workflowID:  workflows[0].ID,
	}
}

// newRepository inserts a repository row directly. Service-level
// CreateRepository additionally validates that local_path is a real git
// checkout, which is beside the point here: these tests only need an id that
// resolves.
func (f *atomicityFixture) newRepository(t *testing.T, name string) *models.Repository {
	t.Helper()
	repository := &models.Repository{WorkspaceID: f.workspaceID, Name: name}
	require.NoError(t, f.repo.CreateRepository(context.Background(), repository))
	return repository
}

// createdTaskID creates a task through the handler and returns its id.
func (f *atomicityFixture) createdTaskID(t *testing.T, title string) string {
	t.Helper()
	resp := f.create(t, map[string]interface{}{"title": title})
	require.NotEqual(t, ws.MessageTypeError, resp.Type, "setup create failed: %s", string(resp.Payload))
	var created map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &created))
	id, _ := created["id"].(string)
	require.NotEmpty(t, id)
	return id
}

func (f *atomicityFixture) taskCount(t *testing.T) int {
	t.Helper()
	tasks, err := f.svc.ListTasks(context.Background(), f.workflowID)
	require.NoError(t, err)
	return len(tasks)
}

func (f *atomicityFixture) create(t *testing.T, overrides map[string]interface{}) *ws.Message {
	return f.createWithContext(t, context.Background(), overrides)
}

func (f *atomicityFixture) createWithContext(
	t *testing.T,
	ctx context.Context,
	overrides map[string]interface{},
) *ws.Message {
	t.Helper()
	payload := map[string]interface{}{
		"workspace_id":     f.workspaceID,
		"workflow_id":      f.workflowID,
		"title":            "Task",
		"description":      "Do the thing",
		"agent_profile_id": "profile-1",
		"start_agent":      false,
	}
	for k, v := range overrides {
		payload[k] = v
	}
	resp, err := f.handlers.handleCreateTask(ctx, makeWSMessage(t, ws.ActionMCPCreateTask, payload))
	require.NoError(t, err)
	require.NotNil(t, resp)
	return resp
}

// memBlockerRepo is an in-memory BlockerRepository. The SQLite task repository
// does not implement the interface (blocker rows are Office-owned), and these
// tests only need edges to be creatable and readable back.
type memBlockerRepo struct {
	edges []*orchmodels.TaskBlocker
	// failCreateAfter makes the (failCreateAfter+1)-th CreateTaskBlocker fail,
	// standing in for a blocker deleted between pre-insert validation and the
	// post-insert write. Zero disables the injection.
	failCreateAfter int
	creates         int
	cancelOnFailure context.CancelFunc
	cleanupCtxErr   error
	cleanupDeadline bool
}

func (m *memBlockerRepo) CreateTaskBlocker(_ context.Context, blocker *orchmodels.TaskBlocker) error {
	m.creates++
	if m.failCreateAfter > 0 && m.creates > m.failCreateAfter {
		if m.cancelOnFailure != nil {
			m.cancelOnFailure()
		}
		return errors.New("blocker vanished")
	}
	m.edges = append(m.edges, blocker)
	return nil
}

// DeleteTaskBlockersForTask satisfies the optional taskDependencyCleaner seam
// the service uses to tear down edges for a deleted task.
func (m *memBlockerRepo) DeleteTaskBlockersForTask(ctx context.Context, taskID string) error {
	m.cleanupCtxErr = ctx.Err()
	_, m.cleanupDeadline = ctx.Deadline()
	kept := m.edges[:0]
	for _, edge := range m.edges {
		if edge.TaskID != taskID && edge.BlockerTaskID != taskID {
			kept = append(kept, edge)
		}
	}
	m.edges = kept
	return nil
}

func (m *memBlockerRepo) ListTaskBlockers(_ context.Context, taskID string) ([]*orchmodels.TaskBlocker, error) {
	var out []*orchmodels.TaskBlocker
	for _, edge := range m.edges {
		if edge.TaskID == taskID {
			out = append(out, edge)
		}
	}
	return out, nil
}

func (m *memBlockerRepo) DeleteTaskBlocker(_ context.Context, taskID, blockerTaskID string) error {
	for i, edge := range m.edges {
		if edge.TaskID == taskID && edge.BlockerTaskID == blockerTaskID {
			m.edges = append(m.edges[:i], m.edges[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *memBlockerRepo) ListTasksBlockedBy(_ context.Context, blockerTaskID string) ([]string, error) {
	var out []string
	for _, edge := range m.edges {
		if edge.BlockerTaskID == blockerTaskID {
			out = append(out, edge.TaskID)
		}
	}
	return out, nil
}

func (m *memBlockerRepo) ListBlockersForTasks(_ context.Context, taskIDs []string) (map[string][]string, error) {
	out := make(map[string][]string)
	for _, id := range taskIDs {
		for _, edge := range m.edges {
			if edge.TaskID == id {
				out[id] = append(out[id], edge.BlockerTaskID)
			}
		}
	}
	return out, nil
}

func (m *memBlockerRepo) ListDependentsForTasks(_ context.Context, blockerTaskIDs []string) (map[string][]string, error) {
	out := make(map[string][]string)
	for _, id := range blockerTaskIDs {
		for _, edge := range m.edges {
			if edge.BlockerTaskID == id {
				out[id] = append(out[id], edge.TaskID)
			}
		}
	}
	return out, nil
}

// TestHandleCreateTask_UnknownRepositoryIDLeavesNoOrphan is the regression for
// the reported defect. The handler inserted the task row, THEN resolved the
// repository, THEN returned an error — so a caller that correctly retried on
// failure ended up with two cards.
//
// Asserting only the error is insufficient: the whole defect is the row that
// survives it, so the row count is asserted directly on both sides of the call.
func TestHandleCreateTask_UnknownRepositoryIDLeavesNoOrphan(t *testing.T) {
	f := newAtomicityFixture(t)
	before := f.taskCount(t)

	resp := f.create(t, map[string]interface{}{
		"repositories": []map[string]interface{}{{"repository_id": unknownReferenceUUID}},
	})

	failure := errorPayload(t, resp)
	require.Equal(t, before, f.taskCount(t), "failed create must leave no orphan task row")

	// The error must be actionable on its own: a caller who cannot read the
	// backend log still has to be able to fix their call from this message.
	require.Equal(t, ws.ErrorCodeValidation, failure.Code, "an unresolvable caller-supplied id is a validation failure")
	require.NotEqual(t, "Failed to create task", failure.Message, "the bare message names neither field nor value")
	require.Contains(t, failure.Message, "repositories[0].repository_id", "error must name the failing field")
	require.Contains(t, failure.Message, unknownReferenceUUID, "error must name the offending value")
}

// TestHandleCreateTask_UnknownBlockerLeavesNoOrphan covers the other reference
// resolved after the insert. blocked_by went through AddBlocker in
// finalizeCreatedTask, so an unknown blocker stranded a task exactly as an
// unknown repository did.
func TestHandleCreateTask_UnknownBlockerLeavesNoOrphan(t *testing.T) {
	f := newAtomicityFixture(t)
	before := f.taskCount(t)

	resp := f.create(t, map[string]interface{}{
		"blocked_by": []string{unknownReferenceUUID},
	})

	failure := errorPayload(t, resp)
	require.Equal(t, before, f.taskCount(t), "failed create must leave no orphan task row")
	require.Equal(t, ws.ErrorCodeValidation, failure.Code)
	require.Contains(t, failure.Message, "blocked_by[0]", "error must name the failing field")
	require.Contains(t, failure.Message, unknownReferenceUUID, "error must name the offending value")
}

// TestHandleCreateTask_UnknownRepositoryIDIsNotPartiallyApplied guards the
// mixed case: a good repository listed before a bad one must not leave the
// task, or the first association, behind.
func TestHandleCreateTask_UnknownRepositoryIDIsNotPartiallyApplied(t *testing.T) {
	f := newAtomicityFixture(t)
	good := f.newRepository(t, "good-repo")
	before := f.taskCount(t)

	resp := f.create(t, map[string]interface{}{
		"repositories": []map[string]interface{}{
			{"repository_id": good.ID},
			{"repository_id": unknownReferenceUUID},
		},
	})

	failure := errorPayload(t, resp)
	require.Equal(t, before, f.taskCount(t), "a later bad reference must not strand the task")
	require.Equal(t, ws.ErrorCodeValidation, failure.Code)
	require.Contains(t, failure.Message, "repositories[1].repository_id", "error must name the failing index")
}

// TestHandleCreateTask_UnknownRepositoryInTemplatedWorkflowIsValidation guards
// the contribution-destination preparation path. That path runs before the
// main reference resolver for templated workflows, so it must use the same
// caller-facing reference error when its repository lookup fails.
func TestHandleCreateTask_UnknownRepositoryInTemplatedWorkflowIsValidation(t *testing.T) {
	f := newAtomicityFixture(t)
	workflow, err := f.repo.GetWorkflow(context.Background(), f.workflowID)
	require.NoError(t, err)
	templateID := "templated-workflow"
	workflow.WorkflowTemplateID = &templateID
	require.NoError(t, f.repo.UpdateWorkflow(context.Background(), workflow))
	f.svc.SetContributionDestinationPreparer(&noOpContributionDestinationPreparer{})

	resp := f.create(t, map[string]interface{}{
		"repositories": []map[string]interface{}{{"repository_id": unknownReferenceUUID}},
	})

	failure := errorPayload(t, resp)
	require.Equal(t, ws.ErrorCodeValidation, failure.Code)
	require.Contains(t, failure.Message, "repositories[0].repository_id")
	require.Contains(t, failure.Message, unknownReferenceUUID)
	require.Equal(t, 0, f.taskCount(t), "failed create must leave no orphan task row")
}

// TestHandleCreateTask_BlockedByWithDeferredLaunchStillSucceeds is the
// no-behaviour-change guard for the exact request shape that produced the
// orphan on the live board: blocked_by plus a deferred launch. Moving
// reference resolution ahead of the insert must not disturb it.
func TestHandleCreateTask_BlockedByWithDeferredLaunchStillSucceeds(t *testing.T) {
	f := newAtomicityFixture(t)
	blockerResp := f.create(t, map[string]interface{}{"title": "Blocker"})
	require.NotEqual(t, ws.MessageTypeError, blockerResp.Type, "setup create failed: %s", string(blockerResp.Payload))
	var blocker map[string]interface{}
	require.NoError(t, json.Unmarshal(blockerResp.Payload, &blocker))
	blockerID, _ := blocker["id"].(string)
	require.NotEmpty(t, blockerID)

	repository := f.newRepository(t, "repo")

	before := f.taskCount(t)
	resp := f.create(t, map[string]interface{}{
		"title":        "Blocked child",
		"blocked_by":   []string{blockerID},
		"start_agent":  true,
		"repositories": []map[string]interface{}{{"repository_id": repository.ID}},
	})
	require.NotEqual(t, ws.MessageTypeError, resp.Type, "create failed: %s", string(resp.Payload))

	var created map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &created))
	require.Equal(t, before+1, f.taskCount(t), "a successful create must still insert exactly one task")

	// The repository association must still be persisted after the insert.
	taskID, _ := created["id"].(string)
	require.NotEmpty(t, taskID)
	task, err := f.svc.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	require.Len(t, task.Repositories, 1, "resolved repositories must still be written")
	require.Equal(t, repository.ID, task.Repositories[0].RepositoryID)

	blockers, err := f.svc.GetBlockers(context.Background(), taskID)
	require.NoError(t, err)
	require.Equal(t, []string{blockerID}, blockers, "blocker edge must still be created")
}

// TestHandleCreateTask_RollbackAlsoRemovesDependencyEdges covers the residual
// TOCTOU path, where a blocker passes pre-insert validation and is gone by the
// time the edge is written.
//
// blocked_by is written one edge at a time, so failing on the second entry
// leaves the first already persisted. task_blockers predates the tasks foreign
// key and nothing cascades, so a rollback that deleted only the task row would
// swap an orphan task for an orphan edge pointing at a task that no longer
// exists.
func TestHandleCreateTask_RollbackAlsoRemovesDependencyEdges(t *testing.T) {
	f := newAtomicityFixture(t)
	first := f.createdTaskID(t, "Blocker one")
	second := f.createdTaskID(t, "Blocker two")

	before := f.taskCount(t)
	f.blockers.failCreateAfter = 1

	resp := f.create(t, map[string]interface{}{
		"title":      "Blocked child",
		"blocked_by": []string{first, second},
	})

	errorPayload(t, resp)
	require.Equal(t, before, f.taskCount(t), "failed create must leave no orphan task row")
	require.Empty(t, f.blockers.edges, "rollback must also remove the edge that was already written")
}

func TestHandleCreateTask_RollbackSurvivesCallerCancellation(t *testing.T) {
	f := newAtomicityFixture(t)
	first := f.createdTaskID(t, "Blocker one")
	second := f.createdTaskID(t, "Blocker two")

	ctx, cancel := context.WithCancel(context.Background())
	f.blockers.failCreateAfter = 1
	f.blockers.cancelOnFailure = cancel
	before := f.taskCount(t)

	resp := f.createWithContext(t, ctx, map[string]interface{}{
		"title":      "Blocked child",
		"blocked_by": []string{first, second},
	})

	errorPayload(t, resp)
	require.Equal(t, before, f.taskCount(t), "rollback must delete the partial task after caller cancellation")
	require.NoError(t, f.blockers.cleanupCtxErr, "rollback cleanup must not inherit caller cancellation")
	require.True(t, f.blockers.cleanupDeadline, "rollback cleanup must remain bounded")
}
