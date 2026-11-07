---
id: integrated-control-office-aggregate-task-04
title: Per-task recorded wait reasons
status: in-progress
wave: 3
depends_on:
  - integrated-control-office-aggregate-task-02
plan: plan.md
requirements:
  - REQ-UI-INTEGRATED-CONTROL-004
acceptance_criteria:
  - AC-UI-INTEGRATED-CONTROL-004.5
  - AC-UI-INTEGRATED-CONTROL-004.6
system_design:
  - ../../specs/ui/system-design/integrated-control-navigation.md
legacy_specs: []
---

# Task 04: Per-task recorded wait reasons

## Scope

- Extend the recorded waiting-reasons surface with the per-task read and render
  that Task 03 left out. Task 03 splits the Native dashboard read; this task adds
  a per-task read of the existing authenticated workspace tasks endpoint and
  names each stopped task's recorded reason.
- New backend read
  `apps/backend/internal/office/repository/sqlite/control_reads.go`:
  `ListControlTasks` projects recorded task and deterministic primary-session
  rows plus the active native tree-hold mode and each queued message's
  current-incarnation session policy.
- New backend route
  `apps/backend/internal/office/dashboard/control_reads.go`: the opt-in
  `view=control` tasks section on the existing `GET /workspaces/:wsId/tasks`
  route. The legacy list payload is unchanged without `view=control`.
- New native read layer: `NativeTask`/`NativeTaskList` DTO, `nativeTasks`
  validator, `readNativeTasks` client, `useNativeTasks` hook.
- New derivation and render: `control-task-wait.ts` `taskWaitReason`, and the
  per-task list added to the waiting-reasons panel.

## Derivation

Reasons come from recorded fields only: hold (`hold_mode`, or hold-stage step),
approval stage, recorded `BLOCKED` state, paused queued messages, queued messages
with a known auto-run policy, queued messages whose policy is unknown, a primary
session waiting for input, and a failed primary session. A task that is finished
or running has no wait reason. Task and step names are never read as signals.

## Unknown preservation

Hold mode and queue-policy counts stay absent (unknown) when their source cannot
be read, never a fabricated zero; a failed optional read is surfaced as an error.
The known/unknown split requires the current session and its incarnation. A
missing or stale policy uses the queue's auto-run default; an unbound legacy
policy retains its stored value. A missing session remains unknown.
The per-workspace list read shows its own loading, denied, unsupported, error,
or dated stale state; it does not turn a missing result into an empty list. If
the non-atomic queued and policy reads disagree, the reason remains unknown.

## Boundaries

- No capacity bars, provider actions, recorded control history, host stats,
  policy writer, or runtime flag.
- No new permanent test infrastructure or test cases; temporary checks exercise
  the existing office repository and dashboard paths.
- The prepared change is an upstream candidate; publication is a separate step.

## Verification

- Backend: `go build`, `go vet`, `gofmt`, existing
  `internal/office/repository/sqlite` and `internal/office/dashboard` tests, and
  temporary focused regression checks.
- Web: `pnpm run typecheck`, changed-file ESLint, `pnpm run i18n:check`, and the
  existing `components/integrated-control` tests.
- Docs: `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`,
  and `validateCoverage` from `.github/scripts/pr-docs.cjs`.

See the [plan](plan.md).
