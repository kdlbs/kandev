---
status: current
system: platform
created: 2026-10-19
requirements:
  - REQ-PLATFORM-INCREMENTAL-SHELL-001
---

# Incremental shell delivery

## Boundary and existing contracts

Platform owns the durable transport and storage representation. This design
extends [stream storage](delivery-stream-storage.md), retaining its pre-admission
250 ms coalescer and all ordering barriers. It supplements
[durable processing](durable-agent-stream-processing.md) and
[delivery retention](backend-agent-delivery-retention.md).
ACP interpretation stays in `adapter/transport/acp/shell_output.go` under ADR-0036;
provider detection never moves to SQL or React. Browser delivery continues to use
ADR-0042's projected summary and authorized whole-tail output endpoint.

No current design is silently superseded. The existing snapshot path remains the
version-1 path. The encoding and canonical storage below define version-2
behavior, recorded by the [decision record](../../../decisions/2026-10-19-incremental-shell-delivery.md).

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-INCREMENTAL-SHELL-001 | Encoding; Byte bounds; Projection; Recovery; Compatibility; Verification |

## Confirmed amplification path

1. `NormalizeShellToolUpdate` interprets every ACP source frame; deltas append,
   cumulative content replaces and authoritative final values can replace either
   stream. `boundShellOutput` keeps a UTF-8-safe 256 KiB tail per field.
   `RawTerminalOutput` is an internal echo-comparison baseline with `json:"-"`.
2. `adapter_tools.go:convertToolCallResultUpdate` emits the accumulated normalized
   payload and converted `ToolCallContents`. Cumulative text can therefore appear
   both in the normalized result and in a content carrier.
3. `shell_snapshot.go:sendShellSnapshot` coalesces consecutive eligible snapshots
   at 250 ms. A once-per-second producer admits every full snapshot. Tool switches
   and unknown metadata are barriers, so coalescing is not a lifetime byte bound.
4. `process/delivery_batch.go:deliveryEventWriter.persist` marshals the event;
   process journal commit stores accepted events before publication. Batching
   improves transactions, not repeated payload bytes.
5. `lifecycle/durable_delivery_stream.go:durableAgentEvent` serializes the event
   into the SQL receipt. `sqlite/agent_delivery.go` inserts its full payload into
   `agent_delivery_inbox`. Hashes identify duplicates of the same event; different
   sequence numbers with different moving tails remain distinct writes.
6. Tool callbacks in `orchestrator/event_handlers_streaming.go` call
   `UpdateToolCallMessage`. `sqlite/message_payload.go` externalizes shell results
   above 4 KiB into gzip/digest-backed `task_message_payloads`. Different full tails
   generally have different digests; compression neither removes inbox writes
   nor guarantees small writes for incompressible output. The current canonical
   delivery projector primarily handles message/reasoning text; adding a patch
   to it without a tool-message persistence integration would be incomplete.
7. Payload grace defaults to one hour. Later pruning frees logical bytes for
   reuse but cannot undo journal, inbox, blob or WAL writes already performed.

The parent investigation is historical correlation, not proof of individual
monitor-to-event mapping; its pruned inbox bodies cannot be reconstructed here.
The isolated real-adapter reproduction is recorded in the plan.

## Encoding

### Typed contract

Introduce a version-2 normalized event variant `shell_output_update`, separate
from ordinary `tool_update`, with `encoding_version: 1` inside the version-2
transport. Keep session/incarnation/harness/stream/sequence/submission identities
in the existing authenticated envelope. The body carries tool ID, parent,
prompt generation, output revision, base revision, tool status presence/value, and result digest. Identity
and command metadata are established by the tool's initial event; changes remain
ordered ordinary events and cannot reuse a predecessor's output epoch.

A checkpoint contains complete bounded normalized stdout/stderr, flags and nullable
exit code. A patch contains independent per-field operations:

- `keep`: preserve an omitted field.
- `shift_append`: remove a stated UTF-8-aligned prefix of the prior retained field
  and append text. Validate the dropped byte count against the bounded base and carry its digest.
- `replace`: authoritative complete replacement, including explicit empty text.

