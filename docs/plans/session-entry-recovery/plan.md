---
created: 2026-09-12
status: complete
requirements:
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001
system_design:
  - ../../specs/platform/system-design/session-subscription-recovery.md
legacy_specs: []
---

# Implementation Plan: Recover delayed session entry

## Overview

Restore chat entry after temporary transport delays, then present accurate loading and recovery states.
Platform owns this work because it owns subscription readiness and shared session recovery.
The [requirement](../../specs/platform/requirements/session-subscription-recovery.md) extends the existing contract.
The [system design](../../specs/platform/system-design/session-subscription-recovery.md) defines timing, ownership, and presentation.

## Evidence and confirmed root cause

On 2026-09-12, task `41227cde-bb02-4738-b9c0-1b776929d68e` received status and subscription responses after 6.9 seconds.
The five-second client deadline had already discarded both requests.
History waited on rejected subscription readiness and never sent its initial request.
Foreground refresh later loaded 100 messages in 351 milliseconds.
The status exception became a generic startup-failure banner; failed history became an empty invitation.
The message-fetch catch also clears cached history, which compounds recovery failures.

Backend registration occurred promptly. The downstream delivery bottleneck remains unconfirmed.
This package repairs the demonstrated timeout, retry, and presentation defects.
It does not claim a backend or network performance fix.

## Scope

### In scope

- Bounded, shared subscription recovery and read-only status retries.
- History retry with preserved cache and truthful initialization state.
- Desktop, phone, and preview feedback with localized recovery controls.
- Regression tests for timing, ownership, mutations, and rendered states.

### Out of scope

- Global timeout increases, automatic mutation retries, and server replay protocols.
- Agent startup policy, archive admission, and unrelated error redesign.
- Proxy tuning or a diagnosis of the original delivery bottleneck.

## Technical approach

Task 01 implements the design's client and hook state. It retains acknowledgement-before-history ordering.
Task 02 connects those outcomes to the shared chat renderer and session feedback, then proves both viewports.
Use typed timeout classification in `lib/ws/request-error.ts`; keep actual backend errors distinct.
Keep registration ownership in `WebSocketClient` and mutation admission in existing session recovery code.
New state belongs to session identity and connection generation, not a global pending boolean.

## ASCII UI preview

UI-01: Open existing task, current failure (desktop and phone).

```text
[!] Couldn't start a session
    WebSocket request timed out: task.session.status
    [Retry]

    No messages yet. Start the conversation!
```

UI-02: Open existing task, proposed desktop chat region.

```text
Loading:   (spinner) Loading conversation...
Retrying:  (spinner) Taking longer than usual. Retrying...
Exhausted: Conversation could not load.  [Retry] [Details v]
Ready:     <conversation, or confirmed empty invitation>
```

UI-03: Same task, proposed phone chat region.

```text
+------------------------------------+
| Conversation could not load.        |
| [ Retry ]  [ Details v ]            |
|                                    |
| <cached conversation stays visible>|
+------------------------------------+
| <existing composer and navigation> |
+------------------------------------+
```

UI-04: Status-only exhaustion with a readable transcript.

```text
Session status is unavailable. [Retry] [Details v]
<conversation remains visible and scrollable>
```

Details expands inline with wrapped technical text. Loading and retrying use a
neutral status region. Neither uses a destructive alert or the empty invitation.
Phone actions occupy their own row and have at least 44-pixel targets.
Desktop actions use the normal 28-pixel size. The chat remains the scroll owner;
existing composer placement and safe-area behavior remain unchanged.
These hierarchy and state choices are required; wording and spacing are illustrative.
All text is localized. Covers AC-002.4 through AC-002.6 and AC-002.8 through AC-002.10
(where AC-002 denotes AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002).

## Tests

- `lib/ws/client.test.ts`: seven-second acknowledgement, timeout retry, exhausted budget, shared consumers, last unsubscribe, and reconnect.
- `hooks/domains/session/use-session-messages.test.ts`: automatic history recovery, no request before acknowledgement, preserved cache, empty success, and session-switch cancellation.
- `hooks/domains/session/use-session-subscription-retry.test.ts`: unknown-session retry joins current registration without resetting its budget.
- `hooks/domains/session/use-session-resumption.test.ts`: seven-second status success, bounded retry, immediate permanent rejection, and one mutation after success.
- `components/task/chat/session-entry-feedback.test.tsx` (new): loading, unavailable, cached, confirmed-empty, and disclosure states.
- Existing `components/task/ensure-session-error.test.tsx`: actual launch and profile errors retain their actions.

