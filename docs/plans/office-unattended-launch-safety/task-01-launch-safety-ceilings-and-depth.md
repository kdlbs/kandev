---
id: "01-launch-safety-ceilings-and-depth"
title: "Enforce Office launch ceilings and causation-depth refusal"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-LAUNCH-SAFETY-001
  - REQ-OFFICE-LAUNCH-SAFETY-003
acceptance_criteria:
  - AC-OFFICE-LAUNCH-SAFETY-001.1
  - AC-OFFICE-LAUNCH-SAFETY-001.2
  - AC-OFFICE-LAUNCH-SAFETY-001.3
  - AC-OFFICE-LAUNCH-SAFETY-001.4
  - AC-OFFICE-LAUNCH-SAFETY-001.5
  - AC-OFFICE-LAUNCH-SAFETY-001.6
  - AC-OFFICE-LAUNCH-SAFETY-001.7
  - AC-OFFICE-LAUNCH-SAFETY-001.8
  - AC-OFFICE-LAUNCH-SAFETY-001.9
  - AC-OFFICE-LAUNCH-SAFETY-003.1
  - AC-OFFICE-LAUNCH-SAFETY-003.2
  - AC-OFFICE-LAUNCH-SAFETY-003.3
  - AC-OFFICE-LAUNCH-SAFETY-003.4
  - AC-OFFICE-LAUNCH-SAFETY-003.5
  - AC-OFFICE-LAUNCH-SAFETY-003.6
  - AC-OFFICE-LAUNCH-SAFETY-003.7
  - AC-OFFICE-LAUNCH-SAFETY-003.8
  - AC-OFFICE-LAUNCH-SAFETY-003.9
  - AC-OFFICE-LAUNCH-SAFETY-003.10
system_design:
  - ../../specs/office/system-design/unattended-launch-safety-01.md
  - ../../specs/office/system-design/unattended-launch-safety-02.md
---

# Task 01: Enforce Office launch ceilings and causation-depth refusal

## Scope

- Add the causation schema (causation id, parent run, depth, actor,
  human-rooted flag) and thread it through every enqueue path: runtime
  actions, approvals, `SpawnAgentRun`, task-boundary carriers (root task
  creation, subtask creation, task-assigned enqueue, routine fire), and
  workflow step actions.
- Enforce claim-time instance/workspace/agent ceilings, each serialized
  against concurrent claims, with configured-value clamping and gate-failure
  observability wired into the config catalog.
- Enforce the enqueue-time causation-depth refusal on the authoritative
  `runs/service` enqueue seam, ahead of idempotency/coalescing resolution,
  without consuming the idempotency key on refusal.
- Consolidate direct `CreateRun`/`CreateRunTx` call sites onto that seam
  (`office/service.queueRunInline`, the authoritative `runs/service` insert,
  and the office test harness) and add a structural test
  (`enqueue_consolidation_test.go`) that fails on a new unreviewed direct
  insert.
- Page the claim-candidate scan past one batch to stop starvation, and record
  windowed-dedup and gate-failure-race review fixes discovered while landing
  this delivery.

## Acceptance

A run is claimed only when the instance, workspace, and agent ceilings all
have spare capacity, evaluated atomically with the claim transition; an
agent's own `max_concurrent_sessions` can never raise the real bound past the
configured workspace/instance ceilings. A chain of agent-caused launches
refuses to enqueue once it would exceed the configured maximum depth, returns
a distinguishable error to the calling agent, records a durable
operator-visible entry, and leaves the request's idempotency key unconsumed
so a later legitimate retry is not silently dropped. Every enqueue path,
including workflow-step-queued runs and task-boundary-carried wakes, is
subject to the same depth refusal because none of them can insert a run
outside the authoritative seam.

## Verification

```bash
cd apps/backend && env -u KANDEV_HEALTH_TIMEOUT_MS go test \
  ./internal/office/scheduler ./internal/office/service ./internal/office/shared \
  ./internal/runs/service ./internal/runs/repository/sqlite ./internal/workflow/engine
python3 scripts/lint-spec-files.py --all
```

## Results

Implemented across the branch's causation, ceiling, and depth-gate commits,
with a review-fixup pass in commit `0dd4a781e89452822d0989aa9e7fa24e5c5a4107`
that: wired a runs service into the scheduler test harness so every
`newChildrenCompletedTestScheduler`-based test enqueues through the
authoritative seam; moved windowed-dedup metric reporting into
`runs/service.enqueueLocked`'s own dedup branch so it is correct for that
sole authoritative path; and rewrote the enqueue-consolidation guard test to
scope its allowlist per function instead of per file, adding a regression
test for that scoping. All focused package tests above pass.

Maintainer fixup commit `5015b0fdfeefdfd1cc34c666d98ae25d0e4f127d` extends the
workspace rule for task-bound global Kanban profiles, preserves the acting
agent and source task across causation boundaries, maps typed runtime refusals
to a safe conflict response, and documents the nine Office launch-safety
startup settings.

Post-fixup verification passed:

```text
go test ./internal/runs/service ./internal/runs/repository/sqlite
go test ./internal/office/service ./internal/office/runtime
go test ./internal/workflow/engine ./internal/backendapp
KANDEV_INTERNAL_CONFIG_FILE= KANDEV_INTERNAL_CONFIG_HOME_FILE= go test ./internal/common/config
node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```
