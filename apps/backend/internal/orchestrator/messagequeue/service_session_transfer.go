package messagequeue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type durableSessionTransferState struct {
	operationID       string
	compensation      *SessionTransferCompensation
	attachmentIDs     []string
	preparationCalled bool
	rollbackSucceeded bool
	operationCtx      context.Context
	stopLeaseRenewal  func() error
	leaseRenewalErr   error
	cancelTransfer    context.CancelFunc
}

// TransferSessionWithDurablePreparation preserves the original callback
// contract for callers whose external state is not attachment-scoped.
func (s *Service) TransferSessionWithDurablePreparation(
	ctx context.Context,
	taskID, oldSessionID, newSessionID string,
	prepare func(context.Context) error,
	rollback func(context.Context) error,
) error {
	return s.TransferSessionWithDurableAttachmentPreparation(
		ctx,
		taskID,
		oldSessionID,
		newSessionID,
		func(callbackCtx context.Context, _ []string) error {
			if prepare == nil {
				return nil
			}
			return prepare(callbackCtx)
		},
		func(callbackCtx context.Context, _ []string) error {
			if rollback == nil {
				return nil
			}
			return rollback(callbackCtx)
		},
	)
}

// TransferSessionWithDurableAttachmentPreparation keeps attachment ownership
// aligned with every queue row, including lifecycle rows hidden from UI status.
// Durable repositories record the exact attachment set before external state
// changes so rollback and startup recovery cannot move unrelated claims.
func (s *Service) TransferSessionWithDurableAttachmentPreparation(
	ctx context.Context,
	taskID, oldSessionID, newSessionID string,
	prepare func(context.Context, []string) error,
	rollback func(context.Context, []string) error,
) error {
	state := &durableSessionTransferState{}
	transferCtx, cancelTransfer := context.WithCancel(ctx)
	defer cancelTransfer()
	state.cancelTransfer = cancelTransfer
	if s.SessionTransferCompensationPersistenceAvailable() {
		state.operationID = uuid.NewString()
	}
	err := s.transferSession(
		transferCtx,
		taskID,
		oldSessionID,
		newSessionID,
		state.operationID,
		func(admittedCtx context.Context) error {
			return s.prepareDurableSessionTransfer(
				admittedCtx, taskID, oldSessionID, newSessionID, prepare, state,
			)
		},
		func(rollbackCtx context.Context) error {
			if !state.preparationCalled || rollback == nil {
				state.rollbackSucceeded = true
				return nil
			}
			rollbackErr := rollback(rollbackCtx, state.attachmentIDs)
			state.rollbackSucceeded = rollbackErr == nil
			return rollbackErr
		},
	)
	if state.stopLeaseRenewal != nil {
		state.leaseRenewalErr = state.stopLeaseRenewal()
		state.stopLeaseRenewal = nil
		if state.leaseRenewalErr != nil {
			err = errors.Join(err, fmt.Errorf("session transfer lease lost: %w", state.leaseRenewalErr))
		}
	}
	if err != nil {
		if state.compensation != nil &&
			state.rollbackSucceeded &&
			state.leaseRenewalErr == nil &&
			!errors.Is(err, ErrSessionTransferOwnershipLost) {
			err = errors.Join(err, s.deleteSessionTransferCompensation(context.WithoutCancel(ctx), *state.compensation))
		}
		return err
	}
	if state.compensation != nil {
		if deleteErr := s.deleteSessionTransferCompensation(context.WithoutCancel(ctx), *state.compensation); deleteErr != nil {
			s.logger.Warn("session transfer committed with pending compensation record", zap.Error(deleteErr))
		}
	}
	return nil
}

