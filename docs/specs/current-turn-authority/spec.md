# Current-turn authority: a lifecycle turn must never shadow the real turn

**Status:** spec
**Area:** backend (task repository, task service, agent lifecycle), frontend (chat overlay), protocol
**Slug:** `current-turn-authority`
**Amends:** [ADR-2026-08-14 Current Turn Owns Active Clarification](../../decisions/2026-08-14-current-turn-clarification-ownership.md)

---

## Why

A pending `ask_user_question_kandev` clarification became invisible to every surface that exists to
show it: `list_pending_questions_kandev` omitted it, the task-list badge cleared, the chat overlay
stopped rendering it, and the Stall Session Watchdog could not tell the session from one with no
open turn. Task `4be500f5` sat on an unanswered architectural question for 70+ minutes, found only
because a human read the raw transcript. The cause is not a race and not data corruption, but a
**stated invariant enforced in exactly one of eleven places**.

`Service.createCompletedTurn` (`apps/backend/internal/task/service/service_turns.go:274`) documents
its own contract:

> persists a synthetic turn that is **never observable as active**. It is used for lifecycle
> messages that must belong to a turn without making an idle task appear to have an active agent
> turn.

That turn is created on every agent **resume** launch: `bootMsgAdapter.CreateMessage`
(`apps/backend/internal/backendapp/worktree.go:57`) passes `CompletedTurn: isResuming` for the
`agent_boot` `script_execution` message. It is a real `task_session_turns` row with
`started_at = completed_at = created_at = now`, ordinary metadata, and one message.

Only `GetActiveTurnBySessionID` honours the invariant, by filtering `completed_at IS NULL`. Every
other consumer resolves "the session's current turn" as

```sql
ORDER BY turn_row.started_at DESC, turn_row.created_at DESC, turn_row.id DESC LIMIT 1
```

gated by `turnAuthorityPredicate`. The synthetic turn passes that predicate on its **first**
disjunct alone (`NOT (prompt_dispatch_pending)` is true; it never needs the message-EXISTS
fallback), and its `started_at` is later. So it wins, and every clarification owned by the real,
still-open turn is reclassified as superseded history.

### Evidence

In session `adddd3fc` the real turn `6d91a053` opened at 01:40:08.937 and the resuming boot script
wrote a synthetic turn 0.8s later, which became "current". The `clarification_request` raised on
`6d91a053` at 02:44 stayed invisible on every surface until 04:08, when that turn completed and the
question became legitimately superseded history. It repeated minutes later: the ordinary resume path,
not a one-off.

Across 6,133 production turns, 775 have `completed_at = started_at` — but **158 carry more than one
message** (real turns buried by `AbandonTurn`) and 27 hold `clarification_request` rows, which is
why D3 forbids inferring "synthetic" from that shape. Shadowing has occurred 207 times across 116
sessions, leaving 7 pending clarifications across 5 sessions unreachable today.

---

## Prior art

### Our own prior reasoning

**The in-repo prior reasoning already answered the design fork.**
[ADR-2026-08-14](../../decisions/2026-08-14-current-turn-clarification-ownership.md) (accepted,
amended 2026-08-15) decided:

> A pending clarification is operational only when its message belongs to the session's **newest
> durable `task_session_turns` record**.

It further requires that **one repository derivation** supply the clarification guard, detach/expiry
fallback, response validation, per-session pending projection and task-summary reconciliation, and
that both dialects select the same turn under dialect-sensitive tests (D9). It explicitly
**rejected** deriving ownership from the latest surviving message, since deleting a newer turn's
last message would roll the boundary backward and reactivate older pending history. This spec does
not reopen that: turn identity stays the boundary.

**Where this spec departs.** "Newest durable record" assumes every durable turn represents
conversational work. `createCompletedTurn` predates the ADR and creates durable records representing
no work at all, whose own doc comment says they must never be treated as active. The two intents
agree; only the word "durable" fails to separate them. This spec narrows it to "newest durable
**conversational** record" (R1) and adds one ordering rule (R2) — which **requires amending the
ADR** (AC-12), not a silent divergence.

Its "one repository derivation" requirement is also only **partly** honoured today: the ten call
sites share `turnAuthorityPredicate` but hand-write their own ordering, and the frontend hand-writes
a third copy. AC-3 and R3 close that.

