package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

var (
	ErrInitialSubmissionAcceptanceUncertain = errors.New(
		"the original task submission may already have reached the agent; send it again from the task chat to continue",
	)
	ErrInitialSubmissionUnavailable = errors.New(
		"the original task attachments are unavailable; reattach them in the task chat before continuing",
	)
)

const initialSubmissionMissingState = "missing"

type initialSubmissionMetadataCAS interface {
	SetSessionMetadataKeyIfJSONValue(context.Context, string, string, interface{}, interface{}) (bool, error)
}

type initialSubmissionAttachmentResolver interface {
	ResolveClaimed(context.Context, string, string, string) (*models.TaskMessageAttachment, error)
}

func (s *Service) initialSubmissionForFreshStart(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
) (*models.InitialPromptSubmission, error) {
	if !eligibleInitialSubmissionRecovery(session) {
		return nil, nil
	}
	submission, exists, err := s.loadInitialSubmissionSource(ctx, session)
	if err != nil || submission == nil {
		return submission, err
	}
	replayable, err := validateInitialSubmissionReplayState(submission)
	if err != nil || !replayable {
		return nil, err
	}
	if err := s.validateInitialSubmissionAttachments(ctx, taskID, session.ID, submission); err != nil {
		return nil, ErrInitialSubmissionUnavailable
	}
	if !exists {
		return s.persistLegacyInitialSubmission(ctx, session.ID, submission)
	}
	return submission, nil
}

func (s *Service) loadInitialSubmissionSource(
	ctx context.Context,
	session *models.TaskSession,
) (*models.InitialPromptSubmission, bool, error) {
	submission, exists, err := models.LoadInitialPromptSubmission(session.Metadata)
	if err != nil {
		return nil, false, fmt.Errorf("load original task submission: %w", err)
	}
	if exists {
		return submission, true, nil
	}
	return s.legacyInitialSubmissionSource(ctx, session)
}

func (s *Service) legacyInitialSubmissionSource(
	ctx context.Context,
	session *models.TaskSession,
) (*models.InitialPromptSubmission, bool, error) {
	preview, found, err := loadInitialPromptPreview(session.Metadata)
	if err != nil {
		return nil, false, ErrInitialSubmissionUnavailable
	}
	if !found || sessionHasProviderSessionToken(session) {
		return nil, false, nil
	}
	hasPrompt, err := s.repo.HasUserPromptHistory(ctx, session.ID)
	if err != nil {
		return nil, false, fmt.Errorf("check original task submission history: %w", err)
	}
	if hasPrompt {
		return nil, false, nil
	}
	submission, err := models.NewInitialPromptSubmission(preview.Content, false, preview.Attachments)
	if err != nil {
		return nil, false, ErrInitialSubmissionUnavailable
	}
	return submission, false, nil
}

func validateInitialSubmissionReplayState(submission *models.InitialPromptSubmission) (bool, error) {
	if submission.State == models.InitialPromptSubmissionAccepted {
		return false, nil
	}
	if submission.State != models.InitialPromptSubmissionPending {
		return false, ErrInitialSubmissionAcceptanceUncertain
	}
	if submission.InlineAttachmentCount > 0 {
		return false, ErrInitialSubmissionUnavailable
	}
	return submission.HasReplayableContent(), nil
}

func (s *Service) persistLegacyInitialSubmission(
	ctx context.Context,
	sessionID string,
	submission *models.InitialPromptSubmission,
) (*models.InitialPromptSubmission, error) {
	stored, err := s.repo.SetSessionMetadataKeyIfAbsent(
		ctx, sessionID, models.SessionMetaKeyInitialPromptSubmission, submission,
	)
	if err != nil {
		return nil, fmt.Errorf("persist recovered original task submission: %w", err)
	}
	if stored {
		return submission, nil
	}
	current, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || current == nil {
		return nil, ErrInitialSubmissionAcceptanceUncertain
	}
	reloaded, found, err := models.LoadInitialPromptSubmission(current.Metadata)
	if err != nil || !found {
		return nil, ErrInitialSubmissionAcceptanceUncertain
	}
	if reloaded.State == models.InitialPromptSubmissionAccepted {
		return nil, nil
	}
	if reloaded.State != models.InitialPromptSubmissionPending || !sameInitialSubmissionSource(submission, reloaded) {
		return nil, ErrInitialSubmissionAcceptanceUncertain
	}
	return reloaded, nil
}

func eligibleInitialSubmissionRecovery(session *models.TaskSession) bool {
	if session == nil || session.State != models.TaskSessionStateFailed {
		return false
	}
	lastError, found := models.LoadLastAgentError(session.Metadata)
	return found && !lastError.IsDismissed() && lastError.Phase == models.LaunchErrorPhaseBootstrap
}

