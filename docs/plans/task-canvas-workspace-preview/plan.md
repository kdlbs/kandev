---
created: 2026-09-23
status: complete
requirements:
  - REQ-CANVASES-WORKSPACE-PREVIEW-001
  - REQ-CANVASES-LOCAL-CREATION-001
  - REQ-CANVASES-AGENT-WEB-APPS-003
system_design:
  - ../../specs/canvases/system-design/task-canvas-workspace-preview.md
  - ../../specs/canvases/system-design/local-creation-authority.md
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
  - ../../specs/plugins/system-design/canvas-workspace-data-scope.md
legacy_specs: []
---

# Implementation Plan: Task canvas workspace data preview

## Overview

Let an owner-created canvas use its declared capabilities across the current
workspace while it remains in the creating task. First add the server-owned
data-scope and review transaction, then expose the scope and legacy upgrade
in the host, authoring guidance, and browser tests. Promotion continues to
control workspace placement.

## Scope

### In scope

- Workspace task lists and all currently supported declared canvas data,
  write, event, state, and network capabilities before promotion.
- A separate server-owned data scope on plugin instances and runtime bindings.
- An explicit, reviewed workspace-data upgrade for existing task canvases.
- Desktop and phone host scope presentation, localized copy, authoring and
  public documentation, and focused browser coverage.

### Out of scope

- Cross-workspace and installation-wide data access.
- Blanket grants for undeclared capabilities, new capability kinds, or
  managed agent-generated backend code.
- Silent widening of retained releases.

## Technical approach

1. Add nullable `data_scope_kind` to plugin instance persistence with null
   fallback to the existing placement `scope_kind`. Create version-2 local
   canvas authority with workspace data scope and exact workspace-ceiling
   declared grants. Keep other instances and historical authority unchanged.
2. Carry data scope through `webapp.CapabilityBinding`, grant validation, and
   the relative browser protocol. Use it for tasks, workflows, writes,
   messages, dependency projection, and events. Bind every operation to the
   trusted workspace and current user authorization. Follow the
   [plugin runtime design](../../specs/plugins/system-design/canvas-workspace-data-scope.md).
3. Add an owner-reviewed workspace-data upgrade for retained task canvases
   using the active release ID, permission digest, and grant generation as
   conditional transaction inputs. Keep placement and package unchanged.
4. Project placement and data scope separately in canvas context and host
   review. Update authoring instructions and public canvas/API docs. Use the
   existing desktop action menu and phone action drawer for legacy upgrade.

Likely backend files include `internal/plugins/instances/store.go`,
`internal/plugins/webapp/tokens.go`, `internal/plugins/webapp_protocol*.go`,
`internal/canvas/authoring.go`, `internal/canvas/service.go`, and the canvas
HTTP handler/controller. Existing E2E fixture expectations for one task must
change to a multi-task workspace fixture.

## ASCII UI preview

`UI-01: Task canvas host, new release (desktop)`. Entry: task canvas panel.
The host header stays fixed; the application owns its existing scroll region.
The badge reports data scope, while Promote controls placement. Labels are
illustrative; the distinction is required by
`AC-CANVASES-WORKSPACE-PREVIEW-001.3-.4`.

```text
Task coordinator   [Workspace data]   [Releases and permissions] [Promote canvas]
--------------------------------------------------------------------------
Existing tasks                                      [Refresh]
Available 3   Active 2   Completed 1
Task A        Task B        Task C
```

`UI-02: Existing task-only canvas (desktop)`. Entry: the same task panel.
The upgrade dialog uses the current permission-review layout and keeps
Cancel and Enable actions visible. It does not promote the canvas.

```text
Task coordinator   [Task data]   [Enable workspace data] [Promote canvas]
                      |
          Enable workspace data for this canvas?
          Release: Task coordinator 1.0.0
          Reads: tasks in this workspace
          Writes/events/network: exact declared permissions
          [Cancel]                         [Enable workspace data]
```

`UI-03: Existing task-only canvas (phone)`. Entry: focused task canvas route.
The existing Actions drawer exposes the upgrade; the review opens as a
full-height view with one scrolling permission region and safe-area actions.

```text
< Task   Task coordinator                        [Actions]
         Task data
          | Actions drawer: Enable workspace data
          v
Enable workspace data
Release and declared permissions  [scrolls]
-------------------------------------------
[Cancel]              [Enable workspace data]  [fixed, safe area]
```

## Tests

| Criteria | Evidence |
| --- | --- |
| `.1`, `.2`, `.8` | Plugin instance and web-app protocol tests for exact grants, all workspace pages, foreign denial, writes, workflows, messages, events, and dependencies. |
| `.3`, `.4` | Canvas service promotion and projection tests. |
| `.5`, `.6` | Conditional legacy-upgrade transaction and stale/owner/revocation tests. |
| `.7` | Canvas host component tests and desktop/mobile E2E. |

## E2E tests

- `apps/web/e2e/tests/canvas/plugin-canvas.spec.ts` (`chromium`): owner-created
  task canvas sees two or more workspace tasks and refreshes before promotion;
  promotion leaves the list intact. Covers `.1`, `.3`, `.4`.
- `apps/web/e2e/tests/canvas/mobile-plugin-canvas.spec.ts`
  (`mobile-chrome`): same data before promotion plus task-only legacy upgrade
  from the phone action drawer. Covers `.5`, `.7`.

## Work orders

- [x] [Task 01: Workspace data scope and legacy upgrade](task-01-workspace-data-scope.md)
- [x] [Task 02: Host scope review and browser proof](task-02-host-scope-review.md)

Task 02 depends on Task 01. Both run in the primary session unless the user
later explicitly authorizes another arrangement.

## Verification results

Passed: backend canvas/plugin tests, focused frontend Vitest coverage, web
typecheck, i18n validation, desktop and mobile canvas Playwright suites, spec
index validation, specification lint, and public documentation validation.

## Risks

- Grant checks currently equate instance scope with data scope. Missing a
  validation or event path could leak data or leave preview inconsistent.
- Existing task releases must retain their one-task behavior until reviewed;
  a broad migration would silently change authority.
- Same-origin canvas code can call ordinary user APIs already; the supported
  relative protocol must remain internally coherent in capability-only hosts.
