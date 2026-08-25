---
spec: docs/specs/ui/requirements/entity-reference-composer.md
created: 2026-08-17
status: completed
---

# Implementation Plan: Entity Reference Trigger Lifecycle

## Overview

Restrict the `#` menu to direct user entry and connect all dismiss actions to the TipTap suggestion state. Preserve multi-word search and result selection.

## Confirmed root cause

`createEntityReferenceSuggestion` accepts every document transaction that leaves a valid `#query` before the cursor. It does not identify the input source.

As a result, a paste transaction can start the suggestion. An isolated Chromium test reproduced this behavior with `#2730 explicit profile overridden seems an issue`.

The test expected zero `entity-reference-menu` elements. The current code kept one element mounted.

The Escape handler clears only React menu state and returns `true`. TipTap 3.19 treats this return value as proof that the consumer also cleared suggestion state.

The consumer does not clear that state. The outside-close callback has the same split state. A later editor transaction can show the menu again.

The suggestion also uses `allowSpaces: true` for multi-word queries. This option keeps a bare `# ` range active unless Kandev rejects leading query whitespace.

## Frontend

### Input-origin gate

- Add a small origin gate for the entity-reference suggestion.
- Observe direct text input without consuming the editor event.
- Start a closed suggestion only for a new `#` from direct text input.
- Continue an existing active query across later direct input transactions.
- Reject paste, drop, draft restore, history recall, and programmatic content updates.
- Reject whitespace as the first query character. Preserve spaces after the first query character.

The gate stays local to the `#` entity-reference path. It does not change the `@` mention or `/` command paths.

### One dismissal path

- Use the supported TipTap `exitSuggestion` API with `EntityReferenceSuggestionPluginKey`.
- Route Escape and popup outside-close actions through this path.
- Clear the React menu state from the suggestion `onExit` callback.
- Keep the draft, cursor focus, result selection, and submit configuration unchanged.
- Permit a later direct `#` entry after the dismissed range ends.

### Mobile design contract

Desktop and phone use the same TipTap editor and suggestion state. This fix does not change the menu layout, touch targets, scroll owner, or viewport rules.

The existing mobile entity-reference composer is the nearest mobile example. It supplies touch selection, viewport containment, internal scrolling, and 44 px rows.

No mobile-specific presentation is necessary. Re-run the existing mobile test to verify selection and geometry after the shared lifecycle change.

## Tests

- **What:** Direct `#` input starts a closed suggestion. Paste, drop, and programmatic input do not start it.
- **File:** `apps/web/components/task/chat/tiptap-suggestion.test.ts` or a focused sibling test.
- **How:** Use a pure input-origin gate with explicit transaction and input sequences.
- **What:** Escape and outside-close exit both the TipTap state and React state.
- **File:** `apps/web/components/task/chat/tiptap-suggestion.test.ts` and `apps/web/components/task/chat/use-entity-reference-composer.test.ts`.
- **How:** Assert the plugin exit call, the closed state, continued literal input, and a later valid trigger.
- **What:** A bare `#` followed by space closes, while `#multi word` stays active.
- **File:** `apps/web/components/task/chat/tiptap-suggestion.test.ts`.
- **How:** Use table-driven query-range cases.

The browser regression must fail before the production change. It must pass after the change.

## E2E tests

- **Scenario:** The user pastes `#2730 explicit profile overridden seems an issue` into task chat.
- **File:** `apps/web/e2e/tests/chat/entity-reference-composer.spec.ts`.
- **What to verify:** The text stays literal. The menu does not open. No mention-search request starts.
- **Scenario:** The user types a valid query, presses Escape, and continues to type in the same range.
- **File:** `apps/web/e2e/tests/chat/entity-reference-composer.spec.ts`.
- **What to verify:** The menu closes, stays closed, and does not change the draft. A later valid trigger can open the menu.
- **Mobile regression:** Re-run `apps/web/e2e/tests/chat/mobile-entity-reference-composer.spec.ts`.
- **What to verify:** Touch selection, menu containment, scrolling, and the submitted reference remain unchanged.

## Verification results

The diagnostic E2E run failed as expected before production changes. The
managed runner removed its isolated processes. After implementation, the
focused desktop regression passed, the existing mobile regression passed, 41
focused unit tests passed, type checking passed, lint passed with zero
warnings, the internationalization check passed, and `git diff --check`
passed. A PR review fixup also covered direct Backspace/Delete edits inside an
active query and direct `#` insertion before existing text.

## Implementation waves

- [x] [Task 01: Correct the entity-reference trigger lifecycle](task-01-correct-trigger-lifecycle.md)

Execution is sequential in the primary conversation. No subagents are authorized.

## Risks and boundaries

- Browser input events and ProseMirror transactions have different lifecycles. The origin gate must observe input without blocking normal insertion.
- Mobile virtual keyboards and input-method editors can use text input without a physical key event. The gate must not depend on `keydown` alone.
- Escape and outside-close can dispatch a metadata-only transaction. The code must not dispatch twice or restore a stale menu state.
- Multi-word title search requires spaces after the first query character.
- This fix does not change backend search, provider results, reference persistence, `@` mentions, or `/` commands.
- This fix does not convert pasted keys into reference chips.
