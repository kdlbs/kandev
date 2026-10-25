---
id: "07-responsive-status-e2e"
title: "Prove responsive status flows"
status: pending
wave: 6
depends_on: ["05-responsive-mcp-status-surface"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
---

# Task 07: Prove responsive status flows

## Acceptance

- Desktop E2E proves hover, focus, click-to-pin, all display states, two
  concurrent session reports, tab switching, reload hydration, diagnostic
  actions, and sanitized copy.
- Mobile E2E uses `.tap()`, proves the 44px trigger and inset Drawer, exercises
  a diagnostic action, and detects horizontal overflow.
- E2E setup uses isolated backend fixtures and asserts through the UI.
- Backend hook integration tests remain the evidence for real MCP protocol
  observation; browser tests do not pretend seeded state proves wire behavior.

## Verification

- `cd apps/web && pnpm e2e:run tests/chat/mcp-status.spec.ts`
- `cd apps/web && pnpm e2e:run tests/chat/mobile-mcp-status.spec.ts -- --project=mobile-chrome`

Follow RED/GREEN/REFACTOR: write each Playwright scenario against the production
build, observe the missing/incorrect UI failure, then add only the necessary
fixture support. Scope visible Radix portals and use bounding boxes for the
44px and overflow contracts.

## Files likely touched

- `apps/web/e2e/tests/chat/mcp-status.spec.ts`
- `apps/web/e2e/tests/chat/mobile-mcp-status.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/pages/session-page.ts`
- `apps/backend/internal/backendapp/e2e_reset.go`

## Dependencies

- Task 05 completes the user-facing status surface and diagnostic requests.

## Parallelism

Sequential in the primary conversation because it validates the integrated
feature and can reveal contract gaps in earlier tasks.

## Inputs

- E2E skill production-build runner and mobile `.tap()` guidance
- Existing session creation/tab page objects
- Existing E2E-only session/message seeding patterns

## Output contract

Report each RED failure, fixture changes, desktop and mobile assertions,
production-build commands, exact final results, artifact paths for any failure,
files changed, blockers, and risks. Mark this task `done` and update its plan
checkbox.
