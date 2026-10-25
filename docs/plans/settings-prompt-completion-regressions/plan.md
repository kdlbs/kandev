---
spec: docs/specs/ui/requirements/settings-prompt-editor.md
created: 2026-08-20
status: done
---

# Fix Plan: Settings Prompt Completion Regressions

## Overview

Correct completion metadata and loading races in the shared Monaco prompt
editor. Placeholder completion must account for an unfinished name suffix
before existing closing braces. Saved-prompt completion must expose the prompt
name as its filtering value, keep the `@name` display label, and continue
filtering across spaces in valid multi-word names. Multiple mounted editors
must share one in-flight prompt request. The repair remains frontend-only and
preserves the existing plain-text and save contracts.

## Confirmed Root Cause

- For `{{|}}`, the placeholder provider uses the cursor-only replacement range
  and always inserts `${key}}}`. The existing `}}` remains after the cursor,
  so selection produces `{{key}}}}`. The same range bug leaves a suffix such
  as `_prompt}}` behind when the cursor is inside `{{ta_prompt}}`.
- For `@c`, the saved-prompt provider uses an `@name` label but its
  replacement range starts after `@`. Monaco filters the typed name prefix
  against the label when `filterText` is absent, so a valid prefix such as
  `c` does not match the label's first character, `@`.
- The `[\w-]*` mention-prefix pattern stops at a space. A valid prompt named
  `Daily Summary` therefore loses its mention start when the user types
  `@Daily `.
- Every mounted editor can observe `loading=false` in the same render and
  start its own request. A late failure can then overwrite the shared prompt
  list with an empty result.

## Frontend

### Completion provider behavior

Update `apps/web/components/settings/profile-edit/script-editor-completions.ts`:

- Make placeholder completion insert only the placeholder name when the text
  after the cursor contains the remaining placeholder name followed by `}}`.
  Extend the replacement range across that suffix.
- Keep the existing insertion of one closing pair for an unfinished token.
- Set each saved-prompt completion item's `filterText` to `prompt.name` while
  retaining the `@${prompt.name}` display label and the existing insertion
  value.
- Find the whitespace-bounded mention start directly, use the complete
  substring after `@` as the active prefix, and retrigger completion after a
  space.
- Share an in-flight `listPrompts` request across mounted hook instances while
  applying the settled result to each subscribing store consumer.

No component API, backend contract, persistence format, or mobile composition
changes are required.

## Tests

- **What:** Placeholder selection does not duplicate an existing closing pair
  and still closes an unfinished token.
  **File:** `apps/web/components/settings/profile-edit/script-editor-completions.test.ts`.
  **How:** Call the provider with `{{}}` and the cursor between the pairs, then
  with `{{` at the end, and assert the exact `insertText` values.
- **What:** Saved-prompt suggestions remain filterable by the typed name.
  **File:** `apps/web/components/settings/profile-edit/script-editor-completions.test.ts`.
  **How:** Assert each suggestion exposes the prompt name as `filterText` while
  its label remains prefixed with `@`.

## E2E Tests

- **Scenario:** **GIVEN** a saved prompt named `changes-walkthrough`, **WHEN**
  the user types `@c` in a shared Settings prompt editor, **THEN** the matching
  suggestion remains visible and can be selected.
  **Files:** `apps/web/e2e/tests/workflow/workflow-step-autocomplete.spec.ts`
  and the existing GitHub quick-action autocomplete coverage if needed.
- **Scenario:** **GIVEN** the editor contains `{{}}` with the cursor inside,
  **WHEN** the user selects a placeholder, **THEN** the persisted draft contains
  one balanced token rather than duplicated closing braces.
  **Files:** `apps/web/e2e/tests/integrations/github-quick-action-prompt-autocomplete.spec.ts`
  and its mobile counterpart.

## Review fixup scope

The review follow-up extends the original repair with these cases:

- A partially typed `{{ta_prompt}}` replaces the remaining name suffix before
  existing `}}` and inserts only the selected key.
- A whitespace-bounded mention such as `@Daily ` keeps a valid multi-word
  prompt name available. Space retriggers use the full text after `@` as the
  active replacement range.
- Two mounted prompt editors share one in-flight `listPrompts` request, and
  each consumer receives the settled result.
- Desktop and mobile fixtures use unique prompt names. The GitHub quick-action
  fixture resets action presets in `finally` after it saves a draft.

The focused provider suite and hook suite cover the first three cases. The
existing desktop and mobile completion scenarios cover browser filtering and
cleanup.

## Verification Results

- RED: `pnpm --filter @kandev/web test -- --run components/settings/profile-edit/script-editor-completions.test.ts` failed as expected with 2 regression failures: duplicated placeholder braces and missing saved-prompt `filterText`.
- GREEN: the same focused provider suite passed 10/10 tests.
- `make build-backend build-web-e2e build-e2e-plugin-package` completed successfully; only existing build warnings were emitted.
- `pnpm e2e:raw --project=chromium e2e/tests/workflow/workflow-step-autocomplete.spec.ts e2e/tests/integrations/github-quick-action-prompt-autocomplete.spec.ts` passed 5/5.
- `pnpm e2e:raw --project=mobile-chrome e2e/tests/integrations/mobile-github-quick-action-prompt-autocomplete.spec.ts` passed 1/1.
- `pnpm run typecheck` passed.
- Focused ESLint passed for the changed completion source and unit test.
- `git diff --check` passed. No temporary source or browser fixture files remain.
- Review fixup RED: provider regressions failed for the partial placeholder and
  multi-word mention, while the two-editor hook regression observed two prompt
  requests.
- Review fixup GREEN: focused provider, hook, shared-editor, and Monaco suites
  passed 21 tests; web typecheck and focused ESLint passed.
- Review fixup GREEN: `make build-backend build-web-e2e
  build-e2e-plugin-package` passed; Chromium coverage passed 5/5 and mobile
  coverage passed 1/1. The GitHub test completed its prompt and preset cleanup.
- Review fixup GREEN: `git diff --check` passed.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-completion-provider-regressions](task-01-completion-provider-regressions.md)

The task is sequential because it changes one shared provider and its unit and
browser regression coverage.

## Out of Scope

- Changes to saved-prompt expansion, placeholder interpolation, or backend APIs.
- Changes to Monaco theme, layout, mobile composition, or completion contents.
- New public documentation, because this repair restores the behavior already
  promised by the shipped prompt-editor spec.

## Risks

- Completion insertion must distinguish a cursor before existing `}}` from an
  unfinished token without changing replacement ranges for typed names.
- `filterText` must affect Monaco's filtering only; the visible `@name` label
  and inserted `name` value must remain unchanged.
