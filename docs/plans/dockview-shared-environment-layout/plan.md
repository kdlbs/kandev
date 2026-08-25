---
spec: docs/specs/ui/requirements/task-layout-profiles.md
created: 2026-07-27
status: implemented
---

# Implementation Plan: Preserve Dockview Layouts Across Shared-Environment Task Switches

## Overview

Make the desktop session-tab handoff atomic when two tasks share the same environment. Today the environment switch correctly reuses the live Dockview layout, but task reconciliation closes the outgoing task's only session panel before adding the incoming panel. Dockview then destroys the empty center group, and the fallback placement creates a nested horizontal split beside the upper-right group while Terminal remains a full-width bottom row. That malformed tree is subsequently persisted for the shared environment.

The fix will add the incoming session panel at the outgoing session panel's group and tab index before stale session panels close. It will preserve same-environment layout reuse, user-customized groups, tab order, and proportions.

## Frontend

Likely files:

- `apps/web/components/task/dockview-session-tabs.ts`
- `apps/web/components/task/dockview-session-tabs.test.ts`

Update `reconcileRemovedSessionPanels` to snapshot stale session panels and, when the incoming session belongs to the active task but is not yet mounted, call the existing session-panel creation path at the first stale panel's live group and tab index before closing any stale panels. The incoming panel should be created inactive so the existing activation policy remains authoritative.

Keep the same-environment no-op in `switchEnvLayout`. Rebuilding the environment layout would discard task-specific customizations and extra environment-scoped panels. Keep the later `ensureSessionPanel` call idempotent so the normal activation, placeholder removal, ordering, and sibling-panel behavior still runs.

The handoff should mirror the atomic replacement already used by `replaceStaleSessionPanels` during cross-environment restoration. If a small shared helper improves clarity without introducing a dependency cycle, reuse it; otherwise keep the session-tabs change local and behaviorally equivalent.

## Tests

### Unit RED

Extend the session-tabs fake Dockview model so closing the last panel destroys its group, matching Dockview behavior. Add a regression test with:

- an outgoing session as the only panel in `group-center`;
- Files or Changes in `group-right-top`;
- Terminal in `group-right-bottom`;
- an incoming session that is current for the selected task but not yet mounted.

The test must first fail by observing the center group disappear or by recording `close:outgoing` before `add:incoming`. The passing assertion will require the incoming panel to be added to `group-center` at the outgoing tab index before the stale close and require the group to remain live.

Also retain existing coverage for sessionless tasks, multiple stale sessions, already-mounted incoming sessions, and activation behavior.

## E2E

Likely files:

- `apps/web/e2e/tests/layout/saved-layout-session-isolation.spec.ts`
- `apps/web/e2e/pages/session-page.ts` only if a reusable geometry assertion is warranted

Create a parent task and a related task whose active sessions resolve to the same environment, then navigate between them through the task UI. Assert after the handoff and after reload that:

- only the selected task's session panel is present;
- the Dockview root orientation remains horizontal;
- the right-side Files or Changes group remains above Terminal rather than Terminal becoming a full-width root row;
- the center session group remains present;
- the environment-scoped saved layout does not adopt the malformed vertical-root tree.

The test should seed state through API helpers and perform the task switch through visible UI. It should explicitly verify that the two sessions share an environment so a fixture regression cannot turn this into a cross-environment test.

## Mobile and Tablet

No mobile or tablet E2E is required for this repair. `TaskLayout` renders `SessionMobileLayout` on phones and `SessionTabletLayout` on tablets; only the desktop workbench mounts Dockview and executes this session-tab reconciliation path. The existing mobile and tablet behavior remains unchanged, as required by the task-layout-profiles spec.

## Implementation Waves

Wave 1:

- [x] [task-01-atomic-session-handoff](task-01-atomic-session-handoff.md)

## Verification

RED and focused unit verification:

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/dockview-session-tabs.test.ts
```

Related Dockview unit verification:

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/dockview-session-tabs.test.ts lib/state/dockview-env-switch-action.test.ts
```

TypeScript verification:

```bash
cd apps/web && pnpm run typecheck
```

Managed production E2E:

```bash
cd apps/web && pnpm e2e:run tests/layout/saved-layout-session-isolation.spec.ts
```

## Risks

- Adding the incoming panel before stale cleanup must happen only when that session belongs to the selected task; otherwise an unhydrated or cross-task panel could be introduced.
- The incoming panel must inherit the stale panel's exact live group and tab index, including user-created groups, rather than assuming `group-center`.
- Creating the panel inactive is important because Files or Changes may be the user's active panel during a task switch.
- Multiple stale session groups are already an ambiguous contaminated state. As in cross-environment restoration, only the first stale group can be re-anchored to one active session; all other stale session panels should still close.
- Geometry assertions must inspect Dockview's tree semantics, not merely positive width and height values, because a malformed vertical root can still report valid dimensions.

## Documentation Impact

No public documentation changes are required. This repair restores the already-specified desktop layout persistence behavior and does not change a public command, configuration key, API, or user-facing workflow.
