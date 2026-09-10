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
# Provider Error Recovery System Design Part 2

## Purpose and boundaries

Part 1 defines the provider-neutral error catalogue, the policy classes, and
the replay and effect-safety gate. [Part 3](provider-error-recovery-03.md)
defines the matching ACP diagnostic projection. This part defines the raw ACP
replay fixture matrix that makes the matching projection falsifiable across
gateway shapes, and the harness that replays it.

[Part 3](provider-error-recovery-03.md) defines the other three: the
single-authority propagation path for the provider-diagnostic candidate marker,
the text correlation that separates a transport diagnostic from model prose
about a transport failure, and the allowlisted structured metadata a generic ACP
prompt-error projection may carry.

Neither part changes classification rules, policy values, retry ownership, or
candidate ordering.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001` | [Replay fixture matrix](#replay-fixture-matrix), [Replay harness semantics](#replay-harness-semantics) |

Acceptance criteria `.17` through `.19` are owned by this part. `.20` through
`.23` are owned by [Part 3](provider-error-recovery-03.md).

## Prior art

**Wiki leg (receipt).** Resolved `OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki`,
`QMD_WIKI_COLLECTION=wiki`. **Could not run**: no `obsidian-wiki` or `qmd` on
`PATH`, no QMD MCP server attached, and the vault directory returns
`Operation not permitted`, so the grep fallback was unavailable too.
Unavailable, not empty — no claim is made about the vault's contents.

**Cross-vendor leg (receipt).** `saas-kb` / `search_fsm_docs` not exposed in
this session and no `tool_search` to attach them. Unavailable; no vendor claims
consulted.

**In-repo prior art (did run).** Three existing patterns are adopted, each for
one thing only. The distinction matters, because the first was previously cited
for a job it cannot do:

- `acp/fold_handoff_fixture_test.go` with `testdata/acp-fold-handoff.json`,
  captured by `scripts/probe-acp-midturn-fold`, is the **fixture document
  format** precedent: a probe writes raw frames, a trimmed and de-volatilised
  JSON file is committed, a typed Go loader pins the shape. It is **not** a
  harness precedent. That test never constructs an adapter and never calls
  `sendPrompt` or `handleACPUpdate`; it iterates the parsed struct and asserts
  properties of the data. A replay matrix built on that pattern could not
  exercise any adapter behaviour at all.
- `acp/adapter_session_test.go`, `acp/adapter_client_capabilities_test.go`,
  `acp/load_burst_test.go` and `acp/sync_barrier_test.go` are the
  **live-adapter harness** precedent, and the shape they establish is the one
  this design uses. `newSessionRequestCaptureAdapter` builds two `io.Pipe`
  pairs, assigns a real `acp.NewClientSideConnection` to `Adapter.acpConn`, and
  puts a real `acp.NewAgentSideConnection` wrapping a **typed fake agent** on
  the far end. Going through `acpConn` is the only way in, because it is a
  concrete `*acp.ClientSideConnection` rather than an interface.
  `opencode_stderr_test.go`'s `providerErrorFakeAgent` extends the same shape to
  a prompt that stays open and then fails, and `sync_barrier_test.go` drives
  `syncNotifQueue` directly. This design composes those two: a fake agent that
  emits notifications and then returns a terminal error, against a held queue
  barrier. What no existing test does is drive `session/prompt` to a
  **structured** terminal error and assert what the adapter produced from it.
- The `routingerr` catalogue's rule-plus-negative-case layout
  (`gateway_server_failure_test.go`), where every accepted signature is paired
  with prose that must stay unclassified.

**Departure.** The fold fixture asserts a *shape* only. The replay matrix also
asserts an *outcome*, because a fixture that pins frames without pinning the
decision they authorize cannot catch the regression this work exists to
prevent. That outcome is not observable in one package, which is why
[Replay harness semantics](#replay-harness-semantics) splits the assertion
across two layers.

**No raw frame handling.** An earlier draft of this design had the harness read
the adapter's outbound `session/prompt` request off the pipe and write back a
correlated error response. That is not implementable alongside the precedent
above and is withdrawn: `acp.NewAgentSideConnection` owns the far end of the
pipe, so a test cannot both attach it and read raw JSON-RPC frames from the
same file descriptor. It is also unnecessary. `toReqErr` passes a
`*acp.RequestError` returned by a typed agent through unchanged, and the client
side returns the decoded `resp.msg.Error` to the caller directly, so a fake
agent returning `&acp.RequestError{Code: ..., Message: ..., Data: ...}` reaches
`sendPrompt` as that same structured error, `Data` included. Request-id
correlation is the SDK's job on both sides and the harness never sees an id.

## Input inventory

Sampled from the code and tests at commit `83dd04208`, not assumed.

A terminal ACP prompt error from a gateway-fronted Claude ACP session, as
observed and pinned in `gateway_server_failure_test.go`:

```json
{"code":-32603,
 "message":"Internal error: API Error: 500 Internal server error. This is a server-side issue, usually temporary - try again in a moment.",
 "data":{"errorKind":"server_error"}}
