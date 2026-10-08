package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func setupWorkspaceClone(t *testing.T) (bootStateTestHarness, *github.Store, secrets.SecretStore, *models.Workspace) {
	t.Helper()
	h := newBootStateTestHarness(t)
	ctx := t.Context()
	gh, err := github.NewStore(h.db, h.db)
	require.NoError(t, err)
	crypto, err := secrets.NewMasterKeyProvider(t.TempDir())
	require.NoError(t, err)
	secretStore, _, err := secrets.Provide(h.db, h.db, crypto)
	require.NoError(t, err)
	workflowStore, err := workflowrepo.NewWithDB(h.db, h.db, nil)
	require.NoError(t, err)
	h.taskSvc.SetWorkspaceCloner(&workspaceClonePersistence{writer: h.db, tasks: h.taskRepo, workflows: workflowStore, github: gh, secrets: secretStore})
	source := &models.Workspace{Name: "Source", Description: "Details", TaskPrefix: "ABC", ACPIdleSuspensionEnabled: true, ACPIdleTimeoutMinutes: 30}
	_, err = h.taskRepo.CreateWorkspaceWithKanban(ctx, source)
	require.NoError(t, err)
	require.NoError(t, h.taskRepo.CreateRepository(ctx, &models.Repository{WorkspaceID: source.ID, Name: "Repo", SourceType: "local", LocalPath: t.TempDir()}))
	require.NoError(t, gh.UpsertWorkspaceSettings(ctx, &github.WorkspaceSettings{WorkspaceID: source.ID, RepoScopeMode: github.RepoScopeModeAll, SavedPresets: []byte(`[{"id":"mine","default":true}]`), DefaultQueryPresets: []byte(`{"pr":[],"issue":[]}`)}))
	require.NoError(t, gh.UpsertWorkspaceConnection(ctx, &github.WorkspaceConnection{WorkspaceID: source.ID, Source: github.ConnectionSourcePAT, GitHubHost: "github.com", Login: "user", Status: github.ConnectionStatusActive}))
	require.NoError(t, secretStore.Create(ctx, &secrets.SecretWithValue{Secret: secrets.Secret{ID: github.WorkspacePATSecretKey(source.ID), Name: "PAT"}, Value: "private-token"}))
	return h, gh, secretStore, source
}

