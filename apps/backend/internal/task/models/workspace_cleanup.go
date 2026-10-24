package models

// WorkspaceCleanupMode separates task removal from physical workspace disposal.
type WorkspaceCleanupMode string

const (
	WorkspaceCleanupDelete WorkspaceCleanupMode = "delete"
	WorkspaceCleanupRetain WorkspaceCleanupMode = "retain"
)

func (m WorkspaceCleanupMode) Normalized() WorkspaceCleanupMode {
	if m == "" {
		return WorkspaceCleanupDelete
	}
	return m
}
