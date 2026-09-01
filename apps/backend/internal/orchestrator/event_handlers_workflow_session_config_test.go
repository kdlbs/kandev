package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
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

	svc.processOnEnter(ctx, "task-config", session, step, "", 0)

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

	svc.processOnEnter(ctx, "task-config-skip", session, step, "", 0)

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

	svc.processOnEnter(ctx, "task-config-restore", session, step, "", 0)

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

	svc.processOnEnter(ctx, "task-config-display", session, step, "", 0)

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

	svc.processOnEnter(ctx, "task-config-unknown", session, step, "", 0)

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

	svc.processOnEnter(ctx, "task-config-other", session, step, "", 0)

	if len(agent.setSessionModelCalls) != 0 {
		t.Fatalf("non-matching agent was configured: %#v", agent.setSessionModelCalls)
	}
	if warnings := messages.workflowSessionConfigWarnings(); len(warnings) != 0 {
		t.Fatalf("deliberate no-op warned the user: %#v", warnings)
	}
}

// The launch path is the one the reported defect was observed on: a CREATED
// session has no prompt-ready ACP process, so the rule is persisted as a runtime
// override for lifecycle initialization to consume rather than applied live.
// Family resolution has to happen on that path too.
func TestApplyWorkflowSessionConfigBeforeLaunchMatchesAgentDisplayName(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-launch-display", "session-launch-display", "step-launch-display")
	if err := repo.SetSessionMetadataKey(ctx, "session-launch-display", models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial); err != nil {
		t.Fatalf("mark original session: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-launch-display")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"agent_name": "claude-acp"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session snapshot: %v", err)
	}

	agent := &mockAgentManager{isAgentReadyFn: func(context.Context, string) bool { return false }}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	step := configureSessionStep("step-launch-display", "Claude", "set", "opus[1m]", nil)

	svc.applyWorkflowSessionConfigBeforeLaunch(ctx, "task-launch-display", session, step)

	updated, err := repo.GetTaskSession(ctx, "session-launch-display")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	overrides, ok := models.LoadSessionRuntimeConfigOverrides(updated.Metadata)
	if !ok || overrides.Model != "opus[1m]" {
		t.Fatalf("runtime overrides = %#v, want launch model opus[1m]", overrides)
	}
}

// Without a resolver wired, matching must degrade to the exact comparison this
// used to perform rather than guessing. This is also the pre-fix behaviour, so
// it pins what the registry actually buys: the same inputs that now match are
// the ones that silently did nothing before.
func TestProcessOnEnterConfigureSessionWithoutResolverRequiresExactAgentName(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-config-noresolver", "session-config-noresolver", "step-config-noresolver")
	if err := repo.SetSessionMetadataKey(ctx, "session-config-noresolver", models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial); err != nil {
		t.Fatalf("mark original session: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-config-noresolver")
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
	svc.agentFamilyResolver = nil

	svc.processOnEnter(ctx, "task-config-noresolver", session,
		configureSessionStep("step-config-noresolver", "Claude", "set", "opus[1m]", nil), "", 0)
	if len(agent.setSessionModelCalls) != 0 {
		t.Fatalf("display name matched without a resolver: %#v", agent.setSessionModelCalls)
	}
	// Nothing is knowable about an agent family without a resolver, so an
	// unmatched rule must not be reported to the user as a workflow typo.
	if warnings := messages.workflowSessionConfigWarnings(); len(warnings) != 0 {
		t.Fatalf("warned without a resolver to judge with: %#v", warnings)
	}

	svc.processOnEnter(ctx, "task-config-noresolver", session,
		configureSessionStep("step-config-noresolver", "claude-acp", "set", "opus[1m]", nil), "", 0)
	if len(agent.setSessionModelCalls) != 1 || agent.setSessionModelCalls[0].ModelID != "opus[1m]" {
		t.Fatalf("exact agent name did not match without a resolver: %#v", agent.setSessionModelCalls)
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

// configureSessionRule is one (agent_name, model) pair for a multi-rule step.
type configureSessionRule struct {
	agentName string
	model     string
}

// multiRuleConfigureSessionStep builds a step whose configure_session action
// carries several "set" rules, in the given order.
func multiRuleConfigureSessionStep(id string, rules ...configureSessionRule) *wfmodels.WorkflowStep {
	raw := make([]interface{}, 0, len(rules))
	for _, rule := range rules {
		raw = append(raw, map[string]interface{}{
			"agent_name": rule.agentName,
			"operation":  "set",
			"model":      rule.model,
		})
	}
	return &wfmodels.WorkflowStep{ID: id, Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
		Type:   wfmodels.OnEnterConfigureSession,
		Config: map[string]interface{}{"rules": raw},
	}}}}
}

