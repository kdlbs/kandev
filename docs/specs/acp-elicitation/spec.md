---
status: draft
created: 2026-08-09
owner: kandev
---

# ACP Form Elicitation

> **Provenance — read this first.** This spec was **split out of
> `docs/specs/disambiguate-waiting/spec.md` on 2026-08-09** by explicit human decision
> taken at that card's Spec Review cap (option (c), narrow scope + option (b), change the
> contract). It was previously "Half A" of that combined spec.
>
> **It has NOT been re-reviewed as a standalone contract.** It carries the verified
> evidence and acceptance criteria from three Spec Review rounds, plus **eight open
> findings** listed at the end that were never closed because the card was split before
> they were worked. Treat those findings as the work list for this spec's first Spec
> Review round.
>
> **No Kandev card exists for this spec yet.** Creating it is the human's decision, per
> the workflow rule that anything cut from a narrowed card becomes a card the human
> creates. Nothing in this file is scheduled work until that happens.
>
> The sibling spec `docs/specs/disambiguate-waiting/spec.md` (Waiting Attribution) keeps
> the parked-attribution half. The two share no acceptance criterion, no data structure,
> and no file except the agentctl agent-stream action switch, and they have different
> ship gates: this one is flag-gated, that one ships unflagged.

## Why

Kandev advertises no ACP elicitation capability, so the Claude bridge strips the
`AskUserQuestion` tool from every session Kandev starts. A Claude agent inside Kandev
physically cannot ask a structured question — prose plus end-of-turn is its only option,
and prose is not a machine-readable "a human is required" signal. Two further behaviours
degrade silently behind the same capability flag: MCP servers requesting user input are
auto-declined by the SDK, and a model refusal with an available fallback ends the turn
instead of offering the retry.

Kandev is manufacturing its own ambiguity. This spec removes that.

## Verified inputs

Every claim below was read from the shipped artifact named, not inferred. This section is
contract input: the shapes here are what an implementation must accept, and what a
conformance test must produce. Line numbers are as of `bf62f39b1`.

### A. What Kandev sends today

`clientCapabilitiesForAgent`
(`apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_helpers.go:16`)
returns `acp.ClientCapabilities{Meta: {"terminal_output": true}}` and nothing
else. `grep -rn "Elicit" --include=*.go apps/backend/` returns zero matches:
elicitation is greenfield in this repo.

### B. What the bridge does with that

From the shipped `dist/` of `@agentclientprotocol/claude-agent-acp@0.65.0`
(deps `@agentclientprotocol/sdk@1.3.0`, `@anthropic-ai/claude-agent-sdk@0.3.220`),
`acp-agent.js:4280-4287`, verbatim:

```js
const elicitationSupport = {
  form: !!this.clientCapabilities?.elicitation?.form,
  url:  !!this.clientCapabilities?.elicitation?.url,
};
const disallowedTools = elicitationSupport.form ? [] : ["AskUserQuestion"];
```

Three behaviours are gated on the same flag:

| Bridge site | Gate | Consequence today |
|---|---|---|
| `acp-agent.js:4287` | `form` | `AskUserQuestion` is added to `disallowedTools` |
| `acp-agent.js:4352` | `form \|\| url` | `onElicitation` unattached ⇒ MCP elicitations hit the SDK default (auto-decline) |
| `acp-agent.js:4361` | `form` | `onUserDialog` + `supportedDialogKinds: [refusal_fallback_prompt]` unattached ⇒ a model refusal with an available fallback ends the turn instead of offering the retry |

**`url` is not reachable for us.** `elicitation/complete` is sent only under
`if (this.clientCapabilities?.elicitation?.url)` (`acp-agent.js:2116-2121`), and
`mode: "url"` is produced only by `mcpElicitationToCreateRequest` when an MCP
server asks for it (`elicitation.js:9-22`). `handleMcpElicitation`
(`acp-agent.js:3779-3805`) declines a url-mode request outright when
`support.url` is false. Advertising `form` alone therefore means Kandev never
receives a url-mode `elicitation/create` and never receives
`elicitation/complete` at all.

### C. The exact form the bridge will send — executed, not read

The shapes below are **output captured by importing the shipped
`dist/elicitation.js` and calling its exported builders directly**, not a
transcription of its source. The builders are pure functions over their
arguments, so this requires no Claude API access and is exactly reproducible:
import the module, call `askUserQuestionsToCreateRequest(questions, sessionId,
toolCallId)` and `applyAskElicitationResponse(response, toolInput, questions)`,
print the JSON. Treat these as the conformance fixtures for the mapping.

**Single question, single-select** — note there is no `description` on the field,
because a lone question's text is carried by `message`:

```json
{
  "mode": "form",
  "sessionId": "sess-1",
  "toolCallId": "tool-42",
  "message": "Which database should the new service use?",
  "requestedSchema": {
    "type": "object",
    "properties": {
      "question_0": {
        "type": "string",
        "title": "Datastore",
        "oneOf": [
          { "const": "Postgres", "title": "Postgres", "description": "Relational, already operated by the team." },
          { "const": "SQLite", "title": "SQLite", "description": "Embedded; no ops burden.",
            "_meta": { "_claude/askUserQuestionOption": { "preview": "file:./app.db" } } },
          { "const": "DynamoDB", "title": "DynamoDB" }
        ]
      },
      "question_0_custom": {
        "type": "string",
        "title": "Other",
        "description": "Type your own answer instead of choosing an option above (optional).",
        "_meta": { "_askUserQuestionCustomAnswer": { "questionId": "question_0", "isCustomAnswer": true } }
      }
    }
  }
}
```

**Two questions, second multi-select** — `message` becomes the literal
`"Please answer the following questions."`, each field gains its own
`description`, and the multi-select field is an array with `items.anyOf`:

```json
{
  "question_1": {
    "type": "array",
    "title": "Envs",
    "description": "Which environments should it deploy to?",
    "items": { "anyOf": [
      { "const": "dev", "title": "dev" },
      { "const": "staging", "title": "staging" },
      { "const": "prod", "title": "prod" }
    ] }
  }
}
```

`requestedSchema` carries **no `required` array** — every field is optional,
mirroring the built-in tool's Skip affordance. The enum `const` is always the
option **label**, because that is what the tool records as the answer.

Executing `applyAskElicitationResponse` pins five behaviours that the source read
did not make obvious, and three of them are contractually load-bearing:

| Input | Result |
|---|---|
| `accept` with selections | `{action: "answered", updatedInput: {…, answers: {"<question text>": "Postgres", "<question text>": "dev, prod"}}}` — keyed by **question text**, multi-select **comma-space joined** |
| `accept` with `question_0_custom: "  CockroachDB  "` | custom wins over the selection **and is trimmed** → `"CockroachDB"` |
| `accept` with **empty** `content` | `{action: "answered", answers: {}}` — **byte-identical to `decline`** |
| `decline` | `{action: "answered", answers: {}}` |
| `cancel`, or any action the bridge does not recognize | `{action: "cancel"}` — aborts the tool call |

The third row matters: an accept carrying no answers is **not** a no-op. It tells
the model the user skipped, exactly as an explicit decline does. So a UI that
submits an empty form and a UI that offers a Skip button produce the same
outcome, and Kandev cannot make the agent distinguish them however it maps the
wire action.

`extractAskUserQuestions` is the gatekeeper and filters per-entry rather than
all-or-nothing: a list of one well-formed and one malformed question returns just
the well-formed one; a list where every entry is malformed, or a non-array,
returns `null`.

**Refusal fallback.** Once `form` is advertised this arrives too, as an ordinary
form elicitation Kandev must render. Executed, it is a single property named
`choice` with two options whose `const` values are `retry_fallback` and
`cancelled`, and a `message` of the form `"Fable declined this request (safety).
Retry with Opus?\n\n<guidance>"`. It carries no `title` and no custom-answer
companion, which is the case that proves the mapping must tolerate both being
absent. `REFUSAL_FALLBACK_DIALOG_KIND` is the literal
`"refusal_fallback_prompt"`.

