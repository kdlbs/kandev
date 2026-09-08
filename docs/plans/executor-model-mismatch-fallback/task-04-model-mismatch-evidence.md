---
id: "04-model-mismatch-evidence"
title: "Prove model-mismatch recovery"
status: done
wave: 4
depends_on: ["03-remove-host-model-gates"]
plan: "plan.md"
spec: "../../specs/agents/requirements/no-silent-model-fallback.md"
---

# Task 04: Prove model-mismatch recovery

## Acceptance

- Desktop E2E proves profile selection, task continuation, one warning, and warning persistence after reload.
- Mobile E2E proves the same outcome through touch input without horizontal overflow.
- Public documentation explains executor authority, optional configuration copying, and the unchanged saved profile model.

## TDD scenarios

1. RED: Update the existing profile-picker E2E to require a selectable host mismatch.
2. RED: Add a task-launch scenario with an executor catalog that omits the requested model.
3. RED: Add reload and mobile scenarios for the persisted warning.
4. GREEN: Complete missing fixtures and selectors without weakening the runtime assertions.
5. GREEN: Update the public agent-profile and executor documentation.
6. REFACTOR: Share desktop and mobile model-mismatch setup helpers.

## Verification

- `cd apps/web && pnpm e2e:run tests/settings/no-silent-model-fallback.spec.ts tests/session/model-mismatch-warning.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/settings/mobile-no-silent-model-fallback.spec.ts tests/session/mobile-model-mismatch-warning.spec.ts`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check`

## Files likely touched

- `apps/web/e2e/tests/settings/no-silent-model-fallback.spec.ts`
- `apps/web/e2e/tests/settings/mobile-no-silent-model-fallback.spec.ts`
- `apps/web/e2e/tests/session/model-mismatch-warning.spec.ts`
- `apps/web/e2e/tests/session/mobile-model-mismatch-warning.spec.ts`
- `apps/web/e2e/tests/session/model-mismatch-warning-helpers.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `docs/public/agents-and-profiles.md`
- `docs/public/executors.md`

## Dependencies

- Task 03 completes runtime, persistence, and frontend behavior.

## Parallelism

Sequential after Task 03.
This task validates the complete user flow.

## Inputs

- The completed model-decision and warning flow.
- Existing no-silent-model-fallback E2E files.
- The mock agent model catalog.
- The public executor documentation.

## Output contract

Report exact E2E commands, reload evidence, mobile geometry, public pages, failures, and final package status.

## Risks

- The host and executor catalogs need a deterministic mismatch fixture.
- Worker-scoped profiles and tasks require cleanup after each scenario.
- A warning visible before reload does not prove persistence.

## Results

Desktop and mobile E2E proved task continuation, one warning, saved-profile
preservation, and warning visibility after reload without horizontal overflow.
Public documentation validation passed with 61 tests and 41 published pages.
