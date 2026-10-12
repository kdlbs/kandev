package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type taskFieldReadGate struct {
	*sqliterepo.Repository
	armed       atomic.Bool
	writerArmed atomic.Bool
	arrived     chan struct{}
	release     chan struct{}
}

func (r *taskFieldReadGate) UpdateTaskFieldsWithParentAdmission(ctx context.Context, id string, update models.TaskFieldUpdate, validate repository.TaskParentValidator) (*models.TaskFieldUpdateResult, error) {
	if id == "field-task" && r.writerArmed.Swap(false) {
		close(r.arrived)
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return r.Repository.UpdateTaskFieldsWithParentAdmission(ctx, id, update, validate)
}

func seedFieldTask(t *testing.T, repo *sqliterepo.Repository, metadata map[string]interface{}) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "field-ws", Name: "Fields"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "field-task", WorkspaceID: "field-ws", Title: "Original", Description: "Original description", Priority: "medium", Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
}

func runFieldInterleaving(t *testing.T, svc *Service, gate *taskFieldReadGate, req *UpdateTaskRequest, afterRead func(context.Context)) (*models.Task, error) {
	t.Helper()
	return runFieldBoundary(t, svc, gate, req, false, func(ctx context.Context, _ context.CancelFunc) { afterRead(ctx) })
}

func runFieldBoundary(t *testing.T, svc *Service, gate *taskFieldReadGate, req *UpdateTaskRequest, writer bool, afterRead func(context.Context, context.CancelFunc)) (*models.Task, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	gate.arrived, gate.release = make(chan struct{}), make(chan struct{})
	gate.armed.Store(!writer)
	gate.writerArmed.Store(writer)
	type outcome struct {
		task *models.Task
		err  error
	}
	result := make(chan outcome, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		task, err := svc.UpdateTask(ctx, "field-task", req)
		result <- outcome{task, err}
	}()
	select {
	case <-gate.arrived:
	case <-ctx.Done():
		t.Fatal("initial task read did not reach gate", ctx.Err())
	}
	afterRead(ctx, cancel)
	close(gate.release)
	select {
	case got := <-result:
		return got.task, got.err
	case <-time.After(15 * time.Second):
		t.Fatal("field update did not settle", ctx.Err())
		return nil, ctx.Err()
	}
}

