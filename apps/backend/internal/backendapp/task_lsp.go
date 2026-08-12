package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	agentctllsp "github.com/kandev/kandev/internal/agentctl/server/lsp"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	tasklsp "github.com/kandev/kandev/internal/lsp"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	usermodels "github.com/kandev/kandev/internal/user/models"
	userservice "github.com/kandev/kandev/internal/user/service"
	"go.uber.org/zap"
)

type taskLSPStatePublisher struct {
	events bus.EventBus
}

func (p taskLSPStatePublisher) PublishTaskLSP(ctx context.Context, snapshot tasklsp.LanguageSnapshot) error {
	if p.events == nil {
		return nil
	}
	return p.events.Publish(ctx, events.TaskLSPStateChanged, bus.NewEvent(
		events.TaskLSPStateChanged,
		"task-lsp-controller",
		snapshot,
	))
}

type taskLSPSettingsProvider struct {
	users taskLSPUserSettingsReader
	tasks taskLSPSettingsOwnerSource
}

type taskLSPUserSettingsReader interface {
	GetUserSettings(ctx context.Context) (*usermodels.UserSettings, error)
}

type taskLSPSettingsOwnerSource interface {
	GetTask(ctx context.Context, taskID string) (*models.Task, error)
	GetWorkspace(ctx context.Context, workspaceID string) (*models.Workspace, error)
}

func (p taskLSPSettingsProvider) TaskLSPSettings(
	ctx context.Context,
	taskID string,
) (tasklsp.TaskSettings, error) {
	settingsCtx, err := p.taskOwnerContext(ctx, taskID)
	if err != nil {
		return tasklsp.TaskSettings{}, err
	}
	settings, err := p.users.GetUserSettings(settingsCtx)
	if err != nil {
		return tasklsp.TaskSettings{}, err
	}
	if settings == nil {
		return tasklsp.TaskSettings{}, nil
	}
	configs := make(map[string]json.RawMessage, len(settings.LspServerConfigs))
	for language, value := range settings.LspServerConfigs {
		encoded, err := json.Marshal(value)
		if err != nil {
			return tasklsp.TaskSettings{}, fmt.Errorf("encode %s LSP configuration: %w", language, err)
		}
		configs[language] = encoded
	}
	return tasklsp.TaskSettings{
		AutoStartLanguages:   append([]string(nil), settings.LspAutoStartLanguages...),
		AutoInstallLanguages: append([]string(nil), settings.LspAutoInstallLanguages...),
		ServerConfigs:        configs,
	}, nil
}

func (p taskLSPSettingsProvider) taskOwnerContext(ctx context.Context, taskID string) (context.Context, error) {
	task, err := p.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil || task.WorkspaceID == "" {
		return nil, fmt.Errorf("resolve LSP settings owner for task %q: task workspace is unavailable", taskID)
	}
	workspace, err := p.tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if workspace == nil {
		return nil, fmt.Errorf("resolve LSP settings owner for task %q: workspace is unavailable", taskID)
	}
	identity := authn.Identity{Role: authn.RoleAdmin, Synthetic: true}
	if workspace.OwnerID != "" {
		identity = authn.Identity{UserID: workspace.OwnerID, Role: authn.RoleMember}
	}
	return authn.WithIdentity(ctx, identity), nil
}

type taskLSPRuntimeProvider struct {
	taskHosts taskLSPTaskHostRuntime
	tasks     taskLSPWorkspaceProvider
}

type taskLSPWorkspace struct {
	executorType   models.ExecutorType
	discoveryRoots []string
}

type taskLSPWorkspaceProvider interface {
	TaskLSPWorkspace(ctx context.Context, taskEnvironmentID string) (*taskLSPWorkspace, error)
}

type taskLSPTaskHostRuntime interface {
	EnsureTaskHost(ctx context.Context, taskEnvironmentID string) (tasklsp.TaskHost, error)
	ExistingTaskHost(ctx context.Context, taskEnvironmentID string) (tasklsp.TaskHost, bool, error)
	RecoverTaskHost(ctx context.Context, taskEnvironmentID string) (bool, error)
	CleanupTaskHost(ctx context.Context, taskEnvironmentID, reason string) error
}

func (p taskLSPRuntimeProvider) RecoverTaskHost(
	ctx context.Context,
	taskEnvironmentID string,
) (bool, error) {
	return p.taskHosts.RecoverTaskHost(ctx, taskEnvironmentID)
}

