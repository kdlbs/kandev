---
id: "01-refresh-workflow-step-badges"
title: "Refresh workflow-step badges"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMMAND-PANEL-TASK-ACTIVITY-001
acceptance_criteria:
  - AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.4
  - AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.5
  - AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.6
  - AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.8
system_design:
  - ../../specs/ui/system-design/command-panel-task-activity-icons.md
---

# Task 01: Refresh Workflow-Step Badges

## Summary

Make each command-panel task result derive its workflow-step badge from the
same timestamp-accepted task projection used by its sidebar-parity icon.

## In scope

- Return the effective workflow-step ID from
  `resolveTaskResultActivity`.
- Use that ID for the existing `stepMap` badge lookup.
- Add RED-GREEN-REFACTOR component coverage for newer and stale live steps.
- Extend the desktop live-move command-panel scenario and retain the mobile
  parity assertions.

## Out of scope

- Backend or WebSocket changes.
- Search ranking, badge styling, navigation, or row geometry changes.
- A new placement freshness policy.

## Acceptance

- A newer accepted live task placement updates the open command-panel badge
  without a new search.
- A stale live placement cannot overwrite the HTTP placement.
- The badge keeps its current name/color presentation and the result remains
  one selectable row on desktop and phone layouts.

## Verification

```bash
cd apps/web && pnpm exec vitest run components/command-panel-task-activity.test.tsx components/command-panel-content-search.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint --max-warnings 0 components/command-panel-results.tsx lib/commands/task-result-activity.ts components/command-panel-task-activity.test.tsx
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/command-panel.spec.ts -- --grep "workflow step badge|task activity icon"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/search/mobile-command-palette-scopes.spec.ts -- --grep "task activity icon|palette scope strip"
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans apps/web
```

## Files likely touched

- `apps/web/lib/commands/task-result-activity.ts`
- `apps/web/components/command-panel-results.tsx`
- `apps/web/components/command-panel-task-activity.test.tsx`
- `apps/web/e2e/tests/command-panel.spec.ts`
- `apps/web/e2e/tests/search/mobile-command-palette-scopes.spec.ts`

## Dependencies

None. The live Kanban projection, step map, and command-panel result wiring
already exist.

## Risks

- The implementation must not bypass `currentLiveTask` or revive an older
  search placement when a live projection explicitly clears activity.
- The desktop test must wait for the WebSocket-backed result rerender.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-COMMAND-PANEL-TASK-ACTIVITY-001.4`, `.5`, `.6`, and `.8`.
- `docs/specs/ui/system-design/command-panel-task-activity-icons.md`.
- `docs/decisions/2026-08-17-separate-task-activity-from-summary-freshness.md`.
- Existing command-panel activity and workflow-step badge tests.

## Results

RED reproduced the stale badge before the fix. The resolver now exposes the
timestamp-accepted workflow-step ID and the result row uses it for the existing
step badge. Focused Vitest, typecheck, ESLint, i18n, desktop Chromium E2E,
mobile Chrome E2E, specification lint, and diff checks passed. No blockers.
