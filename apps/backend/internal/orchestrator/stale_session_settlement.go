package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/adminmetrics"
	"github.com/kandev/kandev/internal/task/models"
)

// ErrStaleSessionNotStale is deliberately narrow: callers may have authority
// over the target relationship, but the durable evidence still says the exact
// requested turn is active or ambiguous. It must never trigger cancellation.
var ErrStaleSessionNotStale = errors.New("stale session control: active_turn/not_stale")

type StaleSessionSettlementRequest struct {
	ActorTaskID     string
	ActorSessionID  string
	TargetTaskID    string
	TargetSessionID string
	TargetTurnID    string
	AuthorityBasis  string
}

type StaleSessionSettlementResult struct {
	Status string `json:"status"`
}

type sessionControlEventStore interface {
	CreateSessionControlEvent(ctx context.Context, event *models.SessionControlEvent) error
}

// pendingTurnToolCallStore lets stale settlement prove that no unfinished tool
// call is still owned by the captured turn. It is intentionally a narrow
// optional repository capability so older test doubles fail closed instead of
// silently widening stale-settlement authority.
type pendingTurnToolCallStore interface {
	HasPendingToolCallsForTurn(ctx context.Context, turnID string) (bool, error)
}

// SettleStaleSession settles only the current durable turn named by request.
// An eligible completion intent is both the terminal evidence and the quiet
// grace proof; general long-running turns remain ineligible by design.
func (s *Service) SettleStaleSession(ctx context.Context, request StaleSessionSettlementRequest) (StaleSessionSettlementResult, error) {
	if err := request.validate(); err != nil {
		return StaleSessionSettlementResult{}, err
	}
	store, audit, err := s.staleSettlementStores()
	if err != nil {
		return StaleSessionSettlementResult{}, err
	}

	lock, release := s.acquireCancelInFlightGuard(request.TargetSessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()

	intent, evidence := s.staleSettlementCandidate(ctx, store, request)
	if evidence != "" {
		return s.rejectStaleSettlement(ctx, audit, request, evidence)
	}

	// The reconciler owns the exact CAS and post-settlement workflow handling.
	// It re-reads state under the same durable predicate, so duplicate manual and
	// periodic attempts cannot close a successor or re-run a transition.
	s.reconcileCompletionIntent(ctx, store, intent)
	if !staleSettlementCommitted(ctx, store, request) {
		return s.rejectStaleSettlement(ctx, audit, request, "settlement_not_committed")
	}
	if err := audit.CreateSessionControlEvent(ctx, &models.SessionControlEvent{
		ActorTaskID: request.ActorTaskID, ActorSessionID: request.ActorSessionID,
		TargetTaskID: request.TargetTaskID, TargetSessionID: request.TargetSessionID,
		TargetTurnID: request.TargetTurnID, AuthorityBasis: request.AuthorityBasis,
		EvidenceCode: "eligible_completion_intent", Result: "settled",
	}); err != nil {
		return StaleSessionSettlementResult{}, fmt.Errorf("record stale session settlement: %w", err)
	}
	return StaleSessionSettlementResult{Status: "settled"}, nil
}

func (request StaleSessionSettlementRequest) validate() error {
	for _, value := range []string{request.ActorTaskID, request.ActorSessionID, request.TargetTaskID, request.TargetSessionID, request.TargetTurnID, request.AuthorityBasis} {
		if value == "" {
			return fmt.Errorf("stale session settlement requires actor, exact target, and authority")
		}
	}
	return nil
}

func (s *Service) staleSettlementStores() (completionIntentReconciliationStore, sessionControlEventStore, error) {
	store, ok := s.repo.(completionIntentReconciliationStore)
	if !ok || s.turnService == nil {
		return nil, nil, errors.New("stale session settlement is not configured")
	}
	audit, ok := s.repo.(sessionControlEventStore)
	if !ok {
		return nil, nil, errors.New("stale session control audit is not configured")
	}
	return store, audit, nil
}

func (s *Service) staleSettlementCandidate(ctx context.Context, store completionIntentReconciliationStore, request StaleSessionSettlementRequest) (*models.CompletionIntent, string) {
	if s.isCancelInFlight(request.TargetSessionID) {
		return nil, "cancellation_in_flight"
	}
	session, err := s.repo.GetTaskSession(ctx, request.TargetSessionID)
	if err != nil || session == nil {
		return nil, "session_not_running"
	}
	if session.TaskID != request.TargetTaskID || session.State != models.TaskSessionStateRunning {
		return nil, "session_not_running"
	}
	turn, err := s.turnService.GetActiveTurn(ctx, request.TargetSessionID)
	if err != nil || turn == nil || turn.ID != request.TargetTurnID {
		return nil, "successor_or_missing_turn"
	}
	if evidence := s.staleSettlementAdministrativeOwnershipEvidence(ctx, request, turn); evidence != "" {
		return nil, evidence
	}
	intent, err := store.GetCompletionIntentForTurn(ctx, request.TargetSessionID, request.TargetTurnID)
	if err != nil || !completionIntentIsEligible(intent, request.TargetTaskID) {
		return nil, "completion_evidence_missing"
	}
	return intent, ""
}

func (s *Service) staleSettlementAdministrativeOwnershipEvidence(
	ctx context.Context,
	request StaleSessionSettlementRequest,
	turn *models.Turn,
) string {
	if staleSettlementHasPromptReservation(turn.Metadata) {
		return "prompt_reservation"
	}
	if s.hasOutstandingBackgroundWork(request.TargetSessionID) {
		return "background_work"
	}
	pendingTools, ok := s.repo.(pendingTurnToolCallStore)
	if !ok {
		return "pending_tool_check_unavailable"
	}
	pending, err := pendingTools.HasPendingToolCallsForTurn(ctx, request.TargetTurnID)
	if err != nil {
		return "pending_tool_check_failed"
	}
	if pending {
		return "pending_tool_call"
	}
	return ""
}

func completionIntentIsEligible(intent *models.CompletionIntent, taskID string) bool {
	if intent == nil || intent.TaskID != taskID || intent.State != models.CompletionIntentStatePending {
		return false
	}
	return !intent.EligibleAt.After(time.Now().UTC())
}

func staleSettlementCommitted(ctx context.Context, store completionIntentReconciliationStore, request StaleSessionSettlementRequest) bool {
	intent, err := store.GetCompletionIntentForTurn(ctx, request.TargetSessionID, request.TargetTurnID)
	if err != nil || intent == nil {
		return false
	}
	return intent.State == models.CompletionIntentStateSettled || intent.State == models.CompletionIntentStateSuperseded
}

func (s *Service) rejectStaleSettlement(ctx context.Context, audit sessionControlEventStore, request StaleSessionSettlementRequest, evidence string) (StaleSessionSettlementResult, error) {
	adminmetrics.RecordStaleControlDenial(evidence)
	if err := audit.CreateSessionControlEvent(ctx, &models.SessionControlEvent{
		ActorTaskID: request.ActorTaskID, ActorSessionID: request.ActorSessionID,
		TargetTaskID: request.TargetTaskID, TargetSessionID: request.TargetSessionID,
		TargetTurnID: request.TargetTurnID, AuthorityBasis: request.AuthorityBasis,
		EvidenceCode: evidence, Result: "not_stale",
	}); err != nil {
		return StaleSessionSettlementResult{}, fmt.Errorf("record stale session refusal: %w", err)
	}
	return StaleSessionSettlementResult{}, ErrStaleSessionNotStale
}

func staleSettlementHasPromptReservation(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	pending, _ := metadata[models.TurnMetaKeyPromptDispatchPending].(bool)
	attempted, _ := metadata[models.TurnMetaKeyPromptDispatchAttempted].(bool)
	return pending || attempted
}