Also read and honoured, both deferring to the same upstream rule rather than restating it:
`external-question-answering/spec.md` (D4, D4a, L2a) and
`tasks/system-design/clarification-active-lifecycle.md`, both under `docs/specs/`.

### What other products shipped

Queried the `saas-kb` corpus for agent-question surfacing and session-resume semantics: every hit
(Warp, OpenHands) was end-user documentation that the feature exists, none documenting turn identity
or resume semantics — the layer this defect lives in. Nothing there makes a *durable, queryable*
"which question is still answerable" the contract, as Kandev already does.

---

## Terminology

- **Turn record** — a row in `task_session_turns`.
- **Lifecycle turn** — a turn created solely to give a lifecycle message a parent, by
  `Service.createCompletedTurn`. It represents no agent work and is born already completed.
- **Conversational turn** — any turn record that is not a lifecycle turn.
- **Current turn** — the single conversational turn owning a session's active state, as resolved by
  R1 + R2 below. May be absent.
- **Authority-eligible** — a turn record that passes `turnAuthorityPredicate` (the existing
  reservation filter) **and** is conversational.
- **Shadowing** — a turn that is not the intended current turn being resolved as the current turn.

---

## Data model

**No schema change. No migration. No backfill.**

One new turn-metadata key, written at insert time:

| key | value | written by |
| --- | --- | --- |
| `lifecycle_only` | JSON `true` | `Service.createCompletedTurn`, on every turn it creates |

Declared as `models.TurnMetaKeyLifecycleOnly = "lifecycle_only"` beside the existing `TurnMetaKey*`
constants in `internal/task/models/models.go`, and read with `turnMetadataFlagPredicate` so both
dialects normalise it identically (D12), exactly as `prompt_dispatch_pending` already does.
`models.Turn` already serialises `metadata` and the TypeScript `Turn` type already declares
`metadata?: Record<string, unknown>`, so the marker reaches the frontend with no DTO change.

---

## Determinism and boundary rules

- **D1. The current-turn resolution is a total order over authority-eligible turn records**, keyed
  in this exact sequence, all named columns:
  1. `CASE WHEN completed_at IS NULL THEN 0 ELSE 1 END` **ascending** (open before completed)
  2. `started_at` **descending**
  3. `created_at` **descending**
  4. `id` **descending**

  Key 4 is a primary key, so the order is total and no tie is broken by chance. Keys 2–4 are already
  in the code; key 1 is the only addition, and it is what stops a zero-duration completed turn
  outranking a genuinely open turn.

- **D2. Key 1 is safe only under the single-open-turn invariant**, namely: a session has at most one
  turn with `completed_at IS NULL`, and no open turn survives a session resume. `AbandonOpenTurns`
  already enforces the second half on the resume path, and the first half holds across all 6,133
  production turn records (zero sessions with more than one open turn). Key 1 changes
  the resolved turn **only** when the newest record is completed while an older record is still
  open — which under this invariant can only be the shadowing defect itself. It is neither assumed
  silently nor claimed as a guarantee: AC-8 requires the enforced half tested and the observed half
  to have a defined, tested resolution when violated.

- **D3. "Lifecycle" is a durable marker, never an inference from `completed_at = started_at`.**
  That equality is produced by two unrelated, both-legitimate paths: `createCompletedTurn` (a
  lifecycle turn) and `Repository.AbandonTurn` (a *real* turn buried after a crash, so its dead hours
  do not poison analytics). They are indistinguishable by shape: 158 zero-duration turns carry
  multi-message real conversations and 27 carry `clarification_request` rows, which any rule keyed on
  zero duration would silence permanently. A hard prohibition, not a preference.

- **D4. An absent `lifecycle_only` key means conversational.** Every turn record written before this
  change is therefore conversational, including the 775 existing zero-duration rows. Deliberate; see
  *Known residual*.

- **D5. A lifecycle turn never holds a `clarification_request`.** `createCompletedTurn` is reachable
  only from the boot-message path; clarification rows are written through `getOrStartTurn`. AC-7
  asserts this so R1's exclusion can never hide a question.

- **D6. Resolution is a pure function of committed rows**, taking no new lock and adding no write.
  Two concurrent readers of the same committed state resolve the same turn. A reader concurrent with
  an insert resolves against whichever snapshot its transaction sees; because lifecycle turns never
  compete for the current-turn slot, the resolved turn is the same whether or not a concurrent
  `createCompletedTurn` has committed. That independence is the point of the fix.