// seedConfigureSessionTest prepares an original, non-passthrough session whose
// stored agent family is agentName, plus a service wired to a live agent and a
// message creator so warnings are observable.
func seedConfigureSessionTest(
	t *testing.T,
	taskID, sessionID, agentName string,
) (*sqliterepo.Repository, *Service, *mockAgentManager, *mockMessageCreator, *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, "step-"+sessionID)
	if err := repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyOrigin, models.SessionOriginTaskInitial); err != nil {
		t.Fatalf("mark original session: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"agent_name": agentName}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session snapshot: %v", err)
	}

	agent := &mockAgentManager{isAgentRunning: true, setSessionModelSupported: true, setSessionConfigSupported: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	return repo, svc, agent, messages, session
}

// A custom TUI agent picks its own slug and display name and shares the
// built-ins' namespace, so "Claude" can name two different installed agents.
// Resolving one of them silently re-points the rule at an agent the workflow
// author never named — the same silent mis-application this card exists to kill.
func TestProcessOnEnterConfigureSessionAmbiguousAgentFamilyWarns(t *testing.T) {
	for _, tc := range []struct {
		name        string
		slug        string
		displayName string
	}{
		{name: "custom slug shadows builtin display name", slug: "claude", displayName: "My Claude"},
		{name: "custom display name collides", slug: "aaa-claude", displayName: "Claude"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo, svc, agent, messages, session := seedConfigureSessionTest(
				t, "task-ambiguous", "session-ambiguous", "claude-acp")

			shadowed := registry.NewRegistry(testLogger())
			shadowed.LoadDefaults()
			if err := shadowed.RegisterCustomTUIAgent(registry.CustomTUIAgentSpec{Slug: tc.slug, DisplayName: tc.displayName, Command: "claude-tui"}); err != nil {
				t.Fatalf("register custom agent: %v", err)
			}
			svc.agentFamilyResolver = shadowed

			svc.processOnEnter(ctx, "task-ambiguous", session,
				configureSessionStep("step-ambiguous", "Claude", "set", "opus[1m]", nil), "", 0)

			if len(agent.setSessionModelCalls) != 0 {
				t.Fatalf("ambiguous family was applied to an agent: %#v", agent.setSessionModelCalls)
			}
			warnings := messages.workflowSessionConfigWarnings()
			if len(warnings) != 1 || !strings.Contains(warnings[0], "more than one") {
				t.Fatalf("warnings = %#v, want one ambiguity warning", warnings)
			}
			updated, err := repo.GetTaskSession(ctx, "session-ambiguous")
			if err != nil {
				t.Fatalf("reload session: %v", err)
			}
			if overrides, ok := models.LoadSessionRuntimeConfigOverrides(updated.Metadata); ok && overrides.Model != "" {
				t.Fatalf("ambiguous family persisted a runtime override: %#v", overrides)
			}
		})
	}
}