func (p taskLSPRuntimeProvider) EnsureTaskHost(
	ctx context.Context,
	taskEnvironmentID string,
) (tasklsp.TaskHost, error) {
	return p.taskHosts.EnsureTaskHost(ctx, taskEnvironmentID)
}

func (p taskLSPRuntimeProvider) ExistingTaskHost(
	ctx context.Context,
	taskEnvironmentID string,
) (tasklsp.TaskHost, bool, error) {
	return p.taskHosts.ExistingTaskHost(ctx, taskEnvironmentID)
}

func (p taskLSPRuntimeProvider) CleanupTaskHost(
	ctx context.Context,
	taskEnvironmentID, reason string,
) error {
	return p.taskHosts.CleanupTaskHost(ctx, taskEnvironmentID, reason)
}

func (p taskLSPRuntimeProvider) DiscoverTaskLanguages(
	ctx context.Context,
	taskEnvironmentID string,
) (*tasklsp.DiscoveryResult, error) {
	if p.tasks == nil {
		return nil, errors.New("task workspace provider is unavailable")
	}
	info, err := p.tasks.TaskLSPWorkspace(ctx, taskEnvironmentID)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, errors.New("task workspace is unavailable for language discovery")
	}
	if info.executorType == models.ExecutorTypeLocalDocker {
		host, ensureErr := p.taskHosts.EnsureTaskHost(ctx, taskEnvironmentID)
		if ensureErr != nil {
			return nil, ensureErr
		}
		if host == nil {
			return nil, errors.New("task host is unavailable for language discovery")
		}
		return host.DiscoverLSP(ctx)
	}
	result := agentctllsp.DiscoverLanguagesAtRoots(ctx, info.discoveryRoots)
	return &result, nil
}

func newTaskLSPController(
	tasks *taskservice.Service,
	store *sqliterepo.Repository,
	users *userservice.Service,
	taskHosts taskLSPTaskHostRuntime,
	eventBus bus.EventBus,
) *tasklsp.Controller {
	if tasks == nil || store == nil || users == nil || taskHosts == nil {
		return nil
	}
	return tasklsp.NewController(tasklsp.ControllerConfig{
		Tasks: tasks, Store: store, Settings: taskLSPSettingsProvider{users: users, tasks: tasks},
		Runtimes: taskLSPRuntimeProvider{
			taskHosts: taskHosts,
			tasks:     taskLSPWorkspaceAdapter{tasks: tasks},
		},
		Capacity:  tasklsp.NewCapacityFromEnv(),
		Publisher: taskLSPStatePublisher{events: eventBus},
	})
}

func configureTaskLSP(
	ctx context.Context,
	services *Services,
	orchestratorService *orchestrator.Service,
	eventBus bus.EventBus,
	log *logger.Logger,
	addCleanup func(func() error),
) {
	controller := services.TaskLSP
	services.Task.SetTaskLSPLifecycle(controller)
	orchestratorService.SetOnTaskStopCleanup(services.Task.StopTaskLSP)
	orchestratorService.SetOnTaskEnvironmentReady(func(readyCtx context.Context, taskID string) {
		if err := controller.ReconcileTask(readyCtx, taskID); err != nil && readyCtx.Err() == nil {
			log.Warn("Task LSP environment-ready reconciliation failed",
				zap.String("task_id", taskID), zap.Error(err))
		}
	})
	addCleanup(func() error {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return controller.Close(closeCtx)
	})
	subscribeTaskLSPSettings(controller, eventBus, log, addCleanup)
	go func() {
		if err := controller.StartReconciler(ctx); err != nil && ctx.Err() == nil {
			log.Warn("Task LSP startup reconciliation completed with errors", zap.Error(err))
		}
	}()
}

func subscribeTaskLSPSettings(
	controller *tasklsp.Controller,
	eventBus bus.EventBus,
	log *logger.Logger,
	addCleanup func(func() error),
) {
	if eventBus == nil {
		return
	}
	subscription, err := eventBus.Subscribe(
		events.UserSettingsUpdated,
		func(context.Context, *bus.Event) error {
			controller.NotifySettingsChanged()
			return nil
		},
	)
	if err != nil {
		log.Warn("Task LSP settings reconciliation subscription failed", zap.Error(err))
		return
	}
	addCleanup(subscription.Unsubscribe)
}
