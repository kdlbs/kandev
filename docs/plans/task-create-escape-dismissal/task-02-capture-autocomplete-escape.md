---
id: "02-capture-autocomplete-escape"
title: "Capture autocomplete Escape and retain focus"
status: complete
wave: 2
depends_on: ["01-guard-dialog-escape"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-create-escape-dismissal.md"
---

# Task 02: Capture Autocomplete Escape And Retain Focus

## Intent

Close the Create Task prompt autocomplete before later browser or dialog handlers can move focus.
Keep the prompt textarea focused and preserve the equivalent `@` and `#` task-chat behavior.

## Acceptance

- Create Task handles an open mention menu's Escape during capture phase.
- The inner autocomplete claims Escape with `preventDefault()` and `stopPropagation()`.
- The menu closes, the dialog remains open, and the prompt textarea regains focus on the next frame.
- Task chat keeps editor focus after dismissing `@` and `#` autocomplete.
- Desktop and mobile hardware-keyboard scenarios prove continued typing after dismissal.

## TDD sequence

1. Add unit regressions for capture-phase delivery and scheduled focus restoration.
2. Run them and record the expected failures on the current implementation.
3. Move mention keyboard handling to textarea capture and add focus restoration.
4. Add desktop, mobile, and task-chat E2E focus/continued-typing assertions.
5. Run the focused unit and E2E commands with retries disabled.

## Files likely touched

- `apps/web/components/task-create-dialog-selectors.tsx`
- `apps/web/components/task-create-dialog-selectors.test.tsx`
- `apps/web/hooks/use-inline-mention.ts`
- `apps/web/hooks/use-inline-mention.test.ts`
- `apps/web/e2e/tests/task/task-create-prompt-autocomplete-qa.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-create-escape.spec.ts`
- `apps/web/e2e/tests/chat/entity-reference-composer.spec.ts`

## Parallelism

`sequential`. Unit and E2E coverage exercise the same keyboard event contract.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  hooks/use-inline-mention.test.ts \
  components/task-create-dialog-selectors.test.tsx
cd apps/web && pnpm e2e:run tests/task/task-create-prompt-autocomplete-qa.spec.ts \
  -- --grep "Escape closes the menu" --retries=0
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome \
  tests/task/mobile-task-create-escape.spec.ts -- --retries=0
cd apps/web && pnpm e2e:run --no-build tests/chat/entity-reference-composer.spec.ts \
  -- --grep "keeps Kandev task suggestions under @|pasted hash text stays literal" --retries=0
cd apps/web && pnpm run typecheck
```

## Risks

- Stopping Escape too early could prevent the inner autocomplete from receiving it.
- Moving all textarea keyboard handling to capture could suppress form shortcuts; only mention
  handling moves, while unclaimed keys continue to the existing bubble handler.
- A focus-only assertion can miss a menu that remained mounted; E2E must assert both menu removal
  and continued typing.

## Results

- RED unit run: 2 expected failures and 32 passing tests.
- GREEN unit run: 34 tests passed.
- Desktop Create Task E2E: 1 autocomplete Escape scenario passed.
- Mobile Create Task E2E: 2 hardware-keyboard Escape scenarios passed.
- Task-chat E2E: 2 `@` and `#` Escape scenarios passed.
- Frontend typecheck and changed-file ESLint passed.

## Output contract

Report RED/GREEN results, exact commands, changed files, preview verification, and remaining risks.
