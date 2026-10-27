---
status: current
system: platform
requirements:
  - REQ-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-008
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

A notice marked `recovered` can outlive its execution after a later readiness
failure reopens the delivery block. Explicit Resume can reuse the current ready
native execution and restore that notice through the same authenticated
retirement transaction. The current execution, native identity, full submission
inventory, and cursor checks remain mandatory. The notice keeps its historical
execution ID, submission, payload, and unknown outcome; only its phase, revision,
and timestamp change. State-only Retry is unchanged.
Delivery: [Recovered notice resume](../../../plans/resume-delivery-notices/plan.md).

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

Legacy prompt bindings may lack original-process identity and prompt generation.
A successful native load does not supply that missing automatic-retirement proof.
For an empty generic agent-delivery block or a submission-bound unknown-outcome
block without active recovery metadata, the composer offers explicit Resume through the existing authenticated retirement
transaction. Retry remains available for missing-record reconstruction, alongside
Stop. An open block outranks a historical restored snapshot; changing that block
invalidates pending UI results. A cancellation status or idle session state cannot
hide an open delivery block when no error card exists. If a current error card
already owns the bound submission, preserve its recovery choices rather than
replacing it with the fallback banner. Active uncertain/reconnecting protocol
metadata retains precedence over that card. Request-local automatic launch failures
do not displace the open block controls; they do not establish a separate persisted
agent failure. Desktop and phone expose the same guarded actions.

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

## Projected terminal reconciliation

Terminal `error` and `complete` events settle a dispatching journal submission in
the same transaction as the event append. An interrupted return path or restart
cannot downgrade those known outcomes. Nonterminal errors and previously stored
unknown outcomes retain their existing semantics.

Legacy journals can contain interrupted-unknown submissions with retained
terminal sequence evidence already projected as a known SQL outcome. An idle,
authenticated native successor may retire these exact submissions without
changing either store's historical state. Eligibility requires matching session,
incarnation, generation, stream, immutable hash, terminal sequence and fully
projected SQL cursor. SQL admission revalidates the pinned terminal state and
outcome along with the complete active inventory, current native execution and
matching recovery fences. Any independent active record or projection gap
refuses resolution. The same proof permits normal loaded-session reconciliation
and explicit Resume; neither path dispatches an old prompt.

Native recovery proof uses the shared 10-second wait with 200 ms polling for the
captured journal high-water mark to reach canonical projection. It rereads proof
when projection or acknowledgments advance under the same authenticated owner
after verified lag or sequence progress, including on the first proof attempt.
A byte-count change alone is not stream progress. Pruning can decrease bytes while retained
sequence counters advance. Cancellation,
owner changes, inventory changes, and unresolved work retain the admission
fence. This wait does not repeat native Resume or dispatch a prompt.

When terminal settlement committed the outcome but not the final submission
binding, recovery may read the exact retained terminal effect instead. This
fallback requires all final binding fields and settlement/pruning markers to be
absent, matching owner, hash, outcome, stream and sequence, a block-resolved or
completed terminal effect with no newer terminal for that submission/stream,
and a fully projected cursor. Partial or conflicting bindings fail closed.
Lifecycle derives only a recovery view; it does not write terminal references,
settle workflow/queue effects or release a block while authenticating the journal.
The existing locked admission transaction revalidates the same SQL predicate
after journal retirement. Full settlement and retention remain separately gated.

## Prompt readiness after native boot

A native boot event establishes runtime availability, not delivery admission.
Before publishing the ordinary idle session state and draining queued work,
orchestrator checks the current execution's durable delivery capability. An
unresolved inventory creates the existing persistent delivery fence before any
first-send attempt. The same guarded loaded-native reconciliation used for an
existing fence may retire eligible history and resolve it transactionally.
When original-owner evidence is insufficient, retain the fence and expose the
existing explicit Resume controls on desktop and phone. Successful native boot
still records its attempt-owned provider recovery and dismisses only that
captured provider error. This does not resolve the independent delivery fence:
do not drain queued instructions or admit prompts while that fence remains open.

Both pending and explicitly selected V1 delivery repeat this reconciliation at
admission, including when no fence existed at entry. Re-read capability after
successful resolution; a changed or unresolved inventory still refuses dispatch.
No implicit recovery sends an old prompt. Rejected instructions retain their
existing blocked delivery record; an unsent composer draft remains retained while the recovery controls replace
the editor and is sent only by a subsequent explicit user submit after recovery.

## Native async turn ownership

Events without a submission or prompt generation after dispatched work has
finished must not inherit that completed prompt's turn. Persist unowned native
activity without the stale turn binding; use the existing orchestrator readiness
snapshot to bind its completion before queued successors can start. Numbered
completions and explicit turn identities retain their ownership checks.

ACP activity retains the prompt generation captured when its notification was
enqueued, including output drained after the prompt's control response. Buffered
human output cannot become native async activity when the current prompt clears.
A surviving prompt retains its restored turn while recovery adopts the first
live completion's generation. That generation-to-turn association remains valid
for delayed output after a successor dispatch, without allowing its completion
to settle the successor.

The ACP async idle finalizer waits for all observed foreground tools to become
terminal. Silence while a foreground tool is active is not terminal evidence.
Adapter-attested background workloads and nested child tools retain their own
lifecycles. A real prompt boundary cancels the async finalizer. Terminal tool
updates alone do not create a new async turn. No historical events, outcomes,
live sessions, or recovery cursors are rewritten by this correction.

Native ACP chunks without provider message IDs receive an adapter-issued ID
before journal append. Chunks within the async reply share it; idle completion
or a real prompt boundary ends that scope. Replay retains the journaled ID.
Legacy peers use a separate opaque message scope for each preceding dispatch,
without assigning that dispatched turn as the native event owner. Tool updates
reuse adapter-recorded child lineage even when incremental metadata omits it.

When even the legacy dispatch scope is unavailable, the durable event sequence
is the replay-stable message scope. Without grouping evidence, separate chunks
remain separate rows rather than merging unrelated replies.
