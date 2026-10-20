package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

func TestTurnChangeHistoryHTTPIsSessionScopedAndExcludesPrivateFieldsAndContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.SetTenancyEnforced(false)
	store := &turnChangeHistoryHTTPRepo{
		set: &models.TurnChangeSet{
			ID: "set-1", TaskID: "task-b", TaskSessionID: "sess-b", TurnID: "turn-1", Revision: 4,
			Availability: models.TurnChangeAvailabilityReady, Complete: true, FileCount: 1,
			RuntimeExecutionID: "private-runtime", SettingsUserID: "private-user", ActorID: "private-actor",
		},
		repositoryChange: &models.TurnRepositoryChangeSet{
			ID: "repository-change-1", TurnChangeSetID: "set-1", CheckoutID: "checkout-1",
			DisplayName: "repo", Availability: models.TurnChangeAvailabilityReady,
		},
		file: &models.TurnFileChange{
			ID: "file-1", RepositoryChangeID: "repository-change-1", CheckoutID: "checkout-1",
			Path: "src/example.go", Kind: "modified", ContentAvailability: models.TurnChangeAvailabilityReady,
			CanonicalContentID: "private-content-id",
		},
		content: &models.TurnChangeContentPayload{
			FileChangeID: "file-1", Variant: models.TurnChangeContentCanonicalPatch, Content: []byte("-old\n+new\n"), Digest: "digest",
		},
	}
	ownerRepo := &foreignSessionRepo{}
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: ownerRepo, Tasks: ownerRepo, Sessions: ownerRepo, TurnChanges: store,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	router := gin.New()
	NewTaskHandlers(svc, nil, nil, nil, log).registerHTTP(router)

	foreign := httptest.NewRecorder()
	foreignRequest := httptest.NewRequest(http.MethodGet, "/api/v1/task-sessions/sess-b/turn-changes", nil)
	foreignRequest = foreignRequest.WithContext(authn.WithIdentity(foreignRequest.Context(), authn.Identity{UserID: "user-a", Role: authn.RoleMember}))
	router.ServeHTTP(foreign, foreignRequest)
	require.Equal(t, http.StatusNotFound, foreign.Code)

	owner := httptest.NewRecorder()
	ownerRequest := httptest.NewRequest(http.MethodGet, "/api/v1/task-sessions/sess-b/turn-changes", nil)
	ownerRequest = ownerRequest.WithContext(authn.WithIdentity(ownerRequest.Context(), authn.Identity{UserID: "user-b", Role: authn.RoleMember}))
	router.ServeHTTP(owner, ownerRequest)
	require.Equal(t, http.StatusOK, owner.Code, owner.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(owner.Body.Bytes(), &body))
	require.NotContains(t, owner.Body.String(), "private-runtime")
	require.NotContains(t, owner.Body.String(), "private-user")
	require.NotContains(t, owner.Body.String(), "private-actor")
	require.NotContains(t, owner.Body.String(), "private-content-id")
	require.Equal(t, 1, store.listCalls)

	files := httptest.NewRecorder()
	filesRequest := httptest.NewRequest(http.MethodGet,
		"/api/v1/task-sessions/sess-b/turn-changes/set-1/repositories/repository-change-1/files", nil)
	filesRequest = filesRequest.WithContext(authn.WithIdentity(filesRequest.Context(), authn.Identity{UserID: "user-b", Role: authn.RoleMember}))
	router.ServeHTTP(files, filesRequest)
	require.Equal(t, http.StatusOK, files.Code, files.Body.String())
	require.Contains(t, files.Body.String(), "src/example.go")
	require.NotContains(t, files.Body.String(), "private-content-id")

	content := httptest.NewRecorder()
	contentRequest := httptest.NewRequest(http.MethodGet,
		"/api/v1/task-sessions/sess-b/turn-changes/set-1/files/file-1/content?variant=canonical_patch", nil)
	contentRequest = contentRequest.WithContext(authn.WithIdentity(contentRequest.Context(), authn.Identity{UserID: "user-b", Role: authn.RoleMember}))
	router.ServeHTTP(content, contentRequest)
	require.Equal(t, http.StatusOK, content.Code, content.Body.String())
	require.Contains(t, content.Body.String(), base64.StdEncoding.EncodeToString([]byte("-old\n+new\n")))

	store.set.Availability = models.TurnChangeAvailabilityExpired
	store.set.ExpiryReason = string(models.TurnChangeReasonExpiredTaskLimit)
	expired := httptest.NewRecorder()
	expiredRequest := httptest.NewRequest(http.MethodGet,
		"/api/v1/task-sessions/sess-b/turn-changes/set-1/files/file-1/content?variant=canonical_patch", nil)
	expiredRequest = expiredRequest.WithContext(authn.WithIdentity(expiredRequest.Context(), authn.Identity{UserID: "user-b", Role: authn.RoleMember}))
	router.ServeHTTP(expired, expiredRequest)
	require.Equal(t, http.StatusGone, expired.Code)
	require.Contains(t, expired.Body.String(), string(models.TurnChangeReasonExpiredTaskLimit))
}

