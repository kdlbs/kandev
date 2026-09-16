---
created: 2026-09-14
status: complete
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
system_design:
  - "../../specs/plugins/system-design/conversation-recovery.md"
  - "../../specs/plugins/system-design/prompt-history-extraction-host.md"
legacy_specs: []
---

# Implementation Plan: PR 3588 Conversation Recovery


## Replacement scope, 2026-09-16

The [conversation storage replacement](../conversation-storage-replacement/plan.md) owns the next implementation.
This file preserves historical scope and results. Do not execute its durable replay mechanics as new work.
The replacement work orders preserve public behavior and provide new source-reconciliation, upgrade, and E2E evidence.


## Overview

Repair the three recovery defects found in PR #3588 at commit
`792264a0736563d9597bda15cdda5ad97bd60f84`.
First correct server grants. Then repair core snapshot recovery. Finally repair
expired plugin continuations using the corrected grant behavior.

The user requested this package on 2026-09-14 and has now authorized its
implementation, commit, push, and PR fixup. The implementation keeps the
package's bounded scope and records validation below.

## Inputs and settled assumptions

- [Existing requirements](../../specs/plugins/requirements/prompt-history-extraction-host.md)
  remain authoritative. These are implementation defects, not new product requirements.
- [Recovery design](../../specs/plugins/system-design/conversation-recovery.md) supplements the
  [existing Host design](../../specs/plugins/system-design/prompt-history-extraction-host.md).
- [Browser API ADR](../../decisions/2026-09-06-browser-plugin-conversation-facade.md)
  remains proposed. This package does not change its status.
- [Original package](../prompt-history-plugin-host/plan.md) records the contributor's
  implementation. Its earlier results do not prove these new regression cases.
- Static source traces establish the failure paths. No reproduction test ran
  during review or planning. Each implementer must obtain RED evidence first.
- The plugin system owns the existing conversation contract. Its design includes
  the core compatibility adapter; this repair does not transfer ownership.

