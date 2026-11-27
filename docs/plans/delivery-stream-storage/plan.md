---
created: 2026-10-13
status: implemented
requirements:
  - REQ-PLATFORM-BOUNDED-SESSION-HISTORY-002
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-003
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
system_design:
  - ../../specs/platform/system-design/delivery-stream-storage.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention-payloads.md
legacy_specs: []
---

# Implementation plan: delivery stream storage

## Overview

Fix temporary shell-output amplification and blocked control-event retirement in one
PR. Implement bounded pre-journal shell snapshots first, then safe control retirement,
then verify the combined journal-to-inbox behavior and history/recovery surfaces.
The user explicitly requested one PR; commit, push and PR creation are authorized
after implementation and the checks below. No delegation was requested.

Platform owns this package because operational storage and authenticated replay
determine which transient snapshots and expired transport records may disappear.
The existing [retention requirements](../../specs/platform/requirements/backend-agent-delivery-retention.md)
already require bounded recent receipts. Extend the owning
[storage requirements](../../specs/platform/requirements/pr-watch-and-bounded-storage.md)
with snapshot cadence and ordered preservation, and clarify known-control eligibility.

## Confirmed root causes and evidence

Investigation baseline: commit `b706496d5b2e54e1cebc1e13f38b516e041d8ec1`.
Recheck overlapping files and specifications against the current base before implementation.
Read-only backup comparison found 4,379,549,696 -> 6,446,542,848 bytes between
October 12 at 17:43 UTC and October 13 at 12:32 UTC. Of the 2,066,993,152-byte
increase, 1,527,902,208 bytes were additional free pages and 539,090,944 were
additional allocated pages. Inbox and effects each gained 208,188 records.

Codex ACP session `197f6ccd-4af3-4990-89de-4af577be86ff`, task
`36412e26-2eb5-4f3a-a2b8-b4cc5ee48cfc`, generated 9,954 tool updates above
250,000 bytes between 02:24:20 and 02:30:22 UTC on October 13. Original stored
byte counts totaled 2,676,905,950. All were later pruned; raw historical bodies
are unavailable, so exact byte-identical duplication was not measured.
The final canonical shell result retained 262,144 stdout bytes. Source shows
normalization of a moving 256 KiB tail followed by full-snapshot emission per update.

All 89 streams in the newer backup start with `agent_capabilities`, with zero
retired cursors. A subsequent live metadata read showed 94 streams, again zero
retired cursors. Candidate SQL excludes that type but deletion requires the next
contiguous sequence, permanently stopping retirement at sequence 1.
This violates AC-PLATFORM-DELIVERY-RETENTION-005.2; partial body pruning leaves
ever-growing receipt rows and indexes.

The task plan retains the detailed aggregate evidence and diagnostic bundle path.
Do not put private protocol bodies, command output, credentials or copies of the live
database into source control or regression fixtures.

## Scope

### In scope

- Fixed-window reduction of consecutive, superseding foreground ACP shell snapshots,
  with final-state and boundary preservation before journal sequencing.
- Proved control payload compaction and contiguous expired receipt retirement using
  existing transaction, identity, hash and settlement safeguards.
- Historical sweep/restart, SQLite and real PostgreSQL coverage, combined small
  journal/inbox storage regression, desktop/phone history/recovery smoke coverage.
- Public explanation and one PR containing both fixes and all delivery records.

### Out of scope

- Live database compaction, deployment, restarts or changes to retention settings.
- Transcript expiry, native harness changes, new transports, new wire/schema formats,
  total storage quotas, flags, new workers and prompt/tool replay.
- Reopening unrelated original retention rollout or broad performance audits.

## Technical approach

Use the [stream storage design](../../specs/platform/system-design/delivery-stream-storage.md)
and [snapshot ADR](../../decisions/2026-10-13-superseding-shell-snapshots.md).
The ACP worker owns one timer and immutable pending snapshot. Normalize all deltas,
flush before barriers, and preserve existing lossless downstream backpressure.
Audit direct/locked emitters, prompt barriers and session transitions for overtaking.
No reduction happens inside the committed journal or backend inbox.

For retention, update candidate SQL, locked reload, typed support and preview in
agreement. Admit only the design's explicit proved control types. Keep the stricter
submission, terminal, hash, dependency, window and contiguous-prefix checks.
Existing unsupported events can still pin streams; report this without claiming
total storage is capped. Follow the existing checkpoint and barrier ADRs.