- **D7. Marking is idempotent by construction.** `createCompletedTurn` writes the marker in the same
  `INSERT` as the row. A retried boot-message creation produces another marked lifecycle turn; N
  marked lifecycle turns are as excluded as one, so retry needs no dedupe.

- **D8. Absent current turn is a legitimate result, not an error.** A session with zero
  authority-eligible turns resolves to no current turn. Every consumer already handles a `LIMIT 1`
  subquery returning no row (the `turn_id = (…)` join matches nothing), unchanged. A session whose
  only records are lifecycle turns now resolves to no current turn where it previously resolved to
  the lifecycle turn — correct, since it has had no conversational turn and holds no active state.

- **D9. SQLite and PostgreSQL must resolve identically**, including the `CASE WHEN completed_at IS
  NULL` key and the metadata-flag normalisation. Restated from the ADR because key 1 is new and
  untested in both dialects.

- **D10. "Current turn" and "active turn" are different questions and must not be merged.**
  `GetActiveTurnBySessionID` answers *"is a turn running right now"* and filters
  `completed_at IS NULL`; the other nine sites answer *"which turn owns this session's state"* and
  do not. Adopting AC-3's shared helper SHALL NOT remove that filter, or the function begins
  returning completed turns and `AbandonOpenTurns` burns its 16-iteration budget re-burying them.
  Because a lifecycle turn is always born completed, R1's exclusion is a **no-op** here, applied for
  uniformity, not effect.

- **D11. Both fragments must compose into the four SQL shapes already in the code**, changing
  nothing else at any site: the scalar subquery (`clarificationBundleQuery` plus the six in
  `message_clarification_response.go`), the `current_turn` CTE
  (`FindActiveClarificationMessagesBySessionID`), the `ROW_NUMBER() OVER (PARTITION BY
  task_session_id ORDER BY …)` window (`pendingActionsBySessionQuery`), and the top-level `ORDER BY
  … LIMIT 1` (`GetActiveTurnBySessionID`, which keeps its own `completed_at IS NULL` per D10).
  `predicate` joins each site's existing `WHERE`; `orderBy` replaces each site's ordering. Hence
  `orderBy` is a bare expression list, not a canned subquery, which would fit only the first shape.
  The window site resolves many sessions in one statement; a helper forcing it back to N scalar
  subqueries is a performance regression on the task list and SHALL NOT be accepted.
  Qualifying `orderBy` with `turnAlias` is safe at the window site, whose current text is
  unqualified, because the alias is in scope.

- **D12. A NULL or empty `metadata` column normalises to conversational.** The column is
  `TEXT DEFAULT '{}'` and `turnMetadataFlagPredicate` already wraps its extraction in
  `COALESCE(…, '')` before the `IN ('true','1')` test, so `NULL`, `''`, `'{}'` and malformed JSON
  all yield "not a lifecycle turn" — the same result D4 requires for an absent key, via the same
  existing helper. No new normalisation is introduced and none SHALL be added.

- **D13. Clock skew and future timestamps change nothing.** D1 keys 2 and 3 are compared as stored,
  unclamped: a turn whose `started_at` is in the future sorts first within its open/completed class,
  exactly as today, and key 4 keeps the order total regardless. This spec deliberately adds no clock
  validation — the defect is not a timestamp defect, and `started_at` ordering is already
  load-bearing in `turnHistoryPredicate`, the turns endpoint, and the frontend comparison.

- **D14. Bundle claim, winner/loser, and conflict semantics are unchanged.** This spec changes only
  *which turn* the claim predicate in `claimActiveClarificationBundle` compares against. Exactly one
  caller still claims a bundle, a loser still receives conflict with no write and no resume, and the
  restore-on-failed-delivery path is untouched. Those rules stay owned by
  `docs/specs/external-question-answering/spec.md` and
  [ADR-2026-08-14](../../decisions/2026-08-14-current-turn-clarification-ownership.md), named here
  so their absence below reads as deferral, not silence.

