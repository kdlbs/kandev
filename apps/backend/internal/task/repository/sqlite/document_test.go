package sqlite

//revive:disable:file-length-limit // Document HEAD + revision persistence coverage is intentionally scenario-heavy.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func seedTaskForDocs(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	if err := repo.CreateTask(context.Background(), &models.Task{ID: taskID, Title: taskID}); err != nil {
		t.Fatalf("CreateTask(%s): %v", taskID, err)
	}
}

func fullDocument(taskID string) *models.TaskDocument {
	return &models.TaskDocument{
		ID:         "doc-full",
		TaskID:     taskID,
		Key:        "spec",
		Type:       "spec",
		Title:      "Feature specification",
		Content:    "# Spec\n\nEverything.",
		AuthorKind: "user",
		AuthorName: "jcfs",
		Filename:   "spec.md",
		MimeType:   "text/markdown",
		SizeBytes:  4096,
		DiskPath:   "/var/kandev/docs/spec.md",
	}
}

func assertDocumentEqual(t *testing.T, got, want *models.TaskDocument) {
	t.Helper()
	if got == nil {
		t.Fatal("document is nil")
	}
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.TaskID != want.TaskID {
		t.Errorf("TaskID = %q, want %q", got.TaskID, want.TaskID)
	}
	if got.Key != want.Key {
		t.Errorf("Key = %q, want %q", got.Key, want.Key)
	}
	if got.Type != want.Type {
		t.Errorf("Type = %q, want %q", got.Type, want.Type)
	}
	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if got.Content != want.Content {
		t.Errorf("Content = %q, want %q", got.Content, want.Content)
	}
	if got.AuthorKind != want.AuthorKind {
		t.Errorf("AuthorKind = %q, want %q", got.AuthorKind, want.AuthorKind)
	}
	if got.AuthorName != want.AuthorName {
		t.Errorf("AuthorName = %q, want %q", got.AuthorName, want.AuthorName)
	}
	if got.Filename != want.Filename {
		t.Errorf("Filename = %q, want %q", got.Filename, want.Filename)
	}
	if got.MimeType != want.MimeType {
		t.Errorf("MimeType = %q, want %q", got.MimeType, want.MimeType)
	}
	if got.SizeBytes != want.SizeBytes {
		t.Errorf("SizeBytes = %d, want %d", got.SizeBytes, want.SizeBytes)
	}
	if got.DiskPath != want.DiskPath {
		t.Errorf("DiskPath = %q, want %q", got.DiskPath, want.DiskPath)
	}
	assertTimeEqual(t, "CreatedAt", got.CreatedAt, want.CreatedAt)
	assertTimeEqual(t, "UpdatedAt", got.UpdatedAt, want.UpdatedAt)
}

func assertDocRevisionEqual(t *testing.T, got, want *models.TaskDocumentRevision) {
	t.Helper()
	if got == nil {
		t.Fatal("revision is nil")
	}
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.TaskID != want.TaskID {
		t.Errorf("TaskID = %q, want %q", got.TaskID, want.TaskID)
	}
	if got.DocumentKey != want.DocumentKey {
		t.Errorf("DocumentKey = %q, want %q", got.DocumentKey, want.DocumentKey)
	}
	if got.RevisionNumber != want.RevisionNumber {
		t.Errorf("RevisionNumber = %d, want %d", got.RevisionNumber, want.RevisionNumber)
	}
	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if got.Content != want.Content {
		t.Errorf("Content = %q, want %q", got.Content, want.Content)
	}
	if got.AuthorKind != want.AuthorKind {
		t.Errorf("AuthorKind = %q, want %q", got.AuthorKind, want.AuthorKind)
	}
	if got.AuthorName != want.AuthorName {
		t.Errorf("AuthorName = %q, want %q", got.AuthorName, want.AuthorName)
	}
	switch {
	case want.RevertOfRevisionID == nil && got.RevertOfRevisionID != nil:
		t.Errorf("RevertOfRevisionID = %q, want nil", *got.RevertOfRevisionID)
	case want.RevertOfRevisionID != nil && got.RevertOfRevisionID == nil:
		t.Errorf("RevertOfRevisionID = nil, want %q", *want.RevertOfRevisionID)
	case want.RevertOfRevisionID != nil && *got.RevertOfRevisionID != *want.RevertOfRevisionID:
		t.Errorf("RevertOfRevisionID = %q, want %q", *got.RevertOfRevisionID, *want.RevertOfRevisionID)
	}
	assertTimeEqual(t, "CreatedAt", got.CreatedAt, want.CreatedAt)
	assertTimeEqual(t, "UpdatedAt", got.UpdatedAt, want.UpdatedAt)
}