Task 01 covers AC-002.1 through AC-002.7 and AC-002.10, plus AC-001.1 through AC-001.8.
Task 02 covers all AC-002 criteria through rendering and E2E.
Use fake timers for deterministic unit timing. Record the expected behavioral RED before changing production code.

## E2E tests

New files: `apps/web/e2e/tests/session/session-entry-recovery.spec.ts` (chromium)
and `apps/web/e2e/tests/session/mobile-session-entry-recovery.spec.ts` (mobile-chrome).
Seed an existing stopped session with known persisted messages and use a real WebSocket proxy.
Delay selected subscription/status responses beyond five seconds with `injectLatency`.
Verify automatic chat appearance without reload or tab visibility changes.
Drop a first response to prove retry; drop every attempt to prove bounded exhaustion and explicit recovery.
Assert request counts, no duplicate launch, no stale startup banner, and no false empty invitation.
Also cover cached refresh failure, a successful empty history, and permanent authorization rejection.
On phone, tap Retry and Details, measure targets, and check overflow at phone width and around the 768-pixel boundary.
Reuse `e2e/helpers/session-capabilities.ts` for frame routing and `causal-waits.ts` for event waits.
The new shared helper is `e2e/helpers/session-entry-recovery.ts`.

## Work orders

- [x] [Task 01: Recover entry requests](task-01-entry-recovery.md)
- [x] [Task 02: Present conversation recovery](task-02-recovery-feedback.md)

## Verification results

Implementation complete. Design-package checks and implementation checks passed:

- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/session-entry-recovery docs/plans/session-subscription-recovery`: passed.
- `cd apps/web && pnpm exec vitest run lib/ws/client.test.ts hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-session-subscription-retry.test.ts hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption.archive.test.ts hooks/domains/session/use-session-message-fetch.test.ts components/task/chat/session-entry-feedback.test.tsx components/task/ensure-session-error.test.tsx components/task/chat/message-list-shared.test.tsx`: 170 tests passed across 9 files.
- `cd apps/web && pnpm run typecheck`: passed.
- The zero-warning ESLint commands listed in both work-order Results: passed.
- `cd apps/web && pnpm run i18n:zh-hant`: passed; generated the Traditional Chinese pair.
- `cd apps/web && pnpm run i18n:check`: passed; all five complete catalogs and pseudo locale are synchronized.
- `cd apps/web && pnpm run i18n:ratchet`: passed; no new untranslated copy.
- `cd apps/web && pnpm e2e:run --project chromium tests/session/session-entry-recovery.spec.ts tests/session/session-resume-recovery.spec.ts`: 5 passed, including the production web build.
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-entry-recovery.spec.ts tests/session/mobile-session-resume-recovery.spec.ts`: 2 passed, including phone touch geometry and overflow checks.
- `git diff --check`: passed.
- Public documentation impact: no public docs change is required for this recovery behavior.

Review remediation validation:

- Status restoration and backend error-payload regressions passed, including the assertion that a
  permanent status error does not launch a session.
- Deferred history generation regressions passed for terminal fetch, session A to B navigation,
  session A to B to A re-entry, unmount-safe finalization, and fresh manual Retry.
- The focused recovery suite passed 170 tests across 9 files after review remediation. It
  includes truthful workspace-restore failures, permanent status error payloads, terminal and
  manual history refreshes, A-to-B and A-to-B-to-A deferred races, stale loading finalization,
  and the single live status retry path. Typecheck, zero-warning ESLint, and formatting checks
  passed.

## Risks

- Retries can duplicate launches if they wrap `processResumeStatus` instead of its status read.
- Per-consumer retry loops can amplify traffic or replace another consumer's readiness promise.
- Clearing loading flags before recovery finishes can reintroduce the false empty state.
- A longer entry deadline improves tolerance but does not remove the original delivery delay.

## Related package

[Original ordering repair](../session-subscription-recovery/plan.md) remains implemented.
Its historical checks do not count as validation of this package.