```

The same test file pins a proxy credential-refusal envelope (`proxy_error` with
credentials-refused wording), which this part does not otherwise constrain.

The transport type carrying the first shape is `acp.RequestError`
(`Code int`, `Message string`, `Data any`) from the vendored
`github.com/kdlbs/acp-go-sdk` fork. `Data` is `any`: a map for the sample
above, but adapter-defined and free to be a string, an array, or absent.
`RequestError.Error()` serialises `Data` into its string form, which is why
the raw error string must never be the boundary value.

**The diagnostic and terminal texts observed together.** This is the sample
that decides [Diagnostic text correlation](provider-error-recovery-03.md#diagnostic-text-correlation), and
it was read rather than assumed:

- In the overloaded case pinned by `orchestrator/dynamic_evidence_test.go`, the
  diagnostic chunk and the terminal error message are the **same string**:
  `API Error: Repeated 529 Overloaded errors. The API is at capacity.` is
  passed to both `observeProviderDiagnostic` and the terminal
  `ErrorMessage`.
- In the gateway 500 case above, the chunk text
  `API Error: 500 Internal server error.` is a **substring** of the terminal
  `message`, which prefixes it with `Internal error: ` and appends advice.

Five observations follow directly and shape the sections below:

- `data.errorKind` is a real, observed field name, not a proposal.
- The JSON-RPC code for these envelopes is `-32603` (Internal error), so the
  code alone carries no gateway semantics and cannot be a classification input.
- The gateway's own status text is embedded inside `message`, which is why the
  existing `acp.gateway_server_failure.v1` rule matches on message text and
  keeps doing so.
- A genuine transport diagnostic is therefore always contained in the terminal
  message it precedes. Model prose *about* a failure is not.
- The two catalogue rules differ in prose resistance. `gatewayServerFailureRe`
  is anchored to the exact ACP wrapper form and has an explicit negative test
  rejecting `the task mentioned status 500 Internal server error`.
  `overloadedRe` matches a `529` adjacent to `overloaded` anywhere on one line,
  so ordinary assistant prose narrating a 529 does classify. The hole is
  rule-specific, and closing it inside the catalogue would change stderr
  classification everywhere `overloadedRe` is used.

## Replay fixture matrix

Fixtures live in one canonical directory, embedded and loaded by a shared
leaf package described in [Replay harness semantics](#replay-harness-semantics),
one JSON file per cell, named `<gateway>-<case>.json`.

**Gateways** (closed enumeration): `claude-direct`, `teamclaude`,
`openrouter`, `litellm`.

**The gateway dimension is an envelope-shape label, not an adapter selector.**
All four gateways front the same ACP agent; none of them is an agent id. The
backend's agent ids are `claude-acp`, `codex-acp`, `opencode-acp` and the Grok
id, and `newACPDialect(agentID)` makes adapter behaviour agent-id dependent,
including notification suppression. So the fixture declares the `agentId` it
replays through as a separate field, defaulting to `claude-acp`, and the
harness constructs the adapter with it. What varies across the gateway columns
is the **error envelope text**, not the adapter. To stop the matrix being
satisfied by sixteen relabelled copies of one envelope, two fixtures sharing a
case must not carry equivalent `frames`; the loader enforces this. Equivalence
is decided on the **canonical re-encoding**: the loader decodes `frames` into
its typed form and compares `json.Marshal` of that value, so object key order,
indentation and insignificant whitespace do not affect the answer and cannot be
used to dress one envelope as two. Raw source bytes are never compared.

**Classifying gateways.** A gateway is `classifying` when the catalogue has a
message-text rule for its terminal envelope. `claude-direct` and `teamclaude`
are classifying today, through `acp.gateway_server_failure.v1` and
`anthropic.overloaded.529.v1`. `openrouter` and `litellm` are **not**: no rule
exists for either envelope, which is recorded in the requirement's
`## Out of scope` with what a follow-up would need. Each fixture declares its
gateway's classifying status, and the loader rejects a fixture whose
declaration disagrees with the matrix's own table.