func TestCreateDocumentRoundTripsEveryField(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-doc-full")

	want := fullDocument("task-doc-full")
	before := time.Now().UTC().Add(-time.Second)
	if err := repo.CreateDocument(ctx, want); err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if want.CreatedAt.Before(before) || !want.CreatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("CreateDocument stamped CreatedAt=%v UpdatedAt=%v, want equal times after %v",
			want.CreatedAt, want.UpdatedAt, before)
	}

	got, err := repo.GetDocument(ctx, "task-doc-full", "spec")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	assertDocumentEqual(t, got, want)

	listed, err := repo.ListDocuments(ctx, "task-doc-full")
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListDocuments returned %d rows, want 1", len(listed))
	}
	assertDocumentEqual(t, listed[0], want)
}

func TestCreateDocumentGeneratesIDWhenEmpty(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-doc-id")

	doc := &models.TaskDocument{TaskID: "task-doc-id", Key: "plan", Type: "plan", Title: "Plan"}
	if err := repo.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if doc.ID == "" {
		t.Fatal("CreateDocument left ID empty; a UUID should have been generated")
	}
	got, err := repo.GetDocument(ctx, "task-doc-id", "plan")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if got.ID != doc.ID {
		t.Errorf("persisted ID = %q, want the generated %q", got.ID, doc.ID)
	}
}

func TestCreateDocumentRejectsDuplicateTaskKey(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-doc-dupe")

	first := &models.TaskDocument{ID: "doc-dupe-1", TaskID: "task-doc-dupe", Key: "plan", Type: "plan", Title: "One"}
	if err := repo.CreateDocument(ctx, first); err != nil {
		t.Fatalf("CreateDocument(first): %v", err)
	}
	second := &models.TaskDocument{ID: "doc-dupe-2", TaskID: "task-doc-dupe", Key: "plan", Type: "plan", Title: "Two"}
	if err := repo.CreateDocument(ctx, second); err == nil {
		t.Fatal("CreateDocument accepted a duplicate (task_id, key); the UNIQUE constraint should reject it")
	}
	got, err := repo.GetDocument(ctx, "task-doc-dupe", "plan")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if got.Title != "One" {
		t.Errorf("Title = %q, want the original %q to survive the rejected insert", got.Title, "One")
	}
}

func TestGetDocumentReturnsNilNilWhenMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()

	got, err := repo.GetDocument(ctx, "task-doc-missing", "plan")
	if err != nil {
		t.Fatalf("GetDocument returned error %v, want the nil,nil not-found contract", err)
	}
	if got != nil {
		t.Errorf("GetDocument = %+v, want nil", got)
	}
}

