---
created: 2026-09-14
status: implemented
requirements:
  - REQ-UI-PROMPT-ALIAS-002
system_design:
  - ../../specs/ui/system-design/prompt-alias-rendering.md
legacy_specs: []
---

# Implementation plan: Compact task-create prompt chips

## Overview

Make editable saved-prompt references smaller and enclose their removal button
inside the green chip. The user accepted the compact ASCII direction on 2026-09-14.
One work order delivers presentation and regression coverage together.
UI owns the existing reusable alias presentation contract.

## Scope

### In scope

- Desktop density, one enclosing border, separate preview/removal targets.
- Truncation, wrapping, focus feedback, and phone/coarse-pointer sizing.
- Regression evidence for editing, previews, undo, and alias serialization.

### Out of scope

- Transcript/history restyling, other chip families, toolbar or dialog redesign.
- Removing user-authored blank lines, changing prompt expansion or persistence.
- Backend changes, new packages, runtime flags, and delegation.

## Technical approach

Follow [Compact editable reference presentation](../../specs/ui/system-design/prompt-alias-rendering.md#compact-editable-reference-presentation).
`TaskPromptReferenceView` owns the enclosing shell and occurrence removal.
An opt-in presentation on `PromptMentionChip` reuses preview state and content.
Keep its read-only defaults unchanged. Use the existing responsive hooks and
semantic sibling actions. No global chip-size or editor-font change is needed.

This follows the completed [task-create package](../task-create-prompt-chips/plan.md).
Its historical results remain valid for that implementation; this package owns
new sizing/containment evidence. The [transcript package](../prompt-alias-rendering/plan.md)
remains unchanged because its layout and contract are outside this change.

## ASCII UI preview

### UI-01: Task creation, desktop reference

Current source and supplied screenshot: `[@fix-repo-issue] x`.
Proposed region:

```text
+-----------------------------------------------------+
| https://github.com/kdlbs/kandev/issues/3663           |
|                                                     |
| [ @fix-repo-issue x ]                               |
|                                                     |
| [Attach] [Enhance]                                  |
+-----------------------------------------------------+
```

One green border encloses preview and removal. Desktop outer height is 24px,
label size is 12px, and the close icon stays visible. Existing draft newlines
are preserved; drawing whitespace does not prescribe added margins.
Maps to `.3`, `.4`, and `.8`; desktop geometry and keyboard checks prove it.

### UI-02: Phone creation, reference and preview

```text
+------------------------------+
| New task                     |
| Goal text                    |
| +--------------------------+ |
| | @fix-repo-issue       x   | |
| +--------------------------+ |
| [Attach] [Enhance]            |
| ... existing form scroll ... |
| [Cancel]        [Start task] |
+------------------------------+
Tap label -> existing preview drawer
Tap x     -> remove this occurrence
```

Both targets are at least 44px square, inside one border. This sizing also
applies to coarse-pointer tablets. Preview keeps its fixed header and internal
safe-area-aware scroll area; dismissal returns to the same draft.
Form/footer and editor scroll ownership follow the existing creation surface.
Maps to `.4`, `.6`, and `.10`; mobile geometry and interaction checks prove it.

### UI-03: Long names and repeated references, both viewports

```text
[ @a-very-long-prompt-na... x ]
[ @fix-repo-issue x ] [ @fix-repo-issue x ]
```

A name truncates before the fixed removal target; complete chips wrap when the
next chip cannot fit. The full name remains accessible. Removing one occurrence
retains the other and surrounding text; undo restores it. With no references,
only the original editable text remains, with no empty chip row.
Maps to `.3`, `.9`, and `.10`; long-name, duplicate, and removal tests prove it.

Grouping, order, containment, and pointer-specific target sizes are required.
ASCII spacing and example content are illustrative. Product labels stay localized.

## Tests

All criterion suffixes below refer to `AC-UI-PROMPT-ALIAS-002`.

| Criteria | Evidence |
| --- | --- |
| `.3`, `.4`, `.10` | Extend `components/task-prompt-reference-editor.test.tsx`, including existing `removes only the selected occurrence through its visible action`, with removal isolation, keyboard activation, no submission/preview, and undo preservation. |
| `.3` | Existing `lib/prompts/task-prompt-document.test.ts` preserves exact plain-text serialization. |
| `.4`, `.10` | Existing `components/task/chat/messages/prompt-mention-components.round2.test.tsx` protects default shared preview behavior; add opt-in versus default composition assertions. |

Use TDD for changed behavior. CSS dimensions require browser evidence, not
jsdom class-name snapshots. Do not add unrelated test suites.

## E2E tests

Extend these existing files. Proposed test titles identify implementation deliverables.

| Project / file | Scenario and criteria |
| --- | --- |
| chromium / `tests/task/task-create-prompt-autocomplete-qa.spec.ts` | `editable prompt chip is compact and contains removal`: outer height 24px within 1px, computed label size 12px, enclosing border contains both controls; hover/focus and keyboard removal/undo. `.3`, `.4`, `.8`, `.10`. |
| chromium / same file | `long prompt chips truncate and wrap without hiding removal`: very long and repeated names, full accessible labels, no target overlap or horizontal overflow. `.9`. |
| chromium / `tests/task/task-create-prompt-autocomplete.spec.ts` and QA file | Existing selection/submission tests retain literal aliases and normal editor behavior. `.3`, `.4`. |
| chromium / `tests/chat/chat-prompt-mention-hover.spec.ts` | Shared read-only chip preview regression; default dimensions/style receive component evidence. `.10`. |
| mobile-chrome / `tests/task/mobile-task-create-prompt-chips.spec.ts` | Extend `edits saved-prompt chips and submits their aliases on mobile`: enclosing-border containment, both target widths/heights at least 44px, tap preview/dismiss/remove, unchanged alias payload. `.3`, `.4`, `.6`, `.10`. |
| mobile-chrome / same file | `long prompt chips remain usable across touch widths`: canonical project phone viewport plus 767px, 768px, and 900px coarse-pointer widths. Check computed sizing, label truncation, no overlap/overflow, and reachable removal. `.6`, `.9`, `.10`. |

Desktop geometry checks also run at 767px and 768px with fine pointers to prove
phone sizing switches to desktop sizing at the canonical boundary. Use the
existing Pixel 5 project for mobile; no per-test device replacement.
Seed disposable prompts and clean up in `finally`. Scope selectors to the open
create dialog and individual reference occurrence. Capture and inspect desktop,
phone, long-name, and focused-control screenshots against UI-01 through UI-03.

## Work orders

- [x] [Task 01: Compact editable prompt chips](task-01-compact-chips.md)

Wave 1, no dependencies, sequential execution. No implementation is authorized
by creation of this package.

## Verification commands

Run from repository root in sequence. Managed E2E commands rebuild the production
web/backend and provide isolated runtime data. Do not use `--no-build` after edits.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task-prompt-reference-editor.test.tsx components/task/chat/messages/prompt-mention-components.round2.test.tsx lib/prompts/task-prompt-document.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task-prompt-reference-node.tsx components/task/chat/messages/prompt-mention-components.tsx components/task-prompt-reference-editor.test.tsx e2e/tests/task/task-create-prompt-autocomplete-qa.spec.ts e2e/tests/task/mobile-task-create-prompt-chips.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run e2e:sleep-ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium tests/task/task-create-prompt-autocomplete.spec.ts tests/task/task-create-prompt-autocomplete-qa.spec.ts tests/chat/chat-prompt-mention-hover.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-task-create-prompt-chips.spec.ts --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Verification results

Implementation validation on 2026-09-14:

- TDD RED recorded a failing compact/touch geometry regression before the implementation; the GREEN focused Vitest run passed 3 files and 33 tests.
- `pnpm run typecheck`: passed.
- Targeted ESLint for the changed production, unit-test, and E2E files: passed.
- `pnpm run i18n:check`, `pnpm run i18n:ratchet`, and `pnpm run e2e:sleep-ratchet`: passed.
- The required desktop managed E2E suite passed 16 tests.
- The required mobile managed E2E suite passed 2 tests.
- `pnpm run build`: passed. The build emitted Vite advisory warnings about deprecated chunk configuration, large chunks, and ineffective dynamic imports, but no errors.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`: passed.
- Additional capture runs passed with `CAPTURE_PR_ASSETS=1`; desktop and mobile compact, preview, and long-name screenshots were inspected against UI-01 through UI-03.
- Production and permanent test files were changed only within the scoped frontend implementation and regression coverage; no backend or locale catalog changes were needed.

Review remediation on 2026-09-14:

- Restored the established 44px coarse-pointer target for default read-only prompt previews while leaving fine-pointer defaults unchanged.
- Constrained the editable shell with border-box sizing and checked its bounds against the editor content box and editor scroll metrics.
- Added an inner truncation label for editable triggers and fallbacks, with long-name checks that verify actual label overflow.
- Focused Vitest regressions passed 3 files and 35 tests after the corrections.
- Typecheck, targeted ESLint, i18n checks and ratchet, E2E sleep ratchet, and `git diff --check` passed after the corrections.
- Fresh managed desktop and mobile capture suites passed 16 and 2 tests; their compact, preview, and long-name screenshots were inspected after the corrections.

PR fixup remediation on 2026-09-14:

- Merged the current `main` base before the fixup so the PR does not remove
  delivery artifacts added after the original branch point.
- Split touch and hover preview prop contracts, used the existing localized
  `task:prompt` description for the touch drawer, and made mobile containment
  assertions cover both axes for both actionable children.
- Guarded the touch preview trigger's pointer press so dismissing its drawer
  does not leave the atom selected when the user continues typing.
- Focused Vitest regressions passed 3 files and 35 tests; typecheck and the
  targeted ESLint command passed.
- The mobile managed E2E suite passed 2 tests after the fixup changes.

Design validation on 2026-09-14:

- `python3 scripts/list-docs.py validate`: passed, 267 decisions and 902 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- `git status --short -- docs/plans/compact-task-prompt-chips`: confirmed both new artifacts.
- The catalog discovers the owning requirement/design pair. Work-order IDs and
  design paths resolve. Implementation and rendered checks completed in the
  implementation turn.

## Risks

- Shared presentation overrides could change read-only chips if defaults leak.
- Borders and flex sizing can reduce a nominal 44px child or clip long labels.
- Event bubbling can open preview while removing a reference or submit a form.
- Inline node resizing must not alter draft whitespace or undo behavior.

## Documentation impact

Internal specifications and plans change. Public docs describe preview and removal
without prescribing control geometry; no public copy or navigation changes are planned.
