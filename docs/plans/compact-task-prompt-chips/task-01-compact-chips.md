---
id: 01-compact-chips
title: Compact editable prompt chips
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-PROMPT-ALIAS-002
acceptance_criteria:
  - AC-UI-PROMPT-ALIAS-002.3
  - AC-UI-PROMPT-ALIAS-002.4
  - AC-UI-PROMPT-ALIAS-002.6
  - AC-UI-PROMPT-ALIAS-002.8
  - AC-UI-PROMPT-ALIAS-002.9
  - AC-UI-PROMPT-ALIAS-002.10
system_design:
  - ../../specs/ui/system-design/prompt-alias-rendering.md
---

# Task 01: Compact editable prompt chips

## Summary

Implement the compact enclosing chip and prove desktop and touch editing behavior.
Read the [requirements](../../specs/ui/requirements/prompt-alias-rendering.md)
and [design](../../specs/ui/system-design/prompt-alias-rendering.md#compact-editable-reference-presentation).

## Scope

Own node-view presentation, opt-in shared preview styling, and focused tests.
Preserve all existing editor handlers, plain-text contracts, and shared defaults.
Implement the complete scenario matrix in the [plan](plan.md#e2e-tests).

## Exclusions

No backend, persistence, prompt matching, expansion, other chip families,
editor-wide fonts/padding, toolbar layout, or transcript restyling.

## Acceptance

1. Rendered desktop/phone geometry and interaction match UI-01 through UI-03.
2. Removal, preview, keyboard operation, undo, and payload regressions pass.
3. All targeted checks pass; record results and screenshots before marking done.

## ASCII UI preview

These are the relevant views from the [combined preview](plan.md#ascii-ui-preview).

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

## Likely files

- `apps/web/components/task-prompt-reference-node.tsx`.
- `apps/web/components/task/chat/messages/prompt-mention-components.tsx`.
- `apps/web/components/task-prompt-reference-editor.test.tsx`.
- `apps/web/components/task/chat/messages/prompt-mention-components.round2.test.tsx`.
- `apps/web/e2e/tests/task/task-create-prompt-autocomplete-qa.spec.ts`.
- `apps/web/e2e/tests/task/mobile-task-create-prompt-chips.spec.ts`.
- This plan, work order, and paired design for accurate delivery results.

Reuse existing translated accessible labels. If new copy proves necessary,
update all locale catalogs under the repository i18n workflow and add those
paths to the work order before completion.

## Dependencies

None. Existing task-create prompt-reference implementation is present.
Execute in the primary session after an explicit implementation request.

## Verification

Use TDD: first record a defect-specific failing geometry or interaction assertion,
then implement and rerun. Missing test selectors alone do not prove the defect.
Follow the plan's named tests and screenshot checks. Run from repository root:

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

## Risks

Keep the close control outside the preview trigger's interactive DOM subtree.
Do not let shared chip classes override compact sizing or leak into read-only
consumers. Verify actual border and target bounds, including long names.

## Results

Completed 2026-09-14.

- TDD RED recorded a failing compact/touch geometry regression before the implementation; the GREEN focused Vitest run passed 3 files and 33 tests.
- `pnpm run typecheck`: passed.
- Targeted ESLint for the changed production, unit-test, and E2E files: passed.
- `pnpm run i18n:check`, `pnpm run i18n:ratchet`, and `pnpm run e2e:sleep-ratchet`: passed.
- The required desktop managed E2E suite passed 16 tests.
- The required mobile managed E2E suite passed 2 tests.
- `pnpm run build`: passed. The build emitted Vite advisory warnings about deprecated chunk configuration, large chunks, and ineffective dynamic imports, but no errors.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`: passed.
- Additional capture runs passed with `CAPTURE_PR_ASSETS=1`; desktop and mobile compact, preview, and long-name screenshots were inspected against UI-01 through UI-03.
- No backend or locale catalog changes were needed.

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
- Focused Vitest regressions passed 3 files and 35 tests; typecheck and the
  targeted ESLint command passed.
- The mobile managed E2E suite passed 2 tests after the fixup changes.
