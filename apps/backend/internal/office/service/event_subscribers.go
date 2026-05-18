package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/costs"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// dispatchEngineTrigger invokes the configured WorkflowEngineDispatcher.
// After Phase 4 the engine is the only routing path for the four
// task-scoped triggers; there is no legacy fallback. When no dispatcher
// is wired (e.g. tests that don't need engine-driven runs) the trigger
// is silently dropped — the test fixture is responsible for queuing
// runs directly.
//
// ErrEngineNoSession is logged at debug and treated as a non-error: the
// task hasn't started a session yet, so there's nothing for the engine
// to evaluate. The bootstrap path (handleTaskUpdated /
// onMovedToInProgress) is what kicks off the very first session.
func (s *Service) dispatchEngineTrigger(
	ctx context.Context, taskID string, trigger engine.Trigger, payload any, opID string,
) error {
	if s.engineDispatcher == nil {
		return nil
	}
	if err := s.engineDispatcher.HandleTrigger(ctx, taskID, trigger, payload, opID); err != nil {
		if errors.Is(err, ErrEngineNoSession) {
			s.logger.Debug("engine trigger skipped: no active session",
				zap.String("task_id", taskID),
				zap.String("trigger", string(trigger)))
			return nil
		}
		return err
	}
	return nil
}

// TaskMovedData represents the payload of a task.moved event.
type TaskMovedData struct {
	TaskID                 string `json:"task_id"`
	WorkspaceID            string `json:"workspace_id"`
	FromStepID             string `json:"from_step_id"`
	ToStepID               string `json:"to_step_id"`
	ToStepName             string `json:"to_step_name"`
	FromStepName           string `json:"from_step_name"`
	AssigneeAgentProfileID string `json:"assignee_agent_profile_id"`
	ParentID               string `json:"parent_id"`
	SessionID              string `json:"session_id"`
}

// TaskUpdatedData represents the payload of a task.updated event.
type TaskUpdatedData struct {
	TaskID                 string `json:"task_id"`
	WorkspaceID            string `json:"workspace_id"`
	AssigneeAgentProfileID string `json:"assignee_agent_profile_id"`
	Title                  string `json:"title"`
}

// CommentPostedData represents a comment event payload.
type CommentPostedData struct {
	TaskID                 string `json:"task_id"`
	CommentID              string `json:"comment_id"`
	AuthorID               string `json:"author_id"`
	AuthorType             string `json:"author_type"`
	AssigneeAgentProfileID string `json:"assignee_agent_profile_id"`
}

// ApprovalResolvedData represents an approval resolved event payload.
type ApprovalResolvedData struct {
	ApprovalID                string `json:"approval_id"`
	Status                    string `json:"status"`
	DecisionNote              string `json:"decision_note"`
	RequestedByAgentProfileID string `json:"requested_by_agent_profile_id"`
	Type                      string `json:"type"`
}

// TaskStatusChangedData represents the payload of an office.task.status_changed event.
type TaskStatusChangedData struct {
	TaskID       string `json:"task_id"`
	WorkspaceID  string `json:"workspace_id"`
	NewStatus    string `json:"new_status"`
	Comment      string `json:"comment"`
	ActorAgentID string `json:"actor_agent_id"`
}

// AgentLifecycleData is the subset of agent lifecycle event data needed by office.
//
// AgentID is populated by the lifecycle manager for taskless runs
// (heartbeats, lightweight routines) so the office completion handler
// can attribute the run without a task lookup. It stays empty for the
// task-bound path that already uses TaskID. Today no caller emits
// taskless lifecycle events; the field is reserved for PR 2 of
// office-heartbeat-rework.
type AgentLifecycleData struct {
	TaskID       string `json:"task_id"`
	AgentID      string `json:"agent_id"`
	SessionID    string `json:"session_id"`
	ErrorMessage string `json:"error_message"`
}

type PromptUsageData struct {
	TaskID    string      `json:"task_id"`
	SessionID string      `json:"session_id"`
	AgentID   string      `json:"agent_id"`
	AgentType string      `json:"agent_type"`
	Model     string      `json:"model"`
	Provider  string      `json:"provider"`
	Usage     UsageTokens `json:"usage"`
}

