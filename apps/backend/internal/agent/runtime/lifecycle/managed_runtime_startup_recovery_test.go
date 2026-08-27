package lifecycle

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentruntime"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestStartupAttemptGenerationRejectsStaleEventsAfterRecovery(t *testing.T) {
	execution := &AgentExecution{}
	first := execution.beginStartupAttempt()
	if !execution.acceptsStartupAttempt(first) {
		t.Fatal("initial startup generation should be current")
	}

	second, ok := execution.beginStartupRecovery()
	if !ok {
		t.Fatal("expected startup recovery generation")
	}
	if second == first || execution.acceptsStartupAttempt(first) {
		t.Fatal("first child generation remained current after recovery")
	}
	if !execution.acceptsStartupAttempt(second) {
		t.Fatal("retry startup generation should be current")
	}
	if _, ok := execution.beginStartupRecovery(); ok {
		t.Fatal("startup recovery should be attempted at most once")
	}
	execution.finishStartupRecovery()
}

func TestOnlineManagedRuntimeArgsPreserveTrustedLaunchIdentity(t *testing.T) {
	spec := agents.ManagedNPMRuntimeSpec{
		Package: "@scope/managed-acp",
		ACPArgs: []string{"--acp", "--model", "fast"},
	}
	initial := []string{"greywall", "--", "npx", "--yes", "--prefer-offline", "@scope/managed-acp@1.2.3", "--acp", "--model", "fast"}

	got, packageSpec, ok := onlineManagedRuntimeArgs(initial, spec)
	if !ok {
		t.Fatal("expected managed runtime recovery command")
	}
	want := []string{"greywall", "--", "npx", "--yes", "--prefer-online", "@scope/managed-acp@1.2.3", "--acp", "--model", "fast"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("online args = %#v, want %#v", got, want)
	}
	if packageSpec != "@scope/managed-acp@1.2.3" {
		t.Fatalf("package spec = %q, want exact selected spec", packageSpec)
	}
}

func TestOnlineManagedRuntimeArgsRejectsNonManagedCommands(t *testing.T) {
	spec := agents.ManagedNPMRuntimeSpec{Package: "managed-acp"}
	for _, args := range [][]string{
		{"native-agent", "--acp"},
		{"npx", "--yes", "--prefer-offline", "other-agent@1.2.3"},
		{"npx", "--yes", "--prefer-online", "managed-acp@1.2.3"},
	} {
		if _, _, ok := onlineManagedRuntimeArgs(args, spec); ok {
			t.Fatalf("command %#v should not be eligible for managed runtime recovery", args)
		}
	}
}

func TestOnlineManagedRuntimeArgsRejectsUnversionedPackage(t *testing.T) {
	spec := agents.ManagedNPMRuntimeSpec{Package: "managed-acp"}
	args := []string{"npx", "--yes", "--prefer-offline", "managed-acp", "--acp"}

	if _, _, ok := onlineManagedRuntimeArgs(args, spec); ok {
		t.Fatal("unversioned managed runtime command should not be eligible")
	}
}

func TestRetryManagedRuntimeStartupUsesAgentctlForExecutorLocalRuntimes(t *testing.T) {
	initialErr := errors.New("ACP session initialization failed")
	for _, runtimeName := range []agentruntime.Runtime{agentruntime.RuntimeDocker, agentruntime.RuntimeSSH} {
		t.Run(runtimeName.String(), func(t *testing.T) {
			mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
			execution.RuntimeName = runtimeName

			attempted, err := mgr.retryManagedRuntimeStartup(
				context.Background(), execution, initialErr, agentConfig, "", "", nil, nil,
			)
			if err != nil {
				t.Fatalf("retryManagedRuntimeStartup: %v", err)
			}
			if !attempted {
				t.Fatal("expected one managed runtime retry")
			}
			if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop", "cache-repair", "configure", "start"}) {
				t.Fatalf("HTTP actions = %#v", got)
			}
		})
	}
}

