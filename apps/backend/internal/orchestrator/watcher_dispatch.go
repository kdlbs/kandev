package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// Log field keys repeated across coordinator log calls. Hoisted to file
// constants so a typo would be a compile error rather than a silent log-
// search miss.
const (
	logFieldSource = "source"
	logFieldTaskID = "task_id"
)

// WatcherDispatchCoordinator owns the cross-integration pipeline that turns
// a freshly-observed external issue (Linear, Jira, future: GitHub issues,
// webhooks) into a Kandev task. It is the single seam where throttling,
// observability, retry, or fairness will land — integration-specific code
// stays in WatcherSource implementations.
//
// Pipeline:
//
//	Reserve → BuildTaskRequest → CreateIssueTask → AttachTaskID
//	       → (optional) StartTask
//
// On any failure between Reserve and a successful CreateIssueTask, Release
// is invoked so the dedup row does not strand the issue.
type WatcherDispatchCoordinator struct {
	taskCreator     IssueTaskCreator
	startTask       taskStarter
	shouldAutoStart func(ctx context.Context, workflowStepID string) bool
	logger          *logger.Logger
}

// taskStarter wraps Service.StartTask so the coordinator can be tested
// without spinning up the full orchestrator service.
type taskStarter interface {
	Start(ctx context.Context, taskID, workflowStepID string, params AutoStartParams) error
}

// AutoStartParams is the data a source contributes when the resulting task's
// workflow step has auto-start enabled.
type AutoStartParams struct {
	AgentProfileID    string
	ExecutorProfileID string
	Prompt            string
	WorkflowStepID    string
}

// WatcherSource encapsulates everything integration-specific about turning
// a freshly-observed external issue into a Kandev task. Each method receives
// the bus event payload as `any`; implementations type-assert at the top.
// A failed assertion is a programming error (the subscriber wired the wrong
// source) — implementations panic via the assertion's `ok` branch.
type WatcherSource interface {
	// Name returns a stable identifier ("linear", "jira", ...). Used for
	// metrics labels and log fields.
	Name() string

	// Reserve atomically claims the dedup slot for this event. Returns
	// (false, nil) when another concurrent reserver already won the race —
	// the coordinator treats that as "nothing to do".
	Reserve(ctx context.Context, evt any) (bool, error)

	// Release rolls back a reservation when downstream work fails. Best
	// effort; errors are logged but not surfaced.
	Release(ctx context.Context, evt any)

	// BuildTaskRequest translates the event into the shape the task creator
	// expects. Returning an error triggers Release.
	BuildTaskRequest(evt any) (*IssueTaskRequest, error)

	// AttachTaskID writes the freshly-created task id back onto the dedup
	// row so a future re-observation can short-circuit. Errors are logged
	// but do not stop the pipeline — matching existing behaviour.
	AttachTaskID(ctx context.Context, evt any, taskID string) error

	// AutoStartParams returns the parameters needed to kick the task off
	// when its workflow step is configured for auto-start.
	AutoStartParams(evt any) AutoStartParams
}

// Dispatch runs one event through the full pipeline. Safe to call from a
// goroutine; callers typically do so in the bus subscriber.
func (c *WatcherDispatchCoordinator) Dispatch(ctx context.Context, src WatcherSource, evt any) {
	reserved, err := src.Reserve(ctx, evt)
	if err != nil {
		c.logger.Error("watcher dispatch: reserve failed",
			zap.String(logFieldSource, src.Name()), zap.Error(err))
		return
	}
	if !reserved {
		c.logger.Debug("watcher dispatch: already reserved by concurrent handler",
			zap.String(logFieldSource, src.Name()))
		return
	}

	req, err := src.BuildTaskRequest(evt)
	if err != nil {
		c.logger.Error("watcher dispatch: build task request failed",
			zap.String(logFieldSource, src.Name()), zap.Error(err))
		src.Release(ctx, evt)
		return
	}

	task, err := c.taskCreator.CreateIssueTask(ctx, req)
	if err != nil {
		c.logger.Error("watcher dispatch: create issue task failed",
			zap.String(logFieldSource, src.Name()), zap.Error(err))
		src.Release(ctx, evt)
		return
	}

	if err := src.AttachTaskID(ctx, evt, task.ID); err != nil {
		c.logger.Error("watcher dispatch: attach task id failed",
			zap.String(logFieldSource, src.Name()),
			zap.String(logFieldTaskID, task.ID),
			zap.Error(err))
		// Do NOT release here — matches existing Linear/Jira behaviour:
		// attach is a best-effort step, the task is already created.
	}

	c.logger.Info("watcher dispatch: created issue task",
		zap.String(logFieldSource, src.Name()),
		zap.String(logFieldTaskID, task.ID))

	if !c.shouldAutoStart(ctx, req.WorkflowStepID) {
		return
	}

	params := src.AutoStartParams(evt)
	if err := c.startTask.Start(ctx, task.ID, req.WorkflowStepID, params); err != nil {
		c.logger.Error("watcher dispatch: auto-start failed",
			zap.String(logFieldSource, src.Name()),
			zap.String(logFieldTaskID, task.ID),
			zap.Error(err))
		return
	}
	c.logger.Info("watcher dispatch: auto-started issue task",
		zap.String(logFieldSource, src.Name()),
		zap.String(logFieldTaskID, task.ID))
}
