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
| SQL completed without any terminal proof, exact abandoned journal unknown | Preserve both labels; apply the guarded unproven-completion assessment below |
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

## Delivery writer failure

The planned [delivery failure extension](durable-delivery-failure.md) adds independent health observation
without changing disposition precedence or native retirement authority.
A failed writer keeps the adapter consumer available for bounded cancellation drain.
Only a real committed terminal, or existing authenticated native recovery proof, can resolve its exact admission fence.
Idle process state and a coarse waiting session state remain insufficient evidence.

## Native acknowledgement and SQL convergence

Require native initialized, exact native ID and current session/incarnation/
generation/stream, authenticated full inventory, and no independent live work.
Validate receipt/projection coverage before retiring history. Retire only exact
eligible SQL unknown records, eligible unproven SQL completions described below,
and their matching journal history. Retain payload/hash, state and terminal absence.
Re-read owner and inventory after retirement.

Carry authenticated stream and exact submission identities into missing-binding
resolution. Pin session/native generation, block identity and current SQL rows
under existing turn/delivery locks; no derivation from a guessed canonical name.
Absent metadata and an earlier already-resolved notice are presentation states,
not new outcome authority. Verified silent-restore history follows the
[repeated native recovery rules](interrupted-prompt-recovery.md#repeated-recovery-after-silent-restoration).
Restore the current notice and resolve only matching
incident fences atomically. A changed source, independent fence, pending restore that still owns its source,
or unprojected receipt refuses resolution. A transaction rollback changes none
of the binding, notice or fence. A retry reuses retired journal proof. Retained inspection has a separate
metadata-only stored-submission request for unretired legacy history; retirement
proof still requires the actual retired marker and preserves the original label.
A recovery block remains as resolved history after settlement. Uniqueness applies
to open blocks by session, incarnation, generation and reason, so a later
same-reason incident can receive its own block. Restart repair does not recreate
a block for a resolved bound submission. A generic block after a resolved
generic fence requires retained current-generation interrupted work created
after that fence resolved. Without that evidence, the old incident remains
closed.
A retained submission with an existing canonical SQL row can repair its recovery
presentation without a user-message row, including initial prompts and internal
continuations. It preserves the stored dispatch-attempt identity: an initial
prompt uses the original execution ID, independently of its submission ID. The
request marks this as existing-record-only, and the SQL transaction refuses a
missing or changed canonical row instead of inserting one. Reconstruction from
journal-only evidence still requires the exact user-message association. A
proven-dead dispatch records an unknown outcome when its prior outcome is empty.
A matching prepared checkpoint with no candidate is completed in that transaction.
An obsolete prepared attempt follows the verified historical-terminal exception
below. Allocated, launching, and all other unrelated attempts still block it. Legacy composer
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

### Historical terminal incidents

This extension covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.9 through
002.11. It follows the existing outcome/admission decision. The repair package is
[automatic restart recovery](../../../plans/automatic-restart-session-recovery/plan.md).

Classify the notice's source before treating its phase as the current recovery
owner. A `settled` notice can describe a different, earlier failed prompt.
It cannot establish the outcome of a later interrupted prompt. Likewise, an
unfinished restore attempt can reference an incident that terminal settlement
already closed. Neither record supplies authority over a later incident.

The repository verifies the historical source under session, turn, and delivery
locks. Require its exact submission, incarnation, generation, payload identity,
canonical terminal binding, completed settlement, and resolved delivery block.
The current interrupted submission must be different. Verify current native
lineage and the complete unresolved inventory independently. An old terminal
label without its terminal evidence fails this classification.

Only a `prepared` restore checkpoint without an allocated candidate can become
obsolete through this rule. Keep its checkpoint and source outcome. Complete it
with stage `terminal`, outcome `blocked`, and reason `source_terminal_settled`.
This outcome records obsolete recovery work, never a successful native restore.
Follow `completeSupersededContextAttemptTx` in `context_continuation_attempt.go`.
Reuse the existing outcome vocabulary and add `SilentRestoreOutcomeBlocked` as
a typed constant. Preserve an existing nonempty failure reason, as that helper does.
Every attempt outside this verified exception remains a guard, including
allocated, launching, changed, and unverifiable work.
Apply the same rule during terminal settlement and recovery, so existing rows
recover after upgrade without a bulk migration or manual database edits.

`restoreAuthenticatedNativeNoticeTx` can pass a proven historical terminal notice
while restoring the current incident. `finishPreparedNativeAttemptTx` handles
only verified obsolete attempts before assessing the current checkpoint.
Keep both changes inside the guarded recovery transaction. Crash recovery
repeats the exact checks, including after journal retirement precedes SQL commit.
Do not broaden the settled-current-notice exception or change historical labels.

Automatic inventory already includes current dispatching and unknown rows.
`repairRestartRecoveryPresentation`, `currentRestartRecoverySnapshot`, and
`loadSilentRestoreSession` must classify historical terminal state before their
phase-based early returns. Reconstruct the later incident through the existing
authenticated record path, then use the bounded restart worker. It can reuse an
idle matching native successor. This work cannot depend on task focus, a browser
request, or a successful manual recovery action.

The first startup after upgrade applies this path to existing persisted states.
Include restart-interrupted sessions already marked `FAILED`, plus stranded
`RUNNING`, `STARTING`, and `WAITING_FOR_INPUT` states. Do not restrict repair to
incidents created by the upgraded binary. Reattach a surviving owner or resume
the saved native conversation after confirmed owner termination. The saved ACP
identity selects the conversation. Successful provider resume and the existing
workspace, ownership, and delivery checks establish readiness. An identifier
alone does not establish that provider state is still available.

An upgrade regression seeds the old persisted state before the corrected backend
starts. It performs no new interrupted turn and no recovery request after startup.
Use both an already-failed session and a ready successor with a retired journal
submission but an open SQL block. Assert automatic recovery before browser focus.
Existing manual Stop, archive, Office, automation, and independent owner rules remain effective.

Startup, runtime replacement, and explicit Resume retain restoration authority.
Retry connection remains state-only: it queries or reconnects retained work and
reads the committed disposition. It never schedules a native restore or launches
a replacement. Automatic recovery already owns eligible dead-source restoration.
Retry connection cannot report success merely because a replacement process is
ready while the delivery block remains open. A blocked result keeps its specific
cause. Recovery never resends the interrupted payload or drains a queued successor.
A later explicit submission repeats normal admission. See the unchanged Retry
contracts in [reattachment](durable-agent-reattachment.md) and
[delivery failure](durable-delivery-failure.md).

### Restored chat presentation

Publish restored metadata and cleared matching blocks only after SQL commits.
Persist one informational `status` message for every successfully restored
interrupted incident. This applies to ordinary automatic restart recovery,
historical-terminal recovery, and explicit native Resume, including criterion
002.6. Reattachment of an uninterrupted live turn and ordinary Resume without
an interrupted incident do not emit this notice.

Follow `writeCeilingSurfaceNote` and `ceilingSurfaceMessageID` in
`internal/orchestrator/ceiling_surface.go`: `uuid.NewSHA1` with
`uuid.NameSpaceOID`, structured metadata, `MessageTypeStatus`, and
`CreateSessionMessageIdempotent`. Hash the prefix
`session-restored-after-interruption:` plus a stable JSON tuple of session ID,
incarnation ID, source harness generation, and source submission ID. These source
fields identify the interrupted incident across successor executions and retries.
Do not include the current execution ID, recovery revision, mutable phase,
resolution timestamp, action name, or a reconstructed block ID. Repeated recovery
through automatic or explicit entry points therefore uses the same message ID.
A later interrupted submission gets a different ID.

Retry publication by that ID, including after a crash between resolution and
message publication. Use the existing lifecycle-only turn convention, without
inventing an agent completion.

Store metadata `kind: session_restored_after_interruption` and its source
identity. Persist this canonical English `content` fallback: "The session was
interrupted. Your conversation is restored. You can continue." The shared
transcript renderer selects translated copy by metadata kind rather than parsing
or translating stored content. The locale key and English tests use that same
canonical string. Keep the old prompt outcome unknown. The notice
has no recovery actions and does not control composer admission. Existing error
entries keep their historical details without reviving controls for this incident.

The shared recovery model selects the ordinary composer from the committed
disposition. Delayed metadata cannot revive an older block or local failure.
An independent open block still owns its actual cause and supported actions.
Keep the normal draft during pending recovery. Pending recovery can disable
submission until proof commits, but it does not require a manual button click.

Desktop uses the existing chat transcript and composer. Phone uses the existing
`task-layout.tsx` chat surface, one transcript scroller, and its safe-area-aware
composer. The notice wraps within that scroller. No dialog, drawer, extra toolbar,
or alternate mobile state machine is required. The primary action is sending a
new instruction through the normal touch control. Preserve keyboard behavior,
drafts, and desktop layout preferences.

Verification combines a real SQLite/PostgreSQL terminal-history fixture with
the restart worker and authenticated bbolt evidence. Include separate stale
notice, stale prepared attempt, combined, and clean control cases. Notice tests
also cover an ordinary interrupted restart without earlier terminal history,
explicit Resume of a recovered notice, and repeated restoration of one incident.
Test startup without an open browser, a generic block without current notice, repeated
restart, interruption between store commits, late publication, and later prompt
admission. A mixed inventory contains eligible history plus independent live
or unverifiable work. Desktop and phone E2E use the real recovery path and assert
one notice, a usable composer, no required recovery buttons, stable native
identity, preserved draft, and no old prompt dispatch.

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
hard-crash and gap tests remain relevant. Desktop and phone E2E use isolated
fixtures. They must not stop or mutate a developer's runtime or database.

## Delivery

[Sequential work package](../../../plans/session-recovery-convergence/plan.md).

## Foreground handoff and unproven SQL completion

This extension implements AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.7 and
002.8. Delivery remains in the
[foreground handoff fix package](../../../plans/foreground-handoff-durable-recovery/plan.md).
The existing [outcome/admission decision](../../../decisions/2026-10-12-total-session-recovery.md)
continues to apply. Historical labels remain immutable, while terminal proof
determines whether an outcome is known.

### Producer boundary

`handlePromptHandoffEvent` releases the matching lifecycle waiter with
`streams.EventTypeForegroundIdle`. `SessionManager.waitForPromptDone` returns
that reason without a terminal delivery event. This release remains valid.
It does not authorize `finishPromptExecutorDispatch` to call
`agentDeliverySubmissionRuntime.markCompleted` for that result.

Keep the original SQL submission dispatching after a successful handoff return.
Do not add a recovery block while the original delivery remains live.
Do not synthesize a terminal event or change foreground prompt admission.
A later real terminal follows canonical projection and settles its exact submission.
If the terminal preceded the handoff return, retain its authoritative outcome.
Legacy delivery, dispatch-only acknowledgement, deterministic rejection, and
existing Claude successor admission retain their established behavior.

### Historical completion assessment

For native recovery, inspect SQL metadata for each authenticated interrupted-unknown
journal submission, including SQL rows omitted from the active-state query.
A SQL row labeled `completed` can qualify as unproven history only when all
these conditions hold:

- The SQL outcome is `completed`, and the payload hash is nonempty.
- Session, incarnation, generation, submission, stream, and immutable hash match
  the authenticated journal and persisted source binding.
- Every terminal binding is empty: stream, sequence, and turn.
- Full-settlement and payload-pruning markers are absent.
- Neither the SQL inbox nor the effect ledger contains terminal evidence for
  that exact submission. Check all streams, so conflicting evidence cannot hide
  outside the expected stream.
- The journal submission is interrupted-unknown with no terminal reference or
  retained terminal. Its complete inventory and stream descriptor remain valid.
- Historical source termination, current idle native identity, and complete
  received/projected cursor coverage pass the existing recovery guards.

These conditions are conjunctive. Validate every unresolved SQL and journal
record before retiring any record. A matching candidate cannot override an
independent live, conflicting, or unverifiable submission. Unknown historical
process ownership remains blocked. A currently dispatching journal retains the
ordinary live-owner disposition until existing recovery proves abandonment.

Retained terminal evidence takes precedence over this assessment. Use the
existing projected-terminal path when its proof is complete. Missing terminal
bindings alone never authorize recovery. Failed and cancelled SQL labels do not
enter this new class because deterministic rejection can establish those outcomes.
Partial bindings and pruned history retain their existing blocked or gap paths.

### Retirement and admission

Add a read-only repository verifier for the SQL predicate. Lifecycle uses it
without running settlement or changing historical state. Carry an explicit
`UnprovenCompletion` classification in `NativeResumeSubmissionEvidence`.
The default value preserves existing unknown and proven-terminal behavior.
Do not derive a fictitious terminal binding or change the row to unknown.

Use the same assessment in SQL row discovery, native inventory validation,
retirement selection, and proof creation. After native restoration, retire only
the exact journal submission through the existing release path. Preserve its
unknown label and all SQL state, outcome, payload, and terminal fields.
Re-read the owner, inventory, and retirement result before constructing proof.
`hasProjectedTerminalRetirement` must not classify this history as proven terminal.
Its terminal-only path cannot substitute for original-owner termination proof.

`verifyNativeResumeInventoryTx` must count this class separately from proven
terminals. Under the existing session/turn/delivery admission locks, repeat the
SQL predicate against the exact proof identities before resolving a matching fence.
Carry and pin each unproven submission's historical stream cursor, then re-read
its session, incarnation, generation, stream, received, projected, retired, and
peer-reported high-water values under those locks. A receipt, projection, or
high-water change invalidates the proof. A fresh proof is required after
complete projection and receipt of the authenticated watermark.
Use the PostgreSQL advisory-lock discipline already required for this transaction.
A terminal that arrives after assessment invalidates the unproven classification.
Retry through ordinary terminal reconciliation instead of clearing a stale fence.

A crash after journal retirement but before SQL resolution retains the exact
retired marker. The next attempt revalidates it and completes the same transaction.
Automatic already-loaded recovery and explicit Resume use the same proof.
Ordinary prompt admission consumes their committed disposition and repeats all
independent gates. Recovery does not dispatch an instruction or drain the queue.
Repeated adoption must accept retired unproven history without reopening its fence.

No table or column change, public API, journal protocol, configuration, or
runtime flag changes are required. One replayable partial inbox index supports
the terminal-absence lookup. Existing recovery revision and publication
mechanisms restore the ordinary composer on desktop and phone. Existing blocked
results retain their cause when evidence remains unavailable.

### Verification boundary

Use real SQLite/bbolt fixtures for the historical mismatch, exact retirement,
SQL resolution, retry after interruption, and later distinct prompt admission.
Add PostgreSQL tests for the SQL predicate and transaction races.
Negative cases cover terminal arrival, partial bindings, pruning, foreign owners,
hash conflicts, projection gaps, and mixed eligible/live inventories.
The existing desktop and phone recovery suites cover the same resulting controls.
Their deterministic fixture must exercise the backend proof path, not replace it
with a successful mocked response.

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
