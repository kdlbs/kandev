---
created: 2026-09-12
status: done
requirements:
  - REQ-UI-PROMPT-ALIAS-002
system_design:
  - ../../specs/ui/system-design/prompt-alias-rendering.md
legacy_specs: []
---

# Implementation plan: Task-create prompt chips

## Overview

Show saved prompt references as inline chips in the new-task editor, including
the canvas preset. First build the text-preserving editor. Then integrate it
with task creation and prove the desktop and phone workflows.

UI owns the reusable presentation contract. Tasks keeps saved-prompt delivery,
and Canvases keeps its preset and launcher. The design package is implemented
and its verification is recorded below.

## Scope

### In scope

- Recognized reference chips, preview, removal, and reference autocomplete in create mode.
- Plain-text serialization, draft restore, selection, undo, paste, and IME behavior.
- Existing attachment, enhancement, voice, plugin, and launch-preview integration.
- Desktop and phone rendered checks, including long drafts and prompt lookup failure.

### Out of scope

- Backend expansion, prompt storage, new dependencies, and canvas permissions.
- New Agent, task editing, passthrough, or general chat editor redesign.
- A rich Markdown authoring toolbar or separate prompt attachment collection.

## Technical approach

Use the [Task creation editor design](../../specs/ui/system-design/prompt-alias-rendering.md#task-creation-editor).
Add `TaskPromptReferenceEditor` and plain-text conversion/position helpers.
Use the installed Tiptap primitives without chat-specific state or capabilities.
Reuse `PromptMentionChip`, `splitMarkdownPromptMentionSegments`, and prompt store subscriptions.

Thread an explicit create-mode choice from `TaskCreateDialog` through
`DialogPromptSection` to `TaskFormInputs`. Keep `TaskFormInputsHandle` unchanged.
Adapt `useDescriptionInput` and plugin insertion to an internal editor handle.
The old textarea remains the default for other consumers.

Reference selection is opt-in for creation. Preserve `useInlineMention` inline
and context modes. Reuse `MentionMenu` positioning and query behavior.
Do not pass ProseMirror positions to plain-string splicing operations.
Retain `task-description-input` on the editable element, and update textarea-only
test helpers for create-mode contenteditable semantics without changing session assertions.

## ASCII UI preview

### UI-01: Desktop create dialog, recognized preset

Current input: plain goal followed by literal `@create-canvas`.
Proposed affected region:

```text
+-------------------------------------------------------+
| Create a coordinator view of the existing tasks.        |
|                                                       |
| [@create-canvas] [x]                                   |
|                                                       |
| [Attach] [Enhance]                                     |
+-------------------------------------------------------+
| Agent / Model               Executor                  |
|                                  [Cancel] [Start task]|
+-------------------------------------------------------+
        chip click / keyboard activation
        -> current prompt preview
```

The chip uses the teal treatment from the transcript, not a new color system.
`[x]` is a separately named removal action for this occurrence.
The surrounding text stays editable. Chip activation never launches the task.
Maps to criteria `.1` through `.4` and `.7` under `AC-UI-PROMPT-ALIAS-002`.

### UI-02: Phone create dialog and prompt preview

```text
+-----------------------------+  +-----------------------------+
| New task                    |  | Dimmed create dialog        |
|-----------------------------|  |                             |
| Create a coordinator view   |  +-----------------------------+
| of the existing tasks.      |  | @create-canvas              |
|                             |  |-----------------------------|
| [@create-canvas] [x]         |  | Current saved instructions  |
| [Attach] [Enhance]           |  | ... scroll inside drawer ...|
| Agent / Model               |  |                             |
| Executor                    |  +-----------------------------+
| ... scrolling form body ... |      Dismiss -> same draft
|-----------------------------|
| [Cancel]       [Start task]  |
+-----------------------------+
```

The full-height form retains its footer and safe-area clearance.
Chip preview opens the existing touch drawer pattern, not a desktop hover card.
Chip/removal hit areas are at least 44px on touch devices. Names wrap or truncate
inside their available width, with the complete name accessible.
Long drafts scroll inside the bounded editor. Long previews scroll inside the drawer.
Maps to `.3`, `.4`, `.6`, and `.7`.

### UI-03: Unknown/loading and creation failure

```text
Prompt unavailable:  Goal text ... @unknown-name
                    (ordinary editable text, submission available)

Creation failure:   Goal text ... [@create-canvas] [x]
                    Existing error surface
                    [Cancel] [Start task]
```

Loading never erases the reference. Retry retains the draft and attachments.
Maps to `.5` and `.7`. All drawings specify structure, not pixel spacing or final copy.
New labels use localized strings. The same views apply to general task creation
and the canvas preset without a second editor implementation.

## Tests

All abbreviated criteria refer to `AC-UI-PROMPT-ALIAS-002`.

| Criteria | Planned evidence |
| --- | --- |
| `.1`, `.3`, `.5` | New `lib/prompts/task-prompt-document.test.ts`: exact text round trips, Unicode offsets, repeated names, code/link exclusions, store reconciliation. |
| `.1` to `.5` | New `components/task-prompt-reference-editor.test.tsx`: initial/typed/pasted aliases, selection, IME, removal, undo/redo, plain-text clipboard, previews, late/deleted prompts. |
| `.2`, `.7` | `hooks/use-inline-mention.test.ts`: reference opt-in and unchanged inline/context behavior. |
| `.3`, `.7` | `components/task-create-dialog-selectors.test.tsx`: synchronous handle updates, two plugin insertions then submit, enhancement, attachment lifecycle, draft/preview switching. |
| `.7` | `components/task/new-session-form-prompt.test.tsx`: unchanged full-content insertion outside create mode. |

## E2E tests

- `tests/task/task-create-prompt-autocomplete.spec.ts` and its `-qa` companion:
  revise creation expectations from definitions to aliases. Cover Enter/Tab and
  pointer selection, unknown names, preview, removal, undo, and captured task payload.
- New `tests/task/mobile-task-create-prompt-chips.spec.ts`: select by touch,
  preview/dismiss, remove one repeated occurrence, restore, and submit.
  Use the configured Pixel 5 project and `composer-visual-viewport` helpers.
  Assert actual 44px preview/removal targets, drawer containment/internal scroll,
  wrapped long names, long draft scrolling, footer reachability, and no document overflow.
  Include reduced viewport height and 767/768px responsive checks.
- Existing `tests/canvas/plugin-canvas.spec.ts` and `mobile-plugin-canvas.spec.ts`:
  retain preset, cancellation/reopen, controlled creation failure/retry, and payload checks.
  Update only editor-specific selectors/assertions. Capture desktop/phone screenshots for UI-01/02.
- Run New Agent desktop/phone specs to prove create-only gating.

Browser tests prove reference serialization and editing, not server expansion.
Existing launch tests remain the authority for expansion behavior.

## Work orders

- [x] [Task 01: Build the reference editor](task-01-reference-editor.md) (done)
- [x] [Task 02: Integrate task creation](task-02-create-integration.md) (done)

Dependency order: 01, then 02. No delegation is required or authorized.

## Companion plans

The [alias rendering package](../prompt-alias-rendering/plan.md) covers read-only
surfaces. Its pending statuses remain unchanged despite existing shared code.
This package reuses that code, not its unrecorded verification results.

The completed [canvas creation package](../canvas-direct-creation/plan.md) remains
the historical record for the plain-text editor. This proposal replaces only its
editor presentation and autocomplete exclusion after implementation.
The [New Agent package](../agent-launch-prompt-composer/plan.md) remains unchanged
because reference mode is explicitly create-only.

## Verification results

Implementation checks on 2026-09-12:

- Task 01: the focused document, editor, mention, and shared chip unit command passed with 4 files and 37 tests; targeted ESLint, typecheck, specification lint, and diff checks passed.
- Task 02: the integration unit command passed with 5 files and 57 tests; full web lint passed with zero warnings; typecheck, i18n check and ratchet, backend build and plugin packaging, E2E build, desktop and phone work-order suites, migrated composer regressions, E2E sleep ratchet, public docs tests and validation, specification tests and lint, and diff checks all passed.
- Browser totals: 26 desktop work-order tests, 7 phone work-order tests, 12 desktop migrated regression tests, and 5 phone migrated regression tests passed.

Review remediation verification on 2026-09-12:

- Reconciled aliases from the full serialized draft in one transaction, restored plain-text anchor and head offsets, and kept reconciliation outside the undo history.
- Inserted plugin text as a plain-text ProseMirror fragment so HTML-like strings, entities, whitespace, and line breaks remain literal across synchronous calls.
- Added explicit textbox semantics to the create-mode contenteditable so the existing dialog locator works in browser accessibility trees.
- `cd apps/web && pnpm exec vitest run components/task-prompt-reference-editor.test.tsx components/task-create-dialog-selectors.test.tsx`: 2 files and 33 tests passed.
- `cd apps/web && pnpm exec vitest run components/task-create-dialog-selectors.test.tsx components/task-create-dialog-form-body.test.tsx components/task-create-dialog.test.tsx components/task/new-session-form-prompt.test.tsx components/canvas/canvas-task-create-launcher.test.tsx`: 5 files and 58 tests passed.
- `cd apps/web && pnpm e2e:run --host --no-build --project=chromium e2e/tests/chat/clarification.spec.ts --grep "pointer-events stuck on body" --retries=0`: 1 test passed.
- The planned desktop and phone work-order suites passed again with retries disabled: 26 Chromium tests and 7 mobile tests, followed by 12 Chromium migrated composer tests and 5 mobile migrated composer tests.
- Typecheck, full lint, Prettier, i18n check and ratchet, E2E sleep ratchet, public-doc tests and validation, specification tests and lint, and diff checks passed.

Design validation on 2026-09-12:

- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Both pending work orders exist and reference requirement 002 and its design.
- Rendered tests were not run during the design-only validation; implementation rendered results are recorded above.

## Risks

- Text offsets differ from document positions, especially around Unicode and atoms.
- Whole-document replacement can erase undo history or move the caret.
- Shared form callers can inherit a behavior change unless create mode is explicit.
- Native textarea-only assertions exist beyond the named E2E specs. Inventory them
  before integration and run every changed test through the affected work order.
- A chip previews current stored content, not a promise of later agent behavior.

## Documentation impact

The saved-prompts how-to in `docs/public/developer-tools.md` and the creation
description in `docs/public/canvases.md` now document editable references.
