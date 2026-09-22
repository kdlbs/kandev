---
created: 2026-09-22
status: draft
requirements:
  - REQ-PLUGINS-WORKFLOW-HISTORY-001
  - REQ-PLUGINS-WORKFLOW-HISTORY-002
  - REQ-PLUGINS-WORKFLOW-HISTORY-003
system_design:
  - ../../specs/plugins/system-design/workflow-transition-history.md
legacy_specs: []
---

# Implementation Plan: Live workflow transition canvas

## Overview

Make the issue #3583 canvas display recorded Kandev task moves. Add bounded
task-ledger reads, expose them through the capability-gated Plugin Host and
canvas browser protocol, then replace the canvas sample with live task and
workspace views. This order makes the data contract testable before the app
uses it. The existing task canvas remains active until a valid new release is
published; workspace promotion remains a user-controlled operation.

## Scope

### In scope

- Per-task recorded trail, including null and removed endpoints.
- Workspace-scoped route groups and current unarchived task counts.
- Desktop and phone canvas views, polling, opt-in sound, and read failures.
- Typed gRPC/SDK and browser API parity, permission checks, public docs, and
  focused E2E coverage.
- Reuse of the already-published canvas ID
  `65d40cfb-7bd3-407a-beb7-be78a551e62a` and its assigned source root.

### Out of scope

- New transition writers, history reconstruction, task writes, and review
  verdicts.
- Direct SQLite access or operation while the Kandev backend is unavailable.
- Automatic workspace promotion or grant approval. The workflow-wide view
  becomes available only after the user promotes the canvas.

## Technical approach

1. Add task service and repository read methods in
   `apps/backend/internal/task/service/` and
   `apps/backend/internal/task/repository/sqlite/step_transitions.go`. Query by
   monotonic transition ID for the trail and aggregate workflow-side groups
   through bounded SQL reads. Add SQLite/Postgres indexes and parity tests.
2. Extend `apps/backend/proto/kandev/plugin/v1/plugin.proto`, generated Go,
   `apps/backend/pkg/pluginsdk/host.go`, `apps/backend/internal/plugins/host_data.go`,
   and `webapp_protocol*.go`. The task route requires `api_read:tasks`; the
   workspace summary requires both task and workflow read grants and a
   workspace binding. Update the embedded authoring browser-API reference at
   `apps/backend/internal/mcp/canvasskill/files/references/browser-api.md` and
   the public plugin/canvas docs.
3. Update only the assigned source root
   `.kandev/canvases/65d40cfb-7bd3-407a-beb7-be78a551e62a/` for the
   canvas release. Replace the sample with live history, support task and
   workspace scope in one app, and publish after local browser checks. Add
   isolated canvas-protocol E2E fixtures under `apps/web/e2e/tests/canvas/`.
   The task-local canvas source is not a tracked repository fixture; the
   repository E2E tests exercise the same host routes with a disposable
   package, and the final local Playwright check exercises the exact source
   before publication.

## ASCII UI preview

The following structure is required; wording and spacing are illustrative.
All user-facing copy is localized in the implementation. The canvas has no
task-write controls.

`UI-01: Task trail`, task canvas after load (`AC-PLUGINS-WORKFLOW-HISTORY-003.1`,
`.3`, `.4`):

```text
Desktop
+---------------------------------------------------------------+
| Workflow movement              Recorded history   [Sound] [Refresh] |
| Task: Fix sign-in edge case    State: In review   Updated: 14:10 |
+----------------------------+----------------------------------+
| Workflow steps             | Task trail (newest first)        |
| 01 Draft                   | Current: Review                  |
| 02 Build                   | -> Review from Build  14:10      |
| 03 Review  [current]       | <- Build from Review 13:20       |
| 04 Done                    | -> Review from Build 10:42      |
+----------------------------+----------------------------------+
| History begins when recording was enabled.                 |
+-----------------------------------------------------------+
```

`UI-02: Workspace overview`, after explicit promotion
(`AC-PLUGINS-WORKFLOW-HISTORY-002.1` to `.4`, `.003.2`):

