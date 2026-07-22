package backendapp

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	agentcontroller "github.com/kandev/kandev/internal/agent/controller"
	agenthandlers "github.com/kandev/kandev/internal/agent/handlers"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/scripts"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	lspinstaller "github.com/kandev/kandev/internal/lsp/installer"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	notificationservice "github.com/kandev/kandev/internal/notifications/service"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	"github.com/kandev/kandev/internal/orchestrator"
	orchestratorhandlers "github.com/kandev/kandev/internal/orchestrator/handlers"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	terminalrepo "github.com/kandev/kandev/internal/terminal/repository"
	terminalservice "github.com/kandev/kandev/internal/terminal/service"
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
	terminalRepo *terminalrepo.Repository,
	githubSvc *github.Service,
	gitlabSvc *gitlab.Service,
	dataDir string,
) (*gateways.Gateway, *notificationservice.Service, *notificationcontroller.Controller, *terminalservice.Service, error) {
	gateway, err := gateways.Provide(log)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// Terminal service — DB-backed first-class user terminals. Backend is
	// nil-safe when no interactive runner is wired (remote-executor mode).
	var terminalSvc *terminalservice.Service
	if terminalRepo != nil {
		var ptyBackend terminalservice.PTYBackend
		if lifecycleMgr != nil {
			ptyBackend = terminalservice.NewInteractiveRunnerBackend(lifecycleMgr.GetInteractiveRunner())
		}
		terminalSvc = terminalservice.New(terminalRepo, ptyBackend, log)
		if taskRepo != nil {
			terminalSvc.SetTaskEnvironmentReader(taskRepo)
		}
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
	queueHandlers := orchestratorhandlers.NewQueueHandlers(orchestratorSvc.GetMessageQueue(), orchestratorSvc.GetEventBus(), log, orchestratorSvc)
	queueHandlers.RegisterHandlers(gateway.Dispatcher)

	if lifecycleMgr != nil && agentRegistry != nil {
		agentCtrl := agentcontroller.NewController(lifecycleMgr, agentRegistry)
		agentHandlers := agenthandlers.NewHandlers(agentCtrl, log)
		agentHandlers.RegisterHandlers(gateway.Dispatcher)

		workspaceFileHandlers := agenthandlers.NewWorkspaceFileHandlers(lifecycleMgr, log)
		workspaceFileHandlers.RegisterHandlers(gateway.Dispatcher)

		shellHandlers := agenthandlers.NewShellHandlers(lifecycleMgr, scriptSvc, log)
		if terminalSvc != nil {
			shellHandlers.SetTerminalService(terminalSvc)
		}
		shellHandlers.RegisterHandlers(gateway.Dispatcher)

		gitHandlers := agenthandlers.NewGitHandlers(lifecycleMgr, &sessionReaderAdapter{repo: taskRepo, logger: log}, log)
		if githubSvc != nil || gitlabSvc != nil {
			router := createdChangeAssociationRouter{
				resolveRepositoryID: func(ctx context.Context, taskID, repo string) string {
					return resolveRepositoryIDForSubpath(ctx, taskRepo, taskID, repo, log)
				},
				resolveWorkspaceID: func(ctx context.Context, taskID string) (string, error) {
					if taskRepo == nil {
						return "", fmt.Errorf("task repository unavailable")
					}
					task, err := taskRepo.GetTask(ctx, taskID)
					if err != nil || task == nil || task.WorkspaceID == "" {
						return "", fmt.Errorf("task workspace unavailable")
					}
					return task.WorkspaceID, nil
				},
			}
			if githubSvc != nil {
				router.associateGitHub = func(
					ctx context.Context,
					workspaceID, sessionID, taskID, repositoryID, prURL, branch string,
				) error {
					return githubSvc.AssociatePRByURLForWorkspace(
						ctx, workspaceID, github.DefaultUserID,
						sessionID, taskID, repositoryID, prURL, branch,
					)
				}
			}
			if gitlabSvc != nil {
				router.associateGitLab = func(ctx context.Context, workspaceID, taskID, repositoryID, mrURL string) error {
					_, err := gitlabSvc.AssociateExistingMRByURL(ctx, workspaceID, taskID, repositoryID, mrURL)
					return err
				}
			}
			gitHandlers.SetOnPRCreated(func(ctx context.Context, sessionID, taskID, provider, prURL, branch, repo string) {
				if err := router.associate(ctx, sessionID, taskID, provider, prURL, branch, repo); err != nil {
					// Provider errors may contain request URLs or credentials.
					// Keep the log generic; retrying creation re-runs association.
					log.Warn("failed to associate created change request", zap.String("task_id", taskID))
				}
			})
		}
		gitHandlers.SetOnBranchRenamed(func(ctx context.Context, sessionID, newName, repo string) {
			repositoryID := resolveRepositoryIDForSessionSubpath(ctx, taskRepo, sessionID, repo, log)
			if repositoryID == "" {
				return
			}
			if err := taskRepo.UpdateTaskSessionWorktreeBranchByRepository(ctx, sessionID, repositoryID, newName); err != nil {
				log.Warn("failed to update renamed branch snapshot",
					zap.String("session_id", sessionID),
					zap.String("repository_id", repositoryID),
					zap.String("branch", newName),
					zap.Error(err))
			}
		})
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
	gateways.RegisterOfficeNotifications(ctx, eventBus, gateway.Hub, log)
	gateways.RegisterRunNotifications(ctx, eventBus, gateway.Hub, log)

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

		_, err = eventBus.Subscribe(events.OfficeInboxItem, func(ctx context.Context, event *bus.Event) error {
			data, ok := event.Data.(map[string]interface{})
			if !ok {
				return nil
			}
			itemType, _ := data["type"].(string)
			title, _ := data["title"].(string)
			notificationSvc.HandleInboxItem(ctx, itemType, title)
			return nil
		})
		if err != nil {
			log.Error("Failed to subscribe to office inbox notifications", zap.Error(err))
		}
	}

	// Wire the task-lifecycle cascade: terminals are PTYs and DB rows;
	// destroying the task should destroy them too. Mirrors the GitHub PR
	// watch / handoff cascade pattern (see CLAUDE.md "Task lifecycle
	// events").
	if terminalSvc != nil && eventBus != nil {
		subscribeTerminalCleanup(ctx, eventBus, terminalSvc, log)
	}

	return gateway, notificationSvc, notificationCtrl, terminalSvc, nil
}

type createdChangeAssociationRouter struct {
	resolveRepositoryID func(context.Context, string, string) string
	resolveWorkspaceID  func(context.Context, string) (string, error)
	associateGitHub     func(context.Context, string, string, string, string, string, string) error
	associateGitLab     func(context.Context, string, string, string, string) error
}

func (r createdChangeAssociationRouter) associate(
	ctx context.Context,
	sessionID, taskID, provider, changeURL, branch, repo string,
) error {
	repositoryID := ""
	if r.resolveRepositoryID != nil {
		repositoryID = r.resolveRepositoryID(ctx, taskID, repo)
	}
	switch provider {
	case "gitlab":
		if r.associateGitLab == nil {
			return nil
		}
		if repositoryID == "" {
			return fmt.Errorf("GitLab repository unavailable")
		}
		if r.resolveWorkspaceID == nil {
			return fmt.Errorf("GitLab workspace resolver unavailable")
		}
		workspaceID, err := r.resolveWorkspaceID(ctx, taskID)
		if err != nil {
			return err
		}
		return r.associateGitLab(ctx, workspaceID, taskID, repositoryID, changeURL)
	case "github", "":
		if r.associateGitHub == nil {
			return nil
		}
		if r.resolveWorkspaceID == nil {
			return fmt.Errorf("GitHub workspace resolver unavailable")
		}
		workspaceID, err := r.resolveWorkspaceID(ctx, taskID)
		if err != nil {
			return err
		}
		return r.associateGitHub(ctx, workspaceID, sessionID, taskID, repositoryID, changeURL, branch)
	}
	return nil
}

// subscribeTerminalCleanup wires the terminal service to task.deleted /
// task.archived (where archived_at is set). It cleans up PTYs and DB rows
// for the task.
func subscribeTerminalCleanup(ctx context.Context, eventBus bus.EventBus, svc *terminalservice.Service, log *logger.Logger) {
	handler := func(eventCtx context.Context, event *bus.Event) error {
		if event == nil {
			return nil
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			return nil
		}
		taskID, _ := data["task_id"].(string)
		if taskID == "" {
			return nil
		}
		archivedAt, _ := data["archived_at"].(string)
		if event.Type == events.TaskUpdated && archivedAt == "" {
			// Plain update — only react on archive transitions.
			return nil
		}
		log.Info("terminal cleanup on task event started",
			zap.String("task_id", taskID),
			zap.String("event", event.Type),
			zap.Bool("archived", archivedAt != ""))
		deleted, err := svc.CleanupTask(eventCtx, taskID)
		if err != nil {
			log.Warn("terminal cleanup on task event",
				zap.String("task_id", taskID),
				zap.String("event", event.Type),
				zap.Error(err))
			return nil
		}
		log.Info("terminal cleanup on task event completed",
			zap.String("task_id", taskID),
			zap.String("event", event.Type),
			zap.Int("deleted_terminal_rows", deleted))
		return nil
	}
	if _, err := eventBus.Subscribe(events.TaskDeleted, handler); err != nil {
		log.Error("subscribe terminal cleanup (deleted)", zap.Error(err))
	}
	if _, err := eventBus.Subscribe(events.TaskUpdated, handler); err != nil {
		log.Error("subscribe terminal cleanup (updated/archived)", zap.Error(err))
	}
}