// @covers AC-TASKS-FIELD-UPDATES-001.2, AC-TASKS-FIELD-UPDATES-001.3
func TestTaskFieldUpdatesRequestPresence(t *testing.T) {
	services, buses, gates := taskFieldServicePair(t)
	repo := gates[0].Repository
	seedFieldTask(t, repo, map[string]interface{}{"old": "remove"})
	title, desc, priority, assignee, position := "Current", "Current description", "high", "user-current", 17
	updated, err := runFieldInterleaving(t, services[0], gates[0], &UpdateTaskRequest{}, func(ctx context.Context) {
		_, err := services[1].UpdateTask(ctx, "field-task", &UpdateTaskRequest{
			Title: &title, Description: &desc, Priority: &priority, AssigneeUserID: &assignee,
			Position: &position, Metadata: map[string]interface{}{"current": "retained"},
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	if err != nil || updated.Title != title || updated.Description != desc || updated.Priority != priority || updated.AssigneeUserID != assignee || updated.Position != position || updated.Metadata["current"] != "retained" {
		t.Fatalf("omitted fields restored a snapshot: task=%+v error=%v", updated, err)
	}
	for i := range buses {
		buses[i].ClearEvents()
	}
	empty, zero := "", 0
	updated, err = services[0].UpdateTask(context.Background(), "field-task", &UpdateTaskRequest{
		Title: &empty, Description: &empty, AssigneeUserID: &empty, ParentID: &empty,
		Position: &zero, Metadata: map[string]interface{}{}, Repositories: []TaskRepositoryInput{},
	})
	if err != nil || updated.Title != "" || updated.Description != "" || updated.AssigneeUserID != "" || updated.Position != 0 || len(updated.Metadata) != 0 || len(updated.Repositories) != 0 {
		t.Fatalf("explicit empty fields: task=%+v error=%v", updated, err)
	}
	last := "Last explicit title"
	updated, err = runFieldInterleaving(t, services[0], gates[0], &UpdateTaskRequest{Title: &last}, func(ctx context.Context) {
		if _, err := services[1].UpdateTask(ctx, "field-task", &UpdateTaskRequest{Title: &title}); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil || updated.Title != last {
		t.Fatalf("same-field later write: task=%+v error=%v", updated, err)
	}
}

// @covers AC-TASKS-FIELD-UPDATES-001.4, AC-TASKS-FIELD-UPDATES-001.5
func TestTaskFieldUpdatesProtectedOwners(t *testing.T) {
	t.Run("pending_claim_omitted_metadata", func(t *testing.T) {
		services, _, gates := taskFieldServicePair(t)
		repo := gates[0].Repository
		seedFieldTask(t, repo, map[string]interface{}{models.MetaKeyAgentTitlePending: true, "nullable": nil})
		description := "Description with a pending title"
		updated, err := runFieldInterleaving(t, services[0], gates[0], &UpdateTaskRequest{Description: &description}, func(ctx context.Context) {
			claimed, _, err := repo.ClaimTaskTitleSession(ctx, "field-task", "pending-owner")
			if err != nil || !claimed {
				t.Fatalf("claim: %t %v", claimed, err)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if !models.IsAgentTitlePending(updated.Metadata) || !models.IsAgentTitleOwner(updated.Metadata, "pending-owner") || updated.Title != "Original" || updated.Description != description {
			t.Fatalf("pending ownership lost: %+v", updated)
		}
		if value, present := updated.Metadata["nullable"]; !present || value != nil {
			t.Fatalf("omitted metadata lost current explicit null: %#v", updated.Metadata)
		}
		updated, err = services[0].UpdateTask(context.Background(), "field-task", &UpdateTaskRequest{Metadata: map[string]interface{}{models.MetaKeyAgentTitlePending: true, "nullable": nil}})
		if err != nil {
			t.Fatal(err)
		}
		if _, present := updated.Metadata["nullable"]; present {
			t.Fatal("supplied pending-title merge no longer applies its existing null deletion")
		}
		if !models.IsAgentTitleOwner(updated.Metadata, "pending-owner") {
			t.Fatal("supplied pending-title merge lost the claimed owner")
		}
	})

	for _, explicitPosition := range []bool{false, true} {
		t.Run(map[bool]string{false: "implicit_position", true: "explicit_position"}[explicitPosition], func(t *testing.T) {
			services, _, gates := taskFieldServicePair(t)
			repo := gates[0].Repository
			seedFieldTask(t, repo, map[string]interface{}{models.MetaKeyAgentTitlePending: true, "ordinary": "delete"})
			desc := "Requested description"
			req := &UpdateTaskRequest{Description: &desc}
			if explicitPosition {
				position := 8
				req.Position = &position
			}
			updated, err := runFieldInterleaving(t, services[0], gates[0], req, func(ctx context.Context) {
				claimed, _, err := repo.ClaimTaskTitleSession(ctx, "field-task", "owner")
				if err != nil || !claimed {
					t.Fatalf("claim: %t %v", claimed, err)
				}
				accepted, err := repo.SetTaskTitleIfPending(ctx, "field-task", "owner", "Agent title")
				if err != nil || !accepted {
					t.Fatalf("set title: %t %v", accepted, err)
				}
			})
			if err != nil || updated.Title != "Agent title" || updated.Description != desc || models.IsAgentTitlePending(updated.Metadata) {
				t.Fatalf("current agent title: task=%+v error=%v", updated, err)
			}
			requested := map[string]interface{}{"new": "metadata", "nullable": nil, models.MetaKeyDeferredLaunch: "forged", models.MetaKeyStepHandoffCarry: "forged", models.MetaKeyOfficeCarrierCausationDepth: 0}
			req.Metadata = requested
			updated, err = runFieldInterleaving(t, services[0], gates[0], req, func(ctx context.Context) {
				stored, lost, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "field-task", sqliterepo.AbsentDeferredLaunch(), map[string]interface{}{"prompt": "Current deferred"})
				if err != nil || !stored || lost {
					t.Fatalf("deferred CAS: stored=%t lost=%t error=%v", stored, lost, err)
				}
				storedHandoffs, _, err := repo.SetTaskHandoffsIfUnchanged(ctx, "field-task", "", `[{"task_id":"current-handoff"}]`)
				if err != nil || !storedHandoffs {
					t.Fatalf("handoffs CAS: %t %v", storedHandoffs, err)
				}
				for key, value := range map[string]interface{}{models.MetaKeyStepHandoffCarry: "Current carry", models.MetaKeyHandoffSource: "Current source", models.MetaKeyOfficeCarrierCausationDepth: 3} {
					if err := repo.SetTaskMetadataKey(ctx, "field-task", key, value); err != nil {
						t.Fatal(err)
					}
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			deferred, _ := updated.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
			if deferred["prompt"] != "Current deferred" || updated.Metadata[models.MetaKeyStepHandoffCarry] != "Current carry" || updated.Metadata[models.MetaKeyHandoffSource] != "Current source" || updated.Metadata[models.MetaKeyOfficeCarrierCausationDepth] != float64(3) || updated.Metadata["new"] != "metadata" {
				t.Fatalf("protected metadata clobbered: %#v", updated.Metadata)
			}
			if _, exists := updated.Metadata["ordinary"]; exists {
				t.Fatal("ordinary replacement became arbitrary merge")
			}
			if value, exists := updated.Metadata["nullable"]; !exists || value != nil {
				t.Fatal("plain replacement lost explicit null")
			}
			handoffs, ok := updated.Metadata[models.MetaKeyHandoffs].([]interface{})
			if !ok || len(handoffs) != 1 || handoffs[0].(map[string]interface{})["task_id"] != "current-handoff" {
				t.Fatalf("handoffs owner lost: %#v", updated.Metadata)
			}
			if requested[models.MetaKeyDeferredLaunch] != "forged" {
				t.Fatal("caller metadata was mutated")
			}
			if err := repo.SetTaskMetadataKey(context.Background(), "field-task", models.MetaKeyAgentTitlePending, true); err != nil {
				t.Fatal(err)
			}
			if _, _, err := repo.ClaimTaskTitleSession(context.Background(), "field-task", "owner"); err != nil {
				t.Fatal(err)
			}
			human := "Human title"
			if _, err := services[0].UpdateTask(context.Background(), "field-task", &UpdateTaskRequest{Title: &human}); err != nil {
				t.Fatal(err)
			}
			if _, accepted, _, err := services[1].SetPendingAgentTitle(context.Background(), "field-task", "owner", "Late agent"); err != nil || accepted {
				t.Fatalf("late agent title: accepted=%t error=%v", accepted, err)
			}
		})
	}
}

// @covers AC-TASKS-FIELD-UPDATES-001.6
func TestTaskFieldUpdatesCurrentRowFailure(t *testing.T) {
	for _, failure := range []string{"storage", "encoding", "malformed_current", "parent", "completion", "cancellation"} {
		t.Run(failure, func(t *testing.T) {
			services, buses, gates := taskFieldServicePair(t)
			repo := gates[0].Repository
			seedFieldTask(t, repo, map[string]interface{}{"keep": "current"})
			ctx := context.Background()
			for _, id := range []string{"parent", "ancestor"} {
				if err := repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "field-ws", Title: id}); err != nil {
					t.Fatal(err)
				}
			}
			title := "Must roll back"
			req := &UpdateTaskRequest{Title: &title}
			if failure == "parent" {
				parent := "parent"
				req.ParentID = &parent
			}
			if failure == "completion" {
				state := v1.TaskStateCompleted
				req.State = &state
			}
			if failure == "encoding" {
				req.Metadata = map[string]interface{}{"invalid": func() {}}
			}
			var before fieldRowSnapshot
			_, err := runFieldBoundary(t, services[0], gates[0], req, true, func(ctx context.Context, cancel context.CancelFunc) {
				switch failure {
				case "storage":
					_, err := repo.DB().ExecContext(ctx, `CREATE TRIGGER reject_field_update BEFORE UPDATE ON tasks WHEN NEW.title='Must roll back' BEGIN SELECT RAISE(ABORT,'field update denied'); END`)
					if err != nil {
						t.Fatal(err)
					}
				case "malformed_current":
					if _, err := repo.DB().ExecContext(ctx, `UPDATE tasks SET metadata='{' WHERE id='field-task'`); err != nil {
						t.Fatal(err)
					}
				case "parent":
					ancestor := "ancestor"
					if _, err := services[1].UpdateTask(ctx, "parent", &UpdateTaskRequest{ParentID: &ancestor}); err != nil {
						t.Fatal(err)
					}
				case "completion":
					_, err := services[1].SetTaskCompletionCriteria(ctx, "field-task", SetTaskCompletionCriteriaRequest{
						Criteria: []models.TaskCompletionCriterion{{ID: "review", Description: "Reviewed", EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceArtifact, ID: "review"}}},
					})
					if err != nil {
						t.Fatal(err)
					}
				case "cancellation":
					// Cancellation is requested after the before-row snapshot below.
				}
				before = snapshotFieldRow(t, repo)
				if failure == "cancellation" {
					cancel()
				}
			})
			if failure == "cancellation" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation cause: %v", err)
				}
			}
			if err == nil {
				t.Fatal("invalid field update succeeded")
			}
			if failure == "completion" && !errors.Is(err, repoerrors.ErrTaskCompletionGateBlocked) {
				t.Fatalf("completion cause lost: %v", err)
			}
			if failure == "parent" && !errors.Is(err, ErrInvalidParent) {
				t.Fatalf("parent cause lost: %v", err)
			}
			if failure == "encoding" {
				var cause *json.UnsupportedTypeError
				if !errors.As(err, &cause) {
					t.Fatalf("encoding cause lost: %v", err)
				}
			}
			after := snapshotFieldRow(t, repo)
			if !reflect.DeepEqual(before, after) || len(buses[0].GetPublishedEvents()) != 0 {
				t.Fatalf("failed request changed row/events: before=%+v after=%+v events=%v", before, after, buses[0].GetPublishedEvents())
			}
			if failure == "storage" {
				if _, err := repo.DB().Exec(`DROP TRIGGER reject_field_update`); err != nil {
					t.Fatal(err)
				}
			}
			if failure == "malformed_current" {
				if _, err := repo.DB().Exec(`UPDATE tasks SET metadata='{}' WHERE id='field-task'`); err != nil {
					t.Fatal(err)
				}
			}
			good := "Successful control"
			current, err := services[0].UpdateTask(ctx, "field-task", &UpdateTaskRequest{Title: &good})
			if err != nil || current.Title != good || len(buses[0].GetPublishedEvents()) != 1 {
				t.Fatalf("positive control: task=%+v error=%v", current, err)
			}
		})
	}
}

func (r *taskFieldReadGate) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task, err := r.Repository.GetTask(ctx, id)
	if err != nil || id != "field-task" || !r.armed.Swap(false) {
		return task, err
	}
	close(r.arrived)
	select {
	case <-r.release:
		return task, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func taskFieldService(t *testing.T, repo *sqliterepo.Repository, gate *taskFieldReadGate) (*Service, *MockEventBus) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatal(err)
	}
	bus := NewMockEventBus()
	svc := NewService(Repos{
		Workspaces: repo, Tasks: gate, TaskRepos: repo, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		RepositorySets: repo, BranchPolicies: repo, RepositoryCleanup: repo, Executors: repo,
		Environments: repo, TaskEnvironments: repo, Reviews: repo, ResourceCleanups: repo,
		Usage: repo, BackgroundWork: repo,
	}, bus, log, RepositoryDiscoveryConfig{})
	svc.SetWorkflowStepGetter(&testWorkflowStepGetter{repo: repo})
	if err := svc.StartTaskResourceCleanupWorker(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.StopTaskResourceCleanupWorker)
	return svc, bus
}

func taskFieldServicePair(t *testing.T) ([2]*Service, [2]*MockEventBus, [2]*taskFieldReadGate) {
	t.Helper()
	first, path := serviceTestSQLiteTemplate.Open(t)
	raw, err := internaldb.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	second := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = second.Close() })
	var services [2]*Service
	var buses [2]*MockEventBus
	var gates [2]*taskFieldReadGate
	for i, database := range []*sqlx.DB{first, second} {
		repo := sqliterepo.NewWithInitializedDB(database, database, nil)
		gates[i] = &taskFieldReadGate{Repository: repo, arrived: make(chan struct{}), release: make(chan struct{})}
		services[i], buses[i] = taskFieldService(t, repo, gates[i])
	}
	return services, buses, gates
}

// @covers AC-TASKS-FIELD-UPDATES-001.1, AC-TASKS-FIELD-UPDATES-001.2
func TestTaskFieldUpdatesConcurrentSQLite(t *testing.T) {
	for _, first := range []int{0, 1} {
		name := []string{"title_first", "description_first"}[first]
		t.Run(name, func(t *testing.T) {
			services, buses, gates := taskFieldServicePair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			var workers sync.WaitGroup
			defer func() { cancel(); workers.Wait() }()
			repo := gates[0].Repository
			if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "field-ws", Name: "Fields"}); err != nil {
				t.Fatal(err)
			}
			base := &models.Task{ID: "field-task", WorkspaceID: "field-ws", Title: "Original", Priority: "medium"}
			if err := repo.CreateTask(ctx, base); err != nil {
				t.Fatal(err)
			}
			title, description := "Concurrent title", "Concurrent description"
			control := &models.Task{ID: "sequential-fields", WorkspaceID: base.WorkspaceID, Title: base.Title}
			if err := repo.CreateTask(ctx, control); err != nil {
				t.Fatal(err)
			}
			for _, req := range []*UpdateTaskRequest{{Description: &description}, {Title: &title}} {
				if _, err := services[0].UpdateTask(ctx, control.ID, req); err != nil {
					t.Fatal(err)
				}
			}
			sequential, err := repo.GetTask(ctx, control.ID)
			if err != nil || sequential.Title != title || sequential.Description != description {
				t.Fatalf("sequential control: task=%+v error=%v", sequential, err)
			}
			results := [2]chan error{make(chan error, 1), make(chan error, 1)}
			for i, req := range []*UpdateTaskRequest{{Title: &title}, {Description: &description}} {
				buses[i].ClearEvents()
				gates[i].armed.Store(true)
				workers.Add(1)
				go func(i int, req *UpdateTaskRequest) {
					defer workers.Done()
					_, err := services[i].UpdateTask(ctx, base.ID, req)
					results[i] <- err
				}(i, req)
			}
			for _, gate := range gates {
				select {
				case <-gate.arrived:
				case <-ctx.Done():
					t.Fatal("independent service snapshots did not arrive", ctx.Err())
				}
			}
			for _, i := range []int{first, 1 - first} {
				close(gates[i].release)
				select {
				case err := <-results[i]:
					if err != nil {
						t.Fatal(err)
					}
				case <-ctx.Done():
					t.Fatal("update did not settle", ctx.Err())
				}
			}
			workers.Wait()
			current, err := repo.GetTask(ctx, base.ID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Title != title || current.Description != description {
				t.Errorf("both disjoint requests succeeded but an edit was lost: title=%q description=%q", current.Title, current.Description)
			}
			if current.ParentID != base.ParentID || current.WorkspaceID != base.WorkspaceID || current.Priority != base.Priority {
				t.Errorf("unrelated fields changed: %+v", current)
			}
			for i, bus := range buses {
				published := bus.GetPublishedEvents()
				if len(published) != 1 || published[0].Type != events.TaskUpdated {
					t.Errorf("service %d events: %+v", i, published)
				}
			}
		})
	}
}

// @covers AC-TASKS-FIELD-UPDATES-001.3
func TestTaskFieldUpdatesPriorityAndAssignee(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "field-ws", Name: "Fields"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "field-task", WorkspaceID: "field-ws", Title: "Original"}); err != nil {
		t.Fatal(err)
	}
	priority, assignee := "high", "user-fields"
	bus.ClearEvents()
	if _, err := svc.UpdateTask(ctx, "field-task", &UpdateTaskRequest{Priority: &priority, AssigneeUserID: &assignee}); err != nil {
		t.Fatal(err)
	}
	current, err := repo.GetTask(ctx, "field-task")
	if err != nil {
		t.Fatal(err)
	}
	if current.Priority != priority || current.AssigneeUserID != assignee {
		t.Fatalf("mixed priority and assignee did not both persist: priority=%q assignee=%q", current.Priority, current.AssigneeUserID)
	}
	priority = "low"
	if _, err := svc.UpdateTask(ctx, current.ID, &UpdateTaskRequest{Priority: &priority}); err != nil {
		t.Fatal(err)
	}
	current, err = repo.GetTask(ctx, current.ID)
	if err != nil || current.Priority != priority || current.AssigneeUserID != assignee {
		t.Fatalf("priority-only control: task=%+v error=%v", current, err)
	}
	assignee = ""
	if _, err := svc.UpdateTask(ctx, current.ID, &UpdateTaskRequest{Priority: &priority, AssigneeUserID: &assignee}); err != nil {
		t.Fatal(err)
	}
	current, err = repo.GetTask(ctx, current.ID)
	if err != nil || current.AssigneeUserID != "" {
		t.Fatalf("mixed unassignment: task=%+v error=%v", current, err)
	}

	if _, err := repo.DB().Exec(`UPDATE workspaces SET owner_id='owner-fields' WHERE id='field-ws'`); err != nil {
		t.Fatal(err)
	}
	bus.ClearEvents()
	before := snapshotFieldRow(t, repo)
	assignee = "unreachable-user"
	priority = "high"
	if _, err := svc.UpdateTask(ctx, current.ID, &UpdateTaskRequest{Priority: &priority, AssigneeUserID: &assignee}); !errors.Is(err, ErrAssigneeCannotReachWorkspace) {
		t.Fatalf("invalid mixed assignee: %v", err)
	}
	if !reflect.DeepEqual(before, snapshotFieldRow(t, repo)) || len(bus.GetPublishedEvents()) != 0 {
		t.Fatal("rejected assignee changed row/events")
	}
}

