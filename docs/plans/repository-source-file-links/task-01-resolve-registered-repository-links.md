---
id: "01-resolve-registered-repository-links"
title: "Resolve registered repository links"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMMENT-MARKDOWN-001
acceptance_criteria:
  - AC-UI-COMMENT-MARKDOWN-001.5
  - AC-UI-COMMENT-MARKDOWN-001.9
  - AC-UI-COMMENT-MARKDOWN-001.10
system_design:
  - ../../specs/ui/system-design/comment-markdown.md
---

# Task 01: Resolve Registered Repository Links

## Summary

Resolve a task-linked repository's canonical source-checkout paths to the matching active session
worktree before opening Markdown file links. Preserve existing URL and direct-workspace behavior,
fail closed for unrelated host paths, and prove the shared desktop/phone outcome.

## In scope

- Add and test the pure repository-identity alias and Markdown href resolver.
- Derive aliases once per interactive transcript and carry them through the shared Markdown context.
- Route eligible links to the existing active-workspace file action and suppress unmatched host-path
  navigation.
- Extend the existing attached-source desktop and phone E2E scenarios.

## Out of scope

- Backend, persistence, file-service, workspace materialization, editor-layout, or localization
  changes.
- Opening the original registered checkout or guessing mappings without matching repository IDs.
- New Markdown syntax or automatic linking of non-link text.

## Acceptance

- A Markdown href beneath a task-linked repository's registered `local_path`, including a supported
  source selector, resolves to the matching active worktree's task-workspace-relative file target.
- Direct workspace paths and external/internal web links keep their current behavior; mismatched,
  ambiguous, traversing, or unrelated host paths produce no file request or task-route navigation.
- The existing desktop and `mobile-chrome` attached-source flows open content from the active
  worktree through their native file surfaces.

## Verification

```bash
cd apps/web
pnpm exec vitest run \
  lib/markdown/file-link-target.test.ts \
  components/shared/markdown-components.test.tsx \
  components/shared/memoized-markdown.test.tsx \
  components/shared/task-markdown-file-link-provider.test.tsx
pnpm exec eslint \
  lib/markdown/file-link-target.ts \
  lib/markdown/file-link-target.test.ts \
  components/shared/markdown-components.tsx \
  components/shared/markdown-components.test.tsx \
  components/shared/memoized-markdown.tsx \
  components/shared/memoized-markdown.test.tsx \
  components/shared/task-markdown-file-link-provider.tsx \
  components/shared/task-markdown-file-link-provider.test.tsx \
  components/task/task-chat-panel.tsx \
  'app/office/tasks/[id]/advanced-panels/chat-panel.tsx' \
  e2e/tests/task/add-workspace-sources.spec.ts \
  e2e/tests/task/mobile-add-workspace-sources.spec.ts \
  --max-warnings=0
pnpm run typecheck
pnpm e2e:run --project chromium tests/task/add-workspace-sources.spec.ts -- --grep "adds a local repository and folder successively"
pnpm e2e:run --project mobile-chrome tests/task/mobile-add-workspace-sources.spec.ts
```

Run `pnpm install --frozen-lockfile` from `apps/` first when workspace dependencies are absent.

## Files likely touched

- `apps/web/lib/markdown/file-link-target.ts`
- `apps/web/lib/markdown/file-link-target.test.ts`
- `apps/web/components/shared/markdown-components.tsx`
- `apps/web/components/shared/markdown-components.test.tsx`
- `apps/web/components/shared/memoized-markdown.tsx`
- `apps/web/components/shared/memoized-markdown.test.tsx`
- `apps/web/components/shared/task-markdown-file-link-provider.tsx`
- `apps/web/components/shared/task-markdown-file-link-provider.test.tsx`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/app/office/tasks/[id]/advanced-panels/chat-panel.tsx`
- `apps/web/e2e/tests/task/add-workspace-sources.spec.ts`
- `apps/web/e2e/tests/task/mobile-add-workspace-sources.spec.ts`

## Dependencies

None.

## Risks

- Incorrect provider scoping can route a link through the globally active session instead of the
  transcript's own session.
- Context changes can bypass the existing Markdown memo boundary or leave rendered links stale.
- Broad absolute-path detection can accidentally intercept legitimate internal application routes.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-COMMENT-MARKDOWN-001`, especially acceptance criteria 1.5, 1.9, and 1.10.
- `docs/specs/ui/system-design/comment-markdown.md` trusted-root and failure boundaries.
- Existing shared Markdown component tests and attached-source desktop/mobile E2E patterns.

## Results

- Added identity-qualified repository source aliases and a fail-closed Markdown file-target
  resolver. Direct workspace links, external URLs, source selectors, traversal rejection, and
  unmatched host paths remain covered by focused unit tests.
- Hardened drive-letter normalization, task-membership filtering, and inert handling for contained
  targets that the resolver does not recognize as openable files.
- Added transcript-scoped alias context for Kanban and Office chat hosts, including the active
  workspace fallback used while task projections hydrate. Existing desktop and phone file-open
  actions receive only task-workspace-relative paths.
- Extended the attached-source desktop and mobile E2E scenarios to cite the registered source
  checkout and assert active-worktree content in the native file surfaces.
- Verification passed: 50 focused Vitest tests, targeted ESLint with zero warnings, Prettier
  checks, TypeScript typecheck, Chromium desktop E2E, and `mobile-chrome` E2E.
