package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

type workspaceRecoveryErrorReporter interface {
	ReportManagedCloneRelocationRequired(
		context.Context,
		models.WorkspaceRecoveryErrorObservation,
	) (string, error)
}

type workspaceRecoveryStatusReader interface {
	WorkspaceRecoveryProjection(context.Context, string) (*models.TaskEnvironmentRecoveryOperation, bool, error)
}

// SetWorkspaceRecoveryErrorReporter wires task-service ownership for durable
// relocation errors discovered by manual resume preflight.
func (s *Service) SetWorkspaceRecoveryErrorReporter(reporter workspaceRecoveryErrorReporter) {
	s.workspaceRecoveryErrorReporter = reporter
}

// SetWorkspaceRecoveryStatusReader wires the read-only environment projection
// used by status queries and duplicate recovery requests.
func (s *Service) SetWorkspaceRecoveryStatusReader(reader workspaceRecoveryStatusReader) {
	s.workspaceRecoveryStatusReader = reader
}

// GetWorkspaceRecoveryStatus authorizes a task/session pair and reads only the
// selected environment's durable projection. It never initializes an agent or
// inspects a checkout.
func (s *Service) GetWorkspaceRecoveryStatus(
	ctx context.Context,
	taskID, sessionID string,
) (*models.TaskEnvironmentRecoveryOperation, bool, error) {
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return nil, false, err
	}
	if s.repo == nil {
		return nil, false, nil
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, false, err
	}
	if session == nil || session.TaskID != taskID {
		return nil, false, ErrTaskSessionPairMismatch
	}
	environmentID := session.TaskEnvironmentID
	if environmentID == "" {
		environment, envErr := s.repo.GetTaskEnvironmentByTaskID(ctx, taskID)
		if envErr != nil {
			return nil, false, envErr
		}
		if environment != nil {
			environmentID = environment.ID
		}
	}
	if environmentID == "" || s.workspaceRecoveryStatusReader == nil {
		return nil, false, nil
	}
	return s.workspaceRecoveryStatusReader.WorkspaceRecoveryProjection(ctx, environmentID)
}
