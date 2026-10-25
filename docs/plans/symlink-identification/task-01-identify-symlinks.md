---
id: "01-identify-symlinks"
title: "Identify workspace symlinks"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-SYMLINK-001
acceptance_criteria:
  - AC-WORKSPACES-SYMLINK-001.1
  - AC-WORKSPACES-SYMLINK-001.2
  - AC-WORKSPACES-SYMLINK-001.3
  - AC-WORKSPACES-SYMLINK-001.4
system_design:
  - ../../specs/workspaces/system-design/symlink-identification.md
---

# Task 01: Identify workspace symlinks

## Summary

Add authoritative symlink identity to workspace change layers and render a
persistent marker beside filenames. Reuse resolved editor metadata for a visible
Symlink label across desktop, phone, and both editor providers.

## In scope

Metadata collection/projection, stale-state handling, shared marker, provider and
preview header wiring, all locales, focused tests and public documentation.

## Out of scope

New file actions, target-path disclosure, persistence, new settings, file-tree
redesign, and classification of remote/historical changes from the live checkout.

## Acceptance

1. Correct per-layer markers survive deleted/broken links and skipped patches;
   metadata-only changes refresh correctly and unknown entries are unmarked.
2. Readable symlinks display the localized editor label for both providers and
   mobile; regular targets do not. Repository isolation and save behavior hold.
3. Desktop/mobile rendered checks prove long-path containment, accessible labels,
   and existing action reachability; every planned check passes or a concrete
   environment blocker remains explicitly recorded.

## ASCII UI preview

UI-01: Workspace Changes, list or tree, desktop and phone.
Entry: task Changes; state: an uncommitted symlink and regular file.

```text
Changes > Unstaged
[+] [link] CLAUDE.md             [edit] [discard]
[+]        AGENTS.md             [edit] [discard]
```

UI-02: Opened symlink, desktop editor toolbar or phone viewer header.
Entry: edit/open CLAUDE.md; state: readable symlink.

```text
CLAUDE.md   [link] Symlink       [existing actions]
------------------------------------------------
                  file contents
```

The same inline content applies on phones inside the existing mobile surfaces;
existing navigation and touch-sized actions remain. Required: marker persists
on hover, filename can truncate, marker does not shrink, editor label is visible.
Spacing and English copy are illustrative, not pixel or localization contracts.
Regular files and unknown metadata omit the marker. Broken links retain the
Changes marker and existing file-open error. No new empty/loading surface.
Maps to AC-WORKSPACES-SYMLINK-001.1 through .4; rendered checks below prove it.

