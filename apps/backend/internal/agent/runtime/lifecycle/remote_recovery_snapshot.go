package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// RemoteRecoverySnapshot pins the durable ownership identity and current
// eligibility of a remote execution during one reconnect attempt.
type RemoteRecoverySnapshot struct {
	TaskID                         string
	SessionID                      string
	SessionState                   models.TaskSessionState
	AgentExecutionID               string
	Runtime                        string
	IsPassthrough                  bool
	RouteState                     string
	RouteGeneration                int64
	TaskWorkspaceID                string
	TaskArchived                   bool
	TaskOrigin                     string
	WorkspaceOwnerID               string
	WorkspaceOrgID                 string
	TaskEnvironmentID              string
	EnvironmentOwnerTaskID         string
	EnvironmentOwnershipGen        int64
	EnvironmentExecutorType        string
	EnvironmentExecutorID          string
	EnvironmentStatus              string
	EnvironmentOwnerArchived       bool
	EnvironmentOwnerCleanupRunning bool
	TaskResourceCleanupRunning     bool
	RecoverySourcePresent          bool
	RecoverableDeliveryPending     bool
	RecoveryExecutionID            string
	RecoverySubmissionID           string
	RecoveryStreamID               string
	RecoveryIncarnationID          string
	RecoveryHarnessGeneration      int64
	RecoveryRevision               int64
}

// RemoteRecoverySnapshotProvider reads task and workspace ownership without
// materializing, repairing, or otherwise changing the execution environment.
type RemoteRecoverySnapshotProvider interface {
	GetRemoteRecoverySnapshot(ctx context.Context, sessionID string) (*RemoteRecoverySnapshot, error)
}

func validateRemoteRecoverySnapshot(snapshot *RemoteRecoverySnapshot, record *models.ExecutorRunning) error {
	if err := validateRemoteRecoverySnapshotIdentity(snapshot, record); err != nil {
		return err
	}
	if err := validateRemoteRecoverySnapshotOwner(snapshot); err != nil {
		return err
	}
	if err := validateRemoteRecoverySnapshotEligibility(snapshot); err != nil {
		return err
	}
	return validateRemoteRecoverySnapshotSession(snapshot)
}

func validateRemoteRecoverySnapshotIdentity(snapshot *RemoteRecoverySnapshot, record *models.ExecutorRunning) error {
	if snapshot == nil || record == nil || record.SessionID == "" || record.TaskID == "" || record.AgentExecutionID == "" {
		return errors.New("remote recovery identity is incomplete")
	}
	if snapshot.SessionID != record.SessionID || snapshot.TaskID != record.TaskID ||
		snapshot.AgentExecutionID != record.AgentExecutionID || snapshot.Runtime != string(record.Runtime) {
		return errors.New("persisted remote execution identity changed")
	}
	return nil
}

func validateRemoteRecoverySnapshotOwner(snapshot *RemoteRecoverySnapshot) error {
	if snapshot.TaskWorkspaceID == "" || snapshot.TaskEnvironmentID == "" ||
		snapshot.EnvironmentOwnerTaskID == "" || snapshot.EnvironmentOwnershipGen <= 0 ||
		snapshot.EnvironmentExecutorType == "" || snapshot.EnvironmentExecutorID == "" {
		return errors.New("remote recovery owner snapshot is incomplete")
	}
	if !snapshot.RecoverySourcePresent {
		return nil
	}
	if snapshot.RecoveryExecutionID != snapshot.AgentExecutionID || snapshot.RecoverySubmissionID == "" ||
		snapshot.RecoveryStreamID == "" || snapshot.RecoveryIncarnationID == "" ||
		snapshot.RecoveryHarnessGeneration <= 0 || snapshot.RecoveryRevision <= 0 {
		return errors.New("remote recovery delivery snapshot is incomplete")
	}
	return nil
}

func validateRemoteRecoverySnapshotEligibility(snapshot *RemoteRecoverySnapshot) error {
	if snapshot.TaskArchived || snapshot.EnvironmentOwnerArchived || snapshot.TaskResourceCleanupRunning ||
		snapshot.EnvironmentOwnerCleanupRunning {
		return errors.New("remote recovery owner is archived or being cleaned up")
	}
	if snapshot.EnvironmentStatus != "ready" {
		return fmt.Errorf("remote recovery environment is %q", snapshot.EnvironmentStatus)
	}
	switch snapshot.RouteState {
	case "", "active", "action_required":
		return nil
	default:
		return fmt.Errorf("remote recovery route is %q", snapshot.RouteState)
	}
}

