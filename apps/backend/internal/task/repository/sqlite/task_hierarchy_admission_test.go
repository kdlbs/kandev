package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Independent writer and reader pools reproduce separate backend handles.
func TestTaskHierarchyAdmissionSQLiteIndependentHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hierarchy.db")
	open := func() *Repository {
		writer, err := internaldb.OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		reader, err := internaldb.OpenSQLiteReader(path)
		if err != nil {
			t.Fatal(err)
		}
		w, ro := sqlx.NewDb(writer, "sqlite3"), sqlx.NewDb(reader, "sqlite3")
		t.Cleanup(func() { _ = ro.Close(); _ = w.Close() })
		repo, err := NewWithDB(w, ro, nil)
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}
	a, b := open(), open()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.CreateWorkspace(ctx, &models.Workspace{ID: "independent-ws", Name: "Independent"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err := a.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "independent-ws", Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	tasks := make([]*models.Task, 2)
	var err error
	tasks[0], err = a.GetTask(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	tasks[1], err = b.GetTask(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	start, results := make(chan struct{}), make(chan error, 2)
	var workers sync.WaitGroup
	for i, repo := range []*Repository{a, b} {
		workers.Add(1)
		go func(i int, repo *Repository) {
			defer workers.Done()
			<-start
			parent := tasks[1-i].ID
			_, err := repo.UpdateTaskWithParentAdmission(ctx, tasks[i], &parent, false, hierarchy.ValidateParent)
			results <- err
		}(i, repo)
	}
	close(start)
	workers.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else if !errors.Is(err, hierarchy.ErrInvalidParent) {
			t.Errorf("unexpected failure: %v", err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d reverse edges, want 1", accepted)
	}
	first, err := a.GetTask(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.GetTask(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	if first.ParentID == second.ID && second.ParentID == first.ID {
		t.Fatal("persisted cycle across handles")
	}
}

func TestTaskHierarchyAdmissionDeletionCompensation(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "compensation-ws", Name: "Compensation"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"parent", "child", "other"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "compensation-ws", Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	child, err := repo.GetTask(ctx, "child")
	if err != nil {
		t.Fatal(err)
	}
	parent := "parent"
	if _, err := repo.UpdateTaskWithParentAdmission(ctx, child, &parent, false, hierarchy.ValidateParent); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReparentDirectChildrenInWorkspace(ctx, "parent", "", "compensation-ws"); err != nil {
		t.Fatal(err)
	}
	// The original root now has a new child. A failed delete must preserve it.
	if err := repo.CreateTask(ctx, &models.Task{ID: "late", WorkspaceID: "compensation-ws", Title: "Late", ParentID: "parent"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteTask(ctx, "parent"); !errors.Is(err, repoerrors.ErrTaskHierarchyConflict) {
		t.Fatalf("delete=%v", err)
	}
	if err := repo.RestoreTaskParentIfUnchanged(ctx, "child", "", "parent", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.RestoreTaskParentIfUnchanged(ctx, "child", "", "other", ""); err == nil {
		t.Fatal("compensation overwrote a replacement parent")
	}
	// Restoring an edge that now closes a cycle is rejected under the same policy.
	if err := repo.ReparentDirectChildren(ctx, "parent", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`UPDATE tasks SET parent_id = 'child' WHERE id = 'parent'`); err != nil {
		t.Fatal(err)
	}
	if err := repo.RestoreTaskParentIfUnchanged(ctx, "child", "", "parent", ""); !errors.Is(err, hierarchy.ErrInvalidParent) {
		t.Fatalf("invalid restore=%v", err)
	}
}

func TestTaskHierarchyAdmissionSQLiteWriterBeforeRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "writer-first.db")
	open := func() *Repository {
		writer, err := internaldb.OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		reader, err := internaldb.OpenSQLiteReader(path)
		if err != nil {
			t.Fatal(err)
		}
		w, ro := sqlx.NewDb(writer, "sqlite3"), sqlx.NewDb(reader, "sqlite3")
		t.Cleanup(func() { _ = ro.Close(); _ = w.Close() })
		repo, err := NewWithDB(w, ro, nil)
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}
	a, b := open(), open()
	ctx := context.Background()
	if err := a.CreateWorkspace(ctx, &models.Workspace{ID: "writer-ws", Name: "Writer"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err := a.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "writer-ws", Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := b.GetTask(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	// A bounded real SQLite busy refusal proves graph reads never begin when
	// the independent handle cannot reserve its writer. The positive retry
	// below uses the same production policy after the competing edge commits.
	if _, err := b.db.Exec(`PRAGMA busy_timeout=1`); err != nil {
		t.Fatal(err)
	}
	holder, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.Exec(`UPDATE tasks SET parent_id='b' WHERE id='a'`); err != nil {
		t.Fatal(err)
	}
	reads := 0
	validate := func(ctx context.Context, reader hierarchy.TaskHierarchyReader, task *models.Task, parent string) error {
		reads++
		return hierarchy.ValidateParent(ctx, reader, task, parent)
	}
	parent := "a"
	_, err = b.UpdateTaskWithParentAdmission(ctx, snapshot, &parent, false, validate)
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("writer did not encounter actual SQLite contention: %v", err)
	}
	if reads != 0 {
		t.Fatal("ancestry read preceded writer reservation")
	}
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.UpdateTaskWithParentAdmission(ctx, snapshot, &parent, false, validate); !errors.Is(err, hierarchy.ErrInvalidParent) {
		t.Fatalf("retry ignored committed reverse edge: %v", err)
	}
	if reads != 1 {
		t.Fatal("positive retry did not reach production validation")
	}
	current, err := b.GetTask(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	if current.ParentID != "" || !current.UpdatedAt.Equal(snapshot.UpdatedAt) {
		t.Fatal("busy/cycle loser changed persisted task")
	}
}

func TestTaskHierarchyAdmissionPromotionReadFailure(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "promotion-read", "wf-1", "step-queue")
	before := stepTransitionRowsForTask(t, repo, task.ID)
	if _, err := repo.db.Exec(`UPDATE tasks SET metadata='{' WHERE id=?`, task.ID); err != nil {
		t.Fatal(err)
	}
	task.WorkflowStepID = "step-dest"
	promoted, err := repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, task, "step-queue", "step-dest", 5)
	if err == nil || promoted {
		t.Fatalf("failed current hierarchy read: promoted=%t error=%v", promoted, err)
	}
	var metadata, step string
	if err := repo.db.QueryRow(`SELECT metadata, workflow_step_id FROM tasks WHERE id=?`, task.ID).Scan(&metadata, &step); err != nil {
		t.Fatal(err)
	}
	if metadata != "{" || step != "step-queue" || len(stepTransitionRowsForTask(t, repo, task.ID)) != len(before) {
		t.Fatal("failed read changed the task or transition ledger")
	}
}

func TestTaskHierarchyAdmissionPromotionEncodingFailure(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := context.Background()
	task := createStepTransitionsTestTask(t, repo, "promotion-encoding", "wf-1", "step-queue")
	if _, err := repo.db.Exec(`UPDATE tasks SET metadata='{"workspace":{"mode":"shared_group","group_id":"encoding-group"},"keep":"current"}' WHERE id=?`, task.ID); err != nil {
		t.Fatal(err)
	}
	before := stepTransitionRowsForTask(t, repo, task.ID)
	task.Metadata = map[string]interface{}{"invalid": func() {}}
	task.WorkflowStepID = "step-dest"
	promoted, err := repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, task, "step-queue", "step-dest", 5)
	if err == nil || promoted {
		t.Fatalf("unencodable promotion: promoted=%t error=%v", promoted, err)
	}
	stored, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := stored.Metadata["workspace"].(map[string]interface{})
	if stored.WorkflowStepID != "step-queue" || workspace["mode"] != "shared_group" || workspace["group_id"] != "encoding-group" || stored.Metadata["keep"] != "current" || len(stepTransitionRowsForTask(t, repo, task.ID)) != len(before) {
		t.Fatal("encoding failure changed promotion state or ledger")
	}
}

func TestTaskHierarchyAdmissionLegacyMetadata(t *testing.T) {
	for _, variant := range []string{"ordinary", "promotion"} {
		for _, shape := range []struct {
			name       string
			value      interface{}
			nullParent bool
		}{
			{"null", nil, false}, {"empty", "", false}, {"whitespace", " \n\t ", false}, {"json_null", "null", false}, {"null_parent", "{}", true},
		} {
			t.Run(variant+"/"+shape.name, func(t *testing.T) {
				repo := newStepTransitionsTestRepo(t)
				ctx := context.Background()
				task := createStepTransitionsTestTask(t, repo, "legacy", "wf-1", "step-queue")
				task, err := repo.GetTask(ctx, task.ID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := repo.db.Exec(`UPDATE tasks SET metadata=? WHERE id=?`, shape.value, task.ID); err != nil {
					t.Fatal(err)
				}
				if shape.nullParent {
					if _, err := repo.db.Exec(`UPDATE tasks SET parent_id=NULL WHERE id=?`, task.ID); err != nil {
						t.Fatal(err)
					}
				}
				task.Title = "Legacy rename"
				task.Metadata = map[string]interface{}{"keep": "requested"}
				expectedStep := "step-queue"
				if variant == "ordinary" {
					err = repo.UpdateTask(ctx, task)
				} else {
					task.WorkflowStepID = "step-dest"
					expectedStep = task.WorkflowStepID
					var promoted bool
					promoted, err = repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, task, "step-queue", "step-dest", 5)
					if err == nil && !promoted {
						t.Fatal("legacy promotion was not applied")
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				stored, err := repo.GetTask(ctx, task.ID)
				if err != nil {
					t.Fatal(err)
				}
				if stored.ParentID != "" || stored.Title != "Legacy rename" || stored.Metadata["keep"] != "requested" || stored.WorkflowStepID != expectedStep {
					t.Fatalf("legacy update=%+v", stored)
				}
			})
		}
	}
}
