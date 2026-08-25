---
spec: docs/specs/ui/requirements/task-layout-profiles.md
created: 2026-07-28
status: implemented
---

# Implementation Plan: Reconcile the Agent Panel After Dockview Restore

## Root cause

The environment restore path replaces stale saved session tabs, but it does not
repair a restored layout that contains no session tab at all. The later React
session-tab effect usually fills that gap, while the remove-panel safety net is
suppressed during `isRestoringLayout`. If no dependency changes after the
restore, the desktop workbench can retain an empty center group.

The active-tab trace is a related but distinct rule: saved non-session tabs such
as Plan are intentionally restored and must remain active. The repair therefore
must add Agent without overriding a valid saved active tab.

## Wave 1

- [x] [task-01-post-restore-session-reconciliation](task-01-post-restore-session-reconciliation.md)

## Design

Add a post-restore reconciliation helper beside the existing stale-session
replacement logic. When an active session has no live `session:*` panel, place
it in this order:

1. the canonical center group;
2. the group containing a contextual Plan/PR/MR tab;
3. the first remaining center-candidate group;
4. the existing sidebar-relative fallback.

Activate the restored Agent only when its target group was empty or no panel is
active. Otherwise add it inactive so a valid saved Plan, Files, or Changes tab
retains focus. Continue restoring sibling sessions as inactive tabs beside the
active session.

Invoke the reconciliation synchronously before `isRestoringLayout` clears so
the workbench cannot expose the empty group. Preserve the atomic stale-session
handoff and existing active-view restoration order. Do not run the missing
Agent repair for a saved maximize overlay because maximizing Terminal, Files,
or another non-Agent group intentionally excludes Agent.

## Tests

RED unit coverage in `apps/web/lib/state/dockview-env-switch.test.ts`:

- a slow-path restore with an empty center group and no saved session panel
  must add and activate the incoming Agent in that group;
- a slow-path restore with Plan active and no saved session panel must add
  Agent to Plan's group without activating it.

Desktop Playwright coverage extends
`apps/web/e2e/tests/layout/saved-layout-session-isolation.spec.ts` to assert
that task switching never exposes the Dockview empty-group watermark and that
the selected task retains a visible Agent tab.

No mobile test is required: phone and tablet task layouts do not mount
Dockview, and the repair changes state normalization only within the desktop
workbench.

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run lib/state/dockview-env-switch.test.ts
cd apps/web && pnpm e2e:run tests/layout/saved-layout-session-isolation.spec.ts
make fmt
make typecheck test lint
```

## Risks and exclusions

- Do not activate Agent over a genuinely saved active Plan or other valid tab.
- Do not rebuild or reshape healthy user-customized groups.
- Do not inject Agent into a deliberately maximized non-Agent group.
- Do not change mobile/tablet task composition.
- Network failures fetching PR, commit, or diff data remain out of scope.

## Result

- RED reproduced both missing Agent outcomes after a normal slow-path restore.
- GREEN restores Agent into the center group, activates it only for an empty
  target, and preserves saved Plan focus.
- A second RED regression prevented the repair from affecting maximized
  non-Agent groups.
- Focused unit tests, the production-build Playwright spec, formatting,
  typecheck, full tests, and lint passed on 2026-07-28.
