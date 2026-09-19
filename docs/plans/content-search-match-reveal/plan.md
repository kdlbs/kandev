---
spec: docs/specs/ui/requirements/task-workspace-content-search.md
created: 2026-08-14
status: done
---

# Implementation Plan: Content Search Match Reveal

## Overview

Make content-search selection a model-aware editor reveal instead of a
mount-only cursor hint. Preserve the existing path, repository, and session
scope; activate the preview first; consume the reveal only after the matching
editor model is ready; then center and briefly highlight the selected line in
Monaco and CodeMirror. Finish with the cached-preview keyboard regression that
exposed the bug.

---

## Root Cause (Confirmed)

`openContentSearchResult` currently writes `{ line, column }`, calls
`scrollEditorIfMounted`, and only then calls `addFileEditorPanel`. When a target
file is cached but the reusable preview currently displays another file,
`scrollEditorIfMounted` cannot find the target model. Dockview then swaps the
existing Monaco editor to the cached model without remounting it.
`useMonacoEditorComments.handleEditorDidMount` is the only delayed Monaco
consumer, so it never sees that pending cursor request after the model swap.

A throwaway production-build Playwright regression reproduced the exact path:
open a 180-line target through Files search, switch the preview to another
file, then press Enter on the target's Contents result. The current build
returned `{ position: 1, visible: false }` instead of
`{ position: 180, visible: true }`. The temporary spec was removed after the
failed run.

The highlight half is absent rather than mistimed. The pending payload contains
only line and column; Monaco and CodeMirror set the cursor, center it, and focus
the editor without creating any transient decoration. Existing E2E coverage
opens a fresh file at line 9 and asserts only `getPosition()`, so it covers
neither an off-screen reveal nor cached preview reuse nor a visible highlight.

---

## Frontend

### Scoped Reveal Request Broker

Update `apps/web/hooks/file-editor-cursor.ts` so the pending value can carry an
optional one-shot line-flash intent while preserving every existing caller and
the current path/repository/session key rules.

- Keep `setPendingCursorPosition` backward compatible and add an optional
  reveal-options argument rather than making file links, walkthroughs, or LSP
  navigation flash unexpectedly.
- Make immediate mounted-editor reveal and delayed model/mount consumption use
  the same target shape and cleanup semantics.
- Scope active flash state and cleanup to the editor instance and latest
  request. Re-selecting a match restarts the flash; an older timer cannot clear
  a newer decoration.
- Keep missing, ambiguous, wrong-repository, and wrong-session editors as
  no-ops that leave the pending request available for the correct consumer.

### Content Search Selection Order

Update `apps/web/lib/commands/content-search-selection.ts` to attach the
one-shot flash intent, activate or replace the file preview, and then try the
immediate mounted-editor reveal. This order handles an already-active target
immediately while leaving cached model swaps and delayed mounts for the editor
lifecycle consumer.

### Monaco Model Lifecycle

Extract a small model-aware cursor-navigation hook beside the Monaco editor,
following the existing `useMonacoModelSnapshot` subscription pattern in
`apps/web/components/editors/monaco/use-monaco-walkthrough-range.ts`.

- Move pending-cursor consumption out of the mount-only callback in
  `use-monaco-editor-state.ts`.
- Subscribe to `onDidChangeModel` and retry when the editor instance, target
  file identity, or active model changes. Consume only after the target model
  is active.
- Apply a whole-line Monaco decoration, center the line, set the returned
  cursor column, and focus the editor. Remove the decoration after the bounded
  flash interval and on teardown/model replacement.

Likely new files:

- `apps/web/components/editors/monaco/use-monaco-cursor-navigation.ts`
- `apps/web/components/editors/monaco/use-monaco-cursor-navigation.test.tsx`

### CodeMirror And Mobile Viewer

