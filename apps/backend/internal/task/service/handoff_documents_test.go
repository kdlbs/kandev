package service

import (
	"context"
	"errors"
	"testing"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// docBlockerRepo is a BlockerRepository stub that only answers the blocker
// lookup the document read guard uses.
type docBlockerRepo struct {
	BlockerRepository
	blockers map[string][]string
	err      error
}

func (d *docBlockerRepo) ListTaskBlockers(_ context.Context, taskID string) ([]*officemodels.TaskBlocker, error) {
	if d.err != nil {
		return nil, d.err
	}
	out := make([]*officemodels.TaskBlocker, 0, len(d.blockers[taskID]))
	for _, blockerID := range d.blockers[taskID] {
		out = append(out, &officemodels.TaskBlocker{TaskID: taskID, BlockerTaskID: blockerID})
	}
	return out, nil
}

// newDocumentHandoffService wires a HandoffService over the real repository
// with a task tree: parent → (child-a, child-b), plus an unrelated task.
func newDocumentHandoffService(t *testing.T, blockers *docBlockerRepo) (*HandoffService, *sqliterepo.Repository) {
	t.Helper()
	_, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-handoff", Name: "Handoff"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-other", Name: "Other"}); err != nil {
		t.Fatalf("create other workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-handoff", WorkspaceID: "ws-handoff", Name: "flow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-other", WorkspaceID: "ws-other", Name: "other flow"}); err != nil {
		t.Fatalf("create other workflow: %v", err)
	}
	seed := func(id, parentID, workspaceID, workflowID string) {
		if err := repo.CreateTask(ctx, &models.Task{
			ID: id, WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: "step-1",
			Title: id, State: v1.TaskStateCreated, Priority: "medium", ParentID: parentID,
		}); err != nil {
			t.Fatalf("create task %s: %v", id, err)
		}
	}
	seed("parent", "", "ws-handoff", "wf-handoff")
	seed("child-a", "parent", "ws-handoff", "wf-handoff")
	seed("child-b", "parent", "ws-handoff", "wf-handoff")
	seed("stranger", "", "ws-handoff", "wf-handoff")
	seed("foreign", "", "ws-other", "wf-other")

	log := accessTestLogger(t)
	docs := NewDocumentService(repo, log)
	var blockerRepo BlockerRepository
	if blockers != nil {
		blockerRepo = blockers
	}
	return NewHandoffService(repo, repo, docs, blockerRepo, nil, log), repo
}

func TestGetDocumentForCallerEnforcesReadAccess(t *testing.T) {
	svc, repo := newDocumentHandoffService(t, nil)
	ctx := context.Background()
	seedHandoffDocument(t, repo, "parent", "spec", "parent body")
	seedHandoffDocument(t, repo, "stranger", "spec", "stranger body")
	seedHandoffDocument(t, repo, "foreign", "spec", "foreign body")

	if _, err := svc.GetDocumentForCaller(ctx, "child-a", "parent", ""); !errors.Is(err, ErrDocumentKeyRequired) {
		t.Fatalf("empty key = %v, want ErrDocumentKeyRequired", err)
	}
	if _, err := svc.GetDocumentForCaller(ctx, "child-a", "", "spec"); !errors.Is(err, ErrDocumentTaskRequired) {
		t.Fatalf("empty target = %v, want ErrDocumentTaskRequired", err)
	}

	// Ancestor, self, and sibling reads are allowed.
	for _, pair := range [][2]string{{"child-a", "parent"}, {"parent", "parent"}} {
		doc, err := svc.GetDocumentForCaller(ctx, pair[0], pair[1], "spec")
		if err != nil {
			t.Fatalf("%s reading %s: %v", pair[0], pair[1], err)
		}
		if doc.Content != "parent body" {
			t.Fatalf("content = %q", doc.Content)
		}
	}
	seedHandoffDocument(t, repo, "child-b", "notes", "sibling body")
	sibling, err := svc.GetDocumentForCaller(ctx, "child-a", "child-b", "notes")
	if err != nil {
		t.Fatalf("sibling read: %v", err)
	}
	if sibling.Content != "sibling body" {
		t.Fatalf("sibling content = %q", sibling.Content)
	}

	// Unrelated same-workspace task and cross-workspace task are denied.
	if _, err := svc.GetDocumentForCaller(ctx, "child-a", "stranger", "spec"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("unrelated read = %v, want ErrAccessDenied", err)
	}
	if _, err := svc.GetDocumentForCaller(ctx, "child-a", "foreign", "spec"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("cross-workspace read = %v, want ErrAccessDenied", err)
	}
}

