package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) RetryBuiltinSessionMCP(ctx context.Context, sessionID, requestID string, version int64) (*mcpconfig.BuiltinSessionView, error) {
	actor, err := mcpconfig.BuiltinHumanActor(ctx)
	if err != nil {
		return nil, err
	}
	task, session, err := s.loadBuiltinSessionOwner(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if s.builtinApplicationAuthorize == nil {
		return nil, mcpconfig.ErrBuiltinPolicyInvalid
	}
	if err := s.builtinApplicationAuthorize(ctx, task.WorkspaceID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	writer, ok := s.repo.(interface {
		RetryBuiltinMCPApplication(context.Context, string, string, string, int64, models.BuiltinMCPRetryTarget) (bool, error)
	})
	if !ok {
		return nil, mcpconfig.ErrBuiltinPolicyInvalid
	}
	profile, err := s.executor.PrepareTaskSessionMCPProfile(ctx, task.ID, session, false)
	if err != nil {
		return nil, err
	}
	if profile.BuiltinPackages == nil {
		return nil, mcpconfig.ErrBuiltinPolicyInvalid
	}
	target := models.BuiltinMCPRetryTarget{TaskRevision: profile.BuiltinPackages.TaskRevision, PolicyRevision: profile.BuiltinPackages.WorkspaceRevision, SelectionHash: mcpconfig.BuiltinSelectionHash(profile.BuiltinPackages.Packages)}
	accepted, err := writer.RetryBuiltinMCPApplication(ctx, sessionID, actor, requestID, version, target)
	if err != nil {
		return nil, err
	}
	if !accepted {
		return nil, mcpconfig.ErrBuiltinApplicationConflict
	}
	s.WakeBuiltinMCPApplications(task.ID)
	return s.ReadBuiltinSessionMCP(ctx, sessionID)
}
