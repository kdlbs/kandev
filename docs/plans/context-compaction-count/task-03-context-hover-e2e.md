---
id: "03-context-hover-e2e"
title: "Cover desktop and mobile hover"
status: done
wave: 3
depends_on: ["02-context-hover-ui"]
plan: "plan.md"
spec: "../../specs/ui/requirements/context-compaction-count.md"
---

# Task 03: Cover desktop and mobile hover

## Intent

Prove that pointer and touch users can read the inferred count and its accuracy disclosure without destabilizing or overflowing the existing context-window hover.

## Acceptance

- The desktop context-hover scenario verifies the seeded count, pointer-reachable accuracy help, and continued visibility of the parent hover.
- The mobile scenario verifies the same count and help through tap-pinned interaction with no document-level horizontal overflow.
- Shared E2E seeding supplies the count through the same `ContextWindowEntry` shape used by production state.

## TDD sequence

1. Extend the shared seed plus desktop/mobile expectations and run both focused specs against the pre-change UI to observe the missing count/help failure.
2. With Task 02 present, rerun the managed production-build E2E commands and inspect any failure artifacts.
3. Reconcile selectors and geometry assertions without changing the feature contract.

## Files likely touched

- `apps/web/e2e/tests/chat/context-window-source-helpers.ts`
- `apps/web/e2e/tests/chat/context-window-source.spec.ts`
- `apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts`

## Dependencies

Task 02.

## Parallelism

`sequential` — browser proof depends on the completed backend-shaped frontend state and hover UI.

## Verification

- `cd apps && pnpm install --frozen-lockfile`
- `cd apps/web && pnpm e2e:run tests/chat/context-window-source.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-context-window-source.spec.ts`

## Inputs

- Spec final scenario and `Out of scope`.
- Plan `Mobile design contract` and `E2E Tests`.
- Existing E2E mechanics in `.agents/skills/e2e/SKILL.md` and current source-help tests.

## Output contract

Report the result, actual files changed, discovered test counts, exact commands, browser artifacts, teardown evidence, blockers, risks, and synchronized task/plan status in this conversation.

## Results

- `cd apps/web && pnpm e2e:run tests/chat/context-window-source.spec.ts` — Chromium desktop passed (1 test).
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-context-window-source.spec.ts` — mobile-chrome passed (1 test).
- Both managed runs built the backend and Vite assets, seeded the shared count (`2`), verified the inline accuracy disclosure, and completed teardown without overflow failures.
