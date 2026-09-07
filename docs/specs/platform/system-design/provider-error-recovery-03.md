---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
created: 2026-09-04
updated: 2026-09-04
owners:
  - Kandev
---
# Provider Error Recovery System Design Part 3

## Purpose and boundaries

[Part 2](provider-error-recovery-02.md) defines the raw ACP replay fixture
matrix and its harness. This part defines the three behaviours those fixtures
assert:

1. the single-authority propagation path for the provider-diagnostic candidate
   marker,
2. the text correlation that separates a transport diagnostic from model prose
   about a transport failure, and
3. the allowlisted structured metadata a generic ACP prompt-error projection
   may carry.

It does not change classification rules, policy values, retry ownership, or
candidate ordering.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001` | [Marker propagation](#marker-propagation), [Diagnostic text correlation](#diagnostic-text-correlation), [Allowlisted prompt-error metadata](#allowlisted-prompt-error-metadata) |

Acceptance criteria `.20` and `.21` are owned by
[Marker propagation](#marker-propagation). `.23` is owned by
[Diagnostic text correlation](#diagnostic-text-correlation). `.22` is owned by
[Allowlisted prompt-error metadata](#allowlisted-prompt-error-metadata).
`.17` through `.19` are owned by [Part 2](provider-error-recovery-02.md), whose
`## Input inventory` records the sampled shapes this part reasons from.

## Marker propagation

`ProviderDiagnosticCandidate` is decided exactly once and then only carried.

The decision site is
`acp/adapter_updates.go` `convertMessageChunkWithProtocolID`, which classifies
the chunk text and sets the marker when the verdict is high confidence and
fallback-eligible. Every hop after that reads the field:

| Hop | Carrier |
| --- | --- |
| agentctl boundary | `streams.AgentEvent.provider_diagnostic_candidate` |
| lifecycle publish | `lifecycle.AgentStreamEventData.ProviderDiagnosticCandidate` (`events.go`) |
| lifecycle activity | `lifecycle` `recordActivity` (`manager_events.go`) |
| orchestrator stream entry | `event_handlers_streaming.go` `handleAgentStreamEvent` |
| orchestrator message path | `event_handlers_streaming.go` `handleMessageStreamingEvent` |
| recovery evidence | `orchestrator/dynamic_evidence.go` `observeProviderDiagnostic` |

Two consumers currently re-derive the verdict from message text instead of
reading the carried field, and both are corrected here:

- `handleAgentStreamEvent` calls `isHighConfidenceProviderDiagnostic(payload.Data.Text)`
  to choose between `observeProviderDiagnostic` and `observePromptAttempt`. It
  reads the carried marker instead. `isHighConfidenceProviderDiagnostic` and
  its second classification of the same bytes are removed.
- `handleMessageStreamingEvent` flips foreground-generating for any non-empty
  text. A marked diagnostic chunk does not flip it, matching the turn-progress
  suppression `recordActivity` already applies on the lifecycle side.

