---
status: draft
created: 2026-09-09
updated: 2026-09-09
owner: kandev
---

# MCP Backend Client: Empty Response Payload Is a Transport Fault

## Why

Both MCP backend clients treat a response carrying **zero payload bytes** as a
successful call. The caller's `result` sink is left at its zero value and `nil` is
returned, so nothing downstream can tell *"the backend said there is nothing"* from
*"the body never arrived."*

`apps/backend/internal/mcp/server/backend_client.go:239`

```go
if result != nil && len(resp.Payload) > 0 {
    if err := json.Unmarshal(resp.Payload, result); err != nil {
        return fmt.Errorf("failed to unmarshal response: %w", err)
    }
}
return nil   // zero-length payload falls through as success
```

`apps/backend/internal/mcp/server/dispatcher_backend_client.go:78` has the same shape.

The cost is concrete at two call sites. `get_task_plan_kandev` renders an empty decoded
result as the literal text `No plan exists for this task yet.`
(`apps/backend/internal/mcp/server/handlers.go:1124`), which is byte-identical to a
genuinely absent plan. On 2026-09-09 an agent on task `e02de3e8` received that text for a
task holding a 54,093-byte plan with 14 revisions, concluded Kandev was down, and held at
max backoff for ~2 hours across 199 `get_task_plan` calls.

`get_walkthrough_kandev` has the identical shape at
`apps/backend/internal/mcp/server/handlers.go:1291`, rendering
`No walkthrough exists for this task yet.` for an empty result against the same `{}`
convention (`internal/mcp/handlers/handlers.go:4339-4341`). It is the same defect with a
smaller blast radius, and it is fixed by the same transport change; neither renderer is
edited.

`handlers.go:1124` is **not** the defect and is not changed here. Given a correct
transport, `len(result) == 0` there legitimately means the backend deliberately sent `{}`,
which is exactly how `handleGetTaskPlan` signals "no plan"
(`apps/backend/internal/mcp/handlers/handlers.go:4210`). That layer is correct. The
transport is the defect.

### Honest scope note, carried forward from the originating ticket

It is **not** established that the `e02de3e8` incident went through this exact path. A hung
channel surfaces as `ctx.Err()`, which is already an error. The empty-payload path is the
only route that yields "no plan" for a task that has one, but it was **not reproduced**.
This spec is justified as removing a class of silent success, not as a proven root cause.
Do not let a later step claim this fix closes that incident.

## Prior art

**Receipt — wiki leg: DID NOT RUN.** Config resolved via the `@henry` inline override:
`~/.obsidian-wiki/config` → `config.henry`, giving
`OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki` and `QMD_WIKI_COLLECTION=wiki`.
The vault path stats successfully but every read is refused by the OS:
`ls`, `cat` and the Read tool all return `Operation not permitted` / `EPERM` on
`index.md`, `hot.md` and directory listing (macOS TCC restriction on `~/Documents` for this
process). Both QMD transports were unavailable independently: no `mcp__qmd__*` tool is
exposed in this session and `command -v qmd` found no CLI, as did `command -v
obsidian-wiki` for the graph pre-pass. So this leg is **unavailable, not empty** — a
compiled page on RPC error contracts may well exist and was not consulted. This was not
worked around.

**Receipt — saas-kb leg: DID NOT RUN.** The `saas-kb` MCP server is not exposed in this
session; `search_fsm_docs` is absent from the tool list, so the `ai_sdlc` category slice
could not be queried. No vendor comparison was gathered.

**Prior art found inside this repository instead**, which is the leg that turned out to
matter. `REQ-AGENTS-MCP-BRIDGE-RELIABILITY-001`
(`docs/specs/agents/requirements/mcp-bridge-reliability.md`, `status: active`, created
2026-09-04) already states the governing intent: *"An agent must receive a result or a
descriptive bridge error when it calls a Kandev tool."* Its
`AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.4` covers *a dispatcher returning **no response***
— the nil-message case, already guarded at
`dispatcher_backend_client.go:65`. It does **not** cover a response that arrives with **no
body**. That is the gap this spec closes, and it is a gap in an existing contract rather
than new ground.

