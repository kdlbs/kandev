package lifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	restoremetrics "github.com/kandev/kandev/internal/agent/runtime/lifecycle/metrics"
	"github.com/kandev/kandev/internal/task/models"
)

// RestoreCoordinatorHooks are the side effects owned by lifecycle and the
// task repository. Hooks are deliberately explicit so a candidate native
// session cannot become current before configuration succeeds.
type RestoreCoordinatorHooks struct {
	LoadNative         func(context.Context, string) error
	CreateNative       func(context.Context, string) (string, error)
	ApplyConfiguration func(context.Context, string) error
	PersistAttempt     func(context.Context, *models.RestoreAttempt) error
	CompleteAttempt    func(context.Context, string, string, time.Time) error
	PersistSnapshot    func(context.Context, *models.ContinuationSnapshot) error
	CommitGeneration   func(context.Context, *models.HarnessSessionGeneration, int64) (bool, error)
}

// RestoreCoordinatorRequest describes one lifecycle restore decision.
type RestoreCoordinatorRequest struct {
	Identity              RestoreIdentity
	AgentType             string
	AdapterVersion        string
	Action                RestoreAction
	ExplicitAuthorization bool
	Capabilities          RestoreCapabilities
	TargetWorkspace       string
	Snapshot              *models.ContinuationSnapshot
}

// RestoreCoordinatorResult is safe to publish only after DispatchAllowed is
// true. A native identity remains the committed identity on every failure.
type RestoreCoordinatorResult struct {
	NativeSessionID string
	Decision        RestoreDecision
	AttemptID       string
	DispatchAllowed bool
}

// RestoreCoordinator is the single ordering boundary for native restore and
// explicit context continuation. It does not dispatch a prompt.
type RestoreCoordinator struct {
	Now func() time.Time
}

// Restore runs the native-first restore sequence. Snapshot persistence and
// generation commit precede any prompt admission for a continuation.
func (c *RestoreCoordinator) Restore(ctx context.Context, request RestoreCoordinatorRequest, hooks RestoreCoordinatorHooks) (RestoreCoordinatorResult, error) {
	now := time.Now().UTC()
	if c != nil && c.Now != nil {
		now = c.Now().UTC()
	}
	result := RestoreCoordinatorResult{NativeSessionID: request.Identity.NativeSessionID}
	defer func() {
		restoremetrics.RecordRestoreAttempt(
			string(result.Decision.Outcome), string(result.Decision.Reason), request.AgentType,
		)
	}()
	attempt := &models.RestoreAttempt{
		ID:                 uuid.NewString(),
		SessionID:          request.Identity.SessionID,
		IncarnationID:      request.Identity.IncarnationID,
		ExpectedGeneration: int64(request.Identity.HarnessGeneration),
		Action:             string(request.Action),
		Outcome:            string(RestoreOutcomeBlocked),
		TargetWorkspace:    request.TargetWorkspace,
		Authorized:         request.ExplicitAuthorization,
		CreatedAt:          now,
	}
	result.AttemptID = attempt.ID
	if hooks.PersistAttempt != nil {
		if err := hooks.PersistAttempt(ctx, attempt); err != nil {
			return result, fmt.Errorf("persist restore attempt: %w", err)
		}
	}
	decision, nativeID, restoreErr := c.decide(ctx, request, hooks)
	result.Decision = decision
	if restoreErr != nil {
		_ = completeRestoreAttempt(ctx, hooks, attempt, result.Decision.Outcome, now)
		return result, restoreErr
	}
	if nativeID != "" {
		if err := c.applyAndCommit(ctx, request, attempt, result.Decision, nativeID, hooks, now, false); err != nil {
			return result, err
		}
		result.NativeSessionID = nativeID
		result.DispatchAllowed = true
		return result, nil
	}
	if result.Decision.Outcome != RestoreOutcomeContextContinued || !result.Decision.ActionAuthorized {
		_ = completeRestoreAttempt(ctx, hooks, attempt, result.Decision.Outcome, now)
		return result, &RestoreRequiredError{Decision: result.Decision}
	}
	candidateID, err := c.continueFromSnapshot(ctx, request, attempt, hooks, now)
	if err != nil {
		return result, err
	}
	result.NativeSessionID = candidateID
	result.DispatchAllowed = true
	return result, nil
}

