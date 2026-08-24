---
status: draft
created: 2026-08-16
revised: 2026-08-20
owner: nova28
---

# External Question Answering — authorized, discoverable, idempotent clarification resolution

> **Revision note (2026-08-20, spec round 6).** This spec was first written against
> `main` on which nothing made clarification resolution atomic. While it was in Build,
> upstream commit `7e2c4ae84` ("fix: enforce active clarification lifecycle", #2669) merged
> its spec, `docs/specs/tasks/requirements/clarification-active-lifecycle.md`, which landed an
> atomic current-turn claim and a turn-supersession lifecycle. That work **supersedes this
> spec's entire claim mechanism** — the `clarification_resolutions` table, `ResolveBundle`'s
> own claim SQL, and the workflow-guard changes are all retired here rather than merged
> alongside it. What remains, and what this revision specifies, is the half upstream did not
> build: **authorization**, **out-of-band discovery**, **idempotent replay for a losing
> caller**, and the MCP surface. See *Upstream baseline* for what is already done, and
> *Retired criteria* for the identity of every rule this revision withdraws and why.

## Why

When a Kandev agent calls `ask_user_question_kandev`, the resulting question is answerable in
exactly one place: a popup rendered over the chat input of that task's session. Everything behind
that popup — the durable record, the REST endpoints, the resume path — already exists, but nothing
outside the browser can discover a question or answer it.

This spec makes answering an out-of-band, first-class API. Two new MCP tools on the **external**
surface let a third party (or an agent running outside Kandev) list the questions it may see and
answer one.

Three defects sit under that surface. Upstream has since fixed one of them; the other two are this
spec's load-bearing deliverable.

1. **Atomicity — FIXED UPSTREAM, not by this spec.** `Store.WaitForResponse` deletes the in-memory
   entry the moment it observes `done`, so the old 409 duplicate guard only covered a second answer
   arriving before the winner's waiter woke. Upstream `7e2c4ae84` replaced that with
   `Repository.CompleteActiveClarificationBundle`, a single-transaction row-level claim whose own
   doc comment states *"Exactly one concurrent responder can transition the rows."* This spec
   **consumes** that primitive and adds no second one.

2. **No ownership check — STILL OPEN, this spec closes it.** `POST /api/v1/clarification/:id/respond`
   takes a bare `pending_id`. The global auth middleware attaches identity, but the clarification
   handlers are constructed with only store/repo/message dependencies and never read the context
   identity. Verified against `origin/main` on 2026-08-20: `internal/clarification/handlers.go`
   contains **no** call to any `Authorize*` helper, so the hole is exactly as wide today as when
   this spec was first written, and it covers `GET /:id`, `GET /:id/wait`, `POST /:id/respond` and
   `POST /:id/cancel` alike. Under enforced auth, any PAT holder who learns a `pending_id` can read
   and answer another user's question. This is the one clarification path that bypasses
   `task/service` entirely — `get_task_conversation_kandev`, which reaches the same messages, *is*
   scoped (`ListMessagesPaginated` calls `AuthorizeSessionAccess`).

3. **A losing caller is told the wrong thing — STILL OPEN, this spec closes it.** Upstream's
   `httpRespond` returns **409 "clarification request is no longer active"** when the claim is lost
   (`internal/clarification/handlers.go`, `claimClarificationResponse`). For a human clicking Submit
   twice that is survivable; for a machine answerer racing the browser it is not, because the caller
   cannot tell "someone else answered, here is what they said" from "this bundle is gone". Adding an
   MCP answerer makes that distinction load-bearing.

Adding a second, unauthorized, non-idempotent writer to a surface a third party can reach is the
thing this card must not do. Fixing (2) and (3) on top of upstream's (1) is the deliverable.

## Upstream baseline — already on `main`, not built by this spec

Every statement here was verified by direct read of `origin/main` on 2026-08-20 and is cited so no
builder re-derives it. A builder SHALL treat this section as fact, not as work.

- **`Repository.CompleteActiveClarificationBundle(ctx, pendingID, status, responses)`**
  (`internal/task/repository/sqlite/message_clarification_response.go`) is the claim. It runs one
  transaction: `claimActiveClarificationBundle` issues a single `UPDATE ... SET status='responding'`
  guarded by `pending_id`, `type='clarification_request'`,
  `COALESCE(metadata.status,'') IN ('','pending')`, a **non-terminal session** predicate, a
  **current-turn** predicate, and a malformed-bundle `NOT EXISTS` guard; then it loads the claimed
  rows and writes the terminal metadata. It returns `(messages, claimed bool, error)`.
  `claimed == false` means someone else won or the bundle is no longer active.
- **Terminal status domain is `answered` or `rejected` only.** The function rejects any other value
  with `invalid clarification terminal status`. There is no `cancelled` claim.
- **What the claim writes per message**: `metadata.status = <terminal>`,
  `metadata.response_delivery_pending = true`, and — on `answered` only — `metadata.response = <the
  Answer value for that question_id>`. This is the record from which a losing caller's replay is
  reconstructed (R2).
- **Every claimed message must carry a non-empty `question_id`**;
  `completeClaimedClarificationMessages` returns `clarification message %s is missing question_id`
  otherwise, aborting the transaction.
- **`RestoreActiveClarificationBundle`** rolls a claimed bundle *back* to pending when its detached
  resume could not be dispatched. Upstream's failure philosophy is **roll back and stay answerable**,
  not *record a failure and stay claimed*.
- **`ExpireActiveClarificationBundle`** and **`DetachActiveClarificationMessagesBySessionID`** are
  separate primitives writing `expired` and `agent_disconnected` respectively.
- **The workflow guard is already correct and already widened.**
  `Service.sessionHasPendingClarification` now calls
  `Repository.FindActiveClarificationMessagesBySessionID`, whose predicate is
  `COALESCE(metadata.status,'') IN ('','pending')` **joined to the session's current turn**. The
  function this spec's first version proposed to modify,
  `FindPendingClarificationMessagesBySessionID`, **no longer exists**.
- **Active is upstream's word and upstream's rule.** Per its approved spec: *"A clarification bundle
  is active only when at least one row in the bundle is pending and the bundle belongs to the
  session's current turn."* A **detached** bundle (`agent_disconnected=true`) stays active and
  answerable. A **superseded** bundle (older turn) and a bundle whose **session reached a terminal
  state** are not active; superseded rows *"remain transcript history but cannot drive a chat
  overlay, task/session pending projection, workflow guard, turn-completion detach pass, or late
  agent resume."*
- **Upstream authorization: none.** Confirmed absent, per *Why* item 2.

## Terminology

- **Bundle** — one clarification request: 1..4 questions sharing one `pending_id`.
- **Active** — upstream's rule, quoted above. This spec adopts it verbatim as its membership test
  and does not restate it in its own words (D4).
- **Resolution** — the terminal outcome of a bundle: `answered` or `rejected`.
- **Claim** — `CompleteActiveClarificationBundle` returning `claimed == true`. Exactly one caller
  claims a bundle.
- **Winner** — the caller whose claim succeeded. **Loser** — any later caller for the same bundle.
- **Unscoped caller** — no identity in request context (event bus, pollers, orchestrator) or a
  synthetic identity (auth disabled). Matches `callerScope` in `task/service/service_access.go`.

## Data model

**This spec adds no table and no column.** The `clarification_resolutions` table specified by the
first version of this spec is retired in full (see *Retired criteria*); upstream's claim writes its
terminal state into the existing `task_session_messages.metadata`, and that is now the sole
authority on whether a bundle is resolved.

Two identity rules survive from the first version because **authorization still needs them** — they
were never about the claim.

- **M5. Resolving `task_id` and `session_id` for a bundle.** `session_id` is
  `task_session_messages.task_session_id`, which is `NOT NULL` and FK-constrained, so it is always
  present for a bundle that has durable messages. `task_id` is read from
  `task_session_messages.task_id`, which is `TEXT DEFAULT ''` and **can legitimately be empty**:
  `httpCreateRequest` logs a warning and continues with `taskID = ""` when the session lookup fails,
  so such bundles exist in the wild. When the message's `task_id` is empty, the system SHALL resolve
  it from the bundle's session row (`task_sessions.task_id`, declared `NOT NULL`). Without this rule
  an empty-`task_id` bundle would pass `AuthorizeTaskAccess(ctx, "")` to a lookup that fails for
  every scoped caller **including its own owner**, making it permanently unanswerable.
- **M5a. When neither source yields a `task_id`, the bundle is unresolvable.** Reachable only if the
  session row is missing or its `task_id` is the empty string. Such a bundle SHALL be treated as
  not-found (A5) for every **scoped** caller, and SHALL be **omitted from L1 for every caller,
  including an unscoped one**. It remains answerable by `pending_id` by an unscoped caller whenever
  upstream's claim would still accept it, which preserves single-user behavior.
  The listing half of that decision is deliberate and is the one a builder would otherwise invent.
  Listing it for unscoped callers only would require the L1c join to become an outer join, which in
  turn forces L3's mandatory `task_id` to carry some invented value for that one row — and L3 has no
  such value, so an MCP client parsing the field would meet a `null` or an empty string with no
  stated meaning. Omitting it keeps the L1c join inner, keeps L3's `task_id` always a real task ID,
  and costs nothing that matters: the bundle is still visible to a human in the chat transcript and
  still reachable through `get_task_conversation_kandev`.

### Existing state, unchanged in shape

- **In-memory** `PendingClarification` keeps its role: it is the delivery channel that unblocks a
  still-waiting agent inside its own turn. Upstream already removed its duty as duplicate guard.
- **Durable per-question messages** (`type = 'clarification_request'`) keep their metadata shape
  verbatim: `pending_id`, `question_id`, `question`, `question_index`, `question_total`, `context`,
  `status`, `response` once answered, plus upstream's `agent_disconnected` and
  `response_delivery_pending`. **This spec adds no metadata key and changes no existing one.**

## The resolution operation

A single service-layer operation is the sole entry point for the REST respond endpoint and the MCP
answer tool. It does **not** implement a claim; it authorizes, validates, and delegates to upstream's.

`ResolveBundle(ctx, pendingID, outcome) -> (Resolution, claimed bool, error)`

Ordered steps:

1. **Resolve identity of the bundle.** Read the bundle's durable `clarification_request` messages by
   `pending_id`, deriving `session_id` and `task_id` per M5. If no messages exist, return not-found.
2. **Authorize.** Call `TaskService.AuthorizeTaskAccess(ctx, taskID)`. A denial returns not-found.
3. **Validate the outcome** against the bundle's question set (N6–N8b, per N8c).
4. **Claim, by delegation.** Call `CompleteActiveClarificationBundle(ctx, pendingID, terminal,
   responses)`. `claimed == true` → this caller won; `claimed == false` → R2; a returned error → R4a.
5. **Winner only:** deliver to the in-memory waiter and publish, using upstream's existing
   `RespondWithDeliveryConfirmation` / restore path unchanged (R7).

Steps 1–3 are this spec's addition. Step 4 is upstream's. Step 5 is upstream's, reached unchanged.

- **P1. There SHALL be exactly one claim mechanism in the codebase.** `ResolveBundle` SHALL NOT
  insert, update, or read any claim record of its own, and SHALL NOT gate the claim on any
  precondition upstream's predicate does not already enforce. A second mechanism, however carefully
  coordinated, reintroduces the cross-entry-point race this card exists to close: a REST answer
  claiming one way and an MCP answer claiming another can both win, and no existing test would catch
  it because REST-vs-MCP races could not be written before the MCP tool existed.
- **P1a. What step 4 passes to the claim.** `terminal` SHALL be `answered` or `rejected` and nothing
  else; upstream rejects any other value. `responses` SHALL be a map **keyed by `question_id`**, one
  entry per question in the bundle, whose value is that question's answer entry — the shape
  `completeClaimedClarificationMessages` indexes as `responses[questionID]` and writes verbatim to
  `metadata.response`. For a `rejected` outcome `responses` SHALL be nil, because upstream writes no
  `response` key on that path.
- **P1b. The answers passed to the claim SHALL be the N3a-normalized ones, not the caller's raw
  input.** `metadata.response` is what R2b replays to a losing caller, so normalizing after the claim
  — or not at all — would make N3a's byte-identity guarantee hold for the winner's own response and
  break for every replay of it. Normalization is therefore applied between step 3 and step 4.
- **P2. `ResolveBundle` SHALL NOT re-derive activeness.** Whether a bundle is claimable is decided
  **only** by upstream's predicate, inside its transaction. A pre-check in `ResolveBundle` would be
  a TOCTOU window and could disagree with the authoritative predicate; steps 1–3 exist to
  authorize and validate, never to pre-judge the claim.

## Determinism and boundary rules

- **D1. A bundle's `created_at` is the minimum `created_at` across its `clarification_request`
  messages.** The bundle's messages are written in a loop and therefore carry distinct timestamps;
  without this rule L6's ordering is not well defined.
- **D2. Questions within a bundle are ordered by `question_index` ascending, then message
  `created_at` ascending, then message `id` ascending.** The two extra keys are not decoration:
  `questionIndexFromMetadata` returns **0** for a missing or unparseable `question_index`, so a
  legacy or corrupt bundle can present several questions all claiming index 0. Message `id` is a
  primary key, so the composite is total.
- **D3. `metadata.status` on a `clarification_request` message is one of `pending`, `answered`,
  `rejected`, `cancelled`, or `expired`** — the `clarification.Status` constants — or **absent**.
  For membership this spec defers entirely to D4; it does **not** define its own effective-pending
  rule. The distinction matters at one boundary and is stated so it is not invented: upstream's
  predicate is `COALESCE(status,'') IN ('','pending')`, so an **absent** status counts as pending
  but an **unrecognized** one does not. The first version of this spec required the opposite for
  unrecognized values (fail closed). That requirement is withdrawn — see *Retired criteria* G5 —
  because two consumers disagreeing about membership is the defect, and upstream's rule is the one
  the workflow guard, the chat overlay, and the claim all already share.
- **D4. A bundle is *pending* — listable by L1, claimable by step 4, and countable by the workflow
  guard — iff it is ACTIVE per upstream's rule.** There is one membership test in the system and it
  is not this spec's. Concretely, and only as a restatement of the upstream predicate a builder will
  read directly: at least one of the bundle's `clarification_request` messages has
  `COALESCE(metadata.status,'') IN ('','pending')`, **and** the bundle's `turn_id` is the session's
  current turn per `turnAuthorityPredicate`, **and** the session's state is not `completed`,
  `failed`, or `cancelled`.
- **D4a. Legacy bundles fall out of D4 with no backfill and no migration.** Every clarification
  answered before either change is a set of messages carrying a terminal `status`, so the first
  conjunct excludes it; every clarification stranded on an older turn is excluded by the second.
  Nothing this spec adds needs to run against historical data, which is what makes "no migration"
  safe rather than merely cheap.
- **D5. A task with an empty `workspace_id` is visible to every authenticated caller**, matching
  `authorizeTaskID`. Its bundles appear in L1 for everyone. This preserves the pre-auth-row behavior
  used everywhere else in the codebase.
- **D6. Pagination is not a snapshot.** A bundle that becomes inactive between two pages simply
  disappears; because the cursor is a `(created_at, pending_id)` key rather than an offset, no other
  bundle shifts position or is skipped. A bundle created with a `created_at` earlier than an
  already-issued cursor (only reachable via clock adjustment) SHALL NOT be returned to that cursor's
  holder; it is returned on any fresh, cursor-less call.
- **D7. `age_seconds` uses the server clock and is floored at 0**, so a bundle whose stored
  `created_at` is in the future reports `0` rather than a negative number.

## What

Each criterion below is observable through the HTTP API, the MCP tool surface, or the database.

### Resolution semantics (applies to every answering surface)

- **R1.** When a caller submits a resolution for an **active** bundle and
  `CompleteActiveClarificationBundle` returns `claimed == true`, that caller SHALL be treated as the
  winner and SHALL be the only caller that performs step 5.
- **R2. A losing caller SHALL be told what the winner decided, not merely that it lost.** When the
  claim returns `claimed == false`, the system SHALL re-read the bundle's `clarification_request`
  messages and branch on their durable status:
  - **At least one message carries `answered` or `rejected`** — a winner exists. The system SHALL
    return a **success** response carrying `claimed: false` plus the winner's `status` and
    reconstructed `response` (R2b), SHALL NOT modify any message, and SHALL NOT publish any event.
  - **No message carries `answered` or `rejected`** — there is no winner; the bundle simply is not
    active (superseded turn, terminal session, `expired`, `cancelled`, or malformed). The system
    SHALL return **409** with upstream's existing message, unchanged.
  This branch is the load-bearing addition, because `CompleteActiveClarificationBundle` returns the
  same `claimed == false` for both states and a caller cannot distinguish them. Collapsing them —
  in either direction — produces a concrete lie: reporting 409 for a duplicate answer hides the
  winner's payload, which is the whole point of idempotent replay; reporting "success, here is the
  winner's answer" for a superseded bundle invents a winner that does not exist and would have the
  MCP tool tell an agent its answer was superseded by an answer nobody gave.
- **R2a. The re-read is not required to agree with the failed claim, and SHALL NOT be retried.** The
  bundle can change again between the claim and the re-read. Whatever the re-read observes is what
  the caller is told; a single read is the contract. Retrying to obtain a "more settled" answer would
  race the same transitions indefinitely, and every outcome of the re-read is already a truthful
  statement about a real moment.
- **R2b. Reconstructing the winner's `response`.** `status` is the status of the **first message in
  D2 order** whose status is `answered` or `rejected`; a bundle claimed by upstream is uniform, so
  this tiebreak only ever binds on a legacy mixed bundle, where it is still total and reproducible.
  `answers` is built by walking the bundle's messages in D2 order and taking each message's
  `metadata.response` where present, in that order; a message without one contributes no entry, so a
  rejection yields `[]`. `rejected` is `true` when `status` is `rejected`.
  **`reject_reason` SHALL be the empty string on every replay.** Upstream stores no reject reason on
  the message rows — `completeClaimedClarificationMessages` writes only `status`,
  `response_delivery_pending` and, for `answered`, `response` — so there is nowhere to read it from.
  This is stated rather than repaired: repairing it means adding a metadata key to a shape the
  *Upstream baseline* freezes, which belongs to the active-lifecycle card, not this one. A loser
  therefore learns *that* the bundle was declined and *by what outcome*, but not the declining
  caller's prose.
- **R2c. Validation runs before the loser branch.** A malformed submission against an
  already-resolved bundle SHALL receive the same validation error it would receive against an active
  one, and SHALL NOT receive the winner's payload. Returning success for a request the system could
  not parse would tell a caller its malformed answer had been accepted. This follows from N8c
  ordering validation ahead of step 4, and is restated here because R2 is where it is observable.
- **R3.** While two callers submit resolutions for the same `pending_id` concurrently, exactly one
  SHALL observe `claimed == true` and the other SHALL observe R2's winner branch. This SHALL hold
  when the two callers are served by different HTTP requests, and — the case no existing test can
  cover, because the MCP tool did not exist when upstream's tests were written — **when one is the
  web UI over REST and the other is `answer_question_kandev` over MCP**.
- **R4.** The durable claim SHALL complete before any resume event is published, so a loser can
  never trigger a resume. This holds by construction once P1 is satisfied: the claim is a committed
  transaction and step 5 runs only for the winner.
- **R4a. When `CompleteActiveClarificationBundle` returns an error rather than a `claimed` boolean**,
  the system SHALL return **500**, SHALL NOT retry, and SHALL log the `pending_id` and the error.
  Reachable causes are upstream's own: a database failure, a claimed/loaded row-count mismatch, and
  a claimed message with an empty `question_id`. The last is why L16 bundles are excluded from L1
  and are not answerable at all (L16); it is not a caller error and SHALL NOT be reported as 400.
- **R6.** When the backend restarts between a bundle's creation and its resolution, a subsequent
  resolution SHALL still be claimed exactly once, and SHALL succeed whenever the bundle is still
  active. The claim SHALL NOT depend on in-memory state. A plain restart does not by itself end a
  turn or terminate a session, so a bundle stranded by one remains active and answerable; a bundle
  whose session has since accepted a newer turn does not, per D4.
- **R7. The winner's delivery and resume path is upstream's, reached unchanged.** The system SHALL
  call the existing `Store.RespondWithDeliveryConfirmation` path and SHALL NOT re-implement live
  delivery, detached resume dispatch, or the `RestoreActiveClarificationBundle` rollback. A live
  waiter therefore receives the response in the same turn; a detached current-turn bundle resumes
  through upstream's bounded-wait dispatch; a dispatch that cannot be accepted rolls the claim back
  and the bundle becomes answerable again.
- **R7a. HTTP 200 on a winning resolution therefore means more than "recorded".** It means the claim
  committed **and** the response either reached a live waiter or had one resume dispatch accepted.
  A failure of either yields a 5xx from upstream's existing handling, with the bundle rolled back.
  This is a stronger guarantee than the four-valued `resume` field the first version of this spec
  specified, which is why that field is retired rather than reproduced (see *Retired criteria* R8a):
  there is no longer a success case in which the caller must be told the resume is in doubt.
- **R9. A rejection SHALL produce the same agent-visible outcome as a human skip.** It is claimed
  with `status = rejected`, and upstream's delivery path decides what the agent sees: a live waiter
  receives a `clarification.Response` with `rejected: true` and the caller's reason; a detached
  current-turn bundle is persisted rejected without resuming the agent. The wire format does not
  distinguish the answerer — a reject from an external agent means the same thing to the blocked
  agent as a human skip.

### REST status codes and response envelope

- **R10.** `POST /api/v1/clarification/:id/respond` SHALL use exactly these outcomes. This table is
  the contract W2/W3 and the E2E duplicate-submit case are written against.

| Outcome | Status | Body |
|---|---|---|
| won the claim (R1) | 200 | `{"success": true, "claimed": true, "status": "...", "response": {...}}` |
| lost to a resolved winner (R2 winner branch) | 200 | same shape, `"claimed": false`, winner's `status`/`response` |
| not active, no winner (R2 no-winner branch) | 409 | `{"error": "clarification request is no longer active"}` |
| validation failure (N6–N8b) | 400 | `{"error": "<message naming the offending field>"}` |
| unknown / unauthorized `pending_id` | 404 | `{"error": "clarification request not found"}` |
| claim error (R4a) or delivery failure (R7) | 500 | `{"error": "<message>"}` |

- **R11. The `success` key SHALL be retained** on `POST` responses so an existing client that only
  checks `res.ok` and `success` keeps working. `claimed`, `status`, and `response` are additive.
  **The 409 is narrowed, not removed.** Upstream returns 409 for every `claimed == false`; after
  this change 409 means *only* "not active and no winner", and the duplicate-answer case that used
  to share it becomes a 200 with `claimed: false`. Clients that already treat 409 as a successful
  submit (W2) keep working in both cases because they also treat 200 as success.
- **R12. `response` SHALL always emit `answers`, `rejected` and `reject_reason`, and each entry's
  `question_id`, `selected_options` and `custom_text`**, using `[]` for an empty array and `""` for
  an empty string. `clarification.Response` declares `omitempty` on `Answers`, `Rejected`,
  `RejectReason` and `Answer.SelectedOptions`/`CustomText`, so marshalling the struct as it stands
  omits the **key entirely** for an empty slice, a `false` bool or an empty string — a rejection's
  payload would carry no `answers` key at all and a client reading `response.answers` would get
  `undefined`. The struct tags **SHALL NOT be changed**: they are the wire shape of
  `ask_user_question_kandev`'s own tool result, which *Out of scope* freezes. An explicit
  serialization for this envelope is therefore required, and it binds the REST body and the MCP
  result alike.

### Authorization

This is the deliverable upstream did not build. Every criterion here is unchanged in intent from the
first version of this spec; only the endpoint set's behavior around the claim has moved.

- **A1.** `POST /api/v1/clarification/:id/respond` SHALL deny a request whose caller cannot access
  the task owning that `pending_id`.
- **A2.** `GET /api/v1/clarification/:id`, `GET /api/v1/clarification/:id/wait`, and
  `POST /api/v1/clarification/:id/cancel` SHALL apply the same check.
- **A3.** A denial under A1/A2 SHALL be a **404** with a body indistinguishable from a nonexistent
  `pending_id`. It SHALL NOT be a 403 and SHALL NOT include the question text, option labels, task
  ID, session ID, or workspace ID.
- **A4.** An unscoped caller SHALL be authorized for every `pending_id`. Concretely: **with auth
  disabled, no caller is ever newly denied**, and any behavior this spec does not deliberately
  change is unchanged. The deliberate changes below apply in **both** auth modes — there is one code
  path, not an auth-disabled variant:
  1. a duplicate submission against a bundle that already has a winner returns 200 with
     `claimed: false` and the winner's payload, instead of 409 (R2, R11);
  2. every successful response gains `claimed`, `status`, and `response` fields (R10, R12);
  3. a `pending_id` with no durable messages returns 404 (A5);
  4. the new validation rules reject submissions that pass today: unknown option IDs (N8),
     `rejected: true` combined with answers (N8a), and `reason`/`custom_text` over 2000 runes (N8b);
  5. an answers submission against a bundle whose messages carry no `question_id` is rejected before
     the claim (L16) instead of reaching upstream's claim and failing there with a 500;
  6. `GET /:id/wait` on a `pending_id` with **no durable messages** returns **404** instead of
     today's **504** (A5a).
  This list is exhaustive. A behavior not named here SHALL be identical to the pre-change behavior
  for an unscoped caller on all four endpoints. In particular, 409 for a genuinely inactive bundle
  is **not** on this list: it is preserved exactly as upstream returns it today.
- **A5.** A `pending_id` whose durable messages cannot be found SHALL produce the same 404 as A3.
  This also covers the M5a case where a bundle's `task_id` cannot be resolved from either source,
  for a scoped caller.
- **A5a. A5 binds all four endpoints, in both auth modes — including `GET /:id/wait`, which returns
  504 for that input today.** A8 resolves the authorizing `task_id` from the durable messages on all
  four endpoints and A7 puts that resolution ahead of the in-memory read, so a `pending_id` with no
  durable messages fails at step 1 on every endpoint, for an unscoped caller as much as a scoped one.
  It is enumerated rather than avoided because the alternative breaks a security property. Exempting
  the two read endpoints from A5 would leave `GET /:id/wait` returning 404 for a **foreign** bundle
  (A3) and 504 for a **nonexistent** one — a status-code oracle telling an unauthorized caller
  exactly which `pending_id`s exist, which is the precise distinction A3 exists to prevent. A
  compatibility promise can be amended by adding a line to A4; an existence-disclosure channel
  cannot be amended at all. The 504 is preserved where it means something: an authorized caller
  waiting on a bundle that **does** exist.
- **A6.** `POST /api/v1/clarification/request` (creation) SHALL be unchanged by this spec. It is
  called by the in-session agent path, which carries the task owner's identity via `internal/mcp/scope`.
- **A7.** The A2 authorization check SHALL run **before** the in-memory read on both read endpoints,
  so an unauthorized caller receives A3's 404 on either and can never distinguish a foreign bundle
  from a nonexistent one by status code. An **authorized** caller's post-authorization behavior is
  unchanged **for a bundle whose durable messages exist**: 404 from `GET /:id` when the in-memory
  entry is gone, 504 from `GET /:id/wait` on drain or timeout.
- **A8.** On all four endpoints the authorizing `task_id` comes from the durable messages via M5,
  never from the in-memory `Request.TaskID`. A7's ordering requires it — the check runs before the
  in-memory read, so the in-memory value is not yet available — and it also removes a discrepancy:
  the in-memory `Request.TaskID` is populated from the same possibly-empty session lookup as the
  message column, so trusting it would reintroduce the M5 hole on two endpoints while the other two
  resolved it correctly.

- **A9. Cancel is authorized without being claimed.** `POST /:id/cancel` does **not** go through
  `ResolveBundle` — X1's routing is retired — so A2's check SHALL be applied by running
  `ResolveBundle` steps 1 and 2 alone (resolve the bundle's `task_id` per M5, then
  `AuthorizeTaskAccess`) ahead of the handler's existing body, and returning A3's 404 on either
  failure. Everything after that check is today's cancel, unchanged: it still requires a live
  in-memory entry, still returns 404 without one, and still applies its per-question updates in a
  log-and-continue loop. Stating this is what stops a builder inferring from A2 that cancel must be
  re-routed through the claim, which P1 and upstream's answered/rejected-only terminal domain forbid.

**BREAKING CHANGE, intentional.** An authenticated caller that today answers any bundle by bare
`pending_id` will receive 404 for bundles outside its workspaces. The only in-tree production caller
is the web UI, which answers from a task it can already read, so it is unaffected — subject to W1
below. The six auth-mode-independent changes enumerated in A4 are also intentional and accepted.

### Web client

- **W1.** The web UI's clarification POSTs (`apps/web/hooks/domains/session/use-clarification-group.ts`,
  both call sites) SHALL send credentials. They currently use a bare `fetch` with the default
  `credentials: "same-origin"`, unlike the shared client (`lib/api/client.ts`, `credentials: "include"`).
  In split-origin dev mode (`__KANDEV_API_PORT` set, browser and API on different ports) the session
  cookie is therefore dropped, so with auth enabled the request is already rejected by the global
  middleware before this spec's changes. Authorization becomes load-bearing on this route, so the
  omission is fixed here.
