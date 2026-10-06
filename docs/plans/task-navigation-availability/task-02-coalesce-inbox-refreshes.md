---
id: "02-coalesce-inbox-refreshes"
title: "Coalesce inbox refreshes"
status: done
wave: 2
depends_on:
  - "01-bound-clarification-reads"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-004
acceptance_criteria:
  - AC-PLATFORM-INTERACTIVE-READS-004.4
  - AC-PLATFORM-INTERACTIVE-READS-004.5
  - AC-PLATFORM-INTERACTIVE-READS-004.6
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
---

# Task 02: Coalesce inbox refreshes

## Summary

Make all existing Inbox triggers share one request and one pending refresh.
Repeated events must respect a common cooldown after temporary errors.

## In scope

- Coordinate mount, reconnect, foreground, periodic, snooze, and WS triggers in
  `useNeedsYouInboxController`. Share the request promise and one dirty marker.
- Return the actual request promise to `useForegroundRefresh`; its current callback
  returns before the request completes, so its own in-flight guard cannot help.
- Use one-second minimum spacing for background starts. Temporary failures apply
  2/5/15/30-second cooldowns, capped at 30 seconds and extended by Retry-After.
- Keep the existing visible-only periodic trigger and badge/list failure semantics.
- Abort through `ApiRequestOptions.init.signal`, clear timers, and fence late results
  across workspace, authentication, feature activation, and unmount changes.

## Out of scope

New refresh event families, cross-tab locking, changed Inbox presentation, and
optimistic badge values. Do not reopen the shipped background-loading repair.

## Acceptance

1. A deferred request plus mixed trigger bursts yields one in-flight request and
   at most one trailing refresh. The latest workspace changes eventually appear.
2. 503 plus continuing WS events cannot bypass cooldown or create a failure loop.
   Success resets cooldown; non-retryable failures suspend automatic work.
3. Scope changes abort old transport and timers. Late results cannot affect the
   new workspace. Existing badge, snooze, and empty/error rendering tests still pass.

## Verification

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/needs-you-inbox/use-needs-you-inbox-controller.test.tsx lib/state/slices/needs-you-inbox/needs-you-inbox-slice.test.ts)
(cd apps/web && pnpm run typecheck)
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --project chromium -- tests/chat/needs-you-inbox-background-refresh.spec.ts)
```

Use causal Playwright waits and existing event injection helpers. Assert bounded
request counts while server responses are held at a barrier; do not rely on sleeps.
This change modifies shared state/transport only, without rendered or touch changes.
The existing phone Inbox flows retain the same controller; targeted controller
tests satisfy mobile parity for this narrow work order.

## Files likely touched

- `apps/web/hooks/domains/needs-you-inbox/use-needs-you-inbox-controller.ts`
- `apps/web/hooks/domains/needs-you-inbox/use-needs-you-inbox-controller.test.tsx`
- A small coordinator beside that hook, only if required by lint limits.
- `apps/web/e2e/tests/chat/needs-you-inbox-background-refresh.spec.ts`

## Dependencies

Task 01 establishes backend overload behavior and bounds work from other tabs.

## Risks

Dropping the dirty marker strands a real change. Reconnect and foreground events
must not reset the failure count. Keep mutation-triggered refreshes coherent with
the existing Inbox state machine. Transport abort supplements generation checks.

## Parallelism

`sequential`

## Inputs

- Clarification workload control in the platform design.
- Existing Inbox refresh design and shipped
  [background-loading repair](../needs-you-inbox/task-03-fix-background-refresh-loading-state.md).
- `apps/web/hooks/use-foreground-refresh.ts` and Stats temporary-error classification.

## Results

Implemented one scoped refresh coordinator for mount, reconnect, WS, foreground,
periodic, and snooze triggers. It shares the active request and one dirty marker,
enforces one-second start spacing, and applies 2/5/15/30-second temporary-error
cooldowns extended by Retry-After. Success resets the backoff; non-retryable
errors suspend automatic refresh until scope changes. Inbox Retry uses a
separate explicit signal so user recovery remains available.

Reads carry an AbortSignal. Workspace, authentication, feature, and unmount
changes cancel the request, delayed refresh, and pending WS debounce; guards
prevent late results from reaching the slice. Foreground refresh returns the
actual coordinator promise. Existing error clearing and visible-only periodic
behavior are retained.

Tests cover mixed triggers, one trailing refresh, all four cooldown intervals,
Retry-After, backoff reset, permanent-error suspension, explicit retry, scope
cancellation, and late-result fencing. The existing Inbox badge and loading
behavior remain covered. No rendered controls or touch behavior changed; the
shared controller continues to drive desktop and phone Inbox surfaces.

Validation passed:

- Focused Vitest controller and slice suites: 28 tests passed.
- `pnpm run typecheck` from `apps/web`.
- `make build-web` and `make build-backend`.
- Targeted Chromium background-refresh E2E: 2 tests passed, including the
  held-response burst and 503 Retry-After recovery.
- `git diff --check`.
