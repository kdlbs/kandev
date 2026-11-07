package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/template"
)

// PreparePromptCall creates the native call owner before durable selection only
// when ordered tiers are enabled. Legacy callers keep the original preparation.
func (s *Service) PreparePromptCall(ctx context.Context, utilityID string, tmplCtx *template.Context, defaults *DefaultUtilitySettings, sessionless bool) (*PromptRequest, error) {
	ownerResolver, ok := s.executionResolver.(interface{ NativeRouteOwnersEnabled() bool })
	if !ok || !ownerResolver.NativeRouteOwnersEnabled() || s.profileResolver == nil {
		return s.PreparePromptRequest(ctx, utilityID, tmplCtx, defaults, sessionless)
	}
	sessionID := ""
	if tmplCtx != nil {
		sessionID = tmplCtx.SessionID
	}
	return s.preparePromptRequest(ctx, utilityID, tmplCtx, defaults, sessionless, &sessionID)
}

func (s *Service) prepareProfilePrompt(ctx context.Context, utilityID, prompt string, profile *agentsettingsmodels.AgentProfile, callSessionID *string) (request *PromptRequest, err error) {
	request = &PromptRequest{UtilityID: utilityID, ResolvedPrompt: prompt, AgentCLI: profile.AgentID, Model: profile.Model, AgentProfileID: profile.ID, ExecutionProfileID: profile.ID}
	var call *models.UtilityAgentCall
	if callSessionID != nil && profile.AgentID != "" {
		call, err = s.CreateCall(ctx, utilityID, *callSessionID, prompt, profile.Model, profile.ID)
		if err != nil {
			return nil, err
		}
		request.CallID = call.ID
	}
	defer func() {
		if err != nil && call != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			err = errors.Join(err, s.FailCall(cleanupCtx, call.ID, err.Error(), 0))
		}
	}()
	if err = s.resolvePreparedExecution(ctx, request, profile); err != nil {
		return nil, err
	}
	if call != nil {
		call.Model = request.Model
		call.ExecutionProfileID = request.ExecutionProfileID
		if err = s.repo.UpdateCall(ctx, call); err != nil {
			return nil, err
		}
	}
	return request, nil
}

func (s *Service) resolvePreparedExecution(ctx context.Context, request *PromptRequest, profile *agentsettingsmodels.AgentProfile) error {
	if s.executionResolver == nil {
		return nil
	}
	var resolved *agentsettingsmodels.AgentProfile
	var concreteID string
	var err error
	if resolver, ok := s.executionResolver.(ExecutionDetailsResolver); ok {
		request.RouteSessionID = promptRouteID(request)
		var details agentruntime.ProfileExecution
		details, err = resolver.ResolveExecutionDetails(ctx, request.RouteSessionID, profile.ID)
		resolved, concreteID = details.Profile, details.ExecutionProfileID
		if details.RouteSessionID != "" {
			request.RouteSessionID = details.RouteSessionID
		}
		request.RouteGeneration = details.Generation
	} else if resolver, ok := s.executionResolver.(SessionExecutionProfileResolver); ok {
		request.RouteSessionID = promptRouteID(request)
		resolved, concreteID, err = resolver.ResolveExecutionForSession(ctx, request.RouteSessionID, profile.ID)
	} else {
		resolved, concreteID, err = s.executionResolver.ResolveExecution(ctx, profile.ID)
	}
	if err != nil {
		return err
	}
	if resolved != nil {
		request.AgentCLI, request.Model = resolved.AgentID, resolved.Model
	}
	if concreteID != "" {
		request.ExecutionProfileID = concreteID
	}
	return nil
}

func promptRouteID(request *PromptRequest) string {
	if request.CallID != "" {
		return "native:utility-call:" + request.CallID
	}
	return "utility:" + uuid.NewString()
}
