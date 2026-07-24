---
spec: docs/specs/tasks/attach-workspace-sources.md
created: 2026-07-22
status: completed
---

# Implementation Plan: Attach Workspace Sources

## Overview

Add a durable task-level source attachment contract above the current worktree-only branch flow,
then teach host and remote runtimes to materialize the same source set. Reuse the task-create
repository controls inside a dedicated responsive Files-panel picker, publish live source/root
updates through the existing task/session data flow, and finish with desktop, mobile, Docker, and
SSH coverage. The plan preserves `task_repositories` for Git behavior and introduces a separate
folder attachment relation, as required by
[ADR-2026-07-22-runtime-mutable-task-workspace-sources](../../decisions/2026-07-22-runtime-mutable-task-workspace-sources.md).
The active UX delta extracts the task-create source-mode presentation into a controlled shared
selector, presents **Local** and **Remote** modes inside Add sources, and combines **Add sources**
with **Open workspace folder** in one Files-toolbar action menu without changing the durable source
union or materialization contract.

---

## Backend

### Persistence and source projection

- Add `TaskWorkspaceFolder` to `apps/backend/internal/task/models/models.go` with task ownership,
  canonical host path, collision-safe display name, global source position, and timestamps.
- Add `task_workspace_folders` to both the base schema and replayable migrations in
  `apps/backend/internal/task/repository/sqlite/base_schema.go` and `base_migrations.go`; enforce
  task/path and task/display-name uniqueness, cascade deletion, and task/position lookup indexes.
- Extend `apps/backend/internal/task/repository/interface.go` and add a focused SQLite repository
  file for folder CRUD plus a transactional batch that creates repository/folder attachments and a
  compensating batch that removes only rows created by the failed operation.
- Extend `Task`, `TaskDTO`, `pkg/api/v1.Task`, boot/snapshot queries, and `task.updated` serialization
  with `workspace_folders`. Build a combined ordered source projection without changing existing
  repository DTOs or Git consumers.
- Allocate positions under the existing per-task mutation lock using the maximum across repository
  and folder rows. Reject runtime-name collisions across both tables after applying the same
  directory-name sanitizer used for multi-repository worktrees.

### Attachment service

- Add `apps/backend/internal/task/service/service_workspace_sources.go` with
  `AttachWorkspaceSources(ctx, request)` and typed repository/folder input/result shapes.
- Validate the whole batch before writes: task exists and is repository-backed; workspace ownership;
  exactly one repository locator; provider-neutral URL and identity consistency; canonical explicit
  local Git paths; canonical arbitrary directories; branches; duplicates; display-name collisions;
  and idle session/turn state.
- Resolve or create repository entities through the existing `ResolveRepositoryRef` path so local
  trust and remote credential rules stay authoritative. Track newly created repository entities for
  rollback without deleting pre-existing workspace repositories.
- Persist the batch, call the runtime materializer synchronously, and compensate database plus owned
  filesystem/runtime entries on failure with a detached bounded context. Publish `task.updated` only
  after both persistence and materialization succeed.
- Refactor `AddBranchToTask` into a compatibility adapter over a one-item repository-source request;
  retain its existing auto-branch naming, duplicate, and slug-collision behavior.

### HTTP, WebSocket, and MCP contracts

- Register `POST /api/v1/tasks/:id/workspace-sources` in
  `apps/backend/internal/task/handlers/task_handlers.go`; define the discriminated source request in
  `task_http_handlers.go`, map typed service errors to `400`/`404`/`409`/`422`, and return the durable
  source projection, effective workspace path, and affected sessions.
- Add `workspace.sources.updated` / `session.workspace_sources_updated` event constants and gateway
  routing in `internal/events/types.go`, `pkg/websocket/actions.go`, and
  `internal/gateway/websocket/task_notifications.go`. Emit only after agentctl has adopted the root.
- Register `add_workspace_sources_kandev` in `internal/mcp/server/server.go` and its backend handler
  in `internal/mcp/handlers/handlers.go`; default `task_id` to the current task and accept the same
  batch union as HTTP. Keep `add_branch_to_task_kandev` externally compatible while routing it
  through the generalized service.

### Host materialization

- Replace the narrow `BranchMaterializer` hook in `internal/task/service/service.go` with a
  `WorkspaceSourceMaterializer` batch interface, retaining a thin compatibility adapter for tests
  and existing call sites.
