package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// RefusalGate names the gate that refused an enqueue
// (AC-OFFICE-LAUNCH-SAFETY-001.7, -003.4, -004.4): a refusal is
// enqueue-time, creates no row, and never consumes an idempotency key —
// distinct from a claim-time deferral, which leaves an already-queued
// row untouched.
type RefusalGate string

const (
	// RefusalWorkspaceMissing means the agent profile's workspace could
	// not be resolved (AC-OFFICE-RUN-CAUSATION-001.20).
	RefusalWorkspaceMissing RefusalGate = "workspace_missing"
	// RefusalCausingRunUnreadable means req.CausingRunID was set but the
	// row could not be read (AC-OFFICE-RUN-CAUSATION-001.21).
	RefusalCausingRunUnreadable RefusalGate = "causing_run_unreadable"
	// RefusalCausationDepth means the resolved causation depth exceeds
	// the effective ceiling (AC-OFFICE-LAUNCH-SAFETY-003.4).
	RefusalCausationDepth RefusalGate = "causation_depth"
	// RefusalSelfTrigger means the agent has re-triggered itself with the
	// same reason more than the effective allowance within
	// SelfTriggerWindow (AC-OFFICE-LAUNCH-SAFETY-004.4).
	RefusalSelfTrigger RefusalGate = "self_trigger"
)

// RefusalError is returned by insertRun (via resolveCausation) instead of
// inserting a row. Callers check errors.As, not string matching.
type RefusalError struct {
	Gate   RefusalGate
	Reason string
}

func (e *RefusalError) Error() string {
	return fmt.Sprintf("run enqueue refused (%s): %s", e.Gate, e.Reason)
}

// causationResolution is the resolved causation-chain identity for a
// new run row, computed by resolveCausation before insertRun mints the
// row's own id.
type causationResolution struct {
	// CausationID is empty for a root cause; insertRun sets it to the
	// new row's own id in that case (AC-OFFICE-RUN-CAUSATION-001.2).
	CausationID    string
	ParentRunID    string
	CausationDepth int
	HumanRooted    bool
	RoutineID      string
	ActorKind      models.ActorKind
	ActorID        string
	WorkspaceID    string
	PriorityClass  models.PriorityClass
}

// resolveCausation implements the enqueue control flow of
// docs/specs/office/system-design/unattended-launch-safety-02.md: actor
// normalization, workspace resolution, causing-run resolution, depth and
// self-trigger refusal gates, and priority-class stamping. Every gate
// fails closed: an unreadable input refuses the enqueue rather than
// silently rooting or defaulting it.
func (s *Service) resolveCausation(
	ctx context.Context, agentInstanceID string, req QueueRunRequest,
) (causationResolution, error) {
	actorKind, actorID := normalizeActor(req)

	workspaceID, err := s.repo.ResolveAgentProfileWorkspaceID(ctx, agentInstanceID)
	if err != nil || workspaceID == "" {
		shared.LaunchRefusedTotal.Add(shared.LaunchSafetyLabel("gate", string(RefusalWorkspaceMissing)), 1)
		return causationResolution{}, &RefusalError{
			Gate:   RefusalWorkspaceMissing,
			Reason: fmt.Sprintf("agent profile %s has no resolvable workspace: %v", agentInstanceID, err),
		}
	}

	res := causationResolution{
		ActorKind:   actorKind,
		ActorID:     actorID,
		WorkspaceID: workspaceID,
		RoutineID:   req.RoutineID,
	}

	if err := s.applyCausationLineage(ctx, req, actorKind, &res); err != nil {
		return causationResolution{}, err
	}

	if res.CausationDepth > s.effectiveMaxCausationDepth() {
		shared.LaunchRefusedTotal.Add(shared.LaunchSafetyLabel("gate", string(RefusalCausationDepth)), 1)
		return causationResolution{}, &RefusalError{
			Gate: RefusalCausationDepth,
			Reason: fmt.Sprintf("causation depth %d exceeds limit %d",
				res.CausationDepth, s.effectiveMaxCausationDepth()),
		}
	}

	if err := s.checkSelfTriggerAllowance(ctx, agentInstanceID, req.Reason, actorKind, actorID); err != nil {
		return causationResolution{}, err
	}

	res.PriorityClass = shared.ClassifyPriority(actorKind, req.Reason, false)
	return res, nil
}

