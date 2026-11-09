---
created: 2026-11-08
status: completed
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004
  - REQ-TASKS-MCP-WORKSPACE-MODE-004
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
legacy_specs: []
---

# Implementation Plan: Inherited executor transition correctness

## Overview

Make an inherited workspace executor transition elect one durable child-owned
environment before the selected executor allocates resources. Keep that
environment canonical for later child sessions and keep the parent or shared
group environment unchanged. The code and regression work comes first. Public
guidance and the MCP tool description follow after behavior is fixed.

The root cause is an ordering gap. Distinct sessions can both pass the mismatch
check, generate separate environment IDs, and allocate separate runtimes. The
task lock runs only during post-launch persistence. The later write can replace
the canonical environment handles while both runtimes remain live. Session
coalescing does not help because it is keyed by session ID.

## Scope

### In scope

- Elect a task-owned environment in a durable repository transaction before
  executor allocation for an eligible inherited transition.
- Reuse the detached child's canonical environment for later sessions. Preserve
  the group environment pointer and the parent or group owner's environment.
- Cover `inherit_parent` and `shared_group`, including ready, creating, and
  failed foreign environments when a different non-Kubernetes executor is
  selected.
- Prevent a prepared session from reusing its old executor runtime or reporting
  workspace-only success before the child environment is persisted.
- Add deterministic concurrency and repository regressions for different
  session IDs.
- Update public workspace-mode guidance and the MCP schema description. Explain
  that a fresh workspace does not include uncommitted parent changes.
- Provide a changed work order for the PR documentation-coverage status.
- After the fix is pushed, reply to and resolve every still-actionable review
  thread and post one concise PR comment about the improvements.

### Out of scope

- Changing Kubernetes inherited-workspace refusal or resume mismatch behavior.
- Changing same-executor inheritance behavior or the workspace-mode enum.
- Removing a task from its workspace group or changing the group's ownership.
- Adding a new environment table, persistent marker, or schema migration.
- Changing frontend transport, rendered UI, or concurrent agent write policy.
- Waiting for unrelated CI failures to be repaired.

## Technical approach

### 1. Claim the child environment before allocation

Extend the durable binding path used by `CreateTaskSessionWithWorkspaceBinding`
and `CreateTaskSessionWithSharedGroupWorkspaceBinding`. Use the existing
task-scoped `task_environments` row, unique task identity, and
`materialization_session_id` owner. Bind the winning session and publish the
creating claim transactionally. The losing session must not call lifecycle
allocation. It waits through the bounded `workspace_preparing` path or attaches
after the winner publishes the ready environment. Do not use `taskEnvLock` as
the cross-session election.

### 2. Keep one environment authority across launch paths

In `admitInheritedEnvironment`, `propagateInheritedEnvironment`,
`inheritFromParentEnvironment`, and `inheritFromSharedGroup`, prefer an existing
child-owned canonical environment over the inherited parent or group
environment. For a first transition, classify the executor mismatch before
waiting for inherited readiness or re-electing a failed foreign environment.
Only a known different non-Kubernetes executor can detach. Other mismatch,
unknown-ownership, resume, and Kubernetes paths retain their typed refusal or
bounded wait.

When a group member detaches, bind that task's sessions to its child-owned row
and leave `task_workspace_groups.materialized_environment_id` unchanged. Other
members continue to bind the group row. Do not rebind the group pointer to the
detached child.

### 3. Retire prepared-session runtime state

When the session has an execution associated with the inherited environment,
retire that session-scoped runtime before launching on the selected executor.
Preserve the foreign environment and its physical resources. A
`HasExecutorRunningRow` result alone cannot authorize reuse across executor
types. A workspace-only launch must persist and bind the child environment
before it returns success or permits agent admission.

### 4. Cover public behavior

