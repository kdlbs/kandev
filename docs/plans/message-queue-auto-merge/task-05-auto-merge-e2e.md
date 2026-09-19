---
id: "05-auto-merge-e2e"
title: "Prove automatic merge flows"
status: completed
wave: 5
depends_on: ["04-auto-merge-settings-ui"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-auto-merge.md"
---

# Task 05: Prove automatic merge flows

## Acceptance

1. Desktop E2E proves the fresh/default value, auto-only save/reload, enabled
   same-user compaction, and disabled separate-row behavior against the real
   backend.
2. Mobile E2E proves the switch is reachable, touch-safe, persistence-equivalent,
   free of horizontal overflow/nested scrolling, and clear of the save bar.
3. Every affected queue E2E scenario that requires multiple compatible rows
   explicitly disables automatic merge, captures all three install-wide queue
   settings, and restores its baseline in teardown even after failure.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run --host tests/system/message-queue-settings.spec.ts tests/chat/message-queue.spec.ts tests/chat/message-queue-reorder.spec.ts
cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/system/mobile-message-queue-settings.spec.ts tests/chat/mobile-message-queue-management.spec.ts tests/chat/mobile-message-queue-reorder.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/system/message-queue-settings.spec.ts`
- `apps/web/e2e/tests/system/mobile-message-queue-settings.spec.ts`
- `apps/web/e2e/tests/chat/message-queue.spec.ts`
- `apps/web/e2e/tests/chat/message-queue-reorder.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-queue-reorder.spec.ts`
- A focused E2E settings helper under `apps/web/e2e/helpers/` if reuse avoids
  duplicated baseline/restore logic.

## Dependencies

Task 04 and all backend tasks.

## Parallelism

Sequential.

## Inputs

- Spec: default, enabled/disabled, cap, independence, persistence, and mobile
  scenarios.
- Plan: `E2E Tests`.
- Existing patterns: API baseline capture in
  `message-queue-settings.spec.ts`, busy-agent queue setup in
  `message-queue.spec.ts`, and mobile layout assertions in
  `mobile-message-queue-settings.spec.ts`.
- E2E fixture contract: install-wide mutable state must be captured and restored
  by the owning spec; mobile changes accompany desktop changes.

## Risks

- Default-on behavior changes setup assumptions in manual merge, send-now, and
  reorder tests. Audit all multi-row queue construction, not only new tests.
- Do not rely on test ordering or the fixture reset to restore install-wide
  settings.
- Assert final authoritative state after the queue response/status event, not a
  transient render.

## Output contract

Report summary, files changed, exact commands/outcomes, baseline restoration and
cleanup evidence, blockers, risks, and update this task plus `plan.md` status in
the same conversation.

## Results

- Added shared E2E setup that captures all three install-wide queue settings,
  disables automatic merging for separate-row scenarios, and restores the full
  baseline after every test.
- Added real-backend desktop coverage for default-on persistence, auto-only
  updates, enabled compaction, disabled fallback, capacity-lock independence,
  and member permissions.
- Added mobile coverage for touch geometry, save/discard persistence, overflow,
  scroll ownership, and save-bar clearance. Audited attribution, steering,
  sidebar-count, reorder, and queue-management scenarios that require separate
  rows.
- Stabilized the existing row Send Now scenario by waiting for observable mock
  turn progress before requesting cancellation.
- Verification passed:
  - desktop Chromium: 20 tests;
  - mobile Chromium: 4 tests;
  - focused attribution, steering, and sidebar-count audit: 3 tests;
  - focused E2E ESLint: zero warnings.
- Blockers: none.

Review remediation makes shared teardown read the effective environment lock
before restoring and omit `max_per_session` when capacity is environment-
controlled.