Full context: [plan previews](plan.md#ascii-ui-preview).

## Verification

Use TDD: add failing focused cases before implementation, then run:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && GOCACHE=/tmp/kandev-symlink-go-cache go test ./internal/agentctl/server/process -run 'Symlink|Mixed' -count=1)
(cd apps/backend && GOCACHE=/tmp/kandev-symlink-go-cache go test ./internal/agentctl/types/... -count=1)
(cd apps/web && pnpm test components/task/changes-panel-helpers.test.ts components/task/changes-panel-file-row.test.tsx components/task/file-editor-content.test.tsx hooks/use-file-editors.build-state.test.ts hooks/file-editors-sync.test.ts hooks/domains/session/git-change-facets.test.ts lib/state/slices/session-runtime/set-git-status-return.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && GOCACHE=/tmp/kandev-symlink-go-cache pnpm e2e:run --project chromium tests/git/symlink-file.spec.ts)
(cd apps/web && GOCACHE=/tmp/kandev-symlink-go-cache pnpm e2e:run --no-build --project mobile-chrome tests/git/mobile-symlink-identification.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The install is needed once per fresh worktree. If additional test files change,
append their exact targeted commands to this record and execute them as well.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/git.go`;
  `apps/backend/internal/agentctl/server/process/workspace_git_status.go`,
  `workspace_git_diff.go`, and new `workspace_git_symlink_test.go`.
- `apps/web/lib/types/backend.ts`,
  `apps/web/lib/state/slices/session-runtime/{types,git-status-state}.ts`;
  Changes layer projection, `changes-panel-helpers.ts`,
  `changes-panel-file-row.tsx`, and equality signatures where necessary.
- `apps/web/hooks/file-editors-sync.ts`, `file-editor-state.ts`;
  `apps/web/components/task/file-editor-{panel,content}.tsx`, `file-tab-content.tsx`,
  `file-viewer-header.tsx`, preview headers, `mobile/mobile-file-viewer-panel.tsx`;
  `apps/web/components/editors/monaco/monaco-editor-toolbar.tsx`,
  `monaco/monaco-code-editor.tsx`, `codemirror/codemirror-code-editor.tsx`, and
  shared indicator component.
- Test files named in the plan, `apps/web/src/locales/`,
  `docs/public/sessions-and-review.md`, and these design/delivery artifacts.

## Dependencies

None. Existing Git execution, access validation, mobile views and editor state
are reused. Inspect every intermediate mapper to preserve optional metadata.

## Risks

See [plan risks](plan.md#risks). Preserve current working-tree security checks
and avoid introducing per-row filesystem requests or Git subprocesses.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/symlink-identification.md)
- [Design](../../specs/workspaces/system-design/symlink-identification.md)
- Existing `symlink-file.spec.ts`, `workspace_files_test.go`, and
  `workspace_git_mixed_changes_test.go` fixture patterns.
- Scoped frontend/backend/agentctl guidance and `/tdd`, `/mobile-parity`, `/e2e`.

## Results

Implementation and initial verification complete.

- Red: seven real-Git metadata scenarios failed on missing `is_symlink`;
  four frontend assertions failed on dropped metadata, layer projection,
  ignored metadata-only snapshots and stale editor identity.
- Green: planned Go Symlink/Mixed tests and `types/...` suites passed. The
  default shared Go build cache produced missing-standard-library errors;
  an isolated `GOCACHE=/tmp/kandev-symlink-go-cache` resolved the environment issue.
- Planned frontend command (including `git-change-facets.test.ts`): 7 files,
  91 tests passed. Additional affected editor/restoration command below:
  13 files, 68 tests passed.
- `pnpm run typecheck` passed. `pnpm run i18n:check` and
  `pnpm run i18n:ratchet` passed after generating the new Traditional Chinese
  and pseudo-locale editor entry. Unrelated generator rewrites were removed.
- Focused ESLint passed with no errors. The CodeMirror toolbar was split to
  retain its function-size limit; new regression cases were moved out of
  oversized test describe blocks. Final focused lint passed without warnings.
- Additional Git status compatibility cases passed. Reorganized frontend
  regression suites passed again (2 files, 20 tests).
- Fresh backend/Vite/fixture build passed. Sandbox E2E attempts could not bind
  the local HTTP socket (`operation not permitted`); the focused runs passed
  with sandbox escalation. Desktop: 4 tests passed (28.4s), covering both
  providers and flat/tree layouts. Mobile: 1 test passed (6.6s), including
  marker containment and 44px edit target. Its initial selector was corrected
  to allow the Changes navigation button's file-count suffix.
- Final artifact validation, 36 specification linter tests, and diff whitespace
  checks passed.

Additional affected-file verification (from repository root):

```bash
(cd apps/backend && GOCACHE=/tmp/kandev-symlink-go-cache go test ./internal/agentctl/server/process -run 'TestGetGitStatus|TestApplyPorcelain|TestUnquoteGitPath' -count=1)
(cd apps/web && pnpm test components/task/file-browser-actions.test.ts components/task/task-center-panel-restoration.test.ts hooks/use-file-editors.build-state.test.ts hooks/use-file-editors.test.tsx hooks/use-file-editors.open-action.test.tsx components/task/file-tab-content.test.tsx components/task/file-tab-content.external-link.test.tsx components/task/html-preview-content.test.tsx components/task/markdown-preview-content.test.ts components/task/markdown-preview-content.external-link.test.tsx components/editors/monaco/monaco-editor-toolbar.test.tsx components/task/file-editor-panel.image.test.tsx components/task/file-editor-panel.download.test.tsx)
```

Design validation before implementation: catalog passed (267 decisions, 911
specifications), specification linter tests passed (36), full spec lint and
whitespace checks passed. Final artifact validation also passed.

The successful browser invocations reused the freshly built, unchanged product
assets after the sandbox restriction was diagnosed:

```bash
(cd apps/web && GOCACHE=/tmp/kandev-symlink-go-cache pnpm e2e:run --no-build --project chromium tests/git/symlink-file.spec.ts)
(cd apps/web && GOCACHE=/tmp/kandev-symlink-go-cache pnpm e2e:run --no-build --project mobile-chrome tests/git/mobile-symlink-identification.spec.ts)
```


User follow-up: use the existing Files tree icon slot for symlink identity on
desktop and mobile. Reuse `FileTreeNode.is_symlink` and the shared glyph, including
symlink directories; retain chevrons and regular-entry icons. Existing mobile
Files rows are the exemplar; no navigation or touch geometry changes. Extend
the existing desktop/provider/directory and mobile rendered checks.

Files icon follow-up verification: original-build desktop regressions failed
on missing tree link glyphs. Fresh `pnpm build:e2e` passed. The existing global
editor-marker assertions then matched the new tree markers too; scoped them
outside Files. Final desktop suite: 4 passed (26.2s); mobile suite: 1 passed
(6.4s). Focused ESLint, typecheck, spec validation/lint, and whitespace checks
passed. Reloaded the isolated Tailscale instance and verified its real Files
icons in a headless browser. No main-instance changes were made.

## Post-fixup verification

Addressed review findings for same-path staged deletion plus untracked
replacement and already-open editor tabs whose symlink type changes. The
backend now preserves both change layers and the tab upsert refreshes
`resolvedPath` without overwriting dirty content.

```bash
(cd apps/backend && GOCACHE=/tmp/kandev-symlink-go-cache go test ./internal/agentctl/server/process -count=1)
(cd apps/web && pnpm test -- --run components/task/task-center-panel-file-tabs.test.ts hooks/use-file-editors.build-state.test.ts hooks/file-editors-sync.test.ts hooks/use-task-center-file-open.test.ts)
(cd apps/web && pnpm exec eslint components/task/task-center-panel-file-tabs.ts components/task/task-center-panel-file-tabs.test.ts hooks/use-task-center-file-open.ts)
```

The focused frontend run passed 34 tests, the full backend process package
passed, and the new regression cases passed before the fixup commit.
