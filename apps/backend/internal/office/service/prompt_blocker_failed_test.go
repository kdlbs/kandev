package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-OFFICE-BLOCKER-POLICY-002.8
func TestBuildPrompt_BlockerFailedFromEscalationPayload(t *testing.T) {
	f := newEscalationFixture(t)
	f.withParentRunner(t)
	f.setState(t, escalationBlocker, v1.TaskStateCancelled)
	run := f.wantEscalations(t, 1, "failure transition")[0]

	prompt := service.BuildPrompt(service.BuildPromptContextForTest(
		f.svc, context.Background(), service.RunReasonTaskBlockerFailed, run.Payload))

	for _, want := range []string{escalationBlocker, "cancelled", "- [dependent] dependent",
		"retried to completion, replaced, or its dependency edge is removed"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

// @covers AC-OFFICE-BLOCKER-POLICY-002.8
func TestBuildPrompt_BlockerFailedBoundsHeldDependents(t *testing.T) {
	held := make([]string, 23)
	for i := range held {
		held[i] = fmt.Sprintf("held-%02d", i)
	}
	payload, err := json.Marshal(map[string]any{
		"blocker_task_id": "blocker-1", "blocker_state": "FAILED", "held_dependent_ids": held,
	})
	blockerMust(t, err)
	prompt := service.BuildPrompt(service.BuildPromptContextForTest(
		newTestService(t), context.Background(), service.RunReasonTaskBlockerFailed, string(payload)))

	if !strings.Contains(prompt, "[blocker-1] is failed") || !strings.Contains(prompt, "- [held-19]") ||
		strings.Contains(prompt, "held-20") || !strings.Contains(prompt, "(+3 more)") {
		t.Fatalf("unexpected prompt:\n%s", prompt)
	}
}

func TestBuildPrompt_IncludesEveryCoalescedBlockerFailure(t *testing.T) {
	contextSnapshot := `{"blocker_failure_alerts":{
		"transition-a":{"parent_task_id":"parent-a","blocker_task_id":"blocker-a","blocker_state":"FAILED","held_dependent_ids":["dependent-a"]},
		"transition-b":{"parent_task_id":"parent-b","blocker_task_id":"blocker-b","blocker_state":"CANCELLED","held_dependent_ids":["dependent-b"]}
	}}`
	pc := service.BuildPromptContextWithSnapshotForTest(
		newTestService(t), context.Background(), service.RunReasonAgentError,
		`{"failed_agent_id":"agent-1"}`, contextSnapshot)
	prompt := service.BuildPrompt(pc)

	for _, want := range []string{"parent-a", "blocker-a", "dependent-a", "parent-b", "blocker-b", "dependent-b"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing coalesced alert detail %q:\n%s", want, prompt)
		}
	}
}
