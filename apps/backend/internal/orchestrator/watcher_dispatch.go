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
	// profileLookup pre-flight checks the watcher's bound agent profile.
	// When the profile has been soft-deleted (reconciler-driven cleanup of an
	// agent type that fell off the registry), the coordinator short-circuits
	// before creating any task and asks the source to self-heal the watcher
	// row. nil means "skip the check" — production wires this in; tests can
	// leave it unset.
	profileLookup ProfileLookup
	// repoChecker pre-flight checks the watcher's bound repository (when the
	// built request carries one). A repository soft-deleted after the watch was
	// bound would otherwise let CreateTask insert the task row before repository
	// association fails — leaving an orphan row and a reservation that a later
	// poll repeats. When the repo is gone the coordinator self-heals the watch
	// and skips. nil means "skip the check".
	repoChecker RepositoryChecker
	logger      *logger.Logger
}

// RepositoryChecker answers "does this repository still exist in the workspace?"
// — used by the watcher dispatch self-heal flow to detect a binding whose
// repository was soft-deleted after the watch was configured. Membership-based
// (a workspace listing) so a definitive "absent" is distinguishable from a
// transient error: (false, nil) = gone → self-heal; a non-nil err = "couldn't
// tell" → fail open (the existing pipeline runs).
type RepositoryChecker interface {
	RepositoryExists(ctx context.Context, workspaceID, repositoryID string) (bool, error)
}

// SetRepositoryChecker wires the repository pre-flight check. nil-ok; the
// check becomes a no-op until a real checker is provided.
func (c *WatcherDispatchCoordinator) SetRepositoryChecker(r RepositoryChecker) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.repoChecker = r
}

func (c *WatcherDispatchCoordinator) getRepositoryChecker() RepositoryChecker {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.repoChecker
}

// ProfileLookup answers "is this agent profile still live, and what was its
// display name?" — used by the watcher dispatch self-heal flow to detect
// orphaned watchers (their agent profile was removed by the orchestrator's
// reconciler when its agent type left the enabled registry).
//
// Returning (true, name, nil) means the row exists but has DeletedAt set;
// (false, _, nil) means the row is live; a non-nil err is treated as
// "couldn't tell" and the dispatch falls through (fail-open).
type ProfileLookup interface {
	LookupProfile(ctx context.Context, profileID string) (deleted bool, name string, err error)
}

// SetProfileLookup wires the pre-flight check into the coordinator. Safe to
// call before or after task-creator wiring; nil-ok and pre-flight just
// becomes a no-op until a real lookup is provided.
func (c *WatcherDispatchCoordinator) SetProfileLookup(p ProfileLookup) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.profileLookup = p
}

func (c *WatcherDispatchCoordinator) getProfileLookup() ProfileLookup {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.profileLookup
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

	// AgentProfileID returns the agent profile bound to the watcher that
	// produced this event. The coordinator uses it for the soft-deleted-
	// profile pre-flight check; empty means "no profile bound" (legacy
	// rows) and the check is skipped.
	AgentProfileID(evt any) string

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

	// WatchMetadataKey returns the task-metadata key under which this
	// integration records the originating watch id (the same key written in
	// BuildTaskRequest's Metadata map). The throttle gate passes it to the
	// task counter so the repository can tally open tasks for a watch WITHOUT
	// hard-coding which integrations exist — the integration owns its own key.
	WatchMetadataKey() string

	// SelfHeal disables the watcher row that produced this event and stamps
	// a human-readable cause so the settings UI can show "disabled because
	// the bound agent profile was removed". Called by the coordinator
	// (and the legacy GitHub createXTask paths) when the pre-flight check
	// detects a soft-deleted profile.
	SelfHeal(ctx context.Context, evt any, cause string) error
}

// terminalAttachErrorClassifier marks attachment failures where the task no
// longer belongs to the event's watch generation. The source has already
// performed any required cleanup, so dispatch must stop before auto-start.
type terminalAttachErrorClassifier interface {
	IsTerminalAttachError(error) bool
}

