---
id: coordinator-answer-proposal-lifecycle
title: Answer proposal lifecycle
status: draft
system: coordinator
owners:
  - kandev
created: 2026-11-07
last_updated: 2026-11-07
---

# Answer proposal lifecycle Requirements

## Overview

How an answer proposal (see [answer proposals](answer-proposals.md)) stays
current, who may use the class, how a manager sees it and how it is recorded.
Terms are defined in [answer proposals](answer-proposals.md#terminology) and
the [system README](../README.md#terms).

Decision record:
[ADR 2026-11-07 coordinator answer proposals](../../../decisions/2026-11-07-coordinator-answer-proposals.md).

## Requirements

### REQ-COORDINATOR-ANSWER-003: Staleness

**Intent:** A draft never answers a question other than the one it was written
for, and never answers twice.

#### Acceptance criteria

- **AC-COORDINATOR-ANSWER-003.1:** At approval, before any delivery, the
  current answerable target of the task shall be read the way
  [relay](relay.md) defines it. If it is absent, or its identity differs from
  the proposal's, the proposal shall end `refused (stale)` and nothing shall be
  delivered. If the resolution operation itself reports the target already
  resolved by another caller, no longer active or not found, the outcome shall
  be the same ([AC-COORDINATOR-ANSWER-002.6](answer-proposals.md#req-coordinator-answer-002-approving-and-delivering-an-answer)(a)).
  If the task is archived, deleted (on every path), left the coordinator's workspace, is no longer in a workflow the
  coordinator watches, or became a coordinator conversation, the proposal shall
  end refused with code `task_archived` and nothing shall be delivered. The
  refusal shall be settled on the claimed proposal (approving to rejected) so it
  is never counted as open. A staleness refusal never applies to a failed
  proposal with `outcome_unknown`.
- **AC-COORDINATOR-ANSWER-003.2:** While the `coordinatorAnswer` flag is
  effective, an answer proposal that is pending or retryable-failed and is
  superseded, or whose task became unavailable
  ([AC-COORDINATOR-ANSWER-003.1](#req-coordinator-answer-003-staleness)),
  shall be refused automatically, with no manager action, at the first of two
  moments: in the same atomic step in which a newer answer proposal for the
  same task is stored
  ([AC-COORDINATOR-ANSWER-001.7](answer-proposals.md#req-coordinator-answer-001-proposing-an-answer));
  or at the next approval attempt. No background check settles it in between:
  until one of those moments the proposal stays pending and is listed, and
  nothing is delivered by anything but an approval, which refuses it first. In
  the approval attempt, a deleted, archived, workspace-leaving, unwatched or
  coordinator-conversation task counts as `task_archived`; a task with no
  primary session, or no target of the proposal's kind, counts as an absent
  target (`stale`), and a permission delivery that reports its task or session
  not found counts as `task_archived` when the task is unavailable and as
  `stale` otherwise. Each target read shall be bounded to five seconds. Settling
  shall be idempotent and fenced on the row as it was read: a row changed since
  is not settled, and a proposal already settled is not settled again and
  records no second activity row. An approval that runs while an in-place
  answer is in flight refuses as stale even if that answer then fails; the
  coordinator proposes again. A stored spec that cannot be parsed makes the approval fail `invalid_draft` (retryable) and is logged; an edit of such a proposal is refused with the field error `stored_spec_unreadable`, so Reject is the way out.
- **AC-COORDINATOR-ANSWER-003.3:** A refused answer proposal shall be stored as status `rejected` with the refusal code as its recorded error (`stale` or `task_archived`) and no recorded decider (`decided_by` empty, even when it had been claimed), and in the activity log shall be a `refused` row with the
  same reason code. A refused proposal is recognised only as a proposal of kind
  `answer`, status `rejected` and error in that set; no failure text of an
  answer proposal equals either code, and other kinds are never read as
  refused. A person's reject of an answer proposal is a `rejected` proposal
  whose error is empty or, when it was failed, that failure's closed code. The
  approval request that ends in a refusal, and any later approval, reject or
  reply request on a refused proposal, shall return HTTP 409 whose body names
  the refusal `code` and carries the current proposal. The card shall show
  **Refused (stale)** or **Refused (task unavailable)**, not an error, and no
  action. A refused proposal is not counted toward the open proposal cap and
  does not appear in the pending list.
- **AC-COORDINATOR-ANSWER-003.4:** When the target read at propose time or
  approval time fails with a read error, the proposal shall never be refused on
  the unread state: a propose call is refused with the read error of
  [AC-COORDINATOR-ANSWER-001.10](answer-proposals.md#req-coordinator-answer-001-proposing-an-answer)
  and stores nothing, and an approval's read during delivery settles the claimed proposal `failed` with the retryable code `not_delivered`, while the read that validates an edit before the claim leaves the proposal unchanged ([AC-COORDINATOR-ANSWER-002.3](answer-proposals.md#req-coordinator-answer-002-approving-and-delivering-an-answer)).
- **AC-COORDINATOR-ANSWER-003.5:** A superseded answer proposal shall not
  reopen when the same question later returns with a new identity, or when its
  target comes back after a refusal; the coordinator must propose again.
### REQ-COORDINATOR-ANSWER-004: Policy class and flag

**Intent:** A manager controls whether a coordinator may draft answers, and the
feature ships dark.

#### Acceptance criteria

- **AC-COORDINATOR-ANSWER-004.1:** A coordinator's May do policy shall have a
  seventh class, `answer`, with the settings `denied` and `requires_approval`.
  The setting `automatic` shall be refused for it at every settings write, with the same `automatic_not_available` error other non-raisable classes return (while the flag is not effective, `answer_unavailable` of 004.4 is returned instead, whatever the value),
  and `answer` shall not be in `raisableClasses`. No automatic path, wake,
  sweep or eligibility review shall approve an answer proposal.
- **AC-COORDINATOR-ANSWER-004.2:** A coordinator's stored policy may have no
  `answer` entry (every coordinator created before this feature, every
  coordinator created through a path that does not name it, and the policy of a
  coordinator whose stored policy is NULL). An absent entry is not a stored
  value. Parsing a stored policy (the NULL policy included) shall leave
  `answer` absent in the parsed actions, never `denied`, and the phase-one
  default policy shall not contain `answer`; an absent entry shall stay absent
  through every read, settings save that omits it, and class lowering, and
  shall resolve to `requires_approval` at the moment the effective policy is
  computed. An explicit stored `denied` shall win over the default; a stored
  `answer` that is `automatic` or unusable parses to `denied`. A stored policy that cannot be read shall deny every class including `answer`, and the absent-entry default never applies to it. A settings
  write refuses `automatic` for `answer` (AC-COORDINATOR-ANSWER-004.1) and
  does not coerce it. A settings write may omit `answer`, in which case the stored value (a stored `denied` included, or its absence) is preserved; it may also name it, with the
  settings of
  [AC-COORDINATOR-ANSWER-004.1](#req-coordinator-answer-004-policy-class-and-flag),
  and all other classes remain required. Change detection, the revision bump,
  the conversation reset and the class-change record compare `answer` by its
  effective value, so a save that names `requires_approval` for a coordinator
  whose entry is absent is not a change. The settings save response and read
  both return the effective policy, except that while the flag is not effective
  every response carrying the policy omits `answer` (AC-COORDINATOR-ANSWER-004.4).
- **AC-COORDINATOR-ANSWER-004.3:** Setting `answer` to `denied` shall remove
  `propose_answer_kandev` from the coordinator's tool surface for its next
  turn and shall refuse a propose call that races the change. It shall not
  change answer proposals that are already pending; they can still be
  approved, rejected or replied to (approval does not consult the class,
  [AC-COORDINATOR-ANSWER-002.2](answer-proposals.md#req-coordinator-answer-002-approving-and-delivering-an-answer)),
  and can still be refused. A propose or an approval reads the policy and the
  flag once, when it starts; a change made while it runs takes effect for the
  next request and for no part of the running one.
- **AC-COORDINATOR-ANSWER-004.4:** The `coordinatorAnswer` runtime flag
  (`features.coordinatorAnswer`) shall exist with the value `false` in the prod
  and dev profiles and `true` in the e2e profile, and shall be effective only
  together with `features.coordinator` and the phase 2 and phase 3 flags. While
  it is not effective, whatever the stored policy holds: the effective setting
  of `answer` is `denied`; no answer proposal can be proposed or approved; a
  settings write naming `answer` is refused with the field error
  `answer_unavailable`; the `answer` class is absent from every response that carries the policy (settings read and save, the coordinator view), so a client never receives or echoes it; the tool is absent; a reply to a pending answer proposal is refused with the same flag error as approval; and a
  pending or failed answer proposal already stored is listed with Reject only
  (no Approve, Edit or reply). A request on a proposal that is already refused
  or in any other state that accepts no such action gets the answer of
  [AC-COORDINATOR-ANSWER-002.7](answer-proposals.md#req-coordinator-answer-002-approving-and-delivering-an-answer)
  or
  [AC-COORDINATOR-ANSWER-003.3](#req-coordinator-answer-003-staleness) before
  any flag check, so Reject is offered only in a state that accepts it.
- **AC-COORDINATOR-ANSWER-004.5:** Pause shall not block proposing, approving
  or rejecting an answer proposal, because each is an attended act or a
  proposal; Pause stops only what the coordinator does on its own.

### REQ-COORDINATOR-ANSWER-005: The answer card

**Intent:** A manager sees the question, the drafted answer and its evidence
at a glance and acts on it.

#### Acceptance criteria

- **AC-COORDINATOR-ANSWER-005.1:** An answer proposal shall appear as a Needs
  you proposal item for managers. The card shall show the question text and its
  options (for a permission: the request title and action summary), the drafted
  answer (the chosen options and free text, or Allow or Deny for the chosen
  permission option), the sources, the confidence (Low, Medium or High) and the
  coordinator's rationale.
- **AC-COORDINATOR-ANSWER-005.2:** When `needs_decision` is true, the card
  shall show a **Needs your decision** badge beside the draft, and the draft
  shall be labelled as a best guess. Approve shall remain available.
- **AC-COORDINATOR-ANSWER-005.3:** A source whose `ref` is an `http` or
  `https` URL shall render as a link that opens in a new tab with no opener
  access. Any other `ref` (a path and line, a Q-row, a decision id) shall render
  as plain monospace text with a copy control, never as a link; the card's
  **Open task** action stays. The label, when present, shall render beside the
  `ref`. A source list longer than three shall collapse behind a count.
- **AC-COORDINATOR-ANSWER-005.4:** The card's actions shall be Approve, Edit
  and Reject, plus the reply-with-condition entry other kinds have. **Edit**
  shall open the draft in draft-only controls (never the in-place resolution
  controls, which would answer the agent without approval), filled with the
  draft, and shall submit through Approve with edits as the top-level `{"draft": ...}` edit map. When the live read shows no target, a different one or fails, Edit uses the proposal's snapshot and draft with a notice that the question may be out of date, and Approve then refuses it as stale if so. For a permission they offer only the allow-once
  and reject-once options of the request. Readers and
  members without `workspace.manage` shall see the card without the actions.
- **AC-COORDINATOR-ANSWER-005.5:** After an approval attempt the card shall
  show: for approved, the delivered answer; for **Refused (stale)**, a notice
  that the question was already answered or replaced and that nothing was
  delivered; for a failed proposal, its failure reason with a fixed action:
  `resolution_in_progress` ("Another answer is being recorded. Try again.") and `not_delivered` ("The answer could not be delivered. Try again.") offer Retry and Reject; `invalid_draft` ("The draft no longer fits the question. Edit it,
  then approve.") and `option_not_offered` ("The chosen option is no longer
  offered. Edit it, then approve.") offer Edit and Reject and no Retry; for `outcome_unknown`, a notice that the answer may
  already have been delivered and the task should be checked, with Reject only
  and no Retry. A card the manager is editing shall stay in place with the
  entered text until one of these outcomes, and a validation failure, a
  conflict or a stale refusal shall not discard the entered text. The card shall
  render its question, options and the permission's title from the proposal's
  own snapshot, so it still shows them after the target is answered, replaced
  or expired; the live relay read is used only to prefill Edit and to tell the
  manager the target is no longer current.
- **AC-COORDINATOR-ANSWER-005.6:** The card, its states and its copy shall work
  on a phone-width viewport with the native phone patterns the other proposal
  cards use, and every action shall have an accessible name. All new user-facing
  strings shall be in every shipped locale, without the em dash.

### REQ-COORDINATOR-ANSWER-006: Recording

**Intent:** Answer proposals are as inspectable as every other kind.

#### Acceptance criteria

- **AC-COORDINATOR-ANSWER-006.1:** The activity log shall record an answer
  proposal as action class `answer` with the outcomes proposed, approved,
  rejected, failed, refused and returned, the approving person, the `edited`
  flag for `approved_with_edits`, and for `refused` the reason code `stale` or
  `task_archived`. A refused proposal's row is distinguishable from a refused
  tool call: it carries the proposal id and authorization `requires_approval`,
  and it is never coalesced into a tool-call refusal count. 
- **AC-COORDINATOR-ANSWER-006.2:** The turn ledger shall record a
  `propose_answer_kandev` call in its call digest like any propose call, and
  the activity summary shall count answer proposals under class `answer` by
  outcome (proposed, approved, approved with edits, rejected, failed, and a new `proposal_refused` count of refused rows that carry a proposal id).
  The existing `refused` count keeps summing only tool-call refusals, so a
  proposal refusal never changes it. Approvals, edits, rejects and returns
  reach the outcomes recorder through the existing decision events like any
  kind; a refusal is not a person's decision and produces no decision event.
- **AC-COORDINATOR-ANSWER-006.3:** Recording an answer proposal shall store the draft's option ids and free text in the proposal; the only other copies are the `approved` row's `detail` below, the proposal's outcome ids ([AC-COORDINATOR-ANSWER-002.6](answer-proposals.md#req-coordinator-answer-002-approving-and-delivering-an-answer)) and the resolver's own record; the outcomes recorder, the ledger and every other row hold none. The `detail` of the
  `proposed` row shall be as for other kinds (the rationale, truncated). The
  `detail` of the `approved` row shall hold at most 200 characters built from the
  delivered answer: for a clarification `Q<n>: <text>` for each answered
  question in bundle order, joined by `; `, where `<text>` is the chosen option
  labels joined by `, ` (labels taken from the live bundle read at approval) or,
  when the entry has none, its free text; for a permission the chosen option's
  name. No `detail` shall hold the sources.

## Out of scope

- **Automatic answers.** The `answer` class never becomes automatic here. A
  follow-up needs its own eligibility evidence (approve and edit rates of
  drafts), a decision on which questions are safe, and a lowering rule.
- **Answers on unwatched boards.** A task in a workflow the coordinator does
  not watch is refused as for every kind.
- **Late-answer recovery.** An answer proposal never revives an expired
  bundle; the resolver's late-message handling stays with the in-place answer.
- **A recorded clarification answerer.** The activity row records the
  approving person; a durable answerer field on the clarification stays an ADR
  residual of [relay](relay.md).
- **Attachments in a draft.** A draft carries option ids and text only.
- **Always-allow permission drafts.** See AC-COORDINATOR-ANSWER-001.4.
- **Changing the behaviour of the clarification resolver, the permission
  operation or the relay read.** They are consumed unchanged. Additive
  exports that expose the resolver's existing outcome validation and question
  building to this tool are allowed and are the only change outside
  `internal/coordinator`, the tool registration, the runtime flag and the web UI.
- **Periodic stale-answer check (deferred).** No background check settles a
  superseded or unavailable answer proposal. Staleness is settled only when a
  newer answer proposal for the task is stored and at the next approval
  attempt; until then a stale proposal stays pending and is listed, and the
  card tells the manager when the live relay read no longer matches it. A
  follow-up would add a once-a-minute pass on the approval sweep tick that
  reads each pending or retryable-failed answer proposal's target and refuses
  it with the existing `stale` or `task_archived` codes through the same
  fenced refusal. It needs: a rolling cursor over `(created_at, id)` with a
  per-run cap (200 was drafted), a retry set for read errors that is visited
  first without starving the cursor, a grace rule that refuses an absent
  target only when it was also absent in the previous consecutive run (with
  the marker cleared by any intervening read error or present target), a
  five-second read bound, a three-minute freshness bound, and ACs for each.
- **Answer class in guided coordinator setup (deferred).** The guided setup
  flow neither shows nor stores the `answer` class: it leaves `answer` absent
  (so it resolves to `requires_approval`) and refuses a setup body that names
  `answer` with the field error `answer_unavailable`. A follow-up would add the
  setup May do row, flag-aware hiding, and storing `requires_approval`
  explicitly for a new coordinator.
- **One open answer proposal per target kind.** One open proposal per task is a
  limit of the existing open-target index; a task holding both a clarification
  and a permission request gets a second proposal only after the first is
  settled.
- **Activity-log display of answer rows (deferred).** The rows, reason codes
  and summary counts are recorded (REQ-COORDINATOR-ANSWER-006); showing them is
  not specified: the web activity log's copy for class `answer`, for
  `refused` proposal rows (reason `stale` or `task_archived`, distinct from a
  tool-call refusal) and for the `proposal_refused` count, the `answer` class
  filter option, and whether the activity summary hides `answer` while the
  flag is not effective. A follow-up needs those strings in all seven locales,
  the filter rule while the flag is off, and ACs for each.
