package main

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	agentcontroller "github.com/kandev/kandev/internal/agent/controller"
	agenthandlers "github.com/kandev/kandev/internal/agent/handlers"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/scripts"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/github"
	lspinstaller "github.com/kandev/kandev/internal/lsp/installer"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	notificationservice "github.com/kandev/kandev/internal/notifications/service"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	"github.com/kandev/kandev/internal/orchestrator"
	orchestratorhandlers "github.com/kandev/kandev/internal/orchestrator/handlers"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// scriptServiceAdapter adapts the task service to scripts.ScriptService.
type scriptServiceAdapter struct {
	taskSvc *taskservice.Service
}

func (a *scriptServiceAdapter) GetRepositoryScript(ctx context.Context, id string) (*scripts.RepositoryScript, error) {
	script, err := a.taskSvc.GetRepositoryScript(ctx, id)
	if err != nil {
		return nil, err
	}
	return &scripts.RepositoryScript{
		ID:      script.ID,
		Name:    script.Name,
		Command: script.Command,
	}, nil
}

// hubModeQuerierAdapter converts the hub's SessionMode enum to lifecycle's
// WorkspacePollMode (same string values) so lifecycle doesn't import websocket.
type hubModeQuerierAdapter struct {
	hub *gateways.Hub
}

func (a *hubModeQuerierAdapter) GetSessionMode(sessionID string) lifecycle.WorkspacePollMode {
	return lifecycle.WorkspacePollMode(a.hub.GetSessionMode(sessionID))
}

// sessionReaderAdapter implements agenthandlers.SessionReader using the task repository.
// This allows git handlers to look up session metadata (like base commit SHA) from the database.
type sessionReaderAdapter struct {
	repo   *sqliterepo.Repository
	logger *logger.Logger
}