func loadInitialPromptPreview(metadata map[string]interface{}) (*models.InitialPromptPreview, bool, error) {
	if metadata == nil {
		return nil, false, nil
	}
	raw, exists := metadata[models.SessionMetaKeyInitialPromptPreview]
	if !exists || raw == nil {
		return nil, false, nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, true, err
	}
	var preview models.InitialPromptPreview
	if err := json.Unmarshal(payload, &preview); err != nil {
		return nil, true, err
	}
	if len(preview.Content) > models.MaxInitialPromptSubmissionBytes || len(preview.Attachments) > models.MaxMessageAttachmentCount {
		return nil, true, models.ErrInvalidInitialPromptSubmission
	}
	for _, attachment := range preview.Attachments {
		if attachment.AttachmentID == "" || attachment.Data != "" || !attachment.HasValidDeliveryMode() {
			return nil, true, models.ErrInvalidInitialPromptSubmission
		}
	}
	return &preview, true, nil
}

func sameInitialSubmissionSource(left, right *models.InitialPromptSubmission) bool {
	if left == nil || right == nil || left.Version != right.Version || left.Content != right.Content ||
		left.PlanMode != right.PlanMode || left.InlineAttachmentCount != right.InlineAttachmentCount ||
		len(left.Attachments) != len(right.Attachments) {
		return false
	}
	for index, attachment := range left.Attachments {
		if attachment.AttachmentID != right.Attachments[index].AttachmentID {
			return false
		}
	}
	return true
}

func (s *Service) validateInitialSubmissionAttachments(
	ctx context.Context,
	taskID, sessionID string,
	submission *models.InitialPromptSubmission,
) error {
	if submission == nil || len(submission.Attachments) == 0 {
		return nil
	}
	resolver, ok := s.attachmentReader.(initialSubmissionAttachmentResolver)
	if !ok {
		return ErrInitialSubmissionUnavailable
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return ErrInitialSubmissionUnavailable
	}
	totalSize := int64(0)
	canonical := make([]v1.MessageAttachment, 0, len(submission.Attachments))
	for _, saved := range submission.Attachments {
		verified, err := s.validateInitialSubmissionAttachment(
			ctx, resolver, taskID, sessionID, task.WorkspaceID, saved,
		)
		if err != nil {
			return ErrInitialSubmissionUnavailable
		}
		totalSize += verified.SizeBytes
		if totalSize > models.MaxMessageAttachmentBytes {
			return ErrInitialSubmissionUnavailable
		}
		canonical = append(canonical, verified)
	}
	submission.Attachments = canonical
	return submission.Validate()
}

func (s *Service) validateInitialSubmissionAttachment(
	ctx context.Context,
	resolver initialSubmissionAttachmentResolver,
	taskID, sessionID, workspaceID string,
	saved v1.MessageAttachment,
) (v1.MessageAttachment, error) {
	attachment, err := resolver.ResolveClaimed(ctx, saved.AttachmentID, taskID, sessionID)
	if err != nil || attachment == nil || attachment.WorkspaceID != workspaceID ||
		!canonicalDescriptorMatches(saved, attachment) {
		return v1.MessageAttachment{}, ErrInitialSubmissionUnavailable
	}
	reader, name, mimeType, size, err := s.attachmentReader.OpenClaimed(ctx, saved.AttachmentID, taskID, sessionID)
	if err != nil || reader == nil {
		return v1.MessageAttachment{}, ErrInitialSubmissionUnavailable
	}
	read, readErr := io.Copy(io.Discard, io.LimitReader(reader, models.MaxMessageAttachmentBytes+1))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || read != attachment.SizeBytes || size != attachment.SizeBytes ||
		name != attachment.Name || mimeType != attachment.MimeType {
		return v1.MessageAttachment{}, ErrInitialSubmissionUnavailable
	}
	return v1.MessageAttachment{
		AttachmentID: attachment.ID,
		Type:         attachment.Kind,
		Name:         attachment.Name,
		MimeType:     attachment.MimeType,
		SizeBytes:    attachment.SizeBytes,
		DeliveryMode: attachment.DeliveryMode,
	}, nil
}

func canonicalDescriptorMatches(saved v1.MessageAttachment, canonical *models.TaskMessageAttachment) bool {
	return canonical != nil && saved.AttachmentID == canonical.ID && saved.Type == canonical.Kind &&
		saved.Name == canonical.Name && saved.MimeType == canonical.MimeType &&
		saved.SizeBytes == canonical.SizeBytes && saved.DeliveryMode == canonical.DeliveryMode
}

