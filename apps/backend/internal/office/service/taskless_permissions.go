package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/events/bus"
)

// RunPermissionResolver answers a pending permission request of a run-owned
// execution by its execution identity.
type RunPermissionResolver interface {
	ResolveRunPermission(ctx context.Context, executionID, requestID, pendingID, optionID string) error
	CancelRunPermission(ctx context.Context, executionID, requestID, pendingID string) error
}

// SetRunPermissionResolver wires the shared runtime seam that answers
// permission requests raised by taskless Office runs.
func (s *Service) SetRunPermissionResolver(resolver RunPermissionResolver) {
	s.runPermissions = resolver
}

const runEventTypePermissionDecision = "permission.decision"

const (
	permissionDecisionApproved  = "approved"
	permissionDecisionRejected  = "rejected"
	permissionDecisionCancelled = "cancelled"
)

type tasklessPermissionOption struct {
	OptionID string `json:"option_id"`
	Kind     string `json:"kind"`
}

type tasklessPermissionRequest struct {
	AgentLifecycleData
	RequestID              string                     `json:"request_id"`
	PendingID              string                     `json:"pending_id"`
	Title                  string                     `json:"title"`
	Options                []tasklessPermissionOption `json:"options"`
	AutoApprovedOptionID   string                     `json:"auto_approved_option_id"`
	AutoApprovedOptionKind string                     `json:"auto_approved_option_kind"`
	AutoApprovalPending    bool                       `json:"auto_approval_pending"`
}

// handleTasklessPermissionRequest answers a permission request of the current
// attempt of a claimed taskless run. No task, session or UI exists to answer
// it, so an unanswered request would block the turn indefinitely.
func (s *Service) handleTasklessPermissionRequest(ctx context.Context, event *bus.Event) error {
	request, err := decodeEventData[tasklessPermissionRequest](event)
	if err != nil {
		return err
	}
	data := &request.AgentLifecycleData
	if data.OwnerKind != string(runtimeapi.ExecutionOwnerRun) || data.TaskID != "" ||
		!exactRunSessionEvent(data) || data.AgentExecutionID == "" || request.PendingID == "" {
		return nil
	}
	run, err := s.resolveLifecycleRun(ctx, *data)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if s.runPermissions == nil {
		return errors.New("taskless permission handling requires a run permission resolver")
	}
	if request.AutoApprovedOptionID != "" && !request.AutoApprovalPending {
		// agentctl already delivered this auto-approval; the event is a record.
		s.recordTasklessPermissionDecision(
			ctx, run.ID, request, permissionDecisionApproved, request.AutoApprovedOptionID, request.AutoApprovedOptionKind,
		)
		return nil
	}
	decision, optionID, optionKind := decideTasklessPermission(request)
	s.recordTasklessPermissionDecision(ctx, run.ID, request, decision, optionID, optionKind)
	s.deliverTasklessPermissionAnswer(ctx, run.ID, request, decision, optionID)
	return nil
}

// recordTasklessPermissionDecision writes the decision before delivery. A
// failed write still lets the answer go out, because withholding it would
// block the turn again; the decision is logged instead.
func (s *Service) recordTasklessPermissionDecision(
	ctx context.Context,
	runID string,
	request *tasklessPermissionRequest,
	decision, optionID, optionKind string,
) {
	err := s.appendRunEventStrict(ctx, runID, runEventTypePermissionDecision, "info", map[string]interface{}{
		"decision":       decision,
		"option_kind":    optionKind,
		"option_id":      optionID,
		"request_id":     request.RequestID,
		"title":          request.Title,
		"run_session_id": request.RunSessionID,
		"execution_id":   request.AgentExecutionID,
		"pending_id":     request.PendingID,
	})
	if err != nil {
		s.logger.Error("taskless permission decision not recorded",
			zap.String("run_id", runID), zap.String("run_session_id", request.RunSessionID),
			zap.String("execution_id", request.AgentExecutionID), zap.String("pending_id", request.PendingID),
			zap.String("request_id", request.RequestID), zap.String("option_id", optionID),
			zap.String("decision", decision), zap.String("option_kind", optionKind),
			zap.String("title", request.Title), zap.Error(err))
	}
}

var defaultTasklessPermissionRetryDelays = []time.Duration{500 * time.Millisecond, 2 * time.Second}

// deliverTasklessPermissionAnswer retries a failed answer and, when it still
// cannot be delivered, stops the execution: agentctl would otherwise hold the
// request open and the run would stay claimed.
func (s *Service) deliverTasklessPermissionAnswer(
	ctx context.Context, runID string, request *tasklessPermissionRequest, decision, optionID string,
) {
	delays := s.permissionRetryDelays
	if delays == nil {
		delays = defaultTasklessPermissionRetryDelays
	}
	err := s.answerTasklessPermission(ctx, request, decision, optionID)
	for _, delay := range delays {
		if tasklessPermissionAnswerCompleted(err) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		err = s.answerTasklessPermission(ctx, request, decision, optionID)
	}
	if tasklessPermissionAnswerCompleted(err) {
		return
	}
	s.logger.Warn("taskless permission answer failed; stopping execution",
		zap.String("run_id", runID), zap.String("run_session_id", request.RunSessionID),
		zap.String("decision", decision), zap.Error(err))
	if s.runStopper == nil {
		return
	}
	if stopErr := s.runStopper.Stop(ctx, request.AgentExecutionID, "office_permission_answer_failed"); stopErr != nil &&
		!runtimeapi.IsNotFound(stopErr) {
		s.logger.Error("stop execution after undeliverable permission answer failed",
			zap.String("run_id", runID), zap.String("execution_id", request.AgentExecutionID), zap.Error(stopErr))
	}
}

func tasklessPermissionAnswerCompleted(err error) bool {
	return err == nil || runtimeapi.IsNotFound(err) || runtimeapi.IsTerminalPermissionError(err)
}

func (s *Service) answerTasklessPermission(
	ctx context.Context, request *tasklessPermissionRequest, decision, optionID string,
) error {
	if decision == permissionDecisionCancelled {
		return s.runPermissions.CancelRunPermission(ctx, request.AgentExecutionID, request.RequestID, request.PendingID)
	}
	return s.runPermissions.ResolveRunPermission(ctx, request.AgentExecutionID, request.RequestID, request.PendingID, optionID)
}

// decideTasklessPermission approves an auto-approval candidate, otherwise
// selects the first reject option, otherwise cancels the request.
func decideTasklessPermission(request *tasklessPermissionRequest) (decision, optionID, optionKind string) {
	if request.AutoApprovalPending && request.AutoApprovedOptionID != "" {
		return permissionDecisionApproved, request.AutoApprovedOptionID, request.AutoApprovedOptionKind
	}
	for _, option := range request.Options {
		if strings.HasPrefix(option.Kind, "reject") {
			return permissionDecisionRejected, option.OptionID, option.Kind
		}
	}
	return permissionDecisionCancelled, "", ""
}
