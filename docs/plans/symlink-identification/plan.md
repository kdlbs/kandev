---
created: 2026-09-14
status: implemented
requirements:
  - REQ-WORKSPACES-SYMLINK-001
system_design:
  - ../../specs/workspaces/system-design/symlink-identification.md
legacy_specs: []
---

# Implementation Plan: Symlink identification

## Overview

Deliver one vertical slice: authoritative change-layer metadata, live projection,
then consistent Changes and editor indicators. One sequential work order owns
integration and verification. Implementation and targeted verification are complete.

## Scope

In scope: workspace staged/unstaged/untracked rows, list/tree layouts, opened
files in both editor providers and existing preview/mobile surfaces, localization,
and stale metadata removal. Out of scope: target navigation, file-tree redesign,
remote PR/historical classification without mode metadata, and changed mutations.

## Technical approach

Follow the [design](../../specs/workspaces/system-design/symlink-identification.md).
Extend `FileInfo`/`FileChangeFacet` through the existing transport and store;
classify each layer independently without reading targets or depending on patch
bodies. Thread existing resolved-path editor state through shared indicators.
Reuse mobile Changes/viewer composition. No new dependency or storage schema.

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

## Tests

- AC 001.1 and 001.3: add `TestSymlinkStatusMetadata` in
  `workspace_git_symlink_test.go`: staged, unstaged, untracked, deletion, rename,
  type replacement, mixed layer, broken/external target, skipped diff and unknown
  metadata. Use real temporary Git repositories. Verify JSON serialization.
- AC 001.1 and 001.4: extend `changes-panel-helpers.test.ts`,
  `changes-panel-file-row.test.tsx` and `set-git-status-return.test.ts` for facet
  mapping, regular/unknown entries, marker persistence and metadata-only refresh.
- AC 001.2 and 001.4: extend `use-file-editors.build-state.test.ts`,
  `file-editors-sync.test.ts`, and `file-editor-content.test.tsx` for both
  providers, symlink-to-regular reload with unchanged contents, same paths in two
  repositories, and preservation of dirty buffers/request identity.

All AC suffixes refer to AC-WORKSPACES-SYMLINK-001.

## E2E tests

Extend `e2e/tests/git/symlink-file.spec.ts` (chromium) with Changes list/tree
markers and editor identification for both providers. Existing target-edit/save
coverage must still pass. Add `e2e/tests/git/mobile-symlink-identification.spec.ts`
(mobile-chrome) using real symlink fixtures and existing mobile navigation:
identify the row, tap edit, assert visible editor label, open the regular target,
and assert the label disappears. Check long paths, document overflow, and retained
action hit areas. Together these cover AC 001.1 through .4.
Managed runners rebuild product assets; run desktop and mobile sequentially.

## Work orders

- [x] [Task 01: Identify workspace symlinks](task-01-identify-symlinks.md)

## Verification

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

## Verification results

Unit, integration, typecheck, localization and lint checks passed. Desktop E2E
passed (4 tests); mobile E2E passed (1 test). Exact commands and results are recorded in
[Task 01](task-01-identify-symlinks.md#results).

## Risks

Mode changes and deleted paths cannot be classified from today's filesystem
alone. Diff budgets must not erase identity. Store equality and editor refresh
can otherwise retain stale labels. Platforms unable to create native symlinks
need explicit test skips with a reason; Unix CI must exercise real fixtures.

## Public documentation

During implementation add a concise explanation of the marker and editor label
to `docs/public/sessions-and-review.md` (how-to). Do not advertise draft behavior
in public docs during this design turn.

User-requested follow-up: Files tree entries use the shared link glyph in the
existing file/folder icon slot when `FileTreeNode.is_symlink` is true. Desktop
and phone Files surfaces share this renderer. Directory chevrons, touch actions,
and ordinary icons are preserved. Search results without metadata are unchanged.

```text
Files (desktop and phone)
  [link]   new-link.md
  [file]   ordinary-file.md
  > [link] linked-directory
```
