---
id: "05-bound-route-boot-data"
title: "Normalize boot data and scope it to the route"
status: complete
wave: 5
depends_on:
  - "03-project-compact-session-data"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-JOURNEY-LOADING-001
acceptance_criteria:
  - AC-PLATFORM-JOURNEY-LOADING-001.1
  - AC-PLATFORM-JOURNEY-LOADING-001.2
  - AC-PLATFORM-JOURNEY-LOADING-001.3
  - AC-PLATFORM-JOURNEY-LOADING-001.4
  - AC-PLATFORM-JOURNEY-LOADING-001.5
  - AC-PLATFORM-JOURNEY-LOADING-001.6
  - AC-PLATFORM-JOURNEY-LOADING-001.7
system_design:
  - ../../specs/platform/system-design/journey-data-loading.md
---

# Task 05: Normalize boot data and scope it to the route

## Summary

Golden boot/decode tests prove unique entities, matching authorized session selection, version-1 compatibility, and omitted optional data recovery.

## In scope

Introduce and decode boot version 2 with unique task/session entities and ID memberships. New frontend accepts legacy version 1. Preserve plugins, auth/runtime, security interlock, and canonical store merge behavior.
Load only the selected board on single-board entry. Make other board containers demand their own snapshots in multi-board mode. For task detail, replace the full workflow snapshot with one existing saved-view sidebar page and compact sibling membership.
Validate requested session membership before loading detail. Preserve authorized primary/eligible fallback, empty tasks, archived views, coverage truthfulness, and optional-resource failures. Update measure_boot.py to understand both versions without losing historical results.

## Out of scope

Other work orders, live installation mutation, unrelated refactors, pool increases, new runtime flags, and deployment.

## Acceptance

- Golden boot/decode tests prove unique entities, matching authorized session selection, version-1 compatibility, and omitted optional data recovery.
- Task detail is at most 512 KiB and single-board boot at most 1.25 MiB on the recorded fixture. Appending unrelated tasks to 10,000 adds no task/session records.
- Desktop and phone preserve sidebar filtering/paging, archived selection, task navigation, and multi-board access without eager hidden-board snapshots.

## ASCII UI preview

