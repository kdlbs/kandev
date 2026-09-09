package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// newAttachmentTestService wires an AttachmentService over the real repository
// with a temp storage root and a recording workspace authorizer.
func newAttachmentTestService(t *testing.T) (*AttachmentService, *sqliterepo.Repository, string, *recordingWorkspaceAuthorizer) {
	t.Helper()
	_, _, repo := createTestService(t)
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-att", Name: "attachments"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	root := t.TempDir()
	auth := &recordingWorkspaceAuthorizer{}
	svc, err := NewAttachmentService(repo, root, auth.authorize, accessTestLogger(t))
	if err != nil {
		t.Fatalf("NewAttachmentService: %v", err)
	}
	return svc, repo, root, auth
}

type recordingWorkspaceAuthorizer struct {
	workspaceIDs []string
	err          error
}

func (a *recordingWorkspaceAuthorizer) authorize(_ context.Context, workspaceID string) error {
	a.workspaceIDs = append(a.workspaceIDs, workspaceID)
	return a.err
}

func stageTestAttachment(t *testing.T, svc *AttachmentService, ownerID, name, body string) *models.TaskMessageAttachment {
	t.Helper()
	attachment, err := svc.Stage(context.Background(), ownerID, "ws-att", name, "image/png", "image", "", strings.NewReader(body))
	if err != nil {
		t.Fatalf("Stage %s: %v", name, err)
	}
	return attachment
}

// nilAttachmentRepo returns a missing attachment as (nil, nil), the shape a
// repository is allowed to use for "no row"; the service must still deny.
type nilAttachmentRepo struct {
	repository.AttachmentRepository
}

func (nilAttachmentRepo) GetMessageAttachment(context.Context, string) (*models.TaskMessageAttachment, error) {
	return nil, nil
}

func TestNewAttachmentServiceValidatesDependencies(t *testing.T) {
	_, _, repo := createTestService(t)
	log := accessTestLogger(t)

	if _, err := NewAttachmentService(nil, t.TempDir(), nil, log); err == nil ||
		!strings.Contains(err.Error(), "repository is required") {
		t.Fatalf("nil repo error = %v", err)
	}
	if _, err := NewAttachmentService(repo, "   ", nil, log); err == nil ||
		!strings.Contains(err.Error(), "storage root is required") {
		t.Fatalf("blank root error = %v", err)
	}
	if _, err := NewAttachmentService(repo, t.TempDir(), nil, nil); err == nil ||
		!strings.Contains(err.Error(), "logger is required") {
		t.Fatalf("nil logger error = %v", err)
	}

	root := t.TempDir()
	svc, err := NewAttachmentService(repo, root, nil, log)
	if err != nil {
		t.Fatalf("NewAttachmentService: %v", err)
	}
	if svc.root != filepath.Join(root, "attachments") {
		t.Fatalf("root = %q, want the attachments subdirectory of %q", svc.root, root)
	}
}

func TestAttachmentStagePersistsRowAndBytes(t *testing.T) {
	svc, repo, root, auth := newAttachmentTestService(t)
	ctx := context.Background()

	before := time.Now().UTC()
	attachment, err := svc.Stage(ctx, "user-a", "ws-att", "diagram.png", "image/png", "image", "", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if len(auth.workspaceIDs) != 1 || auth.workspaceIDs[0] != "ws-att" {
		t.Fatalf("workspace authorizer calls = %v, want one ws-att call", auth.workspaceIDs)
	}
	if attachment.OwnerID != "user-a" || attachment.WorkspaceID != "ws-att" {
		t.Fatalf("ownership = %q/%q", attachment.OwnerID, attachment.WorkspaceID)
	}
	if attachment.Name != "diagram.png" || attachment.MimeType != "image/png" || attachment.Kind != "image" {
		t.Fatalf("metadata = %+v", attachment)
	}
	if attachment.DeliveryMode != attachmentDeliveryModePrompt {
		t.Fatalf("delivery mode = %q, want the prompt default", attachment.DeliveryMode)
	}
	if attachment.State != models.AttachmentStateStaged {
		t.Fatalf("state = %q, want staged", attachment.State)
	}
	if attachment.SizeBytes != int64(len("payload")) {
		t.Fatalf("size = %d, want %d", attachment.SizeBytes, len("payload"))
	}
	if attachment.StorageKey != attachment.ID {
		t.Fatalf("storage key = %q, want the attachment id", attachment.StorageKey)
	}
	if attachment.ExpiresAt.Before(before.Add(AttachmentStagedTTL - time.Minute)) {
		t.Fatalf("expires_at = %v, want roughly now+%v", attachment.ExpiresAt, AttachmentStagedTTL)
	}

	stored, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatalf("GetMessageAttachment: %v", err)
	}
	if stored.Name != "diagram.png" || stored.SizeBytes != int64(len("payload")) {
		t.Fatalf("persisted row = %+v", stored)
	}

	bytes, err := os.ReadFile(filepath.Join(root, "attachments", attachment.StorageKey))
	if err != nil {
		t.Fatalf("read stored bytes: %v", err)
	}
	if string(bytes) != "payload" {
		t.Fatalf("stored bytes = %q, want payload", bytes)
	}
}

