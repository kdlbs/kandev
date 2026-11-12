package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/office/wakeup"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const blockerFailureWakeupReason = "task_blocker_failed"

// QueueBlockerFailureWakeup records one blocker failure before asking the
// shared dispatcher to deliver it. A pause or transient dispatch error leaves
// the request queued for resume or recovery replay.
func (s *Service) QueueBlockerFailureWakeup(
	ctx context.Context,
	recipientID, parentTaskID, blockerID, blockerState, occurrenceID string,
	heldDependentIDs []string,
) error {
	if recipientID == "" || blockerID == "" || occurrenceID == "" {
		return errors.New("blocker failure wakeup identity is incomplete")
	}
	key := fmt.Sprintf("%s:%s:%s", blockerFailureWakeupReason, blockerID, occurrenceID)
	alert := wakeup.BlockerFailureAlert{
		ParentTaskID: parentTaskID, BlockerTaskID: blockerID,
		BlockerState: blockerState, HeldDependentIDs: heldDependentIDs,
	}
	alertKey := blockerID + ":" + occurrenceID
	payload := wakeup.BlockerFailurePayload{
		RunTaskID: parentTaskID, BlockerTaskID: blockerID,
		BlockerState: blockerState, HeldDependentIDs: heldDependentIDs,
		Alerts: map[string]wakeup.BlockerFailureAlert{alertKey: alert},
	}
	encoded, err := wakeup.MarshalPayload(payload)
	if err != nil {
		return fmt.Errorf("encode blocker failure wakeup: %w", err)
	}
	req := &sqlite.WakeupRequest{
		ID: uuid.NewString(), AgentProfileID: recipientID,
		Source: wakeup.SourceBlockerFailure, Reason: blockerFailureWakeupReason,
		Payload: encoded, IdempotencyKey: sql.NullString{String: key, Valid: true},
	}
	req, err = s.createOrLoadBlockerFailureWakeup(ctx, req, key, recipientID)
	if err != nil {
		return fmt.Errorf("persist blocker failure wakeup: %w", err)
	}
	return s.dispatchBlockerFailureWakeup(ctx, req)
}

func (s *Service) createOrLoadBlockerFailureWakeup(
	ctx context.Context, req *sqlite.WakeupRequest, key, recipientID string,
) (*sqlite.WakeupRequest, error) {
	err := s.repo.CreateWakeupRequest(ctx, req)
	if err == nil {
		return req, nil
	}
	if !errors.Is(err, sqlite.ErrWakeupIdempotencyConflict) {
		return nil, err
	}
	req, err = s.repo.GetWakeupRequestByIdempotencyKey(ctx, key)
	if err != nil {
		return nil, err
	}
	return s.updateQueuedBlockerFailureRecipient(ctx, req, recipientID)
}

func (s *Service) updateQueuedBlockerFailureRecipient(
	ctx context.Context, req *sqlite.WakeupRequest, recipientID string,
) (*sqlite.WakeupRequest, error) {
	if req.Status != sqlite.WakeupStatusQueued || req.AgentProfileID == recipientID {
		return req, nil
	}
	updated, err := s.repo.UpdateQueuedWakeupRequestRecipient(ctx, req.ID, recipientID)
	if err != nil {
		return nil, fmt.Errorf("update queued blocker failure recipient: %w", err)
	}
	if updated {
		req.AgentProfileID = recipientID
	}
	return req, nil
}