**What we are doing differently:** nothing is being invented. This spec extends an existing
active requirement in its own id namespace (see below) rather than opening a competing one,
so the 205 existing `@covers` annotations in the backend stay coherent and a later editorial
fold-in of this file into `docs/specs/agents/requirements/mcp-bridge-reliability.md` is a
copy, not a renumber.

## Owning contract and id namespace

The durable owner is the **agents** system, file
`docs/specs/agents/requirements/mcp-bridge-reliability.md`. That file is `status: active`
and is not edited by this card; this spec is the frozen contract for the change and
continues its identifier family:

- New requirement: **`REQ-AGENTS-MCP-BRIDGE-RELIABILITY-002`**
- New criteria: **`AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.1`** onward

Build annotates tests with `// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.N`, matching
the convention already used in `internal/mcp/server/backend_client_test.go:56,73`.

## Input inventory (measured, not assumed)

Every claim below was executed or read, not recalled.

### I1. `ws.NewResponse` cannot produce a zero-length payload

`apps/backend/pkg/websocket/message.go:85` marshals every payload through `json.Marshal`.
Measured with a throwaway Go program on this checkout's toolchain:

| Value passed to `NewResponse` | Marshalled bytes | Length |
| --- | --- | --- |
| `map[string]interface{}{}` | `{}` | 2 |
| `nil` | `null` | 4 |
| `(*T)(nil)` | `null` | 4 |
| `[]string{}` | `[]` | 2 |

There is no input for which `json.Marshal` yields zero bytes without also returning an
error, and `NewResponse` propagates that error instead of building a message. **A
well-formed response therefore never carries zero payload bytes.**

### I2. Zero length is reachable only through a malformed frame

Measured decode of a `ws.Message`-shaped envelope:

| Wire JSON | `len(Payload)` | `Payload == nil` |
| --- | --- | --- |
| `payload` field **absent** | **0** | true |
| `"payload":null` | 4 (`null`) | false |
| `"payload":{}` | 2 (`{}`) | false |

The channel client's responses are decoded from the wire at
`internal/agentctl/server/api/agent.go:202-225`, so a frame that lost its `payload` field
decodes to length 0. The dispatcher client receives an in-process `*ws.Message` and is
reachable only by a hand-built message. A repository-wide grep for hand-constructed
`ws.Message{... MessageTypeResponse ...}` outside tests returned **no** non-test hits: every
production response goes through `NewResponse`/`NewError`.

**Consequence, and it is the key safety property of this change:** for all correct traffic
the new rule is unreachable, so it cannot alter the behavior of any working flow. It fires
only on a dropped or truncated body.

### I3. `{}` really is the "no plan" signal, and it survives

`internal/mcp/handlers/handlers.go:4208-4211` returns
`ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{})` when no plan exists — 2
bytes. Measured: `json.Unmarshal([]byte("{}"), &map)` yields a **non-nil, empty** map. So
`len(result) == 0` at `handlers.go:1124` still holds and the existing wording still applies.

### I4. `null` decodes to a nil map and is *also* indistinguishable — but is out of scope

Measured: `json.Unmarshal([]byte("null"), &map)` succeeds and leaves the map **nil**, so
`len(result) == 0` at `handlers.go:1124` is true and the same "No plan exists" text is
produced. A 4-byte `null` therefore reaches the identical bad outcome while passing a
`len > 0` guard.

It is nevertheless **not** a transport fault, because `null` is a legitimate, currently-used
convention in this repository: `internal/task/handlers/task_plan_handlers.go:61`
(`wsGetTaskPlan`, registered for `ws.ActionTaskPlanGet` at
`internal/task/handlers/task_handlers.go:257`) returns `ws.NewResponse(msg.ID, msg.Action,
nil)` for exactly the "no plan" case. That action is the frontend route and is **not**
called by any MCP tool today — the MCP server uses `ws.ActionMCPGetTaskPlan` — but both are
registered on the **same** `ws.Dispatcher`, so treating `null` as an error would be one
registration away from breaking a live handler. See `## Out of scope`.

### I5. Call-site census

