package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// BackgroundWorkActionDispatcher delivers targeted actions to an active background execution.
type BackgroundWorkActionDispatcher interface {
	ExecuteBackgroundWorkAction(ctx context.Context, executionID string, req streams.BackgroundWorkActionRequest) (streams.BackgroundWorkActionResponse, error)
}

// SetBackgroundWorkActionDispatcher wires the runtime action dispatcher.
func (s *Service) SetBackgroundWorkActionDispatcher(d BackgroundWorkActionDispatcher) {
	s.backgroundWorkActionDispatcher = d
}

func (s *Service) validateActionRequest(sessionID, workID string, req streams.BackgroundWorkActionRequest) error {
	if sessionID == "" || workID == "" {
		return errors.New("session ID and work ID are required")
	}
	if req.Action == streams.WorkloadActionWriteInput && len(req.Data) > 64*1024 {
		return errors.New("input data exceeds 64KB limit")
	}
	return nil
}

const statusSuccess = "success"

func (s *Service) checkExistingActionReceipt(ctx context.Context, sessionID, workID string, req streams.BackgroundWorkActionRequest) (*streams.BackgroundWorkActionResponse, error) {
	if req.OperationID == "" || s.backgroundWork == nil {
		return nil, nil
	}
	existing, err := s.backgroundWork.GetBackgroundActionReceipt(ctx, sessionID, req.OperationID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}
	if existing.WorkloadID != workID || existing.Action != string(req.Action) || (req.RunID != "" && existing.RunID != "" && existing.RunID != req.RunID) {
		return nil, errors.New("operation ID conflict or parameter mismatch")
	}
	if existing.Status == "pending" {
		return &streams.BackgroundWorkActionResponse{
			Success:   false,
			WorkID:    existing.WorkloadID,
			RunID:     existing.RunID,
			Action:    streams.WorkloadActionKind(existing.Action),
			Error:     "action already in progress",
			Uncertain: true,
		}, nil
	}
	return &streams.BackgroundWorkActionResponse{
		Success:   existing.Status == statusSuccess || existing.Status == "accepted",
		WorkID:    existing.WorkloadID,
		RunID:     existing.RunID,
		Action:    streams.WorkloadActionKind(existing.Action),
		Error:     existing.Error,
		Uncertain: existing.Uncertain,
	}, nil
}

func (s *Service) validateWorkloadTarget(ctx context.Context, sessionID, workID string) (*models.BackgroundWorkload, error) {
	if s.backgroundWork == nil {
		return nil, errors.New("background work repository not available")
	}
	workload, err := s.backgroundWork.GetBackgroundWorkload(ctx, sessionID, workID)
	if err != nil || workload == nil {
		return nil, repoerrors.ErrTaskNotFound
	}
	if workload.State != string(streams.RunStateRunning) && workload.State != string(streams.RunStateWaiting) {
		return nil, errors.New("workload is not running")
	}
	return workload, nil
}

func (s *Service) validateRunTarget(ctx context.Context, sessionID, workID, runID string) error {
	if runID == "" || s.backgroundWork == nil {
		return nil
	}
	run, err := s.backgroundWork.GetBackgroundRun(ctx, sessionID, runID)
	if err == nil && run != nil && run.WorkloadID != workID {
		return errors.New("run does not belong to workload")
	}
	return nil
}

func (s *Service) reservePendingReceipt(
	ctx context.Context,
	sessionID, workID, runID, opID string,
	action streams.WorkloadActionKind,
) (*streams.BackgroundWorkActionResponse, error) {
	if s.backgroundWork == nil {
		return nil, nil
	}
	pendingReceipt := &models.BackgroundActionReceipt{
		ID:          uuid.New().String(),
		SessionID:   sessionID,
		WorkloadID:  workID,
		RunID:       runID,
		Action:      string(action),
		OperationID: opID,
		Status:      "pending",
	}
	if err := s.backgroundWork.ReserveBackgroundActionReceipt(ctx, pendingReceipt); err != nil {
		existing, checkErr := s.backgroundWork.GetBackgroundActionReceipt(ctx, sessionID, opID)
		if checkErr == nil && existing != nil {
			if existing.Status != "pending" {
				return &streams.BackgroundWorkActionResponse{
					Success:   existing.Status == statusSuccess || existing.Status == "accepted",
					WorkID:    existing.WorkloadID,
					RunID:     existing.RunID,
					Action:    streams.WorkloadActionKind(existing.Action),
					Error:     existing.Error,
					Uncertain: existing.Uncertain,
				}, nil
			}
			return &streams.BackgroundWorkActionResponse{
				Success:   false,
				WorkID:    workID,
				RunID:     existing.RunID,
				Action:    action,
				Error:     "action already in progress",
				Uncertain: true,
			}, nil
		}
		return nil, err
	}
	return nil, nil
}

