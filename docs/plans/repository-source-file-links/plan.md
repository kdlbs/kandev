---
created: 2026-09-05
status: done
requirements:
  - REQ-UI-COMMENT-MARKDOWN-001
system_design:
  - ../../specs/ui/system-design/comment-markdown.md
legacy_specs: []
---

# Implementation Plan: Registered Repository File Links

## Overview

Make Markdown links that cite a task-linked repository's registered source checkout open the
equivalent file from the active task workspace. The frontend will derive identity-qualified source
aliases once at the transcript boundary, resolve links through a pure fail-closed helper, and reuse
the existing desktop tab and phone viewer actions. One vertical work order keeps the resolver,
context wiring, and desktop/mobile regression evidence reviewable together.

## Confirmed root cause

`resolveAbsoluteMarkdownFileHref` in `components/shared/markdown-components.tsx` accepts an absolute
path only when it is beneath the supplied session `workspace_path` or legacy `worktree_path`. An
agent can instead cite the registered repository's canonical `local_path`, which differs whenever
Kandev created an isolated task worktree. The resolver then returns `null`; `MarkdownFileAnchor`
installs no click handler but retains the leading-slash `href`, so the browser resolves it against
the Kandev origin (for example, `https://kandev.nova/home/...`) instead of opening a file tab.

A throwaway Vitest case reproduced the report with a task worktree under `.kandev/tasks` and a link
to `/home/jcfs/kandev-plugins/.../ui/bundle.js:61`: the expected `openFile("ui/bundle.js")` call had
zero invocations. The throwaway test was removed, and the unchanged
`components/shared/markdown-components.test.tsx` baseline passes all 28 tests.

## Scope

### In scope

- Match a task-linked `Repository.local_path` to the active session worktree with the same
  `repository_id`.
- Convert the cited source-checkout suffix to the corresponding task-workspace-relative file path.
- Preserve current direct workspace/worktree, root-relative, URL, and supported source-selector
  behavior.
- Keep unmatched host filesystem paths inside the current task without issuing a file request.
- Prove the mapped file opens through the existing desktop and phone file viewers.

### Out of scope

- Reading or editing the registered source checkout itself.
- Mapping repositories not linked to the rendered task or absent from the rendered session.
- Guessing repository identity from folder names, URL owners, or path suffixes.
- Turning unlinked plain text, inline code, directories, or `file://` URLs into file actions.
- Changing workspace-source materialization, file-service containment, editor layout, or mobile
  composition.

## Technical approach

### Fail-closed file-link targets

- Add `apps/web/lib/markdown/file-link-target.ts` with pure types and functions for:
  - building source-root aliases from the effective workspace root, task repository links,
    workspace repository records, and identity-qualified session worktrees;
  - normalizing Markdown hrefs and supported source-location suffixes;
  - resolving direct workspace paths first and registered source aliases second; and
  - classifying unmatched absolute host paths so the anchor can suppress accidental same-origin
    navigation without treating them as workspace files.
- Require path-segment containment, reject traversal and malformed encoding, choose the
  most-specific eligible source root, and omit ambiguous mappings.
- Return only task-workspace-relative targets. Do not pass a registered `local_path` to
  `requestFileContent` or any editor action.

### Transcript-scoped repository aliases

- Add `apps/web/components/shared/task-markdown-file-link-provider.tsx` to derive one stable alias
  set for a rendered task/session. Reuse `useTask`, `resolveSessionWorktrees` or
  `useSessionWorktrees`, and `repositories.itemsByWorkspaceId`; correlate every source and target
  by `repository_id`.
- Extend `MarkdownFileLinkContext` in
  `apps/web/components/shared/markdown-components.tsx` with the derived aliases. Keep external URL
  behavior unchanged, intercept resolved files, and prevent default navigation for unmatched
  filesystem-shaped absolute paths.
- Update `apps/web/components/shared/memoized-markdown.tsx` to inherit aliases from the enclosing
  transcript provider while retaining explicit per-renderer workspace-root and open-action
  overrides. Keep the parsed-message memo boundary intact.
- Wrap interactive task transcript hosts with the provider, including
  `apps/web/components/task/task-chat-panel.tsx` and
  `apps/web/app/office/tasks/[id]/advanced-panels/chat-panel.tsx`. The provider must use the
  rendered panel's task/session IDs, not global active-session state, so side-by-side panels cannot
  cross-route links.

### Desktop and phone evidence

- Extend `apps/web/e2e/tests/task/add-workspace-sources.spec.ts` to seed a Markdown link using the
  attached repository's original fixture path plus a source selector. Assert that the active
  worktree file content and Dockview tab open and that the `/t/<task>` route does not change.
- Change or extend `apps/web/e2e/tests/task/mobile-add-workspace-sources.spec.ts` to tap the same
  source-path form. Assert the existing `MobileFileViewerPanel` shows the active-worktree-relative
  path/content and close control with no document horizontal overflow.
- No new mobile composition is needed. The existing inline link is the entry point, and the shipped
  phone file viewer remains the single content/scroll surface.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-UI-COMMENT-MARKDOWN-001.5` | Existing and extended direct workspace-link cases in `markdown-components.test.tsx` |
| `AC-UI-COMMENT-MARKDOWN-001.9` | New pure alias tests, shared Markdown click test, desktop E2E, and phone E2E |
| `AC-UI-COMMENT-MARKDOWN-001.10` | Pure mismatch/traversal tests and inert-anchor component test |

Unit coverage must include primary and sibling repositories, legacy single-worktree fallback,
source selectors, encoded paths, repository-ID mismatch, ambiguous aliases, parent traversal,
unmatched host paths, and unchanged external URLs.

## E2E tests

- `apps/web/e2e/tests/task/add-workspace-sources.spec.ts`, Chromium: a source-checkout absolute link
  opens the matching active worktree file in a Dockview tab without route navigation.
- `apps/web/e2e/tests/task/mobile-add-workspace-sources.spec.ts`, `mobile-chrome`: tapping the same
  source-path form opens the existing native phone file viewer with the expected active-worktree
  content and containment.

## Work orders

- [x] [Task 01: Resolve registered repository links](task-01-resolve-registered-repository-links.md)

## Verification results

- Unit: four focused Vitest files passed, 48 tests total.
- Static checks: targeted ESLint passed with `--max-warnings=0`; targeted Prettier checks passed;
  `pnpm run typecheck` passed.
- Desktop: the attached-source Chromium scenario passed and proved a registered checkout link
  opened the active worktree file/tab without changing the task URL.
- Mobile: the attached-source `mobile-chrome` scenario passed and proved the registered checkout
  link opened active-worktree content in the native viewer.
- `git diff --check` passed.

## Risks

- A task or repository cache can be briefly incomplete during hydration. Alias resolution must fail
  closed and become actionable after authoritative state arrives without mutating message content.
- Multi-repository sessions can contain identical filenames and similar directory names. Repository
  IDs and active worktree paths, never basenames, must select the target.
- Transcript messages are aggressively memoized. Context propagation must update links when alias
  state changes without causing all messages to subscribe independently to repository state.
- The existing link resolver also distinguishes internal routes and external URLs. Filesystem-path
  suppression must not intercept valid `/office/...`, HTTP(S), issue, or application links.
- E2E assertions must prove the file came from the active worktree rather than the original fixture
  checkout.