50 non-test `RequestPayload` call sites. Exactly one passes a `nil` result sink:
`internal/mcp/server/handlers.go:993` (`ActionMCPClarificationTimeout`). Every other site
passes `&result`. The `nil`-sink case must keep working untouched.

### I6. Error-type responses already return an error

When `resp.Type == ws.MessageTypeError` and the payload is empty,
`json.Unmarshal([]byte{}, &ep)` fails, so control reaches
`fmt.Errorf("backend error: %s", string(resp.Payload))` and the caller gets
`"backend error: "` — a poor message, but **an error**, not a silent success. This path is
therefore outside the class this spec removes.

### I7. `result != nil` is an interface test, and stays one

The existing guard tests the `result interface{}` argument for nil. A non-nil interface
holding a nil pointer (`var p *T; RequestPayload(..., p)`) is therefore a *non-nil* sink
today and remains one under this spec — no call site does this (I5), and the classification
of that shape is deliberately unchanged so the check stays a one-line predicate rather than
a reflection-based inspection of the sink's dynamic value.

## Terminology

- **Backend client** — either implementation of `BackendClient.RequestPayload`
  (`internal/mcp/server/server.go:37`): `ChannelBackendClient` (agentctl ⇄ backend
  WebSocket bridge) and `DispatcherBackendClient` (in-process, external MCP endpoint).
- **Result sink** — the `result interface{}` argument. `nil` means the caller reads no body.
- **Success branch** — the point in `RequestPayload` reached after a response message has
  been obtained and the error-type branch has **not** been taken.
- **Empty payload** — `len(resp.Payload) == 0`. Distinct from `{}` (2 bytes) and `null`
  (4 bytes).

## Requirements

### REQ-AGENTS-MCP-BRIDGE-RELIABILITY-002: An empty response body is reported, never returned as success

**Intent:** A backend client must not report success for a response whose body never
arrived. Because a well-formed response can never carry zero payload bytes (I1), zero
length always means the body was dropped or truncated, and reporting it as success is never
right.

#### Acceptance criteria

- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.1:** When a backend client reaches the success
  branch, the caller supplied a non-nil result sink, and the response payload is zero
  bytes, the client shall return a non-nil error instead of `nil`.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.2:** The error returned by AC-002.1 shall satisfy
  `errors.Is(err, ErrEmptyBackendPayload)` against one sentinel shared by both clients, and
  its message shall contain the action string. The message shall not contain the request
  payload or any tool argument. Both clients are in package `mcp`
  (`apps/backend/internal/mcp/server/`), so the sentinel is one package-level `var` visible
  to both; no new package, file-split or interface is required for it.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.3:** When the caller supplied a **nil** result
  sink, a zero-byte payload shall not produce an error. `ActionMCPClarificationTimeout`
  (`handlers.go:993`) shall keep its current behavior.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.4:** The existing error-type branch shall be
  evaluated before the empty-payload check. An error-type response carrying a zero-byte
  payload shall keep its current backend-error behavior (I6) and shall not be reported as
  an empty-payload fault.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.5:** A 2-byte `{}` payload shall continue to
  decode into a non-nil empty map and shall not produce an error. `get_task_plan_kandev`
  shall continue to render `No plan exists for this task yet.` for it, with that wording
  unchanged at `handlers.go:1124`.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.6:** A 4-byte `null` payload shall continue to be
  passed to `json.Unmarshal` unchanged, and shall not be reclassified as an empty-payload
  fault. Where the sink is one `json.Unmarshal` can assign into — every sink in the call-site
  census (I5) — this produces no error. Where it is not, AC-002.7 governs and the existing
  unmarshal error stands: `null` into a non-nil interface holding a nil pointer returns
  `json: Unmarshal(nil *T)` (I7), and that error is preserved rather than suppressed. This
  criterion removes `null` from the empty-payload rule; it does not promise success for a sink
  that cannot receive a value.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.7:** A payload that is non-empty but not valid
  JSON, or not assignable to the result sink, shall keep returning the existing unmarshal
  error and shall not be reclassified.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.8:** Both backend clients shall apply an identical
  rule: the classification of a given (result sink, payload, message type) triple shall not
  differ by transport. Their error *message text* need not be byte-identical — each client
  keeps its own existing phrasing style — but both shall satisfy AC-002.2.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.9:** Each empty-payload rejection shall be recorded
  exactly once at `Warn`, on both clients, with exactly this field set — key names are part of
  the contract so a log-based alert matches both transports:
  - both clients: `request_id` (string) and `action` (string);
  - `ChannelBackendClient` additionally: `session_id` (string) and `duration`, reusing the
    keys its existing terminal-failure log already carries (`backend_client.go:215-221`).
  `request_id` shall be the **outbound id the client generated for the request** — `id` in
  both implementations — never `resp.ID`, which a malformed frame may set to anything; the
  point of the field is to correlate the rejection with the call that provoked it.
  No error field shall be logged: unlike the terminal-failure log this line cites, there is no
  upstream error here — the error is created at this site, so logging it would only repeat the
  message. `DispatcherBackendClient` logs no duration and no session id because it measures
  neither. The log shall not contain the request payload or any tool argument, consistent with
  `AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.7`.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.10:** The check shall introduce no new shared
  state, no new lock, and no retry or backoff. A rejected request shall be surfaced to the
  calling tool handler on its first occurrence.

