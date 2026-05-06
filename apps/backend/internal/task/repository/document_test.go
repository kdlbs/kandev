package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedDocTask creates the prerequisite workspace, workflow, and task for document tests.
func seedDocTask(t *testing.T, ctx context.Context, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
}, taskID string) {
	t.Helper()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-doc", Name: "Doc WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-doc", WorkspaceID: "ws-doc", Name: "WF"})
	now := time.Now().UTC()
	_ = repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws-doc", WorkflowID: "wf-doc",
		Title: "T", State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now,
	})
}

func TestDocument_CreateAndGet(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedDocTask(t, ctx, repo, "task-d1")

	doc := &models.TaskDocument{
		TaskID:     "task-d1",
		Key:        "spec",
		Type:       "spec",
		Title:      "Feature Spec",
		Content:    "## Spec\nHello",
		AuthorKind: "agent",
		AuthorName: "Agent",
	}
	if err := repo.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("create: %v", err)
	}
	if doc.ID == "" {
		t.Fatal("expected ID to be set after create")
	}

	got, err := repo.GetDocument(ctx, "task-d1", "spec")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected document, got nil")
	}
	if got.Key != "spec" || got.Type != "spec" || got.Title != "Feature Spec" {
		t.Errorf("unexpected doc: %+v", got)
	}
}

func TestDocument_GetNonExistent(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedDocTask(t, ctx, repo, "task-d2")

	got, err := repo.GetDocument(ctx, "task-d2", "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing document, got %+v", got)
	}
}

func TestDocument_Update(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedDocTask(t, ctx, repo, "task-d3")

	doc := &models.TaskDocument{
		TaskID: "task-d3", Key: "plan", Type: "plan",
		Title: "Plan", Content: "v1", AuthorKind: "agent", AuthorName: "Agent",
	}
	_ = repo.CreateDocument(ctx, doc)

	doc.Content = "v2"
	doc.Title = "Updated Plan"
	if err := repo.UpdateDocument(ctx, doc); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := repo.GetDocument(ctx, "task-d3", "plan")
	if got.Content != "v2" || got.Title != "Updated Plan" {
		t.Errorf("update not reflected: %+v", got)
	}
}

func TestDocument_Delete(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedDocTask(t, ctx, repo, "task-d4")

	_ = repo.CreateDocument(ctx, &models.TaskDocument{
		TaskID: "task-d4", Key: "notes", Type: "notes",
		Title: "Notes", AuthorKind: "user", AuthorName: "User",
	})

	if err := repo.DeleteDocument(ctx, "task-d4", "notes"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	got, _ := repo.GetDocument(ctx, "task-d4", "notes")
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

func TestDocument_ListDocuments(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedDocTask(t, ctx, repo, "task-d5")

	keys := []string{"plan", "notes", "spec"}
	for _, k := range keys {
		_ = repo.CreateDocument(ctx, &models.TaskDocument{
			TaskID: "task-d5", Key: k, Type: k,
			Title: k, AuthorKind: "agent", AuthorName: "Agent",
		})
	}

	list, err := repo.ListDocuments(ctx, "task-d5")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 documents, got %d", len(list))
	}
	// Should be ordered by key alphabetically: notes, plan, spec
	if list[0].Key != "notes" || list[1].Key != "plan" || list[2].Key != "spec" {
		t.Errorf("unexpected order: %v, %v, %v", list[0].Key, list[1].Key, list[2].Key)
	}
}

func TestDocument_Revisions_InsertAndList(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedDocTask(t, ctx, repo, "task-dr1")

	_ = repo.CreateDocument(ctx, &models.TaskDocument{
		TaskID: "task-dr1", Key: "spec", Type: "spec",
		Title: "Spec", AuthorKind: "agent", AuthorName: "Agent",
	})

	for i := 1; i <= 3; i++ {
		rev := &models.TaskDocumentRevision{
			TaskID:         "task-dr1",
			DocumentKey:    "spec",
			RevisionNumber: i,
			Title:          "Spec",
			Content:        "rev " + string(rune('0'+i)),
			AuthorKind:     "agent",
			AuthorName:     "Agent",
		}
		if err := repo.InsertDocumentRevision(ctx, rev); err != nil {
			t.Fatalf("insert rev %d: %v", i, err)
		}
	}

	list, err := repo.ListDocumentRevisions(ctx, "task-dr1", "spec", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 revisions, got %d", len(list))
	}
	if list[0].RevisionNumber != 3 || list[2].RevisionNumber != 1 {
		t.Errorf("expected newest-first ordering, got %d..%d", list[0].RevisionNumber, list[2].RevisionNumber)
	}
}

func TestDocument_Revisions_GetLatest(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedDocTask(t, ctx, repo, "task-dr2")

	_ = repo.CreateDocument(ctx, &models.TaskDocument{
		TaskID: "task-dr2", Key: "plan", Type: "plan",
		Title: "Plan", AuthorKind: "agent", AuthorName: "Agent",
	})

	latest, _ := repo.GetLatestDocumentRevision(ctx, "task-dr2", "plan")
	if latest != nil {
		t.Errorf("expected nil before any revisions, got %+v", latest)
	}

	_ = repo.InsertDocumentRevision(ctx, &models.TaskDocumentRevision{
		TaskID: "task-dr2", DocumentKey: "plan", RevisionNumber: 1,
		Content: "a", AuthorKind: "agent", AuthorName: "Agent",
	})
	_ = repo.InsertDocumentRevision(ctx, &models.TaskDocumentRevision{
		TaskID: "task-dr2", DocumentKey: "plan", RevisionNumber: 2,
		Content: "b", AuthorKind: "user", AuthorName: "User",
	})

	latest, err := repo.GetLatestDocumentRevision(ctx, "task-dr2", "plan")
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest == nil || latest.RevisionNumber != 2 || latest.Content != "b" {
		t.Errorf("expected rev 2 content 'b', got %+v", latest)
	}
}

func TestDocument_Revisions_NextRevisionNumber(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedDocTask(t, ctx, repo, "task-dr3")

	n, err := repo.NextDocumentRevisionNumber(ctx, "task-dr3", "spec")
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}

	_ = repo.CreateDocument(ctx, &models.TaskDocument{
		TaskID: "task-dr3", Key: "spec", Type: "spec",
		Title: "Spec", AuthorKind: "agent", AuthorName: "Agent",
	})
	_ = repo.InsertDocumentRevision(ctx, &models.TaskDocumentRevision{
		TaskID: "task-dr3", DocumentKey: "spec", RevisionNumber: 1, AuthorKind: "agent",
	})
	_ = repo.InsertDocumentRevision(ctx, &models.TaskDocumentRevision{
		TaskID: "task-dr3", DocumentKey: "spec", RevisionNumber: 2, AuthorKind: "agent",
	})

	n, _ = repo.NextDocumentRevisionNumber(ctx, "task-dr3", "spec")
	if n != 3 {
		t.Errorf("expected 3, got %d", n)
	}
}

