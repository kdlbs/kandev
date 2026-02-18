package client

import (
	"github.com/kandev/kandev/internal/agentctl/types"
)

func dispatchWorkspaceShellMessages(msg types.WorkspaceStreamMessage, callbacks WorkspaceStreamCallbacks) bool {
	switch msg.Type {
	case types.WorkspaceMessageTypeShellOutput:
		if callbacks.OnShellOutput != nil {
			callbacks.OnShellOutput(msg.Data)
		}
	case types.WorkspaceMessageTypeShellExit:
		if callbacks.OnShellExit != nil {
			callbacks.OnShellExit(msg.Code)
		}
	case types.WorkspaceMessageTypeConnected:
		if callbacks.OnConnected != nil {
			callbacks.OnConnected()
		}
	case types.WorkspaceMessageTypeError:
		if callbacks.OnError != nil {
			callbacks.OnError(msg.Error)
		}
	default:
		return false
	}
	return true
}

func dispatchWorkspaceGitMessages(msg types.WorkspaceStreamMessage, callbacks WorkspaceStreamCallbacks) bool {
	switch msg.Type {
	case types.WorkspaceMessageTypeGitStatus:
		if callbacks.OnGitStatus != nil && msg.GitStatus != nil {
			callbacks.OnGitStatus(msg.GitStatus)
		}
	case types.WorkspaceMessageTypeGitCommit:
		if callbacks.OnGitCommit != nil && msg.GitCommit != nil {
			callbacks.OnGitCommit(msg.GitCommit)
		}
	case types.WorkspaceMessageTypeGitReset:
		if callbacks.OnGitReset != nil && msg.GitReset != nil {
			callbacks.OnGitReset(msg.GitReset)
		}
	case types.WorkspaceMessageTypeFileChange:
		if callbacks.OnFileChange != nil && msg.FileChange != nil {
			callbacks.OnFileChange(msg.FileChange)
		}
	default:
		return false
	}
	return true
}

func dispatchWorkspaceProcessMessages(msg types.WorkspaceStreamMessage, callbacks WorkspaceStreamCallbacks) {
	switch msg.Type {
	case types.WorkspaceMessageTypeProcessOutput:
		if callbacks.OnProcessOutput != nil && msg.ProcessOutput != nil {
			callbacks.OnProcessOutput(msg.ProcessOutput)
		}
	case types.WorkspaceMessageTypeProcessStatus:
		if callbacks.OnProcessStatus != nil && msg.ProcessStatus != nil {
			callbacks.OnProcessStatus(msg.ProcessStatus)
		}
	}
}

// dispatchWorkspaceMessage routes a workspace stream message to the appropriate callback.
func dispatchWorkspaceMessage(msg types.WorkspaceStreamMessage, callbacks WorkspaceStreamCallbacks) {
	if dispatchWorkspaceShellMessages(msg, callbacks) {
		return
	}
	if dispatchWorkspaceGitMessages(msg, callbacks) {
		return
	}
	dispatchWorkspaceProcessMessages(msg, callbacks)
}
