---
id: "05-browser-coverage"
title: "Add browser coverage"
status: done
wave: 4
depends_on: ["02-quick-actions", "03-watch-prompts", "04-core-settings-prompts"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-prompt-editor.md"
---

# Task 05: Add browser coverage

## Acceptance

- Desktop E2E proves GitHub quick-action placeholder and saved-prompt completion, save coordination, and reload persistence.
- Custom-prompt E2E proves that an editor offers other prompts but omits itself.
- Mobile E2E proves the same GitHub completion value with touch selection and no viewport overflow.

## Verification

Follow E2E TDD. Run each new scenario before the production migration and record the missing-menu or textarea failure.

```bash
cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run tests/integrations/github-quick-action-prompt-autocomplete.spec.ts tests/settings/prompts-settings.spec.ts tests/workflow/workflow-step-autocomplete.spec.ts && pnpm e2e:run --project mobile-chrome tests/integrations/mobile-github-quick-action-prompt-autocomplete.spec.ts
```

## Files likely touched

- `apps/web/e2e/helpers/settings-prompt-editor.ts`
- `apps/web/e2e/tests/integrations/github-quick-action-prompt-autocomplete.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-github-quick-action-prompt-autocomplete.spec.ts`
- `apps/web/e2e/tests/settings/prompts-settings.spec.ts`
- `apps/web/e2e/tests/workflow/workflow-step-autocomplete.spec.ts`
- `apps/web/e2e/tests/workflow/workflow-settings.spec.ts`
- `apps/web/e2e/tests/workflow/workflow-duplication.spec.ts`
- `apps/web/e2e/pages/workflow-settings-page.ts` only if a reusable editor locator belongs there

## Dependencies

Tasks 02, 03, and 04.

## Parallelism

Parallel-safe with Task 06 after Tasks 02 through 04. This task owns only browser helpers and E2E files.

## Inputs

- Spec: all Scenarios, with emphasis on quick actions, custom prompts, persistence, and phone behavior.
- Plan: `E2E Tests` and `Mobile design contract`.
- Existing patterns: `workflow-step-autocomplete.spec.ts`, `mobile-github-integration-layout.spec.ts`, and Monaco's `native-edit-context` focus target.

## Output contract

Report RED failures, discovered test counts, desktop and mobile results, screenshots or traces, cleanup evidence, overflow measurements, blockers, risks, and synchronized task and plan status.

## Results

- The first desktop run failed because the saved-prompt provider was not registered when prompt data loaded before Monaco's one-shot mount callback. The fix added latest-callback registration and a focused regression test.
- A mobile run then exposed Monaco's default `z-index: 40` competing with the mobile floating Save bar. The Settings prompt editor raises its suggestion widget to `z-index: 50`, allowing real touch selection.
- Desktop GitHub quick-action completion and reload persistence pass: 1 test.
- Mobile GitHub quick-action completion passes with touch editor entry, touch completion selection, viewport containment, and no document horizontal overflow: 1 test.
- Existing workflow completion coverage passes: 3 tests. Custom-prompt exclusion and existing prompt settings coverage passes: 4 tests.
- Fresh desktop and mobile PR asset captures were generated and visually inspected for the migrated GitHub quick-action editor.