- **W2.** The web UI SHALL treat a `claimed: false` success as a successful submit and close the
  overlay, the same way it already treats a 409. Both outcomes remain "someone else settled this";
  the client's existing 409 handling SHALL be retained unchanged, because R11 keeps 409 reachable
  for a genuinely inactive bundle.
- **W3.** On a `claimed: false` **200** the web UI SHALL apply the **winner's** returned `response`
  to the bundle's local message metadata, not the answers this client submitted.
  `postClarificationBatch` and `postClarificationSkip` currently inspect only `res.ok` / `res.status`,
  never the body, and `safeApplyResolvedStatus` then optimistically writes **this client's** answers
  into `metadata.response`. Because R2 guarantees the losing submit produces no
  `session.message.updated` broadcast, nothing would ever correct that write: the losing tab would
  display the losing answers as the transcript until a reload. Both call sites SHALL parse the
  response body and pass the winner's answers to the optimistic update. When `claimed` is absent from
  the body (an older backend, or the 409 path), the client SHALL keep today's behavior.
- **W3a. The winner's `status` domain is `answered` or `rejected`.** `safeApplyResolvedStatus` and
  `applyResolvedStatusToBundle` already accept exactly `"answered" | "rejected"`, which now matches
  the backend's terminal domain exactly, so **no union widening is required.** This is stated
  because the first version of this spec required adding `"cancelled"` to that union; upstream's
  claim cannot produce a cancelled winner (its terminal domain is answered/rejected only), so a
  cancelled bundle now reaches the client as the 409 no-winner branch instead. See *Retired
  criteria* W3a.
