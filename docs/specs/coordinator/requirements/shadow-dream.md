---
id: coordinator-shadow-dream
title: Coordinator shadow dream
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
---

# Coordinator shadow dream Requirements

## Overview

The dream is a periodic, bounded episode in which the coordinator reviews its
own recorded turns, outcomes and overrides and suggests changes to its own
instructions and standing orders. Phase 3.1 runs it end to end in Shadow: the
same trigger, collection, extraction, gate and replay as the later Active
dream, with the same output schema, but the episode has no tool that writes and
its output is a report, not a card. Nothing it suggests is stored anywhere a
turn reads. A manager rates each suggestion, and those ratings, set against
the replay's verdict on the same suggestions, are the evidence that decides
whether phase 3.5 is built. The dream is a system-defined wake source: it
starts only from the trigger below, under phase 3's admission, containment and
spend ceiling.

## Terminology

- **Shadow dream:** one dream episode in Shadow, with its report.
- **Evidence debt:** completed turns since the last accepted dream.
- **Accepted dream:** a dream whose review-result row ended `clean`, `ok` or
  `partial`.
- **Window:** the interval from the previous accepted dream's window end (or,
  for the first, 30 days before now or the first ledger row if later) to the
  moment the episode starts. It is stored before the agent runs and never
  changes.
- **Review-result row:** the stored record of one dream: its window, the turns
  read, its items and its status.
- **Item:** one suggestion in a report: a `note_add`, `note_update`,
  `note_retire`, `context_diff`, `standing_order_add` or
  `standing_order_retire`, with the turns it cites.
- **Gate:** the deterministic checks each item passes or is refused by.
- **Considered, not proposed:** the agent's own list of things it weighed and
  left out; agent-reported, not verified.
- **Health state:** `off`, `waiting`, `running`, `fresh`, `stale` or `failed`.
- **Learning section:** the coordinator settings section that shows all of
  this.
