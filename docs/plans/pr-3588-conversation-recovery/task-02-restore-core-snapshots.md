---
id: "02-restore-core-snapshots"
title: "Restore core snapshots during recovery"
status: done
wave: 2
depends_on:
  - "01-correct-replay-grants"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.5
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.6
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.9
  - AC-PLUGINS-PROMPT-HISTORY-HOST-005.3
system_design:
  - "../../specs/plugins/system-design/conversation-recovery.md"
---

# Task 02: Restore core snapshots during recovery


## Replacement scope, 2026-09-16

The [conversation storage replacement](../conversation-storage-replacement/plan.md) owns the next implementation.
This file preserves historical scope and results. Do not execute its durable replay mechanics as new work.
The replacement work orders preserve public behavior and provide new source-reconciliation, upgrade, and E2E evidence.


## Summary

Repair core Chat after cursor replacement without a socket reconnect.
A successful recovery includes authoritative message and turn reconciliation.

## In scope

- Internal session recovery generation, completion notification, and projection barrier.
- Integration with existing core message and turn hydration.
- Skipped deletion repair, rich fields, active-turn state, retry, and lifecycle fencing.
- Shared B1/B2 real-transport browser regression.

## Out of scope

Public plugin API, plugin page cache ownership, new persistence, and page reload workarounds.

## Acceptance

- An idle mounted Chat receives a changed message and completed turn after invalid resume while the socket remains connected.
- Delayed or failed snapshots cannot release projection or restore removed rows. Newer live changes survive the snapshot handoff.
- Session removal, owner disposal, or a newer recovery generation prevents stale writes and releases abandoned work.

## TDD sequence

1. Add the store-and-hook regression in new `use-session-recovery.test.tsx`.
   Seed an idle session, stale message and turn, and a skipped valid change.
   Keep the socket connected and assert both stores repair.
2. Add tests for skipped deletion, buffered update, failed snapshot, repeated recovery,
   old-generation completion, and a subscribed session without a mounted owner.
3. Connect the transport to existing hydration owners with awaitable completion.
   Use explicit read success. Do not infer success from a resolved best-effort helper.
4. Add the real-transport cases in new `conversation-recovery.spec.ts`.
   Reuse the isolated backend and causal controls from the existing plugin specs.
   Any poison injection belongs to test helpers or the guarded test harness only.

## Files likely touched

- `apps/web/lib/ws/client.ts` and `client.test.ts`
- `apps/web/lib/ws/ordered-session-events.ts` for internal recovery state
- New focused internal recovery module under `apps/web/lib/ws/`, if needed
- `apps/web/hooks/domains/session/use-session-messages.ts` and its tests
- `apps/web/hooks/domains/session/use-session-turns-hydration.ts` and its tests
- New `apps/web/hooks/domains/session/use-session-recovery.test.tsx`
- `apps/web/lib/state/slices/session/turn-actions.ts` only for authoritative recovery semantics
- New `apps/web/e2e/tests/plugins/conversation-recovery.spec.ts`
- `apps/web/e2e/helpers/ws-drop.ts`, `ws-traffic.ts`, or `causal-waits.ts` only for causal fixtures
- `apps/backend/internal/office/testharness/routes.go` only if a guarded poison fixture is required

## Verification

From the repository root, after implementation/validation authorization:

```bash
(cd apps/web && pnpm exec vitest run lib/ws/client.test.ts hooks/domains/session/use-session-recovery.test.tsx hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-session-turns-hydration.test.ts lib/state/slices/session/turn-actions.test.ts)
(cd apps/web && pnpm run typecheck)
make -C apps/backend build
(cd apps/web && pnpm run build:e2e)
make -C apps/backend e2e-plugin-package
(cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/plugins/conversation-recovery.spec.ts)
git diff --check
```

Packaging rebuilds the fixture UI. Do not bypass artifact freshness or overlap E2E suites.
Add the exact focused test command for any additional helper suite changed.

## Dependencies

Task 01 provides correct grants. Later tests must not work around its defect.

## Risks

Do not deadlock a loader on the barrier it must complete.
Existing latest-window merging and best-effort turn hydration need explicit recovery handling.
The current rich core API must remain distinct from sanitized plugin reads.


## Inputs

- [Plan and evidence](plan.md).
- [Recovery design](../../specs/plugins/system-design/conversation-recovery.md).
- [Existing requirement](../../specs/plugins/requirements/prompt-history-extraction-host.md).
- Source baseline: PR #3588, commit `792264a0736563d9597bda15cdda5ad97bd60f84`.
- Read the scoped AGENTS.md and TDD skill before implementation.

## Parallelism

`sequential`. Another agent can execute this work order after the user dispatches it.
Do not spawn agents from this work order.

## Results

RED evidence reproduced the connected recovery gap: replacement advanced the
core session cursor without notifying the message and turn owners, so a valid
change skipped by an invalid resume remained stale while the socket stayed
connected.

The client now pauses projection by recovery generation, keeps buffered events
behind the hydration barrier, and resumes only after authoritative messages
and turns succeed. Core message windows retain pending local rows while
authoritative snapshots remove stale rows. Turn replacement preserves newer
live rows and reconciles the active-turn marker. Failed, stale, disposed, and
terminal generations remain paused or are discarded.

GREEN evidence:

- The focused recovery command passed: 5 files, 130 tests.
- The expanded recovery and Host command passed: 8 files, 171 tests.
- pnpm run typecheck: passed.
- make -C apps/backend build: passed. The build reported only existing
  unsigned Darwin artifact warnings.
- pnpm run build:e2e: passed.
- make -C apps/backend e2e-plugin-package: passed.
- The recovery Chromium E2E passed both recovery cases.
- git diff --check: passed.

The package does not change the public SDK or transport ownership decision.
The checks do not constitute a full backend audit or the full PR review.
