package lifecycle

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
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
	initial := []string{"greywall", "--", "npx", "--yes", managedRuntimePreferOfflineArg, "--prefix", "~/.kandev/managed-npm-runtime", "@scope/managed-acp@1.2.3", "--acp", "--model", "fast"}

	got, packageSpec, ok := onlineManagedRuntimeArgs(initial, spec)
	if !ok {
		t.Fatal("expected managed runtime recovery command")
	}
	want := []string{"greywall", "--", "npx", "--yes", "--prefer-online", "--prefix", "~/.kandev/managed-npm-runtime", "@scope/managed-acp@1.2.3", "--acp", "--model", "fast"}
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
		{"npx", "--yes", managedRuntimePreferOfflineArg, "--prefix", "~/.kandev/managed-npm-runtime", "other-agent@1.2.3"},
		{"npx", "--yes", managedRuntimePreferOfflineArg, "managed-acp@1.2.3"},
	} {
		if _, _, ok := onlineManagedRuntimeArgs(args, spec); ok {
			t.Fatalf("command %#v should not be eligible for managed runtime recovery", args)
		}
	}
}

func TestOnlineManagedRuntimeArgsAcceptsAlreadyOnlineCommand(t *testing.T) {
	spec := agents.ManagedNPMRuntimeSpec{Package: "managed-acp"}
	args := []string{"npx", "--yes", "--prefer-online", "--prefix", "~/.kandev/managed-npm-runtime", "managed-acp@1.2.3", "--acp"}

	got, packageSpec, ok := onlineManagedRuntimeArgs(args, spec)
	if !ok {
		t.Fatal("already online managed runtime command should be eligible for early-exit recovery")
	}
	if !reflect.DeepEqual(got, args) || packageSpec != "managed-acp@1.2.3" {
		t.Fatalf("online recovery = (%#v, %q), want unchanged command and exact package", got, packageSpec)
	}
}

func TestOnlineManagedRuntimeArgsRejectsUnversionedPackage(t *testing.T) {
	spec := agents.ManagedNPMRuntimeSpec{Package: "managed-acp"}
	args := []string{"npx", "--yes", managedRuntimePreferOfflineArg, "--prefix", "~/.kandev/managed-npm-runtime", "managed-acp", "--acp"}

	if _, _, ok := onlineManagedRuntimeArgs(args, spec); ok {
		t.Fatal("unversioned managed runtime command should not be eligible")
	}
}

func TestManagedRuntimeSpecForArgsAcceptsOpenCodeV2(t *testing.T) {
	agent := agents.NewOpenCodeACP()
	args := []string{"npx", "--yes", "--prefer-offline", "--prefix", managedruntime.NPMProjectPrefix,
		"@opencode/cli@2.0.18", "acp", "--print-logs", "--log-level", "error"}
	spec, found := managedRuntimeSpecForArgs(agent, args)
	if !found || spec.Package != "@opencode/cli" {
		t.Fatalf("managedRuntimeSpecForArgs = (%+v, %v), want v2 managed package", spec, found)
	}
	got, packageSpec, ok := onlineManagedRuntimeArgs(args, spec)
	if !ok || packageSpec != "@opencode/cli@2.0.18" {
		t.Fatalf("onlineManagedRuntimeArgs = (%#v, %q, %v), want exact v2 retry", got, packageSpec, ok)
	}
}

