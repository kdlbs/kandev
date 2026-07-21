---
id: "04-e2e"
title: "User message navigation E2E"
status: done
wave: 3
depends_on: ["03-renderer-integration"]
plan: "plan.md"
spec: "../../specs/ui/user-message-navigation.md"
---

# Task 04: User Message Navigation E2E

## Acceptance

- Desktop E2E proves one up action crosses several 20-row pages, down returns through loaded history, the target centers or reaches the nearest boundary and highlights, boundaries disable correctly, and native/Virtuoso behavior matches.
- Mobile E2E proves the same value with touch-visible 44px controls, safe-area containment, readable content, and no document horizontal overflow.
- The old per-message arrows are absent and **Load older messages** remains available as a fallback.

## Verification

```bash
make build-web
cd apps/web && pnpm e2e:run tests/chat/user-message-navigation.spec.ts tests/chat/mobile-user-message-navigation.spec.ts
```

## Files Likely Touched

- `apps/web/e2e/tests/chat/user-message-navigation.spec.ts`
- `apps/web/e2e/tests/chat/mobile-user-message-navigation.spec.ts`
- `apps/web/e2e/tests/chat/message-pagination.spec.ts` if its seeding helper is reused
- `apps/web/e2e/pages/session-page.ts` only if shared rail locators improve clarity

## Dependencies

Task 03.

## Inputs

- Spec `Scenarios`.
- Existing multi-page seed and fallback assertions in `apps/web/e2e/tests/chat/message-pagination.spec.ts`.
- `ApiClient.seedToolCallMessages` and `seedSessionMessage` for deterministic long-history gaps.
- Mobile filename routing and Pixel 5 project rules from `/mobile-parity` and `/e2e`.

## Output Contract

Report exact E2E command, native/Virtuoso and desktop/mobile coverage, pass/fail result, artifact paths for failures, geometry/overflow findings, blockers, and residual risks; set this task to `done` and tick it in `plan.md`.

## Outcome

- Desktop Chromium: native and Virtuoso renderers passed the multi-page up/down flow, hover/focus disclosure, boundary state, reduced-motion highlight, and retained fallback assertions.
- Mobile Chrome: persistent 44px controls passed multi-page navigation, viewport containment, safe content clearance, and horizontal-overflow assertions.
- Verification completed with focused Vitest coverage, TypeScript typecheck, zero-warning ESLint, production Vite build, and Playwright E2E.