- **D15. The frontend rule is a deliberate subset, sound only via the history pre-filter.** The
  backend resolves `turnAuthorityPredicate AND NOT lifecycle`; `newestDurableTurnId` implements D1's
  ordering and D4's marker and **no part of** the authority predicate. That holds because every turn
  it sees arrives from the turns endpoint, already gated by `turnHistoryPredicate`. The predicates
  differ: authority also admits a dispatch-pending, dispatch-attempted, message-less turn, which
  history excludes, so such a turn can be current on the backend and never reach the frontend. That
  gap is accepted, not accidental — such a turn has no message and so no clarification to hide — and
  it is why AC-10's fixture is confined to authority-eligible turns: a fixture fed straight to both
  implementations bypasses the pre-filter the frontend relies on, and would assert an agreement
  neither implementation claims.

---

## What

Each criterion is observable through the database, HTTP API, MCP tool surface, or web UI.

### R1 — a lifecycle turn is never current-turn authority

- **AC-1.** When `Service.createCompletedTurn` persists a turn, the system SHALL write
  `metadata.lifecycle_only = true` in the same insert as the turn row.

- **AC-2.** The system SHALL exclude every turn whose `lifecycle_only` flag normalises to true from
  current-turn resolution, at **every** site resolving a session's current turn. Those sites are
  exactly:

  | file | site |
  | --- | --- |
  | `task/repository/sqlite/session.go` | `GetActiveTurnBySessionID` |
  | `task/repository/sqlite/message.go` | `FindActiveClarificationMessagesBySessionID`; `pendingActionsBySessionQuery` |
  | `task/repository/sqlite/clarification_bundle_query.go` | `clarificationBundleQuery` |
  | `task/repository/sqlite/message_clarification_response.go` | `DetachActiveClarificationMessagesBySessionID`; `ExpireActiveClarificationBundle`; `loadRestorableClarificationBundle`; `restoreClarificationMessages`; `claimActiveClarificationBundle`; `loadClaimedClarificationBundle` |

  A site added later that resolves a current turn without the shared builder is a defect against
  AC-3, not a gap in this list.

- **AC-3.** The system SHALL express D1's ordering and R1's exclusion in **one** named Go helper in
  `internal/task/repository/sqlite/turn_authority.go`:

  ```go
  func currentTurnAuthority(driverName, turnAlias string) (predicate, orderBy string)
  ```

  Two fragments: the rule occupies two SQL positions and no one string sits in both.
  `predicate` is `turnAuthorityPredicate` AND R1's exclusion (`NOT` the `lifecycle_only` flag, via
  `turnMetadataFlagPredicate`) as one parenthesised conjunct, for `WHERE … AND %s`. `orderBy` is
  D1's four keys as a bare expression list qualified by `turnAlias`, with no `ORDER BY` keyword and
  no `LIMIT`, so it drops unchanged into both `ORDER BY %s` and `ROW_NUMBER() OVER (… ORDER BY %s)`.
  Every AC-2 site SHALL take both from this one call and hand-write neither.
  `turnAuthorityPredicate` is today called at exactly those ten sites, so subsuming it affects no
  other consumer. This is the concrete form of the ADR's "one repository derivation" requirement.

- **AC-4.** While a session's real turn is open and a lifecycle turn exists with a later
  `started_at`, the system SHALL resolve the current turn to the real turn, asserted by **a Go
  integration test over the service and repository layers**; all three consumers below are Go-
  reachable, so no browser-level test is required. Reproducing the sequence in *Why*: after the
  lifecycle turn is written, `list_pending_questions_kandev` SHALL return the bundle (through the
  MCP handler in `internal/mcp/server/question_handlers.go`), the session's `pending_action` SHALL
  be `clarification` (through `GetPendingActionsBySessionIDs`), and `POST
  /api/v1/clarification/:pendingId/respond` SHALL succeed rather than return conflict (through
  `internal/clarification`).

  The lifecycle turn SHALL come from the real write path — `Service.CreateMessage` with
  `CompletedTurn: true` — never an inserted row imitating it, so the test fails if AC-1's marker is
  dropped. Ordering SHALL be made deterministic by seeding the conversational turn with an explicit
  **past** `started_at` (the existing `seedBundleTurnAt` idiom), since `createCompletedTurn`
  stamps `time.Now().UTC()` and is therefore strictly later on every run. Starting both turns at
  "now" and relying on them to differ SHALL NOT be accepted: on a coarse clock they tie on
  `started_at` and the assertion turns on `id`, which is unrelated to the behaviour tested.

