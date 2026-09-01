---
created: 2026-09-01
status: done
requirements:
  - REQ-UI-TASK-AGENT-TAB-RECONCILIATION-001
system_design:
  - ../../specs/ui/system-design/task-agent-tab-reconciliation.md
legacy_specs: []
---

# Implementation Plan: Agent Tab Reload Focus

## Overview

Restore the selected current Agent tab after a desktop task reload. First, add
a failing unit regression for saved selection adoption. Then implement the
frontend correction and prove the full reload flow in Chromium.

## Confirmed root cause

A user tab selection updates the in-memory task store. A full reload replaces
that state with the backend boot payload, which selects the primary session.
The Dockview layout retains the selected secondary tab in `sessionStorage`.
However, the session-tab event guard treats restored activation as an automatic
event and redirects it to the boot-selected session.

## Scope

### In scope

- Restore one valid current Agent selection from the environment Dockview layout.
- Align the task store through automatic session selection without a user pin.
- Keep the existing explicit-intent guard for later Dockview activation events.
- Add focused unit and desktop Chromium reload regressions.

### Out of scope

- Backend boot-session selection and session lifecycle behavior.
- New persisted settings, URL state, API fields, or database changes.
- Saved layout geometry and valid non-Agent focus behavior.
- Phone and tablet composition, controls, and selection behavior.

## Technical approach

Add a restoration helper near `setupSessionTabSync`. The helper will inspect
the restored Dockview groups before the normal listener is registered and
whenever a delayed environment-layout restoration completes. It will filter
out invalid selections, then accept only one group-selected `session:<id>` that
belongs to the active task, current session list, and current environment.

Call `setActiveSessionAuto` for a valid restored selection. This call updates
the effective session and `lastSessionByTaskId` without creating a user pin.
Keep `setActiveSession` limited to explicit pointer or keyboard intent.

When saved selection data is stale or ambiguous, keep the boot-selected
session. Do not change the existing fallback or activate a different panel.

## Tests

- `AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.2` remains covered by the existing
  session-tab activation and synchronization unit tests.
- `AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.6` gains focused cases in
  `apps/web/components/task/dockview-session-tab-sync.test.ts`.
- The unit regression covers valid secondary selection, user-pin isolation,
  stale selection, cross-task selection, cross-environment selection, and
  ambiguous selection.

## E2E tests

- Add a Chromium scenario to
  `apps/web/e2e/tests/session/multi-session-ux.spec.ts` for
  `AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.6`.
- Create two current sessions, select the secondary Agent tab, flush the saved
  Dockview layout, and reload the task.
- Verify that the secondary tab remains active and its conversation remains
  visible after hydration.

No mobile Playwright change is required. Phone and tablet do not mount the
desktop Dockview workbench, and this repair changes no shared mobile state or
interaction.

## Work orders

- [x] [Task 01: Restore Agent tab focus after reload](task-01-restore-agent-tab-focus.md)

## Verification results

Passed:

```text
cd apps/web && pnpm exec vitest run components/task/dockview-session-tab-sync.test.ts components/task/dockview-layout-restore.test.ts components/task/dockview-session-tab-activation.test.ts components/task/dockview-session-tabs.hook.test.tsx
Vitest: 47 tests passed in 4 files.

cd apps/web && pnpm run typecheck
TypeScript: passed.

cd apps/web && CAPTURE_PR_ASSETS=true pnpm e2e:run --host tests/session/multi-session-ux.spec.ts -- --grep "reload restores the selected Agent tab"
Playwright: 1 Chromium test passed.

python3 scripts/lint-spec-files.py --all
Specification lint: passed.

git diff --check
Whitespace validation: passed.
```

## Risks

- An unvalidated saved session ID can restore a deleted or unrelated session.
- Calling `setActiveSession` would create a false user pin during boot.
- Selecting from global Dockview focus alone can lose the Agent-group selection
  when a valid non-Agent panel owns global focus.
- A broad event exception can reintroduce automatic session pinning.
