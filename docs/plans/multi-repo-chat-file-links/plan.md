---
spec: docs/specs/tasks/system-design/attach-workspace-sources.md
decision: docs/decisions/2026-07-22-runtime-mutable-task-workspace-sources.md
created: 2026-08-01
status: completed
---

# Implementation Plan: Fix Multi-Repository Chat File Links

## Overview

Keep the primary repository worktree and the effective task workspace root as separate session
fields. The backend will return an environment-backed `workspace_path` in every task-session
projection, while the frontend will use that root for Files and chat path resolution and preserve
`worktree_path` as the primary repository path. The repair covers live source adoption, sibling
materialization, reload/hydration, desktop chat, and the native mobile file viewer.

## Confirmed Root Cause

The failing task had a promoted task root containing three repositories. The requested file existed
at `<task-root>/kandev/docs/public/agents-and-profiles.md`, but the browser restored the session's
flattened primary `worktree_path` (`<task-root>/kandev`) after a refresh. Markdown resolution removed
that prefix and sent `docs/public/agents-and-profiles.md` to agentctl, whose file API was rooted at
`<task-root>`; it therefore looked for `<task-root>/docs/...` and returned `ENOENT`.

Three contracts currently conflict:

1. `session.workspace_sources.updated` and sibling `session.agentctl_ready` temporarily overwrite
   `worktree_path` with the promoted task root.
2. Full and summary session DTOs omit the model's `workspace_path` and flatten the first session
   worktree back into `worktree_path`, so API hydration reverses the live update.
3. Chat and Files consumers use `worktree_path` as both repository identity and file-service root.

The file endpoint itself behaves correctly: with no explicit repository argument it resolves a
safe path beneath agentctl's adopted task root. The repair is to supply that endpoint with the
correct task-root-relative path, not to widen its filesystem boundary.

---

## Backend

### Authoritative session workspace projection

Files:

- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/session_test.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/dto/converters_test.go`

Changes:

- Extend the common task-session read query to left-join the linked task environment and project
  `task_environments.workspace_path` when present, falling back to the persisted
  `task_sessions.workspace_path` for legacy or environment-less sessions.
- Keep session creation/update persistence and the existing first-worktree flattening unchanged;
  `WorkspacePath` represents the effective file-service root and `WorktreePath` remains the primary
  repository path.
- Add `workspace_path` to both `TaskSessionDTO` and `TaskSessionSummaryDTO`, and populate it from the
  model in both converters.
- Do not add a schema migration: both workspace-path columns and the session-to-environment foreign
  key already exist.

### Backend behavioral coverage

- Seed a task session whose persisted workspace and primary worktree point at the repository, then
  promote its task environment to the parent task root. Assert both `GetTaskSession` and
  `ListTaskSessions` return the promoted `WorkspacePath` while retaining the primary worktree.
- Assert the full and summary DTO converters serialize the same `workspace_path` alongside the
  flattened primary `worktree_path`.
- Retain a legacy fallback case with no linked task environment so existing single-repository and
  repository-less sessions continue returning their persisted workspace path.

---

## Frontend

### Separate workspace and worktree state

Files:

- `apps/web/lib/types/http.ts`
- `apps/web/lib/state/slices/session/session-slice.ts`
- `apps/web/lib/state/slices/session/session-slice.upsert.test.ts`
- `apps/web/lib/ws/handlers/agent-session.ts`
- `apps/web/lib/ws/handlers/agent-session.test.ts`
- `apps/web/app/office/tasks/[id]/office-dockview-layout.tsx`
- `apps/web/components/task/file-browser-path.ts`
- `apps/web/components/task/file-browser-path.test.ts`
- `apps/web/components/task/file-tab-content.tsx`
- `apps/web/components/task/file-tab-content.test.tsx`
- `apps/web/components/task/file-editor-panel.tsx`
- `apps/web/components/task/file-editor-panel.image.test.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.test.tsx`
- `apps/web/hooks/use-lsp-file-opener.ts`
- `apps/web/hooks/use-lsp-file-opener.test.ts`

Changes:

- Add optional `workspace_path` to the frontend task-session type and preserve it during partial
  session merges.
- Change workspace-source adoption and sibling-materialization handlers to write
  `workspace_path`; they must not overwrite the primary `worktree_path`.
- Seed `workspace_path` from initial agentctl/ensure-session payloads when available, with
  `worktree_path` as the compatibility fallback for older payloads.
- Update the Office ensure-session bridge to populate `workspace_path` instead of manufacturing a
  `worktree_path` value.
- Add regression tests for event ordering in both directions: a live workspace-root event followed
  by API hydration, and authoritative API hydration followed by a partial event.

### Resolve file paths from the effective root

Files:

- `apps/web/lib/session-workspace-path.ts` (new)
- `apps/web/lib/session-workspace-path.test.ts` (new)
- `apps/web/components/shared/markdown-components.tsx`
- `apps/web/components/shared/markdown-components.test.tsx`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/app/office/tasks/[id]/advanced-panels/chat-panel.tsx`
- `apps/web/components/quick-chat/quick-chat-content.tsx`
- `apps/web/components/task/file-browser.tsx`

Changes:

- Add one pure resolver that selects `workspace_path` first and falls back to `worktree_path` for
  backward compatibility.
