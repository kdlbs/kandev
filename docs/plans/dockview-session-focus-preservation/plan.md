---
spec: docs/specs/ui/requirements/task-layout-profiles.md
created: 2026-07-30
status: completed
---

# Implementation Plan: Preserve Dockview Session Focus During Reconciliation

## Overview

Repair the desktop session-tab reconciliation path that can select Plan while
replacing the generic Chat placeholder. The implementation will preserve the
Agent group's selected content independently from Dockview's globally focused
panel, then prove the exact Files-focused task-switch regression with unit and
production-build browser coverage.

## Confirmed Root Cause

`runAutoSessionTabEffect` records only `api.activePanel`, which is Dockview's
globally active panel. In the reported layout, Files owns global focus while
Chat remains the selected tab in the inactive center group beside Plan.

The effect adds the incoming session inactive, removes the selected Chat
placeholder, and then normalizes session-tab order. Dockview 4.13.1 responds to
removing the selected group tab by opening its most-recent sibling and making
that group globally active. Because the session was appended after Plan, Plan
becomes active. The later activation decision correctly returns
`shouldActivate=false` for the globally focused Files panel, but that branch
does not restore Files or transfer Chat's group-local selection to Agent. The
existing logs already capture the decisive transition
`files -> plan, shouldActivate=false`; no new production logging is required.

## Frontend

### Session placeholder replacement

Likely file:

- `apps/web/components/task/dockview-session-tabs.ts`

Treat the generic Chat placeholder as the session panel's replacement slot,
not merely as a group anchor:

- capture whether Chat is the selected panel in its group separately from
  `api.activePanel`;
- insert the incoming session inactive at Chat's session-first replacement
  position while Chat is still selected, so later tab-order normalization
  does not have to move an active Agent panel past Plan;
- when Chat was selected, establish the incoming Agent as the selected panel
  before removing Chat, so Dockview never chooses Plan as Chat's successor;
- after the Agent group's selection is stable, restore the previously global
  non-session panel when it still exists and the existing activation policy
  says to preserve it;
- retain the current behavior for an already-selected Plan, PR, MR, file
  preview, explicit intra-task session switch, missing panel, and maximized
  layout.

Do not add a delayed correction after Plan activates. Plan activation has an
observable side effect (`PlanTab` marks the plan seen), so the repair must
prevent the transient activation itself.

## Tests

### Unit RED

File:

- `apps/web/components/task/dockview-session-tabs.test.ts`

Extend the fake Dockview model to represent:

- a center group with Chat selected beside Plan;
- a right group with Files globally active;
- Dockview's real behavior where removing the selected group tab opens its
  most-recent sibling and activates that group.

The regression must fail before production changes by showing Plan activation.
The passing assertions require:

- the incoming session is the center group's selected panel;
- Files remains Dockview's global active panel;
- Plan is never present in the recorded activation sequence; and
- the existing Chat-global, saved-Plan, and task/session-switch activation
  cases continue to pass.

### Existing helper coverage

Keep `apps/web/components/task/dockview-session-tab-activation.test.ts` in the
targeted unit command because it owns the surrounding session-activation
policy, even if the pure helper does not require a production change.

## E2E Tests

### Desktop restored-layout regression

File:

- `apps/web/e2e/tests/layout/saved-layout-session-isolation.spec.ts`

Create an agent-authored plan through the mock-agent script, wait for the task
session and plan to be durable, and resolve the task environment ID. Before
navigating to the task, seed that environment's
`kandev.dockview.env-layout-v3.<env-id>` session-storage entry with:

- center `activeView: "chat"` and views `[chat, plan]`;
- right-top `activeView: "files"` and views `[files, changes]`; and
- `activeGroup: "group-right-top"`.

Open the task through the UI, create the agent-authored plan while the page is
subscribed so its unseen indicator is armed, and use the existing E2E store
bridge to replay the late session-list hydration that triggers Agent
reconciliation. Assert afterward that:

- the active session's Agent content is visible rather than Plan content;
- Files remains the globally active Dockview panel;
- the Plan tab is not active and its unseen indicator remains visible; and
- the current session panel is present exactly once.

Use API helpers only for task/plan/layout preconditions and verify the outcome
through the rendered task UI plus the exposed Dockview API for the global-focus
identity.

## Mobile And Tablet

No mobile or tablet test is required. `TaskLayout` renders
`SessionMobileLayout` and `SessionTabletLayout` instead of
`DockviewDesktopLayout`; neither viewport executes `useAutoSessionTab` or the
Chat-placeholder replacement. The closest mobile exemplar is
`apps/web/components/task/mobile/session-mobile-layout.tsx`, whose
single-surface navigation, touch targets, scroll ownership, and task
capabilities remain unchanged. This is state normalization inside the existing
desktop workbench, not a responsive composition or interaction change.

## Implementation Waves

Wave 1 (sequential):

- [x] [Task 01 — Preserve session focus](task-01-preserve-session-focus.md)

## Verification

Unit RED/GREEN and related activation policy:

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  components/task/dockview-session-tabs.test.ts \
  components/task/dockview-session-tab-activation.test.ts
```

Frontend typecheck:

```bash
cd apps/web && pnpm run typecheck
```

Managed production-build desktop E2E:

```bash
cd apps/web && pnpm e2e:run tests/layout/saved-layout-session-isolation.spec.ts \
  -- --grep "preserves Agent selection while Files owns global focus"
```

## Risks

- Dockview distinguishes a group's selected panel from the globally active
  panel. Collapsing those concepts recreates this bug in another group.
- Reacting after Plan activates is insufficient because the Plan tab can mark
  an unseen plan as seen during the transient activation.
- Repositioning the fresh session panel must retain the existing session-first
  tab-order convention and must not move an already-restored session panel.
- Restoring a captured global panel must be conditional on that panel still
  existing; stale cross-task or deleted panels must never be resurrected.

## Out Of Scope

- Changing task-layout persistence, profile schemas, or Dockview versions.
- Altering explicit Plan, PR, MR, preview, Files, or Changes tab selections.
- Changing mobile or tablet task-detail composition.
- Adding more debug logging.

## Documentation Impact

No public documentation changes are required. This repair restores the
existing internal desktop layout-focus contract and changes no command,
configuration key, API, or user-facing terminology.