func TestUpdateDocumentWritesEveryMutableFieldAndReportsMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-doc-update")

	doc := &models.TaskDocument{
		ID: "doc-update", TaskID: "task-doc-update", Key: "notes", Type: "notes",
		Title: "Before", Content: "old", AuthorKind: "agent", AuthorName: "claude",
		Filename: "old.md", MimeType: "text/plain", SizeBytes: 1, DiskPath: "/old",
	}
	if err := repo.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	createdAt := doc.CreatedAt

	doc.Type = "spec"
	doc.Title = "After"
	doc.Content = "new content"
	doc.AuthorKind = "user"
	doc.AuthorName = "jcfs"
	doc.Filename = "new.md"
	doc.MimeType = "text/markdown"
	doc.SizeBytes = 999
	doc.DiskPath = "/new"
	if err := repo.UpdateDocument(ctx, doc); err != nil {
		t.Fatalf("UpdateDocument: %v", err)
	}

	got, err := repo.GetDocument(ctx, "task-doc-update", "notes")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	assertDocumentEqual(t, got, doc)
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want it preserved at %v across the update", got.CreatedAt, createdAt)
	}
	if !got.UpdatedAt.After(createdAt) {
		t.Errorf("UpdatedAt = %v, want it bumped past CreatedAt %v", got.UpdatedAt, createdAt)
	}

	missing := &models.TaskDocument{TaskID: "task-doc-update", Key: "nope"}
	err = repo.UpdateDocument(ctx, missing)
	if err == nil {
		t.Fatal("UpdateDocument on a missing row returned nil")
	}
	if !strings.Contains(err.Error(), "document not found") {
		t.Errorf("UpdateDocument error = %q, want a document-not-found message", err)
	}
}

func TestDeleteDocumentRemovesHeadAndReportsMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-doc-delete")

	for _, key := range []string{"plan", "spec"} {
		if err := repo.CreateDocument(ctx, &models.TaskDocument{
			ID: "doc-del-" + key, TaskID: "task-doc-delete", Key: key, Type: key, Title: key,
		}); err != nil {
			t.Fatalf("CreateDocument(%s): %v", key, err)
		}
	}

	if err := repo.DeleteDocument(ctx, "task-doc-delete", "plan"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_documents WHERE task_id = ?`, "task-doc-delete"); got != 1 {
		t.Errorf("rows after delete = %d, want 1", got)
	}
	gone, err := repo.GetDocument(ctx, "task-doc-delete", "plan")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if gone != nil {
		t.Errorf("deleted document still readable: %+v", gone)
	}
	survivor, err := repo.GetDocument(ctx, "task-doc-delete", "spec")
	if err != nil || survivor == nil {
		t.Fatalf("GetDocument(spec) = %+v, %v; want the sibling key untouched", survivor, err)
	}

	err = repo.DeleteDocument(ctx, "task-doc-delete", "plan")
	if err == nil {
		t.Fatal("second DeleteDocument returned nil; a missing row must be reported")
	}
	if !strings.Contains(err.Error(), "document not found") {
		t.Errorf("DeleteDocument error = %q, want a document-not-found message", err)
	}
}

// TestDeleteDocumentLeavesRevisionsBehind pins the current behavior: the
// revisions table's only foreign key points at tasks(id), not at the document
// HEAD row, so removing a HEAD does not cascade to its history.
func TestDeleteDocumentLeavesRevisionsBehind(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-doc-orphan")

	if err := repo.CreateDocument(ctx, &models.TaskDocument{
		ID: "doc-orphan", TaskID: "task-doc-orphan", Key: "plan", Type: "plan", Title: "Plan",
	}); err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if err := repo.InsertDocumentRevision(ctx, &models.TaskDocumentRevision{
		ID: "rev-orphan", TaskID: "task-doc-orphan", DocumentKey: "plan", RevisionNumber: 1,
		Title: "Plan", Content: "v1", AuthorKind: "agent",
	}); err != nil {
		t.Fatalf("InsertDocumentRevision: %v", err)
	}

	if err := repo.DeleteDocument(ctx, "task-doc-orphan", "plan"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_document_revisions WHERE task_id = ?`, "task-doc-orphan"); got != 1 {
		t.Errorf("revision rows after HEAD delete = %d, want 1 (no cascade from task_documents)", got)
	}
}

func TestDocumentsAndRevisionsCascadeOnTaskDelete(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-doc-cascade")

	if err := repo.CreateDocument(ctx, &models.TaskDocument{
		ID: "doc-cascade", TaskID: "task-doc-cascade", Key: "plan", Type: "plan", Title: "Plan",
	}); err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if err := repo.InsertDocumentRevision(ctx, &models.TaskDocumentRevision{
		ID: "rev-cascade", TaskID: "task-doc-cascade", DocumentKey: "plan", RevisionNumber: 1,
		Title: "Plan", Content: "v1", AuthorKind: "agent",
	}); err != nil {
		t.Fatalf("InsertDocumentRevision: %v", err)
	}

	if err := repo.DeleteTask(ctx, "task-doc-cascade"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_documents WHERE task_id = ?`, "task-doc-cascade"); got != 0 {
		t.Errorf("document rows after task delete = %d, want 0 (FK cascade)", got)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_document_revisions WHERE task_id = ?`, "task-doc-cascade"); got != 0 {
		t.Errorf("revision rows after task delete = %d, want 0 (FK cascade)", got)
	}
}

