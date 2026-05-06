package service

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	officeruntime "github.com/kandev/kandev/internal/office/runtime"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// DefaultTickInterval is the default run processing interval.
const DefaultTickInterval = 5 * time.Second

// staleClaimedRunAge is the age after which a claimed run is recovered
// if no agent lifecycle event returned it to a terminal queue state.
const staleClaimedRunAge = 30 * time.Minute

// TickIntervalFromEnv reads KANDEV_OFFICE_SCHEDULER_TICK_MS and returns the
// corresponding duration. Falls back to DefaultTickInterval when the variable
// is unset or invalid.
func TickIntervalFromEnv() time.Duration {
	raw := os.Getenv("KANDEV_OFFICE_SCHEDULER_TICK_MS")
	if raw == "" {
		return DefaultTickInterval
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return DefaultTickInterval
	}
	return time.Duration(ms) * time.Millisecond
}

// SchedulerIntegration runs the run processing tick loop.
// Each tick claims the next eligible run, validates guards,
// resolves executor config, builds the prompt, and marks the
// run finished. Agent launch is not yet wired.
// TaskContextProvider supplies the office task-handoffs prompt context
// (related tasks, available document keys, workspace group). Optional —
// when nil the prompt builder omits the handoff section. The
// HandoffService in task/service satisfies this interface via
// GetTaskContext.
type TaskContextProvider interface {
	GetTaskContext(ctx context.Context, taskID string) (*v1.TaskContext, error)
}

type SchedulerIntegration struct {
	svc          *Service
	tickInterval time.Duration
	logger       *logger.Logger
	// taskContexts feeds PromptContext.HandoffContext on every run.
	// nil-safe: when unconfigured, the handoff section is omitted.
	taskContexts TaskContextProvider
}

// SetTaskContextProvider wires the office task-handoffs prompt
// enrichment hook. Called by cmd/kandev after the HandoffService is
// constructed.
func (si *SchedulerIntegration) SetTaskContextProvider(p TaskContextProvider) {
	si.taskContexts = p
}

// NewSchedulerIntegration creates a new SchedulerIntegration.
func NewSchedulerIntegration(svc *Service, tickInterval time.Duration) *SchedulerIntegration {
	if tickInterval <= 0 {
		tickInterval = DefaultTickInterval
	}
	return &SchedulerIntegration{
		svc:          svc,
		tickInterval: tickInterval,
		logger:       svc.logger.WithFields(zap.String("component", "office-scheduler")),
	}
}

// Start runs the tick loop until the context is cancelled.
// It should be called in a background goroutine.
func (si *SchedulerIntegration) Start(ctx context.Context) {
	si.logger.Info("office scheduler starting",
		zap.Duration("tick_interval", si.tickInterval))

	ticker := time.NewTicker(si.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			si.logger.Info("office scheduler stopping")
			return
		case <-ticker.C:
			si.tick(ctx)
		}
	}
}

// maxRunsPerTick is the maximum number of runs drained per tick.
const maxRunsPerTick = 10

// Tick implements the runs/scheduler.RunProcessor interface so the
// new runs scheduler (internal/runs/scheduler) can drive this
// processor on both periodic ticks and event-driven signals (B3.5).
// It forwards to the unexported tick implementation that already
// existed for the in-package Start loop.
func (si *SchedulerIntegration) Tick(ctx context.Context) { si.tick(ctx) }

// tick drains up to maxRunsPerTick runs from the queue.
func (si *SchedulerIntegration) tick(ctx context.Context) {
	si.liftParkedRoutingRuns(ctx)
	for i := 0; i < maxRunsPerTick; i++ {
		run, err := si.svc.ClaimNextRun(ctx)
		if err != nil {
			si.logger.Error("failed to claim run", zap.Error(err))
			return
		}
		if run == nil {
			break
		}

		si.processRun(ctx, run)
	}
	si.recoverUnstartedTasks(ctx)
	si.recoverStaleClaimedRuns(ctx)
}