func TestManagedRuntimeRetryFailureClassificationPreservesSecondResult(t *testing.T) {
	retryErr := errors.New("failed to initialize ACP: secret=abcdefghijklmnopqrstuvwxyz123456")
	second := &routingerr.Error{
		Code:       routingerr.CodeAuthRequired,
		RawExcerpt: "authentication required",
	}

	code, details := managedRuntimeRetryFailureClassification(
		routingerr.CodeManagedRuntimeNpmResolution,
		"initial npm details",
		retryErr,
		true,
		second,
	)
	if code != routingerr.CodeAuthRequired || details != "authentication required" {
		t.Fatalf("retry classification = (%q, %q), want auth_required and sanitized second excerpt", code, details)
	}

	code, details = managedRuntimeRetryFailureClassification(
		routingerr.CodeManagedRuntimeNpmResolution,
		"initial npm details",
		retryErr,
		false,
		nil,
	)
	if code != routingerr.CodeAgentRuntime {
		t.Fatalf("configure failure code = %q, want %q", code, routingerr.CodeAgentRuntime)
	}
	if details == retryErr.Error() {
		t.Fatal("configure failure details must be sanitized")
	}
}

func newManagedRuntimeRetryFixture(t *testing.T, failSessionNew bool) (*Manager, *AgentExecution, *restartMockAgentctlServer, agents.Agent) {
	t.Helper()
	mgr := newTestManager(t)
	mock := newRestartMockAgentctlServer(t, false, failSessionNew)
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	if err := client.StreamUpdates(context.Background(), func(agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("connect initial agent stream: %v", err)
	}

	agentConfig := agents.NewOpenCodeACP()
	execution := &AgentExecution{
		ID:            "managed-runtime-retry-execution",
		TaskID:        "task-1",
		SessionID:     "session-1",
		AgentID:       agentConfig.ID(),
		RuntimeName:   agentruntime.RuntimeStandalone,
		AgentCommand:  "npx",
		AgentArgs:     []string{"npx", "--yes", "--prefer-offline", "opencode-ai@1.2.3", "acp", "--print-logs", "--log-level", "ERROR"},
		WorkspacePath: "/workspace",
		Status:        v1.AgentStatusStarting,
		agentctl:      client,
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	execution.beginStartupAttempt()
	return mgr, execution, mock, agentConfig
}

func TestRetryManagedRuntimeStartupLifecycle(t *testing.T) {
	initialErr := errors.New("ACP session initialization failed")

	t.Run("successful recovery uses one online replacement", func(t *testing.T) {
		mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)

		attempted, err := mgr.retryManagedRuntimeStartup(
			context.Background(), execution, initialErr, agentConfig, "", "", nil, nil,
		)
		if err != nil {
			t.Fatalf("retryManagedRuntimeStartup: %v", err)
		}
		if !attempted {
			t.Fatal("expected one managed runtime retry")
		}
		if got := mock.getManagedRuntimeRepairSpecs(); !slices.Equal(got, []string{"opencode-ai@1.2.3"}) {
			t.Fatalf("repair package specs = %#v", got)
		}
		if got := execution.AgentArgs; !slices.Contains(got, "--prefer-online") || slices.Contains(got, "--prefer-offline") {
			t.Fatalf("replacement args = %#v, want online preference only", got)
		}
		if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop", "cache-repair", "configure", "start"}) {
			t.Fatalf("HTTP actions = %#v", got)
		}
		if execution.FailureCode != "" || execution.Status == v1.AgentStatusFailed {
			t.Fatalf("successful recovery left failure state: code=%q status=%q", execution.FailureCode, execution.Status)
		}
	})

	t.Run("transitive ETARGET does not trigger top-level recovery", func(t *testing.T) {
		mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
		mock.stderrLines = []string{
			"npm error code ETARGET",
			"npm error notarget No matching version found for transitive-dependency@9.9.9",
		}

		attempted, err := mgr.retryManagedRuntimeStartup(
			context.Background(), execution, initialErr, agentConfig, "", "", nil, nil,
		)
		if attempted || !errors.Is(err, initialErr) {
			t.Fatalf("mismatched ETARGET result = (%v, %v), want no retry and original error", attempted, err)
		}
		if got := mock.getHTTPActions(); len(got) != 0 {
			t.Fatalf("mismatched ETARGET HTTP actions = %#v, want none", got)
		}
	})

	t.Run("repeated initialization failure is terminal after one retry", func(t *testing.T) {
		mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, true)

		attempted, err := mgr.retryManagedRuntimeStartup(
			context.Background(), execution, initialErr, agentConfig, "", "", nil, nil,
		)
		if !attempted {
			t.Fatal("expected one managed runtime retry")
		}
		var startupErr *routingerr.ManagedRuntimeStartupError
		if !errors.As(err, &startupErr) {
			t.Fatalf("retry error = %v, want structured startup error", err)
		}
		if startupErr.Code != routingerr.CodeManagedRuntimeNpmResolution {
			t.Fatalf("retry code = %q", startupErr.Code)
		}
		if execution.FailureCode != string(routingerr.CodeManagedRuntimeNpmResolution) {
			t.Fatalf("execution failure code = %q", execution.FailureCode)
		}
		if got := mock.getManagedRuntimeRepairSpecs(); len(got) != 1 {
			t.Fatalf("repair package specs = %#v, want one call", got)
		}
		if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop", "cache-repair", "configure", "start"}) {
			t.Fatalf("HTTP actions = %#v", got)
		}
	})

	t.Run("repair failure is generic and does not start a replacement", func(t *testing.T) {
		mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
		mock.failCacheRepair = true

		attempted, err := mgr.retryManagedRuntimeStartup(
			context.Background(), execution, initialErr, agentConfig, "", "", nil, nil,
		)
		if attempted {
			t.Fatal("cache repair failure must not count as a started retry")
		}
		var startupErr *routingerr.ManagedRuntimeStartupError
		if !errors.As(err, &startupErr) {
			t.Fatalf("repair error = %v, want structured startup error", err)
		}
		if startupErr.Code != routingerr.CodeAgentRuntime {
			t.Fatalf("repair code = %q, want generic runtime code", startupErr.Code)
		}
		if strings.Contains(startupErr.Details, "/home/alice/") {
			t.Fatalf("repair details exposed a host path: %q", startupErr.Details)
		}
		if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop", "cache-repair"}) {
			t.Fatalf("HTTP actions = %#v, want no configure/start", got)
		}
		if execution.FailureCode != string(routingerr.CodeAgentRuntime) {
			t.Fatalf("execution failure code = %q", execution.FailureCode)
		}
	})

	t.Run("cancellation wins over recovery", func(t *testing.T) {
		mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
		ctx, cancel := context.WithCancel(context.Background())
		mock.onCacheRepair = cancel

		attempted, err := mgr.retryManagedRuntimeStartup(ctx, execution, initialErr, agentConfig, "", "", nil, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v", err)
		}
		if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop", "cache-repair"}) {
			t.Fatalf("HTTP actions = %#v, want no configure/start", got)
		}
		if attempted {
			t.Fatal("cancellation before replacement start must not report a retry")
		}
	})

	t.Run("remote and native launches are excluded", func(t *testing.T) {
		t.Run("remote docker", func(t *testing.T) {
			mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
			execution.RuntimeName = agentruntime.RuntimeRemoteDocker
			attempted, err := mgr.retryManagedRuntimeStartup(context.Background(), execution, initialErr, agentConfig, "", "", nil, nil)
			if attempted || !errors.Is(err, initialErr) {
				t.Fatalf("remote docker result = (%v, %v), want no retry and original error", attempted, err)
			}
			if got := mock.getHTTPActions(); len(got) != 0 {
				t.Fatalf("remote docker HTTP actions = %#v", got)
			}
		})

		t.Run("sprites", func(t *testing.T) {
			mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
			execution.RuntimeName = agentruntime.RuntimeSprites
			attempted, err := mgr.retryManagedRuntimeStartup(context.Background(), execution, initialErr, agentConfig, "", "", nil, nil)
			if attempted || !errors.Is(err, initialErr) {
				t.Fatalf("sprites result = (%v, %v), want no retry and original error", attempted, err)
			}
			if got := mock.getHTTPActions(); len(got) != 0 {
				t.Fatalf("sprites HTTP actions = %#v", got)
			}
		})

		t.Run("native command", func(t *testing.T) {
			mgr, execution, mock, _ := newManagedRuntimeRetryFixture(t, false)
			agentConfig := agents.NewCopilotACP()
			execution.AgentID = agentConfig.ID()
			execution.AgentCommand = "copilot"
			execution.AgentArgs = []string{"copilot", "--acp"}
			attempted, err := mgr.retryManagedRuntimeStartup(context.Background(), execution, initialErr, agentConfig, "", "", nil, nil)
			if attempted || !errors.Is(err, initialErr) {
				t.Fatalf("native result = (%v, %v), want no retry and original error", attempted, err)
			}
			if got := mock.getHTTPActions(); len(got) != 0 {
				t.Fatalf("native HTTP actions = %#v", got)
			}
		})
	})
}