func TestListDocumentsOrdersByKeyAndScopesToTask(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-doc-list")
	seedTaskForDocs(t, repo, "task-doc-list-other")

	for _, key := range []string{"spec", "alpha", "notes"} {
		if err := repo.CreateDocument(ctx, &models.TaskDocument{
			ID: "doc-list-" + key, TaskID: "task-doc-list", Key: key, Type: key, Title: key,
		}); err != nil {
			t.Fatalf("CreateDocument(%s): %v", key, err)
		}
	}
	if err := repo.CreateDocument(ctx, &models.TaskDocument{
		ID: "doc-list-foreign", TaskID: "task-doc-list-other", Key: "aaa", Type: "plan", Title: "foreign",
	}); err != nil {
		t.Fatalf("CreateDocument(foreign): %v", err)
	}

	got, err := repo.ListDocuments(ctx, "task-doc-list")
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	want := []string{"alpha", "notes", "spec"}
	if len(got) != len(want) {
		t.Fatalf("ListDocuments returned %d rows, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Key != want[i] {
			t.Fatalf("ListDocuments keys = %q at index %d, want %q (ORDER BY key)", got[i].Key, i, want[i])
		}
	}

	empty, err := repo.ListDocuments(ctx, "task-doc-list-nothing")
	if err != nil {
		t.Fatalf("ListDocuments(unknown task): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("ListDocuments(unknown task) = %+v, want empty", empty)
	}
}

func TestInsertDocumentRevisionRoundTripsEveryField(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-rev-full")

	revertOf := "rev-previous"
	want := &models.TaskDocumentRevision{
		ID: "rev-full", TaskID: "task-rev-full", DocumentKey: "spec", RevisionNumber: 7,
		Title: "Spec v7", Content: "seven", AuthorKind: "user", AuthorName: "jcfs",
		RevertOfRevisionID: &revertOf,
		CreatedAt:          time.Date(2026, 3, 3, 3, 3, 3, 456000000, time.UTC),
	}
	if err := repo.InsertDocumentRevision(ctx, want); err != nil {
		t.Fatalf("InsertDocumentRevision: %v", err)
	}
	if want.UpdatedAt.IsZero() {
		t.Fatal("InsertDocumentRevision left UpdatedAt zero")
	}

	got, err := repo.GetDocumentRevision(ctx, "rev-full")
	if err != nil {
		t.Fatalf("GetDocumentRevision: %v", err)
	}
	assertDocRevisionEqual(t, got, want)

	latest, err := repo.GetLatestDocumentRevision(ctx, "task-rev-full", "spec")
	if err != nil {
		t.Fatalf("GetLatestDocumentRevision: %v", err)
	}
	assertDocRevisionEqual(t, latest, want)

	listed, err := repo.ListDocumentRevisions(ctx, "task-rev-full", "spec", 0)
	if err != nil {
		t.Fatalf("ListDocumentRevisions: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListDocumentRevisions returned %d rows, want 1", len(listed))
	}
	assertDocRevisionEqual(t, listed[0], want)
}

func TestInsertDocumentRevisionPreservesSuppliedCreatedAtAndDefaultsID(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-rev-defaults")

	rev := &models.TaskDocumentRevision{
		TaskID: "task-rev-defaults", DocumentKey: "plan", RevisionNumber: 1,
		Title: "Plan", Content: "v1", AuthorKind: "agent",
	}
	before := time.Now().UTC().Add(-time.Second)
	if err := repo.InsertDocumentRevision(ctx, rev); err != nil {
		t.Fatalf("InsertDocumentRevision: %v", err)
	}
	if rev.ID == "" {
		t.Fatal("ID left empty; a UUID should have been generated")
	}
	if rev.CreatedAt.Before(before) {
		t.Errorf("CreatedAt = %v, want a freshly stamped time after %v", rev.CreatedAt, before)
	}

	got, err := repo.GetDocumentRevision(ctx, rev.ID)
	if err != nil {
		t.Fatalf("GetDocumentRevision: %v", err)
	}
	if got.RevertOfRevisionID != nil {
		t.Errorf("RevertOfRevisionID = %q, want nil for a NULL column", *got.RevertOfRevisionID)
	}
}

func TestGetDocumentRevisionReturnsNilNilWhenMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()

	got, err := repo.GetDocumentRevision(ctx, "rev-missing")
	if err != nil {
		t.Fatalf("GetDocumentRevision returned error %v, want the nil,nil not-found contract", err)
	}
	if got != nil {
		t.Errorf("GetDocumentRevision = %+v, want nil", got)
	}
	latest, err := repo.GetLatestDocumentRevision(ctx, "task-missing", "plan")
	if err != nil {
		t.Fatalf("GetLatestDocumentRevision returned error %v, want nil,nil", err)
	}
	if latest != nil {
		t.Errorf("GetLatestDocumentRevision = %+v, want nil", latest)
	}
}

