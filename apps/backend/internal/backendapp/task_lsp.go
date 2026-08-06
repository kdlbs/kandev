package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	agentctllsp "github.com/kandev/kandev/internal/agentctl/server/lsp"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	tasklsp "github.com/kandev/kandev/internal/lsp"
	"github.com/kandev/kandev/internal/orchestrator"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
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
	users *userservice.Service
}

func (p taskLSPSettingsProvider) TaskLSPSettings(ctx context.Context) (tasklsp.TaskSettings, error) {
	settings, err := p.users.GetUserSettings(ctx)
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

type taskLSPRuntimeProvider struct {
	taskHosts taskLSPTaskHostRuntime
	tasks     *taskservice.Service
}

type taskLSPTaskHostRuntime interface {
	EnsureTaskHost(ctx context.Context, taskEnvironmentID string) (tasklsp.TaskHost, error)
	ExistingTaskHost(ctx context.Context, taskEnvironmentID string) (tasklsp.TaskHost, bool, error)
	CleanupTaskHost(ctx context.Context, taskEnvironmentID, reason string) error
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
	info, err := p.tasks.GetWorkspaceInfoForEnvironment(ctx, taskEnvironmentID)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, errors.New("task workspace is unavailable for language discovery")
	}
	result := agentctllsp.DiscoverLanguagesAtRoots(ctx, taskLSPDiscoveryRoots(info))
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
		Tasks: tasks, Store: store, Settings: taskLSPSettingsProvider{users: users},
		Runtimes:  taskLSPRuntimeProvider{taskHosts: taskHosts, tasks: tasks},
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
	orchestratorService.SetOnTaskStopCleanup(services.Task.CleanupTaskLSP)
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
