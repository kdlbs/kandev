---
spec: docs/specs/ui/requirements/task-agent-tab-reconciliation.md
created: 2026-08-26
status: implemented
---

# Implementation Plan: Command-center Task Focus

## Overview

Repair two UI outcomes that follow command-panel task selection. First, make
Agent-tab reconciliation react when Dockview becomes ready after the selected
task's sessions have already hydrated. Second, extend the existing sidebar
reveal with smooth minimum-distance scrolling and a transient row cue that
respects reduced-motion preferences.

Applicable contracts:

- `docs/specs/ui/requirements/task-agent-tab-reconciliation.md`
- `docs/specs/ui/system-design/task-agent-tab-reconciliation.md`
- `docs/specs/ui/requirements/command-panel-sidebar-task-reveal.md`
- `docs/specs/ui/system-design/command-panel-sidebar-task-reveal.md`

## Confirmed root cause

`useAutoSessionTab` reacts to the effective session and the active task's
session-ID key. Its effect calls `runAutoSessionTabEffect`, which reads the
Dockview API through `useDockviewStore.getState()` and returns when that API is
null. The hook does not subscribe to the API value. Route hydration can settle
before Dockview `onReady`. In that order, API publication does not change an
effect dependency. The sibling Agent panels remain absent until another event,
such as reload, runs reconciliation.

A temporary React hook test reproduced this order: hydrate task sessions with
a null API, render the hook, then publish a fake API. The test failed because
the API was never read after publication. The temporary test was removed after
the diagnosis. Task 01 owns the permanent RED regression.

The sidebar already has bounded route-aware reveal behavior, but
`scrollIntoView` has no `behavior` option and the row receives only its normal
active state. The existing UI review-target navigation provides the nearest
reduced-motion and transient-cue precedent.

## Delivery order

Both work orders are frontend-only and have no code dependency. Execute them
sequentially in the primary session because implementation delegation has not
been authorized.

- [x] [Task 01: Reconcile Agent tabs on workbench readiness](task-01-reconcile-agent-tabs-on-readiness.md)
- [x] [Task 02: Animate and mark the command-selected sidebar row](task-02-animate-command-selected-sidebar-row.md)

## Verification strategy

Task 01 uses a hook-level timing regression for the API-late order, existing
pure reconciliation tests, and a user-level Cmd+K multi-session scenario.
Task 02 uses DOM unit tests for scroll options and cue lifecycle, the existing
overflowing-sidebar Playwright scenario, and the existing phone command-panel
navigation regression.

After both work orders pass, run these package-level checks:

1. `cd apps/web && pnpm run typecheck`
2. `cd apps && pnpm --filter @kandev/web lint`
3. `cd apps && pnpm --filter @kandev/web build:vite`
4. `git diff --check`

No new user-facing copy is planned. If implementation adds copy, it must add
all required locale entries and run the repository i18n checks.

## Mobile contract

Phone and tablet keep their existing task route and session controls. They do
not mount desktop Dockview tabs or use the hidden desktop sidebar as a scroll
target. Task 01 preserves the shared session-membership source, and Task 02
retains the existing focused mobile Playwright regression.

## Risks and boundaries

- A broad Dockview subscription can cause redundant reconciliation. Subscribe
  only to API readiness and keep the existing session-key dependency.
- Smooth scrolling makes immediate geometry assertions unreliable. Browser
  coverage must poll for final containment and use unit coverage for the exact
  `scrollIntoView` options.
- Cue cleanup must not let an older timer remove a newer cue.
- Filtered and collapsed rows remain intentional no-ops. The repair must not
  mutate sidebar preferences to expose them.
- No backend API, session lifecycle, saved layout, or public documentation
  contract changes.

## Results

Completed on 2026-08-26.

- Task 01 adds Dockview API readiness to `useAutoSessionTab` dependencies and
  permanently covers the session-hydrated-before-workbench-ready ordering.
- Task 01's focused unit run passed 2 files and 35 tests. Its command-panel
  multi-session Chromium scenario passed 1 test without a page reload.
- Task 02 adds reduced-motion-aware nearest scrolling and a restartable
  transient row cue. Its focused unit run passed 1 file and 10 tests.
- Task 02's desktop reveal scenario passed 1 test, including cue presence,
  nested-viewport containment, active state, and unchanged document scroll.
  The existing mobile command-panel scenario passed 1 test.
- `pnpm run typecheck`, the web lint, and the Vite production build passed.
- The managed E2E runner created disposable backend, fixture-plugin, and web
  build artifacts. None were added to the change.