func TestAttachmentStageStripsDirectoriesFromName(t *testing.T) {
	svc, _, _, _ := newAttachmentTestService(t)

	// The metadata validator rejects separators in the *supplied* name, so the
	// only names that reach filepath.Base are already clean. Assert the stored
	// name is the base form and that a traversal name never gets that far.
	attachment := stageTestAttachment(t, svc, "user-a", "clean.png", "x")
	if attachment.Name != "clean.png" {
		t.Fatalf("name = %q", attachment.Name)
	}
	_, err := svc.Stage(context.Background(), "user-a", "ws-att", "../escape.png", "image/png", "image", "", strings.NewReader("x"))
	if !errors.Is(err, ErrAttachmentInvalid) {
		t.Fatalf("traversal name = %v, want ErrAttachmentInvalid", err)
	}
}

func TestAttachmentStageRejectsInvalidInput(t *testing.T) {
	svc, _, root, auth := newAttachmentTestService(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		owner string
		ws    string
		src   io.Reader
	}{
		{"no workspace", "user-a", "", strings.NewReader("x")},
		{"no owner", "", "ws-att", strings.NewReader("x")},
		{"nil reader", "user-a", "ws-att", nil},
	}
	for _, tc := range cases {
		if _, err := svc.Stage(ctx, tc.owner, tc.ws, "f.png", "image/png", "image", "", tc.src); !errors.Is(err, ErrAttachmentInvalid) {
			t.Fatalf("%s: error = %v, want ErrAttachmentInvalid", tc.name, err)
		}
	}
	if len(auth.workspaceIDs) != 0 {
		t.Fatalf("invalid input must not reach the workspace authorizer, got %v", auth.workspaceIDs)
	}

	metadata := []struct {
		name, mime, kind, delivery string
	}{
		{"", "image/png", "image", "prompt"},
		{"f.png", "", "image", "prompt"},
		{"f.png", "image/png", "video", "prompt"},
		{"f.png", "image/png", "image", "smoke-signal"},
	}
	for _, tc := range metadata {
		_, err := svc.Stage(ctx, "user-a", "ws-att", tc.name, tc.mime, tc.kind, tc.delivery, strings.NewReader("x"))
		if !errors.Is(err, ErrAttachmentInvalid) {
			t.Fatalf("metadata %+v: error = %v, want ErrAttachmentInvalid", tc, err)
		}
	}

	if entries, err := os.ReadDir(filepath.Join(root, "attachments")); err == nil && len(entries) != 0 {
		t.Fatalf("rejected stages must leave no bytes behind, found %d", len(entries))
	}
}

func TestAttachmentStagePropagatesWorkspaceDenial(t *testing.T) {
	svc, _, root, auth := newAttachmentTestService(t)
	denied := errors.New("workspace not found")
	auth.err = denied

	if _, err := svc.Stage(context.Background(), "user-a", "ws-att", "f.png", "image/png", "image", "", strings.NewReader("x")); !errors.Is(err, denied) {
		t.Fatalf("error = %v, want the authorizer's denial", err)
	}
	if entries, err := os.ReadDir(filepath.Join(root, "attachments")); err == nil && len(entries) != 0 {
		t.Fatalf("denied stage wrote %d entries", len(entries))
	}
}