func (s *Service) replayBlockerFailureWakeups(ctx context.Context, workspaceID string, limit int) error {
	if s.wakeupRequestDispatcher == nil {
		return nil
	}
	requests, err := s.repo.ListQueuedWakeupRequestsForWorkspaceAndSource(
		ctx, workspaceID, wakeup.SourceBlockerFailure, limit,
	)
	if err != nil {
		return fmt.Errorf("list queued blocker failure wakeups: %w", err)
	}
	var errs []error
	for _, req := range requests {
		if err := s.replayBlockerFailureWakeup(ctx, req); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) replayBlockerFailureWakeup(ctx context.Context, req *sqlite.WakeupRequest) error {
	payload, err := decodeBlockerFailurePayload(req.Payload)
	if err != nil {
		return fmt.Errorf("decode blocker failure wakeup %s: %w", req.ID, err)
	}
	current, err := s.currentBlockerFailureAlerts(ctx, req.ID, payload.Alerts)
	if err != nil {
		return err
	}
	return s.updateAndDispatchBlockerFailureWakeup(ctx, req, payload, current)
}

func decodeBlockerFailurePayload(raw string) (wakeup.BlockerFailurePayload, error) {
	var payload wakeup.BlockerFailurePayload
	if err := wakeup.UnmarshalPayload(raw, &payload); err != nil {
		return payload, err
	}
	if len(payload.Alerts) == 0 && payload.BlockerTaskID != "" {
		payload.Alerts = map[string]wakeup.BlockerFailureAlert{
			"legacy": {
				BlockerTaskID: payload.BlockerTaskID, BlockerState: payload.BlockerState,
				HeldDependentIDs: payload.HeldDependentIDs,
			},
		}
	}
	return payload, nil
}

func (s *Service) currentBlockerFailureAlerts(
	ctx context.Context, requestID string, alerts map[string]wakeup.BlockerFailureAlert,
) (map[string]wakeup.BlockerFailureAlert, error) {
	current := make(map[string]wakeup.BlockerFailureAlert, len(alerts))
	for occurrenceID, alert := range alerts {
		currentAlert, relevant, err := s.currentBlockerFailureAlert(ctx, requestID, alert)
		if err != nil {
			return nil, err
		}
		if relevant {
			current[occurrenceID] = currentAlert
		}
	}
	return current, nil
}

func (s *Service) currentBlockerFailureAlert(
	ctx context.Context, requestID string, alert wakeup.BlockerFailureAlert,
) (wakeup.BlockerFailureAlert, bool, error) {
	fields, err := s.repo.GetTaskExecutionFields(ctx, alert.BlockerTaskID)
	if errors.Is(err, sqlite.ErrTaskNotFound) {
		return alert, false, nil
	}
	if err != nil {
		return alert, false, fmt.Errorf("read blocker failure wakeup %s task %s: %w", requestID, alert.BlockerTaskID, err)
	}
	if fields.State != alert.BlockerState || fields.State != "FAILED" && fields.State != "CANCELLED" {
		return alert, false, nil
	}
	held, err := s.currentHeldDependents(ctx, fields)
	if err != nil {
		return alert, false, fmt.Errorf("check blocker failure wakeup %s dependents: %w", requestID, err)
	}
	if len(held) == 0 {
		return alert, false, nil
	}
	alert.HeldDependentIDs = held
	return alert, true, nil
}

func (s *Service) updateAndDispatchBlockerFailureWakeup(
	ctx context.Context,
	req *sqlite.WakeupRequest,
	payload wakeup.BlockerFailurePayload,
	current map[string]wakeup.BlockerFailureAlert,
) error {
	if len(current) == 0 {
		return s.repo.MarkWakeupRequestSkipped(ctx, req.ID, "blocker_failure_no_longer_relevant")
	}
	payload.Alerts = current
	first := current[firstBlockerAlertKey(current)]
	payload.BlockerTaskID = first.BlockerTaskID
	payload.BlockerState = first.BlockerState
	payload.HeldDependentIDs = first.HeldDependentIDs
	encoded, err := wakeup.MarshalPayload(payload)
	if err != nil {
		return fmt.Errorf("encode replayed blocker failure wakeup %s: %w", req.ID, err)
	}
	updated, err := s.repo.UpdateQueuedWakeupRequestPayload(ctx, req.ID, encoded)
	if err != nil {
		return err
	}
	if !updated {
		return nil
	}
	return s.dispatchBlockerFailureWakeup(ctx, req)
}

func firstBlockerAlertKey(alerts map[string]wakeup.BlockerFailureAlert) string {
	keys := make([]string, 0, len(alerts))
	for key := range alerts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func (s *Service) currentHeldDependents(
	ctx context.Context, blocker *sqlite.TaskExecutionFields,
) ([]string, error) {
	ids, err := s.repo.ListTasksBlockedBy(ctx, blocker.ID)
	if err != nil {
		return nil, err
	}
	held := make([]string, 0, len(ids))
	for _, id := range ids {
		dependent, err := s.repo.GetTaskExecutionFields(ctx, id)
		if errors.Is(err, sqlite.ErrTaskNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if dependent.IsFromOffice && !dependent.Archived && dependent.WorkspaceID == blocker.WorkspaceID &&
			!taskmodels.IsTerminalTaskState(v1.TaskState(dependent.State)) {
			held = append(held, id)
		}
	}
	return held, nil
}

func (s *Service) dispatchBlockerFailureWakeup(ctx context.Context, req *sqlite.WakeupRequest) error {
	if req.Status != sqlite.WakeupStatusQueued {
		return nil
	}
	if s.wakeupRequestDispatcher == nil {
		return errors.New("blocker failure wakeup dispatcher is not configured")
	}
	err := s.wakeupRequestDispatcher.Dispatch(ctx, req.ID)
	if errors.Is(err, shared.ErrWorkspacePaused) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("dispatch blocker failure wakeup %s: %w", req.ID, err)
	}
	s.logger.Info("office.blocker_failed.dispatched",
		zap.String("wakeup_id", req.ID), zap.String("agent_id", req.AgentProfileID))
	return nil
}