func TestDocument_WriteDocumentRevision_NewRevision(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedDocTask(t, ctx, repo, "task-dw1")

	head := &models.TaskDocument{
		TaskID: "task-dw1", Key: "spec", Type: "spec",
		Title: "Spec", Content: "v1",
		AuthorKind: "agent", AuthorName: "Agent",
	}
	rev := &models.TaskDocumentRevision{
		TaskID: "task-dw1", DocumentKey: "spec",
		Title: "Spec", Content: "v1",
		AuthorKind: "agent", AuthorName: "Agent",
	}

	if err := repo.WriteDocumentRevision(ctx, head, rev, nil); err != nil {
		t.Fatalf("write: %v", err)
	}
	if rev.RevisionNumber != 1 {
		t.Errorf("expected revision_number=1, got %d", rev.RevisionNumber)
	}
	if rev.ID == "" {
		t.Error("expected ID to be set")
	}

	// Second write creates revision 2.
	head2 := &models.TaskDocument{
		TaskID: "task-dw1", Key: "spec", Type: "spec",
		Title: "Spec", Content: "v2",
		AuthorKind: "agent", AuthorName: "Agent",
	}
	rev2 := &models.TaskDocumentRevision{
		TaskID: "task-dw1", DocumentKey: "spec",
		Title: "Spec", Content: "v2",
		AuthorKind: "agent", AuthorName: "Agent",
	}
	if err := repo.WriteDocumentRevision(ctx, head2, rev2, nil); err != nil {
		t.Fatalf("write 2: %v", err)
	}
	if rev2.RevisionNumber != 2 {
		t.Errorf("expected revision_number=2, got %d", rev2.RevisionNumber)
	}

	// HEAD should reflect latest content.
	got, _ := repo.GetDocument(ctx, "task-dw1", "spec")
	if got == nil || got.Content != "v2" {
		t.Errorf("expected HEAD content 'v2', got %+v", got)
	}
}

