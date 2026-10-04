package backendapp

import (
	"context"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.5
func TestGitHubIssueMutationFailures(t *testing.T) {
	h := newIssueMutationHarness(t, nil)
	gh, store := issueMutationAuthenticatedGitHub(t, h, issueMutationRemote)
	router := issueMutationRouter(t, h, gh)
	ctx := context.Background()
	before, err := h.repos[0].GetTask(ctx, "issue-task")
	require.NoError(t, err)
	for _, tt := range []struct {
		body, path string
		status     int
	}{
		{`{`, "issue-task", 400}, {`{}`, "issue-task", 400}, {`{"owner":"acme","repo":"api","number":404}`, "issue-task", 404},
		{`{"owner":"other","repo":"tools","number":42}`, "issue-task", 422}, {`{"owner":"acme","repo":"api","number":42}`, "missing", 404},
	} {
		response := issueMutationHTTP(ctx, router, http.MethodPut, "/api/v1/github/tasks/"+tt.path+"/issue", tt.body)
		require.Equal(t, tt.status, response.Code, response.Body.String())
	}
	require.NoError(t, store.DeleteWorkspaceConnection(ctx, "issue-ws"))
	response := issueMutationHTTP(ctx, router, http.MethodPut, "/api/v1/github/tasks/issue-task/issue", `{"owner":"acme","repo":"api","number":42}`)
	require.Equal(t, 503, response.Code, response.Body.String())
	gh.SetTaskIssueStore(nil)
	response = issueMutationHTTP(ctx, router, http.MethodDelete, "/api/v1/github/tasks/issue-task/issue", "")
	require.Equal(t, 503, response.Code)
	after, err := h.repos[0].GetTask(ctx, "issue-task")
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Empty(t, h.buses[0].snapshot())
}

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.5
func TestGitHubIssueMutationTaskAuthorization(t *testing.T) {
	h := newIssueMutationHarness(t, nil)
	ctx := context.Background()
	require.NoError(t, h.repos[0].TransferWorkspaceOwnership(ctx, "issue-ws", "", "owner"))
	require.NoError(t, h.repos[0].UpsertWorkspaceMember(ctx, &models.WorkspaceMember{WorkspaceID: "issue-ws", UserID: "viewer", Role: "viewer"}))
	before, err := h.repos[0].GetTask(ctx, "issue-task")
	require.NoError(t, err)
	for _, user := range []string{"viewer", "foreign"} {
		scoped := authn.WithIdentity(ctx, authn.Identity{UserID: user, Role: authn.RoleMember})
		_, err := h.services[0].UpdateTaskGitHubIssue(scoped, "issue-task", nil)
		if user == "viewer" {
			require.ErrorIs(t, err, taskservice.ErrForbidden)
		} else {
			require.ErrorIs(t, err, sqliterepo.ErrTaskNotFound)
		}
	}
	after, err := h.repos[0].GetTask(ctx, "issue-task")
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Empty(t, h.buses[0].snapshot())
	owner := authn.WithIdentity(ctx, authn.Identity{UserID: "owner", Role: authn.RoleMember})
	_, err = h.services[0].UpdateTaskGitHubIssue(owner, "issue-task", nil)
	require.NoError(t, err)
	require.Len(t, h.buses[0].snapshot(), 1)
}

type issueMutationAdmissionStore struct {
	githubTaskIssueStoreAdapter
	afterRepository func(context.Context) error
	beforeMutation  func(context.Context) error
	listError       error
}

func (s issueMutationAdmissionStore) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	repo, err := s.githubTaskIssueStoreAdapter.GetRepository(ctx, id)
	if err == nil && s.afterRepository != nil {
		err = s.afterRepository(ctx)
	}
	return repo, err
}
func (s issueMutationAdmissionStore) ListTaskRepositories(ctx context.Context, id string) ([]*models.TaskRepository, error) {
	if s.listError != nil {
		return nil, s.listError
	}
	return s.githubTaskIssueStoreAdapter.ListTaskRepositories(ctx, id)
}
func (s issueMutationAdmissionStore) UpdateTaskGitHubIssue(ctx context.Context, id string, link *models.TaskGitHubIssueLink) (*models.Task, error) {
	if s.beforeMutation != nil {
		if err := s.beforeMutation(ctx); err != nil {
			return nil, err
		}
	}
	return s.githubTaskIssueStoreAdapter.UpdateTaskGitHubIssue(ctx, id, link)
}

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.5
func TestGitHubIssueMutationCancellationBoundary(t *testing.T) {
	for _, afterValidation := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel_fetch", true: "without_cancel_admission"}[afterValidation], func(t *testing.T) {
			h := newIssueMutationHarness(t, nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			transport := issueMutationTransport(func(r *http.Request) (*http.Response, error) {
				if !afterValidation && r.URL.Path == "/repos/acme/api/issues/42" {
					cancel()
					return nil, r.Context().Err()
				}
				return issueMutationRemote(r)
			})
			gh, _ := issueMutationAuthenticatedGitHub(t, h, transport)
			gh.SetTaskIssueStore(issueMutationAdmissionStore{githubTaskIssueStoreAdapter: githubTaskIssueStoreAdapter{svc: h.services[0]}, afterRepository: func(context.Context) error {
				if afterValidation {
					cancel()
				}
				return nil
			}})
			_, err := gh.LinkTaskIssueForUser(ctx, "", "issue-task", issueMutationRequest(42))
			if afterValidation {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, context.Canceled)
			}
			stored, err := h.repos[0].GetTask(context.Background(), "issue-task")
			require.NoError(t, err)
			number := 0
			count := 0
			if afterValidation {
				number = 42
				count = 1
			}
			assertIssueMutationIdentity(t, stored.Metadata, number)
			require.Len(t, h.buses[0].snapshot(), count)
		})
	}
}

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.5
func TestGitHubIssueMutationDeletedBeforeCommit(t *testing.T) {
	h := newIssueMutationHarness(t, nil)
	ctx := context.Background()
	gh, _ := issueMutationGitHub(t, h.services[0])
	gh.SetTaskIssueStore(issueMutationAdmissionStore{githubTaskIssueStoreAdapter: githubTaskIssueStoreAdapter{svc: h.services[0]}, beforeMutation: func(ctx context.Context) error { return h.repos[1].DeleteTask(ctx, "issue-task") }})
	_, err := gh.LinkTaskIssue(ctx, "issue-task", issueMutationRequest(42))
	require.ErrorIs(t, err, github.ErrTaskNotFound)
	require.ErrorIs(t, err, sqliterepo.ErrTaskNotFound)
	require.Empty(t, h.buses[0].snapshot())
}
