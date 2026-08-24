package backendapp

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// pluginsWorkspaceAgentPrincipalSourceAdapter creates the safe plugin Host
// projection from the durable authority repository. It never maps backing
// task/session IDs, operator identity, grant notes/scopes, or target IDs.
type pluginsWorkspaceAgentPrincipalSourceAdapter struct {
	repo taskrepo.WorkspaceAgentPrincipalRepository
}

func (a pluginsWorkspaceAgentPrincipalSourceAdapter) GetPluginWorkspaceAgentPrincipal(ctx context.Context, pluginID, workspaceID, logicalKey string) (*pluginsdk.WorkspaceAgentPrincipal, *pluginsdk.WorkspaceAgentPrincipalStatus, error) {
	if pluginID == "" || workspaceID == "" || logicalKey == "" {
		return nil, nil, status.Error(codes.InvalidArgument, "workspace_id and logical_key are required")
	}
	principal, err := a.repo.GetWorkspaceAgentPrincipalByContext(ctx, workspaceID, pluginID, logicalKey)
	if err == sql.ErrNoRows {
		// Do not disclose whether an ID belongs to a different workspace or
		// installation: absent and foreign are both NotFound.
		return nil, nil, status.Error(codes.NotFound, "workspace agent principal not found")
	}
	if err != nil {
		return nil, nil, err
	}
	if principal == nil || principal.WorkspaceID != workspaceID || principal.PluginInstallationID != pluginID {
		return nil, nil, status.Error(codes.NotFound, "workspace agent principal not found")
	}
	descriptor := pluginWorkspaceAgentPrincipalDescriptor(principal)
	principalStatus := pluginsdk.WorkspaceAgentPrincipalStatus{
		PrincipalID: principal.ID,
		UpdatedAt:   rfc3339(principal.UpdatedAt),
		State:       "active",
	}
	if principal.RevokedAt != nil {
		principalStatus.State = "revoked"
		return &descriptor, &principalStatus, nil
	}
	grants, err := a.repo.ListActiveWorkspaceAgentPrincipalGrants(ctx, principal.ID, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	principalStatus.GrantedCapabilities = grantedCapabilities(grants)
	return &descriptor, &principalStatus, nil
}

func (a pluginsWorkspaceAgentPrincipalSourceAdapter) ListPluginWorkspaceAgentPrincipalAudit(ctx context.Context, pluginID, workspaceID, logicalKey string) ([]pluginsdk.WorkspaceAgentPrincipalAuditEvent, error) {
	principal, _, err := a.GetPluginWorkspaceAgentPrincipal(ctx, pluginID, workspaceID, logicalKey)
	if err != nil {
		return nil, err
	}
	events, err := a.repo.ListWorkspaceAgentPrincipalAuditEvents(ctx, workspaceID, principal.ID, 10_000)
	if err != nil {
		return nil, err
	}
	out := make([]pluginsdk.WorkspaceAgentPrincipalAuditEvent, len(events))
	for i, event := range events {
		out[i] = pluginsdk.WorkspaceAgentPrincipalAuditEvent{
			ID: event.ID, OccurredAt: rfc3339(event.OccurredAt), Action: event.Action,
			Capability: event.Capability, Decision: event.Decision, Result: event.Result,
			DetailCode: event.Detail,
		}
	}
	return out, nil
}

func pluginWorkspaceAgentPrincipalDescriptor(principal *models.WorkspaceAgentPrincipal) pluginsdk.WorkspaceAgentPrincipal {
	return pluginsdk.WorkspaceAgentPrincipal{
		ID: principal.ID, WorkspaceID: principal.WorkspaceID, LogicalKey: principal.LogicalKey,
		CreatedAt: rfc3339(principal.CreatedAt), UpdatedAt: rfc3339(principal.UpdatedAt),
	}
}

func grantedCapabilities(grants []*models.CoordinatorGrant) []string {
	values := map[string]struct{}{}
	for _, grant := range grants {
		for _, capability := range strings.Split(grant.Capabilities, ",") {
			if capability = strings.TrimSpace(capability); capability != "" {
				values[capability] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(values))
	for capability := range values {
		out = append(out, capability)
	}
	sort.Strings(out)
	return out
}

func rfc3339(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
