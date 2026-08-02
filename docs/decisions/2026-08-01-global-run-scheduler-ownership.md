# ADR-2026-08-01-global-run-scheduler-ownership: Separate Global Run Dispatch from Office Maintenance

**Status:** accepted
**Date:** 2026-08-01
**Area:** backend, workflow

## Context

Task-model unification made `runs` a shared queue for workflow-emitted work,
but its loop is still constructed inside Office startup and its processor still
mixes generic queue draining with Office unstarted-task recovery. The loops are
started as untracked goroutines using the application context, whose
cancellation cleanup currently runs after repository cleanup; a periodic tick
can therefore query SQLite after it has closed. Enabling Office also makes the
recovery sweep inspect any assigned `TODO` task, including ordinary Kanban
tasks, even when no workspace has adopted Office.

## Decision

Kandev owns one backend-wide scheduler for the shared `runs` queue. The
scheduler is not replicated per workspace and processes explicit queued work
from every workflow style.

Generic run dispatch and Office maintenance are separate responsibilities:

- Queue draining, missed-signal recovery, scheduled retries, and stale-claim
  recovery belong to the shared runs subsystem.
- Assignment alone is not an autonomy signal for ordinary Kanban tasks.
  Office assignment subscribers and unstarted-task recovery use the same
  authoritative Office-task predicate as `Task.IsFromOffice`: a project-linked
  task or a task in `workspaces.office_workflow_id`.
- Explicit workflow `queue_run` actions remain generic and are processed for
  any workflow style.
- Office unstarted-task recovery is removed from the five-second shared queue
  tick and driven as Office maintenance only when relevant Office
  configuration exists. Routines, budgets, and heartbeat handlers retain
  their own configuration checks.

Every long-running scheduler owns its goroutine with idempotent `Start` and
`Stop` semantics. `Stop` cancels and waits for the active tick and handler
fan-out. Graceful shutdown quiesces these schedulers before stopping downstream
orchestrator/runtime services and before closing repositories or the database.
Shutdown completion is logged only after all owned loops have joined.

This decision refines ADR-0004's generic-runs-queue direction; it does not
introduce an Office workspace process or use workflow style as a generic
dispatch discriminator.

## Consequences

- Multiple Office workspaces share one fair-by-request-time queue and do not
  multiply ticker goroutines.
- Kanban tasks retain user/workflow-controlled launch behavior unless their
  workflow explicitly emits `queue_run`.
- An Office-enabled installation with no Office adoption avoids repeated
  Office task scans, though the shared queue's lightweight safety loop remains
  available.
- Scheduler lifecycles become testable and shutdown can deterministically
  prove that no database access follows pool closure.
- The existing transitional `office/service.SchedulerIntegration` must be
  decomposed enough to move Office recovery out of its generic tick; a full
  migration of all prompt/routing policy out of the Office package is not
  required by this decision.
- Dynamically adding the first Office workflow requires no new per-workspace
  scheduler: persisted configuration is observed by the next Office
  maintenance pass.

## Alternatives Considered

### One scheduler per Office workspace

Rejected because `runs` is a shared queue, per-agent serialization is global,
and workspace-local loops would duplicate timers and complicate fair claiming,
dynamic workspace deletion, and shutdown.

### Start the existing combined scheduler only when an Office workspace exists

Rejected because explicit `queue_run` is workflow-generic and because the
first Office workflow can be created while the backend is running. A startup
count would either miss new work until restart or require a lifecycle
controller solely to recreate a queue consumer.

### Keep the combined global tick and accept idle scans

Rejected because it conflates assignment with autonomy, causes needless
Office database work in Kanban-only installations, and leaves the shutdown
race unresolved.

### Cancel the application context earlier without joining loops

Rejected because cancellation alone does not prove that an in-flight tick has
returned before SQLite closes. Owned goroutines and a join are required.
