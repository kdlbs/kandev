package automation

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

type testOrchestratorTarget struct {
	calls  int
	prompt string
	fail   bool
}

func (t *testOrchestratorTarget) Validate(_ context.Context, ws, id string) error {
	if ws != "ws" || id != "chief" {
		return fmt.Errorf("workspace mismatch")
	}
	return nil
}
func (t *testOrchestratorTarget) Send(_ context.Context, _, _, _, prompt string) (string, error) {
	t.calls++
	t.prompt = prompt
	if t.fail {
		return "", fmt.Errorf("paused")
	}
	return "chief-conversation", nil
}
func TestAutomationDispatchesOnceWithoutOwningConversation(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	target := &testOrchestratorTarget{}
	svc.SetOrchestratorTarget(target)
	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{WorkspaceID: "ws", Name: "Daily review", OrchestratorID: "chief", Prompt: "Find PRs assigned to me"})
	require.NoError(t, err)
	result, err := svc.FireTrigger(ctx, a.ID, "", TriggerTypeScheduled, nil, "day:1")
	require.NoError(t, err)
	require.False(t, result.Skipped)
	require.NotEmpty(t, result.RunID)
	admittedRunID := result.RunID
	result, err = svc.FireTrigger(ctx, a.ID, "", TriggerTypeScheduled, nil, "day:1")
	require.NoError(t, err)
	require.True(t, result.Skipped)
	require.Equal(t, 1, target.calls)
	require.Contains(t, target.prompt, "Find PRs assigned to me")
	runs, err := svc.ListRuns(ctx, a.ID, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, admittedRunID, runs[0].ID)
	require.Equal(t, RunStatusDispatched, runs[0].Status)
	require.Equal(t, "chief-conversation", runs[0].ConversationTaskID)
	require.Empty(t, runs[0].TaskID)
	deleter := &fakeTaskDeleter{}
	svc.SetTaskDeleter(deleter)
	require.NoError(t, svc.DeleteRun(ctx, runs[0].ID))
	require.Empty(t, deleter.deleted)
	target.fail = true
	_, err = svc.FireTrigger(ctx, a.ID, "", TriggerTypeScheduled, nil, "day:2")
	require.ErrorContains(t, err, "paused")
	runs, err = svc.ListRuns(ctx, a.ID, 10)
	require.NoError(t, err)
	require.Equal(t, RunStatusFailed, runs[0].Status)
}

func TestOrchestratorTargetRejectsTaskExecutionOverrides(t *testing.T) {
	for _, configure := range []struct {
		name  string
		apply func(*Automation)
	}{
		{"task mode", func(a *Automation) { a.TaskMode = TaskModeNormalTask }},
		{"repository mode", func(a *Automation) { a.RepositoryMode = RepositoryModeSelected }},
		{"continuation", func(a *Automation) { a.ContinuationPolicy = ContinuationPolicyReuseThread }},
		{"repository binding", func(a *Automation) { a.Repositories = []AutomationRepository{{RepositoryID: "repo"}} }},
	} {
		t.Run(configure.name, func(t *testing.T) {
			svc := newTestService(t)
			svc.SetOrchestratorTarget(&testOrchestratorTarget{})
			a := &Automation{WorkspaceID: "ws", OrchestratorID: "chief", Prompt: "Summarize open tasks"}
			configure.apply(a)
			require.ErrorContains(t, svc.validateOrchestratorTarget(context.Background(), a), "overrides")
		})
	}
}
func TestAutomationTargetValidationAndRoundTrip(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	cfg := &CreateAutomationRequest{WorkspaceID: "ws", Name: "Review", OrchestratorID: "chief", Prompt: "Review PRs"}
	_, err := svc.CreateAutomation(ctx, cfg)
	require.ErrorContains(t, err, "disabled")
	svc.SetOrchestratorTarget(&testOrchestratorTarget{})
	cfg.WorkspaceID = "foreign"
	_, err = svc.CreateAutomation(ctx, cfg)
	require.Error(t, err)
	cfg.WorkspaceID = "ws"
	cfg.AgentProfileID = "override"
	_, err = svc.CreateAutomation(ctx, cfg)
	require.ErrorContains(t, err, "overrides")
	cfg.AgentProfileID = ""
	a, err := svc.CreateAutomation(ctx, cfg)
	require.NoError(t, err)
	require.Equal(t, "chief", a.OrchestratorID)
	bad := "foreign"
	_, err = svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{OrchestratorID: &bad})
	require.Error(t, err)
	empty := ""
	a, err = svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{OrchestratorID: &empty})
	require.NoError(t, err)
	require.Empty(t, a.OrchestratorID)
}
