package main

import (
	"context"

	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/common/logger"
	userservice "github.com/kandev/kandev/internal/user/service"
)

func runInitialAgentSetup(
	ctx context.Context,
	userSvc *userservice.Service,
	agentSettingsController *agentsettingscontroller.Controller,
	log *logger.Logger,
) error {
	// Always run EnsureInitialAgentProfiles to detect newly added agents
	// This is idempotent - it only creates profiles for agents that don't have any
	if err := agentSettingsController.EnsureInitialAgentProfiles(ctx); err != nil {
		return err
	}

	// Mark initial setup as complete if not already
	settings, err := userSvc.GetUserSettings(ctx)
	if err != nil {
		return err
	}
	if settings.InitialSetupComplete {
		return nil
	}
	complete := true
	if _, err := userSvc.UpdateUserSettings(ctx, &userservice.UpdateUserSettingsRequest{
		InitialSetupComplete: &complete,
	}); err != nil {
		return err
	}
	log.Info("Initial agent setup complete")
	return nil
}