func (s *Service) initialSubmissionDispatchCallbacks(
	ctx context.Context,
	taskID, sessionID, prompt string,
	planMode bool,
	attachments []v1.MessageAttachment,
) (func(string) error, func(string), error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return nil, nil, errors.New("load initial submission for agent admission")
	}
	submission, found, err := models.LoadInitialPromptSubmission(session.Metadata)
	if err != nil {
		return nil, nil, fmt.Errorf("load initial task submission for provider admission: %w", err)
	}
	if !found || submission.State != models.InitialPromptSubmissionPending {
		return nil, nil, nil
	}
	candidate, candidateErr := models.NewInitialPromptSubmission(prompt, planMode, attachments)
	if candidateErr != nil || !sameInitialSubmissionSource(submission, candidate) {
		return nil, nil, nil
	}
	before := func(executionID string) error {
		receiptCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return s.transitionInitialSubmissionToDispatching(receiptCtx, sessionID, executionID, "")
	}
	accepted := func(executionID string) {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if transitionErr := s.transitionInitialSubmissionToAccepted(persistCtx, sessionID, executionID, ""); transitionErr != nil {
			s.logger.Warn("failed to persist initial task prompt acceptance",
				zap.String("session_id", sessionID), zap.Error(transitionErr))
		}
	}
	return before, accepted, nil
}

func (s *Service) transitionInitialSubmissionToDispatching(
	ctx context.Context,
	sessionID, executionID, attemptID string,
) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return errors.New("initial task submission is unavailable")
	}
	submission, found, err := models.LoadInitialPromptSubmission(session.Metadata)
	if err != nil || !found || submission.State != models.InitialPromptSubmissionPending {
		state := initialSubmissionMissingState
		if found && submission != nil {
			state = submission.State
		}
		return fmt.Errorf("%w: pending receipt unavailable at provider admission (state %s)", ErrInitialSubmissionAcceptanceUncertain, state)
	}
	updated := *submission
	updated.State = models.InitialPromptSubmissionDispatching
	updated.ExecutionID = executionID
	updated.AttemptID = attemptID
	updated.DispatchStartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.compareAndSetInitialSubmission(ctx, sessionID, submission, &updated); err != nil {
		return fmt.Errorf("mark original task submission dispatching: %w", err)
	}
	return nil
}

func (s *Service) transitionInitialSubmissionToAccepted(
	ctx context.Context,
	sessionID, executionID, attemptID string,
) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return errors.New("initial task submission is unavailable")
	}
	submission, found, err := models.LoadInitialPromptSubmission(session.Metadata)
	if err != nil || !found || submission.State != models.InitialPromptSubmissionDispatching ||
		submission.ExecutionID != executionID || submission.AttemptID != attemptID {
		state := initialSubmissionMissingState
		if found && submission != nil {
			state = fmt.Sprintf("%s/%s/%s", submission.State, submission.ExecutionID, submission.AttemptID)
		}
		return fmt.Errorf("%w: dispatch receipt does not match provider acceptance (state/execution/attempt %s)", ErrInitialSubmissionAcceptanceUncertain, state)
	}
	updated := *submission
	updated.State = models.InitialPromptSubmissionAccepted
	updated.AcceptedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.compareAndSetInitialSubmission(ctx, sessionID, submission, &updated); err != nil {
		return fmt.Errorf("mark original task submission accepted: %w", err)
	}
	return nil
}

func (s *Service) blockInitialSubmissionReplayForOtherPrompt(
	ctx context.Context,
	sessionID, executionID, turnID string,
) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load initial task submission before later prompt admission: %w", err)
	}
	if session == nil {
		return errors.New("load initial task submission before later prompt admission: session not found")
	}
	submission, found, err := models.LoadInitialPromptSubmission(session.Metadata)
	if err != nil {
		return fmt.Errorf("load initial task submission before later prompt admission: %w", err)
	}
	if !found || submission.State != models.InitialPromptSubmissionPending {
		return nil
	}
	updated := *submission
	updated.State = models.InitialPromptSubmissionReplayBlocked
	updated.ReplayBlockedAt = time.Now().UTC().Format(time.RFC3339Nano)
	updated.ReplayBlockedReason = "later_prompt_provider_admission"
	updated.ReplayBlockedExecutionID = executionID
	updated.ReplayBlockedTurnID = turnID
	if err := s.compareAndSetInitialSubmission(ctx, sessionID, submission, &updated); err != nil {
		return fmt.Errorf("retire stale original task submission replay: %w", err)
	}
	return nil
}

