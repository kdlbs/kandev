package backendapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.9
func TestSecretReferenceDeletionWithRealRepositories(t *testing.T) {
	h := newBootStateTestHarness(t)
	ctx := context.Background()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	agents, closeAgents, err := settingsstore.Provide(h.db, h.db, log)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closeAgents()) })
	crypto, err := secrets.NewMasterKeyProvider(t.TempDir())
	require.NoError(t, err)
	store, closeSecrets, err := secrets.Provide(h.db, h.db, crypto)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closeSecrets()) })
	automationStore, err := automation.NewStore(h.db, h.db)
	require.NoError(t, err)
	svc := secrets.NewService(secrets.NewUserVisibleStore(store), log)
	svc.SetReferenceChecker(secretReferenceChecker{
		agents: agents, tasks: h.taskRepo, automations: automationStore,
		authorizeWorkspace: h.taskSvc.AuthorizeWorkspaceAccess,
	}.list)
	secret, err := svc.Create(ctx, &secrets.CreateSecretRequest{Name: "MY_TOKEN", Value: "private-value"})
	require.NoError(t, err)
	seedSecretReferences(t, h, agents, secret.ID)
	require.NoError(t, automationStore.CreateAutomationWithCredentialChoices(ctx, &automation.Automation{
		ID: "secret-reference-automation", WorkspaceID: "secret-workspace", Name: "Daily review", MaxConcurrentRuns: 1,
	}, []automation.AutomationCredentialChoice{{
		Kind: automation.CredentialChoicePersonal, ProfileID: "secret-agent-profile", Key: "AUTO_TOKEN",
		SecretID: secret.ID, State: automation.CredentialChoiceSelected,
	}}))
	router := gin.New()
	secrets.RegisterRoutes(router, ws.NewDispatcher(), svc, log)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/"+secret.ID, nil))
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	var response struct {
		References []secrets.Reference `json:"references"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Len(t, response.References, 4)
	require.Contains(t, response.References, secrets.Reference{
		Kind: "automation", ID: "secret-reference-automation", Name: "Daily review", Key: "AUTO_TOKEN",
	})
	value, err := svc.Reveal(ctx, secret.ID)
	require.NoError(t, err)
	require.Equal(t, "private-value", value)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/"+secret.ID+"?force=true", nil))
	require.Equal(t, http.StatusNoContent, rec.Code)
	_, err = svc.Get(ctx, secret.ID)
	require.ErrorIs(t, err, secrets.ErrNotFound)
	refs, err := (secretReferenceChecker{
		agents: agents, tasks: h.taskRepo, automations: automationStore,
		authorizeWorkspace: h.taskSvc.AuthorizeWorkspaceAccess,
	}).list(ctx, secret.ID)
	require.NoError(t, err)
	require.Len(t, refs, 4, "forced deletion must retain bindings for repair")
}

func seedSecretReferences(t *testing.T, h bootStateTestHarness, agents settingsstore.Repository, secretID string) {
	t.Helper()
	ctx := context.Background()
	agent := &settingsmodels.Agent{Name: "secret-reference-test"}
	require.NoError(t, agents.CreateAgent(ctx, agent))
	require.NoError(t, agents.CreateAgentProfile(ctx, &settingsmodels.AgentProfile{
		ID: "secret-agent-profile", AgentID: agent.ID, Name: "Claude review", Enabled: false,
		EnvVars: []settingsmodels.ProfileEnvVar{{Key: "MY_TOKEN", SecretID: secretID}},
	}))
	require.NoError(t, h.taskRepo.CreateExecutor(ctx, &models.Executor{
		ID: "secret-executor", Name: "Secret executor", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive,
	}))
	require.NoError(t, h.taskRepo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "secret-executor-profile", ExecutorID: "secret-executor", Name: "Local",
		EnvVars: []models.ProfileEnvVar{{Key: "EXEC_TOKEN", SecretID: secretID}},
	}))
	workspace := &models.Workspace{ID: "secret-workspace", Name: "Secret workspace"}
	require.NoError(t, h.taskRepo.CreateWorkspace(ctx, workspace))
	require.NoError(t, h.taskRepo.CreateRepositoryWithSecretBindings(ctx, &models.Repository{
		ID: "secret-repository", WorkspaceID: workspace.ID, Name: "App", SourceType: "local",
	}, []models.RepositorySecretBinding{{Key: "REPO_TOKEN", SecretID: secretID}}))
}
