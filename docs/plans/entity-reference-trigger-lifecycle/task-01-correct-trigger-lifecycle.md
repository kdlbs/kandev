---
id: "01-correct-trigger-lifecycle"
title: "Correct entity-reference trigger lifecycle"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/entity-reference-composer.md"
---

# Task 01: Correct Entity-Reference Trigger Lifecycle

## Acceptance

- Direct user entry of a valid `#` opens the menu. Paste, drop, restored drafts, history recall, and programmatic updates do not open it.
- Escape and popup outside-close exit the TipTap suggestion state. Continued input in the dismissed range does not reopen the menu.
- A bare `#` followed by space becomes literal. Multi-word queries and Enter or Tab selection remain unchanged.

## Verification

1. If dependencies are absent, run `cd apps && pnpm install --frozen-lockfile`.
2. RED: Run `cd apps/web && pnpm e2e:run tests/chat/entity-reference-composer.spec.ts -- --grep "pasted hash text" --retries=0` before production changes.
3. Unit: Run `cd apps && pnpm --filter @kandev/web test -- components/task/chat/tiptap-suggestion.test.ts components/task/chat/use-entity-reference-composer.test.ts components/task/chat/use-tiptap-editor.test.ts`.
4. Type check: Run `cd apps/web && pnpm run typecheck`.
5. Desktop GREEN: Run `cd apps/web && pnpm e2e:run tests/chat/entity-reference-composer.spec.ts -- --grep "pasted hash text|dismissed entity reference" --retries=0`.
6. Mobile regression: Run `cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-entity-reference-composer.spec.ts -- --retries=0`.
7. Repository check: Run `git diff --check` from the repository root.

Verify that Playwright discovers the expected tests before you use either browser command as evidence. The managed runner builds and removes its isolated processes.

## Files likely touched

- `apps/web/components/task/chat/tiptap-entity-reference-suggestion.ts`
- `apps/web/components/task/chat/use-entity-reference-composer.ts`
- `apps/web/components/task/chat/tiptap-input.tsx`
- `apps/web/components/task/chat/use-tiptap-editor.ts`
- `apps/web/components/task/chat/tiptap-suggestion.test.ts`
- `apps/web/components/task/chat/use-entity-reference-composer.test.ts`
- `apps/web/components/task/chat/use-tiptap-editor.test.ts`
- `apps/web/e2e/tests/chat/entity-reference-composer.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. The input gate, dismissal state, and browser regression form one TDD slice.

## Inputs

- Behavioral contract: `docs/specs/ui/requirements/entity-reference-composer.md`.
- Root-cause trace and frontend design: `plan.md`.
- TipTap lifecycle code: `apps/web/components/task/chat/tiptap-entity-reference-suggestion.ts`.
- Editor input path: `apps/web/components/task/chat/use-tiptap-editor.ts`.
- Existing browser coverage: `apps/web/e2e/tests/chat/entity-reference-composer.spec.ts`.
- Mobile regression: `apps/web/e2e/tests/chat/mobile-entity-reference-composer.spec.ts`.

## Output contract

Report the RED result, implementation summary, changed files, exact test results, cleanup evidence, blockers, and risks. Update this task and `plan.md`.

## Results

- RED unit run: the new input-gate assertions failed because the gate and
  suggestion-plugin exit path were not implemented yet.
- Unit: 41 tests passed across the three focused TipTap test files.
- Type check: `pnpm run typecheck` passed.
- Lint: `pnpm run lint` passed with zero warnings.
- Internationalization: `pnpm run i18n:check` passed.
- Desktop E2E: the pasted-hash and dismissed-menu regression passed.
- Mobile E2E: the existing entity-reference composer test passed.
- PR fixup: direct Backspace/Delete edits keep the active menu open, and a
  direct `#` before existing text remains eligible for a suggestion.
- Repository check: `git diff --check` passed.