// applyCausationLineage fills in res's parent/depth/human-rooted/routine
// fields from either the causing run (refusing if it's unreadable) or,
// for a root cause, the task-boundary carrier. Split out of
// resolveCausation to keep that function's nesting under the repo's
// complexity limit.
func (s *Service) applyCausationLineage(
	ctx context.Context, req QueueRunRequest, actorKind models.ActorKind, res *causationResolution,
) error {
	if req.CausingRunID == "" {
		// AC-OFFICE-RUN-CAUSATION-001.9/.13: a human actor always roots a
		// new chain; otherwise the task-boundary carrier reports
		// human-rooted across a boundary with no live causing run to read
		// it from.
		if actorKind == models.ActorKindUser {
			res.HumanRooted = true
		} else if req.CarrierHumanRooted != nil {
			res.HumanRooted = *req.CarrierHumanRooted
		}
		return nil
	}

	causing, err := s.repo.GetRunByID(ctx, req.CausingRunID)
	if err != nil {
		shared.LaunchRefusedTotal.Add(shared.LaunchSafetyLabel("gate", string(RefusalCausingRunUnreadable)), 1)
		return &RefusalError{
			Gate:   RefusalCausingRunUnreadable,
			Reason: fmt.Sprintf("causing run %s unreadable: %v", req.CausingRunID, err),
		}
	}
	// AC-OFFICE-RUN-CAUSATION-001.9: an actor who is human always roots a
	// new causation chain, even when nested inside a human-rooted run's
	// own follow-on work.
	if actorKind == models.ActorKindUser {
		res.HumanRooted = true
		return nil
	}
	res.CausationID = causingCausationID(causing)
	res.ParentRunID = causing.ID
	res.CausationDepth = causing.CausationDepth + 1
	res.HumanRooted = causing.HumanRooted
	if res.RoutineID == "" {
		res.RoutineID = causing.RoutineID
	}
	return nil
}

// normalizeActor applies AC-OFFICE-RUN-CAUSATION-001.16: an absent,
// invalid, or (for ActorKindAgent) identity-less actor resolves to
// ActorKindSystem rather than a guessed identity, and is counted so a
// caller failing to declare its actor stays visible.
func normalizeActor(req QueueRunRequest) (models.ActorKind, string) {
	if !req.ActorKind.Valid() {
		shared.LaunchActorMissingTotal.Add(shared.LaunchSafetyLabel("reason", req.Reason), 1)
		return models.ActorKindSystem, ""
	}
	if req.ActorKind == models.ActorKindAgent && req.ActorID == "" {
		shared.LaunchActorMissingTotal.Add(shared.LaunchSafetyLabel("reason", req.Reason), 1)
		return models.ActorKindSystem, ""
	}
	return req.ActorKind, req.ActorID
}

// causingCausationID returns the causing run's causation id, adopting
// the causing run's own id when that run predates this column
// (AC-OFFICE-RUN-CAUSATION-001.10's most-restrictive-reading idiom
// applied to a legacy empty value).
func causingCausationID(causing *models.Run) string {
	if causing.CausationID != "" {
		return causing.CausationID
	}
	return causing.ID
}

// checkSelfTriggerAllowance refuses an enqueue whose agent has
// re-triggered itself with the same reason more than the effective
// allowance within SelfTriggerWindow (AC-OFFICE-LAUNCH-SAFETY-004).
// Only applies to an agent acting as itself: a system or human actor
// cannot self-trigger by definition.
func (s *Service) checkSelfTriggerAllowance(
	ctx context.Context, agentInstanceID, reason string, actorKind models.ActorKind, actorID string,
) error {
	if actorKind != models.ActorKindAgent || actorID != agentInstanceID {
		return nil
	}
	since := time.Now().UTC().Add(-SelfTriggerWindow)
	count, err := s.repo.CountSelfTriggeredRuns(ctx, agentInstanceID, reason, since)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", string(RefusalSelfTrigger)), 1)
		return &RefusalError{
			Gate:   RefusalSelfTrigger,
			Reason: fmt.Sprintf("self-trigger count unreadable: %v", err),
		}
	}
	if count >= s.effectiveSelfTriggerAllowance() {
		shared.LaunchRefusedTotal.Add(shared.LaunchSafetyLabel("gate", string(RefusalSelfTrigger)), 1)
		return &RefusalError{
			Gate: RefusalSelfTrigger,
			Reason: fmt.Sprintf("agent %s exceeded self-trigger allowance %d for reason %q within %s",
				agentInstanceID, s.effectiveSelfTriggerAllowance(), reason, SelfTriggerWindow),
		}
	}
	return nil
}
