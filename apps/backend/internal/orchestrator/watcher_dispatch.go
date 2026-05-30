package orchestrator

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
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
	// mu guards taskCreator. SetTaskCreator may be called more than once at
	// boot (tests in particular swap creators between scenarios), and
	// Dispatch reads from background goroutines spawned by
	// dispatchWatcherEvent — so the field needs synchronisation.
	mu              sync.RWMutex
	taskCreator     IssueTaskCreator
	startTask       taskStarter
	shouldAutoStart func(ctx context.Context, workflowStepID string) bool
	logger          *logger.Logger
}

// SetTaskCreator atomically updates the task creator the coordinator
// dispatches to. Safe to call concurrently with Dispatch.
func (c *WatcherDispatchCoordinator) SetTaskCreator(tc IssueTaskCreator) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.taskCreator = tc
}

func (c *WatcherDispatchCoordinator) getTaskCreator() IssueTaskCreator {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.taskCreator
}

// taskStarter wraps Service.StartTask so the coordinator can be tested
// without spinning up the full orchestrator service.
type taskStarter interface {
	Start(ctx context.Context, taskID, workflowStepID, prompt string, params AutoStartParams) error
}

// AutoStartParams is the data a source contributes when the resulting task's
// workflow step has auto-start enabled. The prompt is intentionally NOT
// included here — the coordinator passes the created task's Description
// directly so auto-start always uses the persisted task body, matching the
// legacy createXIssueTask call sites byte-for-byte.
type AutoStartParams struct {
	AgentProfileID    string
	ExecutorProfileID string
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

	// WatchID extracts the per-integration watch identifier from the event
	// payload. Used by the throttle gate to key the per-watch pending
	// counter and look up the per-watch cap. Returns "" if the event has
	// no watch (the gate then treats it as unthrottled).
	WatchID(evt any) string

	// MaxInflightTasks returns the per-watch cap on open watcher-created
	// tasks, or nil when the watch is uncapped. The orchestrator gate uses
	// this to decide whether to defer the event.
	MaxInflightTasks(evt any) *int
}

// Dispatch runs one event through the full pipeline. Safe to call from a
// goroutine; callers typically do so in the bus subscriber.
func (c *WatcherDispatchCoordinator) Dispatch(ctx context.Context, src WatcherSource, evt any) {
	reserved, err := src.Reserve(ctx, evt)
	if err != nil {
		c.logger.Error("watcher dispatch: reserve failed",
			zap.String("source", src.Name()), zap.Error(err))
		return
	}
	if !reserved {
		c.logger.Debug("watcher dispatch: already reserved by concurrent handler",
			zap.String("source", src.Name()))
		return
	}

	req, err := src.BuildTaskRequest(evt)
	if err != nil {
		c.logger.Error("watcher dispatch: build task request failed",
			zap.String("source", src.Name()), zap.Error(err))
		src.Release(ctx, evt)
		return
	}

	task, err := c.getTaskCreator().CreateIssueTask(ctx, req)
	if err != nil {
		c.logger.Error("watcher dispatch: create issue task failed",
			zap.String("source", src.Name()), zap.Error(err))
		src.Release(ctx, evt)
		return
	}

	if err := src.AttachTaskID(ctx, evt, task.ID); err != nil {
		c.logger.Error("watcher dispatch: attach task id failed",
			zap.String("source", src.Name()),
			zap.String("task_id", task.ID),
			zap.Error(err))
		// Do NOT release here — matches existing Linear/Jira behaviour:
		// attach is a best-effort step, the task is already created.
	}

	c.logger.Info("watcher dispatch: created issue task",
		zap.String("source", src.Name()),
		zap.String("task_id", task.ID))

	if !c.shouldAutoStart(ctx, req.WorkflowStepID) {
		return
	}

	params := src.AutoStartParams(evt)
	if err := c.startTask.Start(ctx, task.ID, req.WorkflowStepID, task.Description, params); err != nil {
		c.logger.Error("watcher dispatch: auto-start failed",
			zap.String("source", src.Name()),
			zap.String("task_id", task.ID),
			zap.Error(err))
		return
	}
	c.logger.Info("watcher dispatch: auto-started issue task",
		zap.String("source", src.Name()),
		zap.String("task_id", task.ID))
}