- Use that resolver wherever the session root is passed into shared chat Markdown/tool rendering,
  the fallback Markdown link context, the Files browser root, desktop/mobile file viewers, and LSP
  navigation.
- Keep the existing Markdown safety checks: traversal, unrelated host absolute paths, and paths
  outside the known workspace remain rejected.
- When the task root is `<task-root>`, resolve absolute links under the primary repository to
  `kandev/...` and links under siblings to their own top-level source directory. Pass those paths
  through the existing file-open action; do not change the agentctl file API or infer paths with
  `filepath.Dir` in the browser.

### Frontend behavioral coverage

- Extend Markdown component tests with primary and sibling absolute links beneath a multi-source
  task root, a post-hydration session root, a legacy single-repository fallback, and an outside-root
  rejection.
- Extend session store and WS tests to prove `workspace_path` survives refresh races while
  `worktree_path` remains unchanged.
- Cover the pure root selector independently so all chat/File consumers share one precedence rule.
- Cover workspace-relative LSP conversion, registration, cleanup, and cursor scrolling with
  distinct workspace and primary worktree paths.

---

## E2E Tests

Files:

- `apps/web/e2e/tests/task/add-workspace-sources.spec.ts`
- `apps/web/e2e/tests/task/mobile-add-workspace-sources.spec.ts`

Desktop scenario:

1. Create a Worktree task and attach a second local repository through the existing UI flow.
2. Obtain the authoritative environment workspace root, seed an agent message containing absolute
   Markdown links to known files in the primary and attached repositories, and reload the page.
3. Click each link and assert the exact file content opens without the **Failed to open file** toast.
4. Assert the refreshed session payload still reports a primary `worktree_path` distinct from the
   promoted `workspace_path`.

Mobile scenario:

1. Reuse the native mobile add-sources flow, seed an absolute chat link beneath the promoted root,
   and reload.
2. Tap the link from the Chat tab and assert the native mobile file viewer shows the expected file
   content and close control with no horizontal overflow or failure toast.

This is shared state/path normalization with no new controls or layout. The mobile E2E verifies
capability parity; no separate mobile composition is introduced.

---

## Implementation Waves

Wave 1:

- [x] [task-01-expose-authoritative-workspace-root](task-01-expose-authoritative-workspace-root.md)

Wave 2:

- [x] [task-02-resolve-chat-links-from-workspace-root](task-02-resolve-chat-links-from-workspace-root.md)

Execution is sequential in the primary conversation. No task is marked parallel-safe, and these
waves do not authorize subagents.

## Verification Commands

```bash
make -C apps/backend test

cd apps/web
rtk pnpm exec vitest run \
  lib/session-workspace-path.test.ts \
  lib/state/slices/session/session-slice.upsert.test.ts \
  lib/ws/handlers/agent-session.test.ts \
  components/shared/markdown-components.test.tsx \
  components/task/file-browser-path.test.ts \
  components/task/file-tab-content.test.tsx \
  components/task/mobile/mobile-file-viewer-panel.test.tsx \
  components/task/file-editor-panel.image.test.tsx \
  hooks/use-lsp-file-opener.test.ts
rtk pnpm run typecheck
rtk pnpm e2e:run --host --no-build tests/task/add-workspace-sources.spec.ts --grep "adds a local repository"
rtk pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-add-workspace-sources.spec.ts
```

Run `pnpm install --frozen-lockfile` from `apps/` before frontend verification when this worktree
does not yet have `apps/node_modules/`.

## Risks

- The shared session SELECT feeds many read paths; its column order must remain exactly aligned with
  both single-row and multi-row scanners.
- Some legacy sessions have no linked task environment. The SQL fallback and frontend fallback must
  keep those sessions working without inventing a parent directory.
- Treating `worktree_path` as a workspace root anywhere else can reintroduce the race. The shared
  selector should be used only by file-root consumers; repository identity and Git operations must
  continue using worktree/repository fields.
- E2E mock-agent messages must escape absolute paths without changing their Markdown link target.

## Out of Scope

- Changing agent or terminal working directories.
- Changing source attachment, materialization, or agentctl rescan behavior.
- Weakening workspace containment or enabling links outside the adopted task root.
- Redesigning chat file-open callbacks to carry repository metadata; task-root-relative paths are
  already supported by the file service and Files tree.
- Adding or removing workspace sources.
- Public documentation changes; this repairs an existing documented behavior and adds no new user
  controls or terminology.

## Architecture and Documentation Review

No new ADR is required. The plan implements the existing task-scoped source ownership and effective
workspace-root contracts from ADR-2026-07-22 and the existing attach-workspace-sources spec. Public
documentation is unaffected.

## Open Questions

None.

## Verification Results

- `make -C apps/backend test` — passed.
- Focused Vitest suite (workspace selector, session merge/WS handlers, file viewers, LSP paths, and
  Markdown links) — 115 tests passed in 9 files.
- `rtk pnpm run typecheck` — passed.
- Targeted ESLint on changed frontend files — passed with zero warnings.
- Desktop Chromium add-sources regression — passed; absolute links opened in both the primary and
  attached repository after reload.
- Mobile Chrome add-sources regression — passed; the attached-repository link opened in the native
  file viewer after reload.
- `rtk git diff --check` — passed.
