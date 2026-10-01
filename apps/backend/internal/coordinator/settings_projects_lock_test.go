package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// lockProbe makes every project listing try to take the coordinator lock the
// way the repository.deleted cascade does after a listing prunes a repository.
// A listing made while the caller holds the lock cannot get it.
func lockProbe(store *Store, coordinatorID string, probed *int, failed *error) func() {
	return func() {
		*probed++
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := store.withCoordinatorLock(ctx, coordinatorID, func(coordinatorExec) error { return nil }); err != nil && *failed == nil {
			*failed = err
		}
	}
}

func TestSaveSettings_ProjectListingsAreReadOutsideTheCoordinatorLock(t *testing.T) {
	store, c, svc, p := projectsFixture(t)
	var probed int
	var failed error
	p.onList = lockProbe(store, c.ID, &probed, &failed)

	mustSave(t, svc, c.WorkspaceID, c.ID, projectsBody("selected", false, repoEntry("repo-a")))
	if probed == 0 {
		t.Fatal("the save read no listing")
	}
	if failed != nil {
		t.Fatalf("a listing ran while the save held the coordinator lock: %v", failed)
	}
}

func TestPutGoal_BaselineCountIsReadOutsideTheCoordinatorLock(t *testing.T) {
	f := newGoalFixture(t)
	f.createTasksTable()
	f.addTask("t1", "wf1", "TODO", "manual", 0, false)
	p := newFakeProjects()
	p.addRepo("repo-a", "a")
	p.taskRepos["t1"] = []string{"repo-a"}
	f.svc.SetProjectReader(p)
	projectScope(t, f.store, f.c.ID, false, repoEntry("repo-a"))
	var probed int
	var failed error
	p.onList = lockProbe(f.store, f.c.ID, &probed, &failed)

	if _, err := f.svc.PutGoal(context.Background(), f.c.WorkspaceID, f.c.ID, []byte(`{"name":"ship","criteria":[]}`)); err != nil {
		t.Fatal(err)
	}
	if probed == 0 {
		t.Fatal("the goal create read no listing")
	}
	if failed != nil {
		t.Fatalf("a listing ran while the goal write held the coordinator lock: %v", failed)
	}
}

func TestSetup_GoalBaselineCountIsReadOutsideTheCoordinatorLock(t *testing.T) {
	svc, store := setupService(t)
	svc.phase31 = true
	p := newFakeProjects()
	p.addRepo("repo-a", "a")
	svc.SetProjectReader(p)
	var probed int
	var failed error
	p.onList = func() {
		probed++
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		conn, err := store.db.Conn(ctx)
		if err != nil {
			if failed == nil {
				failed = err
			}
			return
		}
		defer func() { _ = conn.Close() }()
		if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
			if failed == nil {
				failed = err
			}
			return
		}
		_, _ = conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
	}
	_, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(setupBody(map[string]string{
		"projects": `{"scope":"selected","entries":[{"kind":"repository","id":"repo-a"}]}`,
		"goal":     `{"name":"ship","criteria":[]}`,
	})))
	if err != nil {
		t.Fatal(err)
	}
	if probed == 0 || failed != nil {
		t.Fatalf("probed %d, failed %v", probed, failed)
	}
}

func TestSetup_ValidationOrderIdentityBeforeProjects(t *testing.T) {
	svc, _ := setupService(t)
	svc.phase31 = true
	entries := `{"scope":"selected","entries":[`
	for i := 0; i < 51; i++ {
		if i > 0 {
			entries += ","
		}
		entries += `{"kind":"repository","id":"r` + string(rune('a'+i%26)) + string(rune('a'+i/26)) + `"}`
	}
	entries += `]}`
	_, err := svc.CreateSetup(context.Background(), testWorkspaceID, []byte(setupBody(map[string]string{
		"name": `""`, "projects": entries,
	})))
	se, ok := err.(*SettingsError)
	if !ok || se.Step != setupStepIdentity {
		t.Fatalf("err = %#v, want the identity step before projects", err)
	}
}

