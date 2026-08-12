package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// newDocumentTestService builds a DocumentService over the real SQLite
// repository used by the rest of the package, plus one task row the document
// FK can point at.
func newDocumentTestService(t *testing.T) (*DocumentService, *sqliterepo.Repository) {
	t.Helper()
	_, _, repo := createTestService(t)
	seedDocumentTask(t, repo, "task-doc")
	return NewDocumentService(repo, accessTestLogger(t)), repo
}

func seedDocumentTask(t *testing.T, repo *sqliterepo.Repository, taskID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-" + taskID, Name: taskID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-" + taskID, WorkspaceID: "ws-" + taskID, Name: taskID}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws-" + taskID, WorkflowID: "wf-" + taskID, WorkflowStepID: "step-1",
		Title: taskID, State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
}

// stubDocRepo wraps the real repository so individual calls can fail on demand.
// Everything not overridden delegates, keeping the surrounding behavior real.
type stubDocRepo struct {
	docRepo
	getDocErr     error
	getDocErrOnNo int // 1 = fail the first GetDocument, 2 = fail the reload
	getDocCalls   int
	getDocNil     bool
	latestErr     error
	writeErr      error
	revisionErr   error
	deleteErr     error
	createErr     error
	updateErr     error
}

func (s *stubDocRepo) GetDocument(ctx context.Context, taskID, key string) (*models.TaskDocument, error) {
	s.getDocCalls++
	if s.getDocErr != nil && (s.getDocErrOnNo == 0 || s.getDocErrOnNo == s.getDocCalls) {
		return nil, s.getDocErr
	}
	if s.getDocNil && s.getDocCalls > 1 {
		return nil, nil
	}
	return s.docRepo.GetDocument(ctx, taskID, key)
}

func (s *stubDocRepo) GetLatestDocumentRevision(ctx context.Context, taskID, key string) (*models.TaskDocumentRevision, error) {
	if s.latestErr != nil {
		return nil, s.latestErr
	}
	return s.docRepo.GetLatestDocumentRevision(ctx, taskID, key)
}

func (s *stubDocRepo) WriteDocumentRevision(ctx context.Context, head *models.TaskDocument, rev *models.TaskDocumentRevision, coalesceID *string) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	return s.docRepo.WriteDocumentRevision(ctx, head, rev, coalesceID)
}

func (s *stubDocRepo) GetDocumentRevision(ctx context.Context, id string) (*models.TaskDocumentRevision, error) {
	if s.revisionErr != nil {
		return nil, s.revisionErr
	}
	return s.docRepo.GetDocumentRevision(ctx, id)
}

func (s *stubDocRepo) DeleteDocument(ctx context.Context, taskID, key string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.docRepo.DeleteDocument(ctx, taskID, key)
}

func (s *stubDocRepo) CreateDocument(ctx context.Context, doc *models.TaskDocument) error {
	if s.createErr != nil {
		return s.createErr
	}
	return s.docRepo.CreateDocument(ctx, doc)
}

func (s *stubDocRepo) UpdateDocument(ctx context.Context, doc *models.TaskDocument) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	return s.docRepo.UpdateDocument(ctx, doc)
}

