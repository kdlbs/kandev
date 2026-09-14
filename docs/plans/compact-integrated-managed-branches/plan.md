---
created: 2026-09-14
status: implemented
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
legacy_specs: []
---

# Implementation Plan: Compact Integrated Managed Branches

## Overview

Bound generated local branch growth at task cleanup boundaries while retaining
every branch that cannot be proven safe to remove. Persist ownership,
integration, and exact recovery metadata first; centralize eligibility and
expected-head deletion in the worktree manager; then reuse that policy from
terminal cleanup and bounded archived maintenance.

## Scope

### In scope

- Persist conservative managed-branch ownership, integration, recovery-head,
  and completion metadata on canonical task-environment repository rows.
- Compact one exact local branch only after ownership, liveness, protected-ref,
  and integration-ancestry checks succeed under the repository lock.
- Restore an exact compacted head during unarchive/recreation and keep recovery
  interruption ordering retryable.
- Route task cleanup, handoff cleanup, environment reset, aged automation
  cleanup, and archived storage maintenance through the shared policy.
- Emit bounded receipts, logs, and cumulative metrics.

### Out of scope

- Branch-name, age, or glob-based ownership inference.
- Network fetches, remote-ref deletion, or force deletion in preserving cleanup.
- Changing explicit permanent task-deletion semantics or adding UI.

## Technical approach

Extend `TaskEnvironmentRepo` and the normalized schema with conservative branch
metadata and compare-and-set repository operations. Propagate the verified
integration ref from branch policy through environment preparation and executor
persistence. After worktree registration removal, `worktree.Manager` evaluates
one deduplicated candidate under its repository lock, creates an opaque recovery
ref, persists the exact head, and removes only
`refs/heads/<branch>` with the expected SHA.

Archive retains ineligible branches. Storage maintenance selects at most 100
archived, inactive, deleted managed rows, rechecks archived state, and delegates
to the same manager. Recovery recreates the exact local ref before remote
materialization, removes the recovery ref before clearing its metadata, and
supports idempotent retry across interruptions.

## Tests

- AC-TASKS-RUNTIME-CLEANUP-001.18: cleanup policy tests cover managed ownership,
  ambiguous/legacy/external/shared rows, liveness, protected refs, integration,
  and concurrent cleanup.
- AC-TASKS-RUNTIME-CLEANUP-001.19: cleanup, archived maintenance, GC reachability,
  recreate, and `RecoverBranchStatus` tests prove exact restoration and
  interruption-safe recovery finalization.
- AC-TASKS-RUNTIME-CLEANUP-001.20: command-boundary tests prove one explicit
  local expected-head deletion and retention on head races.
- AC-TASKS-RUNTIME-CLEANUP-001.21: receipt and metrics tests cover fixed reason
  labels and deduplicated counts.
- AC-TASKS-RUNTIME-CLEANUP-001.22: archived maintenance tests cover bounded
  selection, archive/session races, later integration, idempotency, and live-ref
  restoration.

## Work orders

- [x] [Task 01: Implement safe managed-branch compaction and recovery](task-01-managed-branch-compaction.md)

## Verification results

- RED: `TestRecoverBranchStatus_FinalizesInterruptedPreDeleteRecovery` and
  `TestRecoverBranchStatus_RetainsMetadataWhenRecoveryRefDeleteFails` failed on
  head `986e745fc0b714cf383d9c86e78a99d8926d4c82` before the recovery-ordering fix.
- The complete `internal/worktree` suite and the focused race-enabled cleanup,
  maintenance, recovery, and recreate suite passed after the fix.
- Focused SQLite repository, backend application, and task service suites passed.
- SQL guard, differential Go lint against `origin/main`, backend build, spec
  catalog validation, spec lint, delivery-package coverage, and diff checks
  passed.
- Full `make -C apps/backend lint` reaches 35 violations in current
  `origin/main` files outside this change; differential lint reports zero issues.

## Risks

- A stale ownership or integration record could authorize data loss; every
  incomplete or inconsistent input must retain the branch.
- Git liveness and branch heads can change outside the manager lock; expected-SHA
  ref updates and post-delete liveness repair must remain fail closed.
- Recovery metadata and its opaque ref can diverge across interruption; ordering
  must leave either step safely retryable.
