---
id: "03-e2e-verification"
title: "Prove variation workflows"
status: done
wave: 3
depends_on: ["01-runtime-resolution", "02-frontend-advisory"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002
acceptance_criteria:
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.2
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.3
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.6
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.7
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
---

# Task 03: Prove Variation Workflows

## Summary

Extend the existing model-mismatch browser flows with unique and ambiguous
catalogs. Prove the launch result, durable warning, and touch interaction.

## In scope

- Desktop launch and reload coverage for one variation.
- Desktop no-inference coverage for multiple variations.
- Mobile advisory and warning access through tap.
- Horizontal overflow checks for the changed mobile states.

## Out of scope

- New browser fixtures unrelated to model catalogs.
- Full E2E suite execution.
- Public documentation.

## Acceptance

- `opus` launches as `opus[1m]` only when that is the sole distinct variation.
- Two variations do not select either candidate.
- The warning survives reload and the mobile flow needs no hover interaction.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/settings/no-silent-model-fallback.spec.ts tests/session/model-mismatch-warning.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/settings/mobile-no-silent-model-fallback.spec.ts tests/session/mobile-model-mismatch-warning.spec.ts
git diff --check
```

## Files likely touched

- `apps/web/e2e/tests/settings/no-silent-model-fallback.spec.ts`
- `apps/web/e2e/tests/settings/mobile-no-silent-model-fallback.spec.ts`
- `apps/web/e2e/tests/session/model-mismatch-warning.spec.ts`
- `apps/web/e2e/tests/session/mobile-model-mismatch-warning.spec.ts`
- `apps/web/e2e/tests/session/model-mismatch-warning-helpers.ts`

## Dependencies

Tasks 01 and 02.

## Risks

- Mock catalogs must prove the executor decision, not only the host advisory.
- Each scenario needs isolated profile data so another catalog does not change
  the distinct candidate count.

## Parallelism

`parallel-safe` with Task 04 after Tasks 01 and 02.

## Inputs

- Completed runtime and frontend work.
- Existing desktop and mobile model-mismatch fixtures.

## Results

Added unique and ambiguous mock-agent catalogs and covered launch, reload,
profile persistence, host advisory access, and mobile overflow. The focused
desktop E2E run passed 5 tests; the focused mobile E2E run passed 3 tests.