func TestDocumentServiceRequiresTaskAndKey(t *testing.T) {
	svc, _ := newDocumentTestService(t)
	ctx := context.Background()

	cases := []struct {
		name string
		run  func() error
	}{
		{"create-no-task", func() error {
			_, err := svc.CreateOrUpdateDocument(ctx, "", "k", "", "", "", "", "")
			return err
		}},
		{"get-no-task", func() error { _, err := svc.GetDocument(ctx, "", "k"); return err }},
		{"delete-no-task", func() error { return svc.DeleteDocument(ctx, "", "k") }},
		{"list-no-task", func() error { _, err := svc.ListDocuments(ctx, ""); return err }},
		{"revisions-no-task", func() error { _, err := svc.ListRevisions(ctx, "", "k", 10); return err }},
		{"revert-no-task", func() error { _, err := svc.RevertDocument(ctx, "", "k", "rev"); return err }},
		{"upload-no-task", func() error {
			_, err := svc.UploadAttachment(ctx, "", "k", "f.png", "image/png", nil, t.TempDir())
			return err
		}},
	}
	for _, tc := range cases {
		if err := tc.run(); !errors.Is(err, ErrDocumentTaskRequired) {
			t.Fatalf("%s error = %v, want ErrDocumentTaskRequired", tc.name, err)
		}
	}

	keyCases := []struct {
		name string
		run  func() error
	}{
		{"create-no-key", func() error {
			_, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "", "", "", "", "", "")
			return err
		}},
		{"get-no-key", func() error { _, err := svc.GetDocument(ctx, "task-doc", ""); return err }},
		{"delete-no-key", func() error { return svc.DeleteDocument(ctx, "task-doc", "") }},
		{"revisions-no-key", func() error { _, err := svc.ListRevisions(ctx, "task-doc", "", 10); return err }},
		{"revert-no-key", func() error { _, err := svc.RevertDocument(ctx, "task-doc", "", "rev"); return err }},
		{"upload-no-key", func() error {
			_, err := svc.UploadAttachment(ctx, "task-doc", "", "f.png", "image/png", nil, t.TempDir())
			return err
		}},
	}
	for _, tc := range keyCases {
		if err := tc.run(); !errors.Is(err, ErrDocumentKeyRequired) {
			t.Fatalf("%s error = %v, want ErrDocumentKeyRequired", tc.name, err)
		}
	}
}

func TestDocumentServiceCreateAppliesDefaultsAndPersistsRevision(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	ctx := context.Background()

	doc, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "notes", "", "", "first body", "", "")
	if err != nil {
		t.Fatalf("CreateOrUpdateDocument: %v", err)
	}
	if doc.Type != "custom" {
		t.Fatalf("type = %q, want custom", doc.Type)
	}
	if doc.Title != "notes" {
		t.Fatalf("title = %q, want the key as fallback title", doc.Title)
	}
	if doc.AuthorKind != createdByAgent || doc.AuthorName != "Agent" {
		t.Fatalf("author = %q/%q, want agent/Agent", doc.AuthorKind, doc.AuthorName)
	}
	if doc.Content != "first body" {
		t.Fatalf("content = %q", doc.Content)
	}
	if doc.ID == "" || doc.CreatedAt.IsZero() {
		t.Fatalf("reloaded HEAD must carry DB-set id/timestamps: %+v", doc)
	}

	revs, err := repo.ListDocumentRevisions(ctx, "task-doc", "notes", 10)
	if err != nil {
		t.Fatalf("ListDocumentRevisions: %v", err)
	}
	if len(revs) != 1 {
		t.Fatalf("revisions = %d, want 1", len(revs))
	}
	if revs[0].RevisionNumber != 1 || revs[0].Content != "first body" {
		t.Fatalf("revision = %+v, want number 1 with the created content", revs[0])
	}
	if revs[0].AuthorKind != createdByAgent || revs[0].AuthorName != "Agent" {
		t.Fatalf("revision author = %q/%q, want the defaulted agent author", revs[0].AuthorKind, revs[0].AuthorName)
	}
}

func TestDocumentServiceUserAuthorDefaultsToUserName(t *testing.T) {
	svc, _ := newDocumentTestService(t)

	doc, err := svc.CreateOrUpdateDocument(context.Background(), "task-doc", "k", "custom", "T", "body", "user", "")
	if err != nil {
		t.Fatalf("CreateOrUpdateDocument: %v", err)
	}
	if doc.AuthorKind != "user" || doc.AuthorName != "User" {
		t.Fatalf("author = %q/%q, want user/User", doc.AuthorKind, doc.AuthorName)
	}
}

func TestDocumentServiceCoalescesSameAuthorWithinWindow(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	ctx := context.Background()

	if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "k", "custom", "T", "v1", "user", "Ada"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	updated, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "k", "custom", "T", "v2", "user", "Ada")
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if updated.Content != "v2" {
		t.Fatalf("HEAD content = %q, want v2", updated.Content)
	}

	revs, err := repo.ListDocumentRevisions(ctx, "task-doc", "k", 10)
	if err != nil {
		t.Fatalf("ListDocumentRevisions: %v", err)
	}
	if len(revs) != 1 {
		t.Fatalf("revisions = %d, want 1 (same author inside the window coalesces)", len(revs))
	}
	if revs[0].Content != "v2" || revs[0].RevisionNumber != 1 {
		t.Fatalf("coalesced revision = %+v, want number 1 carrying v2", revs[0])
	}
}

