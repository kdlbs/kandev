---
id: "02-changes-section"
title: "Show and open workspace root files in Changes"
status: done
wave: 2
depends_on:
  - "01-root-file-discovery"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKSPACE-ROOT-FILES-001
  - REQ-TASKS-WORKSPACE-ROOT-FILES-002
acceptance_criteria:
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.1
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.2
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.4
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.5
  - AC-TASKS-WORKSPACE-ROOT-FILES-002.1
system_design:
  - ../../specs/tasks/system-design/workspace-root-files.md
---

# Task 02: Show and open workspace root files in Changes

## Summary

Render an **Outside repositories** section in the shared `ChangesPanelBody`
for desktop and phone. Feed it from a store entry that a domain hook keeps
current. Row activation opens the file through the existing root-scoped editor
path.

## In scope

- `lib/ws/workspace-files.ts`: `listWorkspaceRootFiles(client, sessionId)` and
  response types.
- Session-runtime store entry `workspaceRootFiles.byEnvironmentId[envId]`
  (`{ supported, files, truncated, destinations }`) with a setter.
- `hooks/domains/session/use-workspace-root-files.ts`: fetch on mount,
  environment change, and WS reconnect. Debounce a refetch (300 ms) on
  `session.workspace.file.changes` for the session when any change has an
  empty `repository_name`. Apply only the latest request generation for each
  environment.
- `components/task/changes-panel-root-files.tsx`: the section header with a
  count, rows (a fine-pointer row menu and a coarse-pointer touch row), and the
  truncation note. Row activation calls `onEditFile(path)` with no repository.
  The **Move to repository…** menu item is added in Task 03.
- `changes-panel-body.tsx`: render the section after the timeline. When
  `EmptyChangesPanel` would render and root files exist, show the section
  instead.
- English keys in `src/locales/en/task.json` and all six real locales, plus
  pseudo-locale regeneration.

## Out of scope

The move dialog (Task 03), diffs, and the Files panel.

## ASCII UI preview

See [UI-01 and UI-03 in the plan](plan.md#ascii-ui-preview). This task renders
the section, the rows, **Open**, and the truncation note. The **Move to
repository…** item appears in Task 03.

## Acceptance

1. Hook tests cover the initial fetch, a refetch after an empty-repository
   notification (debounced), no refetch for a repository-scoped notification,
   and rejection of a stale response.
2. Section tests: it is hidden for `supported=false` or an empty list. It
   replaces the empty state when clean. It shows the truncation note. A row
   click calls `onEditFile("dashboard.json", undefined)`.
3. E2E on desktop and `mobile-chrome`: a two-repository task with clean
   checkouts plus a root `dashboard.json` shows the section. Opening the file
   shows its content, and both checkouts stay clean.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-workspace-root-files.test.ts components/task/changes-panel-root-files.test.tsx components/task/changes-panel-body-context.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/git/workspace-root-files.spec.ts -- --grep "shows")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-workspace-root-files.spec.ts -- --grep "shows")
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/lib/ws/workspace-files.ts`
- `apps/web/lib/state/slices/session-runtime/` (types, slice, and a test)
- New `apps/web/hooks/domains/session/use-workspace-root-files.ts` and test
- New `apps/web/components/task/changes-panel-root-files.tsx` and test
- `apps/web/components/task/changes-panel-body.tsx`, `changes-panel-body-props.ts`
- `apps/web/src/locales/*/task.json`, `apps/web/eslint.i18n.options.mjs` (append the new paths)
- New `apps/web/e2e/tests/git/workspace-root-files.spec.ts`
- `apps/web/lib/browser-demo/workspace-runtime.ts`, if the demo socket must answer the new action

## Dependencies

Task 01. Load `/mobile-parity` and `/e2e` before you start.

## Risks

- `ChangesPanelBody` owns the only scroller. Keep the section inside it rather
  than adding a second scroll region.
- CSS-gated desktop and phone trees can both mount. The store entry keyed by
  environment keeps them on one list, and the hook must deduplicate in-flight
  requests per environment.

## Parallelism

`sequential`

## Results

- Added the environment-scoped listing API and store entry, with fetches on mount, environment change, and WebSocket reconnection. Root-scoped notifications are debounced for 300 ms, in-flight reads are deduplicated by environment, and stale responses are ignored.
- Added the shared Changes section, truncation note, accessible row menu, and root-scoped open action. The section replaces the clean empty state when root files exist.
- Added translations for all supported locales and regenerated the pseudo locale. The browser demo returns an unsupported listing.
- Focused Vitest: passed (45 tests across six files); web typecheck, lint, i18n check, and i18n ratchet passed.
- Desktop and phone managed E2E passed. Each opened the root file and verified both repository checkouts remained clean. The mobile case uses its own `mobile-*` filename so Playwright discovers it in the `mobile-chrome` project.
- Hook and section regressions passed, including inventory changes triggering a refresh and pending requests being invalidated after the last observer unmounts. Focused Vitest passed (26 tests across four files); typecheck, lint, i18n check, and the Vite E2E build passed.
- Desktop and phone workspace-root-files E2E passed after the review updates.
- Passed the specification validation, spec lint, and diff whitespace checks.