func validateRemoteRecoverySnapshotSession(snapshot *RemoteRecoverySnapshot) error {
	switch snapshot.SessionState {
	case models.TaskSessionStateCompleted, models.TaskSessionStateCancelled:
		return fmt.Errorf("remote recovery session is terminal: %s", snapshot.SessionState)
	case models.TaskSessionStateFailed:
		if !snapshot.RecoverySourcePresent {
			return fmt.Errorf("failed remote recovery session has no pending delivery proof")
		}
	}
	return nil
}

func sameRemoteRecoveryOwner(initial, current *RemoteRecoverySnapshot, record *models.ExecutorRunning) bool {
	if initial == nil || current == nil || record == nil {
		return false
	}
	return sameRemoteRecoveryIdentity(initial, current) &&
		sameRemoteRecoveryWorkspace(initial, current) &&
		sameRemoteRecoveryEnvironment(initial, current) &&
		sameRemoteRecoveryRoute(initial, current) &&
		sameRemoteRecoveryDelivery(initial, current)
}

func sameRemoteRecoveryIdentity(initial, current *RemoteRecoverySnapshot) bool {
	return initial.TaskID == current.TaskID && initial.SessionID == current.SessionID &&
		initial.AgentExecutionID == current.AgentExecutionID && initial.Runtime == current.Runtime
}

func sameRemoteRecoveryWorkspace(initial, current *RemoteRecoverySnapshot) bool {
	return initial.TaskWorkspaceID == current.TaskWorkspaceID && initial.TaskOrigin == current.TaskOrigin &&
		initial.WorkspaceOwnerID == current.WorkspaceOwnerID && initial.WorkspaceOrgID == current.WorkspaceOrgID
}

func sameRemoteRecoveryEnvironment(initial, current *RemoteRecoverySnapshot) bool {
	return initial.TaskEnvironmentID == current.TaskEnvironmentID &&
		initial.EnvironmentOwnerTaskID == current.EnvironmentOwnerTaskID &&
		initial.EnvironmentOwnershipGen == current.EnvironmentOwnershipGen &&
		initial.EnvironmentExecutorType == current.EnvironmentExecutorType &&
		initial.EnvironmentExecutorID == current.EnvironmentExecutorID
}

func sameRemoteRecoveryRoute(initial, current *RemoteRecoverySnapshot) bool {
	return initial.RouteGeneration == current.RouteGeneration && initial.IsPassthrough == current.IsPassthrough
}

func sameRemoteRecoveryDelivery(initial, current *RemoteRecoverySnapshot) bool {
	return initial.RecoverySourcePresent == current.RecoverySourcePresent &&
		initial.RecoveryExecutionID == current.RecoveryExecutionID &&
		initial.RecoverySubmissionID == current.RecoverySubmissionID &&
		initial.RecoveryStreamID == current.RecoveryStreamID &&
		initial.RecoveryIncarnationID == current.RecoveryIncarnationID &&
		initial.RecoveryHarnessGeneration == current.RecoveryHarnessGeneration
}

func remoteRecoverySnapshotMatchesRecord(snapshot *RemoteRecoverySnapshot, record *models.ExecutorRunning) bool {
	return snapshot != nil && record != nil && snapshot.SessionID == record.SessionID &&
		snapshot.TaskID == record.TaskID && snapshot.AgentExecutionID == record.AgentExecutionID &&
		snapshot.Runtime == string(record.Runtime)
}

func remoteRecoverySnapshotFences(snapshot *RemoteRecoverySnapshot) bool {
	if snapshot == nil || snapshot.TaskArchived || snapshot.EnvironmentOwnerArchived ||
		snapshot.TaskResourceCleanupRunning || snapshot.EnvironmentOwnerCleanupRunning {
		return true
	}
	switch snapshot.SessionState {
	case models.TaskSessionStateCompleted, models.TaskSessionStateCancelled:
		return true
	}
	return false
}

func (m *Manager) pinRemoteRecoverySnapshot(entry *remoteRecoveryRetry, snapshot *RemoteRecoverySnapshot) bool {
	if entry == nil || entry.record == nil || snapshot == nil {
		return false
	}
	m.remoteRecoveryMu.Lock()
	defer m.remoteRecoveryMu.Unlock()
	if m.remoteRecoveryPending[entry.record.SessionID] != entry || entry.snapshot != nil {
		return false
	}
	copySnapshot := *snapshot
	entry.snapshot = &copySnapshot
	return true
}