func TestDocumentServiceAppendsRevisionForDifferentAuthor(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	ctx := context.Background()

	if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "k", "custom", "T", "v1", "user", "Ada"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "k", "custom", "T", "v2", "user", "Grace"); err != nil {
		t.Fatalf("second write: %v", err)
	}

	revs, err := repo.ListDocumentRevisions(ctx, "task-doc", "k", 10)
	if err != nil {
		t.Fatalf("ListDocumentRevisions: %v", err)
	}
	if len(revs) != 2 {
		t.Fatalf("revisions = %d, want 2 (different author must not coalesce)", len(revs))
	}
	// Newest first.
	if revs[0].RevisionNumber != 2 || revs[0].AuthorName != "Grace" {
		t.Fatalf("newest revision = %+v, want number 2 by Grace", revs[0])
	}
	if revs[1].RevisionNumber != 1 || revs[1].AuthorName != "Ada" {
		t.Fatalf("oldest revision = %+v, want number 1 by Ada", revs[1])
	}
}

func TestDocumentServiceZeroCoalesceWindowAlwaysAppends(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	svc.coalesceWindow = 0
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "k", "custom", "T", "body", "user", "Ada"); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	revs, err := repo.ListDocumentRevisions(ctx, "task-doc", "k", 10)
	if err != nil {
		t.Fatalf("ListDocumentRevisions: %v", err)
	}
	if len(revs) != 2 {
		t.Fatalf("revisions = %d, want 2 with coalescing disabled", len(revs))
	}
}

