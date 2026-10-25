---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001
created: 2026-08-16
owners:
  - nova28
---
# External Question Answering — authorized, discoverable, idempotent clarification resolution System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
