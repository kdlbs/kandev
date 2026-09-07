---
created: 2026-08-30
status: in_progress
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-005
  - REQ-CANVASES-AGENT-WEB-APPS-006
  - REQ-CANVASES-AGENT-WEB-APPS-009
  - REQ-PLUGINS-ISOLATED-WEB-APPS-011
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
legacy_specs:
  - ../../specs/canvases/requirements/collaborative-canvases.md
  - ../../specs/canvases/system-design/collaborative-canvases.md
---

# Implementation Plan: Plugin-backed canvases UX follow-up

## Overview

Correct the canvas discovery, creation, host, appearance, and authoring issues
recorded in the follow-up investigation. Keep canvas creation in a normal task
and preserve the isolated plugin runtime.

First, align workspace discovery and task launch. Then connect release events
to the active task host. Correct host guidance and geometry before adding the
appearance protocol. Finish with the authoring bundle because its scaffold
depends on the final appearance contract.

## Scope

### In scope

- A folded-by-default canvas sidebar with explicit preference persistence.
- Feature-aware workspace Canvases cards, tabs, counts, and settings routes.
- Shared task-based canvas creation from workspace Canvases settings.
- Automatic first-release opening on desktop and phone task surfaces.
- Desktop tooltips and touch-visible lifecycle action descriptions.
- Full-height Dockview canvas rendering with direct and phone parity.
- A versioned, presentation-only host appearance protocol.
- A semantic-color authoring scaffold with live theme updates.
- One core authoring bundle, an exact inventory, and a generated scaffold.
- Disabled-path tests for every new `features.canvases` entry point.

### Out of scope

- A blank canvas builder, package import, or direct source editor.
- An absolute Kandev host skill path for task agents.
- A privileged iframe SDK or bidirectional host API.
- A change to the `features.canvases` key, environment variable, restart rule,
  or shipped profile defaults.
- Canvas package, permission, promotion, or persistence model changes.

## Technical approach

### Workspace discovery

Remove Canvases from `SECTION_ROUTE_MAP` so direct routes do not force sidebar
expansion. Preserve explicit section toggles through the current sidebar state.

Make `WORKSPACE_SETTINGS_TABS` feature-aware. Use the same filtered catalog in
the settings tree, tab strip, headings, and workspace summary tiles. Remove
`appendWorkspaceCanvasNodes`. Add active canvas counts only when the feature is
enabled. Use six tiles only at a width that preserves readable labels.

Render `WorkspaceCanvasesPage` inside `WorkspaceSettingsShell` with the
Canvases tab active. Keep the tab in the phone scroll strip even when the
workspace card omits the canvas tile.

### Guided task launch

Extend the task dialog preset contract with scratch source mode and a local
executor preference. Add one `CanvasTaskCreateLauncher` that supplies localized
title and prompt values. It keeps workflow and agent profile fields editable.

Use this launcher from workspace Canvases settings. The sidebar's empty setup
row links to that page and does not create canvases directly.
Successful creation follows the standard task-details route. The mobile
settings entry uses the existing full-screen task dialog.

### First-release host activation

Retain lifecycle payload identity as a bounded hint while HTTP remains the
canvas source of truth. Add a task-scoped hook that reacts only to the active
task's first activated or pending-permission release.

Desktop uses the current Dockview API to add and activate
`canvas:<canvasId>`. It checks for an existing panel before insertion. Phone
uses the focused canvas route. Events for another task do not change focus.

### Host guidance and geometry

Wrap lifecycle actions with the existing tooltip pattern. Disabled controls
use a focusable wrapper and explain the disabled reason. Mobile action rows
show equivalent descriptions.

Add the missing `h-full` flex boundary in the canvas Dockview renderer. Do not
change `PageShell`, the direct route, or phone `h-dvh` calculations.

### Appearance protocol

Add a typed version 1 appearance envelope beside `WebAppFrame`. Resolve the
active Kandev mode and fixed semantic token allowlist from host styles. Send
the initial envelope to the exact iframe window after load. Keep the loading
cover for one animation frame, then reveal the application.

Send another envelope after a live host theme change. The scaffold validates
the parent window, message type, version, mode, token keys, and value bounds.
It maps valid values to documented CSS custom properties with safe fallbacks.

### Authoring efficiency

Change the default `read_canvas_authoring_skill_kandev` response to one compact
core bundle with the exact reference inventory. Keep optional allowlisted path
reads for details.

