package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrate/models"
)

// DefaultTickInterval is the default wakeup processing interval.
const DefaultTickInterval = 5 * time.Second

// SchedulerIntegration runs the wakeup processing tick loop.
// Each tick claims the next eligible wakeup, validates guards,
// resolves executor config, builds the prompt, and marks the
// wakeup finished. Agent launch is not yet wired.
type SchedulerIntegration struct {
	svc          *Service
	tickInterval time.Duration
	logger       *logger.Logger
}

// NewSchedulerIntegration creates a new SchedulerIntegration.
func NewSchedulerIntegration(svc *Service, tickInterval time.Duration) *SchedulerIntegration {
	if tickInterval <= 0 {
		tickInterval = DefaultTickInterval
	}
	return &SchedulerIntegration{
		svc:          svc,
		tickInterval: tickInterval,
		logger:       svc.logger.WithFields(zap.String("component", "orchestrate-scheduler")),
	}
}

// Start runs the tick loop until the context is cancelled.
// It should be called in a background goroutine.
func (si *SchedulerIntegration) Start(ctx context.Context) {
	si.logger.Info("orchestrate scheduler starting",
		zap.Duration("tick_interval", si.tickInterval))

	ticker := time.NewTicker(si.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			si.logger.Info("orchestrate scheduler stopping")
			return
		case <-ticker.C:
			si.tick(ctx)
		}
	}
}

// tick processes one wakeup from the queue.
func (si *SchedulerIntegration) tick(ctx context.Context) {
	wakeup, err := si.svc.ClaimNextWakeup(ctx)
	if err != nil {
		si.logger.Error("failed to claim wakeup", zap.Error(err))
		return
	}
	if wakeup == nil {
		return
	}

	si.processWakeup(ctx, wakeup)
}

// processWakeup runs guard checks, checkout, budget check, resolves executor,
// builds prompt, logs the result, and marks the wakeup finished.
func (si *SchedulerIntegration) processWakeup(ctx context.Context, wakeup *models.WakeupRequest) {
	wakeupID := wakeup.ID
	agentInstanceID := wakeup.AgentInstanceID

	// Guard: check agent status.
	agent, err := si.svc.repo.GetAgentInstance(ctx, agentInstanceID)
	if err != nil {
		si.logger.Error("failed to get agent instance",
			zap.String("wakeup_id", wakeupID), zap.Error(err))
		_ = si.svc.HandleWakeupFailure(ctx, wakeup, err)
		return
	}

	if !isAgentActive(agent.Status) {
		si.logger.Info("wakeup skipped (agent not active)",
			zap.String("wakeup_id", wakeupID),
			zap.String("agent_status", string(agent.Status)))
		_ = si.svc.FinishWakeup(ctx, wakeupID)
		return
	}

	// Atomic task checkout.
	taskID := si.extractTaskID(wakeup.Payload)
	if taskID != "" {
		if !si.tryCheckout(ctx, wakeupID, taskID, agentInstanceID) {
			return
		}
	}

	// Pre-execution budget check.
	if !si.checkBudget(ctx, wakeup, agent, taskID) {
		return
	}

	// Resolve executor config.
	execCfg, err := si.resolveExecutorForWakeup(ctx, agent, wakeup.Payload)
	if err != nil {
		si.logger.Warn("executor resolution failed; retrying wakeup",
			zap.String("wakeup_id", wakeupID), zap.Error(err))
		si.releaseCheckoutIfNeeded(ctx, taskID)
		_ = si.svc.HandleWakeupFailure(ctx, wakeup, err)
		return
	}

	// Build prompt.
	pc := si.buildPromptContext(ctx, wakeup.Reason, wakeup.Payload)
	prompt := BuildPrompt(pc)

	si.logger.Info("processing wakeup for agent (dry run)",
		zap.String("wakeup_id", wakeupID),
		zap.String("agent", agent.Name),
		zap.String("reason", wakeup.Reason),
		zap.String("executor_type", execCfg.Type),
		zap.Int("prompt_len", len(prompt)),
	)

	si.finishWakeup(ctx, wakeup, agent, taskID)
}

// tryCheckout attempts to acquire an exclusive lock on the task. Returns true
// if the checkout succeeded or was not needed, false if blocked.
func (si *SchedulerIntegration) tryCheckout(
	ctx context.Context, wakeupID, taskID, agentID string,
) bool {
	acquired, err := si.svc.repo.CheckoutTask(ctx, taskID, agentID)
	if err != nil {
		si.logger.Error("task checkout error",
			zap.String("wakeup_id", wakeupID), zap.Error(err))
		_ = si.svc.FinishWakeup(ctx, wakeupID)
		return false
	}
	if !acquired {
		si.logger.Info("wakeup skipped (task checked out by another agent)",
			zap.String("wakeup_id", wakeupID),
			zap.String("task_id", taskID))
		_ = si.svc.FinishWakeup(ctx, wakeupID)
		return false
	}
	return true
}

