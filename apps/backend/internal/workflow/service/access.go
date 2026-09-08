package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/auth/authn"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Per-user scoping for the workflow-step surface (opt-in authentication).
//
// Workflows, workspaces and steps are all owned by the task domain — a
// workflow belongs to a workspace, and a workspace belongs to a user — so the
// permission itself is not this package's to decide. The checkers below are
// the same seam SetSessionAccessChecker already uses for step history: the
// task service's authorize* helpers are wired in at startup
// (internal/backendapp/services.go) and this package only decides *which*
// resource each entry point has to authorize.
//
// Scoping keys off the request-context identity exactly as the task service
// does: no identity means an internal caller (event bus, orchestrator,
// pollers), and a synthetic identity means authentication is disabled. Both
// are unscoped, so with auth off nothing here runs at all.
//
// Denials are reported as ErrNotVisible, which is also what a genuinely
// missing workflow, workspace or step produces. That is deliberate: a
// resource belonging to somebody else must be indistinguishable from one that
// does not exist, or the 404/500 split becomes an existence oracle.
//
// Workflow *templates* are exempt: they are install-global, read-only
// definitions with no owner (see the embedded YAML in config/workflows), so
// every authenticated user may read them.

// ErrNotVisible reports that the caller may not see the requested workflow,
// workspace or step — or that it does not exist. The message carries
// "not found" because the handler layer classifies mutation errors by that
// substring (see writeStepMutationError).
var ErrNotVisible = errors.New("workflow resource not found")

// An unwired checker means the workflow service was built without the task
// domain (unit tests, standalone tools) and leaves the corresponding check
// open, the same shape SetSessionAccessChecker has always had. Production
// always wires all four, which backendapp's TestWorkflowAccessCheckersAreWired
// pins. The fail-closed branches above it still run either way: an empty ID is
// refused before the checker is consulted, wired or not.

// SetWorkflowAccessChecker wires the task-domain check for a workflow ID.
// Production passes taskservice.Service.AuthorizeWorkflowAccess.
func (s *Service) SetWorkflowAccessChecker(checker func(context.Context, string) error) {
	s.workflowAccessChecker = checker
}

// SetWorkspaceAccessChecker wires the task-domain check for a workspace ID.
// Production passes taskservice.Service.AuthorizeWorkspaceAccess.
func (s *Service) SetWorkspaceAccessChecker(checker func(context.Context, string) error) {
	s.workspaceAccessChecker = checker
}

// SetTaskAccessChecker wires the task-domain check for a task ID. A step's
// queue_run action can name a task to start work on, so the step-write API
// accepts task IDs too. Production passes
// taskservice.Service.AuthorizeTaskAccess.
func (s *Service) SetTaskAccessChecker(checker func(context.Context, string) error) {
	s.taskAccessChecker = checker
}

// callerIsScoped mirrors internal/task/service's callerScope: false means the
// caller is an internal one or authentication is disabled, and no scoping
// applies. The workflow package repeats the three lines rather than importing
// the task service, which would be a dependency cycle.
func callerIsScoped(ctx context.Context) bool {
	identity, ok := authn.IdentityFromContext(ctx)
	return ok && !identity.Synthetic
}

// AuthorizeWorkflow checks that the caller may see a workflow.
//
// An empty workflow ID fails closed: every route reaching this helper names a
// workflow, so an empty one means the owner could not be resolved, and
// handing "" to the task-domain checker would be read as "no scoping applies"
// (the trap runSubscriptionCheck documents in backendapp/auth.go).
func (s *Service) AuthorizeWorkflow(ctx context.Context, workflowID string) error {
	if !callerIsScoped(ctx) {
		return nil
	}
	if workflowID == "" {
		return ErrNotVisible
	}
	if s.workflowAccessChecker == nil {
		return nil
	}
	return normalizeAccessError(s.workflowAccessChecker(ctx, workflowID))
}

