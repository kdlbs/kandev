package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// newAutoOriginWorkflowStore builds a workflow repository over the same DB as
// repo, so these tests can create the step their fixture tasks sit on.
func newAutoOriginWorkflowStore(t *testing.T, repo *Repository) *workflowrepo.Repository {
	t.Helper()
	store, err := workflowrepo.NewWithDB(repo.db, repo.db, nil)
	if err != nil {
		t.Fatalf("initialize workflow repository: %v", err)
	}
	return store
}

// Automation runs are hidden from the board and from task lists by their
// origin, not by is_ephemeral: they are ordinary persistent tasks that keep
// their worktree and stay repliable, they just have their own destination.
// is_ephemeral keeps its original quick-chat meaning, so these tests pin both
// halves — the automation task disappears from every list read, and the quick
// chat next to it keeps behaving exactly as it did.
const (
	autoOriginWorkspaceID = "ws-auto-origin"
	autoOriginWorkflowID  = "wf-auto-origin"
	autoOriginStepID      = "step-auto-origin"
)

type autoOriginFixture struct {
	repo        *Repository
	boardTaskID string
	autoTaskID  string
	quickTaskID string
}

func seedAutomationOriginFixture(t *testing.T) *autoOriginFixture {
	t.Helper()
	ctx := context.Background()
	repo := newRepoForSessionTests(t)

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: autoOriginWorkspaceID, Name: "Automation origin"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: autoOriginWorkflowID, WorkspaceID: autoOriginWorkspaceID, Name: "WF"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := newAutoOriginWorkflowStore(t, repo).CreateStep(ctx, &wfmodels.WorkflowStep{
		ID: autoOriginStepID, WorkflowID: autoOriginWorkflowID, Name: "Todo", Position: 1,
	}); err != nil {
		t.Fatalf("CreateStep: %v", err)
	}

	create := func(id, title, origin string, ephemeral bool, withWorkflow bool) {
		t.Helper()
		task := &models.Task{
			ID:          id,
			WorkspaceID: autoOriginWorkspaceID,
			Title:       title,
			State:       "BACKLOG",
			Priority:    "medium",
			Origin:      origin,
			IsEphemeral: ephemeral,
		}
		if withWorkflow {
			task.WorkflowID = autoOriginWorkflowID
			task.WorkflowStepID = autoOriginStepID
		}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask(%s): %v", id, err)
		}
	}

	// The automation task sits in the same workflow step as the human task:
	// only its origin may keep it off the board.
	create("t-board", "human work", models.TaskOriginManual, false, true)
	create("t-auto", "nightly sweep", models.TaskOriginAutomationRun, false, true)
	create("t-quick", "quick chat", models.TaskOriginManual, true, false)

	return &autoOriginFixture{repo: repo, boardTaskID: "t-board", autoTaskID: "t-auto", quickTaskID: "t-quick"}
}

