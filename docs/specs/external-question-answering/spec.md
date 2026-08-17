---
status: draft
created: 2026-08-16
owner: nova28
---

# External Question Answering — durable, authorized, idempotent clarification resolution

## Why

When a Kandev agent calls `ask_user_question_kandev`, the resulting question is answerable in
exactly one place: a popup rendered over the chat input of that task's session. Everything behind
that popup — the durable record, the REST endpoints, the resume path — already exists, but nothing
outside the browser can discover a question or answer it.

This spec makes answering an out-of-band, first-class API. Two new MCP tools on the **external**
surface let a third party (or an agent running outside Kandev) list the questions it may see and
answer one. That is the visible half.

The load-bearing half is underneath. Answering a clarification today is neither authorized nor
atomic, and adding a second answerer makes both defects reachable in production:

1. **A live double-answer bug.** `Store.WaitForResponse` deletes the in-memory entry the moment it
   observes `done` (`apps/backend/internal/clarification/store.go:121-127`). The 409 duplicate guard
   (`store.go:162`) lives on that entry, so it only covers a second answer that arrives *before* the
   winner's waiter wakes. A second answer arriving after the deletion gets `ErrNotFound`, falls into
   the event-fallback branch (`handlers.go:326-336`), overwrites the durable chat messages, publishes
   `clarification.answered`, and returns **200**. The agent already consumed answer A inside its
   blocked tool call; answer B rewrites the transcript and starts a second turn. After a backend
   restart the in-memory map is empty, so there is no 409 path at all and **both** answerers take the
   fallback.

2. **No ownership check.** `POST /api/v1/clarification/:id/respond` takes a bare `pending_id`.
   The global auth middleware attaches identity (`internal/backendapp/main.go`), but the clarification
   handlers are constructed with only store/repo/message dependencies
   (`internal/clarification/handlers.go:59-82`) and `httpRespond` (`:255`) never reads the context
   identity. The same hole covers `GET /:id`, `GET /:id/wait`, and `POST /:id/cancel`. Under enforced
   auth, any PAT holder who learns a `pending_id` can read and answer another user's question. This
   is the one clarification path that bypasses `task/service` entirely — `get_task_conversation_kandev`,
   which reaches the same messages, *is* scoped (`ListMessagesPaginated` calls
   `AuthorizeSessionAccess`, `service_messages.go:328`).

3. **Bundle resolution is not transactional.** `applyAnswersToMessages`
   (`internal/clarification/handlers.go:447`) logs per-question failures and continues, and the system
   still publishes and resumes. `UpdateClarificationMessageForQuestion`
   (`internal/task/service/service_messages.go:713`) is an ordinary read-modify-write on
   `metadata`. A bundle can therefore end up half-answered in the transcript while the caller is told
   it succeeded.

The MCP tools are thin wrappers over the fix for (1)–(3). The fix is the deliverable.

## Terminology

- **Bundle** — one clarification request: 1..4 questions sharing one `pending_id`.
- **Resolution** — the terminal outcome of a bundle: `answered`, `rejected`, or `cancelled`.
- **Claim** — the atomic act of becoming the bundle's resolver. Exactly one caller claims a bundle.
- **Winner** — the caller whose claim succeeded. **Loser** — any later caller for the same bundle.
- **Unscoped caller** — no identity in request context (event bus, pollers, orchestrator) or a
  synthetic identity (auth disabled). Matches `callerScope` in `task/service/service_access.go:29`.
- **Applying request** — the single request that won the claim and is executing step 5. Distinct from
  a *loser*, which performs no step 5.

## Data model

### `clarification_resolutions` (new table)

One row per bundle, written at most once per terminal outcome. This row — not the in-memory map, not
the per-question message metadata — is the authority on whether a bundle is resolved.

| Column | Type | Notes |
|---|---|---|
| `pending_id` | TEXT PRIMARY KEY | The bundle identifier. Primary key is the claim. |
| `session_id` | TEXT NOT NULL | Resolved from the bundle's durable messages (`task_session_messages.task_session_id`). **This is the FK column** — see the cascade rule below. |
| `task_id` | TEXT NOT NULL DEFAULT `''` | Resolved at claim time (M5). Diagnostic and query convenience only; **not** an FK and never an authorization input after the claim. `''` is legal, because a legacy bundle can carry an empty `task_id` (M5). |
| `status` | TEXT NOT NULL | `answered` \| `rejected` \| `cancelled`. |
| `response` | TEXT NOT NULL | JSON-serialized **normalized** `clarification.Response` (N3a) — the winning payload, replayed verbatim to losers. Never empty; see M6 for the cancel/reject shape. |
| `resume` | TEXT NOT NULL DEFAULT `'pending'` | `pending` \| `published` \| `failed` \| `not_applicable`. The recorded resume outcome R8a replays to losers. Write ordering is M7. |
| `resolved_by` | TEXT NOT NULL DEFAULT `''` | User ID of the winner; `''` for an unscoped caller. Diagnostic only, never an authorization input. |
| `source` | TEXT NOT NULL | `web` \| `mcp` \| `internal`. Diagnostic only; never an authorization or branching input. The entry-point mapping is M10. |
| `resolved_at` | TIMESTAMP NOT NULL | Claim time, server clock. |

- **M1.** The table SHALL be created in `initSchema` for fresh databases **and** by an idempotent
  migration in `runMigrations()`, per `apps/backend/AGENTS.md` ("Schema & migrations").
- **M2. Cascade.** The table SHALL declare
  `FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE`.
  *Correction to an earlier draft of this spec:* `task_session_messages` does **not** have a foreign
  key on `task_id` — that column is `task_id TEXT DEFAULT ''` with no constraint, and the table's only
  cascades are `task_session_id → task_sessions(id)` and `turn_id → task_session_turns(id)`
  (`internal/task/repository/sqlite/base_schema.go`, `CREATE TABLE task_session_messages`). Clarification
  messages are therefore removed transitively when their **session** row goes, and deleting a task
  removes them only because it removes the task's sessions. Cascading the resolution rows on
  `session_id` matches that real precedent, keeps the table bounded on the same schedule as the
  messages it mirrors, and — unlike an FK on `task_id` — cannot be violated by the empty-`task_id`
  bundles described in M5.
- **M3. No backfill, and what that implies for legacy bundles.** No migration SHALL create a row for
  any bundle that already exists at upgrade time. A bundle still *pending* at upgrade has no row and
  is claimable normally. A bundle already *resolved* before the upgrade also has no row —
  permanently, since nothing backfills one — and is excluded from L1 and from the workflow guard by
  **D4a conjunct 2** (its messages are all terminal), never by conjunct 1. A resolution row is
  therefore never a precondition for correct legacy behavior, which is what makes the no-backfill
  decision safe rather than merely cheap.
- **M4. Dialect portability.** `internal/task/repository/sqlite` also runs against PostgreSQL
  (`apps/backend/AGENTS.md`). Every statement this spec adds SHALL be dialect-portable: JSON access
  goes through `internal/db/dialect` (`JSONExtract`, `JSONExtractIsNotNull`) rather than raw
  `json_extract`, timestamp expressions through the same package's helpers, and the claim uses the
  conflict form in M8. The `resolved_at` column SHALL be written from Go rather than from a
  dialect-specific SQL `now()` expression.
- **M5. Resolving `task_id` and `session_id` for a bundle.** `session_id` is
  `task_session_messages.task_session_id`, which is `NOT NULL` and FK-constrained, so it is always
  present for a bundle that has durable messages. `task_id` is read from
  `task_session_messages.task_id`, which is `TEXT DEFAULT ''` and **can legitimately be empty**:
  `httpCreateRequest` (`clarification/handlers.go:110-134`) logs a warning and continues with
  `taskID = ""` when the session lookup fails, so such bundles exist in the wild. When the message's
  `task_id` is empty, the system SHALL resolve it from the bundle's session row
  (`task_sessions.task_id`, declared `NOT NULL`). Without this rule an empty-`task_id` bundle would
  pass `AuthorizeTaskAccess(ctx, "")` to a lookup that fails for every scoped caller **including its
  own owner**, making it permanently unanswerable.
- **M5a. When neither source yields a `task_id`, the bundle is unresolvable.** Reachable only if the
  session row is missing or its `task_id` is the empty string. Such a bundle SHALL be treated as
  not-found (A5) for every **scoped** caller, and SHALL be **omitted from L1 for every caller,
  including an unscoped one**. Whether it remains *answerable* by `pending_id` depends on which of
  the two arms it is, and **M5b decides that** — the empty-`task_id` arm stays answerable by an
  unscoped caller (A4), which is what preserves single-user behavior; the missing-session arm does
  not, because M8a's foreign key can never be satisfied.
  The listing half of that decision is deliberate and is the one a builder would otherwise invent.
  Listing it for unscoped callers only would require the L1c join to become an outer join, which in
  turn forces L3's mandatory `task_id` to carry some invented value for that one row — and L3 has no
  such value, so an MCP client parsing the field would meet a `null` or an empty string with no stated
  meaning. Omitting it keeps the L1c join inner, keeps L3's `task_id` always a real task ID, and costs
  nothing that matters: the bundle is still visible to a human in the chat transcript, still reachable
  through `get_task_conversation_kandev`, and still answerable and cancellable by `pending_id`. The
  state is in any case near-degenerate — `task_session_messages.task_session_id` is FK-constrained to
  `task_sessions`, so a bundle with durable messages cannot normally have a missing session row.
- **M6. `response` payload per outcome.** The stored `response` is always a serialized
  `clarification.Response` (`clarification/types.go:47`) with `pending_id` set and `responded_at` set
  to the claim time:
  - `answered` — `answers` holds the normalized answer entries (N3a); `rejected` is `false`;
    **`reject_reason` is the empty string `""`, whatever the caller supplied.** A submission carrying
    `rejected: false`, a full `answers` array and a non-empty `reason` is reachable on both surfaces
    — `RespondBody` (`clarification/handlers.go:249-253`) binds `reject_reason` unconditionally, with
    no coupling to `rejected` — and no other rule covers it: N8a forbids only `rejected: true`
    *combined with* answers, and N3a rule 3 would otherwise store the stray reason verbatim. The
    stray value is **discarded, not rejected**: adding a fourth validation error for a field the
    caller merely left set would break submissions that work today for no gain, whereas discarding it
    costs nothing — a reason has no meaning for an answer that was given. This also makes N3a's
    equivalence class total: two otherwise-identical answered submissions differing only in a stray
    `reason` now produce byte-identical answer payloads, which is what the N3a test asserts.
  - `rejected` — `answers` is an empty array; `rejected` is `true`; `reject_reason` carries the
    caller's reason, or `""` when none was supplied.
  - `cancelled` — `answers` is an empty array; `rejected` is `true`; `reject_reason` is the exact
    string `"cancelled"`. A cancel has no caller-supplied answer payload, so this is the shape an
    answer that loses to a cancel receives under R2, and it is caller-visible contract rather than an
    implementation detail.
- **M6a. Every key M6 names is always present, and `encoding/json` on the existing struct will not
  deliver that.** M6 and N3a rule 4 promise an **empty array** and an **empty string** where a value
  is absent; the Go type they name emits neither. `clarification.Response`
  (`clarification/types.go:44-52`) declares `Answers []Answer json:"answers,omitempty"`,
  `Rejected bool json:"rejected,omitempty"` and `RejectReason string json:"reject_reason,omitempty"`,
  and `Answer.SelectedOptions` carries `omitempty` too. `omitempty` omits the **key entirely** for an
  empty slice, a `false` bool or an empty string — producing neither `[]` nor `""` nor `null`. Marshal
  the struct as it stands and a `rejected` bundle's stored `response` has no `answers` key at all, an
  `answered` bundle's has no `rejected` key, and an MCP or HTTP client reading `response.answers` on
  a rejection gets `undefined`. That is precisely the shape instability L4a forbids on the sibling
  list tool, arriving through the payload instead.
  Therefore: the `response` written to the column, replayed to losers (R2, N4) and returned in the
  R10 envelope SHALL be produced by an **explicit serialization that always emits `answers`,
  `rejected`, `reject_reason`, and each entry's `question_id`, `selected_options` and `custom_text`**,
  using `[]` for an empty array and `""` for an empty string, independently of the struct's tags.
  The `clarification.Response` **struct tags SHALL NOT be changed**: they are the wire shape of
  `ask_user_question_kandev`'s own tool result, which *Out of scope* freezes, and widening them here
  would alter what every blocked agent receives — a change this card does not own. A dedicated
  serialization for the resolution payload is the only option that satisfies M6, N3a rule 4 and that
  exclusion at once, so it is named here rather than left to the builder to pick among three.
  This binds the stored column and the response envelope only; it does not change what the in-memory
  waiter delivers to a blocked agent, which is the `clarification.Response` value itself and never a
  JSON document produced here.
