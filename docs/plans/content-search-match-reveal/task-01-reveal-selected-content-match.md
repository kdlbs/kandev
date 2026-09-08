---
id: "01-reveal-selected-content-match"
title: "Reveal and flash selected content match"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-workspace-content-search.md"
---

# Task 01: Reveal And Flash Selected Content Match

## Intent

Make Enter on a workspace content-search result activate the correct cached or
new editor model, center the returned line, and show a bounded one-shot
whole-line highlight in Monaco and CodeMirror.

## Acceptance

1. A content result opened through a reused cached Monaco preview lands at the
   returned one-based line and column with that line inside the visible range;
   first mounts and already-active editors retain the same behavior.
2. Content-search reveals flash the destination line for about 1.2 seconds,
   repeated selection restarts it, reduced motion removes only the fade, and
   stale timers/model switches cannot clear a newer decoration.
3. Repository/session scoping, CodeMirror and read-only mobile consumption,
   ordinary position-only callers, file contents, dirty state, and existing
   multi-repository search behavior remain unchanged.

## TDD Sequence

1. Add the cached-model Monaco unit regression and the deep cached-preview
   Playwright scenario. Run both against current code and record the expected
   RED evidence.
2. Add highlight lifecycle assertions for Monaco, CodeMirror, and
   `FileViewerContent`; confirm they fail because the reveal payload and
   decorations do not yet exist.
3. Implement the smallest model-aware reveal request, editor adapters, selection
   ordering, and scoped CSS treatment needed to make those tests GREEN.
4. Refactor only duplicated reveal/highlight mechanics, rerun exact checks, and
   record results in this file and `plan.md`.

## Verification

Fresh-worktree bootstrap is already complete for the current workspace. In any
new worktree, run this first:

```bash
cd apps && pnpm install --frozen-lockfile
```

Targeted unit tests:

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/commands/content-search-selection.test.ts hooks/use-file-editors.test.tsx components/editors/monaco/use-monaco-cursor-navigation.test.tsx components/editors/codemirror/codemirror-cursor-navigation.test.ts components/editors/codemirror/codemirror-code-editor.cursor.test.tsx components/task/file-viewer-content.test.tsx
```

Typecheck and focused lint:

```bash
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint lib/commands/content-search-selection.ts lib/commands/content-search-selection.test.ts hooks/file-editor-cursor.ts hooks/use-file-editors.test.tsx components/editors/monaco/use-monaco-cursor-navigation.ts components/editors/monaco/use-monaco-cursor-navigation.test.tsx components/editors/monaco/use-monaco-editor-state.ts components/editors/monaco/monaco-code-editor.tsx components/editors/codemirror/codemirror-cursor-navigation.ts components/editors/codemirror/codemirror-cursor-navigation.test.ts components/editors/codemirror/codemirror-code-editor.tsx components/editors/codemirror/codemirror-code-editor.cursor.test.tsx components/task/file-viewer-content.tsx components/task/file-viewer-content.test.tsx e2e/tests/search/task-workspace-content-search.spec.ts
```

Production-build browser regression:

```bash
cd apps/web && pnpm e2e:run tests/search/task-workspace-content-search.spec.ts
```

Confirm Playwright discovers the existing multi-repository scenario plus the new
cached-preview scenario. Do not use a raw timeout to wait for highlight cleanup.

## Files Likely Touched

- `apps/web/lib/commands/content-search-selection.ts`
- `apps/web/lib/commands/content-search-selection.test.ts`
- `apps/web/hooks/file-editor-cursor.ts`
- `apps/web/hooks/use-file-editors.test.tsx`
- `apps/web/components/editors/monaco/use-monaco-cursor-navigation.ts` (new)
- `apps/web/components/editors/monaco/use-monaco-cursor-navigation.test.tsx` (new)
- `apps/web/components/editors/monaco/use-monaco-editor-state.ts`
- `apps/web/components/editors/monaco/monaco-code-editor.tsx`
- `apps/web/components/editors/codemirror/codemirror-cursor-navigation.ts`
- `apps/web/components/editors/codemirror/codemirror-cursor-navigation.test.ts`
- `apps/web/components/editors/codemirror/codemirror-code-editor.tsx`
- `apps/web/components/editors/codemirror/codemirror-code-editor.cursor.test.tsx`
- `apps/web/components/task/file-viewer-content.tsx`
- `apps/web/components/task/file-viewer-content.test.tsx` (new)
- `apps/web/app/globals.css`
- `apps/web/e2e/tests/search/task-workspace-content-search.spec.ts`

Reconcile this list with the actual diff before marking the task done.

## Dependencies

None.

## Parallelism

`sequential`. The broker, provider adapters, CSS lifecycle, and browser
regression share one behavior and cannot be reviewed safely as independent
changes.

## Inputs

- `docs/specs/ui/requirements/task-workspace-content-search.md`, especially result selection
  and cached-preview scenarios.
- `plan.md`, especially Root Cause, Frontend, Tests, and Risks.
- `apps/web/components/editors/monaco/use-monaco-walkthrough-range.ts` for the
  existing model-snapshot subscription pattern.
- `apps/web/components/task/file-viewer-content.tsx` for mobile single-scroll
  ownership and path-change consumption.

## Output Contract

Report RED/GREEN commands and counts, changed files, model/timer cleanup
behavior, desktop rendered evidence, generated artifacts, teardown, remaining
risks, and synchronized task/plan status. External side effects: none.

## Results

Completed on 2026-08-14.

### RED evidence

- The initial focused unit run reported 3 expected failures and 15 passing
  tests. Content selection still sent a position-only request before panel
  activation, and Monaco created no whole-line decoration.
- The permanent cached-preview browser regression ran against the old
  production build and received `{ line: 1, visible: true, flash: false }`
  instead of `{ line: 180, visible: true, flash: true }`.
- The first CodeMirror/mobile unit run reported 2 expected failures and 3
  passing tests because no flash effect existed and `FileViewerContent` still
  had a second position-only implementation.
- The mobile reuse cleanup regression failed once as expected because a flash
  from the prior file was not cleared on an in-place path change.

### GREEN evidence

- Targeted unit command: 6 files passed, 38 tests passed.
- `cd apps/web && pnpm run typecheck`: passed.
- The exact focused ESLint command from this task: passed with zero findings.
- Fresh production build plus the new cached-preview Playwright scenario: 1
  test passed.
- Final full production-artifact run:
  `pnpm e2e:run --no-build tests/search/task-workspace-content-search.spec.ts`:
  3 tests passed (route boundary, multi-repository selection, cached-preview
  reveal/flash).
- `git diff --check`: passed.

The managed runner cleaned its result directories and exited with no Kandev or
Playwright process left running. Build output remained ignored; no generated
artifact was added to the change. Mobile verification is the shared
`FileViewerContent` component regression plus real CodeMirror decoration tests;
there is still no supported mobile command-palette-to-file destination, as
recorded in the plan. No subagents or external side effects were used.

### Seed-environment follow-up (2026-08-15)

- A fresh-editor manual smoke test put the cursor on line 180 but left Monaco's
  visible range at lines 1-49. The deepened multi-repository E2E reproduced the
  missing reveal before the fix.
- `revealMonacoEditor` now lays out the fresh Dockview-mounted editor before it
  sets and centers the target position.
- Focused fresh-editor E2E: 1 test passed. Full content-search E2E: 3 tests
  passed. Targeted units: 6 files and 38 tests passed. Typecheck, focused
  ESLint, and `git diff --check` also passed.
