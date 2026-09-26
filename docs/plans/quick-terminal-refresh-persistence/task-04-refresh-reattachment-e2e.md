---
id: "04-refresh-reattachment-e2e"
title: "Prove refresh reattachment"
status: done
wave: 4
depends_on: ["03-web-descriptor-reconciliation"]
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 04: Prove refresh reattachment

## Acceptance

- Desktop E2E creates a terminal, writes a marker, reloads, reopens Quick Chat, and verifies one
  same-sequence tab reattaches to the same marker-bearing PTY without duplication.
- Mobile E2E covers reload/reopen containment, touch-safe controls, internal terminal scrolling,
  focus/dismissal, and zero document horizontal overflow.
- Every scenario explicitly closes/stops all surviving terminal descriptors in `finally` so worker
  state does not leak into the next test.

## Verification

```bash
(cd apps/web && pnpm e2e --project=chromium tests/terminal/quick-terminal.spec.ts)
(cd apps/web && pnpm e2e --project=mobile-chrome tests/terminal/mobile-quick-terminal.spec.ts)
```

## Files likely touched

- `apps/web/e2e/tests/terminal/quick-terminal.spec.ts`
- `apps/web/e2e/tests/terminal/mobile-quick-terminal.spec.ts`
- `apps/web/e2e/helpers/layout-assertions.ts` (only if a shared assertion is needed)

## Dependencies

Task 03.

## Parallelism

Sequential. These tests share the persisted descriptor and backend worker lifecycle with the
implementation tasks.

## Inputs

- Spec refresh, backend-restart, cross-client, mobile, and cleanup scenarios.
- Existing `closeSurvivingQuickTerminals` helper and marker-buffer assertions.
- Existing Pixel 5 project configuration and backend fixture reset/restart behavior.

## Output contract

Report exact Playwright commands, pass counts, browser/viewport evidence, teardown evidence, and
any environment-only limitations. Synchronize task/plan status and verification results.

## Results

- `(cd apps/web && pnpm e2e:run --host --project chromium tests/terminal/quick-terminal.spec.ts)` passed (2/2), including reload/reopen and marker replay.
- `(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/terminal/mobile-quick-terminal.spec.ts)` passed (1/1) on the Pixel 5 project, including dynamic-viewport containment, touch-safe controls, internal scrolling, focus/dismissal, and zero horizontal overflow.
- Both managed runs completed fixture teardown, and the tests explicitly closed surviving terminal descriptors in `finally` blocks.