func taskIDs(tasks []*models.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func assertExcludesAutomation(t *testing.T, what string, tasks []*models.Task, wantPresent string) {
	t.Helper()
	ids := taskIDs(tasks)
	foundWanted := false
	for _, id := range ids {
		if id == "t-auto" {
			t.Fatalf("%s: automation-origin task must not appear, got %v", what, ids)
		}
		if id == wantPresent {
			foundWanted = true
		}
	}
	if wantPresent != "" && !foundWanted {
		t.Fatalf("%s: expected %s to still be listed, got %v", what, wantPresent, ids)
	}
}

// The kanban reads tasks by workflow and by workflow step. An automation run
// created against a workflow step must not surface in either.
func TestBoardQueriesExcludeAutomationOriginTasks(t *testing.T) {
	f := seedAutomationOriginFixture(t)
	ctx := context.Background()

	byWorkflow, err := f.repo.ListTasks(ctx, autoOriginWorkflowID)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	assertExcludesAutomation(t, "ListTasks", byWorkflow, f.boardTaskID)

	byStep, err := f.repo.ListTasksByWorkflowStep(ctx, autoOriginStepID)
	if err != nil {
		t.Fatalf("ListTasksByWorkflowStep: %v", err)
	}
	assertExcludesAutomation(t, "ListTasksByWorkflowStep", byStep, f.boardTaskID)

	count, err := f.repo.CountTasksByWorkflowStep(ctx, autoOriginStepID)
	if err != nil {
		t.Fatalf("CountTasksByWorkflowStep: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountTasksByWorkflowStep = %d, want 1 (the automation run must not occupy a column)", count)
	}
}

// The task list is the other surface an automation run must stay out of —
// including its search path, and including the "show me quick chats too"
// variant, since including ephemeral tasks is not a request to see automation
// output.
func TestTaskListQueriesExcludeAutomationOriginTasks(t *testing.T) {
	f := seedAutomationOriginFixture(t)
	ctx := context.Background()

	list, total, err := f.repo.ListTasksByWorkspace(ctx, autoOriginWorkspaceID, "", "", "", 1, 50, "", false, false, false, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace: %v", err)
	}
	assertExcludesAutomation(t, "ListTasksByWorkspace", list, f.boardTaskID)
	if total != 1 {
		t.Fatalf("ListTasksByWorkspace total = %d, want 1", total)
	}

	withEphemeral, _, err := f.repo.ListTasksByWorkspace(ctx, autoOriginWorkspaceID, "", "", "", 1, 50, "", false, true, false, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace(includeEphemeral): %v", err)
	}
	assertExcludesAutomation(t, "ListTasksByWorkspace(includeEphemeral)", withEphemeral, f.quickTaskID)

	searched, _, err := f.repo.ListTasksByWorkspace(ctx, autoOriginWorkspaceID, "", "", "sweep", 1, 50, "", false, false, false, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace(search): %v", err)
	}
	if len(searched) != 0 {
		t.Fatalf("search for the automation's own title must find nothing, got %v", taskIDs(searched))
	}

	tree, err := f.repo.ListTaskTree(ctx, autoOriginWorkspaceID, models.TaskTreeFilters{})
	if err != nil {
		t.Fatalf("ListTaskTree: %v", err)
	}
	assertExcludesAutomation(t, "ListTaskTree", tree, f.boardTaskID)
}

func TestTaskListQueriesIncludeVisibleAutomationTasks(t *testing.T) {
	f := seedAutomationOriginFixture(t)
	ctx := context.Background()
	const visibleID = "t-auto-visible"
	if err := f.repo.CreateTask(ctx, &models.Task{
		ID:             visibleID,
		WorkspaceID:    autoOriginWorkspaceID,
		WorkflowID:     autoOriginWorkflowID,
		WorkflowStepID: autoOriginStepID,
		Title:          "visible automation task",
		State:          "BACKLOG",
		Priority:       "medium",
		Origin:         models.TaskOriginAutomationTask,
	}); err != nil {
		t.Fatalf("CreateTask(%s): %v", visibleID, err)
	}

	list, _, err := f.repo.ListTasksByWorkspace(ctx, autoOriginWorkspaceID, "", "", "", 1, 50, "", false, false, false, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace: %v", err)
	}
	ids := taskIDs(list)
	if len(ids) != 2 || !containsTaskID(ids, "t-board") || !containsTaskID(ids, visibleID) {
		t.Fatalf("workspace task list = %v, want both t-board and %s", ids, visibleID)
	}

	byWorkflow, err := f.repo.ListTasks(ctx, autoOriginWorkflowID)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	workflowIDs := taskIDs(byWorkflow)
	if len(workflowIDs) != 2 || !containsTaskID(workflowIDs, "t-board") || !containsTaskID(workflowIDs, visibleID) {
		t.Fatalf("workflow task list = %v, want both t-board and %s", workflowIDs, visibleID)
	}
}

func containsTaskID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// Quick chat owns is_ephemeral and must be unaffected by the origin filter:
// the only-ephemeral listing still returns exactly the quick chat.
func TestOnlyEphemeralListingStillReturnsQuickChat(t *testing.T) {
	f := seedAutomationOriginFixture(t)
	ctx := context.Background()

	only, _, err := f.repo.ListTasksByWorkspace(ctx, autoOriginWorkspaceID, "", "", "", 1, 50, "", false, false, true, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace(onlyEphemeral): %v", err)
	}
	if got := taskIDs(only); len(got) != 1 || got[0] != f.quickTaskID {
		t.Fatalf("onlyEphemeral listing = %v, want [%s]", got, f.quickTaskID)
	}
}

// A finished automation run parks in WAITING_FOR_INPUT so it stays answerable,
// and that state is in the "active session" set these two queries use. Without
// the origin exclusion one nightly report would block its agent profile from
// ever being deleted, and would show up in the user's list of what is blocking
// it — naming a task they cannot find on any board.
func TestAgentProfileBlockersExcludeAutomationRuns(t *testing.T) {
	f := seedAutomationOriginFixture(t)
	ctx := context.Background()

	const profileID = "agent-profile-1"
	for _, spec := range []struct{ sessionID, taskID string }{
		{"sess-auto", f.autoTaskID},
	} {
		if err := f.repo.CreateTaskSession(ctx, &models.TaskSession{
			ID:             spec.sessionID,
			TaskID:         spec.taskID,
			AgentProfileID: profileID,
			State:          models.TaskSessionStateWaitingForInput,
		}); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", spec.sessionID, err)
		}
	}

	blocked, err := f.repo.HasActiveTaskSessionsByAgentProfile(ctx, profileID)
	if err != nil {
		t.Fatalf("HasActiveTaskSessionsByAgentProfile: %v", err)
	}
	if blocked {
		t.Fatal("a parked automation run must not block agent-profile deletion")
	}

	blockers, err := f.repo.GetActiveTaskInfoByAgentProfile(ctx, profileID)
	if err != nil {
		t.Fatalf("GetActiveTaskInfoByAgentProfile: %v", err)
	}
	for _, info := range blockers {
		if info.TaskID == f.autoTaskID {
			t.Fatalf("automation run must not appear in the blocker list, got %+v", blockers)
		}
	}
}

