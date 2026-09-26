package process

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

type recordingWorkspaceNotifier struct {
	outputs  []*types.ProcessOutput
	statuses []*types.ProcessStatusUpdate
}

func (r *recordingWorkspaceNotifier) notifyWorkspaceStreamProcessOutput(output *types.ProcessOutput) {
	r.outputs = append(r.outputs, output)
}

func (r *recordingWorkspaceNotifier) notifyWorkspaceStreamProcessStatus(status *types.ProcessStatusUpdate) {
	r.statuses = append(r.statuses, status)
}

// replacementDuringNotificationNotifier models a graph rescan that replaces
// and retires the active tracker while a process update is being published.
// TryLock is deliberately used only as a test probe: the production publish
// path must keep this writer from acquiring the tracker lock until notify
// returns.
type replacementDuringNotificationNotifier struct {
	runner      *ProcessRunner
	replacement workspaceStreamNotifier
	retired     bool
	outputs     []*types.ProcessOutput
	statuses    []*types.ProcessStatusUpdate
}

func (r *replacementDuringNotificationNotifier) notifyWorkspaceStreamProcessOutput(
	output *types.ProcessOutput,
) {
	if r.tryReplace() {
		return
	}
	r.outputs = append(r.outputs, output)
}

func (r *replacementDuringNotificationNotifier) notifyWorkspaceStreamProcessStatus(
	status *types.ProcessStatusUpdate,
) {
	if r.tryReplace() {
		return
	}
	r.statuses = append(r.statuses, status)
}

func (r *replacementDuringNotificationNotifier) tryReplace() bool {
	if !r.runner.workspaceTrackerMu.TryLock() {
		return false
	}
	r.retired = true
	r.runner.workspaceTracker = r.replacement
	r.runner.workspaceTrackerMu.Unlock()
	return true
}

func TestProcessRunnerKeepsTrackerSelectedDuringPublication(t *testing.T) {
	proc := &commandProcess{
		info: ProcessInfo{
			ID:        "process-1",
			SessionID: "session-1",
			Kind:      types.ProcessKind("dev"),
			Status:    types.ProcessStatus("running"),
		},
	}

	tests := []struct {
		name    string
		publish func(*ProcessRunner, *commandProcess)
		got     func(*replacementDuringNotificationNotifier) int
	}{
		{
			name: "output",
			publish: func(runner *ProcessRunner, proc *commandProcess) {
				runner.publishOutput(proc, ProcessOutputChunk{
					Stream:    "stdout",
					Data:      "hello",
					Timestamp: time.Now().UTC(),
				})
			},
			got: func(probe *replacementDuringNotificationNotifier) int {
				return len(probe.outputs)
			},
		},
		{
			name: "status",
			publish: func(runner *ProcessRunner, proc *commandProcess) {
				runner.publishStatus(proc)
			},
			got: func(probe *replacementDuringNotificationNotifier) int {
				return len(probe.statuses)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := NewProcessRunner(nil, newTestLogger(t), 1024)
			replacement := &recordingWorkspaceNotifier{}
			probe := &replacementDuringNotificationNotifier{
				runner:      runner,
				replacement: replacement,
			}
			runner.workspaceTracker = probe

			test.publish(runner, proc)

			if probe.retired {
				t.Fatal("tracker replacement acquired the lock during publication")
			}
			if got := test.got(probe); got != 1 {
				t.Fatalf("active tracker received %d publications, want 1", got)
			}
		})
	}
}
