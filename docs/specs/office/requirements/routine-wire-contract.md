---
status: draft
system: office
created: 2026-09-17
owners:
  - kandev
---

# Office Routine Wire Contract Requirements

## Overview

The Office routine HTTP contract is snake_case in both directions:
`CreateRoutineRequest`, `UpdateRoutineRequest` and `CreateTriggerRequest`
(`apps/backend/internal/office/routines/dto.go`) bind keys like `task_template`
and `cron_expression`, and `models.Routine`, `models.RoutineTrigger` and
`models.RoutineRun` serialize the same keys. The web client sends and reads
camelCase. Go's `encoding/json` matches case-insensitively but does not fold
underscores, so every multi-word key crosses the boundary and is discarded.

This is measured, not inferred: binding the real DTOs through `gin`
`ShouldBindJSON`, a camelCase routine body binds `name` alone, a camelCase
`cronExpression` is rejected as a 400, and a `task_template` sent as a JSON
object fails to bind.

Both consequences are live. On write, a routine's assignee, policies, catch-up
max and task template are silently dropped, so the UI reports success and
persists defaults. On the trigger path nothing is silent: a cron schedule cannot
be armed at all, removing the product's only path to an unattended routine. On read, only `routine-row.tsx` has snake_case fallbacks
and only for two fields, so the detail form, the expanded row, the Schedule
card and the Runs list render blanks.

This capability makes the web client speak the contract the backend publishes.
It changes no HTTP contract, no Go struct tag, no database column and no
scheduler behavior.

## Terminology