- **M7. `resume` write ordering.** The claim (step 4) inserts `resume = 'pending'`. After step 5
  completes, the applying request SHALL update the row's `resume` to `published`, `failed`, or
  `not_applicable`. This is the only permitted post-claim mutation of a resolution row, it SHALL NOT
  change any other column, and it SHALL NOT alter `claimed` semantics: R2 still applies to every later
  caller. A loser that reads `resume = 'pending'` (the applying request has not finished, or died
  mid-step-5) SHALL report `resume: "pending"` verbatim rather than guessing — that is the honest
  answer, and it is distinguishable from both success and failure.
- **M7a. When the M7 `resume` update itself fails.** Reachable when step 5 completed but the
  single-column write that records its outcome does not — a database error arriving after the claim,
  the message updates and the event have all already succeeded. Every neighbouring failure in this
  spec is pinned (R5 for message updates, R8c for a failed publish, M9 for a vanished conflict row),
  so this one is pinned too rather than left to the builder:
  - The applying request SHALL report the `resume` value it actually **computed** under R8a — what
    happened — and NOT the stale `pending` still sitting on the row.
  - Its HTTP status SHALL be whatever R10 already gives that outcome. A failed `resume` write SHALL
    NOT turn a 200 into a 500. R5's 500 exists because a failed per-question **message** update loses
    transcript state; this loses only bookkeeping about a resume that has already happened or already
    failed, and the answer itself is recorded either way. That is why R10 has no row for it: it is
    not an outcome of the request.
  - The failure SHALL be logged.
  - The row keeps `resume = 'pending'`, so **losers read `pending` indefinitely** for a bundle that
    may in fact have resumed. This is accepted, not repaired: M7 already defines `pending` as the
    honest "we do not know", and a loser genuinely does not know — the only caller that observed the
    outcome is the applying request, and it has already been told the truth.
  No retry is specified; retrying this write is the same class of thing as retrying a failed publish,
  which *Out of scope* already declines. **No fifth `resume` value is introduced**, consistently with
  R8a's closing clause and R8c.
- **M8. The claim statement.** The claim SHALL be
  `INSERT INTO clarification_resolutions (...) VALUES (...) ON CONFLICT (pending_id) DO NOTHING`,
  followed by a read of the row when zero rows were affected. A bare `INSERT` that relies on catching
  a primary-key violation is **not** acceptable: on PostgreSQL a failed statement aborts the
  surrounding transaction, so the follow-up read fails unless it is wrapped in a savepoint. The
  `ON CONFLICT ... DO NOTHING` form is the idiom already used in this package
  (`base_migrations.go:821`) and behaves identically on both dialects. *An earlier draft also cited
  `document.go:291` here; that statement is `ON CONFLICT(task_id, key) DO UPDATE SET …`, an upsert
  with different semantics, and the citation is withdrawn. `base_migrations.go:821` is the package's
  DO NOTHING precedent.*
- **M8a. The claim insert can fail on the `session_id` foreign key.** M2 puts
  `FOREIGN KEY (session_id) REFERENCES task_sessions(id)` on the row, and M8's
  `ON CONFLICT (pending_id) DO NOTHING` targets the **primary key** — it suppresses a duplicate
  `pending_id` and nothing else. A missing `task_sessions` row therefore surfaces as a foreign-key
  violation: an error, neither "one row affected" nor "zero rows affected". Two states reach it, and
  both are real:
  1. **The claim-window race.** The session row is deleted between step 1, which read the bundle's
     messages successfully, and step 4. M9 already accepts that this window exists and is reachable —
     it pins the mirror-image case where the session vanishes between a *conflicting* insert and the
     follow-up read. This is the same race arriving one step earlier, with no prior winner.
  2. **An orphaned bundle**: `clarification_request` messages whose session row is already gone.
     `task_session_messages.task_session_id` is FK-constrained, so this is not normally reachable,
     but it is not impossible on a database that historically ran without foreign-key enforcement.

  In **both** cases the system SHALL treat the bundle as not-found (A5) rather than surfacing a raw
  database error or retrying. This mirrors M9's rationale verbatim — *a bundle whose session is gone
  has no agent left to resume* — and it keeps one status code for one underlying condition: without
  it, the same deleted-session race would return 404 when it lands after a conflict and 500 when it
  lands before one. The failure SHALL be logged with the `pending_id`. No row is written, so nothing
  is left half-claimed, and R10 needs no new row: this outcome is A5's existing 404.
  **Consequence for M5a, stated rather than left implicit:** a resolution row cannot exist without a
  session row, so an orphaned bundle is not merely unclaimable by a scoped caller — it is
  permanently unclaimable by **every** caller, unscoped included. See M5b.
- **M5b. Correction to M5a's and *Failure modes*' answerability claim.** M5a says a bundle whose
  `task_id` resolves from neither source "remains answerable by `pending_id` by an unscoped caller",
  and the final *Failure modes* bullet repeats it. That promise holds for **one** of M5a's two arms
  and not the other, and the distinction is now stated because M8a makes it load-bearing:
  - **`task_id` is the empty string on both the message and the session row.** The session row
    exists, so the claim's foreign key is satisfiable. This bundle **is** answerable and cancellable
    by an unscoped caller, exactly as M5a promises, and single-user installs can always clear it.
  - **The session row is missing.** The claim can never satisfy M2's foreign key, so the bundle is
    unclaimable by anyone (M8a) and every answer and cancel returns 404. It is **not** answerable,
    and the earlier blanket sentence was wrong about it.

  M5a's SHALL-clauses are unchanged — not-found for scoped callers, omitted from L1 for every
  caller. Only the descriptive consequence is corrected. Nothing is lost by accepting this: a bundle
  with no session row has no session to resume, its `agent_disconnected` waiter is long gone, and G1
  no longer lets its messages wedge turn-complete, so there is nothing left that needs clearing.
- **M9. The conflict read can find nothing.** Between a conflicting insert and the follow-up read, the
  winning row can disappear — M2 cascades it away if the bundle's session row is deleted in that
  window. When the read returns no row, the system SHALL treat the bundle as not-found (A5) rather
  than retrying the insert. Retrying would race the same cascade indefinitely, and a bundle whose
  session is gone has no agent left to resume.
- **M10. Which entry point stamps which `source`.** The value names the **surface the resolution
  arrived through**, not the identity or user agent of the client, and the mapping is exhaustive:
  - `web` — the REST endpoints `POST /api/v1/clarification/:id/respond` and `POST /:id/cancel`. A PAT
    script calling the same route also records `web`. That is accepted rather than papered over:
    there is no reliable way to distinguish a browser from a script on one HTTP route, the column is
    diagnostic only, and inventing a fourth value to guess at it would make the enum less trustworthy
    rather than more.
  - `mcp` — `answer_question_kandev`.
  - `internal` — **reserved**. It is defined for a resolution originated inside the backend with no
    external caller, and **this spec routes no such flow through `ResolveBundle`**: the canceller's
    detach path explicitly does not change `status` and therefore does not resolve, wiring
    `ExpireSessionAndNotify` is out of scope, and X1 puts only the REST cancel *endpoint* through the
    claim (which is `web`). No code path in this change writes `internal`; it exists so that a future
    orchestrator- or canceller-originated resolution has a value that does not require a migration.
    A builder SHALL NOT invent a producer for it.

### Existing state, unchanged in shape

- **In-memory** `PendingClarification` (`clarification/types.go:56`) keeps its role: it is the
  delivery channel that unblocks a still-waiting agent inside its own turn. It is no longer the
  duplicate guard.
- **Durable per-question messages** (`type = 'clarification_request'`, created in
  `backendapp/adapters.go:975`) keep their metadata shape verbatim: `pending_id`, `question_id`,
  `question` (`{id,title,prompt,options[{option_id,label,description}]}`), `question_index`,
  `question_total`, `context`, `status`, and `response` once answered. The canceller additionally
  writes `agent_disconnected: true` on detach (`clarification/canceller.go`, `markMessagesDetached`)
  **without** changing `status`, so a detached bundle is still `pending` and still listable. This spec
  adds no metadata key and changes no existing one.

## The resolution claim

A single service-layer operation replaces the ad-hoc sequence in `httpRespond`. Both the REST
handler and the MCP tool call it; there is no second implementation.

`ResolveBundle(ctx, pendingID, outcome) -> (Resolution, claimed bool, error)`

Ordered steps, all-or-nothing at step 4:

1. **Resolve identity of the bundle.** Read the bundle's durable `clarification_request` messages by
   `pending_id`, deriving `session_id` and `task_id` per M5. If no messages exist, return not-found.
2. **Authorize.** Call `TaskService.AuthorizeTaskAccess(ctx, taskID)`. A denial returns not-found.
3. **Validate the outcome** against the bundle's question set (N6–N8b, per N8c). A **cancel** outcome
   skips this step entirely (X5).
4. **Claim.** Insert the `clarification_resolutions` row using M8. The insert has **three** possible
   outcomes, not two. One row affected means this caller won: return it with `claimed = true`. Zero
   rows affected means someone else already won: read the existing row and return it with
   `claimed = false` (M9 covers the read finding nothing). The insert **failing** — which
   `ON CONFLICT ... DO NOTHING` does not prevent, because it suppresses only the primary-key
   conflict — is the third, and M8a decides it.
5. **Applying request only:** apply per-question metadata to the durable messages (D2), deliver to the
   in-memory waiter, publish the resume event, then record `resume` per M7. In that order.

Steps 4 and 5 are ordered deliberately: the durable claim lands **before** any event is published,
so a loser can never trigger a resume.

Losers perform none of step 5. They receive the winner's stored `response` and `resume`.

### Cancel joins the same claim

`POST /api/v1/clarification/:id/cancel` is a resolution like any other. It claims the bundle with
`status = cancelled`, marks every question `cancelled`, and publishes `clarification.cancelled`.

- **X1.** Cancel SHALL go through `ResolveBundle` and SHALL obey R1–R6.
- **X2.** Cancel SHALL succeed for a bundle whose in-memory entry is gone, provided the bundle has an
  unresolved durable record. Today it returns 404 in that case (`clarification/handlers.go:409-417`),
  which means a bundle stranded by a restart cannot be cancelled at all — the durable pending guard
  keeps blocking workflow transitions with no way to clear it.
- **X3.** Cancel SHALL close the in-memory `CancelCh` whenever an entry exists, so a blocked agent
  unblocks immediately rather than waiting out its timeout. **This holds for a cancel that wins the
  claim and for one that loses it**, and it is a deliberate, narrowly-scoped exception to
  "losers perform none of step 5".
- **X3a. Why the losing cancel is exempted, and exactly what it may do.** Read without X3's second
  sentence, R2 makes a losing cancel a pure no-op, and the state where that hurts is reachable: after
  an R5/R5a partial application the bundle is claimed while the waiter is still **live** — application
  stops before delivery — so the agent sits blocked inside its tool call with R2 making every retry a
  no-op and X1 making every cancel a no-op. Nothing would remain that could release it short of the
  2h store timeout or stopping the session out of band. That is G2's wedge one layer down, in memory
  instead of in the guard.
  The exemption is safe because closing `CancelCh` is not a resolution and touches nothing R2
  protects. A losing cancel SHALL close `CancelCh` and SHALL do **nothing else**: it SHALL NOT modify
  the `clarification_resolutions` row, SHALL NOT modify any `clarification_request` message, SHALL
  NOT publish any event, and SHALL return R2's normal `claimed: false` response carrying the
  **winner's** `status`, `response` and `resume` — not a cancelled one. The only observable effect is
  that the blocked tool call returns its existing "clarification cancelled (agent moved on)" error
  instead of hanging; no turn is started and no transcript is written.
  This also needs **no A4 item**, which is the second reason to prefer it: `httpCancelRequest`
  (`clarification/handlers.go:407-418`) already closes the channel unconditionally whenever an entry
  exists, so a losing cancel behaves here exactly as it does today. Scoping X3 to the winner instead
  would have *changed* today's behavior and required enumerating it.
- **X4.** Cancel stores the M6 `cancelled` payload and reports `resume: "not_applicable"`, since
  `clarification.cancelled` is not a resume event.
- **X5. Cancel SHALL skip `ResolveBundle` step 3 entirely.** N6–N8b all validate a caller-supplied
  answer payload, and a cancel supplies none: read literally against an unscoped step 3, a cancel
  would fail N6's "exactly one entry per question" on its empty `answers` and could never claim a
  bundle at all. Cancel is therefore exempt rather than being made to impersonate the M6 rejection
  shape — the M6 `cancelled` payload is constructed by the server **after** the claim, so validating
  against it would be validating the server's own output. Concretely: cancel runs steps 1, 2, 4 and 5;
  it can return 404 (A5) and 500 (R5) but never 400, which is why R10's cancel rows are exactly those
  **two** plus the two 200s — **four** rows.