func TestRetryManagedRuntimeStartupUsesAgentctlForExecutorLocalRuntimes(t *testing.T) {
	initialErr := managedACPInitializeFailure("ACP session initialization failed")
	for _, runtimeName := range []agentruntime.Runtime{agentruntime.RuntimeDocker, agentruntime.RuntimeSSH} {
		t.Run(runtimeName.String(), func(t *testing.T) {
			mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
			execution.RuntimeName = runtimeName

			attempted, err := mgr.retryManagedRuntimeStartup(
				context.Background(), execution, initialErr, agentConfig, "", nil, nil,
			)
			if err != nil {
				t.Fatalf("retryManagedRuntimeStartup: %v", err)
			}
			if !attempted {
				t.Fatal("expected one managed runtime retry")
			}
			if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop", "configure", "start"}) {
				t.Fatalf("HTTP actions = %#v", got)
			}
		})
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.5
func TestManagedStartupRecoverySinglePromptKeepsSessionIdentity(t *testing.T) {
	mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
	mgr.streamManager.callbacks.OnAgentEventWithGeneration = func(*AgentExecution, agentctl.AgentEvent, uint64) {}
	mgr.streamManager.callbacks.OnStreamDisconnectWithGeneration = func(*AgentExecution, error, uint64, uint64) {}
	mgr.sessionManager.SetInitialPromptFailureHandler(nil)
	promptDispatched := make(chan struct{})
	execution.setInitialPromptDispatchCallbacks(nil, func() { close(promptDispatched) }, nil)
	initialArgs := append([]string(nil), execution.AgentArgs...)
	originalID, originalTask, originalSession := execution.ID, execution.TaskID, execution.SessionID
	originalAgent, originalWorkspace, originalRuntime := execution.AgentID, execution.WorkspacePath, execution.RuntimeName

	attempted, err := mgr.retryManagedRuntimeStartup(
		context.Background(), execution, managedACPInitializeFailure("initial ACP initialize failed"),
		agentConfig, "do this task", nil, nil,
	)
	if err != nil || !attempted {
		t.Fatalf("recovery result = (%v, %v), want one successful replacement", attempted, err)
	}
	select {
	case <-promptDispatched:
	case <-time.After(time.Second):
		t.Fatal("initial prompt was not dispatched after recovery")
	}
	if got := mock.getPromptCalls(); got != 1 {
		t.Fatalf("prompt calls = %d, want exactly one", got)
	}
	if execution.ID != originalID || execution.TaskID != originalTask || execution.SessionID != originalSession ||
		execution.AgentID != originalAgent || execution.WorkspacePath != originalWorkspace || execution.RuntimeName != originalRuntime {
		t.Fatalf("recovery changed execution identity: %+v", execution)
	}
	wantArgs := append([]string(nil), initialArgs...)
	for i, arg := range wantArgs {
		if arg == managedRuntimePreferOfflineArg {
			wantArgs[i] = "--prefer-online"
		}
	}
	if !reflect.DeepEqual(execution.AgentArgs, wantArgs) {
		t.Fatalf("recovery args = %#v, want same launch settings with online npm preference", execution.AgentArgs)
	}
}

func TestManagedStartupRecoveryStopFailurePreventsReplacement(t *testing.T) {
	mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
	mock.failStop = true

	attempted, err := mgr.retryManagedRuntimeStartup(
		context.Background(), execution, managedACPInitializeFailure("ACP initialize failed"), agentConfig, "", nil, nil,
	)
	if attempted {
		t.Fatal("failed stop must not count as an attempted replacement")
	}
	var startupErr *routingerr.ManagedRuntimeStartupError
	if !errors.As(err, &startupErr) || startupErr.Code != routingerr.CodeManagedRuntimeStartup {
		t.Fatalf("error = %v, want structured managed startup cleanup failure", err)
	}
	if startupErr.Details != "reason=cleanup_failed attempts=1" {
		t.Fatalf("failure details = %q, want bounded cleanup reason", startupErr.Details)
	}
	if startupErr.Reason != "cleanup_failed" || startupErr.Attempts != 1 ||
		execution.StartupFailureReason != "cleanup_failed" || execution.StartupFailureAttempts != 1 {
		t.Fatalf("typed failure metadata = (%q, %d, %q, %d)", startupErr.Reason, startupErr.Attempts,
			execution.StartupFailureReason, execution.StartupFailureAttempts)
	}
	if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop"}) {
		t.Fatalf("HTTP actions = %#v, want stop only", got)
	}
}

