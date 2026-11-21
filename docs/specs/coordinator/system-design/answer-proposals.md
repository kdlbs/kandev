---
id: coordinator-answer-proposals-design
title: Answer proposals design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-11-07
last_updated: 2026-11-07
requirements:
  - REQ-COORDINATOR-ANSWER-001
  - REQ-COORDINATOR-ANSWER-002
  - REQ-COORDINATOR-ANSWER-003
  - REQ-COORDINATOR-ANSWER-004
  - REQ-COORDINATOR-ANSWER-005
  - REQ-COORDINATOR-ANSWER-006
---

# Answer proposals System Design

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-ANSWER-001` | [Kind and spec](#kind-and-spec), [Propose](#propose), [Tool](#tool) |
| `REQ-COORDINATOR-ANSWER-002` | [Delivery](#delivery), [Failure mapping](#failure-mapping) |
| `REQ-COORDINATOR-ANSWER-003` | [Refusal](#refusal), [Staleness](#staleness) |
| `REQ-COORDINATOR-ANSWER-004` | [Policy class and flag](#policy-class-and-flag) |
| `REQ-COORDINATOR-ANSWER-005` | [Card](#card) |
| `REQ-COORDINATOR-ANSWER-006` | [Recording](#recording) |

## Context and boundaries

`answer` is a fifth non-`create_task` kind in the `KindExecutor` registry
(`kinds.go`, `registerKinds`), beside `resume`, `message`, `move` and
`improvement`. It reuses the proposal record, the claim and settle path in
`approve.go`/`approve_kinds.go`, the reply route (`reply.go`, one new
`replyTexts` entry using `taskKindReplyText`), the activity log and the Needs
you card. It consumes, unchanged: the relay's target reads, `clarification.Resolver.ResolveBundle`
and the orchestrator's `RespondToPermission` (the one `permission.respond` calls). Two additive exports are the only change outside coordinator, MCP, flag and web code: `clarification.ValidateOutcome(questions, outcome)` (wraps
`validateOutcome`) and `clarification.BundleQuestions(msgs)`
(wraps `bundleQuestions`, not `questionsFromMessages`, which drops options), each tested against the resolver.

The target reads in `relay.go` (`relayClarification`, `relayPermission`, the primary-session lookup) move into one unexported
`readTargets(ctx, taskID)` that `ReadRelay` and the new `AnswerTargets` seam both call. For a clarification it also fetches the bundle's task-model messages with `relayReader.FindMessagesByPendingIDs(ctx, []string{pendingID})`, the call `HydrateBundle` makes, and passes them to `clarification.BundleQuestions`; `ReadRelay` still renders through `HydrateBundle` and its response is unchanged. `ReadRelay` keeps its authorization, phase gate and response.

## Kind and spec

`ProposalKindAnswer = "answer"`, action `ActionAnswer = "answer"`. The stored
spec (`answerSpec`) is:

```json
{
  "task_id": "…",
  "target": "clarification | permission",
  "pending_id": "…",
  "request_id": "… (permission only)",
  "draft": {
    "answers": [{"question_id": "…", "option_ids": ["…"], "custom_text": "…"}],
    "option_id": "… (permission only)"
  },
  "snapshot": {
    "questions": [{"id": "…", "text": "…", "options": [{"id": "…", "label": "…"}]}],
    "permission": {"title": "…", "summary": "…", "options": [{"id": "…", "name": "…", "kind": "…"}]}
  },
  "sources": [{"ref": "…", "label": "…"}],
  "confidence": "low | med | high",
  "needs_decision": false,
  "rationale": "…"
}
```

`request_id` and `snapshot` are read from the live target at propose time,
never supplied by the caller; `snapshot` holds `questions` for a clarification
and `permission` for a permission, texts truncated to 2,000 runes. A snapshot question's `text` is the bundle question's `Prompt` (its `Title` when the prompt is empty) and it carries `allow_custom_text` (omitted when unset). The permission `title` is the permission message's `Content`; its `summary` is built server-side from the message metadata `action_type` and `action_details` (compact JSON), empty when neither exists. A
clarification draft uses `answers`; a permission draft uses `option_id`; the
other member, and any member not shown, is rejected as `unknown_field`. The
validator never reorders: a draft whose entries are out of bundle order, or
miss or repeat a question, is a field error. After normalization an entry with no option ids and no custom text is `FieldError{draft, "empty_answer"}`, part of stage 1's draft shape check (entry order), so it applies to edits too. Text is trimmed and option ids
de-duplicated (first occurrence kept) by one `normalizeDraft` function, called
before validation, storage and edit comparison.

The proposal's `TargetTaskID` is `task_id`; its open-target key is
`(coordinator, "answer", task_id)`, the existing unique index
`coordinator_proposals_open_target` (`pending`, `approving`, `failed`). That index, the 25-proposal cap (`openProposalStatuses`) and the
pending list are unchanged and define "open", so a refused proposal must leave that set (see [Refusal](#refusal)).

## Propose

`ProposeKind` is reused, with optional hooks on `answerKind` (small interfaces checked beside `startsAgentReporter`). Order in one call: stage 1 (shape), the `OpenMatcher` early return, stage 2 (target and draft content). A kind that implements `ownsArgs` (answer does) makes `ProposeKind` skip its own `proposeTaskID` and `checkOrderIDsShape` early errors: a wrong-typed or missing `task_id` is stage 1's `task_id` field error, in stage 1 order.

- `OpenMatcher.OpenMatches(open *Proposal, args json.RawMessage) bool`: today `ProposeKind` returns any open proposal before validation. For `answer` it returns after stage 1, and only when the open row's `target` and `pending_id` equal the call's (a permission's `request_id` is not compared: the caller cannot supply it, so a permission whose request changed under the same `pending_id` dedupes to the old proposal and settles `stale` at approval).   A stored spec that cannot be parsed is not a match. Every other call continues to stage 2.
- `SupersedeOnPropose.Supersede(ctx, tx, open *Proposal) (SupersedeVerdict, error)`: runs in the existing pre-insert callback, inside the coordinator write transaction, after the in-transaction `OpenTargetProposal` re-read.

Carrier: `ProposeKind` puts a `proposeScope` (a pointer) in the context it passes to `ValidatePropose` and the pre-insert callback. `ValidatePropose` records the `TargetSet` of its single read and the instant it began (`ReadStartedAt`); the callback reads both back. No second read happens.

Stage 1 (`ValidateShape`, called before the dedupe check) returns first the `unknown_field` error of the alphabetically first top-level member the tool does not define, then the first failing member's `FieldError` in the order `task_id`, `target`, `pending_id`, `draft`, `sources`, `confidence`, `needs_decision`, `rationale`, `standing_order_ids`. The wrapper decodes the arguments member by member (`map[string]json.RawMessage`), so a wrong JSON type is that member's field error; `standing_order_ids` is shape-checked with `checkOrderIDsShape`. Under `ownsArgs` the handler passes `nil` ids and stage 1 stores the validated ids in `proposeScope.OrderIDs`; after stage 1 `ProposeKind` sets its local `orderIDs` from `proposeScope.OrderIDs`, so its own `checkOrdersActive` and `MarkApplied` run on them and `buildKindProposal` stores them as `StandingOrderIDs`. Field names are the `kinds.go` constants (`fieldTaskID`) plus new `fieldTarget`, `fieldPendingID`, `fieldDraft`, `fieldSources`, `fieldConfidence`, `fieldNeedsDecision`.

Stage 2 (`ValidatePropose`), only when stage 1 passed:

1. the watched, same-workspace, not archived, not coordinator-origin task check shared with the other kinds (`KindTaskReader.GetTarget`);
2. one read of the task's targets through `AnswerTargets.CurrentTargets(ctx, workspaceID, taskID) (TargetSet, error)` (over `readTargets`), with a five-second deadline. `TargetSet` holds `Clarification` (`PendingID`, the ordered `[]clarification.Question`) and `Permission` (`PendingID`, `RequestID`, `SessionID`, title, summary, options with kinds), each nil when absent. A task with no primary session yields an empty set; a task that does not exist or is in another workspace is `ErrNotFound`; any other failure is a read error, and so is a bundle whose pending id returns no messages or whose messages yield zero questions;
3. the requested `target`'s member must exist with a `PendingID` equal to the call's, else `nothing_to_answer`; a read error is returned as such; `ErrNotFound` is the task-not-found refusal;
4. draft content. Clarification: `normalizeDraft`, the exact question-set and order check, then `clarification.ValidateOutcome`. Permission: `option_id` in the live options and of kind `allow_once` or `reject_once` (a missing or unknown kind is refused).

The pre-insert callback (same transaction as the insert, holding the coordinator write lock) first applies `OpenMatches` to the in-transaction open row: a hit returns verdict `DedupeOpen`, which `ProposeKind` returns as the open row with `deduplicated: true` (a racing equal call, AC-001.7). It decides any other open row using the `TargetSet` and `ReadStartedAt`: `approving` returns `approval_in_progress`; a row failed with `outcome_unknown` returns `answer_pending`; a `pending` or retryable-`failed` row created at or after `ReadStartedAt` returns `answer_pending` (a concurrent call); one created before it whose target is not current in the set calls `settleRefusedTx` with `stale` and lets the insert proceed; one whose target is still current returns `answer_pending`. The swap is atomic: `settleRefusedTx` removes the old row before the insert. Claim and reject write without the coordinator lock, so `settleRefusedTx` can match no row (an approval or reject committed first): the callback then re-reads the open row in the same transaction and decides again by this list (a closed or absent row lets the insert proceed), so the caller never sees a unique-index error (PostgreSQL test). The approve-time check is authoritative for a target that changes between the read and the insert.

`rationale` and `standing_order_ids` reuse the rationale limit and `checkOrderIDsShape`. `sources` are normalised by trimming; their order is kept. `needs_decision` accepts only JSON booleans.

### Typed errors

The three refusals are `*ProposeRefusal{Code, Field}` (AC-001.10); a target read failure is `ErrAnswerTargetRead`.
`proposeKindErrorResponse` (`mcp/handlers/coordinator_propose_kinds.go`) maps a
`*ProposeRefusal` to `ws.ErrorCodeValidation` (message `Code`, details `{code, field}` via `fieldErrorDetailsKey`), unlogged, and `ErrAnswerTargetRead` to `ws.ErrorCodeInternalError` (logged). A
`CurrentTargets` `ErrNotFound` after the watch check becomes the watch check's task-not-found `FieldError`, never `ErrNotFound` ("coordinator not found"). Each has a handler test.

## Tool

A nested `unknown_field` carries the member path as its `Field`: `draft.answers[<i>].<name>`, `draft.<name>`, `sources[<i>].<name>`. The handler response carries `details {code, field}`; the answer tool wrapper (the MCP result path drops details) renders every validation error as `<message> (field: <field>)`, tested there.

`propose_answer_kandev` is registered in `internal/mcp/server/coordinator_tools.go`
beside `propose_task_kandev`. The existing `proposeKindHandler` forwards string arguments only, so the answer tool gets a thin wrapper that forwards the raw arguments object, JSON types preserved. The
dispatch-side handler (`coordinator_propose_kinds.go`) does not
decode `standing_order_ids` for `answer`: it passes the
unmodified payload and `nil` ids (see `ownsArgs`), so a non-array value is the member's field error, never "Invalid JSON payload".
Registrations, each with a test that fails when missing:

- `ActionProposeAnswer = "coordinator.propose_answer"` in `mcpcontract/contract.go`;
- the principal-only and surface sets `coordinatorPrincipalOnlyActions` and
  `coordinatorSurfaceActions` (`internal/mcp/handlers/coordinator_authorization.go`),
  which refuse a non-coordinator principal (AC-001.8);
- the tool-name allowlist in `internal/mcp/profile/coordinator_policy.go` and
  `RegisterFunc` in `internal/mcp/handlers/handlers.go`;
- `toolprofile.go`: `proposeTool[ActionAnswer] = "propose_answer_kandev"`,
  `actionTools`, and `ActionAnswer` appended to `proposeOrder`; `ToolNamesPhase31`
  includes the tool when `p.Allows(ActionAnswer)` on the effective policy;
- `knownProposalKind` (`decision_phase2.go`; without it reject answers 500), the
  `KindExecutors` order (`kinds.go`), and `kindAction` (`approve.go`), whose
  default `ActionCreateTask` would re-check the wrong class;
- the `no_merge_surface_test` name scan still passes (no new name has `merge`, `complete`, `done` or `finish`).

## Delivery

`answerKind.Execute(ctx, claim)`:

1. Reads the stored spec; re-reads the task through `KindTaskReader`
   (archived, deleted, other workspace, unwatched, coordinator-origin) and
   returns `refusal(taskArchived)` on any of those.
2. Calls `AnswerTargets.CurrentTargets`. `ErrNotFound` (a deletion racing step
   1) returns `refusal(taskArchived)`; an absent member for the proposal's kind
   or any identity difference (`pending_id`, and for a permission
   `request_id`) returns `refusal(stale)`; a read error is `failed` with code
   `not_delivered` (nothing was delivered).
3. Re-validates the (possibly edited) draft against that live target with the
   propose-time validator; a failure is `failed` with code `invalid_draft`.
4. Delivers:
   - clarification: `Resolver.ResolveBundle(ctx, pending_id, Outcome{Answers})`
     with the context's authorisation resolving to the approving manager, so
     `AuthorizeTaskAccess` applies as it does to the manager answering in place;
   - permission: `RespondToPermission(ctx, taskID, sessionID, requestID,
     pendingID, optionID, false, false)`; the orchestrator records source `web`.
5. Maps the result ([Failure mapping](#failure-mapping)). `Execute` always returns after the operation returns and converts every error to an `*execFailure` or `*execRefusal`, so `executeKind`'s deadline branch is never reached for an answer: the deadline has no special case, and a context error is classified by the result shape like any other (pre-claim `(nil,false,ctxErr)` is `not_delivered`; after the claim won, `outcome_unknown`).

Policy re-check: `recheckPolicy` (`approve.go`) is skipped for the class setting
when the proposal kind is `answer`; instead it returns the existing
`PolicyDeniedError{Action: answer}` only when the `coordinatorAnswer` flag is
not effective. The flag and policy are read once per request.

Exactly-once rests on three guards: the fenced proposal claim (AC-002.4), the step 2 identity re-check, and the operation's own claim (AC-002.5). No new lock.

`ReRunsOnStaleClaim()` returns false.  A claim that goes stale mid-delivery is settled `failed` / `outcome_unknown` by the existing recovery (`approval_sweep.go`) and never re-run. The request-triggered form of that recovery, `approveApprovingKind` on a claim older than `staleApproveWindow` (`settleStaleKind`), applies unchanged to an answer and is the only exception to AC-002.7's conflict; an edit-carrying approve still gets the conflict.
The answer claim is fenced: `ClaimProposalRawTx` takes an optional fence (the `updated_at` the approver read) and, for kind `answer`, never matches an `error = 'outcome_unknown'` row, so an approver that read a retryable failure loses to a row since settled `outcome_unknown` and gets the proposal-conflict answer. A manager's retry of a retryable `failed` answer goes through
`approveKind` again, repeating steps 1 to 5. An
`outcome_unknown` proposal is not claimable by approve: for kind `answer` only,
`approveKind` returns the existing proposal-conflict answer for it (resume,
message and move keep their Try again), so a possibly delivered answer is never repeated or rewritten to `stale`. Every error `Execute` returns becomes an `*execFailure` with a code of the closed set, so generic `err.Error()` text is never stored. The same holds for
errors `runClaimedKind` and `executeKind` raise before or around `Execute`
(coordinator unreadable, step 1 task read fails, the default branch): for `answer` they settle `failed` / `not_delivered`.

### Edits

`answerKind.ValidateEdits(base, edits)` has its own check and leaves the shared `editFieldNames` list (read by `carriesKindEdits` and `refuseAllEdits` for `resume`, `message` and `move`) alone. `edits` absent, `null` or `{}` is no edit. Otherwise
the keys are checked in order: `target`, `pending_id`, `sources`, `confidence`,
`needs_decision`, `snapshot` return `*FieldError{Field: name, Message:
"not_editable"}`; any key other than `draft` returns `unknown_field`. `draft`
replaces `base.draft` whole (no merge) and is normalized by `normalizeDraft`
(trim text; de-duplicate option ids keeping the first; drop an empty `custom_text` and an empty `option_ids`). If the normalized edited
draft equals the normalized stored draft, `ValidateEdits` returns `base`, so
approval is not edited.

Live validation runs before the claim, and only when `ValidateEdits` returned
an edit (not `base`): `prepareKindApproval` (`approve_kinds.go`) calls an
optional `EditValidator.ValidateEditsLive(ctx, exec base, edited)` on the kind;
for `answer` it reads `CurrentTargets`. If the task is unavailable, or the
target is absent or differs in identity from the stored one, it returns nil
(the claim proceeds and `Execute` refuses: the 409 refusal). If the
target is current it runs the propose-time draft validator and returns a
`*FieldError` (HTTP 400), row unchanged. A read error returns HTTP 500, body code `target_read_failed`, row unchanged (inside `Execute`, after the claim, it settles `failed` / `not_delivered`). A stored spec that does not unmarshal makes `ValidateEdits` return `*FieldError{draft, "stored_spec_unreadable"}`, so Reject is the way out. `ValidateEdits` also shape-checks the normalized edited draft (member names, types, entry members) with stage 1's draft checker, needing no live target, so a malformed draft is a field error even without a current target; only content validation needs one. A `draft` of `null` is an incomplete draft
(`FieldError{draft}`). The approve body is the edit map itself
(`ApproveProposalRequest`), so the key checked is `draft` at the top level.
An optional `EditCarrier.CarriesEdits(edits ApproveProposalRequest) bool` replaces `carriesKindEdits` for `answer` (true for any key present, `draft: null` included), so an edit-carrying approve on an `approving` row gets the 409 conflict. A retry of a `failed` proposal starts from the stored final draft, so `invalid_draft` fails again until edited.
`approved_with_edits` is set from `Claim.TextEdited` (`!sameSpec(final, stored)` in `runClaimedKind`), as for every non-create kind.

## Failure mapping

`Execute` returns `*execRefusal{code}` for a definite refusal, a new error type
handled in `executeKind` beside `*execFailure` and settled by
`RefuseClaimedTx` (see [Refusal](#refusal)). Every other branch uses the
existing `failKind` with a closed code.

| Resolution result | Proposal | Activity |
| --- | --- | --- |
| recorded (`ResolveBundle` `(res, true, nil)`; permission nil error) | approved; outcome JSON `{session_id, pending_id, answers:[{question_id, option_ids}] \| option_id}` | approved (`edited` if edits) |
| `IsNotActiveError`, `(res, false, nil)` (the loser shape: lost to another caller, with the winner's resolution), `ErrPermissionNotFound`, `ErrPermissionStale`, `ErrPermissionAlreadyResolved`, identity differs | refused `stale` | refused, reason `stale` |
| clarification `ErrBundleNotFound` and `ErrPermissionTaskOrSessionNotFound`: Execute re-reads the task through `KindTaskReader`; unavailable (deleted, archived, left the workspace, unwatched, coordinator-origin) | refused `task_archived`; otherwise (session gone) refused `stale` | refused, reason as settled |
| task archived, deleted, left workspace, unwatched | refused `task_archived` | refused, reason `task_archived` |
| `clarification.IsValidationError`, or the draft failing Execute step 3 | failed `invalid_draft`; `retryable` | failed |
| `ErrPermissionOptionNotOffered` | failed `option_not_offered`; `retryable` | failed |
| `ErrPermissionResolutionInProgress` | failed `resolution_in_progress`; `retryable` | failed |
| `ResolveBundle` `(nil, false, err)` for any other error (a timeout or transport error the resolver reports before its claim, claim-write error, failed re-read after a lost claim); a target read error in Execute step 2; `ErrPermissionHumanInputRequired` (reachable only for source `automation`, mapped here for completeness) | failed `not_delivered`; `retryable` | failed |
| `ResolveBundle` `(nil, true, err)` (claim won, then attachment or delivery error, with or without the resolver restoring the claim); `ErrPermissionDeliveryFailed`; `ErrPermissionAuditFailed` and its post-delivery subtype; any unclassified error; an operation that returned no result (a crash) | failed `outcome_unknown`; not retryable | failed |

`retryable` is computed from the code (the first four failed rows) and serialised on the DTO.

Refusal HTTP shape: `approve`, `reject` and `reply` on a refused proposal, and the approve that settles one, return the existing
`ProposalConflictError` status (409) with body `{error: "refused", code, proposal}`. Elsewhere 409 stays as it is.

## Refusal

A refused answer proposal is status `rejected` with `error` set to `stale` or
`task_archived`; `reject_reason` stays empty and both refuse updates set `decided_by` to NULL, so no person is recorded as having decided it, even when the row had been claimed by an approval. Within kind `answer` no `failed` code equals either string, so `(kind answer, status rejected, error in the set)` identifies a refusal; other kinds are never read as refused. A person's reject keeps the row's failure code. This removes the row from the open set (unique index,
cap, pending list, re-claim) with no new status and no schema change.

Two store methods are added in `store_proposals.go`, both returning whether a
row matched:

- `RefuseOpenTx(ctx, tx, id, observedUpdatedAt, code, now)`: `UPDATE ... SET status='rejected', error=?, decided_by=NULL, updated_at=? WHERE id=? AND status IN
  ('pending','failed') AND updated_at=? AND (error IS NULL OR error <>
  'outcome_unknown')`. The fence on the observed `updated_at` makes a refusal
  decided on a stale observation a no-op. For supersede-on-propose, the only caller.
- `RefuseClaimedTx(ctx, tx, id, token, code, now)`: the same update `WHERE id=? AND status='approving' AND claim_token=?`, clearing the token and `decided_by`. For a refusal
  found in `Execute` (the row is `approving`); a lost fence means another path
  settled it and the existing `errSettleFenced` handling returns the current row.

`settleRefusedTx(ctx, tx, p, code)` calls the matching method and, only on a match, `Service.Record(ctx, tx, ActivityRow{Outcome: ActivityRefused,
ReasonCode: &code, Authorization: AuthRequiresApproval, ProposalID: &p.ID,
ActionClass: ActionAnswer, ...})`, so a second call changes nothing and records
nothing. The row carries the proposal id and is not coalesced by
`recordRefusalTx`; the outcomes summary counts rows with a proposal id as
proposal refusals and leaves `refusal_count` for tool-call refusals. It takes
the open transaction because `settleDecision` opens its own transaction and cannot run inside the propose callback;
approve-time refusals use `RefuseClaimedTx` in the existing settle path.

## Staleness

Two settle sites share `settleRefusedTx`; no background check exists: **on propose** of a newer proposal for the task
(inside the propose transaction), and **on approve** (`Execute` steps 1 and 2,
immediate; a deleted task is `task_archived`, a task with no
primary session an absent member, `stale`). A spec that does not unmarshal
fails `invalid_draft`, logged. Until then a superseded proposal stays pending with a "no longer current" notice; a settled one is final (AC-003.5).

## Policy class and flag

`ActionAnswer` is added to the const block and to `AllActions` (now seven).
`raisableClasses` is unchanged, so `Validate` already returns
`automatic_not_available` for it; `isPolicyAction` accepts it.

Storage keeps `answer` optional, and absence survives parsing. `deniedActions()` keeps iterating `AllActions`, so it returns seven entries with `answer: denied`, which is what the unreadable-policy paths need. `PhaseOnePolicy()` and `ParsePolicy`, which seed from it, delete the `answer` key from the seeded map and then read `answer` only when the stored object has the key, parsing an invalid or `automatic` value to `denied`; the original six are filled as before. A NULL policy
(`ParsePolicy(nil)` = `PhaseOnePolicy()`) and every pre-feature policy
therefore parse with `answer` absent. Both unreadable-policy paths (`unreadablePolicy` and `policyFor`'s error branch, which returns `deniedActions()`) deny all seven, and `policyFor` returns from that branch before the absent-to-`requires_approval` step. When a settings request omits `answer`, the write copies the stored entry (`denied` included, or its absence) into the request policy before `policyDelta`, `checkRaise` and the write. `LowerClass`,
the settings write and any other write-back copy the map as parsed, so absence
is never turned into `denied`; nothing reads `Allows(ActionAnswer)` on a parsed policy.

`parsePolicyMember` (`settings.go`, the write-side parser shared with guided
setup) is distinct from `ParsePolicy`: it requires the original six classes,
refuses `automatic` for any non-raisable class with `automatic_not_available`
(`answer` included, never coercing it), and takes an `answerAllowed` argument
for `answer`: absent is fine; present with `answerAllowed` false is the field
error `answer_unavailable`; present otherwise is validated and stored. The settings write checks `answerAllowed` before the `automatic` refusal, so with the flag off `answer_unavailable` wins; it passes `answerAllowed` = the flag is effective; guided setup
passes false, so a setup body naming `answer` is refused and an omitted one
leaves it absent.  No creation path writes `answer`; an absent entry resolves in `policyFor`.

One function, `(s *Service) policyFor(c *Coordinator)` (`reads_phase2.go`),
computes the effective policy: it resolves an absent `answer` to
`requires_approval`, then forces `answer` to `denied` when the
`coordinatorAnswer` flag is not effective (a stored `requires_approval`
included). The branch of `Policy()` that returns `PhaseOnePolicy().Actions` when
phase 2 is off omits `answer` (the flag needs phase 2). The tool surface
(`ToolNamesPhase31`) and propose use `policyFor`. Every response that carries the policy (the settings read and save, and the coordinator DTO in `coordinatorDTO`) is built through one `publicPolicy(actions, answerEffective)` over the `policyFor` result, which deletes `answer` while the flag is not effective, so no client receives a class it cannot name. 
`actionsEqual` and `recordClassChanges` compare `answer` by effective value
(absent equals `requires_approval`), so a first save on a pre-feature
coordinator is no change and a change row's `from_value` for an absent entry is
`requires_approval`. The web keeps `CONTROL_ACTIONS` (six) for guided setup (`initialSetupState`, `setup-review`), whose bodies never name `answer`. The May do table, `isPolicyDirty`, `mergeStored` and `settleAfterSave` iterate a settings-only list, `CONTROL_ACTIONS` plus `answer` when the policy received includes `answer` (UI label, no `automatic` choice), so an answer-only change marks the draft dirty and is saved.

Reply: `ReplyToProposal` checks only `phase3` today; for kind `answer` it also
returns the flag `PolicyDeniedError{Action: answer}` when the flag is not
effective and the proposal is pending, after the state check (so a refused or
settled proposal gets its conflict answer first), and `replyTitleVerbs` gains
`ProposalKindAnswer: "Answer"`, so the condition text names the task.

The flag is a typed registration in `apps/backend/internal/runtimeflags/registry.go`
(key `features.coordinatorAnswer`, env `KANDEV_FEATURES_COORDINATOR_ANSWER`),
with `prod: "false"`, `dev: "false"`, `e2e: "true"` in `profiles.yaml`, and a
frontend flag mirror; the work order uses `/runtime-feature-flags`. Effective
means flag true and `features.coordinator`, `coordinatorPhase2` and
`coordinatorPhase3` effective (`Service.phase3` already requires the first
two).

Pause: none of the operations consults `pause` (AC-004.5).

## Card

The Needs you proposal card (`apps/web/app/coordinator/components/`, the card
that already switches on `proposal.kind`) gets an `AnswerProposalCard`:

```text
| Answer  Q: Which database?                      [Needs your decision] |
| Draft:  (•) Redis   ( ) Memcached               confidence: Medium |
| Sources: docs/adr/0007.md:42 [copy] · https://… ↗                  |
| [Approve] [Edit] [Reject] [Reply with condition]    Open task      |
```

The question, options and permission title render from the proposal's
`snapshot`. The live relay read (`GET .../relay/:taskId`) is used only to prefill **Edit** and to show a "no longer current" notice when the live target differs. When it shows no target, a different identity or fails, Edit uses the snapshot and the stored draft with an "out of date" notice and keeps the entered text. Edit swaps in draft-only controls
(`AnswerDraftEditor`, a presentational component with per-question option and
text inputs; for a permission only `allow_once` and `reject_once`), not
`question-answer.tsx` or `permission-answer.tsx`, which resolve in place on
click and offer lasting options. Approve calls the
existing approve route with the body `{"draft": {...}}` when edited (the route
body is the edit map itself). A 409 refusal body
renders **Refused (stale)** or **Refused (task unavailable)** from `code`; a
validation or conflict response keeps the entered text. A `failed` proposal
shows its failure with a fixed action set by code (AC-005.5): Retry and Reject for `resolution_in_progress` and `not_delivered`, Edit and Reject for `invalid_draft` and `option_not_offered`; an
`outcome_unknown` proposal shows the may-have-been-delivered notice and Reject only. On a
phone width the actions stack in a bottom row of 44-pixel targets. Source `ref` rendering
is a pure helper (`isHttpUrl`: scheme exactly `http` or `https` after parsing, else text), so a `javascript:` ref is never a link.
Strings live under the `coordinator` namespace in all seven locales (`pnpm run i18n:zh-hant` for the Traditional Chinese pair).

When the flag is not effective, the card renders Reject only (no Approve, Edit
or reply).

## Recording

`kindAction` (`approve.go`) gains a case returning `ActionAnswer` for `ProposalKindAnswer`; the activity service already writes proposed, approved, rejected, failed, refused and returned rows per class. The `proposed` row uses the rationale as detail. The `approved` row detail is built by one pure function from the delivered answer and the labels read in Execute (200 runes, AC-006.3). A refusal code in `{stale, task_archived}` is recorded `refused` with that reason by `settleRefusedTx`, never `failed`; failure texts of an answer are the closed codes of the failure mapping. The ledger digest gains `propose_answer_kandev` if it keeps a closed tool-name list. The activity summary (`ActivityCounts`) groups by class and outcome, so `answer` appears; it also groups refused rows by whether a proposal id is present: `OutcomeCounts.Refused` keeps `SUM(refusal_count)` over rows with no proposal id and a new `ProposalRefused` counts the rows with one, so a proposal refusal never changes the tool-call count. A refusal emits no decision event (`notifyDecided`); approvals, edits, rejects and returns reach the outcomes recorder through existing events. No undo handler is registered for the kind (AC-002.8), so the log offers no Undo.

## Security

- Proposer identity is the coordinator principal guard (`coordinatorPrincipalOnlyActions`); approve and reject stay REST-only (`ActionApproveProposal`).
- Free text reaches the agent as the resolver sanitises it. No lasting permissions (AC-001.4), no unwatched tasks (AC-001.6).
- `AnswerTargets` has no authorization of its own; only propose (after its watch check) and Execute call it.

## Failure and recovery

A crash after the claim leaves `approving`, which startup recovery settles `failed` / `outcome_unknown` without re-running; before it, `pending`.

## Related ADRs

[ADR 2026-11-07 coordinator answer proposals](../../../decisions/2026-11-07-coordinator-answer-proposals.md)
amends relay D5 and the "failed keeps Try again" rule for refused answers.