- Refactor `internal/backendapp/branch_materializer.go` into host source materialization that reuses
  `worktree.Manager` for repository branches and creates a Kandev-owned task root for mixed local
  sources. Link live local sources into that root with platform-native directory links (Unix
  symlinks, Windows directory junctions), while validating the canonical target before and after
  link creation.
- Never copy, move, delete, or write marker files into user-owned local sources. Rollback and task
  cleanup remove only Kandev-owned worktrees, links, and task roots.
- Extend lifecycle workspace rebinding so an idle Local execution can restart/rebind at the mixed
  source root without losing its task conversation; Worktree executions keep their existing live
  sibling materialization path. Persist the promoted `task_environments.workspace_path` for future
  sessions and resets.
- Generalize agentctl rescan from repository-only reconciliation to workspace-source reconciliation:
  update the workdir, rebuild repository trackers for Git entries, refresh file-tree scope for plain
  folders, and fail the attachment if the running agentctl cannot adopt the new root.

### Container and remote materialization

- Add authenticated agentctl endpoints and client methods for validated repository clone/checkout
  materialization under the current workspace root.
- Route local Docker, SSH, and Sprites attachments through their live `AgentExecution` agentctl
  client. Reuse existing executor credential resolution for repository clones; do not persist or log
  tokens or credentialed URLs.
- Reject folder inputs for Docker, SSH, Sprites, and remote Docker during service capability
  validation, before persistence or executor filesystem access. The frontend uses the same executor
  capability to omit the folder source kind.
- Make subsequent launch/reset preparation derive every repository from the durable source
  projection. Remove the task-create frontend guard that forces multi-repository work to Worktree
  only after the corresponding runtime launch paths support repository projection.
- Add the same materializer capability to the remote-Docker interface and tests; the executor remains
  unavailable until its existing `CreateInstance` implementation is completed.

---

## Frontend

### Shared source picker

- Extract reusable repository selection leaves from
  `task-create-dialog-workspace-repo-chips.tsx`, `task-create-dialog-remote-repo-chip.tsx`,
  `task-create-dialog-remote-repo-chips.tsx`, and `folder-picker.tsx` into
  `components/workspace-source-picker/` without changing the task-create dialog behavior.
- Generalize `task-create-dialog-source-mode.tsx` into a controlled segmented source-mode
  presentation with configurable options, labels, and stable test IDs. Task creation keeps
  **Repo / Remote / None** and its existing state transitions; Add sources uses **Local / Remote**
  and owns separate batch state.
- Add typed mixed rows (`repository`, `remote_repository`, `folder`) and a
  `useWorkspaceSourcePickerState` hook. The **Local** mode reuses the combined saved/discovered
  repository selector and exposes a local-folder/manual-path affordance when supported. The
  **Remote** mode reuses provider-backed and pasted-URL controls. Mode switching changes available
  add controls without removing configured rows, so one submission can mix source kinds on
  Local/Worktree. Executor capability removes the folder affordance on container/remote tasks.
- Decouple reusable remote-row presentation from create-task-only `DialogFormState`; keep branch,
  provider metadata, discovery, validation, and payload conversion owned by the consuming dialog.
- Add `attachTaskWorkspaceSources` to `lib/api/domains/kanban-api.ts`, source types to
  `lib/types/http.ts` and the kanban slice, and unit-tested payload/error normalization.
- Once backend runtime support lands, delete the non-Worktree multi-repository disable resolver and
  auto-switch effect in `task-create-dialog-computed.ts` and
  `task-create-dialog-multi-repo-guard.ts`; keep the no-repository Worktree restriction.

### Files panel surface and live state

- Replace the separate **Add sources** and **Open workspace folder** toolbar controls with one
  labeled/icon **Workspace actions** trigger in `file-browser-toolbar.tsx`, threaded through
  `file-browser.tsx`, `files-panel.tsx`, `task-files-panel.tsx`, and their hooks. Its dropdown menu
  contains both actions. Keep the trigger enabled whenever opening the workspace folder is valid;
  disable only the **Add sources** row for archived/non-repository tasks, active turns/tool calls,
  or loading source state, and render the reason as visible menu copy.
