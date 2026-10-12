---
status: current
system: platform
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
---

# Session recovery convergence

## Boundaries

Platform owns the contract. Lifecycle authenticates the runtime and assesses the
complete SQL/journal inventory; orchestrator retains admission authority; SQL and
journal retain their own transactions. This extends [restart reconciliation](durable-restart-reconciliation.md)
and [pruned recovery](pruned-delivery-recovery.md), not their dead-owner or gap
guards. The [decision](../../../decisions/2026-10-12-total-session-recovery.md)
separates historical outcome from current admission.

## Disposition precedence

| Evidence | Disposition |
| --- | --- |
| Archived/cancelled, Office/workflow/automation or recipient restriction | Preserve independent policy |
| Exact original owner alive | Reattach; no native replacement or old dispatch |
| Owner unknown, foreign identity/hash, future generation or incomplete inventory | Refuse with missing/conflicting evidence |
| Retained terminal with real SQL receipt | Ordered projection and terminal settlement |
| SQL dispatching, exact abandoned journal unknown | Guarded SQL unknown transition, then restore-only |
| SQL unknown, exact journal unknown | Restore-only and retire exact history |
| SQL unknown, legacy terminal label without any retained terminal reference | Preserve both labels; retire as historical uncertainty after native load and idle owner proof |
| SQL known terminal, prepared/accepted or independent peer live work | Preserve known result or refuse takeover |
| SQL inbox covers replay expiry | Project actual inbox; no fabricated receipt |
| Actually missing history with proven eligible dead source | Existing pinned successor and immutable gap archive |
| Unsupported peer/storage/native evidence | Specific dependency refusal, supported retry |

Unknown enum values refuse. Evaluate every unresolved record before mutation.
A failed label alone does not authorize SQL dispatch-to-unknown conversion.
No terminal label is downgraded. Retired uncertain history is excluded only
with exact authenticated journal evidence; repeat adoption uses the same rule.

## Producer ordering

Capture the admitted child's shutdown-intent flag for dispatch. Publish interruption
before closing its adapter, so a teardown-generated RequestError is uncertain.
Keep the update forwarder and journal writer open through adapter closure and
drain already-queued events before closing the writer. Do not consult mutable replacement status or acquire the teardown
mutex inside settlement. A deterministic provider rejection remains a failure;
a retained terminal wins over a late transport error. Settle with a bounded
cancellation-independent context. Journal close or failed writes leave retained
progress for constructor reconciliation, never authorize redispatch.

## Native acknowledgement and SQL convergence

Require native initialized, exact native ID and current session/incarnation/
generation/stream, authenticated full inventory, and no independent live work.
Validate receipt/projection coverage before retiring history. Retire only exact
eligible SQL unknown records and journal unknown records; retain payload/hash,
state and terminal absence. Re-read owner and inventory after retirement.

Carry authenticated stream and exact submission identities into missing-binding
resolution. Pin session/native generation, block identity and current SQL rows
under existing turn/delivery locks; no derivation from a guessed canonical name.
Absent metadata and an earlier already-resolved notice are presentation states,
not new outcome authority. Restore the current notice and resolve only matching
incident fences atomically. A changed source, independent fence, pending restore
or unprojected receipt refuses resolution. A transaction rollback changes none
of the binding, notice or fence. A retry reuses retired journal proof. Retained inspection has a separate
metadata-only stored-submission request for unretired legacy history; retirement
proof still requires the actual retired marker and preserves the original label.
A retained submission with an existing canonical SQL row can repair its recovery
presentation without a user-message row, including initial prompts and internal
continuations. It preserves the stored dispatch-attempt identity: an initial
prompt uses the original execution ID, independently of its submission ID. The
request marks this as existing-record-only, and the SQL transaction refuses a
missing or changed canonical row instead of inserting one. Reconstruction from
journal-only evidence still requires the exact user-message association. A
proven-dead dispatch records an unknown outcome when its prior outcome is empty.
A matching prepared checkpoint with no candidate is completed in that transaction;
an allocated/launching or unrelated attempt still blocks it. Legacy composer
uncertainty is cleared only for an exactly retired submission. An explicit null
propagates that clearing to clients even when no recovery notice existed.

## Entry points

Startup/runtime replacement and explicit Resume reuse existing restore ownership
and source-death checks. Normal launch/open and direct/queued prompt admission
consume the same committed disposition; read-only inspection cannot grant
admission. An already-loaded idle native successor is reused after verifying the persisted
original prompt binding and original process death. Legacy failed history with
no recovery notice uses the same native Resume transaction only after retained
source evidence, exact SQL history and complete stream projection are rechecked. Recovery supplies
no prompt and does not revive a dispatched queue entry. A distinct new prompt
requires a new immutable submission and normal workflow/queue authority.

## Verification

Generate reachable states using admission, dispatch, terminal, shutdown,
retirement and restore transitions. Exercise all supported SQL/journal state
pairs with complete and absent terminal references and exact/foreign identities.
Independently assert no old dispatch, no invented completion/receipt, no live
owner takeover, and eventual new-prompt admission when proofs are complete.

Interrupt at dispatch-to-call, terminal-to-response, journal retirement-to-SQL
resolution, SQL resolution-to-publication and ACK-to-prune boundaries. Include
failed writes and cancellation, then repeat restart and recovery. Use real bbolt,
SQLite, PostgreSQL conformance and a real isolated process-manager Stop. Existing
hard-crash and gap tests remain relevant. Desktop and phone E2E run only in CI;
no host shutdown, production mutation or local browser testing is authorized.

## Delivery

[Sequential work package](../../../plans/session-recovery-convergence/plan.md).
