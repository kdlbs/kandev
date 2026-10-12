---
status: draft
system: tasks
created: 2026-09-04
owners:
  - kandev
---

# Workflow completion-signal payload delivery Requirements

## Overview

`step_complete_kandev` accepts `summary`, `handoff` and `blockers`; only `summary`
survives the transition it triggers, the orchestrator having lifted it onto the
audit row before deleting the signal bag. `tasks` owns the contract: the durable
artifacts are the session signal bag and the step-transition audit row.
Measured evidence, ADR citations, provenance, per-requirement framing, and the
code shapes behind every criterion are in the paired system design,
`docs/specs/tasks/system-design/workflow-completion-signal-payload-delivery.md`.

## Terminology

- **Completion signal:** the `PendingStepCompletionSignal` written to
  `TaskSession.Metadata` by `step_complete_kandev` or the manual fallback.
- **Completion handoff:** the `handoff` argument of a completion signal, and the
  value this capability delivers. Not the queued hand-off message.
- **Queued hand-off message:** a pre-existing, unrelated message-queue entry a
  `move_task_kandev` call can attach, already merged into a step entry prompt
  after the step's own content. Different provenance and lifetime; unchanged here.
- **Consuming transition:** an orchestrator-driven step transition that both runs
  on the turn-completion trigger AND carries the session lifecycle of a turn
  actually ending. The trigger value alone is not the test: guarded quorum-decision
  re-evaluation reuses the same turn-completion trigger without a turn having
  completed, and is excluded by the session-lifecycle half. Transitions on other
  triggers, including turn start and children completed, manual moves, approvals,
  and guarded quorum decisions, are not consuming transitions.
- **Consumed signal:** the completion signal a consuming transition finds for the
  step it is leaving, when it commits. A consuming transition may find none.
- **Transition audit record:** the `session_step_history` row a transition writes,
  whose `metadata` already carries `signal_source` and `signal_summary`.
- **Handoff carry token:** a single-slot, task-scoped record holding one consumed
  signal's completion handoff and the step it addresses. It is the `tasks.metadata`
  key `step_handoff_carry`, an object with `handoff` (the bounded text), `step_id`
  (the next step's identifier) and `stamp` (a fresh unique value per token write,
  which the claim compares).
- **Step entry:** one attempt to put a task to work on a step it has entered,
  from a dispatch path's start to a dispatched prompt or a decision not to
  dispatch. It may compose more than one prompt when a launch is replaced after
  a failure.
- **Step entry prompt:** the prompt composed for a step entry.
- **Next step:** a consuming transition's destination step.

## Prior art

The wiki leg ran and returned nothing useful; the vendor leg did not run, its
tool being unreachable. Neither informed this document; receipts and the
precedents followed are in the paired system design.

## Requirements

### REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-001: A completed step's completion handoff reaches the next step's agent exactly once

#### Acceptance criteria

- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.1:** When a consuming transition
  commits and its consumed signal carries a non-empty completion handoff, the
  system shall record a handoff carry token holding that handoff text and the
  next step's identifier.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.2:** When a consuming transition
  commits and its consumed signal carries an empty or whitespace-only completion
  handoff, or the transition found no signal, the system shall remove any
  existing handoff carry token for that task, so a handoff never survives a
  later consuming transition that wrote none.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.2a:** A transition that is not a
  consuming transition shall neither record nor remove a handoff carry token, so
  an unclaimed token survives any turn-start transition, children-completed
  transition, manual move, approval move, or guarded quorum decision occurring
  before the step is entered.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.3:** The system shall keep at most one
  handoff carry token per task. Recording a token shall replace any existing
  token rather than append to it, so no accumulation across steps is possible.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.4:** When a branch will send the agent
  something per -001.6c, the system shall claim the task's handoff carry
  token in a single atomic operation that removes and returns it ONLY IF the
  stored token names the step being entered, and otherwise removes nothing and
  returns nothing. The destination-step comparison shall be evaluated inside that
  one operation, against the stored value; any earlier read is advisory only, so a
  token replaced between that read and the claim is neither delivered nor
  destroyed. When two step entries race for one token, exactly one shall receive
  it and the other none.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.5:** The system shall deliver a handoff
  carry token only to an entry into the step it names. When the task's token names
  the step being entered, the system shall claim it per -001.4 and include its
  handoff text in the step entry prompt. When it names any other step, the system
  shall leave it in place unclaimed and compose the prompt with no handoff text,
  so a later entry into the step it does name can still receive it. The text
  delivered shall be the text the -001.4 claim returned, never a copy read before it.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.6:** The system shall append the
  completion handoff text LAST in the dispatched prompt: after the step entry
  prompt's own composed content and after any queued hand-off message merged into
  it. The text shall be introduced by the fixed heading line
  `## Context from the previous workflow step`, alone on its line, with one blank
  line before it and one after. The system shall not reorder, suppress, or
  otherwise change how a queued hand-off message is merged, and shall not change
  the prompt of an entry claiming no token.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.6a:** Every decision a dispatch branch
  makes about WHETHER to send the agent anything shall be evaluated on content
  that excludes the completion handoff text, using the rule that branch already
  applies today, so a completion handoff can never on its own cause a dispatch
  that would not otherwise happen. That decision shall precede the claim; claiming
  first and deciding afterwards violates this criterion even when the discarded
  prompt would have sent nothing.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.6b:** The system shall not apply
  saved-prompt reference expansion to the completion handoff text, and shall apply
  it to the entry prompt's own composed content exactly as today, so a completing
  agent cannot pull unrelated saved-prompt content into the next agent's prompt.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.6c:** When a branch decides to send the
  agent something, the system shall claim the token per -001.4 and append the
  handoff per -001.6. This includes a branch that finds its composed step prompt
  empty and dispatches only a drained queued message: that dispatch is a step
  entry and shall carry the handoff. When a branch sends the agent nothing at all,
  the system shall claim nothing and shall leave the token in place for a later
  entry into the step it names. Branches of one dispatch path may apply different
  rules; each shall follow its own rather than a unified one.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.7:** Every branch of every step-entry
  dispatch path that sends the agent anything shall evaluate the handoff carry
  token, so no branch can dispatch without the token having been considered, and
  shall do so through one shared claim-and-place implementation rather than logic
  duplicated per branch. Coverage is judged per branch, not per path: a path whose
  ACP branch claims and whose passthrough branch does not is a violation.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.8:** When one step entry composes more
  than one prompt because a launch was replaced after a failure, the system shall
  claim the token at most once per step entry — only a claim that returned text
  counts as that one claim — and shall include that text in every prompt composed
  from that claim onward, rather than re-claiming it per prompt. A prompt composed
  before any claim returned text carries none and does not violate this criterion.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.9:** After a step entry has claimed a
  token, a later entry into the same step shall receive no handoff text. The
  system shall not restore or re-record a claimed token, including when the entry
  that claimed it subsequently fails to dispatch.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.10:** When the next step's agent
  profile differs from the completing step's, so the destination step runs on a
  different session, the completion handoff shall still reach the prompt composed
  for that session.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.11:** When a consuming transition
  commits but the destination step's entry is deferred until a queue promotion
  admits the task, the handoff shall reach the prompt composed at that promotion.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.12:** When a manual move takes a task
  away from a step whose token was never claimed and later returns it to that
  same step, the system shall deliver that token on the return and shall add no
  time-based expiry to suppress it. Any consuming transition in between removes
  or replaces the token per -001.2 and -001.3.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.13:** The completion handoff and
  blockers a transition carries shall be those of the first completion signal
  accepted for that step. When a second `step_complete_kandev` call is rejected
  as a duplicate, its handoff and blockers shall not be recorded, delivered,
  merged, or allowed to overwrite the accepted signal's.

### REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-002: A signal's blockers are retrievable after the transition

#### Acceptance criteria

- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-002.1:** When a consuming transition writes
  its transition audit record, the system shall include the consumed signal's
  blockers under the key `signal_blockers` and its completion handoff under the
  key `signal_handoff`, alongside the `signal_source` and `signal_summary` it
  already writes.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-002.2:** The system shall omit an empty or
  whitespace-only blockers value, and an empty or whitespace-only completion
  handoff value, from the audit record rather than writing an empty entry.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-002.3:** The system shall capture the
  blockers and completion handoff into the audit record before the pending signal
  bag is cleared and before the carry token is recorded, so no ordering can lose
  them. Where a queued worker writes the row asynchronously, capture at enqueue
  satisfies this: the row may land later but shall carry the values the consuming
  transition read.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-002.4:** The system shall expose these
  values through the existing session step-history read path, adding no endpoint
  and changing its response shape only by the added metadata keys.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-002.5:** The system shall record blockers
  and completion handoff only for a transition that consumed a signal; one that
  consumed none shall continue to write no signal metadata.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-002.6:** The system shall store these
  values in the existing audit record metadata, adding no table and no column.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-002.7:** An audit record written before
  this capability shall remain readable and shall not be rewritten or migrated.
  A consumer shall treat `signal_blockers` and `signal_handoff` as
  absent-by-default.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-002.8:** The values recorded shall be those
  of the first signal accepted for the step, per -001.13. A duplicate signal
  shall neither append a second audit record nor amend the existing one.

### REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-003: Neither argument can grow without bound

#### Acceptance criteria

- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-003.1:** The system shall bound the stored
  completion handoff text and the stored blockers text to 8,192 bytes each,
  measured as UTF-8 byte length and inclusive of any truncation marker appended,
  and shall apply the bound where the completion signal is first recorded, so the
  signal bag, the audit record, and the carry token hold the same bounded value.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-003.2:** Bounding shall be measured and
  applied AFTER the handler's existing leading/trailing whitespace trim, on the
  trimmed value. When that trimmed value is at most 8,192 bytes the system shall
  store it byte-for-byte and append no marker. At 8,193 bytes or more it shall
  store a truncated value whose total length, marker included, is at most 8,192
  bytes.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-003.3:** When the system truncates a value
  it shall cut on a UTF-8 character boundary, never storing a partial character,
  and shall append the fixed marker `[truncated: over 8192-byte limit]` so a
  later reader cannot mistake a cut value for a complete one. The retained content
  shall be the longest whole-character prefix leaving room for that marker.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-003.4:** When the system bounds a value, it
  shall still accept the completion signal and shall drive the step transition
  exactly as it would for an unbounded value. An oversized completion handoff or
  blockers value shall never strand a step.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-003.5:** When the system truncates one or
  more values on a call it accepts, that call's `step_complete_kandev` result
  shall carry `truncated`, a JSON array of the lowercase argument names that were
  truncated, and `truncation_limit_bytes`, a JSON number holding the ceiling. The
  array shall list `"handoff"` before `"blockers"` when both were truncated,
  contain exactly one entry when one was, and never repeat a name. When nothing
  was truncated the system shall omit both fields, so a result omitting them means
  every submitted value was stored whole.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-003.5a:** When a call is rejected as a
  duplicate for the step, its result shall keep its existing rejection shape and
  report no truncation, none of that call's values having been stored. Bounding
  shall never convert a rejected duplicate into an accepted signal.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-003.6:** The ceiling shall be a fixed value
  compiled into the backend, identical across deployments, and readable or
  writable from no configuration, environment, or runtime feature toggle.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-003.7:** The system shall bound each value
  independently per signal; the ceiling shall not accumulate across signals,
  steps, or a task's transition history.

### REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-004: The tool describes what it actually does

#### Acceptance criteria

- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-004.1:** The registered description of the
  `handoff` argument shall state that it is delivered once to the
  immediately-following step's agent and is not carried beyond that step.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-004.2:** The registered description of the
  `blockers` argument shall state that it is recorded on the step-transition
  record and not delivered to the next step's agent, so it is not used to pass
  context forward.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-004.3:** The registered descriptions of
  both arguments shall state the byte ceiling from
  REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-003.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-004.4:** The tool's top-level description
  shall claim no forwarding behavior the delivered mechanism does not provide.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-004.5:** The system shall keep both
  arguments registered and optional, with their existing names and types.

### REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-005: Delivery never costs a transition

#### Acceptance criteria

- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-005.1:** When recording or removing a carry
  token fails, the system shall log it and complete the transition unchanged,
  without retrying, rolling back, or deferring it.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-005.2:** When claiming a carry token fails,
  the system shall compose the step entry prompt exactly as today, with no
  handoff text, and shall not fail the step entry. A failed claim removed and
  returned nothing, so it shall not count as the step entry's one claim under
  -001.8: a replacement launch in the same step entry, and any later entry into
  the same step, shall attempt the claim again. The system shall not retry within
  one composed prompt.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-005.3:** When the configured repository
  provides no atomic task-metadata write or claim operation, the system shall
  behave as today: no token recorded, none claimed, no handoff text, and no error
  surfaced to the transition or the step entry.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-005.4:** When two consuming transitions for
  the same task resolve concurrently, the last token write shall win — the
  single-row overwrite's own commit order decides which is last, and no version or
  sequence column is added — and neither outcome shall deliver a handoff to a step
  the winning token does not name, because -001.4 compares the destination step
  inside the same atomic operation that removes it.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-005.5:** When a duplicate delivery re-runs
  a consuming transition that already recorded its token, re-recording the same
  handoff and destination step shall be indistinguishable from recording them
  once in what is DELIVERED: at most one entry into that step receives that
  handoff. The stamp is minted fresh per write, so the stored tokens differ at
  that field; that is expected, and the stamp shall never be made stable or
  content-derived to satisfy this criterion.
- **AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-005.6:** A task with no handoff carry token
  shall compose step entry prompts byte-identically to today.

## Out of scope

- **Attention routing for blockers.** Making them retrievable is this capability.
  Surfacing them on a board, in chat, or in a notification is excluded: no
  endpoint, no event, no frontend change, no copy, and therefore no localization.
- **Rendering or collapsing the handoff block in chat.** It is agent-facing
  English composed into the prompt, not localized, the same class as the
  workflow-instructions heading. No frontend affordance.
- **Changing the queued hand-off message.** Its provenance, merge position, and
  requeue-on-failure behavior are untouched. AC-001.6 states only where the
  completion handoff sits.
- **Unifying the actionability rules.** Dispatch branches apply different rules
  today; the design records which. AC-001.6a defers to a branch's own rule rather
  than reconciling them.
- **Ordering of the audit read path.** `ListHistoryBySession` orders by
  `created_at` with no tiebreak column, and this capability changes neither; two
  rows sharing a `created_at` stay in the order the driver returns them.
- **Putting handoff or blockers on the bus-event wire.** ADR 0015 kept both off
  the `workflow.step_completion_signaled` payload. Delivery happens at transition
  time from the persisted bag, not through the event.
- **Multi-hop retention.** Nothing accumulates handoffs across steps. Multi-hop
  receipts remain the task plan's job; its three tools are unchanged.
- **Restructuring the completion payload into typed evidence fields.** The three
  arguments keep their names, types, and optionality; a structured evidence bundle
  is a separate item.
- **The manual completion fallback button.** Its source value is covered by every
  rule above, but the button remains unimplemented and out of scope here, per
  `REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001`.
- **Amending ADR 0015.** Updating its text is a documentation change, not a
  contract change owned here.