func TestListDocumentRevisionsOrdersDescendingScopedByKeyAndHonoursLimit(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-rev-list")

	for _, key := range []string{"plan", "spec"} {
		for number := 1; number <= 3; number++ {
			if err := repo.InsertDocumentRevision(ctx, &models.TaskDocumentRevision{
				ID:     "rev-" + key + "-" + string(rune('0'+number)),
				TaskID: "task-rev-list", DocumentKey: key, RevisionNumber: number,
				Title:   key,
				Content: key, AuthorKind: "agent",
			}); err != nil {
				t.Fatalf("InsertDocumentRevision(%s %d): %v", key, number, err)
			}
		}
	}

	all, err := repo.ListDocumentRevisions(ctx, "task-rev-list", "plan", 0)
	if err != nil {
		t.Fatalf("ListDocumentRevisions(limit=0): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("limit=0 returned %d rows, want 3 (only the plan key)", len(all))
	}
	for i, wantNumber := range []int{3, 2, 1} {
		if all[i].RevisionNumber != wantNumber {
			t.Fatalf("revision numbers = %d at index %d, want %d (newest first)", all[i].RevisionNumber, i, wantNumber)
		}
		if all[i].DocumentKey != "plan" {
			t.Fatalf("document key = %q at index %d, want plan", all[i].DocumentKey, i)
		}
	}

	limited, err := repo.ListDocumentRevisions(ctx, "task-rev-list", "plan", 2)
	if err != nil {
		t.Fatalf("ListDocumentRevisions(limit=2): %v", err)
	}
	if len(limited) != 2 || limited[0].RevisionNumber != 3 || limited[1].RevisionNumber != 2 {
		t.Errorf("limit=2 returned %+v, want revisions 3 then 2", limited)
	}

	latest, err := repo.GetLatestDocumentRevision(ctx, "task-rev-list", "spec")
	if err != nil {
		t.Fatalf("GetLatestDocumentRevision: %v", err)
	}
	if latest.RevisionNumber != 3 || latest.DocumentKey != "spec" {
		t.Errorf("latest spec revision = %+v, want revision 3 of spec", latest)
	}
}

func TestNextDocumentRevisionNumber(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-rev-next")

	next, err := repo.NextDocumentRevisionNumber(ctx, "task-rev-next", "plan")
	if err != nil {
		t.Fatalf("NextDocumentRevisionNumber: %v", err)
	}
	if next != 1 {
		t.Errorf("NextDocumentRevisionNumber with no history = %d, want 1", next)
	}

	if err := repo.InsertDocumentRevision(ctx, &models.TaskDocumentRevision{
		ID: "rev-next-5", TaskID: "task-rev-next", DocumentKey: "plan", RevisionNumber: 5,
		Title: "Plan", Content: "v5", AuthorKind: "agent",
	}); err != nil {
		t.Fatalf("InsertDocumentRevision: %v", err)
	}
	// A different key must not influence the plan key's numbering.
	if err := repo.InsertDocumentRevision(ctx, &models.TaskDocumentRevision{
		ID: "rev-next-spec-9", TaskID: "task-rev-next", DocumentKey: "spec", RevisionNumber: 9,
		Title: "Spec", Content: "v9", AuthorKind: "agent",
	}); err != nil {
		t.Fatalf("InsertDocumentRevision(spec): %v", err)
	}

	next, err = repo.NextDocumentRevisionNumber(ctx, "task-rev-next", "plan")
	if err != nil {
		t.Fatalf("NextDocumentRevisionNumber: %v", err)
	}
	if next != 6 {
		t.Errorf("NextDocumentRevisionNumber = %d, want 6 (MAX(5)+1 for the plan key only)", next)
	}
}

func TestWriteDocumentRevisionCreatesHeadAndFirstRevision(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-write-doc")

	head := &models.TaskDocument{
		TaskID: "task-write-doc", Key: "plan", Type: "plan", Title: "Plan v1",
		Content: "first", AuthorKind: "agent", AuthorName: "claude",
		Filename: "plan.md", MimeType: "text/markdown", SizeBytes: 5, DiskPath: "/plan.md",
	}
	rev := &models.TaskDocumentRevision{
		TaskID: "task-write-doc", DocumentKey: "plan",
		Title: "Plan v1", Content: "first", AuthorKind: "agent", AuthorName: "claude",
	}
	if err := repo.WriteDocumentRevision(ctx, head, rev, nil); err != nil {
		t.Fatalf("WriteDocumentRevision: %v", err)
	}
	if head.ID == "" {
		t.Fatal("head ID left empty; a UUID should have been generated")
	}
	if rev.RevisionNumber != 1 {
		t.Errorf("RevisionNumber = %d, want 1 for the first revision", rev.RevisionNumber)
	}

	gotHead, err := repo.GetDocument(ctx, "task-write-doc", "plan")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	assertDocumentEqual(t, gotHead, head)

	gotRev, err := repo.GetLatestDocumentRevision(ctx, "task-write-doc", "plan")
	if err != nil {
		t.Fatalf("GetLatestDocumentRevision: %v", err)
	}
	assertDocRevisionEqual(t, gotRev, rev)
}

func TestWriteDocumentRevisionUpdatesHeadInPlaceAndAppendsRevisions(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-write-doc-append")

	head := &models.TaskDocument{
		ID: "head-append", TaskID: "task-write-doc-append", Key: "plan", Type: "plan",
		Title: "v1", Content: "one", AuthorKind: "agent",
	}
	if err := repo.WriteDocumentRevision(ctx, head, &models.TaskDocumentRevision{
		TaskID: "task-write-doc-append", DocumentKey: "plan", Title: "v1", Content: "one", AuthorKind: "agent",
	}, nil); err != nil {
		t.Fatalf("WriteDocumentRevision(first): %v", err)
	}
	createdAt := head.CreatedAt

	head.Title = "v2"
	head.Content = "two"
	head.SizeBytes = 3
	second := &models.TaskDocumentRevision{
		TaskID: "task-write-doc-append", DocumentKey: "plan", Title: "v2", Content: "two", AuthorKind: "user",
	}
	if err := repo.WriteDocumentRevision(ctx, head, second, nil); err != nil {
		t.Fatalf("WriteDocumentRevision(second): %v", err)
	}
	if second.RevisionNumber != 2 {
		t.Errorf("second RevisionNumber = %d, want 2", second.RevisionNumber)
	}

	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_documents WHERE task_id = ?`, "task-write-doc-append"); got != 1 {
		t.Errorf("HEAD rows = %d, want 1 (the upsert must not insert a second row)", got)
	}
	gotHead, err := repo.GetDocument(ctx, "task-write-doc-append", "plan")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if gotHead.ID != "head-append" {
		t.Errorf("HEAD id = %q, want the original head-append", gotHead.ID)
	}
	if gotHead.Title != "v2" || gotHead.Content != "two" || gotHead.SizeBytes != 3 {
		t.Errorf("HEAD = %+v, want the v2 values", gotHead)
	}
	if !gotHead.CreatedAt.Equal(createdAt) {
		t.Errorf("HEAD CreatedAt = %v, want it preserved at %v", gotHead.CreatedAt, createdAt)
	}

	history, err := repo.ListDocumentRevisions(ctx, "task-write-doc-append", "plan", 0)
	if err != nil {
		t.Fatalf("ListDocumentRevisions: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history has %d revisions, want 2", len(history))
	}
	if history[0].RevisionNumber != 2 || history[0].Content != "two" || history[0].AuthorKind != "user" {
		t.Errorf("newest revision = %+v, want revision 2 authored by user", history[0])
	}
	if history[1].RevisionNumber != 1 || history[1].Content != "one" {
		t.Errorf("oldest revision = %+v, want revision 1 with the original content", history[1])
	}
}

func TestWriteDocumentRevisionCoalescesIntoExistingRevision(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-write-doc-merge")

	head := &models.TaskDocument{
		ID: "head-merge", TaskID: "task-write-doc-merge", Key: "plan", Type: "plan",
		Title: "v1", Content: "one", AuthorKind: "agent",
	}
	first := &models.TaskDocumentRevision{
		ID: "rev-merge", TaskID: "task-write-doc-merge", DocumentKey: "plan",
		Title: "v1", Content: "one", AuthorKind: "agent", AuthorName: "claude",
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	if err := repo.WriteDocumentRevision(ctx, head, first, nil); err != nil {
		t.Fatalf("WriteDocumentRevision(first): %v", err)
	}

	head.Title = "v1 edited"
	head.Content = "one edited"
	merged := &models.TaskDocumentRevision{
		TaskID: "task-write-doc-merge", DocumentKey: "plan",
		Title: "v1 edited", Content: "one edited", AuthorKind: "ignored", AuthorName: "ignored",
	}
	coalesceID := "rev-merge"
	if err := repo.WriteDocumentRevision(ctx, head, merged, &coalesceID); err != nil {
		t.Fatalf("WriteDocumentRevision(merge): %v", err)
	}
	if merged.ID != "rev-merge" {
		t.Errorf("merged revision ID = %q, want rev-merge", merged.ID)
	}

	history, err := repo.ListDocumentRevisions(ctx, "task-write-doc-merge", "plan", 0)
	if err != nil {
		t.Fatalf("ListDocumentRevisions: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history has %d revisions, want 1 (coalesce must merge, not append)", len(history))
	}
	got := history[0]
	if got.Title != "v1 edited" || got.Content != "one edited" {
		t.Errorf("merged revision = %+v, want the edited title/content", got)
	}
	if got.RevisionNumber != 1 {
		t.Errorf("RevisionNumber = %d, want 1 preserved across the merge", got.RevisionNumber)
	}
	if got.AuthorKind != "agent" || got.AuthorName != "claude" {
		t.Errorf("author = %q/%q, want the original agent/claude preserved", got.AuthorKind, got.AuthorName)
	}
	if !got.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("CreatedAt = %v, want the original %v preserved", got.CreatedAt, first.CreatedAt)
	}
	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Errorf("UpdatedAt = %v, want it bumped past CreatedAt %v", got.UpdatedAt, got.CreatedAt)
	}
}

func TestWriteDocumentRevisionRollsBackWhenCoalesceTargetMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-write-doc-rollback")

	head := &models.TaskDocument{
		ID: "head-rollback", TaskID: "task-write-doc-rollback", Key: "plan", Type: "plan",
		Title: "should not persist", Content: "nope", AuthorKind: "agent",
	}
	missingID := "rev-does-not-exist"
	err := repo.WriteDocumentRevision(ctx, head, &models.TaskDocumentRevision{
		TaskID: "task-write-doc-rollback", DocumentKey: "plan", Title: "x", Content: "y",
	}, &missingID)
	if err == nil {
		t.Fatal("WriteDocumentRevision with an unknown coalesce id returned nil")
	}
	if !strings.Contains(err.Error(), "document revision not found") {
		t.Errorf("error = %q, want a document-revision-not-found message", err)
	}

	// The HEAD upsert happened inside the same transaction and must roll back.
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_documents WHERE task_id = ?`, "task-write-doc-rollback"); got != 0 {
		t.Errorf("HEAD rows after a failed merge = %d, want 0 (transaction must roll back)", got)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_document_revisions WHERE task_id = ?`, "task-write-doc-rollback"); got != 0 {
		t.Errorf("revision rows after a failed merge = %d, want 0", got)
	}
}

func TestWriteDocumentRevisionTreatsEmptyCoalesceIDAsInsert(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-write-doc-empty")

	head := &models.TaskDocument{
		ID: "head-empty", TaskID: "task-write-doc-empty", Key: "plan", Type: "plan",
		Title: "v1", Content: "one", AuthorKind: "agent",
	}
	empty := ""
	rev := &models.TaskDocumentRevision{
		TaskID: "task-write-doc-empty", DocumentKey: "plan", Title: "v1", Content: "one", AuthorKind: "agent",
	}
	if err := repo.WriteDocumentRevision(ctx, head, rev, &empty); err != nil {
		t.Fatalf("WriteDocumentRevision: %v", err)
	}
	if rev.RevisionNumber != 1 {
		t.Errorf("RevisionNumber = %d, want 1 — an empty coalesce id means append", rev.RevisionNumber)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_document_revisions WHERE task_id = ?`, "task-write-doc-empty"); got != 1 {
		t.Errorf("revision rows = %d, want 1", got)
	}
}

