package share

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type allowShareAccess struct{}

func (allowShareAccess) AuthorizeTaskSessionAccess(context.Context, string, string) error {
	return nil
}

func (allowShareAccess) AuthorizeSessionAccess(context.Context, string) error {
	return nil
}

type pairShareAccess struct{}

func (pairShareAccess) AuthorizeTaskSessionAccess(_ context.Context, taskID, sessionID string) error {
	if taskID != "t-1" || sessionID != "s-1" {
		return ErrNotFound
	}
	return nil
}

func (pairShareAccess) AuthorizeSessionAccess(_ context.Context, sessionID string) error {
	if sessionID != "s-1" {
		return ErrNotFound
	}
	return nil
}

type recordingShareAccess struct {
	taskSessionCalls int
	sessionCalls     int
	taskSessionErr   error
	taskSessionErrs  []error
	sessionErr       error
}

func (a *recordingShareAccess) AuthorizeTaskSessionAccess(context.Context, string, string) error {
	call := a.taskSessionCalls
	a.taskSessionCalls++
	if call < len(a.taskSessionErrs) {
		return a.taskSessionErrs[call]
	}
	return a.taskSessionErr
}

func (a *recordingShareAccess) AuthorizeSessionAccess(context.Context, string) error {
	a.sessionCalls++
	return a.sessionErr
}

type countingTaskReader struct {
	base             TaskReader
	getTaskCalls     int
	getSessionCalls  int
	listMessageCalls int
}

func (r *countingTaskReader) GetTask(ctx context.Context, id string) (*models.Task, error) {
	r.getTaskCalls++
	return r.base.GetTask(ctx, id)
}

func (r *countingTaskReader) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	r.getSessionCalls++
	return r.base.GetTaskSession(ctx, id)
}

func (r *countingTaskReader) ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error) {
	r.listMessageCalls++
	return r.base.ListMessages(ctx, sessionID)
}

func TestNormalizeAuthorizationError(t *testing.T) {
	t.Parallel()
	operationalErr := errors.New("db transient")
	tests := []struct {
		name string
		in   error
		want error
	}{
		{name: "nil", in: nil, want: nil},
		{name: "share not found", in: ErrNotFound, want: ErrNotFound},
		{name: "task not found", in: repoerrors.ErrTaskNotFound, want: ErrNotFound},
		{name: "workspace not found", in: repoerrors.ErrWorkspaceNotFound, want: ErrNotFound},
		{name: "session not found", in: models.ErrTaskSessionNotFound, want: ErrNotFound},
		{name: "operational failure", in: operationalErr, want: ErrAuthorization},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeAuthorizationError(tt.in)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("normalizeAuthorizationError(%v) = %v, want nil", tt.in, got)
				}
				return
			}
			if !errors.Is(got, tt.want) {
				t.Fatalf("normalizeAuthorizationError(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestService_TaskSessionAuthorizationPrecedesSensitiveReads(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	reader := &countingTaskReader{base: completedSession()}
	backend := &mockBackend{}
	authorizer := &recordingShareAccess{taskSessionErr: ErrNotFound}
	svc := New(repo, reader, authorizer, backend, nil, "v")
	ctx := context.Background()

	if _, err := svc.PreviewSnapshot(ctx, "t-foreign", "s-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("preview error = %v, want ErrNotFound", err)
	}
	if _, err := svc.CreateShare(ctx, "t-foreign", "s-1", "en"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("create error = %v, want ErrNotFound", err)
	}
	if err := svc.CheckBackendAccess(ctx, "t-foreign", "s-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("backend access error = %v, want ErrNotFound", err)
	}
	if _, err := svc.ListBySession(ctx, "t-foreign", "s-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("list error = %v, want ErrNotFound", err)
	}

	if reader.getTaskCalls != 0 || reader.getSessionCalls != 0 || reader.listMessageCalls != 0 {
		t.Fatalf("authorization denial reached task reader: %+v", reader)
	}
	if backend.uploads != 0 || len(backend.accessWorkspaces) != 0 {
		t.Fatalf("authorization denial reached backend: uploads=%d access=%v", backend.uploads, backend.accessWorkspaces)
	}
	if authorizer.taskSessionCalls != 4 {
		t.Fatalf("task-session authorization calls = %d, want 4", authorizer.taskSessionCalls)
	}
}

func TestService_RevokeAuthorizesBeforeRevokedShortcutAndMutation(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	backend := &mockBackend{nextID: "gist-1"}
	authorizer := &recordingShareAccess{}
	svc := New(repo, completedSession(), authorizer, backend, nil, "v")

	share, err := svc.CreateShare(context.Background(), "t-1", "s-1", "en")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	authorizer.sessionErr = ErrNotFound
	if err := svc.RevokeShare(context.Background(), share.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoke error = %v, want ErrNotFound", err)
	}
	if len(backend.deletes) != 0 {
		t.Fatalf("denied revoke called backend Delete: %v", backend.deletes)
	}
	row, err := repo.GetByID(context.Background(), share.ID)
	if err != nil {
		t.Fatalf("get share: %v", err)
	}
	if row.RevokedAt != nil {
		t.Fatal("denied revoke mutated the share row")
	}

	authorizer.sessionErr = nil
	if err := svc.RevokeShare(context.Background(), share.ID); err != nil {
		t.Fatalf("authorized revoke: %v", err)
	}
	if len(backend.deletes) != 1 {
		t.Fatalf("authorized revoke Delete calls = %d, want 1", len(backend.deletes))
	}
	authorizer.sessionErr = ErrNotFound
	if err := svc.RevokeShare(context.Background(), share.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("denied repeat revoke error = %v, want ErrNotFound", err)
	}
	if len(backend.deletes) != 1 {
		t.Fatalf("denied repeat revoke called backend again: %v", backend.deletes)
	}
}