- **W4.** The overlay's free-text input (`clarification-input-overlay.tsx`, which enforces no limit
  today) SHALL stop a human at the same boundary N8b enforces on the server, so a long answer is
  caught at the input rather than returning an opaque 400 on submit.
- **W4a. The client-side guard SHALL count runes, and the HTML `maxLength` attribute alone SHALL NOT
  be the mechanism.** N8b's cap is 2000 UTF-8 **runes**, chosen explicitly so a non-Latin answer is
  not cut short. The `maxLength` attribute counts **UTF-16 code units**, so every astral character —
  emoji, and much CJK Extension B — consumes two of its budget. `maxLength={2000}` would stop a user
  at 1000 emoji the server would have accepted: not the opaque-400 failure W4 exists to prevent, but
  the mirror-image one, a client refusing input the contract permits, with no error message at all
  because `maxLength` fails silently. The overlay SHALL enforce the limit by counting runes (code
  points) on the input's value and SHALL surface the boundary to the user rather than silently
  truncating. `maxLength` MAY be set in addition, as a coarse backstop, only at a value that cannot
  reject anything the server accepts — **no lower than 4000**, since a rune is at most two UTF-16
  code units. It SHALL NOT be set to 2000. Boundary values follow N8b exactly: 2000 runes is
  accepted, 2001 is not, and the count is over code points, not bytes and not UTF-16 units.

