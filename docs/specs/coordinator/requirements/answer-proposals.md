---
id: coordinator-answer-proposals
title: Answer proposals
status: draft
system: coordinator
owners:
  - kandev
created: 2026-11-07
last_updated: 2026-11-07
---

# Answer proposals Requirements

## Overview

A watched card's agent often stops on a question or a permission request that
only a person can settle. Phase 3 lets a manager answer it in place
([relay](relay.md)), but the coordinator itself cannot help: a `message`
proposal cannot resolve a pending clarification. This document adds a
proposal kind, `answer`, in which the coordinator drafts the answer to the
card's pending clarification bundle, or decides its pending permission request,
and a manager approves, edits or rejects the draft before anything is
delivered. The coordinator drafts an answer to every question, maintainer-level ones
included; the manager's approval is the safeguard (owner decision,
2026-11-07). Staleness, policy class and flag, the card and recording are in
[answer proposal lifecycle](answer-proposal-lifecycle.md).

Decision record:
[ADR 2026-11-07 coordinator answer proposals](../../../decisions/2026-11-07-coordinator-answer-proposals.md).

## Terminology

- **Answer proposal:** a proposal of kind `answer`. Its **target** is the
  watched task's current answerable clarification bundle (a
  **clarification answer**) or its current pending permission request (a
  **permission answer**), as the [relay read](relay.md) defines them.
- **Target identity:** the target's `pending_id`; for a permission answer also
  its `request_id`. Two targets are the same only when every identity value is
  equal.
- **Draft:** the coordinator's complete proposed response: for a clarification
  answer, one entry per question of the bundle; for a permission answer, one
  chosen option.
- **Source:** one piece of evidence the coordinator cites for its draft.
- **Superseded:** an answer proposal whose target identity is no longer the
  task's current answerable target, because the target was answered, cancelled,
  expired or replaced by a different one.
- **Refused:** the terminal state of an answer proposal the system ended
  without a person's decision. A refused proposal has status `rejected`, no
  person recorded as having decided it, and a refusal code in the closed set
  `stale` or `task_archived`. A person's own reject
  is a `rejected` proposal with no refusal code.
- **Open answer proposal:** an answer proposal for the task whose status is
  `pending`, `approving` or `failed`.