- **AC-7.** On the boot-on-resume path, every `clarification_request` written by the open
  conversational turn SHALL keep **that** turn's `turn_id`, asserted as a repository-level test over
  a fixture exercising the path. The broader claim — that no `clarification_request` anywhere can
  carry a lifecycle `turn_id` — holds only because `CreateMessageRequest.CompletedTurn` is tagged
  `json:"-"` (no API client can set it) and has one production caller, which writes a
  `script_execution` message. Neither property is type-enforced, so AC-7 SHALL also assert the
  first: unmarshalling a body that sets `completed_turn` SHALL leave the field false. A caller later
  setting it while writing a `clarification_request` voids the claim and is a defect here.

### R2 — an open turn outranks a completed one

- **AC-5.** The system SHALL order current-turn candidates by D1's four keys in that order, so a
  turn with `completed_at IS NULL` is preferred over any completed turn regardless of `started_at`,
  and ties within each class fall through to `started_at`, `created_at`, `id` descending.

- **AC-6.** If a session has an open conversational turn and an unmarked legacy zero-duration turn
  with a later `started_at`, then the system SHALL resolve the current turn to the open turn. This
  rescues the pre-existing 775 unmarked rows in the only case where invisibility is actively
  harmful; see *Known residual* for the case it does not rescue.

- **AC-8.** The system SHALL assert the two halves of D2 separately, because only one is a
  guarantee. That `AbandonOpenTurns` leaves no turn with `completed_at IS NULL` is enforced and
  SHALL be tested directly. "At most one open turn per session" is **observed**, not enforced — no
  unique or partial index constrains it — so the system SHALL instead assert the defined behaviour
  when it does not hold: given two open turns on one session, resolution SHALL pick the one winning
  D1 keys 2 through 4, deterministically and without error. A test asserting the invariant itself
  could only check the fixture it just built, and would pass while production drifted.

### R3 — the frontend consumes the same rule

- **AC-9.** `newestDurableTurnId` in `apps/web/lib/utils/pending-clarification.ts` SHALL apply the
  same two rules — skip lifecycle turns, prefer a turn with no `completed_at` — before its existing
  `started_at` / `created_at` / `id` comparison, reading the marker from the turn payload rather
  than inferring anything from timestamps.

  It SHALL treat a turn as lifecycle **only** when `metadata.lifecycle_only` is one of exactly four
  values: boolean `true`, number `1`, string `"true"`, string `"1"`. Everything else — absent key,
  `null`, `undefined`, `false`, `0`, `""`, `"false"`, `"0"`, `"yes"`, any object or array — is
  conversational. A bare truthiness test SHALL NOT be used, nor a `String(value)` coercion:
  truthiness misreads `"false"`, `"0"`, `"yes"`, `{}` and `[]` as lifecycle, coercion misreads
  `["1"]`, and D12's backend predicate (`… IN ('true','1')`) treats none of them as lifecycle. A
  divergence hides a clarification the backend is serving — the symptom this spec exists to
  remove. AC-10's fixture SHALL pin these values.

  Its open-turn check SHALL be `turn.completed_at == null`, treating `null` and `undefined` alike:
  the payload type declares `completed_at?: string`, so an open turn arrives as an absent key while
  AC-10's fixture writes `null`. When every turn is a lifecycle turn it SHALL return `null`, not
  `undefined` — `null` means history loaded with no durable turn and hides all messages, whereas
  `undefined` means history not loaded and falls back to `pendingAction`, reopening the shadowing
  this spec removes.

- **AC-10.** The Go and TypeScript resolutions SHALL be validated against **one shared fixture
  file**, consumed by both a Go repository test and a TypeScript unit test. Two implementations of
  one rule are tolerable only while a single artifact proves they agree, so it is specified here.

  Path: `apps/backend/internal/task/repository/sqlite/testdata/current_turn_resolution.json`, under
  the Go package because `go:embed` cannot reach outside its tree while the TypeScript test can
  reach it by relative path. Format: a JSON object with a `cases` array, each case
  `{ name, turns, expected_current_turn_id }` and each turn
  `{ id, started_at, created_at, completed_at, metadata }`, timestamps RFC 3339 UTC and
  `completed_at: null` when open. `expected_current_turn_id: null` means **no** current turn
  resolves; the fixture SHALL carry at least one such case (D8, Scenario 5).

  Go SHALL read it with `os.ReadFile("testdata/current_turn_resolution.json")` — under `go test` the
  working directory is the package directory — then seed each case and run the real query.
  TypeScript SHALL resolve the path from the test file via `import.meta.url`, **not** the process
  working directory, which under Vitest is `apps/web`. If the file is missing, unreadable or
  unparseable both tests SHALL fail and neither may skip, because a fixture that silently stops
  being read is indistinguishable from two implementations that agree. Both SHALL run every
  case; either may add its own but SHALL NOT filter the shared ones. The cases SHALL cover every
  value in AC-9's list: the four resolving as lifecycle, and ten conversational — AC-9 names eleven,
  but an absent key and `undefined` are indistinguishable in JSON and SHALL be written once, as the
  absent key.

  The fixture describes turns of **one** session and carries no `task_session_id` or `task_id`; both
  are `NOT NULL` under a foreign key, so the Go test SHALL supply its own session and task and stamp
  every turn with them. No fixture turn SHALL set a `prompt_dispatch_*` key, so every fixture turn is
  authority-eligible by construction and the fixture exercises D1's ordering and D4's marker only —
  the scope D15 requires. The TypeScript loader SHALL map `completed_at: null` to an absent property
  on each `Turn` and `expected_current_turn_id: null` to `null`; the production payload type is not
  widened to admit `null`.