// A run that is still working is using the profile right now. Excluding it
// would let the profile be deleted out from under live automation work, leaving
// an enabled automation and an in-flight run pointing at nothing — so only the
// parked state is exempt, not the origin.
func TestAgentProfileBlockersReportRunningAutomationRuns(t *testing.T) {
	f := seedAutomationOriginFixture(t)
	ctx := context.Background()

	const profileID = "agent-profile-running"
	if err := f.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:             "sess-auto-running",
		TaskID:         f.autoTaskID,
		AgentProfileID: profileID,
		State:          models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	blocked, err := f.repo.HasActiveTaskSessionsByAgentProfile(ctx, profileID)
	if err != nil {
		t.Fatalf("HasActiveTaskSessionsByAgentProfile: %v", err)
	}
	if !blocked {
		t.Error("a running automation run must block agent-profile deletion")
	}

	blockers, err := f.repo.GetActiveTaskInfoByAgentProfile(ctx, profileID)
	if err != nil {
		t.Fatalf("GetActiveTaskInfoByAgentProfile: %v", err)
	}
	var found bool
	for _, info := range blockers {
		if info.TaskID == f.autoTaskID {
			found = true
		}
	}
	if !found {
		t.Errorf("a running automation run must be named as a blocker, got %+v", blockers)
	}
}

// The same states on a human task still block, so the exclusion above narrows
// the query by origin rather than defeating it.
func TestAgentProfileBlockersStillReportHumanTasks(t *testing.T) {
	f := seedAutomationOriginFixture(t)
	ctx := context.Background()

	const profileID = "agent-profile-2"
	if err := f.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:             "sess-human",
		TaskID:         f.boardTaskID,
		AgentProfileID: profileID,
		State:          models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	blocked, err := f.repo.HasActiveTaskSessionsByAgentProfile(ctx, profileID)
	if err != nil {
		t.Fatalf("HasActiveTaskSessionsByAgentProfile: %v", err)
	}
	if !blocked {
		t.Fatal("a human task waiting on input must still block deletion")
	}

	blockers, err := f.repo.GetActiveTaskInfoByAgentProfile(ctx, profileID)
	if err != nil {
		t.Fatalf("GetActiveTaskInfoByAgentProfile: %v", err)
	}
	if len(blockers) != 1 || blockers[0].TaskID != f.boardTaskID {
		t.Fatalf("expected only the human task as a blocker, got %+v", blockers)
	}
}