// liftParkedRoutingRuns clears routing-block status on runs whose
// earliest_retry_at has passed so subsequent claim passes can re-dispatch
// them through the routing path. No-op when no dispatcher is wired.
func (si *SchedulerIntegration) liftParkedRoutingRuns(ctx context.Context) {
	type lifter interface {
		LiftParkedRuns(ctx context.Context, now time.Time) (int, error)
	}
	rd, ok := si.svc.routingDispatcher.(lifter)
	if !ok {
		return
	}
	lifted, err := rd.LiftParkedRuns(ctx, time.Now().UTC())
	if err != nil {
		si.logger.Warn("lift parked runs failed", zap.Error(err))
		return
	}
	if lifted > 0 {
		si.logger.Info("lifted parked routing runs", zap.Int("count", lifted))
	}
}

func (si *SchedulerIntegration) recoverStaleClaimedRuns(ctx context.Context) {
	count, err := si.svc.repo.RecoverStale(ctx, time.Now().UTC().Add(-staleClaimedRunAge))
	if err != nil {
		si.logger.Error("failed to recover stale claimed runs", zap.Error(err))
		return
	}
	if count > 0 {
		si.logger.Info("recovered stale claimed runs", zap.Int64("count", count))
	}
}

// processRun runs guard checks, checkout, budget check, resolves executor,
// builds prompt, logs the result, and marks the run finished.
func (si *SchedulerIntegration) processRun(ctx context.Context, run *models.Run) {
	runID := run.ID
	agentInstanceID := run.AgentProfileID

	// Lifecycle: run picked up by the scheduler. Drives the first
	// row in the run detail page's Events log.
	si.svc.AppendRunEvent(ctx, runID, "init", "info", map[string]interface{}{
		"agent_profile_id": agentInstanceID,
		"reason":           run.Reason,
	})

	// Guard: check agent status.
	agent, err := si.svc.GetAgentFromConfig(ctx, agentInstanceID)
	if err != nil {
		si.logger.Error("failed to get agent instance",
			zap.String("run_id", runID), zap.Error(err))
		_ = si.svc.HandleRunFailure(ctx, run, err)
		return
	}

	if !isAgentActive(agent.Status) {
		si.logger.Info("run skipped (agent not active)",
			zap.String("run_id", runID),
			zap.String("agent_status", string(agent.Status)))
		_ = si.svc.FinishRun(ctx, runID)
		return
	}

	// Staleness check.
	if cancel, reason := si.evaluateRunStaleness(ctx, run); cancel {
		si.cancelStaleRun(ctx, run, agent, reason)
		return
	}

	// Idle skip: heartbeat with no actionable tasks.
	if si.checkIdleSkip(ctx, run, agent) {
		si.logger.Info("run skipped (no actionable tasks)",
			zap.String("run_id", runID),
			zap.String("agent", agent.Name))
		si.svc.LogActivityWithRun(ctx, agent.WorkspaceID,
			"scheduler", "office-scheduler",
			"run_idle_skipped", "run", runID,
			mustJSON(map[string]string{
				"agent":    agent.Name,
				"agent_id": agent.ID,
			}), runID, "")
		_ = si.svc.FinishRun(ctx, runID)
		return
	}

	// Atomic task checkout.
	taskID := si.extractTaskID(run.Payload)
	if !si.checkoutTask(ctx, runID, taskID, agentInstanceID) {
		return
	}

	// Pre-execution budget check.
	if !si.checkBudget(ctx, run, agent, taskID) {
		return
	}

	// Resolve executor config (needed before delivery to choose strategy).
	execCfg, err := si.resolveExecutorForRun(ctx, agent, run.Payload)
	if err != nil {
		si.logger.Warn("executor resolution failed; retrying run",
			zap.String("run_id", runID), zap.Error(err))
		si.releaseCheckoutIfNeeded(ctx, taskID)
		_ = si.svc.HandleRunFailure(ctx, run, err)
		return
	}

	si.prepareAndLaunch(ctx, run, agent, taskID, execCfg)
}