**Cases** (closed enumeration):

| Case key | Frames | Declared disposition |
| --- | --- | --- |
| `matched` | diagnostic chunk, then a terminal prompt error classifying to the same code, chunk text contained in the terminal message | classifying gateway: pre-result, automatic recovery permitted. Non-classifying gateway: unclassified, manual recovery |
| `mismatched` | diagnostic chunk, then a terminal prompt error classifying to a different code | output fence retained; manual recovery |
| `later-output` | diagnostic chunk, then ordinary assistant text or a tool call, then a terminal prompt error | output fence retained; manual recovery |
| `queued` | diagnostic chunk enqueued but not yet drained when the prompt RPC returns its error | diagnostic delivered first, then as `matched` |
| `prose-matched` | model prose narrating a provider failure as the generation's first chunk, then a terminal error classifying to the same code but not containing the chunk text | output fence retained; manual recovery |
| `prose-tool` | model prose as above, then a genuine tool call, then the matching terminal error | effect fence holds; manual recovery |
| `prose-output` | model prose as above, then ordinary assistant output, then the matching terminal error | recorded diagnostic cleared; manual recovery |

`(gateway, case)` is the fixture identity. Both components come from closed
enumerations, so the pair is unique by construction; a second file claiming an
already-declared pair is a test failure, not a last-write-wins overwrite.

**Required cells.** The first four cases are required for all four gateways:
sixteen cells. The three `prose-` cases are required only for a classifying
gateway, because their assertion is vacuous when the terminal envelope does not
classify — six more today, for `claude-direct` and `teamclaude`. Twenty-two in
total. Flipping a gateway to classifying later makes its three `prose-` cells
required automatically, with no change to this design. A required cell with no
fixture fails: no skip, no build tag, no environment gate. A cell that cannot
be captured is still a required file, satisfied by a `reconstructed` fixture,
so the gap is visible in review rather than invisible in a skipped subtest.

### Fixture document

```json
{
  "gateway": "teamclaude",
  "agentId": "claude-acp",
  "classifying": true,
  "case": "matched",
  "capture": "recorded",
  "capturedAt": "2026-09-04",
  "source": "scripts/probe-acp-provider-error --gateway teamclaude",
  "identity": {
    "sessionId": "teamclaude-matched",
    "executionId": "exec-teamclaude-matched",
    "promptGeneration": 7
  },
  "frames": [
    {"kind": "message_chunk", "role": "assistant", "text": "API Error: 500 Internal server error."},
    {"kind": "prompt_error", "code": -32603, "message": "Internal error: API Error: 500 Internal server error.", "data": {"errorKind": "server_error"}}
  ],
  "expect": {
    "events": ["message_chunk:diagnostic", "error"],
    "providerError": {"source": "acp_prompt", "rpcCode": -32603, "errorKind": "server_error", "providerId": "claude-acp"},
    "diagnosticCode": "provider_unavailable",
    "preResultSafe": true
  }
}
```

**`identity` is required on every fixture** and exists for the recovery-evidence
layer, which cannot decide the fence without it. `promptAttemptPreResultSafe`
rejects an empty `SessionID`, an empty `AgentExecutionID`, a zero
`PromptGeneration`, or `EvidenceKnown` false, so a fixture that did not carry
these three values could only ever assert `preResultSafe: false`, and would do
so for the wrong reason. `sessionId` must be unique across the corpus — the
loader enforces it — because evidence records are keyed by session and two
fixtures sharing one would observe each other under `t.Parallel`.
`promptGeneration` must be non-zero.

