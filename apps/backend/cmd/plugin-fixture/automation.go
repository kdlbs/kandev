package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"strings"
)

func (*fixturePlugin) DescribeAutomationCondition(_ context.Context, req *pluginsdk.AutomationConditionRequest) (*pluginsdk.AutomationConditionResponse, error) {
	return &pluginsdk.AutomationConditionResponse{Available: req.WorkspaceId != "", ConnectionId: req.WorkspaceId, ConnectionRevision: "fixture-1"}, nil
}
func (*fixturePlugin) VerifyAutomationWebhook(_ context.Context, req *pluginsdk.AutomationWebhookRequest) (*pluginsdk.AutomationWebhookResponse, error) {
	signature := req.Headers["x-fixture-signature"]
	digest, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	mac := hmac.New(sha256.New, []byte(req.Secret))
	_, _ = mac.Write(req.Body)
	if err != nil || !strings.HasPrefix(signature, "sha256=") || !hmac.Equal(digest, mac.Sum(nil)) {
		return &pluginsdk.AutomationWebhookResponse{Outcome: "rejected"}, nil
	}
	if !json.Valid(req.Body) {
		return &pluginsdk.AutomationWebhookResponse{Outcome: "malformed"}, nil
	}
	return &pluginsdk.AutomationWebhookResponse{Outcome: "accepted", EventKind: req.ConditionKey, Data: req.Body}, nil
}
