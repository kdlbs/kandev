---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001
created: 2026-08-16
owners:
  - nova28
---
# External Question Answering — authorized, discoverable, idempotent clarification resolution System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