`GET /api/v1/clarification/:id` and `GET /:id/wait` remain reads of the in-memory store; this spec
adds only the A2 authorization check to them, and A7 pins their status codes, which are **not** both
404 today.

## The workflow pending-clarification guard

`Service.sessionHasPendingClarification` (`orchestrator/clarification_guard.go`) is a second reader of
the same durable rows, and it decides whether a session may complete a turn. It calls
`FindPendingClarificationMessagesBySessionID` (`task/repository/sqlite/message.go:432`), which selects
`clarification_request` messages whose `metadata.status = 'pending'`, and
`turnCompleteBlockedByUserInput` defers every `on_turn_complete` transition while any exist. It fails
closed: a query error also blocks.

- **G1.** The guard's membership test SHALL become **D4a**, in full — both conjuncts, with the same
  meaning they carry for L1. Concretely: the guard keeps a **status predicate** (conjunct 2) and
  gains an **exclusion** on top of it (conjunct 1) — a `clarification_request` message whose
  `pending_id` has a `clarification_resolutions` row SHALL NOT count as pending, regardless of its
  own `metadata.status`. The exclusion is an **addition, never a replacement** for the status
  predicate: dropping the status predicate would make every historically-answered clarification count
  as pending and block turn-complete forever on every session that ever answered one (D4a states
  why). G4 describes the join the exclusion needs, and **G5 states the one respect in which the
  guard's existing status predicate must change.**
- **G5. The guard's status predicate SHALL widen to D3's effective-pending form.** The existing query
  tests strict equality — `JSONExtract(metadata,'status') = 'pending'`
  (`task/repository/sqlite/message.go:437`) — and a message whose `status` key is **absent** yields
  SQL `NULL`, which does not satisfy it. D3 and D4a conjunct 2 define effectively-pending as
  including an absent or unrecognized status. Left strict, the guard would implement a **narrower**
  rule than L1 while G1 and D4a both claim the two consumers share one membership test, and the
  divergence is caller-visible: a bundle with a defective `status` would be listed by
  `list_pending_questions_kandev` and be answerable, yet the agent's turn could complete while that
  answerable question was still outstanding. The predicate SHALL therefore be satisfied when the
  extracted `status` is `pending`, is SQL `NULL` (the key is absent), or is any value outside the
  five `clarification.Status` constants D3 enumerates — equivalently, when it is **not** one of the
  four terminal values `answered`, `rejected`, `cancelled`, `expired`. Express it against the
  terminal set rather than by enumerating the pending case, so a status value added later fails
  closed rather than silently ceasing to block.
  Three consequences are stated so they are not re-derived. **(i)** This is a deliberate change to an
  existing production query, not a port: it newly blocks `on_turn_complete` for a session holding a
  defect-status clarification. That is the correct direction — G3 already fails closed on a *query*
  error, and a corrupt row is the same kind of "we do not know", so the guard should hold rather than
  release. **(ii)** It cannot wedge a session, because such a bundle is listable (L1 admits it via
  D3) and answerable, and resolving it writes a resolution row that conjunct 1 then excludes.
  **(iii)** It does not resurrect the legacy bundle: `answered`, `rejected`, `cancelled` and
  `expired` are all in the terminal set, so a pre-upgrade resolved bundle stays excluded exactly as
  D4a conjunct 2 requires. The widened expression SHALL go through `internal/db/dialect` per G4 and
  M4, and its `NULL` handling SHALL be written so that it behaves identically on SQLite and
  PostgreSQL.
- **G2.** G1 is required, not cosmetic. Without it the R5 partial-application state and the
  claim-then-crash state each wedge the session permanently: the messages stay `pending` forever, so
  the guard blocks every turn-complete transition, while R2 makes re-answering a no-op and X1 makes
  cancelling a no-op — so no caller can ever clear it. That is exactly the failure X2 exists to fix,
  reintroduced through a different door.
- **G3.** The guard SHALL keep failing closed on a query error, unchanged.
- **G4.** `pending_id` lives in message metadata JSON, not in a column, so the exclusion in G1 joins
  `clarification_resolutions` on a JSON-extracted key. That expression SHALL go through
  `internal/db/dialect` (`JSONExtract`), exactly as the existing
  `FindPendingClarificationMessagesBySessionID` query already does for `metadata.status`
  (`task/repository/sqlite/message.go:432-447`), so the guard keeps working on both dialects (M4).

## Determinism and boundary rules

These fix the values every ordering, cursor, and comparison in this spec depends on.

- **D1. A bundle's `created_at` is the minimum `created_at` across its `clarification_request`
  messages.** The bundle's messages are written in a loop
  (`backendapp/adapters.go:975-1020`) and therefore carry distinct timestamps; without this rule
  L6's ordering is not well defined. `MIN(created_at)` is the bundle's creation instant.
- **D2. Per-question durable updates within one winning resolution are applied in `question_index`
  ascending, then message `created_at` ascending, then message `id` ascending.** The two extra keys
  are not decoration: `questionIndexFromMetadata` (`clarification/handlers.go:736-749`) returns **0**
  for a missing or unparseable `question_index`, so a legacy or corrupt bundle can present several
  questions all claiming index 0. Message `id` is a primary key, so the composite is total and the
  partial prefix R5 leaves behind is reproducible rather than arbitrary.
- **D3. `metadata.status` on a `clarification_request` message is one of `pending`, `answered`,
  `rejected`, `cancelled`, or `expired`** — the `clarification.Status` constants
  (`clarification/types.go:69-75`). `expired` is written by `Canceller.markMessagesExpired`
  (`clarification/canceller.go`), reached from `ExpireSessionAndNotify`, which is **currently unwired**
  — it has no production caller today but is in-tree and test-covered, and `isTerminalStatus`
  (`canceller.go:36-42`) already treats `expired` as terminal alongside the other three. This spec
  therefore treats `expired` as a recognized terminal status rather than asserting nothing writes it.
  A message with an absent or unrecognized `status` SHALL be treated as `pending`, so a bundle is
  never hidden from L1 by a metadata defect. Per D4, none of these values decides whether a bundle is
  *resolved* — only the resolution row does. They do decide whether it is *pending*, via D4a
  conjunct 2, which is the distinction D4/D4a draws.
- **D4. The `clarification_resolutions` row is the sole authority on whether a bundle is
  *resolved*.** A bundle that has a row is resolved, whatever its per-question message metadata says;
  a projection that still reads `pending` does not override the row. That one-way rule is what stops
  a half-applied bundle (R5) or a claim-then-crash bundle from being re-listed or re-answered on the
  strength of stale metadata. **D4 settles resolvedness only. It is NOT the whole membership test** —
  that is D4a, and the difference is load-bearing rather than editorial.
- **D4a. A bundle is *pending* — listable by L1 (L2) and countable by the workflow guard (G1) — iff
  BOTH of these hold:**
  1. it has **no** `clarification_resolutions` row (D4); **and**
  2. **at least one** of its `clarification_request` messages is effectively pending per D3, where an
     absent or unrecognized `status` counts as pending.

  Both conjuncts are normative. Neither is an optimization and neither may be dropped. Conjunct 2
  means the **same** thing for both consumers: the effectively-pending test — absent or unrecognized
  counts as pending — binds the workflow guard exactly as it binds L1. The guard's existing query
  implements a narrower, strict-equality version of it today, and **G5** is what widens it; without
  G5 this "iff" would be false for the guard, and a defect-status bundle would be listed and
  answerable while failing to block turn-complete.

  Conjunct 2 exists to exclude the **all-terminal legacy bundle**, and without it this spec breaks
  every existing install on the day it ships. M3 performs no backfill, so every clarification answered
  *before* this change is a set of messages carrying a terminal `status` and **no** resolution row —
  permanently, because nothing ever removes them: `UpdateClarificationMessageForQuestion`
  (`task/service/service_messages.go:715`) flips `metadata.status` in place, and no purge, retention
  or cleanup path deletes a `clarification_request` message. Under conjunct 1 alone every one of
  those bundles would count as unresolved, so `list_pending_questions_kandev` would return the
  install's entire clarification history as claimable, and answering one would publish
  `clarification.answered` and resume an agent whose question was answered months ago. The workflow
  guard would count the same messages and, failing closed (G3), would block every `on_turn_complete`
  transition on every session that has ever answered a clarification — G2's wedge, reintroduced by
  the upgrade itself.

  Conjunct 1 exists to exclude the resolved-but-not-fully-applied bundle, which conjunct 2 alone
  would admit. L12's mixed-status bundle satisfies both conjuncts and is therefore listed; that is
  the rule working, not an exception to it.

  Three other states fall out of conjunct 2 and are named so they are not re-derived. A **detached**
  bundle stays pending, because `markMessagesDetached` sets `agent_disconnected: true` without
  touching `status`. A bundle with **unparseable `question` metadata** (L15) or **no `question_id`**
  (L16) also stays pending, because `status` is a separate metadata key from `question` and is
  unaffected by either defect. An **`expired`** bundle does NOT stay pending: `expired` is terminal
  per D3, so conjunct 2 excludes it — correct, since its waiter is gone and nothing is left to answer,
  and in any case `ExpireSessionAndNotify` has no production caller today.
- **D5. A task with an empty `workspace_id` is visible to every authenticated caller**, matching
  `authorizeTaskID` (`task/service/service_access.go:67-69`). Its bundles appear in L1 for everyone.
  This preserves the pre-auth-row behavior used everywhere else in the codebase.
- **D6. Pagination is not a snapshot.** A bundle resolved between two pages simply disappears; because
  the cursor is a `(created_at, pending_id)` key rather than an offset, no other bundle shifts
  position or is skipped. A bundle created with a `created_at` earlier than an already-issued cursor
  (only reachable via clock adjustment) SHALL NOT be returned to that cursor's holder; it is returned
  on any fresh, cursor-less call.
- **D7. `age_seconds` uses the server clock and is floored at 0**, so a bundle whose stored
  `created_at` is in the future reports `0` rather than a negative number.

## What

Each criterion below is observable through the HTTP API, the MCP tool surface, or the database.

### Resolution semantics (applies to every answering surface)

- **R1.** When a caller submits a resolution for a `pending_id` that has no
  `clarification_resolutions` row, the system SHALL insert that row with the caller's outcome and
  SHALL treat that caller as the winner.
- **R2.** When a caller submits a resolution for a `pending_id` that already has a
  `clarification_resolutions` row, the system SHALL NOT modify that row, SHALL NOT modify any
  `clarification_request` message, SHALL NOT publish any event, and SHALL return a success response
  carrying the **already-recorded** `status`, `response`, and `resume` plus `claimed: false`.
- **R2a.** R2 SHALL be evaluated **after** validation (N8c), not before. A malformed submission
  against an already-resolved bundle SHALL therefore receive the same validation error it would
  receive against an unresolved one, and SHALL NOT receive the winner's payload. Returning success for
  a request the system could not parse would tell a caller its malformed answer had been accepted.
- **R3.** While two callers submit resolutions for the same `pending_id` concurrently, exactly one
  SHALL be recorded as the winner and the other SHALL observe R2. This SHALL hold when the two
  callers are served by different HTTP requests and when one is the web UI and the other is the MCP
  tool.
- **R4.** When the winner's resolution is recorded, the system SHALL write the per-question durable
  message updates before publishing any resume event.
- **R5.** If any per-question durable message update fails while applying a winning resolution, the
  system SHALL NOT publish a resume event, SHALL leave the `clarification_resolutions` row in place
  (the bundle stays resolved and un-reanswerable), SHALL set the row's `resume` to `failed`, and SHALL
  return an error to **the applying request** naming the bundle as partially applied. The applying
  request SHALL NOT receive a success response. Later callers observe R2 as normal and read
  `resume: "failed"`, which is how they learn the bundle is recorded but not fully applied.
- **R5a. Application STOPS at the first failed per-question update.** The system SHALL NOT attempt the
  remaining questions; it stops immediately and proceeds to R5's error handling. Because D2 fixes a
  total order over the bundle's questions, the set actually applied is therefore always a **prefix**
  of that order — which is the property D2's rationale already asserts ("the partial prefix R5 leaves
  behind is reproducible rather than arbitrary") and which R5's own wording, read alone, does not
  deliver.
  This is a deliberate change from today's behavior and the builder must not port the existing loop
  unchanged: `applyAnswersToMessages` (`clarification/handlers.go:447`) logs each per-question failure
  and **continues**, in both its rejected branch and its answers branch, so today's leftover is an
  arbitrary subset determined by which individual writes happened to fail. Continuing buys nothing
  here — R2 makes the bundle non-reanswerable either way, and repairing the state is explicitly out
  of scope — while a prefix is reproducible, assertable, and cheaper. If the **first** update fails,
  the prefix is empty and no message is modified at all; that is a legal R5 outcome, not a special
  case. Enumerated in A4 item 6 as part of the same partial-application carve-out.
