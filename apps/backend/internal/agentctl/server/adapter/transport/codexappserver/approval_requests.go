package codexappserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	agenttypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

const (
	codexDecisionAccept    = "accept"
	codexDecisionAcceptRaw = `"` + codexDecisionAccept + `"`
)

func (a *Adapter) handleServerRequest(ctx context.Context, request protocol.ServerRequest) (any, error) {
	method := request.Method
	if serverRequestDispositions[method] != serverRequestSupported {
		return nil, &protocol.RPCError{Code: -32601, Message: "method not found"}
	}
	if method == protocol.ServerRequestToolUserInput {
		return a.handleUserInputRequest(ctx, request.Params)
	}
	var params map[string]any
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return nil, &protocol.RPCError{Code: -32602, Message: "invalid approval request"}
	}
	var rawParams map[string]json.RawMessage
	if err := json.Unmarshal(request.Params, &rawParams); err != nil {
		return nil, &protocol.RPCError{Code: -32602, Message: "invalid approval request"}
	}
	threadID := stringField(params, "threadId")
	itemID := stringField(params, "itemId")
	if threadID == "" || itemID == "" {
		return nil, &protocol.RPCError{Code: -32602, Message: "approval request is missing its thread or item ID"}
	}
	permissionRequest := codexApprovalPermissionRequest(params, method, threadID, itemID)
	choices := defaultCodexApprovalDecisions()
	if availableDecisions := rawParams["availableDecisions"]; len(availableDecisions) > 0 && string(availableDecisions) != "null" {
		options, offeredChoices, err := codexApprovalDecisionOptions(availableDecisions)
		if err != nil {
			return nil, &protocol.RPCError{Code: -32602, Message: "invalid offered approval decisions"}
		}
		permissionRequest.Options = options
		choices = offeredChoices
	}
	return a.resolvePermissionRequest(ctx, permissionRequest, choices)
}

func codexApprovalPermissionRequest(params map[string]any, method, threadID, itemID string) *agenttypes.PermissionRequest {
	title := "Run command"
	actionType := string(streams.ActionTypeCommand)
	fields := []string{"command", "cwd", "reason", "commandActions"}
	if method != protocol.ServerRequestCommandExecutionApproval {
		title = "Apply file changes"
		actionType = string(streams.ActionTypeFileWrite)
		fields = []string{"grantRoot", "reason", "fileChanges"}
	}
	actionDetails := make(map[string]any, len(fields))
	for _, key := range fields {
		if value, ok := params[key]; ok {
			actionDetails[key] = value
		}
	}
	return &agenttypes.PermissionRequest{
		SessionID:     threadID,
		ToolCallID:    itemID,
		Title:         title,
		ActionType:    actionType,
		ActionDetails: actionDetails,
		Options: []agenttypes.PermissionOption{
			{OptionID: "allow-once", Name: "Allow once", Kind: streams.PermissionOptionKindAllowOnce},
			{OptionID: "allow-always", Name: "Allow for this session", Kind: streams.PermissionOptionKindAllowAlways},
			{OptionID: "reject-once", Name: "Reject", Kind: streams.PermissionOptionKindRejectOnce},
		},
	}
}

func (a *Adapter) resolvePermissionRequest(ctx context.Context, req *agenttypes.PermissionRequest, choices map[string]json.RawMessage) (any, error) {
	a.mu.RLock()
	handler := a.permission
	a.mu.RUnlock()
	if handler == nil {
		for _, decision := range choices {
			if string(decision) == `"decline"` {
				return map[string]any{"decision": decision}, nil
			}
		}
		return nil, &protocol.RPCError{Code: -32602, Message: "approval request has no safe default decision"}
	}
	response, err := handler(ctx, req)
	if err != nil || response == nil || response.Cancelled {
		return map[string]any{"decision": "cancel"}, nil
	}
	decision, ok := choices[response.OptionID]
	if !ok {
		return nil, &protocol.RPCError{Code: -32602, Message: "approval choice was not offered"}
	}
	return map[string]any{"decision": decision}, nil
}