**MCP passthrough.** `mcpElicitationToCreateRequest` forwards an MCP server's
schema with arbitrary property names untouched and only force-stamps
`type: "object"` — executed with `{properties: {apiKey: {type: "string"}}}` it
emits exactly that, with **no** companion property. This is the empirical proof
case for the rule in the API surface below: a custom-answer companion must be
found by its `_meta` marker, never by appending a suffix to a question key. It
returns `null` for a url-mode request carrying no url, so that never reaches us.

### D. What the pinned Go SDK already provides

`github.com/coder/acp-go-sdk`, replaced by
`github.com/kdlbs/acp-go-sdk v0.13.6-0.20260722160645-1ce4653527f6`
(`apps/backend/go.mod:8,125`):

- `ClientCapabilities.Elicitation *ElicitationCapabilities` with
  `Form *ElicitationFormCapabilities` / `Url *ElicitationUrlCapabilities`
  (`types_gen.go:1057, 2031-2079`). Supplying `{}` means supported; omitted or
  `null` both mean unsupported.
- Method names `elicitation/create` and `elicitation/complete`
  (`constants_gen.go:42-43`) — matching the bridge. (`session/create_elicitation`
  appears only in a stale docstring; it is not a wire method.)
- `UnstableCreateElicitationRequest` is a four-variant union discriminated by
  `mode` × scope (`FormSession`, `FormRequest`, `UrlSession`, `UrlRequest`).
  `Validate()` **rejects** a payload carrying neither `sessionId` nor
  `requestId`, and one carrying both (`elicitation_test.go:75-115`).
- Elicitation is **unstable-only**, which is what justifies the runtime flag
  rather than an unconditional advertisement. In the SDK's vendored schema at
  version `1.20.0`, the string `elicitation` occurs **51 times in
  `schema/schema.unstable.json` and 0 times in `schema/schema.json`**. Every Go
  type carries the header "This capability is not part of the spec yet, and may
  be removed or changed at any point," and the generated names are prefixed
  `Unstable*` accordingly.
- `UnstableCreateElicitationResponse` is a union over
  `accept | decline | cancel | other`, where `accept` carries
  `Content map[string]any` (`types_gen.go:7978-8030, 8072`).
- Dispatch is by optional-interface assertion: `ClientSideConnection.handle`
  (`client_gen.go:30-47`) checks whether the client implements
  `UnstableCreateElicitation(context.Context, UnstableCreateElicitationRequest)
  (UnstableCreateElicitationResponse, error)` and returns `MethodNotFound`
  otherwise. **The handler blocks the JSON-RPC request for its whole duration.**

### E. The operator machinery that already exists

`internal/clarification` is a structural match for a form elicitation and
already drives every operator surface this feature needs:

```
Question{ID, Title, Prompt, Options []Option{ID, Label, Description}}
Answer{QuestionID, SelectedOptions []string, CustomText string}
Response{PendingID, Answers, Rejected, RejectReason}
Store.CreateRequest / WaitForResponse (blocking, 2h default) / Respond
PendingClarification{done chan, CancelCh chan}
```

An agent-authored `clarification_request` message with `requests_input: true`
and `metadata.pending_id` (`internal/backendapp/gateway.go:489-504`) is what
produces `pending_action: "clarification"`
(`internal/task/repository/sqlite/message.go:451`,
`internal/task/models/models.go:854-858`), which drives the
`IconMessageQuestion` affordance
(`apps/web/lib/ui/state-icons.tsx`), the
`session.clarification_requested` notification, and the
`sidebar-pending-question` e2e spec. In `getSessionStateIconConfig`,
`canRequestInput` is `state === "RUNNING" || state === "WAITING_FOR_INPUT"`, so a
pending clarification raised **mid-turn** already renders correctly.

The agentctl↔backend round trip for a blocking agent-side request already
exists for permissions: `Manager.handlePermissionRequest`
(`internal/agentctl/server/process/manager.go:2337`) parks a
`PendingPermission{ResponseCh}` in a map, emits a notification over the updates
channel, and blocks; the answer arrives as the **WebSocket action**
`agent.permissions.respond` on the agent stream, dispatched by
`Server.handleWSPermissionRespond` (`server/api/agent.go`) →
`Manager.RespondToPermission`, driven from the backend by
`lifecycle.Manager.RespondToPermission`
(`internal/agent/runtime/lifecycle/manager_interaction.go:1520`) →
`Client.RespondToPermission` →
`sendStreamRequest(ctx, "agent.permissions.respond", …)`
(`internal/agent/runtime/agentctl/client_stream.go:26`).

**It is not an HTTP route.** `POST /api/v1/acp/permissions/respond` appears in
this repo **only** in two stale doc comments
(`internal/agentctl/types/streams/permission.go:108`, `.../streams/doc.go:23`).
`grep -rn '"/acp' internal/agentctl/` matches no registered route and the
agentctl router declares no `/api/v1/acp` group. Anything this spec models on
"the permissions round trip" therefore means the WS action, never a URL.

### F. What notifications actually do today

The ticket's premise that the notification path "keys on WAITING_FOR_INPUT" does
not hold. `apps/web/lib/ws/handlers/notifications.ts` keys on backend-emitted
semantic events, and `playWaitingForInputSound` is a misleading function name
that plays for every notification event. The backend decides:

- `events.TurnCompleted` → `isAbandonedTurnCompletion` filter →
  `HandleTaskTurnFinished` → `session.turn_finished`
  (`internal/backendapp/gateway.go:343-357, 483-487`).
- agent-authored clarification message → `HandleClarificationRequested` →
  `session.clarification_requested` (`gateway.go:361-375`).

Per `docs/specs/platform/notifications.md` and `ensureDefaultProviders`
(`internal/notifications/service/service.go:375, 410`), **`session.turn_finished`
is opt-in and off by default**; `session.clarification_requested` is on by
default. So today a parked-on-background turn end pings only operators who
deliberately enabled turn-finished. The primary damage is therefore visual, not
audible: the board and the task list are what cannot distinguish.

### K. The wire probe — OQ-3 closed

The ticket asked for an actual `elicitation/create` frame before locking the
design, because §B was read from `dist/`. **That frame has now been observed**, in
a controlled A/B against bridge v0.65.0 driven by a throwaway raw JSON-RPC client
(scratchpad only, no repo code). The two runs differed in exactly one field: the
presence of `elicitation: {form: {}}` in `initialize`'s `clientCapabilities`.
Both were given the identical prompt — "use the AskUserQuestion tool right now to
ask me to pick a colour."

