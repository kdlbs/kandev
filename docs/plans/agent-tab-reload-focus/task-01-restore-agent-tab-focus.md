---
id: "01-restore-agent-tab-focus"
title: "Restore Agent tab focus after reload"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-AGENT-TAB-RECONCILIATION-001
acceptance_criteria:
  - AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.2
  - AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.6
system_design:
  - ../../specs/ui/system-design/task-agent-tab-reconciliation.md
---

# Task 01: Restore Agent Tab Focus After Reload

## Summary

Adopt one valid restored Agent selection before normal Dockview tab-event
synchronization starts. Keep the current safety guard and prove the task reload
flow through the desktop UI.

## In scope

- Add a focused failing unit regression for restored Agent selection.
- Resolve one valid group-selected current session after environment layout restore.
- Apply the restored session through `setActiveSessionAuto`.
- Add a desktop Chromium regression to the existing multi-session specification.

## Out of scope

- Backend boot payload changes.
- New persistence, URL parameters, API contracts, or store fields.
- Changes to saved layout geometry or non-Agent panel focus.
- Mobile and tablet UI changes.

## Acceptance

- A valid selected secondary Agent panel becomes the effective session after reload without creating a user pin.
- Stale, cross-task, cross-environment, and ambiguous saved selections keep the normal boot fallback.
- The selected secondary Agent tab and its conversation remain visible after a full desktop task reload.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/dockview-session-tab-sync.test.ts components/task/dockview-layout-restore.test.ts components/task/dockview-session-tab-activation.test.ts components/task/dockview-session-tabs.hook.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --host tests/session/multi-session-ux.spec.ts -- --grep "reload restores the selected Agent tab")
git diff --check
```

Make sure that Playwright discovers and executes one Chromium scenario.

## Files likely touched

- `apps/web/components/task/dockview-desktop-layout.tsx`
- `apps/web/components/task/dockview-session-tab-sync.ts`
- `apps/web/components/task/dockview-session-tab-sync.test.ts`
- `apps/web/e2e/tests/session/multi-session-ux.spec.ts`

## Dependencies

None.

## Risks

- Dockview can keep one selected panel per group while another group owns global focus.
- Restored session panels can be stale before current membership validation.
- Event-order changes can reintroduce automatic user pins.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/task-agent-tab-reconciliation.md`
- `docs/specs/ui/system-design/task-agent-tab-reconciliation.md`
- Existing Dockview restore, activation-intent, and multi-session test patterns.

## Results

Completed.

- Added restored Agent-selection adoption before normal Dockview tab-event synchronization
  and after delayed environment-layout restoration.
- Validated current task membership and environment ownership before changing the active session.
- Filtered stale selected panels before deciding whether the valid selection is ambiguous.
- Used `setActiveSessionAuto` so page restoration does not create a user pin.
- Added unit coverage for valid, stale, cross-task, cross-environment, ambiguous,
  mixed stale-and-valid, and delayed-restoration layouts.
- Added a desktop Chromium reload regression and captured the restored secondary-tab state.
- Verification passed: 47 focused unit tests, TypeScript typecheck, and one Chromium E2E test.