func defaultCodexApprovalDecisions() map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"allow-once":   json.RawMessage(codexDecisionAcceptRaw),
		"allow-always": json.RawMessage(`"acceptForSession"`),
		"reject-once":  json.RawMessage(`"decline"`),
	}
}

func codexApprovalDecisionOptions(raw json.RawMessage) ([]agenttypes.PermissionOption, map[string]json.RawMessage, error) {
	var decisions []json.RawMessage
	if err := json.Unmarshal(raw, &decisions); err != nil || decisions == nil || len(decisions) == 0 {
		return nil, nil, errors.New("availableDecisions must be a non-empty array")
	}
	options := make([]agenttypes.PermissionOption, 0, len(decisions))
	choices := make(map[string]json.RawMessage, len(decisions))
	for index, rawDecision := range decisions {
		label, kind, decisionKind, err := codexApprovalDecisionPresentation(rawDecision)
		if err != nil {
			return nil, nil, err
		}
		optionID := fmt.Sprintf("codex-app-server-decision-%d", index)
		options = append(options, agenttypes.PermissionOption{
			OptionID: optionID,
			Name:     label,
			Kind:     kind,
			Metadata: map[string]interface{}{
				"codex_app_server": true,
				"codex_decision":   decisionKind,
			},
		})
		choices[optionID] = append(json.RawMessage(nil), rawDecision...)
	}
	return options, choices, nil
}

func codexApprovalDecisionPresentation(raw json.RawMessage) (label string, kind streams.PermissionOptionKind, decisionKind string, err error) {
	var decision string
	if json.Unmarshal(raw, &decision) == nil {
		return codexStringDecisionPresentation(decision)
	}
	var structured map[string]json.RawMessage
	if err := json.Unmarshal(raw, &structured); err != nil || len(structured) != 1 {
		return "", "", "", errors.New("approval decision must be a string or single-key object")
	}
	for name, value := range structured {
		return codexStructuredDecisionPresentation(name, value)
	}
	return "", "", "", errors.New("empty approval decision")
}

func codexStringDecisionPresentation(decision string) (string, streams.PermissionOptionKind, string, error) {
	switch decision {
	case codexDecisionAccept:
		return "Approve once", streams.PermissionOptionKindAllowOnce, codexDecisionAccept, nil
	case "acceptForSession":
		return "Always allow", streams.PermissionOptionKindAllowAlways, "accept_for_session", nil
	case "decline":
		return "Deny", streams.PermissionOptionKindRejectOnce, "decline", nil
	case "cancel":
		return "Cancel turn", streams.PermissionOptionKindRejectOnce, "cancel", nil
	default:
		return "", "", "", fmt.Errorf("unknown approval decision %q", decision)
	}
}

func codexStructuredDecisionPresentation(name string, value json.RawMessage) (string, streams.PermissionOptionKind, string, error) {
	switch name {
	case "acceptWithExecpolicyAmendment":
		return "Approve with command policy", streams.PermissionOptionKindAllowAlways, "accept_with_execpolicy_amendment", nil
	case "applyNetworkPolicyAmendment":
		return codexNetworkPolicyPresentation(value)
	default:
		return "", "", "", fmt.Errorf("unknown approval decision %q", name)
	}
}

func codexNetworkPolicyPresentation(value json.RawMessage) (string, streams.PermissionOptionKind, string, error) {
	var amendment struct {
		NetworkPolicyAmendment struct {
			Action string `json:"action"`
		} `json:"networkPolicyAmendment"`
	}
	if err := json.Unmarshal(value, &amendment); err != nil {
		return "", "", "", errors.New("invalid network policy approval decision")
	}
	switch amendment.NetworkPolicyAmendment.Action {
	case "deny":
		return "Block network access", streams.PermissionOptionKindRejectAlways, "apply_network_policy_deny", nil
	case "allow":
		return "Allow network access", streams.PermissionOptionKindAllowAlways, "apply_network_policy_allow", nil
	default:
		return "", "", "", errors.New("network policy approval decision has no recognized action")
	}
}