Update `docs/public/automation-and-mcp.md`, `docs/public/coordination.md`, and
the `workspace_mode` description in
`apps/backend/internal/mcp/server/server.go`. State that inheritance normally
reuses the parent's materialized workspace, but a new launch on a different
non-Kubernetes executor receives a separate child-owned workspace. State that
uncommitted parent changes are not copied. Keep Kubernetes and resume limits
clear. Update `docs/public/coverage.json` only if the existing area evidence
does not cover the changed source or regression tests.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.5` | Executor transition tests for `inherit_parent` and `shared_group`; ready, creating, and failed foreign environments; same-executor, resume, and Kubernetes controls. |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.6` | A later session and a workspace read resolve the detached child's persisted canonical environment. |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.7` | A real repository test proves the group pointer and another member's binding remain unchanged. |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.8` | A barrier-based test launches two distinct session IDs with different executor types and proves both bind to one canonical environment, the losing executor is refused before a second physical allocation, and only one workspace is materialized. |
| `AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.5` | A failed foreign environment is never re-elected or changed by an eligible child transition; non-eligible starts retain `workspace_reuse_unsafe`. |
| `AC-TASKS-MCP-WORKSPACE-MODE-004.2` | Public docs and the MCP schema description state the exception and file-transfer limit. |

The Go regression uses real repository binding semantics and a controlled
lifecycle barrier. It asserts that the child session's persisted
`TaskEnvironmentID` and `WorkspacePath` match the created environment and that
the launch request does not forward the inherited workspace path. No rendered
UI changes; the browser transport remains unchanged.

Claim recovery coverage also proves that cancellation permits a successor to
take over when the existing physical-evidence guard allows it, and that a stale
former owner cannot publish completion after takeover. Concurrent selections
of different executor types bind to the same canonical child environment; the
losing target receives a typed mismatch refusal before it allocates a second
workspace.

## E2E tests

None. The correction changes backend workspace ownership and launch admission.
The targeted repository/orchestrator integration test exercises this contract;
the browser workflow and layout do not change.

## Review findings to disposition after the push

| Comment | Planned disposition |
| --- | --- |
| [4505309466](https://github.com/kdlbs/kandev/pull/5175#discussion_r4505309466) | Cover the shared-group transition, propagation/admission paths, unchanged group pointer, and sibling behavior. |
| [4505312278](https://github.com/kdlbs/kandev/pull/5175#discussion_r4505312278) | Let the transactional shared-group binder claim the child environment on mismatch; cover it with a real repository test. |
| [4505312281](https://github.com/kdlbs/kandev/pull/5175#discussion_r4505312281) | Retire the inherited session runtime and ensure workspace-only launch persists the child environment before success. |
| [4505334499](https://github.com/kdlbs/kandev/pull/5175#discussion_r4505334499) | Classify before inherited readiness waits or failed-environment recovery; verify both creating and failed parent cases. |
| [4505334503](https://github.com/kdlbs/kandev/pull/5175#discussion_r4505334503) | Keep the detached child environment authoritative during later session preparation and workspace reads. |
| CodeRabbit review [5816948855](https://github.com/kdlbs/kandev/pull/5175#pullrequestreview-5816948855) | Assert an empty inherited `WorkspacePath`, the persisted child session binding, and provisioned environment state for prepared launches. |

Reply to and resolve only threads that remain unresolved at the exact pushed
head. Verify each posted reply and final thread state. Post one concise PR
comment that explains the durable election, preserved group workspace, and
regression coverage.

## Work orders

- [x] [Task 01: Elect and preserve detached environments](task-01-elect-detached-environments.md)
- [x] [Task 02: Document the executor transition](task-02-document-executor-transition.md)

## Verification results

Both work orders are complete. The exact focused Go package command, runtime
retirement and persistence tests, concurrency race checks, changed-file Go lint,
formatting, MCP contract test, public-doc tests, and document validators passed.
Primary review corrections cover cleanup retry after committed binding,
pre-claim refusal for live and persisted running agents, and a follower-only
same-executor allocation barrier. That barrier also passed 10 repeated runs.

The three PostgreSQL transaction tests compiled and explicitly skipped because
`KANDEV_TEST_POSTGRES_DSN` is unset. The work orders record exact check commands
and RED/GREEN evidence. Merge-result verification and GitHub delivery evidence
are recorded in the external Kandev plan after the reviewed commit is created.

## Risks

- Runtime retirement must stop the old session executor without deleting or
  changing the inherited environment it used.
- A losing session must not allocate a container, worktree, or other executor
  resource while another session owns the creating claim.
- The failed-parent exception must never re-elect or mutate the failed foreign
  environment; it only creates or claims the child's own environment.
- Validate the merge result against current `main` before pushing. Resolve any
  actual branch conflict and rerun affected checks. GitHub currently reports
  the PR as mergeable with no branch conflict.

## Delivery

The PR head repository is `dreampiler/kandev`, and maintainer edits are enabled.
Push over the existing `dreampiler` SSH remote to
`codex/upstream-inherited-executor-transition-20261108`. Before push, refresh
the PR head and base. After push, require local `HEAD` and the PR head OID to
match, confirm mergeability, refresh the review threads, post the requested PR
comment, then resolve the addressed threads with a verified reply.
