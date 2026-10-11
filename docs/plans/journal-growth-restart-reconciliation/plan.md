---
created: 2026-10-11
status: done
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
legacy_specs: []
---

# Implementation plan: journal growth and restart reconciliation

## Overview

Prevent journal growth from crashing shared agentctl and make backend restart or confirmed runtime replacement automatically reconcile provable delivery inconsistencies. Restore the original conversation without replaying the interrupted instruction, then persist and publish usable session state. Implement four work orders sequentially: mapping safety, abandoned journal dispatch classification, automatic reconciliation, and coherent recovery commit with end-to-end evidence.

This package extends the existing [journal shutdown repair](../agentctl-journal-shutdown-recovery/plan.md) and [interrupted-prompt recovery](../durable-agent-session-reconciliation/plan.md). Earlier passing results cover their original scope, not this mmap race or restart mismatch. Their completed work orders remain historical; this package owns the additional repair. Implementation is authorized in the original session. Delivery continues through local checks, PR checks, and merge.

## Evidence and root cause

- On 2026-10-11 at 13:58:10 Europe/Lisbon, shared agentctl exited with SIGSEGV in bbolt v1.3.11 `DB.Info`, called by `journal.reusableJournalPageBytes` during `AppendBatch` disk reservation. The runtime coordinator replaced the shared process while the backend stayed running.
- `DB.Info` reads `db.data` without the mmap lock. Concurrent bbolt writes replace that mapping. The journal lifetime lock protects Close/Compact but permits these concurrent writes.
- An isolated temporary database reproduced a data race between `DB.Info` and `munmap`, followed by the same SIGSEGV. A second probe captured page size before concurrent access and used synchronized Stats; it completed under `-race` without a report. This is root-cause evidence, not a production regression test.
- The crash bypassed dispatch-error handling. Retained journal submissions stayed `dispatching` while SQL became `interrupted_unknown`. Exact native-resume evidence validation correctly rejected the mismatch; replacement initialization did not normalize abandoned journal dispatches.
- Native Resume subsequently returned success and resolved SQL blocks, but six sessions retained `agent_delivery_recovery.phase=uncertain`. Browser recovery controls read that structured snapshot and continued showing interruption. A separate idle session kept a client pinned to the retired runtime lease, causing repeated reclaim warnings.
- User-authorized repair backed up owner journals and exact SQL rows, released only blocked idle instances, normalized exact abandoned submissions, and resumed ten existing native conversations. Six proved resumes needed revisioned restored metadata. No historical uncertainty was changed into completion and no interrupted prompt was resent.
- Host evidence is retained in a private host repair directory (not committed); isolated reproduction is copied to ignored `.kandev/diagnostics/mmap-growth-20261011/`. Credentials are excluded.

## Scope

### In scope

- Mapping-safe capacity accounting, including every open/reopen path and reserve correctness.
- Owner-scoped classification of dispatches abandoned before a new process manager owns the journal.
- Automatic reconciliation on backend startup and confirmed local runtime replacement, including missing/stale recovery presentation and dead runtime handles.
- Exact identity/hash comparison across every unresolved SQL record, preserved uncertainty, authenticated replay, and existing restore-only checkpoints.
- Atomic restored snapshot/block settlement and publication for automatic and explicit native recovery.
- Desktop and phone evidence for normal composer availability, reload, a distinct follow-up instruction, no replay, and independent live sessions.

### Out of scope

- Replacing native conversations, replaying historical instructions, automatically draining paused queues, inventing terminal outcomes, or bypassing Office/automation admission.
- Broad database cleanup, new recovery controls, API endpoints, credentials, feature flags, or storage-format changes.
- Recovering erased native state or ambiguous/foreign ownership by guessing.

## Technical approach

`journal.Journal` captures immutable page-size metadata before publishing a database handle. `disk_capacity.go` uses it with synchronized Stats. Compact refreshes it for both successful and failed-swap reopen paths. Avoid nested transactions under reservation locks.

`process.NewManager` classifies exact preexisting abandoned dispatches before any harness starts or prompt admission is exposed. Add an owner-scoped journal operation; diagnostic/read-only Open remains observational. Classification preserves payload/hash and unknown outcome and never retires or dispatches work.

Extend the existing orchestrator restart worker and repository candidate inventory to reconcile records lacking current metadata. Use current owner context, native generation, runtime birth identity/epoch, existing lifecycle/session admission locks, authenticated journal evidence, and CAS transactions. Trigger the same bounded worker after confirmed shared-runtime replacement. A retired lease initiates identity reconciliation; it does not prove termination. Recheck original-process death before restore and clean only the exact obsolete execution handle.

Reuse current reconstruction and ordered terminal-projection paths. Validate all unresolved records before uncertain retirement. Journal writes and SQL writes are separate crash checkpoints: pin identity, reread both stores, and converge from recorded progress. Preserve independent recovery blocks. Use the existing Auto-run pause and restore-only launch with no initial prompt.

Both explicit Resume and automatic restore commit a newer restored snapshot and matching block resolution, then publish complete session metadata and the cleared block array. Preserve historical recovery identity so existing frontend revision/error fences remain effective. Account for already-resolved blocks and prior journal retirement idempotently. A broad `recovery_resolved_at` timestamp alone is never sufficient evidence.

