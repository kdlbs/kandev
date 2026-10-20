package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

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
	decision, optionID, optionKind := decideTasklessPermission(request)
	s.AppendRunEvent(ctx, run.ID, runEventTypePermissionDecision, "info", map[string]interface{}{
		"decision":       decision,
		"option_kind":    optionKind,
		"title":          request.Title,
		"run_session_id": data.RunSessionID,
		"execution_id":   data.AgentExecutionID,
		"pending_id":     request.PendingID,
	})
	if decision == permissionDecisionCancelled {
		err = s.runPermissions.CancelRunPermission(ctx, data.AgentExecutionID, request.RequestID, request.PendingID)
	} else {
		err = s.runPermissions.ResolveRunPermission(ctx, data.AgentExecutionID, request.RequestID, request.PendingID, optionID)
	}
	if err != nil && !runtimeapi.IsNotFound(err) {
		s.logger.Warn("taskless permission answer failed",
			zap.String("run_id", run.ID), zap.String("run_session_id", data.RunSessionID),
			zap.String("decision", decision), zap.Error(err))
	}
	return nil
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