- **AC-11.** `GET /api/v1/task-sessions/:sessionId/turns` SHALL include `metadata.lifecycle_only` on
  a lifecycle turn in its response payload, so AC-9 has the input it needs. `turnHistoryPredicate`
  SHALL keep including lifecycle turns in visible history — the boot message stays grouped under its
  own turn in the transcript, unchanged.

### R4 — the decision record

- **AC-12.** The system SHALL amend
  `docs/decisions/2026-08-14-current-turn-clarification-ownership.md` so its stated rule reads
  "newest durable **conversational** turn record", names the `lifecycle_only` marker, and records
  D1's ordering including the open-turn key. It SHALL also correct that ADR's *Consequences*
  section, which restates the rule as "SQLite and PostgreSQL must select the same newest durable
  turn". The ADR is the only place the rule is written down as a decision, and leaving it saying
  "newest durable record" would reintroduce this defect the next time someone implements against it.

- **AC-13.** The system SHALL update
  `docs/specs/tasks/system-design/clarification-active-lifecycle.md`, whose *Data model* section
  states "The newest authoritative durable `task_session_turns` record for the session identifies the
  current turn", whose *API surface* section orders visible history "matching the reverse ordering
  used to select the current turn", and whose *Persistence guarantees* section calls clarification
  state reconstructable from "the newest authoritative durable turn" — all three inaccurate under D1.

---

## Failure modes

- **A lifecycle turn is written without the marker** (a future second producer): treated as
  conversational, can shadow again. AC-1 writes the marker inside `createCompletedTurn` rather than
  at its call sites, so every caller inherits it.
- **A new consumer hand-writes its own current-turn subquery**: silent divergence. AC-3's shared
  helper is the only thing that makes it visible in review.
- **Two open turns coexist**: D1 keys 2 to 4 choose between them, the same answer today's rule
  gives. AC-8 requires that resolution tested, not assumed away.
- **An unmarked legacy lifecycle turn is newest and the real turn is also completed**: it stays
  current and the older question stays invisible. Not fixed; see *Known residual*.
- **`metadata.lifecycle_only` present but outside `{true, 1, "true", "1"}`**: normalised to "not a
  lifecycle turn" by `turnMetadataFlagPredicate` (D12) and AC-9's enumeration. No new rule.

---

## Known residual

**The 775 existing turn records carry no `lifecycle_only` marker and this spec adds no backfill.**
D3 explains why no migration can safely identify them: their only shared observable shape,
`completed_at = started_at`, is also the shape of 158 real conversations and 27 turns holding
`clarification_request` rows a backfill would silence permanently — converting a bounded visibility
bug into unbounded data loss. Consequently:

- A legacy lifecycle turn **can no longer shadow an open turn** — R2/AC-6 covers that, the case
  where a live question is hidden from a human who could still answer it.
- A legacy lifecycle turn **still shadows a completed turn**. Concretely, 4 of the 7 currently
  unreachable pending clarifications (2026-08-10 to 2026-08-18) keep a legacy lifecycle turn as their
  resolved current turn while their own turn is closed. Under
  [ADR-2026-08-14](../../decisions/2026-08-14-current-turn-clarification-ownership.md) a question
  whose turn closed with no successor is superseded history anyway, so their observable outcome
  matches the intended contract even though the path to it is wrong. Named so their persistence is
  not mistaken for an incomplete fix.