Extend `apps/web/components/editors/codemirror/codemirror-cursor-navigation.ts`
with a small `StateEffect`/`StateField` decoration extension so desktop
CodeMirror and the read-only mobile `FileViewerContent` can render and clear
the same optional whole-line flash. Reuse the existing line/column clamping,
centered `scrollIntoView`, and focus behavior. Install the extension in both
CodeMirror surfaces and have `FileViewerContent` reuse the shared reveal helper
instead of maintaining a second position-only implementation.

### Visual Treatment

Add one scoped editor-result class and one one-shot keyframe in
`apps/web/app/globals.css`.

- Use the existing warning/search hue at restrained opacity so the line reads
  as a destination, not an error or persistent selection.
- Fade only the background over about 1.2 seconds. Do not animate layout,
  scrolling, transform, or unrelated properties, and do not use
  `transition: all`.
- Under `prefers-reduced-motion: reduce`, suppress the fade but retain the
  static highlight for the same bounded interval before cleanup.
- The class is shared by Monaco and CodeMirror and coexists with comment and
  walkthrough decorations.

### Mobile Design Contract

- **Desktop outcome and entry:** Cmd/Ctrl+Shift+F, query, then Enter opens the
  selected Dockview file preview with its result line centered and flashed.
- **Nearest mobile exemplar:**
  `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx` and its
  `FileViewerContent` child already provide a full-height, single-scroll-owner
  CodeMirror viewer and consume pending cursor positions.
- **Presentation:** inline whole-line decoration inside the existing editor.
  No drawer, route, toolbar, touch target, safe-area, or scroll-owner change.
- **Shared behavior:** path/repository/session scoping, clamping, reveal intent,
  and highlight cleanup are shared; editor-provider adapters own only their
  decoration APIs.
- **Mobile E2E boundary:** the current command-palette content selection calls
  Dockview's `addFileEditorPanel`, while `SessionMobileLayout` owns a separate
  `selectedFile` flow. There is no supported mobile command-palette destination
  to exercise without adding a new navigation contract. That capability is out
  of scope; focused `FileViewerContent` coverage proves the shared mobile
  viewer consumes and clears a flashed reveal request.

No new user-facing copy, API, persistence, backend, or public documentation is
needed.

---

## Tests

- **Content selection contract:** update
  `apps/web/lib/commands/content-search-selection.test.ts` to prove the reveal
  request carries flash intent and the panel is activated before the immediate
  mounted-editor attempt.
- **Broker scoping and cleanup:** update
  `apps/web/hooks/use-file-editors.test.tsx` for direct Monaco reveal, latest
  request/timer ownership, and unchanged repository/session ambiguity behavior.
- **Monaco cached model swap:** add
  `apps/web/components/editors/monaco/use-monaco-cursor-navigation.test.tsx`.
  Mount once on file A, queue a flashed target for cached file B, fire the model
  change, and assert position, centered reveal, decoration, restart, and cleanup
  with fake timers. This test must fail before production code changes.
- **CodeMirror behavior:** extend
  `apps/web/components/editors/codemirror/codemirror-cursor-navigation.test.ts`
  and `codemirror-code-editor.cursor.test.tsx` for the optional whole-line
  decoration, repeated flash, cleanup, and unchanged clamping/scope behavior.
- **Read-only mobile consumer:** add
  `apps/web/components/task/file-viewer-content.test.tsx` for delayed/path-change
  consumption through the shared CodeMirror reveal helper.