| | **Advertised** `elicitation.form` | **Control** (today's Kandev) |
|---|---|---|
| `elicitation/create` on the wire | **yes** | **no** |
| tool calls emitted | the AskUserQuestion round trip | **none** |
| how the question was asked | structured form, `toolCallId` present | prose |
| `session/prompt` requests sent | **1** | 1 |
| `stopReason` | `end_turn`, on the original prompt | `end_turn` |

The received frame matched the §C fixture field for field — `mode: "form"`, the
session id, a `toolCallId`, `message: "Pick a colour"`, and a `requestedSchema`
carrying `question_0` (a titled `oneOf` of `Red`/`Blue`) plus its
`question_0_custom` companion with the `_askUserQuestionCustomAnswer` marker.
Replying `{"action": "accept", "content": {"question_0": "Red"}}` continued the
turn, and **the `end_turn` arrived on the original `session/prompt` — exactly one
prompt request was sent for the whole exchange**. That is the empirical basis for
AC-13; it was previously an inference from the request/response shape.

In the control run the agent stated the problem in its own words before falling
back to prose:

> I don't have the AskUserQuestion tool available in this session, so I can't
> use it. Instead, plainly: **Pick a colour — Red or Blue?**

That is the ticket's premise, first-person, on the wire: Kandev's own silence
about a capability is what forces the agent into the ambiguous prose-plus-
end-of-turn shape this feature exists to remove.

**One incidental discovery, deliberately not built upon.** In both runs the CLI
emitted a `post_turn_summary` frame that the bridge does not recognize — it logs
`Unexpected case:` to stderr and drops it. In the control run that frame read
`status_category: "blocked"`, `status_detail: "awaiting colour choice: Red or
Blue?"`, `needs_action: "pick Red or Blue"`. So a "a human is required" signal
does exist at the CLI layer today and is discarded before reaching ACP. It is
recorded here because it independently corroborates the problem statement, but it
carries the same warning the ticket attaches to `async_launched`: it is
CLI-internal passthrough that the bridge does not model, does not forward, and
does not version. **No behaviour in this spec may depend on it.**

## Requirements

### Half A — advertise form elicitation

- Kandev SHALL advertise `elicitation: { form: {} }` in its ACP
  `ClientCapabilities` when the `features.acpElicitation` runtime flag is on,
  so a Claude session inside Kandev regains the `AskUserQuestion` tool, MCP
  elicitation forwarding, and the refusal-fallback retry dialog.
- Kandev SHALL NOT advertise `elicitation.url`. It is out of scope; see
  Verified inputs §B for why advertising `form` alone is self-consistent.
- An in-flight form elicitation SHALL present as a Kandev clarification: the
  same pending record, the same `pending_action: "clarification"`, the same
  question-mark affordance, the same `session.clarification_requested`
  notification, and the same answer UI as an `ask_user_question_kandev` call.
- Answering an elicitation SHALL continue the **same** agent turn. It is a
  tool-call round trip inside an open `session/prompt`, never a new prompt.
- Every malformed or unsupported shape SHALL fail closed, matching the
  `agentAdvertisesPromptQueueing` discipline
  (`.../transport/acp/prompt_queueing.go`).
- An agent that ignores the capability SHALL behave exactly as it does today.

## Data model

### Elicitation (in agentctl process memory, per execution)

```
pendingElicitation
  pending_id      string   PK, minted by agentctl, globally unique (UUID);
                           reused verbatim as the clarification pending_id
  session_id      string   ACP session id from the request
  tool_call_id    string   nullable; present for AskUserQuestion
  message         string   the elicitation's human-readable message
  questions       []Question  derived from requestedSchema (see API surface)
  companion_keys  map[string]string  question ID -> verbatim custom-answer
                           property key; absent when that question declared none
  response_ch     chan     buffered(1)
  created_at      timestamp
```

**`Question.ID` IS the verbatim schema property key.** Not an ordinal, not a
surrogate, not a re-derivation — the key exactly as it appeared in
`requestedSchema.properties`. This is what makes the response mapping's "content
keys are the original schema property keys, verbatim and never re-derived" rule
mechanically true: the key is carried in the field the answer is already keyed by,
so replay is an identity, not a lookup. It also removes the ordinal correspondence
an earlier draft implied by storing a positional `field_keys` list alongside a
separately-ordered question list — two orderings that could drift. `companion_keys`
is therefore keyed by question ID as well, and the D1 sort determines only the order
questions are *presented*, never how an answer is addressed.

Lifetime is the ACP request. It does not survive an agentctl restart; see
Persistence guarantees.

## API surface

### ACP client capability

`clientCapabilitiesForAgent` returns, when `features.acpElicitation` is on:

```json
{ "_meta": { "terminal_output": true }, "elicitation": { "form": {} } }
```

When the flag is off, the value is byte-identical to today's. The `_meta`
agent-scoping behaviour of `acpcompat.ClientCapabilityMeta` is unchanged.

### `elicitation/create` (client-side handler, agentctl)

Kandev implements
`UnstableCreateElicitation(ctx, UnstableCreateElicitationRequest)
(UnstableCreateElicitationResponse, error)`.

Accepted: the `FormSession` variant only, and only while
`features.acpElicitation` is on. Every other variant, every malformed payload,
and every request received while the flag is off is answered
`{"action": "decline"}` without reaching the operator. The handler is gated on
the same flag as the advertisement so that an agent which sends an
unadvertised elicitation cannot reach the operator through it.

Schema → questions mapping, applied to `requestedSchema.properties`. **Question
order is defined by D1, not by declaration order** — the Go binding types
`Properties` as a map, so declaration order does not survive parsing. Companions
are resolved and removed before ordering (D2).

The rules are **ordered**, and the first match wins. Order is load-bearing: a
custom-answer companion is itself a `{type: "string"}`, so rule 3 must be tested
before rule 4 or every companion would become a question of its own.

| # | Schema property | Becomes |
|---|---|---|
| 1 | `{type: "string", oneOf: [...]}` | one single-select `Question`; each `oneOf` entry → `Option{ID: const, Label: title \|\| const, Description: description}` |
| 2 | `{type: "array", items: {anyOf: [...]}}` | one multi-select `Question`; options as above |
| 3 | `{type: "string"}` carrying `_meta._askUserQuestionCustomAnswer.questionId` | the free-text companion of the named question, not a question of its own |
| 4 | `{type: "string"}` with no `oneOf` and no companion marker | one **free-text** `Question` with **zero options**; the operator answers it by typing |
| 5 | anything else | ignored |

**Why rule 4 exists.** Without it the mapping is self-contradictory and silently
defeats one of the three behaviours this capability is supposed to restore:

- The *Response mapping* below already says free text is written to "the
  question's own key **only if that key carries no enum constraint**." Rules 1–3
  can only ever produce enum-constrained questions, so that clause would describe
  a question the mapping could never emit — dead prose masking a gap.
- Verified inputs §C establishes by execution that `mcpElicitationToCreateRequest`
  forwards an MCP server's schema with arbitrary property names untouched and
  **no companion field**, and gives `{properties: {apiKey: {type: "string"}}}` as
  the captured example. Under rules 1–3 plus "anything else → ignored", that maps
  to zero questions and is **declined** (AC-18). "MCP elicitation forwarding" —
  named in §B and in the Half A requirements as one of the three things
  advertising `form` restores — would move from *SDK* auto-decline to *Kandev*
  decline, i.e. restored in name only. Rule 4 is what actually restores it.

A rule-4 question is answered exclusively by free text. Because it declares no
companion, its free text is written to **its own key** — which is exactly the
branch the Response mapping already specifies, now reachable.

`Question.Prompt` is the property's `description` when present, otherwise the
request's `message`, otherwise the property's `title`, otherwise the property key
(D10). `Question.Title` is the property's `title` when present. When mapping
yields zero questions, the handler declines.

Response mapping:

| Operator action | ACP response |
|---|---|
| answers and submits | `{"action": "accept", "content": {"<question property key>": <label \| [labels]>, "<companion property key>": "<free text>"}}` |
| skips / rejects the bundle | `{"action": "decline"}` |
| turn cancelled, session torn down, or agentctl shutting down | `{"action": "cancel"}` |

`content` keys are the original schema property keys, **verbatim and never
re-derived** — the mapping records each key at parse time and replays it. A
selected option is written as its enum `const`, which is the option label the
tool records as the answer. A free-text answer is written to the companion
property identified by that question's `_meta._askUserQuestionCustomAnswer`
marker; the companion is found by its marker, never by appending a suffix to the
question key, so an MCP-sourced form that names its properties arbitrarily still
round-trips. When a question declares no companion, free text is written to the
question's own key only if that key carries no enum constraint (a rule-4
free-text question, or a rule-1/2 question whose enum came back empty), and is
otherwise dropped.