// An ambiguous reference names several agents. If none of them is the session's
// family it cannot govern this step at all, so it is the same deliberate no-op as
// any other rule for a different agent — and it must not take the session's own
// rule down with it. Refusing the whole action here would hand a user who
// installed one colliding custom agent exactly the defect this card exists to
// kill: a per-step model that silently stops applying, for every agent at once.
func TestProcessOnEnterConfigureSessionUnrelatedAmbiguousRuleStillApplies(t *testing.T) {
	ctx := context.Background()
	repo, svc, agent, messages, session := seedConfigureSessionTest(
		t, "task-unrelated", "session-unrelated", "codex-acp")

	shadowed := registry.NewRegistry(testLogger())
	shadowed.LoadDefaults()
	// "Claude" now names both claude-acp and this agent — and neither is codex-acp.
	if err := shadowed.RegisterCustomTUIAgent(registry.CustomTUIAgentSpec{Slug: "aaa-claude", DisplayName: "Claude", Command: "claude-tui"}); err != nil {
		t.Fatalf("register custom agent: %v", err)
	}
	svc.agentFamilyResolver = shadowed

	svc.processOnEnter(ctx, "task-unrelated", session, multiRuleConfigureSessionStep("step-unrelated",
		configureSessionRule{agentName: "Codex", model: "gpt-5.6-luna"},
		configureSessionRule{agentName: "Claude", model: "opus[1m]"},
	), "", 0)

	if len(agent.setSessionModelCalls) != 1 || agent.setSessionModelCalls[0].ModelID != "gpt-5.6-luna" {
		t.Fatalf("model calls = %#v, want one gpt-5.6-luna switch", agent.setSessionModelCalls)
	}
	// The ambiguity is real but belongs to a step this session never runs under;
	// telling the user about it here would be noise on an applied rule.
	if warnings := messages.workflowSessionConfigWarnings(); len(warnings) != 0 {
		t.Fatalf("unrelated ambiguity warned the user: %#v", warnings)
	}
	updated, err := repo.GetTaskSession(ctx, "session-unrelated")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	overrides, ok := models.LoadSessionRuntimeConfigOverrides(updated.Metadata)
	if !ok || overrides.Model != "gpt-5.6-luna" {
		t.Fatalf("runtime overrides = %#v, want model gpt-5.6-luna", overrides)
	}
}

// The inverse boundary of the test above: an ambiguity that DOES name this
// session's family still refuses, and is not outvoted by an exact rule elsewhere
// in the same action. Both rules claim this session and only one of them says
// which agent it means, so array order would decide the model.
// Both orderings are exercised because the ambiguity verdict returns from inside
// the classification loop. Whichever rule the loop reaches first decides where
// the refusal comes from, so a single ordering would leave the other one free to
// change under a refactor — accumulating matches before checking for ambiguity,
// say — with nothing failing. Array order must not decide the outcome.
func TestProcessOnEnterConfigureSessionRelevantAmbiguityRefusesDespiteExactRule(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rules []configureSessionRule
	}{
		{
			name: "exact rule first",
			rules: []configureSessionRule{
				{agentName: "claude-acp", model: "sonnet"},
				{agentName: "Claude", model: "opus[1m]"},
			},
		},
		{
			// The ambiguous rule is classified first and returns immediately,
			// before the exact rule is ever looked at.
			name: "ambiguous rule first",
			rules: []configureSessionRule{
				{agentName: "Claude", model: "opus[1m]"},
				{agentName: "claude-acp", model: "sonnet"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo, svc, agent, messages, session := seedConfigureSessionTest(
				t, "task-relevant", "session-relevant", "claude-acp")

			shadowed := registry.NewRegistry(testLogger())
			shadowed.LoadDefaults()
			if err := shadowed.RegisterCustomTUIAgent(registry.CustomTUIAgentSpec{Slug: "aaa-claude", DisplayName: "Claude", Command: "claude-tui"}); err != nil {
				t.Fatalf("register custom agent: %v", err)
			}
			svc.agentFamilyResolver = shadowed

			svc.processOnEnter(ctx, "task-relevant", session,
				multiRuleConfigureSessionStep("step-relevant", tc.rules...), "", 0)

			if len(agent.setSessionModelCalls) != 0 {
				t.Fatalf("relevant ambiguity applied a model: %#v", agent.setSessionModelCalls)
			}
			warnings := messages.workflowSessionConfigWarnings()
			if len(warnings) != 1 || !strings.Contains(warnings[0], "more than one") {
				t.Fatalf("warnings = %#v, want one ambiguity warning", warnings)
			}
			updated, err := repo.GetTaskSession(ctx, "session-relevant")
			if err != nil {
				t.Fatalf("reload session: %v", err)
			}
			if overrides, ok := models.LoadSessionRuntimeConfigOverrides(updated.Metadata); ok && overrides.Model != "" {
				t.Fatalf("relevant ambiguity persisted a runtime override: %#v", overrides)
			}
		})
	}
}