func TestDocumentServiceCanDocCoalesceRules(t *testing.T) {
	svc, _ := newDocumentTestService(t)
	now := time.Now().UTC()
	revertID := "rev-0"

	cases := []struct {
		name   string
		latest *models.TaskDocumentRevision
		window time.Duration
		want   bool
	}{
		{"no latest revision", nil, time.Minute, false},
		{"latest is a revert", &models.TaskDocumentRevision{
			AuthorKind: "user", AuthorName: "Ada", UpdatedAt: now, RevertOfRevisionID: &revertID,
		}, time.Minute, false},
		{"different author kind", &models.TaskDocumentRevision{
			AuthorKind: "agent", AuthorName: "Ada", UpdatedAt: now,
		}, time.Minute, false},
		{"different author name", &models.TaskDocumentRevision{
			AuthorKind: "user", AuthorName: "Grace", UpdatedAt: now,
		}, time.Minute, false},
		{"outside window", &models.TaskDocumentRevision{
			AuthorKind: "user", AuthorName: "Ada", UpdatedAt: now.Add(-2 * time.Minute),
		}, time.Minute, false},
		{"inside window", &models.TaskDocumentRevision{
			AuthorKind: "user", AuthorName: "Ada", UpdatedAt: now.Add(-30 * time.Second),
		}, time.Minute, true},
	}
	for _, tc := range cases {
		svc.coalesceWindow = tc.window
		if got := svc.canDocCoalesce(tc.latest, "user", "Ada", now); got != tc.want {
			t.Fatalf("%s: canDocCoalesce = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDocumentServiceGetDeleteAndList(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	ctx := context.Background()

	if _, err := svc.GetDocument(ctx, "task-doc", "missing"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("get missing = %v, want ErrDocumentNotFound", err)
	}
	if err := svc.DeleteDocument(ctx, "task-doc", "missing"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("delete missing = %v, want ErrDocumentNotFound", err)
	}

	if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "b-key", "custom", "B", "b", "user", "Ada"); err != nil {
		t.Fatalf("create b: %v", err)
	}
	if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "a-key", "custom", "A", "a", "user", "Ada"); err != nil {
		t.Fatalf("create a: %v", err)
	}

	got, err := svc.GetDocument(ctx, "task-doc", "a-key")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if got.Key != "a-key" || got.Title != "A" || got.Content != "a" {
		t.Fatalf("document = %+v, want a-key/A/a", got)
	}

	list, err := svc.ListDocuments(ctx, "task-doc")
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(list) != 2 || list[0].Key != "a-key" || list[1].Key != "b-key" {
		t.Fatalf("list = %+v, want a-key then b-key", list)
	}

	if err := svc.DeleteDocument(ctx, "task-doc", "a-key"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	remaining, err := repo.ListDocuments(ctx, "task-doc")
	if err != nil {
		t.Fatalf("ListDocuments after delete: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Key != "b-key" {
		t.Fatalf("remaining = %+v, want only b-key", remaining)
	}
}

func TestDocumentServiceListRevisionsHonorsLimit(t *testing.T) {
	svc, _ := newDocumentTestService(t)
	svc.coalesceWindow = 0
	ctx := context.Background()

	for _, body := range []string{"v1", "v2", "v3"} {
		if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "k", "custom", "T", body, "user", "Ada"); err != nil {
			t.Fatalf("write %s: %v", body, err)
		}
	}
	revs, err := svc.ListRevisions(ctx, "task-doc", "k", 2)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revs) != 2 {
		t.Fatalf("revisions = %d, want the 2 requested", len(revs))
	}
	if revs[0].Content != "v3" || revs[1].Content != "v2" {
		t.Fatalf("revisions = %q/%q, want newest-first v3/v2", revs[0].Content, revs[1].Content)
	}
}

func TestDocumentServiceRevertRestoresTargetRevision(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	svc.coalesceWindow = 0
	ctx := context.Background()

	if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "k", "plan", "First", "v1", "user", "Ada"); err != nil {
		t.Fatalf("write v1: %v", err)
	}
	if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "k", "plan", "Second", "v2", "user", "Ada"); err != nil {
		t.Fatalf("write v2: %v", err)
	}
	revs, err := repo.ListDocumentRevisions(ctx, "task-doc", "k", 10)
	if err != nil {
		t.Fatalf("ListDocumentRevisions: %v", err)
	}
	target := revs[len(revs)-1] // oldest = v1

	rev, err := svc.RevertDocument(ctx, "task-doc", "k", target.ID)
	if err != nil {
		t.Fatalf("RevertDocument: %v", err)
	}
	if rev.Content != "v1" || rev.Title != "First" {
		t.Fatalf("revert revision = %+v, want v1/First", rev)
	}
	if rev.RevertOfRevisionID == nil || *rev.RevertOfRevisionID != target.ID {
		t.Fatalf("revert_of_revision_id = %v, want %s", rev.RevertOfRevisionID, target.ID)
	}
	if rev.AuthorKind != "user" || rev.AuthorName != "User" {
		t.Fatalf("revert author = %q/%q, want user/User", rev.AuthorKind, rev.AuthorName)
	}

	head, err := svc.GetDocument(ctx, "task-doc", "k")
	if err != nil {
		t.Fatalf("GetDocument after revert: %v", err)
	}
	if head.Content != "v1" || head.Title != "First" {
		t.Fatalf("HEAD after revert = %q/%q, want v1/First", head.Content, head.Title)
	}
	if head.Type != "plan" {
		t.Fatalf("HEAD type = %q, want the existing type preserved", head.Type)
	}
}

func TestDocumentServiceRevertRejectsBadTargets(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	ctx := context.Background()

	if _, err := svc.RevertDocument(ctx, "task-doc", "k", ""); err == nil || !strings.Contains(err.Error(), "revision_id is required") {
		t.Fatalf("empty revision id error = %v", err)
	}
	if _, err := svc.RevertDocument(ctx, "task-doc", "k", "no-such-revision"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("unknown revision = %v, want ErrDocumentNotFound", err)
	}

	if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "other", "custom", "T", "body", "user", "Ada"); err != nil {
		t.Fatalf("seed other document: %v", err)
	}
	otherRevs, err := repo.ListDocumentRevisions(ctx, "task-doc", "other", 10)
	if err != nil {
		t.Fatalf("ListDocumentRevisions: %v", err)
	}
	_, err = svc.RevertDocument(ctx, "task-doc", "k", otherRevs[0].ID)
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("cross-document revert error = %v, want a belonging error", err)
	}
}

