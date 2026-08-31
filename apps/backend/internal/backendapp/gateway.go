package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	agentcontroller "github.com/kandev/kandev/internal/agent/controller"
	agenthandlers "github.com/kandev/kandev/internal/agent/handlers"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/scripts"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	notificationservice "github.com/kandev/kandev/internal/notifications/service"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	"github.com/kandev/kandev/internal/orchestrator"
	orchestratorhandlers "github.com/kandev/kandev/internal/orchestrator/handlers"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
	terminalrepo "github.com/kandev/kandev/internal/terminal/repository"
	terminalservice "github.com/kandev/kandev/internal/terminal/service"
	userservice "github.com/kandev/kandev/internal/user/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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
	referenceValidator entityrefs.SubmissionValidator,
	authSvc *auth.Service,
	dataDir string,
	lspMaxConnections ...int,
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
		gateway.SetLSPHandler(lifecycleMgr, userSvc, lspMaxConnections...)
		gateway.SetVscodeProxy(lifecycleMgr)
		gateway.SetPortProxy(lifecycleMgr)
		gateway.SetPortTunnel(lifecycleMgr)
	}

	orchestratorHandlers := orchestratorhandlers.NewHandlers(orchestratorSvc, log)
	orchestratorHandlers.RegisterHandlers(gateway.Dispatcher)

	// Register message queue handlers
	queueHandlers := orchestratorhandlers.NewQueueHandlers(
		orchestratorSvc.GetMessageQueue(),
		orchestratorSvc.GetEventBus(),
		log,
		orchestratorSvc,
		taskSvc,
		orchestratorSvc.SessionTaskID,
		referenceValidator,
	)
	queueHandlers.SetAttachmentClaimer(taskSvc)
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
				router.associateGitLab = func(ctx context.Context, workspaceID, sessionID, taskID, repositoryID, mrURL string) error {
					_, err := gitlabSvc.AssociateExistingMRByURLForSession(ctx, workspaceID, sessionID, taskID, repositoryID, mrURL)
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
					zap.String(branchFieldKey, newName),
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
	if taskRepo != nil && eventBus != nil {
		var loadPullRequests statussummary.PullRequestLoader
		if githubSvc != nil {
			reader := &githubTaskStatusSummaryPRReader{gh: githubSvc}
			loadPullRequests = func(ctx context.Context, taskID string) ([]statussummary.PullRequestInput, error) {
				byTask, err := reader.ListTaskStatusSummaryPullRequests(ctx, []string{taskID})
				if err != nil {
					return nil, err
				}
				return byTask[taskID], nil
			}
		}
		activityProvider, countQueuedPrompts := taskStatusRuntimeProviders(orchestratorSvc)
		projector := statussummary.NewProjector(statussummary.ProjectorConfig{
			Store:    taskRepo,
			EventBus: eventBus,
			LoadTaskActivity: func(ctx context.Context, taskID string) (*time.Time, error) {
				byTask, err := taskRepo.LoadTaskLastActivity(ctx, []string{taskID})
				if err != nil {
					return nil, err
				}
				activityAt, ok := byTask[taskID]
				if !ok {
					return nil, nil
				}
				return &activityAt, nil
			},
			LoadSessionObservations: func(ctx context.Context, taskID string) (statussummary.SessionObservationSnapshot, error) {
				return loadTaskSessionObservations(ctx, taskRepo, activityProvider, taskID)
			},
			LoadTaskLaunchError: func(ctx context.Context, taskID string) (statussummary.TaskLaunchErrorObservation, error) {
				return loadTaskLaunchErrorObservation(ctx, taskRepo, taskID)
			},
			LoadPendingActions: func(ctx context.Context, taskID string) (map[string]string, error) {
				return loadTaskPendingActions(ctx, taskRepo, taskID)
			},
			LoadGitObservations: func(ctx context.Context, taskID string) ([]statussummary.GitObservation, error) {
				return loadTaskGitObservations(ctx, taskRepo, taskID)
			},
			LoadPullRequests: loadPullRequests,
			ResolveWorkspace: func(ctx context.Context, taskID string) (string, error) {
				task, err := taskRepo.GetTask(ctx, taskID)
				if err != nil {
					return "", err
				}
				if task == nil {
					return "", fmt.Errorf("task %q not found", taskID)
				}
				return task.WorkspaceID, nil
			},
			CountQueuedPrompts: countQueuedPrompts,
			Logger:             log,
		})
		taskSvc.SetTaskStatusSummaryEventProjector(projector)
		if err := projector.Start(ctx); err != nil {
			log.Error("failed to start task status summary projector", zap.Error(err))
		}
	}
	gateways.RegisterUserNotifications(ctx, eventBus, gateway.Hub, log)
	gateways.RegisterOfficeNotifications(ctx, eventBus, gateway.Hub, taskWorkspaceResolver(taskRepo), log)
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
	//ws:global — payload carries only session_id + poll mode (no content);
	// scoping it would need a session→workspace resolve per transition.
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

	// taskRepo is a typed pointer that may be nil here, and a nil pointer in a
	// non-nil interface would defeat the service's own nil check when it
	// resolves a notification's owning workspace.
	var notificationTasks notificationservice.TaskContextReader
	if taskRepo != nil {
		notificationTasks = taskRepo
	}
	notificationSvc := notificationservice.NewService(
		notificationRepo, notificationTasks, gateway.Hub, log, notificationAuthEnforced(authSvc))
	notificationCtrl := notificationcontroller.NewController(notificationSvc)
	if eventBus != nil {
		_, err = eventBus.Subscribe(events.TurnCompleted, func(ctx context.Context, event *bus.Event) error {
			data, ok := event.Data.(map[string]interface{})
			if !ok {
				return nil
			}
			taskID, _ := data["task_id"].(string)
			sessionID, _ := data["session_id"].(string)
			turnID, _ := data["id"].(string)
			if isAbandonedTurnCompletion(data) {
				return nil
			}
			notificationSvc.HandleTaskTurnFinished(ctx, taskID, sessionID, turnID)
			return nil
		})
		if err != nil {
			log.Error("Failed to subscribe to turn notifications", zap.Error(err))
		}

		_, err = eventBus.Subscribe(events.MessageAdded, func(ctx context.Context, event *bus.Event) error {
			data, ok := event.Data.(map[string]interface{})
			if !ok {
				return nil
			}
			taskID, sessionID, pendingID, eligible := clarificationNotificationOccurrence(data)
			if !eligible {
				return nil
			}
			notificationSvc.HandleClarificationRequested(ctx, taskID, sessionID, pendingID)
			return nil
		})
		if err != nil {
			log.Error("Failed to subscribe to clarification notifications", zap.Error(err))
		}

		_, err = eventBus.Subscribe(events.OfficeInboxItem, func(ctx context.Context, event *bus.Event) error {
			data, ok := event.Data.(map[string]interface{})
			if !ok {
				return nil
			}
			itemType, _ := data["type"].(string)
			title, _ := data["title"].(string)
			workspaceID, _ := data["workspace_id"].(string)
			notificationSvc.HandleInboxItem(ctx, workspaceID, itemType, title)
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

type taskStatusActivityProvider interface {
	ForegroundActivity(sessionID string) v1.ForegroundActivity
	ActiveSubagentCount(sessionID string) int
}

func taskStatusRuntimeProviders(
	orchestratorSvc *orchestrator.Service,
) (taskStatusActivityProvider, func(context.Context, string) (int, error)) {
	if orchestratorSvc == nil {
		return nil, nil
	}
	queue := orchestratorSvc.GetMessageQueue()
	if queue == nil {
		return orchestratorSvc, nil
	}
	return orchestratorSvc, queue.CountPendingByTask
}

func loadTaskSessionObservations(
	ctx context.Context,
	taskRepo *sqliterepo.Repository,
	activityProvider taskStatusActivityProvider,
	taskID string,
) (statussummary.SessionObservationSnapshot, error) {
	sessions, err := taskRepo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return statussummary.SessionObservationSnapshot{}, err
	}
	snapshot := statussummary.SessionObservationSnapshot{
		Sessions:         make([]statussummary.RebuildSession, 0, len(sessions)),
		ActivityObserved: activityProvider != nil,
		ErrorsObserved:   true,
	}
	for _, session := range sessions {
		if session == nil || session.ID == "" {
			continue
		}
		input := statussummary.RebuildSession{
			ID:        session.ID,
			State:     string(session.State),
			IsPrimary: session.IsPrimary,
		}
		if activityProvider != nil {
			activity := activityProvider.ForegroundActivity(session.ID)
			if session.State == models.TaskSessionStateRunning || activity == v1.ForegroundActivityBackground {
				input.ForegroundActivity = string(activity)
			}
			input.ActiveSubagentCount = maxNonNegative(activityProvider.ActiveSubagentCount(session.ID))
		}
		if lastError, ok := models.LoadLastAgentError(session.Metadata); ok && !lastError.IsDismissed() {
			input.ActiveError = &statussummary.ActiveErrorSummary{
				SessionID:        session.ID,
				TaskRepositoryID: lastError.TaskRepositoryID,
				Stamp:            lastError.Stamp(),
				OccurredAt:       lastError.OccurredAt,
				Preview:          lastError.Message,
				Category:         lastError.Code,
				RecoveryActions:  lastError.RecoveryActions,
			}
		}
		snapshot.Sessions = append(snapshot.Sessions, input)
	}
	return snapshot, nil
}

func loadTaskLaunchErrorObservation(
	ctx context.Context,
	taskRepo *sqliterepo.Repository,
	taskID string,
) (statussummary.TaskLaunchErrorObservation, error) {
	task, err := taskRepo.GetTask(ctx, taskID)
	if err != nil {
		return statussummary.TaskLaunchErrorObservation{}, err
	}
	if task == nil {
		return statussummary.TaskLaunchErrorObservation{}, fmt.Errorf("task %q not found", taskID)
	}
	if _, present := task.Metadata[models.MetaKeyLastLaunchError]; !present {
		return statussummary.TaskLaunchErrorObservation{Observed: true}, nil
	}
	errorValue, ok := models.LoadTaskLaunchError(task.Metadata)
	if !ok {
		return statussummary.TaskLaunchErrorObservation{}, nil
	}
	return statussummary.TaskLaunchErrorObservation{
		Observed: true,
		Error: &statussummary.ActiveErrorSummary{
			TaskRepositoryID: errorValue.TaskRepositoryID,
			Stamp:            errorValue.Stamp(),
			OccurredAt:       errorValue.OccurredAt,
			Preview:          errorValue.Message,
			Category:         errorValue.Code,
			RecoveryActions:  errorValue.RecoveryActions,
		},
	}, nil
}

func loadTaskPendingActions(
	ctx context.Context,
	taskRepo *sqliterepo.Repository,
	taskID string,
) (map[string]string, error) {
	sessions, err := taskRepo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	// Session state and pending actions are separate snapshots. A concurrent
	// transition can briefly omit an owner; the next lifecycle/message event
	// rebuilds the summary from fresh snapshots.
	sessionIDs := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if session == nil || (session.State != models.TaskSessionStateRunning &&
			session.State != models.TaskSessionStateWaitingForInput) {
			continue
		}
		sessionIDs = append(sessionIDs, session.ID)
	}
	if len(sessionIDs) == 0 {
		return map[string]string{}, nil
	}
	actions, err := taskRepo.GetPendingActionsBySessionIDs(ctx, sessionIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(actions))
	for sessionID, action := range actions {
		result[sessionID] = string(action)
	}
	return result, nil
}

func loadTaskGitObservations(
	ctx context.Context,
	taskRepo *sqliterepo.Repository,
	taskID string,
) ([]statussummary.GitObservation, error) {
	sessions, err := taskRepo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	environmentIDs := make([]string, 0, len(sessions)+1)
	seenEnvironmentIDs := make(map[string]struct{}, len(sessions)+1)
	for _, session := range sessions {
		if session == nil || session.TaskEnvironmentID == "" {
			continue
		}
		if _, seen := seenEnvironmentIDs[session.TaskEnvironmentID]; seen {
			continue
		}
		seenEnvironmentIDs[session.TaskEnvironmentID] = struct{}{}
		environmentIDs = append(environmentIDs, session.TaskEnvironmentID)
	}
	if environment, err := taskRepo.GetTaskEnvironmentByTaskID(ctx, taskID); err != nil {
		return nil, err
	} else if environment != nil && environment.ID != "" {
		if _, seen := seenEnvironmentIDs[environment.ID]; !seen {
			seenEnvironmentIDs[environment.ID] = struct{}{}
			environmentIDs = append(environmentIDs, environment.ID)
		}
	}
	snapshots, err := taskRepo.GetLatestGitStatusSnapshotsByTaskEnvironmentIDs(ctx, environmentIDs)
	if err != nil {
		return nil, err
	}
	observations := make([]statussummary.GitObservation, 0, len(snapshots))
	for _, snapshot := range snapshots {
		observation, ok := taskGitObservation(nil, snapshot)
		if ok {
			observations = append(observations, observation)
		}
	}
	return observations, nil
}

func taskGitObservation(
	session *models.TaskSession,
	snapshot *models.GitSnapshot,
) (statussummary.GitObservation, bool) {
	if snapshot == nil {
		return statussummary.GitObservation{}, false
	}
	repository := statussummary.RootRepositoryKey
	if session != nil {
		repository = session.ID
	}
	if name, ok := snapshot.Metadata["repository_name"].(string); ok && name != "" {
		repository = name
	}
	comparisonStatus, _ := snapshot.Metadata["comparison_status"].(string)
	return statussummary.GitObservation{
		Repository: repository,
		Summary: statussummary.GitSummary{
			Additions:             nonNegativeMetadataInt(snapshot.Metadata, "branch_additions"),
			Deletions:             nonNegativeMetadataInt(snapshot.Metadata, "branch_deletions"),
			ChangedFiles:          len(snapshot.Files),
			Ahead:                 maxNonNegative(snapshot.Ahead),
			Behind:                maxNonNegative(snapshot.Behind),
			ComparisonUnavailable: comparisonStatus == "unavailable",
		},
	}, true
}

func nonNegativeMetadataInt(metadata map[string]interface{}, key string) int {
	if metadata == nil {
		return 0
	}
	switch value := metadata[key].(type) {
	case int:
		return maxNonNegative(value)
	case int64:
		return maxNonNegative(int(value))
	case float64:
		return maxNonNegative(int(value))
	case json.Number:
		parsed, err := value.Int64()
		if err == nil {
			return maxNonNegative(int(parsed))
		}
	}
	return 0
}

func maxNonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func isAbandonedTurnCompletion(data map[string]interface{}) bool {
	startedAt, started := data["started_at"].(time.Time)
	completedAt, completed := data["completed_at"].(*time.Time)
	return started && completed && completedAt != nil && startedAt.Equal(*completedAt)
}

func clarificationNotificationOccurrence(data map[string]interface{}) (taskID, sessionID, pendingID string, ok bool) {
	if data["type"] != "clarification_request" || data["requests_input"] != true || data["author_type"] != "agent" {
		return "", "", "", false
	}
	metadata, metadataOK := data["metadata"].(map[string]interface{})
	pendingID, pendingOK := metadata["pending_id"].(string)
	if !metadataOK || !pendingOK || pendingID == "" {
		return "", "", "", false
	}
	taskID, taskOK := data["task_id"].(string)
	sessionID, sessionOK := data["session_id"].(string)
	if !taskOK || !sessionOK || taskID == "" || sessionID == "" {
		return "", "", "", false
	}
	return taskID, sessionID, pendingID, true
}

type createdChangeAssociationRouter struct {
	resolveRepositoryID func(context.Context, string, string) string
	resolveWorkspaceID  func(context.Context, string) (string, error)
	associateGitHub     func(context.Context, string, string, string, string, string, string) error
	associateGitLab     func(context.Context, string, string, string, string, string) error
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
		return r.associateGitLab(ctx, workspaceID, sessionID, taskID, repositoryID, changeURL)
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

// taskWorkspaceResolver adapts the task repository into the office
// broadcaster's TaskWorkspaceResolver so run events (task_id, no workspace_id)
// route to the owning workspace instead of every connected user. Returns a nil
// resolver when the repo is unavailable so the broadcaster fails closed.
func taskWorkspaceResolver(taskRepo *sqliterepo.Repository) gateways.TaskWorkspaceResolver {
	if taskRepo == nil {
		return nil
	}
	return func(ctx context.Context, taskID string) (string, error) {
		task, err := taskRepo.GetTask(ctx, taskID)
		if err != nil {
			return "", err
		}
		return task.WorkspaceID, nil
	}
}
