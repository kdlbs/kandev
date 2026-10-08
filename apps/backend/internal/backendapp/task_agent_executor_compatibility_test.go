package backendapp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

type compatibilityAgentReaderStub struct {
	agent *settingsmodels.Agent
}

func (r compatibilityAgentReaderStub) GetAgent(context.Context, string) (*settingsmodels.Agent, error) {
	return r.agent, nil
}

type compatibilityProfileExecutionValidatorStub struct {
	err error
}

func (v compatibilityProfileExecutionValidatorStub) ValidateProfile(context.Context, string) error {
	return v.err
}

func newCompatibilityRegistry(t *testing.T, agent agents.Agent) *registry.Registry {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	agentRegistry := registry.NewRegistry(log)
	require.NoError(t, agentRegistry.Register(agent))
	return agentRegistry
}

func TestTaskAgentExecutorCompatibilityAllowsLocalExecution(t *testing.T) {
	agent := agents.NewMockAgent()
	agent.SetEnabled(true)
	validator := taskAgentExecutorCompatibilityValidator{
		profiles:      compatibilityAgentReaderStub{agent: &settingsmodels.Agent{Name: agent.ID()}},
		agentRegistry: newCompatibilityRegistry(t, agent),
	}

	err := validator.ValidateAgentProfileForExecutor(
		context.Background(),
		&settingsmodels.AgentProfile{ID: "replacement", AgentID: "agent-row"},
		&models.Executor{ID: "local", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive},
		nil,
	)
	require.NoError(t, err)
}

func TestTaskAgentExecutorCompatibilityRejectsRemoteExecutionWithoutCredentials(t *testing.T) {
	agent := agents.NewMockAgentWithID("codex-acp", "Codex ACP Agent", "Codex")
	agent.SetEnabled(true)
	validator := taskAgentExecutorCompatibilityValidator{
		profiles:      compatibilityAgentReaderStub{agent: &settingsmodels.Agent{Name: agent.ID()}},
		agentRegistry: newCompatibilityRegistry(t, agent),
	}

	err := validator.ValidateAgentProfileForExecutor(
		context.Background(),
		&settingsmodels.AgentProfile{ID: "replacement", AgentID: "agent-row"},
		&models.Executor{ID: "ssh", Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive},
		nil,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "remote executor profile is required")
}

func TestTaskAgentExecutorCompatibilityAllowsValidatedDynamicProfile(t *testing.T) {
	validator := taskAgentExecutorCompatibilityValidator{
		profiles:        compatibilityAgentReaderStub{agent: &settingsmodels.Agent{Name: agents.DynamicAgentID}},
		agentRegistry:   newCompatibilityRegistry(t, agents.NewDynamicAgent()),
		dynamicResolver: compatibilityProfileExecutionValidatorStub{},
	}

	err := validator.ValidateAgentProfileForExecutor(
		context.Background(),
		&settingsmodels.AgentProfile{ID: "dynamic-profile", AgentID: agents.DynamicAgentID},
		&models.Executor{ID: "local", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive},
		nil,
	)
	require.NoError(t, err)
}

func TestTaskAgentExecutorCompatibilityRejectsDynamicProfileWithoutValidator(t *testing.T) {
	validator := taskAgentExecutorCompatibilityValidator{
		profiles:      compatibilityAgentReaderStub{agent: &settingsmodels.Agent{Name: agents.DynamicAgentID}},
		agentRegistry: newCompatibilityRegistry(t, agents.NewDynamicAgent()),
	}

	err := validator.ValidateAgentProfileForExecutor(
		context.Background(),
		&settingsmodels.AgentProfile{ID: "dynamic-profile", AgentID: agents.DynamicAgentID},
		&models.Executor{ID: "local", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive},
		nil,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dynamic agent routing validator is unavailable")
}

func TestTaskAgentExecutorCompatibilityRequiresManagedPairAndCatalogModel(t *testing.T) {
	cloudAgent := agents.NewCursorCloudAgent()
	store := newBackendappSecretStore()
	require.NoError(t, store.Create(context.Background(), &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "cursor-key", Scope: secrets.ScopeGlobal}, Value: "provider-key",
	}))
	modelChecks := 0
	validator := taskAgentExecutorCompatibilityValidator{
		profiles:           compatibilityAgentReaderStub{agent: &settingsmodels.Agent{Name: agents.CursorCloudAgentID}},
		agentRegistry:      newCompatibilityRegistry(t, cloudAgent),
		secretStore:        store,
		cursorCloudEnabled: true,
		cursorCloudModels: cursorCloudModelValidatorFunc(func(_ context.Context, secretID, model string) error {
			modelChecks++
			require.Equal(t, "cursor-key", secretID)
			require.Equal(t, "model-1", model)
			return nil
		}),
	}
	profile := &settingsmodels.AgentProfile{ID: "agent-profile", AgentID: "agent-row", Model: "model-1"}
	executor := &models.Executor{ID: "cloud", Type: models.ExecutorTypeCursorCloud, Status: models.ExecutorStatusActive}
	executorProfile := &models.ExecutorProfile{Config: map[string]string{
		cursorcloud.ExecutorConfigSecretID:    "cursor-key",
		cursorcloud.ExecutorConfigCallbackURL: "https://callback.example.test",
	}}
	require.NoError(t, validator.ValidateAgentProfileForExecutor(context.Background(), profile, executor, executorProfile))
	require.Equal(t, 1, modelChecks)

	validator.cursorCloudEnabled = false
	require.Error(t, validator.ValidateAgentProfileForExecutor(context.Background(), profile, executor, executorProfile))
	validator.cursorCloudEnabled = true
	wrongExecutor := &models.Executor{ID: "local", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive}
	require.Error(t, validator.ValidateAgentProfileForExecutor(context.Background(), profile, wrongExecutor, nil))

	otherAgent := agents.NewMockAgent()
	otherAgent.SetEnabled(true)
	validator.profiles = compatibilityAgentReaderStub{agent: &settingsmodels.Agent{Name: otherAgent.ID()}}
	validator.agentRegistry = newCompatibilityRegistry(t, otherAgent)
	require.Error(t, validator.ValidateAgentProfileForExecutor(context.Background(), profile, executor, executorProfile))
}

func TestCursorCloudDisabledEntryPoints(t *testing.T) {
	cloudAgent := agents.NewCursorCloudAgent()
	validator := taskAgentExecutorCompatibilityValidator{
		profiles:           compatibilityAgentReaderStub{agent: &settingsmodels.Agent{Name: agents.CursorCloudAgentID}},
		agentRegistry:      newCompatibilityRegistry(t, cloudAgent),
		cursorCloudEnabled: false,
	}
	err := validator.ValidateAgentProfileForExecutor(context.Background(),
		&settingsmodels.AgentProfile{ID: "agent-profile", AgentID: "agent-row", Model: "model-1"},
		&models.Executor{ID: "cloud", Type: models.ExecutorTypeCursorCloud, Status: models.ExecutorStatusActive},
		&models.ExecutorProfile{Config: map[string]string{
			cursorcloud.ExecutorConfigSecretID:    "cursor-key",
			cursorcloud.ExecutorConfigCallbackURL: "https://callback.example.test",
		}},
	)
	require.Error(t, err, "disabled cloud profiles must not reach dispatch validation")
}
