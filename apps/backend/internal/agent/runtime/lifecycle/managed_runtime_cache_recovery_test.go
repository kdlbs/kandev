package lifecycle

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

func TestManagedRuntimeCacheStartupEvidenceRequiresVerifiedMatch(t *testing.T) {
	agent := agents.NewClaudeACP()
	spec, ok := agents.BuiltinManagedNPMRuntime(agent.ID())
	if !ok {
		t.Fatal("Claude ACP is not a registered managed runtime")
	}
	packageSpec := spec.PackageSpec("1.2.3")
	args := spec.RuntimeCommand("1.2.3").Args()
	newEvidence := func() *agentctltypes.ManagedStartupEvidence {
		return &agentctltypes.ManagedStartupEvidence{
			ProcessGeneration: 7, ExitDisposition: agentctltypes.ManagedStartupExitOrdinary,
			CollectionComplete: true, NPMDiagnosticPresent: true, NPMDiagnosticComplete: true,
			UnclassifiedNPMCode: true,
			CacheEvidence: &agentctltypes.ManagedRuntimeCacheEvidence{
				Condition:   agentctltypes.ManagedRuntimeCacheRootManifestMissing,
				PackageSpec: packageSpec, ProcessGeneration: 7,
			},
		}
	}
	valid := newEvidence()
	retry := managedRuntimeStartupRetryForEvidence(valid, nil, nil, args, packageSpec)
	if retry == nil || retry.preparation == nil || retry.preparation.PackageSpec != packageSpec ||
		retry.preparation.FailedProcessGeneration != 7 || !slices.Equal(retry.retryArgs, args) {
		t.Fatalf("verified ENOENT recovery = %#v, want an exact-version private preparation", retry)
	}

	for _, test := range []struct {
		name   string
		mutate func(*agentctltypes.ManagedStartupEvidence)
	}{
		{name: "unclassified ENOENT without cache evidence", mutate: func(e *agentctltypes.ManagedStartupEvidence) { e.CacheEvidence = nil }},
		{name: "wrong condition", mutate: func(e *agentctltypes.ManagedStartupEvidence) { e.CacheEvidence.Condition = "other_missing_file" }},
		{name: "wrong exact package", mutate: func(e *agentctltypes.ManagedStartupEvidence) { e.CacheEvidence.PackageSpec = spec.PackageSpec("1.2.4") }},
		{name: "cache generation mismatch", mutate: func(e *agentctltypes.ManagedStartupEvidence) { e.CacheEvidence.ProcessGeneration++ }},
		{name: "incomplete diagnostics", mutate: func(e *agentctltypes.ManagedStartupEvidence) { e.NPMDiagnosticComplete = false }},
		{name: "intentional stop", mutate: func(e *agentctltypes.ManagedStartupEvidence) {
			e.ExitDisposition = agentctltypes.ManagedStartupExitIntentional
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			evidence := newEvidence()
			test.mutate(evidence)
			if retry := managedRuntimeStartupRetryForEvidence(evidence, nil, nil, args, packageSpec); retry != nil {
				t.Fatalf("unverified evidence produced retry %#v", retry)
			}
		})
	}
	if slices.Contains(args, "--prefer-online") {
		t.Fatal("cache recovery command should preserve the selected offline package command")
	}
}

func TestManagedRuntimeCacheResume(t *testing.T) {
	t.Run("managed ACP", func(t *testing.T) {
		runManagedRuntimeCacheResume(t, agents.NewClaudeACP(), nil, true)
	})
	t.Run("managed native Codex app-server", func(t *testing.T) {
		runManagedRuntimeCacheResume(t, agents.NewCodexAppServer(true), nil, true)
	})
}

func TestManagedRuntimeCacheResumeWithCLIArgs(t *testing.T) {
	t.Run("managed ACP", func(t *testing.T) {
		runManagedRuntimeCacheResume(t, agents.NewClaudeACP(), []string{"--dangerously-skip-permissions"}, true)
	})
	t.Run("managed native Codex app-server", func(t *testing.T) {
		runManagedRuntimeCacheResume(t, agents.NewCodexAppServer(true), []string{"--profile-flag", "ordered-value"}, true)
	})
}

func TestManagedRuntimeCacheStartWithCLIArgs(t *testing.T) {
	runManagedRuntimeCacheResume(t, agents.NewClaudeACP(), []string{"--dangerously-skip-permissions"}, false)
}

