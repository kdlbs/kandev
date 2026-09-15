---
created: 2026-09-15
status: implemented
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
legacy_specs: []
---

# Implementation plan: repair durable agent streaming

## Overview

Repair the confirmed streaming and storage defects in PR #3598 before release.
This package follows the completed session and adoption packages. Their prior results remain historical evidence.
The source baseline is `62851c51fa2d347bbc0d162634c25c541c441d21` on 2026-09-15.
The change after reviewed head `98ed9c555` only adjusts E2E assertions; the reviewed production paths remain unchanged.
The checkout was clean before planning. Recheck HEAD and concurrent edits before implementation.

The user authorized implementation after the design checkpoint. Do not commit these planning files.
Production code, permanent tests, public recovery documentation, and validation artifacts are now present in the working tree.

## Scope

### In scope

- Confirmed Opus findings 1-4, safe pruning from finding 5, and overload recovery from finding 8.
- Complete the existing bounded journal and submission-retention design.
- Preserve canonical output, terminal order, and visible desktop/mobile recovery.

### Out of scope

- Splitting PR #3598, new feature flags, native-session redesign, or automatic prompt resend.
- The unsupported claim that accepted submissions are unreachable.
- Broad routingerr refactoring, speculative lifecycle cleanup, or unconfirmed performance multipliers.
- Repeating the completed terminal-evidence, adoption high-water, historical-summary, capability-cache, and idle-status fixes.

## Source contracts and evidence

Use the [delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md),
[delivery design](../../specs/platform/system-design/durable-agent-delivery.md), and
[boundary ADR](../../decisions/2026-09-10-durable-harness-session-boundaries.md).
Existing requirement IDs remain unchanged. These repairs implement the recorded contract.
The design remains draft; this package does not claim release readiness or promote its status.

| Evidence | Consequence | Work order |
| --- | --- | --- |
| streams.go: projectCanonicalAgentEvent lacks durable identity guard | Legacy canonical events query absent inbox rows | 01 |
| durable_delivery.go: loadAgentStreamReplay calls Replay once | Reconnect can skip retained suffix after 1000 events | 02 |
| streams.go: ACK precedes notification; SQL updates full content per event | Lost notification and excessive per-chunk work | 03 |
| process/manager.go: persistDeliveryEvent calls journal.Append per event | Durable commit transaction per normalized event | 04 |
| journal.go: PutSubmission omits quota accounting; shared reserve check | Payload growth and unusable terminal reserve | 05 |
| journal.go: deletion mutates a cursor traversal | Pruning may skip entries after node materialization | 05 |
| No submission-count rollover enforcement | Historical payloads grow beyond the designed bound | 06 |
| agentctl/agent.go: fixed item queue closes at overflow | Legacy cannot replay missing output; failures need visible recovery | 07 |

## Technical approach

Keep journal commit before publication and SQL projection before canonical notification.
Retain projection-before-ACK during this repair; moving ACK to inbox-only would require an independent durable projector recovery proof.
Batch transaction work and cumulative ACK traffic while preserving every sequence and semantic barrier.
Maintain one authoritative stream worker and cancel its resources on owner replacement.
Use bounded recovery errors and existing session guards, not raw provider text or new admission locks.

No SQL table is assumed necessary. If a checkpoint or schema change is required, update the registered owner and both SQL dialects.
Journal accounting and rollover changes require explicit version compatibility and restart-safe migration.
Never delete retained journals to make an upgrade pass. An incompatible old binary must fail closed.

## ASCII UI preview

UI-01: Existing session chat, delivery recovery status. Labels are illustrative and must use localization.

```text
Desktop: [Committed conversation output]
         Delivery interrupted. Outcome uncertain.
         [Retry connection] [Stop]

Phone:   [Committed conversation output]
         Delivery interrupted.
         Outcome uncertain.
         [Retry connection]
         [Stop]
```

The same region shows Reconnecting while bounded recovery runs. On success, normal chat returns without duplicate text.
An ACK-only retry after successful projection does not hide output or claim the prompt failed.
Use the session recovery surfaces as exemplars; do not route a streaming error through bootstrap-only predicates.
The existing chat owns scrolling. Do not add a modal, nested scroll area, or fixed overlay.
Phone controls retain 44px touch targets and safe-area clearance; desktop controls retain their existing compact size.
Stop remains available during retry. No generic Resume action may resend the uncertain submission.
Map this view to AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2 and 006.3.

## Tests

Each work order maps acceptance IDs to named regressions and exact commands.
Use real SQL repositories where interface fakes concealed the defect. Use barriers and injected clocks for concurrency tests.
Count journal transactions, canonical updates, ACK requests, queue bytes, and completion effects.
The burst-count gates are synthetic acceptance bounds, not claimed production latency improvements.
Record benchmark environment, input bytes, event count, elapsed time, and allocations.
Do not run product tests during planning.

## E2E tests

Task 07 owns Chromium and mobile-chrome coverage for legacy rendering, multipage recovery, overload, visible uncertainty, Stop, and retry.
Use the real backend plus controlled test fixtures. Assert ordered markers, one terminal effect, and no automatic resend.
Existing survival/pause/stream-isolation specs remain regression inputs; the new specs must exercise actual transport failure boundaries.

## Work orders

- [x] [Task 01: Restore legacy streaming through the real repository](task-01-legacy-streaming.md)
- [x] [Task 02: Replay every retained page before joining live delivery](task-02-complete-replay.md)
- [x] [Task 03: Batch canonical projection and retry cumulative acknowledgments](task-03-projection-and-acks.md)
- [x] [Task 04: Commit bounded journal batches before publication](task-04-journal-batching.md)
- [x] [Task 05: Enforce journal accounting and terminal reserve](task-05-quota-and-pruning.md)
- [x] [Task 06: Bound submission retention with safe idle rollover](task-06-submission-rollover.md)
- [x] [Task 07: Surface stream failures and preserve Stop on desktop and phone](task-07-overload-recovery.md)

## Verification results

Planning and implementation validation passed on 2026-09-15:

- `python3 scripts/list-docs.py validate`: 268 decisions and 907 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Package audit: eight files, seven sequential work orders, valid links, source paths, acceptance IDs, and Go package commands.
- Backend race suites, SQL guard, store conformance, journal and canonical delivery benchmarks, frontend tests, lint, typecheck, i18n checks, and desktop/mobile E2E passed. See each work order for commands and scope.
- The plan remains uncommitted by design. The implementation commit stages only production code, regression tests, and the public recovery documentation.

## Risks

- ACK retries and batching must not cross ownership, turn, or terminal barriers.
- Rollover spans journal and SQL; each interruption point needs restart evidence.
- Bounded legacy streaming cannot guarantee replay. Overload must preserve truthful uncertainty and control responsiveness.
- Storage performance varies by filesystem and executor. Report measurements before making speed claims.
- This release has no operator toggle. Finish the transport milestone before publishing it.

## Handoff

Tasks 01-07 are implemented and validated sequentially with TDD.
The filtered benchmark commands passed without matching benchmark output in this checkout; they remain recorded as command-level validation, while the journal benchmark produced measurements.
Preserve user changes and keep this package uncommitted.
