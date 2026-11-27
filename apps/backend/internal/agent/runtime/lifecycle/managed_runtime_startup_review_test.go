package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

func TestManagedRuntimeNpmStartupFailureDoesNotNestClientLease(t *testing.T) {
	mgr, execution, mock, _ := newManagedRuntimeRetryFixture(t, false)
	mock.stderrLines = []string{
		"npm error code ETARGET",
		"npm error notarget No matching version found for opencode-ai@1.2.3",
	}

	client, release := execution.AcquireAgentCtlClient()
	writerStarted := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		close(writerStarted)
		execution.agentctlLifecycleMu.Lock()
		execution.agentctl = client
		execution.agentctlLifecycleMu.Unlock()
		close(writerDone)
	}()
	<-writerStarted
	waitForAgentCtlWriter(t, execution)

	diagnosticDone := make(chan struct{})
	go func() {
		_ = mgr.managedRuntimeNpmStartupFailure(
			context.Background(), client, execution, errors.New("initialize failed"), "opencode-ai@1.2.3",
		)
		close(diagnosticDone)
	}()

	completedUnderPinnedLease := false
	select {
	case <-diagnosticDone:
		completedUnderPinnedLease = true
	case <-time.After(5 * time.Second):
	}
	release()
	select {
	case <-diagnosticDone:
	case <-time.After(time.Second):
		t.Fatal("startup diagnostic did not finish after releasing the outer client lease")
	}
	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("replacement writer did not acquire the lifecycle lock")
	}
	if !completedUnderPinnedLease {
		t.Fatal("startup diagnostic waited for a nested client lease while a replacement writer was queued")
	}
}

func TestManagedRuntimeStartupRetryPreservesFinalCause(t *testing.T) {
	exitCode := 1
	cases := []struct {
		name               string
		initializeError    string
		sessionNewError    string
		configureError     string
		startError         string
		wantReason         string
		wantDiagnostic     string
		wantAuthentication bool
	}{
		{
			name:               "authentication refusal during initialize",
			initializeError:    "Authentication required: please log in",
			wantReason:         "retry_initialize_failed",
			wantDiagnostic:     "Authentication required",
			wantAuthentication: true,
		},
		{
			name:               "authentication refusal during session creation",
			sessionNewError:    "Authentication required: please log in",
			wantReason:         "retry_session_setup_failed",
			wantDiagnostic:     "Authentication required",
			wantAuthentication: true,
		},
		{
			name:            "session creation error retains final cause",
			sessionNewError: "session database unavailable",
			wantReason:      "retry_session_setup_failed",
			wantDiagnostic:  "session database unavailable",
		},
		{
			name:           "configuration error is not post initialize",
			configureError: "configuration rejected for test secret=super-secret",
			wantReason:     "retry_process_setup_failed",
			wantDiagnostic: "configuration rejected for test",
		},
		{
			name:           "process start error is not post initialize",
			startError:     "process start rejected for test",
			wantReason:     "retry_process_setup_failed",
			wantDiagnostic: "process start rejected for test",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			mgr, execution, mock, agentConfig := newManagedRuntimeRetryFixture(t, false)
			client, release := execution.AcquireAgentCtlClient()
			if _, err := client.Start(context.Background()); err != nil {
				release()
				t.Fatalf("seed initial process generation: %v", err)
			}
			release()
			mock.mu.Lock()
			mock.httpActions = nil
			mock.initializeError = tt.initializeError
			mock.sessionNewError = tt.sessionNewError
			mock.configureError = tt.configureError
			mock.startError = tt.startError
			mock.stderrLines = []string{"npm error code ECONNRESET"}
			mock.stderrConfigured = false
			mock.mu.Unlock()

			initial := managedACPInitializeEvidenceFailure("initial npm ECONNRESET", &agentctltypes.ManagedStartupEvidence{
				ProcessGeneration:     1,
				ExitDisposition:       agentctltypes.ManagedStartupExitOrdinary,
				ExitCode:              &exitCode,
				NPMCode:               "ECONNRESET",
				CollectionComplete:    true,
				NPMDiagnosticPresent:  true,
				NPMDiagnosticComplete: true,
			})
			attempted, err := mgr.retryManagedRuntimeStartup(
				context.Background(), execution, initial, agentConfig, "", nil, nil,
			)
			if !attempted || err == nil {
				t.Fatalf("retry result = (%v, %v), want exhausted replacement", attempted, err)
			}
			var startupErr *routingerr.ManagedRuntimeStartupError
			if !errors.As(err, &startupErr) {
				t.Fatalf("retry error = %v, want structured startup error", err)
			}
			if startupErr.Reason != tt.wantReason || startupErr.Attempts != 2 {
				t.Fatalf("startup metadata = reason %q attempts %d, want %q and 2", startupErr.Reason, startupErr.Attempts, tt.wantReason)
			}
			if startupErr.Code != routingerr.CodeAgentRuntime {
				t.Fatalf("final failure code = %q, want agent runtime for final non-policy cause", startupErr.Code)
			}
			if !strings.Contains(startupErr.Details, tt.wantDiagnostic) || !strings.Contains(startupErr.Error(), tt.wantDiagnostic) {
				t.Fatalf("final diagnostic was lost: details=%q error=%q", startupErr.Details, startupErr.Error())
			}
			if strings.Contains(startupErr.Details, "super-secret") {
				t.Fatalf("final diagnostic exposed secret: %q", startupErr.Details)
			}
			if tt.wantAuthentication && !strings.Contains(strings.ToLower(startupErr.Error()), "authentication required") {
				t.Fatalf("authentication refusal missing from final error: %q", startupErr.Error())
			}
			if execution.StartupFailureReason != tt.wantReason || execution.StartupFailureAttempts != 2 {
				t.Fatalf("execution metadata = reason %q attempts %d", execution.StartupFailureReason, execution.StartupFailureAttempts)
			}
		})
	}
}