func TestAttachmentStageAcceptsPathDeliveryMode(t *testing.T) {
	svc, _, _, _ := newAttachmentTestService(t)

	attachment, err := svc.Stage(context.Background(), "user-a", "ws-att", "notes.txt", "text/plain", "resource",
		attachmentDeliveryModePath, strings.NewReader("x"))
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if attachment.DeliveryMode != attachmentDeliveryModePath {
		t.Fatalf("delivery mode = %q, want path", attachment.DeliveryMode)
	}
}

func TestAttachmentStageRejectsOversizedStreamWithoutLeavingBytes(t *testing.T) {
	svc, repo, root, _ := newAttachmentTestService(t)

	oversized := io.LimitReader(zeroReader{}, MaxAttachmentBytes+1)
	_, err := svc.Stage(context.Background(), "user-a", "ws-att", "big.bin", "application/octet-stream", "resource", "", oversized)
	if !errors.Is(err, ErrAttachmentTooLarge) {
		t.Fatalf("error = %v, want ErrAttachmentTooLarge", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "attachments"))
	if err != nil {
		t.Fatalf("read storage root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("oversized upload left %d files behind", len(entries))
	}
	if rows, err := repo.ListMessageAttachments(context.Background(), []string{"any"}); err != nil || len(rows) != 0 {
		t.Fatalf("oversized upload must not create rows: %v %v", rows, err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestAttachmentStageRemovesBytesWhenRegistryWriteFails(t *testing.T) {
	_, _, repo := createTestService(t)
	root := t.TempDir()
	// No workspace row exists, so the FK rejects the registry insert.
	svc, err := NewAttachmentService(repo, root, nil, accessTestLogger(t))
	if err != nil {
		t.Fatalf("NewAttachmentService: %v", err)
	}
	if _, err := svc.Stage(context.Background(), "user-a", "ws-missing", "f.png", "image/png", "image", "", strings.NewReader("x")); err == nil {
		t.Fatal("stage into a nonexistent workspace must fail")
	}
	entries, err := os.ReadDir(filepath.Join(root, "attachments"))
	if err != nil {
		t.Fatalf("read storage root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed registry write left %d orphan files", len(entries))
	}
}

func TestAttachmentGetScopesToOwner(t *testing.T) {
	svc, _, _, _ := newAttachmentTestService(t)
	ctx := context.Background()
	attachment := stageTestAttachment(t, svc, "user-a", "f.png", "x")

	got, err := svc.Get(ctx, "user-a", attachment.ID)
	if err != nil {
		t.Fatalf("owner Get: %v", err)
	}
	if got.ID != attachment.ID || got.Name != "f.png" {
		t.Fatalf("got = %+v", got)
	}
	if _, err := svc.Get(ctx, "user-b", attachment.ID); !errors.Is(err, ErrAttachmentForbidden) {
		t.Fatalf("foreign Get = %v, want ErrAttachmentForbidden", err)
	}
	if _, err := svc.Get(ctx, "user-a", "no-such-id"); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("unknown id = %v, want ErrAttachmentNotFound", err)
	}
}

func TestAttachmentGetDeniesWhenRepositoryReturnsNoRow(t *testing.T) {
	svc, err := NewAttachmentService(nilAttachmentRepo{}, t.TempDir(), nil, accessTestLogger(t))
	if err != nil {
		t.Fatalf("NewAttachmentService: %v", err)
	}
	if _, err := svc.Get(context.Background(), "user-a", "id"); !errors.Is(err, ErrAttachmentForbidden) {
		t.Fatalf("error = %v, want ErrAttachmentForbidden", err)
	}
}

func TestAttachmentOpenReturnsBytesAndScopes(t *testing.T) {
	svc, repo, root, _ := newAttachmentTestService(t)
	ctx := context.Background()
	attachment := stageTestAttachment(t, svc, "user-a", "f.png", "payload")

	got, file, err := svc.Open(ctx, "user-a", attachment.ID)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if got.ID != attachment.ID {
		t.Fatalf("opened %q, want %q", got.ID, attachment.ID)
	}
	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read attachment: %v", err)
	}
	if string(content) != "payload" {
		t.Fatalf("content = %q", content)
	}

	if _, _, err := svc.Open(ctx, "user-b", attachment.ID); !errors.Is(err, ErrAttachmentForbidden) {
		t.Fatalf("foreign Open = %v, want ErrAttachmentForbidden", err)
	}

	// Bytes deleted underneath the row surface as not-found, not a raw os error.
	if err := os.Remove(filepath.Join(root, "attachments", attachment.StorageKey)); err != nil {
		t.Fatalf("remove bytes: %v", err)
	}
	if _, _, err := svc.Open(ctx, "user-a", attachment.ID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("missing bytes = %v, want ErrAttachmentNotFound", err)
	}

	// A storage key that is not a bare filename is rejected before any open.
	traversal := &models.TaskMessageAttachment{
		ID: "att-traversal", OwnerID: "user-a", WorkspaceID: "ws-att", Name: "f.png",
		MimeType: "image/png", Kind: "image", DeliveryMode: attachmentDeliveryModePrompt,
		SizeBytes: 1, StorageKey: "../escape", State: models.AttachmentStateStaged,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := repo.CreateMessageAttachment(ctx, traversal); err != nil {
		t.Fatalf("seed traversal row: %v", err)
	}
	if _, _, err := svc.Open(ctx, "user-a", traversal.ID); !errors.Is(err, ErrAttachmentInvalid) {
		t.Fatalf("traversal storage key = %v, want ErrAttachmentInvalid", err)
	}
}

func TestAttachmentOpenClaimedAuthorizesByBinding(t *testing.T) {
	svc, _, root, _ := newAttachmentTestService(t)
	ctx := context.Background()
	attachment := stageTestAttachment(t, svc, "user-a", "f.png", "payload")

	// Staged rows are not deliverable.
	if _, _, _, _, err := svc.OpenClaimed(ctx, attachment.ID, "task-1", "sess-1"); !errors.Is(err, ErrAttachmentForbidden) {
		t.Fatalf("staged OpenClaimed = %v, want ErrAttachmentForbidden", err)
	}
	if err := svc.Claim(ctx, "user-a", "ws-att", "task-1", "sess-1", []string{attachment.ID}); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	reader, name, mime, size, err := svc.OpenClaimed(ctx, attachment.ID, "task-1", "sess-1")
	if err != nil {
		t.Fatalf("OpenClaimed: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	if name != "f.png" || mime != "image/png" || size != int64(len("payload")) {
		t.Fatalf("metadata = %q/%q/%d", name, mime, size)
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read claimed attachment: %v", err)
	}
	if string(content) != "payload" {
		t.Fatalf("content = %q", content)
	}

	if _, _, _, _, err := svc.OpenClaimed(ctx, attachment.ID, "task-2", "sess-1"); !errors.Is(err, ErrAttachmentForbidden) {
		t.Fatalf("foreign task = %v, want ErrAttachmentForbidden", err)
	}
	if _, _, _, _, err := svc.OpenClaimed(ctx, attachment.ID, "task-1", "sess-2"); !errors.Is(err, ErrAttachmentForbidden) {
		t.Fatalf("foreign session = %v, want ErrAttachmentForbidden", err)
	}
	if _, _, _, _, err := svc.OpenClaimed(ctx, "no-such-id", "task-1", "sess-1"); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("unknown id = %v, want ErrAttachmentNotFound", err)
	}

	if err := os.Remove(filepath.Join(root, "attachments", attachment.StorageKey)); err != nil {
		t.Fatalf("remove bytes: %v", err)
	}
	if _, _, _, _, err := svc.OpenClaimed(ctx, attachment.ID, "task-1", "sess-1"); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("missing bytes = %v, want ErrAttachmentNotFound", err)
	}
}

func TestAttachmentOpenClaimedAllowsAuthorizedTaskReader(t *testing.T) {
	svc, _, _, _ := newAttachmentTestService(t)
	ctx := context.Background()
	attachment := stageTestAttachment(t, svc, "user-a", "shared.png", "payload")
	if err := svc.Claim(ctx, "user-a", "ws-att", "task-1", "sess-1", []string{attachment.ID}); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	svc.SetTaskAuthorizer(func(ctx context.Context, taskID string) error {
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok || identity.UserID != "user-b" || taskID != "task-1" {
			return ErrAttachmentForbidden
		}
		return nil
	})
	_, reader, err := svc.Open(ctxAs("user-b"), "user-b", attachment.ID)
	if err != nil {
		t.Fatalf("authorized transcript reader Open: %v", err)
	}
	defer func() { _ = reader.Close() }()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read claimed attachment: %v", err)
	}
	if string(content) != "payload" {
		t.Fatalf("content = %q, want payload", content)
	}

	if _, _, err := svc.Open(ctxAs("user-c"), "user-c", attachment.ID); !errors.Is(err, ErrAttachmentForbidden) {
		t.Fatalf("unauthorized transcript reader Open = %v, want ErrAttachmentForbidden", err)
	}
}

func TestAttachmentDeleteOnlyRemovesStagedRows(t *testing.T) {
	svc, repo, root, _ := newAttachmentTestService(t)
	ctx := context.Background()
	staged := stageTestAttachment(t, svc, "user-a", "staged.png", "x")
	claimed := stageTestAttachment(t, svc, "user-a", "claimed.png", "y")
	if err := svc.Claim(ctx, "user-a", "ws-att", "task-1", "sess-1", []string{claimed.ID}); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if err := svc.Delete(ctx, "user-a", claimed.ID); !errors.Is(err, ErrAttachmentClaimConflict) {
		t.Fatalf("delete claimed = %v, want ErrAttachmentClaimConflict", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, claimed.ID); err != nil {
		t.Fatalf("claimed row must survive a rejected delete: %v", err)
	}
	if err := svc.Delete(ctx, "user-b", staged.ID); !errors.Is(err, ErrAttachmentForbidden) {
		t.Fatalf("foreign delete = %v, want ErrAttachmentForbidden", err)
	}

	if err := svc.Delete(ctx, "user-a", staged.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, staged.ID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("deleted row = %v, want ErrAttachmentNotFound", err)
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", staged.StorageKey)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bytes still present after delete: %v", err)
	}
}

func TestAttachmentClaimAuthorizesWorkspace(t *testing.T) {
	svc, repo, _, auth := newAttachmentTestService(t)
	ctx := context.Background()
	attachment := stageTestAttachment(t, svc, "user-a", "f.png", "x")

	denied := errors.New("workspace not found")
	auth.err = denied
	if err := svc.Claim(ctx, "user-a", "ws-att", "task-1", "sess-1", []string{attachment.ID}); !errors.Is(err, denied) {
		t.Fatalf("denied claim = %v, want the authorizer's error", err)
	}
	stored, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatalf("GetMessageAttachment: %v", err)
	}
	if stored.State != models.AttachmentStateStaged {
		t.Fatalf("state = %q, want staged after a denied claim", stored.State)
	}

	auth.err = nil
	if err := svc.Claim(ctx, "user-a", "ws-att", "task-1", "sess-1", []string{attachment.ID}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	stored, err = repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatalf("GetMessageAttachment: %v", err)
	}
	if stored.State != models.AttachmentStateClaimed || stored.TaskID != "task-1" || stored.SessionID != "sess-1" {
		t.Fatalf("claimed row = %+v", stored)
	}
}