func (s *Service) startSessionTransferCompensation(
	ctx context.Context,
	taskID, oldSessionID, newSessionID string,
	state *durableSessionTransferState,
) error {
	if !s.SessionTransferCompensationPersistenceAvailable() {
		return nil
	}
	state.compensation = &SessionTransferCompensation{
		OperationID: state.operationID, TaskID: taskID,
		FromSessionID: oldSessionID, ToSessionID: newSessionID,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.UpsertSessionTransferCompensation(ctx, *state.compensation); err != nil {
		return fmt.Errorf("persist session transfer fence: %w", err)
	}
	renewInterval := sessionTransferCompensationLeaseDuration / 2
	if s.sessionTransferCompensationLeaseRenewInterval > 0 {
		renewInterval = s.sessionTransferCompensationLeaseRenewInterval
	}
	state.operationCtx, state.stopLeaseRenewal = s.maintainSessionTransferCompensationLease(
		ctx, *state.compensation, state.operationID, state.cancelTransfer, renewInterval,
	)
	return nil
}

func (s *Service) prepareDurableSessionTransfer(
	ctx context.Context,
	taskID, oldSessionID, newSessionID string,
	prepare func(context.Context, []string) error,
	state *durableSessionTransferState,
) error {
	if err := s.startSessionTransferCompensation(
		ctx, taskID, oldSessionID, newSessionID, state,
	); err != nil {
		return err
	}
	entries, err := s.repo.ListBySession(ctx, oldSessionID)
	if err != nil {
		return fmt.Errorf("snapshot session queue for transfer: %w", err)
	}
	entryIDs := make([]string, 0, len(entries))
	seenEntryIDs := make(map[string]struct{}, len(entries))
	attachmentIDs := make([]string, 0)
	seenAttachmentIDs := make(map[string]struct{})
	for _, entry := range entries {
		entryIDs = append(entryIDs, entry.ID)
		seenEntryIDs[entry.ID] = struct{}{}
		attachmentIDs = appendTransferAttachmentIDs(attachmentIDs, seenAttachmentIDs, entry.Attachments)
	}
	dispatchEntryIDs, dispatchAttachments, err := s.snapshotPendingDispatchTransfer(
		ctx, oldSessionID, seenEntryIDs, seenAttachmentIDs,
	)
	if err != nil {
		return err
	}
	entryIDs = append(entryIDs, dispatchEntryIDs...)
	attachmentIDs = append(attachmentIDs, dispatchAttachments...)
	cleanupEntryIDs, cleanupAttachments, cleanupLocators, err := s.snapshotAttachmentCleanupTransfer(
		ctx, taskID, oldSessionID, seenEntryIDs, seenAttachmentIDs,
	)
	if err != nil {
		return err
	}
	entryIDs = append(entryIDs, cleanupEntryIDs...)
	attachmentIDs = append(attachmentIDs, cleanupAttachments...)
	state.attachmentIDs = attachmentIDs
	if state.compensation != nil {
		state.compensation.EntryIDs = entryIDs
		state.compensation.AttachmentIDs = attachmentIDs
		state.compensation.CleanupLocators = cleanupLocators
		if err := s.UpsertSessionTransferCompensation(ctx, *state.compensation); err != nil {
			return fmt.Errorf("persist session transfer compensation: %w", err)
		}
	}
	if prepare == nil {
		return nil
	}
	state.preparationCalled = true
	prepareCtx := ctx
	if state.operationCtx != nil {
		prepareCtx = state.operationCtx
	}
	if err := prepare(prepareCtx, attachmentIDs); err != nil {
		return fmt.Errorf("prepare durable session transfer: %w", err)
	}
	if state.operationCtx != nil {
		select {
		case <-state.operationCtx.Done():
			return state.operationCtx.Err()
		default:
		}
	}
	return nil
}

func (s *Service) snapshotPendingDispatchTransfer(
	ctx context.Context,
	sessionID string,
	seenEntryIDs, seenAttachmentIDs map[string]struct{},
) ([]string, []string, error) {
	if !s.PendingQueueDispatchPersistenceAvailable() {
		return nil, nil, nil
	}
	pending, err := s.ListPendingQueueDispatches(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot in-flight queue dispatches for transfer: %w", err)
	}
	entryIDs := make([]string, 0)
	attachmentIDs := make([]string, 0)
	for _, dispatch := range pending {
		entry := dispatch.Message
		if entry.SessionID != sessionID {
			continue
		}
		if _, seen := seenEntryIDs[entry.ID]; !seen {
			entryIDs = append(entryIDs, entry.ID)
			seenEntryIDs[entry.ID] = struct{}{}
		}
		attachmentIDs = appendTransferAttachmentIDs(attachmentIDs, seenAttachmentIDs, entry.Attachments)
	}
	return entryIDs, attachmentIDs, nil
}

func (s *Service) snapshotAttachmentCleanupTransfer(
	ctx context.Context,
	taskID, sessionID string,
	seenEntryIDs, seenAttachmentIDs map[string]struct{},
) ([]string, []string, []AttachmentCleanupLocator, error) {
	if !s.AttachmentCleanupPersistenceAvailable() {
		return nil, nil, nil, nil
	}
	cleanups, err := s.ListAttachmentCleanups(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("snapshot attachment cleanups for transfer: %w", err)
	}
	entryIDs := make([]string, 0)
	attachmentIDs := make([]string, 0)
	locators := make([]AttachmentCleanupLocator, 0)
	for _, cleanup := range cleanups {
		currentSessionID := cleanup.CurrentSessionID
		if currentSessionID == "" {
			currentSessionID = cleanup.SessionID
		}
		if cleanup.TaskID != taskID || currentSessionID != sessionID {
			continue
		}
		if _, seen := seenEntryIDs[cleanup.EntryID]; !seen {
			entryIDs = append(entryIDs, cleanup.EntryID)
			seenEntryIDs[cleanup.EntryID] = struct{}{}
		}
		attachmentIDs = appendTransferAttachmentIDs(attachmentIDs, seenAttachmentIDs, cleanup.Attachments)
		locators = append(locators, AttachmentCleanupLocator{
			SessionID: cleanup.SessionID, EntryID: cleanup.EntryID, OperationID: cleanup.OperationID,
		})
	}
	return entryIDs, attachmentIDs, locators, nil
}

func appendTransferAttachmentIDs(
	ids []string,
	seen map[string]struct{},
	attachments []MessageAttachment,
) []string {
	for _, attachment := range attachments {
		if attachment.AttachmentID == "" {
			continue
		}
		if _, ok := seen[attachment.AttachmentID]; ok {
			continue
		}
		seen[attachment.AttachmentID] = struct{}{}
		ids = append(ids, attachment.AttachmentID)
	}
	return ids
}

func (s *Service) deleteSessionTransferCompensation(
	ctx context.Context,
	compensation SessionTransferCompensation,
) error {
	if err := s.DeleteSessionTransferCompensation(
		ctx,
		compensation.OperationID,
		compensation.TaskID,
		compensation.FromSessionID,
		compensation.ToSessionID,
	); err != nil {
		return fmt.Errorf("delete session transfer compensation: %w", err)
	}
	return nil
}