// The session side of the same refusal. Sessions are expected to store a
// canonical agent ID, which resolves alone through the registry's exact-key path
// and can never come back ambiguous — so this branch only fires if that
// convention is broken by an import, a migration, or a hand-edited row. When it
// does, nothing about which agent the session runs is knowable, so no rule may be
// applied and the user is told why rather than silently getting the profile's
// model at every step.
func TestProcessOnEnterConfigureSessionAmbiguousSessionFamilyRefuses(t *testing.T) {
	ctx := context.Background()
	// Stored as the alias "Claude" rather than the canonical "claude-acp".
	repo, svc, agent, messages, session := seedConfigureSessionTest(
		t, "task-session-ambiguous", "session-session-ambiguous", "Claude")

	shadowed := registry.NewRegistry(testLogger())
	shadowed.LoadDefaults()
	// "Claude" now answers for both claude-acp and this agent.
	if err := shadowed.RegisterCustomTUIAgent(registry.CustomTUIAgentSpec{Slug: "aaa-claude", DisplayName: "Claude", Command: "claude-tui"}); err != nil {
		t.Fatalf("register custom agent: %v", err)
	}
	svc.agentFamilyResolver = shadowed

	// The rule names one agent unambiguously; the session is the ambiguous side.
	svc.processOnEnter(ctx, "task-session-ambiguous", session,
		configureSessionStep("step-session-ambiguous", "claude-acp", "set", "opus[1m]", nil), "", 0)

	if len(agent.setSessionModelCalls) != 0 {
		t.Fatalf("ambiguous session family applied a model: %#v", agent.setSessionModelCalls)
	}
	warnings := messages.workflowSessionConfigWarnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "more than one") {
		t.Fatalf("warnings = %#v, want one ambiguity warning", warnings)
	}
	updated, err := repo.GetTaskSession(ctx, "session-session-ambiguous")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if overrides, ok := models.LoadSessionRuntimeConfigOverrides(updated.Metadata); ok && overrides.Model != "" {
		t.Fatalf("ambiguous session family persisted a runtime override: %#v", overrides)
	}
}

// ParseConfigureSessionRules rejects duplicates by the raw agent_name only, so
// two textually different rules can name one family once references resolve.
// Array order deciding which model a step runs on is not a contract anyone
// wrote down, so neither is applied.
func TestProcessOnEnterConfigureSessionConflictingRulesWarn(t *testing.T) {
	ctx := context.Background()
	repo, svc, agent, messages, session := seedConfigureSessionTest(
		t, "task-conflict", "session-conflict", "claude-acp")

	svc.processOnEnter(ctx, "task-conflict", session, multiRuleConfigureSessionStep("step-conflict",
		configureSessionRule{agentName: "Claude", model: "opus[1m]"},
		configureSessionRule{agentName: "claude-acp", model: "sonnet"},
	), "", 0)

	if len(agent.setSessionModelCalls) != 0 {
		t.Fatalf("conflicting rules applied a model: %#v", agent.setSessionModelCalls)
	}
	warnings := messages.workflowSessionConfigWarnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "same agent") {
		t.Fatalf("warnings = %#v, want one conflicting-rules warning", warnings)
	}
	updated, err := repo.GetTaskSession(ctx, "session-conflict")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if overrides, ok := models.LoadSessionRuntimeConfigOverrides(updated.Metadata); ok && overrides.Model != "" {
		t.Fatalf("conflicting rules persisted a runtime override: %#v", overrides)
	}
}

// The conflict check must not fire on the normal shape it most resembles: one
// rule per family, several families, exactly one of them the session's.
func TestProcessOnEnterConfigureSessionDistinctFamiliesStillApply(t *testing.T) {
	ctx := context.Background()
	_, svc, agent, messages, session := seedConfigureSessionTest(
		t, "task-distinct", "session-distinct", "claude-acp")

	svc.processOnEnter(ctx, "task-distinct", session, multiRuleConfigureSessionStep("step-distinct",
		configureSessionRule{agentName: "Codex", model: "gpt-5.6-luna"},
		configureSessionRule{agentName: "Claude", model: "opus[1m]"},
		configureSessionRule{agentName: "Gemini", model: "gemini-pro"},
	), "", 0)

	if len(agent.setSessionModelCalls) != 1 || agent.setSessionModelCalls[0].ModelID != "opus[1m]" {
		t.Fatalf("model calls = %#v, want one opus[1m] switch", agent.setSessionModelCalls)
	}
	if warnings := messages.workflowSessionConfigWarnings(); len(warnings) != 0 {
		t.Fatalf("one-rule-per-family warned the user: %#v", warnings)
	}
}