**`content` is sparse: a key appears only when it carries an answer.** The
elicitation schema declares no `required` array (§C), so every field is optional
and omission is the wire's own way of saying "unanswered". Stated exhaustively,
because a partially-answered multi-question form is the common case and the rule
above only covers a fully-answered one:

| Situation | That question's key | Its companion key |
|---|---|---|
| option(s) selected, no free text | present — `const` label, or a JSON array of labels for multi-select | omitted |
| free text only, question declares a companion | **omitted** | present — the trimmed free text |
| free text **and** a selection, question declares a companion | **omitted** | present — the trimmed free text |
| free text, question declares **no** companion (rule 4, or empty enum) | present — the trimmed free text | n/a |
| question left entirely unanswered | omitted | omitted |
| every question unanswered | `content` is `{}` (AC-12b) | — |

A key is **never** emitted as `null` or as an empty string to signal "no answer";
omission is the only such signal. The third row is what AC-12a pins: the
custom-answer companion already overrides the selection when the bridge folds the
response back (§C, executed), so also sending the now-dead selection would put a
value on the wire that the bridge is guaranteed to discard. Omitting it keeps
Kandev's `content` a faithful statement of what the operator actually chose,
which matters for MCP consumers that have no such override rule.

Whitespace-only free text is treated as no free text: it is trimmed first (§C
shows the bridge trims), and if nothing remains the question falls back to its
selection, or to omission when there is none.

### Answering an elicitation (backend → agentctl)

**Correction to an earlier draft of this spec.** A previous revision specified
`POST /api/v1/acp/elicitations/respond` and justified it as mirroring
`POST /api/v1/acp/permissions/respond`. That route **does not exist**. The string
appears only in two stale doc comments
(`internal/agentctl/types/streams/permission.go:108` and `.../streams/doc.go:23`);
`grep -rn '"/acp' internal/agentctl/` matches **no registered route**, and the
agentctl router has no `/api/v1/acp` group at all. Building against it would
produce a route the backend has no client for.

The real mechanism is a **WebSocket action on the existing agent stream**.
`Client.RespondToPermission` calls
`sendStreamRequest(ctx, "agent.permissions.respond", payload)`
(`internal/agent/runtime/agentctl/client_stream.go:26`), and agentctl dispatches
it from the action switch in `server/api/agent.go` (`case
"agent.permissions.respond"` → `handleWSPermissionRespond`). Elicitation follows
exactly that pattern and adds no new transport:

| | |
|---|---|
| **Action** | `agent.elicitations.respond` |
| **Direction** | backend → agentctl, over the already-open agent stream |
| **Dispatch** | a new `case` in the `server/api/agent.go` action switch, beside `agent.permissions.respond` |
| **Backend entry point** | a new `Client` method using `sendStreamRequest`, mirroring `RespondToPermission` |

Request payload:

```jsonc
{
  "pending_id": "…",
  "answers": [ { "question_id": "…", "selected_options": ["…"], "custom_text": "…" } ],
  "rejected": false,
  "reject_reason": ""
}
```

Response is the `{success: true}` shape used by `PermissionRespondResponse`. An
unknown or already-settled pending id returns a WS error with
`ws.ErrorCodeNotFound` — the transport-level equivalent of the `404` the earlier
draft named, and what AC-12g asserts. `ErrAgentStreamNotConnected` (the stream is
down) is the "answer could not be delivered" case in AC-20a.

### Answering an elicitation — the backend half

The section above specifies the backend→agentctl hop. This one specifies how an
operator's answer *reaches* that hop, because the two ends do not meet by
themselves: the only operator-answer entry point in the tree today is the
clarification handler's `Store.Respond(pendingID, resp)`
(`internal/clarification/handlers.go`), which resolves a **local** waiter — and for
an elicitation the waiter is in another process. Left unstated, a builder would
invent the join, and the natural invention contradicts D5 (see the ordering rule
below).

**Identity.** The `pending_id` is minted by **agentctl** when the
`elicitation/create` handler parks its `pendingElicitation`, and must be globally
unique (a UUID, not a per-process counter — the backend's clarification store is
global across every execution, while an agentctl instance only knows its own). The
backend creates its clarification pending record **with that same id** rather than
minting one: one identity spans both processes, so the answer needs no mapping
table. This requires the store to accept a caller-supplied id; that is the one
capability the existing `CreateRequest` does not have today.

**Marking.** The clarification pending record created from an elicitation is
recorded as **elicitation-backed**, carrying the session and execution it came from.
That marker is the discriminator for everything below. An
`ask_user_question_kandev`-backed record carries no marker and is untouched by this
feature.

**Dedup.** An elicitation-backed creation bypasses `CreateRequest`'s
same-normalised-questions dedup and always creates a new record — this is D4, and
the marker is how the store knows to bypass it.

**The answer fork, and the ordering that makes D5 true.** When a submission arrives
for an elicitation-backed record, the backend does **not** call the local resolve
path at all. It forwards, then reports:

1. Send `agent.elicitations.respond` to the owning agentctl.
2. `{success: true}` → the ACP request has been answered. The backend settles its
   clarification record and message.
3. WS error `ws.ErrorCodeNotFound` → agentctl says this pending is unknown or
   **already settled**. The backend reports that to the operator and settles its
   record if it has not already.
4. `ErrAgentStreamNotConnected` → the answer never left. The record **stays open**
   (AC-20a), consistent with *Out of scope*'s "no retry queue": the pending remains
   until the answer deadline declines it.

The load-bearing part is that step 1 comes first and there is **no local
already-answered short-circuit in front of it**. `Store.Respond` returns
`ErrAlreadyResponded` for a second response, so routing elicitation submissions
through it would settle duplicates in the backend and agentctl would never see
them — making D5's and AC-12g's `ws.ErrorCodeNotFound` unobservable. Elicitation
idempotency is adjudicated in exactly one place, and that place is agentctl, which
is the only process holding the ACP request.

### Stream event (agentctl → backend)

A new normalized agent event carrying `pending_id`, `session_id`,
`tool_call_id`, `message`, and the mapped questions. The backend converts it to
an agent-authored `clarification_request` message with `requests_input: true`
and `metadata.pending_id`, so `clarificationNotificationOccurrence`
(`gateway.go:489`), `GetPendingActionsBySessionIDs`, and every existing operator
surface apply unchanged.

### The answer surface — what the operator actually interacts with

The Half A requirement says an elicitation presents with "the same answer UI as an
`ask_user_question_kandev` call". That reuse is right, and it is **not free**: the
shipped clarification overlay cannot express three of this spec's own acceptance
criteria. Verified in the tree, not inferred
(`apps/web/components/task/chat/clarification-input-overlay.tsx`):

- selection is derived as `selected.length > 0 ? selected[0] : null` — it reads
  **only the first** element;
- every commit is `commitAnswer({… selected_options: [optionId]})` — always exactly
  one option;
- `customActive = !selectedOption && hasCustomText`, with the in-file comment
  *"custom_text and selected_options are mutually exclusive"*.

So the overlay is **strictly single-select** and **forbids free text coexisting with
a selection**. AC-12 (select two options), AC-12a and AC-12i (free text *and* a
selection) are therefore unreachable through it, and mapping rule 4's zero-option
question is a shape it has never had to render. Without this section a builder would
either invent the whole interaction or ship mapping-layer tests over inputs no
operator can produce. Three additions, each scoped to what an AC needs:

**1. Multi-select.** A question mapped by rule 2 (`type: "array"`, `items.anyOf`)
renders a multi-select control. Selecting accumulates rather than replaces;
deselecting removes. `selected_options` is written in **option declaration order** —
the `anyOf` array order the mapping preserved — **never in click order**. Click order
is an operator artifact and would make AC-12's byte-level array assertion
non-deterministic; declaration order is already the spec's rule for options
everywhere else (D1: "options render in array order and are never sorted").