func TestManagedRuntimeStartupFailureMetadataIsPublished(t *testing.T) {
	mgr, execution, _, _ := newManagedRuntimeRetryFixture(t, false)
	startup := managedRuntimeStartupFailureMetadata{reason: "early_exit", attempts: 2}
	err := mgr.publishManagedRuntimeStartupFailure(
		execution,
		"managed runtime startup failed",
		routingerr.CodeManagedRuntimeStartup,
		"reason=early_exit attempts=2",
		startup,
		errors.New("initialization failed"),
	)
	var structured *routingerr.ManagedRuntimeStartupError
	if !errors.As(err, &structured) {
		t.Fatalf("error = %v, want structured startup failure", err)
	}
	if structured.Reason != "early_exit" || structured.Attempts != 2 {
		t.Fatalf("typed startup error = (%q, %d), want early_exit and 2", structured.Reason, structured.Attempts)
	}
	payload := newAgentEventPayload(execution)
	if payload.StartupFailureReason != "early_exit" || payload.StartupFailureAttempts != 2 {
		t.Fatalf("event payload startup metadata = (%q, %d), want early_exit and 2",
			payload.StartupFailureReason, payload.StartupFailureAttempts)
	}
}

func TestManagedStartupRecoveryParentDeadlinePreventsReplacement(t *testing.T) {
	mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
	mgr.startupRecoveryDelay = func() time.Duration { return managedRuntimeStartupBackoff }
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	attempted, err := mgr.retryManagedRuntimeStartup(
		ctx, execution, managedACPInitializeFailure("ACP initialize failed"), agentConfig, "", nil, nil,
	)
	if attempted || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("recovery result = (%v, %v), want no replacement before parent deadline", attempted, err)
	}
	if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop"}) {
		t.Fatalf("HTTP actions = %#v, want confirmed stop only", got)
	}
}