- **R6.** When the backend restarts between a bundle's creation and its resolution, a subsequent
  resolution SHALL still be claimed exactly once — the claim SHALL NOT depend on in-memory state.
- **R7.** When the winner's bundle has a live in-memory waiter, the system SHALL deliver the response
  through that waiter (resolving the agent's blocked tool call in the same turn) and SHALL publish
  `clarification.primary_answered`.
- **R7a.** Liveness is checked at delivery time, not at claim time, and delivery MAY fail: the entry
  can be removed between step 4 and step 5 by the 2h store timeout, by `CancelSession` when the
  agent's turn completes, or by `CancelRequest`. When delivery to a believed-live waiter fails for any
  reason, the system SHALL fall through to **the no-waiter branch for that resolution's own outcome** —
  R8 for an answer, R9 for a rejection — rather than reporting a delivery that did not happen. The
  fall-through is outcome-aware, not unconditional: a failed **rejection** delivery SHALL NOT publish
  `clarification.answered`, because that would resume the blocked agent with "User declined to answer"
  on precisely the path R9 exists to keep quiet. The resulting `resume` value is decided by the branch
  actually taken. "Any reason" includes `Store.Respond` returning `ErrAlreadyResponded`:
  M3 performs no backfill, so a bundle created before the upgrade can have an in-memory entry that was
  already resolved while no `clarification_resolutions` row exists, letting a caller win the claim and
  then find the waiter already satisfied. That is a delivery failure like any other, not an error to
  the caller.
- **R8.** When the winner's resolution is an **answer** (`status = answered`) and the bundle has no
  live in-memory waiter, the system SHALL publish `clarification.answered` so the orchestrator resumes
  the agent in a new turn.
- **R8b. R8 is scoped to the answered outcome, deliberately.** Every outcome has exactly one no-waiter
  branch and the three do not overlap: an answer takes R8, a rejection takes R9, a cancel takes X4
  (X1 scopes cancel to R1–R6, so R7–R9 never apply to it). Without this qualifier a rejection with no
  live waiter would satisfy R8's antecedent as well as R9's, and R8's "SHALL publish
  `clarification.answered`" would directly contradict R9's "SHALL NOT publish a resume event". A
  builder reading them in order would resume an agent the human meant to dismiss, discarding today's
  behavior at `clarification/handlers.go:316-324` that R9 exists to preserve.
- **R8a.** Every successful resolution response SHALL carry a `resume` field with one of four values.
  `resume` answers exactly one question — **did the answer reach the agent, or the machinery that
  resumes it?** — and its value SHALL be decided by the following rules, evaluated **in order**, with
  the first matching rule winning. The order is the contract; the clauses are not independent tests.
  1. `pending` — the applying request has not yet recorded an outcome (M7).
  2. `failed` — the durable claim succeeded but the per-question message updates could not all be
     applied (R5), **or** there was no live waiter and the resume event could not be published or its
     event context could not be resolved.
  3. `not_applicable` — the outcome publishes no resume event by design: a rejection with no live
     waiter (R9) or a cancel (X4).
  4. `published` — the **resolution** was delivered to a live waiter — an answer or a rejection
     alike, per R7 and R8c — **or** it was an **answer** with no live waiter whose resume event was
     published. The first clause says "resolution" and not "answer" deliberately: elsewhere this spec
     uses "answer" as the technical `status = answered` outcome in opposition to a rejection (R8,
     R8b), and rule 3 is scoped to a rejection with **no** live waiter, so an answer-only reading of
     rule 4 would leave a successfully delivered rejection (N9's live-waiter branch) matching no rule
     at all and force the builder to invent a value.
  Rule 2 precedes rule 3 deliberately. A rejection or a cancel can also hit R5's partial application,
  and if `not_applicable` were tested first it would mask that failure behind a value meaning "nothing
  needed to happen" — while R10 requires that same request to return 500. Ordered this way, a
  partially applied cancel reports `resume: "failed"` and a 500, consistently.
  `not_applicable` is not weakened by a failed publish of `clarification.cancelled` or
  `clarification.stale_dismissed`: neither is a resume event, so rule 2's second clause is vacuous for
  them and the value stays `not_applicable`. Such a failure SHALL be logged. No fifth `resume` value
  is introduced for it.
  A loser (R2) SHALL carry the value stored on the winning row. Today this failure is logged and
  reported as plain success (`clarification/handlers.go:515-528`), so an answerer cannot tell a
  recorded-and-resumed answer from a recorded-but-stranded one.
- **R8c. A successful delivery to a live waiter is `published` even if the accompanying
  `clarification.primary_answered` publish fails.** Rule 3 is scoped to the no-waiter path on purpose.
  On the live-waiter path the *delivery itself* is the resume — the agent's blocked tool call returns
  with the answer inside its own turn — and `clarification.primary_answered` is not what resumes it:
  its subscriber (`orchestrator.handleClarificationPrimaryAnswered`) only writes the task-in-progress
  projection and arms the 15s watchdog that re-triggers a fallback resume if the agent's MCP client
  dropped the response. Reporting `failed` there would tell an answerer its answer never reached the
  agent when it demonstrably did. The failed publish SHALL be logged, and the known consequence — no
  in-progress projection and no armed watchdog for that bundle — is accepted rather than surfaced as a
  fifth `resume` value. Without this rule an unordered reading of R8a assigns both `published` (rule 4,
  "delivered to a live waiter") and `failed` (rule 3, "could not be published") to the same state.
- **R9.** When a resolution is a rejection and no live waiter exists, the system SHALL mark every
  question in the bundle `rejected`, SHALL publish `clarification.stale_dismissed`, SHALL NOT publish
  a resume event, and SHALL report `resume: "not_applicable"`. (Preserves today's stale-dismiss
  behavior at `clarification/handlers.go:316-324`.)
- **R9a. "Every question" in R9 includes one whose `question_id` is empty.** Today's rejected branch
  of `applyAnswersToMessages` skips those outright (`if questionID == "" { continue }`), so rejecting
  an L16 bundle — the one outcome L16 says such a bundle is answerable by — currently marks nothing
  at all, and the transcript still shows pending questions after a rejection the system reported as
  successful. Under this spec the per-question update SHALL be applied by matching the bundle's
  `clarification_request` **messages** in D2 order, not by matching a non-empty `question_id`, so an
  L16 bundle is fully marked `rejected` and L16's promise actually holds end to end. This rule
  removes the skip rather than reclassifying it: a skipped message was never an update *failure* and
  never triggered R5, and under R9a there is nothing left to skip. Enumerated as A4 item 8.

### REST status codes and response envelope

- **R10.** The four clarification endpoints SHALL use exactly these outcomes. This table is the
  contract W2 and the E2E duplicate-submit case are written against.

| Endpoint | Outcome | Status | Body |
|---|---|---|---|
| `POST /:id/respond` | won the claim | 200 | `{"success": true, "claimed": true, "status": "...", "response": {...}, "resume": "..."}` |
| `POST /:id/respond` | lost the claim (R2) | 200 | same shape, `"claimed": false`, winner's `status`/`response`/`resume` |
| `POST /:id/respond` | validation failure (N6–N8b) | 400 | `{"error": "<message naming the offending field>"}` |
| `POST /:id/respond` | unknown / unauthorized `pending_id` | 404 | `{"error": "clarification request not found"}` |
| `POST /:id/respond` | partial application (R5) | 500 | `{"error": "<message naming the bundle as partially applied>"}` |
| `POST /:id/cancel` | won the claim | 200 | same envelope, `status: "cancelled"`, `resume: "not_applicable"` (X4) |
| `POST /:id/cancel` | lost the claim (R2) | 200 | same envelope, `"claimed": false`, winner's `status`/`response`/`resume` |
| `POST /:id/cancel` | unknown / unauthorized `pending_id` | 404 | `{"error": "clarification request not found"}` |
| `POST /:id/cancel` | partial application (R5) | 500 | `{"error": "<message naming the bundle as partially applied>"}` |
| `GET /:id` | found / not found or unauthorized | 200 / 404 | unchanged shape |
| `GET /:id/wait` | resolved / timeout or missing entry | 200 / 504 | unchanged shape (A7) |

  `POST /:id/cancel` has **no 400 row**, and that is a statement rather than an omission: X5 exempts
  cancel from step-3 validation, so there is no caller-supplied payload it can reject. Its 404 and 500
  rows exist because X1 puts cancel through the same A1–A5 authorization and the same R1–R6 claim as
  an answer, so both outcomes are reachable; R5 forbids a success response to the applying request, so
  a partially applied cancel returning 200 would contradict it directly.

- **R11.** The `success` key SHALL be retained on `POST` responses so an existing client that only
  checks `res.ok` and `success` keeps working. `claimed`, `status`, `response`, and `resume` are
  additive. The **409** that `httpRespond` returns today for a duplicate submit
  (`handlers.go:301-305`) SHALL be removed: under R2 a duplicate is a 200 with `claimed: false`.
  Clients that treat 409 as success (W2) continue to work because they also treat 200 as success.

### Authorization

- **A1.** `POST /api/v1/clarification/:id/respond` SHALL deny a request whose caller cannot access
  the task owning that `pending_id`.
- **A2.** `GET /api/v1/clarification/:id`, `GET /api/v1/clarification/:id/wait`, and
  `POST /api/v1/clarification/:id/cancel` SHALL apply the same check.
- **A3.** A denial under A1/A2 SHALL be a **404** with a body indistinguishable from a nonexistent
  `pending_id`. It SHALL NOT be a 403 and SHALL NOT include the question text, option labels, task
  ID, session ID, or workspace ID.
- **A4.** An unscoped caller SHALL be authorized for every `pending_id`. Concretely: **with auth
  disabled, no caller is ever newly denied**, and any behavior this spec does not deliberately change
  is unchanged. The deliberate changes below apply in **both** auth modes — there is one code path,
  not an auth-disabled variant:
  1. a duplicate submission returns 200 with `claimed: false` instead of 409 (R2, R11);
  2. every successful response gains `claimed`, `status`, `response`, and `resume` fields (R8a, R10);
  3. cancelling a bundle whose in-memory entry is gone succeeds instead of returning 404 (X2);
  4. a `pending_id` with no durable messages returns 404 instead of today's 200 (A5) — today
     `respondViaEventFallback`'s failure path falls through to `httpRespond`'s unconditional
     `c.JSON(http.StatusOK, gin.H{"success": true})` at `handlers.go:336`;
  5. the new validation rules reject submissions that pass today: unknown option IDs (N8),
     `rejected: true` combined with answers (N8a), and `reason`/`custom_text` over 2000 characters
     (N8b);
  6. a bundle whose per-question durable updates cannot all be applied now returns **500** to the
     applying request instead of today's 200 (R5, R10), **and application now stops at the first
     failure instead of continuing** (R5a). Today `applyAnswersToMessages` logs each per-question
     failure and continues, and `httpRespond` still reaches its unconditional
     `c.JSON(http.StatusOK, gin.H{"success": true})` — so a half-applied bundle is currently reported
     as a plain success, and the messages it leaves updated are an arbitrary subset rather than a
     D2-ordered prefix;
  7. an answers submission against a bundle whose messages carry no `question_id` is now rejected with
     a validation error (N6a) instead of being accepted. Today `expectedQuestionIDs` skips empty ids,
     so such a bundle yields an empty expected set and `validateRespondAnswers` takes its permissive
     branch (`handlers.go:350-354`) and accepts anything; N6a removes that branch from the
     `ResolveBundle` path.
  8. rejecting a bundle whose messages carry **no `question_id`** now marks those messages `rejected`
     instead of leaving them `pending` (R9a). Today `applyAnswersToMessages`' rejected branch skips
     every message with an empty `question_id`, so the rejection is recorded as the bundle's outcome
     but never reaches the transcript — which makes L16's "answerable only by rejection" true in the
     response and false in the chat;
  9. `GET /:id/wait` on a `pending_id` with **no durable messages** now returns **404** instead of
     today's **504** (A5a). This is the only item on this list that changes a status code on a *read*
     endpoint, and the only one whose pre-change value is not 200. It follows from A8 resolving the
     authorizing task from the durable messages and A7 running that resolution before the in-memory
     read; A5a records why the alternative — exempting the reads from A5 — was rejected as an
     existence-disclosure oracle rather than accepted as a cheaper compatibility story.
  This list is exhaustive. A behavior not named here SHALL be identical to the pre-change behavior for
  an unscoped caller on all four endpoints. Items 6 through 9 are enumerated here for the same reason
  as 1–5: A4 is the compatibility contract the auth-disabled test run is written against, so a change
  omitted from this list would make that run assert the wrong thing.
- **A5.** A `pending_id` whose durable messages cannot be found SHALL produce the same 404 as A3.
  This also covers the M5a case where a bundle's `task_id` cannot be resolved from either source, for
  a scoped caller, and the M8a case where the claim's foreign key cannot be satisfied.
- **A5a. A5 binds all four endpoints, in both auth modes — including `GET /:id/wait`, which returns
  504 for that input today.** A8 resolves the authorizing `task_id` from the durable messages on all
  four endpoints and A7 puts that resolution ahead of the in-memory read, so a `pending_id` with no
  durable messages fails at step 1 on every endpoint, for an unscoped caller as much as a scoped one.
  On `GET /:id` that is a 404 today and SHALL stay one. On `GET /:id/wait` it is **504 today**
  (`httpWaitForResponse`, `clarification/handlers.go:233-243`) and SHALL become **404**, for every
  caller in both auth modes. That change is deliberate and is enumerated as **A4 item 9**.
  It is enumerated rather than avoided because the alternative breaks a security property. Exempting
  the two read endpoints from A5 would leave `GET /:id/wait` returning 404 for a **foreign** bundle
  (A3) and 504 for a **nonexistent** one — a status-code oracle that tells an unauthorized caller
  exactly which `pending_id`s exist, which is the precise distinction A7 says a caller "can never
  make" and A3 exists to prevent. A compatibility promise can be amended by adding a line to A4; an
  existence-disclosure channel cannot be amended at all. The 504 is preserved where it means
  something: an authorized caller waiting on a bundle that **does** exist (A7).
- **A6.** `POST /api/v1/clarification/request` (creation) SHALL be unchanged by this spec. It is
  called by the in-session agent path, which carries the task owner's identity via `internal/mcp/scope`.
- **A7.** `GET /:id/wait` returns **504** `StatusGatewayTimeout` today for a missing or drained entry,
  not 404 (`httpWaitForResponse`, `clarification/handlers.go:233-243`); only `GET /:id` returns 404
  (`httpGetRequest`, `:221-231`). The A2 authorization check SHALL run **before** the in-memory read on
  both, so an unauthorized caller receives A3's 404 on either endpoint and can never distinguish a
  foreign bundle from a nonexistent one by status code. An **authorized** caller's post-authorization
  behavior is unchanged **for a bundle whose durable messages exist**: 404 from `GET /:id`, 504 from
  `GET /:id/wait`. The qualifier is load-bearing and is A5a's subject — a `pending_id` with **no**
  durable messages never reaches the post-authorization read on either endpoint, so `GET /:id/wait`
  returns 404 rather than 504 for it, in both auth modes. That is the only surviving 504: a wait that
  drains or times out on a bundle that genuinely exists.
- **A8.** On all four endpoints the authorizing `task_id` comes from the durable messages via M5,
  never from the in-memory `Request.TaskID`. A7's ordering requires it — the check runs before the
  in-memory read, so the in-memory value is not yet available — and it also removes a discrepancy:
  the in-memory `Request.TaskID` is populated from the same possibly-empty session lookup as the
  message column (`httpCreateRequest`), so trusting it would reintroduce the M5 hole on two endpoints
  while the other two resolved it correctly.

**BREAKING CHANGE, intentional.** An authenticated caller that today answers any bundle by bare
`pending_id` will receive 404 for bundles outside its workspaces. The only in-tree production caller
is the web UI, which answers from a task it can already read, so it is unaffected — subject to W1
below. The nine auth-mode-independent changes enumerated in A4 are also intentional and accepted.

### Web client

- **W1.** The web UI's clarification POSTs (`apps/web/hooks/domains/session/use-clarification-group.ts`,
  both call sites) SHALL send credentials. They currently use a bare `fetch` with the default
  `credentials: "same-origin"`, unlike the shared client (`lib/api/client.ts:80`, `credentials: "include"`).
  In split-origin dev mode (`__KANDEV_API_PORT` set, browser and API on different ports —
  `lib/config.ts:36-40`) the session cookie is therefore dropped, so with auth enabled the request is
  already rejected by the global middleware before this spec's changes. Authorization becomes
  load-bearing on this route, so the omission is fixed here.
- **W2.** The web UI SHALL treat a `claimed: false` success as a successful submit and close the
  overlay, the same way it already treats a 409 (`use-clarification-group.ts:52-56`).
- **W3.** On a `claimed: false` response the web UI SHALL apply the **winner's** returned `response` to
  the bundle's local message metadata, not the answers this client submitted. `postClarificationBatch`
  and `postClarificationSkip` currently inspect only `res.ok` / `res.status`, never the body, and
  `safeApplyResolvedStatus` then optimistically writes **this client's** answers into
  `metadata.response` (`use-clarification-group.ts:66-97`). Because R2 guarantees the losing submit
  produces no `session.message.updated` broadcast, nothing would ever correct that write: the losing
  tab would display the losing answers as the transcript until a reload, contradicting Scenario 2.
  Both call sites SHALL therefore parse the response body and pass the winner's answers to the
  optimistic update. When `claimed` is absent from the body (an older backend), the client SHALL keep
  today's behavior and apply its own answers.
- **W3a. The winner's `status` can be `cancelled`, and the client SHALL apply it as such.** An answer
  that loses to a cancel receives `status: "cancelled"` with the M6 cancelled payload (*Failure
  modes*), but `safeApplyResolvedStatus` and `applyResolvedStatusToBundle` accept only
  `"answered" | "rejected"` today (`use-clarification-group.ts:66-97`). That union SHALL be widened to
  include `"cancelled"` and the client SHALL write the status the backend actually returned, rather
  than coercing a cancelled winner into one of the two existing values. Coercion is the failure mode
  worth naming: rendering a cancelled bundle as `answered` would show the losing tab a transcript
  state that never existed, and rendering it as `rejected` would misattribute a cancel to a human
  skip. All three values arrive in the same response body W3 already requires the client to parse, so
  this costs one union member and no extra request.
- **W4.** The overlay's free-text input (`clarification-input-overlay.tsx`, which enforces no limit
  today) SHALL stop a human at the same boundary N8b enforces on the server, so a long answer is
  caught at the input rather than returning an opaque 400 on submit.