func TestDocument_WriteDocumentRevision_Coalesce(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedDocTask(t, ctx, repo, "task-dw2")

	head := &models.TaskDocument{
		TaskID: "task-dw2", Key: "plan", Type: "plan",
		Title: "Plan", Content: "v1",
		AuthorKind: "agent", AuthorName: "Agent",
	}
	rev := &models.TaskDocumentRevision{
		TaskID: "task-dw2", DocumentKey: "plan",
		Title: "Plan", Content: "v1",
		AuthorKind: "agent", AuthorName: "Agent",
	}
	_ = repo.WriteDocumentRevision(ctx, head, rev, nil)
	firstRevID := rev.ID

	// Coalesce into the first revision.
	head2 := &models.TaskDocument{
		TaskID: "task-dw2", Key: "plan", Type: "plan",
		Title: "Plan", Content: "v2",
		AuthorKind: "agent", AuthorName: "Agent",
	}
	rev2 := &models.TaskDocumentRevision{
		TaskID: "task-dw2", DocumentKey: "plan",
		Title: "Plan", Content: "v2",
		AuthorKind: "agent", AuthorName: "Agent",
	}
	if err := repo.WriteDocumentRevision(ctx, head2, rev2, &firstRevID); err != nil {
		t.Fatalf("coalesce write: %v", err)
	}
	if rev2.ID != firstRevID {
		t.Errorf("expected coalesced revision to have original ID %q, got %q", firstRevID, rev2.ID)
	}

	// Still only one revision row.
	list, _ := repo.ListDocumentRevisions(ctx, "task-dw2", "plan", 0)
	if len(list) != 1 {
		t.Errorf("coalesce should not add rows; have %d", len(list))
	}
	if list[0].Content != "v2" {
		t.Errorf("expected merged content 'v2', got %q", list[0].Content)
	}
}

func TestDocument_Revisions_RevertOf(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedDocTask(t, ctx, repo, "task-dr4")

	_ = repo.CreateDocument(ctx, &models.TaskDocument{
		TaskID: "task-dr4", Key: "spec", Type: "spec",
		Title: "Spec", AuthorKind: "agent", AuthorName: "Agent",
	})

	target := &models.TaskDocumentRevision{
		TaskID: "task-dr4", DocumentKey: "spec", RevisionNumber: 1,
		Content: "original", AuthorKind: "agent", AuthorName: "Agent",
	}
	_ = repo.InsertDocumentRevision(ctx, target)

	refID := target.ID
	revertRev := &models.TaskDocumentRevision{
		TaskID: "task-dr4", DocumentKey: "spec", RevisionNumber: 2,
		Content: "original", AuthorKind: "user", AuthorName: "User",
		RevertOfRevisionID: &refID,
	}
	_ = repo.InsertDocumentRevision(ctx, revertRev)

	got, _ := repo.GetDocumentRevision(ctx, revertRev.ID)
	if got.RevertOfRevisionID == nil || *got.RevertOfRevisionID != refID {
		t.Errorf("expected revert_of_revision_id=%q, got %v", refID, got.RevertOfRevisionID)
	}
}

func TestDocument_UniqueKeyConstraint(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedDocTask(t, ctx, repo, "task-du1")

	doc := &models.TaskDocument{
		TaskID: "task-du1", Key: "spec", Type: "spec",
		Title: "Spec", AuthorKind: "agent", AuthorName: "Agent",
	}
	if err := repo.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("first create: %v", err)
	}
	// Second create with same key should fail.
	doc2 := &models.TaskDocument{
		TaskID: "task-du1", Key: "spec", Type: "spec",
		Title: "Spec 2", AuthorKind: "agent", AuthorName: "Agent",
	}
	if err := repo.CreateDocument(ctx, doc2); err == nil {
		t.Fatal("expected UNIQUE violation on duplicate (task_id, key)")
	}
}

func TestDocument_IsolatedByTask(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedDocTask(t, ctx, repo, "task-di1")
	seedDocTask(t, ctx, repo, "task-di2")

	_ = repo.CreateDocument(ctx, &models.TaskDocument{
		TaskID: "task-di1", Key: "plan", Type: "plan",
		Title: "Plan A", AuthorKind: "agent", AuthorName: "Agent",
	})
	_ = repo.CreateDocument(ctx, &models.TaskDocument{
		TaskID: "task-di2", Key: "plan", Type: "plan",
		Title: "Plan B", AuthorKind: "agent", AuthorName: "Agent",
	})

	list1, _ := repo.ListDocuments(ctx, "task-di1")
	list2, _ := repo.ListDocuments(ctx, "task-di2")

	if len(list1) != 1 || list1[0].Title != "Plan A" {
		t.Errorf("task-di1 should have Plan A: %+v", list1)
	}
	if len(list2) != 1 || list2[0].Title != "Plan B" {
		t.Errorf("task-di2 should have Plan B: %+v", list2)
	}
}