- Other terms are defined in the [system README](../README.md#terms).

## Requirements

### REQ-COORDINATOR-ANSWER-001: Proposing an answer

**Intent:** The coordinator can put a drafted answer in front of a manager for
a question or permission its card is waiting on.

**User story:** As a workspace manager, I want the coordinator to draft the
answer to my cards' questions and say what it based it on, so that I approve
or fix a draft instead of researching each question.

#### Acceptance criteria

- **AC-COORDINATOR-ANSWER-001.1:** While the `answer` class is available to a
  coordinator ([REQ-COORDINATOR-ANSWER-004](answer-proposal-lifecycle.md#req-coordinator-answer-004-policy-class-and-flag)),
  the coordinator's tool surface shall include `propose_answer_kandev`, beside
  `propose_task_kandev`; otherwise the tool shall be absent from the surface
  and a call to it shall be refused.
- **AC-COORDINATOR-ANSWER-001.2:** `propose_answer_kandev` shall accept
  `task_id`, `target` (`clarification` or `permission`), `pending_id`, a
  `draft`, `sources`, `confidence`, `needs_decision`, `rationale` and
  `standing_order_ids` (the last two as the other propose tools define them,
  `rationale` at most 10,000 characters). `task_id`, `target`, `pending_id`,
  `draft`, `confidence` and `needs_decision` are required; `sources` defaults to
  an empty list. A required member that is absent or `null` is missing, and an
  absent or `null` `sources`, `rationale` or `standing_order_ids` takes its
  default. A member the tool does not define, at the top level or inside
  `draft` or a source, shall be refused with the field error `unknown_field`, whose field is the
  member's path (`draft.answers[0].x`, `sources[2].x`, or the top-level name).
  The tool decodes its arguments as a raw JSON object, so a member of the
  wrong JSON type (a non-array `standing_order_ids` included) gets that
  member's field error, never a whole-call parse failure. A call with several invalid members returns one field error, chosen in two stages.
  Stage 1 (shape): first a top-level member the tool does not define (the
  alphabetically first such name, field error `unknown_field`), then the first
  failing member in the order `task_id`, `target`,
  `pending_id`, `draft` (its shape: entry members and types, in entry order),
  `sources` (by index), `confidence`, `needs_decision`, `rationale`,
  `standing_order_ids`, so a `standing_order_ids` shape error is reported only when every earlier-listed member is valid. Stage 2, only when
  stage 1 passed: the task and target
  checks of [AC-COORDINATOR-ANSWER-001.6](#req-coordinator-answer-001-proposing-an-answer),
  then the draft's content against the live target (unknown question or option,
  completeness, the resolver's rules; entries in order).
  `needs_decision` takes the JSON booleans only.
- **AC-COORDINATOR-ANSWER-001.3:** A clarification draft shall hold exactly
  one entry for each question of the bundle, in the bundle's question order,
  each naming the question and carrying the chosen option ids, free text, or
  both. Free text is trimmed and option ids are de-duplicated, keeping the given
  order, before anything is validated, stored or compared. An entry left with neither
  an option id nor text is the field error `empty_answer` on `draft`. The
  normalized draft shall then pass the resolver's own outcome validation (rules
  and limits unchanged); the exact-set and order rule is this tool's addition. A
  permission draft shall name exactly one of the request's current option ids.
  A draft that is incomplete, names an unknown question or option, or breaks
  the resolver's own rules shall be refused with a field error and nothing
  shall be stored. `needs_decision` shall not relax this: when no source
  settles the question, the draft is still the coordinator's complete best
  guess.
- **AC-COORDINATOR-ANSWER-001.4:** A permission draft shall name only an option
  whose kind is `allow_once` or `reject_once`. An option of kind `allow_always`
  or `reject_always`, and an option whose kind is missing or not one of the
  four known kinds, shall be refused with a field error, as shall an edited
  draft that names such an option. A manager can still choose a lasting option in place.
- **AC-COORDINATOR-ANSWER-001.5:** `sources` shall be a list of at most 10
  entries, kept in the order given, each with a `ref` of 1 to 300 characters
  after trimming (e.g. `path:line`, a Q-row, a decision id, a URL) and
  an optional `label` of at most 120 characters. It may be empty. `confidence`
  shall be one of `low`, `med` or `high`; `needs_decision` shall be a boolean.
  Any other value shall be refused with a field error.
- **AC-COORDINATOR-ANSWER-001.6:** `propose_answer_kandev` shall validate its
  target the way [relay](relay.md) defines an answerable one, at call time:
  the task is in a workflow the coordinator watches, is in the coordinator's
  workspace, is not archived and is not a coordinator conversation; and the
  task's primary session currently has a target whose identity equals the
  call's `target` and `pending_id` (and, for a permission, whose options
  include the draft's option). If the task is not watched, not found or
  archived, the call shall be refused as the other propose tools refuse it. If
  the task has no current answerable target of that kind, or its identity
  differs, the call shall be refused with the typed error `nothing_to_answer`
  ([AC-COORDINATOR-ANSWER-001.10](#req-coordinator-answer-001-proposing-an-answer))
  and nothing shall be stored. When the task has both a clarification bundle and a permission
  request, the call's `target` selects which one it addresses. A failure to
  read the target shall be refused with the read error of
  AC-COORDINATOR-ANSWER-001.10 and nothing shall be stored; a task that
  vanishes between the checks is refused as not found. The approval-time check
  ([003.1](answer-proposal-lifecycle.md#req-coordinator-answer-003-staleness))
  is the authoritative one.
- **AC-COORDINATOR-ANSWER-001.7:** The coordinator shall hold at most one open
  answer proposal per task, whatever its target kind. A call whose `target` and
  `pending_id` equal the open proposal's, whether the open one is pending,
  approving or failed, shall store nothing and return the open proposal with
  `deduplicated: true` and without replacing its draft; this answer is given after stage 1 of AC-COORDINATOR-ANSWER-001.2, before the target and draft content are validated, and also inside the store step, so a racing call that finds an equal row there returns it the same way. Any other call on a task
  that has an open proposal shall be decided as follows, and only after the
  call itself has passed validation:
  (a) when the open proposal is approving, the call shall be refused with the
  typed error `approval_in_progress` (AC-COORDINATOR-ANSWER-001.10) and nothing stored;
  (b) when the open proposal is pending or failed (other than `outcome_unknown`,
  see (c)), was created before the call's target read began, and its target is
  no longer current according to that read ([Superseded](#terminology)), the
  open proposal shall be refused with code `stale` and the new proposal stored,
  in one atomic step, so that after the step exactly one answer proposal for the
  task is open;
  (c) otherwise (the open proposal's target is still current, or it was created
  after the call's read began, or it is failed with `outcome_unknown`) the call
  shall be refused with the typed error `answer_pending` (AC-COORDINATOR-ANSWER-001.10) and nothing stored; a
  failed `outcome_unknown` proposal keeps blocking until a person rejects it
  ([AC-COORDINATOR-ANSWER-002.6](#req-coordinator-answer-002-approving-and-delivering-an-answer)).
  Racing calls are serialized by the coordinator's write lock; the second is decided against the first's row. If an approval or reject changes the open proposal between the call's read and the refusal in (b), the call re-reads it and decides again; a closed one lets the store proceed.
- **AC-COORDINATOR-ANSWER-001.8:** Only a coordinator principal shall be able
  to call `propose_answer_kandev`. Any other principal, including a human
  session and an agent that is not a coordinator conversation, shall be
  refused, and nothing shall be stored, whatever the caller's other permissions.
- **AC-COORDINATOR-ANSWER-001.9:** A stored answer proposal shall be pending and
  shall change no task, session or conversation state. Storing
  it shall record a `proposed` activity row of class `answer`
  ([REQ-COORDINATOR-ANSWER-006](answer-proposal-lifecycle.md#req-coordinator-answer-006-recording)). It
  shall also store a snapshot of the target at call time (the questions with
  their options, or the permission's title, summary and options, each text at
  most 2,000 characters), so the card can show the question after the target is gone. The snapshot is not editable.
- **AC-COORDINATOR-ANSWER-001.10:** The typed refusals `nothing_to_answer`,
  `answer_pending` and `approval_in_progress` shall reach the coordinator as a
  validation error of the propose tools (never an internal error, and never
  logged as an error) whose message is exactly the typed name and whose details
  carry `code` equal to that name and `field`: `pending_id` for
  `nothing_to_answer`, `task_id` for `answer_pending` and
  `approval_in_progress`. The answer tool's result text for any validation
  error is `<message> (field: <field>)`, so the agent sees the field. A failure to read the task's target shall reach the
  coordinator as an internal error with the message `could not read the task's
  current question`, logged, with nothing stored. The refusals differ from field errors by `code`.

### REQ-COORDINATOR-ANSWER-002: Approving and delivering an answer

**Intent:** Approving a draft answers the card's agent exactly once, the way a
manager's own answer would.

**User story:** As a workspace manager, I want to approve the drafted answer,
or fix it first, and have the agent continue as if I had answered myself.

#### Acceptance criteria

- **AC-COORDINATOR-ANSWER-002.1:** Approving an answer proposal shall deliver
  its draft through the same resolution operations the in-place answer uses:
  the clarification resolver for a clarification answer, and the permission
  response operation (the one behind `permission.respond`) for a permission
  answer. It shall add no second resolution path; those operations' claim, validation and race handling apply unchanged. A permission delivery records audit source `web`.
- **AC-COORDINATOR-ANSWER-002.2:** Approval shall be permitted to a person with
  `workspace.manage`, and to no other principal, under the same rules as other
  proposal kinds. A coordinator or unresolved principal shall not approve. Approval of an answer proposal shall not consult the `answer` class
  setting (a pending proposal stays approvable after the class is set to
  `denied`, [AC-COORDINATOR-ANSWER-004.3](answer-proposal-lifecycle.md#req-coordinator-answer-004-policy-class-and-flag)),
  and shall be refused while the `coordinatorAnswer` flag is not effective.
- **AC-COORDINATOR-ANSWER-002.3:** Approval may carry edits. The approve request body is the edit map
  itself, as for every other kind, so an edit is `{"draft": {...}}` at the top
  level and never nested under another key (`{"edits": ...}` is `unknown_field`). The only editable
  member is `draft`, and an edit's `draft` replaces the whole stored draft (it
  is not merged, and a partial draft is refused as incomplete). Edits that are
  absent, `null` or empty, and an edit whose normalized `draft`
  ([AC-COORDINATOR-ANSWER-001.3](#req-coordinator-answer-001-proposing-an-answer))
  equals the normalized stored draft, shall count as no edit. The edited draft shall be validated by the rules of
  AC-COORDINATOR-ANSWER-001.3 and
  [AC-COORDINATOR-ANSWER-001.4](#req-coordinator-answer-001-proposing-an-answer)
  against the target as it is at approval, before the proposal is claimed,
  only for a real edit: a failing edit against a still-current target gets a
  field error (HTTP 400) and the proposal is unchanged. When the
  target is absent, differs in identity, or the task is unavailable, there is
  no live target to validate content against: only the draft's shape (names, types, entry members, `empty_answer`) is checked, and a draft that passes is claimed and delivery refuses it as in
  [AC-COORDINATOR-ANSWER-003.1](answer-proposal-lifecycle.md#req-coordinator-answer-003-staleness)
  (a 409 refusal, not a 400). A target read failure before the claim returns HTTP 500, body code `target_read_failed`, proposal unchanged. An edit whose `draft` is `null` is an
  incomplete draft (field error `draft`). An
  edit naming `target`, `pending_id`, `sources`, `confidence`, `needs_decision`
  or `snapshot` shall be refused with `not_editable`, and an edit naming any
  other member shall be refused with `unknown_field`; in both cases nothing
  is delivered and the proposal is unchanged. A non-empty edit on an approving proposal gets the existing
  proposal-conflict answer. Delivering an edited draft shall be recorded as
  `approved_with_edits`, and the delivered answer shall be the edited one.
- **AC-COORDINATOR-ANSWER-002.4:** When two approvals of the same proposal
  race, exactly one shall claim it and deliver; the other shall receive the
  existing proposal-conflict answer for an already-claimed proposal and shall
  deliver nothing. The claim of an answer proposal is fenced on the failure state the approver read, so an approval that read a retryable failure cannot claim a row since settled `outcome_unknown`; it gets that same answer.
- **AC-COORDINATOR-ANSWER-002.5:** When a manager answers the same target in
  place while an approval is in flight, or the other way round, exactly one
  answer shall be delivered to the agent. The loser shall deliver nothing: an
  approval that loses to an in-place answer shall settle as superseded
  ([AC-COORDINATOR-ANSWER-003.1](answer-proposal-lifecycle.md#req-coordinator-answer-003-staleness)); an
  in-place answer that loses reports the resolver's own lost outcome.
- **AC-COORDINATOR-ANSWER-002.6:** A successful delivery shall settle the
  proposal as approved, unblock the card's agent with the delivered answer, and
  record the delivered answer in the proposal's outcome: for a clarification
  the answered question ids each with its chosen option ids, in bundle order,
  and for a permission the chosen option id. The delivered free text is not stored in the
  outcome. The resolution operations' results shall be classified
  as follows, and nothing else shall be inferred. A failed answer proposal's
  recorded failure text is always one of the closed codes below, never free
  text:
  (a) the target is gone, no longer active, already resolved, stale, or the
  operation was won by another caller: refused with code `stale`;
  (b) nothing was delivered and the failure is `retryable`: `invalid_draft` (the resolver's or
  the draft's own validation failed, including a target change that made an
  edited draft invalid), `option_not_offered`, `resolution_in_progress`, and
  `not_delivered` (every other error the resolver returned before its claim
  was won, a failed re-read after a lost claim, a failure reading the target
  before delivery, and the permission operation's human-input-required);
  (c) anything else, including every error after the resolver's claim won or
  after the permission operation began delivering, and an operation that
  returned no result (a crash): `failed` with the code `outcome_unknown`, and no retry is offered. A result is classified by the shape the operation returned, never by the kind of error, and the approval's deadline has no special case: a timeout or cancellation reported before the claim was won is (b), the same after it is (c).
  Errors raised by the approval itself before the operation was called (the
  coordinator or the task cannot be read) deliver nothing and are
  `not_delivered`.
  The proposal record shall expose `retryable`, true exactly for the codes of
  (b); the card offers Retry only when it is true and the code is
  `resolution_in_progress` or `not_delivered`
  ([AC-COORDINATOR-ANSWER-005.5](answer-proposal-lifecycle.md#req-coordinator-answer-005-the-answer-card)).
  Only the `answer` kind refuses to re-claim an `outcome_unknown` failure; the
  other kinds keep their Try again. A failed proposal with
  `outcome_unknown` shall not be refused by an approval attempt or by a newer
  proposal; it ends only by a person's reject, and an
  approve of it shall get the existing proposal-conflict answer. Approval shall
  never run the delivery again on a claim that went stale, and a manager's
  explicit retry of a retryable failed answer proposal shall repeat the whole
  approval path, including the target check, so it cannot deliver twice.
- **AC-COORDINATOR-ANSWER-002.7:** Rejecting an answer proposal shall deliver
  nothing and shall leave the target pending for the agent and for in-place
  answers. Replying with a condition shall settle it as returned and send the
  condition to the coordinator's conversation, as for other kinds. Reject shall
  be accepted for a pending or failed answer proposal; reply shall be accepted
  for a pending one only. Any other action on an answer proposal that is
  approving, approved, returned, rejected or refused shall get the existing
  proposal-conflict answer carrying the current proposal, and change nothing,
  except that an approve of an approving proposal claimed longer ago than the
  existing stale-claim window settles it `failed` / `outcome_unknown`, never re-run.
- **AC-COORDINATOR-ANSWER-002.8:** An answer proposal shall not be undoable;
  the activity log shall not offer Undo for it.

## Out of scope

See [answer proposal lifecycle](answer-proposal-lifecycle.md#out-of-scope).