// Capture every persisted task column and transactional workflow bookkeeping.
type fieldRowSnapshot struct {
	Row             map[string]interface{}
	Ledger, Entries int
}

func snapshotFieldRow(t *testing.T, repo *sqliterepo.Repository) fieldRowSnapshot {
	t.Helper()
	rows, err := sqlx.NewDb(repo.DB(), "sqlite3").Queryx(`SELECT * FROM tasks WHERE id='field-task'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	snapshot := fieldRowSnapshot{Row: map[string]interface{}{}}
	if !rows.Next() {
		t.Fatal("missing field task")
	}
	if err := rows.MapScan(snapshot.Row); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot.Ledger = countLedgerRows(t, repo, "field-task")
	if err := repo.DB().QueryRow(`SELECT count(*) FROM workflow_step_entries WHERE task_id='field-task'`).Scan(&snapshot.Entries); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// @covers AC-TASKS-FIELD-UPDATES-001.3, AC-TASKS-FIELD-UPDATES-001.6
func TestTaskFieldUpdatesCoupledWorkflow(t *testing.T) {
	for _, requested := range []bool{false, true} {
		t.Run(map[bool]string{false: "omitted", true: "explicit"}[requested], func(t *testing.T) {
			services, buses, gates := taskFieldServicePair(t)
			repo := gates[0].Repository
			ctx := context.Background()
			if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-stamp", Name: "Fields"}); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-stamp", WorkspaceID: "ws-stamp", Name: "Fields"}); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.DB().Exec(`INSERT INTO workflow_steps(id,workflow_id,name,position) VALUES ('step-a','wf-stamp','A',0),('step-b','wf-stamp','B',1),('step-c','wf-stamp','C',2)`); err != nil {
				t.Fatal(err)
			}
			if err := repo.CreateTask(ctx, &models.Task{ID: "field-task", WorkspaceID: "ws-stamp", WorkflowID: "wf-stamp", WorkflowStepID: "step-a", Title: "Fields"}); err != nil {
				t.Fatal(err)
			}
			baselineLedger := countLedgerRows(t, repo, "field-task")
			desc := "Concurrent description"
			req := &UpdateTaskRequest{Description: &desc}
			target, completed := "step-c", v1.TaskStateCompleted
			if requested {
				req.WorkflowStepID = &target
				req.State = &completed
			}
			updated, err := runFieldInterleaving(t, services[0], gates[0], req, func(ctx context.Context) {
				step, state, position := "step-b", v1.TaskStateInProgress, 11
				if _, err := services[1].UpdateTask(ctx, "field-task", &UpdateTaskRequest{WorkflowStepID: &step, State: &state, Position: &position}); err != nil {
					t.Fatal(err)
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			if updated.Description != desc || updated.Position != 11 || updated.WorkflowID != "wf-stamp" {
				t.Fatalf("coupled fields lost: %+v", updated)
			}
			wantStep, wantState, wantLedger := "step-b", v1.TaskStateInProgress, baselineLedger+1
			if requested {
				wantStep = target
				wantState = completed
				wantLedger = baselineLedger + 2
			}
			if updated.WorkflowStepID != wantStep || updated.State != wantState || countLedgerRows(t, repo, "field-task") != wantLedger {
				t.Fatalf("workflow bookkeeping: %+v", updated)
			}
			if requested {
				var from, to string
				if err := repo.DB().QueryRow(`SELECT from_workflow_step_id,to_workflow_step_id FROM task_step_transitions WHERE task_id='field-task' ORDER BY occurred_at DESC, id DESC LIMIT 1`).Scan(&from, &to); err != nil {
					t.Fatal(err)
				}
				if from != "step-b" || to != "step-c" {
					t.Fatalf("locked transition=%s -> %s", from, to)
				}
			}
			published := buses[0].GetPublishedEvents()
			if requested {
				if len(published) != 2 || published[0].Type != events.TaskStateChanged || published[0].Data.(map[string]interface{})["old_state"] != string(v1.TaskStateInProgress) {
					t.Fatalf("state events=%+v", published)
				}
			} else if len(published) != 1 || published[0].Type != events.TaskUpdated {
				t.Fatalf("omitted state emitted transition: %+v", published)
			}
		})
	}
}