func TestDocumentServiceUploadAndDownloadAttachment(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	ctx := context.Background()
	base := t.TempDir()

	doc, err := svc.UploadAttachment(ctx, "task-doc", "shot", "screen.png", "image/png", []byte("bytes"), base)
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if doc.Type != docTypeAttachment || doc.Filename != "screen.png" || doc.MimeType != "image/png" {
		t.Fatalf("attachment doc = %+v", doc)
	}
	if doc.SizeBytes != int64(len("bytes")) {
		t.Fatalf("size = %d, want %d", doc.SizeBytes, len("bytes"))
	}
	wantPath := filepath.Join(base, "attachments", "task-doc", "shot.png")
	if doc.DiskPath != wantPath {
		t.Fatalf("disk path = %q, want %q", doc.DiskPath, wantPath)
	}
	written, err := os.ReadFile(doc.DiskPath)
	if err != nil {
		t.Fatalf("read written attachment: %v", err)
	}
	if string(written) != "bytes" {
		t.Fatalf("written bytes = %q", written)
	}

	path, downloaded, err := svc.DownloadAttachment(ctx, "task-doc", "shot")
	if err != nil {
		t.Fatalf("DownloadAttachment: %v", err)
	}
	if path != wantPath || downloaded.Key != "shot" {
		t.Fatalf("download = %q/%+v", path, downloaded)
	}

	// Re-upload replaces in place: same row, preserved id and created_at.
	second, err := svc.UploadAttachment(ctx, "task-doc", "shot", "screen2.png", "image/png", []byte("more"), base)
	if err != nil {
		t.Fatalf("second UploadAttachment: %v", err)
	}
	if second.ID != doc.ID {
		t.Fatalf("re-upload id = %q, want the existing %q", second.ID, doc.ID)
	}
	if !second.CreatedAt.Equal(doc.CreatedAt) {
		t.Fatalf("re-upload created_at = %v, want preserved %v", second.CreatedAt, doc.CreatedAt)
	}
	docs, err := repo.ListDocuments(ctx, "task-doc")
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(docs) != 1 || docs[0].Filename != "screen2.png" {
		t.Fatalf("documents after re-upload = %+v, want one updated row", docs)
	}
}

func TestDocumentServiceUploadRejectsUnsafeComponents(t *testing.T) {
	svc, _ := newDocumentTestService(t)
	ctx := context.Background()
	base := t.TempDir()

	cases := []struct{ name, taskID, key, filename string }{
		{"dot-dot task", "..", "k", "f.png"},
		{"dot task", ".", "k", "f.png"},
		{"slash task", "a/b", "k", "f.png"},
		{"backslash task", `a\b`, "k", "f.png"},
		{"nul task", "a\x00b", "k", "f.png"},
		{"dot-dot key", "task-doc", "..", "f.png"},
		{"slash key", "task-doc", "a/b", "f.png"},
	}
	for _, tc := range cases {
		if _, err := svc.UploadAttachment(ctx, tc.taskID, tc.key, tc.filename, "image/png", []byte("x"), base); !errors.Is(err, ErrInvalidPathComponent) {
			t.Fatalf("%s: error = %v, want ErrInvalidPathComponent", tc.name, err)
		}
	}

	if _, err := svc.UploadAttachment(ctx, "task-doc", "k", "f."+string(rune(0))+"png", "image/png", []byte("x"), base); !errors.Is(err, ErrInvalidPathComponent) {
		t.Fatalf("NUL extension must be rejected, got %v", err)
	}

	if entries, err := os.ReadDir(base); err != nil {
		t.Fatalf("read base dir: %v", err)
	} else if len(entries) != 0 {
		t.Fatalf("rejected uploads must not create files, found %d entries", len(entries))
	}
}

func TestDocumentServiceUploadRejectsOversizedPayload(t *testing.T) {
	svc, _ := newDocumentTestService(t)

	_, err := svc.UploadAttachment(context.Background(), "task-doc", "k", "big.bin", "application/octet-stream",
		make([]byte, maxAttachmentBytes+1), t.TempDir())
	if !errors.Is(err, ErrAttachmentTooLarge) {
		t.Fatalf("oversized upload = %v, want ErrAttachmentTooLarge", err)
	}
}