func TestAttachmentReleaseRemovesClaimedRowsAndBytes(t *testing.T) {
	svc, repo, root, _ := newAttachmentTestService(t)
	ctx := context.Background()
	released := stageTestAttachment(t, svc, "user-a", "released.png", "x")
	kept := stageTestAttachment(t, svc, "user-a", "kept.png", "y")
	if err := svc.Claim(ctx, "user-a", "ws-att", "task-1", "sess-1", []string{released.ID, kept.ID}); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if err := svc.Release(ctx, "user-a", "task-1", "sess-1", []string{released.ID}); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, released.ID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("released row = %v, want ErrAttachmentNotFound", err)
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", released.StorageKey)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("released bytes still present: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, kept.ID); err != nil {
		t.Fatalf("unreferenced release must not touch other rows: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", kept.StorageKey)); err != nil {
		t.Fatalf("kept bytes removed: %v", err)
	}

	// A release scoped to a different session removes nothing.
	if err := svc.Release(ctx, "user-a", "task-1", "sess-other", []string{kept.ID}); err != nil {
		t.Fatalf("Release with a foreign session: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, kept.ID); err != nil {
		t.Fatalf("foreign-session release deleted the row: %v", err)
	}
}

func TestAttachmentDeleteByTaskRemovesStagedAndClaimed(t *testing.T) {
	svc, repo, root, _ := newAttachmentTestService(t)
	ctx := context.Background()
	claimed := stageTestAttachment(t, svc, "user-a", "claimed.png", "x")
	other := stageTestAttachment(t, svc, "user-a", "other.png", "y")
	if err := svc.Claim(ctx, "user-a", "ws-att", "task-1", "sess-1", []string{claimed.ID}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := svc.Claim(ctx, "user-a", "ws-att", "task-2", "sess-2", []string{other.ID}); err != nil {
		t.Fatalf("Claim other: %v", err)
	}

	if err := svc.DeleteByTask(ctx, "task-1"); err != nil {
		t.Fatalf("DeleteByTask: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, claimed.ID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("task-1 row = %v, want ErrAttachmentNotFound", err)
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", claimed.StorageKey)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("task-1 bytes still present: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, other.ID); err != nil {
		t.Fatalf("task-2 row must survive: %v", err)
	}
}

func TestAttachmentCleanupExpiredRemovesStagedBytes(t *testing.T) {
	svc, repo, root, _ := newAttachmentTestService(t)
	ctx := context.Background()

	fresh := stageTestAttachment(t, svc, "user-a", "fresh.png", "x")
	stale := stageTestAttachment(t, svc, "user-a", "stale.png", "y")
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE task_message_attachments SET expires_at = ? WHERE id = ?`,
		time.Now().UTC().Add(-time.Hour), stale.ID); err != nil {
		t.Fatalf("age the staged row: %v", err)
	}

	count, err := svc.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if count != 1 {
		t.Fatalf("expired count = %d, want 1", count)
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", stale.StorageKey)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired bytes still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", fresh.StorageKey)); err != nil {
		t.Fatalf("unexpired bytes removed: %v", err)
	}

	again, err := svc.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("second CleanupExpired: %v", err)
	}
	if again != 0 {
		t.Fatalf("second sweep = %d, want 0", again)
	}
}

func TestAttachmentCleanupExpiredPropagatesRepositoryError(t *testing.T) {
	boom := errors.New("registry unavailable")
	svc, err := NewAttachmentService(&failingAttachmentRepo{err: boom}, t.TempDir(), nil, accessTestLogger(t))
	if err != nil {
		t.Fatalf("NewAttachmentService: %v", err)
	}
	ctx := context.Background()
	if count, err := svc.CleanupExpired(ctx); !errors.Is(err, boom) || count != 0 {
		t.Fatalf("CleanupExpired = %d, %v; want 0 and %v", count, err, boom)
	}
	if err := svc.Release(ctx, "user-a", "task-1", "sess-1", []string{"id"}); !errors.Is(err, boom) {
		t.Fatalf("Release error = %v, want %v", err, boom)
	}
	if err := svc.DeleteByTask(ctx, "task-1"); !errors.Is(err, boom) {
		t.Fatalf("DeleteByTask error = %v, want %v", err, boom)
	}
}

type failingAttachmentRepo struct {
	repository.AttachmentRepository
	err error
}

func (f *failingAttachmentRepo) MarkExpiredMessageAttachments(context.Context, time.Time) ([]*models.TaskMessageAttachment, error) {
	return nil, f.err
}

func (f *failingAttachmentRepo) DeleteClaimedMessageAttachments(context.Context, []string, string, string, string) ([]*models.TaskMessageAttachment, error) {
	return nil, f.err
}

func (f *failingAttachmentRepo) DeleteMessageAttachmentsByTask(context.Context, string) ([]*models.TaskMessageAttachment, error) {
	return nil, f.err
}

func TestAttachmentDescriptorExposesNoStorageKey(t *testing.T) {
	svc, _, _, _ := newAttachmentTestService(t)
	attachment := stageTestAttachment(t, svc, "user-a", "f.png", "payload")

	descriptor := svc.Descriptor(attachment)
	want := AttachmentDescriptor{
		ID: attachment.ID, Name: "f.png", MimeType: "image/png", Kind: "image",
		DeliveryMode: attachmentDeliveryModePrompt, SizeBytes: int64(len("payload")),
		State: models.AttachmentStateStaged, ExpiresAt: attachment.ExpiresAt,
	}
	if descriptor != want {
		t.Fatalf("descriptor = %+v, want %+v", descriptor, want)
	}
}

func TestAttachmentRemoveBytesToleratesMissingFile(t *testing.T) {
	svc, _, _, _ := newAttachmentTestService(t)

	// Neither a missing file nor an empty storage key may panic or log-fail.
	svc.removeBytes(nil)
	svc.removeBytes(&models.TaskMessageAttachment{ID: "a"})
	svc.removeBytes(&models.TaskMessageAttachment{ID: "a", StorageKey: "never-written"})
}
