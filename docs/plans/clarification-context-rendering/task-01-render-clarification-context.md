---
id: "01-render-clarification-context"
title: "Render clarification context"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/clarification-context.md"
---

# Task 01: Render clarification context

## Acceptance

- A non-empty `ask_user_question_kandev.context` value appears exactly once at
  the bundle level and remains visible while navigating between questions.
- The context is normal text with top spacing and no border, padding, or filled
  container.
- Missing or whitespace-only context renders no empty region; existing answer,
  skip, and carousel behavior remains unchanged.
- Desktop and `mobile-chrome` E2E coverage proves visibility, single rendering,
  wrapping containment, and no document-level horizontal overflow.

## Verification

Bootstrap once if workspace dependencies are absent:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run RED before changing production code, then rerun both commands after the
GREEN implementation and final refactor:

```bash
(cd apps/web && pnpm e2e:run tests/chat/clarification.spec.ts -- --grep "shared context")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep "shared context")
```

Confirm each focused command discovers and runs one intended test. The managed
runner must build current production assets and report teardown completion.

## Files likely touched

- `apps/web/components/task/chat/clarification-input-overlay.tsx`
- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/chat/clarification.spec.ts`
- `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. The browser regression and UI renderer share one behavior boundary
and must preserve the RED-to-GREEN evidence in a single implementation pass.

## Inputs

- Spec `What`, `Scenarios`, and `Out of scope`.
- Plan sections `Pending clarification overlay`, `E2E page object`, and
  `Mobile design contract`.
- Existing `clarification-multi` mock-agent scenario, which already supplies
  the shared context string.
- Existing task and mobile clarification E2E suites and `SessionPage` locators.

## Output contract

Report the RED failure, GREEN results, files changed, exact commands and test
counts, managed-runner cleanup, remaining risks, and synchronized task/plan
status. Do not introduce new UI copy or backend contract changes.

## Results

- Added `readSharedContext` and rendered the first sorted message's non-empty
  context once between the overlay header and active question card. Original
  whitespace is preserved for authored line breaks; `whitespace-pre-wrap` and
  `break-words` contain long content.
- Added the `SessionPage.clarificationContext()` locator.
- Desktop coverage proves one rendering, exact text, placement above the card,
  exclusion from the question card, persistence across carousel navigation, and
  no region for the existing context-omitting single-question scenario.
- Mobile coverage proves one rendering, containment within the overlay, no
  document-level horizontal overflow, and persistence after touch navigation.
- RED and GREEN results are recorded in `plan.md`; both managed runner commands
  built production assets and completed teardown.
- A disposable production-build capture test produced and visually validated
  desktop and phone screenshots for the PR; the capture test was removed after
  asset generation.
- PR fixup addressed the valid review comments by scoping clarification
  locators to the active chat and making the two task commands independent;
  both focused E2E commands passed again against fresh production builds.
- Visual refinement follow-up added 12px top spacing and removed the context
  container's padding, border, and background; both focused E2E commands passed
  again against fresh production builds.
- `pnpm run typecheck`, changed-file ESLint, `pnpm run i18n:ratchet`, and
  `git diff --check` passed.