- Build `components/task/add-workspace-sources/` around the shared picker. Desktop uses a Dialog;
  phone uses a full-height Drawer with fixed header/footer, `min-h-0` internal scrolling,
  `100dvh` sizing, safe-area bottom padding, and a single submit action. Preserve keyboard focus,
  Enter behavior, dismissal, pending state, per-source errors, retryable batch contents, and rows
  configured in inactive Local/Remote modes.
- Coordinate Radix menu close autofocus before opening the controlled Dialog/Drawer so dismissal
  returns focus to the workspace-actions trigger. On phones, use the existing responsive
  `DropdownMenu` bottom-sheet treatment rather than introducing a second action-menu drawer.
- Subscribe to `task.updated` for repository/folder state and
  `session.workspace_sources_updated` for the adopted workspace root. Update the kanban/session
  slices, reset the file-tree cache for affected environment/session IDs, and let existing
  repository/worktree handlers refresh Changes and editor pickers.
- Keep the new phone entry point visible and touch-usable (at least 44px active dimension) without
  hiding any source kind supported by the task's executor. The closest shipped mobile exemplar is
  `components/task/mobile/mobile-picker-sheet.tsx`; reuse its focused picker hierarchy while using a
  full-height surface because mixed rows, branch search, validation, and batch errors can be deep.

### Mobile design contract

- **Desktop outcome:** one compact labeled/icon Files-toolbar trigger opens a dropdown with **Add
  sources** and **Open workspace folder**. Selecting Add sources opens the multi-row dialog and
  refreshes the workspace tree after one atomic submission.
- **Mobile entry point:** the same workspace-actions trigger is visible in the Files toolbar with an
  actual 44px touch target. The shared responsive DropdownMenu becomes a short inset bottom-sheet
  action menu; it does not depend on hover, context click, or desktop Dockview chrome.
- **Hierarchy and primary action:** the action menu chooses between workspace actions. The Add
  sources surface then shows a shared **Local / Remote** segmented selector, while configured rows
  and validation own the single scroll body. Cancel and **Add sources** stay fixed in the footer,
  with submit as the sole primary action.
- **Presentation rationale:** the two workspace actions are a short temporary choice, so they fit
  the shared inset menu. Source attachment can contain several searchable, branch-aware rows, so it
  continues into a full-height Drawer rather than stacking the full form inside that menu.
- **Shared versus responsive:** source state, validation, payload construction, and API calls are
  shared. The source-mode switch is controlled presentation reused by task creation and Add sources;
  each dialog keeps its own state semantics. Only Dialog/Drawer composition, responsive menu
  treatment, and toolbar hitbox treatment differ.
- **Geometry:** one internal vertical scroll owner, dynamic viewport height, safe-area clearance,
  44px phone trigger/mode/menu rows, contained repository popovers/drawers, and no document
  horizontal overflow.

---

## Tests

- **What:** folder persistence, canonical uniqueness, cross-table display-name/position allocation,
  cascade deletion, and SQLite/Postgres replay safety.  
  **File:** `internal/task/repository/*workspace_source*_test.go` and
  `internal/task/repository/sqlite/*workspace_folder*_test.go`.  
  **How:** real database tests, including migration replay.
- **What:** valid mixed batches, repository resolution, idle gate, duplicate/cross-workspace/path
  failures, partial materialization rollback, client cancellation, and legacy add-branch parity.  
  **File:** `internal/task/service/service_workspace_sources_test.go` and existing branch tests.  
  **How:** table-driven service tests with real SQLite plus fake materializer failure injection.
- **What:** HTTP status mapping, MCP current-task defaulting and batch forwarding, event truthfulness,
  and compatibility tool behavior.  
  **File:** task handler, MCP handler/server, gateway, and service event tests beside the source.  
  **How:** HTTP/WS integration tests and MCP captured-payload tests.
- **What:** host root promotion, link safety, Windows/Unix path behavior, idle restart/rebind,
  agentctl source rescan, rollback, and relaunch reconstruction.  
  **File:** `internal/backendapp/workspace_source_materializer_test.go`, lifecycle tests, and agentctl
  process/API tests.  
  **How:** temporary Git repositories/folders plus fake lifecycle clients; platform-specific tests
  where link semantics differ.
- **What:** Docker/SSH/Sprites repository clone transport, unsupported-folder rejection, credential
  redaction, cancellation, cleanup, and reset reconstruction.  
  **File:** lifecycle executor tests and agentctl workspace-source API tests.  
  **How:** table-driven unit/integration tests with fake agentctl plus existing container harnesses.