The separate request to justify durable replay remains unresolved:
[maintainer direction](https://github.com/kdlbs/kandev/issues/3567#issuecomment-5619730030).
It does not block planning bounded conformance fixes within the proposed transport.
It remains a merge-scope decision, not implicit approval of that transport.

## Scope

### In scope

- Resume grants that accept each contiguous replay ACK.
- Core message and turn repair after cursor replacement while connected.
- Plugin continuation recovery after expiry, including suspended tabs.
- Focused TDD and visible regression evidence for these outcomes.
- Lifecycle cancellation, terminal removal, and strict authorization during recovery.

### Out of scope

- Replacing the journal, changing schemas or retention, or redesigning transport ownership.
- Public SDK changes, new error codes, token lifetime increases, or weaker token validation.
- CI remediation unrelated to these findings, other existing review comments, and the unfinished full-PR review.
- Plugin extraction, publication, core panel removal, and layout migration.
- New controls, copy, navigation, or layout. Existing loading and retry surfaces remain.

## Technical approach

### B1: grants

`acceptOrderedSessionSubscription` currently passes `replay.watermark` to
`MintSessionStreamGrant`. That method uses it for both tokens.
`ValidateSessionResume` rejects a smaller ACK sequence.
A reconnect from 10 with replay 11 and 12 therefore rejects ACK 11.

Separate snapshot cutoff from the registered cursor sequence. Determine the
replay disposition and effective cursor before minting grants.
Keep token identity and contiguous ACK validation strict.

### B2: core snapshot recovery

`recoverCoreSessionPoison` currently updates only stream sequence and token.
It does not trigger or await the core message and turn hydration owners.
An idle mounted Chat can retain stale rows after `invalid_resume`.

Add an internal, session-scoped recovery completion path between the client and
the existing hydration owners. Pause projection for that session, refresh both
snapshots, commit the matching recovery generation, then resume buffered events.
Transport readiness and hydration completion are separate barriers.
Do not solve this with a synthetic connection-status change or a page reload.

### B3: expired continuation

`renewContinuation` refreshes the binding but preserves expired cursor and
snapshot tokens. The renewal route rejects them with `invalid_query`.
The public retry method correctly refuses non-retryable errors.

Detect known expiry before renewal and start one fresh scoped subscription and
snapshot cycle. Reuse the existing rebind notification and page revision
mechanisms. Do not repeatedly retry the same expired request.
Keep valid pre-expiry renewal at its original cutoff.

## Tests

Names below are proposed new regression names, not claims of existing coverage.

| Finding | Criteria | RED test and file |
| --- | --- | --- |
| B1 | AC-PLUGINS-PROMPT-HISTORY-HOST-002.5, AC-PLUGINS-PROMPT-HISTORY-HOST-002.13, AC-PLUGINS-PROMPT-HISTORY-HOST-002.15 | `TestOrderedReplayGrantAcceptsEveryContiguousAck` in `apps/backend/internal/gateway/websocket/client_session_ack_test.go` |
| B1 | AC-PLUGINS-PROMPT-HISTORY-HOST-002.7, AC-PLUGINS-PROMPT-HISTORY-HOST-002.15 | `TestOrderedReplayGrantRejectsInvalidAck` in the same file |
| B1 | AC-PLUGINS-PROMPT-HISTORY-HOST-002.5, AC-PLUGINS-PROMPT-HISTORY-HOST-002.15 | `keeps projecting after a multi-event reconnect replay` in `apps/web/lib/plugins/conversation-host.test.tsx` |
| B2 | AC-PLUGINS-PROMPT-HISTORY-HOST-002.5, AC-PLUGINS-PROMPT-HISTORY-HOST-002.6, AC-PLUGINS-PROMPT-HISTORY-HOST-002.9, AC-PLUGINS-PROMPT-HISTORY-HOST-002.15 | `rehydrates both core snapshots before resuming after invalid resume` in new `apps/web/hooks/domains/session/use-session-recovery.test.tsx` |
| B2 | AC-PLUGINS-PROMPT-HISTORY-HOST-002.6, AC-PLUGINS-PROMPT-HISTORY-HOST-002.15 | `buffers events and fences stale recovery completion` in `apps/web/lib/ws/client.test.ts` |
| B3 | AC-PLUGINS-PROMPT-HISTORY-HOST-002.2, AC-PLUGINS-PROMPT-HISTORY-HOST-002.6, AC-PLUGINS-PROMPT-HISTORY-HOST-002.10, AC-PLUGINS-PROMPT-HISTORY-HOST-002.13 | `recovers an expired continuation without remount` in `apps/web/lib/plugins/conversation-host.test.tsx` |
| B3 | AC-PLUGINS-PROMPT-HISTORY-HOST-002.2, AC-PLUGINS-PROMPT-HISTORY-HOST-002.6 | `joins expiry recovery and isolates panel queries` in `apps/web/lib/plugins/conversation-host-isolation.test.tsx` |
| B3 | AC-PLUGINS-PROMPT-HISTORY-HOST-002.7, AC-PLUGINS-PROMPT-HISTORY-HOST-002.10 | `TestConversationContinuationRenewRejectsExpiredTokens` in `apps/backend/internal/plugins/conversation_handlers_test.go` |

## E2E tests

B1 and B2 share a real transport scenario owned by Task 02.
Add `conversation-recovery.spec.ts` under `apps/web/e2e/tests/plugins/`.
Use the existing test-base backend, plugin fixture, and causal WS controls.
Reconnect after multiple durable mutations, then assert ACK success and a later
visible update. In a second case, force cursor replacement while the socket
stays connected and assert repaired core message and turn state.
Map these cases to AC-PLUGINS-PROMPT-HISTORY-HOST-002.5, AC-PLUGINS-PROMPT-HISTORY-HOST-002.9, AC-PLUGINS-PROMPT-HISTORY-HOST-002.15, and
AC-PLUGINS-PROMPT-HISTORY-HOST-005.3.

Task 03 adds an expired-continuation case to that file.
Use an isolated test token manager clock if real expiry is necessary.
Any new clock control belongs only to the existing test harness.
A browser clock cannot expire server tokens by itself.
Do not shorten production TTL or wait ten wall-clock minutes.

Mobile uses the same repaired hooks. This package changes state recovery only:
no layout, touch, scrolling, or navigation change is planned.
Existing `mobile-prompt-history-plugin.spec.ts` remains the mobile parity check.
No new mobile composition or ASCII UI preview is needed.

## Work orders

- [x] [Task 01: Correct replay grants](task-01-correct-replay-grants.md)
- [x] [Task 02: Restore core snapshots during recovery](task-02-restore-core-snapshots.md)
- [x] [Task 03: Recover expired plugin continuations](task-03-recover-expired-continuations.md)

Execution order is 01 -> 02 -> 03. Shared transport, tests, and documentation
make sequential execution appropriate. This order does not authorize delegation.

## Verification results

RED evidence was obtained for all three defects. The original replay grant
rejected an intermediate ACK, connected core recovery advanced without
snapshot repair, and expired continuation state reached the renewal endpoint
and became non-retryable.

GREEN evidence:

- Backend package tests and race tests passed.
- The focused core recovery command passed 5 files and 130 tests.
- The expanded frontend recovery and Host command passed 8 files and 171
  tests.
- TypeScript typecheck and targeted ESLint passed.
- Backend build, E2E frontend build, and fixture packaging passed. The backend
  build reported only existing unsigned Darwin artifact warnings.
- The combined Chromium recovery, packaged-plugin, and core prompt-history
  run passed all 5 tests.
- The mobile prompt-history run passed its 1 test.
- Specification validation and specification lint passed.
- git diff --check passed.

The checks are scoped to this repair and its affected integration paths. They
do not constitute a full backend audit, a complete security audit, or the full
PR review. The broader durable transport scope decision remains separate.

## Risks

- Grant signatures must cover every consumer branch and every internal call site.
- A hydration callback that awaits its own completion barrier will deadlock.
- Existing hydration helpers can resolve after failure or merge without removing
  stale rows. Neither is sufficient evidence that recovery committed.
- Snapshot responses and buffered events can overlap. Older events must not
  overwrite newer snapshot state, and deleted rows must not reappear.
- Expiry, removal, access revocation, and panel replacement can race.
- Real transport tests must use fresh binaries and bundles.
