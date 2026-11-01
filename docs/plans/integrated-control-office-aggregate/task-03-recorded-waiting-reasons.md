---
id: integrated-control-office-aggregate-task-03
title: Recorded waiting reasons
status: in-progress
wave: 3
depends_on:
  - integrated-control-office-aggregate-task-02
plan: plan.md
requirements:
  - REQ-UI-INTEGRATED-CONTROL-004
acceptance_criteria:
  - AC-UI-INTEGRATED-CONTROL-004.1
  - AC-UI-INTEGRATED-CONTROL-004.2
  - AC-UI-INTEGRATED-CONTROL-004.3
  - AC-UI-INTEGRATED-CONTROL-004.4
system_design:
  - ../../specs/ui/system-design/integrated-control-navigation.md
legacy_specs: []
---

# Task 03: Recorded waiting reasons

## Scope

- New pure derivation `apps/web/components/integrated-control/control-wait-reasons.ts`.
- New presentational panel `apps/web/components/integrated-control/control-wait-reasons-panel.tsx`,
  mounted inside `ControlNativeComparison` after the two comparison tables and before
  the per-workspace read-state lines.
- Reuses the Native dashboard rows that the comparison already read. No new
  endpoint, timer, server field, write, action, or runtime flag; the host surface
  stays behind the existing `features.integratedControl` and `features.office`
  gates.
- Prerequisite fix in `control-aggregate-overview.tsx`: type the
  `figures.map` callback return value as `ColumnDef<WorkspaceAggregateEntry>` so
  the existing comparison table typechecks. This is a same-commit fix to
  existing #4628 code, not new behavior.

## Derivation

All four figures are recorded task counts from one workspace-scoped Native
dashboard read.

- Input wait: `waiting_input_tasks`, the unfinished tasks whose deterministic
  primary session waits for input.
- Blocked: recorded `BLOCKED` task state, summed from `task_stages` when present
  and from the state histogram otherwise.
- Approval wait: `stage_type=approval` tasks, excluding terminal/failed states
  and excluding `BLOCKED` tasks.
- On hold: `stage_type=hold` tasks, same exclusions.

`BLOCKED` takes precedence over the stage reasons so one task is not counted
twice across them. Input wait and the stage reasons remain independent
observations: one task can appear under both, so the panel is not a partition
and shows no total. Without `task_stages`, approval and hold are unknown, never
zero. An invalid recorded count makes that reason unknown instead of a partial
sum. A missing, denied, loading, or failed read is unknown, never a measured
zero. Capacity and slot waits are not recorded in this surface and stay out;
review and unconsumed messages already have their own comparison columns.

## UI

- Located after the task and session comparison tables, before the read-state
  lines, in the Overview Office/Native branch.
- One row per authorized workspace with its name linking to
  `/control/workspaces?workspace=<id>`.
- Four labelled figures per row; phone widths use a two-column wrap with no page
  overflow and retain the existing 44px link target.
- A basis note states that the figures are recorded task counts and independent.
- Copy lives in `sidebar:controlSummary.waitReasons.*`: en and ko written by
  hand, ja, pt-pt, and zh-cn translated, zh-hk and zh-tw from
  `pnpm run i18n:zh-hant`, pseudo from `pnpm run i18n:pseudo`. No em dash.

## Verification

No new permanent test, CI job, or e2e assertion. New behavior is checked once in
a temporary overlay outside regular collection and removed after verification;
existing valid checks are reused.

- One-off: a temporary specification rendering the derivation and the panel,
  covering blocked precedence, terminal/failed exclusion, absent
  `task_stages` (unknown, not zero), invalid matching counts (unknown, not a
  partial sum), unavailable read, links and values.
- Existing unit suites under `components/integrated-control` and
  `hooks/domains/coordinator/use-control-aggregate.test.tsx` remain the
  regression check.
- `pnpm run typecheck` (with `NODE_OPTIONS=--max-old-space-size=8192` on this
  worktree), changed-file ESLint, `pnpm run i18n:check`,
  `python3 scripts/lint-spec-files.py --all`, and
  `python3 scripts/list-docs.py validate`.
- Existing browser specs are read-only reuse; no new assertion is added.

## Boundaries

- No upstream push, PR creation, or comment; the fork branch update is a
  separate approval immediately before push.
- No capacity, provider, conversation, execution, recovery, or ledger data is
  added.
- No new runtime flag; the existing default-off feature gates are unchanged.
