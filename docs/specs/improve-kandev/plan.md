# Plan — Improve Kandev

> Implementation plan for the `improve-kandev` spec.
> Workflow: TDD (Red → Green → Refactor) per `.claude/skills/tdd/SKILL.md`.

## 1. Overview

Deliver an in-app entry point for filing bug reports and feature suggestions
that produces a real kandev task running on the user's own agent. The task
follows a hidden three-step workflow (improve → test → pr) and optionally
attaches a logs bundle written to a temp folder.

Goals:

- Capture recent backend and frontend logs in bounded ring buffers.
- Add a hidden workflow `improve-kandev` with three manually-advanced steps.
- Add an Improve Kandev settings entry that opens a tailored task-create
  dialog with the kandev repo, `main` branch, and a starter description
  pre-filled.
- Filter hidden workflows out of management UI and the workflow picker.

Out of scope (deferred):

- Automatic step transitions, rate limiting, log redaction, fork management.
- A generic feedback inbox.

## 2. Prerequisites

- No external services. Uses existing `repoclone.Cloner`,
  `worktree.Manager`, `task.Service`, `orchestrator.Service`, and the
  workflow YAML loader.
- New temp directory layout: `os.MkdirTemp("", "kandev-improve-*")`.
- Workflows YAML loader at `apps/backend/config/workflows/loader.go` already
  supports loading additional templates from `apps/backend/config/workflows/`.
- The `Workflow` model gains a `hidden` boolean column. SQLite migration required.

## 3. Implementation Steps

### Step 1 — Backend log ring buffer (RED → GREEN)

- **File (new)**: `apps/backend/internal/common/logger/buffer/buffer.go`
  - `RingBuffer` struct: thread-safe, FIFO, capped at 2000 entries.
  - `Entry` struct: `{Timestamp, Level, Logger, Message, Fields, Caller}`.
  - `Snapshot()` returns a copy of current entries.
  - `WriteSyncer` adapter (implements `zapcore.WriteSyncer`) that decodes the
    line emitted by the configured zap encoder back into an `Entry`. Prefer
    using a small custom `zapcore.Core` that hooks into entry creation rather
    than parsing strings — `zap.Hooks` returns a wrapped core.
- **File (modify)**: `apps/backend/internal/common/logger/logger.go`
  - In `NewLogger`, wrap the core with `zap.Hooks(...)` (or a small custom
    `zapcore.Core` tee) that pushes each entry into the ring buffer.
  - Expose `(l *Logger) BufferSnapshot() []buffer.Entry`.
- **File (new)**: `apps/backend/internal/common/logger/buffer/buffer_test.go`
  - FIFO eviction at capacity.
  - Concurrent writes are serialized.
  - `Snapshot()` returns an isolated copy.

### Step 2 — Frontend log ring buffer + console interceptor (RED → GREEN)

- **File (new)**: `apps/web/lib/logger/buffer.ts`
  - In-memory ring buffer (cap 500). `Entry` shape mirrors backend
    (`timestamp`, `level`, `source`, `message`, `args`).
  - `snapshot()`, `clear()` exports.
- **File (new)**: `apps/web/lib/logger/intercept.ts`
  - `installConsoleInterceptor()` wraps `console.debug/info/warn/error` so the
    original behavior is preserved and each call also pushes to the ring.
  - Hooks `window.onerror` and `window.addEventListener("unhandledrejection")`.
  - Idempotent (no-op on repeat calls).
- **File (modify)**: `apps/web/app/layout.tsx` (or the closest top-level
  client boot) — call `installConsoleInterceptor()` once on the client.
- **Files (new tests)**: `apps/web/lib/logger/buffer.test.ts`,
  `apps/web/lib/logger/intercept.test.ts`.

### Step 3 — `hidden` flag on Workflow (RED → GREEN)

- **File (modify)**: `apps/backend/internal/task/models/models.go` — add
  `Hidden bool` to `Workflow`.
- **File (new)**: `apps/backend/internal/task/repository/migrations/NNNN_workflow_hidden.sql`
  (use the existing migrations pattern; add `ALTER TABLE workflows ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0`).
- **Files (modify)**: workflow repository read/write paths — include `hidden`
  in `SELECT` / `INSERT` / `UPDATE`.
- **File (modify)**: workflow service `ListWorkflows` — accept an
  `IncludeHidden bool` option (default false). Existing callers pass false.
- **File (modify)**: workflow API DTOs and the OpenAPI/types generation if
  applicable; ensure the `hidden` field flows to the frontend.
- **Files (modify, frontend)**:
  - `apps/web/lib/state/slices/settings/...` (or wherever workflows hydrate)
    — preserve `hidden` field on the type.
  - `apps/web/app/settings/workspace/workspace-workflows-client.tsx` — filter
    `workflow.hidden !== true` before rendering `WorkflowList`.
  - `apps/web/components/workflow-selector-row.tsx` — same filter on the
    picker list.
- **Tests**: repository test for `hidden` round-trip; one frontend store test
  ensuring hidden workflows are excluded from the picker selector.