func TestDocumentServiceDownloadRejectsNonAttachments(t *testing.T) {
	svc, repo := newDocumentTestService(t)
	ctx := context.Background()

	if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "notes", "custom", "T", "body", "user", "Ada"); err != nil {
		t.Fatalf("seed document: %v", err)
	}
	if _, _, err := svc.DownloadAttachment(ctx, "task-doc", "notes"); err == nil || !strings.Contains(err.Error(), "not an attachment") {
		t.Fatalf("non-attachment download = %v", err)
	}
	if _, _, err := svc.DownloadAttachment(ctx, "task-doc", "missing"); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("missing download = %v, want ErrDocumentNotFound", err)
	}

	// An attachment row whose disk path was lost still fails loudly.
	if err := repo.CreateDocument(ctx, &models.TaskDocument{
		ID: "doc-pathless", TaskID: "task-doc", Key: "pathless", Type: docTypeAttachment,
		Title: "x", AuthorKind: "user", AuthorName: "User",
	}); err != nil {
		t.Fatalf("seed pathless attachment: %v", err)
	}
	if _, _, err := svc.DownloadAttachment(ctx, "task-doc", "pathless"); err == nil || !strings.Contains(err.Error(), "no disk path") {
		t.Fatalf("pathless download = %v", err)
	}
}

func TestDocumentServiceUpdatePreservesAttachmentFields(t *testing.T) {
	svc, _ := newDocumentTestService(t)
	ctx := context.Background()

	if _, err := svc.UploadAttachment(ctx, "task-doc", "shot", "screen.png", "image/png", []byte("bytes"), t.TempDir()); err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	updated, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "shot", docTypeAttachment, "Caption", "note", "user", "Ada")
	if err != nil {
		t.Fatalf("CreateOrUpdateDocument: %v", err)
	}
	if updated.Filename != "screen.png" || updated.MimeType != "image/png" || updated.DiskPath == "" {
		t.Fatalf("attachment fields lost on update: %+v", updated)
	}
	if updated.SizeBytes != int64(len("bytes")) {
		t.Fatalf("size = %d, want the preserved %d", updated.SizeBytes, len("bytes"))
	}

	// Changing the type away from attachment drops them, by design.
	retyped, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "shot", "custom", "Caption", "note", "user", "Ada")
	if err != nil {
		t.Fatalf("retype: %v", err)
	}
	if retyped.Filename != "" || retyped.DiskPath != "" {
		t.Fatalf("non-attachment type must not keep attachment fields: %+v", retyped)
	}
}