- The residual may shrink as eligible sessions expire; AC-1 prevents any new unmarked lifecycle turn.

---

## Out of scope

Named exclusions are contracts. Silence would be a defect.

- **The Stall Session Watchdog automation** (`065b95a3-f69f-43dc-96e9-6fedbf6df021`). Its STEP 3
  hand-writes `ORDER BY started_at DESC LIMIT 1` with no authority predicate and no tiebreakers,
  then tests `completed_at IS NULL`, so a lifecycle turn makes a genuinely stalled session look like
  it has no open turn and the watchdog never nudges it. **A real second symptom of the same root
  cause**, but the prompt lives in the `automations` table, not version control, so its owner
  must update it — to D1's ordering, or at minimum "newest turn that is not `lifecycle_only`".
- **Task `35d108bb`'s turn-completion duplicate race.** That concerns
  `acquireTurnCompletionCriticalSection` double-dispatching workflow `on_enter` actions: duplicate
  **dispatches**, not duplicate turn rows. The extra row here is created deliberately, by name, on a
  documented path. **Unrelated**; `35d108bb` landing would change nothing here.
- **Backfilling or deleting the 775 existing unmarked lifecycle turns.** Prohibited by D3,
  quantified in *Known residual*.
- **Changing what `AbandonTurn` writes.** `completed_at = started_at` on a buried orphan is
  deliberate analytics behaviour, untouched.
- **Removing the lifecycle turn concept**, e.g. a nullable `turn_id` on the boot message or
  attaching it to the real turn. A larger change to the message/turn foreign key and to transcript
  grouping, not needed to close this defect.
- **Replacing the frontend's independent resolution with a server-supplied `current_turn_id` field.**
  A cleaner end state, but it changes the turns endpoint envelope. AC-9 + AC-10 close the drift risk
  at a fraction of the blast radius. A follow-up card, not this one.
- **The `turnHistoryPredicate` visible-history rule.** Unchanged by AC-11.
- **Anything about permissions, authorization, or the visibility predicate** in
  `clarificationBundleQuery`. Untouched.

---

## Scenarios

1. **Resume during an open turn, question asked afterwards.** Real turn opens; a marked lifecycle
   turn is written 0.8s later; the agent asks on the real turn. The question stays listed, badged
   and answerable throughout. (AC-1, AC-2, AC-4)
2. **Resume between turns.** Previous turn completes, a marked lifecycle turn is written, a new real
   turn opens 3s later; current turn is the new real turn, so the ordinary path is unchanged. (AC-5)
3. **Legacy unmarked lifecycle turn over an open turn.** No marker, `completed_at = started_at`,
   later than an open conversational turn; current turn is the open turn. (AC-6)
4. **Legacy unmarked lifecycle turn over a completed turn.** Same fixture, real turn closed; the
   legacy turn stays current and the older bundle stays superseded, asserting the residual so it
   cannot change silently. (*Known residual*)
5. **Session with only lifecycle turns.** No current turn resolves, nothing is listed, no consumer
   errors. (D8)
6. **Both dialects.** Scenarios 1-5 give identical resolved turn ids on SQLite and PostgreSQL. (D9)
7. **Frontend agreement.** AC-10's fixture yields the same resolved turn id from the Go helper and
   from `newestDurableTurnId`. (AC-9, AC-10)

---

## Verification notes

- The repository-level tests belong beside `clarification_bundle_query_test.go`, whose header
  comment documents the old `(started_at, created_at, id)` ordering and must be updated for key 1.
- AC-4's integration test proves the backend-owned surfaces recover (MCP listing, `pending_action`,
  the respond endpoint). The chat overlay is frontend-owned, proven by AC-9 and AC-10, not AC-4.
- `apps/backend/internal/orchestrator/clarification_guard.go` is the most severe consumer: a shadowed
  clarification makes `sessionHasPendingClarification` return false, letting a workflow step
  auto-advance past an unanswered question. Worth an explicit test.

### User-visible surfaces touched

The chat clarification overlay (`pending-clarification.ts`); the task-list pending badge via
`pending_action`; `POST /api/v1/clarification/:pendingId/respond` success vs. conflict; MCP
`list_pending_questions_kandev` and `answer_question_kandev`; workflow step auto-advance via the
clarification guard. No new UI copy, so no i18n catalogue work.