func TestManagedRuntimeStartupRetryRejectsUnknownOrIncompleteNPMEvidence(t *testing.T) {
	exitCode := 1
	cases := []struct {
		name             string
		evidence         *agentctltypes.ManagedStartupEvidence
		stderr           []string
		stderrConfigured bool
		wantRetry        bool
	}{
		{
			name: "unknown usage code",
			evidence: &agentctltypes.ManagedStartupEvidence{
				CollectionComplete:   true,
				NPMDiagnosticPresent: true, NPMDiagnosticComplete: true, UnclassifiedNPMCode: true,
			},
			stderr: []string{"npm error code EUSAGE"},
		},
		{
			name: "unknown platform code",
			evidence: &agentctltypes.ManagedStartupEvidence{
				CollectionComplete:   true,
				NPMDiagnosticPresent: true, NPMDiagnosticComplete: true, UnclassifiedNPMCode: true,
			},
			stderr: []string{"npm error code EBADPLATFORM"},
		},
		{
			name: "oversized diagnostics containing permanent code",
			evidence: &agentctltypes.ManagedStartupEvidence{
				CollectionComplete:   true,
				NPMDiagnosticPresent: true, NPMDiagnosticComplete: false,
			},
			stderr: []string{"npm error code EACCES"},
		},
		{
			name: "non-npm stderr is not empty evidence",
			evidence: &agentctltypes.ManagedStartupEvidence{
				CollectionComplete: true, NPMDiagnosticComplete: false,
			},
			stderr: []string{"configuration file is invalid"},
		},
		{
			name: "genuinely empty stderr remains eligible",
			evidence: &agentctltypes.ManagedStartupEvidence{
				CollectionComplete:    true,
				NPMDiagnosticComplete: true,
			},
			stderrConfigured: true,
			wantRetry:        true,
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

			tt.evidence.ProcessGeneration = 1
			tt.evidence.ExitDisposition = agentctltypes.ManagedStartupExitOrdinary
			tt.evidence.ExitCode = &exitCode
			attempted, err := mgr.retryManagedRuntimeStartup(
				context.Background(), execution,
				managedACPInitializeEvidenceFailure("process exited during initialize", tt.evidence),
				agentConfig, "", nil, nil,
			)
			if tt.wantRetry {
				if err != nil || !attempted {
					t.Fatalf("retry result = (%v, %v), want one successful replacement", attempted, err)
				}
				if got := mock.getHTTPActions(); !slices.Equal(got, []string{"stop", "configure", "start"}) {
					t.Fatalf("HTTP actions = %#v, want one replacement", got)
				}
				return
			}
			if attempted || err == nil {
				t.Fatalf("retry result = (%v, %v), want fail-closed original error", attempted, err)
			}
			if got := mock.getHTTPActions(); len(got) != 0 {
				t.Fatalf("HTTP actions = %#v, want no replacement", got)
			}
		})
	}
}

func TestAgentEventPayloadSnapshotsManagedStartupFailureFields(t *testing.T) {
	store := NewExecutionStore()
	execution := &AgentExecution{ID: "startup-failure-snapshot"}
	if err := store.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	start := make(chan struct{})
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		<-start
		for attempts := 1; attempts <= 200; attempts++ {
			_ = store.WithLock(execution.ID, func(current *AgentExecution) {
				current.setStartupFailureMetadata(managedRuntimeStartupFailureMetadata{
					reason: fmt.Sprintf("reason-%d", attempts), attempts: attempts, npmCode: fmt.Sprintf("code-%d", attempts),
				})
			})
		}
	}()

	close(start)
	for range 200 {
		payload := newAgentEventPayload(execution)
		if payload.StartupFailureAttempts == 0 {
			continue
		}
		if want := fmt.Sprintf("reason-%d", payload.StartupFailureAttempts); payload.StartupFailureReason != want {
			t.Fatalf("startup failure payload reason = %q, want %q", payload.StartupFailureReason, want)
		}
		if want := fmt.Sprintf("code-%d", payload.StartupFailureAttempts); payload.StartupFailureNPMCode != want {
			t.Fatalf("startup failure payload npm code = %q, want %q", payload.StartupFailureNPMCode, want)
		}
	}
	<-writeDone
}

func waitForAgentCtlWriter(t *testing.T, execution *AgentExecution) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if execution.agentctlLifecycleMu.TryRLock() {
			execution.agentctlLifecycleMu.RUnlock()
		} else {
			return
		}
		select {
		case <-deadline:
			t.Fatal("replacement writer did not queue behind the pinned client lease")
		default:
			runtime.Gosched()
		}
	}
}
