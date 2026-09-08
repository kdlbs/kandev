---
created: 2026-08-26
status: done
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-001
  - REQ-PLUGINS-ISOLATED-WEB-APPS-002
  - REQ-PLUGINS-ISOLATED-WEB-APPS-003
  - REQ-PLUGINS-ISOLATED-WEB-APPS-004
  - REQ-PLUGINS-ISOLATED-WEB-APPS-005
  - REQ-PLUGINS-ISOLATED-WEB-APPS-006
  - REQ-PLUGINS-ISOLATED-WEB-APPS-007
  - REQ-PLUGINS-ISOLATED-WEB-APPS-008
  - REQ-PLUGINS-ISOLATED-WEB-APPS-009
  - REQ-PLUGINS-ISOLATED-WEB-APPS-010
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-002
  - REQ-CANVASES-AGENT-WEB-APPS-003
  - REQ-CANVASES-AGENT-WEB-APPS-004
  - REQ-CANVASES-AGENT-WEB-APPS-005
  - REQ-CANVASES-AGENT-WEB-APPS-006
  - REQ-CANVASES-AGENT-WEB-APPS-007
  - REQ-CANVASES-AGENT-WEB-APPS-008
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
legacy_specs:
  - ../../specs/canvases/requirements/collaborative-canvases.md
  - ../../specs/canvases/system-design/collaborative-canvases.md
---

# Implementation Plan: Plugin-backed canvases

## Overview

Create agent-authored canvas applications as isolated plugin web applications.
Agents create task canvases. Users promote useful canvases to workspace scope
and edit them through Quick Chat.

Task 00 moves this package to a clean branch from current `origin/main`. It
then closes PR #3061 as superseded. The implementation does not include the
declarative canvas code from that pull request.

Add the release flag and package foundation first. Then add the browser
security boundary, data protocol, event transport, and canvas lifecycle. Add
the shared task host after these contracts are stable. Then add authoring,
promotion, editing, and workspace management.

## Scope

### In scope

- A safe transition from PR #3061 to a clean implementation branch.
- One fail-closed canvas runtime feature flag.
- Isolated plugin web-application contributions.
- Scoped instances, immutable releases, grants, state, and artifact budgets.
- Response-level sandboxing for browser and Tauri hosts.
- Relative HTTP data, message, state, action, and event endpoints.
- Task workflow-step projections and task prompt writes.
- Bounded SSE connections, replay, heartbeat, and resync.
- Agent-created task canvases and streamed source collection.
- Canvas-specific system-skill delivery outside Office skill deployment.
- Atomic publish, permission review, rollback, and cleanup.
- Human-only promotion from task scope to workspace scope.
- Quick Chat editing of workspace canvas source.
- Task, workspace, direct-route, desktop, and phone host surfaces.
- Public authoring, security, operations, and user documentation.

### Out of scope

- A native visual canvas builder.
- Direct canvas source editing in the Kandev UI.
- An agent-generated backend process on the Kandev host.
- Remote scripts or unreviewed external network origins.
- Canvas package import, export, marketplace publication, or federation.
- Canvas collaborators, invitations, presence, or live source editing.
- Workspace-to-task demotion.
- General plugin contributions for every Kandev UI slot.

## Technical approach

### Baseline transition

Preserve the complete design package in one documentation commit. Create a
new branch from the latest `origin/main`, and transfer only that commit. Do not
push the documentation commit to the branch for PR #3061.

Before implementation, compare the new branch with `origin/main`. The diff
must contain only the design package. Close PR #3061 after this check passes.
Add a comment that names the replacement ADR and the clean implementation
baseline.

### Release gate

Use `features.canvases` and `KANDEV_FEATURES_CANVASES`. Set all shipped
profile defaults to `false`. Gate HTTP, WebSocket, SSE, MCP, background work,
boot data, routes, settings, and navigation.

Create the canvas repository, services, API, stores, hooks, and components
from current `main`.

### Plugin package and artifact foundation

Extend `internal/plugins/manifest.UISection` with validated `web_apps`.
Add scoped instances, immutable releases, grants, state, and cleanup jobs.
Store artifacts under the Kandev data directory.

Enforce 2 GiB per workspace and 10 GiB per installation. Keep one active valid
release, one prior valid release, and one pending release for each instance.
Reconcile artifact paths and digests before runtime registration.

### Isolated browser and desktop runtime

Issue a short-lived capability URL for an authorized instance and release.
Apply the sandbox through both the iframe attribute and the response CSP.
Direct capability-URL navigation remains an opaque sandbox.

Add explicit content, referrer, MIME, cache, framing, and cross-origin headers.
Permit only configured web hosts and the supported Tauri origins. Update the
desktop shell `frame-src` policy for the loopback backend.

### Browser data and state

Extract shared Host data services from `internal/plugins/host_data_*`.
Add `workflow_step_id` to the task read DTO. Preserve the current task update
service for workflow-step moves.

Add a bounded task-message route that reuses Plugin Host `SendMessage`.
Add optimistic instance state with monotonic revisions and stable conflicts.

### Live event transport