**2. Free text alongside a selection, for elicitation-backed questions only.** The
existing mutual exclusion is **retained for `ask_user_question_kandev`-backed
clarifications** — that path does not change. For an elicitation-backed question the
selection is *retained in the answer record* when the operator also types, because
Verified inputs §C establishes by execution that the bridge itself defines a
precedence (custom answer wins over the selection, trimmed). A precedence rule no
operator can trigger is dead contract, and "Postgres — actually, CockroachDB" is a
real interaction. The existing visual language already communicates it: options
visually deselect while the custom input is active, which is exactly "your typed
answer will be used". **No new user-facing string is required**, so this adds no i18n
surface; the change is that the committed record keeps the selection instead of
clearing it. What goes on the wire is unchanged and already specified — AC-12i omits
the question's own key in this case.

**3. A zero-option question.** A question with no options — mapping rule 4, or a
rule-1/2 question whose enum came back empty (D10) — renders the free-text control
alone, with no empty option list and no dead affordance, and is submittable. This is
the shape that carries MCP elicitation forwarding, so an unanswerable card here would
restore that behaviour in name only, which is the very failure rule 4 was added to
prevent.

Everything else about the overlay is unchanged, and AC-57 pins that as a no-change
assertion over the existing `ask_user_question_kandev` path.


### Runtime flag

Registered in `internal/runtimeflags/registry.go` per `/runtime-feature-flags`:

| Field | Value |
|---|---|
| `Key` | `features.acpElicitation` |
| `EnvVar` | `KANDEV_FEATURES_ACP_ELICITATION` |
| `Kind` | `KindFeature` |
| `Stability` | `StabilityExperimental` |
| `RiskLevel` | `RiskMedium` |
| `RestartRequired` | `true` |
| `Mutable` | `true` |

`profiles.yaml` sets `prod: "false"`, `dev: "false"`, `e2e: "false"`. E2E specs
that exercise elicitation set the variable per-spec.


## State machine — elicitation lifecycle

| State | Trigger | Next |
|---|---|---|
| — | `elicitation/create` arrives, maps to ≥1 question | pending; clarification message created; session shows `pending_action: "clarification"` |
| — | request is not `FormSession`, is malformed, or maps to 0 questions | declined immediately; no operator surface |
| pending | operator submits answers | accepted; the open `session/prompt` continues |
| pending | operator rejects the bundle | declined; the agent is told the user skipped; the turn continues |
| pending | turn cancelled, session stopped, or agentctl shutting down | cancelled; the agent aborts the tool call |


## Determinism, concurrency, and boundaries

### D1 — Question ordering is NOT declaration order, because that order does not survive

This is the one an implementation is most likely to get wrong. The bridge emits
`requestedSchema.properties` as an ordered JSON object, but the pinned Go binding
types it as `Properties map[string]any` (`types_gen.go:8704`). Go map iteration
order is deliberately randomized, so **declaration order is destroyed before any
Kandev code sees it**. An implementation that ranges over `Properties` renders the
operator's questions in a different order on every request.

Questions are therefore ordered by a **total, deterministic sort over the property
key**, defined as:

1. Split each key at its **maximal trailing run of digits**: the numeric part is
   the longest digit suffix, and the prefix is *everything before it*, whatever
   that contains. `question_10` → `"question_"` + `10`; `apiKey` → `"apiKey"` +
   none. The prefix is not required to be digit-free — it is simply the
   remainder. `step1_field2` → `"step1_field"` + `2`, **not** `"step"` + an
   unparseable tail; `100_items` → `"100_items"` + none, because the key does not
   *end* in digits; `42` → `""` + `42`. This is spelled out because MCP
   passthrough forwards arbitrary property names untouched (Verified inputs §C),
   so keys with interior digits, leading digits, or no digits at all are all
   reachable, and a "leading non-digit prefix" reading would leave the
   interior-digit case with no valid parse.
2. Compare prefixes bytewise. Lower sorts first.
3. When prefixes are equal, a key with a trailing number sorts before one
   without; two numeric suffixes compare **numerically**, so `question_2`
   precedes `question_10` — a plain bytewise sort gets this backwards, which is
   the specific trap this rule exists to close.
4. Tiebreak: full key, bytewise. Distinct map keys cannot tie, so this branch is
   unreachable; it is stated to make the order total rather than partial.

Option order is a separate matter and needs no rule: `oneOf` and `anyOf` are JSON
**arrays**, which decode to `[]any` with order intact. Options render in array
order and are never sorted.

### D2 — Companion resolution, including its degenerate cases

A custom-answer companion is located by its `_meta._askUserQuestionCustomAnswer.questionId`
marker (never by suffix — see Verified inputs §C for the MCP proof case).
Companions are removed from the property set **before** the D1 sort, so they never
occupy an ordinal position. Two degenerate shapes, both named rather than left open:

- A companion whose `questionId` names no question property present in the
  schema is **ignored**, and its own property does not become a question.
- If two companions name the same question, the one whose own key sorts first
  under D1 wins and the other is ignored.

### D3 — Concurrency on one session

Nothing serializes elicitations. Two may be in flight for one session — parallel
tool calls, or an MCP elicitation arriving while an AskUserQuestion is open.

- Each `elicitation/create` gets its **own** pending record and is answered
  **independently**. Answering one never settles another.
- `pending_action` continues to follow the existing latest-unresolved rule
  (`GetPendingActionsBySessionIDs`). This spec does not extend that rule, so a
  session with two pending elicitations projects one `pending_action` exactly as
  a session with two pending clarifications does today.

### D4 — The clarification store's dedup must not apply to elicitations

`Store.CreateRequest` deduplicates: an existing pending entry for the same
session with identical normalised questions causes the existing pending ID to be
returned with `isNew=false`. That is correct for `ask_user_question_kandev`,
where the question text *is* the identity.

It is **wrong** for an elicitation, whose identity is the JSON-RPC request. Two
distinct `elicitation/create` requests that happen to carry identical question
text would collapse onto one pending record; answering it would settle one ACP
request and **leave the other blocked until its timeout**. Elicitation-backed
requests therefore bypass that dedup and always create a new pending record.

### D5 — Answer idempotency and racing operators

- The first response to a pending elicitation wins. Any later response for the
  same pending ID — a retried submission, a second browser tab, two operators — is
  rejected without touching the ACP request.
- **agentctl is the single point of truth for this.** The backend never decides
  locally that an elicitation-backed pending is already answered; it forwards every
  submission and reports agentctl's verdict. That is what makes the bullet below
  observable rather than aspirational, and it is specified in *Answering an
  elicitation — the backend half*.
- The ACP request is answered **exactly once**. The
  `agent.elicitations.respond` action returns a WS error with
  `ws.ErrorCodeNotFound` for an unknown *or already-settled* pending ID, so a
  duplicate delivery is distinguishable from a lost one — and distinguishable in
  turn from `ErrAgentStreamNotConnected`, which means the answer never arrived.
- A response arriving after the agent already cancelled the request is discarded;
  the wire already carries `cancel`.


## Scenarios

- **AC-01** — **GIVEN** `features.acpElicitation` is on, **WHEN** Kandev
  initializes an ACP session, **THEN** the `initialize` request's
  `clientCapabilities` contains `elicitation.form` as an object.
- **AC-02** — **GIVEN** `features.acpElicitation` is off, **WHEN**
  `clientCapabilitiesForAgent` is called for **each of** `acpcompat.CursorAgentID`
  and `claude-acp`, **THEN** each JSON-serialized `clientCapabilities` equals its
  **own** checked-in fixture, pinned to the pre-change output byte for byte, and
  neither contains an `elicitation` key. Two fixtures, not one: the returned value
  is agent-scoped — `acpcompat.ClientCapabilityMeta` gives cursor-agent a
  different `_meta` map from every other agent, and
  `adapter_client_capabilities_test.go` already distinguishes those two agents —
  so a single fixture would leave the cursor shape unpinned and silently
  under-cover the flag-off guarantee this AC exists to protect.
