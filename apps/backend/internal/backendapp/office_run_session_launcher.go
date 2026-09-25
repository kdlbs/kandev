package backendapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeservice "github.com/kandev/kandev/internal/office/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// officeRunSessionLauncher is the backend composition seam for taskless
// Office runs. It reserves the Office-owned attempt first, then delegates
// process allocation and prompt delivery to the shared runtime.
type officeRunSessionLauncher struct {
	repo     *officesqlite.Repository
	runtime  runtimeapi.Runtime
	recovery runtimeapi.RunOwnerRecovery
	logger   *logger.Logger
}

var _ officeservice.RunSessionLauncher = (*officeRunSessionLauncher)(nil)
var _ runtimeapi.OwnerAdmission = (*officeRunSessionLauncher)(nil)

func newOfficeRunSessionLauncher(
	repo *officesqlite.Repository, runtimeBackend runtimeapi.Backend, log *logger.Logger,
) *officeRunSessionLauncher {
	recovery, _ := runtimeBackend.(runtimeapi.RunOwnerRecovery)
	return &officeRunSessionLauncher{
		repo:     repo,
		runtime:  runtimeapi.New(runtimeBackend),
		recovery: recovery,
		logger:   log.WithFields(zap.String("component", "office-run-session-launcher")),
	}
}

