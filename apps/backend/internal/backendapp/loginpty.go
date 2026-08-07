package backendapp

import (
	"github.com/kandev/kandev/internal/agent/loginpty"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/quickterminal"
	quickterminalrepo "github.com/kandev/kandev/internal/quickterminal/repository"
)

func buildLoginPTYServices(
	log *logger.Logger,
	quickTerminalRepo *quickterminalrepo.Repository,
	taskAuthorizer quickterminal.WorkspaceAuthorizer,
	agentSettingsController *agentsettingscontroller.Controller,
	addCleanup func(func() error),
) (*loginpty.Manager, *quickterminal.Service) {
	loginMgr := loginpty.NewManager(log, func(_ string, _ int, _ error) {
		if agentSettingsController != nil {
			agentSettingsController.InvalidateDiscoveryCache()
		}
	})
	if addCleanup != nil {
		addCleanup(loginMgr.StopAll)
	}
	quickTerminalSvc := quickterminal.NewService(quickTerminalRepo, loginMgr, taskAuthorizer)
	loginMgr.SetSessionExitCallback(quickTerminalSvc.HandleSessionExit)
	return loginMgr, quickTerminalSvc
}