// prepareAndLaunch builds the skill manifest, env vars, prompt, and launches the
// agent. Extracted from processRun to keep it within funlen limits.
func (si *SchedulerIntegration) prepareAndLaunch(
	ctx context.Context, run *models.Run,
	agent *models.AgentInstance, taskID string, execCfg *ExecutorConfig,
) {
	// ADR 0005 Wave E: skill + instruction file delivery moved into the
	// runtime (internal/agent/runtime/lifecycle/skill). We still build
	// the manifest here to extract AGENTS.md content for the prompt and
	// to compute the deterministic instructionsDir path the runtime
	// will write to. No filesystem side effects from this call.
	manifest := si.buildSkillManifest(ctx, agent, defaultWorkspaceName)
	instructionsDir, agentsMD := si.resolveInstructionsForPrompt(manifest, execCfg.Type)
	si.snapshotRunSkills(ctx, run.ID, manifest, instructionsDir)

	runCtx, err := (&officeruntime.ContextBuilder{
		Agents: si.svc,
		Runs:   si.svc.repo,
	}).BuildAndPersist(ctx, run)
	if err != nil {
		si.logger.Warn("runtime context build failed; retrying run",
			zap.String("run_id", run.ID), zap.Error(err))
		si.releaseCheckoutIfNeeded(ctx, taskID)
		_ = si.svc.HandleRunFailure(ctx, run, err)
		return
	}
	si.svc.AppendRunEvent(ctx, run.ID, "runtime.context", "info", map[string]interface{}{
		"agent_id":     runCtx.AgentID,
		"task_id":      runCtx.TaskID,
		"capabilities": run.Capabilities,
		"session_id":   runCtx.SessionID,
		"workspace_id": runCtx.WorkspaceID,
		"wake_reason":  runCtx.Reason,
	})

	token, err := si.mintRuntimeToken(run, agent, runCtx)
	if err != nil {
		si.logger.Warn("runtime token mint failed; retrying run",
			zap.String("run_id", run.ID), zap.Error(err))
		si.releaseCheckoutIfNeeded(ctx, taskID)
		_ = si.svc.HandleRunFailure(ctx, run, err)
		return
	}
	env := si.buildEnvVars(run, agent, token, agent.WorkspaceID)
	si.injectKandevCLI(env, execCfg.Type)

	if payload, pErr := si.svc.BuildWakePayload(ctx, &RunPayloadInput{
		Payload: run.Payload,
	}); pErr == nil && payload != "" {
		env["KANDEV_WAKE_PAYLOAD_JSON"] = payload
	}

	prompt := si.assembleAgentPrompt(ctx, run, agent, taskID, runCtx, instructionsDir, agentsMD)
	profileID := si.resolveProfileForRun(ctx, run.Reason, taskID, agent)

	si.logger.Debug("session env vars prepared",
		zap.String("run_id", run.ID),
		zap.Int("env_count", len(env)))

	launchCtx := LaunchContext{
		Prompt:    prompt,
		Env:       env,
		ProfileID: profileID,
	}
	if !si.launchOrLog(ctx, run, agent, taskID, execCfg.Type, launchCtx) {
		return
	}
	// When a real task starter launched the agent, leave the run
	// `claimed` and let the AgentCompleted/AgentStopped event subscribers
	// in event_subscribers.go finish it. This serves as the "agent is
	// busy on this task" lock that ClaimNextEligibleRun respects, so
	// new runs (comments, status changes) for the same agent + task
	// queue up rather than racing the active turn.
	if taskID != "" && si.svc.taskStarter != nil {
		return
	}

	si.finishRun(ctx, run, agent, taskID)
}