func TestDocumentMethodsPropagateBackendErrors(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-doc-broken")
	if err := repo.CreateDocument(ctx, &models.TaskDocument{
		ID: "doc-broken", TaskID: "task-doc-broken", Key: "plan", Type: "plan", Title: "Plan",
	}); err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	closeRepoDB(t, repo)

	if err := repo.CreateDocument(ctx, &models.TaskDocument{
		ID: "doc-after-close", TaskID: "task-doc-broken", Key: "spec",
	}); err == nil {
		t.Error("CreateDocument on a closed database returned nil")
	}
	if _, err := repo.GetDocument(ctx, "task-doc-broken", "plan"); err == nil {
		t.Error("GetDocument on a closed database returned nil")
	}
	if err := repo.UpdateDocument(ctx, &models.TaskDocument{TaskID: "task-doc-broken", Key: "plan"}); err == nil {
		t.Error("UpdateDocument on a closed database returned nil")
	}
	if err := repo.DeleteDocument(ctx, "task-doc-broken", "plan"); err == nil {
		t.Error("DeleteDocument on a closed database returned nil")
	}
	if _, err := repo.ListDocuments(ctx, "task-doc-broken"); err == nil {
		t.Error("ListDocuments on a closed database returned nil")
	}
	if err := repo.InsertDocumentRevision(ctx, &models.TaskDocumentRevision{
		TaskID: "task-doc-broken", DocumentKey: "plan", RevisionNumber: 1,
	}); err == nil {
		t.Error("InsertDocumentRevision on a closed database returned nil")
	}
	if _, err := repo.GetDocumentRevision(ctx, "rev-broken"); err == nil {
		t.Error("GetDocumentRevision on a closed database returned nil")
	}
	if _, err := repo.ListDocumentRevisions(ctx, "task-doc-broken", "plan", 0); err == nil {
		t.Error("ListDocumentRevisions on a closed database returned nil")
	}
	if _, err := repo.NextDocumentRevisionNumber(ctx, "task-doc-broken", "plan"); err == nil {
		t.Error("NextDocumentRevisionNumber on a closed database returned nil")
	}
	if err := repo.WriteDocumentRevision(ctx,
		&models.TaskDocument{TaskID: "task-doc-broken", Key: "plan"},
		&models.TaskDocumentRevision{TaskID: "task-doc-broken", DocumentKey: "plan"},
		nil); err == nil {
		t.Error("WriteDocumentRevision on a closed database returned nil")
	}
}