func (s *Service) recordActionExecutionReceipt(
	ctx context.Context,
	sessionID, workID, opID string,
	req streams.BackgroundWorkActionRequest,
	resp streams.BackgroundWorkActionResponse,
	dispatchErr error,
) {
	if s.backgroundWork == nil {
		return
	}
	uncertain := false
	status := statusSuccess
	errMsg := resp.Error
	if dispatchErr != nil {
		status = "failed"
		errMsg = dispatchErr.Error()
		if errors.Is(dispatchErr, context.DeadlineExceeded) || errors.Is(dispatchErr, context.Canceled) {
			uncertain = true
		}
	} else if !resp.Success {
		status = "rejected"
		if resp.Uncertain {
			uncertain = true
		}
	}

	receipt := &models.BackgroundActionReceipt{
		ID:          uuid.New().String(),
		SessionID:   sessionID,
		WorkloadID:  workID,
		RunID:       req.RunID,
		Action:      string(req.Action),
		OperationID: opID,
		Status:      status,
		Error:       errMsg,
		Uncertain:   uncertain,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	_ = s.backgroundWork.RecordBackgroundActionReceipt(ctx, receipt)
}

func (s *Service) resolveSessionExecutionID(ctx context.Context, sessionID string) (string, error) {
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return "", repoerrors.ErrTaskNotFound
	}
	if session.AgentExecutionID != "" {
		return session.AgentExecutionID, nil
	}
	return session.ID, nil
}

// ExecuteBackgroundWorkAction executes an action (stop, interrupt, write_input, close_input) on a background workload.
func (s *Service) ExecuteBackgroundWorkAction(
	ctx context.Context,
	sessionID string,
	req streams.BackgroundWorkActionRequest,
) (streams.BackgroundWorkActionResponse, error) {
	sessionID = strings.TrimSpace(sessionID)
	workID := strings.TrimSpace(req.WorkID)

	if err := s.validateActionRequest(sessionID, workID, req); err != nil {
		return streams.BackgroundWorkActionResponse{Success: false, WorkID: workID, RunID: req.RunID, Action: req.Action, Error: err.Error()}, err
	}
	if err := s.AuthorizeSessionAccess(ctx, sessionID); err != nil {
		return streams.BackgroundWorkActionResponse{Success: false, WorkID: workID, RunID: req.RunID, Action: req.Action, Error: "access denied"}, err
	}
	if receiptResp, err := s.checkExistingActionReceipt(ctx, sessionID, workID, req); err != nil {
		return streams.BackgroundWorkActionResponse{Success: false, WorkID: workID, RunID: req.RunID, Action: req.Action, Error: err.Error()}, err
	} else if receiptResp != nil {
		return *receiptResp, nil
	}

	workload, err := s.validateWorkloadTarget(ctx, sessionID, workID)
	if err != nil {
		errMsg := err.Error()
		if workload != nil {
			errMsg = fmt.Sprintf("workload is not running (state=%s)", workload.State)
		}
		return streams.BackgroundWorkActionResponse{Success: false, WorkID: workID, RunID: req.RunID, Action: req.Action, Error: errMsg}, err
	}

	if err := s.validateRunTarget(ctx, sessionID, workID, req.RunID); err != nil {
		return streams.BackgroundWorkActionResponse{Success: false, WorkID: workID, RunID: req.RunID, Action: req.Action, Error: err.Error()}, err
	}

	opID := req.OperationID
	if opID == "" {
		opID = uuid.New().String()
	}

	if reservedResp, err := s.reservePendingReceipt(ctx, sessionID, workID, req.RunID, opID, req.Action); err != nil {
		return streams.BackgroundWorkActionResponse{Success: false, WorkID: workID, RunID: req.RunID, Action: req.Action, Error: err.Error()}, err
	} else if reservedResp != nil {
		return *reservedResp, nil
	}

	executionID, err := s.resolveSessionExecutionID(ctx, sessionID)
	if err != nil {
		return streams.BackgroundWorkActionResponse{Success: false, WorkID: workID, RunID: req.RunID, Action: req.Action, Error: "session not found"}, err
	}
	if s.backgroundWorkActionDispatcher == nil {
		return streams.BackgroundWorkActionResponse{Success: false, WorkID: workID, RunID: req.RunID, Action: req.Action, Error: "dispatcher not configured"}, errors.New("dispatcher not configured")
	}

	resp, dispatchErr := s.backgroundWorkActionDispatcher.ExecuteBackgroundWorkAction(ctx, executionID, req)
	recordCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.recordActionExecutionReceipt(recordCtx, sessionID, workID, opID, req, resp, dispatchErr)
	return resp, dispatchErr
}