// assembleAgentPrompt builds the wake-context prompt, decides whether the
// session is a resume, loads any continuation summary, runs BuildAgentPrompt,
// and persists prompt artifacts. Returns the rendered prompt ready for launch.
// Extracted from prepareAndLaunch to keep that function within funlen limits.
func (si *SchedulerIntegration) assembleAgentPrompt(
	ctx context.Context,
	run *models.Run,
	agent *models.AgentInstance,
	taskID string,
	runCtx officeruntime.RunContext,
	instructionsDir, agentsMD string,
) string {
	pc := si.buildPromptContext(ctx, run.Reason, run.Payload)
	pc.RunID = runCtx.RunID
	pc.AgentID = runCtx.AgentID
	pc.SessionID = runCtx.SessionID
	pc.TaskScope = append([]string(nil), runCtx.Capabilities.AllowedTaskIDs...)
	pc.AllowedActions = runCtx.Capabilities.AllowedKeys()
	wakeContext := BuildPrompt(pc)

	// Resume = the (task, agent_instance) session has run before. On resume
	// the agent CLI's --resume restores the prior conversation (which already
	// contains the role prompt), so we skip re-sending AGENTS.md.
	isResume := false
	if taskID != "" {
		if has, hErr := si.svc.repo.HasPriorSessionForAgent(ctx, taskID, agent.ID); hErr == nil {
			isResume = has
		}
	}

	continuationSummary := si.loadContinuationSummary(ctx, agent.ID, taskID)
	promptResult := si.svc.BuildAgentPrompt(
		run, agent, instructionsDir, agentsMD, isResume, wakeContext,
		taskID, continuationSummary,
	)
	si.persistPromptArtifacts(ctx, run, promptResult.Prompt, promptResult.SummaryInjected)
	return promptResult.Prompt
}

// loadContinuationSummary fetches the per-(agent, scope) continuation
// summary for a taskless run. Returns "" when:
// - taskID is non-empty (task-bound runs don't use the summary doc)
// - the summary table has no row yet (first heartbeat ever for the agent)
// - the lookup errors out (best-effort, fall back to no-summary)
//
// Today no caller passes taskID==""; this branch is a no-op until
// PR 2 lands the agent_heartbeat cron handler.
func (si *SchedulerIntegration) loadContinuationSummary(
	ctx context.Context, agentID, taskID string,
) string {
	if taskID != "" || agentID == "" {
		return ""
	}
	prior, err := si.svc.repo.GetContinuationSummary(ctx, agentID, "heartbeat")
	if err != nil || prior == nil {
		return ""
	}
	return prior.Content
}

// persistPromptArtifacts stores the assembled prompt and the summary
// snapshot onto the run row so the run-detail UI can render exactly
// what the agent saw. Errors are logged at warn — the run still
// proceeds because the artifact persistence is purely diagnostic.
func (si *SchedulerIntegration) persistPromptArtifacts(
	ctx context.Context, run *models.Run, prompt, summaryInjected string,
) {
	if run == nil || run.ID == "" {
		return
	}
	run.AssembledPrompt = prompt
	run.SummaryInjected = summaryInjected
	if err := si.svc.repo.UpdateRunPromptArtifacts(ctx, run.ID, prompt, summaryInjected); err != nil {
		si.logger.Warn("failed to persist run prompt artifacts",
			zap.String("run_id", run.ID), zap.Error(err))
	}
}

func (si *SchedulerIntegration) snapshotRunSkills(ctx context.Context, runID string, manifest *SkillManifest, instructionsDir string) {
	if manifest == nil || len(manifest.Skills) == 0 {
		return
	}
	snapshots := make([]models.RunSkillSnapshot, 0, len(manifest.Skills))
	for _, skill := range manifest.Skills {
		snapshots = append(snapshots, models.RunSkillSnapshot{
			RunID:            runID,
			SkillID:          skill.ID,
			Version:          skill.Version,
			ContentHash:      skill.ContentHash,
			MaterializedPath: instructionsDir,
		})
	}
	if err := si.svc.repo.CreateRunSkillSnapshots(ctx, snapshots); err != nil {
		si.logger.Warn("failed to snapshot run skills", zap.String("run_id", runID), zap.Error(err))
	}
}