### Step 4 — `improve-kandev` workflow YAML (GREEN)

- **File (new)**: `apps/backend/config/workflows/improve-kandev.yml`
  - Three steps with `is_start_step` on improve, `allow_manual_move: true` on
    all, `on_enter: auto_start_agent`, and per-step `prompt` text matching
    the spec. Top-level YAML does not yet support `hidden`; add support:
- **File (modify)**: `apps/backend/config/workflows/loader.go` — extend
  `templateYAML` with `Hidden bool` and propagate when materializing the
  template. When the loader instantiates a workflow from a template marked
  hidden, the resulting workflow is created with `Hidden: true`.
- **File (modify)**: workflow template materialization path (the place that
  creates a `Workflow` row from a `WorkflowTemplate`) — copy `IsSystem` /
  `Hidden` into the resulting workflow.
- **Test**: a loader test that parses `improve-kandev.yml` and asserts the
  `hidden` flag is propagated.

### Step 5 — Bootstrap endpoint (RED → GREEN)

- **File (new)**: `apps/backend/internal/system/handlers/improve_kandev.go`
  - `POST /api/v1/system/improve-kandev/bootstrap`
    - Ensures kandev repo is cloned via
      `repoclone.Cloner.EnsureCloned("https://github.com/kdlbs/kandev", "kdlbs", "kandev")`.
    - Ensures a `Repository` row exists in the user's active workspace pointing
      at the local path; creates one on first call, reuses thereafter (idempotent).
    - Ensures the hidden `improve-kandev` workflow exists in the workspace
      (creates it from the template on first call).
    - Creates a temp directory via `os.MkdirTemp("", "kandev-improve-*")` and
      writes `metadata.json` (version, OS/arch, health snapshot, captured_at)
      and `backend.log` (zap ring buffer snapshot, formatted as plain text,
      one entry per line).
    - Response: `{repository_id, workflow_id, branch: "main",
      bundle_dir, bundle_files: {metadata, backend_log, frontend_log_target}}`.
  - `POST /api/v1/system/improve-kandev/bundle/frontend-log`
    - Body: `{bundle_dir, entries: FrontendLogEntry[]}`.
    - Writes plain-text `frontend.log` into the bundle dir.
    - Validates `bundle_dir` matches the temp pattern to refuse arbitrary writes.
- **File (modify)**: `cmd/kandev/helpers.go` — register the new routes.
- **File (new tests)**: `improve_kandev_test.go`
  - First call clones + creates repo row + workflow row; second call is
    idempotent.
  - Bundle dir validation rejects paths outside the temp pattern.

### Step 6 — TaskCreateDialog extension props (RED → GREEN)

- **File (modify)**: `apps/web/components/task-create-dialog.tsx`
  - Add two optional props to `TaskCreateDialogProps`:
    - `extraFormSlot?: React.ReactNode` — rendered inside the form, directly
      below the description textarea.
    - `transformDescriptionBeforeSubmit?: (desc: string) => Promise<string> | string`
      — invoked once with the current description text just before the
      create-task API call; the returned string is used as the description.
  - Submit handler awaits the transform, swaps the description, then proceeds.
- **File (new test)**: `task-create-dialog-extra-slot.test.tsx`
  - Rendering with a slot mounts the node.
  - The transform is awaited and its result is sent in the API payload.

### Step 7 — `ImproveKandevDialog` wrapper (GREEN)

- **File (new)**: `apps/web/components/improve-kandev-dialog.tsx`
  - Props: `{open, onOpenChange, onSuccess?}`.
  - On mount: calls bootstrap endpoint to obtain `repository_id`,
    `workflow_id`, `bundle_dir`, `bundle_files`.
  - Owns local state: `includeLogs: boolean` (default true),
    `logsPreviewOpen: boolean`.
  - Renders `<TaskCreateDialog>` with:
    - `workspaceId` = active workspace id from store.
    - `workflowId` = bootstrap response.
    - `initialValues = { title: "", description: STARTER_TEMPLATE,
      repositoryId, branch: "main" }`.
    - `extraFormSlot` = a `Card` containing the dialog's explanatory header
      copy ("Help improve kandev — flow: Improve → Test → PR — contribute…")
      plus the "Include recent logs" checkbox with an expandable preview.
    - `transformDescriptionBeforeSubmit` = when `includeLogs` is true, sends
      the current ring-buffer snapshot to the frontend-log endpoint to write
      `frontend.log`, then appends:
      ```
      ---
      Context bundle for the agent:
      - {metadata path}
      - {backend.log path}
      - {frontend.log path}
      ```
- **File (new test)**: `improve-kandev-dialog.test.tsx`
  - Bootstrap is called once on open.
  - With logs on, the description sent to create-task contains all three
    bundle paths.
  - With logs off, no bundle reference is appended; the frontend-log endpoint
    is not called.

### Step 8 — Top-bar entry point (GREEN)

