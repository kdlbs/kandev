---
created: 2026-10-11
status: in_progress
requirements:
  - REQ-PLATFORM-PRUNED-DELIVERY-RECOVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
system_design:
  - ../../specs/platform/system-design/pruned-delivery-recovery.md
legacy_specs: []
---

# Implementation plan: Pruned delivery recovery

## Overview

Prevent journal pruning ahead of power-loss-durable receipt, report actual
receipt gaps during startup, and restore a verified saved conversation when
an acknowledged historical tail is unavailable through supported reads. Implement four
sequential work orders in the original session. No delegation or new Kandev
task/session is authorized. E2E runs exclusively in CI.

## Evidence and scope reconciliation

PR [4416](https://github.com/kdlbs/kandev/pull/4416) merged as
`e8c1761cb5b7006b905c0aa995925372ce8664e1`. Its abandoned-dispatch classification
worked in the new case, but cursor coverage failed before native recovery.
The prior package explicitly excluded storage-format changes and did not
permit continuation past absent events. This package defines that additional
contract rather than weakening its missing-evidence guards.

Private evidence for task `84b3497a-4b1c-488a-ba69-56afab796711`, session
`a08fcf40-7a11-4ae9-8ddb-152f7b4e12c2`, is retained outside the repository.
SQL receipt/projection is 5494; the authenticated journal ACK/high-water is
5499 and first-retained is 5500 with no retained event bytes. Replay from zero
or 5494 is expired. The old prompt is journal `interrupted_unknown`, SQL
`dispatching`, with no recovery block. The original native conversation remains
saved. Do not include actual prompt payloads, tokens, or private snapshots in
fixtures. Use synthetic exact-shape evidence.

The initial zero-cursor bootstrap hypothesis is unconfirmed and is not a
required fix. `Executor.applyDeliveryIdentity` selects the canonical generation
stream, and lifecycle forwards that requested identity to the execution. The
presence of a session fallback in another path does not establish its use in
this incident. Startup error propagation still needs to expose a real receipt
gap instead of an ACP connection failure.

Subsequent operational evidence recovered all five original event records from
unused bbolt pages of the pre-repair copy. The events were nonterminal tool_call,
tool_update, and context_window records. With exact baseline/owner guards, the
peer restored those records into a copy, lowered journal ACK to actual SQL
receipt 5494, validated real journal replay, and installed it while the target
was stopped. Normal replay/projected receipt recovered all five, and native
Resume succeeded on the original generation/conversation without a cursor
patch. Verification records the task RUNNING with a distinct new prompt,
receipt/projection 5833, no open block, and the old prompt still unknown with
no terminal evidence or replay. The private verification and helper are in
`/home/jcfs/.kandev/repairs/ci-recovery-20261011T205322Z/`.

This contradicts the earlier claim that the five events were irretrievably
lost. Free-page survival is not guaranteed after page reuse or compaction and
does not provide a supported production replay source. Distinguish this
recovered-event case from a synthetic truly unavailable gap. Automated raw-page
salvage is excluded; do not report loss when complete actual events can replay.
The confirmed defect is journal acknowledgment/pruning ahead of surviving SQL
receipt. SQLite WAL NORMAL power-loss rollback remains a plausible persistence
mechanism, not independently established ordering for this incident.

## Scope

### In scope

- FULL writer durability and receipt/projection-before-ACK enforcement.
- Canonical stream identity controls and typed actual receipt-gap diagnosis.
- Durable gap/checkpoint evidence, exact-dead-owner restoration on one fresh
  generation, and repeated-restart convergence without cursor fabrication.
- Existing automatic restoration and explicit Resume, normal later admission,
  historical loss presentation, localization, desktop/phone CI evidence.

### Out of scope

- Live database/journal edits as implementation or diagnostic shortcuts.
- Recovering missing event contents, replacing the native conversation, prompt
  replay, queue draining, or ordinary rollover guard relaxation.
- Taking over live/unknown owners or discarding retained suffixes.
- Local E2E, browser, Playwright, production restarts, or host power-loss tests.
- Automated scanning of unused bbolt pages or speculative bootstrap cursor
  replacement without a failing production-path regression.

## Technical approach

Implement [the platform design](../../specs/platform/system-design/pruned-delivery-recovery.md)
and [accepted ADR](../../decisions/2026-10-11-honest-delivery-loss-recovery.md).
Task 01 closes the acknowledgment durability boundary in `internal/db` and
delivery scheduling. Task 02 verifies the existing canonical stream binding
and carries real gap errors through readiness. Task 03 reuses `session_restore_attempts` with pinned gap evidence and the
existing successor/native retirement path. It archives immutable
`agent_delivery_history_gaps` in session metadata in the restore commit; no new
SQL table or generation allocator is introduced. Task 04
projects the cause and historical notice into existing chat recovery surfaces
and supplies desktop/phone CI regressions.

| Peer/executor | Identity and capability | Behavior | Verification/fallback |
| --- | --- | --- | --- |
| Local/worktree durable peer supporting gap checkpoints | Authenticated descriptor and exact process birth proof | Restore original native conversation using pinned fresh generation | Real journal + SQLite + lifecycle/orchestrator regressions; CI desktop/phone |
| Supported remote peer after proved source termination | Same capability plus provider-specific owner death proof | Same guarded recovery if evidence is complete | Shared contract tests; no claim of host-only death proof for a remote owner |
| Surviving durable agent | Current authenticated live owner | Reattach only if actual receipt covers replay; otherwise specifically block | Live/unknown controls; no reload or cancellation |
| Older durable peer | No gap-checkpoint capability | Preserve evidence and show unsupported recovery cause | Mixed-version regression; no downgrade |
| Positively identified legacy peer | No unresolved durable SQL history | Existing legacy bootstrap | Legacy controls; unknown transport is not legacy |

## ASCII UI preview

UI-01: Existing task-chat recovery region before restoration. Illustrative
localized copy; range details are collapsed. Reuse the existing inline card.

```text
Desktop
Some output from before the restart is unavailable.
Restoring your saved conversation...
[Resume]  [Details v]

Phone
Some earlier output is unavailable.
Restoring your saved conversation...
[             Resume             ]
[ Details v ]
```

Resume is the existing action, available only when current eligibility permits
and disabled during restoration. Phone targets retain at least 44px hit areas;
card content wraps and scrolls internally within the existing height bound.
No new form, modal, hover-only disclosure, or horizontal table is introduced.

UI-02: After verified restoration on both desktop and phone.

```text
History: Some earlier output is unavailable.
         The previous prompt outcome is uncertain. [Details v]

[                 Ordinary composer                 ]
```

The historical note remains after reload; it does not block the composer.
Details disclose the historical stream and missing interval. Structure maps to
AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.7; spacing and copy are illustrative.
The nearest phone exemplar is `session-recovery-card.tsx`, with stacked actions
and a bounded internal scroll region. Chat remains the primary scroll owner.

## Tests

- .1: `internal/db/sqlite_test.go`: new `TestSQLiteWriterPowerLossDurability`
  verifies FULL on initial and reopened actual writer connections. Lifecycle
  tests verify failed receipt/projection never causes ACK and real persisted
  projection precedes ACK. These do not simulate physical disk failure.
- .2/.3/.8: `delivery_stream_preflight_test.go` and `pruned_delivery_startup_test.go`: existing constructor-selected
  generation cursor, missing/foreign checkpoint, positive legacy controls,
  unknown capability, typed expired-cursor failure, and inbox-covered expiry.
- .3/.4/.5/.6/.8: new `pruned_delivery_recovery_test.go` in journal, repository,
  lifecycle, and orchestrator: exact synthetic 5494/5499 fixture, missing
  metadata/block, original native ID, payload/outcome preservation, unchanged
  old cursor, source liveness controls, all unresolved evidence, retained
  suffix refusal, mixed versions, and each cross-store crash checkpoint.
- .3/.5: a separate fixture with the five exact-shape events actually retained
  proves ordinary replay/projection and same-generation native recovery, with
  no loss notice or invented successor. Never use this fixture as evidence for
  recovery when the event records are genuinely unavailable.
- .6: actual journal reopen and repository reopen followed by authenticated
  successor adoption and one distinct new submission with no old replay.
- .7: recovery service/store/component tests prove typed cause, stale-notice
  fences, historical notice persistence, and restored normal composer state.

## E2E tests

**CI only.** Add `e2e/tests/session/pruned-delivery-recovery.spec.ts` for chromium
and `mobile-pruned-delivery-recovery.spec.ts` for mobile-chrome. Use the owning
session recovery fixtures and reduced synthetic journal evidence; never kill
the developer host. Each scenario covers .2 through .7: automatic restoration,
explicit eligible Resume, specific blocked live/unknown control, reload, another
backend restart, original native/session/workspace identities, preserved old
uncertainty, and a distinct new instruction. Assert zero old prompt dispatches,
unchanged old receipt/projection cursors, for the unavailable-event scenario. Focused inbox coverage controls verify
normal projection uses the real receipts without a loss notice.

## Work orders

- [ ] [Task 01: Make delivery acknowledgment power-loss durable](task-01-durable-ack.md)
- [ ] [Task 02: Diagnose actual receipt gaps during startup](task-02-gap-diagnosis.md)
- [ ] [Task 03: Restore verified conversations across historical loss](task-03-guarded-restoration.md)
- [ ] [Task 04: Present historical loss and verify restart continuity](task-04-loss-presentation.md)

Dependencies are 01 -> 02 -> 03 -> 04. Every work order uses TDD for changed
logic and owns its targeted checks. Implementation is in progress on
`feature/pruned-delivery-recovery`; PostgreSQL and browser validation remain CI gates. Implementation
must use a follow-up branch based on merged main; never push PR 4416's merged
head. Do not create a new conversation or reset this original worktree.

## Verification results

Implementation follows the accepted design using the existing restore checkpoint,
sole-block admission guard, native load, and generation commit. Tests reproduced
NORMAL writer durability, unsealed retired source streams, missing historical
archives, stale receipt acceptance, and generic startup failures before their fixes.
Focused journal race tests pass. Focused lifecycle and orchestrator race tests,
frontend unit tests, web type checking, localization, catalog/spec lint, and public
operations documentation checks have passed. Further focused tests are running
for the latest compatibility and no-gap incident additions.

The no-gap incident has matching SQL/journal receipt coverage, journal unknown
versus SQL dispatching, an empty generic block, and a restored snapshot for an
older resolved instruction (or absent metadata). It requires exact authenticated
unknown classification and retirement, not a pruned-gap successor. Its SQL
regressions reproduced both metadata refusals. Add native-resume parity and
independent active-work/metadata guards before delivery.

Desktop and phone E2E scenarios are written for CI. They cover passive automatic
restoration, a second backend restart, and one distinct new prompt without old
replay. Explicit Resume has focused orchestration and component coverage. Browser,
PostgreSQL, and new-head CI results remain pending; no local E2E or live maintenance
has been run. Interrupted or missing command receipts do not count as passed.

## Risks

- FULL synchronization increases commit latency. Preserve batching and collect
  focused synthetic throughput evidence without starting the full E2E suite.
- Free-page salvage recovered this incident's contents but is not dependable
  retention. Historical loss is shown only when actual event contents remain
  unavailable; recovered events use ordinary projection.
- Candidate generation, journal sealing, native load, retirement, and SQL
  publication are separate crash boundaries requiring exact idempotent proof.
- Older peers and rollback must preserve pending gaps and refuse unsupported
  recovery. Host-only liveness proof cannot authorize a remote takeover.
