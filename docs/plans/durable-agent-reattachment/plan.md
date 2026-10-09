---
created: 2026-09-27
status: implemented
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
system_design:
  - ../../specs/platform/system-design/durable-agent-reattachment.md
legacy_specs: []
---

# Implementation plan: durable reconciliation and surviving-agent reattachment

## Overview

Address the five items promised to @nova28 on PR #3598.
Sources: [review](https://github.com/kdlbs/kandev/pull/3598#issuecomment-5852538250) and
[maintainer response](https://github.com/kdlbs/kandev/pull/3598#issuecomment-5859526728).

Baseline HEAD: `710d67fa3d33a1c95d5c280747cc729438d1ec21` on 2026-09-27.
The worktree also contains the other agent's uncommitted runtime-replacement implementation and documents.
Its package reports completed Linux/local validation, with PostgreSQL and native Windows/macOS gates still outstanding.
That is reported evidence, not a new independent review. Preserve all those edits and outstanding gates.

The user explicitly requested this package and a message directing the existing task agent to implement it.
This handoff authorizes implementation in the shared worktree. It does not authorize commits, pushes, merges, or additional delegation.
Keep this package and the existing dirty work uncommitted.

## Scope

### In scope

- Repeatable state-only reconciliation with the documented initial recovery window.
- Submission-specific automatic block settlement after authoritative terminal evidence.
- Persisted reconnecting/uncertain state and desktop/mobile feedback from real failure paths.
- Detached journal output without memory-channel stalls or missing quiet tails.
- Live reuse as adoption, with cleanup limited to resources owned by the failed attempt.

### Out of scope

- Executor-specific SSH/remote Docker/Sprites redial and long-horizon reachability retry policy.
- Detached MCP waiting, offline budgets, and richer remote-working presentation.
- Automatic resend, native conversation replacement, or treating transport failure as confirmed agentctl death.
- Repeating completed stream repair or local-runtime replacement implementation.

## Source contracts

- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md): existing IDs plus 004.4, 006.4-006.6, and 007.6.
- [Reattachment design](../../specs/platform/system-design/durable-agent-reattachment.md).
- [Parent delivery design](../../specs/platform/system-design/durable-agent-delivery.md).
- [Existing boundary ADR](../../decisions/2026-09-10-durable-harness-session-boundaries.md).

The platform owns durable evidence and session recovery presentation. No new ADR is needed to apply existing no-resend and ownership boundaries.

## Evidence and technical approach

| Review item | Current code evidence | Work order |
| --- | --- | --- |
| One-shot recovery | reconcileDisconnectedSubmission has one three-second query | 02 |
| Block remains after replay | RetrySessionDelivery has no exact-submission settlement; block model lacks submission ID | 03 |
| Notice is not proved persistent | execution failure code reaches an event; composer reads session metadata; E2E seeds it | 04 |
| Detached producer stalls | forwardUpdates blocks on updatesCh after journal commit | 01 |
| Reuse initializes survivor | reuseExisting still calls initializeAgentSession; generic startup failure force-stops | 05 |

Sequence work through the journal wakeup boundary, shared reconciler, exact settlement, persistent presentation, and live adoption.
Use real task repositories and journal stores for state transitions; interface fakes alone missed these gaps.
Add typed persisted submission binding where needed. Register migrations, preserve unknown legacy records, and test interruption between settlement stages.
Avoid a broad clear-recovery action: resolving one transport outcome cannot erase another recovery cause.

## Coordination with runtime replacement

Reuse its runtime lease retirement guards for local execution; do not make remote reattachment depend on a local epoch.
Only confirmed local runtime loss enters the replacement coordinator. A live remote server behind a missing tunnel stays recoverable.
Do not clear local-runtime or native-state blocks while settling a different delivery block.
Record any new overlap before editing shared startup, event, or admission code. The implementer owns integration with existing dirty changes.

## ASCII UI preview

UI-01: Existing session chat recovery region, above the composer. Labels are illustrative localized copy.

```text
Desktop: [Saved conversation]
         Reconnecting to the agent...                  [Stop]
         (after the bounded window)
         Delivery uncertain. [Retry connection]        [Stop]

Phone:   [Saved conversation]
         Delivery uncertain.
         [Retry connection]
         [Stop]
```

After confirmed running reattachment, show normal running controls; do not enable a second prompt as if idle.
After terminal settlement, clear only this delivery notice and restore normal admission through existing guards.
Keep one chat scroll owner, phone safe-area clearance and 44px targets, and compact desktop controls.
No modal or new navigation is needed. Keyboard access and text status must work without color cues.
Session recovery can coexist with the global local-runtime alert. Stop reports unconfirmed cancellation honestly.
This view covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2 and 006.6.

## Tests

Each task contains proposed regression names and exact commands. New names must exist before claiming those commands prove acceptance.
Use fake clocks for the ten-second window and barriers for commit/wakeup, settlement, and stale cleanup schedules.
Test both event-before-successor and delayed-old-event-after-successor orderings.
Keep real SQL boot/reopen/upgrade and PostgreSQL coverage for schema changes; skipped infrastructure is an explicit release gap.

## E2E tests

Tasks 04 and 05 own tests/session/durable-reattachment.spec.ts and its mobile counterpart.
Drive actual backend disconnect/reconciliation and reuse startup paths. Do not seed last_agent_error as the only evidence.
Assert complete ordered transcript, persisted status after reload, state-only retry, usable Stop, no reinitialize, and one terminal effect.
Use controlled fixtures and an already-restored transport; remote redial implementation remains outside this package.

## Work orders

- [x] [Task 01: Make detached durable output independent of the notification queue](task-01-detached-journal.md)
- [x] [Task 02: Unify disconnect and later retry under one reconciler](task-02-repeatable-reconciliation.md)
- [x] [Task 03: Resolve only delivery blocks proved settled by durable evidence](task-03-outcome-settlement.md)
- [x] [Task 04: Persist truthful session recovery and render it on desktop and phone](task-04-persistent-recovery-ui.md)
- [x] [Task 05: Reattach surviving agents without initialization or destructive cleanup](task-05-safe-live-adoption.md)

## Verification results

Implementation checks passed on 2026-09-28:

- All five work orders are implemented and their Results sections record their own regression evidence. Tasks 01 and 02 were already complete before this implementation pass; their original test caveats remain documented.
- Backend lint, focused terminal-settlement and adoption race tests, the complete lifecycle race suite, Task 04 task-service/WebSocket gateway race suites, SQL guard, SQLite store conformance, document catalog validation, full spec lint, and whitespace checks passed.
- Related frontend recovery, rendering, and hydration tests passed 14/14; lint, typecheck, i18n checks, and ratchet passed. Final desktop durable-recovery E2E passed 3/3, mobile passed 2/2, and live-survivor E2E passed 2/2 desktop and 1/1 mobile.
- Public-doc tests passed 62/62 and all 47 published pages validated. Public recovery guidance was updated.
- PostgreSQL conformance was skipped because `KANDEV_TEST_POSTGRES_DSN` is unavailable. Native Windows/macOS containment and process verification remain release gates; Linux results do not cover those platforms.
- No implementation was committed or staged.

## Risks

- A lost wake can hide the final output forever even when later-gap catch-up works.
- Coalesced recovery causes can make a broad unblock release work without authorization.
- Generic error cleanup can destroy a healthy survivor unless ownership follows every error path.
- Runtime replacement and remote reconnect have different death evidence and must remain separate.
- /tmp was full during planning. Use an owned scratch directory on the workspace filesystem for validation if still necessary; do not delete another agent's artifacts.

## Implementation status

Tasks 01-05 are implemented sequentially with TDD, and each work order records its results. All edits remain in the worktree and uncommitted.

Task 01's full process race suite still has the independently reported `TestWorkspaceTracker_StopsWhenGitBroken` goroutine-shutdown timeout; its durable producer, terminal-tail, and detach/reattach regressions pass. PostgreSQL and native Windows/macOS validation remain release gates and are not represented as passing.
