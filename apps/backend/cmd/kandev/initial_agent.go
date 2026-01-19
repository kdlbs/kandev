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
	settings, err := userSvc.GetUserSettings(ctx)
	if err != nil {
		return err
	}
	if settings.InitialSetupComplete {
		return nil
	}
	if err := agentSettingsController.EnsureInitialAgentProfiles(ctx); err != nil {
		return err
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
