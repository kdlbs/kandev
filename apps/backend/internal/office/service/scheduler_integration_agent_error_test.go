package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
)

// TestSchedulerIntegration_BuildPromptContext_AgentError_PathA drives
// buildPromptContext with the on_agent_error queue_run payload shape
// (failed_agent_id/failed_session_id/error, as projected by
// workflow/engine.queueRunPayload) and asserts the rendered CEO prompt
// names the real failure instead of falling back to "unknown".
func TestSchedulerIntegration_BuildPromptContext_AgentError_PathA(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	payload := `{"task_id":"task-1","failed_agent_id":"worker-1","failed_session_id":"sess-1","error":"exit status 1: context deadline exceeded"}`
	pc := service.BuildPromptContextForTest(svc, ctx, service.RunReasonAgentError, payload)
	prompt := service.BuildPrompt(pc)

	for _, want := range []string{"worker-1", "sess-1", "exit status 1: context deadline exceeded"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("Path A agent_error prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "unknown") {
		t.Errorf("Path A agent_error prompt should not say unknown:\n%s", prompt)
	}
}

// TestSchedulerIntegration_BuildPromptContext_AgentError_PathB drives
// buildPromptContext with the pre-existing queueCEOAgentError payload shape
// (failed_agent_id/run_id/error, retry.go) and asserts the same outcome.
func TestSchedulerIntegration_BuildPromptContext_AgentError_PathB(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	payload := `{"failed_agent_id":"worker-2","run_id":"run-9","error":"provider timeout"}`
	pc := service.BuildPromptContextForTest(svc, ctx, service.RunReasonAgentError, payload)
	prompt := service.BuildPrompt(pc)

	for _, want := range []string{"worker-2", "provider timeout"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("Path B agent_error prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "unknown") {
		t.Errorf("Path B agent_error prompt should not say unknown:\n%s", prompt)
	}
}
