---
id: "04-e2e-and-verification"
title: "E2E and verification"
status: pending
wave: 3
depends_on: ["02-backend-expiration-service", "03-boot-hydration"]
plan: "plan.md"
spec: "../../specs/tasks/quick-chat-expiration.md"
---

# Task 04: E2E and Verification

## Acceptance

- Quick-chat E2E coverage proves a reload restores at least one existing quick-chat tab with prior history.
- Multi-tab reload behavior is covered without adding a count cap.
- Final verification runs targeted backend/frontend tests plus format/typecheck/lint commands appropriate for changed files.

## Verification

- `cd apps/web && pnpm e2e -- tests/chat/quick-chat.spec.ts`
- `make -C apps/backend test`
- `cd apps/web && pnpm run typecheck`
- `cd apps && pnpm --filter @kandev/web lint`

## Files likely touched

- `apps/web/e2e/tests/chat/quick-chat.spec.ts`
- Test helpers only if the reload flow needs a reusable selector/helper.

## Dependencies

- Task 02 for backend expiration coverage.
- Task 03 for reload hydration behavior.

## Inputs

- Spec scenarios for reload restoration and 12-tab/no-cap behavior.
- Existing quick-chat E2E helpers in `apps/web/e2e/tests/chat/quick-chat.spec.ts`.

## Output contract

Update this task status to `done`, update the Wave 3 checkbox in `plan.md`, and report tests run, any skipped checks, and residual risks.