func TestWorkspaceCloneAtomicCreation(t *testing.T) {
	h, gh, secretStore, source := setupWorkspaceClone(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	target, err := h.taskSvc.CloneWorkspace(ctx, source.ID, "  Copy  ")
	require.NoError(t, err)
	require.Equal(t, "Copy", target.Name)
	require.Equal(t, source.Description, target.Description)
	require.Equal(t, source.TaskPrefix, target.TaskPrefix)
	require.Zero(t, target.TaskSequence)
	reopened, err := h.taskRepo.GetWorkspace(ctx, target.ID)
	require.NoError(t, err)
	require.Equal(t, target, reopened)
	settings, err := gh.GetWorkspaceSettings(ctx, target.ID)
	require.NoError(t, err)
	require.JSONEq(t, `[{"id":"mine","default":true}]`, string(settings.SavedPresets))
	workflows, err := h.taskSvc.ListWorkflows(ctx, target.ID, false)
	require.NoError(t, err)
	require.Len(t, workflows, 1)
	steps, err := h.workflowSvc.ListStepsByWorkflow(ctx, workflows[0].ID)
	require.NoError(t, err)
	require.NotEmpty(t, steps)
	var count int
	require.NoError(t, h.db.GetContext(ctx, &count, h.db.Rebind(`SELECT count(*) FROM tasks WHERE workspace_id = ?`), target.ID))
	require.Zero(t, count)
	require.NoError(t, secretStore.Delete(ctx, github.WorkspacePATSecretKey(source.ID)))
	token, err := secretStore.Reveal(ctx, github.WorkspacePATSecretKey(target.ID))
	require.NoError(t, err)
	require.Equal(t, "private-token", token)
}

func TestWorkspaceCloneEmptyBootstrapAndStaleSource(t *testing.T) {
	h, _, _, source := setupWorkspaceClone(t)
	ctx := t.Context()
	workflows, err := h.taskSvc.ListWorkflows(ctx, source.ID, false)
	require.NoError(t, err)
	for _, workflow := range workflows {
		require.NoError(t, h.taskRepo.DeleteWorkflow(ctx, workflow.ID))
	}
	target, err := h.taskSvc.CloneWorkspace(ctx, source.ID, "Empty copy")
	require.NoError(t, err)
	cloned, err := h.taskSvc.ListWorkflows(ctx, target.ID, false)
	require.NoError(t, err)
	require.Len(t, cloned, 1)
	require.Equal(t, "Kanban", cloned[0].Name)
	stale := *source
	source.Description = "Changed"
	require.NoError(t, h.taskRepo.UpdateWorkspace(ctx, source))
	copier := clonerPersistence(h.db, h, t)
	_, err = copier.CloneWorkspace(ctx, &stale, &models.Workspace{Name: "Rejected"})
	require.ErrorIs(t, err, repoerrors.ErrTaskVersionConflict)
}

type cancellingCloneIntegration struct {
	store  *github.Store
	cancel context.CancelFunc
}

func (c cancellingCloneIntegration) CopyWorkspaceConfigurationTx(ctx context.Context, tx *sqlx.Tx, sourceID, targetID string) (bool, error) {
	needsPAT, err := c.store.CopyWorkspaceConfigurationTx(ctx, tx, sourceID, targetID)
	c.cancel()
	return needsPAT, err
}

func TestWorkspaceCloneCancellation(t *testing.T) {
	h, gh, secretStore, source := setupWorkspaceClone(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	copier := clonerPersistence(h.db, h, t)
	copier.github = cancellingCloneIntegration{store: gh, cancel: cancel}
	copier.secrets = secretStore.(secrets.CredentialCopier)
	_, err := copier.CloneWorkspace(ctx, source, &models.Workspace{ID: "cancelled", Name: "Cancelled"})
	require.ErrorIs(t, err, context.Canceled)
	_, err = h.taskRepo.GetWorkspace(t.Context(), "cancelled")
	require.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)
	_, err = secretStore.Get(t.Context(), github.WorkspacePATSecretKey("cancelled"))
	require.ErrorIs(t, err, secrets.ErrNotFound)
}

func TestWorkspaceCloneReviewProfileAdmission(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unavailable", true: "shared"}[allowed], func(t *testing.T) {
			h, _, secretStore, source := setupWorkspaceClone(t)
			ctx := t.Context()
			copier := clonerPersistence(h.db, h, t)
			copier.secrets = secretStore.(secrets.CredentialCopier)
			workflows, err := h.taskSvc.ListWorkflows(ctx, source.ID, false)
			require.NoError(t, err)
			steps, err := copier.workflows.ListStepsByWorkflow(ctx, workflows[0].ID)
			require.NoError(t, err)
			steps[0].Events.OnEnter = []workflowmodels.OnEnterAction{{
				Type:   workflowmodels.OnEnterRunCodeReview,
				Config: map[string]any{workflowmodels.ReviewAgentProfileConfigKey: "reviewer"},
			}}
			require.NoError(t, copier.workflows.UpdateStep(ctx, steps[0]))
			copier.validateProfile = func(_ context.Context, id string) error {
				require.Equal(t, "reviewer", id)
				if !allowed {
					return repoerrors.ErrWorkspaceCloneConfiguration
				}
				return nil
			}
			target := &models.Workspace{ID: "review-copy", Name: "Review copy"}
			cloned, err := copier.CloneWorkspace(ctx, source, target)
			if !allowed {
				require.ErrorIs(t, err, repoerrors.ErrWorkspaceCloneConfiguration)
				_, err = h.taskRepo.GetWorkspace(ctx, target.ID)
				require.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)
				return
			}
			require.NoError(t, err)
			copiedSteps, err := copier.workflows.ListStepsByWorkflow(ctx, cloned[0].ID)
			require.NoError(t, err)
			require.Equal(t, "reviewer", copiedSteps[0].Events.OnEnter[0].Config[workflowmodels.ReviewAgentProfileConfigKey])
		})
	}
}