func TestManagedStartupRecoveryCancellationDuringCleanupPreventsReplacement(t *testing.T) {
	mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
	stopStarted := make(chan struct{})
	mock.onStop = func(ctx context.Context) {
		close(stopStarted)
		<-ctx.Done()
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	type result struct {
		attempted bool
		err       error
	}
	done := make(chan result, 1)
	go func() {
		attempted, err := mgr.retryManagedRuntimeStartup(
			ctx, execution, managedACPInitializeFailure("ACP initialize failed"), agentConfig, "", nil, nil,
		)
		done <- result{attempted: attempted, err: err}
	}()
	select {
	case <-stopStarted:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not start")
	}
	cancel()
	select {
	case got := <-done:
		if got.attempted || !errors.Is(got.err, context.Canceled) {
			t.Fatalf("recovery result = (%v, %v), want cancellation before replacement", got.attempted, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("recovery did not stop after cleanup cancellation")
	}
	if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop"}) {
		t.Fatalf("HTTP actions = %#v, want cleanup only", got)
	}
}

func TestManagedRuntimeStartupRetryDelayIsBounded(t *testing.T) {
	for _, test := range []struct {
		jitter time.Duration
		want   time.Duration
	}{
		{jitter: -time.Second, want: 2 * time.Second},
		{jitter: 0, want: 2 * time.Second},
		{jitter: time.Second - time.Nanosecond, want: 3*time.Second - time.Nanosecond},
		{jitter: time.Second, want: 3*time.Second - time.Nanosecond},
	} {
		if got := managedRuntimeStartupDelay(test.jitter); got != test.want {
			t.Errorf("managedRuntimeStartupDelay(%s) = %s, want %s", test.jitter, got, test.want)
		}
	}
}

func TestManagedRuntimeStartupRecoveryContextIsBoundedByEarlierDeadline(t *testing.T) {
	ctx, cancel := managedRuntimeStartupRecoveryContext(context.Background())
	deadline, ok := ctx.Deadline()
	if !ok {
		cancel()
		t.Fatal("recovery context has no deadline")
	}
	remaining := time.Until(deadline)
	cancel()
	if remaining < managedRuntimeStartupRecoveryBudget-time.Second || remaining > managedRuntimeStartupRecoveryBudget {
		t.Fatalf("recovery budget = %s, want no more than %s", remaining, managedRuntimeStartupRecoveryBudget)
	}

	parent, cancelParent := context.WithTimeout(context.Background(), time.Second)
	defer cancelParent()
	bounded, cancelBounded := managedRuntimeStartupRecoveryContext(parent)
	defer cancelBounded()
	parentDeadline, _ := parent.Deadline()
	recoveryDeadline, _ := bounded.Deadline()
	if recoveryDeadline.After(parentDeadline) {
		t.Fatalf("recovery deadline %s exceeds parent deadline %s", recoveryDeadline, parentDeadline)
	}
}

func newManagedRuntimeRetryFixture(t *testing.T, failSessionNew bool) (*Manager, *AgentExecution, *restartMockAgentctlServer, agents.Agent) {
	t.Helper()
	mgr := newTestManager(t)
	mgr.startupRecoveryDelay = func() time.Duration { return 0 }
	mock := newRestartMockAgentctlServer(t, false, failSessionNew)
	client := createTestClient(t, mock.server.URL)
	var cleanupExecution *AgentExecution
	t.Cleanup(func() {
		mgr.closeStopCh()
		if mgr.streamManager != nil {
			mgr.streamManager.Wait()
		}
		if cleanupExecution != nil {
			deadline := time.NewTimer(time.Second)
			defer deadline.Stop()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			waiting := true
			for waiting {
				if cleanupExecution.promptMu.TryLock() {
					cleanupExecution.promptMu.Unlock()
					waiting = false
					continue
				}
				select {
				case <-deadline.C:
					t.Errorf("initial prompt did not stop during fixture cleanup")
					waiting = false
				case <-ticker.C:
				}
			}
		}
		client.Close()
		mock.closeUpdateStreams()
		for range mock.getUpdateStreamCount() {
			select {
			case <-mock.updateStreamClosed:
			case <-time.After(time.Second):
				t.Errorf("updates stream handler did not stop during fixture cleanup")
				return
			}
		}
		deadline := time.NewTimer(time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for client.HasAgentStream() {
			select {
			case <-deadline.C:
				t.Errorf("updates stream reader did not stop during fixture cleanup")
				return
			case <-ticker.C:
			}
		}
	})
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
		AgentArgs:     []string{"npx", "--yes", managedRuntimePreferOfflineArg, "--prefix", "~/.kandev/managed-npm-runtime", "opencode-ai@1.2.3", "acp", "--print-logs", "--log-level", "ERROR"},
		WorkspacePath: "/workspace",
		Status:        v1.AgentStatusStarting,
		agentctl:      client,
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}
	cleanupExecution = execution
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	execution.beginStartupAttempt()
	return mgr, execution, mock, agentConfig
}

func managedACPInitializeFailure(message string) error {
	return &SessionInitializationPhaseError{
		Phase: SessionInitializationPhaseACPInitialize,
		Cause: &agentctl.InitializeError{Message: message},
	}
}

func managedACPInitializeEvidenceFailure(message string, evidence *agentctltypes.ManagedStartupEvidence) error {
	return &SessionInitializationPhaseError{
		Phase: SessionInitializationPhaseACPInitialize,
		Cause: &agentctl.InitializeError{Message: message, StartupEvidence: evidence},
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.1
func TestManagedStartupRecoveryRetriesTransientNpmAndSilentExit(t *testing.T) {
	exitCode := 1
	cases := []struct {
		name             string
		evidence         *agentctltypes.ManagedStartupEvidence
		stderr           []string
		stderrConfigured bool
		wantOnline       bool
	}{
		{
			name: "transient npm error switches metadata online",
			evidence: &agentctltypes.ManagedStartupEvidence{
				ProcessGeneration: 1, ExitDisposition: agentctltypes.ManagedStartupExitOrdinary,
				ExitCode: &exitCode, NPMCode: "ECONNRESET", CollectionComplete: true,
				NPMDiagnosticPresent: true, NPMDiagnosticComplete: true,
			},
			stderr: []string{"npm error code ECONNRESET"}, wantOnline: true,
		},
		{
			name: "empty stderr ordinary exit preserves command",
			evidence: &agentctltypes.ManagedStartupEvidence{
				ProcessGeneration: 1, ExitDisposition: agentctltypes.ManagedStartupExitOrdinary,
				ExitCode: &exitCode, CollectionComplete: true, NPMDiagnosticComplete: true,
			},
			stderrConfigured: true,
		},
		{
			name: "strict ETARGET without npm code switches metadata online",
			evidence: &agentctltypes.ManagedStartupEvidence{
				ProcessGeneration: 1, ExitDisposition: agentctltypes.ManagedStartupExitOrdinary,
				ExitCode: &exitCode, CollectionComplete: true,
				NPMDiagnosticPresent: true, NPMDiagnosticComplete: true,
			},
			stderr: []string{
				"npm error code ETARGET",
				"npm error notarget No matching version found for opencode-ai@1.2.3.",
			},
			wantOnline: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
			client, release := execution.AcquireAgentCtlClient()
			if _, err := client.Start(context.Background()); err != nil {
				release()
				t.Fatalf("seed process generation: %v", err)
			}
			release()
			mock.mu.Lock()
			mock.httpActions = nil
			mock.stderrLines = append([]string(nil), tt.stderr...)
			mock.stderrConfigured = tt.stderrConfigured
			mock.mu.Unlock()
			initialArgs := append([]string(nil), execution.AgentArgs...)

			attempted, err := mgr.retryManagedRuntimeStartup(
				context.Background(), execution,
				managedACPInitializeEvidenceFailure("agent process exited before ACP initialize", tt.evidence),
				agentConfig, "", nil, nil,
			)
			if err != nil || !attempted {
				t.Fatalf("recovery result = (%v, %v), want one successful replacement", attempted, err)
			}
			if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop", "configure", "start"}) {
				t.Fatalf("HTTP actions = %#v, want stop then one replacement", got)
			}
			if got := mock.getManagedRuntimeRepairSpecs(); len(got) != 0 {
				t.Fatalf("recovery called cache repair: %#v", got)
			}
			wantArgs := append([]string(nil), initialArgs...)
			if tt.wantOnline {
				for i, arg := range wantArgs {
					if arg == managedRuntimePreferOfflineArg {
						wantArgs[i] = "--prefer-online"
					}
				}
			}
			if !reflect.DeepEqual(execution.AgentArgs, wantArgs) {
				t.Fatalf("retry args = %#v, want %#v", execution.AgentArgs, wantArgs)
			}
		})
	}
}

func TestManagedRuntimeReleaseAgePolicySkipsCacheRepair(t *testing.T) {
	initialErr := managedACPInitializeFailure("ACP session initialization failed")
	mgr, execution, mock, _ := newManagedRuntimeRetryFixture(t, false)
	agentConfig := agents.NewClaudeACP()
	execution.AgentID = agentConfig.ID()
	execution.AgentArgs = []string{
		"npx", "--yes", managedRuntimePreferOfflineArg, "--prefix", "~/.kandev/managed-npm-runtime",
		"@agentclientprotocol/claude-agent-acp@0.81.0", "acp",
	}
	mock.stderrLines = []string{
		"npm error code ETARGET",
		"npm error notarget No matching version found for @agentclientprotocol/claude-agent-acp@0.81.0 with a date before 9/22/2026, 12:28:47 PM.",
	}

	attempted, err := mgr.retryManagedRuntimeStartup(
		context.Background(), execution, initialErr, agentConfig, "", nil, nil,
	)
	if !attempted {
		t.Fatal("policy failure should be handled as a managed runtime failure")
	}
	var startupErr *routingerr.ManagedRuntimeStartupError
	if !errors.As(err, &startupErr) {
		t.Fatalf("error = %v, want structured policy startup failure", err)
	}
	if got, want := startupErr.Code, routingerr.Code("managed_runtime_npm_policy"); got != want {
		t.Fatalf("failure code = %q, want %q", got, want)
	}
	if !strings.Contains(startupErr.Details, "<release-date>") {
		t.Fatalf("failure details = %q, want sanitized release-date marker", startupErr.Details)
	}
	if !strings.Contains(startupErr.Details, "reason=npm_release_policy") || !strings.Contains(startupErr.Details, "attempts=1") {
		t.Fatalf("failure details = %q, want stable reason and actual attempt count", startupErr.Details)
	}
	if got := mock.getHTTPActions(); len(got) != 0 {
		t.Fatalf("policy failure triggered repair or restart actions: %#v", got)
	}
	if execution.FailureCode != string(startupErr.Code) {
		t.Fatalf("persisted failure code = %q, want %q", execution.FailureCode, startupErr.Code)
	}
}

func TestManagedRuntimeReleaseAgePolicyIsClassifiedWithoutRepairSupport(t *testing.T) {
	initialErr := managedACPInitializeFailure("ACP session initialization failed")
	for _, runtime := range []agentruntime.Runtime{agentruntime.RuntimeKubernetes, agentruntime.RuntimeSprites} {
		t.Run(string(runtime), func(t *testing.T) {
			mgr, execution, mock, _ := newManagedRuntimeRetryFixture(t, false)
			agentConfig := agents.NewClaudeACP()
			execution.AgentID = agentConfig.ID()
			execution.RuntimeName = runtime
			execution.AgentArgs = []string{
				"npx", "--yes", managedRuntimePreferOfflineArg, "--prefix", "~/.kandev/managed-npm-runtime",
				"@agentclientprotocol/claude-agent-acp@0.81.0", "acp",
			}
			mock.stderrLines = []string{
				"npm error code ETARGET",
				"npm error notarget No matching version found for @agentclientprotocol/claude-agent-acp@0.81.0 with a date before 9/22/2026, 12:28:47 PM.",
			}

			attempted, err := mgr.retryManagedRuntimeStartup(
				context.Background(), execution, initialErr, agentConfig, "", nil, nil,
			)
			if !attempted {
				t.Fatal("policy failure should be classified without cache-repair support")
			}
			var startupErr *routingerr.ManagedRuntimeStartupError
			if !errors.As(err, &startupErr) || startupErr.Code != routingerr.CodeManagedRuntimeNpmPolicy {
				t.Fatalf("error = %v, want structured npm policy startup failure", err)
			}
			if actions := mock.getHTTPActions(); len(actions) != 0 {
				t.Fatalf("unsupported runtime policy failure called repair endpoint: %#v", actions)
			}

			genericMgr, genericExecution, genericMock, _ := newManagedRuntimeRetryFixture(t, false)
			genericExecution.AgentID = agentConfig.ID()
			genericExecution.RuntimeName = runtime
			genericExecution.AgentArgs = append([]string(nil), execution.AgentArgs...)
			genericMock.stderrLines = []string{
				"npm error code ETARGET",
				"npm error notarget No matching version found for @agentclientprotocol/claude-agent-acp@0.81.0.",
			}
			attempted, err = genericMgr.retryManagedRuntimeStartup(
				context.Background(), genericExecution, initialErr, agentConfig, "", nil, nil,
			)
			if attempted || !errors.Is(err, initialErr) {
				t.Fatalf("ordinary resolution failure result = (%v, %v), want original generic error", attempted, err)
			}
			if actions := genericMock.getHTTPActions(); len(actions) != 0 {
				t.Fatalf("unsupported runtime ordinary failure called repair endpoint: %#v", actions)
			}
		})
	}
}

func TestRetryManagedRuntimeStartupLifecycle(t *testing.T) {
	initialErr := managedACPInitializeFailure("ACP session initialization failed")

	t.Run("successful recovery uses one online replacement", func(t *testing.T) {
		mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)

		attempted, err := mgr.retryManagedRuntimeStartup(
			context.Background(), execution, initialErr, agentConfig, "", nil, nil,
		)
		if err != nil {
			t.Fatalf("retryManagedRuntimeStartup: %v", err)
		}
		if !attempted {
			t.Fatal("expected one managed runtime retry")
		}
		if got := mock.getManagedRuntimeRepairSpecs(); len(got) != 0 {
			t.Fatalf("automatic startup retry called cache repair: %#v", got)
		}
		if got := execution.AgentArgs; !slices.Contains(got, "--prefer-online") || slices.Contains(got, managedRuntimePreferOfflineArg) {
			t.Fatalf("replacement args = %#v, want online preference only", got)
		}
		if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop", "configure", "start"}) {
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
			context.Background(), execution, initialErr, agentConfig, "", nil, nil,
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
			context.Background(), execution, initialErr, agentConfig, "", nil, nil,
		)
		if !attempted {
			t.Fatal("expected one managed runtime retry")
		}
		var startupErr *routingerr.ManagedRuntimeStartupError
		if !errors.As(err, &startupErr) {
			t.Fatalf("retry error = %v, want structured startup error", err)
		}
		if startupErr.Code != routingerr.CodeAgentRuntime {
			t.Fatalf("retry code = %q", startupErr.Code)
		}
		if execution.FailureCode != string(routingerr.CodeAgentRuntime) {
			t.Fatalf("execution failure code = %q", execution.FailureCode)
		}
		if got := mock.getManagedRuntimeRepairSpecs(); len(got) != 0 {
			t.Fatalf("automatic retry called cache repair: %#v", got)
		}
		if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop", "configure", "start"}) {
			t.Fatalf("HTTP actions = %#v", got)
		}
	})

	t.Run("stop failure is terminal and does not start a replacement", func(t *testing.T) {
		mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
		mock.failStop = true

		attempted, err := mgr.retryManagedRuntimeStartup(
			context.Background(), execution, initialErr, agentConfig, "", nil, nil,
		)
		if attempted {
			t.Fatal("stop failure must not count as a started retry")
		}
		var startupErr *routingerr.ManagedRuntimeStartupError
		if !errors.As(err, &startupErr) {
			t.Fatalf("stop error = %v, want structured startup error", err)
		}
		if startupErr.Code != routingerr.CodeManagedRuntimeStartup {
			t.Fatalf("stop code = %q, want managed startup code", startupErr.Code)
		}
		if startupErr.Details != "reason=cleanup_failed attempts=1" {
			t.Fatalf("stop details = %q, want bounded cleanup details", startupErr.Details)
		}
		if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop"}) {
			t.Fatalf("HTTP actions = %#v, want stop only", got)
		}
		if execution.FailureCode != string(routingerr.CodeManagedRuntimeStartup) {
			t.Fatalf("execution failure code = %q", execution.FailureCode)
		}
	})

	t.Run("cancellation wins over recovery", func(t *testing.T) {
		mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
		ctx, cancel := context.WithCancel(context.Background())
		delaySelected := make(chan struct{})
		mgr.startupRecoveryDelay = func() time.Duration {
			close(delaySelected)
			return time.Hour
		}
		result := make(chan struct {
			attempted bool
			err       error
		}, 1)
		go func() {
			attempted, err := mgr.retryManagedRuntimeStartup(ctx, execution, initialErr, agentConfig, "", nil, nil)
			result <- struct {
				attempted bool
				err       error
			}{attempted: attempted, err: err}
		}()
		<-delaySelected
		cancel()
		outcome := <-result
		if !errors.Is(outcome.err, context.Canceled) {
			t.Fatalf("cancellation error = %v", outcome.err)
		}
		if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop"}) {
			t.Fatalf("HTTP actions = %#v, want confirmed stop only", got)
		}
		if outcome.attempted {
			t.Fatal("cancellation before replacement start must not report a retry")
		}
	})

	t.Run("remote and native launches are excluded", func(t *testing.T) {
		t.Run("remote docker", func(t *testing.T) {
			mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
			execution.RuntimeName = agentruntime.RuntimeRemoteDocker
			attempted, err := mgr.retryManagedRuntimeStartup(context.Background(), execution, initialErr, agentConfig, "", nil, nil)
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
			attempted, err := mgr.retryManagedRuntimeStartup(context.Background(), execution, initialErr, agentConfig, "", nil, nil)
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
			attempted, err := mgr.retryManagedRuntimeStartup(context.Background(), execution, initialErr, agentConfig, "", nil, nil)
			if attempted || !errors.Is(err, initialErr) {
				t.Fatalf("native result = (%v, %v), want no retry and original error", attempted, err)
			}
			if got := mock.getHTTPActions(); len(got) != 0 {
				t.Fatalf("native HTTP actions = %#v", got)
			}
		})
	})
}
