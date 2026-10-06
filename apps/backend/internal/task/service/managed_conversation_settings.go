package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	managed "github.com/kandev/kandev/internal/task/repository/managedconversation"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EnsureManaged creates or reconciles a retained conversation keyed by the
// host-minted installation identity. Its storage and deletion rules are
// separate from the legacy plugin-id keyed conversation lifecycle.
func (s *AgentConversationService) EnsureManaged(
	ctx context.Context,
	pluginID, installationID string,
	spec pluginsdk.ManagedAgentConversationSpec,
	operationID, payloadDigest string,
) (pluginsdk.ManagedAgentConversationDescriptor, string, error) {
	spec.AgentToolNames = append([]string(nil), spec.AgentToolNames...)
	sort.Strings(spec.AgentToolNames)
	if err := validateManagedConversationIdentity(pluginID, installationID, spec, operationID, payloadDigest); err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", err
	}
	key := managedConversationIdentity(installationID, spec.WorkspaceID, spec.InstanceKey)
	unlock := s.lockEnsureKey(key)
	defer unlock()
	prepared, usable, err := s.prepareManagedConversationSpec(ctx, spec)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", err
	}
	if !usable {
		return pluginsdk.ManagedAgentConversationDescriptor{}, AgentConversationStatusConfigurationRequired, nil
	}
	spec = prepared
	existing, err := s.findRetainedManagedConversation(ctx, installationID, spec.WorkspaceID, spec.InstanceKey)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", err
	}
	if existing != nil {
		return s.reconcileManagedConversation(ctx, pluginID, installationID, existing, spec, operationID, payloadDigest)
	}
	return s.createManagedConversation(ctx, pluginID, installationID, spec, operationID, payloadDigest)
}

func validateManagedConversationIdentity(
	pluginID, installationID string,
	spec pluginsdk.ManagedAgentConversationSpec,
	operationID, payloadDigest string,
) error {
	if pluginID == "" || installationID == "" || spec.WorkspaceID == "" || spec.InstanceKey == "" ||
		strings.TrimSpace(spec.InstanceKey) != spec.InstanceKey || len(spec.InstanceKey) > 128 || operationID == "" || payloadDigest == "" {
		return status.Error(codes.InvalidArgument, "installation, workspace, and instance key are required")
	}
	if spec.ApprovalRevision == 0 || len(spec.ManifestDigest) != 64 || mcpprofile.ValidateManagedToolNames(spec.AgentToolNames) != nil {
		return status.Error(codes.InvalidArgument, "managed conversation tool policy is invalid")
	}
	return nil
}

func (s *AgentConversationService) prepareManagedConversationSpec(
	ctx context.Context,
	spec pluginsdk.ManagedAgentConversationSpec,
) (pluginsdk.ManagedAgentConversationSpec, bool, error) {
	workspace, err := s.tasks.GetWorkspace(ctx, spec.WorkspaceID)
	if err != nil {
		return spec, false, fmt.Errorf("failed to resolve managed conversation workspace: %w", err)
	}
	if workspace == nil {
		return spec, false, status.Error(codes.NotFound, "workspace not found")
	}
	profileID, usable, err := s.resolveEffectiveProfile(ctx, spec.WorkspaceID, spec.AgentProfileID)
	spec.AgentProfileID = profileID
	return spec, usable, err
}

func (s *AgentConversationService) createManagedConversation(
	ctx context.Context, pluginID, installationID string, spec pluginsdk.ManagedAgentConversationSpec, operationID, payloadDigest string,
) (pluginsdk.ManagedAgentConversationDescriptor, string, error) {
	return s.admitManagedConversation(ctx, pluginID, installationID, spec, operationID, payloadDigest, "")
}

