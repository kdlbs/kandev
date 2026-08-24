---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001
created: 2026-08-16
owners:
  - nova28
---
# External Question Answering — authorized, discoverable, idempotent clarification resolution System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