**What the marker is, and is not.** The marker is a *candidate* flag, and the
word is load-bearing. No transport-level signal at the ACP boundary separates a
gateway diagnostic from model text: `convertMessageChunkWithProtocolID` receives
a session id, a content block, a role and a protocol message id, and a gateway
diagnostic arrives as an ordinary `agent_message_chunk` — the same frame type
the model's own prose arrives on.
So the marker is necessarily a text classification: moving the decision to the
boundary removes *drift between two classification sites* without adding
*discrimination*. It is necessary evidence, never sufficient. What separates a
transport diagnostic from prose about one is
[Diagnostic text correlation](#diagnostic-text-correlation), which runs where
both texts are known.

**Accumulation.** `.20` requires the marker carried on every streaming,
lifecycle and evidence hop, and "carried" means the field survives on the
in-flight event value each hop receives — which it does, because those hops
pass the same struct. Message accumulation and flush are a different thing:
they neither read nor act on the marker, no message-level column is added, and
the persisted transcript row does not carry it. The diagnostic text is
persisted and rendered as ordinary transcript content, which Part 1 already
requires. The marker affects only turn-progress bookkeeping and recovery
evidence, both of which are per-event.

**Absent marker.** An event that arrives without the field is ordinary output.
This is a deliberate behaviour change for a version-skewed remote executor
running an older agentctl: it loses automatic recovery for this specific
correlation and falls back to manual recovery. Two classification sites
disagreeing is the exact drift AC `.20` exists to forbid, and the fail-closed
direction costs an automatic retry, never correctness.

**Concurrency.** Stream handling already serialises per session under
`acquireCancelInFlightGuard`, and `promptAttemptEvidence` guards its own
fields with a mutex. Two chunks for the same session therefore apply in
delivery order under one lock; two chunks for different sessions are
independent records. A marked chunk whose execution ID or prompt generation
does not match the recorded attempt is dropped by the existing identity fence
and records nothing.

**Clearing.** The two paths are not symmetric, and the asymmetry is the
existing behaviour rather than a change:

- An unmarked non-empty assistant or thought chunk in the same generation
  clears `providerDiagnosticCode`, through the `output` branch of
  `observePromptAttempt`, and restores the ordinary output fence.
- Tool activity does **not** clear the recorded code. `observePromptAttempt`
  clears only on `output`; `effect` is recorded separately and never clears.
  The attempt is still not pre-result safe, because the effect fence fails it
  independently. This design keeps that behaviour and does not extend clearing
  to `effect`: widening a clearing rule to reach an outcome another fence
  already guarantees would add a second way to be right and to regress.
- A second marked chunk in the same generation does not clear it: the first
  recorded code stands, because letting a later diagnostic overwrite it would
  let a gateway retry its own way into a match.

## Diagnostic text correlation

Code equality is not enough. Two texts classify alike whenever they trip the
same catalogue rule, and `anthropic.overloaded.529.v1` trips on any line
pairing `529` with `overloaded` — including an assistant sentence narrating
that the upstream returned 529. Such a chunk is marked, routed to
`observeProviderDiagnostic`, records the code, then lifts the output fence when
the terminal failure classifies to the same code. That is real assistant output
replayed on a second provider, which
`AC-AGENTS-DYNAMIC-AGENT-ROUTING-ROLLOUT-BLOCKERS-001.1` forbids.

So a recorded diagnostic authorizes pre-result recovery only when **both** hold:

1. the diagnostic and the terminal failure classify to the same high-confidence,
   fallback-eligible code, as Part 1 already requires, and
2. the diagnostic's normalized text is **contained in** the terminal failure's
   normalized message.

Normalization trims leading and trailing whitespace and collapses internal
whitespace runs to a single space. Comparison is case-sensitive, because
gateway envelope text is stable and a case-insensitive compare would widen the
match for no observed benefit. Containment is directional: the diagnostic must
be contained in the terminal message, never the reverse. That direction is what
rejects prose, because a sentence that quotes an error and adds narration
around it is longer than the error and is not contained in it.

The rule is grounded in the two shapes recorded in
[Input inventory](provider-error-recovery-02.md#input-inventory): the overloaded
pair is text-identical, and
the gateway 500 pair is a strict substring. Both satisfy containment. The
reported prose case does not.

An empty or whitespace-only normalized diagnostic never satisfies containment,
which matters because the empty string is otherwise contained in everything. No
minimum length is imposed beyond that: a string short enough to be incidentally
contained in an unrelated message would not classify at high confidence, so it
never reaches this rule.

**Where the diagnostic text is kept.** Containment needs both operands at
correlation time, and today only one survives: `promptAttemptEvidence` records
`providerDiagnosticCode` and `observeProviderDiagnostic` drops its `message`
argument after classifying it. So the evidence record gains one field beside the
code — the diagnostic's **normalized** text — with these rules:

- **Written** in `observeProviderDiagnostic`, in the same guarded assignment
  that records `providerDiagnosticCode`, so the two are never separately
  present. Normalization happens at write time for the diagnostic and at compare
  time for the terminal message: the diagnostic is stored once and compared
  many times, the terminal message arrives once, and normalizing the stored copy
  keeps the record small and the comparison total.
- **Cleared** with `providerDiagnosticCode`, on the same `output` branch of
  `observePromptAttempt` and in the same critical section. Clearing one and not
  the other would leave a half-record that either authorizes on a stale text or
  fails containment for a code that was legitimately re-recorded.
- **Not overwritten** by a second marked chunk in the same generation, matching
  the code's own rule above: the first recorded pair stands, so a gateway cannot
  retry its way into a match by emitting a better-shaped diagnostic second.
- **Not persisted and not published.** It is per-attempt evidence with the same
  lifetime as the code, and it never reaches a message row or an event payload.

An empty stored text can therefore only mean "no diagnostic recorded", which is
the same thing the zero code means, and both fail containment for the same
reason.

## Allowlisted prompt-error metadata

`providerErrorFromACPPrompt` projects a sanitised message from a terminal
`acp.RequestError`. It gains four allowlisted fields and no others.

| Field | Source | Rule |
| --- | --- | --- |
| `rpc_code` | `RequestError.Code` | Carried verbatim. JSON-RPC forbids `0`, so `0` means absent. |
| `error_kind` | `RequestError.Data["errorKind"]` | Accepted only when `Data` is a map and the value is a string of at most 64 bytes matching `[A-Za-z0-9_.-]+`. Anything else is dropped. |
| `provider_id` | the adapter's negotiated agent id | Never parsed out of the error text. |
| `model_id` | the session's settled model id | Omitted when no model is settled. |

**Settled model.** A session's model is settled once the adapter has emitted a
session-configuration or model event carrying a non-empty current model id for
that session — the value produced by `currentModelFromConfig` or by the
session's `CurrentModelID`, whichever the adapter last published. Before that,
and for an agent that never surfaces a model id, no model is settled and
`model_id` is omitted. The definition is observable rather than internal
because an omitted `modelId` in a fixture is an assertion of absence.

**Where the two new fields live.** `streams.ProviderError` already carries
`ProviderID` and `ModelID`, so `provider_id` and `model_id` need no new
storage. It has nothing for the other two, so it gains exactly two fields:
`RPCCode int` (`json:"rpc_code,omitempty"`) and `ErrorKind string`
(`json:"error_kind,omitempty"`). Both are omitempty, which is what makes the
"`0` means absent" rule above expressible on the wire without a pointer.
`ProviderError.Valid()` is **unchanged** — it gates on `Source`, `Message` and
`OccurredAt`, and neither new field participates, because a projection that
carries a message but no metadata is still a valid projection. A fixture
asserting `rpcCode` or `errorKind` is asserting content, never validity.

**Contention on the settled model.** A session-configuration or model event can
land while a terminal error is being projected. The rule is last-published-wins,
read once: the projection reads the adapter's current model id under the
adapter's existing lock at the moment it is built, and a model event that
arrives afterwards does not retroactively add `model_id` to an already-built
projection. No new synchronization is introduced and no ordering is imposed
between the two, because `model_id` is descriptive metadata that authorizes
nothing in this version — a projection that misses a model settled in the same
microsecond is a slightly thinner log line, not a wrong decision. This is also
why a fixture's `model_settled` frame is ordered strictly before its
`prompt_error` frame rather than raced against it.

Raw `RequestError.Data` never crosses the boundary, and no field other than
`errorKind` is read from it. A `Data` that is a string, an array, `nil`, or a
map without `errorKind` yields a projection with no `error_kind`; the
projection stays valid on its message alone.

**Call site and merge order.** The merge happens in `ProviderErrorFromError`,
which is already the ordered extractor chain: a correlated stderr
`providerPromptError`, then `providerErrorFromACPActionURL`, then
`providerErrorFromACPPrompt`. Today those are three mutually exclusive early
returns; they become compute-then-merge. The allowlisted metadata is derived
once from the terminal `acp.RequestError` and merged onto whichever projection
won, filling only fields that projection left empty, so a richer
provider-specific extractor is never overwritten by the generic one. When the
underlying error is not an `acp.RequestError` at all, there is nothing to
derive and the winning projection is returned unchanged. Because
`provider_id` and `model_id` are adapter state rather than error state, the
merge needs adapter context: `ProviderErrorFromError` takes it as an explicit
argument rather than reaching for a package-level value. The Cursor and Codex
paths in `adapter_prompt.go` return before reaching this chain and are
unchanged; they carry their own provider-specific projections and this design
does not add metadata to them.

**Not an authorisation input.** The metadata is transported, logged, and
assertable in fixtures. No classifier rule keys on it in this version. Making
`errorKind` outrank the message-text catalogue would let a gateway authorise
dynamic recovery through a field no fixture constrains yet, so it is deferred
to a later contract change.

## Failure modes

- Metadata present but the message unclassifiable: the projection carries the
  metadata, the diagnostic stays ordinary output, recovery fails closed.
- The marker is absent from a skewed remote: manual recovery, as above.
- A diagnostic classifies to the terminal code but is not contained in the
  terminal message: the fence holds and manual recovery is exposed. This is the
  `prose-matched` case, and it is the one the current code gets wrong.

## Scenarios

- **GIVEN** a TeamClaude session emits `API Error: 500 Internal server error.`
  as an `agent_message_chunk` and then returns `-32603` with
  `data.errorKind = server_error`, **WHEN** no other output or tool activity
  occurred in that generation, **THEN** the transcript keeps the chunk, the
  projection carries `rpc_code -32603` and `error_kind server_error`, the chunk
  text is contained in the terminal message, and the attempt is pre-result safe.
- **GIVEN** the same diagnostic followed by a terminal error classifying to a
  different code, **THEN** the output fence is retained and manual recovery is
  exposed.
- **GIVEN** the same diagnostic followed by ordinary assistant text before the
  terminal error, **THEN** the recorded diagnostic code is cleared and the
  attempt is not pre-result safe.
- **GIVEN** the diagnostic notification is still queued when `session/prompt`
  returns its error, **THEN** the diagnostic chunk still reaches the event
  stream before the terminal failure event.
- **GIVEN** an assistant chunk reading
  `I ran the build and the upstream returned 529 because the provider is
  overloaded; retrying now.` as the generation's first chunk, **AND** a
  terminal failure reading
  `API Error: Repeated 529 Overloaded errors. The API is at capacity.`,
  **WHEN** both classify to `provider_overloaded`, **THEN** the chunk text is
  not contained in the terminal message, the output fence is retained, and the
  attempt is not pre-result safe.
- **GIVEN** that same prose chunk followed by a genuine tool call and then the
  matching terminal failure, **THEN** the effect fence fails the attempt even
  though the recorded diagnostic code was never cleared.
- **GIVEN** a terminal `acp.RequestError` whose `Data` is the string
  `"unavailable"`, **THEN** the projection carries `rpc_code` and no
  `error_kind`, and no part of `Data` appears in the message.
- **GIVEN** a message chunk arrives from an agentctl build that does not send
  `provider_diagnostic_candidate`, **THEN** it counts as ordinary output and
  automatic recovery does not start.

## Out of scope

Carried from the requirement's `## Out of scope` and owned by this part:
classifier rules keyed on `errorKind` or the JSON-RPC code, tightening the
overloaded signature, UI rendering of the marker, retry policy changes, and the
remaining gateway usage adapters. Fixture and capture exclusions live in
[Part 2](provider-error-recovery-02.md#out-of-scope).

One further exclusion belongs to this part: extending the clearing rule so tool
activity clears the recorded diagnostic code. The effect fence already fails
such an attempt, so the change would alter a mechanism without altering an
outcome; [Marker propagation](#marker-propagation) records why.