## Determinism: evaluation order inside `RequestPayload`

`RequestPayload` has no rows and no collection to sort, so ordering here means the fixed
sequence of classification checks. It is part of the contract because AC-002.4 and AC-002.7
are only well-defined relative to it.

**This is two different things, and only the second is a total order.** Conflating them would
license a builder to restructure code this spec does not change.

### Phase A — obtaining an outcome (NOT ordered, NOT changed)

Before anything can be classified, the call must produce either an error or a response
message. How that happens differs per client and is **outside** the ordered sequence below:

- `ChannelBackendClient` waits in a **single `select`** over three cases — a delivered
  `backendResponse`, `ctx.Done()`, and `c.done` (client closed). These are **peers, not steps**:
  when more than one is ready Go chooses pseudo-randomly, so which of them wins is
  **deliberately nondeterministic and stays that way**. A delivered response additionally
  carries `response.err`, checked immediately on that branch.
- `DispatcherBackendClient` has **no** cancellation or closed case at all. Context
  cancellation reaches it only as an error returned by `Dispatch`, and it additionally rejects
  a `nil` response message.

Nothing in this spec reorders, merges, or adds a case to Phase A. In particular the `select`
shall NOT be restructured to test `ctx.Err()` before reading the response channel: that would
change which error a caller sees when a response and a cancellation are ready together, on a
path this change does not touch.

### Phase B — classifying a response message (ordered; this IS the contract)

Once Phase A has yielded a non-nil response message and no error, both clients shall evaluate
in exactly this order:

1. **Error-type response** — `resp.Type == ws.MessageTypeError` → backend error. *Unchanged;
   AC-002.4.*
2. **Nil result sink** — → return `nil` without inspecting the payload. *AC-002.3.*
3. **Empty payload** — `len(resp.Payload) == 0` → `ErrEmptyBackendPayload`. **New;
   AC-002.1.**
4. **Otherwise** — `json.Unmarshal` into the sink; propagate its error. *Unchanged;
   AC-002.7.*

Phase B step 1 keys on `resp.Type == ws.MessageTypeError` only. Any other message type — including
a `notification` or an unrecognized type reaching the dispatcher client from a hand-built
handler response — falls through to Phase B steps 2 to 4 and is classified by payload alone. This is
the existing behavior and is not changed; it is stated so it is not re-invented.

Phase B step 2 precedes step 3 deliberately: the nil-sink caller has asked for no body, so the
absence of one is not observable to it and must not be an error.

## Payload value table (nil / empty / error / defaults / boundaries)

Behavior for every payload the success branch can see, with a non-nil result sink that
`json.Unmarshal` can assign into — which is every sink in the call-site census (I5):

| `len(resp.Payload)` | Bytes | Reachable in production? | Outcome |
| --- | --- | --- | --- |
| 0 | `` (field absent, or nil) | Only via a malformed/truncated frame (I2) | **Error** (new) |
| 2 | `{}` | Yes — `handleGetTaskPlan` "no plan" (I3) | Success; non-nil empty map |
| 2 | `[]` | Yes — empty list responses | Success; empty slice |
| 4 | `null` | Yes on the shared dispatcher (I4) | Success; nil map. Unchanged |
| >0 | valid JSON | Yes | Success; decoded |
| >0 | invalid JSON, or type mismatch | Corruption | Existing unmarshal error |

