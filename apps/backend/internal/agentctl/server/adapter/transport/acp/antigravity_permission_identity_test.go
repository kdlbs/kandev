package acp

import (
	"context"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestAntigravityCallIdentityPublishesWithActiveCall(t *testing.T) {
	a := newTestAdapterForAgent(acpcompat.AntigravityAgentID)
	a.cfg.AuthPolicy = &acpcompat.AuthPolicy{RuntimeVariant: acpcompat.AntigravityRuntimeCLI}
	toolName := "mcp_kandev_step_complete_kandev"
	call := &acpsdk.SessionUpdateToolCall{
		ToolCallId: "call-1",
		Name:       &toolName,
		Kind:       acpsdk.ToolKindOther,
	}

	a.mu.Lock()
	writerStarted := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		close(writerStarted)
		a.trackToolCallPayload("session-1", "call-1", &streams.NormalizedPayload{}, call)
		close(writerDone)
	}()
	<-writerStarted

	waitCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	type waitResult struct {
		tracked   bool
		name      *string
		ambiguous bool
	}
	waitDone := make(chan waitResult, 1)
	go func() {
		tracked, name, ambiguous := a.waitForAntigravityPermissionToolCall(waitCtx, "session-1", "call-1", time.Second)
		waitDone <- waitResult{tracked: tracked, name: name, ambiguous: ambiguous}
	}()
	a.mu.Unlock()

	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("tool notification did not publish")
	}
	select {
	case result := <-waitDone:
		if !result.tracked || result.name == nil || *result.name != toolName || result.ambiguous {
			t.Fatalf("identity waiter result = %+v, want the published tool identity", result)
		}
	case <-time.After(time.Second):
		t.Fatal("permission waiter did not observe active call")
	}

	observed, ambiguous := a.antigravityPermissionToolName("session-1", "call-1")
	if ambiguous || observed == nil || *observed != toolName {
		t.Fatalf("identity after active publication = (%v, %t), want %q", observed, ambiguous, toolName)
	}
}

func TestAntigravityPermissionIdentityWaitUsesSessionAndCallBoundary(t *testing.T) {
	a := newTestAdapterForAgent(acpcompat.AntigravityAgentID)
	a.cfg.AuthPolicy = &acpcompat.AuthPolicy{RuntimeVariant: acpcompat.AntigravityRuntimeCLI}
	otherName := "mcp_kandev_step_complete_kandev"
	a.mu.Lock()
	a.antigravityPermissionToolNames[permissionToolNameKey{sessionID: "other-session", toolCallID: "call-1"}] = permissionToolName{name: otherName}
	a.activeToolCalls["call-1"] = &streams.NormalizedPayload{}
	a.mu.Unlock()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	tracked, observed, ambiguous := a.waitForAntigravityPermissionToolCall(ctx, "session-1", "call-1", time.Second)
	if tracked || observed != nil || ambiguous {
		t.Fatalf("unmatched session identity = (%t, %v, %t), want no identity", tracked, observed, ambiguous)
	}
}

func TestAntigravityCallIdentityConflictSurvivesTypedName(t *testing.T) {
	a := newTestAdapterForAgent(acpcompat.AntigravityAgentID)
	a.cfg.AuthPolicy = &acpcompat.AuthPolicy{RuntimeVariant: acpcompat.AntigravityRuntimeCLI}
	firstName := "mcp_kandev_step_complete_kandev"
	secondName := "mcp_other_search"
	a.mu.Lock()
	a.trackAntigravityPermissionToolNameLocked("session-1", &acpsdk.SessionUpdateToolCall{
		ToolCallId: "call-1", Name: &firstName,
	})
	a.trackAntigravityPermissionToolNameLocked("session-1", &acpsdk.SessionUpdateToolCall{
		ToolCallId: "call-1", Name: &secondName,
	})
	a.activeToolCalls["call-1"] = &streams.NormalizedPayload{}
	a.mu.Unlock()

	observed, ambiguous := a.antigravityPermissionToolName("session-1", "call-1")
	if !ambiguous || observed != nil {
		t.Fatalf("conflicting identity = (%v, %t), want ambiguity", observed, ambiguous)
	}
	requestName := firstName
	request := &PermissionRequest{
		SessionID: "session-1", ToolCallID: "call-1", ToolName: &requestName,
	}
	observedRequest := make(chan *PermissionRequest, 1)
	a.SetPermissionHandler(func(_ context.Context, req *PermissionRequest) (*PermissionResponse, error) {
		observedRequest <- req
		return &PermissionResponse{Cancelled: true}, nil
	})
	if _, err := a.handlePermissionRequest(t.Context(), request); err != nil {
		t.Fatalf("handlePermissionRequest: %v", err)
	}
	select {
	case got := <-observedRequest:
		if got.ToolName == nil || *got.ToolName != "" {
			t.Fatalf("typed name survived conflicting notifications: %v", got.ToolName)
		}
	case <-time.After(time.Second):
		t.Fatal("permission handler did not receive request")
	}
}