- **W4a. The client-side guard SHALL count runes, and the HTML `maxLength` attribute alone SHALL NOT
  be the mechanism.** N8b's cap is 2000 UTF-8 **runes**, chosen explicitly so "a non-Latin answer is
  not cut short". The `maxLength` attribute counts **UTF-16 code units**, so every astral character —
  emoji, and much CJK Extension B — consumes two of its budget. `maxLength={2000}` would therefore
  stop a user at 1000 emoji that the server would have accepted: not the opaque-400 failure W4 exists
  to prevent, but the mirror-image one, a client refusing input the contract permits, with no error
  message at all because `maxLength` fails silently. The two criteria cannot both be satisfied by
  that attribute, which is why the mechanism is named here instead of implied.
  The overlay SHALL enforce the limit by counting runes (code points) on the input's value, and
  SHALL surface the boundary to the user rather than silently truncating. `maxLength` MAY be set in
  addition, as a coarse backstop, only at a value that cannot reject anything the server accepts —
  that means **no lower than 4000**, since a rune is at most two UTF-16 code units. It SHALL NOT be
  set to 2000. Boundary values follow N8b exactly: 2000 runes is accepted, 2001 is not, and the count
  is over code points, not bytes and not UTF-16 units.

### `list_pending_questions_kandev` (external MCP surface only)

- **L1.** The tool SHALL return every unresolved clarification bundle in the workspaces visible to
  the caller, and no bundle outside them.
- **L1a.** Visibility SHALL be resolved by the same rule as `filterWorkspacesForCaller`
  (`task/service/service_access.go:184-196`): an unscoped caller sees every workspace; a scoped caller
  sees a workspace whose `owner_id` is empty or matches the caller. The tool SHALL resolve that
  workspace-ID set first and apply it as a **predicate of the bundle query**, not as a post-query
  filter over an already-limited page. Per-bundle `AuthorizeTaskAccess` calls are NOT the mechanism
  here: N calls per page would be N round trips, and — decisively — filtering after `LIMIT` would make
  L10's `limit` mean "rows examined" rather than "bundles returned", so a page could come back short
  or empty while more matching bundles exist, and a cursor-polling caller could not distinguish that
  from exhaustion. `limit` counts **returned** bundles.
- **L1b.** When the resolved workspace-ID set is empty — a scoped caller who owns no workspaces and
  can see no unowned ones — the tool SHALL omit the `workspace_id IN (...)` **term** from the L1c
  disjunction rather than issuing it, because an empty `IN ()` is a syntax error on both dialects.
  It SHALL NOT short-circuit the whole query, and there is **no** whole-query short-circuit branch in
  this tool. L1c's other two disjuncts are predicates on the task row rather than on the caller, so
  they remain applicable no matter what the caller can see: a scoped caller with no visible workspaces
  can still legitimately be shown bundles whose task has an empty `workspace_id` or a dangling
  workspace reference, exactly as `authorizeTaskID` would allow them to answer those bundles. An
  earlier draft of this criterion returned the L11 empty response in this case; that was wrong for
  precisely that reason and is superseded here.
- **L1c. The predicate SHALL reproduce `authorizeTaskAccess`, not a narrower workspace-membership
  test.** The list tool and the answer tool MUST agree on visibility: a bundle `answer_question_kandev`
  will answer and `list_pending_questions_kandev` will not show is a silent discovery hole, and
  discovery is what this card exists to add. `AuthorizeTaskAccess` delegates to `authorizeTaskID`
  (`task/service/service_access.go`), which **allows** a scoped caller in three cases a bare
  `workspace_id IN (visible set)` term cannot express. All three SHALL be included as additional
  disjuncts:
  1. **The task's `workspace_id` is empty.** `authorizeTaskID` returns nil early on
     `task.WorkspaceID == ""`. This is D5's case, and D5 already states such bundles "appear in L1 for
     everyone" — the predicate must actually deliver that.
  2. **The task's `workspace_id` names no existing workspace row.** `authorizeTaskID` treats a failed
     workspace lookup as visible, by an explicit `//nolint:nilerr` fallback whose comment reads "a
     dangling workspace reference should not hide the task from the single user who can already see
     everything else about it". `filterWorkspacesForCaller` cannot express this because it filters a
     list of workspaces that *exist*.
  3. **The bundle's task is the one M5 resolves, not the raw message column.** The join SHALL reach
     the task through M5's rule — `task_session_messages.task_id` when non-empty, otherwise the
     bundle's session row's `task_id` — because the obvious `JOIN tasks ON tasks.id = messages.task_id`
     silently drops every legacy empty-`task_id` bundle (M5 documents that these exist in the wild)
     for **every** caller, unscoped included.
  For an unscoped caller the predicate is satisfied unconditionally, matching `callerScope` returning
  `ok=false`. The **Permissions** section's one-line summary of this rule names only the workspace-owner
  case for brevity; L1c is the normative statement and wins where they differ.
- **L1d. Mechanism for L1c, so it is not invented.** Disjunct 1 is a comparison against the literal
  empty string, because `tasks.workspace_id` is `TEXT NOT NULL DEFAULT ''` with no foreign key — empty
  is its default, not an anomaly. Disjunct 2 is an absence test against the `workspaces` table
  (`NOT EXISTS (SELECT 1 FROM workspaces w WHERE w.id = t.workspace_id)`, or the equivalent left join
  with a null check); it is a plain relational predicate and needs **no** `internal/db/dialect` helper,
  unlike the JSON access M4 and G4 govern. Disjunct 3 is the workspace-ID set from L1a, subject to
  L1b's empty-set handling. The three are combined with `OR` and the whole disjunction is `AND`-ed
  with **both conjuncts** of the D4a membership test, all inside the single bundle query — per L1a
  visibility is a query predicate,
  never a post-`LIMIT` filter, so `limit` still counts returned bundles.
- **L2.** The tool SHALL derive its result from the durable `clarification_request` messages, NOT from
  `Store.ListPending`, and SHALL therefore return the correct set after a backend restart. The
  membership test is **D4a**, and **both** of its conjuncts SHALL be applied as predicates of the
  bundle query: the absence of a `clarification_resolutions` row **and** the presence of at least one
  effectively-pending message per D3. Neither conjunct is optional. Dropping the row check would show
  a bundle that is resolved but whose message updates did not all land; dropping the status check
  would return every clarification the install has ever answered, because M3 backfills no row for
  them (D4a). An earlier draft of this criterion called the status predicate an index-assisted
  pre-filter that *may* be applied — that was wrong, because applying it changes the result set
  rather than only the plan, and it is superseded here. A metadata defect still cannot hide a bundle:
  D3 makes an absent or unrecognized `status` count as `pending`, so conjunct 2 admits it. Message
  `status` is additionally reported per question (L12).
