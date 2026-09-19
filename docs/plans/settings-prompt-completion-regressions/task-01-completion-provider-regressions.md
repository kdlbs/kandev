---
id: "01-completion-provider-regressions"
title: "Fix prompt completion insertion and filtering"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-prompt-editor.md"
---

# Task 01: Fix prompt completion insertion and filtering

## Acceptance

- Selecting a placeholder at `{{|}}` produces exactly `{{key}}` and does not
  duplicate the existing closing braces.
- Selecting a placeholder after `ta` in `{{ta_prompt}}` replaces the full
  partial token name and produces exactly `{{key}}`.
- Selecting a placeholder at an unfinished `{{|` still produces exactly
  `{{key}}`.
- Saved-prompt completion labels remain `@name`, while typing a name prefix
  keeps matching suggestions available and selection still inserts `@name`.
- Saved prompts with spaces remain filterable after typing a prefix such as
  `@Daily `.
- Two mounted prompt editors share one in-flight saved-prompt request.
- Desktop and mobile shared prompt-editor flows retain the same behavior.

## Verification

Follow TDD. Add the provider regression tests first and confirm they fail for
the current insertion metadata and missing `filterText`.

From `apps`:

```bash
pnpm --filter @kandev/web test -- --run components/settings/profile-edit/script-editor-completions.test.ts
pnpm --filter @kandev/web test -- --run hooks/domains/settings/use-custom-prompts.test.tsx
```

After the unit fix, rebuild the production web/backend artifacts required by
the E2E fixture and run the focused desktop and mobile scenarios:

```bash
make build-backend build-web-e2e build-e2e-plugin-package
cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/workflow/workflow-step-autocomplete.spec.ts e2e/tests/integrations/github-quick-action-prompt-autocomplete.spec.ts
cd apps/web && pnpm e2e:raw --project=mobile-chrome e2e/tests/integrations/mobile-github-quick-action-prompt-autocomplete.spec.ts
```

Also run `cd apps/web && pnpm run typecheck`, the focused ESLint command for
the changed files, and `git diff --check` before marking the task done.

## Files likely touched

- `apps/web/components/settings/profile-edit/script-editor-completions.ts`
- `apps/web/components/settings/profile-edit/script-editor-completions.test.ts`
- `apps/web/e2e/tests/workflow/workflow-step-autocomplete.spec.ts`
- `apps/web/e2e/tests/integrations/github-quick-action-prompt-autocomplete.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-github-quick-action-prompt-autocomplete.spec.ts`
- `docs/specs/ui/requirements/settings-prompt-editor.md`
- `docs/plans/settings-prompt-completion-regressions/plan.md`
- `docs/plans/settings-prompt-completion-regressions/task-01-completion-provider-regressions.md`

## Dependencies

None.

## Parallelism

Sequential. The provider and all regression tests share the same completion
contract.

## Inputs

- The repaired behavior and scenarios in `docs/specs/ui/requirements/settings-prompt-editor.md`.
- The confirmed root causes and exact source files in `plan.md`.
- Existing Monaco provider tests and the shared prompt-editor E2E helpers.

## Output contract

Report the RED failures, implementation result, files changed, exact commands
and outcomes, any browser screenshot or cleanup evidence, and synchronized task
and plan status.

## Review fixup scope

- Cover the partial `{{ta_prompt}}` replacement range and multi-word `@Daily `
  filtering contract.
- Deduplicate prompt loading for two mounted editors and deliver one settled
  result to both consumers.
- Use unique prompt fixtures and restore GitHub action presets in `finally`.

## Results

- RED: `pnpm --filter @kandev/web test -- --run components/settings/profile-edit/script-editor-completions.test.ts` failed with the expected two assertions: `filterText` was undefined and `{{}}` completion returned `task_prompt}}`.
- `apps/web/components/settings/profile-edit/script-editor-completions.ts` now preserves an existing trailing `}}` and sets saved-prompt `filterText` to the prompt name.
- Added unit regressions for balanced and unfinished placeholder tokens and `@` name filtering.
- Updated desktop workflow E2E coverage to complete inside `{{}}` and type `@c`; prompt fixtures use unique `c-` names to avoid seeded-data collisions.
- Updated the mobile GitHub quick-action E2E flow to type `@c` and verify touch filtering.
- GREEN: focused provider suite passed 10/10.
- `make build-backend build-web-e2e build-e2e-plugin-package` completed successfully.
- Desktop Playwright command passed 5/5; mobile Playwright command passed 1/1.
- `pnpm run typecheck`, focused ESLint, and `git diff --check` passed.
- No temporary browser artifacts were added; generated E2E failure artifacts were produced only under the existing ignored test-results directory.
- Review fixup GREEN: the partial placeholder, multi-word mention, and
  two-editor request regressions passed. Chromium passed 5/5 and mobile passed
  1/1 after the fixture cleanup changes.
