package handlers

import (
	"context"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/utility/controller"
	"github.com/kandev/kandev/internal/utility/service"
)

type profilePromptCall struct {
	sessionID string
	profileID string
}

type profilePromptExecutor struct {
	responses []*agentctlutil.PromptResponse
	calls     []profilePromptCall
}

func (e *profilePromptExecutor) ExecuteInferencePrompt(context.Context, string, string, string, string) (*agentctlutil.PromptResponse, error) {
	return nil, nil
}

func (e *profilePromptExecutor) ListInferenceAgentsWithContext(context.Context) []lifecycle.InferenceAgentInfo {
	return nil
}

func (e *profilePromptExecutor) ExecuteInferenceProfilePrompt(_ context.Context, sessionID, profileID, _ string) (*agentctlutil.PromptResponse, error) {
	e.calls = append(e.calls, profilePromptCall{sessionID: sessionID, profileID: profileID})
	response := e.responses[0]
	e.responses = e.responses[1:]
	return response, nil
}

func TestExecuteSessionProfilePromptUsesConcreteProfileAndUpdatesAttribution(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	repo := newFakeUtilityRepo()
	svc := service.NewService(repo)
	svc.SetExecutionProfileResolver(typedProfileFailureResolver{})
	ctrl := controller.NewController(svc)
	h := NewHandlers(ctrl, nil, nil, nil, log)
	call, err := svc.CreateCallWithExecutionProfile(context.Background(), "utility-1", "task-session", "prompt", "gpt-5", "logical", "first")
	if err != nil {
		t.Fatalf("CreateCallWithExecutionProfile: %v", err)
	}
	executor := &profilePromptExecutor{responses: []*agentctlutil.PromptResponse{
		{Success: false, Error: "too many requests"},
		{Success: true, Response: "done"},
	}}
	prepared := &service.PromptRequest{
		AgentProfileID:     "logical",
		ExecutionProfileID: "first",
		RouteSessionID:     "route-session",
		RouteGeneration:    1,
		AgentCLI:           "codex-acp",
		ResolvedPrompt:     "prompt",
	}

	response, err := h.executeSessionProfilePrompt(context.Background(), executor, "task-session", prepared, call.ID)
	if err != nil || response == nil || !response.Success {
		t.Fatalf("executeSessionProfilePrompt() = (%#v, %v), want success", response, err)
	}
	if len(executor.calls) != 2 {
		t.Fatalf("profile calls = %#v, want two attempts", executor.calls)
	}
	if executor.calls[0] != (profilePromptCall{sessionID: "task-session", profileID: "first"}) ||
		executor.calls[1] != (profilePromptCall{sessionID: "task-session", profileID: "second"}) {
		t.Fatalf("profile calls = %#v, want first then second concrete profile", executor.calls)
	}
	if got := repo.call(call.ID).ExecutionProfileID; got != "second" {
		t.Fatalf("execution profile attribution = %q, want second", got)
	}
}

func TestExecuteSessionProfilePromptDoesNotFallbackAfterPartialResponse(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	svc := service.NewService(newFakeUtilityRepo())
	svc.SetExecutionProfileResolver(typedProfileFailureResolver{})
	h := NewHandlers(controller.NewController(svc), nil, nil, nil, log)
	executor := &profilePromptExecutor{responses: []*agentctlutil.PromptResponse{
		{Success: false, Response: "partially generated answer", Error: "provider disconnected"},
		{Success: true, Response: "must not run"},
	}}
	prepared := &service.PromptRequest{
		AgentProfileID: "logical", ExecutionProfileID: "first", RouteSessionID: "route-session",
		RouteGeneration: 1, AgentCLI: "codex-acp", ResolvedPrompt: "prompt",
	}

	response, err := h.executeSessionProfilePrompt(context.Background(), executor, "task-session", prepared, "call")
	if err != nil || response == nil || response.Response != "partially generated answer" {
		t.Fatalf("response = %#v, err = %v", response, err)
	}
	if len(executor.calls) != 1 {
		t.Fatalf("profile calls = %#v, want one attempt", executor.calls)
	}
}

func TestExecuteSessionProfilePromptDoesNotRouteConcreteProfile(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	svc := service.NewService(newFakeUtilityRepo())
	svc.SetExecutionProfileResolver(typedProfileFailureResolver{})
	h := NewHandlers(controller.NewController(svc), nil, nil, nil, log)
	executor := &profilePromptExecutor{responses: []*agentctlutil.PromptResponse{
		{Success: false, Error: "provider disconnected"},
		{Success: true, Response: "must not run"},
	}}
	prepared := &service.PromptRequest{
		AgentProfileID: "concrete", ExecutionProfileID: "concrete", RouteSessionID: "utility:call",
		RouteGeneration: 1, AgentCLI: "codex-acp", ResolvedPrompt: "prompt",
	}

	response, err := h.executeSessionProfilePrompt(context.Background(), executor, "task-session", prepared, "call")
	if err != nil || response == nil || response.Error != "provider disconnected" {
		t.Fatalf("response = %#v, err = %v", response, err)
	}
	if len(executor.calls) != 1 {
		t.Fatalf("concrete profile calls = %#v, want one attempt", executor.calls)
	}
}

type typedProfileFailureResolver struct{}

func (typedProfileFailureResolver) ResolveExecution(context.Context, string) (*agentsettingsmodels.AgentProfile, string, error) {
	return nil, "", nil
}

func (typedProfileFailureResolver) ResolveExecutionAfterFailure(
	context.Context, string, string, string, int64, *routingerr.Error,
) (agentruntime.ProfileExecution, error) {
	return agentruntime.ProfileExecution{
		ExecutionProfileID: "second",
		Generation:         2,
		Profile:            &agentsettingsmodels.AgentProfile{ID: "second", AgentID: "claude-acp", Model: "sonnet"},
	}, nil
}
