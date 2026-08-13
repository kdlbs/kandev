package backendapp

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/common/logger"
	orchexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/plugins"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// pluginStarterService is the narrow slice of the orchestrator the start
// adapter needs, so its fire-and-forget launch is unit-testable with a fake.
// *orchestrator.Service satisfies it.
type pluginStarterService interface {
	StartTask(ctx context.Context, taskID, agentProfileID, executorID, executorProfileID, priority, prompt, workflowStepID string, planMode, autoStart bool, attachments []v1.MessageAttachment) (*orchexecutor.TaskExecution, error)
}

// pluginsTaskStarterAdapter adapts the orchestrator to the plugins package's
// taskStarter interface (Host data API CreateTask start_agent flag, ADR 0043
// phase 2). Plugin-selected agent/executor profiles, prompt, and plan mode
// flow to the orchestrator; empty workflow-step and executor IDs retain the
// normal default-resolving launch path used by the office scheduler.
//
// StartTask returns immediately: the launch runs in a fire-and-forget goroutine
// on a detached, time-bounded context (matching the MCP launchAutoStartTask
// pattern), so a plugin's CreateTask RPC never blocks on the orchestrator's
// substantial inline start work, and the plugin's request deadline can't abort
// a launch of a task that already exists. Best-effort — a launch error is
// logged, never surfaced.
type pluginsTaskStarterAdapter struct {
	orch pluginStarterService
	log  *logger.Logger
}

func (a pluginsTaskStarterAdapter) StartTask(ctx context.Context, taskID string, launch plugins.TaskLaunchInput) error {
	launchCtx := context.WithoutCancel(ctx)
	go func() {
		startCtx, cancel := context.WithTimeout(launchCtx, constants.AgentLaunchTimeout)
		defer cancel()
		if _, err := a.orch.StartTask(
			startCtx, taskID, launch.AgentProfileID, "", launch.ExecutorProfileID, "", launch.Prompt, "", launch.PlanMode, false, nil,
		); err != nil {
			a.log.Warn("plugins: best-effort start_agent failed", zap.String("task_id", taskID), zap.Error(err))
		}
	}()
	return nil
}