// checkBudget runs pre-execution budget checks. Returns true if allowed.
func (si *SchedulerIntegration) checkBudget(
	ctx context.Context, wakeup *models.WakeupRequest,
	agent *models.AgentInstance, taskID string,
) bool {
	projectID := si.extractProjectID(ctx, wakeup.Payload)
	allowed, reason, err := si.svc.CheckPreExecutionBudget(
		ctx, agent.ID, projectID, agent.WorkspaceID)
	if err != nil {
		si.logger.Error("budget check failed",
			zap.String("wakeup_id", wakeup.ID), zap.Error(err))
		return true // fail-open on error
	}
	if !allowed {
		si.logger.Info("wakeup skipped (budget exceeded)",
			zap.String("wakeup_id", wakeup.ID), zap.String("reason", reason))
		si.releaseCheckoutIfNeeded(ctx, taskID)
		_ = si.svc.FinishWakeup(ctx, wakeup.ID)
		si.svc.LogActivity(ctx, agent.WorkspaceID,
			"scheduler", "orchestrate-scheduler",
			"wakeup_budget_blocked", "wakeup", wakeup.ID,
			mustJSON(map[string]string{
				"agent":  agent.Name,
				"reason": reason,
			}))
		return false
	}
	return true
}

// finishWakeup marks the wakeup as finished, releases checkout, records
// cooldown timestamp, and logs activity.
func (si *SchedulerIntegration) finishWakeup(
	ctx context.Context, wakeup *models.WakeupRequest,
	agent *models.AgentInstance, taskID string,
) {
	if err := si.svc.FinishWakeup(ctx, wakeup.ID); err != nil {
		si.logger.Error("failed to finish wakeup",
			zap.String("wakeup_id", wakeup.ID), zap.Error(err))
		return
	}

	si.releaseCheckoutIfNeeded(ctx, taskID)

	// Record cooldown timestamp.
	now := time.Now().UTC()
	if err := si.svc.repo.UpdateLastWakeupFinished(ctx, agent.ID, now); err != nil {
		si.logger.Error("failed to update last wakeup finished",
			zap.String("agent_id", agent.ID), zap.Error(err))
	}

	si.svc.LogActivity(ctx, agent.WorkspaceID,
		"scheduler", "orchestrate-scheduler",
		"wakeup_processed", "wakeup", wakeup.ID,
		mustJSON(map[string]string{
			"agent":  agent.Name,
			"reason": wakeup.Reason,
		}))
}

// releaseCheckoutIfNeeded releases the task checkout if a task ID is present.
func (si *SchedulerIntegration) releaseCheckoutIfNeeded(ctx context.Context, taskID string) {
	if taskID == "" {
		return
	}
	if err := si.svc.repo.ReleaseTaskCheckout(ctx, taskID); err != nil {
		si.logger.Error("failed to release task checkout",
			zap.String("task_id", taskID), zap.Error(err))
	}
}

// extractTaskID parses the task_id from a wakeup payload.
func (si *SchedulerIntegration) extractTaskID(payload string) string {
	return ParseWakeupPayload(payload)["task_id"]
}

// extractProjectID looks up the project ID for a task in the payload.
func (si *SchedulerIntegration) extractProjectID(ctx context.Context, payload string) string {
	taskID := ParseWakeupPayload(payload)["task_id"]
	if taskID == "" {
		return ""
	}
	info, err := si.svc.repo.GetTaskBasicInfo(ctx, taskID)
	if err != nil || info == nil {
		return ""
	}
	return info.ProjectID
}

// isAgentActive returns true if the agent status allows processing wakeups.
func isAgentActive(status models.AgentStatus) bool {
	return status == models.AgentStatusIdle || status == models.AgentStatusWorking
}

// resolveExecutorForWakeup resolves the executor config for a wakeup.
// Priority: task execution_policy -> agent preference -> project config -> fallback.
func (si *SchedulerIntegration) resolveExecutorForWakeup(
	ctx context.Context, agent *models.AgentInstance, payload string,
) (*ExecutorConfig, error) {
	parsed := ParseWakeupPayload(payload)

	taskExecPolicy := ""
	if taskID := parsed["task_id"]; taskID != "" {
		fields, err := si.svc.repo.GetTaskExecutionFields(ctx, taskID)
		if err == nil && fields != nil {
			taskExecPolicy = fields.ExecutionPolicy
		}
	}

	// Workspace default is not available in the orchestrate repo;
	// pass empty and let agent/project resolution handle it.
	return si.svc.ResolveExecutor(ctx, taskExecPolicy, agent.ID, "", "")
}

// buildPromptContext assembles a PromptContext from wakeup data.
func (si *SchedulerIntegration) buildPromptContext(
	ctx context.Context, reason, payload string,
) *PromptContext {
	parsed := ParseWakeupPayload(payload)
	pc := &PromptContext{Reason: reason}

	if taskID := parsed["task_id"]; taskID != "" {
		si.enrichTaskContext(ctx, pc, taskID)
	}

	if reason == WakeupReasonApprovalResolved {
		pc.ApprovalStatus = parsed["status"]
		pc.ApprovalNote = parsed["decision_note"]
	}

	return pc
}

// enrichTaskContext populates task-related fields on the PromptContext.
func (si *SchedulerIntegration) enrichTaskContext(
	ctx context.Context, pc *PromptContext, taskID string,
) {
	pc.TaskID = taskID
	info, err := si.svc.repo.GetTaskBasicInfo(ctx, taskID)
	if err != nil || info == nil {
		return
	}
	pc.TaskTitle = info.Title
	pc.TaskDescription = info.Description
	pc.TaskIdentifier = info.Identifier
	pc.TaskPriority = info.Priority

	if info.ProjectID != "" {
		project, projErr := si.svc.repo.GetProject(ctx, info.ProjectID)
		if projErr == nil && project != nil {
			pc.ProjectName = project.Name
		}
	}
}