- **AC-02a** — **GIVEN** `features.acpElicitation` is **on**, **WHEN**
  `clientCapabilitiesForAgent` is called for `acpcompat.CursorAgentID`, **THEN**
  the result is that agent's flag-off fixture plus `elicitation.form` and nothing
  else — proving the capability is added *orthogonally* to the existing
  agent-scoped `_meta`, rather than replacing it.
- **AC-03** — **GIVEN** an ACP client connection, **WHEN** the agent sends
  `elicitation/create` with a `mode: "form"`, `sessionId`-scoped request whose
  schema declares one `oneOf` string property, **THEN** the SDK's optional-
  interface assertion (`client_gen.go:36-40`) resolves and the request reaches
  Kandev's handler, rather than being answered `MethodNotFound` — the regression
  guard for the wiring that makes every other Half A criterion reachable.
- **AC-04** — **GIVEN** a form elicitation carrying one `oneOf` property with
  three options plus its custom-answer companion property, **WHEN** it is mapped,
  **THEN** exactly one question with three options is produced, the option IDs
  equal the enum `const` values, and the companion property produces no separate
  question.
- **AC-05** — **GIVEN** a form elicitation with two questions, **WHEN** it is
  mapped, **THEN** each question's prompt is its property's `description` and the
  request `message` is not used as a prompt.
- **AC-06** — **GIVEN** a form elicitation with exactly one question and no
  property `description`, **WHEN** it is mapped, **THEN** that question's prompt
  is the request's `message`.
- **AC-07** — **GIVEN** a multi-select property (`type: "array"`,
  `items.anyOf`), **WHEN** it is mapped, **THEN** one multi-select question is
  produced with one option per `anyOf` entry.
- **AC-08** — **GIVEN** a mapped elicitation, **WHEN** the pending record is
  created, **THEN** an agent-authored `clarification_request` message exists for
  the session with `requests_input: true` and a non-empty
  `metadata.pending_id`, and `GetPendingActionsBySessionIDs` reports
  `clarification` for that session.
- **AC-09** — **GIVEN** a pending elicitation on a `RUNNING` session, **WHEN**
  the task row renders, **THEN** `data-testid="task-state-waiting-for-input"` is
  present and neither `task-state-running` nor `task-state-background-running`
  is.
- **AC-10** — **GIVEN** a pending elicitation, **WHEN** the operator selects the
  option whose label is `L` and submits, **THEN** the ACP response is
  `{"action": "accept", "content": {"question_0": "L"}}` using the schema's own
  property key.
- **AC-11** — **GIVEN** a pending elicitation whose schema declares a
  custom-answer companion property named `answer_freeform` carrying
  `_meta._askUserQuestionCustomAnswer.questionId`, **WHEN** the operator submits
  free text `T` for that question, **THEN** the response content carries `T` at
  the key `answer_freeform` and not at any suffix-derived key.
- **AC-12** — **GIVEN** a pending multi-select elicitation, **WHEN** the operator
  selects two options, **THEN** the response content carries a JSON array of the
  two labels at that question's key.
- **AC-12a** — **GIVEN** a pending elicitation, **WHEN** the operator submits the
  free text `"  CockroachDB  "` for a question that also has a selection,
  **THEN** the response content carries that question's companion key with the
  value `"CockroachDB"` — trimmed. The earlier wording also asserted what "the
  agent-side fold-back records", which lives inside an external Node process and no
  Kandev test surface can observe; that the bridge lets the custom answer override
  the selection is established by execution in Verified inputs §C and is the
  *reason* AC-12i omits the question's own key, not a behaviour Kandev can assert.
  The observable half is asserted here and in AC-12i/AC-12j.
- **AC-12b** — **GIVEN** a pending elicitation, **WHEN** the operator submits the
  form having answered nothing, **THEN** Kandev sends
  `{"action": "accept", "content": {}}` rather than `decline`. Both are
  byte-identical once the bridge folds them back (Verified inputs §C), so this
  fixes one deterministic shape rather than leaving the choice to the
  implementer; `decline` remains reserved for an explicit reject (AC-14).
- **AC-12c** — **GIVEN** the refusal-fallback dialog delivered as a form
  elicitation — one property named `choice`, two options with `const` values
  `retry_fallback` and `cancelled`, no `title` and no custom-answer companion —
  **WHEN** it is mapped, **THEN** exactly one two-option question is produced
  whose prompt is the request's `message`, and selecting the first option yields
  `{"action": "accept", "content": {"choice": "retry_fallback"}}`.
- **AC-12d** — **GIVEN** a form elicitation whose schema declares eleven question
  properties `question_0` … `question_10`, **WHEN** it is mapped **one hundred
  times**, **THEN** every mapping yields the identical question order, and
  `question_2` precedes `question_10` (D1). A test that maps once proves nothing
  here: the defect is Go's randomized map iteration, so the repetition is the
  assertion.
- **AC-12e** — **GIVEN** a form elicitation with a companion property whose
  `questionId` names no property in the schema, **WHEN** it is mapped, **THEN**
  that companion contributes no question of its own and is ignored (D2).
- **AC-12f** — **GIVEN** two `elicitation/create` requests on one session
  carrying **identical** question text, **WHEN** both are mapped, **THEN** two
  distinct pending records exist, and answering the first leaves the second still
  awaiting a response rather than settling it (D4).
- **AC-12g** — **GIVEN** a pending elicitation that has already been answered,
  **WHEN** a second response for the same pending ID is submitted, **THEN** the
  submission is still forwarded as `agent.elicitations.respond`, agentctl answers
  it with a WS error carrying `ws.ErrorCodeNotFound`, no second ACP response is
  sent, and the agent observes exactly one response (D5). The backend does **not**
  adjudicate this locally — see *Answering an elicitation — the backend half*.
  (Earlier drafts of this AC said "the respond route returns `404`". That wording
  predates the correction that removed the non-existent `/api/v1/acp/...` route;
  the assertion is, and always was, the duplicate-suppression behaviour, which now
  names the transport that actually carries it.)
- **AC-13** — **GIVEN** a pending elicitation raised inside an open
  `session/prompt`, **WHEN** the operator submits an answer, **THEN** no
  additional `session/prompt` request is sent to the agent, the original prompt's
  generation counter is unchanged, and that original request is the one that
  subsequently returns a `StopReason`.
- **AC-14** — **GIVEN** a pending elicitation, **WHEN** the operator rejects the
  bundle, **THEN** the ACP response is `{"action": "decline"}`.
- **AC-15** — **GIVEN** a pending elicitation, **WHEN** the turn is cancelled,
  **THEN** the ACP response is `{"action": "cancel"}` and the pending operator
  surface is withdrawn.
- **AC-16** — **GIVEN** an `elicitation/create` whose `mode` is `"url"`,
  **WHEN** it is handled, **THEN** the response is `{"action": "decline"}` and no
  clarification message is created.
- **AC-17** — **GIVEN** an `elicitation/create` carrying a `requestId` scope
  instead of a `sessionId`, **WHEN** it is handled, **THEN** the response is
  `{"action": "decline"}`.
- **AC-18** — **GIVEN** an `elicitation/create` whose `requestedSchema.properties`
  is empty, contains only unrecognized shapes, or is absent, **WHEN** it is
  handled, **THEN** the response is `{"action": "decline"}` and no operator
  surface is created.