Add an SSE adapter for authorized public Kandev events. Flush headers and
events immediately. Send a heartbeat every 15 seconds.

Limit streams to two for one user and instance and 20 for one user. Retain at
most 1,000 events or five minutes per instance. Send a resync event after a
generation or replay gap.

### Canvas lifecycle

Create canvas metadata that references one plugin instance. Keep task cleanup,
workspace filtering, archive, restore, and removal.

Limit one task to 10 non-removed canvases. Limit one workspace to 100
non-removed canvases across all scopes. Count archived canvases. Make count and
restore admission atomic.

Workspace deletion removes all canvas database state and records durable
artifact-cleanup jobs before commit.

### Task canvas host

Add one shared web-application host for the direct route and desktop task
panel. Keep status and lifecycle controls outside the iframe.

Use seeded fixtures in component tests. The complete agent-authoring and
workspace-management flow runs after those services are available.

### Agent authoring

Add create, inspect, publish, skill-read, and state MCP tools. Gate tool
registration with `features.canvases`. Derive actor and scope from the MCP
session.

Use a canvas-owned skill embed and
`<kandev-home>/system-skills/kandev-canvas-authoring`. Canvas MCP reads this
inventory for local, Docker, and remote agents. Do not deploy it into task
workspaces or Office skill rows.

Add a bounded agentctl tar stream for the assigned source root. Permit 10
publish attempts for one agent session in five minutes and one in-flight
publish for one canvas.

### Promotion, editing, and releases

Add promotion preview and confirmation. Show every requested permission and
the task-to-workspace scope change. Change scope and grants atomically.

Add pending-permission approval, release history, rollback, archive, restore,
and removal. Rollback selects the one retained prior release.

Add a canvas edit-session endpoint. It creates a Quick Chat task with trusted
canvas metadata and a draft of the active source. The edit session can publish
only its target canvas.

### Workspace management surfaces

Keep a folded workspace Canvases sidebar section. Workspace settings manage
permissions, releases, editing, archive, restore, and removal.

On phones, use a direct `h-dvh` route and one focused canvas. Use an inset
bottom drawer for the canvas picker and secondary actions. Do not mount
Dockview on phones.

### Public documentation

Document `ui.web_apps`, browser isolation, runtime permissions, canvas
authoring, promotion, editing, storage recovery, the feature flag, and feature
status. Add the new public page to `docs/public/meta.json`.

## Tests

| Acceptance area                           | Main evidence                                                                        |
| ----------------------------------------- | ------------------------------------------------------------------------------------ |
| Feature disabled and enabled paths        | runtime-flag contract tests, route and MCP inventory tests                           |
| Manifest, package, and storage validation | manifest, package, artifact-store, quota, and reconcile tests                        |
| Browser and desktop isolation             | runtime handler, policy, direct-navigation, Tauri CSP, and component tests           |
| Browser data and state parity             | Host wire, browser adapter, message, workflow-step, and conflict tests               |
| Event scope and recovery                  | SSE heartbeat, flush, cap, replay, gap, cancellation, and leak tests                 |
| Canvas lifecycle and limits               | repository, concurrency, task cleanup, workspace cleanup, archive, and restore tests |
| Trusted source publishing                 | MCP, agentctl archive, executor integration, rate, and cancellation tests            |
| Promotion and releases                    | handler, service, permission, rollback, and retention tests                          |
| Quick Chat editing                        | handler, trusted target, cleanup, and frontend launcher tests                        |
| Task and direct host surfaces             | component tests and desktop Playwright task flow                                     |
| Workspace and mobile surfaces             | navigation, settings, geometry, and mobile Playwright flows                          |
| Public contracts                          | public-doc validation and link tests                                                 |

Tests use `@covers AC-*` comments where the file path does not make the
mapping clear.

## E2E tests

- `apps/web/e2e/tests/canvas/plugin-canvas.spec.ts` covers agent creation,
  task grouping, the Continue action, workflow-step movement, live updates,
  promotion, editing, permission review, rollback, and restart recovery.
- `apps/web/e2e/tests/canvas/mobile-plugin-canvas.spec.ts` covers task entry,
  workspace navigation, the focused route, action drawer, Edit canvas, touch
  targets, safe areas, and host viewport containment.
- Desktop packaging smoke coverage loads a canvas inside the Tauri shell and
  proves that direct capability navigation remains sandboxed.

## Work orders

- [x] [Task 00: Move to the plugin-backed baseline](task-00-baseline-transition.md)
- [x] [Task 01: Canvas release gate](task-01-canvas-release-gate.md)
- [x] [Task 02: Plugin web-app package foundation](task-02-plugin-web-app-package-foundation.md)
- [x] [Task 03: Isolated browser runtime](task-03-isolated-browser-runtime.md)
- [x] [Task 04: Browser application protocol](task-04-browser-data-state.md)
- [x] [Task 05: Live event transport](task-05-live-event-transport.md)
- [x] [Task 06: Canvas lifecycle](task-06-canvas-lifecycle.md)
- [x] [Task 07: Task canvas host surfaces](task-07-canvas-host-task-surfaces.md)
- [x] [Task 08: Agent canvas authoring](task-08-agent-canvas-authoring.md)
- [x] [Task 09: Canvas release governance](task-09-promotion-release-management.md)
- [x] [Task 10: Quick Chat canvas editing](task-10-quick-chat-canvas-editing.md)
- [x] [Task 11: Responsive workspace canvas management](task-11-workspace-mobile-surfaces.md)
- [x] [Task 12: Public canvas documentation](task-12-public-canvas-documentation.md)