//nolint:cyclop,funlen // The launch sequence keeps reservation, identity binding, admission, and cleanup together.
func (l *officeRunSessionLauncher) StartRunSession(
	ctx context.Context,
	run *models.Run,
	agent *models.AgentInstance,
	launch officeservice.LaunchContext,
	route *officeservice.RouteOverride,
) (officeservice.RunSessionLaunch, error) {
	if run == nil || agent == nil {
		return officeservice.RunSessionLaunch{}, errors.New("run-session launch requires run and agent")
	}
	if run.ID == "" || agent.ID == "" || agent.WorkspaceID == "" {
		return officeservice.RunSessionLaunch{}, errors.New("run-session launch has incomplete owner identity")
	}
	sessions, err := l.repo.ListRunSessions(ctx, run.ID)
	if err != nil {
		return officeservice.RunSessionLaunch{}, fmt.Errorf("list run sessions: %w", err)
	}
	attempt := 1
	for _, existing := range sessions {
		if existing.Attempt >= attempt {
			attempt = existing.Attempt + 1
		}
	}
	now := time.Now().UTC()
	session := &models.RunSession{
		ID:             uuid.NewString(),
		WorkspaceID:    agent.WorkspaceID,
		AgentProfileID: agent.ID,
		RunID:          run.ID,
		Attempt:        attempt,
		State:          models.RunSessionStatePreparing,
		CreatedAt:      now,
		Version:        1,
	}
	reserved, err := l.repo.ReserveRunSession(ctx, session)
	if err != nil {
		return officeservice.RunSessionLaunch{}, err
	}
	if !reserved {
		return officeservice.RunSessionLaunch{}, fmt.Errorf("run %q attempt %d was not admitted", run.ID, attempt)
	}

	executionProfileID := launch.ProfileID
	if route != nil && route.ExecutionProfileID != "" {
		executionProfileID = route.ExecutionProfileID
	}
	env := cloneStringMap(launch.Env)
	if route != nil {
		for key, value := range route.Env {
			env[key] = value
		}
	}
	owner := runtimeapi.ExecutionOwner{
		Kind:           runtimeapi.ExecutionOwnerRun,
		WorkspaceID:    agent.WorkspaceID,
		RunID:          run.ID,
		RunSessionID:   session.ID,
		Attempt:        attempt,
		AgentProfileID: agent.ID,
	}
	request := &runtimeapi.LaunchRequest{
		WorkspaceID:          agent.WorkspaceID,
		AgentProfileID:       agent.ID,
		ExecutionProfileID:   executionProfileID,
		StartAgent:           true,
		TaskDescription:      launch.Prompt,
		Env:                  env,
		AdditionalSkillSlugs: append([]string(nil), launch.AdditionalSkillSlugs...),
		ExecutorType:         launch.ExecutorID,
		McpMode:              "office",
		Owner:                owner,
		OwnerAdmission:       l,
		Metadata: map[string]interface{}{
			"office_run_id":         run.ID,
			"office_run_session_id": session.ID,
			"office_run_attempt":    attempt,
		},
	}
	if route != nil {
		request.ModelOverride = route.Model
		request.RouteOverride = &runtimeapi.RouteOverride{
			ExecutionProfileID: route.ExecutionProfileID,
			ProviderID:         route.ProviderID,
			Model:              route.Model,
			Tier:               route.Tier,
			Mode:               route.Mode,
			Flags:              append([]string(nil), route.Flags...),
			Env:                cloneStringMap(route.Env),
		}
	}
	ref, err := l.runtime.Launch(ctx, runtimeapi.LaunchSpec{
		AgentProfileID: agent.ID,
		ExecutorID:     launch.ExecutorID,
		Prompt:         launch.Prompt,
		McpMode:        "office",
		Metadata:       map[string]any{"launch_request": request},
		Owner:          owner,
		OwnerAdmission: l,
	})
	if err != nil {
		_, _ = l.repo.FinishRunSession(context.WithoutCancel(ctx), session.ID, models.RunSessionStateFailed, err.Error())
		return officeservice.RunSessionLaunch{}, fmt.Errorf("start run session: %w", err)
	}
	execution, err := l.runtime.GetExecution(ctx, ref.ID)
	if err != nil {
		_, _ = l.repo.FinishRunSession(context.WithoutCancel(ctx), session.ID, models.RunSessionStateFailed, err.Error())
		return officeservice.RunSessionLaunch{}, fmt.Errorf("read started run session: %w", err)
	}
	adapter := agent.AgentID
	model := agent.Model
	if route != nil {
		if route.ProviderID != "" {
			adapter = route.ProviderID
		}
		if route.Model != "" {
			model = route.Model
		}
	}
	bound, err := l.repo.BindRunSessionExecution(
		ctx, session.ID, ref.ID, executionProfileID, adapter, model, execution.ACPSessionID,
	)
	if err != nil {
		_ = l.runtime.Stop(context.WithoutCancel(ctx), ref.ID, "run_session_identity_persist_failed")
		return officeservice.RunSessionLaunch{}, err
	}
	if !bound {
		_ = l.runtime.Stop(context.WithoutCancel(ctx), ref.ID, "run_session_not_admitted")
		return officeservice.RunSessionLaunch{}, fmt.Errorf("run session %q was no longer preparing", session.ID)
	}
	if err := l.AdmitExecution(ctx, owner); err != nil {
		_ = l.runtime.Stop(context.WithoutCancel(ctx), ref.ID, "run_session_start_admission_failed")
		_, _ = l.repo.FinishRunSession(context.WithoutCancel(ctx), session.ID, models.RunSessionStateInterrupted, err.Error())
		return officeservice.RunSessionLaunch{}, err
	}
	if err := l.runtime.StartExecution(ctx, ref.ID); err != nil {
		_ = l.runtime.Stop(context.WithoutCancel(ctx), ref.ID, "run_session_start_failed")
		_, _ = l.repo.FinishRunSession(context.WithoutCancel(ctx), session.ID, models.RunSessionStateFailed, err.Error())
		return officeservice.RunSessionLaunch{}, fmt.Errorf("start run session: %w", err)
	}
	execution, err = l.runtime.GetExecution(ctx, ref.ID)
	if err != nil {
		_ = l.runtime.Stop(context.WithoutCancel(ctx), ref.ID, "run_session_state_read_failed")
		_, _ = l.repo.FinishRunSession(context.WithoutCancel(ctx), session.ID, models.RunSessionStateFailed, err.Error())
		return officeservice.RunSessionLaunch{}, fmt.Errorf("read started run session: %w", err)
	}
	started, err := l.repo.MarkRunSessionStarted(ctx, session.ID, ref.ID, executionProfileID, adapter, model, execution.ACPSessionID)
	if err != nil {
		_ = l.runtime.Stop(context.WithoutCancel(ctx), ref.ID, "run_session_persist_failed")
		return officeservice.RunSessionLaunch{}, err
	}
	if !started {
		_ = l.runtime.Stop(context.WithoutCancel(ctx), ref.ID, "run_session_not_admitted")
		return officeservice.RunSessionLaunch{}, fmt.Errorf("run session %q was no longer preparing", session.ID)
	}
	l.logger.Info("started taskless Office run",
		zap.String("run_id", run.ID), zap.String("run_session_id", session.ID),
		zap.String("execution_id", ref.ID), zap.Int("attempt", attempt))
	return officeservice.RunSessionLaunch{
		SessionID:          session.ID,
		ExecutionID:        ref.ID,
		ExecutionProfileID: executionProfileID,
		Adapter:            adapter,
		Model:              model,
		ACPSessionID:       execution.ACPSessionID,
	}, nil
}

