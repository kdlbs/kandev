---
id: "01-share-task-activity-icons"
title: "Share task activity icons"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMMAND-PANEL-TASK-ACTIVITY-001
acceptance_criteria:
  - AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.1
  - AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.2
  - AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.3
  - AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.4
  - AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.5
  - AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.6
  - AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.7
system_design:
  - ../../specs/ui/system-design/command-panel-task-activity-icons.md
---

# Task 01: Share Task Activity Icons

## Summary

Use one task-state icon component for the sidebar and command panel. Keep command-panel results current with the newest bounded task projection.

## In scope

- Extract and reuse the sidebar task-state icon.
- Map HTTP and store task data into the shared icon input.
- Reconcile status summaries by revision.
- Pass the effective workflow's final step ID into the shared icon and preserve live foreground-activity clears.
- Add focused component, desktop, and phone tests.

## Out of scope

- Backend or protocol changes.
- Search, badge, metadata, layout, or navigation changes.
- New task-state rules.

## Acceptance

- Running and preparing tasks show the sidebar spinner in command-panel results.
- Other tasks show the matching sidebar icon, and live updates change the icon without a new search.
- The option remains accessible, selectable, and readable on desktop and phone layouts.

## Verification

```bash
cd apps/web && pnpm exec vitest run components/command-panel-content-search.test.tsx components/command-panel-task-activity.test.tsx components/task/task-item.test.tsx lib/task-status-summary.test.ts
cd apps/web && pnpm e2e:run tests/command-panel.spec.ts -- --grep "task activity icon"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/search/mobile-command-palette-scopes.spec.ts -- --grep "task activity icon|palette scope strip"
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans apps/web
```

## Files likely touched

- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/task-state-icon.tsx`
- `apps/web/components/command-panel.tsx`
- `apps/web/components/command-panel-results.tsx`
- `apps/web/components/command-panel-content-search.test.tsx`
- `apps/web/components/command-panel-task-activity.test.tsx`
- `apps/web/components/task/task-item.test.tsx`
- `apps/web/lib/commands/task-result-activity.ts`
- `apps/web/e2e/tests/command-panel.spec.ts`
- `apps/web/e2e/tests/search/mobile-command-palette-scopes.spec.ts`

## Dependencies

None.

## Risks

- A stale HTTP response can replace a newer WebSocket state without a revision guard.
- An incomplete extraction can change the sidebar icon priority.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-COMMAND-PANEL-TASK-ACTIVITY-001`
- `docs/specs/ui/system-design/command-panel-task-activity-icons.md`
- Existing sidebar and command-panel icon tests.

## Results

- Extracted the sidebar task-state icon into a shared component without changing its priority or background-work tooltip.
- Replaced the command-panel hammer/archive glyph with the shared task-state icon and localized accessible state labels.
- Reconciled task-search results with live workflow snapshots by task timestamp and status-summary revision.
- Preserved final-step workflow completion and accepted live projections that clear foreground activity, including legacy projections without `updatedAt`.
- Added focused stale/live projection tests plus desktop and phone Playwright coverage.
- Passed all verification commands in this work order, including the remediation checks after PR review.