- **L3.** Each returned bundle SHALL carry: `pending_id`, `task_id`, `session_id`, `created_at`
  (RFC3339 UTC), `age_seconds` (integer, server clock minus `created_at`, floored at 0), `context`,
  and `questions`.
- **L4.** Each question SHALL carry `question_id`, `title`, `prompt`, `status`, and `options`, where
  each option carries `option_id`, `label`, and `description` — the exact identifiers
  `answer_question_kandev` accepts.
- **L4a. Every field named in L3 and L4 is always present and is never JSON `null`.** This is the
  general rule L15 and L16 are special cases of, and it applies to every bundle, not only malformed
  ones. When a value is unknown, absent, or unparseable:
  - a **string** field (`task_id`, `session_id`, `context`, `question_id`, `title`, `prompt`,
    `option_id`, `label`, `description`) SHALL be emitted as the **empty string** `""`;
  - an **array** field (`questions`, `options`) SHALL be emitted as an **empty array** `[]`;
  - `age_seconds` SHALL be emitted as an integer, floored at 0 per D7, never null;
  - `created_at` and `status` are the two fields with no unknown case, so the empty-string rule never
    applies to them: `created_at` is D1's `MIN(created_at)` over rows that exist by construction, and
    an absent or unrecognized message `status` is reported as the string `pending` per D3 rather than
    as `""`.
  Without this rule L15 pins only `prompt` and `options` while L4 still mandates `title`, `status` and
  the per-option fields, so a builder emitting Go zero values through one path and `null` through
  another produces a shape that changes between bundles. An MCP client iterating `options` or reading
  `label` on a degraded bundle would then meet `null` where the contract implied an array or a string
  and fail at the client, with no criterion catching it. `task_id` in particular is never empty in
  practice, because M5a omits from L1 the one class of bundle whose `task_id` cannot be resolved.
- **L5.** Questions within a bundle SHALL be ordered by the same total key as D2: `question_index`
  ascending, then message `created_at` ascending, then message `id` ascending.
- **L6.** Bundles SHALL be ordered by `created_at` ascending, then by `pending_id` ascending. The
  `pending_id` tiebreak exists solely to make the order total for cursor pagination; it carries no
  meaning. Oldest-first is the useful order because the oldest blocked agent is the most urgent.
- **L7.** The tool SHALL accept an optional `workspace_id`. When supplied, results SHALL be limited to
  bundles whose task — resolved per M5, as in L1c point 3 — carries exactly that `workspace_id`, and
  a workspace the caller cannot access SHALL produce the same empty-result response as an empty
  workspace (no existence leak, consistent with A3).