func (s *AgentConversationService) admitManagedConversation(
	ctx context.Context, pluginID, installationID string, spec pluginsdk.ManagedAgentConversationSpec, operationID, payloadDigest, taskID string,
) (pluginsdk.ManagedAgentConversationDescriptor, string, error) {
	task := s.newManagedConversationTask(pluginID, installationID, spec, operationID, payloadDigest)
	if taskID != "" {
		task.ID = taskID
	}
	result, err := s.tasks.EnsureManagedConversation(ctx, managed.EnsureRequest{
		Identity:      managed.Identity{TaskID: task.ID, InstallationID: installationID, WorkspaceID: spec.WorkspaceID, InstanceKey: spec.InstanceKey},
		Configuration: managed.FromTask(task), Task: task, PrimaryID: conversationPrimarySessionID(task.ID),
		ExpectedRevision: spec.ExpectedRevision, OperationID: operationID, PayloadDigest: payloadDigest,
	})
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", managedAdmissionError(err)
	}
	outcome := AgentConversationStatusExists
	if result.Replayed {
		outcome = AgentConversationStatusAlreadyApplied
	}
	if result.Created {
		outcome = AgentConversationStatusCreated
		s.publishTaskCreated(ctx, result.Task)
	} else if result.Changed {
		s.publishManagedTaskUpdated(ctx, result.Task)
	}
	return managedConversationDescriptor(installationID, result.Task, result.Primary), outcome, nil
}

func (s *AgentConversationService) publishManagedTaskUpdated(ctx context.Context, task *models.Task) {
	if s.eventer == nil {
		return
	}
	eventCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	event := bus.NewEvent(events.TaskUpdated, "agent-conversation-service", map[string]interface{}{
		"task_id": task.ID, "workspace_id": task.WorkspaceID, "title": task.Title,
		"updated_at": task.UpdatedAt.Format(time.RFC3339Nano), "is_ephemeral": true,
	})
	_ = s.eventer.Publish(eventCtx, events.TaskUpdated, event)
}

func managedAdmissionError(err error) error {
	switch {
	case errors.Is(err, managed.ErrRevision):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, managed.ErrBusy), errors.Is(err, repoerrors.ErrTaskCleanupInProgress), errors.Is(err, repoerrors.ErrTaskHierarchyConflict), errors.Is(err, recoveryclaim.ErrBusy):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, managed.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, managed.ErrUnavailable):
		return status.Error(codes.Unavailable, err.Error())
	default:
		return err
	}
}

// GetManaged returns a conversation only when both its installation and
// workspace match the request.
func (s *AgentConversationService) GetManaged(
	ctx context.Context, installationID, workspaceID, instanceKey string,
) (pluginsdk.ManagedAgentConversationDescriptor, error) {
	task, err := s.findRetainedManagedConversation(ctx, installationID, workspaceID, instanceKey)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, err
	}
	if task == nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.NotFound, "managed conversation not found")
	}
	if managedConversationDetached(task) {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.NotFound, "managed conversation not found")
	}
	primary, err := s.sess.GetPrimarySessionByTaskID(ctx, task.ID)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, err
	}
	return managedConversationDescriptor(installationID, task, primary), nil
}