[UI-01: full composition and states](plan.md#ascii-ui-preview). Applies to the acceptance IDs in this work order.

```text
UI-01 desktop: [Sidebar page <=100 rows] | [Selected board OR task detail]
UI-01 phone:   [Task/workflow picker] -> [One focused board OR task detail]
               [Existing loading / empty / Retry state in its own region]
```

Existing controls and scroll ownership remain. Detail demand follows visibility, not mounting or keyboard focus. Preview spacing is illustrative.

## Verification

Use TDD. New test filenames and named methods below are planned deliverables, not existing passing evidence.
Run from the repository root. Install `apps/` dependencies first only if absent. Each command is independently rooted.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/webapp ./internal/backendapp -run 'Test(Boot|.*Boot|HandlerInjects|DevHandler|Journey|NormalizeBootPayloadGraph|TaskDetailSidebarQuery)' -count=1)
(cd apps/web && pnpm exec vitest run src/boot-payload.test.ts lib/ssr/session-page-state.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts hooks/domains/kanban/use-swimlane-render-data.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/task/journey-boot-loading.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-journey-boot-loading.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 src/boot-payload.ts src/boot-payload.test.ts lib/ssr/session-page-state.ts hooks/domains/kanban/use-all-workflow-snapshots.ts hooks/domains/kanban/use-swimlane-render-data.ts hooks/domains/kanban/use-swimlane-render-data.test.tsx components/kanban-board.tsx components/kanban/swimlane-container.tsx components/kanban/mobile-column-tabs.tsx lib/kanban/view-registry.ts components/task/task-page-content.tsx e2e/tests/task/journey-boot-loading-helpers.ts e2e/tests/task/journey-boot-loading.spec.ts e2e/tests/task/mobile-journey-boot-loading.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the web typecheck and targeted ESLint for every changed TS/TSX file after the listed tests. If localized copy changes, run `pnpm run i18n:check` and `pnpm run i18n:ratchet` from `apps/web`. Managed E2E commands build fresh assets and enforce resource limits.

## Files likely touched

- `apps/backend/internal/webapp/payload.go`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_journey_test.go (new)`
- `apps/web/src/boot-payload.ts`
- `apps/web/src/boot-payload.test.ts`
- `apps/web/lib/ssr/session-page-state.ts`
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`
- `apps/web/components/kanban/swimlane-container.tsx`
- `apps/web/lib/state/slices/task-overview-normalize.ts`
- `apps/web/e2e/tests/task/journey-boot-loading.spec.ts (new)`
- `apps/web/e2e/tests/task/mobile-journey-boot-loading.spec.ts (new)`
- `docs/plans/journey-data-efficiency/measure_boot.py`
- `apps/web/AGENTS.md (boot version and hydration boundary)`

## Dependencies

[Task 03](task-03-project-compact-session-data.md)

## Risks

Incomplete scope must not appear complete. A full selected entity and compact navigation copy must merge before serialization. Old open clients retain REST compatibility.

## Parallelism

`sequential`

## Inputs

- [Plan, contract inventory, and test mapping](plan.md).
- [Journey loading design](../../specs/platform/system-design/journey-data-loading.md).
- [Measured baseline](evidence.md) and `evidence/` artifacts.
- Read the owned source and nearby tests before the first edit. Preserve existing user changes.

## Results

Complete. Boot version 2 now stores each task/session once and uses ID memberships, while the decoder still accepts version 1. A single-board boot contains only its selected snapshot; the phone picker retains access to unloaded workflows and fetches the selected board on demand. Task detail includes one saved-view sidebar page, compact sibling sessions, one selected full session, and an explicitly incomplete workflow placeholder. Requested sessions are accepted only when they belong to the task. The measurement tool reads both boot versions, and the fixture can append unrelated tasks through 10,000 without changing selected-board membership.

Verification passed: race-enabled backend boot tests; five web Vitest files (92 tests); web typecheck; targeted ESLint; managed Chromium (2 tests) and mobile-Chrome (1 test); two measurement-script unit tests; `python3 scripts/list-docs.py validate`; `python3 scripts/lint-spec-files.py --all`; and `git diff --check`. The first phone E2E exposed that a selected-board-only snapshot also removed other workflows from its picker. The navigator now lists active-workspace workflows with unknown counts for unloaded boards, and the final phone E2E passed.

Measurements are in [`implementation-evidence.md`](implementation-evidence.md) and `evidence/boot-{10,1000,10000}-v2.json`. At 1,000 tasks, the selected board was 1,025,986 bytes and task detail was 315,817–320,269 bytes. At 10,000 tasks, unrelated additions left board membership and task/session entity counts unchanged; detail remained 315,821–320,273 bytes. The 10,000-task detail request timings were 6.7–12.6 seconds versus 0.2–0.4 seconds at 1,000 tasks. This repeatable synthetic-fixture observation is recorded for Task 08 to investigate against its integrated benchmark; shared-host work was active during capture, so it is not attributed to a single query or treated as an SLA.

### Review correction

All-workflow cold boot seeds exactly the first displayed board. Existing complete workflow coverage skips known-empty candidates, while saved column preferences and the phone picker retain their semantics. Compact sibling boot rows preserve runtime status without rich session metadata. New boot and desktop/phone browser regressions cover these corrections. Historical boot measurements above remain unchanged. See the [review correction results](implementation-evidence.md#review-remediation).

### PR fixup

PR #4404 fixup restricts task entity expansion to normalized board, task-page, detail, and sidebar fields. Saved selected-task views, drafts, and ordinary task identities retain their ID fields. The boot parser regression passed with positive controls for normalized collections. See [fixup results](implementation-evidence.md#pr-4404-fixup).