// UsageTokens mirrors streams.PromptUsage on the wire. All counts are int64
// to match the stream type and to handle workspaces that accumulate over a
// million tokens. ProviderReportedCostSubcents is forwarded from claude-acp's
// usage_update.cost.amount (USD float * 10000); when > 0 the subscriber uses
// it directly and skips the models.dev lookup. Estimated is true when the
// adapter synthesised tokens (codex-acp cumulative-delta inference).
type UsageTokens struct {
	InputTokens                  int64 `json:"input_tokens"`
	OutputTokens                 int64 `json:"output_tokens"`
	CachedReadTokens             int64 `json:"cached_read_tokens"`
	CachedWriteTokens            int64 `json:"cached_write_tokens"`
	ThoughtTokens                int64 `json:"thought_tokens,omitempty"`
	TotalTokens                  int64 `json:"total_tokens,omitempty"`
	ProviderReportedCostSubcents int64 `json:"provider_reported_cost_subcents,omitempty"`
	Estimated                    bool  `json:"estimated,omitempty"`
}

// RegisterEventSubscribers subscribes to system events and queues runs.
// High-frequency global events (AgentCompleted, AgentFailed, PromptUsage)
// run in goroutines to avoid blocking the synchronous event bus publisher.
// Call SetSyncHandlers(true) before this method in tests that need
// deterministic handler completion.
func (s *Service) RegisterEventSubscribers(eb bus.EventBus) error {
	s.eb = eb

	// maybeAsync wraps the handler in a goroutine unless syncHandlers is set.
	maybeAsync := func(h bus.EventHandler) bus.EventHandler {
		if s.syncHandlers {
			return h
		}
		return func(_ context.Context, event *bus.Event) error {
			go func() { _ = h(context.Background(), event) }()
			return nil
		}
	}

	subs := []struct {
		subject string
		handler bus.EventHandler
	}{
		{events.TaskUpdated, s.handleTaskUpdated},
		{events.TaskMoved, s.handleTaskMoved},
		{events.OfficeApprovalResolved, s.handleApprovalResolved},
		{events.OfficeCommentCreated, s.handleCommentCreated},
		{events.OfficeTaskStatusChanged, s.handleTaskStatusChanged},
		{events.AgentCompleted, maybeAsync(s.handleAgentCompleted)},
		// AgentStopped fires when StopAgent is called (e.g. office
		// fire-and-forget turn-complete teardown). Same handler — both
		// signal "the agent is no longer running on this task and the
		// claimed run should be finished so the next run for the
		// same agent can be claimed". Without this, comments / status
		// changes / mentions queue runs that never fire because the
		// previous run stays in 'claimed' state forever.
		{events.AgentStopped, maybeAsync(s.handleAgentCompleted)},
		{events.AgentFailed, maybeAsync(s.handleAgentFailed)},
		{events.BuildSessionPromptUsageWildcardSubject(), maybeAsync(s.handlePromptUsage)},
		{events.AgentTurnMessageSaved, maybeAsync(s.handleAgentTurnMessageSaved)},
	}
	for _, sub := range subs {
		if _, err := eb.Subscribe(sub.subject, sub.handler); err != nil {
			return fmt.Errorf("subscribe %s: %w", sub.subject, err)
		}
	}
	return nil
}

// AgentTurnMessageData is the payload of an agent.turn.message_saved event.
type AgentTurnMessageData struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	AgentText string `json:"agent_text"`
	AgentID   string `json:"agent_id"`
}

