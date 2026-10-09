---
id: "02-task-entry"
title: "Reconcile canvases on task entry"
status: done
wave: 2
depends_on:
  - "01-runtime-responses"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-006
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-001.2
  - AC-CANVASES-AGENT-WEB-APPS-001.13
  - AC-CANVASES-AGENT-WEB-APPS-001.14
  - AC-CANVASES-AGENT-WEB-APPS-001.15
  - AC-CANVASES-AGENT-WEB-APPS-001.16
  - AC-CANVASES-AGENT-WEB-APPS-006.1
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
  - ../../specs/canvases/system-design/task-entry-presentation.md
---

# Task 02: Reconcile canvases on task entry

## Summary

Show unseen published task canvases from the HTTP inventory after task entry.
Keep live publication working and respect closed panels across same-tab reloads.

## In scope

- First add a failing hook test: empty lifecycle hints, one published HTTP
  canvas, settled target layout, and no receipt must open the main editor panel.
- Enable the task-page inventory query on desktop and share it with activation.
  Refresh after reconnect. Keep failed or stale requests distinct from valid data.
- Filter every candidate by current scope, task, workspace, status, and release.
  Reuse existing pending-permission and active-release metadata rules.
- Wait for `isRestoringLayout` to clear and the target environment to match.
  Recheck generations before insertion and never mark an abandoned attempt offered.
- Add tab-local presentation receipts using the PR-panel offered-marker pattern.
  Include user/workspace/task/canvas identity, detect duplicated session-storage
  namespaces, and keep a memory fallback for storage errors.
  Record manual opening, existing restored panels, and successful automatic opening.
- Add a batch once, in creation-time/ID order, and focus the last new panel once.
  Preserve existing panel placement and active state.
- On phones, confirm route entry before recording the candidate batch offered.
  Back must return to the task without another automatic redirect. Keep all
  other canvases available in the existing picker.
- Update public task-entry documentation and relevant historical plan links.

## Out of scope

Cross-device history, new navigation controls, new translated copy, automatic
workspace-canvas opening, and backend canvas persistence changes are excluded.

## Acceptance

1. Publication before navigation and publication during an open task both present
   eligible canvases automatically. Reload and reconnect require no retained event.
2. Closed or existing panels never cause repeated focus changes. Wrong-task,
   stale, disabled, draft-only, archived, removed, and invalid candidates do not open.
3. Desktop uses the main editor group after restoration. Phone navigation occurs
   once and Back remains usable. Manual access remains available after dismissal.

## ASCII UI preview

UI-01 and UI-02, from the [full preview](plan.md#ascii-ui-preview):

```text
Desktop: [Agent] [Plan] [Task coordinator *] [+]
         Existing toolbar, then full-height canvas

Phone:   Existing header and host actions
         Focused canvas route
         Back -> task without redirect loop
```

These views implement `AC-CANVASES-AGENT-WEB-APPS-001.13` through `.16` and
`AC-CANVASES-AGENT-WEB-APPS-006.1`. Use existing host geometry and touch controls.

## Verification

Add focused tests for mixed eligible/ineligible inventory, duplicate hints,
multiple candidates, stale HTTP results, failed requests, restoration races,
manual reopening, reload receipts, reconnect, storage failures, and identity changes.
Tests must cover pending permission without mounting an executable iframe.
Rendered tests seed publication before navigation, then prove panel placement
and phone Back behavior. Preserve existing live-publication scenarios.

Run from the repository root after Task 01 installs dependencies:

```bash
(cd apps/web && pnpm exec vitest run components/task/dockview-canvas-activation.test.tsx hooks/domains/task/use-task-canvases.test.ts lib/canvas-presentation-storage.test.ts lib/canvas-presentation-storage.tab.test.ts components/settings/canvas-host-route.test.tsx components/task/dockview-add-panel-items.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/canvas/plugin-canvas.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/task-page-content.tsx`
- `apps/web/components/task/task-layout.tsx`
- `apps/web/components/task/dockview-canvas-activation.ts`
- `apps/web/components/task/dockview-canvas-reconciliation.ts` (new)
- `apps/web/components/task/dockview-canvas-activation.test.tsx`
- `apps/web/hooks/domains/task/use-task-canvases.ts`
- `apps/web/hooks/domains/task/use-task-canvases.test.ts`
- `apps/web/lib/canvas-presentation-storage.ts` (new)
- `apps/web/lib/canvas-presentation-storage.test.ts` (new)
- `apps/web/lib/canvas-presentation-storage.tab.test.ts` (new)
- `apps/web/components/settings/canvas-host-route.tsx`
- `apps/web/e2e/tests/canvas/plugin-canvas.spec.ts`
- `apps/web/e2e/tests/canvas/mobile-plugin-canvas.spec.ts`
- `docs/public/canvases.md`

## Dependencies

Task 01 establishes runtime recovery and the workspace install. There is no
schema dependency, but this package runs sequentially.

## Risks

Old canvases have no receipts and can appear once on the first corrected visit.
Record only successful presentation. An interrupted navigation must not consume
an offer. New tabs intentionally have independent presentation history, including
when a browser duplicates a session-storage namespace.

## Parallelism

`sequential`

## Inputs

- Canvas design: Task-entry reconciliation and Mobile design contract.
- `useTaskCanvases`, `canvasLifecycleActivationDecision`, and `fallbackGroupPosition`.
- PR-panel receipt pattern in `apps/web/lib/local-storage.ts`.
- Existing task and canvas host mobile surfaces and E2E fixtures.

## Results

Implemented authoritative task inventory reconciliation for desktop and phone
entry, tab-local presentation receipts, delayed main-group placement, manual
presentation recording, and one-way phone navigation. Existing panels retain
their placement and active state. Failed inventories remain distinct from an
authoritative empty inventory, and reconnect/lifecycle revisions trigger a
fresh inventory request. Pending inventory requests are scoped by lifecycle
generation and authenticated identity, so an invalidated request cannot satisfy
a later entry. Desktop insertion rechecks the current task environment and
layout ownership after asynchronous hint lookups, so a canvas cannot enter a
previous task's settled layout. Hydrated task-session metadata supplies the
environment during the interval before the active-session mapping is available.
Hint lookups are generation-scoped, and duplicated session-storage namespaces
rotate their tab identity before receipts are read. The shared discoverable
canvas status predicate now serves both the task panel menu and reconciliation.

Recorded verification:

- Focused Vitest suite passed: 6 files, 61 tests, including deferred
  reconnect/lifecycle invalidation, authenticated identity, mixed inventory,
  delayed restoration, ownership recheck, batch focus, restored-panel receipts,
  duplicated-tab receipt isolation, and reload persistence.
- `pnpm run lint` passed with zero warnings.
- `pnpm run i18n:check` passed.
- `pnpm run typecheck` passed.
- `pnpm e2e:run --project chromium tests/canvas/plugin-canvas.spec.ts` passed, 6 tests,
  including the inventory/restoration boundary before the no-reopen assertion.
- `pnpm e2e:run --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts` passed, 4 tests.
- `pnpm --filter @kandev/web build:vite` passed.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` passed.
