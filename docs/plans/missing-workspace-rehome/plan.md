---
created: 2026-09-03
status: draft
requirements:
  - REQ-TASKS-MISSING-WORKSPACE-REHOME-001
  - REQ-TASKS-MISSING-WORKSPACE-REHOME-002
  - REQ-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001
system_design:
  - ../../specs/tasks/system-design/missing-workspace-rehome.md
  - ../../specs/executors/system-design/coder-task-root-durability.md
legacy_specs: []
---

# Implementation Plan: Missing Workspace Rehome

## Overview

Add a typed missing-workspace cause, durable loss evidence and rehome claim,
then route automatic and explicit launches through one bounded recovery path.
Expose the resulting warning/action on existing task error surfaces and warn
operators when a Coder SSH profile uses an unproved task root. Persistence lands
first because every later concurrency and failure guarantee depends on it.

## Scope

### In scope

- Same-task environment-binding refresh and replacement launch.
- Task-scoped idempotency across concurrent calls and backend restart.
- Exactly one retry and durable original/recovery failure projection.
- Evidence-gated automatic recovery and stamped human authorization.
- Coder SSH task-root fail-fast admission and public `/work/.kandev` guidance.
- Exact-root SSH repository materialization beneath an ancestor checkout.
- Desktop and mobile recovery and profile-warning coverage.

### Out of scope

- Recovering files already deleted from a remote host.
- Automatically pushing or deleting repository refs.
- General repair of every workspace reuse failure.
- Automatic cleanup of superseded remote task directories.

## Technical approach

### Persistence and loss evidence

Use the existing task environment row as the stable binding and add a
transactional compare-and-swap from ready/stopped to creating. The claim clears
only stale physical handles and repository inventory while preserving task,
session, workflow, profile, repository-selection, conversation, and plan rows;
no schema migration is required. Evaluate completion snapshots for the complete
repository inventory under the snapshot environment lock, scoped to the
launching session, and fail closed on missing, partial, stale, or malformed
evidence.

### Typed classification and recovery coordinator

Add a structured `missing_task_workspace` reason beneath
`ErrWorkspaceReuseUnsafe` in the lifecycle boundary. Introduce an orchestrator
coordinator used by step-entry auto-start, created-session start, resume, and
the stamped `task.launch.recover` action. It claims the durable operation,
materializes the winner, consumes one retry, joins concurrent followers, and
terminalizes failed replacement sessions instead of leaving `CREATED` rows.

The default SSH and remote-contribution prepare scripts reuse a repository only
when its canonical `--show-toplevel` equals the canonical task workspace. An
ancestor checkout is not task repository identity and causes initialization in
the task root rather than an origin comparison against the parent.

### Projection and responsive recovery

Extend `last_launch_error` / `TaskStatusSummary.active_error` with the new safe
category, loss state, paired bounded errors, and
`authorize_fresh_rehome`. Reuse `task-launch-error-entry.tsx` and the current
mobile task Chat error-card/drawer composition; share action state and keep one
mobile scroll owner with touch-sized controls.

### Coder profile admission and docs

Detect Coder at the live SSH boundary and require an authoritative profile
policy before remote task creation or agent startup. Validate that the root is
dedicated, available, and writable with an isolated probe; retain an explicitly
risky ephemeral escape hatch. Keep the responsive warning and update
`docs/public/executors.md` with the exact policy values and durable-root examples.

## Tests

- `AC-TASKS-MISSING-WORKSPACE-REHOME-001.1` through `.6`: repository and
  orchestrator integration tests cover deleted SSH task directories, phase
  transition identity, concurrency, one retry, failed replacement state, and
  unchanged normal reuse.
- `AC-TASKS-MISSING-WORKSPACE-REHOME-002.1` through `.5`: loss-assessment and
  recovery-action tests cover multi-repository clean/reachable and unique work,
  incomplete/stale inventory, current stamped authorization, and responsive UI.
- `AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.1` through `.3`: SSH health,
  profile service, and serialization tests cover warning presence and absence.
- `AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.4`: local-shell and remote SSH
  regressions place the task root beneath another checkout and require the task
  root itself to become the repository top level.
- `AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.5` through `.7`: lifecycle tests
  cover pre-Direction rejection, live path usability, the explicit risky escape
  hatch, ordinary SSH compatibility, and collision-free concurrent probes.

## E2E tests

- Extend `apps/web/e2e/tests/task/launch-failure-recovery.spec.ts` for the
  warning, authorized rehome, and visible failed replacement.
- Extend `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts` for
  touch authorization and zero horizontal overflow.
- Extend SSH container coverage for phase A completion, deleted/rebuilt remote
  directory, reset, and phase B auto-start with the same task and step.
- Add desktop/mobile executor profile warning coverage under
  `apps/web/e2e/tests/settings/`.

## Work orders

- [x] [Task 01: Persist atomic rehome claims and loss evidence](task-01-persist-rehome-generations.md)
- [x] [Task 02: Recover launches with one idempotent retry](task-02-recover-launches.md)
- [x] [Task 03: Expose recovery and Coder durability warnings](task-03-expose-recovery-warnings.md)
- [x] [Task 04: Prove end-to-end phase recovery](task-04-prove-phase-recovery.md)
- [x] [Task 05: Gate Coder launch on workdir admission](task-05-gate-coder-workdir.md)

## Verification results

Focused race tests cover the atomic claim, loss gate, exactly-once retry, and
paired failure projection. Backend lifecycle tests cover typed missing-directory
classification and exact-root materialization. The container-backed SSH proof
reached `RUNNING` at `/opt/jumprope-fullstack/.kandev` below a parent checkout.

## Risks

- Automatic recovery depends on the latest durable Git snapshot; absent or
  ambiguous evidence intentionally requires human authorization.
- Git snapshot freshness must be tied to the completed phase; an older clean
  observation cannot authorize automatic rehome after later edits.
- Stale runtime events from the superseded generation could otherwise fail the
  replacement session.
- Coder mount layouts are customizable, so the operator policy remains an
  explicit durability assertion rather than inferred proof about `/work`.
- Shell path comparison must canonicalize both sides without accepting a
  symlinked ancestor as the task checkout.