func runManagedRuntimeCacheResume(t *testing.T, agent agents.Agent, cliFlags []string, savedConversation bool) {
	t.Helper()
	manager, execution, mock, _ := newManagedRuntimeRetryFixture(t, false)
	mock.supportsManagedRuntimePreparation = true
	mock.supportsSessionLoad = true
	client, release := execution.AcquireAgentCtlClient()
	if _, err := client.Start(context.Background()); err != nil {
		release()
		t.Fatalf("start failed generation: %v", err)
	}
	failedGeneration := client.ProcessGeneration()
	release()

	runtimeSpec, ok := agents.BuiltinManagedNPMRuntime(agent.ID())
	if !ok {
		t.Fatalf("%s is not registered as a managed npm runtime", agent.ID())
	}
	packageSpec := runtimeSpec.PackageSpec("1.2.3")
	originalArgs := runtimeSpec.RuntimeCommand("1.2.3").Args()
	originalArgs = append(originalArgs, cliFlags...)
	execution.AgentID = agent.ID()
	execution.AgentCommand = originalArgs[0]
	execution.AgentArgs = append([]string(nil), originalArgs...)
	if savedConversation {
		execution.ACPSessionID = "saved-native-conversation"
	}
	execution.setRuntimeEnvironment(map[string]string{
		"NPM_CONFIG_REGISTRY":   "https://registry.example.test/",
		"RUNTIME_AUTH_SENTINEL": "preserve-this-value",
	})
	evidence := &agentctltypes.ManagedStartupEvidence{
		ProcessGeneration:     failedGeneration,
		ExitDisposition:       agentctltypes.ManagedStartupExitOrdinary,
		CollectionComplete:    true,
		NPMDiagnosticPresent:  true,
		NPMDiagnosticComplete: true,
		UnclassifiedNPMCode:   true,
		CacheEvidence: &agentctltypes.ManagedRuntimeCacheEvidence{
			Condition:         agentctltypes.ManagedRuntimeCacheRootManifestMissing,
			PackageSpec:       packageSpec,
			ProcessGeneration: failedGeneration,
		},
	}

	promptDispatched := make(chan struct{})
	execution.setInitialPromptDispatchCallbacks(nil, func() { close(promptDispatched) }, nil)
	attempted, err := manager.retryManagedRuntimeStartup(
		context.Background(), execution, managedACPInitializeEvidenceFailure("npm execution root is incomplete", evidence),
		agent, "resume and continue task", nil, nil,
	)
	if err != nil || !attempted {
		t.Fatalf("cache recovery result = (%v, %v), want one successful replacement", attempted, err)
	}
	select {
	case <-promptDispatched:
	case <-time.After(time.Second):
		t.Fatal("task prompt was not dispatched after cache recovery")
	}

	got := mock.preparationDirectives
	if len(got) != 1 {
		t.Fatalf("private preparation directives = %#v", got)
	}
	directive := got[0]
	if directive.RemainingBudgetSeconds < 1 || directive.RemainingBudgetSeconds > 90 {
		t.Fatalf("recovery budget = %d, want a bounded remaining deadline", directive.RemainingBudgetSeconds)
	}
	if directive.DeadlineUnixNano <= 0 {
		t.Fatal("recovery directive omitted the exact deadline")
	}
	directive.RemainingBudgetSeconds = 0
	directive.DeadlineUnixNano = 0
	if directive != (agentctltypes.ManagedRuntimePreparationDirective{
		Condition:               agentctltypes.ManagedRuntimeCacheRootManifestMissing,
		PackageSpec:             packageSpec,
		FailedProcessGeneration: failedGeneration,
	}) {
		t.Fatalf("private preparation directive = %#v", directive)
	}
	wantLoaded := []string(nil)
	if savedConversation {
		wantLoaded = []string{"saved-native-conversation"}
	}
	if got := mock.loadedSessionIDs; !slices.Equal(got, wantLoaded) {
		t.Fatalf("native conversations loaded = %#v, want %#v", got, wantLoaded)
	}
	if mock.getPromptCalls() != 1 {
		t.Fatalf("accepted prompt calls = %d, want exactly one", mock.getPromptCalls())
	}
	if savedConversation && execution.ACPSessionID != "saved-native-conversation" {
		t.Fatalf("recovery changed the saved conversation: %q", execution.ACPSessionID)
	}
	if !savedConversation && execution.ACPSessionID == "" {
		t.Fatal("fresh recovery did not create a conversation")
	}
	if !reflect.DeepEqual(execution.AgentArgs, originalArgs) {
		t.Fatalf("recovery changed command: args=%#v want %#v", execution.AgentArgs, originalArgs)
	}
	if mock.configuredEnv["NPM_CONFIG_REGISTRY"] != "https://registry.example.test/" || mock.configuredEnv["RUNTIME_AUTH_SENTINEL"] != "preserve-this-value" {
		t.Fatalf("replacement environment lost authorized values: %#v", mock.configuredEnv)
	}
	if got := mock.getHTTPActions(); !slices.Equal(got, []string{"start", "stop", "configure", "start"}) {
		t.Fatalf("startup actions = %#v", got)
	}
}
