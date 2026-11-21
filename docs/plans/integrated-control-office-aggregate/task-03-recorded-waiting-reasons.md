---
id: integrated-control-office-aggregate-task-03
title: Recorded waiting reasons
status: done
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
  `/control/overview?workspace=<id>`. The selected overview filters its tables
  and totals to that workspace while preserving the same authorization list.
- Four labelled figures per row; phone widths use a two-column wrap with no page
  overflow and retain the existing 44px link target.
- A basis note states that the figures are recorded task counts and independent.
- Copy lives in `sidebar:controlSummary.waitReasons.*`: en and ko written by
  hand, ja, pt-pt, and zh-cn translated, zh-hk and zh-tw from
  `pnpm run i18n:zh-hant`, pseudo from `pnpm run i18n:pseudo`. No em dash.

## Verification

Permanent regression tests cover the Native API, read state, parser, stage
counts, waiting reasons, and workspace navigation.

- Backend repository query: workspace isolation, archived/ephemeral/automation
  exclusions, deterministic primary-session selection, queued-message scope,
  inclusive 24-hour bounds, distinct completed tasks, and stage groups.
- Backend handler: the existing authorized workspace route selects the Native
  projection and accepts only the supported 24-hour window.
- Web parser and derivations: workspace/window contract, malformed counts,
  blocked precedence, terminal/failed exclusion, and unknown versus zero.
- Web read hook: same-row stale retention after transient errors, access-denial
  clearing, and response rejection after the workspace identity changes.
- Component and browser coverage: selected workspace content after clicks on
  the Native table link, table row, and waiting-reason link on desktop and phone.
- `pnpm exec vitest run lib/control-ui/native-control.test.ts components/integrated-control/control-stages.test.ts components/integrated-control/control-wait-reasons.test.ts hooks/domains/coordinator/use-control-native.test.tsx components/integrated-control/integrated-control-page.test.tsx hooks/domains/coordinator/control-refresh-scheduler.test.ts hooks/domains/coordinator/use-control-aggregate.test.tsx hooks/domains/coordinator/use-integrated-control-data.test.tsx` (8 files, 44 tests passed).
- `go test ./internal/office/repository/sqlite -run 'Test(ReadControlMetricsScopesCountsAndAnchorsTheHistoryWindow|CountAgentInstancesByWorkspaceIDsBatchesAndFiltersRows)' -count=1`.
- `go test ./internal/office/dashboard -run 'Test(GetWorkspacesAggregate|ControlDashboardUsesTheAuthorizedWorkspaceRoute)' -count=1`.
- `pnpm e2e:run --host --shards 1 --project chromium tests/coordinator/control-aggregate-dashboard.spec.ts` (2 tests passed).
- `pnpm e2e:run --host --shards 1 --project mobile-chrome tests/coordinator/mobile-control-aggregate-dashboard.spec.ts` (2 tests passed).
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passes.
- Targeted changed-file ESLint, changed-Go lint, and Prettier checks pass.

## Boundaries

- The original task required separate authorization before an upstream push or
  comment. The user authorized this maintainer follow-up and asked for a commit,
  push, and one concise PR comment.
- No capacity, provider, conversation, execution, recovery, or ledger data is
  added.
- No new runtime flag; the existing default-off feature gates are unchanged.
