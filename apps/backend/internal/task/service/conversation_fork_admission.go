package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (s *Service) prepareTaskConversationForkAdmission(
	ctx context.Context,
	req *CreateTaskRequest,
) (*models.ConversationForkAdmission, *CreateTaskResult, bool, error) {
	if req.ConversationForkID == "" && req.ConversationForkRequestID == "" {
		return nil, nil, false, nil
	}
	admission, err := s.newTaskConversationForkAdmission(ctx, req)
	if err != nil {
		return nil, nil, true, err
	}
	if result, found, err := s.findConversationForkTaskRetry(ctx, &admission); found || err != nil {
		return &admission, &result, true, err
	}
	draft, err := s.getTaskConversationForkDraft(ctx, admission)
	if err != nil {
		return nil, nil, true, err
	}
	if err := s.validateTaskConversationForkDestination(ctx, req, draft); err != nil {
		return nil, nil, true, err
	}
	return &admission, nil, true, nil
}

func (s *Service) newTaskConversationForkAdmission(ctx context.Context, req *CreateTaskRequest) (models.ConversationForkAdmission, error) {
	if req.ConversationForkID == "" || req.ConversationForkRequestID == "" || len(req.ConversationForkRequestID) > 128 {
		return models.ConversationForkAdmission{}, models.ErrConversationForkConflict
	}
	ownerID, err := s.conversationForkOwnerID(ctx, req.WorkspaceID)
	if err != nil {
		return models.ConversationForkAdmission{}, models.ErrConversationForkNotFound
	}
	fingerprint, err := taskForkDestinationFingerprint(req)
	if err != nil {
		return models.ConversationForkAdmission{}, err
	}
	kind := "task"
	if req.ParentID != "" {
		kind = "child_task"
	}
	return models.ConversationForkAdmission{
		OwnerID: ownerID, WorkspaceID: req.WorkspaceID, ForkID: req.ConversationForkID,
		DestinationKind: kind, DestinationRequestID: req.ConversationForkRequestID, RequestFingerprint: fingerprint,
	}, nil
}

func (s *Service) getTaskConversationForkDraft(ctx context.Context, admission models.ConversationForkAdmission) (models.ConversationForkDraft, error) {
	drafts, ok := s.messages.(taskrepo.ConversationForkDraftRepository)
	if !ok {
		return models.ConversationForkDraft{}, models.ErrConversationForkSourceUnavailable
	}
	draft, err := drafts.GetConversationForkDraft(ctx, admission.OwnerID, admission.ForkID, time.Now().UTC())
	if err != nil {
		return models.ConversationForkDraft{}, err
	}
	return draft, nil
}

func (s *Service) validateTaskConversationForkDestination(ctx context.Context, req *CreateTaskRequest, draft models.ConversationForkDraft) error {
	if draft.Descriptor.State != conversationForkStateDraft {
		return models.ErrConversationForkConflict
	}
	if err := s.AuthorizeSessionAccess(ctx, draft.Descriptor.SourceSessionID); err != nil {
		return err
	}
	if draft.WorkspaceID != req.WorkspaceID {
		return models.ErrConversationForkNotFound
	}
	if req.IsEphemeral || req.ProjectID != "" || req.ParentID != "" && req.ParentID != draft.Descriptor.SourceTaskID {
		return models.ErrConversationForkUnsupportedDestination
	}
	if err := validateTaskConversationForkAttachments(draft.Descriptor.AttachmentDescriptors, req.InitialAttachments); err != nil {
		return err
	}
	if err := validateTaskConversationForkWorkspaceMode(s, ctx, req); err != nil {
		return err
	}
	if err := s.prepareWorkspacePolicyForCreation(ctx, req); err != nil {
		return err
	}
	return nil
}

func validateTaskConversationForkAttachments(forkAttachments []models.ConversationForkAttachment, newAttachments []v1.MessageAttachment) error {
	if len(forkAttachments)+len(newAttachments) > models.MaxMessageAttachmentCount {
		return models.ErrConversationForkLimitExceeded
	}
	var total int64
	for _, attachment := range forkAttachments {
		if attachment.Size < 0 || attachment.Size > models.MaxMessageAttachmentBytes-total {
			return models.ErrConversationForkLimitExceeded
		}
		total += attachment.Size
	}
	for _, attachment := range newAttachments {
		if attachment.SizeBytes < 0 || attachment.SizeBytes > models.MaxMessageAttachmentBytes-total {
			return models.ErrConversationForkLimitExceeded
		}
		total += attachment.SizeBytes
	}
	return nil
}

func validateTaskConversationForkWorkspaceMode(s *Service, ctx context.Context, req *CreateTaskRequest) error {
	mode := workspaceModeForForkRequest(req)
	if mode == workspaceModeSharedGroup || req.ParentID == "" && mode == workspaceModeInheritParent {
		return models.ErrConversationForkUnsupportedDestination
	}
	if req.ParentID == "" || mode == "" || mode == workspaceModeNewWorkspace {
		if !s.conversationForkExecutorCanIsolateWorkspace(ctx, req) {
			return models.ErrConversationForkUnsupportedDestination
		}
	}
	return nil
}

func (s *Service) conversationForkExecutorCanIsolateWorkspace(ctx context.Context, req *CreateTaskRequest) bool {
	executorID, ok := s.resolveConversationForkExecutorID(ctx, req)
	if !ok || executorID == models.ExecutorIDLocal || s.executors == nil {
		return false
	}
	executor, err := s.executors.GetExecutor(ctx, executorID)
	return err == nil && executor != nil && executor.Status == models.ExecutorStatusActive && conversationForkExecutorTypeCanIsolate(executor.Type)
}

