package service

import (
	"context"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

type BuiltinCreationWriter interface {
	ValidateTaskCreation(context.Context, string, *mcpconfig.BuiltinCreationPreferences) error
	ApplyTaskCreation(context.Context, string, *mcpconfig.BuiltinCreationPreferences) error
}

func (s *Service) SetBuiltinCreationWriter(writer BuiltinCreationWriter) {
	s.builtinCreationWriter = writer
}

func (s *Service) validateBuiltinTaskCreation(ctx context.Context, req *CreateTaskRequest) error {
	if req.BuiltinMCPPreferences == nil {
		return nil
	}
	if s.builtinCreationWriter == nil {
		return mcpconfig.ErrBuiltinPolicyInvalid
	}
	return s.builtinCreationWriter.ValidateTaskCreation(ctx, req.WorkspaceID, req.BuiltinMCPPreferences)
}

func (s *Service) applyBuiltinTaskCreation(ctx context.Context, taskID string, req *CreateTaskRequest) error {
	if req.BuiltinMCPPreferences == nil {
		return nil
	}
	if s.builtinCreationWriter == nil {
		return mcpconfig.ErrBuiltinPolicyInvalid
	}
	return s.builtinCreationWriter.ApplyTaskCreation(ctx, taskID, req.BuiltinMCPPreferences)
}
