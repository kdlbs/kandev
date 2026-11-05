package backendapp

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type executionSnapshotRecorder struct {
	tasks    *taskrepo.Repository
	policies *sandbox.Service
	eventBus bus.EventBus
	changed  func()
}

var errExecutionSnapshotUnavailable = errors.New("execution snapshot is unavailable")

const queueIncarnationPayloadKey = "queue_incarnation_id"

func (r executionSnapshotRecorder) record(ctx context.Context, execution *runtime.AgentExecution, snapshot models.ExecutionSnapshot, launch *contract.Launch) error {
	if r.tasks == nil || execution == nil || snapshot.SessionID != execution.SessionID || snapshot.TaskID != execution.TaskID || snapshot.ExecutionID != execution.ID || snapshot.AgentProfileID != execution.AgentProfileID || snapshot.Sandbox != nil {
		return errExecutionSnapshotUnavailable
	}
	if (execution.SandboxSelection != nil) != (launch != nil) {
		return errExecutionSnapshotUnavailable
	}
	if launch != nil {
		if snapshot.AttemptID != launch.AttemptID {
			return errExecutionSnapshotUnavailable
		}
		captured, err := r.sandboxSnapshot(ctx, execution, launch)
		if err != nil {
			return err
		}
		snapshot.Sandbox = captured
	}
	captured, err := r.tasks.CaptureExecutionSnapshot(ctx, snapshot)
	if err != nil {
		return err
	}
	if r.changed != nil && captured.Sandbox != nil {
		r.changed()
	}
	return r.publish(ctx, captured)
}

func (r executionSnapshotRecorder) publish(ctx context.Context, captured *models.ExecutionSnapshot) error {
	if r.eventBus == nil {
		return nil
	}
	session, err := r.tasks.GetTaskSession(ctx, captured.SessionID)
	if err != nil {
		return err
	}
	if session.ExecutionSnapshot == nil || session.ExecutionSnapshot.AttemptID != captured.AttemptID {
		return errExecutionSnapshotUnavailable
	}
	task, err := r.tasks.GetTask(ctx, captured.TaskID)
	if err != nil {
		return err
	}
	payload := map[string]any{
		taskIDPayloadKey: captured.TaskID, sessionIDPayloadKey: captured.SessionID,
		workspaceIDPayloadKey: task.WorkspaceID, queueIncarnationPayloadKey: session.QueueIncarnationID,
		"execution_snapshot": captured, "execution_observation": session.ExecutionObservation,
	}
	return r.eventBus.Publish(ctx, events.TaskSessionExecutionChanged,
		bus.NewEvent(events.TaskSessionExecutionChanged, "execution", payload))
}

func (r executionSnapshotRecorder) sandboxSnapshot(ctx context.Context, execution *runtime.AgentExecution, launch *contract.Launch) (*models.ExecutionSandboxSnapshot, error) {
	if r.policies == nil {
		return nil, errExecutionSnapshotUnavailable
	}
	attempt, err := r.policies.ReadAttempt(ctx, execution.SandboxSelection.OrgID, launch.AttemptID)
	if err != nil {
		return nil, err
	}
	stored := attempt.Snapshot
	if attempt.State != sandbox.AttemptPrepared || stored.SessionID != execution.SessionID || stored.TaskID != execution.TaskID || stored.ExecutionID != execution.ID || stored.AgentProfileID != execution.AgentProfileID || stored.Launch.Generation != launch.Generation || stored.Launch.PolicyDigest != launch.PolicyDigest || stored.Launch.PackageDigest != launch.PackageDigest {
		return nil, errExecutionSnapshotUnavailable
	}
	provider, resolved := stored.Provider, stored.Resolved
	return &models.ExecutionSandboxSnapshot{
		AttemptID: attempt.ID, Generation: stored.Launch.Generation,
		InstallationID: provider.InstallationID, ProviderKey: provider.Key, ProviderVersion: provider.Version,
		PackageDigest: provider.PackageDigest, RuntimeVersion: provider.RuntimeVersion, RuntimeDigest: provider.RuntimeDigest,
		RevisionID: resolved.Revision.ID, RevisionNumber: resolved.Revision.Number, PolicyName: resolved.Revision.Policy.Name,
		PolicyDigest: resolved.Revision.Digest, SelectionSource: resolved.SelectionSource,
		CeilingRevisionID: resolved.Ceiling.ID, CeilingDigest: resolved.Ceiling.Digest,
	}, nil
}
