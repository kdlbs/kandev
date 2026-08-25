---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001
created: 2026-08-16
owners:
  - nova28
---
# External Question Answering — authorized, discoverable, idempotent clarification resolution System Design Part 4

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
