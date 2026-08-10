package client

import (
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// recordingCallbacks builds a WorkspaceStreamCallbacks whose every hook appends
// its name (and the payload identity it received) to `calls`.
func recordingCallbacks(calls *[]string) WorkspaceStreamCallbacks {
	return WorkspaceStreamCallbacks{
		OnShellOutput:   func(data string) { *calls = append(*calls, "shell_output:"+data) },
		OnShellExit:     func(code int) { *calls = append(*calls, "shell_exit:"+strconv.Itoa(code)) },
		OnGitStatus:     func(*GitStatusUpdate) { *calls = append(*calls, "git_status") },
		OnGitCommit:     func(*GitCommitNotification) { *calls = append(*calls, "git_commit") },
		OnGitReset:      func(*GitResetNotification) { *calls = append(*calls, "git_reset") },
		OnBranchSwitch:  func(*GitBranchSwitchNotification) { *calls = append(*calls, "branch_switch") },
		OnFileChange:    func(*FileChangeNotification) { *calls = append(*calls, "file_change") },
		OnProcessOutput: func(*types.ProcessOutput) { *calls = append(*calls, "process_output") },
		OnProcessStatus: func(*types.ProcessStatusUpdate) { *calls = append(*calls, "process_status") },
		OnConnected:     func() { *calls = append(*calls, "connected") },
		OnError:         func(err string) { *calls = append(*calls, "error:"+err) },
	}
}

// Every message type must reach exactly its own callback. A misrouted case in
// the dispatch switch silently drops UI updates rather than erroring.
func TestDispatchWorkspaceMessage_RoutesEachTypeToItsOwnCallback(t *testing.T) {
	tests := []struct {
		name string
		msg  types.WorkspaceStreamMessage
		want string
	}{
		{
			name: "shell output",
			msg:  types.WorkspaceStreamMessage{Type: types.WorkspaceMessageTypeShellOutput, Data: "hi"},
			want: "shell_output:hi",
		},
		{
			name: "shell exit",
			msg:  types.WorkspaceStreamMessage{Type: types.WorkspaceMessageTypeShellExit, Code: 137},
			want: "shell_exit:137",
		},
		{
			name: "connected",
			msg:  types.WorkspaceStreamMessage{Type: types.WorkspaceMessageTypeConnected},
			want: "connected",
		},
		{
			name: "error",
			msg:  types.WorkspaceStreamMessage{Type: types.WorkspaceMessageTypeError, Error: "boom"},
			want: "error:boom",
		},
		{
			name: "git status",
			msg: types.WorkspaceStreamMessage{
				Type: types.WorkspaceMessageTypeGitStatus, GitStatus: &GitStatusUpdate{},
			},
			want: "git_status",
		},
		{
			name: "git commit",
			msg: types.WorkspaceStreamMessage{
				Type: types.WorkspaceMessageTypeGitCommit, GitCommit: &GitCommitNotification{},
			},
			want: "git_commit",
		},
		{
			name: "git reset",
			msg: types.WorkspaceStreamMessage{
				Type: types.WorkspaceMessageTypeGitReset, GitReset: &GitResetNotification{},
			},
			want: "git_reset",
		},
		{
			name: "branch switch",
			msg: types.WorkspaceStreamMessage{
				Type: types.WorkspaceMessageTypeBranchSwitch, BranchSwitch: &GitBranchSwitchNotification{},
			},
			want: "branch_switch",
		},
		{
			name: "file change",
			msg: types.WorkspaceStreamMessage{
				Type: types.WorkspaceMessageTypeFileChange, FileChange: &FileChangeNotification{},
			},
			want: "file_change",
		},
		{
			name: "process output",
			msg: types.WorkspaceStreamMessage{
				Type: types.WorkspaceMessageTypeProcessOutput, ProcessOutput: &types.ProcessOutput{},
			},
			want: "process_output",
		},
		{
			name: "process status",
			msg: types.WorkspaceStreamMessage{
				Type: types.WorkspaceMessageTypeProcessStatus, ProcessStatus: &types.ProcessStatusUpdate{},
			},
			want: "process_status",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			dispatchWorkspaceMessage(tc.msg, recordingCallbacks(&calls))

			if len(calls) != 1 {
				t.Fatalf("callbacks fired = %v, want exactly [%s]", calls, tc.want)
			}
			if calls[0] != tc.want {
				t.Errorf("callback fired = %q, want %q", calls[0], tc.want)
			}
		})
	}
}