Make `create_canvas_kandev` materialize the complete minimal scaffold and
return its exact inventory. Agents use native file tools after creation. Do not
return an absolute Kandev host path. Keep the response shape equal for local,
Docker, and SSH executors.

## Tests

| Acceptance criteria                     | Evidence                                                                                                 |
| --------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `AC-CANVASES-AGENT-WEB-APPS-001.2`      | Lifecycle handler and Dockview deduplication tests, desktop and phone Playwright                         |
| `AC-CANVASES-AGENT-WEB-APPS-001.7`      | Feature-off catalog, count, launcher, route, and event tests                                             |
| `AC-CANVASES-AGENT-WEB-APPS-001.8`      | Core-bundle and inventory golden tests, handler-level ACP contract test; external ACP evaluation pending |
| `AC-CANVASES-AGENT-WEB-APPS-001.9`      | Local, Docker, and SSH response-shape tests                                                              |
| `AC-CANVASES-AGENT-WEB-APPS-005.2-.9`   | Sidebar state, settings catalog, responsive card, route, and navigation tests                            |
| `AC-CANVASES-AGENT-WEB-APPS-006.1-.7`   | Host action tests and rendered desktop and phone geometry assertions                                     |
| `AC-CANVASES-AGENT-WEB-APPS-009.1-.5`   | Task preset tests and desktop and phone task-creation flows                                              |
| `AC-PLUGINS-ISOLATED-WEB-APPS-011.1-.4` | Appearance schema, frame delivery, source validation, and computed-color tests                           |

## E2E tests

- `apps/web/e2e/tests/canvas/plugin-canvas.spec.ts` covers folded navigation,
  workspace discovery, sidebar setup guidance, automatic panel opening,
  lifecycle help, full-height geometry, and live appearance changes.
- `apps/web/e2e/tests/canvas/mobile-plugin-canvas.spec.ts` covers the workspace
  Create canvas action through the full-screen task dialog, scratch/local
  defaults, editable workflow and agent controls, focused first-release route,
  visible permission approval, action descriptions, viewport containment, and
  live appearance changes.
- Both files run with Playwright retries disabled during implementation. They
  assert user outcomes and bounding-box relationships, not fixed sleeps.

## Work orders

- [x] [Task 01: Align workspace canvas discovery](task-01-workspace-canvas-discovery.md)
- [x] [Task 02: Add guided canvas task launch](task-02-guided-canvas-task-launch.md)
- [x] [Task 03: Open the first task release](task-03-first-release-host-activation.md)
- [x] [Task 04: Explain canvas lifecycle actions](task-04-canvas-action-guidance.md)
- [x] [Task 05: Fill the canvas host viewport](task-05-canvas-host-viewport.md)
- [x] [Task 06: Add isolated application appearance](task-06-isolated-application-appearance.md)
- [ ] [Task 07: Reduce authoring skill reads](task-07-authoring-bundle-and-scaffold.md)

Implementation is sequential in the primary conversation. A later user request
can authorize Codex implementation subagents. No work order authorizes Kandev
task or session delegation.

## Verification results

Tasks 01 through 06 are implemented. Task 07 has the implementation and
in-process contract coverage, but remains in progress until an external ACP
provider evaluation is captured. Earlier focused web verification passed 7
test files and 69 tests. The follow-up focused web suite passed 15 files and
73 tests. The affected backend packages passed 1,149 tests, and the full
backend suite passed with the repository config override cleared. The full web
suite passed 1,687 files, 14,375 tests, and 4 skipped tests. Desktop and phone
canvas Playwright flows passed with retries disabled. Web typecheck, lint, i18n
checks, the new-code ratchet, specification lint, harness lint, shell syntax,
and diff checks passed. Current remediation-focused backend, lifecycle, and
resource-guard tests also pass; the external ACP evaluation is outstanding.

The local web test runners now bound Vitest parallelism and E2E shard/worker
counts by default. E2E runs use one worker per shard and at most three local
shards, with lower limits on low-memory hosts. Higher parallelism requires an
explicit opt-in environment variable, and the E2E skill documents the safe
defaults for future sessions.

## Risks

- A feature-aware tab catalog can accidentally fetch canvas data while the
  feature is disabled.
- A task preset can overwrite durable last-used choices instead of applying a
  launch-only preference.
- A delayed lifecycle event can open a duplicate panel or steal another task's
  focus.
- An unbounded appearance message can become an accidental host API.
- Theme reveal timing can flash stale colors if the iframe is shown too early.
- Scaffold files and the embedded skill inventory can drift without one golden
  source.