Targeted unit command:

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/commands/content-search-selection.test.ts hooks/use-file-editors.test.tsx components/editors/monaco/use-monaco-cursor-navigation.test.tsx components/editors/codemirror/codemirror-cursor-navigation.test.ts components/editors/codemirror/codemirror-code-editor.cursor.test.tsx components/task/file-viewer-content.test.tsx
```

Then run direct web typechecking and focused lint:

```bash
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint lib/commands/content-search-selection.ts lib/commands/content-search-selection.test.ts hooks/file-editor-cursor.ts hooks/use-file-editors.test.tsx components/editors/monaco/use-monaco-cursor-navigation.ts components/editors/monaco/use-monaco-cursor-navigation.test.tsx components/editors/monaco/use-monaco-editor-state.ts components/editors/monaco/monaco-code-editor.tsx components/editors/codemirror/codemirror-cursor-navigation.ts components/editors/codemirror/codemirror-cursor-navigation.test.ts components/editors/codemirror/codemirror-code-editor.tsx components/editors/codemirror/codemirror-code-editor.cursor.test.tsx components/task/file-viewer-content.tsx components/task/file-viewer-content.test.tsx e2e/tests/search/task-workspace-content-search.spec.ts
```

---

## E2E Tests

- **Scenario:** GIVEN a deep target file has already been opened and the shared
  preview now displays another cached file, WHEN the user opens Contents search
  and presses Enter on the target result, THEN the target Monaco model is
  active, its returned cursor line is inside `getVisibleRanges()`, the scoped
  line-flash decoration appears, and that decoration clears without a raw
  sleep.
- **File:**
  `apps/web/e2e/tests/search/task-workspace-content-search.spec.ts`.
- **Method:** seed the target beyond the first viewport (about line 180), warm
  its model through Files search, switch the reusable preview, and select the
  Contents result with the keyboard. Preserve the existing multi-repository
  assertions.

Production-build command:

```bash
cd apps/web && pnpm e2e:run tests/search/task-workspace-content-search.spec.ts
```

The managed runner rebuilds the production frontend and backend. Confirm it
discovers the intended desktop scenarios before treating the run as evidence.

---

## Verification Results

Completed on 2026-08-14.

- RED: focused selection/Monaco tests failed 3 assertions while 15 tests
  passed; the cached-preview production regression returned line 1 with no
  flash; CodeMirror/mobile tests then failed the two missing shared-highlight
  behaviors; the mobile reuse cleanup test failed before its fix.
- GREEN: the exact targeted unit set passed 6 files and 38 tests. Direct web
  typecheck and the exact focused ESLint command passed.
- Rendered production evidence: the fresh-build cached-preview scenario passed,
  then the full content-search spec passed all 3 desktop scenarios against the
  same production artifacts. It proved Enter activates the cached model, line
  180 is within Monaco's visible ranges, the whole-line decoration appears,
  and polling observes its bounded cleanup.
- Mobile boundary: shared `FileViewerContent` and real CodeMirror decoration
  tests prove install, path-change cleanup, repeated flash ownership, and
  bounded removal. No mobile E2E was added because the command palette still
  has no supported mobile file destination.
- The managed runner tore down cleanly, build/test artifacts stayed ignored,
  `git diff --check` passed, and no external side effect or subagent was used.

Seed-environment follow-up on 2026-08-15:

- A fresh-editor smoke test with the match on line 180 exposed a second path:
  Monaco accepted the cursor position before Dockview had measured the active
  container, leaving the viewport at lines 1-49. The permanent
  multi-repository E2E was deepened to assert line visibility and the flash;
  it failed before the follow-up fix.
- The Monaco reveal now forces a layout measurement before centering. The
  focused fresh-editor E2E passed, the full content-search spec passed all 3
  scenarios, the targeted unit set passed 6 files and 38 tests, and typecheck,
  focused ESLint, and `git diff --check` passed.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Reveal and flash selected content match](task-01-reveal-selected-content-match.md)

Single sequential task. Editor lifecycle, decoration cleanup, and its E2E
regression form one coupled behavior. No subagents are authorized.

---

## Risks And Boundaries

- Monaco keeps cached models and can change the active model without remounting;
  correctness must follow model identity, not React mount timing.
- Repeated selections and model switches can leave stale timers or decorations;
  latest-request ownership and teardown are acceptance requirements.
- Existing LSP, walkthrough, chat-file-link, and plain file-open callers must
  keep position-only behavior and must not start flashing.
- CodeMirror decorations must map through document changes while active and
  clear without mutating file content or dirty state.
- Mobile command-palette-to-file navigation is a separate missing integration,
  not part of this desktop Dockview repair.

## Open Questions

None.
