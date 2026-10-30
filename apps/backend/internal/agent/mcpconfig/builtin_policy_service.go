package mcpconfig

import (
	"context"
	"errors"
	"maps"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/mcp/origin"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/mcp/scope"
)

var (
	ErrBuiltinHumanRequired  = errors.New("built-in MCP changes require a human request")
	ErrBuiltinPolicyConflict = errors.New("built-in MCP policy revision conflict")
	ErrBuiltinPackageBlocked = errors.New("built-in MCP package is blocked by workspace policy")
)

type BuiltinPolicySnapshot struct {
	Policy      *BuiltinWorkspacePolicy `json:"policy"`
	Preferences *BuiltinTaskPreferences `json:"preferences,omitempty"`
}

type BuiltinPolicyConflictError struct {
	Current *BuiltinPolicySnapshot
}

func (e *BuiltinPolicyConflictError) Error() string { return ErrBuiltinPolicyConflict.Error() }
func (e *BuiltinPolicyConflictError) Unwrap() error { return ErrBuiltinPolicyConflict }

type BuiltinPolicyRepository interface {
	GetBuiltinPolicySnapshot(context.Context, string, string) (*BuiltinPolicySnapshot, error)
	PutBuiltinWorkspacePolicy(context.Context, *BuiltinWorkspacePolicy, int64) (*BuiltinWorkspacePolicy, error)
	PutBuiltinTaskPreferences(context.Context, *BuiltinTaskPreferences, int64, int64, mcpprofile.Surface) (*BuiltinPolicySnapshot, error)
}

type BuiltinPolicyAuthorizer func(context.Context, string, authz.Scope) error

type BuiltinTaskScope func(context.Context, string) (string, mcpprofile.Surface, error)

type BuiltinPolicyService struct {
	repo      BuiltinPolicyRepository
	authorize BuiltinPolicyAuthorizer
	taskScope BuiltinTaskScope
}

func NewBuiltinPolicyService(repo BuiltinPolicyRepository, authorize BuiltinPolicyAuthorizer, taskScope BuiltinTaskScope) *BuiltinPolicyService {
	return &BuiltinPolicyService{repo: repo, authorize: authorize, taskScope: taskScope}
}

func (s *BuiltinPolicyService) UpdateWorkspace(ctx context.Context, workspaceID string, expectedRevision int64, choices BuiltinPackageChoices) (*BuiltinWorkspacePolicy, error) {
	actor, err := builtinHumanActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspace(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	policy := DefaultBuiltinWorkspacePolicy(workspaceID)
	if choices != nil {
		policy.Packages = maps.Clone(choices)
	}
	policy.Revision, policy.UpdatedBy, policy.UpdatedAt = expectedRevision, actor, time.Now().UTC()
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return s.repo.PutBuiltinWorkspacePolicy(ctx, policy, expectedRevision)
}

func (s *BuiltinPolicyService) UpdateTask(ctx context.Context, taskID string, expectedRevision, expectedPolicyRevision int64, choices BuiltinPackageChoices) (*BuiltinPolicySnapshot, error) {
	actor, err := builtinHumanActor(ctx)
	if err != nil {
		return nil, err
	}
	if s.taskScope == nil || expectedPolicyRevision < 0 {
		return nil, ErrBuiltinPolicyInvalid
	}
	workspaceID, surface, err := s.taskScope(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspace(ctx, workspaceID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	preferences := &BuiltinTaskPreferences{
		WorkspaceID: workspaceID, TaskID: taskID, Version: 1, Revision: expectedRevision,
		Packages: maps.Clone(choices), UpdatedBy: actor, UpdatedAt: time.Now().UTC(),
	}
	if err := preferences.Validate(surface); err != nil {
		return nil, err
	}
	for id, value := range preferences.Packages {
		if value == BuiltinPreferenceInherit {
			delete(preferences.Packages, id)
		}
	}
	return s.repo.PutBuiltinTaskPreferences(ctx, preferences, expectedRevision, expectedPolicyRevision, surface)
}

func builtinHumanActor(ctx context.Context) (string, error) {
	identity, identified := authn.IdentityFromContext(ctx)
	_, agent := scope.PrincipalFromContext(ctx)
	_, execution := streams.MCPExecutionContextFromContext(ctx)
	if !identified || identity.UserID == "" || agent || execution || origin.IsTrustedExternalTransport(ctx) || origin.IsTrustedInternalCall(ctx) {
		return "", ErrBuiltinHumanRequired
	}
	return identity.UserID, nil
}

func (s *BuiltinPolicyService) authorizeWorkspace(ctx context.Context, workspaceID string, requested authz.Scope) error {
	if s.repo == nil || s.authorize == nil || workspaceID == "" {
		return ErrMCPWorkspaceAccess
	}
	return s.authorize(ctx, workspaceID, requested)
}