With a **nil** result sink, every row is success, unchanged.

Two qualifications on the sink, so the table is not read wider than it is. A non-nil interface
holding a **nil pointer** counts as a non-nil sink (I7), but `json.Unmarshal` cannot assign
into it: every `len > 0` row then returns the existing `json: Unmarshal(nil *T)` error under
AC-002.7, including the `null` row. The zero-byte row is unaffected by this — it is decided by
AC-002.1 before any unmarshal is attempted, so it is an empty-payload fault regardless of what
the sink could have received. No call site passes that shape (I5); it is stated only so the
table and AC-002.6 cannot be read as promising success for a sink that cannot take a value.

## Idempotency and retry

Which Phase B branch a response takes is a pure function of
`(result sink is nil, len(resp.Payload), resp.Type)`. Branch 4's *outcome* additionally depends
on whether the sink can receive the decoded value, which is a property of the caller's
argument, not of the response (I7); that does not affect the empty-payload verdict, which is
decided at branch 3 before any unmarshal.
It mutates nothing, so re-evaluating the same response yields the same verdict, and
replaying the same request yields the same verdict for the same response. Neither client
retries internally, and this change adds no retry, no backoff and no dead-letter path
(AC-002.10). The error is returned once to the calling tool handler, which already converts
it via `mcp.NewToolResultError(err.Error())`; whether the agent retries is the agent's
decision and is unchanged.

## Concurrency

Two callers issuing requests on the same client concurrently are unaffected. The new check
reads only call-local values — the `resp` message and the `result` argument — and runs
**after** the pending-map entry for that request id has already been removed under
`pendingMu` (`backend_client.go:129-143, 180-184`). It touches neither `pending`,
`sessionID`, `done`, nor `publishWG`, so it adds no lock, no ordering constraint between
goroutines, and no new failure interleaving. Two concurrent requests that both receive an
empty payload each get their own error; neither observes the other.

A response arriving concurrently with `Reset()` or `FailStreamRequests` is **not** settled by
any precedence rule in this spec. `completeRequest` and `FailStreamRequests` both take
`pendingMu`, look up the request id, delete the entry, and only then send on the pending
channel (`backend_client.go:113-143`), so **whichever goroutine acquires the lock first is the
one that supplies the caller's outcome**, and the loser finds no entry and sends nothing. That
lock-winner semantics is existing behavior and is **preserved unchanged**. Two consequences
follow, and both are intended:

- If reset/disconnect wins, the caller gets its cancellation or disconnect error and Phase B
  never runs.
- If the response wins, the caller gets that response and it is classified by Phase B like any
  other — **including by the new empty-payload rule.** A response that wins this race and
  carries no body is reported as `ErrEmptyBackendPayload`, not as a disconnect error. This is
  correct: a message genuinely arrived and genuinely had no body, and the disconnect error
  would be a guess about a cause this layer cannot observe.

This does **not** weaken the safety property in I2. Winning the race does not make a payload
empty — a well-formed response still carries at least 2 bytes, so a correct response that wins
still decodes normally. The race decides *which* outcome a caller sees; it never manufactures
a zero-byte body.

## Behavior explicitly preserved

- The wording at `handlers.go:1124` is unchanged (AC-002.5). That layer is correct.
- `handleGetTaskPlan`'s `{}` "no plan" signal is unchanged (I3).
- `wsGetTaskPlan`'s `null` "no plan" signal is unchanged (I4).
- `ActionMCPClarificationTimeout`'s nil-sink call is unchanged (AC-002.3, I5).
- Error-type responses, context cancellation, client-closed, disconnect and reset errors are
  all unchanged.
- No new configuration, no runtime feature flag, no environment variable. The rule is
  unconditional; it needs no kill switch because it is unreachable for correct traffic (I2).

## Out of scope

Each exclusion below is a contract, not silence. A later step that wants one of these routes
back to Spec.