// resolveProfileForRun returns the agent profile id to launch with.
// Provider routing is the single seam for cheap/expensive variants now
// (see internal/office/routing's TierPerReason). The legacy
// cheap_agent_profile_id mechanism was removed in the wake-reason tier
// policy patch — every run uses agent.ID, and the resolver picks the
// concrete provider/model based on wake reason + workspace policy.
func (si *SchedulerIntegration) resolveProfileForRun(_ context.Context, _, _ string, agent *models.AgentInstance) string {
	return agent.ID
}

func (si *SchedulerIntegration) mintRuntimeToken(
	run *models.Run,
	agent *models.AgentInstance,
	runCtx officeruntime.RunContext,
) (string, error) {
	if si.svc.agentTokenMinter == nil {
		return "", nil
	}
	return si.svc.agentTokenMinter.MintRuntimeJWT(
		agent.ID,
		runCtx.TaskID,
		agent.WorkspaceID,
		run.ID,
		runCtx.SessionID,
		run.Capabilities,
	)
}

// checkoutTask performs the atomic task checkout guard. Returns false if the
// caller should abort processing (tree-gated or checkout failed).
func (si *SchedulerIntegration) checkoutTask(ctx context.Context, runID, taskID, agentInstanceID string) bool {
	if taskID == "" {
		return true
	}
	if si.isTaskTreeGated(ctx, runID, taskID) {
		return false
	}
	return si.tryCheckout(ctx, runID, taskID, agentInstanceID)
}

func (si *SchedulerIntegration) isTaskTreeGated(ctx context.Context, runID, taskID string) bool {
	hold, err := si.svc.repo.GetActiveHoldForMember(ctx, taskID)
	if err != nil || hold == nil {
		if err != nil {
			si.logger.Warn("tree hold gate check failed",
				zap.String("run_id", runID),
				zap.String("task_id", taskID),
				zap.Error(err))
		}
		return false
	}
	si.logger.Info("run gated by active task tree hold",
		zap.String("run_id", runID),
		zap.String("task_id", taskID),
		zap.String("hold_id", hold.ID),
		zap.String("mode", hold.Mode))
	_ = si.svc.FinishRun(ctx, runID)
	return true
}

// launchOrLog starts the agent via the orchestrator or logs the run when
// no task starter is configured. Returns false if the launch failed and the
// caller should abort (the failure is already handled).
func (si *SchedulerIntegration) launchOrLog(
	ctx context.Context, run *models.Run,
	agent *models.AgentInstance, taskID, executorType string,
	launch LaunchContext,
) bool {
	runID := run.ID

	if taskID == "" || si.svc.taskStarter == nil {
		si.logger.Info("processing run (no task starter or task ID)",
			zap.String("run_id", runID),
			zap.String("agent", agent.Name),
			zap.String("reason", run.Reason),
			zap.String("executor_type", executorType),
			zap.Int("prompt_len", len(launch.Prompt)),
		)
		return true
	}

	si.logger.Info("launching agent for run",
		zap.String("run_id", runID),
		zap.String("agent", agent.Name),
		zap.String("task_id", taskID),
		zap.String("executor_type", executorType),
		zap.Int("prompt_len", len(launch.Prompt)),
	)
	// Lifecycle: orchestrator handed the prompt to the adapter. Pin
	// model + executor on the event so the run detail Events log can
	// render them inline.
	si.svc.AppendRunEvent(ctx, runID, "adapter.invoke", "info", map[string]interface{}{
		"agent":         agent.Name,
		"task_id":       taskID,
		"profile_id":    launch.ProfileID,
		"executor_type": executorType,
		"prompt_len":    len(launch.Prompt),
	})
	if si.tryRoutingDispatch(ctx, run, agent, taskID, launch) {
		return true
	}
	var err error
	if starter, ok := si.svc.taskStarter.(TaskStarterWithEnv); ok {
		err = starter.StartTaskWithEnv(ctx, taskID, launch.ProfileID, "", "", "",
			launch.Prompt, "", false, nil, launch.Env)
	} else {
		err = si.svc.taskStarter.StartTask(ctx, taskID, launch.ProfileID, "", "", "",
			launch.Prompt, "", false, nil)
	}
	if err != nil {
		si.logger.Error("agent launch failed",
			zap.String("run_id", runID), zap.Error(err))
		si.svc.AppendRunEvent(ctx, runID, "error", "error", map[string]interface{}{
			"phase":         "adapter.invoke",
			"error_message": err.Error(),
		})
		si.releaseCheckoutIfNeeded(ctx, taskID)
		_ = si.svc.HandleRunFailure(ctx, run, err)
		return false
	}
	return true
}

