---
created: 2026-09-22
status: draft
requirements:
  - REQ-PLUGINS-WORKFLOW-HISTORY-001
  - REQ-PLUGINS-WORKFLOW-HISTORY-002
  - REQ-PLUGINS-WORKFLOW-HISTORY-003
  - REQ-CANVASES-NAME-SHARE-001
  - REQ-CANVASES-NAME-SHARE-002
system_design:
  - ../../specs/plugins/system-design/workflow-transition-history.md
  - ../../specs/canvases/system-design/canvas-name-and-share-defaults.md
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

The same delivery improves the host controls exposed by the example canvas:
rename its instance from the toolbar and prepare share downloads from accurate
active-release defaults. These host changes apply to all task and workspace
canvases, independently of the workflow-history data API.

## Scope

### In scope

- Per-task recorded trail, including null and removed endpoints.
- Workspace-scoped route groups and current unarchived task counts.
- Desktop and phone canvas views, polling, opt-in sound, and read failures.
- Typed gRPC/SDK and browser API parity, permission checks, public docs, and
  focused E2E coverage.
- Reuse of the already-published canvas ID
  `65d40cfb-7bd3-407a-beb7-be78a551e62a` and its assigned source root.
- Authorized rename of a canvas's host-visible instance title, including
  desktop and phone surfaces and navigation refresh.
- Release-bound share defaults, short required-gap form, editable package
  details, and the existing reviewed bundle/source downloads.

### Out of scope

- New transition writers, history reconstruction, task writes, and review
  verdicts.
- Direct SQLite access or operation while the Kandev backend is unavailable.
- Automatic workspace promotion or grant approval. The workflow-wide view
  becomes available only after the user promotes the canvas.
- Automatic license choice, repository publication, registry submission, or
  mutation of a live release's package metadata during rename or sharing.

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
4. Add a canvas-service title update and owner-authorized host route, then
   expose Rename in desktop toolbar and phone host actions. Refresh host and
   navigation projections without restarting the canvas iframe.
5. Expose active-release metadata that the HTTP canvas response currently
   omits. Add an authorized export-defaults read that also supplies the
   server's compatible distribution minimum, and seed Share from it. Show only
   missing required details initially, keep populated fields editable, and
   retain the existing export validation and review/download safeguards.

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

`UI-04: Host rename`, task or workspace canvas
(`AC-CANVASES-NAME-SHARE-001.1` to `.4`):

```text
Desktop: [Issue 3583 investigation] [Rename]  [Releases] [Share]
         Rename -> [Issue 3583 investigation________] [Cancel] [Save]
Phone:   [Issue 3583 investigation] [Actions]
         Actions -> Rename -> focused name sheet [Cancel] [Save]
```

`UI-05: Quick sharing`, selected active release
(`AC-CANVASES-NAME-SHARE-002.1` to `.6`):

```text
Share canvas                         Release: 1.0.0
Package: Issue 3583 investigation   canvas-65d40cfb7bd3
Needs your input: License [________]
[Prepare downloads]
[Package details v]  ID, version, author, description, compatibility
After preparation: inventory + sizes + private-content reminder
                   [Download bundle] [Download source]
Phone: same content in one full-height, safe-area-aware scroll surface
```

## Tests

| Acceptance | Evidence |
| --- | --- |
| `001.1` to `001.3` | Task repository tests for creation, backward move, same-time tie, null endpoints, removed steps, and keyset pages; SQLite and Postgres parity. |
| `001.4`, `001.5` | Host and canvas-protocol tests for task scope, missing grant, forbidden task, no actor/session fields, and no write side effect. |
| `002.1` to `002.4` | Repository grouping tests with active/archived/deleted tasks and cross-workflow entry/exit; host scope tests and paginated task-list projection. |
| `003.1` to `003.6` | Canvas E2E fixtures plus exact-source desktop/phone Playwright smoke before publish. |
| `NAME-SHARE-001.1` to `.4` | Service, repository, HTTP, event, and desktop/phone host tests for rename, authorization, persistence, navigation refresh, and no iframe reload. |
| `NAME-SHARE-002.1` to `.6` | Distribution/default tests and desktop/phone E2E for release metadata, missing license, compatibility fallback, preparation, stale release, and both downloads. |

## E2E tests

- `apps/web/e2e/tests/canvas/workflow-transition-history.spec.ts`, project
  `chromium`: seeded task moves through Review and back; browser reads only
  recorded rows; workspace summary after promotion; permission denial and
  refresh recovery (`001.1`, `001.4`, `002.1`, `002.3`, `003.1` to `.4`, `.6`).
- `apps/web/e2e/tests/canvas/mobile-workflow-transition-history.spec.ts`,
  project `mobile-chrome`: select workflow/task, switch Steps/Trail, inspect a
  return, retry an error, and assert touch targets and no horizontal overflow
  (`003.2`, `.3`, `.5`, `.6`).
- `apps/web/e2e/tests/canvas/canvas-host-rename.spec.ts`, projects
  `chromium` and `mobile-chrome`: rename from task and workspace hosts; check
  picker/navigation, validation, and no iframe remount.
- Extend `canvas-sharing.spec.ts` and `mobile-canvas-sharing.spec.ts`:
  active-release defaults, one missing license, editable details, stale
  release handling, reviewed downloads, focus, and touch sizing.

## Work orders

- [ ] [Task 01: Read recorded moves and route groups](task-01-transition-reads.md)
- [ ] [Task 02: Expose scoped Host and canvas reads](task-02-host-api.md)
- [ ] [Task 03: Publish the live workflow canvas](task-03-live-canvas.md)
- [ ] [Task 04: Rename a canvas from host chrome](task-04-canvas-rename.md)
- [ ] [Task 05: Simplify reviewed canvas sharing](task-05-quick-sharing.md)

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
- Share defaults must come from the exact active release. A UI fallback from
  the canvas title cannot safely supply package identity, license, or
  compatibility; the server must provide and validate those values.
