package github

import (
	"context"
	"errors"
	"testing"
)

type reviewWatchConnectionReader struct {
	workspace *WorkspaceConnection
	user      *UserConnection
	err       error
}

func (r reviewWatchConnectionReader) GetWorkspaceConnection(
	context.Context, string,
) (*WorkspaceConnection, error) {
	return r.workspace, r.err
}

func (r reviewWatchConnectionReader) GetUserConnection(
	context.Context, string, string,
) (*UserConnection, error) {
	return r.user, r.err
}

func TestCreateReviewWatchForUserRequiresWorkspace(t *testing.T) {
	service := &Service{}
	for _, request := range []*CreateReviewWatchRequest{nil, {}} {
		_, err := service.CreateReviewWatchForUser(context.Background(), "user-1", request)
		if !errors.Is(err, ErrGitHubWorkspaceRequired) {
			t.Fatalf("CreateReviewWatchForUser() error = %v, want workspace required", err)
		}
	}
}

func TestCreateReviewWatchForUserPropagatesResolutionFailureWithoutCreatingWatch(t *testing.T) {
	store := newTestStore(t)
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	t.Cleanup(service.Stop)
	wantErr := errors.New("connection lookup failed")
	service.resolver = NewCredentialResolver(reviewWatchConnectionReader{err: wantErr}, nil)

	_, err := service.CreateReviewWatchForUser(
		context.Background(), "user-1", &CreateReviewWatchRequest{WorkspaceID: "workspace-1"},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("CreateReviewWatchForUser() error = %v, want %v", err, wantErr)
	}
	assertNoReviewWatches(t, store, "workspace-1")
}

func TestCreateReviewWatchForUserRequiresVerifiedLogin(t *testing.T) {
	service, store := reviewWatchAuthTestService(t, "")

	_, err := service.CreateReviewWatchForUser(
		context.Background(), "user-1", &CreateReviewWatchRequest{WorkspaceID: "workspace-1"},
	)
	if !errors.Is(err, ErrGitHubPersonalRequired) {
		t.Fatalf("CreateReviewWatchForUser() error = %v, want personal required", err)
	}
	assertNoReviewWatches(t, store, "workspace-1")
}

func TestCreateReviewWatchForUserPersistsResolvedTargetLogin(t *testing.T) {
	service, store := reviewWatchAuthTestService(t, "octocat")

	watch, err := service.CreateReviewWatchForUser(
		context.Background(), "user-1", &CreateReviewWatchRequest{WorkspaceID: "workspace-1"},
	)
	if err != nil {
		t.Fatalf("CreateReviewWatchForUser(): %v", err)
	}
	if watch.TargetLogin != "octocat" {
		t.Fatalf("TargetLogin = %q, want octocat", watch.TargetLogin)
	}
	stored, err := store.GetReviewWatch(context.Background(), watch.ID)
	if err != nil {
		t.Fatalf("GetReviewWatch(): %v", err)
	}
	if stored == nil || stored.TargetLogin != "octocat" {
		t.Fatalf("stored watch = %+v, want target login octocat", stored)
	}
}

func reviewWatchAuthTestService(t *testing.T, login string) (*Service, *Store) {
	t.Helper()
	store := newTestStore(t)
	client := NewMockClient()
	service := NewService(client, AuthMethodNone, nil, store, nil, testLogger(t))
	t.Cleanup(service.Stop)
	reader := reviewWatchConnectionReader{
		workspace: &WorkspaceConnection{
			WorkspaceID: "workspace-1", Source: ConnectionSourcePAT,
			Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
		user: &UserConnection{
			WorkspaceID: "workspace-1", UserID: "user-1", Login: login,
			Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
	}
	service.resolver = NewCredentialResolver(reader, nil)
	service.resolver.SetUserProvider(fixedUserCredentialProvider{client: client})
	service.resolver.SetAutomationProvider(testAutomationCredentialProvider{client: client})
	return service, store
}

func assertNoReviewWatches(t *testing.T, store *Store, workspaceID string) {
	t.Helper()
	watches, err := store.ListReviewWatches(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("ListReviewWatches(): %v", err)
	}
	if len(watches) != 0 {
		t.Fatalf("review watches = %+v, want none", watches)
	}
}
