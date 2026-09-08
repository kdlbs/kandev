---
id: "01-seed-turn-metadata-and-prove-overflow"
title: "Seed turn metadata and prove the overflow in E2E"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/message-metadata-overflow.md"
---

# Task 01: Seed Turn Metadata and Prove the Overflow in E2E

## Acceptance

- The e2e mock harness accepts an optional `turn_metadata` on the
  `POST /api/v1/_test/messages` request and persists it on the ensured turn,
  so the reported field (`turn_metadata`) can be exercised end-to-end.
- `ApiClient.seedSessionMessage` forwards an optional `turnMetadata` object.
- `message-metadata-overflow.spec.ts` fails on the current layout: the
  entries container reports `scrollHeight == clientHeight` (no scrollbar) and
  the `turn_metadata` field sits below the dialog's visible area.
- `mobile-message-metadata-overflow.spec.ts` reproduces the same failure on
  the Pixel-5 viewport.

## TDD sequence

1. RED: extend `seedMessageRequest`/`seedMessageHandler` in
   `apps/backend/internal/office/testharness/routes.go` and
   `seedSessionMessage` in `apps/web/e2e/helpers/api-client.ts`.
2. RED: add the two E2E specs; run them and confirm the scroll assertions
   fail for the expected reason (no scrollbar, field below the fold).
3. These tests stay red until Task 02 applies the layout fix; do not change
   production code in this task.

## Verification

```bash
(cd apps/backend && make test ./internal/office/testharness/...)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --host -- tests/chat/message-metadata-overflow.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- \
  tests/chat/mobile-message-metadata-overflow.spec.ts)
```

Expected RED: both specs fail on the scroll/reachability assertions.

## Files likely touched

- `apps/backend/internal/office/testharness/routes.go`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/chat/message-metadata-overflow.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-metadata-overflow.spec.ts`

## Dependencies

None (harness change is additive and test-support only).

## Parallelism

`sequential`. The harness change and the E2E specs are one RED cycle and must
land together with Task 02's layout fix.

## Inputs

- `docs/specs/ui/requirements/message-metadata-overflow.md`
- `apps/web/e2e/tests/chat/mobile-message-timestamp-tooltip.spec.ts` (fixture
  pattern)
- `apps/web/e2e/fixtures/test-base.ts` (seed fixture)

## Output contract

Record the RED failure output of both specs (assertion + measured
scrollHeight/clientHeight) for the plan's verification results.
