---
id: "01-release-departed-owners"
title: "Release editor subscriptions and departed Dockview ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-004
acceptance_criteria:
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.3
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
---

# Task 01: Release departed owners

## Outcome and scope

Return the matching unsubscribe cleanup for the client acquired by `useEditors`.
Track the API acquired by each desktop layout. Clear shared API and portal
ownership on unmount only when that API remains current. Local listener and
pending-save cleanup continues for every layout.

Owned production files are `apps/web/hooks/domains/settings/use-editors.ts`,
`apps/web/components/task/dockview-desktop-layout-hooks.ts` and
`apps/web/components/task/dockview-desktop-layout.tsx`. Focused tests and the
owner-release browser regression accompany them. Extract the existing sidebar
size synchronization effect into a local hook to meet the function-size limit.

This work reuses the reviewed source package and the existing
[UI lifecycle design](../../specs/ui/system-design/session-refresh-efficiency.md).
It excludes comparator, scroll, plan-editor and benchmark changes. No dependency
on the other extracted contributions is required.

## Acceptance

1. Editor mount, rerender, repeated remount, absent-client and Strict Mode paths
   balance acquired references and preserve the independent root reference.
2. Departed Dockview cleanup releases its API, portals, listeners and pending
   saves. Cleanup from a replaced or never-ready layout preserves the new owner.
3. Same-document Settings navigation and responsive transitions release the API.
   Desktop and phone task navigation preserve drafts and existing layout behavior.

## Verification

From `apps/web`, using pnpm 9.15.9. For browser commands launched from a Kandev
session, remove inherited `KANDEV_INTERNAL_CONFIG_FILE` and
`KANDEV_INTERNAL_CONFIG_HOME_FILE`, and set `KANDEV_SERVER_HOST=127.0.0.1`.
Also remove `KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG`, `KANDEV_PARENT_PIPE_FD`,
`KANDEV_SUPERVISOR_MANIFEST` and `KANDEV_SUPERVISOR_SOCKET`. This keeps the
disposable backend independent of the parent instance config. On this Nix host,
Chromium also requires the installed browser libraries on `LD_LIBRARY_PATH`.

```bash
pnpm exec vitest run hooks/domains/settings/use-editors.test.ts components/task/dockview-desktop-layout-hooks.test.tsx
pnpm exec vitest run lib/state/dockview-env-switch.test.ts lib/state/dockview-env-switch-pinned.test.ts lib/state/dockview-env-switch-active-views.test.ts
pnpm exec eslint --max-warnings 0 hooks/domains/settings/use-editors.ts hooks/domains/settings/use-editors.test.ts components/task/dockview-desktop-layout-hooks.ts components/task/dockview-desktop-layout-hooks.test.tsx components/task/dockview-desktop-layout.tsx e2e/tests/layout/dockview-owner-release.spec.ts
NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck
flock /tmp/kandev-contribution-validation-5936a872.lock pnpm e2e:run --project chromium tests/layout/dockview-owner-release.spec.ts tests/task/task-switch-efficiency.spec.ts
flock /tmp/kandev-contribution-validation-5936a872.lock pnpm e2e:run --project mobile-chrome tests/task/mobile-task-switch-efficiency.spec.ts
```

## Results

Before production changes, seven ownership assertions failed: four editor cases
and three Dockview cases. The 100-cycle editor test retained 201 references
instead of the one independent root reference. Strict Mode also reproduced an
unbalanced reference. These are unit reference counts, not heap measurements.

The combined five-file unit run passed 52 tests. Strict lint and documentation
catalog/specification validation passed. The browser regression checks actual
API panel IDs across three same-document Settings visits and responsive changes.
The first browser attempt failed in fixture setup because an inherited parent
config selected a non-loopback bind. It did not execute product assertions. The next attempt reached backend
readiness but Chromium could not load the Nix host libraries. A process-local
library path corrected this second setup failure.
The first typecheck exhausted the default 4 GB Node heap. The 8 GB retry exposed
a timer type mismatch in a new test, corrected with an explicit browser/Node timer-handle conversion.
The final 8 GB typecheck passed. The fresh production build passed. With the
corrected process environment, all four Chromium checks passed
(`/tmp/kandev-run.e2e.lMZvRZOV.log`) and all three mobile-chrome checks passed
(`/tmp/kandev-run.e2e.Ws5Aaxfw.log`). Both runs used the same verified build.
The final timer-test change passed all four hook tests and strict lint.
Desktop and phone captures were inspected: the draft remained visible and the
existing composition was intact. No actionable UI-polish finding was observed.
Browser assertions establish API ownership release and restoration, not heap
leak freedom or a broad task-switch speed improvement. Public docs need no changes because user procedures,
configuration, visible composition and public contracts remain the same.


CI exposed incomplete WebSocket client doubles in three existing consumer
fixtures. Both frontend shards failed when the new cleanup called the missing
`unsubscribeUser` method. A local run reproduced 26 failures before the fix.
The file-browser, chat-input and plan-draft fixtures now model the matching
subscribe/unsubscribe methods. The combined seven-file consumer and ownership
run passed 81 tests; strict lint and the 8 GB typecheck passed again.

```bash
pnpm exec vitest run components/task/file-browser-move-selection.test.tsx components/task/file-browser-move-settlement.test.tsx components/task/chat/chat-input-area-context-submission.test.tsx hooks/domains/kanban/use-plan-implementation-draft.test.tsx hooks/domains/kanban/use-implement-fresh-draft.test.tsx hooks/domains/settings/use-editors.test.ts components/task/dockview-desktop-layout-hooks.test.tsx
```
