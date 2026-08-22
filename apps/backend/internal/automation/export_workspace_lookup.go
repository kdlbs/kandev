package automation

import "context"

// ExportWorkspaceLookup answers whether a workspace exists, for AC-44 step 2's
// existence check. Deliberately not WorkflowLocator or RepositoryLookup:
// those answer ownership questions this package can already ask through
// authorizeWorkspace or the store. This lookup exists for the unscoped
// caller — Kandev's default mode — where step 1's authorizer returns nil
// without reading anything and Service.ListAutomations cannot distinguish a
// real but empty workspace from one that was never there (AC-35).
//
// Implementations return nil when the workspace exists, an error wrapping
// repoerrors.ErrWorkspaceNotFound when it does not, and any other error for
// an infrastructure failure. Satisfied by an adapter over the task service,
// injected via SetExportWorkspaceLookup rather than imported directly, to
// avoid a cyclic import — the same reason WorkflowLocator and
// RepositoryLookup are interfaces here rather than concrete task-service
// types.
type ExportWorkspaceLookup interface {
	WorkspaceExists(ctx context.Context, workspaceID string) error
}