| Executor/peer | Identity and transport | Behavior | Evidence / unsupported fallback |
| --- | --- | --- | --- |
| Native/worktree durable v1 | Authenticated current runtime epoch and process birth identity | Reconcile after backend restart or confirmed local replacement; restore native identity only after source death | Real bbolt + orchestrator tests and isolated backend browser tests |
| Surviving remote durable v1 (Docker, SSH, Kubernetes, Sprites) | Authenticated retained owner and provider identity | Reattach/replay live work; no local replacement or native reload of live work | Lifecycle fakes for owner/generation fences; current provider suites remain authoritative |
| Reopened remote durable v1 | Authenticated new owner plus supported provider death evidence | Owner journal classification; restore only when evidence is complete | Missing provider death evidence remains specifically blocked |
| Legacy or unavailable/corrupt journal | Missing capability or unreadable evidence | Preserve compatibility for legacy; unresolved durable evidence stays blocked | No downgrade, database recreation, or guessed success |

## Tests

- AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.3/001.4: `disk_capacity_test.go`, new `TestCapacityAccountingDuringMmapGrowth`; include actual mmap growth, concurrent reserve/pressure reads, close/compaction, page reuse, and reserve thresholds.
- AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1/003.2 and 006.12 plus AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3: new `abandoned_dispatch_test.go` in journal/process; reopen retained dispatching state, preserve hash/payload, refuse resend, and keep live/foreign/corrupt/mixed evidence blocked.
- AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20 through 006.24 plus AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1 through 001.5: `interrupted_session_restart_test.go`, `silent_restart_recovery_boundaries_test.go`, SQLite `silent_restore_test.go`, and retired-client lifecycle tests. Include inconsistent stores, missing presentation metadata, stale dead handles, live siblings, and crash-after-journal-write retries.
- AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2 and 006.9/006.12/006.24 plus AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4: `session_recovery_integration_test.go` and repository recovery tests. Resume succeeds only after retained retirement; snapshot revision and block settlement commit together; unrelated/newer recovery errors survive.

## E2E tests

Extend `agent-survival-restart.spec.ts` and `mobile-agent-survival-restart.spec.ts` using the existing silent-restart helper. Seed the actual SQL/journal mismatch, restart the isolated backend, and require the same conversation, normal composer, no uncertain banner after reload, no interrupted-prompt dispatch, and a distinct new instruction accepted normally. Preserve an independently live sibling. Use `chromium` and `mobile-chrome` projects.

Extend `durable-stream-recovery.spec.ts` and `mobile-durable-stream-recovery.spec.ts` to cover explicit native Resume after verified termination and stale uncertain metadata. Existing layout and localized controls remain unchanged; do not mask the bug by deleting browser state in the test. Browser suites run sequentially with the guarded runner.

## Work orders

- [x] [Task 01: Make capacity accounting safe during mmap growth](task-01-mapping-safe-capacity.md)
- [x] [Task 02: Classify abandoned owner journal dispatches](task-02-abandoned-dispatches.md)
- [x] [Task 03: Reconcile restart inconsistencies automatically](task-03-automatic-reconciliation.md)
- [x] [Task 04: Commit and publish restored recovery state](task-04-coherent-recovery-state.md)

## Verification results

Design validation passed: specification catalog and lint, linked work-order documentation coverage, and diff checks. Fresh final recovery regressions passed under `-race -trimpath -tags fts5` across lifecycle, journal, process manager, backend adapter, orchestrator, and SQLite repositories, including the PostgreSQL recovery cases. Backend lint passed. Desktop and phone automatic runtime-crash recovery passed, preserving native identity, admitting a distinct new prompt without replay, and remaining usable through a subsequent backend restart. Desktop and phone explicit native Resume, reload, and normal follow-up passed; the concurrent workspace-reader admission regression now passes with ordinary admission fences preserved. Live-stream reattachment and existing uncertain-delivery controls passed. Earlier full journal/process/lifecycle/orchestrator race suites passed; the separate million-row retention storage load test timed out after 15 minutes and is not counted as passed.

Operational repair verified ten existing native resumes and working status endpoints. The later read-only live audit found zero unarchived open recovery blocks and zero uncertain/reconnecting snapshots. The originally reported waiting session remains usable; a separately cancelled session remains cancelled. Unknown outcomes and histories are preserved. Implementation and local validation are complete; commit, PR checks, review remediation, and authorized merge remain delivery gates.

## Risks

- Journal and SQL cannot share a transaction; every interruption point needs retained proof and idempotent CAS.
- A retired socket, missing execution row, or no active dispatch marker cannot independently prove external process termination.
- Candidate discovery must remain bounded and must not allow one matching row to hide another conflict.
- Page-size cache refresh on failed compaction reopen and restored metadata revisions can be overlooked.
- Automatic restore and explicit recovery must share locks without duplicate native loads or queued dispatch.
- Current public recovery guidance is reviewed at implementation; this design-only package does not publish operational promises.