type turnChangeHistoryHTTPRepo struct {
	repository.TurnChangesRepository
	set              *models.TurnChangeSet
	repositoryChange *models.TurnRepositoryChangeSet
	file             *models.TurnFileChange
	content          *models.TurnChangeContentPayload
	listCalls        int
}

func (r *turnChangeHistoryHTTPRepo) ListTurnChangeSets(context.Context, string, string, int, int) ([]*models.TurnChangeSet, int, error) {
	r.listCalls++
	if r.set == nil {
		return []*models.TurnChangeSet{}, 0, nil
	}
	return []*models.TurnChangeSet{r.set}, 1, nil
}

func (r *turnChangeHistoryHTTPRepo) GetTurnChangeSet(_ context.Context, taskID, sessionID, setID string) (*models.TurnChangeSet, error) {
	if r.set == nil || r.set.TaskID != taskID || r.set.TaskSessionID != sessionID || r.set.ID != setID {
		return nil, repoerrors.ErrTurnChangeSetNotFound
	}
	return r.set, nil
}

func (r *turnChangeHistoryHTTPRepo) ListTurnRepositoryChanges(_ context.Context, setID string) ([]*models.TurnRepositoryChangeSet, error) {
	if r.set == nil || r.set.ID != setID {
		return nil, repoerrors.ErrTurnChangeSetNotFound
	}
	return []*models.TurnRepositoryChangeSet{r.repositoryChange}, nil
}

func (r *turnChangeHistoryHTTPRepo) GetTurnRepositoryChange(_ context.Context, setID, repositoryChangeID string) (*models.TurnRepositoryChangeSet, error) {
	if r.set == nil || r.repositoryChange == nil || r.set.ID != setID || r.repositoryChange.ID != repositoryChangeID {
		return nil, repoerrors.ErrTurnChangeRelationship
	}
	return r.repositoryChange, nil
}

func (r *turnChangeHistoryHTTPRepo) ListTurnFileChanges(_ context.Context, setID, repositoryChangeID string, _, _ int) ([]*models.TurnFileChange, int, error) {
	if r.set == nil || r.repositoryChange == nil || r.set.ID != setID || r.repositoryChange.ID != repositoryChangeID {
		return nil, 0, repoerrors.ErrTurnChangeRelationship
	}
	return []*models.TurnFileChange{r.file}, 1, nil
}

func (r *turnChangeHistoryHTTPRepo) AcquireTurnChangeContentLease(context.Context, string, time.Duration) (*models.TurnChangeContentLease, error) {
	return &models.TurnChangeContentLease{ID: "lease"}, nil
}

func (r *turnChangeHistoryHTTPRepo) ReadTurnChangeContent(_ context.Context, setID, fileID string, variant models.TurnChangeContentVariant) (*models.TurnChangeContentPayload, error) {
	if r.set == nil || r.file == nil || r.set.ID != setID || r.file.ID != fileID || r.content.Variant != variant {
		return nil, repoerrors.ErrTurnChangeContentNotFound
	}
	return r.content, nil
}

func (*turnChangeHistoryHTTPRepo) ReleaseTurnChangeContentLease(context.Context, string) error {
	return nil
}
