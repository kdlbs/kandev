---
id: "02-e2e-coverage"
title: "Prove desktop and mobile collapse flows"
status: done
wave: 2
depends_on: ["01-collapsible-section"]
plan: "plan.md"
spec: "../../specs/ui/available-to-install-collapsible.md"
---

# Task 02: Prove desktop and mobile collapse flows

## Acceptance

- Desktop E2E proves: the section is expanded by default; clicking the heading row collapses the card grid (`aria-expanded` becomes false, grid cards hidden); clicking again re-expands it.
- Mobile E2E proves the same toggle by tap at the `mobile-chrome` viewport, with no document horizontal overflow.
- Both tests seed a discoverable-but-not-installed agent so the section renders.

## Files likely touched

- `apps/web/e2e/tests/settings/available-to-install-collapsible.spec.ts`
- `apps/web/e2e/tests/settings/mobile-available-to-install-collapsible.spec.ts`

## Inputs

- Spec: Scenarios 1, 2, 3, 5.
- Plan: **Tests** and **E2E Tests** sections.
- Patterns: `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts` (visits `/settings/agents` with seeded runtime data), `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts` (mobile tap and viewport assertions), `apps/web/e2e/tests/settings/agent-install-streaming.spec.ts` (mock-agent install fixtures).

## TDD sequence

1. Write both specs; the desktop one fails because `available-to-install-trigger` does not exist.
2. After task 01 lands, run the desktop spec, then the mobile spec, against the freshly built production Vite bundle served by the Go backend (`pnpm e2e:run` rebuilds).
3. Assert the collapsed state with `toBeHidden()` (Radix unmounts closed content) rather than element-presence checks.

## Verification

- `cd apps && pnpm install --frozen-lockfile` (fresh-worktree bootstrap; skip when dependencies are already installed)
- `cd apps/web && pnpm e2e:run tests/settings/available-to-install-collapsible.spec.ts tests/settings/mobile-available-to-install-collapsible.spec.ts`

## Dependencies

- `01-collapsible-section`

## Output contract

Summary, files changed, exact command/result, screenshot or rendered evidence paths when available, uncertainties, and this task file updated to `done` with a `## Results` section; synchronize `plan.md`'s Wave 2 checkbox and Verification Results.

## Results

- Desktop spec `apps/web/e2e/tests/settings/available-to-install-collapsible.spec.ts` (chromium project): `pnpm e2e:run --host tests/settings/available-to-install-collapsible.spec.ts` → `1 passed (6.6s)`. Covers default-expanded, click-to-collapse (`aria-expanded` false, card hidden via `toBeHidden()`), click-to-re-expand.
- Mobile spec `apps/web/e2e/tests/settings/mobile-available-to-install-collapsible.spec.ts` (mobile-chrome project, Pixel 5): `pnpm e2e:run --host --project mobile-chrome tests/settings/mobile-available-to-install-collapsible.spec.ts` → `1 passed (6.8s)`. Covers ≥44px touch target, tap collapse/expand, and no document horizontal overflow in both states.
- Both specs intercept `/api/v1/agents/available` to seed one discoverable-but-not-installed agent (`codex`) so the section renders.
- Web unit suite (`make test-web`): 10495 passed, 4 skipped; 7 failures were reproduced in isolation as environment-only — 3 in `lib/http-git-server.test.ts` fail in fixture setup (`Could not determine Docker bridge gateway: undefined`, requires a Docker daemon) and the rest were parallel-load timeouts that pass in isolation. None touch the changed code.
- External side-effect boundaries: none (tests use the existing mock backend fixtures).