func TestWorkspaceCloneRollback(t *testing.T) {
	for _, table := range []string{"workspace_members", "repositories", "workflows", "workflow_steps", "github_workspace_settings", "github_workspace_connections", "secrets"} {
		t.Run(table, func(t *testing.T) {
			h, _, _, source := setupWorkspaceClone(t)
			if table == "workspace_members" {
				source.OwnerID = "owner"
				_, err := h.db.ExecContext(t.Context(), h.db.Rebind(`UPDATE workspaces SET owner_id = ? WHERE id = ?`), source.OwnerID, source.ID)
				require.NoError(t, err)
			}
			_, err := h.db.Exec(`CREATE TRIGGER reject_clone BEFORE INSERT ON ` + table + ` BEGIN SELECT RAISE(ABORT,'injected clone failure'); END`)
			require.NoError(t, err)
			var before, after int
			require.NoError(t, h.db.Get(&before, `SELECT count(*) FROM workspaces`))
			target := &models.Workspace{Name: "Rejected", OwnerID: "owner"}
			cloner := h.taskSvc
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			if table == "workspace_members" {
				// Exercise the owner insert without a separate placement write.
				copier := clonerPersistence(h.db, h, t)
				_, err = copier.CloneWorkspace(ctx, source, target)
			} else {
				_, err = cloner.CloneWorkspace(ctx, source.ID, "Rejected")
			}
			require.Error(t, err)
			require.ErrorContains(t, err, "injected clone failure")
			require.NoError(t, h.db.Get(&after, `SELECT count(*) FROM workspaces`))
			require.Equal(t, before, after)
			require.NoError(t, h.db.Get(&after, `SELECT count(*) FROM secrets`))
			require.Equal(t, 1, after)
		})
	}
}

func clonerPersistence(writer *sqlx.DB, h bootStateTestHarness, t *testing.T) *workspaceClonePersistence {
	gh, err := github.NewStore(writer, writer)
	require.NoError(t, err)
	wf, err := workflowrepo.NewWithDB(writer, writer, nil)
	require.NoError(t, err)
	return &workspaceClonePersistence{writer: writer, tasks: h.taskRepo, workflows: wf, github: gh}
}

func TestWorkspaceCloneHTTP(t *testing.T) {
	h, _, _, source := setupWorkspaceClone(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers.RegisterWorkspaceRoutes(router, websocket.NewDispatcher(), h.taskSvc, cloneLogger(t))
	for _, tc := range []struct {
		name, body string
		status     int
	}{{"success", `{"name":"Copy"}`, 201}, {"blank", `{"name":" "}`, 400}, {"reserved", `{"name":"Improve Kandev"}`, 400}} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+source.ID+"/clone", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			require.NotContains(t, rec.Body.String(), "private-token")
		})
	}
}

func TestWorkspaceCloneHTTPAuthority(t *testing.T) {
	h, _, _, source := setupWorkspaceClone(t)
	_, err := h.db.ExecContext(t.Context(), h.db.Rebind(`UPDATE workspaces SET owner_id = ? WHERE id = ?`), "owner", source.ID)
	require.NoError(t, err)
	require.NoError(t, h.taskRepo.UpsertWorkspaceMember(t.Context(), &models.WorkspaceMember{WorkspaceID: source.ID, UserID: "viewer", Role: "viewer"}))
	router := gin.New()
	handlers.RegisterWorkspaceRoutes(router, websocket.NewDispatcher(), h.taskSvc, cloneLogger(t))
	for _, tc := range []struct {
		user   string
		status int
	}{{"stranger", 404}, {"viewer", 403}, {"owner", 201}} {
		t.Run(tc.user, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+source.ID+"/clone", strings.NewReader(`{"name":"Owned copy","owner_id":"intruder"}`))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: tc.user, Role: authn.RoleMember}))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			if tc.status == 201 {
				require.Contains(t, rec.Body.String(), `"owner_id":"owner"`)
				require.Contains(t, rec.Body.String(), `"secret.manage"`)
				require.Contains(t, rec.Body.String(), `"member_count":1`)
				require.NotContains(t, rec.Body.String(), "intruder")
			}
		})
	}
}

var _ taskservice.WorkspaceClonePersistence = (*workspaceClonePersistence)(nil)

func cloneLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	return log
}