func (c *RestoreCoordinator) decide(ctx context.Context, request RestoreCoordinatorRequest, hooks RestoreCoordinatorHooks) (RestoreDecision, string, error) {
	switch {
	case request.Identity.NativeSessionID == "":
		return decideWithoutNativeIdentity(request), "", nil
	case hooks.LoadNative == nil:
		return DecideRestore(RestoreRequest{
			Identity:              request.Identity,
			Capabilities:          request.Capabilities,
			Failure:               RestoreFailure{Reason: RestoreReasonNativeResumeUnsupported},
			Action:                request.Action,
			ExplicitAuthorization: request.ExplicitAuthorization,
		}), "", nil
	default:
		if err := hooks.LoadNative(ctx, request.Identity.NativeSessionID); err == nil {
			return DecideRestore(RestoreRequest{
				Identity:     request.Identity,
				Capabilities: request.Capabilities,
				Action:       RestoreActionNativeResume,
			}), request.Identity.NativeSessionID, nil
		} else {
			restoreErr := newRestoreRequiredError(request.Identity, err)
			if request.Action == RestoreActionContinueFromHistory && request.ExplicitAuthorization && restoreErr.Decision.AllowsContextContinuation {
				return DecideRestore(RestoreRequest{
					Identity:              request.Identity,
					Capabilities:          request.Capabilities,
					Failure:               RestoreFailure{Reason: restoreErr.Decision.Reason, Detail: err.Error()},
					Action:                request.Action,
					ExplicitAuthorization: true,
				}), "", nil
			}
			return restoreErr.Decision, "", restoreErr
		}
	}
}

func decideWithoutNativeIdentity(request RestoreCoordinatorRequest) RestoreDecision {
	if request.Action == RestoreActionContinueFromHistory && request.ExplicitAuthorization {
		return DecideRestore(RestoreRequest{
			Identity:              request.Identity,
			Action:                request.Action,
			ExplicitAuthorization: true,
		})
	}
	return RestoreDecision{
		Outcome:                   RestoreOutcomeBlocked,
		Reason:                    RestoreReasonNativeStateMissing,
		PreserveNativeIdentity:    true,
		AllowsContextContinuation: true,
	}
}

func (c *RestoreCoordinator) continueFromSnapshot(ctx context.Context, request RestoreCoordinatorRequest, attempt *models.RestoreAttempt, hooks RestoreCoordinatorHooks, now time.Time) (string, error) {
	if request.Snapshot == nil || hooks.PersistSnapshot == nil || hooks.CreateNative == nil {
		return "", fmt.Errorf("context continuation requires a snapshot and native creation hooks")
	}
	request.Snapshot.AttemptID = attempt.ID
	request.Snapshot.SessionID = request.Identity.SessionID
	request.Snapshot.Status = models.ContinuitySnapshotPrepared
	if err := hooks.PersistSnapshot(ctx, request.Snapshot); err != nil {
		return "", fmt.Errorf("persist continuation snapshot: %w", err)
	}
	candidateID, err := hooks.CreateNative(ctx, request.TargetWorkspace)
	if err != nil {
		return "", fmt.Errorf("create continuation session: %w", err)
	}
	if err := c.applyAndCommit(ctx, request, attempt, RestoreDecision{Outcome: RestoreOutcomeContextContinued, ActionAuthorized: true}, candidateID, hooks, now, true); err != nil {
		return "", err
	}
	return candidateID, nil
}

func (c *RestoreCoordinator) applyAndCommit(ctx context.Context, request RestoreCoordinatorRequest, attempt *models.RestoreAttempt, decision RestoreDecision, nativeID string, hooks RestoreCoordinatorHooks, now time.Time, commitGeneration bool) error {
	if hooks.ApplyConfiguration != nil {
		if err := hooks.ApplyConfiguration(ctx, nativeID); err != nil {
			return fmt.Errorf("restore session configuration: %w", err)
		}
	}
	if commitGeneration && hooks.CommitGeneration != nil {
		generation := &models.HarnessSessionGeneration{
			SessionID:             request.Identity.SessionID,
			IncarnationID:         request.Identity.IncarnationID,
			Generation:            int64(request.Identity.HarnessGeneration) + 1,
			PredecessorGeneration: int64(request.Identity.HarnessGeneration),
			NativeSessionID:       nativeID,
			AgentType:             request.AgentType,
			AdapterVersion:        request.AdapterVersion,
			OriginalWorkspace:     request.Identity.OriginalWorkspace,
			CurrentWorkspace:      request.TargetWorkspace,
			NativeStateReference:  request.Identity.NativeStateReference,
			CreationReason:        string(decision.Outcome),
			CreatedAt:             now,
			CommittedAt:           now,
		}
		committed, err := hooks.CommitGeneration(ctx, generation, int64(request.Identity.HarnessGeneration))
		if err != nil {
			return fmt.Errorf("commit restore generation: %w", err)
		} else if !committed {
			return fmt.Errorf("commit restore generation: stale generation")
		}
	}
	if err := completeRestoreAttempt(ctx, hooks, attempt, decision.Outcome, now); err != nil {
		return err
	}
	return nil
}

func completeRestoreAttempt(ctx context.Context, hooks RestoreCoordinatorHooks, attempt *models.RestoreAttempt, outcome RestoreOutcome, completedAt time.Time) error {
	if attempt == nil {
		return nil
	}
	attempt.Outcome = string(outcome)
	attempt.CompletedAt = &completedAt
	if hooks.CompleteAttempt != nil {
		return hooks.CompleteAttempt(ctx, attempt.ID, string(outcome), completedAt)
	}
	if hooks.PersistAttempt == nil {
		return nil
	}
	return nil
}
