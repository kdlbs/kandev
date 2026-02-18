package codex

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/pkg/codex"
	"go.uber.org/zap"
)

// handleRequest processes Codex requests (approval requests) and calls permissionHandler.
func (a *Adapter) handleRequest(id any, method string, params json.RawMessage) {
	a.logger.Debug("codex: received request",
		zap.Any("id", id),
		zap.String("method", method))

	a.mu.RLock()
	handler := a.permissionHandler
	a.mu.RUnlock()

	switch method {
	case codex.NotifyItemCmdExecRequestApproval:
		var p codex.CommandApprovalParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse command approval request", zap.Error(err))
			if err := a.client.SendResponse(id, nil, &codex.Error{Code: codex.InvalidParams, Message: "invalid params"}); err != nil {
				a.logger.Warn("failed to send invalid params response", zap.Error(err))
			}
			return
		}
		a.handleApprovalRequest(id, handler, p.ThreadID, p.ItemID, types.ActionTypeCommand, p.Command, map[string]any{
			"command":   p.Command,
			"cwd":       p.Cwd,
			"reasoning": p.Reasoning,
		}, p.Options)

	case codex.NotifyItemFileChangeRequestApproval:
		var p codex.FileChangeApprovalParams
		if err := json.Unmarshal(params, &p); err != nil {
			a.logger.Warn("failed to parse file change approval request", zap.Error(err))
			if err := a.client.SendResponse(id, nil, &codex.Error{Code: codex.InvalidParams, Message: "invalid params"}); err != nil {
				a.logger.Warn("failed to send invalid params response", zap.Error(err))
			}
			return
		}
		a.handleApprovalRequest(id, handler, p.ThreadID, p.ItemID, types.ActionTypeFileWrite, p.Path, map[string]any{
			"path":      p.Path,
			"diff":      p.Diff,
			"reasoning": p.Reasoning,
		}, p.Options)

	default:
		a.logger.Warn("unhandled request", zap.String("method", method))
		if err := a.client.SendResponse(id, nil, &codex.Error{Code: codex.MethodNotFound, Message: "method not found"}); err != nil {
			a.logger.Warn("failed to send method not found response", zap.Error(err))
		}
	}
}

// handleApprovalRequest handles permission request logic for both command and file change approvals.
func (a *Adapter) handleApprovalRequest(
	id any,
	handler PermissionHandler,
	threadID string,
	itemID string,
	actionType string,
	title string,
	details map[string]any,
	optionStrings []string,
) {
	req := &PermissionRequest{
		SessionID:     threadID,
		ToolCallID:    itemID,
		Title:         title,
		Options:       buildPermissionOptions(optionStrings),
		ActionType:    actionType,
		ActionDetails: details,
	}

	if handler == nil {
		// Auto-approve if no handler
		if err := a.client.SendResponse(id, &codex.CommandApprovalResponse{
			Decision: decisionAccept,
		}, nil); err != nil {
			a.logger.Warn("failed to send approval response", zap.Error(err))
		}
		return
	}

	ctx := context.Background()
	resp, err := handler(ctx, req)
	if err != nil {
		a.logger.Error("permission handler error", zap.Error(err))
		if err := a.client.SendResponse(id, &codex.CommandApprovalResponse{
			Decision: decisionDecline,
		}, nil); err != nil {
			a.logger.Warn("failed to send decline response", zap.Error(err))
		}
		return
	}

	decision := mapResponseToDecision(resp)
	if err := a.client.SendResponse(id, &codex.CommandApprovalResponse{
		Decision: decision,
	}, nil); err != nil {
		a.logger.Warn("failed to send approval response", zap.Error(err))
	}
}

// buildPermissionOptions converts Codex option strings to PermissionOption slice.
// Falls back to default approve/reject options when no options are provided.
func buildPermissionOptions(optionStrings []string) []PermissionOption {
	if len(optionStrings) == 0 {
		return []PermissionOption{
			{OptionID: "approve", Name: "Approve", Kind: "allow_once"},
			{OptionID: "reject", Name: "Reject", Kind: "reject_once"},
		}
	}
	options := make([]PermissionOption, len(optionStrings))
	for i, opt := range optionStrings {
		kind := "allow_once"
		switch opt {
		case "approveAlways":
			kind = "allow_always"
		case "reject":
			kind = "reject_once"
		}
		options[i] = PermissionOption{OptionID: opt, Name: opt, Kind: kind}
	}
	return options
}

// mapResponseToDecision maps a PermissionResponse to a Codex decision string.
// Codex accepts: "accept", "acceptForSession", "decline", "cancel".
func mapResponseToDecision(resp *PermissionResponse) string {
	if resp.Cancelled {
		return decisionCancel
	}
	switch resp.OptionID {
	case "approve", "allow", decisionAccept:
		return decisionAccept
	case "approveAlways", "allowAlways", decisionAcceptSession:
		return decisionAcceptSession
	case "reject", "deny", decisionDecline:
		return decisionDecline
	case decisionCancel:
		return decisionCancel
	default:
		if resp.OptionID != "" {
			return resp.OptionID
		}
		return decisionAccept
	}
}