// preflightDeletedProfile returns true when the watcher's bound profile has
// been soft-deleted (the production bug that orphans watchers via the
// reconciler's cleanup of disabled agent types). On a true return the
// coordinator MUST stop — SelfHeal has already been invoked.
func (c *WatcherDispatchCoordinator) preflightDeletedProfile(ctx context.Context, src WatcherSource, evt any) bool {
	lookup := c.getProfileLookup()
	if lookup == nil {
		return false
	}
	profileID := src.AgentProfileID(evt)
	if profileID == "" {
		return false
	}
	deleted, name, err := lookup.LookupProfile(ctx, profileID)
	if err != nil {
		if c.logger != nil {
			c.logger.Warn("watcher dispatch: profile lookup failed, falling through",
				zap.String("source", src.Name()),
				zap.String("profile_id", profileID),
				zap.Error(err))
		}
		return false
	}
	if !deleted {
		return false
	}
	cause := formatDeletedProfileCause(profileID, name)
	if c.logger != nil {
		c.logger.Warn("watcher dispatch: agent profile soft-deleted, self-healing",
			zap.String("source", src.Name()),
			zap.String("profile_id", profileID),
			zap.String("profile_name", name))
	}
	if err := src.SelfHeal(ctx, evt, cause); err != nil && c.logger != nil {
		c.logger.Error("watcher dispatch: self-heal failed",
			zap.String("source", src.Name()),
			zap.String("profile_id", profileID),
			zap.Error(err))
	}
	return true
}

// preflightDeletedRepository checks the bound repository of an already-built
// request. When the repo was soft-deleted after binding, it self-heals the
// watch (so a later poll doesn't repeat) and returns true so the caller skips
// task creation — avoiding an orphan task row from CreateTask's non-atomic
// task-then-repository insert. A lookup error fails open (returns false).
func (c *WatcherDispatchCoordinator) preflightDeletedRepository(ctx context.Context, src WatcherSource, evt any, req *IssueTaskRequest) bool {
	checker := c.getRepositoryChecker()
	if checker == nil || req == nil {
		return false
	}
	for _, r := range req.Repositories {
		if r.RepositoryID == "" {
			continue
		}
		exists, err := checker.RepositoryExists(ctx, req.WorkspaceID, r.RepositoryID)
		if err != nil {
			if c.logger != nil {
				c.logger.Warn("watcher dispatch: repository check failed, falling through",
					zap.String("source", src.Name()),
					zap.String("repository_id", r.RepositoryID),
					zap.Error(err))
			}
			return false
		}
		if exists {
			continue
		}
		cause := "bound repository " + r.RepositoryID + " was removed"
		if c.logger != nil {
			c.logger.Warn("watcher dispatch: bound repository removed, self-healing",
				zap.String("source", src.Name()),
				zap.String("repository_id", r.RepositoryID))
		}
		if err := src.SelfHeal(ctx, evt, cause); err != nil && c.logger != nil {
			c.logger.Error("watcher dispatch: self-heal failed",
				zap.String("source", src.Name()),
				zap.String("repository_id", r.RepositoryID),
				zap.Error(err))
		}
		return true
	}
	return false
}

// formatDeletedProfileCause renders the human-readable string stamped onto
// the watcher's last_error column. Centralised so every integration uses
// the same phrasing.
//
// profileName is user-typed in the settings UI with no DB-level length
// constraint; truncate at the producer so an arbitrarily-long name does
// not pollute last_error / the settings banner.
func formatDeletedProfileCause(profileID, profileName string) string {
	name := truncateProfileNameForCause(profileName)
	if name != "" {
		return "agent profile \"" + name + "\" (" + profileID + ") was removed"
	}
	return "agent profile " + profileID + " was removed"
}

// profileNameCauseMaxLen caps the rendered profile name in
// formatDeletedProfileCause. 80 runes matches the watcher-label cap in
// cmd/kandev — both end up in the same settings UI surface.
const profileNameCauseMaxLen = 80

func truncateProfileNameForCause(s string) string {
	runes := []rune(s)
	if len(runes) <= profileNameCauseMaxLen {
		return s
	}
	return string(runes[:profileNameCauseMaxLen-1]) + "…"
}

// Dispatch runs one event through the full pipeline. Safe to call from a
// goroutine; callers typically do so in the bus subscriber.
//
// Pre-flight: when a ProfileLookup is wired AND the source exposes a
// non-empty AgentProfileID, the coordinator first checks that the profile is
// not soft-deleted. A deleted profile short-circuits to SelfHeal — no task
// is created, no dedup reservation is taken. A lookup error fails open: the
// existing pipeline runs and any genuine error surfaces downstream.
func (c *WatcherDispatchCoordinator) Dispatch(ctx context.Context, src WatcherSource, evt any) {
	if c.preflightDeletedProfile(ctx, src, evt) {
		return
	}
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

	// A bound repository deleted after the watch was configured would let
	// CreateTask insert a task row before repository association fails. Self-heal
	// and release the reservation instead of creating an orphan task.
	if c.preflightDeletedRepository(ctx, src, evt, req) {
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
		if classifier, ok := src.(terminalAttachErrorClassifier); ok && classifier.IsTerminalAttachError(err) {
			return
		}
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