// handleAgentTurnMessageSaved auto-bridges an agent session response to a
// task comment so it appears in the office chat thread.
// Only runs for office tasks (tasks that have an assignee agent instance).
// Deduplicates: if a comment with source="session" already exists for this
// task, no new comment is created.
func (s *Service) handleAgentTurnMessageSaved(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[AgentTurnMessageData](event)
	if err != nil || data.TaskID == "" {
		return nil
	}

	// Only bridge for office tasks (those with an assignee agent instance).
	fields, err := s.repo.GetTaskExecutionFields(ctx, data.TaskID)
	if err != nil || fields.AssigneeAgentProfileID == "" {
		return nil
	}

	// For streaming agents, Data.Text is empty because chunks were already
	// drained. Fall back to the last agent message saved in session messages.
	agentText := data.AgentText
	if agentText == "" && data.SessionID != "" && s.taskWorkspace != nil {
		if lastMsg, msgErr := s.taskWorkspace.GetLastAgentMessage(ctx, data.SessionID); msgErr == nil && lastMsg != "" {
			agentText = lastMsg
		}
	}
	if agentText == "" {
		return nil
	}

	// Per-turn dedup: skip only if a session-source comment with this
	// exact body already exists for the task. Office sessions are
	// reused across turns (same DB session_id, same ACP session id), so
	// a per-task dedup would suppress every turn after the first. Body
	// equality is a stable per-turn signal because each turn yields a
	// fresh agent response.
	exists, err := s.repo.HasCommentWithSourceAndBody(ctx, data.TaskID, "session", agentText)
	if err != nil {
		s.logger.Warn("hasCommentWithSourceAndBody check failed",
			zap.String("task_id", data.TaskID), zap.Error(err))
	}
	if exists {
		return nil
	}

	comment := &models.TaskComment{
		TaskID:     data.TaskID,
		AuthorType: "agent",
		AuthorID:   fields.AssigneeAgentProfileID,
		Body:       agentText,
		Source:     "session",
	}
	if cErr := s.repo.CreateTaskComment(ctx, comment); cErr != nil {
		s.logger.Error("failed to create agent session comment",
			zap.String("task_id", data.TaskID), zap.Error(cErr))
		return cErr
	}
	s.publishCommentCreated(ctx, comment)
	// Successful turn → reset the agent's consecutive-failure counter
	// regardless of which task succeeded. A bridged comment is the
	// only place we know a turn produced real output.
	s.RecordAgentSuccess(ctx, fields.AssigneeAgentProfileID)
	// Lifecycle: each successful turn under an in-flight run lands a
	// "step" event so the run detail page's Events log gets a row per
	// turn. Resolve the run from the currently-claimed run for this
	// task; bridged comments outside a run (orphaned messages, etc.)
	// don't get attributed.
	if runID := s.ResolveRunForTask(ctx, data.TaskID); runID != "" {
		s.AppendRunEvent(ctx, runID, "step", "info", map[string]interface{}{
			"task_id":    data.TaskID,
			"session_id": data.SessionID,
			"comment_id": comment.ID,
			"chars":      len(agentText),
		})
	}
	return nil
}