- **L7a. How `workspace_id` composes with L1c, and what the empty string means.** L7 was written
  before L1c's three disjuncts existed, so the composition is stated here rather than left to the
  builder:
  1. **`workspace_id` is one additional predicate `AND`-ed with the whole L1c disjunction** — never a
     substitute for it, and never a fourth disjunct. Adding an `AND` term can only ever NARROW the
     result, so supplying `workspace_id` can never reveal a bundle that an unfiltered call would
     have withheld.
  2. L7's empty-result guarantee for an inaccessible workspace **falls out of that `AND`** and needs
     no short-circuit branch. For a workspace that exists and the caller cannot see: disjunct 1 is
     false (the task's `workspace_id` is non-empty), disjunct 2 is false (the workspace row exists),
     and disjunct 3 is false (it is not in the L1a set). L1b's "there is no whole-query short-circuit
     in this tool" therefore holds without exception, here included.
  3. A **fabricated or deleted** `workspace_id` leaves disjunct 2 satisfiable, so bundles whose task
     carries that dangling reference ARE returned. That is correct rather than a leak: they are
     exactly the bundles `authorizeTaskID`'s dangling-workspace fallback already lets this caller
     answer, and an unfiltered L1 call would have listed them anyway (L1c point 2). Nothing is
     disclosed about a workspace that does not exist.
  4. **`workspace_id: ""` means the parameter was not supplied.** It SHALL NOT be read as "filter to
     tasks whose `workspace_id` is empty", even though L1c disjunct 1 and D5 make that a real and
     meaningful class. An omitted optional string decodes to `""`, so the other reading would
     silently narrow every caller who omits the argument down to the empty-workspace class alone.
     Those bundles are reached by making an **unfiltered** call, where disjunct 1 includes them for
     everyone. This spec deliberately provides no filter that selects them exclusively; that is a
     named exclusion, not an oversight.
- **L8.** The tool SHALL accept an optional `created_since` (RFC3339). When supplied, only bundles
  with `created_at >= created_since` SHALL be returned. The parameter is named for the column it
  filters: a bundle's `created_at` never changes, so an `updated_since` name would promise
  change-feed semantics this tool does not have.
- **L9.** The tool SHALL accept an optional `cursor` — the opaque encoding of the last returned
  `(created_at, pending_id)` pair — and SHALL return only bundles ordered strictly after it under
  L6. It SHALL return a `next_cursor` when more results exist and an empty `next_cursor` when they
  do not.
- **L10.** The tool SHALL accept an optional `limit`, defaulting to **50** and capped at **200**. A
  `limit` below 1, or absent, SHALL be treated as the default; a `limit` above the cap SHALL be
  clamped to the cap rather than rejected. Per L1a, `limit` counts bundles actually returned.
- **L11.** The response envelope SHALL be
  `{"bundles": [...], "count": <int>, "next_cursor": "<string>"}`. `bundles` is the top-level array —
  each element carries its own `questions` array per L3, so the two names never collide. `count` is
  the number of elements in `bundles` on **this page** and is always equal to `len(bundles)`; it is a
  convenience, not a total. When no bundle matches, the tool SHALL return an empty `bundles` array,
  `count: 0`, and an empty `next_cursor`, and SHALL NOT return an error.
- **L11a.** A grand total across all pages is deliberately NOT provided. Producing one would require a
  second, unbounded, authorization-filtered aggregate query per call, and it would be stale the moment
  it was computed (D6). Callers that need to know whether more work exists read `next_cursor`.
- **L12.** A bundle with **no resolution row** whose per-question messages disagree on `status` (some
  `pending`, some terminal) SHALL be returned, and each question SHALL carry its own `status`. This is
  the legacy half-applied bundle: before this spec, `applyAnswersToMessages`
  (`clarification/handlers.go:447`) logged per-question failures and continued, and no claim row
  existed, so a bundle could be left mixed with nothing recording it. Such a bundle satisfies both
  D4a conjuncts — no row, and at least one message still `pending` — so it is genuinely unanswered
  and answerable; returning it lets a caller finish it. A bundle that **has** a resolution row is
  excluded by conjunct 1 regardless of its message statuses — it is resolved, and listing it would
  invite an answer that can only ever return `claimed: false`. A bundle with **no** row whose
  messages are **all** terminal is excluded by conjunct 2: that is the pre-upgrade legacy bundle, it
  is not half-applied, and nobody needs to finish it.
- **L13.** An unparseable `created_since` or an unparseable/corrupt `cursor` SHALL produce a
  validation error naming the offending argument. Neither SHALL be silently ignored: a caller polling
  with a cursor it thinks is being honored, that is in fact being dropped, re-reads the whole backlog
  every tick and re-answers questions it already handled.
- **L14.** Supplying both `cursor` and `created_since` SHALL be accepted; both constraints apply
  (intersection). `cursor` is the pagination position, `created_since` is a filter, and they are
  independent.
- **L15.** A bundle whose durable messages carry no parseable `question` metadata SHALL still be
  returned, with the affected question carrying its `question_id` and, per L4a, an empty `title`,
  `prompt` and `options` rather than null ones. Such a question cannot be answered by option ID —
  every `selected_options` entry would fail N8 against an empty option list — but hiding the bundle
  would strand its agent invisibly, so it remains answerable by `custom_text` alone (N7) or by
  rejection. (`questionFromMessageMetadata`, `clarification/handlers.go:751`, already degrades this
  way for the resume-summary path.)
- **L16.** A bundle whose messages carry **no** `question_id` at all in either
  `metadata.question_id` or `metadata.question.id` SHALL still be listed under L15, and its questions
  SHALL carry an empty `question_id`. Such a bundle cannot satisfy N6 — an entry with an empty
  `question_id` fails N6 condition 2, and any non-empty id fails condition 3 against an expected set
  of empty strings (N6a) — so it is answerable only by `rejected: true`, which needs no
  `question_id`. This claim rests specifically on N6 condition 2; it is not a consequence of the
  count or uniqueness rules, which a one-question bundle would otherwise satisfy. The tool SHALL
  report it rather than hide it, so a caller can at least clear the blocked agent. R9a is what makes
  that rejection reach the transcript as well as the response; without it the one available outcome
  would mark nothing. Such a bundle stays listable until it is resolved, because its `status` is
  untouched by the missing `question_id` (D4a).

### `answer_question_kandev` (external MCP surface only)

- **N1.** The tool SHALL accept `pending_id` plus either `answers` (one entry per question) or
  `rejected: true` with an optional `reason`.
- **N2.** The tool SHALL resolve the bundle through the same `ResolveBundle` operation as the REST
  endpoint, and SHALL therefore inherit R1–R9 and A1–A5 without a second code path.
- **N3.** On winning, the tool SHALL return `claimed: true` with the recorded `status`, the
  normalized `response` (N3a), and `resume`.
- **N3a. Normalization** is the canonical form written to the `response` column and replayed to
  losers. It is defined so that two callers submitting semantically identical answers produce
  byte-identical **answer payloads** — that is, the `answers`, `rejected` and `reject_reason` fields.
  The guarantee is explicitly scoped to those fields and **excludes the two server-set fields**,
  `pending_id` and `responded_at` (rule 5). It has to be: `responded_at` is the claim time (M6), so two
  resolutions at different instants can never be byte-identical across the whole `Response`, and an
  unscoped reading of this promise would name an assertion no implementation can satisfy. A builder
  SHALL NOT freeze or normalize the clock to manufacture whole-payload identity — `responded_at` is
  real information a loser reads. Rules:
  1. `answers` entries are ordered by the bundle's own question order (L5), **not** by the order the
     caller supplied them.
  2. Within an entry, `selected_options` is ordered by the option's position in the question's
     `options` array, and exact duplicates are removed.
  3. `custom_text` and `reason` are stored verbatim after trimming leading and trailing whitespace.
     No other transformation is applied.
  4. An absent `answers`, `selected_options`, or `options` array is stored as an empty JSON array
     `[]`, never `null` — and never as an **omitted key**, which is what marshalling the existing
     struct would produce. **M6a states the mechanism** and is not optional here: the guarantee in
     this rule is unreachable through `encoding/json` on `clarification.Response` as tagged.
  5. Fields absent from `clarification.Response` are not invented; `pending_id` and `responded_at` are
     set by the server per M6.
- **N4.** On losing, the tool SHALL return `claimed: false` with the **winner's** recorded `status`,
  `response`, and `resume`, and SHALL NOT report an error. Answering an already-answered question is a
  successful no-op that tells the caller what the answer was.
- **N5.** A `pending_id` the caller cannot access SHALL produce the same not-found error text as a
  nonexistent one.
- **N6.** An `answers` array SHALL be rejected with a validation error, and SHALL NOT claim the
  bundle, when **any** of these four conditions holds. The list is exhaustive and is the complete
  rule; do not treat the citation as covering anything not written here.
  1. It does not contain exactly one entry per question in the bundle.
  2. An entry carries an **empty** `question_id`.
  3. An entry references a `question_id` not in the bundle's expected set.
  4. An entry repeats a `question_id` already used by an earlier entry.
  All four are the existing rule in `validateRespondAnswers` (`clarification/handlers.go:348-377`);
  condition 2 in particular is already implemented there as
  `if a.QuestionID == "" { return "answer N is missing question_id" }` and is enumerated explicitly
  because L16 and N6a both depend on it. Without condition 2 stated, a one-question L16 bundle would
  have an expected set of `{""}`, an answers array of one entry with `question_id: ""` would satisfy
  conditions 1, 3 and 4, and L16's "answerable only by rejection" claim would be false — the
  resulting per-question update would then match no message and land in R5 partial application on a
  bundle this spec says cannot reach it.
- **N6a.** The expected question-id set SHALL be derived from the bundle's durable messages, counting
  **one expected answer per `clarification_request` message** in the bundle. Today
  `validateRespondAnswers` falls back to permissive acceptance when it cannot determine that set
  (`handlers.go:350-354`); under this spec step 1 has already proven the messages exist, so the
  expected **count** is always derivable and that permissive branch SHALL NOT be reachable from
  `ResolveBundle`. The count is the number of `clarification_request` messages in the bundle; the
  expected id set is those messages' `question_id` values, and an id SHALL be included in the set even
  when it is the empty string. That last clause is the change from today's behavior: the existing
  `expectedQuestionIDs` *skips* empty ids, so a bundle whose messages carry none yields an **empty**
  set and reaches the permissive branch, which is exactly the hole A4 item 7 records as a deliberate
  break. A bundle whose messages carry no `question_id` (L16) therefore has a non-empty expected count
  but only empty-string ids, and every possible `answers` array fails N6 — an entry with an empty
  `question_id` fails N6 condition 2, and any non-empty id fails condition 3. Such a bundle is
  answerable only by rejection, as L16 states.
- **N7.** An answer entry MAY carry `selected_options` (option IDs), `custom_text`, or both. An entry
  carrying neither SHALL be accepted and SHALL render as "(no answer)" in the resume prompt,
  preserving `formatAnswerBody` (`clarification/handlers.go:835`).
- **N8.** A `selected_options` entry naming an `option_id` not present on that question SHALL be
  rejected with a validation error and SHALL NOT claim the bundle. *(This is stricter than today's
  REST endpoint, which does not check option IDs. External agents fabricate identifiers; the human at
  the keyboard clicks a rendered button. The check is applied on both surfaces so the two cannot drift.)*
  N8 constrains **membership only**. It does not constrain cardinality: `selected_options` is a slice
  (`clarification/types.go:38-42`) and nothing in the existing model marks a question single- or
  multi-select, so an entry naming several valid option IDs SHALL be accepted. Inventing a
  single-select rule here would reject answers the overlay itself can produce.
- **N8a.** A request carrying both `rejected: true` and a non-empty `answers` array SHALL be rejected
  with a validation error and SHALL NOT claim the bundle. The two are mutually exclusive outcomes and
  guessing which the caller meant would silently discard one of them. A request carrying
  `rejected: false` with an empty `answers` array is the N6 count-mismatch case and is likewise
  rejected, **including for a single-question bundle**: N7 governs an answer *entry* that carries
  neither field, and an empty array contains no entry at all, so N7 cannot apply to it. A caller that
  means "no answer" for a one-question bundle sends one entry with neither field (N7); a caller that
  means "I decline" sends `rejected: true` (N9).
- **N8b.** `reason` SHALL be capped at **2000 characters**; a longer value SHALL be rejected with a
  validation error rather than truncated, since the reason is replayed verbatim into the blocked
  agent's resume prompt. `custom_text` on an answer SHALL be capped at the same limit, per entry. The
  cap is enforced inside `ResolveBundle`, so it binds the REST endpoint and the web overlay as well as
  this tool (W4 adds the matching client-side guard). The limit counts UTF-8 **runes**, not bytes, so
  a non-Latin answer is not cut short at a third of the visible length.
- **N8c.** Validation (N6, N6a, N7, N8, N8a, N8b) SHALL run **before** the claim in step 4, so a
  malformed request never resolves a bundle, and before the R2 already-resolved check (R2a). A
  validation error SHALL leave the bundle answerable.
- **N9.** A rejection SHALL be recorded with `status = rejected` and SHALL produce the same
  agent-visible outcome as a human skip, in both waiter states:
  - **Live waiter** — the rejection is delivered through the waiter as a `clarification.Response` with
    `rejected: true` and the caller's `reject_reason`; the agent's blocked tool call returns that
    payload directly. `buildAnswerSummary` is not involved on this path.
  - **No live waiter** — R9 applies: every question is marked `rejected`,
    `clarification.stale_dismissed` is published, and no resume prompt is built at all.
  `buildAnswerSummary` (`clarification/handlers.go:788`), which renders "User declined to answer" with
  the reason appended, feeds the `answer_text` of the `clarification.answered` event and is therefore
  reached only when a rejection accompanies an answered-path resume — not on either branch above.
  The wire format does not distinguish the answerer: a reject from an external agent means the same
  thing to the blocked agent as a human skip.

### Surface placement

- **S1.** Both tools SHALL be registered for `SurfaceExternal` only.
- **S2.** Neither tool SHALL appear on `SurfaceKanbanTask`, `SurfaceOfficeTask`, or
  `SurfaceConfiguration`. In-session MCP scoping resolves to the workspace **owner**
  (`internal/mcp/scope/scope.go`), not to a task relationship, so a running agent on the kanban
  surface would be able to list and answer human questions across every task that owner can see.
  That defeats the human-input boundary and collides with autopilot's parent-only interaction model
  (`ask_parent_question_kandev`).
- **S3.** `ask_user_question_kandev` SHALL remain absent from the external surface, as today.
- **S4.** Neither tool SHALL be added to the session agent system prompt, since neither is visible to
  session agents.

### `list_tasks_kandev` enrichment

- **T1.** `list_tasks_kandev` SHALL include `task_pending_action` and
  `primary_session_pending_action` for each task, using the same projection the HTTP task list
  already returns (`GetPendingActionsForSessions`, `task/dto/dto.go:168-169`), so one call finds
  every blocked task in a workflow.
- **T2.** A task with no blocked session SHALL carry JSON `null` for both fields rather than an empty
  string, matching the HTTP DTO — both fields are `*string` with no `omitempty`
  (`task/dto/dto.go:168-169`), so the key is always present and its value is `null`.

### Catalog and documentation

- **C1.** `apps/web/lib/settings/external-mcp-tools.ts` SHALL list both new tools with localized
  descriptions, in a group whose title reflects answering agent questions.
- **C2.** The catalog's KNOWN DRIFT SHALL be closed in the same change. The backend registers **35**
  external tools (`TestServerModeExternal_ToolCount`, `internal/mcp/server/server_test.go:1008-1018`)
  and the catalog lists **30**; the file's own note and its test's note both say 33, which is itself
  stale. The five missing entries are `list_repositories_kandev`, `import_workflow_kandev`,
  `get_task_conversation_kandev`, `add_task_dependency_kandev`, and `remove_task_dependency_kandev`.
  After this change the catalog SHALL list **37** and the drift note SHALL be deleted rather than
  renumbered.
- **C3.** Every new catalog entry SHALL resolve to an existing `en/settings.json` key, per the
  existing pinning test.

## Permissions

This spec introduces no new permission concept. It applies the existing per-user workspace scoping
rule (`apps/backend/AGENTS.md`, "Opt-in authentication & per-user scoping") to a service that missed it:

- No identity in context, or a synthetic identity → unscoped, today's pre-auth behavior.
- Real identity → the bundle is visible if the workspace owning its task has an empty `owner_id` or an
  `owner_id` matching the caller, **and also** in the three further cases `authorizeTaskID` itself
  allows: the task has no workspace, the task's workspace row does not exist, or the task is the one
  M5 resolves from the session. This bullet is a summary; **L1c is the normative statement** and the
  list tool's query predicate SHALL be written against L1c, not against this line.
- Denial uses the not-found sentinel (`repoerrors.ErrTaskNotFound` via `AuthorizeTaskAccess`), so a
  foreign bundle and a missing bundle are indistinguishable.

The authorization input is the `pending_id` → `task_id` mapping read from the durable messages (M5).
It SHALL NOT be read from a caller-supplied `task_id`; a caller that supplies one alongside a
`pending_id` has it ignored.

External MCP callers reach this check because `DispatcherBackendClient.RequestPayload`
(`internal/mcp/server/dispatcher_backend_client.go:41-48`) passes the HTTP request context — carrying
the identity the auth middleware attached — straight into `Dispatch`.

**An earlier proposal to authorize via the MCP scope resolver was wrong.** `internal/mcp/scope`
attaches the owning identity of an in-session agent stream; it neither resolves a `pending_id` nor
authorizes one, and it does not apply to the external endpoint at all.

## Failure modes

- **Bundle's session is terminal or cancelled at answer time.** The claim still succeeds and the
  durable messages are still updated — the transcript is a record, not a live channel. The resume
  event is published; the orchestrator's existing handling applies. The response carries
  `resume: "published"` so the caller knows the answer was recorded and a resume was attempted.
- **Bundle's session row is deleted.** M2 cascades the resolution row away with the session, and the
  session cascade has already removed the bundle's messages, so step 1 finds nothing and the answer
  returns not-found (A5) — the same response as an unauthorized bundle, by design. This is the same
  path as a deleted task, since deleting a task deletes its sessions. The "session terminal" bullet
  above concerns a session that still exists in a terminal *state*, which is a different thing from a
  deleted row.
- **Event context cannot be resolved, or publication fails, on the no-waiter path.** Today the handler
  logs and returns success (`clarification/handlers.go:515-528`), so the answerer is told "success"
  while no work resumes. Under this spec the claim and the durable updates have already succeeded and
  MUST NOT be rolled back, but the row and the response carry `resume: "failed"` instead of
  `"published"`, and the caller can tell the two apart. The answer is recorded either way; only the
  resume is in doubt. Re-answering will not retry it — it returns R2, carrying `resume: "failed"`.
  This bullet is the **no-waiter** case (R8a rule 3). The live-waiter case is different and is
  governed by R8c: there the delivery already resumed the agent, so a failed
  `clarification.primary_answered` publish is logged and still reports `resume: "published"`.
- **Live-waiter REJECTION arms the resume watchdog.** R7 publishes `clarification.primary_answered`
  for any resolution delivered to a live waiter, a rejection included, and that subscriber arms a 15s
  watchdog whose fallback re-triggers a resume if the agent's MCP client appears to have dropped the
  response (`orchestrator/event_handlers_clarification.go`). For a **rejection** that fallback resume
  is the very thing R9 exists to suppress, so the two sit in tension. This is **accepted as a
  pre-existing hazard and deliberately not fixed here.** It is not introduced by this spec: today's
  code already publishes `primary_answered` on the live-waiter path regardless of outcome, and R7
  preserves that verbatim. The watchdog belongs to the orchestrator's resume machinery, which this
  spec leaves untouched on principle — *Out of scope* already declines to retry or re-route a resume
  for the same reason. It is also narrow: it fires only when a rejection was delivered to a live
  waiter **and** that delivery appears to have been lost, in which case the agent is unblocked
  either way and the disagreement is about which message it sees, not whether it resumes. Making the
  watchdog outcome-aware is a change to the orchestrator with its own lifecycle questions and is
  named in *Out of scope*.
- **Orchestrator receives the event but the prompt fails.** Out of scope for the answerer's response;
  the subscriber logs it (`orchestrator/event_handlers_clarification.go:150`). The answer remains
  recorded. Recovering a failed resume is the orchestrator's existing retry path
  (`retryClarificationAfterCancel`), unchanged.
- **Bundle is cancelled (`POST /:id/cancel`) concurrently with an answer.** Both go through the same
  claim. Whichever inserts first wins; the other observes R2. A cancel that loses to an answer
  returns the answer; an answer that loses to a cancel returns `status: cancelled` with
  `claimed: false` and the M6 cancelled payload, so the caller can see its answer was not applied.
- **In-memory waiter already timed out (2h) but the bundle is unresolved.** The claim succeeds and
  R8 applies: no waiter, so `clarification.answered` resumes the agent in a new turn.
- **Waiter disappears between the claim and delivery.** R7a: delivery fails, the system falls through
  to R8, and `resume` reflects whether that publish succeeded.
- **Two questions in a bundle, the first durable message write succeeds and the second fails.** R5:
  no event, an error to the applying request, `resume = failed` on the row, and the bundle stays
  claimed. Per R5a nothing beyond the failure point is attempted, so the applied set is the D2-ordered
  prefix — here, question 1 only. It is
  **not** re-listed (D4a conjunct 1 — the row exists) and is not re-answerable (R2), so no second
  agent turn can be triggered by a retry. Crucially it also does **not** wedge the session: G1 adds
  conjunct 1 as an exclusion over the guard's status predicate, so the still-`pending` messages no
  longer block turn-complete transitions. The
  transcript is stale and the `clarification_resolutions` row records the intended outcome.
- **Claim row written, process dies before durable updates.** The bundle is resolved but its messages
  still read `pending` and its `resume` is still `pending` (M7). It is excluded from the list
  (D4a conjunct 1),
  excluded from the workflow guard (G1), and a retry returns R2 with `resume: "pending"` — which is
  precisely the "we do not know" answer, and is distinguishable from both success and failure. The
  transcript shows an unanswered question that cannot be answered, visible to a human in chat and
  reconcilable from the row. Repairing it automatically is deliberately not attempted; see
  *Out of scope*.
- **Legacy bundle left half-applied by the pre-change code path.** No claim row exists, so it is still
  listed (L12) with mixed per-question status and is answerable. Answering it claims it and applies
  the remaining questions.
- **Legacy bundle already fully resolved before the upgrade — the common case on any existing
  install.** No claim row exists and none is ever created (M3), and its messages are all terminal, so
  D4a conjunct 2 excludes it: it is not listed by L1 and does not count toward the workflow guard.
  Its transcript is unchanged and a human still sees the answered question in chat. This bullet is
  here because conjunct 1 alone would admit every such bundle, which on a real install means the list
  tool returning the entire clarification history and the guard blocking turn-complete on every
  session that ever answered one.
- **Legacy bundle with an empty `task_id`.** M5: resolved from the session row, and then visible and
  listable normally — L1c point 3 requires the list query to use that same resolved task, so the
  bundle does not silently vanish from L1 while remaining answerable.
- **Bundle whose `task_id` resolves from neither source.** M5a: not-found for scoped callers, omitted
  from L1 for **every** caller including unscoped. Answerability splits by arm, per M5b: when the
  session row exists and both `task_id` values are empty, the bundle is still answerable and
  cancellable by `pending_id` by an unscoped caller, so single-user installs can always clear it;
  when the **session row is missing**, M8a's foreign key can never be satisfied, so the bundle is
  unclaimable by every caller and answers and cancels alike return 404. Near-degenerate in practice,
  since `task_session_messages.task_session_id` is FK-constrained to `task_sessions`.
- **Session row is deleted between step 1 and step 4.** M8a: the claim insert fails the `session_id`
  foreign key, no row is written, and the caller receives A5's 404 — the same answer M9 gives when
  the same deletion lands one step later, so one race does not produce two status codes.

## Scenarios

1. **External agent answers a live question.** Agent A on task T calls `ask_user_question_kandev` and
   blocks. An external client lists pending questions, sees T's bundle with its option IDs, and calls
   `answer_question_kandev`. The claim is inserted, the messages are marked `answered`, the in-memory
   waiter is delivered, `resume` is recorded as `published`, and agent A's blocked tool call returns
   the answers **in the same turn**. The chat overlay closes for anyone watching, via the existing
   `session.message.updated` broadcast.

2. **Human and external agent answer simultaneously.** The human clicks Submit while the external
   client posts a different answer. One insert wins. The winner's answers reach agent A and appear in
   the transcript. The loser gets `claimed: false` and the winner's payload; the losing tab renders
   the winner's answers, not its own (W3); no second turn starts, no transcript overwrite. Today the
   loser's answer can overwrite the transcript and start a second turn.

3. **Answer after a backend restart.** A bundle is created, the backend restarts, and the in-memory
   entry is gone. An external client lists pending questions — the bundle is still there, because the
   list reads durable messages (L2). It answers. No waiter exists, so `clarification.answered` is
   published and the orchestrator resumes the agent with a new turn. A second answerer gets R2 rather
   than a second resume (today both would fall through to the fallback).

4. **Foreign bundle.** User B holds a PAT and learns a `pending_id` belonging to user A's workspace.
   `answer_question_kandev` returns not-found, identical to a fabricated ID. `list_pending_questions_kandev`
   never showed it. `GET /:id` and `GET /:id/wait` both return 404 rather than their usual 404/504
   split, because A7 puts the authorization check ahead of the in-memory read.

5. **Auth disabled.** Identity is synthetic and every caller is unscoped, so nothing is ever denied:
   every bundle remains listable and answerable exactly as before. Nine behaviors do change, and they
   change here identically to how they change under enforced auth (A4): a duplicate answer is now an
   idempotent 200 with `claimed: false` rather than a 409; every success response gains `claimed`,
   `status`, `response`, and `resume`; cancelling a restart-stranded bundle now succeeds instead of
   404ing; an unknown `pending_id` now returns 404 instead of a misleading 200; submissions with
   fabricated option IDs, with `rejected` plus answers, or with over-2000-character text are now
   rejected; a bundle whose per-question updates cannot all be applied now returns 500 instead of a
   misleading 200 and stops at the first failure rather than continuing (R5a); and an answers
   submission against a bundle carrying no `question_id` is now
   rejected instead of accepted by the permissive validation branch; *rejecting* such a bundle now
   actually marks its messages `rejected` instead of silently skipping every one of them (R9a); and
   `GET /:id/wait` on a `pending_id` with no durable messages now returns 404 instead of 504 (A5a) —
   the one change on this list whose pre-change value is not 200. There is one code path, not an
   auth-disabled variant of one.

## Out of scope

Each exclusion below is a decision, not an omission.

- **`wait_for_question_kandev` / long-poll / push.** Deliberately dropped. It recreates the long-held
  MCP connection that `ask_user_question_kandev` already papers over with progress pings — justified
  for an agent blocked on its own question, wrong for a discovery API. Callers poll the list tool with
  a cursor. Notification is reconsidered only alongside a real subscription contract with reconnect
  and missed-event recovery; progress pings are not that.
- **A grand total in the list response.** L11a. `next_cursor` answers "is there more".
- **Extending the two tools to the in-session task surface.** Requires its own threat model (S2).
- **The clarification popup's own UX** — Escape committing a skip rather than dismissing, shortcuts
  focus-scoped to the overlay, submit gated on the whole bundle. Separate card on the Fix/Chore board.
  W4's `maxLength` is in scope only because N8b makes an over-long answer newly rejectable.
- **The Office inbox workspace leak.** `DashboardService.inboxPermissionItems`
  (`internal/office/dashboard/service_inbox.go:431`) calls `Store.ListPendingPermissions()` with no
  workspace argument while every sibling call in the same function takes `wsID` (`:35-58`). **Verified
  during this spec's input inventory**, not merely suspected: pending clarifications from every
  workspace appear in every workspace's inbox. `ListPendingPermissions` is also restart-lossy and
  option-less. It is untouched here, and this spec's tools deliberately do not reuse it. It needs its
  own card.
- **Wiring `Canceller.ExpireSessionAndNotify`.** D3 recognizes the `expired` status it writes, so this
  spec is correct whether or not that method is ever wired up, but wiring it into terminal teardown is
  a separate change with its own lifecycle questions.
- **Repairing a bundle whose claim landed but whose message updates did not.** A reconciliation sweep
  is a separate concern with its own ordering and idempotency questions. This spec makes that state
  *non-reanswerable* (R2), *non-resumable* (R5 publishes nothing), *non-listed* (D4a conjunct 1) and
  *non-blocking* (G1), which is the safe set: the transcript is stale, but no agent is double-resumed,
  no answer is silently overwritten, and no session is wedged.
- **Retrying a failed resume publication.** Surfaced to the caller (`resume: "failed"`) but not
  retried. The orchestrator owns resume retries.
- **Making the 15s clarification watchdog outcome-aware.** Its fallback resume can fire for a
  *rejection* delivered to a live waiter, which is in tension with R9. Pre-existing, unchanged by
  this spec, accepted with its reasoning recorded in *Failure modes*. Changing it means changing when
  the orchestrator resumes an agent, which needs its own card and its own lifecycle analysis.
- **Changing `ask_user_question_kandev`'s own shape, validation, or response envelope.** Unchanged.
- **A `pending_id`-aware WS gateway backstop entry.** `Client.authorizeAction`
  (`internal/gateway/websocket/dispatch_scope.go`) keys off `task_id` / `session_id` / `id`. The new
  actions are reachable only through the external MCP dispatcher, which does not go through the
  gateway client, and they authorize at the service layer. If these actions are ever exposed over the
  browser WS, the backstop must learn `pending_id` first.

## Verification notes

**E2E decision.** This touches one user-visible surface: the clarification overlay in chat, via
W1–W4a.
`apps/web/e2e/tests/chat/clarification.spec.ts` already covers the overlay end to end and already
intercepts `**/api/v1/clarification/*/respond` (`:803`). Extend it with a duplicate-submit case
asserting the overlay closes on a `claimed: false` success **and** that the rendered answer is the
winner's, not the loser's (W3); do not add a new spec file. The two MCP tools have no browser surface
and need no E2E — they are covered by Go tests at the MCP server and handler layers.

**Assertions that must exist.**
- Concurrency (R3) needs a real two-goroutine test against a shared database, not a mocked store — the
  claim is a database constraint and mocking it proves nothing.
- The claim (M8) is dialect-sensitive, so R3 and R1/R2 need an environment-gated PostgreSQL run
  alongside SQLite, following the existing `*_postgres_test.go` pattern in
  `internal/task/repository/sqlite` (gated on `KANDEV_TEST_POSTGRES_DSN`). Schema replay alone is
  insufficient, per `apps/backend/AGENTS.md`.
- Restart behavior (R6, L2) needs a test that clears the in-memory store and re-runs the resolution.
- The authorization tests (A1–A5a, A7, A8) need both the enabled and disabled auth modes, since A4 is
  the compatibility guarantee. The auth-disabled run asserts all **nine** A4 carve-outs are present,
  not absent — including item 6's 500 on partial application **and its stop-at-first-failure prefix**
  (R5a), item 7's rejection of an answers submission against a bundle with no `question_id`,
  item 8's marking of that same bundle's messages on a *rejection* (R9a), which today's skip leaves
  untouched, and item 9's **404 rather than 504** from `GET /:id/wait` on a `pending_id` with no
  durable messages (A5a).
- A5a additionally needs an auth-**enabled** test asserting that `GET /:id/wait` returns the **same**
  status for a foreign bundle and for a fabricated `pending_id`. Asserting only the 404 on the
  fabricated id would pass even if the foreign case returned something else, which is exactly the
  oracle A5a exists to close.
- G5 needs a test over a `clarification_request` message whose `metadata.status` key is **absent**,
  and a second whose value is unrecognized, asserting `turnCompleteBlockedByUserInput` **does** defer
  for that session — and a companion asserting it does **not** defer once a resolution row exists for
  the same bundle, so the widened predicate and the conjunct-1 exclusion are proven independently.
  Run both on SQLite and, per the dialect note above, on PostgreSQL: the widening turns on `NULL`
  handling in a JSON extraction, which is the most dialect-sensitive expression this spec adds.
- M8a needs a test that deletes the bundle's `task_sessions` row after step 1 has read the messages
  and before the claim, asserting **404** and that no `clarification_resolutions` row exists
  afterwards. Asserting only "an error" would pass against a raw 500, which is the outcome M8a
  rejects.
- M6a needs a test asserting the stored `response` and the response envelope contain the **keys**
  `answers`, `rejected` and `reject_reason` on every outcome — on a rejection `answers` is `[]` and
  present, on an answer `rejected` is `false` and present. Assert key presence explicitly
  (`json.RawMessage` / map lookup), not just the unmarshalled Go value: unmarshalling an absent key
  yields the same zero value as an emitted empty one, so a test that only reads the struct back
  passes against the exact bug M6a exists to prevent.
- M6's `answered` + stray `reject_reason` rule needs a test submitting `rejected: false`, a full
  `answers` array and a non-empty `reason`, asserting a 200 whose stored `reject_reason` is `""` —
  paired with the N3a byte-identity test, which must place that submission and the same submission
  without the stray reason in the same equivalence class.
- X3a needs a test where a bundle is claimed, its per-question application fails partway (R5a), and a
  **losing** cancel then arrives: assert the blocked waiter is released, and assert the cancel's
  response still carries the winner's `status`/`response`/`resume` with `claimed: false` and that no
  event was published and no message was modified by it.
- R5a needs a test on a multi-question bundle where a middle per-question update fails, asserting that
  every question **after** the failure point is left untouched — asserting only that the bundle
  errored would pass equally under today's continue-loop and prove nothing.
- D4a needs a test over a bundle whose messages are **all terminal and have no resolution row** (the
  pre-upgrade legacy state), asserting it is absent from `list_pending_questions_kandev` AND that
  `turnCompleteBlockedByUserInput` does not defer for its session. This is the single most important
  regression test in this spec: without conjunct 2 both assertions fail on every existing install.
- G1/G2 need a test that claims a bundle, leaves its messages `pending`, and asserts
  `turnCompleteBlockedByUserInput` no longer defers.
- L1a needs a test with more matching bundles than `limit` across two workspaces, asserting the page
  is full rather than short.
- N3a needs a test that two differently-ordered but semantically identical submissions produce
  byte-identical **answer payloads** — the `answers`, `rejected` and `reject_reason` fields of the
  stored `response`. The assertion SHALL exclude `pending_id` and `responded_at`, and SHALL NOT be
  made to pass by freezing the clock: per N3a those two fields are server-set and deliberately outside
  the guarantee, so comparing the whole serialized `Response` is a test that cannot pass.

**Tests that need deliberate updating.**
- `internal/mcp/server/server_test.go:1008-1018` — `TestServerModeExternal_ToolCount` moves 35 → 37.
- `internal/mcp/server/external_integration_test.go:65` — its `NotContains "ask_user_question_kandev"`
  assertion stays **true and unchanged** (S3); add positive assertions for the two new tool names.
- `apps/web/lib/settings/external-mcp-tools.test.ts` — the pinned count moves 30 → 37 and the stale
  drift note is deleted (C2).
- Any test asserting a **409** from `POST /:id/respond` on duplicate submit moves to 200 with
  `claimed: false` (R11).
- `internal/mcp/handlers/handler_inventory_test.go` measures the dispatcher delta dynamically and
  needs **no** change when handlers are added.