- **File (modify)**: `apps/web/components/kanban/kanban-header.tsx`
  - Add a new `ImproveKandevTopbarButton` component next to
    `GitHubTopbarButton` and `JiraTopbarButton` (inside the same
    `TooltipProvider`), following the same icon-only `Tooltip` +
    `Button variant="outline" size="icon"` pattern.
  - Icon: `IconBug` (or similar, e.g. `IconSparkles`) from `@tabler/icons-react`.
  - Tooltip text: "Improve kandev".
  - Click handler toggles a `useState` boolean that mounts
    `<ImproveKandevDialog open={…} onOpenChange={…} onSuccess={…} />`.
  - On `onSuccess`, navigate to `/t/{taskId}` so the user lands on the
    running task.
- **File (modify)**: `apps/web/components/kanban/kanban-header-mobile.tsx`
  - Add the same button to the mobile header for parity (icon-only,
    visible alongside the existing icon controls).
- **File (new test)**: `apps/web/components/kanban/kanban-header.test.tsx`
  (or extend an existing test) — assert the button renders with its
  tooltip and that clicking it opens the dialog.

### Step 9 — Pre-flight `gh auth` check (GREEN)

- **File (modify)**: `improve-kandev-dialog.tsx`
  - On mount, call `/api/v1/system/health`; if the GitHub-auth checker reports
    failure, render a blocking banner inside the dialog (replacing the form)
    explaining the issue and link to the relevant settings.
  - Disable submit while the banner is visible.
- **File (new test)**: extends the dialog test to assert that an unhealthy
  `gh auth` result blocks submission.

### Step 10 — Verify

- From `apps/`:
  - `pnpm --filter @kandev/web test -- logger`
  - `pnpm --filter @kandev/web test -- task-create-dialog-extra-slot`
  - `pnpm --filter @kandev/web test -- improve-kandev-dialog`
- From `apps/backend`:
  - `make test`
- From repo root:
  - `make fmt` then `make typecheck test lint` per AGENTS.md.

## 4. File Changes Summary

Created (backend):

- `internal/common/logger/buffer/buffer.go` (+ test).
- `internal/system/handlers/improve_kandev.go` (+ test).
- `config/workflows/improve-kandev.yml`.
- Migration file for `workflows.hidden` column.

Modified (backend):

- `internal/common/logger/logger.go` — wire ring buffer.
- `internal/task/models/models.go` — `Hidden` field on `Workflow`.
- `internal/task/repository/...` — read/write `hidden`.
- `internal/workflow/service/...` — filter hidden in list calls.
- `config/workflows/loader.go` — propagate `hidden` from YAML.
- `cmd/kandev/helpers.go` — register new routes.

Created (frontend):

- `lib/logger/buffer.ts` + `intercept.ts` (+ tests).
- `lib/api/domains/improve-kandev-api.ts`.
- `components/improve-kandev-dialog.tsx` (+ test).

Modified (frontend):

- `app/layout.tsx` — install console interceptor.
- `components/task-create-dialog.tsx` — `extraFormSlot` +
  `transformDescriptionBeforeSubmit` props.
- `components/workflow-selector-row.tsx` — filter hidden workflows.
- `app/settings/workspace/workspace-workflows-client.tsx` — filter hidden.
- `components/kanban/kanban-header.tsx` — `ImproveKandevTopbarButton`
  with tooltip; mounts `<ImproveKandevDialog>` and routes to `/t/{taskId}`
  on success.
- `components/kanban/kanban-header-mobile.tsx` — same button for mobile.
- workflow store types/hydration to keep the `hidden` field.

Deleted: none.

## 5. Testing Strategy

Backend (Go, `*_test.go`):

- `buffer_test.go`: FIFO eviction, concurrency, snapshot isolation.
- Workflow loader test: `improve-kandev.yml` parses with `hidden: true`.
- Workflow repository test: `hidden` round-trips through SQLite.
- Workflow service test: `ListWorkflows` filters hidden by default.
- Improve-kandev handler test: bootstrap is idempotent; bundle-dir
  validation rejects paths outside the temp pattern.

Frontend (vitest):

- Logger buffer + interceptor tests.
- TaskCreateDialog extra-slot + transform-before-submit tests.
- ImproveKandevDialog dialog tests (bootstrap, logs on/off, gh auth gating).

Manual smoke:

1. Start kandev. Open Settings → General → Improve kandev.
2. Submit with logs on; verify the new task contains a bundle reference.
3. Watch the agent run on Improve; move to Test; verify URLs appear.
4. Move to PR; verify a PR is opened against `main`.

## 6. Rollback Plan

- Frontend changes are additive (a new page, a new dialog, two optional
  dialog props). Reverting the merge restores the previous behavior.
- Backend `workflows.hidden` column is `DEFAULT 0`; existing rows are
  unaffected. The migration is reversible by `ALTER TABLE … DROP COLUMN`.
- The `improve-kandev` workflow YAML is only instantiated on first use of
  the feature; reverting before any user invokes it leaves no rows behind.

## 7. Estimated Effort

- Complexity: **medium-large** (cross-cutting: backend log infra, schema
  change, workflow loader change, dialog extension, new settings page).
- Time: ~2 days including TDD cycles, manual smoke, and CI fixups.
