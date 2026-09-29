package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"

	"go.uber.org/zap"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const detailResumeDeferred = "It will resume when there is room"

// resumeSpec is the stored spec of a resume proposal.
type resumeSpec struct {
	TaskID    string `json:"task_id"`
	Rationale string `json:"rationale"`
}

type resumeKind struct{ svc *Service }

func (k *resumeKind) Kind() string             { return ProposalKindResume }
func (k *resumeKind) Action() Action           { return ActionResume }
func (k *resumeKind) ReRunsOnStaleClaim() bool { return false }

func (k *resumeKind) ValidateEdits(base, edits json.RawMessage) (json.RawMessage, error) {
	return refuseAllEdits(base, edits)
}

// sessionResumable reports whether s can be resumed now: not COMPLETED or
// CREATED, no live execution, and either an executor record to resume from, a
// FAILED or CANCELLED state, or an interrupted recovery waiting.
func sessionResumable(s *TargetSession, live bool) bool {
	switch taskmodels.TaskSessionState(s.State) {
	case taskmodels.TaskSessionStateCompleted, taskmodels.TaskSessionStateCreated:
		return false
	case taskmodels.TaskSessionStateFailed, taskmodels.TaskSessionStateCancelled:
		return !live
	}
	return !live && (s.HasExecutorRecord || s.RecoveryPending)
}

type resumeResult struct {
	started bool
	err     error
}

func (k *resumeKind) Execute(ctx context.Context, claim Claim) (Outcome, error) {
	var spec resumeSpec
	if err := json.Unmarshal(claim.Spec, &spec); err != nil {
		return Outcome{}, failWith("stored resume spec is unreadable")
	}
	deps := k.svc.kindDeps
	if deps.Tasks == nil || deps.Resumer == nil {
		return Outcome{}, failWith("session resume is not wired")
	}
	target, err := deps.Tasks.GetTarget(ctx, claim.TargetTaskID)
	if errors.Is(err, ErrTaskNotFound) {
		return Outcome{}, failWith(failTaskArchived)
	}
	if err != nil {
		return Outcome{}, failWith(err.Error())
	}
	if target.ArchivedAt != nil {
		return Outcome{}, failWith(failTaskArchived)
	}
	if err := checkExecuteTarget(target, claim, spec.TaskID); err != nil {
		return Outcome{}, err
	}
	if target.Primary == nil || !sessionResumable(target.Primary, deps.Tasks.HasLiveExecution(ctx, target.Primary.ID)) {
		return Outcome{}, failWith(failNotResumable)
	}
	sessionID := target.Primary.ID
	res, err := k.launch(ctx, claim, sessionID)
	if err != nil {
		return Outcome{}, err
	}
	out := map[string]any{"session_id": sessionID}
	detail := "Resumed"
	if !res.started {
		out["deferred"] = true
		detail = detailResumeDeferred
	}
	raw, _ := json.Marshal(out)
	return Outcome{TaskID: claim.TargetTaskID, OutcomeJSON: string(raw), Detail: detail}, nil
}

// launch runs the resume in a goroutine that outlives ctx, so a cold launch
// never holds the approve request past its deadline. A launch that finishes
// after Execute gave up writes nothing and logs the fence.
func (k *resumeKind) launch(ctx context.Context, claim Claim, sessionID string) (resumeResult, error) {
	done := make(chan resumeResult, 1)
	var abandoned atomic.Bool
	launchCtx := context.WithoutCancel(ctx)
	k.svc.launchWG.Add(1)
	go func() {
		defer k.svc.launchWG.Done()
		started, err := k.svc.kindDeps.Resumer.ResumeTaskSession(launchCtx, claim.TargetTaskID, sessionID)
		if abandoned.Load() {
			k.svc.logger.Warn("execute_settle_fenced",
				zap.String("proposal_id", claim.ProposalID), zap.String("task_id", claim.TargetTaskID), zap.Bool("started", started), zap.Error(err))
		}
		done <- resumeResult{started: started, err: err}
	}()
	select {
	case r := <-done:
		return r, r.err
	case <-ctx.Done():
		abandoned.Store(true)
		return resumeResult{}, ctx.Err()
	}
}

// WaitResumeLaunchesStopped blocks until every resume launch goroutine has
// returned, so a graceful shutdown or a test can join them.
func (s *Service) WaitResumeLaunchesStopped() { s.launchWG.Wait() }