```text
Desktop
+---------------------------------------------------------------+
| [Workflow: Feature delivery v] [Sound] [Refresh]              |
| Current tasks: 3     Recorded routes: 9                       |
+----------------------------+----------------------------------+
| Steps and route counts     | Selected task trail              |
| Draft       1 now          | [Task: Fix sign-in edge case v] |
|   -> Build  3 recorded     | Current: Review                  |
| Build       0 now          | <- Build from Review 13:20       |
|   -> Review 3 recorded     | -> Review from Build 10:42      |
| Review      1 now          |                                  |
|   <- Build  2 recorded     |                                  |
+----------------------------+----------------------------------+
| Entry/exit and removed-step groups remain visible separately. |
+---------------------------------------------------------------+
```

`UI-03: Phone focus`, both scopes (`AC-PLUGINS-WORKFLOW-HISTORY-003.5`, `.6`):

```text
Phone, one document scroll owner
+-----------------------------+
| Workflow movement           |
| [Workflow v]  [Task v]      |  task scope fixes these to this task
| [Steps] [Trail]             |  44 px touch controls
+-----------------------------+
| Trail (selected)            |
| Current: Review             |
| <- Build from Review        |
| -> Review from Build        |
| ...                         |
+-----------------------------+
| [Enable sound] [Refresh]    |  clears phone safe area
+-----------------------------+
Loading: Loading recorded history...
Empty: No recorded moves. Current step may still be known.
Unsupported: This Kandev version cannot read recorded history.
Denied: Access changed. Reopen or request workspace permission.
Error: Could not refresh. [Retry]
```

## Tests

| Acceptance | Evidence |
| --- | --- |
| `001.1` to `001.3` | Task repository tests for creation, backward move, same-time tie, null endpoints, removed steps, and keyset pages; SQLite and Postgres parity. |
| `001.4`, `001.5` | Host and canvas-protocol tests for task scope, missing grant, forbidden task, no actor/session fields, and no write side effect. |
| `002.1` to `002.4` | Repository grouping tests with active/archived/deleted tasks and cross-workflow entry/exit; host scope tests and paginated task-list projection. |
| `003.1` to `003.6` | Canvas E2E fixtures plus exact-source desktop/phone Playwright smoke before publish. |

## E2E tests

- `apps/web/e2e/tests/canvas/workflow-transition-history.spec.ts`, project
  `chromium`: seeded task moves through Review and back; browser reads only
  recorded rows; workspace summary after promotion; permission denial and
  refresh recovery (`001.1`, `001.4`, `002.1`, `002.3`, `003.1` to `.4`, `.6`).
- `apps/web/e2e/tests/canvas/mobile-workflow-transition-history.spec.ts`,
  project `mobile-chrome`: select workflow/task, switch Steps/Trail, inspect a
  return, retry an error, and assert touch targets and no horizontal overflow
  (`003.2`, `.3`, `.5`, `.6`).

## Work orders

- [ ] [Task 01: Read recorded moves and route groups](task-01-transition-reads.md)
- [ ] [Task 02: Expose scoped Host and canvas reads](task-02-host-api.md)
- [ ] [Task 03: Publish the live workflow canvas](task-03-live-canvas.md)

## Verification results

Pending implementation.

## Risks

- The ledger began recording after some tasks were created. The app must show
  only recorded history and label the start boundary.
- Current step order can change, so a historic move's return label is a
  current-order interpretation, not a persisted rejection fact.
- Grouped historical counts include archived tasks while current markers do
  not. The UI must name the two measures separately.
- The task-scoped canvas cannot show workflow-wide counts until the user
  promotes it and its workspace grants are active.
- The current running host may predate the new routes. The republished canvas
  must retain a useful current-task view and show an unsupported-host state
  until a host with the API is running.
- Task-local canvas source is not part of a PR checkout; the final release and
  exact-source browser check are task artifacts, while Host API code, tests,
  and docs are reviewable repository changes.