### `list_pending_questions_kandev` (external MCP surface only)

- **L1.** The tool SHALL return every **active** bundle (D4) in the workspaces visible to the caller,
  and no bundle outside them.
- **L1a.** Visibility SHALL be resolved by the same rule as `filterWorkspacesForCaller`
  (`task/service/service_access.go`): an unscoped caller sees every workspace; a scoped caller sees a
  workspace whose `owner_id` is empty or matches the caller. The tool SHALL resolve that
  workspace-ID set first and apply it as a **predicate of the bundle query**, not as a post-query
  filter over an already-limited page. Per-bundle `AuthorizeTaskAccess` calls are NOT the mechanism:
  N calls per page would be N round trips, and — decisively — filtering after `LIMIT` would make
  L10's `limit` mean "rows examined" rather than "bundles returned", so a page could come back short
  or empty while more matching bundles exist, and a cursor-polling caller could not distinguish that
  from exhaustion. `limit` counts **returned** bundles.
- **L1b.** When the resolved workspace-ID set is empty — a scoped caller who owns no workspaces and
  can see no unowned ones — the tool SHALL omit the `workspace_id IN (...)` **term** from the L1c
  disjunction rather than issuing it, because an empty `IN ()` is a syntax error on both dialects.
  It SHALL NOT short-circuit the whole query, and there is **no** whole-query short-circuit branch in
  this tool. L1c's other disjuncts are predicates on the task row rather than on the caller, so they
  remain applicable no matter what the caller can see: a scoped caller with no visible workspaces can
  still legitimately be shown bundles whose task has an empty `workspace_id` or a dangling workspace
  reference, exactly as `authorizeTaskID` would allow them to answer those bundles.
- **L1c. The visibility predicate SHALL reproduce `authorizeTaskAccess`, not a narrower
  workspace-membership test.** The list tool and the answer tool MUST agree on visibility: a bundle
  `answer_question_kandev` will answer and `list_pending_questions_kandev` will not show is a silent
  discovery hole, and discovery is what this card exists to add. `AuthorizeTaskAccess` delegates to
  `authorizeTaskID`, which permits a scoped caller in **two** cases a bare
  `workspace_id IN (visible set)` term cannot express. The predicate is therefore a three-way
  disjunction, evaluated over the **M5-resolved** task:
  1. **The task's `workspace_id` is empty.** `authorizeTaskID` returns nil early on
     `task.WorkspaceID == ""`. This is D5's case, and D5 already states such bundles appear in L1
     for everyone — the predicate must actually deliver that.
  2. **The task's `workspace_id` names no existing workspace row.** `authorizeTaskID` treats a failed
     workspace lookup as visible, by an explicit `//nolint:nilerr` fallback whose comment reads "a
     dangling workspace reference should not hide the task from the single user who can already see
     everything else about it". `filterWorkspacesForCaller` cannot express this because it filters a
     list of workspaces that *exist*.
  3. **The task's `workspace_id` is in the L1a visible set.** This is the ordinary case, and it is
     the disjunct the `IN` term expresses.
  For an unscoped caller the predicate is satisfied unconditionally, matching `callerScope` returning
  `ok=false`.
