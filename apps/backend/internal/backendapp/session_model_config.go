package backendapp

import (
	"context"
	"fmt"

	settingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	settingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// sessionModelConfigResolver previews a candidate model's provider options with
// the session profile's saved launch context. It does not mutate the session.
type sessionModelConfigResolver struct {
	sessions *taskservice.Service
	settings *settingscontroller.Controller
}

func (r *sessionModelConfigResolver) ResolveSessionModelConfig(
	ctx context.Context,
	sessionID string,
	request taskhandlers.SessionModelConfigRequest,
) (*settingsdto.AgentModelConfigResponse, error) {
	session, err := r.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	agentName := models.StringFromAny(session.AgentProfileSnapshot["agent_name"])
	if agentName == "" {
		return nil, fmt.Errorf("session agent identity is unavailable")
	}
	response, err := r.settings.ResolveAgentModelConfig(ctx, agentName, settingsdto.ResolveAgentModelConfigRequest{
		Model:              request.Model,
		Mode:               request.Mode,
		ConfigOptions:      request.ConfigOptions,
		ProfileID:          session.AgentProfileID,
		AuthorizationScope: "session:" + sessionID,
	})
	if err == nil {
		return response, nil
	}
	// Dynamic and role profiles cannot use a concrete-profile probe. Retry
	// with the agent-wide context so the editor still receives provider data.
	return r.settings.ResolveAgentModelConfig(ctx, agentName, settingsdto.ResolveAgentModelConfigRequest{
		Model:              request.Model,
		Mode:               request.Mode,
		ConfigOptions:      request.ConfigOptions,
		AuthorizationScope: "session:" + sessionID,
	})
}