// Typed messages carry a nillable payload; dispatching one with a nil body must
// not invoke the callback (which would panic on the nil deref downstream).
func TestDispatchWorkspaceMessage_SkipsCallbacksWithNilPayloads(t *testing.T) {
	nilPayloadTypes := []types.WorkspaceMessageType{
		types.WorkspaceMessageTypeGitStatus,
		types.WorkspaceMessageTypeGitCommit,
		types.WorkspaceMessageTypeGitReset,
		types.WorkspaceMessageTypeBranchSwitch,
		types.WorkspaceMessageTypeFileChange,
		types.WorkspaceMessageTypeProcessOutput,
		types.WorkspaceMessageTypeProcessStatus,
	}
	for _, msgType := range nilPayloadTypes {
		t.Run(string(msgType), func(t *testing.T) {
			var calls []string
			dispatchWorkspaceMessage(
				types.WorkspaceStreamMessage{Type: msgType}, recordingCallbacks(&calls))
			if len(calls) != 0 {
				t.Errorf("callbacks fired = %v, want none for a nil payload", calls)
			}
		})
	}
}

// A message whose callback is unset (the common case — most callers wire only a
// few hooks) must be a no-op rather than a nil-func panic.
func TestDispatchWorkspaceMessage_ToleratesUnsetCallbacks(t *testing.T) {
	allTypes := []types.WorkspaceMessageType{
		types.WorkspaceMessageTypeShellOutput,
		types.WorkspaceMessageTypeShellExit,
		types.WorkspaceMessageTypeConnected,
		types.WorkspaceMessageTypeError,
		types.WorkspaceMessageTypeGitStatus,
		types.WorkspaceMessageTypeGitCommit,
		types.WorkspaceMessageTypeGitReset,
		types.WorkspaceMessageTypeBranchSwitch,
		types.WorkspaceMessageTypeFileChange,
		types.WorkspaceMessageTypeProcessOutput,
		types.WorkspaceMessageTypeProcessStatus,
	}
	for _, msgType := range allTypes {
		t.Run(string(msgType), func(t *testing.T) {
			// Payloads are populated so the nil-payload guard doesn't mask a
			// nil-callback panic.
			msg := types.WorkspaceStreamMessage{
				Type:          msgType,
				GitStatus:     &GitStatusUpdate{},
				GitCommit:     &GitCommitNotification{},
				GitReset:      &GitResetNotification{},
				BranchSwitch:  &GitBranchSwitchNotification{},
				FileChange:    &FileChangeNotification{},
				ProcessOutput: &types.ProcessOutput{},
				ProcessStatus: &types.ProcessStatusUpdate{},
			}
			dispatchWorkspaceMessage(msg, WorkspaceStreamCallbacks{})
		})
	}
}

// Ping/pong and shell_input are handled elsewhere in the stream loop; the
// dispatcher must fall through them without firing an unrelated callback.
func TestDispatchWorkspaceMessage_IgnoresUnroutedTypes(t *testing.T) {
	for _, msgType := range []types.WorkspaceMessageType{
		types.WorkspaceMessageTypePing,
		types.WorkspaceMessageTypePong,
		types.WorkspaceMessageTypeShellInput,
		types.WorkspaceMessageTypeShellResize,
		"totally_unknown",
	} {
		t.Run(string(msgType), func(t *testing.T) {
			var calls []string
			dispatchWorkspaceMessage(
				types.WorkspaceStreamMessage{Type: msgType, Data: "x", Error: "y"},
				recordingCallbacks(&calls))
			if len(calls) != 0 {
				t.Errorf("callbacks fired = %v, want none for an unrouted type", calls)
			}
		})
	}
}

// The dispatcher is layered shell → git → process, with the first two
// short-circuiting on a match. These assertions pin the handled/unhandled
// contract each layer reports.
func TestDispatchWorkspaceLayers_ReportWhetherTheyHandledTheMessage(t *testing.T) {
	var calls []string
	callbacks := recordingCallbacks(&calls)

	shellMsg := types.WorkspaceStreamMessage{Type: types.WorkspaceMessageTypeShellOutput, Data: "x"}
	if !dispatchWorkspaceShellMessages(shellMsg, callbacks) {
		t.Error("shell layer reported unhandled for a shell_output message")
	}
	if dispatchWorkspaceGitMessages(shellMsg, callbacks) {
		t.Error("git layer claimed a shell_output message")
	}

	gitMsg := types.WorkspaceStreamMessage{
		Type: types.WorkspaceMessageTypeGitStatus, GitStatus: &GitStatusUpdate{},
	}
	if dispatchWorkspaceShellMessages(gitMsg, callbacks) {
		t.Error("shell layer claimed a git_status message")
	}
	if !dispatchWorkspaceGitMessages(gitMsg, callbacks) {
		t.Error("git layer reported unhandled for a git_status message")
	}

	processMsg := types.WorkspaceStreamMessage{
		Type: types.WorkspaceMessageTypeProcessOutput, ProcessOutput: &types.ProcessOutput{},
	}
	if dispatchWorkspaceShellMessages(processMsg, callbacks) {
		t.Error("shell layer claimed a process_output message")
	}
	if dispatchWorkspaceGitMessages(processMsg, callbacks) {
		t.Error("git layer claimed a process_output message")
	}
}
