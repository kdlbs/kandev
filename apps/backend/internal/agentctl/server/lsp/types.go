package lsp

import sharedlsp "github.com/kandev/kandev/internal/lsp"

type Activity = sharedlsp.Activity

const (
	ActivityIdle       = sharedlsp.ActivityIdle
	ActivityServerWork = sharedlsp.ActivityServerWork
)

type WorkItem = sharedlsp.WorkItem
type CompletedWorkItem = sharedlsp.CompletedWorkItem
type Snapshot = sharedlsp.RuntimeSnapshot
type WorkspaceFolder = sharedlsp.WorkspaceFolder
type StartRequest = sharedlsp.TaskHostStartRequest
type ConfigurationRequest = sharedlsp.TaskHostConfigurationRequest
type StopRequest = sharedlsp.TaskHostStopRequest

type Config struct {
	WorkDir          string
	WorkspaceURI     string
	WorkspaceFolders []WorkspaceFolder
	DiscoveryRoots   []string
	OwnerID          string
}
