package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestCursorMCPDiagnosticSurvivesPreparation(t *testing.T) {
	manager, eventBus := newPrepareEventsTestManager(t, "cursor-diagnostic-profile")
	recorder := manager.newPreparationAttemptRecorder("task-diagnostic", "session-diagnostic")
	code := 0
	diagnostic := &mcpconfig.NativeMCPDiagnostic{
		Operation: mcpconfig.NativeMCPDiagnosticOperation("enable"),
		Stage:     mcpconfig.NativeMCPDiagnosticStage("wait"),
		Kind:      mcpconfig.NativeMCPDiagnosticKind("output_wait_timeout"),
		Message:   "exec: WaitDelay expired before I/O complete",
		ExitCode:  &code,
	}
	index := appendCursorMCPProgress(recorder, "plugin-example", PrepareStepKindAgentMCPApproval, PrepareStepRunning, "", nil, nil)
	updateCursorMCPProgress(recorder, index, "plugin-example", PrepareStepKindAgentMCPApproval, PrepareStepFailed, "connection_failed", timeForMCPDiagnosticTest(), timeForMCPDiagnosticTest(), diagnostic)

	steps := recorder.Steps()
	require.Equal(t, diagnostic, steps[index].Diagnostic)
	progress := prepareProgressPayloads(eventBus)
	require.Len(t, progress, 2)
	require.Equal(t, diagnostic, progress[1].Diagnostic)

	saved := SerializePrepareResult(&EnvPrepareResult{Success: true, Steps: steps})
	hydrated := persistedPrepareSteps(map[string]interface{}{"prepare_result": saved})
	require.Len(t, hydrated, 1)
	require.Equal(t, diagnostic, hydrated[0].Diagnostic)

	manager.publishLaunchPrepareCompleted(&LaunchRequest{TaskID: "task-diagnostic", SessionID: "session-diagnostic"}, nil, recorder, "/workspace", true, nil)
	completed := prepareCompletedPayloads(eventBus)
	require.Len(t, completed, 1)
	require.Equal(t, diagnostic, completed[0].Steps[0].Diagnostic)

	retry := cursorMCPRetryReadiness("plugin-example", mcpconfig.NativeMCPReadiness{
		Status: mcpconfig.NativeMCPStatusConnectionFailed, ReasonCode: "connection_failed", Diagnostic: diagnostic,
	})
	require.Equal(t, diagnostic, retry.Diagnostic)
	encoded, err := json.Marshal(retry)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"mcp_diagnostic"`)
}

func TestCursorMCPFenceDiagnosticRetainsOnlyEndedContextCauses(t *testing.T) {
	diagnostic := &mcpconfig.NativeMCPDiagnostic{
		Operation: "enable", Stage: "wait", Kind: "output_wait_timeout", Message: "command wait expired",
	}
	activeContext, cancelActive := context.WithCancel(context.Background())
	defer cancelActive()
	require.Nil(t, cursorMCPFenceDiagnostic(activeContext, diagnostic))

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	require.Equal(t, diagnostic, cursorMCPFenceDiagnostic(canceledContext, diagnostic))

	deadlineContext, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelDeadline()
	require.ErrorIs(t, deadlineContext.Err(), context.DeadlineExceeded)
	require.Equal(t, diagnostic, cursorMCPFenceDiagnostic(deadlineContext, diagnostic))
}

func TestCursorMCPRecoveryClearsOnlyRecoveredDiagnostic(t *testing.T) {
	recorder := newPrepareProgressRecorder(nil)
	first := &mcpconfig.NativeMCPDiagnostic{Operation: "enable", Stage: "wait", Kind: "wait_failed", Message: "first failure"}
	second := &mcpconfig.NativeMCPDiagnostic{Operation: "list_tools", Stage: "wait", Kind: "wait_failed", Message: "second failure"}
	firstIndex := appendCursorMCPProgress(recorder, "server-a", PrepareStepKindAgentMCPApproval, PrepareStepFailed, "connection_failed", nil, nil)
	secondIndex := appendCursorMCPProgress(recorder, "server-b", PrepareStepKindAgentMCPVerification, PrepareStepFailed, "connection_failed", nil, nil)
	updateCursorMCPProgress(recorder, firstIndex, "server-a", PrepareStepKindAgentMCPApproval, PrepareStepFailed, "connection_failed", timeForMCPDiagnosticTest(), timeForMCPDiagnosticTest(), first)
	updateCursorMCPProgress(recorder, secondIndex, "server-b", PrepareStepKindAgentMCPVerification, PrepareStepFailed, "connection_failed", timeForMCPDiagnosticTest(), timeForMCPDiagnosticTest(), second)
	updateCursorMCPProgress(recorder, firstIndex, "server-a", PrepareStepKindAgentMCPApproval, PrepareStepCompleted, "", timeForMCPDiagnosticTest(), timeForMCPDiagnosticTest())

	steps := recorder.Steps()
	require.Nil(t, steps[firstIndex].Diagnostic)
	require.Equal(t, second, steps[secondIndex].Diagnostic)
}

func TestRetryCursorMCPDiagnosticStaysOnFailedApprovalStep(t *testing.T) {
	manager, execution, profile := newCursorMCPRecoveryFixture(t)
	prepareCursorMCPRecoveryWorkspace(t, manager, execution, profile)
	code := 0
	diagnostic := &mcpconfig.NativeMCPDiagnostic{
		Operation: "enable", Stage: "wait", Kind: "output_wait_timeout",
		Message: "exec: WaitDelay expired", ExitCode: &code,
	}
	runner := &cursorMCPFixedDiagnosticRunner{
		result: mcpconfig.NativeMCPCommandResult{ExitCode: 0, ExitCodeObserved: true, Diagnostic: diagnostic},
		err:    exec.ErrWaitDelay,
	}
	core, observed := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	manager.logger = log
	manager.SetCursorNativeMCPCommandRunner(runner)

	result, err := manager.RetryCursorMCPConnection(context.Background(), execution.SessionID, "plugin-harness-figma")

	require.NoError(t, err)
	require.Equal(t, diagnostic, result.Diagnostic)
	var approval, verification *PrepareStep
	for index := range execution.PrepareResult.Steps {
		step := &execution.PrepareResult.Steps[index]
		if step.MCPServerID != "plugin-harness-figma" {
			continue
		}
		switch step.Kind {
		case PrepareStepKindAgentMCPApproval:
			approval = step
		case PrepareStepKindAgentMCPVerification:
			verification = step
		}
	}
	require.NotNil(t, approval)
	require.Equal(t, PrepareStepFailed, approval.Status)
	require.Equal(t, diagnostic, approval.Diagnostic)
	require.NotNil(t, verification)
	require.Equal(t, PrepareStepSkipped, verification.Status)
	require.Nil(t, verification.Diagnostic)

	entries := observed.FilterMessage("native MCP preparation failed").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, execution.TaskID, fields["task_id"])
	require.Equal(t, execution.SessionID, fields["session_id"])
	require.NotEmpty(t, fields["preparation_id"])
	require.Equal(t, "plugin-harness-figma", fields["server_id"])
}

func TestCursorMCPPreparationFencesRetainContextEndedDiagnostic(t *testing.T) {
	for _, operation := range []string{"enable", "list-tools"} {
		t.Run(operation, func(t *testing.T) {
			manager, execution, profile := newCursorMCPRecoveryFixture(t)
			writeCursorMCPRecoveryPlugin(t, filepath.Join(os.Getenv("HOME"), ".cursor"))
			manager.cursorInventoryLoader = func(context.Context) (mcpconfig.CursorNativeInventory, error) {
				return mcpconfig.CursorNativeInventory{}, nil
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			diagnosticOperation := mcpconfig.NativeMCPDiagnosticOperation("enable")
			if operation == "list-tools" {
				diagnosticOperation = "list_tools"
			}
			diagnostic := &mcpconfig.NativeMCPDiagnostic{
				Operation: diagnosticOperation, Stage: "wait", Kind: "output_wait_timeout",
				Message: "exec: WaitDelay expired",
			}
			manager.SetCursorNativeMCPCommandRunner(&cursorMCPContextCancelDiagnosticRunner{
				cancelOperation: operation, cancel: cancel, diagnostic: diagnostic,
			})
			recorder := manager.newPreparationAttemptRecorder(execution.TaskID, execution.SessionID)

			err := manager.reconcileAndMaterializeCursorProjectMCPWithPreparation(
				ctx, execution, agents.NewCursorACP(), profile, string(executor.NameLocal),
				agents.NewCursorACP().Runtime().ProjectMCPStrategy, recorder,
			)

			require.NoError(t, err)
			steps := recorder.Steps()
			var approval, verification *PrepareStep
			for index := range steps {
				step := &steps[index]
				if step.MCPServerID != "plugin-harness-figma" {
					continue
				}
				switch step.Kind {
				case PrepareStepKindAgentMCPApproval:
					approval = step
				case PrepareStepKindAgentMCPVerification:
					verification = step
				}
			}
			require.NotNil(t, approval)
			require.NotNil(t, verification)
			if operation == "enable" {
				require.Equal(t, PrepareStepFailed, approval.Status)
				require.Equal(t, "canceled", approval.FailureCode)
				require.Equal(t, PrepareStepSkipped, verification.Status)
				require.Equal(t, diagnostic, approval.Diagnostic)
				require.Nil(t, verification.Diagnostic)
			} else {
				require.Equal(t, PrepareStepCompleted, approval.Status)
				require.Equal(t, PrepareStepFailed, verification.Status)
				require.Equal(t, "canceled", verification.FailureCode)
				require.Nil(t, approval.Diagnostic)
				require.Equal(t, diagnostic, verification.Diagnostic)
			}
		})
	}
}

func TestCursorMCPDiagnosticLogIsStructuredAndRedacted(t *testing.T) {
	core, observed := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	manager := &Manager{logger: log}
	execution := &AgentExecution{TaskID: "task-log", SessionID: "session-log"}
	recorder := &prepareProgressRecorder{preparationID: "preparation-log"}
	diagnostic := &mcpconfig.NativeMCPDiagnostic{
		Operation: "enable", Stage: "wait", Kind: "wait_failed",
		Message: "helper failed token=private-token at https://private.example/account/path",
	}

	manager.logCursorMCPDiagnostic(execution, recorder, "server-log", diagnostic)

	entries := observed.FilterMessage("native MCP preparation failed").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "task-log", fields["task_id"])
	require.Equal(t, "session-log", fields["session_id"])
	require.Equal(t, "preparation-log", fields["preparation_id"])
	require.Equal(t, "server-log", fields["server_id"])
	require.Equal(t, "enable", fields["operation"])
	require.Equal(t, "wait", fields["stage"])
	require.Equal(t, "wait_failed", fields["kind"])
	require.NotContains(t, fields["message"], "private-token")
	require.NotContains(t, fields["message"], "private.example")
	require.True(t, strings.Contains(fields["message"].(string), "[url-redacted]"))
}

func TestRetryCursorMCPConnectionDoesNotPublishStaleCommandDiagnostic(t *testing.T) {
	for _, operation := range []string{"enable", "list-tools"} {
		t.Run(operation, func(t *testing.T) {
			manager, execution, profile := newCursorMCPRecoveryFixture(t)
			prepareCursorMCPRecoveryWorkspace(t, manager, execution, profile)
			kind := mcpconfig.NativeMCPDiagnosticOperation("enable")
			if operation == "list-tools" {
				kind = "list_tools"
			}
			runner := &cursorMCPDiagnosticBarrierRunner{
				blockedOperation: operation,
				started:          make(chan struct{}),
				release:          make(chan struct{}),
				result: mcpconfig.NativeMCPCommandResult{Diagnostic: &mcpconfig.NativeMCPDiagnostic{
					Operation: kind, Stage: "wait", Kind: "output_wait_timeout", Message: "exec: WaitDelay expired",
				}},
				err: exec.ErrWaitDelay,
			}
			manager.SetCursorNativeMCPCommandRunner(runner)
			type retryOutcome struct {
				result CursorMCPRetryResult
				err    error
			}
			done := make(chan retryOutcome, 1)
			go func() {
				result, err := manager.RetryCursorMCPConnection(context.Background(), execution.SessionID, "plugin-harness-figma")
				done <- retryOutcome{result: result, err: err}
			}()
			<-runner.started
			key := cursorProjectMCPWorkspaceKey(execution.WorkspacePath)
			nextGeneration := beginCursorProjectMCPPreparation(key)
			defer finishCursorProjectMCPPreparation(key, nextGeneration)
			close(runner.release)

			outcome := <-done
			require.NoError(t, outcome.err)
			require.Equal(t, string(mcpconfig.NativeMCPStatusUnavailable), outcome.result.Status)
			require.Nil(t, outcome.result.Diagnostic)
			var approval, verification *PrepareStep
			for index := range execution.PrepareResult.Steps {
				step := &execution.PrepareResult.Steps[index]
				if step.MCPServerID != "plugin-harness-figma" {
					require.Nil(t, step.Diagnostic)
					continue
				}
				require.Nil(t, step.Diagnostic)
				if step.Kind == PrepareStepKindAgentMCPApproval {
					approval = step
				}
				if step.Kind == PrepareStepKindAgentMCPVerification {
					verification = step
				}
			}
			require.NotNil(t, approval)
			require.NotNil(t, verification)
			if operation == "enable" {
				require.Equal(t, "stale", approval.FailureCode)
				require.Equal(t, PrepareStepSkipped, verification.Status)
			} else {
				require.Equal(t, PrepareStepCompleted, approval.Status)
				require.Equal(t, "stale", verification.FailureCode)
			}
		})
	}
}

func TestRetryCursorMCPConnectionRetainsContextEndedDiagnostic(t *testing.T) {
	for _, operation := range []string{"enable", "list-tools"} {
		t.Run(operation, func(t *testing.T) {
			manager, execution, profile := newCursorMCPRecoveryFixture(t)
			prepareCursorMCPRecoveryWorkspace(t, manager, execution, profile)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			diagnosticOperation := mcpconfig.NativeMCPDiagnosticOperation("enable")
			if operation == "list-tools" {
				diagnosticOperation = "list_tools"
			}
			diagnostic := &mcpconfig.NativeMCPDiagnostic{
				Operation: diagnosticOperation, Stage: "wait", Kind: "output_wait_timeout",
				Message: "exec: WaitDelay expired",
			}
			manager.SetCursorNativeMCPCommandRunner(&cursorMCPContextCancelDiagnosticRunner{
				cancelOperation: operation, cancel: cancel, diagnostic: diagnostic,
			})

			result, err := manager.RetryCursorMCPConnection(ctx, execution.SessionID, "plugin-harness-figma")

			require.NoError(t, err)
			require.Equal(t, string(mcpconfig.NativeMCPStatusUnavailable), result.Status)
			require.Equal(t, pluginExecutorStateUnavailable, result.ReasonCode)
			require.Equal(t, diagnostic, result.Diagnostic)
			var approval, verification *PrepareStep
			for index := range execution.PrepareResult.Steps {
				step := &execution.PrepareResult.Steps[index]
				if step.MCPServerID != "plugin-harness-figma" {
					continue
				}
				switch step.Kind {
				case PrepareStepKindAgentMCPApproval:
					approval = step
				case PrepareStepKindAgentMCPVerification:
					verification = step
				}
			}
			require.NotNil(t, approval)
			require.NotNil(t, verification)
			if operation == "enable" {
				require.Equal(t, diagnostic, approval.Diagnostic)
				require.Equal(t, PrepareStepSkipped, verification.Status)
			} else {
				require.Equal(t, PrepareStepCompleted, approval.Status)
				require.Equal(t, PrepareStepFailed, verification.Status)
				require.Equal(t, diagnostic, verification.Diagnostic)
			}
		})
	}
}

type cursorMCPContextCancelDiagnosticRunner struct {
	cancelOperation string
	cancel          context.CancelFunc
	diagnostic      *mcpconfig.NativeMCPDiagnostic
}

func (r *cursorMCPContextCancelDiagnosticRunner) Run(_ context.Context, _ string, args []string, _ string, _ map[string]string) (mcpconfig.NativeMCPCommandResult, error) {
	if args[1] == r.cancelOperation {
		r.cancel()
		return mcpconfig.NativeMCPCommandResult{
			ExitCode: 0, ExitCodeObserved: true, Diagnostic: r.diagnostic,
		}, exec.ErrWaitDelay
	}
	if args[1] == "enable" {
		return mcpconfig.NativeMCPCommandResult{ExitCode: 0, ExitCodeObserved: true}, nil
	}
	return mcpconfig.NativeMCPCommandResult{
		ExitCode: 0, ExitCodeObserved: true,
		Stdout: []byte("Tools for " + args[2] + " (1):\n- fixture_tool ()\n"),
	}, nil
}

type cursorMCPDiagnosticBarrierRunner struct {
	blockedOperation string
	started          chan struct{}
	release          chan struct{}
	result           mcpconfig.NativeMCPCommandResult
	err              error
	calls            []string
	once             sync.Once
}

type cursorMCPFixedDiagnosticRunner struct {
	result mcpconfig.NativeMCPCommandResult
	err    error
}

func (r cursorMCPFixedDiagnosticRunner) Run(context.Context, string, []string, string, map[string]string) (mcpconfig.NativeMCPCommandResult, error) {
	return r.result, r.err
}

func (r *cursorMCPDiagnosticBarrierRunner) Run(_ context.Context, _ string, args []string, _ string, _ map[string]string) (mcpconfig.NativeMCPCommandResult, error) {
	operation := args[1]
	r.calls = append(r.calls, operation)
	if operation == r.blockedOperation {
		r.once.Do(func() {
			close(r.started)
			<-r.release
		})
		return r.result, r.err
	}
	if operation == "enable" {
		return mcpconfig.NativeMCPCommandResult{ExitCode: 0, ExitCodeObserved: true}, nil
	}
	return mcpconfig.NativeMCPCommandResult{ExitCode: 0, ExitCodeObserved: true, Stdout: []byte("Tools for " + args[2] + " (1):\n- fixture_tool ()\n")}, nil
}

func timeForMCPDiagnosticTest() time.Time {
	return time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
}