func TestGetDocumentForCallerGrantsBlockerReads(t *testing.T) {
	blockers := &docBlockerRepo{blockers: map[string][]string{"stranger": {"child-a"}}}
	svc, repo := newDocumentHandoffService(t, blockers)
	ctx := context.Background()
	seedHandoffDocument(t, repo, "child-a", "spec", "producer body")

	doc, err := svc.GetDocumentForCaller(ctx, "stranger", "child-a", "spec")
	if err != nil {
		t.Fatalf("blocker read: %v", err)
	}
	if doc.Content != "producer body" {
		t.Fatalf("content = %q", doc.Content)
	}

	// The edge is directional: the producer cannot read back through it.
	seedHandoffDocument(t, repo, "stranger", "spec", "consumer body")
	if _, err := svc.GetDocumentForCaller(ctx, "child-a", "stranger", "spec"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("reverse blocker read = %v, want ErrAccessDenied", err)
	}
	// Blocker edges never grant writes.
	if _, err := svc.WriteDocumentForCaller(ctx, "stranger", "child-a", "spec", "", "", "x", "", ""); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("blocker write = %v, want ErrAccessDenied", err)
	}
}

func TestListDocumentsForCallerStripsContent(t *testing.T) {
	svc, repo := newDocumentHandoffService(t, nil)
	ctx := context.Background()
	seedHandoffDocument(t, repo, "parent", "spec", "secret body")
	seedHandoffDocument(t, repo, "parent", "notes", "another secret")

	if _, err := svc.ListDocumentsForCaller(ctx, "child-a", ""); !errors.Is(err, ErrDocumentTaskRequired) {
		t.Fatalf("empty target = %v, want ErrDocumentTaskRequired", err)
	}
	if _, err := svc.ListDocumentsForCaller(ctx, "child-a", "stranger"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("unrelated list = %v, want ErrAccessDenied", err)
	}

	listed, err := svc.ListDocumentsForCaller(ctx, "child-a", "parent")
	if err != nil {
		t.Fatalf("ListDocumentsForCaller: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d documents, want 2", len(listed))
	}
	keys := map[string]bool{}
	for _, doc := range listed {
		keys[doc.Key] = true
		if doc.Content != "" {
			t.Fatalf("document %q leaked content %q through the metadata listing", doc.Key, doc.Content)
		}
	}
	if !keys["spec"] || !keys["notes"] {
		t.Fatalf("listed keys = %v, want spec and notes", keys)
	}

	// The stripping is a copy: the stored rows keep their bodies.
	stored, err := repo.GetDocument(ctx, "parent", "spec")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if stored.Content != "secret body" {
		t.Fatalf("stored content = %q, want it untouched", stored.Content)
	}
}

func TestWriteDocumentForCallerEnforcesWriteAccess(t *testing.T) {
	svc, repo := newDocumentHandoffService(t, nil)
	ctx := context.Background()

	if _, err := svc.WriteDocumentForCaller(ctx, "child-a", "parent", "", "", "", "body", "", ""); !errors.Is(err, ErrDocumentKeyRequired) {
		t.Fatalf("empty key = %v, want ErrDocumentKeyRequired", err)
	}
	if _, err := svc.WriteDocumentForCaller(ctx, "child-a", "", "spec", "", "", "body", "", ""); !errors.Is(err, ErrDocumentTaskRequired) {
		t.Fatalf("empty target = %v, want ErrDocumentTaskRequired", err)
	}
	if _, err := svc.WriteDocumentForCaller(ctx, "child-a", "stranger", "spec", "", "", "body", "", ""); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("unrelated write = %v, want ErrAccessDenied", err)
	}
	if _, err := svc.WriteDocumentForCaller(ctx, "child-a", "foreign", "spec", "", "", "body", "", ""); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("cross-workspace write = %v, want ErrAccessDenied", err)
	}

	doc, err := svc.WriteDocumentForCaller(ctx, "child-a", "child-a", "spec", "custom", "Spec", "own body", "user", "Ada")
	if err != nil {
		t.Fatalf("self write: %v", err)
	}
	if doc.Content != "own body" || doc.Title != "Spec" || doc.AuthorName != "Ada" {
		t.Fatalf("document = %+v", doc)
	}
	stored, err := repo.GetDocument(ctx, "child-a", "spec")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if stored.Content != "own body" {
		t.Fatalf("persisted content = %q", stored.Content)
	}
}

func seedHandoffDocument(t *testing.T, repo *sqliterepo.Repository, taskID, key, content string) {
	t.Helper()
	if err := repo.CreateDocument(context.Background(), &models.TaskDocument{
		TaskID: taskID, Key: key, Type: "custom", Title: key, Content: content,
		AuthorKind: "user", AuthorName: "User",
	}); err != nil {
		t.Fatalf("seed document %s/%s: %v", taskID, key, err)
	}
}