**Frames.** `frames` is a non-empty ordered array. Every frame carries a `kind`
from a closed enumeration, and the enumeration is what makes each case in the
table above expressible:

| `kind` | Other fields | Replayed as | Token |
| --- | --- | --- | --- |
| `message_chunk` | `role` (`assistant` \| `user`, default `assistant`), `text` | `AgentMessageChunk` / `UserMessageChunk` session notification | `message_chunk`, or `message_chunk:diagnostic` |
| `thought_chunk` | `text` | `AgentThoughtChunk` session notification | `thought_chunk` |
| `tool_call` | `toolCallId`, `title`, `status` (default `pending`) | `ToolCall` session notification | `tool_call` |
| `tool_update` | `toolCallId`, `status` | `ToolCallUpdate` session notification | `tool_update` |
| `model_settled` | `modelId` | `ConfigOptionUpdate` session notification carrying `modelId` as the current model | none — see below |
| `prompt_error` | `code`, `message`, `data` (optional) | the `*acp.RequestError` the fake agent returns from `Prompt` | `error` |

`model_settled` emits no token. It exists so AC `.22`'s two `model_id` cases are
writable: a fixture with a `model_settled` frame before its `prompt_error`
asserts the projected `modelId`, and a fixture without one asserts its absence.
It is deliberately outside the token vocabulary because the adapter's
session-configuration event is not part of the correlation sequence the matrix
pins, and admitting it as a token would make every fixture's `expect.events`
depend on adapter bookkeeping that has nothing to do with the diagnostic.

Structural rules the loader enforces, each a load error rather than a silent
pass: exactly one `prompt_error` frame; it is the **last** frame; a
`tool_update` names a `toolCallId` introduced by an earlier `tool_call` in the
same fixture; a `message_chunk` or `thought_chunk` carries non-empty `text`; and
`role` is present only on `message_chunk`. There is no frame direction field —
every non-`prompt_error` frame is an agent-to-client notification and
`prompt_error` is the response to the single outbound `session/prompt` request,
so direction carries no information and cannot be got wrong.

`capture` is `recorded` or `reconstructed`. `recorded` requires `source` to
name the capture command and `capturedAt` to carry the capture date.
`reconstructed` requires `source` to cite the published contract the frames
were built from and `capturedAt` to be absent; the loader rejects a
`reconstructed` fixture whose `source` is empty or names a capture command, and
a `recorded` fixture missing either field. Recording a fixture as `recorded`
when it was hand-written is the one failure this format cannot detect
mechanically, so the distinction is a declared, reviewable field rather than an
inferred one.

**The `expect.events` vocabulary** is closed. Legal tokens:

| Token | Emitted for |
| --- | --- |
| `message_chunk` | an assistant or user message chunk without the marker |
| `message_chunk:diagnostic` | a message chunk carrying `ProviderDiagnosticCandidate` |
| `thought_chunk` | an agent thought chunk |
| `tool_call` | a tool call event |
| `tool_update` | a tool call status or output update |
| `error` | the terminal provider failure event |

A token is the event type, plus the `:diagnostic` suffix if and only if the
event carries the marker. Comparison is exact, ordered and complete over the
**tokenised subset** of the observed stream: the harness keeps only events whose
type appears in this table, and the kept sequence must equal the declared
sequence element for element, with no subsequence matching and no ignored
trailing events.

The subset is what makes the `model_settled` frame usable. It replays a
`ConfigOptionUpdate`, which does emit a real session-configuration event on
`updatesCh`, and that event type is deliberately absent from this table, so it
is filtered out rather than forcing every `.22` fixture to declare an adapter
bookkeeping token in its correlation sequence. The exclusion is closed, not
open-ended: only the event types a `model_settled` frame can produce are
filtered, and any **other** untokenised event type is a load error, because it
means the fixture replayed something outside the matrix's scope.