- **L1d. Mechanism for L1c, so it is not invented.** Disjunct 1 is a comparison against the literal
  empty string, because `tasks.workspace_id` is `TEXT NOT NULL DEFAULT ''` with no foreign key —
  empty is its default, not an anomaly. Disjunct 2 is an absence test against the `workspaces` table
  (`NOT EXISTS (SELECT 1 FROM workspaces w WHERE w.id = t.workspace_id)`, or the equivalent left join
  with a null check); it is a plain relational predicate and needs **no** `internal/db/dialect`
  helper. Disjunct 3 is the workspace-ID set from L1a, subject to L1b's empty-set handling. The three
  are combined with `OR` and the whole disjunction is `AND`-ed with D4's activeness test, all inside
  the single bundle query — per L1a visibility is a query predicate, never a post-`LIMIT` filter.
  **Separately from the disjunction**, the join reaches the task through **M5's rule** —
  `task_session_messages.task_id` when non-empty, otherwise the bundle's session row's `task_id` —
  because the obvious `JOIN tasks ON tasks.id = messages.task_id` silently drops every legacy
  empty-`task_id` bundle for **every** caller, unscoped included. That is a join-path rule, not a
  fourth disjunct, and it decides which task row disjuncts 1–3 evaluate against.
- **L2. The tool SHALL derive its result from the durable `clarification_request` messages**, NOT
  from `Store.ListPending`, and SHALL therefore return the correct set after a backend restart.
  `Store.ListPending` returns nothing after a restart because the map is empty. The membership test
  is **D4**, applied as predicates of the bundle query.
- **L2a. The list query SHALL reuse upstream's activeness expression rather than restate it.** D4 is
  upstream's rule, and the list tool is the third consumer of it after the workflow guard and the
  claim. The current-turn and non-terminal-session predicates SHALL be built from the same helpers
  those two already use (`turnAuthorityPredicate`, the non-terminal-session predicate) rather than
  hand-written, so a future change to activeness cannot leave this tool behind. This is the concrete
  form of upstream's own requirement that *"all backend consumers derive active clarification state
  from one repository rule"*.
- **L3.** Each returned bundle SHALL carry: `pending_id`, `task_id`, `session_id`, `created_at`
  (RFC3339 UTC), `age_seconds` (integer, server clock minus `created_at`, floored at 0), `context`,
  and `questions`.
- **L4.** Each question SHALL carry `question_id`, `title`, `prompt`, `status`, and `options`, where
  each option carries `option_id`, `label`, and `description` — the exact identifiers
  `answer_question_kandev` accepts.
- **L4a. Every field named in L3 and L4 is always present and is never JSON `null`.** When a value is
  unknown, absent, or unparseable:
  - a **string** field (`task_id`, `session_id`, `context`, `question_id`, `title`, `prompt`,
    `option_id`, `label`, `description`) SHALL be emitted as the **empty string** `""`;
  - an **array** field (`questions`, `options`) SHALL be emitted as an **empty array** `[]`;
  - `age_seconds` SHALL be emitted as an integer, floored at 0 per D7, never null;
  - `created_at` and `status` have no unknown case: `created_at` is D1's `MIN(created_at)` over rows
    that exist by construction, and a message whose `status` key is absent is reported as the string
    `pending`, matching the `COALESCE(status,'') IN ('','pending')` that admitted it.
  Without this rule a builder emitting Go zero values through one path and `null` through another
  produces a shape that changes between bundles, and an MCP client iterating `options` or reading
  `label` on a degraded bundle would meet `null` where the contract implied an array or a string.
- **L5.** Questions within a bundle SHALL be ordered by D2's total key.
- **L6.** Bundles SHALL be ordered by `created_at` ascending, then by `pending_id` ascending. The
  `pending_id` tiebreak exists solely to make the order total for cursor pagination; it carries no
  meaning. Oldest-first is the useful order because the oldest blocked agent is the most urgent.
- **L7.** The tool SHALL accept an optional `workspace_id`. When supplied, results SHALL be limited
  to bundles whose task — resolved per M5 — carries exactly that `workspace_id`, and a workspace the
  caller cannot access SHALL produce the same empty-result response as an empty workspace (no
  existence leak, consistent with A3).
