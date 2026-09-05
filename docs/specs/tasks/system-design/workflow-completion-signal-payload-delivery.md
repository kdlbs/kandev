---
status: draft
system: tasks
requirements:
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-001
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-002
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-003
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-004
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-005
created: 2026-09-05
owners:
  - kandev
---

# Workflow completion-signal payload delivery System Design

## Purpose and boundaries

The paired requirement is the contract. This document records the measured
current-tree shapes those criteria were written against, the mechanism chosen for
each, and the prior art followed rather than re-derived. Where the two disagree,
the requirement wins and this document is wrong.

Line numbers are omitted: they drifted between sessions of this card. Every element
below is named by symbol, so a builder greps for it.

## Provenance

**Why this exists.** ADR 0015 records the mechanism accurately at its line 42
("`handoff` and `blockers` are persisted in the bag only, not on the wire") but
justifies it at line 114 on the opposite outcome ("the summary is cheap, the
next-step agent benefits from a handoff blurb"). The mechanism never delivered
the rationale. This capability closes that gap; it does not reverse the ADR.
Amending the ADR's text is a documentation change, out of scope for the contract.

**Measured cost of the defect.** Both shipped boards tell every agent to fill
`handoff` on every step while also declaring the task plan IS the handoff, a
workaround that exists because the argument does not work. One card made 26 step
transitions on 2026-09-03/04; every handoff it wrote was discarded.

**Prior-art leg receipts.** The wiki leg RAN and returned nothing useful:
`OBSIDIAN_VAULT_PATH` = `/Users/henry/Documents/henry/wiki` from
`~/.obsidian-wiki/config.henry` (pinned, not the mutable `config` symlink), QMD
collection `wiki`; all three transports were unavailable (`obsidian-wiki` not
installed, `qmd` not on `PATH`, `~/Documents` reads returning
`Operation not permitted` under this process's macOS privacy scope). The vendor leg
DID NOT RUN: `saas-kb` / `search_fsm_docs` are not registered in this session, so
`ai_sdlc` was not queried. Neither informed the requirement.

## Measured input

Re-sampled against the working tree on 2026-09-05, and re-verified during spec
review. Every claim here was read from the code, not inferred.

### The lifecycle today

1. `step_complete_kandev` is registered in `internal/mcp/server`'s
   `registerStepCompleteTool`, with `summary` required and `handoff` and
   `blockers` optional.
2. `internal/mcp/handlers`'s `handleStepComplete` trims all three and builds a
   `models.PendingStepCompletionSignal`, then calls `claimStepCompletionSignal`.
3. That claim is `SetSessionMetadataKeyIfAbsentOrDifferentStep[IfTaskAtStep]`,
   so a second call at the same step returns `stored=false` and the first signal
   stands. This is the mechanism behind AC-001.13 and AC-003.5a: the duplicate
   path runs `handleDuplicateStepComplete`, which re-publishes the event and
   returns `{accepted:false, reason:"already_signaled"}`. The second call's
   `handoff` and `blockers` are already discarded today, and stay discarded.
4. Two consuming-transition sites exist in `internal/orchestrator`, both in
   `event_handlers_workflow.go`: `executeStepTransition` (legacy
   on_turn_complete) and **`applyEngineTransitionWithCommitMode`** (engine path).
   Grep for that full name: `applyEngineTransition`,
   `applyEngineTransitionWithCommit` and `applyEngineTransitionWithMode` are thin
   wrappers that contain none of the consuming logic. Each site builds
   `consumedSignal` by `models.LoadPendingStepSignal`, calls
   `recordAutoStepTransition`, then clears the bag. Both must be changed, or the
   two funnels drift.
5. `recordAutoStepTransition` in `event_handlers_step_completion.go` builds
   `map[string]interface{}{"signal_source", "signal_summary"}`. This is the
   single place the payload is narrowed, and the single place REQ-002 widens. It
   leaves `metadata` nil when the signal is nil, which is what makes AC-002.5
   hold with no added branch.
6. `clearPendingStepSignal` / `clearPendingStepSignalByID` delete the bag entry.
   This is where `handoff` and `blockers` die today.

### Trigger gating: the asymmetry AC-001.2a exists for

`recordAutoStepTransition` is called **unconditionally on every trigger** at both
sites, with `consumedSignal` nil for anything that is not turn completion.
The bag clear, by contrast, **is** trigger-gated: `if triggerOnEnter` in
`executeStepTransition`, and `if sessionLifecycle && trigger ==
engine.TriggerOnTurnComplete` in `applyEngineTransitionWithCommitMode`.

Placing the carry-token write beside `recordAutoStepTransition` without adopting
that gate would let a turn-start or children-completed transition satisfy
AC-001.2's "found no signal" clause and erase a token written moments earlier by
the consuming transition that delivered the task into the step. The token write
and removal therefore take the **bag clear's gate**, not the audit row's.

**The trigger value alone is not the gate, and this is the trap.**
`applyFirstSatisfiedGuardedTransition` in `internal/workflow/engine/quorum.go`
drives a guarded quorum/approval decision through
`ApplyTransitionIfAtStep(..., TriggerOnTurnComplete)` — the SAME trigger constant a
real turn completion uses. (`TriggerOnApprovalResolved` exists in `engine/types.go`
but is not what this path passes.) So a reviewer clicking approve arrives carrying
`TriggerOnTurnComplete` with no turn having ended, on the DECIDER's session rather
than the assignee's. What excludes it is the other half of the condition:
`sessionLifecycle := mode != transitionLifecycleGuardedDecision` in
`applyEngineTransitionWithCommitMode`, which is why the gate is the compound
`sessionLifecycle && trigger == engine.TriggerOnTurnComplete` and not the trigger
test alone. A builder implementing AC-001.2 from a trigger check alone fires the
token write/remove on every quorum-gated approval, and AC-001.2's "found no
signal, so remove" branch then wipes a pending, unclaimed token the instant a
reviewer approves. Copy the compound condition; do not paraphrase it.

### The step-entry surface

- **`buildWorkflowEntryPrompt`** (`internal/orchestrator/task_operations.go`) is
  the single prompt-**composition** seam, at five call sites: four in
  `event_handlers_workflow.go` (the `launchAfterOnEnterDispatch` branches,
  including three replacement launches after a failed or terminalized launch) and
  one in `task_operations.go` inside `StartSessionForWorkflowStep` (queue
  promotion and manual auto-start).
- **Composition is not dispatch.** There are **two step-entry dispatch paths**:
  `launchAfterOnEnterDispatch` and `StartSessionForWorkflowStep`. The five seam
  call sites sit inside those two. But the path is NOT the unit of coverage:
  `launchAfterOnEnterDispatch` forks into an ACP branch (`autoStartStepPrompt`) and
  a passthrough branch, and those two decide differently on identical input. AC-001.7
  therefore judges coverage per branch, which is why "I changed both paths" is not
  evidence that every dispatch was covered.
- **Actionability is decided after the seam returns, by three different rules.**
  `promptForEmptinessCheck` covers one call site, not all three.
  - `StartSessionForWorkflowStep`: `promptForEmptinessCheck :=
    s.effectivePromptForSession(...)`, then `strings.TrimSpace(...) == ""` returns
    without dispatching.
  - `autoStartStepPrompt`: its own `effectiveAgentPrompt` check, which returns
    early only when the prompt is blank **and** `len(attachments) == 0`, so an
    attachment-only entry is actionable there and nowhere else.
  - the passthrough branch of `launchAfterOnEnterDispatch`: a bare
    `strings.TrimSpace(effectivePrompt) == ""`, which then calls
    `drainQueuedMessageForPromptableSession` and returns. **That branch still
    DISPATCHES** — it sends the drained queued message to the agent. It is not a
    "nothing happened" path, which is why AC-001.6c names it explicitly and treats
    it as a step entry that carries the handoff.
  Note these are rules of BRANCHES, not paths: `autoStartStepPrompt` and the
  passthrough branch both sit inside `launchAfterOnEnterDispatch` and disagree on
  the same input (empty composed prompt plus a non-empty queued message). AC-001.6a
  defers to the branch's own rule; unification is a named exclusion.
- **A queued hand-off message already merges into this prompt.**
  `autoStartStepPrompt` calls `takeAndMergeHandoffMessage`, which drains an
  Auto-run-enabled queued message (typically a `move_task_kandev` hand-off
  prompt) and merges it with documented ordering "auto-start content first,
  hand-off after", forwarding attachments and restoring the message on terminal
  failure via `requeueTaken`. This is a different concept from the completion
  handoff, with different provenance and lifetime, and the requirement names both
  so a builder cannot conflate them. **This is also why AC-001.6 places the
  completion handoff LAST, after the queued hand-off.**
  `takeAndMergeHandoffMessage` runs BEFORE `autoStartStepPrompt`'s actionability
  test and concatenates in one shot (`basePrompt + "\n\n" + msg.Content`) with no
  splice point. Placing the completion handoff between the composed content and the
  queued hand-off would force the claim before the actionability decision, violating
  AC-001.6a, or restructure how the queued hand-off merges, which AC-001.6 forbids.
  Appending last satisfies both and is reachable on every branch: after the
  actionability test, before dispatch.
- **Saved-prompt reference expansion runs over the joined composition.**
  `buildWorkflowPromptWithTrustedContext` ends in
  `expandPromptReferencesWithContext(ctx, joined, isPassthrough)`. Appending the
  completion handoff before that call would expand `@name` references inside
  agent-authored text; AC-001.6b forbids it for that reason.

### Shapes the mechanism relies on

- **`session_step_history.metadata`** is a nullable `TEXT` column
  (`internal/workflow/repository/sqlite.go`) holding free-form JSON. Adding keys
  needs no migration, satisfying AC-002.6.
- **The read path already ships.** `GET /sessions/:id/workflow/history`
  (`internal/workflow/handlers`) plus a WS action reach
  `Controller.ListHistoryBySession`, whose `models.SessionStepHistory` carries
  `Metadata map[string]interface{}` with `json:"metadata,omitempty"`. Added map
  keys appear automatically, satisfying AC-002.4 with no endpoint and no shape
  change, and `omitempty` gives AC-002.7's absent-by-default for free.
  `ListHistoryBySession` is `ORDER BY created_at` with no tiebreak column; the
  requirement names that as an exclusion rather than changing it.
- **Recording is asynchronous in production.** `recordAutoStepTransition` prefers
  `asyncStepHistoryRecorder.EnqueueStepTransition`; only test doubles take the
  synchronous `CreateStepTransition` branch. The metadata map is built from the
  signal's fields before the enqueue, so values are captured at enqueue time even
  though the row lands later. Hence AC-002.3 says *capture* before the clear, not
  *write* before it.
- **`tasks.metadata`** is an existing JSON bag with concurrent-key-safe patch
  helpers on the SQLite repository: `SetTaskMetadataKey` (a plain overwrite, no
  CAS, which is what makes AC-005.4's last-write-wins the primitive's actual
  semantics) and `RemoveTaskMetadataKey`.
- **`RemoveTaskMetadataKey` cannot serve as the claim on its own.** Its signature
  is `(ctx, taskID, key string) (bool, error)`: the boolean reports *presence*,
  and the removed **value is not returned**. AC-001.4 requires one atomic
  operation that both returns the content and removes it, so this capability adds
  one. See "The claim primitive" below.
- **`internal/common/truncate`** already exposes `UTF8(s string, maxBytes int)`,
  which cuts on a rune boundary. It appends no marker, so REQ-003's marker
  handling wraps it rather than replacing it.

## Mechanism

### The claim primitive (new)

A new repository method beside `RemoveTaskMetadataKey` in
`internal/task/repository/sqlite/task.go`, returning the removed value as well as
whether it was present, so read-and-remove is one claim:

    TakeTaskMetadataKeyIfDestinationStep(
        ctx, taskID, key, expectedStepID, expectedStamp string,
    ) (json.RawMessage, bool, error)

**Why a plain take plus a pre-check is not equivalent.** `SetTaskMetadataKey` is an
unconditional overwrite, so a concurrent consuming transition can replace the token
between a check and an unconditional take, after which the take returns a token
addressed to a different step. Delivering that breaks AC-001.5; discarding it destroys
a token AC-001.5 requires be left in place, and AC-001.9 forbids putting it back.

**`RemoveTaskMetadataKeyIfStamp` supplies the conditional-compare half only, NOT the
return value.** It reports presence through `RowsAffected` and does not return what it
removed, and no single statement can: neither SQLite's nor this repository's Postgres
`RETURNING` can read a pre-`UPDATE` value. Every `UPDATE ... RETURNING` over a JSON
`metadata` column in this tree — for example the compaction-count increment in
`internal/task/repository/sqlite/session.go` — returns the POST-update value, which
after a removal is null. "Return the removed value" is therefore NOT a one-line
extension of that precedent, and a builder must not plan for it as one.

**The mechanism: compare-and-swap on the token's stamp.** The write side mints a fresh
`stamp` into every token (see "Handoff carriage"). The claim is an advisory read
followed by ONE conditional `UPDATE` whose `WHERE` compares BOTH nested fields
(paths built from `key`, as the precedent builds its own; shown here concrete):

    UPDATE tasks SET metadata = json_remove(metadata, '$.step_handoff_carry')
    WHERE id = ?
      AND json_extract(metadata, '$.step_handoff_carry.step_id') = ?
      AND json_extract(metadata, '$.step_handoff_carry.stamp')   = ?

`RowsAffected == 1` means the row removed WAS the row read, because the stamp it is
keyed on is unique per token write. The method returns the handoff text from the
advisory read, and that text is provably the removed one — which is exactly what
AC-001.5's "never a copy read before it" protects against, and what AC-001.4 means by
an earlier read being advisory only. `RowsAffected == 0` means the token was replaced
or names another step: remove nothing, return nothing, compose with no handoff text,
and leave whatever is there for a later entry into the step it names.

The destination-step comparison sits in the `WHERE` clause rather than in Go, so
AC-001.4's "evaluated inside that one operation" holds literally. The stamp is what
makes the claim safe with NO isolation-level or transaction assumption: no `BeginTxx`,
no `SELECT ... FOR UPDATE`, no `BEGIN IMMEDIATE`, and no dependence on SQLite's
deferred-transaction upgrade behavior. Mirror `RemoveTaskMetadataKeyIfStamp`'s
dual-dialect body — it already carries the Postgres `jsonb_extract_path_text` / `#-`
form beside the SQLite `json_extract` / `json_remove` form — rather than adding a
SQLite-only method to a file whose every neighbour handles both.

The new method ships with the matching interface assertion added where
`RemoveTaskMetadataKey` is already declared: the narrow interface in
`internal/orchestrator/event_handlers_workflow.go` and the repository interface
in `internal/orchestrator/service.go`. AC-005.3's degrade-to-today branch mirrors
the existing `asyncStepHistoryRecorder` and `stepCompletionSignalClaimer`
optional-interface assertions: a repository that lacks the method takes today's
behavior rather than erroring.

### Handoff carriage (REQ-001)

A **handoff carry token** on `tasks.metadata` under the key `step_handoff_carry`,
declared as a `MetaKey...` constant beside `MetaKeyQueuePromotionPending` and its ~25
neighbours in `internal/task/models/models.go`. Naming it there is not cosmetic:
`Task.Metadata` is `map[string]interface{}` with `json:"metadata,omitempty"`, so the
key ships to the frontend in every task payload, and an unregistered name can silently
collide with another feature's one-shot token. Its value is a JSON object with exactly
three fields:

- `handoff` — the bounded completion handoff text (REQ-003 bounds it, so a token
  holds at most 8,192 bytes and the task payload grows by that much until claimed);
- `step_id` — the next step's identifier, which the claim compares;
- `stamp` — a fresh unique value minted on EVERY token write (`uuid.NewString()`;
  `workflow_profile_session_lifecycle.go` already sets a `Stamp` that way). It must NOT
  be content-derived: `models.StableLaunchErrorStamp` is a SHA-256 of its inputs, so two
  successive tokens carrying the same handoff would share a stamp and the claim's
  compare-and-swap would stop telling them apart. See "The claim primitive".

Single-slot by construction, since `SetTaskMetadataKey` overwrites, which is how
AC-001.3 forbids accumulation without a length check.

**Written** at both consuming-transition sites, under the same trigger gate the
bag clear uses (see "Trigger gating" above), next to `recordAutoStepTransition`
and before the bag clear, so AC-002.3's ordering falls out of placement rather
than a lock. A consumed signal with a blank handoff, or a consuming transition
that found no signal, removes the key instead of writing it (AC-001.2). A
non-consuming transition touches neither (AC-001.2a).

**Claimed** once per step entry, through one shared helper rather than logic
duplicated per branch (AC-001.7). The order is fixed by AC-001.6a, AC-001.6c and
AC-001.4:

1. compose the step entry prompt through `buildWorkflowEntryPrompt`, with no
   handoff text, and let the branch merge any queued hand-off message exactly as
   it does today;
2. apply that branch's existing rule for whether it will send the agent anything,
   over content that excludes the handoff text (AC-001.6a);
3. if it will send nothing at all, stop: claim nothing, leave the token in place
   (AC-001.6c). If it will send something — INCLUDING the passthrough branch that
   found its composed prompt empty and is dispatching only a drained queued
   message — continue;
4. read the token (advisory); if one is there, call
   `TakeTaskMetadataKeyIfDestinationStep` with the step being entered AND the stamp
   just read. If the read finds nothing, claim nothing. A token naming another step
   is left untouched: AC-001.12 requires it still be deliverable when a manual move
   later returns the task to that step. See "The claim primitive" for why this
   settles both races (AC-001.4, AC-001.5, AC-005.4);
5. append the returned text LAST in the prompt, under the fixed heading
   `## Context from the previous workflow step` (AC-001.6), without running
   reference expansion over it (AC-001.6b). Appending after the composition seam
   is what gives AC-001.6b for free: `expandPromptReferencesWithContext` runs
   inside `buildWorkflowPromptWithTrustedContext`, which is upstream of here;
6. dispatch.

Steps 2 and 3 are per BRANCH, not per path: `launchAfterOnEnterDispatch` contains
two branches whose rules disagree (see "Actionability"). The shared helper takes the
branch's own send-or-not answer as an argument rather than re-deriving it.

A token addressed to a step that is never re-entered is not leaked: it is
single-slot, and the next consuming transition replaces it (AC-001.3) or removes
it (AC-001.2).

Composing first and claiming second is what makes AC-001.6a satisfiable: the
send-or-not rules live downstream of the composition seam, so a claim inside the
seam would necessarily precede the decision. Hence AC-001.7 is a coverage guarantee
over branches plus one shared implementation, not "claim inside
`buildWorkflowEntryPrompt`".

The claimed text is held by the dispatch path for the remainder of that step entry,
which lets a replacement launch reuse it (AC-001.8) even though the replacement
re-enters the composition seam with a fresh session. The seam's own
`ClaimInitialPromptFallback` treats a replacement as a fresh prompt boundary; the
carry token deliberately does not, so that precedent is noted to stop a builder
copying it.

Task-keyed, not session-keyed, because a transition may switch the task onto a
different session for the destination step's profile; a session-keyed token would
be written on the source session and read by nobody. That is what makes AC-001.10
and AC-001.11 hold rather than being separate code paths.

### Blockers retrievability (REQ-002)

Two more keys on the map `recordAutoStepTransition` already builds, named
`signal_blockers` and `signal_handoff` to match the `signal_source` /
`signal_summary` literals there, omitted when blank (AC-002.2). No table, no
column, no endpoint. The handoff is recorded here too, so the audit row states what
was carried as well as what was blocked.

These two names are a wire contract, not an internal detail: they surface
verbatim in the `metadata` map of `GET /sessions/:id/workflow/history`. Update the
doc comment on `CreateStepTransition` in `internal/workflow/service/service.go`,
which currently documents the shape as `{"signal_source", "signal_summary"}`, so
the documented shape and the written shape stay in step.

### Bounding (REQ-003)

Applied in `handleStepComplete`, where the signal is first constructed and
already `strings.TrimSpace`-normalised, upstream of the bag, the audit row and
the carry token, so all three hold the same bounded value from one enforcement
point. A fixed compiled-in constant, following
`REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-001`'s pattern.

Order matters and AC-003.2 pins it: TRIM FIRST, then measure. So "stored
byte-for-byte" in AC-003.2 means byte-for-byte against the TRIMMED value, not the
raw argument.

The ceiling is inclusive of the marker (AC-003.1), so the algorithm is: if the
trimmed value is at most 8,192 bytes, store it as-is; otherwise `truncate.UTF8` the
content to `8192 - len(marker)` bytes, which already cuts on a rune boundary, then
append the marker, giving a stored value of at most 8,192 bytes. The marker
`[truncated: over 8192-byte limit]` is 33 bytes, so the retained prefix is at most
8,159 bytes.

Bounding never rejects (AC-003.4): unlike the plan-size limit, refusing here
would strand a task on a step whose work is finished. The truncation report rides
the accepted result only: `truncated` is a JSON array of lowercase argument names
in the fixed order `["handoff", "blockers"]`, never a bare string and never a map,
alongside `truncation_limit_bytes` as a JSON number (AC-003.5). Both are omitted
entirely when nothing was truncated, so their absence is the signal that every
value was stored whole. `handleStepComplete` already returns a
`map[string]interface{}` that the MCP layer's `stepCompleteHandler` marshals
straight through, so the two fields reach the agent with no extra plumbing. The
duplicate path's rejection shape is untouched (AC-003.5a) because none of that
call's values were stored — note that bounding happens at signal construction,
which is upstream of the duplicate check, so a rejected duplicate must report no
truncation even though it computed one.

### Tool description (REQ-004)

Text-only change at the registration site. Non-localized: the string is sent to
the model, the same class as `workflowInstructionsHeading`, which the codebase
already documents as deliberately not i18n'd. No locale catalogs are touched, so
the i18n gate does not apply. Same for the fixed handoff heading in the prompt.

### Failure posture (REQ-005)

Every added write is best-effort, matching the contract the audit row already
has: `recordAutoStepTransition` logs and swallows, and this capability's writes
sit beside it under the same rule.

**A failed claim is not a spent claim, and that asymmetry with AC-005.1 is deliberate.**
AC-005.1 forbids retrying a failed token WRITE, because the transition it belongs to has
already committed and a retry would race the next one. The claim is the opposite case: a
claim that errors removed nothing and returned nothing, so the token is still there,
still addressed to this step, and a later attempt is free. AC-005.2 therefore does not
spend AC-001.8's one-claim-per-step-entry budget on a failure — a replacement launch in
the same step entry attempts the claim again, and so does a later entry into the same
step. What AC-005.2 does forbid is a retry loop INSIDE one composed prompt: one attempt
per composed prompt, and a failure composes with no handoff text. The claim's
compare-and-swap is what makes re-attempting safe rather than double-delivering.

**AC-005.5's re-run is not a delivery hazard.** `reconcileStepCompletionSignalLocked`
returns early on `task.WorkflowStepID != stepID`, so a replayed signal cannot re-run a
transition that already committed: the task is at the destination step by then and the
replay names the source. A re-record therefore lands only before any entry into that
step could have claimed, so AC-005.5 needs no double-delivery guard.

## Requirement framing
Each requirement's intent and user story, held here so the paired requirements
document stays inside its ceiling. These frame the contract; they are not
themselves acceptance criteria.

### REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-001

*A completed step's completion handoff reaches the next step's agent exactly once*

**Intent:** Deliver what `handoff` already promises, on every path a step entry can
take, and stop there. Per-step context that is never dropped becomes a document
every later step pays to read.

**User story:** As an agent starting a workflow step, I want the previous step's
closing context in my first prompt, so I do not reconstruct it from a record that
grows on every hop.

### REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-002

*A signal's blockers are retrievable after the transition*

**Intent:** Stop destroying the field. Its cheapest durable home is the audit
record already carrying the signal's summary and source, on an endpoint that
already ships.

**User story:** As someone reconstructing why a task moved, I want the blockers
the completing agent reported, so a stated unresolved issue survives the step
advancing.

### REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-003

*Neither argument can grow without bound*

**Intent:** One hop of context is the deliverable. An oversized signal must still
advance the step: refusing it would strand a task on a step whose work is done.

**User story:** As an operator, I want a fixed ceiling on what one signal carries
forward, so delivering a handoff cannot reproduce the growth that made the running
record expensive to read.

### REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-004

*The tool describes what it actually does*

**Intent:** The description must state delivery's exact extent: an agent believing
`handoff` persists across steps will use it as a running record.

**User story:** As an agent choosing what to put in each argument, I want the tool
to state where each lands and how long it lives, so I do not put multi-step
bookkeeping into a one-hop field.

### REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-005

*Delivery never costs a transition*

**Intent:** Every added write is best-effort work on an already-committed
transition, matching the audit row's contract. A failed carry is a lost blurb; one
that rolls back or blocks a transition is a stuck card.

**User story:** As a user whose card is mid-transition, I want a carriage failure
to cost me the handoff and nothing else, so a step never stops advancing because a
context blurb could not be stored.

## Prior art followed

- **ADR 0015** widened `session_step_history.metadata` rather than adding a
  table, explicitly rejecting a dedicated signal table as overkill. REQ-002 adds
  keys to that same map for the same data, applying the decision rather than
  reopening it.
- **`MetaKeyQueuePromotionPending` / `MetaKeyAutoStartOnCreate`** establish the
  one-shot task-metadata token claimed by whichever delivery wins the atomic
  remove. The carry token is that shape plus the step and stamp fields.
  **Copy the claim, NOT the restore.** `restoreTaskLifecycleToken` puts a
  queue-promotion token BACK at six failure/recovery sites and is generic over
  `key`, so aiming it at `step_handoff_carry` is a one-line change made by
  symmetry. AC-001.9 forbids it: a claimed token is never restored, even when the
  entry that claimed it then fails to dispatch. That handoff is lost; REQ-005
  accepts that.
- **`REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-001`** supplies the bounding pattern: one
  fixed ceiling, compiled in, not configurable, enforced at a shared seam.

**What differs.** Those precedents claim a value that already had a reader, and
their claim needs only "was it there". This capability adds the reader, which is the
whole defect, and needs the VALUE back — which is why it adds a new primitive rather
than reusing `RemoveTaskMetadataKey`, and why that primitive is a compare-and-swap
rather than the plain conditional remove `RemoveTaskMetadataKeyIfStamp` performs.

## Testing notes

The two consuming-transition sites are the drift risk: a change applied to only
one leaves the other silently discarding handoffs. Cover both
`executeStepTransition` and `applyEngineTransitionWithCommitMode`, and cover a
non-consuming trigger at each to prove AC-001.2a's gate.

**Cover the guarded-decision carve-out explicitly.** A test that only varies the
trigger cannot catch F15's failure mode, because the guarded path carries
`TriggerOnTurnComplete` too. Drive a guarded quorum decision
(`transitionLifecycleGuardedDecision`) against a task holding an unclaimed carry
token and assert the token is STILL THERE afterwards. Without this test, an
implementation gated on the trigger alone passes every other test in this list.

The dispatch BRANCHES are the second drift risk, and there are more of them than
there are paths. Cover `launchAfterOnEnterDispatch` twice — the ACP branch via
`autoStartStepPrompt` (including a replacement launch, for AC-001.8) and the
passthrough branch that dispatches only a drained queued message (AC-001.6c) — plus
`StartSessionForWorkflowStep` (for AC-001.11's queue promotion). Assert the handoff
lands LAST, after any queued hand-off content, on the branch where both are present.

**The claim race needs a test that fails against a pre-check implementation.**
Between reading a token for step A and claiming it, overwrite the key with a token
for step B, then run the claim. Assert the entry into A receives no handoff AND the
step-B token is still present. An unconditional take plus a pre-check passes a naive
single-threaded test and fails this one, which is the whole point.

Add the same-step variant, which the step comparison alone cannot catch: replace the
token with a DIFFERENT token also naming step A, then claim. Assert the entry receives
nothing and the replacement is still present. An implementation that compares only
`step_id` removes the replacement and delivers the stale text, passing the step-B test
above; only the stamp compare-and-swap fails it.

Cover a failed claim followed by a replacement launch in the same step entry: assert the
second attempt runs and delivers, per the failure-posture rule that a failed claim does
not spend the entry's one claim.

Also assert an entry that sends the agent nothing claims nothing (AC-001.6a), so a
handoff can never be consumed by a dispatch that did not happen.

Assert a claimed token is NOT restored when the claiming entry then fails to dispatch
(AC-001.9): claim, fail the launch terminally, then assert `tasks.metadata` has no
`step_handoff_carry` and a later entry into that step gets no handoff. Without it the
`restoreTaskLifecycleToken` symmetry ships silently.

The async recorder means an assertion on a durable row can race. Assert on the
metadata map handed to the recorder (the test double takes the synchronous
branch), which is also the value AC-002.3 pins, and assert the exact key names
`signal_blockers` / `signal_handoff` rather than just presence.

AC-003.2's boundary pair (8,192 stored whole, 8,193 truncated) is measured AFTER
the trim, so include a case padded with whitespace that is oversized before
trimming and in-bounds after: it must store whole and report no truncation.
AC-003.3's marker-inside-the-ceiling rule needs a multi-byte-character case so a
cut landing mid-rune is caught. Assert `truncated` is an array in the order
`["handoff", "blockers"]` when both truncate, and that both fields are absent when
neither does.
