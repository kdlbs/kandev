package service

import (
	"context"
	"fmt"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
)

// CallInferenceContext derives relationships from the actual pending call.
// Runtime executors add the task and execution relationship they own later.
func (s *Service) CallInferenceContext(ctx context.Context, callID string) (context.Context, error) {
	call, err := s.repo.GetCallByID(ctx, callID)
	if err != nil {
		return ctx, err
	}
	if call == nil || call.ID != callID || call.Status != "pending" {
		return ctx, fmt.Errorf("utility inference call is not pending")
	}
	return agentruntime.WithInferenceIdentity(ctx, agentruntime.InferenceIdentity{
		CallID: call.ID, SessionID: call.SessionID, LogicalProfileID: call.AgentProfileID,
		ExecutionProfileID: call.ExecutionProfileID, OwnerKind: "utility",
	}), nil
}

// SetCallExecutionAttribution runs before a fallback invokes its actual profile.
// A failed write prevents inference through a candidate absent from history.
func (s *Service) SetCallExecutionAttribution(ctx context.Context, callID, profileID, model string) error {
	call, err := s.repo.GetCallByID(ctx, callID)
	if err != nil {
		return err
	}
	if call == nil || call.Status != "pending" {
		return fmt.Errorf("utility inference call is not pending")
	}
	call.ExecutionProfileID = profileID
	if model != "" {
		call.Model = model
	}
	return s.repo.UpdateCall(ctx, call)
}

// ReleaseNativeRoute removes only an unused early capacity alias.
func (s *Service) ReleaseNativeRoute(id string) {
	if release, ok := s.executionResolver.(interface{ ReleaseNativeRoute(string) }); ok {
		release.ReleaseNativeRoute(id)
	}
}
