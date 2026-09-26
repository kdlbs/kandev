package lifecycle

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

// TestApplyResumeIntent pins the first half of issue #2330: a passthrough
// session with no prior agent execution — the state a task created with
// start_agent:false is in — has never run, so its first launch must not carry
// the CLI resume flag. A session that did run before (its execution lost from
// the in-memory store by a backend restart) must still resume.
func TestApplyResumeIntent(t *testing.T) {
	tests := []struct {
		name                string
		previousExecutionID string
		wantResumed         bool
	}{
		{
			name:                "session that has never run is a fresh launch",
			previousExecutionID: "",
			wantResumed:         false,
		},
		{
			name:                "session with a prior execution is a recovery",
			previousExecutionID: "exec-previous",
			wantResumed:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execution := &AgentExecution{ID: "exec-1", SessionID: "sess-1"}
			applyResumeIntent(execution, &ExecutorCreateRequest{
				SessionID:           "sess-1",
				PreviousExecutionID: tt.previousExecutionID,
			})
			if execution.isResumedSession != tt.wantResumed {
				t.Errorf("isResumedSession = %v, want %v", execution.isResumedSession, tt.wantResumed)
			}
		})
	}
}

// TestApplyResumeIntentRoutesPassthroughLaunch pins the mapping from that
// intent to the launch path actually taken. The distinction matters beyond the
// resume flag: only startPassthroughSession delivers the session's stored
// prompt, while ResumePassthroughSession deliberately does not (it would
// duplicate the prompt in agent history).
func TestApplyResumeIntentRoutesPassthroughLaunch(t *testing.T) {
	tests := []struct {
		name                string
		previousExecutionID string
		wantFreshLaunch     bool
	}{
		{
			name:                "never ran routes to the fresh-launch path",
			previousExecutionID: "",
			wantFreshLaunch:     true,
		},
		{
			name:                "prior execution routes to the resume path",
			previousExecutionID: "exec-previous",
			wantFreshLaunch:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newTestManager(t)
			mgr.profileResolver = &mockAgentProfileResolver{cliPassthrough: true}

			execution := &AgentExecution{
				ID:             "exec-1",
				SessionID:      "sess-1",
				AgentProfileID: "profile-1",
				IsPassthrough:  true,
			}
			if err := mgr.executionStore.Add(execution); err != nil {
				t.Fatalf("seed execution: %v", err)
			}
			applyResumeIntent(execution, &ExecutorCreateRequest{
				SessionID:           "sess-1",
				PreviousExecutionID: tt.previousExecutionID,
			})

			err := mgr.startPassthroughExecution(context.Background(), execution, nil)
			if err == nil {
				t.Fatal("expected error (no interactive runner)")
			}
			// Both paths bail on the missing interactive runner, but with
			// distinct messages: startPassthroughSession's carries the
			// "for passthrough mode" suffix, ResumePassthroughSession's does not.
			const (
				freshLaunchError = "interactive runner not available for passthrough mode"
				resumeError      = "interactive runner not available"
			)
			errText := err.Error()
			gotFreshLaunch := errText == freshLaunchError
			if !gotFreshLaunch && errText != resumeError {
				t.Fatalf("unexpected runner-missing error: %v", err)
			}
			if gotFreshLaunch != tt.wantFreshLaunch {
				t.Errorf("fresh-launch path taken = %v, want %v (error: %v)", gotFreshLaunch, tt.wantFreshLaunch, err)
			}
		})
	}
}

// TestManager_HandlePassthroughStatus_ResumeFailureIsSticky pins the second
// half of issue #2330: any non-zero exit of a resume launch must set the sticky
// resume-failed guard, not only one landing inside the 2s fast-fail window. A
// real CLI spends seconds on PTY setup, shell startup and its own boot before
// it can report "No conversation found to continue", so gating the guard on
// fast-fail left it unset and every auto-restart re-attached the same broken
// resume flag — an endless loop.
func TestManager_HandlePassthroughStatus_ResumeFailureIsSticky(t *testing.T) {
	tests := []struct {
		name       string
		usedResume bool
		exitCode   int
		uptime     time.Duration
		wantFailed bool
	}{
		{
			name:       "slow resume failure (real CLI startup cost)",
			usedResume: true,
			exitCode:   1,
			uptime:     3400 * time.Millisecond,
			wantFailed: true,
		},
		{
			name:       "fast resume failure (bad flag, missing binary)",
			usedResume: true,
			exitCode:   127,
			uptime:     100 * time.Millisecond,
			wantFailed: true,
		},
		{
			name:       "clean exit keeps the resume intent",
			usedResume: true,
			exitCode:   0,
			uptime:     3 * time.Second,
			wantFailed: false,
		},
		{
			name:       "failure of a fresh launch is not a resume failure",
			usedResume: false,
			exitCode:   1,
			uptime:     3 * time.Second,
			wantFailed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mgr := newTestManager(t)
				// Shut the manager down so the auto-restart goroutine
				// short-circuits: this test covers the synchronous guard flip,
				// not the relaunch it enables.
				if err := mgr.StopAllAgents(context.Background()); err != nil {
					t.Fatalf("StopAllAgents: %v", err)
				}

				startedAt := time.Now()
				execution := &AgentExecution{
					ID:                          "exec-1",
					SessionID:                   "sess-1",
					PassthroughProcessID:        "proc-1",
					PassthroughStartedAt:        startedAt,
					passthroughLaunchUsedResume: tt.usedResume,
					isResumedSession:            tt.usedResume,
				}
				if err := mgr.executionStore.Add(execution); err != nil {
					t.Fatalf("seed execution: %v", err)
				}

				exitCode := tt.exitCode
				mgr.handlePassthroughStatus(&agentctltypes.ProcessStatusUpdate{
					SessionID: "sess-1",
					ProcessID: "proc-1",
					Status:    agentctltypes.ProcessStatusExited,
					ExitCode:  &exitCode,
					Timestamp: startedAt.Add(tt.uptime),
				})

				if execution.passthroughResumeFailed != tt.wantFailed {
					t.Errorf("passthroughResumeFailed = %v, want %v",
						execution.passthroughResumeFailed, tt.wantFailed)
				}
				// A tripped guard must also clear the resume intent, otherwise
				// the next launch would still be routed as a recovery.
				if tt.wantFailed && (execution.isResumedSession || execution.passthroughLaunchUsedResume) {
					t.Errorf("resume intent not cleared: isResumedSession=%v passthroughLaunchUsedResume=%v",
						execution.isResumedSession, execution.passthroughLaunchUsedResume)
				}
			})
		})
	}
}