- **L7a. How `workspace_id` composes with L1c, and what the empty string means.**
  1. **`workspace_id` is one additional predicate `AND`-ed with the whole L1c disjunction** — never a
     substitute for it, and never a fourth disjunct. Adding an `AND` term can only ever NARROW the
     result, so supplying `workspace_id` can never reveal a bundle an unfiltered call would withhold.
  2. L7's empty-result guarantee for an inaccessible workspace **falls out of that `AND`** and needs
     no short-circuit branch. For a workspace that exists and the caller cannot see: disjunct 1 is
     false (the task's `workspace_id` is non-empty), disjunct 2 is false (the workspace row exists),
     and disjunct 3 is false (it is not in the L1a set). L1b's "there is no whole-query short-circuit
     in this tool" therefore holds without exception.
  3. A **fabricated or deleted** `workspace_id` leaves disjunct 2 satisfiable, so bundles whose task
     carries that dangling reference ARE returned. That is correct rather than a leak: they are
     exactly the bundles `authorizeTaskID`'s dangling-workspace fallback already lets this caller
     answer, and an unfiltered L1 call would have listed them anyway. Nothing is disclosed about a
     workspace that does not exist.
  4. **`workspace_id: ""` means the parameter was not supplied.** It SHALL NOT be read as "filter to
     tasks whose `workspace_id` is empty", even though disjunct 1 and D5 make that a real class. An
     omitted optional string decodes to `""`, so the other reading would silently narrow every caller
     who omits the argument down to the empty-workspace class alone. Those bundles are reached by
     making an **unfiltered** call. This spec deliberately provides no filter selecting them
     exclusively; that is a named exclusion, not an oversight.
- **L8.** The tool SHALL accept an optional `created_since` (RFC3339). When supplied, only bundles
  with `created_at >= created_since` SHALL be returned. The parameter is named for the column it
  filters: a bundle's `created_at` never changes, so an `updated_since` name would promise
  change-feed semantics this tool does not have.
- **L9.** The tool SHALL accept an optional `cursor` — the opaque encoding of the last returned
  `(created_at, pending_id)` pair — and SHALL return only bundles ordered strictly after it under L6.
  It SHALL return a `next_cursor` when more results exist and an empty `next_cursor` when they do not.
- **L10.** The tool SHALL accept an optional `limit`, defaulting to **50** and capped at **200**. A
  `limit` below 1, or absent, SHALL be treated as the default; a `limit` above the cap SHALL be
  clamped to the cap rather than rejected. Per L1a, `limit` counts bundles actually returned.
- **L11.** The response envelope SHALL be
  `{"bundles": [...], "count": <int>, "next_cursor": "<string>"}`. `bundles` is the top-level array —
  each element carries its own `questions` array per L3, so the two names never collide. `count` is
  the number of elements in `bundles` on **this page** and is always equal to `len(bundles)`; it is a
  convenience, not a total. When no bundle matches, the tool SHALL return an empty `bundles` array,
  `count: 0`, and an empty `next_cursor`, and SHALL NOT return an error.
- **L11a.** A grand total across all pages is deliberately NOT provided. Producing one would require
  a second, unbounded, authorization-filtered aggregate query per call, and it would be stale the
  moment it was computed (D6). Callers that need to know whether more work exists read `next_cursor`.
- **L12. A bundle whose per-question messages disagree on `status` (some pending, some terminal) is
  returned iff it satisfies D4** — that is, iff at least one message is still pending and the bundle
  is on the current turn of a non-terminal session. Each question SHALL carry its own `status`. Such
  a bundle is a legacy artifact: upstream's claim writes every row in one transaction, so nothing
  produced after this change can be mixed. Returning it lets a caller finish it; the claim will then
  transition only the still-pending rows, which is exactly upstream's predicate.
- **L13.** An unparseable `created_since` or an unparseable/corrupt `cursor` SHALL produce a
  validation error naming the offending argument. Neither SHALL be silently ignored: a caller polling
  with a cursor it thinks is being honored, that is in fact being dropped, re-reads the whole backlog
  every tick and re-answers questions it already handled.
- **L14.** Supplying both `cursor` and `created_since` SHALL be accepted; both constraints apply
  (intersection). `cursor` is the pagination position, `created_since` is a filter, and they are
  independent.
- **L15.** A bundle whose durable messages carry no parseable `question` metadata SHALL still be
  returned when it satisfies D4 and L16, with the affected question carrying its `question_id` and,
  per L4a, an empty `title`, `prompt` and `options` rather than null ones. Such a question cannot be
  answered by option ID — every `selected_options` entry would fail N8 against an empty option list —
  but hiding the bundle would strand its agent invisibly, so it remains answerable by `custom_text`
  alone (N7) or by rejection.
- **L16. A bundle in which ANY message carries an empty or absent `question_id` SHALL be excluded
  from L1, and SHALL be rejected by `answer_question_kandev` and the REST endpoint before the claim.**
  Upstream's `completeClaimedClarificationMessages` returns
  `clarification message %s is missing question_id` and aborts its transaction, so such a bundle
  cannot be resolved by any outcome — not by an answer, and not by a rejection. Listing a bundle no
  caller can act on is a discovery lie: the tool would advertise a blocked agent and every attempt to
  clear it would return 500. Excluding it is therefore the honest contract, and it is the whole
  bundle that is excluded, not the offending message, because the claim aborts on the first one it
  meets. The pre-claim rejection SHALL name the condition rather than surfacing upstream's 500 (R4a).
  A human still sees the bundle in the chat transcript, and it is still reachable through
  `get_task_conversation_kandev`. Making such a bundle resolvable requires a change to upstream's
  claim and is named in *Out of scope*.

### `answer_question_kandev` (external MCP surface only)

- **N1.** The tool SHALL accept `pending_id` plus either `answers` (one entry per question) or
  `rejected: true` with an optional `reason`.
- **N2.** The tool SHALL resolve the bundle through the same `ResolveBundle` operation as the REST
  endpoint, and SHALL therefore inherit R1–R9 and A1–A5 without a second code path.
- **N3.** On winning, the tool SHALL return `claimed: true` with the recorded `status` and the
  normalized `response` (N3a).
- **N3a. Normalization** is the canonical form of the answer payload. It is defined so that two
  callers submitting semantically identical answers produce byte-identical **answer payloads** — the
  `answers`, `rejected` and `reject_reason` fields. Rules:
  1. `answers` entries are ordered by the bundle's own question order (L5), **not** by the order the
     caller supplied them.
  2. Within an entry, `selected_options` is ordered by the option's position in the question's
     `options` array, and exact duplicates are removed.
  3. `custom_text` and `reason` are stored verbatim after trimming leading and trailing whitespace.
     No other transformation is applied. Trimming happens **after** N8b's length validation, which
     runs on the caller's raw input at step 3, so a value one rune over the cap is rejected even when
     trimming would have brought it under.
  4. An absent `answers`, `selected_options`, or `options` array is emitted as an empty JSON array
     `[]`, never `null` and never as an **omitted key**. **R12 states the mechanism** and is not
     optional here: the guarantee is unreachable through `encoding/json` on `clarification.Response`
     as tagged.
- **N4.** On losing to a resolved winner, the tool SHALL return `claimed: false` with the winner's
  `status` and reconstructed `response` (R2b), and SHALL NOT report an error. Answering an
  already-answered question is a successful no-op that tells the caller what the answer was.
- **N4a. On losing to an inactive bundle with no winner (R2's second branch), the tool SHALL report
  an error** naming the bundle as no longer active, distinct from both success and not-found. A
  caller must be able to tell "you were beaten, here is the answer" from "this question is no longer
  answerable and nobody answered it", because the two imply different next actions: the first is
  done, the second may need a human.
- **N5.** A `pending_id` the caller cannot access SHALL produce the same not-found error text as a
  nonexistent one.
- **N6.** An `answers` array SHALL be rejected with a validation error, and SHALL NOT reach the
  claim, when **any** of these four conditions holds. The list is exhaustive:
  1. It does not contain exactly one entry per question in the bundle.
  2. An entry carries an **empty** `question_id`.
  3. An entry references a `question_id` not in the bundle's expected set.
  4. An entry repeats a `question_id` already used by an earlier entry.
  All four are the existing rule in `validateRespondAnswers`.
- **N6a.** The expected question-id set SHALL be derived from the bundle's durable messages, counting
  **one expected answer per `clarification_request` message** in the bundle. Today
  `validateRespondAnswers` falls back to permissive acceptance when it cannot determine that set;
  under this spec step 1 has already proven the messages exist and L16 has already excluded every
  bundle whose ids are empty, so the expected set is always non-empty and that permissive branch
  SHALL NOT be reachable from `ResolveBundle`.
- **N7.** An answer entry MAY carry `selected_options` (option IDs), `custom_text`, or both. An entry
  carrying neither SHALL be accepted and SHALL render as "(no answer)" in the resume prompt,
  preserving `formatAnswerBody`.
- **N8.** A `selected_options` entry naming an `option_id` not present on that question SHALL be
  rejected with a validation error and SHALL NOT reach the claim. *(This is stricter than today's
  REST endpoint, which does not check option IDs. External agents fabricate identifiers; the human at
  the keyboard clicks a rendered button. The check is applied on both surfaces so the two cannot
  drift.)* N8 constrains **membership only**. It does not constrain cardinality: `selected_options` is
  a slice and nothing in the existing model marks a question single- or multi-select, so an entry
  naming several valid option IDs SHALL be accepted. Inventing a single-select rule here would reject
  answers the overlay itself can produce.
- **N8a.** A request carrying both `rejected: true` and a non-empty `answers` array SHALL be rejected
  with a validation error and SHALL NOT reach the claim. The two are mutually exclusive outcomes and
  guessing which the caller meant would silently discard one of them. A request carrying
  `rejected: false` with an empty `answers` array is the N6 count-mismatch case and is likewise
  rejected, **including for a single-question bundle**: N7 governs an answer *entry* that carries
  neither field, and an empty array contains no entry at all. A caller that means "no answer" for a
  one-question bundle sends one entry with neither field (N7); a caller that means "I decline" sends
  `rejected: true`.
- **N8b.** `reason` SHALL be capped at **2000 runes**; a longer value SHALL be rejected with a
  validation error rather than truncated, since the reason is replayed verbatim into the blocked
  agent's resume prompt. `custom_text` on an answer SHALL be capped at the same limit, per entry. The
  cap is enforced inside `ResolveBundle`, so it binds the REST endpoint and the web overlay as well
  as this tool (W4a adds the matching client-side guard). The limit counts UTF-8 **runes**, not
  bytes, so a non-Latin answer is not cut short at a third of the visible length.
- **N8c.** Validation (N6, N6a, N7, N8, N8a, N8b, L16) SHALL run **before** the claim in step 4, so a
  malformed request never resolves a bundle, and before R2's loser branch (R2c). A validation error
  SHALL leave the bundle untouched.
- **N8d. Validation order among the rules is unspecified and deliberately so.** When one submission
  violates several, any one of them MAY be reported; R10 requires only a 400 whose message names the
  offending field. Pinning an order would constrain the error string without changing any state
  outcome — every ordering rejects the same set of submissions, writes nothing, and leaves the bundle
  answerable. A test SHALL therefore assert the 400 and the named field for each rule in isolation,
  never a particular winner among simultaneous violations.
- **N9. Argument schema.** Both tools SHALL declare their arguments using the same
  `mcp.NewTool` / `mcp.WithString` / `mcp.WithBoolean` / `mcp.WithNumber` / `mcp.WithArray` /
  `mcp.WithObject` helpers every other tool on this surface uses, with `mcp.Required()` on
  `pending_id` and on nothing else. `answers` is an array of objects whose shape mirrors
  `ask_user_question_kandev`'s own nested `questions[].options[]` declaration, which is the in-repo
  precedent for an array-of-objects argument. A raw JSON Schema via `NewToolWithRawSchema` is
  permitted where the helpers cannot express the nesting. No new schema convention is introduced.

### Surface placement

- **S1.** Both tools SHALL be registered for `SurfaceExternal` only.
- **S2.** Neither tool SHALL appear on `SurfaceKanbanTask`, `SurfaceOfficeTask`, or
  `SurfaceConfiguration`. In-session MCP scoping resolves to the workspace **owner**
  (`internal/mcp/scope`), not to a task relationship, so a running agent on the kanban surface would
  be able to list and answer human questions across every task that owner can see. That defeats the
  human-input boundary and collides with autopilot's parent-only interaction model
  (`ask_parent_question_kandev`).
- **S3.** `ask_user_question_kandev` SHALL remain absent from the external surface, as today.
- **S4.** Neither tool SHALL be added to the session agent system prompt, since neither is visible to
  session agents.

### `list_tasks_kandev` enrichment

- **T1.** `list_tasks_kandev` SHALL include `task_pending_action` and `primary_session_pending_action`
  for each task, using the same projection the HTTP task list already returns
  (`GetPendingActionsForSessions`, `task/dto/dto.go`), so one call finds every blocked task in a
  workflow.
- **T2.** A task with no blocked session SHALL carry JSON `null` for both fields rather than an empty
  string, matching the HTTP DTO — both fields are `*string` with no `omitempty`, so the key is always
  present and its value is `null`.

### Catalog and documentation

- **C1.** `apps/web/lib/settings/external-mcp-tools.ts` SHALL list both new tools with localized
  descriptions, in a group whose title reflects answering agent questions.
- **C2.** The catalog's KNOWN DRIFT SHALL be closed in the same change. The backend's external tool
  count and the catalog's pinned count SHALL be re-derived against the post-rebase `main` at
  implementation time and made equal; the stale drift note in both
  `external-mcp-tools.ts` and `external-mcp-tools.test.ts` SHALL be **deleted rather than
  renumbered**. The first version of this spec pinned the arithmetic at 35 → 37 and 30 → 37 against
  the pre-rebase merge base; upstream has since landed 60+ commits, so those literals are recorded
  here as **provenance, not as targets**, and a builder SHALL read the live values from
  `TestServerModeExternal_ToolCount` and `countExternalMcpTools()` instead of trusting them.
- **C3.** Every new catalog entry SHALL resolve to an existing `en/settings.json` key, per the
  existing pinning test.

## Permissions

This spec introduces no new permission concept. It applies the existing per-user workspace scoping
rule (`apps/backend/AGENTS.md`, "Opt-in authentication & per-user scoping") to a service that missed
it:

- No identity in context, or a synthetic identity → unscoped, today's pre-auth behavior.
- Real identity → the bundle is visible if the workspace owning its task has an empty `owner_id` or
  an `owner_id` matching the caller, **and also** in the two further cases `authorizeTaskID` itself
  allows: the task has no workspace, or the task's workspace row does not exist. **L1c is the
  normative statement** and the list tool's query predicate SHALL be written against it.
- Denial uses the not-found sentinel (`repoerrors.ErrTaskNotFound` via `AuthorizeTaskAccess`), so a
  foreign bundle and a missing bundle are indistinguishable.

The authorization input is the `pending_id` → `task_id` mapping read from the durable messages (M5).
It SHALL NOT be read from a caller-supplied `task_id`; a caller that supplies one alongside a
`pending_id` has it ignored.

External MCP callers reach this check because `DispatcherBackendClient.RequestPayload` passes the
HTTP request context — carrying the identity the auth middleware attached — straight into `Dispatch`.

**An earlier proposal to authorize via the MCP scope resolver was wrong.** `internal/mcp/scope`
attaches the owning identity of an in-session agent stream; it neither resolves a `pending_id` nor
authorizes one, and it does not apply to the external endpoint at all.

## Failure modes

- **Two callers answer the same active bundle.** Upstream's claim admits one. The winner runs step 5;
  the loser takes R2's winner branch and receives the winner's status and reconstructed answers with
  `claimed: false`. No transcript overwrite, no second resume.
- **A caller answers a bundle whose turn has been superseded.** The claim's current-turn predicate
  rejects it, no message carries a terminal status, and R2's second branch returns 409. This is a
  deliberate narrowing relative to the first version of this spec, which would have answered it and
  resumed the agent in a new turn; upstream's approved contract states superseded rows "cannot drive
  ... a late agent resume", and this spec defers to it rather than reopening that decision.
- **A caller answers a bundle whose session has reached a terminal state.** Same path, same 409. The
  first version of this spec explicitly allowed this ("the transcript is a record, not a live
  channel"); that promise is withdrawn for the same reason.
- **A caller answers a detached bundle on the current turn.** Detachment sets `agent_disconnected`
  without changing `status`, so the bundle is still active: the claim succeeds and upstream's
  bounded-wait detached-resume dispatch runs. This is the "the agent moved on but the answer still
  matters" case, and it still works.
- **A backend restart strands a bundle.** A restart does not by itself end a turn or terminate a
  session, so the bundle remains active, is still listed by L1, and is still answerable (R6).
- **The resume dispatch cannot be accepted.** Upstream rolls the claim back via
  `RestoreActiveClarificationBundle` and the endpoint returns a server error; the bundle becomes
  answerable again. This spec adds nothing here and deliberately reports no partial-success state.
- **A bundle's messages carry no `question_id`.** L16 excludes it from L1 and rejects it before the
  claim, because upstream's claim would abort mid-transaction. It is visible in the transcript and
  clearable only by whatever upstream's lifecycle does to it (expiry on session teardown, or
  supersession by a newer turn).
- **A bundle's `task_id` resolves from neither source.** M5a: not-found for scoped callers, omitted
  from L1 for every caller. Answerable by an unscoped caller whenever upstream's claim still accepts
  it. Near-degenerate in practice, since `task_session_messages.task_session_id` is FK-constrained.
- **A bundle's session row is deleted.** The session cascade removes the bundle's messages, so step 1
  finds nothing and the answer returns not-found (A5) — the same response as an unauthorized bundle,
  by design. Deleting a task deletes its sessions, so this is also the deleted-task path.
- **The loser's re-read races another transition.** R2a: one read, no retry, report what was seen.

## Scenarios

1. **External agent answers a live question.** Agent A on task T calls `ask_user_question_kandev` and
   blocks. An external client lists pending questions, sees T's bundle with its option IDs, and calls
   `answer_question_kandev`. The claim commits, the in-memory waiter is delivered, and agent A's
   blocked tool call returns the answers **in the same turn**. The chat overlay closes for anyone
   watching, via the existing `session.message.updated` broadcast.

2. **Human and external agent answer simultaneously.** The human clicks Submit while the external
   client posts a different answer. One claim wins. The winner's answers reach agent A and appear in
   the transcript. The loser gets a 200 with `claimed: false` and the winner's payload; the losing tab
   renders the winner's answers, not its own (W3); no second turn starts, no transcript overwrite.
   **This is the race no test in the tree can currently exercise**, because the MCP answerer did not
   exist when upstream's tests were written (R3).

3. **Answer after a backend restart.** A bundle is created and the backend restarts, so the in-memory
   entry is gone. An external client lists pending questions — the bundle is still there, because the
   list reads durable messages (L2) and the restart did not supersede its turn. It answers; there is
   no live waiter, so upstream's detached-resume dispatch resumes the agent. A second answerer gets
   R2's winner branch rather than a second resume.

4. **Foreign bundle.** User B holds a PAT and learns a `pending_id` belonging to user A's workspace.
   `answer_question_kandev` returns not-found, identical to a fabricated ID.
   `list_pending_questions_kandev` never showed it. `GET /:id` and `GET /:id/wait` both return 404
   rather than their usual 404/504 split, because A7 puts the authorization check ahead of the
   in-memory read.

5. **Superseded bundle.** The session accepted a newer turn while an older clarification was still
   pending. The bundle is absent from `list_pending_questions_kandev`, does not block turn-complete,
   and an answer attempt returns 409 naming it as no longer active. The transcript still shows it.

6. **Auth disabled.** Identity is synthetic and every caller is unscoped, so nothing is ever denied:
   every bundle remains listable and answerable exactly as before. The six behaviors that do change
   are enumerated in A4 and change identically under enforced auth. There is one code path.

## Retired criteria

Five rounds of spec review and six Build rounds referenced the identities below. Each is withdrawn
here, with its reason, so a reader of that history can tell **retired** from **missing**. No retired
rule may be reintroduced without reopening the collision this revision resolves.

- **M1, M2, M3, M4, M6, M6a, M7, M7a, M8, M8a, M9, M10** — the entire `clarification_resolutions`
  table: its schema, FK cascade, no-backfill rule, dialect portability, response payload shape,
  `resume` write ordering, the `ON CONFLICT ... DO NOTHING` claim statement, its FK-violation and
  vanished-conflict-row edge cases, and the `source` column. **Retired: the table is not created.**
  Upstream's `CompleteActiveClarificationBundle` is the claim (P1). M6a's substance survives as
  **R12**, because the `omitempty` problem it identified is a property of `clarification.Response`,
  not of the table.
- **M5b** — the answerability split between M5a's two arms, which depended on M8a's foreign key.
  **Retired with the FK.**
- **D4/D4a's two-conjunct membership rule** — "no resolution row AND at least one effectively-pending
  message". **Retired: conjunct 1 named a row that no longer exists.** D4 now defers to upstream's
  activeness rule outright, and D4a records why no migration is needed.
- **G1, G2, G3, G4, G5** — the workflow-guard changes. **Retired: the guard is already correct.**
  Upstream repointed `sessionHasPendingClarification` at `FindActiveClarificationMessagesBySessionID`,
  which already widens `= 'pending'` to `COALESCE(status,'') IN ('','pending')` and adds current-turn
  scoping. The function G1/G4 proposed to modify no longer exists. **G5 is additionally withdrawn on
  the merits**, not merely as dead code: it required an *unrecognized* status to count as pending
  (fail closed), while upstream's shared predicate counts it as not-pending. Two consumers disagreeing
  about membership is precisely the defect G5 was written to prevent, so this spec adopts upstream's
  rule rather than diverging from it (D3).
- **R5, R5a** — partial application of per-question updates, the stop-at-first-failure prefix, and
  the 500 that reported it. **Retired: partial application is now impossible.** Upstream's claim
  writes every row of the bundle in one transaction, so a half-applied bundle cannot be produced.
  The corresponding R10 row and A4 item are gone.
- **R8, R8a, R8b, R8c** — the four-valued `resume` field and its rule ordering. **Retired: there is
  no success case in which the resume is in doubt.** Upstream's respond path returns success only
  after delivery confirmation or accepted dispatch, and rolls back otherwise (R7a).
- **R9a** — marking a bundle's messages `rejected` when they carry no `question_id`. **Retired:**
  upstream's claim refuses such a bundle entirely, so the rejection cannot land at all. L16 now
  excludes the bundle from discovery instead of promising an outcome that would 500.
- **X1, X2, X3, X3a, X4, X5** — routing `POST /:id/cancel` through the claim with
  `status = cancelled`, the losing-cancel `CancelCh` exemption, and the four-row cancel status table.
  **Retired: upstream's claim has no `cancelled` terminal status** — `CompleteActiveClarificationBundle`
  rejects any status other than `answered` or `rejected` — and adding one is a change to the
  active-lifecycle contract, not to this card. Cancel keeps its existing behavior and gains **only**
  the A2 authorization check. See *Out of scope*.
- **W3a's union widening** — adding `"cancelled"` to the client's `ResolvedStatus` union.
  **Retired: the backend can no longer return a cancelled winner** (W3a).
- **C2's literal counts (35 → 37, 30 → 37)** — **retired as targets, kept as provenance.** Upstream
  landed 60+ commits after those numbers were measured. C2 now requires re-deriving them.

## Out of scope

Each exclusion below is a decision, not an omission.

- **Answering a superseded or terminal-session bundle.** Upstream's approved active-lifecycle spec
  states superseded rows cannot drive a late agent resume, and that a terminal session's pending
  bundles expire. The first version of this spec promised the opposite. **That promise is withdrawn
  rather than fought**: upstream's spec is approved and merged, this one is draft and unmerged, and
  two contracts disagreeing about when a clarification is answerable is exactly the defect this
  revision exists to remove. Anyone who wants stale-bundle answering back should change the
  active-lifecycle contract, on its own card, with its own threat model.
- **Making cancel atomic, authorized-and-claimed, or usable on a restart-stranded bundle.** Cancel
  gains A2's authorization check and nothing else. Its existing defects — it requires a live
  in-memory entry, it applies per-question updates in a log-and-continue loop, and it cannot clear a
  bundle whose entry is gone — are untouched here because fixing them needs a `cancelled` terminal
  status in `CompleteActiveClarificationBundle`, which is upstream's primitive and upstream's
  contract. Adding a second cancel-claim in this card would violate P1 outright.
- **Resolving a bundle whose messages carry no `question_id`.** L16 excludes it. Making it resolvable
  requires upstream's claim to tolerate an empty `question_id`, which is a change to a shared
  primitive with its own lifecycle questions.
- **Storing the winner's `reject_reason` durably** so a loser can replay it. R2b reports `""`.
  Repairing it means adding a metadata key to a shape the *Upstream baseline* freezes.
- **`wait_for_question_kandev` / long-poll / push.** Deliberately dropped. It recreates the long-held
  MCP connection that `ask_user_question_kandev` already papers over with progress pings — justified
  for an agent blocked on its own question, wrong for a discovery API. Callers poll the list tool
  with a cursor. Notification is reconsidered only alongside a real subscription contract with
  reconnect and missed-event recovery; progress pings are not that.
- **A grand total in the list response.** L11a. `next_cursor` answers "is there more".
- **Extending the two tools to the in-session task surface.** Requires its own threat model (S2).
- **The clarification popup's own UX** — Escape committing a skip rather than dismissing, shortcuts
  focus-scoped to the overlay, submit gated on the whole bundle. Separate card. W4 is in scope only
  because N8b makes an over-long answer newly rejectable.
- **The Office inbox workspace leak.** `DashboardService.inboxPermissionItems` calls
  `Store.ListPendingPermissions()` with no workspace argument while every sibling call in the same
  function takes `wsID`. **Verified during this spec's input inventory**, not merely suspected:
  pending clarifications from every workspace appear in every workspace's inbox.
  `ListPendingPermissions` is also restart-lossy and option-less. It is untouched here, and this
  spec's tools deliberately do not reuse it. It needs its own card.
- **Changing `ask_user_question_kandev`'s own shape, validation, or response envelope.** Unchanged.
- **A `pending_id`-aware WS gateway backstop entry.** `Client.authorizeAction` keys off
  `task_id` / `session_id` / `id`. The new actions are reachable only through the external MCP
  dispatcher, which does not go through the gateway client, and they authorize at the service layer.
  If these actions are ever exposed over the browser WS, the backstop must learn `pending_id` first.

## Verification notes

**E2E decision.** This touches one user-visible surface: the clarification overlay in chat, via
W1–W4a. `apps/web/e2e/tests/chat/clarification.spec.ts` already covers the overlay end to end and
already intercepts `**/api/v1/clarification/*/respond`. Extend it with a duplicate-submit case
asserting the overlay closes on a `claimed: false` **200** and that the rendered answer is the
winner's, not the loser's (W3); do not add a new spec file. The two MCP tools have no browser surface
and need no E2E.

**Assertions that must exist.**
- **R3's cross-entry-point race is the single most important new test in this revision.** It needs a
  real two-goroutine test against a shared database in which one caller answers over the REST handler
  and the other over `answer_question_kandev`, asserting exactly one observes `claimed == true` and
  the other receives the winner's payload. A REST-vs-REST or MCP-vs-MCP race does **not** substitute:
  the collision this revision resolves was precisely two entry points claiming by different
  mechanisms, and only a cross-entry-point test proves P1 holds.
- **P1 needs a structural assertion, not only a behavioral one.** Assert that no table named
  `clarification_resolutions` is created by `initSchema` or any migration, and that the resolution
  path contains no claim statement of its own. A behavioral test passes just as well with a dormant
  second mechanism present.
- R2's two branches need separate tests: a duplicate answer against a bundle with a terminal status
  (expect 200, `claimed: false`, winner's payload) and an answer against a superseded bundle (expect
  409). A test that only covers the first would pass with the branches collapsed, which is the exact
  defect R2 exists to prevent.
- R2b needs a test that a replayed **rejection** carries `answers: []` and `reject_reason: ""`, and a
  test that a replayed **answer** carries the winner's answers in D2 order — not the loser's.
- The authorization tests (A1–A5a, A7, A8) need both the enabled and disabled auth modes, since A4 is
  the compatibility guarantee. The auth-disabled run asserts all **six** A4 carve-outs are present,
  not absent.
- A5a additionally needs an auth-**enabled** test asserting that `GET /:id/wait` returns the **same**
  status for a foreign bundle and for a fabricated `pending_id`. Asserting only the 404 on the
  fabricated id would pass even if the foreign case returned something else, which is exactly the
  oracle A5a exists to close.
- R12 needs a test asserting the response envelope contains the **keys** `answers`, `rejected` and
  `reject_reason` on every outcome — on a rejection `answers` is `[]` and present, on an answer
  `rejected` is `false` and present. Assert key *presence* explicitly (`json.RawMessage` / map
  lookup), not the unmarshalled Go value: unmarshalling an absent key yields the same zero value as
  an emitted empty one, so a test that only reads the struct back passes against the exact bug R12
  exists to prevent.
- L16 needs a test that a bundle with an empty `question_id` on any message is absent from
  `list_pending_questions_kandev` **and** that answering it returns the pre-claim validation error
  rather than a 500 from upstream's aborted transaction.
- L2a needs a test that a superseded bundle and a terminal-session bundle are both absent from the
  list, proving the tool uses upstream's activeness predicate rather than a status-only filter.
- L1a needs a test with more matching bundles than `limit` across two workspaces, asserting the page
  is full rather than short.
- N3a needs a test that two differently-ordered but semantically identical submissions produce
  byte-identical answer payloads, and one proving N3a rule 3's ordering: a `custom_text` of exactly
  2001 runes with a trailing space is **rejected**, not trimmed to 2000 and accepted.
- N8b needs both boundary tests: exactly 2000 runes accepted, 2001 rejected, counted over code points
  with a multi-byte fixture.
- N8d needs per-rule isolation tests only. A test asserting which error wins among simultaneous
  violations SHALL NOT be written; it would pin an unspecified behavior.

**Tests that need deliberate re-derivation against upstream.** The first version of this spec was
built and reviewed across six Build rounds. The following existing artifacts test the retired
mechanism and SHALL be re-derived or deleted rather than rebased: `internal/clarification/resolver_test.go`
and `resolver_restart_test.go`'s claim assertions, `internal/task/repository/sqlite/clarification_resolution_test.go`
and its Postgres sibling in full, and the `clarification_bundle_query_*` tests' D4a-conjunct-1 arms.
`handlers_authz_test.go`, `outcome_validation_test.go`, `question_handlers_test.go`, and
`task_pending_actions_enrich_test.go` test criteria this revision keeps and should largely survive.

**Tests that need deliberate updating.**
- `internal/mcp/server/server_test.go` — `TestServerModeExternal_ToolCount` moves to the re-derived
  post-rebase count (C2).
- `internal/mcp/server/external_integration_test.go` — its `NotContains "ask_user_question_kandev"`
  assertion stays **true and unchanged** (S3); add positive assertions for the two new tool names.
- `apps/web/lib/settings/external-mcp-tools.test.ts` — the pinned count moves to the re-derived value
  and the stale drift note is deleted (C2).
- Any test asserting a **409** from `POST /:id/respond` on a duplicate submit moves to 200 with
  `claimed: false`; tests asserting 409 for a genuinely inactive bundle stay (R11).
- `internal/mcp/handlers/handler_inventory_test.go` measures the dispatcher delta dynamically and
  needs **no** change when handlers are added.
