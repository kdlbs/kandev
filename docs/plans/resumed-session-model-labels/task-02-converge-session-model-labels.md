---
id: "02-converge-session-model-labels"
title: "Converge resumed session model labels"
status: done
wave: 2
depends_on: ["01-gate-startup-model-events"]
plan: "plan.md"
spec: "../../specs/ui/acp-model-configuration-summary.md"
---

# Task 02: Converge Resumed Session Model Labels

## Acceptance

- Persisted Sol state remains visible through an unsettled Luna startup event and converges on settled Sol.
- Each unnamed tab and task selector show the same authoritative current model.
- Later user model changes still update desktop and mobile selectors.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run lib/ws/handlers/session-models.test.ts lib/ws/handlers/session-models-startup.test.ts components/task/session-tab-title.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run tests/session/session-resume.spec.ts -- --grep "keeps distinct model titles during multi-session resume"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-model-selector.spec.ts -- --grep "model"
```

## Files Likely Touched

- `apps/web/lib/ws/handlers/session-models.ts`
- `apps/web/lib/ws/handlers/session-models.test.ts`
- `apps/web/lib/ws/handlers/session-models-startup.test.ts`
- `apps/web/components/task/session-tab-title.ts`
- `apps/web/components/task/session-tab-title.test.ts`
- `apps/web/e2e/tests/session/session-resume.spec.ts`
- `docs/specs/ui/acp-model-configuration-summary.md`
- `docs/plans/resumed-session-model-labels/plan.md`
- `docs/plans/resumed-session-model-labels/task-02-converge-session-model-labels.md`

## Dependencies

Task 01.

## Parallelism

`sequential`. The restart E2E proves the combined backend and frontend contract.

## Inputs

- Spec scenarios for multi-session resume and live model changes.
- The persisted runtime guard in `apps/web/lib/ws/handlers/session-models.ts`.
- The title precedence in `apps/web/components/task/session-tab-title.ts`.
- The existing multi-session restart fixture in `apps/web/e2e/tests/session/session-resume.spec.ts`.
- The existing mobile selector fixture in `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts`.

## TDD Sequence

1. Add the `Sol -> Luna -> Sol` handler test and record the expected failure.
2. Add the contradictory title-state test and record the expected failure.
3. Keep the persisted guard active until startup settlement and normalize the stored model option.
4. Make the tab title prefer `currentModelId`, with an option-only fallback.
5. Add the two-profile restart E2E and capture tab labels from first render through settlement.
6. Run the focused unit tests, typecheck, desktop E2E, and existing mobile E2E.
7. Update this task, the plan checkbox, the spec status, and verification results.

## Risks

- A permanent hydration barrier can block real post-start changes. The state and settlement tests must prove that the barrier releases.
- The model config option and `currentModelId` must converge before selectors render.
- Mutation-history E2E assertions must ignore missing tabs during layout restoration and inspect only mounted session tabs.

## Output Contract

Report both RED assertions, state and title changes, exact unit and E2E results, files changed, mobile evidence, blockers, risks, and synchronized artifact status.

## Results

- RED: The initial focused frontend run failed in the existing live-option precedence assertion and the new startup hydration assertion. The title test first observed the option-only model label, and the handler test observed the unsettled `gpt-5.6-luna` value instead of persisted `gpt-5.6-sol`.
- GREEN: `cd apps && pnpm --filter @kandev/web test -- --run lib/ws/handlers/session-models.test.ts lib/ws/handlers/session-models-startup.test.ts components/task/session-tab-title.test.ts` passed with 3 files and 29 tests.
- Typecheck: `cd apps/web && pnpm run typecheck` passed.
- Desktop resume E2E: `cd apps/web && pnpm e2e:run tests/session/session-resume.spec.ts -- --grep "keeps distinct model titles during multi-session resume"` passed with 1 test in 9.3 seconds. Both distinct model tab labels remained visible after backend restart and reload.
- Mobile parity E2E: `cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-model-selector.spec.ts -- --grep "model"` passed with 1 test in 11.6 seconds. The existing touch selector path remains usable; no new mobile composition or interaction was required.
- PR evidence: capture runs produced fresh desktop and mobile PNGs, both manifest entries mapped to existing files, both images were inspected for seeded-only content, and `pngquant-bin` compressed them successfully.
- Files changed: `apps/web/lib/ws/handlers/session-models.ts`, `apps/web/lib/ws/handlers/session-models.test.ts`, `apps/web/components/task/session-tab-title.ts`, `apps/web/components/task/session-tab-title.test.ts`, `apps/web/e2e/tests/session/session-resume.spec.ts`, and the synchronized spec/plan/task records.