// tryRoutingDispatch routes through the provider-routing dispatcher when
// one is wired. Returns true when the dispatcher took over (launched OR
// parked OR error-handled); false to fall through to the legacy launch
// path (routing disabled / not configured).
func (si *SchedulerIntegration) tryRoutingDispatch(
	ctx context.Context, run *models.Run, agent *models.AgentInstance,
	taskID string, launch LaunchContext,
) bool {
	rd := si.svc.routingDispatcher
	if rd == nil {
		return false
	}
	launched, parked, err := rd.DispatchWithRouting(ctx, run, agent, launch)
	if err != nil {
		si.logger.Error("routing dispatch failed",
			zap.String("run_id", run.ID), zap.Error(err))
		si.svc.AppendRunEvent(ctx, run.ID, "error", "error", map[string]interface{}{
			"phase":         "routing.dispatch",
			"error_message": err.Error(),
		})
		si.releaseCheckoutIfNeeded(ctx, taskID)
		_ = si.svc.HandleRunFailure(ctx, run, err)
		return true
	}
	if launched {
		return true
	}
	if parked {
		si.releaseCheckoutIfNeeded(ctx, taskID)
		return true
	}
	return false
}

// tryCheckout attempts to acquire an exclusive lock on the task. Returns true
// if the checkout succeeded or was not needed, false if blocked.
func (si *SchedulerIntegration) tryCheckout(
	ctx context.Context, runID, taskID, agentID string,
) bool {
	acquired, err := si.svc.repo.CheckoutTask(ctx, taskID, agentID)
	if err != nil {
		si.logger.Error("task checkout error",
			zap.String("run_id", runID), zap.Error(err))
		_ = si.svc.FinishRun(ctx, runID)
		return false
	}
	if !acquired {
		si.logger.Info("run skipped (task checked out by another agent)",
			zap.String("run_id", runID),
			zap.String("task_id", taskID))
		_ = si.svc.FinishRun(ctx, runID)
		return false
	}
	return true
}

// checkBudget runs pre-execution budget checks. Returns true if allowed.
func (si *SchedulerIntegration) checkBudget(
	ctx context.Context, run *models.Run,
	agent *models.AgentInstance, taskID string,
) bool {
	projectID := si.extractProjectID(ctx, run.Payload)
	allowed, reason, err := si.svc.CheckPreExecutionBudget(
		ctx, agent.ID, projectID, agent.WorkspaceID)
	if err != nil {
		si.logger.Error("budget check failed",
			zap.String("run_id", run.ID), zap.Error(err))
		return true // fail-open on error
	}
	if !allowed {
		si.logger.Info("run skipped (budget exceeded)",
			zap.String("run_id", run.ID), zap.String("reason", reason))
		si.releaseCheckoutIfNeeded(ctx, taskID)
		_ = si.svc.FinishRun(ctx, run.ID)
		si.svc.LogActivityWithRun(ctx, agent.WorkspaceID,
			"scheduler", "office-scheduler",
			"run_budget_blocked", "run", run.ID,
			mustJSON(map[string]string{
				"agent":    agent.Name,
				"agent_id": agent.ID,
				"reason":   reason,
			}), run.ID, "")
		return false
	}
	return true
}

