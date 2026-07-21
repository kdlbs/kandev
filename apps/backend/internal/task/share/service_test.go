package share

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
)

// mockBackend records uploads and supports configurable upload/delete errors.
type mockBackend struct {
	uploads   int
	deletes   []string
	uploadErr error
	deleteErr error
	nextID    string
	nextURL   string
}

func (m *mockBackend) Name() string { return BackendGitHubGist }

func (m *mockBackend) Upload(_ context.Context, _ *Snapshot) (string, string, error) {
	if m.uploadErr != nil {
		return "", "", m.uploadErr
	}
	m.uploads++
	id := m.nextID
	if id == "" {
		id = "gist-1"
	}
	url := m.nextURL
	if url == "" {
		url = "https://gist.github.com/u/" + id
	}
	return id, url, nil
}

func (m *mockBackend) Delete(_ context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deletes = append(m.deletes, id)
	return nil
}

func completedSession() *stubReader {
	completed := time.Now().UTC()
	return &stubReader{
		task: &models.Task{ID: "t-1", Title: "test task"},
		session: &models.TaskSession{
			ID: "s-1", TaskID: "t-1", State: models.TaskSessionStateCompleted,
			StartedAt: completed.Add(-time.Minute), CompletedAt: &completed,
		},
		messages: []*models.Message{
			{ID: "m-1", AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage, Content: "hi", CreatedAt: completed},
		},
	}
}

func TestService_CreateShare_HappyPath(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	mock := &mockBackend{nextID: "abc", nextURL: "https://gist.github.com/u/abc"}
	svc := New(repo, completedSession(), mock, nil, "test-version")

	share, err := svc.CreateShare(context.Background(), "s-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if share.ExternalID != "abc" || share.ExternalURL != "https://gist.github.com/u/abc" {
		t.Fatalf("unexpected share: %+v", share)
	}
	if share.SnapshotSizeBytes <= 0 {
		t.Fatalf("expected non-zero snapshot size, got %d", share.SnapshotSizeBytes)
	}
	got, err := repo.GetByID(context.Background(), share.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != share.ID {
		t.Fatalf("row missing after create")
	}
}

func TestService_CreateShare_RejectsPreHistorySession(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	reader := completedSession()
	reader.session.State = models.TaskSessionStateCreated
	mock := &mockBackend{}
	svc := New(repo, reader, mock, nil, "v")

	_, err := svc.CreateShare(context.Background(), "s-1")
	if !errors.Is(err, ErrSessionNotShareable) {
		t.Fatalf("expected ErrSessionNotShareable, got %v", err)
	}
	if mock.uploads != 0 {
		t.Fatalf("backend should not be called when session is pre-history")
	}
}

func TestService_CreateShare_AllowsRunningSession(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	reader := completedSession()
	reader.session.State = models.TaskSessionStateRunning
	mock := &mockBackend{nextID: "g1"}
	svc := New(repo, reader, mock, nil, "v")

	share, err := svc.CreateShare(context.Background(), "s-1")
	if err != nil {
		t.Fatalf("running session should be shareable, got %v", err)
	}
	if share.ExternalID != "g1" {
		t.Fatalf("expected backend to be called, got share %+v", share)
	}
}

func TestService_CreateShare_BackendErrorLeavesNoRow(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	mock := &mockBackend{uploadErr: errors.New("gateway down")}
	svc := New(repo, completedSession(), mock, nil, "v")

	_, err := svc.CreateShare(context.Background(), "s-1")
	if err == nil {
		t.Fatal("expected upload error")
	}
	if !strings.Contains(err.Error(), "gateway down") {
		t.Fatalf("unexpected error: %v", err)
	}
	rows, err := repo.ListByTaskSession(context.Background(), "s-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected zero rows after backend failure, got %d", len(rows))
	}
}

func TestService_RevokeShare_DeletesAndMarksRevoked(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	mock := &mockBackend{nextID: "abc"}
	svc := New(repo, completedSession(), mock, nil, "v")

	share, err := svc.CreateShare(context.Background(), "s-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.RevokeShare(context.Background(), share.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if len(mock.deletes) != 1 || mock.deletes[0] != "abc" {
		t.Fatalf("expected backend Delete(abc), got %v", mock.deletes)
	}
	got, err := repo.GetByID(context.Background(), share.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RevokedAt == nil {
		t.Fatal("expected RevokedAt to be set")
	}
}

func TestService_RevokeShare_IsIdempotentOnSecondCall(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	mock := &mockBackend{nextID: "abc"}
	svc := New(repo, completedSession(), mock, nil, "v")

	share, err := svc.CreateShare(context.Background(), "s-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.RevokeShare(context.Background(), share.ID); err != nil {
		t.Fatalf("first revoke: %v", err)
	}
	if err := svc.RevokeShare(context.Background(), share.ID); err != nil {
		t.Fatalf("second revoke should be a no-op, got %v", err)
	}
	// Backend Delete should only have been called once; the second revoke
	// short-circuits because IsRevoked() is true.
	if len(mock.deletes) != 1 {
		t.Fatalf("expected 1 delete call, got %d", len(mock.deletes))
	}
}

func TestService_RevokeShare_SurvivesUpstream404(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	mock := &mockBackend{nextID: "abc"}
	svc := New(repo, completedSession(), mock, nil, "v")

	share, err := svc.CreateShare(context.Background(), "s-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	mock.deleteErr = &github.GitHubAPIError{StatusCode: http.StatusNotFound, Endpoint: "/gists/abc"}

	if err := svc.RevokeShare(context.Background(), share.ID); err != nil {
		t.Fatalf("revoke should swallow 404, got %v", err)
	}
	got, err := repo.GetByID(context.Background(), share.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RevokedAt == nil {
		t.Fatal("revoked_at should still be set after upstream 404")
	}
}

func TestService_RevokeShare_PropagatesOtherBackendErrors(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	mock := &mockBackend{nextID: "abc"}
	svc := New(repo, completedSession(), mock, nil, "v")

	share, err := svc.CreateShare(context.Background(), "s-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	mock.deleteErr = errors.New("network down")

	err = svc.RevokeShare(context.Background(), share.ID)
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("expected propagated error, got %v", err)
	}
	got, err := repo.GetByID(context.Background(), share.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RevokedAt != nil {
		t.Fatal("row should NOT be marked revoked when backend delete failed for a non-404 reason")
	}
}

func TestService_PreviewSnapshot_ReturnsRedactedWithoutUpload(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	mock := &mockBackend{}
	svc := New(repo, completedSession(), mock, nil, "v")

	snap, err := svc.PreviewSnapshot(context.Background(), "s-1")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if snap == nil || snap.Task.Title == "" {
		t.Fatalf("expected non-empty snapshot, got %+v", snap)
	}
	if mock.uploads != 0 {
		t.Fatalf("preview must not call backend Upload")
	}
}