- **A 4-byte `null` payload is not treated as a fault.** It reaches the same
  indistinguishable "No plan exists" outcome as a zero-byte payload (I4), so this is a real
  and deliberately unclosed gap. It is excluded because, unlike zero length, `null` is
  *producible by a correct handler* — `wsGetTaskPlan` produces it today for exactly the "no
  plan" case, on an action registered on the same dispatcher. Erroring on it would convert a
  working convention into a failure and would be a behavior change, not a defect fix. A
  follow-up that wants to close it must first decide whether `null` means "absent" or
  "malformed" **per action**, which is a contract decision this card does not own.
- **Improving `"backend error: "` for an error-type response with an empty payload** (I6).
  It is a poor message but it is an error, so it is outside the silent-success class this
  spec removes.
- **Any change to `handlers.go:1124`** or to any other tool handler's rendering of an empty
  result. Explicitly forbidden by AC-002.5.
- **A schema or non-emptiness check on decoded payloads.** This spec distinguishes "no body"
  from "a body"; it does not validate the body's contents.
- **The SSH keepalive fix and the agent-backoff cap**, both named in the originating ticket
  as related. They are separate cards. The backoff cap in particular is what bounded the
  2-hour stall's cost and is not addressed here.
- **Reproducing the `e02de3e8` incident**, or claiming this change closes it. See the honest
  scope note above.
- **Any frontend or user-interface change.** There is none; see the E2E decision below.

## Verification

Tests are Go unit tests alongside the source, per `apps/backend/AGENTS.md`. No new test
harness is needed: both target files already have a test file with a usable fake
(`fakeDispatcher` at `dispatcher_backend_client_test.go:16`, and direct channel driving at
`backend_client_test.go:74-90`).

- `internal/mcp/server/dispatcher_backend_client_test.go` — empty payload with a non-nil sink
  errors and matches the sentinel (002.1, 002.2); with a nil sink does not (002.3); an
  error-type message with an empty payload keeps its backend-error text (002.4); `{}`
  decodes to a non-nil empty map (002.5); `null` still decodes without error (002.6).
- `internal/mcp/server/backend_client_test.go` — the same matrix driven through
  `HandleResponse`, plus the `Warn` log assertion via the `zaptest/observer` already imported
  at line 13 (002.9), asserting exactly one entry and the `request_id` / `action` /
  `session_id` / `duration` keys AC-002.9 fixes. The identical-matrix requirement (002.8) is
  satisfied by asserting the same expectations in both files.
- `internal/mcp/server/dispatcher_backend_client_test.go` — **a second `Warn` log assertion for
  `DispatcherBackendClient` (002.9)**, asserting exactly one entry with `request_id` and
  `action`, that `request_id` equals the id on the request the fake dispatcher received (not
  `resp.ID`), and that no tool argument appears in the entry. Without this bullet AC-002.9 is
  verified on one of the two clients it governs. This test must build its own logger via
  `logger.NewFromZap(zap.New(core))` — the pattern already working at
  `backend_client_test.go:108-109` — and pass it to `NewDispatcherBackendClient`, which accepts
  a `*logger.Logger`. It must **not** reuse `newTestLogger` (`server_test.go:143-148`): that
  helper has no observer core and is built at level `error`, so a `Warn` would not be recorded
  even if one were attached.
- A handler-level test that `get_task_plan_kandev` still renders
  `No plan exists for this task yet.` for a `{}` payload (002.5), using the `testBackend`
  fake at `config_handlers_test.go:16`. `get_walkthrough_kandev`
  (`handlers.go:1291`) shares the shape; covering it is optional, and covering it does not
  license editing either renderer.

Commands: `make -C apps/backend test` and `make -C apps/backend lint`. A targeted run is
`go test ./internal/mcp/server/... -run 'BackendClient'` from `apps/backend`.

**E2E decision: no Playwright E2E.** This change touches no user-visible surface — no route,
no component, no copy, no locale key. It is confined to two Go files in
`internal/mcp/server/` plus their tests. Its only externally observable effect is an error
string returned to an agent through an MCP tool result on a path that correct traffic never
reaches (I2). Go unit tests are the complete verification.