// ListManaged returns installation-owned conversations for one workspace in
// stable task-id order.
func (s *AgentConversationService) ListManaged(
	ctx context.Context, installationID, workspaceID string,
) ([]pluginsdk.ManagedAgentConversationDescriptor, error) {
	if installationID == "" || workspaceID == "" {
		return nil, status.Error(codes.InvalidArgument, "installation and workspace are required")
	}
	tasks, err := s.listRetainedManagedConversations(ctx, installationID, workspaceID)
	if err != nil {
		return nil, err
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	out := make([]pluginsdk.ManagedAgentConversationDescriptor, 0, len(tasks))
	for _, task := range tasks {
		if managedConversationDetached(task) {
			continue
		}
		primary, err := s.sess.GetPrimarySessionByTaskID(ctx, task.ID)
		if errors.Is(err, taskrepo.ErrNoPrimarySession) {
			primary = nil
		} else if err != nil {
			return nil, err
		}
		out = append(out, managedConversationDescriptor(installationID, task, primary))
	}
	return out, nil
}

// SetManagedPaused changes only the desired admission state. Stopping a live
// generation is a separate execution-control operation.
func (s *AgentConversationService) SetManagedPaused(
	ctx context.Context, installationID, workspaceID, instanceKey string, expectedRevision uint64, paused bool, operationID, payloadDigest string,
) (pluginsdk.ManagedAgentConversationDescriptor, error) {
	if operationID == "" || payloadDigest == "" {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.InvalidArgument, "managed conversation operation identity is required")
	}
	unlock := s.lockEnsureKey(managedConversationIdentity(installationID, workspaceID, instanceKey))
	defer unlock()
	task, err := s.findRetainedManagedConversation(ctx, installationID, workspaceID, instanceKey)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, err
	}
	if task == nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.NotFound, "managed conversation not found")
	}
	result, err := s.tasks.ChangeManagedConversationState(ctx, managed.StateRequest{
		Identity: managedTaskIdentity(task), Kind: managed.PauseExact, Paused: paused, ExpectedRevision: expectedRevision,
		OperationID: operationID, PayloadDigest: payloadDigest, PrimaryID: conversationPrimarySessionID(task.ID),
	})
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, managedAdmissionError(err)
	}
	if !result.Replayed {
		s.publishManagedTaskUpdated(ctx, result.Task)
	}
	if !paused && result.Primary != nil {
		s.notifyManagedInputQueue(ctx, task.ID, result.Primary.ID)
	}
	return managedConversationDescriptor(installationID, result.Task, result.Primary), nil
}

func managedTaskIdentity(task *models.Task) managed.Identity {
	return managed.Identity{TaskID: task.ID, InstallationID: models.StringFromAny(task.Metadata[metaKeyManagedInstall]),
		WorkspaceID: task.WorkspaceID, InstanceKey: models.StringFromAny(task.Metadata[metaKeyManagedInstance])}
}

// DeleteManaged removes one retained conversation only at the expected
// revision. Host lifecycle cleanup deliberately does not call this method.
func (s *AgentConversationService) DeleteManaged(
	ctx context.Context, installationID, workspaceID, instanceKey string, expectedRevision uint64, operationID, payloadDigest string,
) error {
	unlock := s.lockEnsureKey(managedConversationIdentity(installationID, workspaceID, instanceKey))
	defer unlock()
	deleter, ok := s.getTaskDeleter().(interface {
		DeleteManagedConversationTask(context.Context, managed.DeleteRequest) error
	})
	native, nativeOK := s.tasks.(managed.DeletionRepository)
	if !ok || !nativeOK {
		return managedAdmissionError(managed.ErrUnavailable)
	}
	if operationID == "" || payloadDigest == "" {
		operationID, payloadDigest = uuid.NewString(), uuid.NewString()
	}
	request := managed.DeleteRequest{Identity: managed.Identity{InstallationID: installationID, WorkspaceID: workspaceID, InstanceKey: instanceKey}, ExpectedRevision: expectedRevision, OperationID: operationID, PayloadDigest: payloadDigest}
	previous, job, err := native.InspectManagedDeletion(ctx, request)
	if err != nil {
		return &ManagedDeletionError{Outcome: "outcome_uncertain", Cause: err}
	}
	if previous != nil && (previous.Phase == managed.DeleteCommitted || job.State != models.TaskResourceCleanupStateCancelled) {
		return deleter.DeleteManagedConversationTask(ctx, previous.DeleteRequest)
	}
	task, err := s.findRetainedManagedConversation(ctx, installationID, workspaceID, instanceKey)
	if err != nil {
		return managedHistoryError(previous, err)
	}
	if task == nil || managedConversationDetached(task) {
		return managedHistoryError(previous, managedAdmissionError(managed.ErrNotFound))
	}
	request.Identity = managedTaskIdentity(task)
	request.TaskCreatedAt = task.CreatedAt
	return deleter.DeleteManagedConversationTask(ctx, request)
}

