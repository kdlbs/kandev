package plugins

import (
	"context"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

// workspaceAgentPrincipalReader binds every request to the calling plugin's
// registry identity. It deliberately has no create, grant, revoke, rebind, or
// delete method: those are operator/host lifecycle operations, never plugin
// authority.
type workspaceAgentPrincipalReader struct {
	pluginID string
	source   workspaceAgentPrincipalSource
}

func (r workspaceAgentPrincipalReader) Get(ctx context.Context, workspaceID, logicalKey string) (*pluginsdk.WorkspaceAgentPrincipal, error) {
	principal, _, err := r.source.GetPluginWorkspaceAgentPrincipal(ctx, r.pluginID, workspaceID, logicalKey)
	return principal, err
}

func (r workspaceAgentPrincipalReader) Status(ctx context.Context, workspaceID, logicalKey string) (*pluginsdk.WorkspaceAgentPrincipalStatus, error) {
	_, principalStatus, err := r.source.GetPluginWorkspaceAgentPrincipal(ctx, r.pluginID, workspaceID, logicalKey)
	return principalStatus, err
}

func (r workspaceAgentPrincipalReader) ListAudit(ctx context.Context, workspaceID, logicalKey string, page pluginsdk.Page) ([]pluginsdk.WorkspaceAgentPrincipalAuditEvent, *pluginsdk.PageInfo, error) {
	items, err := r.source.ListPluginWorkspaceAgentPrincipalAudit(ctx, r.pluginID, workspaceID, logicalKey)
	if err != nil {
		return nil, nil, err
	}
	out, info := paginate(items, page)
	return out, info, nil
}

type deniedWorkspaceAgentPrincipalReader struct{}

func (deniedWorkspaceAgentPrincipalReader) Get(context.Context, string, string) (*pluginsdk.WorkspaceAgentPrincipal, error) {
	return nil, permissionDenied(apiReadCapability(resourceWorkspaceAgentPrincipals))
}

func (deniedWorkspaceAgentPrincipalReader) Status(context.Context, string, string) (*pluginsdk.WorkspaceAgentPrincipalStatus, error) {
	return nil, permissionDenied(apiReadCapability(resourceWorkspaceAgentPrincipals))
}

func (deniedWorkspaceAgentPrincipalReader) ListAudit(context.Context, string, string, pluginsdk.Page) ([]pluginsdk.WorkspaceAgentPrincipalAuditEvent, *pluginsdk.PageInfo, error) {
	return nil, nil, permissionDenied(apiReadCapability(resourceWorkspaceAgentPrincipals))
}