func (s *Service) resolveConversationForkExecutorID(ctx context.Context, req *CreateTaskRequest) (string, bool) {
	executorID := strings.TrimSpace(req.ExecutorID)
	executorProfileID := strings.TrimSpace(req.ExecutorProfileID)
	if executorID == "" {
		executorID = conversationForkMetadataString(req.Metadata, models.MetaKeyExecutorID)
	}
	if executorProfileID == "" {
		executorProfileID = conversationForkMetadataString(req.Metadata, models.MetaKeyExecutorProfileID)
	}
	if executorProfileID != "" {
		if s.executors == nil {
			return "", false
		}
		profile, err := s.executors.GetExecutorProfile(ctx, executorProfileID)
		if err != nil || profile == nil || strings.TrimSpace(profile.ExecutorID) == "" {
			return "", false
		}
		if executorID != "" && executorID != profile.ExecutorID {
			return "", false
		}
		executorID = strings.TrimSpace(profile.ExecutorID)
	}
	if executorID == "" {
		workspaceExecutorID, ok := s.conversationForkWorkspaceExecutorID(ctx, req.WorkspaceID)
		if !ok {
			return "", false
		}
		executorID = workspaceExecutorID
	}
	if executorID == "" {
		executorID = models.ExecutorIDLocal
	}
	return executorID, true
}

func (s *Service) conversationForkWorkspaceExecutorID(ctx context.Context, workspaceID string) (string, bool) {
	if s.workspaces == nil {
		return "", true
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, workspaceID)
	if err != nil || workspace == nil || workspace.DefaultExecutorID == nil {
		return "", err == nil && workspace != nil
	}
	return strings.TrimSpace(*workspace.DefaultExecutorID), true
}

func conversationForkMetadataString(metadata map[string]interface{}, key string) string {
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func conversationForkExecutorTypeCanIsolate(executorType models.ExecutorType) bool {
	switch executorType {
	case models.ExecutorTypeWorktree,
		models.ExecutorTypeLocalDocker,
		models.ExecutorTypeRemoteDocker,
		models.ExecutorTypeSprites,
		models.ExecutorTypeSSH,
		models.ExecutorTypeKubernetes:
		return true
	default:
		return false
	}
}

func (s *Service) findConversationForkTaskRetry(
	ctx context.Context,
	admission *models.ConversationForkAdmission,
) (CreateTaskResult, bool, error) {
	destinations, ok := s.messages.(taskrepo.ConversationForkDestinationRepository)
	if !ok {
		return CreateTaskResult{}, false, models.ErrConversationForkSourceUnavailable
	}
	draft, err := destinations.GetConversationForkByDestinationRequest(ctx, admission.OwnerID, admission.DestinationRequestID)
	if errors.Is(err, models.ErrConversationForkNotFound) {
		return CreateTaskResult{}, false, nil
	}
	if err != nil {
		return CreateTaskResult{}, true, err
	}
	if draft.Descriptor.ID != admission.ForkID || draft.DestinationFingerprint != admission.RequestFingerprint || draft.WorkspaceID != admission.WorkspaceID || draft.Descriptor.DestinationKind != admission.DestinationKind {
		return CreateTaskResult{}, true, models.ErrConversationForkConflict
	}
	task, err := s.tasks.GetTask(ctx, draft.Descriptor.DestinationTaskID)
	if err != nil {
		return CreateTaskResult{}, true, err
	}
	if task == nil {
		return CreateTaskResult{}, true, models.ErrConversationForkConflict
	}
	outcome := CreateTaskOutcomeFoundSettled
	if !draft.Descriptor.DestinationComplete {
		outcome = CreateTaskOutcomeFoundUnsettled
	}
	return CreateTaskResult{Task: task, Outcome: outcome}, true, nil
}

func (s *Service) MarkConversationForkTaskDestinationComplete(ctx context.Context, workspaceID, forkID, taskID string) error {
	ownerID, err := s.conversationForkOwnerID(ctx, workspaceID)
	if err != nil {
		return models.ErrConversationForkNotFound
	}
	destinations, ok := s.messages.(taskrepo.ConversationForkTaskDestinationRepository)
	if !ok {
		return models.ErrConversationForkSourceUnavailable
	}
	return destinations.MarkConversationForkTaskDestinationComplete(ctx, ownerID, forkID, taskID)
}

func (s *Service) RollbackTaskCreationWithConversationFork(ctx context.Context, taskID string) error {
	destinations, ok := s.messages.(taskrepo.ConversationForkTaskDestinationRepository)
	if !ok {
		return models.ErrConversationForkSourceUnavailable
	}
	if err := destinations.RestoreConversationForkTaskDestinationForRollback(ctx, taskID); err != nil {
		return err
	}
	return s.DeleteTaskWithLifecycle(ctx, taskID)
}

func taskForkDestinationFingerprint(req *CreateTaskRequest) (string, error) {
	encoded, err := json.Marshal(struct {
		ForkID    string            `json:"fork_id"`
		Workspace *WorkspacePolicy  `json:"workspace_policy,omitempty"`
		Create    CreateTaskRequest `json:"create"`
	}{req.ConversationForkID, req.WorkspacePolicy, *req})
	if err != nil {
		return "", fmt.Errorf("encode conversation fork destination request: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func workspaceModeForForkRequest(req *CreateTaskRequest) string {
	if req.WorkspacePolicy != nil && req.WorkspacePolicy.Mode != "" {
		return req.WorkspacePolicy.Mode
	}
	workspace, _ := req.Metadata["workspace"].(map[string]interface{})
	mode, _ := workspace["mode"].(string)
	return mode
}