// PauseManagedForInstallation blocks new turns and stops any current execution
// while preserving queued work and the host-owned transcript.
func (s *AgentConversationService) PauseManagedForInstallation(ctx context.Context, installationID string) error {
	if installationID == "" {
		return status.Error(codes.InvalidArgument, "installation_id is required")
	}
	return s.changeManagedInstallationState(ctx, installationID, "", managed.PauseInstallation)
}

// InvalidateManagedForInstallationWorkspace blocks turns after policy revocation.
func (s *AgentConversationService) InvalidateManagedForInstallationWorkspace(ctx context.Context, installationID, workspaceID string) error {
	if installationID == "" || workspaceID == "" {
		return status.Error(codes.InvalidArgument, "installation_id and workspace_id are required")
	}
	return s.changeManagedInstallationState(ctx, installationID, workspaceID, managed.Invalidate)
}

// DetachManagedForInstallation retains a paused host-owned transcript.
func (s *AgentConversationService) DetachManagedForInstallation(ctx context.Context, installationID string) error {
	if installationID == "" {
		return status.Error(codes.InvalidArgument, "installation_id is required")
	}
	return s.changeManagedInstallationState(ctx, installationID, "", managed.Detach)
}

func (s *AgentConversationService) changeManagedInstallationState(ctx context.Context, installationID, workspaceID string, kind managed.StateKind) error {
	tasks, err := s.tasks.ListEphemeralTasksAllWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("failed to list retained managed conversations: %w", err)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	var failures []error
	for _, candidate := range tasks {
		if candidate == nil || candidate.Metadata == nil {
			continue
		}
		identity := managedTaskIdentity(candidate)
		if workspaceID != "" && identity.WorkspaceID != workspaceID {
			continue
		}
		if !isRetainedManagedConversation(candidate, installationID, identity.WorkspaceID, identity.InstanceKey) {
			continue
		}
		err := s.changeManagedLifecycleTask(ctx, identity, kind)
		if err == nil {
			continue
		}
		if kind != managed.Invalidate {
			return err
		}
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func (s *AgentConversationService) changeManagedLifecycleTask(ctx context.Context, identity managed.Identity, kind managed.StateKind) error {
	unlock := s.lockEnsureKey(managedConversationIdentity(identity.InstallationID, identity.WorkspaceID, identity.InstanceKey))
	defer unlock()
	result, err := s.tasks.ChangeManagedConversationState(ctx, managed.StateRequest{Identity: identity, Kind: kind})
	if errors.Is(err, managed.ErrNotFound) {
		return nil
	}
	if err != nil {
		return managedAdmissionError(err)
	}
	if result.Changed {
		s.publishManagedTaskUpdated(ctx, result.Task)
	}
	return s.stopManagedConversationExecution(ctx, result.Task)
}

func (s *AgentConversationService) stopManagedConversationExecution(ctx context.Context, task *models.Task) error {
	primary, err := s.sess.GetPrimarySessionByTaskID(ctx, task.ID)
	if errors.Is(err, taskrepo.ErrNoPrimarySession) || (err == nil && primary == nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to inspect managed conversation session %s: %w", task.ID, err)
	}
	if primary.State != models.TaskSessionStateRunning && primary.State != models.TaskSessionStateStarting && primary.AgentExecutionID == "" {
		return nil
	}
	s.mu.RLock()
	stop := s.managedExecutionStopper
	s.mu.RUnlock()
	if stop == nil {
		return status.Error(codes.Unavailable, "managed conversation execution stopper is unavailable")
	}
	if err := stop(ctx, task.ID); err != nil {
		return fmt.Errorf("failed to stop managed conversation execution %s: %w", task.ID, err)
	}
	return nil
}

func (s *AgentConversationService) newManagedConversationTask(
	pluginID, installationID string, spec pluginsdk.ManagedAgentConversationSpec, operationID, payloadDigest string,
) *models.Task {
	metadata := map[string]interface{}{
		metaKeyPluginID: pluginID, metaKeyWorkspaceID: spec.WorkspaceID,
		metaKeyConversationKey: spec.InstanceKey, metaKeyEphemeral: true,
		metaKeyManagedByPlugin: pluginID, metaKeyManagedRetained: true,
		metaKeyManagedInstall: installationID, metaKeyManagedInstance: spec.InstanceKey,
		metaKeyManagedRevision: "1", metaKeyManagedPaused: false, models.MetaKeyManagedConversationDetached: false,
		metaKeyManagedApprovalRevision:  strconv.FormatUint(spec.ApprovalRevision, 10),
		metaKeyManagedManifestDigest:    spec.ManifestDigest,
		metaKeyManagedToolNames:         append([]string(nil), spec.AgentToolNames...),
		metaKeyRetentionMode:            managedConversationRetentionMode,
		metaKeyManagedOperation:         operationID,
		metaKeyManagedPayload:           payloadDigest,
		models.MetaKeyAgentProfileID:    spec.AgentProfileID,
		models.MetaKeyExecutorID:        spec.ExecutorID,
		models.MetaKeyExecutorProfileID: spec.ExecutorProfileID,
		metaKeyInstructionVer:           spec.InstructionVersion,
	}
	if spec.BasePrompt != "" {
		metadata["kandev.base_prompt"] = spec.BasePrompt
	}
	return &models.Task{
		ID:          managedConversationTaskID(installationID, spec.WorkspaceID, spec.InstanceKey),
		WorkspaceID: spec.WorkspaceID,
		Title:       defaultAgentConversationTitle + " - " + spec.InstanceKey,
		State:       v1.TaskStateCreated, Priority: "medium", IsEphemeral: true,
		Origin: models.TaskOriginManual, Metadata: metadata,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
}

func (s *AgentConversationService) reconcileManagedConversation(
	ctx context.Context, pluginID, installationID string, task *models.Task, spec pluginsdk.ManagedAgentConversationSpec, operationID, payloadDigest string,
) (pluginsdk.ManagedAgentConversationDescriptor, string, error) {
	// Discovery is advisory. The repository admits against its locked current rows.
	if _, err := s.managedConversationPrimary(ctx, task.ID); err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", err
	}
	return s.admitManagedConversation(ctx, pluginID, installationID, spec, operationID, payloadDigest, task.ID)
}

func (s *AgentConversationService) managedConversationPrimary(ctx context.Context, taskID string) (*models.TaskSession, error) {
	primary, err := s.sess.GetPrimarySessionByTaskID(ctx, taskID)
	if errors.Is(err, taskrepo.ErrNoPrimarySession) {
		return nil, nil
	}
	return primary, err
}

func (s *AgentConversationService) findRetainedManagedConversation(
	ctx context.Context, installationID, workspaceID, instanceKey string,
) (*models.Task, error) {
	var found *models.Task
	err := s.eachEphemeralTask(ctx, workspaceID, func(task *models.Task) bool {
		if isRetainedManagedConversation(task, installationID, workspaceID, instanceKey) {
			found = task
			return false
		}
		return true
	})
	return found, err
}

func (s *AgentConversationService) listRetainedManagedConversations(
	ctx context.Context, installationID, workspaceID string,
) ([]*models.Task, error) {
	var found []*models.Task
	err := s.eachEphemeralTask(ctx, workspaceID, func(task *models.Task) bool {
		if isRetainedManagedConversation(task, installationID, workspaceID, "") {
			found = append(found, task)
		}
		return true
	})
	return found, err
}

func (s *AgentConversationService) eachEphemeralTask(ctx context.Context, workspaceID string, visit func(*models.Task) bool) error {
	if workspaceID == "" {
		return status.Error(codes.InvalidArgument, "workspace_id is required")
	}
	for page := 1; ; page++ {
		tasks, total, err := s.tasks.ListTasksByWorkspace(ctx, workspaceID, "", "", "", page, managedConversationPageSize, "", false, true, true, false)
		if err != nil {
			return err
		}
		for _, task := range tasks {
			if !visit(task) {
				return nil
			}
		}
		if len(tasks) < managedConversationPageSize || page*managedConversationPageSize >= total {
			return nil
		}
	}
}

func managedConversationDescriptor(
	installationID string, task *models.Task, session *models.TaskSession,
) pluginsdk.ManagedAgentConversationDescriptor {
	metadata := task.Metadata
	descriptor := pluginsdk.ManagedAgentConversationDescriptor{
		InstallationID: installationID, TaskID: task.ID, WorkspaceID: task.WorkspaceID,
		InstanceKey:        models.StringFromAny(metadata[metaKeyManagedInstance]),
		Revision:           managedConversationRevision(task),
		AgentProfileID:     models.StringFromAny(metadata[models.MetaKeyAgentProfileID]),
		ExecutorID:         models.StringFromAny(metadata[models.MetaKeyExecutorID]),
		ExecutorProfileID:  models.StringFromAny(metadata[models.MetaKeyExecutorProfileID]),
		BasePrompt:         models.StringFromAny(metadata["kandev.base_prompt"]),
		InstructionVersion: models.StringFromAny(metadata[metaKeyInstructionVer]),
		DesiredPaused:      managedConversationPaused(task),
		RetentionMode:      models.StringFromAny(metadata[metaKeyRetentionMode]),
		Detached:           managedConversationDetached(task),
		AgentToolNames:     managedConversationToolNames(metadata[metaKeyManagedToolNames]),
	}
	if session != nil {
		descriptor.SessionID = session.ID
	}
	return descriptor
}

func managedConversationUint(value any) uint64 {
	parsed, _ := strconv.ParseUint(models.StringFromAny(value), 10, 64)
	return parsed
}

func managedConversationToolNames(value any) []string {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var names []string
	if err := json.Unmarshal(encoded, &names); err != nil {
		return nil
	}
	sort.Strings(names)
	return names
}

func managedConversationRevision(task *models.Task) uint64 {
	revision, err := strconv.ParseUint(models.StringFromAny(task.Metadata[metaKeyManagedRevision]), 10, 64)
	if err != nil || revision == 0 {
		return 1
	}
	return revision
}

func managedConversationPaused(task *models.Task) bool {
	paused, _ := task.Metadata[metaKeyManagedPaused].(bool)
	return paused
}

func managedConversationDetached(task *models.Task) bool {
	detached, _ := task.Metadata[metaKeyManagedDetached].(bool)
	return detached
}

func managedConversationTaskID(installationID, workspaceID, instanceKey string) string {
	return conversationIdentity("managed-task", installationID, workspaceID, instanceKey)
}

func managedConversationIdentity(installationID, workspaceID, instanceKey string) string {
	return installationID + "/" + workspaceID + "/" + instanceKey
}

func isRetainedManagedConversation(task *models.Task, installationID, workspaceID, instanceKey string) bool {
	if task == nil || task.Metadata == nil || installationID == "" || workspaceID == "" {
		return false
	}
	retained, _ := task.Metadata[metaKeyManagedRetained].(bool)
	ephemeral, _ := task.Metadata[metaKeyEphemeral].(bool)
	installedBy, _ := task.Metadata[metaKeyManagedInstall].(string)
	workspace, _ := task.Metadata[metaKeyWorkspaceID].(string)
	instance, _ := task.Metadata[metaKeyManagedInstance].(string)
	return retained && ephemeral && installedBy == installationID && workspace == workspaceID &&
		(instanceKey == "" || instance == instanceKey)
}

func managedConversationSessionIdle(primary *models.TaskSession) bool {
	return primary == nil || (primary.State != models.TaskSessionStateRunning && primary.State != models.TaskSessionStateStarting && primary.AgentExecutionID == "")
}