| Boundary | Supported behavior | Evidence and fallback |
| --- | --- | --- |
| Codex/Claude/OpenCode and other ACP dialects | Complete normalized foreground shell snapshots only | Delta/cumulative/final provider-shape tests; ambiguous shapes emit unchanged |
| Native Codex transport | Existing behavior | No admission of unproved native snapshot shapes |
| Legacy ACP delivery | Same final bounded output and order, fewer superseded snapshots | Adapter stream tests without durable metadata |
| Durable delivery v1 | Sequence assigned after reduction, immutable journal thereafter | Combined persistence and replay test; no schema/capability change |
| SQLite/PostgreSQL format 1 | Exact identity/effect proofs, atomic contiguous retirement | Both-engine integration and race coverage; unavailable DSN is a reported gap |
| Unknown control, permission, pending effect, gap | Keep evidence and stop prefix | Mixed-state negative tests, not an allowlist guess |
| Older compatible backend/agentctl | Same full event representation and hashes | Existing replay tests; old producers may still amplify output |

## Tests

- Task 01 introduces `TestShellSnapshotCoalescingStorageBound` and boundary/quiet
  trailing/provider-shape regressions in the ACP package. Before the fix 10,000
  saturated snapshots escape; after it at most 42 snapshots over ten simulated
  seconds and under 16 MiB serialized output escape, with identical final tails.
- Task 02 introduces `TestDeliveryRetentionControlPrefix` using a real bootstrap
  sequence followed by output. Before the fix the expired prefix stays at zero;
  after it only proved contiguous rows retire. Cover each initial supported control,
  compact/hash duplicates, expiry, mixed protected rows, restart and rollback.
- Task 03 introduces `TestShellSnapshotDurableStorage` through ACP updates, real
  journal persistence, task receive/projection and shared retention. Assert marshaled
  bytes, contiguous sequence, final canonical output and duplicate/expired behavior.

## E2E tests

Run existing `tests/session/delivery-retention.spec.ts` on `chromium` and
`tests/session/mobile-delivery-retention.spec.ts` on `mobile-chrome` for
AC-PLATFORM-DELIVERY-RETENTION-006.5. These cover preserved history, shell-output
detail, Stop and recovery behavior. No layout or product-copy change is proposed.
Use isolated fixtures; the developer's live installation is not a test target.

## Work orders

- [x] [Task 01: Bound streamed shell snapshots](task-01-shell-snapshots.md)
- [x] [Task 02: Retire projected control prefixes](task-02-control-retirement.md)
- [x] [Task 03: Prove combined storage and recovery](task-03-storage-recovery.md)

## Verification results

All three work orders are complete; their Results sections record final task-defined checks. Both repairs are ready for one PR. Execution used base 1cb568ac5f7a50ce3183ef31fa9c9f04b7c1d57a after reconciling main; no overlapping repair files changed upstream. No agents were delegated.

Planning validation on October 13 passed:

- Catalog validation: 381 decisions and 1,581 specifications.
- Specification linter tests: 36 passed; full specification lint passed.
- Local work-order audit: all three orders passed ID/AC/design coverage,
  dependency, Markdown link, existing-source-path and verification-block checks.
- Repository PR documentation preflight: actual documentation-only change exempt;
  projected ACP and repository production changes covered by this package.
- Specification/design/plan whitespace checks passed. The package was left unstaged and uncommitted at the initial design
  handoff; the later explicit implementation request authorized all three orders.

## Risks

- A trailing timer or direct emitter can reorder pending output across terminal or
  successor-generation boundaries. Keep one owner and test barriers/cancellation.
- Coalescing raw deltas instead of full normalized snapshots loses final output.
- Treating a generic receipt as universal authority could delete protected recovery
  evidence. The control allowlist and all existing negative proofs remain explicit.
- Independent terminal/submission gaps can remain after the bootstrap blocker is
  removed. No age-based inference or forced settlement is permitted.
- Quiet final state and restart must preserve bounded stdout/stderr and configuration.
- Logical cleanup does not shrink SQLite or existing backups. Physical compaction is
  separately authorized operational work, excluded here.

## Companion implementation records

The original [retention package](../backend-agent-delivery-retention/plan.md),
[legacy payload repair](../legacy-delivery-payload-reclamation/plan.md) and
[throughput package](../database-maintenance-throughput/plan.md) remain historical
records. This package adds a new repair; retain their exact results and exclusions.
The parent retention design links here without changing their completion claims.
