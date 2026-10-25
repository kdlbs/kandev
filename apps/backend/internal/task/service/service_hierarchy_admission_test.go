package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// hierarchyReadBarrier delegates the real preflight child read before releasing
// competing callers. Transaction-bound admission never reads through this pool.
type hierarchyReadBarrier struct {
	*sqliterepo.Repository
	mu       sync.Mutex
	arrived  int
	expected int
	release  chan struct{}
	armed    bool
}

func (r *hierarchyReadBarrier) ListChildren(ctx context.Context, id string) ([]*models.Task, error) {
	children, err := r.Repository.ListChildren(ctx, id)
	if err != nil || !r.armed {
		return children, err
	}
	r.mu.Lock()
	r.arrived++
	if r.arrived == r.expected {
		close(r.release)
	}
	r.mu.Unlock()
	select {
	case <-r.release:
		return children, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestTaskHierarchyAdmissionConcurrentMoves(t *testing.T) {
	for _, scenario := range []struct {
		size   int
		office bool
	}{{2, false}, {3, false}, {3, true}} {
		size := scenario.size
		t.Run(fmt.Sprintf("nodes_%d_office_%v", size, scenario.office), func(t *testing.T) {
			var barrier *hierarchyReadBarrier
			svc, bus, repo := createTestServiceWithTaskAndSessionRepos(t, func(r *sqliterepo.Repository) repository.TaskRepository {
				barrier = &hierarchyReadBarrier{Repository: r}
				return barrier
			}, func(r *sqliterepo.Repository) repository.SessionRepository { return r })
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "hierarchy-ws", Name: "Hierarchy"}); err != nil {
				t.Fatal(err)
			}
			if scenario.office {
				if _, err := repo.DB().ExecContext(ctx, `UPDATE workspaces SET office_workflow_id='hierarchy-wf' WHERE id='hierarchy-ws'`); err != nil {
					t.Fatal(err)
				}
			}
			ids := make([]string, size)
			for i := range ids {
				ids[i] = fmt.Sprintf("hierarchy-%d", i)
				if err := repo.CreateTask(ctx, &models.Task{ID: ids[i], WorkspaceID: "hierarchy-ws", WorkflowID: "hierarchy-wf", Title: ids[i]}); err != nil {
					t.Fatal(err)
				}
			}
			bus.ClearEvents()
			barrier.expected = size
			barrier.release = make(chan struct{})
			barrier.armed = true
			results := make(chan error, size)
			var workers sync.WaitGroup
			for i, id := range ids {
				workers.Add(1)
				go func(id, parent string) {
					defer workers.Done()
					_, err := svc.UpdateTask(ctx, id, &UpdateTaskRequest{ParentID: &parent})
					results <- err
				}(id, ids[(i+1)%size])
			}
			workers.Wait()
			close(results)
			accepted := 0
			for err := range results {
				if err == nil {
					accepted++
				} else if !errors.Is(err, ErrInvalidParent) {
					t.Errorf("unexpected admission error: %v", err)
				}
			}
			if accepted == size {
				t.Fatalf("all %d competing edges committed; canonical admission allowed a cycle", size)
			}
			if accepted == 0 {
				t.Fatal("no ordinary valid edge committed")
			}
			if len(bus.GetPublishedEvents()) != accepted {
				t.Fatalf("success events=%d, accepted=%d", len(bus.GetPublishedEvents()), accepted)
			}
			persisted := make(map[string]*models.Task)
			for _, id := range ids {
				task, err := repo.GetTask(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				persisted[id] = task
				if scenario.office && !task.IsFromOffice {
					t.Fatal("Office control did not exercise Office depth exemption")
				}
			}
			for _, task := range persisted {
				visited := make(map[string]bool)
				for current := task; current != nil && current.ParentID != ""; current = persisted[current.ParentID] {
					if visited[current.ID] {
						t.Fatal("persisted graph has a cycle")
					}
					visited[current.ID] = true
				}
			}
		})
	}
}

func TestTaskHierarchyAdmissionSnapshotPreservation(t *testing.T) {
	for _, variant := range []string{"ordinary", "explicit_position", "exact", "workflow"} {
		t.Run(variant, func(t *testing.T) {
			svc, _, repo, create := reparentFixture(t)
			ctx := context.Background()
			a, b, child := create("A"), create("B"), create("Child")
			if _, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: &a.ID}); err != nil {
				t.Fatal(err)
			}
			setInheritedWorkspaceMode(t, ctx, repo, child.ID)
			now := time.Now().UTC()
			if _, err := repo.DB().ExecContext(ctx, `INSERT INTO task_workspace_groups(id,workspace_id,owner_task_id,materialized_kind,materialized_environment_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, "group-1", child.WorkspaceID, a.ID, "single_repo", "aba-env", now, now); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.DB().ExecContext(ctx, `INSERT INTO task_workspace_group_members(workspace_group_id,task_id,role,created_at) VALUES(?,?,?,?)`, "group-1", child.ID, "member", now); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "aba-env", TaskID: a.ID, ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: t.TempDir(), Status: models.TaskEnvironmentStatusReady}); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "aba-session", TaskID: child.ID, State: models.TaskSessionStateRunning, IsPrimary: true}); err != nil {
				t.Fatal(err)
			}

			withMetadata, err := repo.GetTask(ctx, child.ID)
			if err != nil {
				t.Fatal(err)
			}
			withMetadata.Metadata["unrelated"] = "delete by omission"
			if err := repo.UpdateTask(ctx, withMetadata); err != nil {
				t.Fatal(err)
			}
			stale, err := repo.GetTask(ctx, child.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, parent := range []string{b.ID, a.ID} {
				if _, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: &parent}); err != nil {
					t.Fatal(err)
				}
			}
			current, err := repo.GetTask(ctx, child.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertWorkspaceMode(t, current.Metadata, "shared_group")
			stale.Title = "Legitimate rename"
			delete(stale.Metadata, "unrelated")
			stale.Metadata["keep"] = "requested"
			switch variant {
			case "ordinary":
				err = repo.UpdateTask(ctx, stale)
			case "explicit_position":
				stale.Position = 41
				err = repo.UpdateTaskWithExplicitPosition(ctx, stale)
			case "exact":
				_, err = repo.UpdateTaskExactOperation(ctx, stale, stale.WorkspaceID, current.UpdatedAt.Format(time.RFC3339Nano), "hierarchy-exact", "digest")
			case "workflow":
				_, err = repo.UpdateTaskWithWorkflowStepAdmission(ctx, stale, stale.WorkflowStepID, stale.WorkflowStepID, 0)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := repo.GetTask(ctx, child.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertWorkspaceMode(t, got.Metadata, "shared_group")
			if _, present := got.Metadata["unrelated"]; present {
				t.Fatal("unrelated metadata deletion was lost")
			}
			workspace := got.Metadata["workspace"].(map[string]interface{})
			if workspace["group_id"] != "group-1" {
				t.Fatal("materialized group changed")
			}
			var owner, environment string
			var generation int64
			if err := repo.DB().QueryRowContext(ctx, `SELECT owner_task_id,materialized_environment_id,ownership_generation FROM task_workspace_groups WHERE id='group-1'`).Scan(&owner, &environment, &generation); err != nil {
				t.Fatal(err)
			}
			if owner != a.ID || environment != "aba-env" || generation != 1 {
				t.Fatalf("materialized identity changed: %s/%s/%d", owner, environment, generation)
			}
			session, err := repo.GetTaskSession(ctx, "aba-session")
			if err != nil {
				t.Fatal(err)
			}
			if session.State != models.TaskSessionStateRunning || session.TaskID != child.ID {
				t.Fatal("active session changed")
			}

			if got.ParentID != a.ID || got.Title != stale.Title || got.Metadata["keep"] != "requested" {
				t.Fatalf("lost requested fields or parent: %+v", got)
			}
		})
	}
}

// createParentBarrier pauses after the service's preparation, delegating the
// actual insertion to the real repository after the competing move commits.
type createParentBarrier struct {
	*sqliterepo.Repository
	prepared chan struct{}
	release  chan struct{}
}

func (r *createParentBarrier) CreateTask(ctx context.Context, task *models.Task) error {
	if task.ParentID != "" {
		close(r.prepared)
		select {
		case <-r.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return r.Repository.CreateTask(ctx, task)
}
func TestTaskHierarchyAdmissionCreationDepth(t *testing.T) {
	barrier := &createParentBarrier{prepared: make(chan struct{}), release: make(chan struct{})}
	svc, bus, repo := createTestServiceWithTaskAndSessionRepos(t, func(r *sqliterepo.Repository) repository.TaskRepository {
		barrier.Repository = r
		return barrier
	}, func(r *sqliterepo.Repository) repository.SessionRepository { return r })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "creation-ws", Name: "Creation"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "creation-wf", WorkspaceID: "creation-ws", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"creation-a", "creation-b"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "creation-ws", WorkflowID: "creation-wf", Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	bus.ClearEvents()
	result := make(chan error, 1)
	go func() {
		_, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "creation-ws", WorkflowID: "creation-wf", Title: "Late child", ParentID: "creation-b"})
		result <- err
	}()
	select {
	case <-barrier.prepared:
	case <-ctx.Done():
		t.Fatal("child preparation did not finish")
	}
	parent := "creation-a"
	_, moveErr := svc.UpdateTask(ctx, "creation-b", &UpdateTaskRequest{ParentID: &parent})
	close(barrier.release)
	createErr := <-result
	if moveErr != nil {
		t.Fatal(moveErr)
	}
	if !errors.Is(createErr, ErrSubtaskDepthExceeded) {
		t.Fatalf("late child admission=%v, want depth rejection", createErr)
	}
	children, err := repo.ListChildren(ctx, "creation-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 0 || len(bus.GetPublishedEvents()) != 1 {
		t.Fatal("rejected child persisted or published success")
	}
}

func TestTaskHierarchyAdmissionFinalDeletion(t *testing.T) {
	_, _, repo, create := reparentFixture(t)
	ctx := context.Background()
	parent := create("Parent")
	for _, archived := range []bool{false, true} {
		child := &models.Task{ID: fmt.Sprintf("late-child-%v", archived), WorkspaceID: parent.WorkspaceID, Title: "Late child", ParentID: parent.ID, IsEphemeral: archived, Origin: models.TaskOriginAutomationRun}
		if err := repo.CreateTask(ctx, child); err != nil {
			t.Fatal(err)
		}
		if archived {
			if err := repo.ArchiveTask(ctx, child.ID); err != nil {
				t.Fatal(err)
			}
		}
		if err := repo.DeleteTask(ctx, parent.ID); err == nil {
			t.Fatal("final deletion removed a parent with a structural child")
		}
		if _, err := repo.GetTask(ctx, parent.ID); err != nil {
			t.Fatalf("parent was not retained: %v", err)
		}
		if err := repo.DeleteTask(ctx, child.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.DeleteTask(ctx, parent.ID); err != nil {
		t.Fatalf("ordinary deletion after children removed: %v", err)
	}
}

func TestTaskHierarchyAdmissionMalformedAncestry(t *testing.T) {
	for _, state := range []string{"missing", "cycle", "read_failure", "target_read_failure"} {
		t.Run(state, func(t *testing.T) {
			svc, bus, repo, create := reparentFixture(t)
			a, b, subject := create("A"), create("B"), create("Subject")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			// Office-shaped canonical calls still enforce cycles, but permit depth.
			if _, err := repo.DB().ExecContext(ctx, `UPDATE workspaces SET office_workflow_id='wf-1' WHERE id='ws-1'`); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.DB().ExecContext(ctx, `UPDATE tasks SET parent_id=? WHERE id=?`, b.ID, a.ID); err != nil {
				t.Fatal(err)
			}
			switch state {
			case "missing":
				if _, err := repo.DB().ExecContext(ctx, `UPDATE tasks SET parent_id='missing-ancestor' WHERE id=?`, a.ID); err != nil {
					t.Fatal(err)
				}
			case "cycle":
				if _, err := repo.DB().ExecContext(ctx, `UPDATE tasks SET parent_id=? WHERE id=?`, a.ID, b.ID); err != nil {
					t.Fatal(err)
				}
			case "target_read_failure":
				if _, err := repo.DB().ExecContext(ctx, `UPDATE tasks SET workflow_agent_overrides='{' WHERE id=?`, a.ID); err != nil {
					t.Fatal(err)
				}
			case "read_failure":
				if _, err := repo.DB().ExecContext(ctx, `UPDATE tasks SET workflow_agent_overrides='{' WHERE id=?`, b.ID); err != nil {
					t.Fatal(err)
				}
			}
			bus.ClearEvents()
			_, err := svc.UpdateTask(ctx, subject.ID, &UpdateTaskRequest{ParentID: &a.ID})
			if state == "missing" {
				if err != nil {
					t.Fatalf("positive missing ancestor should remain a historical root: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("malformed ancestry accepted")
			}
			if state == "cycle" && !errors.Is(err, ErrInvalidParent) {
				t.Fatalf("cycle error=%v", err)
			}
			if (state == "read_failure" || state == "target_read_failure") && errors.Is(err, ErrInvalidParent) {
				t.Fatalf("real hierarchy decode failure masked as invalid input: %v", err)
			}
			if state == "target_read_failure" {
				if !errors.Is(err, models.ErrMalformedWorkflowAgentOverrides) {
					t.Fatalf("direct target decode error identity lost: %v", err)
				}
			}
			current, readErr := repo.GetTask(ctx, subject.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if current.ParentID != "" || len(bus.GetPublishedEvents()) != 0 {
				t.Fatal("invalid ancestry changed task or published success")
			}
		})
	}
}

func TestTaskHierarchyAdmissionSnapshotEncodingFailure(t *testing.T) {
	svc, bus, repo, create := reparentFixture(t)
	parent, child := create("Parent"), create("Child")
	ctx := context.Background()
	if _, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: &parent.ID}); err != nil {
		t.Fatal(err)
	}
	setInheritedWorkspaceMode(t, ctx, repo, child.ID)
	empty := ""
	if _, err := svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{ParentID: &empty}); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: "encoding-session", TaskID: child.ID, State: models.TaskSessionStateRunning, IsPrimary: true}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	before, err := repo.GetTask(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeSession, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	bus.ClearEvents()
	title := "Must not commit"
	_, err = svc.UpdateTask(ctx, child.ID, &UpdateTaskRequest{Title: &title, Metadata: map[string]interface{}{"invalid": func() {}, "keep": "requested"}})
	if err == nil {
		t.Fatal("unencodable snapshot was accepted")
	}
	after, err := repo.GetTask(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterSession, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(beforeSession, afterSession) || len(bus.GetPublishedEvents()) != 0 {
		t.Fatal("encoding failure changed task/session or published success")
	}
}