Wire shape, model shape, adapter, envelope, arm, patch field, effective
timezone and trimmed expression are defined in the system design's
[Terminology](../system-design/routine-wire-contract.md#terminology).

## Requirements

### REQ-OFFICE-ROUTINE-WIRE-001: Routine configuration written from the UI is persisted

**Intent:** A create or save that reports success must have persisted what the
operator chose. Today every multi-word field is discarded and the operator is
told it worked.


**User story:** As an operator, I want the assignee, policies and task template
I pick to be saved, so that a routine does what I configured it to do.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-WIRE-001.1:** When the client creates a routine, the
  system shall send each supplied value AC-001.11 admits under its contract
  key: `name`,
  `description`, `task_template`, `assignee_agent_profile_id`,
  `concurrency_policy`, `catch_up_policy`, `catch_up_max`, `variables`.
- **AC-OFFICE-ROUTINE-WIRE-001.2:** When the client creates a routine, the
  system shall send no key outside AC-001.1's set, and in particular no
  camelCase spelling of any of them.
- **AC-OFFICE-ROUTINE-WIRE-001.3:** When the client sends `task_template` or
  `variables`, the system shall send a JSON-encoded string, never a JSON object,
  because the bound fields are Go strings and an object is rejected as a 400.
- **AC-OFFICE-ROUTINE-WIRE-001.4:** When the client updates a routine, the
  system shall include exactly the patch fields AC-001.10 and AC-001.11 admit,
  so a field the caller did not supply is absent from the request body and the
  stored value is unchanged.
- **AC-OFFICE-ROUTINE-WIRE-001.5:** When a caller supplies an empty string for
  `assignee_agent_profile_id`, `description` or `task_template` on update, the
  system shall send it, because an empty string clears and an absent key
  preserves (AC-001.11).
- **AC-OFFICE-ROUTINE-WIRE-001.6:** When the client creates a routine, the
  system shall not send `workspace_id` or `status`, because the handler takes
  the workspace from the route and forces `active`.
- **AC-OFFICE-ROUTINE-WIRE-001.7:** When a caller does not supply
  `concurrency_policy` or `catch_up_policy`, the system shall omit that key and
  shall not substitute a client-side default, so the server defaults
  (`skip_if_active`, `summarize_missed`) apply.
- **AC-OFFICE-ROUTINE-WIRE-001.8:** When the routine list toggle pauses or
  resumes a routine, the system shall send `status` alone and shall not widen
  the patch to any other field.
- **AC-OFFICE-ROUTINE-WIRE-001.9:** When the client updates a routine, the
  system shall send no key outside AC-001.1's set plus `status`, and in
  particular no camelCase spelling of any of them. `status` is an update key
  and not a create key (AC-001.6).
- **AC-OFFICE-ROUTINE-WIRE-001.10:** When a caller supplies an empty
  `concurrency_policy` or `catch_up_policy` on update, the system shall omit
  that key rather than send the empty string, because `Valid()`
  (`models/enums.go`) rejects it with a 400. Those two keys are the only ones
  where empty means omit; every other update key transmits its empty string and
  clears (AC-001.11).
- **AC-OFFICE-ROUTINE-WIRE-001.11:** When the system builds an update body, the
  clearable set shall be `name`, `description`, `task_template`,
  `assignee_agent_profile_id`, `status` and `variables`, not AC-001.5's three:
  `UpdateRoutineRequest` types all six as pointers and `applyRoutineUpdates`
  (`routines/handler.go`) assigns each one unvalidated, so an empty string
  supplied for any of the six is stored rather than ignored. The system shall
  not send an empty `status` (AC-003.17), and shall omit a `catch_up_max` below 1
  from a create or an update body rather than send it, because `*int` stores
  what is sent (AC-003.13).

### REQ-OFFICE-ROUTINE-WIRE-002: A cron schedule can be armed and changed from the UI

**Intent:** A routine with no cron trigger never fires unattended. The trigger
create call is currently rejected outright, so the UI cannot arm one.

**User story:** As an operator, I want to set a routine's schedule in the UI,
so that the routine runs without me firing it by hand.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-WIRE-002.1:** When the client creates a routine trigger,
  the system shall send `kind`, `cron_expression`, `timezone`, `public_id`,
  `signing_mode` and `secret` under those keys, omitting any the caller did not
  supply.
- **AC-OFFICE-ROUTINE-WIRE-002.2:** When the create-routine flow is submitted
  with trigger kind `cron` and a non-empty trimmed expression, the system shall
  arm a cron trigger on the new routine, and the created trigger's `next_run_at`
  shall be non-null. A next fire time is a trigger field, not a routine field.
- **AC-OFFICE-ROUTINE-WIRE-002.3:** When the detail view opens for a routine
  that has a cron trigger, the system shall seed the form with that trigger's
  stored expression and timezone.
- **AC-OFFICE-ROUTINE-WIRE-002.4:** When the detail view is saved, the draft's
  trimmed expression matches the stored trigger's expression exactly, and the
  draft's effective timezone matches the stored trigger's effective timezone,
  the system shall issue no trigger create and no trigger delete.
- **AC-OFFICE-ROUTINE-WIRE-002.5:** When the detail view is saved with a changed
  expression or timezone, the system shall create the replacement trigger before
  deleting the superseded one. When the routine has no cron trigger yet, the
  system shall create one and delete nothing.
- **AC-OFFICE-ROUTINE-WIRE-002.6:** When that replacement create is rejected,
  the system shall leave the existing trigger in place, shall issue no delete,
  shall show the server's error message in an error toast, and shall show no
  success toast.
- **AC-OFFICE-ROUTINE-WIRE-002.7:** When the replacement create succeeds and the
  subsequent delete fails, the system shall re-list the routine's triggers and
  show an error toast naming the delete failure and stating how many cron
  triggers that re-list found, because two cron triggers both fire. When the
  re-list itself fails, the toast shall instead state that the
  trigger's fate is unknown and the page needs a reload.
- **AC-OFFICE-ROUTINE-WIRE-002.8:** When a routine has more than one cron
  trigger, the system shall seed, replace and display the first cron trigger in
  the order the list endpoint returned (`ORDER BY created_at`), and shall leave
  the others untouched. That order is always read from an endpoint response
  (AC-002.11).
- **AC-OFFICE-ROUTINE-WIRE-002.9:** When a save updates the routine and its
  trigger create or delete then fails, the system shall show an error toast
  naming the trigger error and stating that the routine's own fields were saved
  and the schedule was not, shall show no success toast, and shall re-list the
  routine's triggers. A failed re-list is AC-002.11's case, not this one.
- **AC-OFFICE-ROUTINE-WIRE-002.10:** When the create-routine flow creates the
  routine and its cron trigger create then fails, the system shall close the
  create dialog, refresh the routine list so the new routine is visible, and
  show an error toast naming the trigger error and stating that the routine was
  created without a schedule. The routine is not deleted.
- **AC-OFFICE-ROUTINE-WIRE-002.11:** When the system creates or deletes a
  trigger, it shall render the routine's triggers from a fresh list response
  rather than from a locally spliced array. When that re-list fails after the
  create and delete both succeeded, the system shall report the save as
  succeeded and state that the displayed schedule may be stale until a reload.
- **AC-OFFICE-ROUTINE-WIRE-002.12:** When AC-002.7, AC-002.9, AC-002.10 or
  AC-002.11 shows a message today's code has no string for, that message shall
  be a translated `office`-namespace key present in English, `pt-pt`, `zh-cn`,
  `zh-hk` and `zh-tw`.
- **AC-OFFICE-ROUTINE-WIRE-002.13:** When the detail view is saved with a cron
  expression that is empty after trimming, the system shall issue no trigger
  create and no trigger delete, leaving any existing trigger armed. Clearing a
  schedule is out of scope below.
- **AC-OFFICE-ROUTINE-WIRE-002.14:** When the system decides whether a cron
  expression is empty, it shall decide on the trimmed expression, and when it
  sends one it shall send that trimmed string, so whitespace alone is empty at
  every gate and never reaches the server. The three live gates disagree today
  (system design, [Control flow](../system-design/routine-wire-contract.md#control-flow)).

### REQ-OFFICE-ROUTINE-WIRE-003: Routine and trigger reads render persisted values

**Intent:** The list, the expanded row, the detail form and the Schedule card
must show what is stored. Today they show blanks, placeholders and, for declared
variables, the characters of a JSON string one per line.

**User story:** As an operator, I want the routine pages to show the stored
configuration, so that I can see what a routine will actually do.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-WIRE-003.1:** When a routine endpoint returns a routine,
  the system shall produce a model whose `taskTemplate`,
  `assigneeAgentProfileId`, `concurrencyPolicy`, `catchUpPolicy`, `catchUpMax`,
  `variables`, `lastRunAt`, `workspaceId`, `createdAt` and `updatedAt` carry the
  persisted values.
- **AC-OFFICE-ROUTINE-WIRE-003.2:** When a routine endpoint returns a trigger,
  the system shall produce a model whose `routineId`, `cronExpression`,
  `timezone`, `publicId`, `signingMode`, `nextRunAt`, `lastFiredAt`, `enabled`,
  `createdAt` and `updatedAt` carry the persisted values. `created_at` and
  `updated_at` are mapped, not direct: the model declares them camelCase.
- **AC-OFFICE-ROUTINE-WIRE-003.3:** When the wire carries `task_template` or
  `variables` as a JSON-encoded string, the system shall parse it into an object
  before it reaches a component.
- **AC-OFFICE-ROUTINE-WIRE-003.4:** When that string is empty, is not valid
  JSON, or does not decode to a JSON object, the system shall produce an empty
  object rather than raising or passing the raw string through. A JSON object
  here means a non-null, non-array object, so `null`, `[]`, `[{"a":1}]`, `3` and
  `"text"` all produce the empty object.
- **AC-OFFICE-ROUTINE-WIRE-003.5:** When a response is enveloped, the system
  shall unwrap it by reading the envelope key alone, so a caller receives the
  resource or the array and never a wrapper object. On a list endpoint a missing
  key, a `null` payload and a non-array payload all produce the empty array. On
  a single-resource endpoint the same three cases are an error (AC-003.15), not
  an empty model. A body that is not an object has no envelope key and takes the
  same branch as a missing one.
- **AC-OFFICE-ROUTINE-WIRE-003.6:** When a routine or trigger field is absent or
  `null`, the system shall produce the empty string for a field the model
  declares as a required string, `{}` for one it declares as a required object
  (`taskTemplate`), `undefined` for a field the model declares optional, and
  `false` for `enabled`, and shall not substitute a policy value the server did
  not send. `status` and `kind` are required strings and take the
  same rule as any other (AC-003.16).
- **AC-OFFICE-ROUTINE-WIRE-003.7:** When a list response contains an element
  that is not a JSON object in AC-003.4's sense, the system shall drop that
  element and keep the rest.
- **AC-OFFICE-ROUTINE-WIRE-003.8:** When a list response is returned, the system
  shall preserve the server's element order and shall not re-sort.
- **AC-OFFICE-ROUTINE-WIRE-003.9:** When a trigger is normalized, the system
  shall map only the fields the model declares, so the wire's `secret` key does
  not enter the client model or the store.
- **AC-OFFICE-ROUTINE-WIRE-003.10:** When a component renders a routine, trigger
  or run, it shall read only model-shape fields. The claim is checked by a scan
  over `apps/web/app/office/routines/**` plus
  `apps/web/src/office-routine-client-routes.tsx` asserting two negatives: no
  snake_case wire key from the three mapping tables, and no `as unknown as`.
  That removes `routine-row.tsx`'s two snake_case fallback casts and all four
  envelope casts. The fourth is `syncCronTrigger`'s, which alone has no
  bare-resource fallback and would silently drop the created trigger if it
  survived the unwrap (system design,
  [Components and responsibilities](../system-design/routine-wire-contract.md#components-and-responsibilities)).
- **AC-OFFICE-ROUTINE-WIRE-003.11:** When a routine declares variables, the
  expanded row shall render one entry per declared variable name, not one entry
  per character of the encoded string.
- **AC-OFFICE-ROUTINE-WIRE-003.12:** When the wire carries `catch_up_max`, the
  system shall carry the stored number into the model unchanged and shall not
  clamp it, because the server normalizes at use
  (`models.NormaliseCatchUpMax`, ceiling 1000) and the model must report what is
  stored.
- **AC-OFFICE-ROUTINE-WIRE-003.13:** When a routine form seeds a catch-up max
  below 1, or the operator enters one below 1, the system shall use 25, because
  the server treats a sub-1 stored value as 25. `min={1}` enforces nothing here:
  no routine form saves through a form submit.
- **AC-OFFICE-ROUTINE-WIRE-003.14:** When the wire carries an empty `status`,
  the system shall carry the empty string into the model unchanged, because the
  empty string is a firing status the scheduler and `isRoutineFiring` both
  honour, and substituting `active` would assert a value no writer set.
- **AC-OFFICE-ROUTINE-WIRE-003.15:** When a single-resource endpoint returns a
  body whose resource is missing, `null`, not a JSON object in AC-003.4's sense,
  or an object carrying no non-empty `id`, the system shall reject the call with
  an error rather than produce a model built from nothing. The create-routine flow reads the new routine's `id`
  to arm its trigger, so an empty model would skip arming silently.
- **AC-OFFICE-ROUTINE-WIRE-003.16:** When the adapter produces a routine,
  trigger or run model, it shall do so without a type assertion, so
  `Routine.status`, `RoutineRun.status` and `RoutineTrigger.kind` shall each
  admit any string the server stored. None is validated on write, so `""`
  (AC-003.14, AC-004.5) and an arbitrary `unknown` are both reachable.
- **AC-OFFICE-ROUTINE-WIRE-003.17:** When the detail form seeds from a routine
  whose `status` is not one of the control's three options, `""` included, the
  control shall show no selected option, and a save that left it untouched shall
  omit `status` from the patch rather than write back a value no writer set
  (AC-001.11). A `""` routine shall still read as firing wherever firing state
  is shown (AC-003.14). No `unset` option is added.
- **AC-OFFICE-ROUTINE-WIRE-003.18:** When a file outside AC-003.10's scanned set
  reads `s.office.routines`, it shall consume model shape and shall test firing
  state through `isRoutineFiring` rather than comparing `status` to a literal.
  Two such files exist, both under `app/office/agents/[id]/`, and both are
  outside the scan because they also consume `AgentRunSummary`, a wire shape
  this contract does not own. The scan shall assert that the set of files
  reading `s.office.routines` under `apps/web/app/**` is exactly the three that
  do today, enumerated in the system design's
  [Testing](../system-design/routine-wire-contract.md#testing), so a new
  consumer fails the check rather than escaping the bound. AC-003.10's set is
  larger: most of its files take routines as props.
- **AC-OFFICE-ROUTINE-WIRE-003.19:** When `CoordinatorRoutineHint` renders for
  a CEO agent with a firing assigned routine present in `s.office.routines`, it
  shall not show the "no scheduled wake-ups" hint. The criterion is the
  component, not `/office/agents/:id`: nothing on that route populates the
  store, so the hint shows there regardless (system design, Out of scope). This
  capability owns the change: `assigneeAgentProfileId` is `undefined` for every
  routine today, so the hint is unconditional until the assignee transmits.

### REQ-OFFICE-ROUTINE-WIRE-004: Routine run reads render persisted values

**Intent:** The Runs tab reads the same boundary and is broken the same way:
`dispatch_fingerprint`, `linked_task_id` and `created_at` never reach the model,
so every run shows placeholders and no linked task.

**User story:** As an operator, I want the Runs list to show when each run
happened and which task it created, so that I can tell whether a routine is
working.

#### Acceptance criteria

- **AC-OFFICE-ROUTINE-WIRE-004.1:** When a runs endpoint returns runs, the
  system shall produce models whose `id`, `source`, `status`, `routineId`,
  `triggerId`, `triggerPayload`, `linkedTaskId`, `coalescedIntoRunId`,
  `dispatchFingerprint`, `catchUpMissedTicks`, `catchUpFirstMissedAt`,
  `catchUpTruncated`, `startedAt`, `completedAt` and `createdAt` carry the
  persisted values. `id`, `source` and `status` are named even though their
  spellings match, because the adapter maps declared fields explicitly and
  never spreads.
- **AC-OFFICE-ROUTINE-WIRE-004.2:** When a run's `catch_up_missed_ticks` is
  absent, the system shall produce `undefined` and shall not produce `0`,
  because absence is never represented as a stored zero (`routine-catch-up.md`
  AC-002.3).
- **AC-OFFICE-ROUTINE-WIRE-004.3:** When a run row renders, it shall show the
  run's creation time and, when the run created a task, the linked task id.
  `run-row.tsx` already renders both; this verifies the values stop being
  placeholders.
- **AC-OFFICE-ROUTINE-WIRE-004.4:** When the manual fire endpoint is called, the
  system shall continue to send `{"variables": ...}` unchanged, which already
  matches the contract.
- **AC-OFFICE-ROUTINE-WIRE-004.5:** When a run field is absent or `null`, the
  system shall apply AC-003.6's rule to it. Absence is ordinary: a manually
  fired run has no `trigger_id`, and a `received` run has neither `started_at`
  nor `completed_at`. An empty or unrecognized `status` renders the unlabeled
  badge `run-row.tsx` already produces, so no new copy is added.

## Concurrency, retry and ordering

Stated in the system design's
[Concurrency, retry and ordering](../system-design/routine-wire-contract.md#concurrency-retry-and-ordering):
they are properties of the server contract, not client choices, and AC-001.4,
AC-002.8, AC-002.11 and AC-003.8 depend on them.

## Out of scope

Listed in the system design's
[Out of scope](../system-design/routine-wire-contract.md#out-of-scope). Each
names what is not built here and what a later capability would have to settle.
