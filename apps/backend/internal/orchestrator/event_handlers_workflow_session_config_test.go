package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestProcessOnEnterConfigureSessionAppliesMatchingOriginalSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-config", "session-config", "step-config")
	if err := repo.SetSessionMetadataKey(ctx, "session-config", models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial); err != nil {
		t.Fatalf("mark original session: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-config")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"agent_name": "codex"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session snapshot: %v", err)
	}

	agent := &mockAgentManager{isAgentRunning: true, setSessionModelSupported: true, setSessionConfigSupported: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	step := configureSessionStep("step-config", "codex", "set", "gpt-5.6-luna", map[string]string{"reasoning_effort": "max"})

	svc.processOnEnter(ctx, "task-config", session, step, "")

	if len(agent.setSessionModelCalls) != 1 || agent.setSessionModelCalls[0].ModelID != "gpt-5.6-luna" {
		t.Fatalf("model calls = %#v, want one luna switch", agent.setSessionModelCalls)
	}
	if len(agent.setSessionConfigCalls) != 1 || agent.setSessionConfigCalls[0].ConfigID != "reasoning_effort" || agent.setSessionConfigCalls[0].Value != "max" {
		t.Fatalf("config calls = %#v, want max effort", agent.setSessionConfigCalls)
	}

	updated, err := repo.GetTaskSession(ctx, "session-config")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	overrides, ok := models.LoadSessionRuntimeConfigOverrides(updated.Metadata)
	if !ok || overrides.Model != "gpt-5.6-luna" || overrides.ConfigOptions["reasoning_effort"] != "max" {
		t.Fatalf("runtime overrides = %#v, want model and effort", overrides)
	}
}

func TestProcessOnEnterConfigureSessionSkipsNonMatchingAgent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-config-skip", "session-config-skip", "step-config-skip")
	if err := repo.SetSessionMetadataKey(ctx, "session-config-skip", models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial); err != nil {
		t.Fatalf("mark original session: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-config-skip")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"agent_name": "grok"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session snapshot: %v", err)
	}

	agent := &mockAgentManager{isAgentRunning: true, setSessionModelSupported: true, setSessionConfigSupported: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	step := configureSessionStep("step-config-skip", "codex", "set", "gpt-5.6-luna", map[string]string{"reasoning_effort": "max"})

	svc.processOnEnter(ctx, "task-config-skip", session, step, "")

	if len(agent.setSessionModelCalls) != 0 || len(agent.setSessionConfigCalls) != 0 {
		t.Fatalf("non-matching agent was configured: model=%#v config=%#v", agent.setSessionModelCalls, agent.setSessionConfigCalls)
	}
}

func TestProcessOnEnterConfigureSessionRestoreOriginal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-config-restore", "session-config-restore", "step-config-restore")
	if err := repo.SetSessionMetadataKey(ctx, "session-config-restore", models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial); err != nil {
		t.Fatalf("mark original session: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-config-restore")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"agent_name": "claude"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session snapshot: %v", err)
	}
	original := models.SessionOriginalEffectiveConfiguration{
		Model:         "claude-sonnet-original",
		ConfigOptions: map[string]string{"reasoning_effort": "low"},
	}
	if err := repo.SetSessionMetadataKey(ctx, "session-config-restore", models.SessionMetaKeyOriginalEffectiveConfig, original); err != nil {
		t.Fatalf("persist original config: %v", err)
	}

	agent := &mockAgentManager{isAgentRunning: true, setSessionModelSupported: true, setSessionConfigSupported: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	step := configureSessionStep("step-config-restore", "claude", "restore_original", "", nil)

	svc.processOnEnter(ctx, "task-config-restore", session, step, "")

	if len(agent.setSessionModelCalls) != 1 || agent.setSessionModelCalls[0].ModelID != "claude-sonnet-original" {
		t.Fatalf("restore model calls = %#v", agent.setSessionModelCalls)
	}
	if len(agent.setSessionConfigCalls) != 1 || agent.setSessionConfigCalls[0].Value != "low" {
		t.Fatalf("restore config calls = %#v", agent.setSessionConfigCalls)
	}
}

func TestApplyWorkflowSessionConfigBeforeLaunchPersistsRuntimeLayer(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-config-start", "session-config-start", "step-config-start")
	if err := repo.SetSessionMetadataKey(ctx, "session-config-start", models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial); err != nil {
		t.Fatalf("mark original session: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-config-start")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"agent_name": "codex"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session snapshot: %v", err)
	}

	// A CREATED session has no prompt-ready ACP process yet. The matching rule
	// must be durable before lifecycle initialization consumes it.
	agent := &mockAgentManager{isAgentReadyFn: func(context.Context, string) bool { return false }}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	step := configureSessionStep("step-config-start", "codex", "set", "gpt-5.6-luna", map[string]string{"reasoning_effort": "max"})

	svc.applyWorkflowSessionConfigBeforeLaunch(ctx, "task-config-start", session, step)

	updated, err := repo.GetTaskSession(ctx, "session-config-start")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	overrides, ok := models.LoadSessionRuntimeConfigOverrides(updated.Metadata)
	if !ok || overrides.Model != "gpt-5.6-luna" || overrides.ConfigOptions["reasoning_effort"] != "max" {
		t.Fatalf("runtime overrides = %#v, want launch model and effort", overrides)
	}
}