func (s *Service) handleAgentCompleted(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[AgentLifecycleData](event)
	if err != nil {
		return nil
	}
	// Taskless completion (PR 1 of office-heartbeat-rework): heartbeat
	// or lightweight-routine fires that don't carry a task_id. Today no
	// caller emits these, so this branch is dead until PR 2 lands the
	// agent_heartbeat cron handler.
	if data.TaskID == "" {
		return s.handleTasklessAgentCompleted(ctx, data)
	}
	run, err := s.repo.GetClaimedRunByTaskID(ctx, data.TaskID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	// Lifecycle: terminal "complete" event for the run detail Events log.
	s.AppendRunEvent(ctx, run.ID, "complete", "info", map[string]interface{}{
		"task_id":    data.TaskID,
		"session_id": data.SessionID,
	})
	s.markRoutingSuccess(ctx, run)
	return s.FinishRun(ctx, run.ID)
}

// markRoutingSuccess delegates to the routing dispatcher (when wired)
// so the resolved provider's health scopes flip back to healthy.
func (s *Service) markRoutingSuccess(ctx context.Context, run *models.Run) {
	rd := s.routingDispatcher
	if rd == nil || run == nil {
		return
	}
	if run.ResolvedProviderID == nil || *run.ResolvedProviderID == "" {
		return
	}
	agent, err := s.GetAgentFromConfig(ctx, run.AgentProfileID)
	if err != nil {
		return
	}
	rd.MarkRunSuccessHealth(ctx, run, agent)
}

// handleTasklessAgentCompleted attributes a taskless run completion,
// finishes the run, and refreshes the per-(agent, "heartbeat")
// continuation summary so the next fire has bridge context. The
// summary is built deterministically from the run's result_json,
// workspace activity, and the prior summary — see the office/summary
// package.
//
// Best-effort: any failure in the summary build path is logged but
// does NOT fail the completion event. We always at least call
// FinishRun so the run row reaches a terminal state.
func (s *Service) handleTasklessAgentCompleted(
	ctx context.Context, data *AgentLifecycleData,
) error {
	if data == nil || data.AgentID == "" {
		return nil
	}
	run, err := s.repo.GetClaimedTasklessRunForAgent(ctx, data.AgentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	s.AppendRunEvent(ctx, run.ID, "complete", "info", map[string]interface{}{
		"agent_id":   data.AgentID,
		"session_id": data.SessionID,
	})
	s.refreshContinuationSummary(ctx, run, data.AgentID)
	return s.FinishRun(ctx, run.ID)
}

// refreshContinuationSummary rebuilds the continuation summary for the
// given agent and upserts it under a scope keyed off the wakeup that
// produced the run. Errors are logged at warn — the prior row stays
// intact (last-good wins) and the run completion proceeds.
//
// Scope rules (office-heartbeat-as-routine):
//   - run came from a routine wakeup (payload has routine_id) →
//     "routine:<routine_id>" so each routine bridges its own context.
//   - any other source (self / user / direct dispatch) →
//     "agent:<agent_id>" so the agent still has somewhere to land its
//     summary even when no routine is in play.
//
// The legacy "heartbeat" scope is retired alongside the agent-level
// heartbeat cron — every scheduled wake now flows through a routine.
func (s *Service) refreshContinuationSummary(
	ctx context.Context, run *models.Run, agentID string,
) {
	if s.repo == nil || run == nil || agentID == "" {
		return
	}
	scope := summaryScopeForRun(run, agentID)
	inputs, err := summaryLoadInputs(ctx, s.repo, run, agentID, scope)
	if err != nil {
		s.logger.Warn("continuation-summary load inputs failed",
			zap.String("run_id", run.ID), zap.Error(err))
		return
	}
	body := summaryBuild(inputs)
	upsertErr := s.repo.UpsertContinuationSummary(ctx, sqlite.AgentContinuationSummary{
		AgentProfileID: agentID,
		Scope:          scope,
		Content:        body,
		ContentTokens:  approxTokenCount(body),
		UpdatedByRunID: run.ID,
	})
	if upsertErr != nil {
		s.logger.Warn("continuation-summary upsert failed",
			zap.String("run_id", run.ID), zap.Error(upsertErr))
	}
}

// approxTokenCount is the same crude 4-chars-per-token approximation
// used elsewhere in the codebase. Good enough for budget-line logging
// — the summary is capped at 8 KB so the absolute number is small.
func approxTokenCount(s string) int {
	return (len(s) + 3) / 4
}

// summaryScopeForRun returns the (agent_profile_id, scope) scope value
// for the continuation-summary upsert. Reads run.ContextSnapshot for a
// routine_id (set by the wakeup dispatcher when source="routine") and
// returns "routine:<id>" when present; falls back to "agent:<id>" so
// non-routine fires still have a stable upsert key.
func summaryScopeForRun(run *models.Run, agentID string) string {
	if run == nil {
		return "agent:" + agentID
	}
	if id := extractRoutineID(run.ContextSnapshot); id != "" {
		return "routine:" + id
	}
	return "agent:" + agentID
}

// extractRoutineID pulls routine_id out of a JSON snapshot. Returns ""
// for missing / malformed payloads so the caller falls back to the
// agent-scoped summary key.
func extractRoutineID(snapshot string) string {
	if snapshot == "" {
		return ""
	}
	var p struct {
		RoutineID string `json:"routine_id"`
	}
	if err := json.Unmarshal([]byte(snapshot), &p); err != nil {
		return ""
	}
	return p.RoutineID
}

func (s *Service) handleAgentFailed(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[AgentLifecycleData](event)
	if err != nil || data.TaskID == "" {
		return nil
	}
	run, err := s.repo.GetClaimedRunByTaskID(ctx, data.TaskID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	// Lifecycle: terminal "error" event for the run detail Events log.
	s.AppendRunEvent(ctx, run.ID, "error", "error", map[string]interface{}{
		"task_id":       data.TaskID,
		"session_id":    data.SessionID,
		"error_message": data.ErrorMessage,
	})
	if s.tryPostStartFallback(ctx, run, data.ErrorMessage) {
		return nil
	}
	// Office failure path (v1): every agent error is terminal. The
	// retry-by-classifier path lives behind HandleRunFailure for
	// rate-limit-retry callers; we deliberately do NOT call into it
	// here. See docs/specs/office-agent-error-handling.
	return s.HandleAgentFailure(ctx, run, data.ErrorMessage)
}

// tryPostStartFallback delegates to the routing dispatcher when one is
// wired and the run was launched via routing. Returns true when the
// dispatcher requeued the run; the caller should NOT escalate.
func (s *Service) tryPostStartFallback(
	ctx context.Context, run *models.Run, errorMessage string,
) bool {
	rd := s.routingDispatcher
	if rd == nil {
		return false
	}
	if run.ResolvedProviderID == nil || *run.ResolvedProviderID == "" {
		return false
	}
	agent, err := s.GetAgentFromConfig(ctx, run.AgentProfileID)
	if err != nil {
		s.logger.Warn("post-start fallback: agent lookup failed",
			zap.String("run_id", run.ID), zap.Error(err))
		return false
	}
	handled, err := rd.HandlePostStartFailure(ctx, run, agent, errorMessage)
	if err != nil {
		s.logger.Warn("post-start fallback failed",
			zap.String("run_id", run.ID), zap.Error(err))
		return false
	}
	return handled
}

// handlePromptUsage records a cost event from a session/prompt usage
// update. Cost resolution follows the three-layer order from
// docs/specs/office-costs/spec.md:
//
//  1. Provider-reported cost (Layer A) — claude-acp emits exact USD per
//     turn on usage_update.cost.amount; the adapter forwards this as
//     ProviderReportedCostSubcents. When > 0 the row is recorded
//     verbatim and pricing lookup is skipped. This is the only accurate
//     path for claude-acp, whose model identifiers are logical aliases
//     (default / sonnet / haiku) with no real-name mapping.
//  2. models.dev (Layer B) — when tokens are reported but no cost,
//     normalize the model id and look up pricing. On miss the row
//     records cost_subcents=0 with estimated=true.
//
// After insert the session totals (tokens_in / tokens_out / cost_subcents)
// are incremented on task_sessions, and any applicable budget policy is
// evaluated. Estimated rows count toward budget totals at face value.
func (s *Service) handlePromptUsage(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[PromptUsageData](event)
	if err != nil || data.TaskID == "" || data.SessionID == "" {
		return nil
	}
	fields, err := s.repo.GetTaskExecutionFields(ctx, data.TaskID)
	if err != nil {
		return nil
	}

	costSubcents, estimated := s.resolveCostForUsage(ctx, *data)
	provider := resolveProvider(*data)

	costEvent := &models.CostEvent{
		SessionID:      data.SessionID,
		TaskID:         data.TaskID,
		AgentProfileID: fields.AssigneeAgentProfileID,
		ProjectID:      s.projectIDForTask(ctx, data.TaskID),
		Model:          data.Model,
		Provider:       provider,
		TokensIn:       data.Usage.InputTokens,
		TokensCachedIn: data.Usage.CachedReadTokens + data.Usage.CachedWriteTokens,
		TokensOut:      data.Usage.OutputTokens,
		CostSubcents:   costSubcents,
		Estimated:      estimated,
		OccurredAt:     time.Now().UTC(),
	}
	if err := s.repo.CreateCostEvent(ctx, costEvent); err != nil {
		return err
	}

	s.incrementSessionUsageTotals(
		ctx, data.SessionID,
		data.Usage.InputTokens, data.Usage.OutputTokens, costSubcents,
	)

	if fields.WorkspaceID != "" {
		if err := s.CheckBudget(
			ctx, fields.WorkspaceID, fields.AssigneeAgentProfileID, costEvent.ProjectID,
		); err != nil {
			s.logger.Warn("post-event budget check failed",
				zap.String("task_id", data.TaskID), zap.Error(err))
		}
	}
	return nil
}

// resolveCostForUsage applies the Layer A / Layer B lookup. Returns
// (costSubcents, estimated). Layer A wins when the adapter forwarded a
// non-zero provider-reported cost (claude-acp's usage_update.cost.amount).
// Layer B (models.dev) is queried when a PricingLookup is wired; on miss
// or when no PricingLookup is configured the row records 0/estimated.
func (s *Service) resolveCostForUsage(
	ctx context.Context, data PromptUsageData,
) (int64, bool) {
	if data.Usage.ProviderReportedCostSubcents > 0 {
		return data.Usage.ProviderReportedCostSubcents, data.Usage.Estimated
	}
	if s.pricingLookup == nil || data.Model == "" {
		return 0, true
	}
	pricing, ok := s.pricingLookup.LookupForModel(ctx, data.Model)
	if !ok {
		return 0, true
	}
	cost := costs.CalculateCostSubcents(
		data.Usage.InputTokens,
		data.Usage.CachedReadTokens,
		data.Usage.CachedWriteTokens,
		data.Usage.OutputTokens,
		costs.ModelPricing{
			InputPerMillion:       pricing.InputPerMillion,
			CachedReadPerMillion:  pricing.CachedReadPerMillion,
			CachedWritePerMillion: pricing.CachedWritePerMillion,
			OutputPerMillion:      pricing.OutputPerMillion,
		},
	)
	return cost, data.Usage.Estimated
}

// resolveProvider derives the provider id for the cost row. AgentType
// (the CLI engine slug — claude-acp, codex-acp, ...) is the most reliable
// source; AgentID is checked as a fallback for legacy bus events that
// publish the CLI name there. ProviderForModel handles canonical model
// prefixes (gpt-*, gemini-*) for CLIs that surface real model names.
// The explicit `provider` field is honoured last.
func resolveProvider(data PromptUsageData) string {
	if p := providerFromCLI(data.AgentType); p != "" {
		return p
	}
	if p := providerFromCLI(data.AgentID); p != "" {
		return p
	}
	if p := costs.ProviderForModel(data.Model); p != "" {
		return p
	}
	return data.Provider
}

// providerFromCLI maps the upstream CLI id (the agent_id stream field)
// to a provider name. Used because claude-acp emits logical model
// aliases (sonnet / haiku) that can't be matched on prefix.
func providerFromCLI(cli string) string {
	switch cli {
	case "claude-acp":
		return "anthropic"
	case "codex-acp", "openai-acp":
		return "openai"
	case "gemini", "gemini-acp":
		return "google"
	}
	return ""
}

func (s *Service) incrementSessionUsageTotals(
	ctx context.Context, sessionID string, tokensIn, tokensOut, costSubcents int64,
) {
	if s.sessionUsageWriter == nil || sessionID == "" {
		return
	}
	if err := s.sessionUsageWriter.IncrementTaskSessionUsage(
		ctx, sessionID, tokensIn, tokensOut, costSubcents,
	); err != nil {
		s.logger.Warn("increment task_session usage failed",
			zap.String("session_id", sessionID), zap.Error(err))
	}
}

func (s *Service) projectIDForTask(ctx context.Context, taskID string) string {
	projectID, err := s.repo.GetTaskProjectID(ctx, taskID)
	if err != nil {
		return ""
	}
	return projectID
}

// handleTaskUpdated fires a task_assigned run when an agent is assigned.
func (s *Service) handleTaskUpdated(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[TaskUpdatedData](event)
	if err != nil || data.AssigneeAgentProfileID == "" {
		return nil
	}
	payload := mustJSON(map[string]string{"task_id": data.TaskID})
	key := fmt.Sprintf("task_assigned:%s", data.TaskID)
	return s.QueueRun(ctx, data.AssigneeAgentProfileID, RunReasonTaskAssigned, payload, key)
}

// handleTaskMoved logs the step change to the activity log and queues
// downstream blocker / children-completed runs when a task lands in a
// terminal step. Stage progression itself is owned by the workflow
// engine (the orchestrator subscribes to TaskMoved and fires
// on_exit / on_enter); this handler covers the side-effects the engine
// path doesn't yet emit.
func (s *Service) handleTaskMoved(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[TaskMovedData](event)
	if err != nil || data.AssigneeAgentProfileID == "" {
		return nil
	}

	// Log the step change to the activity log. The originating run is
	// resolved from the currently-claimed run for this task so the
	// activity row joins back to it on the run detail page's Tasks
	// Touched surface.
	if data.WorkspaceID != "" && data.FromStepName != data.ToStepName {
		runID := s.ResolveRunForTask(ctx, data.TaskID)
		s.LogActivityWithRun(ctx, data.WorkspaceID, "system", "orchestrator",
			"task_status_changed", "task", data.TaskID,
			fmt.Sprintf(`{"new_status":%q,"old_status":%q}`, data.ToStepName, data.FromStepName),
			runID, data.SessionID)
	}

	if categorizeStep(data.ToStepName) == stepCategoryDone {
		return s.finalizeDone(ctx, data)
	}
	return nil
}

// finalizeDone resolves blockers and notifies parents when a task lands
// in a terminal step. Both side-effects route through the engine via
// dispatchEngineTrigger (on_blocker_resolved / on_children_completed).
func (s *Service) finalizeDone(ctx context.Context, data *TaskMovedData) error {
	if err := s.queueBlockersResolvedRuns(ctx, data.TaskID); err != nil {
		s.logger.Error("blocker resolution runs failed", zap.Error(err))
	}
	if data.ParentID != "" {
		if err := s.queueChildrenCompletedRun(ctx, data.ParentID); err != nil {
			s.logger.Error("children completed run failed", zap.Error(err))
		}
	}
	return nil
}

// queueBlockersResolvedRuns checks tasks blocked by the completed task and
// queues runs for those whose blockers are all resolved.
func (s *Service) queueBlockersResolvedRuns(ctx context.Context, completedTaskID string) error {
	blockedTaskIDs, err := s.repo.ListTasksBlockedBy(ctx, completedTaskID)
	if err != nil {
		return fmt.Errorf("list blocked tasks: %w", err)
	}
	for _, blockedID := range blockedTaskIDs {
		if err := s.resolveAndWakeIfUnblocked(ctx, blockedID, completedTaskID); err != nil {
			s.logger.Error("resolve blocker run failed",
				zap.String("blocked_task", blockedID), zap.Error(err))
		}
	}
	return nil
}

func (s *Service) resolveAndWakeIfUnblocked(ctx context.Context, blockedTaskID, resolvedBlockerID string) error {
	blockers, err := s.repo.ListTaskBlockers(ctx, blockedTaskID)
	if err != nil {
		return err
	}
	for _, b := range blockers {
		if b.BlockerTaskID == resolvedBlockerID {
			continue
		}
		done, err := s.repo.IsTaskInTerminalStep(ctx, b.BlockerTaskID)
		if err != nil || !done {
			return err // still blocked
		}
	}
	key := fmt.Sprintf("blockers_resolved:%s", blockedTaskID)
	return s.dispatchEngineTrigger(ctx, blockedTaskID, engine.TriggerOnBlockerResolved,
		engine.OnBlockerResolvedPayload{
			ResolvedBlockerIDs: []string{resolvedBlockerID},
		}, key)
}

// lookupChildPRLinks resolves the PR URLs (one or more, multi-repo capable)
// for each child task id. Returns an empty map when the office service has
// no PR lister wired or when the lookup errors — PR enrichment is
// best-effort, never blocking the parent's wakeup.
func (s *Service) lookupChildPRLinks(
	ctx context.Context, children []sqlite.ChildSummary,
) map[string][]string {
	out := make(map[string][]string, len(children))
	if s.taskPRs == nil || len(children) == 0 {
		return out
	}
	taskIDs := make([]string, 0, len(children))
	for _, c := range children {
		if c.TaskID != "" {
			taskIDs = append(taskIDs, c.TaskID)
		}
	}
	if len(taskIDs) == 0 {
		return out
	}
	prsByTask, err := s.taskPRs.ListTaskPRsByTaskIDs(ctx, taskIDs)
	if err != nil {
		s.logger.Warn("list child task PRs failed",
			zap.Int("children", len(taskIDs)), zap.Error(err))
		return out
	}
	for taskID, links := range prsByTask {
		urls := make([]string, 0, len(links))
		for _, l := range links {
			if l.URL != "" {
				urls = append(urls, l.URL)
			}
		}
		if len(urls) > 0 {
			out[taskID] = urls
		}
	}
	return out
}

// queueChildrenCompletedRun checks if all children of a parent are terminal
// and, if so, dispatches an on_children_completed trigger to the engine
// with child summaries in the payload.
func (s *Service) queueChildrenCompletedRun(ctx context.Context, parentID string) error {
	allDone, err := s.repo.AreAllChildrenTerminal(ctx, parentID)
	if err != nil || !allDone {
		return err
	}

	children, _, err := s.repo.GetChildSummaries(ctx, parentID)
	if err != nil {
		s.logger.Error("get child summaries failed", zap.Error(err))
		children = nil
	}

	key := fmt.Sprintf("children_completed:%s", parentID)
	summaries := make([]engine.ChildSummary, 0, len(children))
	prsByTask := s.lookupChildPRLinks(ctx, children)
	for _, c := range children {
		summaries = append(summaries, engine.ChildSummary{
			TaskID:  c.TaskID,
			Status:  c.State,
			Summary: c.LastComment,
			PRLinks: prsByTask[c.TaskID],
		})
	}
	return s.dispatchEngineTrigger(ctx, parentID, engine.TriggerOnChildrenCompleted,
		engine.OnChildrenCompletedPayload{ChildSummaries: summaries}, key)
}

// handleTaskStatusChanged logs status-change activity. Stage progression
// (Work → Review → Approval → Done) is owned by the workflow engine via
// the on_exit / on_enter triggers fired by the orchestrator on
// TaskMoved; the legacy ExecutionPolicy transition path was removed in
// Phase 4 of task-model-unification.
func (s *Service) handleTaskStatusChanged(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[TaskStatusChangedData](event)
	if err != nil || data.TaskID == "" || data.NewStatus == "" {
		return nil
	}

	fields, _ := s.repo.GetTaskExecutionFields(ctx, data.TaskID)
	if fields != nil && fields.WorkspaceID != "" {
		actorType := "system"
		actorID := "office-scheduler"
		if data.ActorAgentID != "" {
			actorType = participantTypeAgent
			actorID = data.ActorAgentID
		}
		runID := s.ResolveRunForTask(ctx, data.TaskID)
		s.LogActivityWithRun(ctx, fields.WorkspaceID, actorType, actorID,
			"task_status_changed", "task", data.TaskID,
			fmt.Sprintf(`{"new_status":%q}`, data.NewStatus), runID, "")
	}
	return nil
}

// handleCommentCreated loads the comment and relays it to external channels.
func (s *Service) handleCommentCreated(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[CommentPostedData](event)
	if err != nil {
		return nil
	}
	if err := s.queueCommentRun(ctx, *data); err != nil {
		s.logger.Error("queue comment run failed",
			zap.String("task_id", data.TaskID),
			zap.String("comment_id", data.CommentID),
			zap.Error(err))
	}
	comment, err := s.repo.GetTaskComment(ctx, data.CommentID)
	if err != nil {
		s.logger.Error("load comment for relay failed",
			zap.String("comment_id", data.CommentID), zap.Error(err))
		return nil
	}
	if err := s.relay.RelayComment(ctx, comment); err != nil {
		s.logger.Error("relay comment failed",
			zap.String("comment_id", data.CommentID), zap.Error(err))
	}
	return nil
}

func (s *Service) queueCommentRun(ctx context.Context, data CommentPostedData) error {
	if data.TaskID == "" || data.CommentID == "" {
		return nil
	}
	// Self-comment short-circuit: if the agent that wrote the comment is
	// also the task's primary participant we don't want to queue a run.
	// The engine's queue_run resolver enforces the same rule, but
	// short-circuiting here saves an engine evaluation on every agent
	// reply.
	fields, err := s.repo.GetTaskExecutionFields(ctx, data.TaskID)
	if err == nil && fields != nil &&
		data.AuthorType == participantTypeAgent && fields.AssigneeAgentProfileID == data.AuthorID {
		return nil
	}
	key := fmt.Sprintf("task_comment:%s", data.CommentID)
	return s.dispatchEngineTrigger(ctx, data.TaskID, engine.TriggerOnComment,
		engine.OnCommentPayload{
			CommentID: data.CommentID,
			AuthorID:  data.AuthorID,
		}, key)
}

// handleApprovalResolved dispatches an on_approval_resolved trigger to
// the workflow engine. The engine path is the only path after Phase 4 —
// no legacy fallback — so an approval with no associated task or no
// active session is dropped (with a debug log via dispatchEngineTrigger).
func (s *Service) handleApprovalResolved(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[ApprovalResolvedData](event)
	if err != nil || data.RequestedByAgentProfileID == "" {
		return nil
	}
	taskID := s.resolveApprovalTaskID(ctx, data.ApprovalID)
	if taskID == "" {
		s.logger.Debug("approval_resolved: no associated task; dropping",
			zap.String("approval_id", data.ApprovalID))
		return nil
	}
	key := fmt.Sprintf("approval_resolved:%s", data.ApprovalID)
	return s.dispatchEngineTrigger(ctx, taskID, engine.TriggerOnApprovalResolved,
		engine.OnApprovalResolvedPayload{
			ApprovalID: data.ApprovalID,
			Status:     data.Status,
			Note:       data.DecisionNote,
		}, key)
}

// resolveApprovalTaskID returns the task id associated with an approval by
// reading the JSON-encoded `task_id` field from the approval's payload.
// Returns empty if the approval has no task or the lookup/decode fails.
// Used by the engine-driven path which requires a task id to load
// workflow state.
func (s *Service) resolveApprovalTaskID(ctx context.Context, approvalID string) string {
	if approvalID == "" {
		return ""
	}
	a, err := s.repo.GetApproval(ctx, approvalID)
	if err != nil || a == nil || a.Payload == "" {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(a.Payload), &raw); err != nil {
		return ""
	}
	if id, ok := raw["task_id"].(string); ok {
		return id
	}
	return ""
}

// Step categories for task.moved events.
type stepCategory int

const (
	stepCategoryUnknown stepCategory = iota
	stepCategoryInProgress
	stepCategoryDone
	stepCategoryInReview
)

// categorizeStep maps step names to categories.
func categorizeStep(name string) stepCategory {
	switch name {
	case "In Progress", "in_progress":
		return stepCategoryInProgress
	case "Done", "done", "Cancelled", "cancelled":
		return stepCategoryDone
	case "In Review", "in_review":
		return stepCategoryInReview
	default:
		return stepCategoryUnknown
	}
}

// decodeEventData extracts typed data from an event.
func decodeEventData[T any](event *bus.Event) (*T, error) {
	b, err := json.Marshal(event.Data)
	if err != nil {
		return nil, err
	}
	var data T
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// mustJSON marshals v to JSON string, returning "{}" on error.
func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