- **AC-19** — **GIVEN** a non-Claude ACP agent from the registry that never sends
  `elicitation/create`, **WHEN** the same scripted session is run once with the
  flag on and once with it off, **THEN** the two runs agree on: the ordered
  sequence of JSON-RPC method names exchanged, the session's final
  `TaskSessionState`, the ordered list of persisted message types, and the
  resulting `pending_action`. The `initialize` request's `clientCapabilities` is
  the only permitted difference.
- **AC-20** — **GIVEN** `features.acpElicitation` is off, **WHEN** an agent sends
  a well-formed `mode: "form"`, `sessionId`-scoped `elicitation/create` anyway,
  **THEN** the response is `{"action": "decline"}` and no clarification message,
  pending action, or notification is produced.
- **AC-20a** — **GIVEN** a pending elicitation whose agent connection has dropped,
  **WHEN** the operator submits an answer, **THEN** the submission fails with
  `ErrAgentStreamNotConnected`, the answer is not reported as delivered to the
  agent, and no ACP frame is sent for it.
- **AC-20b** — **GIVEN** a pending elicitation, **WHEN** the operator's answer is
  delivered, **THEN** it travels as the WebSocket action
  `agent.elicitations.respond` on the existing agent stream and **no HTTP request
  is made to any `/api/v1/acp/...` path** — that route group does not exist on the
  agentctl router.
- **AC-04a** — **GIVEN** a form elicitation carrying a single property
  `{"apiKey": {"type": "string"}}` with no `oneOf` and no custom-answer companion
  — the MCP passthrough shape captured in Verified inputs §C — **WHEN** it is
  mapped, **THEN** exactly **one** free-text question with **zero** options is
  produced and the request is **not** declined. This is the regression guard for
  MCP elicitation forwarding: under the pre-correction mapping this shape produced
  zero questions and was declined.
- **AC-11a** — **GIVEN** the pending elicitation from AC-04a, **WHEN** the
  operator submits the free text `T`, **THEN** the ACP response is
  `{"action": "accept", "content": {"apiKey": "T"}}` — the free text goes to the
  question's **own** key, because it declares no companion.
- **AC-12h** — **GIVEN** a pending elicitation with three questions where the
  operator answers only the second, **WHEN** the response is built, **THEN**
  `content` contains **exactly one** key — the answered question's — and the two
  unanswered questions contribute **no** key at all: not `null`, not `""`.
- **AC-12i** — **GIVEN** a pending elicitation whose question declares a companion,
  **WHEN** the operator both selects an option **and** submits free text, **THEN**
  `content` carries the companion key with the trimmed free text and **omits the
  question's own key**, since the bridge's fold-back would discard the selection
  anyway (Verified inputs §C).
- **AC-12j** — **GIVEN** a pending elicitation, **WHEN** the operator submits free
  text consisting only of whitespace for a question with no selection, **THEN**
  that question contributes no key to `content`.
- **AC-55** — **GIVEN** a pending elicitation carrying a rule-2 multi-select
  question whose options are declared `["dev", "staging", "prod"]`, **WHEN** the
  operator selects `prod` first and then `dev`, **THEN** the answer record holds
  both, and the response content carries `["dev", "prod"]` at that question's key —
  **declaration order, not click order**. Selecting a second option must not
  replace the first, which is what today's single-select overlay does.
- **AC-56** — **GIVEN** the pending elicitation from AC-04a (one rule-4 question,
  zero options), **WHEN** the answer surface renders it, **THEN** a free-text
  control is present and submittable, no empty option list is rendered, and
  submitting free text `T` produces `{"action": "accept", "content": {"apiKey": "T"}}`.
- **AC-57** — **GIVEN** a clarification raised by `ask_user_question_kandev` rather
  than by an elicitation, **WHEN** the operator interacts with the answer surface,
  **THEN** its behaviour is unchanged from today for every combination of
  single-select, custom text, and skip — including the existing mutual exclusion
  between a selection and custom text. A no-change assertion: the three additions in
  *The answer surface* apply to elicitation-backed questions and must not alter the
  path that already ships.
- **AC-60** — **GIVEN** an `elicitation/create` that maps to at least one question,
  **WHEN** the pending record is created, **THEN** the clarification pending record
  carries **agentctl's** `pending_id` verbatim rather than a backend-minted one,
  that id is globally unique across agentctl instances, and a second
  `elicitation/create` carrying identical question text on the same session creates
  a **second** record rather than being collapsed into the first (D4, AC-12f).
- **AC-61** — **GIVEN** an elicitation-backed pending that has already been
  answered, **WHEN** a second submission for it arrives at the backend, **THEN** the
  backend still sends `agent.elicitations.respond` for it — it does **not**
  short-circuit on local state — and the `ws.ErrorCodeNotFound` the operator sees
  originates from agentctl. This is the assertion that distinguishes the specified
  design from the one the existing `Store.Respond` path would produce, where the
  duplicate is rejected locally and agentctl never hears about it.
- **AC-64** — **GIVEN** a pending elicitation that the operator never answers,
  **WHEN** the elicitation answer deadline (`KANDEV_ELICITATION_ANSWER_TIMEOUT`)
  elapses, **THEN** the ACP request is answered `{"action": "decline"}` — not
  `cancel`, and not left blocked — the turn continues, and the deadline used is
  strictly less than `KANDEV_ACP_IDLE_TIMEOUT` so the decline is attributable to
  Kandev rather than to a connection teardown.

## Permissions

Scoped by the existing rules; grants no new access. The elicitation answer reaches
agentctl only through `lifecycle.Manager`, which applies the session-access guard used by
`RespondToPermission`. Notification bodies are unchanged and still contain the task title
but never the question text.

## Failure modes

| Condition | Behaviour |
|---|---|
| Agent does not advertise or does not use elicitation | Unchanged. The capability is inert. |
| `elicitation/create` arrives with `mode: "url"`, a request scope, or an unparseable body | Decline. Not surfaced to the operator. |
| `requestedSchema` maps to zero questions | Decline. |
| `requestedSchema` contains a property type the mapping does not recognize | That property is ignored; recognized properties still form the question set. |
| Agent cancels the ACP request while the operator is deciding | The pending record is settled as cancelled and its operator surface is withdrawn. |
| Operator never answers | The handler stays blocked until the **elicitation answer deadline** (`KANDEV_ELICITATION_ANSWER_TIMEOUT`, default `55m`), then answers `decline` — not `cancel`, because the user skipping is what happened and `cancel` would abort the tool call. The turn continues rather than hanging. The deadline is deliberately below the ACP idle timeout's `1h` default (`KANDEV_ACP_IDLE_TIMEOUT`, `agentctl/server/config/config.go`) so this decline is Kandev's own act rather than a connection teardown; it does **not** inherit the clarification store's 2h default, which is longer than both and would lose that race. Asserted by AC-64. **See open finding F7: nothing carries this decline back to the backend.** |
| Agent stream is down when the operator submits, but the pending is still live | The submission is forwarded and fails with `ErrAgentStreamNotConnected`. The operator is told it did **not** reach the agent, the pending record **stays open**, and no ACP frame is sent (AC-20a). |
| agentctl restarts, or the agent connection drops, with an elicitation pending | The ACP request died with the connection. The pending record is gone and the stale clarification message is marked `agent_disconnected` by the existing `clarification.Canceller`. A submission arriving after that point is accepted by the UI and recorded on the message as a record of what the operator chose, then discarded rather than delivered. The distinguishing fact against the row above is whether the pending still exists, not whether the stream is up. |
| `features.acpElicitation` off | Byte-identical capabilities to today, and the handler declines every request it receives, so an agent that sends an unadvertised elicitation cannot reach the operator. |

## Persistence guarantees

Nothing added by this feature survives a restart of either process. A pending elicitation
is bound to the open ACP request and dies with the agentctl connection; it is never
written to disk and never replayed. The clarification **message** it created is durable,
like any `ask_user_question_kandev` message, and is reconciled by the existing
`clarification.Canceller` when its turn ends. No new column, table, or migration.