- Other terms are defined in [turn ledger](turn-ledger.md#terminology),
  [outcomes](outcomes.md#terminology) and [replay](replay.md#terminology).

## Requirements

### REQ-COORDINATOR-SHADOW-DREAM-001: Trigger, window and one at a time

**Intent:** The dream runs when there is something to learn from, once, and
never twice at the same time.

#### Acceptance criteria

- **AC-COORDINATOR-SHADOW-DREAM-001.1:** While the phase 3.1 flag is
  effective, a coordinator has autonomy on, a manager has enabled its Shadow
  dream (off by default), it is not paused, and the window holds at least 5
  completed turns that are not dreams and at least one decided proposal or
  override, and at least 24 hours have passed since the last dream episode
  started, the system shall start at most one dream. It shall check this on
  every wake backstop pass.
- **AC-COORDINATOR-SHADOW-DREAM-001.2:** Before starting the agent the system
  shall store the review-result row with status `running`, the window and the
  input hash, and shall start no second dream for the same coordinator while
  a `running` row exists that is not expired. A row shall expire 5 minutes
  after its last refresh, which the running episode shall repeat every minute.
- **AC-COORDINATOR-SHADOW-DREAM-001.3:** When a `running` row has expired, the
  system shall set it `failed` with the reason `lease_lost`, cancel its
  episode if it is still active, and leave the window unchanged so the next
  dream reads the same evidence; a late completion of that episode shall
  change nothing.
- **AC-COORDINATOR-SHADOW-DREAM-001.4:** The input hash shall be the hash of
  the projected evidence, the prompt version and the model that will answer.
  When an accepted row has the same hash, the system shall not start the agent
  and shall store one row with status `skipped` and the reason `unchanged`, at
  most once per hash.
- **AC-COORDINATOR-SHADOW-DREAM-001.5:** A dream shall read nothing that a
  dream wrote and shall not count its own turn as evidence: turns with the
  trigger `dream` shall be outside every window and every count.
- **AC-COORDINATOR-SHADOW-DREAM-001.6:** When any admission condition fails
  (containment, spend unmeasurable, spend at the ceiling, Pause, autonomy off),
  or the 24 hours since the last episode have not passed, the system shall start nothing and store no row; the health state shall name the condition. When autonomy is turned off, Pause is set or the containment check fails while an episode is running, the system shall cancel it within one minute and set its row `failed` with the reason `autonomy_off`, `paused` or `containment`. The lease of `001.2` shall cover the whole dream, replays included, and a running dream shall be expired whatever the coordinator's autonomy.

### REQ-COORDINATOR-SHADOW-DREAM-002: An episode with no write tools

**Intent:** Shadow can change nothing, by construction and not by instruction.

#### Acceptance criteria

- **AC-COORDINATOR-SHADOW-DREAM-002.1:** The episode shall run in its own ephemeral session, in a task of the coordinator's workspace that is marked as a dream task of that coordinator so that its usage counts as the coordinator's spend, and shall touch neither the conversation nor its queue, and shall not archive, repoint or create a conversation; conversation binding and cleanup shall ignore a dream task.
- **AC-COORDINATOR-SHADOW-DREAM-002.2:** The episode's Kandev tool profile
  shall hold exactly `list_coordinator_turns_kandev`, which shall return no turn with the trigger `dream` to it. Every other Kandev
  action called from that session shall be refused as an unknown action by the
  guard and written nowhere; the profile shall never be derived from the
  coordinator's May do settings.
- **AC-COORDINATOR-SHADOW-DREAM-002.3:** The system shall refuse to start an
  episode whose containment check fails
  ([containment](containment.md#req-coordinator-containment-001-the-containment-check)),
  and shall stop it at the spend ceiling as an unattended turn is stopped
  ([spend](spend.md#req-coordinator-spend-003-stopping-at-the-ceiling)); its
  usage shall count toward the coordinator's spend.
- **AC-COORDINATOR-SHADOW-DREAM-002.4:** The episode shall be one turn with a
  wall-clock bound of 20 minutes, after which the system shall cancel it and
  set the row `failed` with the reason `timeout`, and its opening message
  shall hold at most 200 turns and 60,000 characters.
- **AC-COORDINATOR-SHADOW-DREAM-002.5:** The opening message shall be built by
  the server from stored rows and shall hold, per turn, the trigger, verdict,
  call counts by action, and per decided proposal its kind, decision, edited
  field names, reason code, task result, cost and its title truncated to 80
  characters, marked as data. It shall hold no message text, task
  description, child-task conversation or intake text. The episode's tool profile
  (`002.2`) has no tool that reads a run record or a conversation.
- **AC-COORDINATOR-SHADOW-DREAM-002.6:** The only path that starts a turn for a
  dream shall be the trigger of `001.1`; the coordinator's tool surface, a
  manager's request, a wake and a delivery shall not start one.
- **AC-COORDINATOR-SHADOW-DREAM-002.7:** The episode's turn shall have its own
  ledger row with the trigger `dream`, the stamp of the model that answered,
  and shall be archived with its ephemeral session when it ends, and shall
  never be delivered to the coordinator's conversation or shown in Needs you.

### REQ-COORDINATOR-SHADOW-DREAM-003: The report

**Intent:** Every dream ends in one stored, readable result, including "nothing
to propose".

#### Acceptance criteria

- **AC-COORDINATOR-SHADOW-DREAM-003.1:** The agent's answer shall be one JSON
  document of at most 10 items, at most 20 considered-not-proposed entries and
  no other keys. The system shall parse it once; an answer that is not valid,
  exceeds those limits or has an unknown item kind shall set the row `failed`
  with the reason `bad_output` and store no item.
- **AC-COORDINATOR-SHADOW-DREAM-003.2:** Every dream that reaches an answer
  shall store its row with the window, the turn ids read, the items, the
  considered-not-proposed list labelled agent-reported, the model stamp, the
  cost and the status: `ok` when there are no items ("nothing to propose"),
  `clean` when every item passed the gate and was replayed, `partial` when
  any item was refused or not measured, `failed` for `bad_output`,
  `timeout`, `lease_lost`, a stop at the ceiling or a run error.
- **AC-COORDINATOR-SHADOW-DREAM-003.3:** Each item shall carry an id, its
  kind, its text (at most 500 characters; a context diff at most the
  coordinator context's limit), at least two distinct cited turn ids, its gate
  result and its replay result. The report shall keep refused items, marked
  with their reason.
- **AC-COORDINATOR-SHADOW-DREAM-003.4:** The system shall store the report
  and its status in one transaction, and a row that is not `running` shall
  never change again except by ratings (`006`) and by the retention deletion of `003.5`.
- **AC-COORDINATOR-SHADOW-DREAM-003.5:** Reports shall be kept for 400 days and
  deleted with their coordinator.

### REQ-COORDINATOR-SHADOW-DREAM-004: Gate and replay

**Intent:** A suggestion is checked before it is shown, and measured before it
is trusted.

#### Acceptance criteria

- **AC-COORDINATOR-SHADOW-DREAM-004.1:** After the answer is parsed, the system
  shall replay up to 5 items that passed the gate, in report order, through the
  harness ([replay](replay.md)), each as a candidate that applies the item to
  the coordinator's current configuration, and shall store each item's replay
  result (`blocked`, `improvement`, `not_an_improvement` or `unmeasured` with
  its reason). An item beyond the fifth, or one for which a run's budget was
  exhausted, shall be `unmeasured`.
- **AC-COORDINATOR-SHADOW-DREAM-004.2:** The gate shall refuse an item, with a
  named reason, when any cited turn does not exist, belongs to another
  coordinator, lies outside the window or is a dream turn
  (`citation_unresolved`); when its text matches the containment check's
  credential patterns (`credential`); when it is larger than its limit
  (`size`); when its kind needs a target that does not exist yet (`no_target`,
  which is every `note_update` and `note_retire` in phase 3.1); when it names a
  standing order that is retired or not the coordinator's (`bad_target`);
  when it names the coordinator's permissions, watches, autonomy, ceiling, tool
  profile or human-written context as something to change (`forbidden_target`);
  or when it cites fewer than two distinct turns (`thin_evidence`).
- **AC-COORDINATOR-SHADOW-DREAM-004.3:** The gate shall run before any replay
  and refuse or pass an item without a model call; an item's gate result shall
  be the same for the same item and evidence.
- **AC-COORDINATOR-SHADOW-DREAM-004.4:** No item, whatever its gate or replay
  result, shall be written to the coordinator's context, standing orders,
  notes, settings or any store a turn reads; the coordinator's next turn
  shall not depend on any report.
- **AC-COORDINATOR-SHADOW-DREAM-004.5:** A `standing_order_add`,
  `standing_order_retire` or `context_diff` item shall be described as it would
  apply, and shall never change the coordinator's configuration revision.

### REQ-COORDINATOR-SHADOW-DREAM-005: Screens

**Intent:** A manager can turn Shadow on, see that it is healthy and read what
it found.

#### Acceptance criteria

- **AC-COORDINATOR-SHADOW-DREAM-005.1:** While the phase 3.1 flag is effective,
  the coordinator's settings shall have a Learning section, visible to any
  workspace member, holding the Shadow dream switch (changeable by a manager
  only, saved on its own request, refused with 403 to a reader and to a
  coordinator principal), a health line, the measures of
  `AC-COORDINATOR-OUTCOMES-003.4`, and the list of reports.
- **AC-COORDINATOR-SHADOW-DREAM-005.2:** Turning the switch on while autonomy
  is off shall be accepted and shall show "Waiting: autonomy is off"; the
  switch shall be disabled with the reason while the workspace's coordinator
  list has not loaded.
- **AC-COORDINATOR-SHADOW-DREAM-005.3:** The report list shall show reports
  newest first by start time (ties by id descending) with status, window,
  item count and cost, 20 per page, and "No dreams yet" when empty. A report
  shall open a detail view with the window, the number of turns read, every
  item with its cited turns, gate result and replay verdict, the
  considered-not-proposed list marked "Reported by the agent, not verified",
  and a "Retire or supersede" section shown even when it has no item.
- **AC-COORDINATOR-SHADOW-DREAM-005.4:** The Learning section shall state that
  Shadow changes nothing, and shall never offer to apply, approve or copy an
  item into the coordinator's settings.
- **AC-COORDINATOR-SHADOW-DREAM-005.5:** On a phone the section shall stack
  the health line, measures and list as single-column cards with touch targets
  of at least 44 pixels, the detail view as a full-screen page, and the rating
  buttons as a segmented control.
- **AC-COORDINATOR-SHADOW-DREAM-005.6:** A report that cannot load shall show
  Try again and no partial item; a list that cannot load shall not show
  "No dreams yet".

### REQ-COORDINATOR-SHADOW-DREAM-006: Ratings and health

**Intent:** The owner's judgement of each item is kept next to the replay's.

#### Acceptance criteria

- **AC-COORDINATOR-SHADOW-DREAM-006.1:** A manager shall be able to rate any
  item of a report `useful`, `not_useful` or `harmful`, and to change their
  rating; the system shall keep one rating per item and manager, the last
  committed request winning, and shall treat the newest rating of an item
  across managers (ties by user id) as its rating.
- **AC-COORDINATOR-SHADOW-DREAM-006.2:** A reader and a coordinator principal
  shall be refused a rating with 403 and change nothing; a rating of an item
  that does not belong to the named report shall be not found.
- **AC-COORDINATOR-SHADOW-DREAM-006.3:** The health state shall be `off` while
  Shadow is not enabled, `running` while a dream is, `waiting` (with the
  condition) while an admission condition fails or the trigger of `001.1` is
  not met, `fresh` when the last dream was accepted within 36 hours or the
  evidence debt has not existed that long, `stale` when the debt has been met
  for more than 36 hours without an accepted dream and `failed` at more than
  72 hours, or when the last dream ended `failed`; each with the fix it
  implies ("Turn autonomy on", "Resume", "Raise the ceiling").
- **AC-COORDINATOR-SHADOW-DREAM-006.4:** The agreement measure shall count an
  item only when it has a rating and a replay verdict of `blocked`,
  `improvement` or `not_an_improvement`, and shall count it as agreeing when
  `improvement` meets `useful`, or `blocked` or `not_an_improvement` meets
  `not_useful` or `harmful`.

## Out of scope

- Any Active behaviour: a card, per-item approve, apply, revert, notes,
  the note index in the prompt, expiry of notes, model-change protocol.
- Routine suggestions, and any user-defined trigger.
- Dream reads of child-task conversations or intake text.
- Project-scoped evidence rules (a project-scoped item becoming a
  workspace-wide rule); phase 3.5 decides them when items can apply.
- Re-judging refused items with a stronger model.
- A manual "Dream now" control.