// Workflow definitions are hand-authored and name agent families the way a
// person writes them ("Claude"), while a session stores the canonical agent ID
// ("claude-acp"). Matching those with a raw string comparison silently dropped
// every rule in the shipped workflows, so the step prompt ran on the agent
// profile's model at every step.
func TestProcessOnEnterConfigureSessionMatchesAgentDisplayName(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-config-display", "session-config-display", "step-config-display")
	if err := repo.SetSessionMetadataKey(ctx, "session-config-display", models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial); err != nil {
		t.Fatalf("mark original session: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-config-display")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"agent_name": "claude-acp"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session snapshot: %v", err)
	}

	agent := &mockAgentManager{isAgentRunning: true, setSessionModelSupported: true, setSessionConfigSupported: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	step := configureSessionStep("step-config-display", "Claude", "set", "opus[1m]", nil)

	svc.processOnEnter(ctx, "task-config-display", session, step, "")

	if len(agent.setSessionModelCalls) != 1 || agent.setSessionModelCalls[0].ModelID != "opus[1m]" {
		t.Fatalf("model calls = %#v, want one opus[1m] switch", agent.setSessionModelCalls)
	}
	updated, err := repo.GetTaskSession(ctx, "session-config-display")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	overrides, ok := models.LoadSessionRuntimeConfigOverrides(updated.Metadata)
	if !ok || overrides.Model != "opus[1m]" {
		t.Fatalf("runtime overrides = %#v, want model opus[1m]", overrides)
	}
	if warnings := messages.workflowSessionConfigWarnings(); len(warnings) != 0 {
		t.Fatalf("applied rule warned the user: %#v", warnings)
	}
}

// A rule naming a family that resolves to no known agent is a typo in the
// workflow, and losing it silently is what made this defect invisible.
func TestProcessOnEnterConfigureSessionUnknownAgentFamilyWarns(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-config-unknown", "session-config-unknown", "step-config-unknown")
	if err := repo.SetSessionMetadataKey(ctx, "session-config-unknown", models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial); err != nil {
		t.Fatalf("mark original session: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-config-unknown")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"agent_name": "claude-acp"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session snapshot: %v", err)
	}

	agent := &mockAgentManager{isAgentRunning: true, setSessionModelSupported: true, setSessionConfigSupported: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	step := configureSessionStep("step-config-unknown", "grok-9000", "set", "opus[1m]", nil)

	svc.processOnEnter(ctx, "task-config-unknown", session, step, "")

	if len(agent.setSessionModelCalls) != 0 {
		t.Fatalf("unknown family was configured: %#v", agent.setSessionModelCalls)
	}
	if warnings := messages.workflowSessionConfigWarnings(); len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one unresolvable-family warning", warnings)
	}
}

// A rule for a real but different agent is a deliberate no-op, not a typo, so
// it must stay silent even though nothing was applied.
func TestProcessOnEnterConfigureSessionKnownOtherAgentStaysSilent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-config-other", "session-config-other", "step-config-other")
	if err := repo.SetSessionMetadataKey(ctx, "session-config-other", models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial); err != nil {
		t.Fatalf("mark original session: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-config-other")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"agent_name": "claude-acp"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session snapshot: %v", err)
	}

	agent := &mockAgentManager{isAgentRunning: true, setSessionModelSupported: true, setSessionConfigSupported: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	step := configureSessionStep("step-config-other", "Codex", "set", "gpt-5.6-luna", nil)

	svc.processOnEnter(ctx, "task-config-other", session, step, "")

	if len(agent.setSessionModelCalls) != 0 {
		t.Fatalf("non-matching agent was configured: %#v", agent.setSessionModelCalls)
	}
	if warnings := messages.workflowSessionConfigWarnings(); len(warnings) != 0 {
		t.Fatalf("deliberate no-op warned the user: %#v", warnings)
	}
}

// workflowSessionConfigWarnings returns the user-visible warnings raised by
// configure_session handling, identified by the marker warnWorkflowSessionConfig
// stamps on their metadata.
func (m *mockMessageCreator) workflowSessionConfigWarnings() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	warnings := make([]string, 0)
	for _, message := range m.sessionMessages {
		if marker, _ := message.metadata["workflow_session_config"].(bool); marker {
			warnings = append(warnings, message.content)
		}
	}
	return warnings
}

func configureSessionStep(id, agentName, operation, model string, options map[string]string) *wfmodels.WorkflowStep {
	config := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"agent_name": agentName,
				"operation":  operation,
			},
		},
	}
	rule := config["rules"].([]interface{})[0].(map[string]interface{})
	if model != "" {
		rule["model"] = model
	}
	if len(options) > 0 {
		raw := make(map[string]interface{}, len(options))
		for key, value := range options {
			raw[key] = value
		}
		rule["config_options"] = raw
	}
	return &wfmodels.WorkflowStep{ID: id, Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
		Type:   wfmodels.OnEnterConfigureSession,
		Config: config,
	}}}}
}
