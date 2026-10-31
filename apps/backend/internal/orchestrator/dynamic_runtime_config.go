package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type dynamicCandidateSnapshotUpdater interface {
	UpdateTaskSessionIfCurrentSnapshot(context.Context, *models.TaskSession, models.TaskSessionState, time.Time, map[string]interface{}) (bool, error)
}

// DynamicCandidateRuntimeResetRequired reports whether a route projection moves
// a session from one concrete candidate profile to a different one. The first
// selection and a same-candidate retry keep their runtime settings.
func DynamicCandidateRuntimeResetRequired(previousProfileID, nextProfileID string) bool {
	return previousProfileID != "" && nextProfileID != "" && previousProfileID != nextProfileID
}

// PersistDynamicCandidateSwitch persists a route projection that selected a new
// concrete candidate, together with the candidate runtime reset. It reports
// false when the session changed since expectedState/expectedUpdatedAt.
func (s *Service) PersistDynamicCandidateSwitch(
	ctx context.Context, session *models.TaskSession,
	expectedState models.TaskSessionState, expectedUpdatedAt time.Time,
) (bool, error) {
	settingsLock := s.sessionSettingsSnapshotLock(session.ID)
	settingsLock.Lock()
	defer settingsLock.Unlock()
	return s.persistDynamicCandidateRuntimeReset(ctx, session, expectedState, expectedUpdatedAt)
}

// persistDynamicRouteProjection writes a route projection applied to session.
// A candidate switch also drops the previous provider's runtime settings.
func (s *Service) persistDynamicRouteProjection(
	ctx context.Context, session *models.TaskSession, previousProfileID string,
	expectedState models.TaskSessionState, expectedUpdatedAt time.Time,
) error {
	if !DynamicCandidateRuntimeResetRequired(previousProfileID, session.ExecutionProfileID) {
		return s.repo.UpdateTaskSession(ctx, session)
	}
	changed, err := s.PersistDynamicCandidateSwitch(ctx, session, expectedState, expectedUpdatedAt)
	if err != nil {
		return err
	}
	if !changed {
		return dynamicruntime.ErrStaleGeneration
	}
	return nil
}

// persistDynamicCandidateRuntimeReset is called with the settings snapshot lock
// held. The model, mode, and ACP options observed from the previous concrete
// provider are dropped so the next candidate does not launch with them. Explicit
// user overrides survive, and the current workflow step's declared mode is
// restored. The write is guarded by the session revision.
func (s *Service) persistDynamicCandidateRuntimeReset(
	ctx context.Context, session *models.TaskSession,
	expectedState models.TaskSessionState, expectedUpdatedAt time.Time,
) (bool, error) {
	updater, ok := s.repo.(dynamicCandidateSnapshotUpdater)
	if !ok {
		return false, errors.New("dynamic candidate switch requires guarded metadata updates")
	}
	mode, err := s.dynamicCandidateWorkflowMode(ctx, session.TaskID)
	if err != nil {
		return false, err
	}
	session.Metadata = cloneSessionMetadata(session.Metadata)
	delete(session.Metadata, models.SessionMetaKeyRuntimeConfig)
	delete(session.Metadata, models.SessionMetaKeySessionMode)
	if mode != "" {
		session.Metadata[models.SessionMetaKeySessionMode] = mode
	}
	changed, err := updater.UpdateTaskSessionIfCurrentSnapshot(ctx, session, expectedState, expectedUpdatedAt, session.Metadata)
	if err != nil {
		return false, fmt.Errorf("persist dynamic candidate runtime reset: %w", err)
	}
	return changed, nil
}

// dynamicCandidateWorkflowMode returns the session mode declared by the task's
// current workflow step, or "" when the step declares none.
func (s *Service) dynamicCandidateWorkflowMode(ctx context.Context, taskID string) (string, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("read dynamic candidate task: %w", err)
	}
	if task == nil || task.WorkflowStepID == "" || s.workflowStepGetter == nil {
		return "", nil
	}
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil {
		return "", fmt.Errorf("read dynamic candidate workflow step: %w", err)
	}
	if step == nil {
		return "", nil
	}
	var mode string
	for _, action := range step.Events.OnEnter {
		if action.Type != wfmodels.OnEnterSetSessionMode {
			continue
		}
		if declared, ok := action.Config["mode"].(string); ok && declared != "" {
			mode = declared
		}
	}
	return mode, nil
}