// finishRun marks the run as finished, releases checkout, records
// cooldown timestamp, and logs activity.
func (si *SchedulerIntegration) finishRun(
	ctx context.Context, run *models.Run,
	agent *models.AgentInstance, taskID string,
) {
	if err := si.svc.FinishRun(ctx, run.ID); err != nil {
		si.logger.Error("failed to finish run",
			zap.String("run_id", run.ID), zap.Error(err))
		return
	}

	si.releaseCheckoutIfNeeded(ctx, taskID)

	// Record cooldown timestamp in-memory and DB.
	now := time.Now().UTC()
	agent.LastRunFinishedAt = &now
	_ = si.svc.repo.UpdateRuntimeLastRunFinished(ctx, agent.ID, now)

	si.svc.LogActivityWithRun(ctx, agent.WorkspaceID,
		"scheduler", "office-scheduler",
		"run_processed", "run", run.ID,
		mustJSON(map[string]string{
			"agent":    agent.Name,
			"agent_id": agent.ID,
			"reason":   run.Reason,
		}), run.ID, "")
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

// extractTaskID parses the task_id from a run payload.
func (si *SchedulerIntegration) extractTaskID(payload string) string {
	return ParseRunPayload(payload)["task_id"]
}

// extractProjectID looks up the project ID for a task in the payload.
func (si *SchedulerIntegration) extractProjectID(ctx context.Context, payload string) string {
	taskID := ParseRunPayload(payload)["task_id"]
	if taskID == "" {
		return ""
	}
	info, err := si.svc.repo.GetTaskBasicInfo(ctx, taskID)
	if err != nil || info == nil {
		return ""
	}
	return info.ProjectID
}

// isAgentActive returns true if the agent status allows processing runs.
func isAgentActive(status models.AgentStatus) bool {
	return status == models.AgentStatusIdle || status == models.AgentStatusWorking
}

// checkIdleSkip returns true if the run should be skipped because the agent
// is configured to skip idle heartbeats and has no actionable tasks assigned.
// Returns false (do not skip) on any DB error to fail open.
func (si *SchedulerIntegration) checkIdleSkip(
	ctx context.Context, run *models.Run, agent *models.AgentInstance,
) bool {
	if run.Reason != RunReasonHeartbeat {
		return false
	}
	if !agent.SkipIdleRuns {
		return false
	}
	count, err := si.svc.repo.CountActionableTasksForAgent(ctx, agent.ID)
	if err != nil {
		si.logger.Warn("idle skip check failed; proceeding",
			zap.String("run_id", run.ID), zap.Error(err))
		return false // fail open
	}
	return count == 0
}

// resolveExecutorForRun resolves the executor config for a run.
// Priority: agent preference -> project config -> fallback. The legacy
// task-level execution_policy override was retired in Phase 4 of
// task-model-unification.
func (si *SchedulerIntegration) resolveExecutorForRun(
	ctx context.Context, agent *models.AgentInstance, _ string,
) (*ExecutorConfig, error) {
	return si.svc.ResolveExecutor(ctx, "", agent.ID, "", "")
}

// buildPromptContext assembles a PromptContext from run data.
func (si *SchedulerIntegration) buildPromptContext(
	ctx context.Context, reason, payload string,
) *PromptContext {
	parsed := ParseRunPayload(payload)
	pc := &PromptContext{Reason: reason}

	if taskID := parsed["task_id"]; taskID != "" {
		si.enrichTaskContext(ctx, pc, taskID)
		si.enrichHandoffContext(ctx, pc, taskID)
	}

	if reason == RunReasonApprovalResolved {
		pc.ApprovalStatus = parsed["status"]
		pc.ApprovalNote = parsed["decision_note"]
	}

	if reason == RunReasonTaskChildrenCompleted {
		si.enrichChildrenContext(pc, payload)
	}

	if reason == RunReasonTaskAssigned {
		pc.StageID = parsed["stage_id"]
		pc.StageType = parsed["stage_type"]
		pc.ReviewFeedback = parsed["feedback"]

		if pc.StageType == "review" && pc.TaskID != "" {
			si.enrichBuilderComments(ctx, pc)
		}
	}

	if reason == RunReasonTaskComment {
		si.enrichCommentContext(ctx, pc, parsed["comment_id"])
	}

	return pc
}

// enrichHandoffContext populates pc.HandoffContext from the office
// task-handoffs context API so the run prompt's BuildPrompt can render
// the Related-tasks / Documents-available / Workspace section.
// Failures are logged at debug — the prompt still runs, just without
// the handoff section.
func (si *SchedulerIntegration) enrichHandoffContext(
	ctx context.Context, pc *PromptContext, taskID string,
) {
	if si.taskContexts == nil {
		return
	}
	hc, err := si.taskContexts.GetTaskContext(ctx, taskID)
	if err != nil {
		si.logger.Debug("load handoff context for prompt failed",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	pc.HandoffContext = hc
}

// enrichCommentContext loads the triggering comment and populates the
// comment-specific PromptContext fields. Failures are logged at debug and
// leave the fields empty rather than failing the wakeup pipeline.
func (si *SchedulerIntegration) enrichCommentContext(
	ctx context.Context, pc *PromptContext, commentID string,
) {
	if commentID == "" {
		return
	}
	comment, err := si.svc.repo.GetTaskComment(ctx, commentID)
	if err != nil || comment == nil {
		si.logger.Debug("load comment for prompt context failed",
			zap.String("comment_id", commentID), zap.Error(err))
		return
	}
	pc.CommentBody = comment.Body
	pc.CommentAuthorType = comment.AuthorType
	pc.CommentAuthor = si.resolveCommentAuthor(ctx, comment)
}

// resolveCommentAuthor returns a display label for the comment author.
// Agents are looked up via GetAgentFromConfig; users get a generic "User"
// label since the office service does not currently track user names.
func (si *SchedulerIntegration) resolveCommentAuthor(
	ctx context.Context, comment *models.TaskComment,
) string {
	if comment.AuthorType == "agent" && comment.AuthorID != "" {
		agent, err := si.svc.GetAgentFromConfig(ctx, comment.AuthorID)
		if err == nil && agent != nil && agent.Name != "" {
			return agent.Name
		}
		return "Agent"
	}
	return "User"
}

// enrichBuilderComments fetches the most recent comments left by the task
// assignee (builder) and appends them to pc.BuilderComments.
func (si *SchedulerIntegration) enrichBuilderComments(ctx context.Context, pc *PromptContext) {
	comments, err := si.svc.repo.ListRecentTaskComments(ctx, pc.TaskID, 5)
	if err != nil {
		return
	}
	for _, c := range comments {
		if c.Body != "" {
			pc.BuilderComments = append(pc.BuilderComments, c.Body)
		}
	}
}

// enrichChildrenContext parses child summaries from the run payload.
func (si *SchedulerIntegration) enrichChildrenContext(pc *PromptContext, payload string) {
	var data struct {
		Children []struct {
			Identifier  string `json:"identifier"`
			Title       string `json:"title"`
			State       string `json:"state"`
			LastComment string `json:"last_comment"`
		} `json:"children"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return
	}
	for _, c := range data.Children {
		pc.ChildSummaries = append(pc.ChildSummaries, ChildSummaryPrompt{
			Identifier:  c.Identifier,
			Title:       c.Title,
			State:       c.State,
			LastComment: c.LastComment,
		})
	}
	pc.ChildSummariesTruncated = data.Truncated
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
		project, projErr := si.svc.GetProjectFromConfig(ctx, info.ProjectID)
		if projErr == nil && project != nil {
			pc.ProjectName = project.Name
		}
	}
}
