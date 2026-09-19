package backendapp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
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