func (a *sessionReaderAdapter) GetSessionBaseCommit(ctx context.Context, sessionID string) string {
	session, err := a.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		if a.logger != nil {
			a.logger.Warn("failed to load session for base commit lookup",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		return ""
	}
	if session == nil {
		return ""
	}
	return session.BaseCommitSHA
}

func (a *sessionReaderAdapter) GetSessionBaseBranch(ctx context.Context, sessionID string) string {
	session, err := a.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		if a.logger != nil {
			a.logger.Warn("failed to load session for base branch lookup",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
		return ""
	}
	if session == nil {
		return ""
	}
	return session.BaseBranch
}

func provideGateway(
	ctx context.Context,
	log *logger.Logger,
	eventBus bus.EventBus,
	taskSvc *taskservice.Service,
	userSvc *userservice.Service,
	orchestratorSvc *orchestrator.Service,
	lifecycleMgr *lifecycle.Manager,
	agentRegistry *registry.Registry,
	notificationRepo notificationstore.Repository,
	taskRepo *sqliterepo.Repository,
	githubSvc *github.Service,
	dataDir string,
) (*gateways.Gateway, *notificationservice.Service, *notificationcontroller.Controller, error) {
	gateway, err := gateways.Provide(log)
	if err != nil {
		return nil, nil, nil, err
	}

	// Enable dedicated terminal WebSocket for passthrough mode
	scriptSvc := &scriptServiceAdapter{taskSvc: taskSvc}
	if lifecycleMgr != nil {
		gateway.SetLifecycleManager(lifecycleMgr, userSvc, scriptSvc)
		gateway.SetLSPHandler(lifecycleMgr, userSvc, lspinstaller.NewRegistry(dataDir, log))
		gateway.SetVscodeProxy(lifecycleMgr)
		gateway.SetPortProxy(lifecycleMgr)
		gateway.SetPortTunnel(lifecycleMgr)
	}

	orchestratorHandlers := orchestratorhandlers.NewHandlers(orchestratorSvc, log)
	orchestratorHandlers.RegisterHandlers(gateway.Dispatcher)

	// Register message queue handlers
	queueHandlers := orchestratorhandlers.NewQueueHandlers(orchestratorSvc.GetMessageQueue(), orchestratorSvc.GetEventBus(), log)
	queueHandlers.RegisterHandlers(gateway.Dispatcher)

	if lifecycleMgr != nil && agentRegistry != nil {
		agentCtrl := agentcontroller.NewController(lifecycleMgr, agentRegistry)
		agentHandlers := agenthandlers.NewHandlers(agentCtrl, log)
		agentHandlers.RegisterHandlers(gateway.Dispatcher)

		workspaceFileHandlers := agenthandlers.NewWorkspaceFileHandlers(lifecycleMgr, log)
		workspaceFileHandlers.RegisterHandlers(gateway.Dispatcher)

		shellHandlers := agenthandlers.NewShellHandlers(lifecycleMgr, scriptSvc, log)
		shellHandlers.RegisterHandlers(gateway.Dispatcher)

		gitHandlers := agenthandlers.NewGitHandlers(lifecycleMgr, &sessionReaderAdapter{repo: taskRepo, logger: log}, log)
		if githubSvc != nil {
			gitHandlers.SetOnPRCreated(func(ctx context.Context, sessionID, taskID, prURL, branch string) {
				githubSvc.AssociatePRByURL(ctx, sessionID, taskID, prURL, branch)
			})
		}
		gitHandlers.SetOnGitOperationFailed(func(ctx context.Context, sessionID, taskID, operation, errorOutput string) {
			fixPrompt := fmt.Sprintf("The git %s command failed with the following error:\n\n```\n%s\n```\n\nPlease fix the issues reported above.", operation, errorOutput)
			if _, err := taskSvc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
				TaskSessionID: sessionID,
				TaskID:        taskID,
				Content:       fmt.Sprintf("Git %s failed", operation),
				AuthorType:    "agent",
				Type:          "error",
				Metadata: map[string]interface{}{
					"git_operation_error": true,
					"operation":           operation,
					"error_output":        errorOutput,
					"session_id":          sessionID,
					"task_id":             taskID,
					"variant":             "error",
					"actions": []map[string]interface{}{{
						"type": "ws_request", "label": "Fix", "icon": "sparkles",
						"tooltip": "Ask the agent to fix the git error",
						"test_id": "git-fix-button",
						"params": map[string]interface{}{
							"method":  "message.add",
							"payload": map[string]interface{}{"task_id": taskID, "session_id": sessionID, "content": fixPrompt},
						},
					}},
				},
			}); err != nil {
				log.Error("failed to create git operation error message",
					zap.String("session_id", sessionID),
					zap.String("operation", operation),
					zap.Error(err))
			}
		})
		gitHandlers.RegisterHandlers(gateway.Dispatcher)

		passthroughHandlers := agenthandlers.NewPassthroughHandlers(lifecycleMgr, log)
		passthroughHandlers.RegisterHandlers(gateway.Dispatcher)

		vscodeHandlers := agenthandlers.NewVscodeHandlers(lifecycleMgr, gateway.VscodeProxyHandler, log)
		vscodeHandlers.RegisterHandlers(gateway.Dispatcher)

		portHandlers := agenthandlers.NewPortHandlers(lifecycleMgr, gateway.TunnelManager, log)
		portHandlers.RegisterHandlers(gateway.Dispatcher)
	}

	go gateway.Hub.Run(ctx)
	gateways.RegisterTaskNotifications(ctx, eventBus, gateway.Hub, log)
	gateways.RegisterUserNotifications(ctx, eventBus, gateway.Hub, log)

	// Route session focus/subscription transitions from the hub into the
	// lifecycle manager so it can push poll-mode changes to agentctl.
	if lifecycleMgr != nil {
		gateway.Hub.AddSessionModeListener(func(sessionID string, mode gateways.SessionMode) {
			lifecycleMgr.HandleSessionMode(sessionID, lifecycle.WorkspacePollMode(mode))
		})
		// Expose hub's live mode state to lifecycle so it can query on
		// execution-ready and proactively push the right mode even when
		// gateway events fired before the execution was registered.
		lifecycleMgr.SetSessionModeQuerier(&hubModeQuerierAdapter{hub: gateway.Hub})
	}

	// Broadcast session poll mode transitions to all WS clients. Uses Broadcast
	// (not BroadcastToSession) because focus events fire before subscribe events
	// on page load, so BroadcastToSession misses the focused-but-not-yet-
	// subscribed client. Volume is low (debounced down-transitions) and clients
	// filter by session_id in the payload.
	gateway.Hub.AddSessionModeListener(func(sessionID string, mode gateways.SessionMode) {
		msg, err := ws.NewNotification(ws.ActionSessionPollModeChanged, map[string]interface{}{
			"session_id": sessionID,
			"poll_mode":  string(mode),
		})
		if err != nil {
			return
		}
		gateway.Hub.Broadcast(msg)
	})

	notificationSvc := notificationservice.NewService(notificationRepo, taskRepo, gateway.Hub, log)
	notificationCtrl := notificationcontroller.NewController(notificationSvc)
	if eventBus != nil {
		_, err = eventBus.Subscribe(events.TaskSessionStateChanged, func(ctx context.Context, event *bus.Event) error {
			data, ok := event.Data.(map[string]interface{})
			if !ok {
				return nil
			}
			taskID, _ := data["task_id"].(string)
			sessionID, _ := data["session_id"].(string)
			newState, _ := data["new_state"].(string)
			notificationSvc.HandleTaskSessionStateChanged(ctx, taskID, sessionID, newState)
			return nil
		})
		if err != nil {
			log.Error("Failed to subscribe to task session notifications", zap.Error(err))
		}
	}

	return gateway, notificationSvc, notificationCtrl, nil
}