## Out of scope

- URL-mode elicitation and the `elicitation/complete` client method. Not reachable while
  only `form` is advertised (Verified inputs §B).
- Rendering an option's `preview` from `_meta._claude/askUserQuestionOption`.
- Replacing `ask_user_question_kandev`. Both paths coexist and land on the same operator
  surface.
- De-duplicating an elicitation against a concurrent `ask_user_question_kandev` bundle
  carrying the same questions. The two paths are never merged.
- Retrying a failed elicitation response delivery, and retrying a submission that failed
  with `ErrAgentStreamNotConnected`. Kandev queues nothing and retries nothing on the
  operator's behalf.
- **Multi-select, coexisting free text, and zero-option rendering for
  `ask_user_question_kandev` clarifications.** *The answer surface* scopes all three to
  elicitation-backed questions; AC-57 exists to stop this feature answering that question
  by accident.
- Everything in the sibling spec `docs/specs/disambiguate-waiting/spec.md` — the parked
  projection, the liveness probe, and the notification deferral. This spec changes no
  notification timing.

## Open findings — the work list for this spec's first Spec Review

These were raised against the combined spec in Spec Review rounds 2 and 3, verified in the
tree, and **never closed**, because the card was split before they were worked. They are
reproduced here so the evidence is not lost. Anchors are stable section names, not line
numbers.

- **F3 — (AC-12b, AC-12h, AC-04a, AC-11a, AC-56 | §"The answer surface" + §"API surface —
  Response mapping", `internal/clarification`#request-validation, internal-consistency).**
  The reused clarification machinery **rejects three shapes these ACs require**, in the Go
  layer. `internal/clarification/handlers.go:206-209` rejects any question with
  `len(Options) < 2` or `> 6` — but AC-04a, AC-11a and AC-56 all require a **zero-option**
  rule-4 question. `validateRespondAnswers` (`handlers.go:339-348`) enforces an
  **all-required gate**, and `types.go` documents `Questions // 1-N questions, all
  required`, `Response` "has exactly one entry per question", and `SelectedOptions //
  (single-choice ⇒ at most one)` — but AC-12h requires a partial answer and AC-12b
  requires an empty one. The frontend agrees with the backend, not with the spec:
  `allAnswered` gates submit (`disabled={!allAnswered || isSubmitting}`), so AC-12b's
  `accept {}` is **unreachable by any operator gesture** — the only zero-answer path is
  Skip, which is the reject AC-14 maps to `decline`. *The answer surface* section fixed
  only the React layer and never mentions the Go validation.
- **F5 — (AC-01, AC-02a, AC-20 | §"ACP client capability" + §"Runtime flag",
  `internal/agentctl`#clientCapabilitiesForAgent, forced-to-invent).** The flag is
  registered in a process that **cannot reach the process that reads it**, and no
  propagation is specified. Both flag-gated behaviours live in agentctl; the flag lives in
  the backend's `internal/runtimeflags/registry.go`, whose precedence includes a
  **backend-only SQLite override** tier. Verified, all three negative:
  `grep -rn "runtimeflags" internal/agentctl/ --include="*.go"` → 0;
  `grep -rn "KANDEV_FEATURES" internal/agentctl/ --include="*.go"` → 0;
  `Client.Initialize(ctx, clientName, clientVersion)` carries no config. An env-injection
  answer silently makes the flag's declared `Mutable: true` Settings toggle a **no-op** for
  Docker/SSH/Sprites executors. AC-01, AC-02a and AC-20 are unit-level and pass regardless.
- **F6 — (AC-55, AC-57 | §"Data model" + §"Stream event",
  `internal/clarification`#Question, forced-to-invent).** Nothing on the wire tells the
  answer surface which branch to take. `clarification.Question` is exactly
  `{ID, Title, Prompt, Options}` (`types.go:18-23`) — no arity, no provenance. The overlay
  must render multi-select for rule-2 questions (AC-55) and relax the selection/free-text
  mutual exclusion **only** for elicitation-backed questions (AC-57). The *Marking*
  paragraph makes the record elicitation-backed inside the backend store and never says
  the marker is projected to the client.
- **F7 — (AC-64, AC-15 | §"Failure modes" + §"Stream event", AC-completeness).** Every
  path where **agentctl** settles a pending on its own is invisible to the backend: the
  55m timeout decline, the agent cancelling the request, and agentctl shutting down. The
  *Stream event* section defines exactly one event, in the create direction. The backend
  meanwhile holds a durable `clarification_request` on the store's 2h default, so the card
  keeps showing "a human is required" for a question nobody can answer. *Persistence
  guarantees* does not save it — it reconciles "when its turn ends", and AC-64's point is
  that the turn does **not** end.
- **F9 — (AC-08 | Requirements, 3rd bullet, AC-completeness).** **No AC observes
  `session.clarification_requested`.** The requirement names it as one of five things an
  elicitation must produce, and §F establishes it is the notification that is **on by
  default**. AC-08 asserts only the persisted message and `GetPendingActionsBySessionIDs`.
  A build that creates the message but never dispatches the notification passes every AC.
- **F10 — (§"Answering an elicitation (backend → agentctl)" | forced-to-invent).** The
  `agent.elicitations.respond` payload is **never validated** against the pending's mapped
  questions, and D5/AC-61 deliberately remove the backend's local adjudication — so
  agentctl is the only validator and is never told to validate. Undefined: an `answers[]`
  entry whose `question_id` matches no question; `selected_options` with two entries for a
  rule-1 single-select; a `selected_options` value that is not a mapped option `const`.
  Sub-item: the payload carries `rejected: bool`, and nothing says which overlay control
  sets it.
- **F12 — (AC-55 | §"The answer surface" → multi-select, shadow-paths).** The
  declaration-order rule is stated for the **UI** but asserted at the **wire**. Nothing
  says agentctl — the process that builds `content`, and which holds the options in
  declaration order — re-normalises, so the invariant is enforced in one submitting
  surface while AC-55 passes in a unit test.
- **F13a — (§"Runtime flag" / timing | premise).** `KANDEV_ELICITATION_ANSWER_TIMEOUT` is
  the one duration this spec owns and it is **read by agentctl**, not the backend. State
  that explicitly; the combined spec's timing table promised "a declared home" per key and
  never gave one, and the other four keys went to the sibling spec.

## Notes for implementation

- All new operator-facing copy goes through `t()` / `<Trans>`; a hardcoded literal on a
  changed line fails `pnpm run i18n:ratchet`.
- E2E coverage needs a `cmd/mock-agent` scenario that issues `elicitation/create`. The
  existing `clarification` scenarios drive the **MCP tool**, not ACP elicitation
  (`cmd/mock-agent/mcp_client.go`), so a new scenario is required. The §K captured frame is
  a ready-made fixture. The flag is restart-required, so set it per-spec via
  `backend.restart(env)`.
- `agent.elicitations.respond` is a WS action on the **existing** agent stream: a `case` in
  the `server/api/agent.go` action switch and a `Client` method using `sendStreamRequest`.
  There is an existing completeness test over the action list (`agent_test.go`), so the new
  action must be added to it. Do **not** add an `/api/v1/acp` HTTP group; none exists.
- The answer-surface work lands in `clarification-input-overlay.tsx` and
  `clarification-overlay-parts.tsx`; `clarification-custom-input.test.tsx` is the natural
  home for AC-55/AC-56/AC-57.
- `apps/backend/bin/acpdbg` advertises no elicitation capability. Adding
  `elicitation: {form: {}}` to its client capabilities would make the §K A/B re-runnable
  in-tree, so the next bridge bump can be regression-checked in one command. Not required
  by any AC.