func (s *Service) wrapInitialSubmissionReplayPrompt(
	ctx context.Context,
	taskID, sessionID, prompt string,
	attachments []v1.MessageAttachment,
	session *models.TaskSession,
) (string, error) {
	if session == nil || session.ID != sessionID || session.TaskID != taskID {
		return "", errors.New("initial task submission replay session identity changed")
	}
	dbTask, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("load task policy for initial submission replay: %w", err)
	}
	if dbTask == nil {
		return "", errors.New("load task policy for initial submission replay: task not found")
	}
	isOfficeTask, err := s.lookupOfficeTask(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("load Office policy for initial submission replay: %w", err)
	}
	configMode, _ := session.Metadata["config_mode"].(bool)
	titleOwner := false
	if !configMode {
		titleOwner, err = s.ClaimTaskTitleSession(ctx, taskID, sessionID)
		if err != nil {
			return "", fmt.Errorf("claim task title for initial submission replay: %w", err)
		}
	}
	includeCanvasGuidance := false
	if (prompt != "" || len(attachments) > 0) && !isOfficeTask && !session.IsPassthrough && !configMode {
		includeCanvasGuidance, err = s.taskSessionCanvasGuidanceEnabled(ctx, taskID, session, true)
		if err != nil {
			return "", fmt.Errorf("resolve canvas prompt capability for initial submission replay: %w", err)
		}
	}
	return s.wrapCreatedSessionPrompt(
		ctx, prompt, taskID, sessionID, session, dbTask,
		isOfficeTask, configMode, titleOwner, includeCanvasGuidance, nil, "",
	), nil
}

func (s *Service) compareAndSetInitialSubmission(
	ctx context.Context,
	sessionID string,
	expected, updated *models.InitialPromptSubmission,
) error {
	store, ok := s.repo.(initialSubmissionMetadataCAS)
	if !ok {
		return errors.New("session metadata store does not support submission receipts")
	}
	stored, err := store.SetSessionMetadataKeyIfJSONValue(
		ctx, sessionID, models.SessionMetaKeyInitialPromptSubmission, expected, updated,
	)
	if err != nil {
		return err
	}
	if !stored {
		return fmt.Errorf("%w: submission receipt changed concurrently", ErrInitialSubmissionAcceptanceUncertain)
	}
	return nil
}

func (s *Service) replayInitialPromptSubmission(
	ctx context.Context,
	taskID, sessionID string,
	submission *models.InitialPromptSubmission,
	attempt *resumeAttempt,
) error {
	if submission == nil || attempt == nil || !submission.HasReplayableContent() {
		return ErrInitialSubmissionUnavailable
	}
	registry := s.resumeAttemptStore()
	if !registry.holdForInitialPrompt(attempt) {
		s.cleanupCancelledResumeAttempt(attempt)
		return ErrResumeAttemptCancelled
	}
	defer registry.releaseInitialPromptHold(attempt)
	if err := s.validateResumeAttempt(attempt); err != nil {
		s.cleanupCancelledResumeAttempt(attempt)
		return err
	}
	options := promptTaskOptions{
		resumeAttempt:                   attempt,
		firstLaunchPromptContext:        true,
		preserveInitialSubmissionReplay: true,
		disableDispatchRetry:            true,
		beforeProviderAdmission: func() error {
			if err := s.validateResumeAttempt(attempt); err != nil {
				return err
			}
			return s.transitionInitialSubmissionToDispatching(
				ctx, sessionID, attempt.execution(), attempt.identity(),
			)
		},
		onAccepted: func(string) {
			persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if transitionErr := s.transitionInitialSubmissionToAccepted(
				persistCtx, sessionID, attempt.execution(), attempt.identity(),
			); transitionErr != nil {
				s.logger.Warn("failed to persist recovered task prompt acceptance",
					zap.String("session_id", sessionID), zap.Error(transitionErr))
			}
			registry.releaseInitialPromptHold(attempt)
		},
	}
	_, err := s.promptTask(
		ctx, taskID, sessionID, submission.Content, "", submission.PlanMode,
		submission.Attachments, false, launchOriginManual, options,
	)
	return err
}

func (s *Service) backfillInitialSubmissionIfMissing(
	ctx context.Context,
	taskID, sessionID string,
	submission *models.InitialPromptSubmission,
) {
	if submission == nil || s.messageCreator == nil || !submission.HasReplayableContent() {
		return
	}
	msgs, err := s.repo.ListMessages(ctx, sessionID)
	if err != nil {
		s.logger.Warn("backfill initial task submission: list messages failed",
			zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	if len(msgs) > 0 {
		return
	}
	s.recordInitialMessage(
		ctx, taskID, sessionID, submission.Content, submission.PlanMode, false, submission.Attachments,
	)
}
