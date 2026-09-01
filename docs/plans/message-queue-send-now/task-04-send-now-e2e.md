---
id: "04-send-now-e2e"
title: "Prove Send Now end to end"
status: done
wave: 4
depends_on: ["02-replacement-turn-dispatch", "03-send-now-queue-controls"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-send-now.md"
---

# Task 04: Prove Send Now end to end

## Intent

Exercise the real cancel-and-replacement flow through the production frontend
and backend on desktop and mobile, including exact row selection, bulk ordering,
workflow-step preservation, and touch geometry.

## Acceptance

1. Desktop Playwright proves selected Send Now dispatches the chosen row while
   preserving remaining order and proves header Send Now creates one FIFO-
   concatenated replacement turn.
2. The workflow step does not advance from cancellation semantics, and the
   authoritative queue/transcript settle without duplicate turns.
3. Mobile Playwright uses `.tap()`, proves both controls are at least 44px and
   reachable without hover, and asserts no document horizontal overflow.

## Files likely touched

- `apps/web/e2e/tests/chat/message-queue.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`
- a focused sibling helper only if needed to keep either spec below repository
  size/complexity limits

## Dependencies

- Task 02 backend action and cancellation behavior.
- Task 03 frontend controls and stable test IDs.

## Parallelism

The E2E and public-doc files are disjoint, so implementation work can overlap
after dependencies complete. Final status/results edits to the shared `plan.md`
record are serialized after both tasks finish. User authorization is still
required before using subagents.

## Inputs

- Spec scenarios for selected, all, workflow preservation, and phone parity.
- Plan: E2E Tests and Responsive and Mobile Contract.
- Existing helpers: `openQueuePanel`, `typeWhileBusy`, `SessionPage`, and
  `mobile-message-queue-management.spec.ts` touch geometry helper.

## Verification

Bootstrap once if needed:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run the focused scenarios through the managed runner:

```bash
cd apps/web && pnpm e2e --project=chromium e2e/tests/chat/message-queue.spec.ts --grep 'Send Now' --retries=0
cd apps/web && pnpm e2e --project=mobile-chrome e2e/tests/chat/mobile-message-queue-management.spec.ts --grep 'Send Now' --retries=0
```

If the managed runner is required for a fresh embedded build, run the
repository build/sync wrapper first, then use the same focused Playwright
commands. Confirm each command discovers the intended test count before
recording it as evidence.

## Output contract

Report files changed, RED and GREEN runs, discovered/passed test counts,
desktop/mobile rendered evidence or artifact paths, teardown evidence, blockers
or flake analysis, and synchronize this task plus `plan.md` status/results.

## Results

- Added desktop row and bulk replacement scenarios plus the Pixel 5 row scenario. Desktop proves exact second-row selection, remaining FIFO order, one bulk FIFO-concatenated prompt, no ordinary cancellation message, and workflow-step preservation. Mobile uses `.tap()`, checks both Send Now controls, 44px geometry, no hover dependency, replacement content, and document horizontal containment.
- Discovery: desktop queue spec listed 12 tests and mobile queue-management listed 2 tests.
- GREEN desktop: `cd apps/web && pnpm e2e --project=chromium e2e/tests/chat/message-queue.spec.ts --grep 'Send Now' --retries=0` — 2 passed in 33.4s.
- GREEN mobile: `cd apps/web && pnpm e2e --project=mobile-chrome e2e/tests/chat/mobile-message-queue-management.spec.ts --grep 'Send Now' --retries=0` — 1 passed in 13.1s.
- The managed runner initially used stale embedded frontend assets; rebuilding with `make build-web`, `make sync-embedded-web && make build-backend` produced the final no-retry pass. No E2E artifacts are checked in.
