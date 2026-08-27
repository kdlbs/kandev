---
id: "02-resolve-chat-links-from-workspace-root"
title: "Resolve chat links from workspace root"
status: done
wave: 2
depends_on: ["01-expose-authoritative-workspace-root"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/attach-workspace-sources.md"
---

# Task 02: Resolve chat links from the workspace root

## Acceptance

- Frontend session state stores `workspace_path` independently of `worktree_path` and preserves both
  across live events, partial merges, API hydration, and reload.
- Workspace-source and sibling-materialization events update only the effective workspace root; the
  primary worktree remains stable for repository-aware consumers.
- Shared task, Office, quick-chat, Markdown-fallback, and Files-root consumers prefer
  `workspace_path` and fall back to `worktree_path` for legacy payloads.
- Desktop, native-mobile, and LSP file viewers use the same workspace-root selector for display,
  navigation, and relative-path conversion.
- Absolute chat links under primary and attached repositories resolve to the correct
  task-root-relative file path; traversal and outside-root absolute paths remain blocked.
- Desktop and native mobile flows open the linked file after a full reload without showing a file
  failure notification.

## Verification

```bash
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
rtk pnpm e2e:run -- --project=chromium tests/task/add-workspace-sources.spec.ts
rtk pnpm e2e:run -- --project=mobile-chrome tests/task/mobile-add-workspace-sources.spec.ts
```

If dependencies are absent, first run `rtk pnpm install --frozen-lockfile` from `apps/`.

## Files Likely Touched

- `apps/web/lib/types/http.ts`
- `apps/web/lib/session-workspace-path.ts`
- `apps/web/lib/session-workspace-path.test.ts`
- `apps/web/lib/state/slices/session/session-slice.ts`
- `apps/web/lib/state/slices/session/session-slice.upsert.test.ts`
- `apps/web/lib/ws/handlers/agent-session.ts`
- `apps/web/lib/ws/handlers/agent-session.test.ts`
- `apps/web/components/shared/markdown-components.tsx`
- `apps/web/components/shared/markdown-components.test.tsx`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/app/office/tasks/[id]/advanced-panels/chat-panel.tsx`
- `apps/web/app/office/tasks/[id]/office-dockview-layout.tsx`
- `apps/web/components/quick-chat/quick-chat-content.tsx`
- `apps/web/components/task/file-browser.tsx`
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
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/task/add-workspace-sources.spec.ts`
- `apps/web/e2e/tests/task/mobile-add-workspace-sources.spec.ts`

## Dependencies

- Task 01: full and summary session responses must provide the authoritative `workspace_path` used
  after reload.

## Parallelism

`sequential`. This task consumes Task 01 and owns the shared frontend root-selection contract plus
browser verification.

## Inputs

- Spec sections: **API surface**, **Failure modes**, and the multi-repository chat-link scenarios.
- Existing patterns: `mergeTaskSession`, `handleWorkspaceSourcesUpdated`,
  `buildAgentctlReadySessionUpdate`, `resolveAbsoluteMarkdownFileHref`, and
  `resolveFileBrowserPaths`.
- Existing desktop and native-mobile add-workspace-sources E2E fixtures.

## TDD Sequence

1. Extend the desktop and mobile workspace-source scenarios with absolute chat links and a reload;
   confirm RED with the current wrong-root file-not-found behavior.
2. Add unit regressions for workspace-root precedence, session merge/event ordering, primary and
   sibling Markdown links, legacy fallback, and outside-root rejection; confirm RED.
3. Add `workspace_path` to the session type/store, correct the WS/Office writers, and implement the
   shared root selector.
4. Route chat, Markdown fallback, Files, desktop/mobile viewer, and LSP consumers through the
   selector without changing the file API or repository-aware Git fields.
5. Reach GREEN on the focused Vitest set, typecheck, desktop E2E, and mobile E2E.

## Mobile-Parity Notes

This is a shared data/path correction, not a new control or layout. Keep the existing native mobile
Chat tab and full-screen file viewer. The mobile E2E must use touch actions and verify the viewer's
existing close affordance and absence of horizontal document overflow.

## Output Contract

Report the expected RED failures, state/event precedence rules, resolved primary and sibling paths,
desktop/mobile behavior, files changed, exact command results, risks, and blockers. Mark this task
`done` and update its plan checkbox in the primary conversation.