Execution is sequential in the primary conversation. Plan waves describe
dependency order only. They do not authorize implementation subagents.

## Verification results

- Task 00 passed on 2026-08-27. `feature/plugin-backed-canvases` is based on
  `origin/main`, carries only the 26-file documentation package, and has no
  non-docs diff. Specification lint and diff checks pass. PR #3061 is closed
  as superseded by the plugin-backed web-application design.
- Task 01 passed on 2026-08-27. The typed `features.canvases` release gate is
  registered as a restart-required, experimental, high-risk flag, and its
  backend, profile, frontend, and startup-catalog contracts are covered by
  focused tests. The exact backend verification passes with the internal
  configuration-file environment cleared, and the frontend contract test
  passes.
- Task 02 passed on 2026-08-27. Static web-app manifest validation, bounded
  immutable artifacts, scoped plugin-instance/release/grant storage, atomic
  count and byte admission, cleanup inventory, and startup artifact
  reconciliation are implemented. The work-order backend verification passes.
- Task 03 passed on 2026-08-27. Capability-bound immutable runtime serving,
  opaque response and iframe isolation, normalized CSP network policy, browser
  and Tauri framing rules, and responsive host-frame lifecycle coverage are
  implemented. Backend, focused frontend, i18n, and desktop type checks pass;
  the broad plugin frontend suite reports one pre-existing Monaco command
  registration race after all 258 tests pass.
- Task 04 passed on 2026-08-28. Relative browser data, task-message,
  workflow-step, action, and revisioned state routes are authorized by the
  capability binding and reuse the existing Host services. The exact backend
  verification passes with 871 tests in 12 packages.
- Task 05 passed on 2026-08-28. Bounded SSE replay, heartbeat, generation,
  resync, stream admission, cancellation, and event-bus projection are
  implemented. The web-app event package has 23 focused tests, and the exact
  plugin verification passes.
- Task 06 passed on 2026-08-28. Canvas scope, release identity, atomic count
  admission, atomic lifecycle transactions, archive and restore, task cleanup,
  workspace cleanup, startup orphan reconciliation, durable artifact-cleanup
  jobs, and unavailable-release reconciliation are covered by the focused
  canvas, backendapp, plugin, and instance-store tests.
- Task 07 passed on 2026-08-28. The shared canvas host, direct route, task
  Dockview panel, recovery states, lifecycle controls, and localized UI are
  covered by 9 frontend files with 59 passing tests, frontend lint and
  typecheck, and the desktop canvas Playwright flow.
- Task 08 passed on 2026-08-28. Scoped Canvas MCP tools, the separate
  authoring-skill inventory, bounded authenticated agentctl source transfer,
  publish admission, and executor runtime wiring pass the exact MCP, canvas,
  agentctl, and agent-runtime verification.
- Task 09 passed on 2026-08-28. Human-only promotion, permission review,
  atomic grants and scope changes, pending-release approval, rollback, archive,
  restore, removal, retention, and authorization pass the canvas, plugin, and
  backendapp verification plus the canvas API tests.
- Task 10 passed on 2026-08-28. Quick Chat edit sessions carry a trusted canvas
  target, materialize immutable source without changing the active release,
  and preserve retained releases. Backend edit tests and the focused frontend
  host/API tests pass.
- Task 11 passed on 2026-08-28. Workspace navigation, settings management,
  mobile focused routes, inset action drawer, safe-area sizing, and no-Dockview
  phone behavior pass frontend lint, typecheck, i18n checks, 59 focused tests,
  and both desktop and mobile canvas Playwright flows.
- Task 12 passed on 2026-08-28. Public canvas, plugin, security,
  configuration, operations, and feature-status documentation is published.
  Public-doc tests pass (61 tests and 42 published pages), full specification
  lint passes, and the repository's public-doc validator checks local and
  heading links. The work order references `scripts/check-links.py`, which is
  not present in current `main`.

## Risks

- A branch transition can lose uncommitted design files or include superseded
  production code.
- A missing response sandbox can let direct navigation inherit host authority.
- A wrong CORP or `frame-ancestors` value can block browser or Tauri hosts.
- A capability URL can become an ambient credential through logs or referrers.
- A database-only restore can leave release artifacts unavailable.
- A source archive can cross the assigned root or consume excess resources.
- Browser and gRPC data adapters can drift without shared contract tests.
- Event streams can leak subscriptions or bypass current scope after reconnect.
- A count or byte reservation can race with create, restore, or publish.
- Office skill synchronization can absorb the canvas skill if embeds overlap.
- Permission changes can activate broader access before user confirmation.
- Arbitrary application CSS can still produce a poor phone layout.
