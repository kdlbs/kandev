---
created: 2026-10-12
status: done
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
legacy_specs: []
---

# Implementation Plan: Recover abandoned and failed unmaterialized task environments

## Overview

The first session of a task owns the task's `creating` environment until its
launch publishes a workspace. Several early exits leave that claim behind, so
every later start returns `workspace_preparing` forever. When the owner reaches
a terminal state, the next binding marks the row `failed`, and every later
start returns `workspace_reuse_unsafe` until a manual environment reset, even
though nothing physical was created. Issue #4322 reports both paths.

This package lets the owner release its own claim when it gives up, lets a
start take over a claim whose owner is gone, and lets a start re-materialize a
`failed` environment that never created anything physical. Workspaces that
exist, and launches that are still live, stay protected. A `failed` environment
that does hold a workspace keeps refusing, and its error now names the reset
action.

## Scope

### In scope

- A conditional repository write that fails a `creating` environment only for
  its current owner, used by every owner-failure path.
- Claim release on `LaunchPreparedSession` errors and on `startTask` early
  returns for a session the start created.
- Abandonment of a `creating` claim when the owner is missing, terminal, or
  silent past the launch budget, decided inside the binding transaction, and
  re-election of such a claim.
- Re-election of a `failed` row without physical evidence, for task-local and
  shared-group bindings and for the launch of a session already bound to its own
  task's environment.
- Refusal of readiness publication and failure writes by a former owner.
- Reset guidance for a `failed` row with physical evidence.
- The contract amendment REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004, its system
  design section, and the clarifying paragraph in the additional-session
  workspace reuse requirements.

### Out of scope

- A periodic sweep or a new setting for abandoned environments.
- Re-election of an environment inherited from a parent task.
- Changes to session state on release; Kanban sessions keep `CREATED`.
- A second retry after the election-loser wait observes a `failed` row.
- Rendered UI.

## Not covered by this package

A launch that fails after the worktree store recorded a worktree, for example
in a setup script, leaves a `failed` environment with physical evidence. This
package keeps refusing it, with reset guidance instead of the current retry
advice. Replacing it automatically while the old checkout is retained needs a
schema decision: how a task keeps an inactive environment next to the active
one (for example a retirement timestamp with a uniqueness rule for active rows
only). That work also meets #3849, which removes the selected-recovery admission
block in `prepareSessionAttempt` where such a replacement would run and adds a
consumer admission class for recovery claims, and the existing preserved-checkout
inventory recovery. It is a follow-up for #4322 after this package.

## Contract changes

- `additional-session-workspace-reuse.md` treated a `failed` environment only
  as non-attachable. It now states that a start can elect a new materializer
  for an abandoned `creating` row or a `failed` row without physical evidence.
  An attach to a `failed` environment remains impossible.
- The pathless failure package latched a failed `session.ensure` until an
  explicit retry and kept automatic environment reset out of scope. That latch
  and the visible failure remain. A new binding, start, or launch request can
  now re-elect an eligible environment; nothing retries in the background, and
  no physical resource is reset.
- An abandoned `creating` row used to be marked `failed` and returned
  `workspace_reuse_unsafe` to the start that found it. That start now becomes
  the new owner when the row has no physical evidence.
- The `spawn_session_kandev` retry guidance for a `failed` environment with
  physical evidence changes from restoring the workspace to resetting the task
  environment. The reason stays `workspace_reuse_unsafe`.

## Technical approach

Work order 01 handles abandoned `creating` claims. The repository gains
`FailTaskEnvironmentMaterialization`, a write that succeeds only for the
current owner, and the executor routes every owner-failure path through it.
`LaunchPreparedSession` becomes a wrapper that releases the session's claim
after any error except `ErrExecutionAlreadyRunning`, and the `startTask`
early-return helper releases the claim of a session the start created unless the
error is `ErrCeilingLaunchDeferred`. `failAbandonedWorkspaceMaterialization`
gains the launch-budget rule, and `reelectTaskEnvironmentMaterializationTx`
hands an abandoned claim to the binding session. `persistTaskEnvironment`
refuses publication when the launch started as the initial materializer and the
fresh row is no longer its claim.

Work order 02 handles `failed` rows. The binding transactions re-elect a row
without owner and physical evidence, and `ReelectFailedTaskEnvironmentMaterialization`
does the same for the launch of a bound session, with the cleanup barrier and
recovery-claim admission. A lost launch race waits for or adopts the winner. A
row with physical evidence is refused with `models.ErrTaskEnvironmentResetRequired`,
and `spawn_session_kandev` maps it to reset guidance.

## Tests

- Work order 01: AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.1, 004.2, 004.3,
  004.6 (abandoned claims), 004.7, and 004.9, in repository, executor, and
  orchestrator tests.
- Work order 02: AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.4, 004.5, 004.6
  (failed rows), 004.8, and 004.10, in repository, executor, and MCP handler
  tests.

## E2E tests

No rendered UI changes. The repository tests run against the real SQLite
schema, and the executor, orchestrator, and handler tests drive the real entry
points with existing test doubles. No browser scenario is added.

## Work orders

- [x] [Task 01: Release and take over abandoned creating claims](task-01-release-abandoned-creating-claims.md)
- [x] [Task 02: Re-elect failed unmaterialized environments and point the rest at reset](task-02-reelect-failed-environments.md)

## Verification results

Both work orders record the contributor's historical RED and GREEN tests. Those
results and Windows exclusions below are not current-head merge acceptance.

- Some tests fail on Windows with and without this package and are excluded
  below:
  - in `executor`, 22 Kubernetes resume and workspace inventory repair or
    attestation tests;
  - in `sqlite`, `TestUpdateEnvironmentWritesMutableFieldsAndReportsMissing`.
- All other tests pass in the related `sqlite`, `executor`, `orchestrator`,
  and `mcp/handlers` groups.
- `go vet`, `golangci-lint --new-from-rev`, `go build ./...`, the
  documentation catalog and specification checks, and the local
  `pr-docs.cjs` coverage check of the changed files pass.

## Review remediation validation

Permanent launch regressions cover foreign-task refusal and a committed election
followed by a read failure. A hosted tests-first run demonstrated the foreign-task
claim-release defect and five PostgreSQL paths returning without the required
environment advisory lock. Shared cases exposed Go boolean parameters bound to
the real PostgreSQL INTEGER ownership column before reaching the lock assertion.

The remediation restricts release to validated task membership, serializes all
materialization paths with task-before-environment locking, and uses the existing
integer ownership encoder. Independent PostgreSQL holder, worker, and observer
connections cover changed owners and changed states after waiting, including
finalization inventory rollback. Both PostgreSQL 16 and 18 must execute these
tests; PostgreSQL 18 has explicit RUN/PASS guards. A permanent duration test also
covers saturation for extreme accepted preparation settings.

Current-head acceptance requires hosted Go, real PostgreSQL 16/18, native Windows,
actual frontend tests and build, all web E2E shards, native desktop, and Kubernetes
commands. Skipped wrappers and historical results provide no coverage. The
public executor guide describes empty-environment recovery and reset boundaries.

## Risks

- A launch that is still preparing after the launch budget is treated as
  abandoned. The former owner cannot publish over the new owner, but its
  partial runtime is cleaned up by the normal launch failure path.
- Shared-group siblings observe `workspace_preparing` again while a re-elected
  owner prepares the group environment.
- #4415 changes the same binding code for the election-loser retry. Whichever
  lands second is rebased onto the other.
