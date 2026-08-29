package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	commoncosts "github.com/kandev/kandev/internal/common/costs"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/costs"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/runs/commentkeys"
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
		if errors.Is(err, shared.ErrEngineNoSession) {
			s.logger.Debug("engine trigger skipped: no active session",
				zap.String("task_id", taskID),
				zap.String("trigger", string(trigger)))
			return nil
		}
		return err
	}
	return nil
}

// dispatchEngineTriggerForRecovery dispatches a level-triggered wake through
// the workflow engine and reports whether the engine accepted the trigger.
// A missing session is a normal no-op: recording a receipt in that case would
// suppress a later delivery after the session is created.
//
// The production dispatcher exposes HandleTriggerHandled so this path can
// distinguish a missing session from a valid no-action step. Small test
// dispatchers only implement WorkflowEngineDispatcher, so they use the
// existing error-only contract and count a successful call as accepted.
func (s *Service) dispatchEngineTriggerForRecovery(
	ctx context.Context, taskID string, trigger engine.Trigger, payload any, opID string,
) (bool, error) {
	if s.engineDispatcher == nil {
		return false, nil
	}

	type handledDispatcher interface {
		HandleTriggerHandled(
			context.Context, string, engine.Trigger, any, string,
		) (bool, error)
	}
	if d, ok := s.engineDispatcher.(handledDispatcher); ok {
		_, err := d.HandleTriggerHandled(ctx, taskID, trigger, payload, opID)
		if errors.Is(err, shared.ErrEngineNoSession) {
			s.logger.Debug("engine recovery trigger skipped: no active session",
				zap.String("task_id", taskID),
				zap.String("trigger", string(trigger)))
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}

	if err := s.dispatchEngineTrigger(ctx, taskID, trigger, payload, opID); err != nil {
		return false, err
	}
	return true, nil
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
	EngineDispatched       string `json:"engine_dispatched"`
}

// ApprovalResolvedData represents an approval resolved event payload.
type ApprovalResolvedData struct {
	ApprovalID                string `json:"approval_id"`
	Status                    string `json:"status"`
	DecisionNote              string `json:"decision_note"`
	RequestedByAgentProfileID string `json:"requested_by_agent_profile_id"`
	Type                      string `json:"type"`
}

// AgentLifecycleData is the subset of agent lifecycle event data needed by office.
//
// AgentID is the underlying agent type (for example, "claude-acp"). It is
// not an Office agent identity and must not be used to look up runs or
// continuation summaries.
//
// AgentProfileID is the office agent instance's own identity
// (lifecycle.AgentEventPayload's "agent_profile_id", sourced from
// AgentExecution.officeProfileID()). runs.agent_profile_id is keyed on this
// same identity, so taskless fallback resolution and summary writes use this
// field, not AgentID.
type AgentLifecycleData struct {
	TaskID         string                 `json:"task_id"`
	RunID          string                 `json:"run_id"`
	AgentID        string                 `json:"agent_id"`
	AgentProfileID string                 `json:"agent_profile_id"`
	SessionID      string                 `json:"session_id"`
	TurnID         string                 `json:"turn_id,omitempty"`
	ErrorMessage   string                 `json:"error_message"`
	ProviderError  *streams.ProviderError `json:"provider_error,omitempty"`
}

// resolveLifecycleRun resolves the exact claimed run named by a lifecycle
// event. The task-and-agent fallback keeps compatibility with older events
// that predate run_id, while every current Office launch carries the exact
// identity and avoids predecessor/successor races.
func (s *Service) resolveLifecycleRun(ctx context.Context, data AgentLifecycleData) (*models.Run, error) {
	if data.RunID != "" {
		run, err := s.repo.GetClaimedRunByID(ctx, data.RunID)
		if err != nil {
			return nil, err
		}
		if data.AgentProfileID != "" && run.AgentProfileID != data.AgentProfileID {
			return nil, sql.ErrNoRows
		}
		if data.TaskID != "" && taskIDFromRunPayload(run.Payload) != data.TaskID {
			return nil, sql.ErrNoRows
		}
		return run, nil
	}
	if data.TaskID != "" {
		return s.repo.GetClaimedRunByTaskAndAgent(ctx, data.TaskID, data.AgentProfileID)
	}
	agentProfileID := data.AgentProfileID
	if agentProfileID == "" {
		// Legacy taskless events used AgentID for the Office identity.
		agentProfileID = data.AgentID
	}
	return s.repo.GetClaimedTasklessRunForAgent(ctx, agentProfileID)
}

type PromptUsageData struct {
	TaskID         string      `json:"task_id"`
	SessionID      string      `json:"session_id"`
	AgentID        string      `json:"agent_id"`
	AgentProfileID string      `json:"agent_profile_id,omitempty"`
	AgentType      string      `json:"agent_type"`
	Model          string      `json:"model"`
	Provider       string      `json:"provider"`
	Usage          UsageTokens `json:"usage"`
	TurnID         string      `json:"turn_id,omitempty"`
	UsageEventID   string      `json:"usage_event_id,omitempty"`
}

// UsageTokens mirrors streams.PromptUsage on the wire. All counts are int64
// to match the stream type and to handle workspaces that accumulate over a
// million tokens. ProviderReportedCostSubcents is forwarded from claude-acp's
// usage_update.cost.amount (USD float * 10000); when
// ProviderReportedCostPresent is true (including an explicit zero), the
// subscriber uses it directly and skips the models.dev lookup. The legacy
// positive-value check remains in the resolver for older events. Estimated is
// true when the token counts are not authoritative for a complete turn: the
// adapter synthesised them (codex-acp cumulative-delta inference), or the
// provider returned a frame that covers only part of the turn.
// OutputTokensPresent distinguishes an observed zero from a missing
// output-token sample. The pointer is nil only for legacy events that
// predate this field, so the subscriber can apply the old inference rule to
// those events without overriding an explicit false from a new event.
type UsageTokens struct {
	InputTokens                  int64 `json:"input_tokens"`
	OutputTokens                 int64 `json:"output_tokens"`
	OutputTokensPresent          *bool `json:"output_tokens_present,omitempty"`
	CachedReadTokens             int64 `json:"cached_read_tokens"`
	CachedWriteTokens            int64 `json:"cached_write_tokens"`
	ThoughtTokens                int64 `json:"thought_tokens,omitempty"`
	TotalTokens                  int64 `json:"total_tokens,omitempty"`
	ProviderReportedCostSubcents int64 `json:"provider_reported_cost_subcents,omitempty"`
	ProviderReportedCostPresent  bool  `json:"provider_reported_cost_present,omitempty"`
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
		{events.TaskCreated, s.handleTaskCreated},
		{events.TaskUpdated, s.handleTaskUpdated},
		{events.TaskMoved, s.handleTaskMoved},
		{events.OfficeApprovalResolved, s.handleApprovalResolved},
		{events.OfficeCommentCreated, s.handleCommentCreated},
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
	TurnID    string `json:"turn_id"`
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
	if agentText == "" && data.TurnID != "" && s.taskWorkspace != nil {
		if lastMsg, msgErr := s.taskWorkspace.GetLastAgentMessageForTurn(ctx, data.TurnID); msgErr == nil && lastMsg != "" {
			agentText = lastMsg
		}
	}
	if agentText == "" && data.TurnID == "" && data.SessionID != "" && s.taskWorkspace != nil {
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
	// Taskless completion: heartbeat or lightweight-routine fires that
	// don't carry a task_id. Lightweight routines are already created
	// today (wakeup/dispatcher.go's createFreshRun runs on every
	// coordinator heartbeat fire), but the scheduler cannot yet launch a
	// taskless run (WO-35: SchedulerIntegration.launchAgent fails it
	// instead), so no agent ever completes one and no production caller
	// emits this event yet. Once a taskless launch seam lands (tracked
	// as a follow-up feature, not part of WO-35), this branch is what
	// will finish those runs.
	if data.TaskID == "" {
		return s.handleTasklessAgentCompleted(ctx, data)
	}
	run, err := s.resolveLifecycleRun(ctx, *data)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
	s.recordRunOutputSummary(ctx, run, *data)
	s.warnIfReviewDecisionMissing(ctx, run)
	// resolveLifecycleRun prefers the immutable run ID from the event and
	// falls back to task plus agent only for legacy events. Releasing here
	// (rather than unconditionally inside transitionRunTerminal) is safe —
	// see transitionRunTerminal's doc comment.
	//
	// Finish before releasing: if FinishRun's DB update fails, the checkout
	// must stay held so ReapStaleCheckouts (not a same-agent race) is what
	// eventually reclaims it, instead of releasing a lock for a run that
	// never actually reached a terminal state.
	if err := s.FinishRun(ctx, run.ID, RunOutcomeProcessed); err != nil {
		return err
	}
	s.releaseTaskCheckoutForRun(ctx, run)
	s.stampRunFinished(ctx, run)
	return nil
}

// warnIfReviewDecisionMissing flags a review or approval run that finished
// without the agent ever calling record_step_decision_kandev. A reviewer can
// post a full critique and reject the work in a comment, but if that comment
// never becomes a recorded decision the workflow engine has nothing to act
// on and the task strands in its current step forever. This does not fix
// the missing decision — it only surfaces it as a warn-level run event so
// the stall is visible instead of silent.
func (s *Service) warnIfReviewDecisionMissing(ctx context.Context, run *models.Run) {
	if run == nil {
		return
	}
	parsed := ParseRunPayload(run.Payload)
	taskID, stepID, stageType := s.resolveReviewStage(ctx, run.Reason, parsed)
	if !isReviewOrApprovalStage(stageType) {
		return
	}
	if taskID == "" || stepID == "" {
		return
	}
	has, err := s.repo.HasActiveStepDecision(ctx, taskID, stepID, run.AgentProfileID)
	if err != nil {
		s.logger.Warn("failed to check step decision presence",
			zap.String("task_id", taskID),
			zap.String("run_id", run.ID),
			zap.Error(err))
		return
	}
	if has {
		return
	}
	s.logger.Warn("review/approval run finished without a recorded step decision",
		zap.String("task_id", taskID),
		zap.String("run_id", run.ID),
		zap.String("stage_type", stageType),
		zap.String("step_id", stepID),
		zap.String("agent_profile_id", run.AgentProfileID))
	s.AppendRunEvent(ctx, run.ID, "decision.missing", string(models.RunEventLevelWarn), map[string]interface{}{
		"task_id":    taskID,
		"stage_type": stageType,
		"step_id":    stepID,
	})
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
// finishes the run, and refreshes the per-agent, per-scope continuation
// summary so the next fire has bridge context. The
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
	if data == nil || (data.AgentID == "" && data.AgentProfileID == "" && data.RunID == "") {
		return nil
	}
	run, err := s.resolveLifecycleRun(ctx, *data)
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
	s.refreshContinuationSummary(ctx, run, run.AgentProfileID)
	s.recordRunOutputSummary(ctx, run, *data)
	// run came from GetClaimedTasklessRunForAgent: it is the run that
	// actually launched. Taskless runs typically carry no task_id, so this
	// is a no-op in the common case, but call it for the same reason as
	// handleAgentCompleted — see transitionRunTerminal's doc comment.
	//
	// Finish before releasing, same as handleAgentCompleted: a failed
	// FinishRun must not still give up the checkout.
	if err := s.FinishRun(ctx, run.ID, RunOutcomeProcessed); err != nil {
		return err
	}
	s.releaseTaskCheckoutForRun(ctx, run)
	s.stampRunFinished(ctx, run)
	return nil
}

// refreshContinuationSummary rebuilds the continuation summary for the
// given agent and upserts it under run.ContinuationScope — the scope key
// models.ContinuationScopeForRun computed once, at run-creation time, and
// persisted onto the row (see runs/repository/sqlite.CreateRun). Errors
// are logged at warn — the prior row stays intact (last-good wins) and
// the run completion proceeds.
//
// This deliberately reads the persisted field rather than recomputing it:
// run here comes from resolveLifecycleRun's fresh DB fetch, which can
// observe a context_snapshot a routine wakeup coalesced into this run
// after it was claimed (MarkWakeupRequestCoalesced patches only
// context_snapshot). Recomputing against that drifted snapshot could
// disagree with the scope the claim-time scheduler used when it read the
// prior continuation summary into the prompt — the persisted column is
// the single source of truth both sides read.
func (s *Service) refreshContinuationSummary(
	ctx context.Context, run *models.Run, agentProfileID string,
) {
	if s.repo == nil || run == nil || agentProfileID == "" {
		return
	}
	scope := run.ContinuationScope
	inputs, err := summaryLoadInputs(ctx, s.repo, run, agentProfileID, scope)
	if err != nil {
		s.logger.Warn("continuation-summary load inputs failed",
			zap.String("run_id", run.ID), zap.Error(err))
		return
	}
	body := summaryBuild(inputs)
	upsertErr := s.repo.UpsertContinuationSummary(ctx, sqlite.AgentContinuationSummary{
		AgentProfileID: agentProfileID,
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

// recordRunOutputSummary persists the agent's final message as the run's
// output_summary. Best-effort — same contract as refreshContinuationSummary
// above: any failure is logged at warn and never fails the completion
// event, since the run must still reach a terminal state either way.
//
// The lookup uses the exact turn when a completion event carries one.
// Legacy events use messages created at or after run.ClaimedAt because Office
// task-bound sessions are reused across runs. A nil ClaimedAt falls back to
// the zero time, matching the old unscoped behavior.
//
// failure_reason is deliberately left "" on this write.
// UpdateRunOutputSummary is output_summary's only writer, so this never
// clobbers a value nothing else sets today; giving failure_reason real
// semantics (skipped / checkout-contended / failed) is card b5cf0858's
// territory, not this one's.
func (s *Service) recordRunOutputSummary(ctx context.Context, run *models.Run, data AgentLifecycleData) {
	if s.repo == nil || run == nil {
		return
	}
	summary, err := s.lookupFinalAgentMessage(ctx, run, data)
	if err != nil {
		s.logger.Warn("run output summary: final agent message lookup failed",
			zap.String("run_id", run.ID), zap.Error(err))
		return
	}
	if summary == "" {
		return
	}
	if updateErr := s.repo.UpdateRunOutputSummary(ctx, run.ID, summary, ""); updateErr != nil {
		s.logger.Warn("run output summary: update failed",
			zap.String("run_id", run.ID), zap.Error(updateErr))
	}
}

func (s *Service) lookupFinalAgentMessage(
	ctx context.Context, run *models.Run, data AgentLifecycleData,
) (string, error) {
	if data.TurnID != "" && s.taskWorkspace != nil {
		message, err := s.taskWorkspace.GetLastAgentMessageForTurn(ctx, data.TurnID)
		if err != nil {
			return "", err
		}
		return truncateRunOutputSummary(message), nil
	}
	if data.SessionID == "" {
		return "", nil
	}
	var since time.Time
	if run.ClaimedAt != nil {
		since = *run.ClaimedAt
	}
	return s.repo.GetFinalAgentMessage(ctx, data.SessionID, since)
}

const maxRunOutputSummaryChars = 500

func truncateRunOutputSummary(content string) string {
	runes := []rune(content)
	if len(runes) <= maxRunOutputSummaryChars {
		return content
	}
	return string(runes[:maxRunOutputSummaryChars])
}

func (s *Service) handleAgentFailed(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[AgentLifecycleData](event)
	if err != nil || data.TaskID == "" {
		return nil
	}
	run, err := s.resolveLifecycleRun(ctx, *data)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
	if s.tryPostStartFallback(ctx, run, data.ErrorMessage, data.ProviderError) {
		return nil
	}
	// Office failure path (v1): every agent error is terminal. The
	// retry-by-classifier path lives behind HandleRunFailure for
	// rate-limit-retry callers; we deliberately do NOT call into it
	// here. See docs/specs/office/requirements/runtime.md.
	errMsg := enrichModelFailureMessage(run, data.ErrorMessage)
	if err := s.HandleAgentFailure(ctx, run, errMsg); err != nil {
		return err
	}
	s.dispatchAgentErrorTrigger(ctx, run, data.TaskID, data.SessionID, errMsg)
	return nil
}

// dispatchAgentErrorTrigger wakes the workspace CEO agent (WO-05) after a
// terminal agent-session failure. This is Path A in the office failure
// model; Path B (HandleRunFailure -> escalateFailure, pre-launch retry
// exhaustion) queues its own CEO run directly and never reaches this
// dispatcher, so the two mechanisms cannot double-fire for one failure.
// A dispatch error is logged rather than returned: it must not mask the
// failure bookkeeping HandleAgentFailure already committed.
func (s *Service) dispatchAgentErrorTrigger(
	ctx context.Context, run *models.Run, taskID, sessionID, errMsg string,
) {
	key := fmt.Sprintf("agent_error:%s", run.ID)
	if err := s.dispatchEngineTrigger(ctx, taskID, engine.TriggerOnAgentError,
		engine.OnAgentErrorPayload{
			FailedAgentID:   run.AgentProfileID,
			FailedSessionID: sessionID,
			ErrorMessage:    errMsg,
		}, key); err != nil {
		s.logger.Error("engine trigger on_agent_error failed",
			zap.String("task_id", taskID),
			zap.String("run_id", run.ID),
			zap.Error(err))
	}
}

// enrichModelFailureMessage prepends the actionable "change the model" copy
// when a run fails with a provider/model availability error in strict mode
// (no auto-fallback toggle, no fallback model — the routing dispatcher
// escalates instead of re-dispatching). The raw agent error is preserved
// after the hint so the run detail and chat still show the original cause.
func enrichModelFailureMessage(run *models.Run, message string) string {
	if run == nil || run.ResolvedModel == nil || *run.ResolvedModel == "" || message == "" {
		return message
	}
	provider := ""
	if run.ResolvedProviderID != nil {
		provider = *run.ResolvedProviderID
	}
	classified := routingerr.Classify(routingerr.Input{
		Phase:      routingerr.PhaseStreaming,
		ProviderID: provider,
		Stderr:     message,
	})
	if !routingerr.IsAvailabilityCode(classified.Code) {
		return message
	}
	return routingerr.ModelUnavailableMessage(*run.ResolvedModel) + "\n\n" + message
}

// tryPostStartFallback delegates to the routing dispatcher when one is
// wired and the run was launched via routing. Returns true when the
// dispatcher requeued the run; the caller should NOT escalate.
func (s *Service) tryPostStartFallback(
	ctx context.Context, run *models.Run, errorMessage string,
	providerError *streams.ProviderError,
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
	handled, err := rd.HandlePostStartFailure(ctx, run, agent, errorMessage, providerError)
	if err != nil {
		s.logger.Warn("post-start fallback failed",
			zap.String("run_id", run.ID), zap.Error(err))
		return false
	}
	return handled
}

// handlePromptUsage records a cost event from a session/prompt usage
// update. Cost resolution follows the two-layer order from
// docs/specs/office/requirements/costs.md, and CostSource on the row records which
// layer actually produced the dollar amount (see resolveCostForUsage in
// prompt_usage_cost.go — distinct from Estimated, a usage-authority flag):
//
//  1. Provider-reported cost (Layer A) — claude-acp emits exact USD per
//     turn on usage_update.cost.amount; the adapter forwards this as
//     ProviderReportedCostSubcents plus a presence bit. When present (even
//     when zero), the row is recorded verbatim and pricing lookup is skipped.
//     This is the only accurate
//     path for claude-acp, whose model identifiers are logical aliases
//     (default / sonnet / haiku) with no real-name mapping.
//  2. models.dev (Layer B) — when tokens are reported but no cost,
//     normalize the model id and look up pricing. On miss the row
//     records cost_subcents=0 with cost_source=unpriced; Estimated tracks
//     data.Usage.Estimated verbatim on this path, not hardcoded true.
//
// buildCostEvent (prompt_usage_cost.go) also records the cache read/write
// split when the usage frame reports cache data, and turn_id /
// usage_event_id threaded from the publish site. This function only inserts
// the office_cost_events row; it no longer increments the task_sessions
// rollup columns (docs/specs/task-cost-ledger/spec.md AC-21):
// internal/task/usage's writer is now the sole writer of those columns
// (AC-10), inserting task_usage_events and incrementing task_sessions
// atomically in its own transaction (internal/task/repository/sqlite's
// insertUsageEventAndRollup). Before this change, Office incremented the
// same columns here too, so an Office-enabled install (the dev/e2e default)
// double-counted every turn. Any applicable budget policy is evaluated
// after the insert. Estimated rows count toward budget totals at face
// value. Every early return and the insert path record a writer-health
// metric (cost_metrics.go) so a silently-stopped writer is observable.
func (s *Service) handlePromptUsage(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[PromptUsageData](event)
	if err != nil {
		s.recordCostEventDropped(costDropReasonDecodeError, "")
		return nil
	}
	if data.TaskID == "" || data.SessionID == "" {
		s.recordCostEventDropped(costDropReasonMissingIDs, data.TaskID)
		return nil
	}
	fields, err := s.repo.GetTaskExecutionFields(ctx, data.TaskID)
	if err != nil {
		s.recordCostEventDropped(costDropReasonTaskFieldsError, data.TaskID)
		return nil
	}

	sessionAgentProfileID := data.AgentProfileID
	if sessionAgentProfileID == "" {
		id, lookupErr := s.repo.GetSessionAgentProfileID(ctx, data.TaskID, data.SessionID)
		if lookupErr != nil {
			// Best-effort attribution fallback: log and continue with an
			// unattributed row rather than dropping the cost event over a
			// failed lookup, which would reproduce the exact symptom this
			// fallback exists to fix.
			s.logger.Warn("session agent profile lookup failed",
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID),
				zap.Error(lookupErr))
		} else {
			sessionAgentProfileID = id
		}
	}

	resolution := s.resolveCostForUsage(ctx, *data)
	provider := resolveProvider(*data)
	costEvent := buildCostEvent(
		*data, fields, s.projectIDForTask(ctx, data.TaskID), provider, resolution, sessionAgentProfileID,
	)

	if err := s.repo.CreateCostEvent(ctx, costEvent); err != nil {
		if errors.Is(err, sqlite.ErrDuplicateUsageEvent) {
			s.recordCostEventDropped(costDropReasonDuplicate, data.TaskID)
			return nil
		}
		s.recordCostEventDropped(costDropReasonInsertError, data.TaskID)
		return err
	}
	s.recordCostEventWritten(string(resolution.source), provider)
	s.publishCostRecorded(ctx, fields.WorkspaceID, costEvent)

	if fields.WorkspaceID != "" {
		if err := s.CheckBudget(
			ctx, fields.WorkspaceID, costEvent.AgentProfileID, costEvent.ProjectID,
		); err != nil {
			s.logger.Warn("post-event budget check failed",
				zap.String("task_id", data.TaskID), zap.Error(err))
		}
	}
	return nil
}

// publishCostRecorded emits the durable cost write as an Office notification.
// The database insert is authoritative; a publish failure is logged and does
// not turn a successful cost write into a failed prompt-usage event.
func (s *Service) publishCostRecorded(ctx context.Context, workspaceID string, costEvent *models.CostEvent) {
	if s.eb == nil || workspaceID == "" || costEvent == nil {
		return
	}
	data := map[string]interface{}{
		"workspace_id":     workspaceID,
		"task_id":          costEvent.TaskID,
		"session_id":       costEvent.SessionID,
		"agent_profile_id": costEvent.AgentProfileID,
		"project_id":       costEvent.ProjectID,
		"model":            costEvent.Model,
		"provider":         costEvent.Provider,
		"cost_subcents":    costEvent.CostSubcents,
	}
	event := bus.NewEvent(events.OfficeCostRecorded, "office-service", data)
	if err := s.eb.Publish(ctx, events.OfficeCostRecorded, event); err != nil {
		s.logger.Debug("publish cost recorded event failed",
			zap.String("task_id", costEvent.TaskID), zap.Error(err))
	}
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
// aliases (sonnet / haiku) that can't be matched on prefix. Delegates to
// internal/common/costs, the single source of truth shared with the task
// usage ledger writer (docs/specs/task-cost-ledger/spec.md).
func providerFromCLI(cli string) string {
	return commoncosts.ProviderFromCLI(cli)
}

func (s *Service) projectIDForTask(ctx context.Context, taskID string) string {
	projectID, err := s.repo.GetTaskProjectID(ctx, taskID)
	if err != nil {
		return ""
	}
	return projectID
}

// handleTaskCreated fires a task_assigned run when a newly-created task
// already has a runner. The task.created payload may not carry the
// projected assignee, so this falls back to the stored runner row.
func (s *Service) handleTaskCreated(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[TaskUpdatedData](event)
	if err != nil {
		return nil
	}
	return s.queueTaskAssignedRun(ctx, data.TaskID, data.AssigneeAgentProfileID, true)
}

// handleTaskUpdated fires a task_assigned run when an agent is assigned.
func (s *Service) handleTaskUpdated(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[TaskUpdatedData](event)
	if err != nil {
		return nil
	}
	return s.queueTaskAssignedRun(ctx, data.TaskID, data.AssigneeAgentProfileID, false)
}

func (s *Service) queueTaskAssignedRun(
	ctx context.Context,
	taskID string,
	agentProfileID string,
	fallbackToStoredRunner bool,
) error {
	if taskID == "" {
		return nil
	}
	fields, err := s.repo.GetTaskExecutionFields(ctx, taskID)
	if err != nil {
		if errors.Is(err, sqlite.ErrTaskNotFound) {
			return nil
		}
		return fmt.Errorf("get task execution fields for assignment: %w", err)
	}
	if fields == nil || !fields.IsFromOffice {
		return nil
	}
	if agentProfileID == "" && fallbackToStoredRunner {
		agentProfileID = fields.AssigneeAgentProfileID
	}
	if agentProfileID == "" {
		return nil
	}
	payload := mustJSON(map[string]string{"task_id": taskID})
	key := fmt.Sprintf("task_assigned:%s:%s", taskID, agentProfileID)
	return s.QueueRun(ctx, agentProfileID, RunReasonTaskAssigned, payload, key)
}

// handleTaskMoved keeps the legacy named-step activity fallback and queues
// downstream blocker / children-completed runs when a task lands in a
// terminal step. Canonical task-state activity is written by the task service
// before task.state_changed is published, so the workflow move path is durable
// before any WebSocket refetch can run. Stage progression itself is owned by
// the workflow engine (the orchestrator subscribes to TaskMoved and fires
// on_exit / on_enter).
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
	if data.EngineDispatched == commentkeys.EngineDispatchedValue {
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
	key := commentkeys.TaskComment(data.CommentID)
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