**Sanitisation, scoped.** A fixture-lint test scans the **replayed payload** —
the `frames` and `expect` subtrees — for credential and identity shapes:
`Bearer` tokens, `sk-`/`sess-`/`org-` prefixed strings, `ses_` and `wrk_`
OpenCode identifiers, any `http`/`https` URL, absolute paths under `/Users`,
`/home` or `/var`, any email address, and any RFC 4122 UUID. A hit fails the
lint.

**This list is the definition, not a sample of it.** The requirement forbids a
"credential, bearer token, account or organization identifier" in the payload,
and a rule that enumerated only vendor key prefixes would leave the two shapes a
real captured envelope is most likely to carry — an account UUID and an
operator's email address — passing a lint they plainly violate. They are
therefore named above rather than left to a reader's judgement. `identity`'s
`sessionId` and `executionId` are exempt by construction: the loader requires
them to be fixture-invented labels, and a UUID there would fail the lint like
any other. Extending the list is a catalogue revision, not a design change; the
requirement points at this enumeration so the two cannot drift.

The **provenance fields** (`capture`, `source`, `capturedAt`) are scanned by a
narrower rule: no credential or identifier shape, and no private host — no
userinfo component, no `localhost`, `127.0.0.1`, RFC1918 literal, `.internal`
or `.local` host. A public documentation URL is permitted there and nowhere
else. This split is deliberate: `reconstructed` fixtures must cite a published
contract, whose natural form is a URL, and a lint forbidding all URLs over the
whole file would make that unsatisfiable for exactly the cells that cannot be
captured. The requirement forbids a *private* URL in the payload; the payload
rule is stricter on purpose, because no envelope needs a URL to replay
faithfully, and one embedding a documentation link has it stripped during
de-volatilisation like any other volatile value.

## Replay harness semantics

**Where the fixtures and loader live.** Both the ACP adapter layer and the
recovery-evidence layer must read the same fixture files, and the two live in
packages with no import path between them: `internal/orchestrator` does not
import the ACP transport package and the transport package does not import the
orchestrator. So the fixture corpus and its typed loader live in a **leaf
package with no production dependencies**, which both test packages import, with
the fixture JSON embedded via `go:embed`. This avoids a relative `testdata` path
climbing out of one package, and keeps the corpus one source of truth rather
than two drifting copies.

**Where the terminal failure actually appears.** This decides the layer split,
so it is stated before it: for the four gateways in this matrix the adapter
does **not** emit the terminal failure onto its own event stream.
`sendPrompt` *returns* the prompt RPC error, and the terminal `error`
`AgentEvent` is built one layer up, by the agentctl API's async-prompt
goroutine, which calls `ProviderErrorFromError` and then
`procMgr.SendErrorEventWithProviderError`. The two `EventTypeError` emissions
inside `adapter_prompt.go` are the Cursor stream-reset and Codex capacity paths;
Part 3 excludes both, and no matrix gateway takes them. So `updatesCh` carries
the chunk, thought and tool tokens, and nothing else.

**Which layer asserts what.** A fixture is satisfied only when both layers pass:

| Layer | Asserts | Observed on |
| --- | --- | --- |
| ACP transport | `expect.events` up to but excluding the trailing `error` token | the adapter's `updatesCh` |
| ACP transport | the `error` token, `expect.providerError`, `expect.diagnosticCode` | `ProviderErrorFromError` applied to the error `Adapter.Prompt` returned |
| Recovery evidence | `expect.preResultSafe`, and the recorded diagnostic code after the frames | `promptAttemptPreResultSafe` |

The `error` token is therefore an assertion about the **projection**, not about
a stream event, and its position last in `expect.events` carries a real
ordering claim: every preceding token must already be on `updatesCh` at the
moment `Adapter.Prompt` returns. That is exactly the guarantee `syncNotifQueue`
exists to provide, which is why the `queued` case can falsify it.

The split still follows what each layer can observe. `expect.diagnosticCode` is
a `routingerr.Code` and the transport package already imports `routingerr`.
`expect.preResultSafe` is decided by `dynamicPreResultSafe` and
`promptAttemptPreResultSafe` in the orchestrator and is not reachable from the
transport package at all. Neither layer duplicates the other's decision logic: a
helper that recomputed the fence inside the transport test would assert a copy
of the decision rather than the decision.