Encode inserted/checkpoint text bytes as base64, then validate decoded UTF-8.
Two saturated fields require about 683 KiB of base64 rather than up to 3 MiB of
JSON-escaped text. Normal checkpoints remain below 1 MiB within the
[shared event budget](bounded-tool-delivery.md#capacity-contract). Count encoded bytes in all budgets. Oversized unrelated metadata retains
the existing typed size failure; it is not truncated to make a checkpoint fit.

Compute each field's SHA-256 directly over its UTF-8 bytes, then SHA-256 the JSON
record containing those two fixed-size hashes, nullable exit and all truncation
flags. This binds exact results without serializing the full tail for every
base/result digest.

Carry exact resultant per-field truncation flags, aggregate truncation and exit
presence/value. `null` exit is unknown; absence preserves it. Verify result byte
lengths, digest and the 256 KiB bound before commit. No cross-stream deduplication
or provider raw-output parsing occurs here.

### Producer boundary

Keep full accumulation and echo removal unchanged in ACP. Encode after the
existing coalescer has selected the logical view, before the journal writer
marshals/accepts it. Version 1 continues to emit ordinary full snapshots.
The ACP adapter attaches an internal, nonserialized eligibility marker using
`shellSnapshotEligible` and its terminal counterpart. The process writer must not
reconstruct raw-carrier eligibility from normalized text. For version 2, handle
only grounded normalized foreground shell shapes. Recognized
output carriers already represented by the patch must be removed from the new
body, so cumulative content is not serialized a second time. Preserve unrelated
content and unknown metadata through the ordinary ordered path. Do not strip
terminal content needed by foreground-activity or background-work consumers;
version-2 terminal events carry an explicit normalized result/substantive-output
signal consumed at the same semantic boundary.

Derive a lossless patch from the last *accepted* normalized tail to the selected
new tail. For each field, prefer append, or maximal previous-suffix/new-prefix
overlap for a saturated sliding tail; use a linear-time overlap algorithm rather
than quadratic suffix trials. If no compact overlap exists, replace the field.
An overlap is a representation optimization, never a claim about upstream delta
provenance. Repeated text, CRs, echo stripping or a cumulative reset must reconstruct
exactly the normalizer's selected value. Final authoritative output uses replacement
semantics even if earlier live text differs.

Emit a first checkpoint, then a checkpoint after 512 KiB of changed text or 512
changed output events, whichever comes first. Count per-tool accepted changes,
not elapsed time. Tool interleaving and barriers flush pending views but do not
reset checkpoint counters or force full checkpoints. Flush terminal output as an
immediate final checkpoint before completion; do not wait for a cadence timer.
Output-only no-ops do not receive a patch/revision or advance a checkpoint clock.
Non-output state changes remain ordered even if the text is identical.

Serialize encoder preparation/commit with the existing writer. Advance baselines
only after successful journal commit. A failed batch leaves the old baseline;
retry never derives a patch against an unaccepted view. Do not derive a delta against an unaccepted pending view.
Use immutable event snapshots; a mutable normalizer pointer cannot be the baseline.
Once a tool has a compact baseline, unknown-shape updates with normalized output
also pass through the compact transactional projector; their unrelated rich
contents remain intact and ordered. Unknown first updates without a grounded
baseline retain the ordinary full-snapshot path. Unsupported rich-content cost
is reported separately from the supported output bound. A
metadata-only barrier does not reseed a baseline or emit a full checkpoint.
An ordinary normalized replacement outside compact eligibility clears that
tool's accepted encoder baseline; a later compact view starts a fresh epoch.
Metadata-only barriers that keep a complete compact view retain their baseline.
State is bounded per active tool, reclaimed at terminal/session cleanup. Baseline
loss starts a new explicit checkpoint epoch before the next patch.

## Projection

### Compact inbox and canonical tail

Receive and hash the compact event itself. Never expand it and marshal the full
tail back into `agent_delivery_inbox`. Introduce a dedicated SQL tool-output
projector under the existing session/stream barriers. In one transaction:

1. Revalidate owner, exact event/effect, tool identity and contiguous cursor.
2. Load the current tool-output revision and bounded affected chunks, validate
   the base, and apply checkpoint or patch.
3. Update the tool message's projected summary/status and non-empty title/content,
   canonical output, exact
   completed generic effect and projected cursor atomically.
4. Publish the existing lightweight message notification after commit.

Foreground shell start events create their canonical tool card in this same
durable projection pipeline before advancing the cursor. An asynchronous event
subscriber only publishes the committed card; compact output does not depend on
subscriber timing or re-running a callback after a crash. A delivery identity
change requires a fresh self-contained checkpoint with base revision zero;
incremental patches cannot cross that boundary. Replace the old head and chunks
atomically when admitting that checkpoint.

Add `task_message_shell_output` (one head per message) and
`task_message_shell_output_chunks` (message/stream/chunk ordinal primary key).
The head records output epoch/revision, digest, lengths, truncation, exit,
logical prefix offsets and last applied transport sequence. Store canonical text
in chunks of at most 4 KiB; logical byte offsets are over UTF-8 bytes. Append
rewrites at most the partly filled final chunk per field, then inserts new chunks.
Drop-prefix advances a logical head offset and deletes fully obsolete chunks;
no renumbering or rewriting unchanged suffix chunks. Checkpoint/replacement
rewrites only this tool's bounded current chunks. Keep at most one partial head
and tail chunk per field in addition to the supported text bound. A replacement
of identical output can update metadata without rewriting its text chunks.

New live compact tools must bypass `externalizeMessagePayload`; otherwise each
patch would again create a full digest blob. Message metadata contains a versioned
internal storage reference and projected summary, never a false legacy digest.
When an ordinary complete tool result replaces compact output, remove the
internal reference and canonical chunks in the same message transaction. A
summary-only metadata update preserves/restores its internal reference; a new
externalized legacy digest is a replacement and retires the old chunks. Rollback
keeps the previous reference and baseline intact.
The scoped detail reader joins metadata and head/chunks in a consistent read
transaction (REPEATABLE READ on PostgreSQL)
and returns the existing whole-tail API shape. Final completion retains the same canonical chunks and atomically commits the
terminal checkpoint and status. No terminal conversion to a digest blob is
required, and terminal detail reads use the same consistent snapshot reader.

Payload removal revokes a valid incremental storage reference even for empty or
short output whose receipt exceeds the removed body. Reclaimed-byte accounting
is clamped to zero when the tombstone increases metadata size; it never prevents
the atomic tombstone and chunk deletion.

Lifecycle/orchestrator receive a committed projection marker plus the typed
semantic summary. Skip duplicate `UpdateToolCallMessage` persistence only for that
exact proved projection. Preserve status, substantive-output attribution, late
terminal reconciliation, parent nesting and turn/lifecycle consumers. A failed
projection must propagate to delivery recovery instead of becoming a logged,
acknowledged callback failure. No fallback-created message may recreate a removed
payload or create a new turn for a settled late terminal.

### Retention integration

Current output is canonical, independent of replay body retention. Delete chunk
rows on message/session deletion and include the schema in reset, backup, required
store descriptors and SQLite/PostgreSQL conformance. Opt-in tool-payload removal
must remove this representation too and preserve its existing removal marker.
Later replay must not rehydrate removed text. Do not copy historical output or
rescan the full database during startup.

Only extend retention's typed eligibility to the new event after its exact atomic
canonical projection proof exists. Existing grace, settlement, hashes and contiguous
retirement rules remain. Retained legacy bodies/hashes remain byte-identical.
A projected patch can be pruned independently because its effect is already in
canonical chunks, not because a later checkpoint merely exists.

## Recovery

Each patch's base refers to the preceding committed output revision for the same
epoch. SQL canonical head/chunks are the durable projector baseline. Journal ACK
remains projection-before-ACK. Losing an ACK can replay an identical event; its
receipt/effect wins without another append. Changed payload, wrong generation,
wrong tool/parent/submission or noncontiguous sequence fails closed.

For an old retained v1 producer, preserve the existing v1 full-snapshot receipt
and callback contract; do not silently translate its accepted events into v2.

On backend restart, first project any received but unprojected inbox prefix, then
replay from the authenticated cursor. Neither path needs an old already-pruned
patch if SQL has the matching canonical revision. A missing/corrupt SQL baseline
blocks the stream unless an exact still-retained checkpoint and its contiguous
patch suffix can reconstruct it under the normal authenticated recovery rules.
A later checkpoint never authorizes skipping missing receipt or required effects.

Journal encoder restart must not depend on acknowledged/pruned event bodies.
Persist its bounded per-active-tool encoder baseline/head with the event transaction,
or start a new explicit full checkpoint epoch on the next normalized update.
Choose the latter for normalizer reinitialization; never treat persisted tail state
as proof of harness reattachment or replay acceptance. Event data and epoch commits
must be atomic. Process loss before journal acceptance retains today's unaccepted
queue boundary; no new guarantee is claimed for pending in-memory output.

No expiry of unacknowledged patches during disconnection. No prompt resend,
replacement conversation, synthetic success or ACK advancement on encoding error.
Keep Stop and existing localized recovery UI available.

## Byte bounds

Let S = 512 KiB (two maximum fields), A = changed text bytes including complete
non-append replacements, N = changed accepted output events, K = first/reset/final
checkpoint epochs, and E = actual serialized non-output/envelope metadata bytes.
For grounded append/sliding-tail fixtures, each journal/inbox payload stream must
satisfy P <= 12A + E + 2S(K + floor(N/512)). JSON text can expand up to sixfold;
the factor 12 also budgets checkpoints driven by byte progress. Partial final
streams and metadata-only events must be accounted separately rather than hidden
in A. Identical cumulative snapshots contribute zero text changes.

The event-count checkpoint term bounds restart work for sparse 1-byte events;
it is about 1 KiB per event at the worst two-field saturation, rather than
512 KiB per event. No periodic full checkpoints while quiet. One full replacement
per barrier is forbidden. Forced resets are reported in K, so a lifetime map
that evicts/reseeds on every interleaved update cannot pass the fixture budget.

For canonical chunk writes, assert submitted text bytes <= 2A + 8 KiB*N + 2S*K,
including partial-chunk rewrite and checkpoint overhead; count actual small head,
message, effect and cursor writes separately. These are logical write-input
bounds, not promises about SQLite page/WAL or bbolt B-tree write amplification.
Measure those physical effects on fresh fixtures with the same transaction,
checkpoint and retention settings. Compression is optional and measured separately;
incompressible fixtures must pass the uncompressed transport bounds.

## Compatibility

Introduce delivery protocol version 2 and journal format version 2; distinguish
supported protocol versions from the maximum journal format constant. Replace
all equality-to-`journal.CurrentVersion` assumptions in negotiation, adoption,
preflight, recovery, ACK and retention with explicit supported-contract checks.
The internal `durable_v1` lifecycle mode remains the existing durable-family
marker; the separately advertised delivery protocol selects supported v1/v2 wire
contracts.
A new backend supports v1 and v2; a retained old agentctl remains v1 and retains
its existing storage cost. Do not retrofit received legacy hashes or silently
replace the runtime to claim the new bound.

A new agentctl upgrades a v1 journal in one validated version transaction on owned
startup before accepting v2 output; preserve all existing event bytes, sequences,
submissions and owner metadata. Read mixed legacy and new records explicitly.
The existing old journal opener rejects a newer on-disk version. Status advertises
v2; every stream/replay/submission entry point requires explicit v2 client support
before returning compact data or admitting work. Omitted support is not consent.
An old backend cannot attach via a legacy stream endpoint to bypass negotiation.
Status/Stop remain available without interpreting compact output.

There is no safe transparent downgrade of new canonical SQL chunks to an old
backend. Both live and completed chunk references are unknown to old binaries. Document forward repair as the normal recovery path;
rollback requires an offline verified matched pre-upgrade database/journal backup
and explicit treatment of work accepted afterward. Never advertise restoring that
backup as lossless for subsequent tool effects, or reset the native conversation
in response to an unsupported reader. This implementation does not operate on the user's live database or restart an instance.

## Verification

Use TDD fixtures through the actual ACP -> writer -> inbox -> canonical projector
boundaries. Record separately: supplied delta bytes, cumulative wire bytes,
normalized changed bytes, serialized accepted bytes, blob/chunk write-input bytes,
effect counts, journal file size and closed DB/WAL sizes. Include incompressible,
JSON-escaped and multibyte text; stdout/stderr saturation; explicit empty and
omitted fields; terminal replacement; cancellation; identical cumulative updates;
barriers every update; multiple interleaved tools; long idle gaps; failed commit;
crash before/after canonical transaction; lost ACK; changed duplicate; retention
past grace and checkpoint pruning; removal-marker replay; v1/v2 retained runtime
and old-reader rejection. All fixtures are disposable, never the user's live DB.

The browser uses the existing disclosure/polling contract on desktop and phone.
No frontend layout, navigation or touch behavior changes are required; mobile-parity
requires equivalent data/reload results, not a new composition or ASCII preview.
Use desktop and mobile end-to-end tests for live output, reload and terminal
snapshot; no full-suite or live-instance verification is needed for this change.