func TestDocumentServicePropagatesRepositoryErrors(t *testing.T) {
	_, _, repo := createTestService(t)
	seedDocumentTask(t, repo, "task-doc")
	ctx := context.Background()
	boom := errors.New("db unavailable")

	t.Run("get document fails", func(t *testing.T) {
		svc := NewDocumentService(&stubDocRepo{docRepo: repo, getDocErr: boom}, accessTestLogger(t))
		_, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "k", "", "", "", "", "")
		if !errors.Is(err, boom) || !strings.Contains(err.Error(), "get document") {
			t.Fatalf("error = %v, want a wrapped get document failure", err)
		}
		if _, err := svc.GetDocument(ctx, "task-doc", "k"); !errors.Is(err, boom) {
			t.Fatalf("GetDocument error = %v, want %v", err, boom)
		}
		if err := svc.DeleteDocument(ctx, "task-doc", "k"); !errors.Is(err, boom) {
			t.Fatalf("DeleteDocument error = %v, want %v", err, boom)
		}
		if _, err := svc.UploadAttachment(ctx, "task-doc", "k", "f.png", "image/png", []byte("x"), t.TempDir()); !errors.Is(err, boom) {
			t.Fatalf("UploadAttachment error = %v, want %v", err, boom)
		}
	})

	t.Run("latest revision fails", func(t *testing.T) {
		svc := NewDocumentService(&stubDocRepo{docRepo: repo, latestErr: boom}, accessTestLogger(t))
		_, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "k", "", "", "", "", "")
		if !errors.Is(err, boom) || !strings.Contains(err.Error(), "get latest revision") {
			t.Fatalf("error = %v, want a wrapped latest-revision failure", err)
		}
	})

	t.Run("write revision fails", func(t *testing.T) {
		svc := NewDocumentService(&stubDocRepo{docRepo: repo, writeErr: boom}, accessTestLogger(t))
		if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "k", "", "", "", "", ""); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}

		// Revert reaches WriteDocumentRevision only once the target revision
		// resolves, so seed a real one first — otherwise the revision lookup
		// short-circuits and the injected write error is never exercised.
		seed := NewDocumentService(repo, accessTestLogger(t))
		if _, err := seed.CreateOrUpdateDocument(ctx, "task-doc", "revert-write-k", "", "", "body", "", ""); err != nil {
			t.Fatalf("seed: %v", err)
		}
		revs, err := repo.ListDocumentRevisions(ctx, "task-doc", "revert-write-k", 1)
		if err != nil || len(revs) != 1 {
			t.Fatalf("seed revisions = %v, %v", revs, err)
		}
		if _, err := svc.RevertDocument(ctx, "task-doc", "revert-write-k", revs[0].ID); !errors.Is(err, boom) {
			t.Fatalf("revert write error = %v, want %v", err, boom)
		}
	})

	t.Run("reload fails", func(t *testing.T) {
		svc := NewDocumentService(&stubDocRepo{docRepo: repo, getDocErr: boom, getDocErrOnNo: 2}, accessTestLogger(t))
		_, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "reload-k", "", "", "body", "", "")
		if !errors.Is(err, boom) || !strings.Contains(err.Error(), "reload document") {
			t.Fatalf("error = %v, want a wrapped reload failure", err)
		}
	})

	t.Run("reload returns nothing", func(t *testing.T) {
		svc := NewDocumentService(&stubDocRepo{docRepo: repo, getDocNil: true}, accessTestLogger(t))
		doc, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "nil-k", "", "", "body", "", "")
		if err != nil {
			t.Fatalf("CreateOrUpdateDocument: %v", err)
		}
		if doc == nil || doc.Content != "body" || doc.Key != "nil-k" {
			t.Fatalf("doc = %+v, want the in-memory HEAD as fallback", doc)
		}
	})

	t.Run("revert head lookup fails", func(t *testing.T) {
		seed := NewDocumentService(repo, accessTestLogger(t))
		if _, err := seed.CreateOrUpdateDocument(ctx, "task-doc", "revert-k", "", "", "body", "", ""); err != nil {
			t.Fatalf("seed: %v", err)
		}
		revs, err := repo.ListDocumentRevisions(ctx, "task-doc", "revert-k", 1)
		if err != nil || len(revs) != 1 {
			t.Fatalf("seed revisions = %v, %v", revs, err)
		}
		svc := NewDocumentService(&stubDocRepo{docRepo: repo, getDocErr: boom}, accessTestLogger(t))
		if _, err := svc.RevertDocument(ctx, "task-doc", "revert-k", revs[0].ID); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})

	t.Run("revision lookup fails", func(t *testing.T) {
		svc := NewDocumentService(&stubDocRepo{docRepo: repo, revisionErr: boom}, accessTestLogger(t))
		if _, err := svc.RevertDocument(ctx, "task-doc", "k", "rev"); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})

	t.Run("delete fails", func(t *testing.T) {
		svc := NewDocumentService(&stubDocRepo{docRepo: repo, deleteErr: boom}, accessTestLogger(t))
		if _, err := svc.CreateOrUpdateDocument(ctx, "task-doc", "del-k", "", "", "body", "", ""); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := svc.DeleteDocument(ctx, "task-doc", "del-k"); !errors.Is(err, boom) {
			t.Fatalf("error = %v, want %v", err, boom)
		}
	})

	t.Run("attachment persistence fails", func(t *testing.T) {
		createSvc := NewDocumentService(&stubDocRepo{docRepo: repo, createErr: boom}, accessTestLogger(t))
		if _, err := createSvc.UploadAttachment(ctx, "task-doc", "new-att", "f.png", "image/png", []byte("x"), t.TempDir()); !errors.Is(err, boom) ||
			!strings.Contains(err.Error(), "create attachment document") {
			t.Fatalf("create error = %v, want a wrapped create failure", err)
		}

		updateSvc := NewDocumentService(&stubDocRepo{docRepo: repo, updateErr: boom}, accessTestLogger(t))
		base := t.TempDir()
		if _, err := updateSvc.UploadAttachment(ctx, "task-doc", "att", "f.png", "image/png", []byte("x"), base); err != nil {
			t.Fatalf("first upload: %v", err)
		}
		if _, err := updateSvc.UploadAttachment(ctx, "task-doc", "att", "f.png", "image/png", []byte("y"), base); !errors.Is(err, boom) ||
			!strings.Contains(err.Error(), "update attachment document") {
			t.Fatalf("update error = %v, want a wrapped update failure", err)
		}
	})
}