// ReconcileRunSessions runs after lifecycle recovery and before the Office
// scheduler can claim work. A replacement is allowed only after the exact
// predecessor is proven absent or terminal.
//
//nolint:cyclop,gocognit // Recovery must handle each durable predecessor state before replacement.
func (l *officeRunSessionLauncher) ReconcileRunSessions(ctx context.Context) error {
	sessions, err := l.repo.ListUnfinishedRunSessions(ctx)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session.ExecutionID == "" {
			if err := l.stopRecoveredRunSession(ctx, session); err != nil {
				return err
			}
			if _, err := l.repo.FinishRunSession(ctx, session.ID, models.RunSessionStateInterrupted, "runtime execution was not registered"); err != nil {
				return err
			}
			_, _ = l.repo.RequeueClaimedRun(ctx, session.RunID)
			continue
		}
		execution, err := l.runtime.GetExecution(ctx, session.ExecutionID)
		if err != nil {
			if !runtimeapi.IsNotFound(err) {
				return fmt.Errorf("reconcile run session %q: inspect execution: %w", session.ID, err)
			}
			if err := l.stopRecoveredRunSession(ctx, session); err != nil {
				return err
			}
			if _, finishErr := l.repo.FinishRunSession(ctx, session.ID, models.RunSessionStateInterrupted, "runtime execution was not found after restart"); finishErr != nil {
				return finishErr
			}
			_, _ = l.repo.RequeueClaimedRun(ctx, session.RunID)
			continue
		}
		if !sameRunOwner(execution.Owner, session) {
			return fmt.Errorf("reconcile run session %q: runtime owner identity is uncertain", session.ID)
		}
		if isLiveExecution(execution.Status) {
			continue
		}
		state := models.RunSessionStateInterrupted
		message := "runtime execution ended during restart"
		switch execution.Status {
		case v1.AgentStatusCompleted:
			state = models.RunSessionStateFinished
			message = ""
		case v1.AgentStatusFailed:
			state = models.RunSessionStateFailed
			message = execution.ErrorMessage
		}
		if _, finishErr := l.repo.FinishRunSession(ctx, session.ID, state, message); finishErr != nil {
			return finishErr
		}
		if state == models.RunSessionStateFinished {
			outcome := officeservice.RunOutcomeProcessed
			if _, finishErr := l.repo.FinishRun(ctx, session.RunID, officeservice.RunStatusFinished, &outcome); finishErr != nil {
				return finishErr
			}
		}
		if state == models.RunSessionStateInterrupted || state == models.RunSessionStateFailed {
			_, _ = l.repo.RequeueClaimedRun(ctx, session.RunID)
		}
	}
	return nil
}

func sameRunOwner(owner runtimeapi.ExecutionOwner, session models.RunSession) bool {
	return owner.Kind == runtimeapi.ExecutionOwnerRun && owner.WorkspaceID == session.WorkspaceID &&
		owner.RunID == session.RunID && owner.RunSessionID == session.ID && owner.Attempt == session.Attempt &&
		owner.AgentProfileID == session.AgentProfileID
}

func isLiveExecution(status v1.AgentStatus) bool {
	return status == v1.AgentStatusPending || status == v1.AgentStatusStarting ||
		status == v1.AgentStatusRunning || status == v1.AgentStatusReady
}

// AdmitExecution is the fail-closed owner check used before allocation, at
// lifecycle registration, and immediately before process start.
//
//nolint:cyclop // The ordered fail-closed checks are the owner admission invariant.
func (l *officeRunSessionLauncher) AdmitExecution(ctx context.Context, owner runtimeapi.ExecutionOwner) error {
	if owner.Kind != runtimeapi.ExecutionOwnerRun || owner.WorkspaceID == "" || owner.RunID == "" ||
		owner.RunSessionID == "" || owner.Attempt <= 0 || owner.AgentProfileID == "" {
		return errors.New("run execution owner is incomplete")
	}
	run, err := l.repo.GetRun(ctx, owner.RunID)
	if err != nil {
		return fmt.Errorf("admit run execution: read run: %w", err)
	}
	if run.Status != "claimed" || run.AgentProfileID != owner.AgentProfileID {
		return fmt.Errorf("admit run execution: run %q is not the claimed owner", owner.RunID)
	}
	session, err := l.repo.GetRunSession(ctx, owner.RunSessionID)
	if err != nil {
		return fmt.Errorf("admit run execution: read session: %w", err)
	}
	if session == nil || session.RunID != owner.RunID || session.Attempt != owner.Attempt ||
		session.WorkspaceID != owner.WorkspaceID || session.AgentProfileID != owner.AgentProfileID ||
		session.CancelRequestedAt != nil ||
		(session.State != models.RunSessionStatePreparing && session.State != models.RunSessionStateRunning) {
		return fmt.Errorf("admit run execution: run session %q is stale", owner.RunSessionID)
	}
	paused, err := l.repo.GetActiveWorkspacePause(ctx, owner.WorkspaceID)
	if err != nil {
		return fmt.Errorf("admit run execution: read workspace pause: %w", err)
	}
	if paused != nil {
		return fmt.Errorf("admit run execution: workspace %q is paused", owner.WorkspaceID)
	}
	agent, err := l.repo.GetAgentInstance(ctx, owner.AgentProfileID)
	if err != nil {
		return fmt.Errorf("admit run execution: read agent: %w", err)
	}
	if agent.Status != models.AgentStatusIdle && agent.Status != models.AgentStatusWorking {
		return fmt.Errorf("admit run execution: agent %q is not eligible", owner.AgentProfileID)
	}
	return nil
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func (l *officeRunSessionLauncher) stopRecoveredRunSession(ctx context.Context, session models.RunSession) error {
	if l.recovery == nil {
		return errors.New("run recovery requires runtime inventory reconciliation")
	}
	return l.recovery.StopRunOwnerForRecovery(ctx, runtimeapi.ExecutionOwner{
		Kind: runtimeapi.ExecutionOwnerRun, WorkspaceID: session.WorkspaceID,
		RunID: session.RunID, RunSessionID: session.ID, Attempt: session.Attempt,
		AgentProfileID: session.AgentProfileID,
	})
}