- **What:** mixed picker state, payload construction, duplicate errors, disabled busy state,
  task/session event merges, and file-tree cache reset.  
  **File:** focused `*.test.ts(x)` files beside the new picker, API helper, toolbar, and WS handlers.  
  **How:** Vitest and Testing Library.
- **What:** the shared source-mode presentation keeps task-create labels/test IDs while Add sources
  renders Local/Remote, preserves mixed rows across mode switches, and provides 44px mobile options.
  **File:** `task-create-dialog-repo-chips.test.tsx`,
  `add-workspace-sources-dialog.test.tsx`, and a focused shared-mode component test.
  **How:** Testing Library controlled-state and responsive-geometry assertions.
- **What:** one workspace-actions trigger exposes both actions, keeps Open workspace folder enabled
  when Add sources is disabled, shows the disabled reason, and hands focus to/from the source
  surface.
  **File:** a focused test beside `file-browser-toolbar.tsx`.
  **How:** Testing Library menu interaction and callback assertions.

## E2E Tests

- **Scenario:** GIVEN an idle one-repository worktree task, WHEN a desktop user adds a second local
  repository and a plain folder through **Workspace actions → Add sources → Local**, THEN both
  top-level entries appear and only the repository appears in Changes.
  **File:** `apps/web/e2e/tests/task/add-workspace-sources.spec.ts`.  
  **What to verify:** both menu actions, open-folder request routing, shared Local selector, Remote
  mode switching with row preservation, dialog flow, atomic success, Files refresh,
  repository-aware UI, and persistence after reload.
- **Scenario:** GIVEN the Files tab on a Pixel 5 viewport, WHEN the user opens the full-height source
  picker through the workspace-actions bottom-sheet menu and attaches two sources across Local and
  Remote, THEN the same task value is delivered without horizontal overflow.
  **File:** `apps/web/e2e/tests/task/mobile-add-workspace-sources.spec.ts`.  
  **What to verify:** 44px trigger/menu/mode rows, both actions, open-folder request routing,
  internal scroll, fixed safe-area footer, submit outcome, dismissal/focus return, and document
  containment.
- **Scenario:** GIVEN a Docker task, WHEN a remote repository is attached, THEN it is readable inside
  the running container; the folder source kind is absent and a forged folder request is rejected.  
  **File:** `apps/web/e2e/tests/docker/add-workspace-sources.spec.ts`.  
  **What to verify:** live agentctl repository materialization, capability gating, rollback, and
  reset/relaunch.
- **Scenario:** GIVEN an SSH task, WHEN a repository source is attached, THEN it appears in the
  remote task directory and survives backend reconnect.  
  **File:** `apps/web/e2e/tests/ssh/add-workspace-sources.spec.ts`.  
  **What to verify:** remote placement, rescan, persistence, and cleanup ownership.

---

## Implementation Waves

Wave 1:

- [x] [Task 01: Durable source contracts](task-01-durable-source-contracts.md)

Wave 2:

- [x] [Task 02: Attachment service](task-02-attachment-service.md)

Wave 3 (parallel):

- [x] [Task 03: Protocol surfaces](task-03-protocol-surfaces.md)
- [x] [Task 04: Host materialization](task-04-host-materialization.md)
- [x] [Task 06: Shared source picker](task-06-shared-source-picker.md)

Wave 4 (parallel after dependencies):

- [x] [Task 05: Remote materialization](task-05-remote-materialization.md)
- [x] [Task 07: Files panel surface](task-07-files-panel-surface.md)

Wave 5 (parallel):

- [x] [Task 08: End-to-end coverage](task-08-end-to-end-coverage.md)
- [x] [Task 09: Public documentation](task-09-public-documentation.md)

Wave 6:

- [x] [Task 10: Final verification](task-10-final-verification.md)

Wave 7 (parallel UX delta):

- [x] [Task 11: Shared Local and Remote selector](task-11-shared-local-remote-selector.md)
- [x] [Task 12: Unified Files workspace actions](task-12-unified-files-workspace-actions.md)

Wave 8 (parallel after Tasks 11 and 12):

- [x] [Task 13: UX delta end-to-end coverage](task-13-ux-delta-e2e.md)
- [x] [Task 14: UX terminology documentation](task-14-ux-terminology-documentation.md)