// AuthorizeWorkspace checks that the caller may see a workspace. An empty
// workspace ID fails closed for the same reason as AuthorizeWorkflow.
func (s *Service) AuthorizeWorkspace(ctx context.Context, workspaceID string) error {
	if !callerIsScoped(ctx) {
		return nil
	}
	if workspaceID == "" {
		return ErrNotVisible
	}
	if s.workspaceAccessChecker == nil {
		return nil
	}
	return normalizeAccessError(s.workspaceAccessChecker(ctx, workspaceID))
}

// AuthorizeTask checks that the caller may see a task named inside a step's
// event actions. An empty ID fails closed for the same reason as
// AuthorizeWorkflow; callers strip the "this" sentinel before calling.
func (s *Service) AuthorizeTask(ctx context.Context, taskID string) error {
	if !callerIsScoped(ctx) {
		return nil
	}
	if taskID == "" {
		return ErrNotVisible
	}
	if s.taskAccessChecker == nil {
		return nil
	}
	return normalizeAccessError(s.taskAccessChecker(ctx, taskID))
}

// AuthorizeStep checks that the caller may see a workflow step, resolving the
// step's owning workflow (and through it, its workspace) first. A step whose
// workflow cannot be resolved fails closed.
//
// Reading the step row before deciding is unavoidable — the step ID is the
// only thing the caller supplied — but nothing is disclosed: every rejection,
// including a step that does not exist at all, returns ErrNotVisible.
func (s *Service) AuthorizeStep(ctx context.Context, stepID string) error {
	if !callerIsScoped(ctx) {
		return nil
	}
	if stepID == "" {
		return ErrNotVisible
	}
	if s.workflowAccessChecker == nil {
		return nil
	}
	step, err := s.repo.GetStep(ctx, stepID)
	if err != nil {
		if isNotFoundError(err) {
			return ErrNotVisible
		}
		return err
	}
	if step == nil {
		// The repository reports a miss as an error rather than a nil row; this
		// only keeps the dereference below safe.
		return ErrNotVisible
	}
	// A step whose workflow_id is empty has no resolvable owner and lands in
	// AuthorizeWorkflow's fail-closed branch.
	if err := s.AuthorizeWorkflow(ctx, step.WorkflowID); err != nil {
		if errors.Is(err, ErrNotVisible) {
			return ErrNotVisible
		}
		return err
	}
	return nil
}

// IsNotFound reports whether err is the one answer this package gives for
// both a resource the caller may not see and one that does not exist. Handler
// layers use it to pick a not-found reply without having to re-derive the
// classification (and without accidentally reporting a genuine failure as a
// miss, which would hide an outage behind an empty board).
func IsNotFound(err error) bool {
	return isNotFoundError(err)
}

// normalizeAccessError collapses a denial into ErrNotVisible and lets a
// genuine failure (a database error, say) through as a server error.
func normalizeAccessError(err error) error {
	if err == nil {
		return nil
	}
	if isNotFoundError(err) {
		return fmt.Errorf("%w", ErrNotVisible)
	}
	return err
}

// isNotFoundError recognizes both the task domain's sentinels and its
// formatted misses. The task repository reports a missing workflow or
// workspace as fmt.Errorf("workflow not found: %s", id) rather than a
// sentinel, and the task service returns those verbatim from its authorize*
// helpers, so a sentinel-only test would classify "this workflow does not
// exist" as a server error while "this workflow is not yours" answered 404 —
// which is the existence leak this guard exists to close.
//
// Every sentinel a caller relies on belongs in the typed set below even when
// its message would also satisfy the textual fallback: the fallback is a net
// for the formatted misses, not a contract, and a sentinel matched only by its
// wording silently stops being classified the moment somebody rewords it.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotVisible) ||
		errors.Is(err, repoerrors.ErrWorkspaceNotFound) ||
		errors.Is(err, repoerrors.ErrTaskNotFound) ||
		errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