**What this deliberately does not cover.** The hop from
`ProviderErrorFromError` to `SendErrorEventWithProviderError` in
`internal/agentctl/server/api/agent.go` is a pass-through and the harness does
not construct it. Standing up a process manager inside a transport test would
pull in the dependency the leaf fixture package exists to avoid, to cover two
statements. The residual — that the API layer forwards the projection it was
given — is named in [Out of scope](#out-of-scope) with what would close it, so
it is a recorded gap rather than an assumed one.

**Driving the adapter.** The transport harness constructs an `Adapter` with the
fixture's `agentId` and wires it exactly as `newSessionRequestCaptureAdapter`
does: two `io.Pipe` pairs, a real `acp.NewClientSideConnection` assigned to
`Adapter.acpConn`, and a real `acp.NewAgentSideConnection` on the far end
wrapping a fixture-driven typed fake agent. Unlike
`newSessionRequestCaptureAdapter`, the harness **retains** the
`AgentSideConnection`, because the fake agent needs it to push notifications
while the prompt is in flight.

The fake agent's `Prompt` method replays the fixture: for each frame before the
`prompt_error`, it calls `AgentSideConnection.SessionUpdate` with the
notification that frame's `kind` names; then it returns
`&acp.RequestError{Code, Message, Data}` built from the `prompt_error` frame.
The SDK carries that through unchanged in both directions, so `sendPrompt`
receives the structured error the fixture declared. No raw frame is written or
read by the harness, and no JSON-RPC request id is ever handled by it.

**The evidence layer** does not re-run the transport and does not re-derive the
event sequence. It reads the same fixture and drives the orchestrator directly:

1. `beginPromptAttempt(identity.sessionId, identity.executionId,
   identity.promptGeneration, false)` — `dynamic` is always false; the matrix
   covers the interactive prompt path, and a dynamic attempt fails the fence for
   an unrelated reason.
2. For each frame before the `prompt_error`, in array order, one call:
   a `message_chunk` whose token carries `:diagnostic` calls
   `observeProviderDiagnostic(sessionId, executionId, promptGeneration, text)`;
   any other `message_chunk` or a `thought_chunk` calls
   `observePromptAttempt(..., output: true, effect: false)`; a `tool_call` or
   `tool_update` calls `observePromptAttempt(..., output: false, effect: true)`;
   a `model_settled` frame drives nothing here.
3. The `prompt_error` frame becomes a `watcher.AgentEventData` carrying
   `SessionID`, `AgentExecutionID` and `PromptGeneration` from `identity`,
   `EvidenceKnown: true`, `DynamicRouteAttempt: false`, and `ProviderError` set
   to the projection the transport layer declared in `expect.providerError`.
   `promptAttemptPreResultSafe` is then asserted against
   `expect.preResultSafe`.

The mapping in step 2 is a fixed table, not a judgement, so the two layers
cannot disagree about what a frame means. `OutputObserved` and `EffectObserved`
on the terminal event stay false: they describe the terminal frame itself, and
the evidence the fence reads was already recorded by the per-frame calls.

- **Ordering.** Frames are applied in recorded array order. The array index is
  the only ordering key; there is no timestamp sort and no tiebreak, because
  two frames cannot share an index.
- **Determinism and idempotency.** Replaying the same fixture twice in the same
  process, or in either order relative to another fixture, produces byte-equal
  event sequences and the same fence verdict. The second replay is not
  contaminated by the first: `beginPromptAttempt` stores a fresh evidence record
  under the fixture's session id, replacing any record left behind, so a repeat
  starts from the same state as a first run. The harness asserts no wall-clock duration and inserts no
  sleeps. `ProviderError.OccurredAt` is the one non-deterministic field: it is
  asserted non-zero and non-decreasing across a replay, never compared to a
  literal.
- **Concurrency.** Each fixture replays against its own adapter instance with
  its own pipe pair, notification queue and event channel, and against its own
  evidence record keyed by a fixture-unique session id; no fixture mutates
  package-level state, so both layers are safe under `t.Parallel`. Two fixtures
  for the same gateway share no adapter and cannot observe each other's frames.
- **The `queued` case.** The harness does not sleep and does not race. It holds
  the adapter's update worker at a barrier so the diagnostic notification is
  enqueued but undrained when the prompt RPC settles, then releases it. The
  assertion is that the diagnostic chunk is on `updatesCh` before
  `Adapter.Prompt` returns. This is the executable form of the `syncNotifQueue`
  drain `sendPrompt` performs before returning a prompt error; without that
  drain the chunk is still queued when the API layer emits the terminal
  failure, and the assertion fails, which is the point.

  **The seam and the release trigger, because one obvious reading deadlocks.**
  The barrier is `syncNotifQueueThen`: it posts a FIFO item whose `afterBarrier`
  callback runs *on the worker goroutine*, so blocking inside that callback
  holds the worker, and it must be posted from a goroutine other than the one
  that will call `Adapter.Prompt`, because the posting call itself blocks until
  the barrier closes. The release trigger is the fake agent **returning its
  error**, not `Adapter.Prompt` returning. Waiting for `Adapter.Prompt` to
  return before releasing deadlocks, and deadlocks for the very reason the
  fixture exists: `sendPrompt` calls `syncNotifQueue()` before it returns, which
  posts a second barrier behind the held one, so `Adapter.Prompt` cannot return
  until the release that is waiting on it has happened. Releasing on the fake
  agent's return is both sufficient and safe — the queue is FIFO, so the
  diagnostic enqueued before the release is drained before the barrier
  `sendPrompt` posted after it. `acp/sync_barrier_test.go` is the working
  precedent for driving this primitive from a test.
- **Empty and malformed input.** A load error, failing the test rather than
  passing emptily: zero frames; an unknown `gateway`, `case`, `agentId`, frame
  `kind` or event token; a missing or structurally invalid `identity`; a
  `sessionId` already used by another fixture; a violation of any frame
  structural rule in [Fixture document](#fixture-document); a `classifying` flag
  disagreeing with the matrix table; or two fixtures in one case whose canonical
  `frames` encodings are equal.
- **Expectation defaults.** Every field of `expect` is required except
  `providerError.errorKind` and `providerError.modelId`, which default to
  absent, meaning the projection must not carry them. An omitted expectation is
  an assertion of absence, never an assertion of "don't care" — a fixture
  cannot weaken itself by leaving a field out.

## Failure modes

- A fixture cell cannot be captured against a live gateway: commit a
  `reconstructed` fixture citing the gateway's published error envelope. The
  matrix stays complete and the weaker provenance is visible in the file.
- A gateway changes its envelope: the matching fixture's classification
  assertion fails. The catalogue rule and the fixture are updated together;
  neither alone is a fix.
- The `syncNotifQueue` drain is removed from `sendPrompt`: a `queued` fixture's
  diagnostic is still undrained when `Adapter.Prompt` returns, so the token that
  must precede the terminal failure is absent from the stream at that moment and
  the ordering assertion fails. This is the regression the case exists to catch.
- A `queued` harness releases its barrier on `Adapter.Prompt` returning rather
  than on the fake agent returning its error: the test deadlocks instead of
  failing. A deadlocked subtest reports as a timeout, so the release trigger is
  stated explicitly in [Replay harness semantics](#replay-harness-semantics).

## Out of scope

Carried from the requirement's `## Out of scope`: new OpenRouter and LiteLLM
catalogue signatures, live gateway calls in tests, and retry policy changes.
Two exclusions belong to this part specifically:

- A generic ACP frame-recording facility in the product. Capture stays a
  developer-run script under `scripts/`, matching `probe-acp-midturn-fold`.
- Coverage of the agentctl API hop that turns the returned prompt error into the
  terminal `error` event. `internal/agentctl/server/api/agent.go` calls
  `ProviderErrorFromError` and passes the result to
  `procMgr.SendErrorEventWithProviderError`; the matrix asserts the projection
  that call receives, not that the call forwards it. Closing this needs a test
  at the API layer asserting that the emitted event's `ProviderError` is the
  projection and its `Error` is the projection's message — one test, not
  twenty-two, because the hop does not vary per gateway.
- Fixture coverage for gateways outside the four named shapes. Adding a fifth
  gateway extends the enumeration and adds four cells, or seven if it is
  classifying; that is a catalogue revision, not a design change.