func TestSaveSettings_ProjectsChangeArchivesConversationAndPublishesOnce(t *testing.T) {
	store, c, svc, _ := projectsFixture(t)
	conv := newFakeConversationTasks()
	svc.SetConversationDeps(conv, newFakeSessionEnsurer())
	memBus := bus.NewMemoryEventBus(newTestLogger(t))
	capture := &captureCoordinatorUpdated{}
	if _, err := memBus.Subscribe(events.CoordinatorUpdated, capture.record); err != nil {
		t.Fatal(err)
	}
	svc.eventBus = memBus
	opened := struct{ TaskID string }{"conv-1"}
	conv.tasks[opened.TaskID] = &taskmodels.Task{ID: opened.TaskID, WorkspaceID: c.WorkspaceID}
	mustExec(t, store, `UPDATE coordinators SET conversation_task_id = ? WHERE id = ?`, opened.TaskID, c.ID)

	got := mustSave(t, svc, c.WorkspaceID, c.ID, combinedBody(map[string]string{"create_task": "denied"}, projectsBody("selected", false, repoEntry("repo-a"))))
	if got.PolicyRevision != 1 || revisionOf(t, store, c.ID) != 1 {
		t.Fatalf("a combined save must raise the revision once, got %d", got.PolicyRevision)
	}
	if len(conv.archivedIDs) != 1 || conv.archivedIDs[0] != opened.TaskID {
		t.Fatalf("archived = %v, want [%s]", conv.archivedIDs, opened.TaskID)
	}
	waitForEvents(t, capture, 1)
	if n := len(capture.snapshot()); n != 1 {
		t.Fatalf("coordinator.updated events = %d, want 1", n)
	}

	mustSave(t, svc, c.WorkspaceID, c.ID, projectsBody("selected", false, repoEntry("repo-a")))
	if n := len(capture.snapshot()); n != 1 || len(conv.archivedIDs) != 1 {
		t.Fatalf("an unchanged save published %d events and archived %d", n, len(conv.archivedIDs))
	}
}

func TestProjectDeleted_PublishesOncePerChangeAndDoesNotArchive(t *testing.T) {
	store, c, svc, _ := projectsFixture(t)
	conv := newFakeConversationTasks()
	svc.SetConversationDeps(conv, newFakeSessionEnsurer())
	conv.tasks["conv-1"] = &taskmodels.Task{ID: "conv-1", WorkspaceID: c.WorkspaceID}
	mustExec(t, store, `UPDATE coordinators SET conversation_task_id = 'conv-1' WHERE id = ?`, c.ID)
	mustSave(t, svc, c.WorkspaceID, c.ID, projectsBody("selected", false, setEntry("set-1"), repoEntry("repo-a")))
	conv.archivedIDs = nil
	memBus := bus.NewMemoryEventBus(newTestLogger(t))
	capture := &captureCoordinatorUpdated{}
	if _, err := memBus.Subscribe(events.CoordinatorUpdated, capture.record); err != nil {
		t.Fatal(err)
	}
	svc.eventBus = memBus

	for i := 0; i < 2; i++ {
		if err := svc.ProjectDeleted(context.Background(), projectKindSet, "set-1"); err != nil {
			t.Fatal(err)
		}
	}
	waitForEvents(t, capture, 1)
	if n := len(capture.snapshot()); n != 1 {
		t.Fatalf("events = %d, want exactly 1 (the repeat removed nothing)", n)
	}
	if len(conv.archivedIDs) != 0 {
		t.Fatalf("the cascade archived %v", conv.archivedIDs)
	}
}

// combinedBody merges the members of two settings bodies into one object.
func combinedBody(policyOverrides map[string]string, projects string) string {
	policy := policyBody(policyOverrides)
	return policy[:len(policy)-1] + "," + projects[1:]
}

func TestPutGoal_ActiveGoalGoneBeforeTheLockStillCreates(t *testing.T) {
	f := newGoalFixture(t)
	f.createTasksTable()
	f.mustPut(`{"name":"first","criteria":[]}`)
	fired := false
	f.svc.afterGoalBaselineRead = func() {
		if fired {
			return
		}
		fired = true
		if _, err := f.store.db.Exec(`DELETE FROM coordinator_goals`); err != nil {
			t.Fatal(err)
		}
	}

	g, err := f.put(`{"name":"second","criteria":[]}`)
	if err != nil {
		t.Fatalf("PutGoal with the active goal gone before the lock: %v", err)
	}
	if g == nil || g.Name != "second" {
		t.Fatalf("goal = %#v, want the created second goal", g)
	}
}
