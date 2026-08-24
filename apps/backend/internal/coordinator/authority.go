// Package coordinator evaluates explicit, operator-granted task authority.
// It depends only on task models, keeping it safe to use from both task
// services and MCP handlers without creating an import cycle.
package coordinator

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

type Capability string

const (
	CapabilityInspect     Capability = "inspect"
	CapabilityOrchestrate Capability = "orchestrate"
)

type Basis string

const (
	BasisDirectParent Basis = "direct_parent"
	BasisGrant        Basis = "grant"
	BasisDenied       Basis = "denied"
)

const (
	ScopeWorkspace = "workspace"
	ScopeWorkflow  = "workflow"
)

type Request struct {
	ActorTask      *models.Task
	TargetTask     *models.Task
	ActorSessionID string
	Action         string
	Capability     Capability
}

// Decision is deliberately opaque on denial. DenyReason remains for source
// compatibility but Authorize never exposes the internal audit reason there.
type Decision struct {
	Allowed    bool
	Basis      Basis
	GrantID    string
	AuditID    string
	DenyReason string
}

type Store interface {
	GetActiveWorkspaceAgentPrincipalForTask(ctx context.Context, workspaceID, taskID string) (*models.WorkspaceAgentPrincipal, error)
	ListActiveWorkspaceAgentPrincipalGrants(ctx context.Context, principalID, workspaceID string) ([]*models.CoordinatorGrant, error)
	CreateCoordinatorAuditEvent(ctx context.Context, event *models.CoordinatorAuditEvent) error
	FinishCoordinatorAuditEvent(ctx context.Context, id, result, detail string) error
}

type Authority struct {
	store   Store
	enabled func() bool
}

func New(store Store, enabled func() bool) *Authority {
	if enabled == nil {
		enabled = func() bool { return false }
	}
	return &Authority{store: store, enabled: enabled}
}

func (a *Authority) Authorize(ctx context.Context, request Request) (Decision, error) {
	if request.ActorTask == nil || request.TargetTask == nil || request.ActorTask.ID == "" || request.TargetTask.ID == "" {
		return Decision{Basis: BasisDenied}, nil
	}
	if request.ActorTask.WorkspaceID != "" && request.ActorTask.WorkspaceID == request.TargetTask.WorkspaceID && request.TargetTask.ParentID == request.ActorTask.ID {
		return Decision{Allowed: true, Basis: BasisDirectParent}, nil
	}
	if a == nil || a.store == nil || !a.enabled() || request.ActorTask.WorkspaceID == "" {
		return Decision{Basis: BasisDenied}, nil
	}
	return a.authorizePrincipalGrant(ctx, request)
}

func (a *Authority) authorizePrincipalGrant(ctx context.Context, request Request) (Decision, error) {
	principal, err := a.store.GetActiveWorkspaceAgentPrincipalForTask(ctx, request.ActorTask.WorkspaceID, request.ActorTask.ID)
	if err != nil {
		return Decision{Basis: BasisDenied}, err
	}
	if principal == nil || principal.RevokedAt != nil || principal.PluginInstallationID == "" || principal.LogicalKey == "" {
		return Decision{Basis: BasisDenied}, nil
	}
	if principal.BackingSessionID == "" || request.ActorSessionID != principal.BackingSessionID {
		return Decision{Basis: BasisDenied}, nil
	}
	grants, err := a.store.ListActiveWorkspaceAgentPrincipalGrants(ctx, principal.ID, request.ActorTask.WorkspaceID)
	if err != nil {
		return Decision{Basis: BasisDenied}, err
	}
	if len(grants) == 0 {
		return Decision{Basis: BasisDenied}, nil
	}

	grant, reason := matchingGrant(grants, request)
	if reason != "" {
		return a.auditDenied(ctx, request, principal.ID, reason)
	}
	if request.ActorTask.ArchivedAt != nil {
		return a.auditDenied(ctx, request, principal.ID, "archived_actor")
	}

	event := &models.CoordinatorAuditEvent{
		ID:             uuid.NewString(),
		PrincipalID:    principal.ID,
		ActorTaskID:    request.ActorTask.ID,
		ActorSessionID: request.ActorSessionID,
		TargetTaskID:   request.TargetTask.ID,
		WorkspaceID:    request.ActorTask.WorkspaceID,
		Action:         request.Action,
		Capability:     string(request.Capability),
		Decision:       "allowed",
		GrantID:        grant.ID,
		Result:         "pending",
		Detail:         "grant",
	}
	if err := a.store.CreateCoordinatorAuditEvent(ctx, event); err != nil {
		return Decision{Basis: BasisDenied}, err
	}
	return Decision{Allowed: true, Basis: BasisGrant, GrantID: grant.ID, AuditID: event.ID}, nil
}

func (a *Authority) Finish(ctx context.Context, decision Decision, operationErr error) error {
	if a == nil || a.store == nil || decision.AuditID == "" {
		return nil
	}
	result := "ok"
	detail := ""
	if operationErr != nil {
		result = "error"
		detail = "operation_error"
	}
	return a.store.FinishCoordinatorAuditEvent(ctx, decision.AuditID, result, detail)
}

func (a *Authority) auditDenied(ctx context.Context, request Request, principalID, reason string) (Decision, error) {
	event := &models.CoordinatorAuditEvent{
		ID:             uuid.NewString(),
		PrincipalID:    principalID,
		ActorTaskID:    request.ActorTask.ID,
		ActorSessionID: request.ActorSessionID,
		TargetTaskID:   request.TargetTask.ID,
		WorkspaceID:    request.ActorTask.WorkspaceID,
		Action:         request.Action,
		Capability:     string(request.Capability),
		Decision:       "denied",
		DenyReason:     reason,
		Result:         "ok",
		Detail:         reason,
	}
	if err := a.store.CreateCoordinatorAuditEvent(ctx, event); err != nil {
		return Decision{Basis: BasisDenied}, err
	}
	return Decision{Basis: BasisDenied, AuditID: event.ID}, nil
}

func matchingGrant(grants []*models.CoordinatorGrant, request Request) (*models.CoordinatorGrant, string) {
	if request.ActorTask.WorkspaceID != request.TargetTask.WorkspaceID {
		return nil, "cross_workspace"
	}
	for _, grant := range grants {
		if grant.WorkspaceID != request.ActorTask.WorkspaceID {
			continue
		}
		if !scopeMatches(grant, request.TargetTask) {
			continue
		}
		if hasCapability(grant.Capabilities, request.Capability) {
			return grant, ""
		}
	}
	return nil, "scope_or_capability"
}

func scopeMatches(grant *models.CoordinatorGrant, target *models.Task) bool {
	switch grant.ScopeKind {
	case ScopeWorkspace:
		return grant.ScopeID == target.WorkspaceID
	case ScopeWorkflow:
		return grant.ScopeID != "" && grant.ScopeID == target.WorkflowID
	default:
		return false
	}
}

func hasCapability(encoded string, wanted Capability) bool {
	for _, capability := range strings.Split(encoded, ",") {
		if Capability(strings.TrimSpace(capability)) == wanted {
			return true
		}
	}
	return false
}
