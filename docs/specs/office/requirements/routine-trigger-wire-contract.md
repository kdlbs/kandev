---
status: draft
system: office
created: 2026-09-17
owners:
  - kandev
---

# Office Routine Trigger Wire Contract Requirements

## Overview

The Office routine trigger API speaks snake_case: `models.RoutineTrigger` tags
`cron_expression`, `next_run_at` and six more, and `CreateTriggerRequest` binds
`cron_expression`. The web `RoutineTrigger` type declares those same fields in
camelCase, and `fetchJson` is a raw `JSON.parse`, so nothing translates between
the two.

Both directions are broken; the write direction is the severe one. Reading, the
cron expression and next-fire countdown are `undefined` for every routine.
Writing, `createRoutineTrigger` serializes camelCase,
`CreateTriggerRequest.CronExpression` binds empty, and `CreateRoutineTrigger`
rejects it with `cron trigger requires a cron_expression` as HTTP 400. **A cron
schedule cannot be armed through the product at all.**

This capability makes the trigger wire shape cross the API boundary in one place,
in both directions, and makes the routine row honest about which trigger it
describes.

## Terminology

- **Wire shape:** the JSON object the backend emits or accepts, snake_case.
- **Domain shape:** the `RoutineTrigger` type the web app's components read,
  camelCase.
- **Normalizer:** the read-direction function converting a wire trigger to a
  domain trigger.
- **Serializer:** the write-direction function converting a create input into the
  `CreateTriggerRequest` body.
- **Trigger sync:** the routine detail page's save path reconciling the drafted
  cron expression and timezone against its AC-004.8 sync target.
- **Primary cron trigger:** the single cron trigger a routine's surfaces describe
  when it has more than one.

## Requirements

### REQ-OFFICE-TRIGGER-WIRE-001: A fetched trigger populates every field the UI reads

**Intent:** The countdown and the cron text are the only evidence an operator has
that a routine is armed and when it fires, so they must be driven by the values the
backend sent.

**User story:** As an operator, I want a routine's cron expression and next fire
time to display, so I can tell an armed routine from a dormant one.

#### Acceptance criteria

- **AC-OFFICE-TRIGGER-WIRE-001.1:** When the API returns a trigger object, the
  system shall produce a domain trigger taking `cronExpression`, `timezone`,
  `publicId`, `signingMode`, `nextRunAt`, `lastFiredAt`, `routineId`, `kind`,
  `enabled`, `id`, `createdAt` and `updatedAt` from the wire keys
  `cron_expression`, `timezone`, `public_id`, `signing_mode`, `next_run_at`,
  `last_fired_at`, `routine_id`, `kind`, `enabled`, `id`, `created_at` and
  `updated_at` respectively.
- **AC-OFFICE-TRIGGER-WIRE-001.2:** When a wire object carries the camelCase key
  for a field with a value that is neither `null` nor `undefined`, the system shall
  use that value and shall not read that field's snake_case key. A camelCase key
  present but `null` or `undefined` shall fall through to the snake_case key.
- **AC-OFFICE-TRIGGER-WIRE-001.3:** When a domain trigger produced by the
  normalizer is passed to the normalizer again, the system shall produce a value
  deeply equal to its input.
- **AC-OFFICE-TRIGGER-WIRE-001.4:** When `next_run_at` or `last_fired_at` is a
  non-empty string, the system shall carry that string through unchanged, parsing and
  formatting neither. For every other wire value, including `null`, absent, the empty
  string and any non-string JSON type, the system shall set the corresponding domain
  field to `undefined`. These two fields are the exception AC-001.5 and AC-001.12 name.
- **AC-OFFICE-TRIGGER-WIRE-001.5:** When a string-valued wire field other than the
  two AC-001.4 timestamps holds the empty string, the system shall set the domain field
  to the empty string and not to `undefined`, so AC-001.12's guarantee that every such
  field is a string holds for a supplied empty value too.
- **AC-OFFICE-TRIGGER-WIRE-001.6:** When `enabled` is absent or is not a JSON
  boolean, the system shall set the domain field to `false`.
- **AC-OFFICE-TRIGGER-WIRE-001.7:** When a wire object carries keys the
  `RoutineTrigger` domain type does not declare, the system shall omit them from
  the result. In particular the domain trigger shall never carry `secret`, whatever
  the wire value.
- **AC-OFFICE-TRIGGER-WIRE-001.8:** When the trigger list endpoint returns, the
  system shall normalize every array element that is a JSON object, preserve the
  response's element order, and drop every element that is not. When `triggers` is
  absent or is not an array, the system shall produce an empty array.
- **AC-OFFICE-TRIGGER-WIRE-001.9:** When the trigger create endpoint returns a
  body whose `trigger` is absent, is not a JSON object, or normalizes to a trigger
  that AC-001.13 drops, the system shall treat the call as having produced no
  trigger and shall add no entry to the caller's trigger state.
- **AC-OFFICE-TRIGGER-WIRE-001.11:** When `kind` holds a value the domain union
  does not declare, the system shall carry it through unchanged rather than
  dropping the trigger or coercing it. The domain type's `kind` shall widen from
  `RoutineTriggerKind` to `string`, with `RoutineTriggerKind` retained unchanged as
  the set of values consumers act on rather than as the field's type.
- **AC-OFFICE-TRIGGER-WIRE-001.10:** When any caller in the web app obtains
  routine triggers, it shall receive the domain shape without doing its own key
  mapping. Normalization shall be applied inside the API layer functions returning
  triggers, as `normalizeProject` and `normalizeActivityEntry` are today.
- **AC-OFFICE-TRIGGER-WIRE-001.12:** When a string-valued wire field other than
  `next_run_at` and `last_fired_at` is absent, JSON `null`, or not a JSON string,
  the system shall set the domain field to the empty string, required and optional
  alike, so each is a string on every normalized trigger, which is what makes
  AC-004.4's `===` total. AC-001.4's two timestamps are the one exception.
- **AC-OFFICE-TRIGGER-WIRE-001.13:** When a normalized trigger's `id` is the empty
  string, the system shall drop that trigger rather than returning it, on both the
  list and create paths. `id` is the delete target and REQ-003's tiebreak column.

### REQ-OFFICE-TRIGGER-WIRE-002: A cron schedule can be armed from the UI

**Intent:** Scheduling recurring work is the purpose of a routine, and a create the
server rejects on every attempt means the feature does not exist.

**User story:** As an operator, I want to save a cron expression on a routine and
have it accepted, so the routine actually runs on a schedule.

#### Acceptance criteria

- **AC-OFFICE-TRIGGER-WIRE-002.1:** When the web app requests creation of a cron
  trigger, the serialized body shall carry the supplied expression under the key
  `cron_expression`, the supplied timezone under `timezone`, and the kind under
  `kind`.
- **AC-OFFICE-TRIGGER-WIRE-002.2:** When the web app serializes a trigger create
  request, the body shall contain no camelCase key for a field with a snake_case
  wire name. Emitting both spellings is not acceptable.
- **AC-OFFICE-TRIGGER-WIRE-002.3:** A field is supplied when its property is
  present on the create input with a value other than `undefined`. When the caller
  does not supply a field, the system shall omit that field's key from the body
  rather than emitting an empty value for it. The empty string is a supplied value
  and shall be emitted as `""`, not omitted.
- **AC-OFFICE-TRIGGER-WIRE-002.4:** When the caller supplies webhook fields, they
  shall be emitted as `public_id`, `signing_mode` and `secret`.
- **AC-OFFICE-TRIGGER-WIRE-002.5:** When a cron trigger is created with an
  expression the backend accepts, the API shall respond 201 and the normalized
  trigger the web app returns shall carry a defined `nextRunAt`.
- **AC-OFFICE-TRIGGER-WIRE-002.6:** When a caller constructs a trigger create
  input, the type shall not accept `id`, `enabled`, `nextRunAt`, `lastFiredAt`,
  `createdAt` or `updatedAt`. Those are server-owned.
- **AC-OFFICE-TRIGGER-WIRE-002.7:** The trigger create input shall be a type
  declared for this purpose and not derived from `RoutineTrigger`. Its fields
  shall be exactly `kind`, required, plus the optional strings `cronExpression`,
  `timezone`, `publicId`, `signingMode` and `secret`. `routineId` shall not be a
  field: the routine is named by the path argument.
- **AC-OFFICE-TRIGGER-WIRE-002.8:** When the create-routine dialog creates the routine
  successfully and the trigger create it issues next then fails, the system shall report
  that the routine was created without a schedule, distinctly from a failure to create
  the routine itself, shall close the dialog, and shall refresh the routine list so the
  created routine is visible. The operator shall not be left to retry a create that
  would persist a second routine. The message shall be localized on the same terms as
  AC-004.2's.

### REQ-OFFICE-TRIGGER-WIRE-003: A routine row describes one trigger, consistently

**Intent:** Once these fields populate, a routine with more than one cron trigger
can render one trigger's expression beside a different trigger's countdown: the
list row selects by array position while the countdown selects by earliest time.

**User story:** As an operator, I want the schedule text and the countdown on a
routine row to describe the same trigger, so I can trust what the row tells me.

#### Acceptance criteria

- **AC-OFFICE-TRIGGER-WIRE-003.1:** The system shall select a routine's primary
  cron trigger from the triggers whose `kind` is `cron`, whose `enabled` is true,
  and whose `nextRunAt` the shared `parseTurnTimestamp` parser
  (`apps/web/lib/state/slices/session/turn-actions.ts`) resolves to a non-null
  value, ordered by that value ascending and tiebroken by `id` ascending in
  lexicographic byte order. The first entry is the primary. `parseTurnTimestamp`
  is required; `Date.parse` and `new Date(...)` are not acceptable substitutes.
  AC-001.13 makes the tiebreak total.
- **AC-OFFICE-TRIGGER-WIRE-003.2:** When a primary cron trigger exists, the row
  shall render both its cron expression and its next-fire countdown from that
  same trigger.
- **AC-OFFICE-TRIGGER-WIRE-003.3:** When the routine has at least one `cron`
  trigger but no primary, the routine row shall render the cron expression of the
  trigger with the lowest `id` in lexicographic byte order among all `cron`
  triggers, enabled or not, and shall render no countdown.
- **AC-OFFICE-TRIGGER-WIRE-003.4:** When the routine has no `cron` trigger, the
  row shall render neither a cron expression nor a countdown.
- **AC-OFFICE-TRIGGER-WIRE-003.5:** The routine detail page's last-fired and
  next-fire card shall resolve its trigger exactly as the routine row does: the
  AC-003.1 primary when one exists, otherwise the AC-003.3 fallback, so the list
  and the detail page cannot disagree about which trigger a routine is on. On the
  fallback its last-fired shall come from that trigger and it shall render no
  next-fire value, no primary being precisely the absence of a usable
  `nextRunAt`. With no cron trigger it shall render neither.
- **AC-OFFICE-TRIGGER-WIRE-003.6:** The existing suppression of the next-fire
  value for a non-firing routine shall be preserved, including its two divergent
  inputs, which this capability deliberately does not unify. The list row
  suppresses on `isRoutineFiring(routine.status)`, the persisted status fetched
  with the list; the detail card suppresses on `isRoutineFiring(draft.status)`,
  the live unsaved status control, so the detail countdown responds to that
  control before Save while the list row does not. Unifying them belongs to
  whoever owns routine status. When either gate
  suppresses the countdown, it shall be suppressed whatever the primary
  selection. Last-fired is status-gated on neither surface and shall stay
  ungated.
- **AC-OFFICE-TRIGGER-WIRE-003.7:** The routine detail page's editable cron
  expression and timezone fields shall be seeded from the same trigger AC-003.5
  resolves: the AC-003.1 primary when one exists, otherwise the AC-003.3 fallback.
  With no `cron` trigger the expression shall seed empty and the timezone shall
  take its existing default. Seeding happens when the draft is initialized for the
  loaded routine; a later change to trigger state, including the one trigger sync
  itself makes, shall not re-seed these fields, so unsaved edits survive. Seeding
  by array position is not acceptable. Which trigger *kind* the draft
  starts on depends only on whether a `cron` or `webhook` trigger exists, and is
  unchanged.

### REQ-OFFICE-TRIGGER-WIRE-004: A failed trigger sync does not leave a phantom schedule

**Intent:** Trigger sync deletes before it creates. When the create fails the
routine is left with no schedule while the page still shows the old one, and the
operator's next signal is the routine never running again.

**User story:** As an operator, I want to be told when saving leaves my routine
with no schedule, so I do not walk away believing it is still armed.

#### Acceptance criteria

- **AC-OFFICE-TRIGGER-WIRE-004.1:** When trigger sync deletes a routine's cron
  trigger and the replacement create fails, the system shall remove the deleted
  trigger from the displayed trigger state.
- **AC-OFFICE-TRIGGER-WIRE-004.2:** When that failure occurs, the system shall
  surface an error distinct from the generic save-failure message, and shall choose it
  from the displayed trigger state left by AC-004.1, that being the only state
  observable without a refetch this AC does not require. With no cron trigger left in
  it, the message shall state that the routine now has no cron schedule; with other
  cron triggers left, reachable only through AC-004.7's duplicates, it shall instead
  state that the new schedule was not saved and the routine is still on a previous
  one. Both messages shall be localized through `t()` and added to all five
  locale catalogs.
- **AC-OFFICE-TRIGGER-WIRE-004.3:** When trigger sync's create succeeds, the
  displayed trigger state shall contain the created trigger and not the deleted
  one.
- **AC-OFFICE-TRIGGER-WIRE-004.4:** When the drafted cron expression and timezone
  both equal those of the AC-004.8 sync target, the system shall issue neither a
  delete nor a create. Equality shall be evaluated with `===`, with no defaulting
  applied to the stored trigger's values at comparison time. The drafted timezone shall
  first be resolved per AC-004.12, and that resolved value shall be both the one
  compared here and the one serialized under AC-002.1.
- **AC-OFFICE-TRIGGER-WIRE-004.5:** When the delete itself fails, the system
  shall not issue the create, shall leave displayed trigger state unchanged, and
  shall report the failure.
- **AC-OFFICE-TRIGGER-WIRE-004.6:** When the create returns success but its body
  yields no usable trigger under AC-001.9, the system shall refetch the routine's
  triggers rather than leaving the created trigger out of displayed state. A refetch
  that succeeds but yields no `cron` trigger shall be treated as the failed read-back of
  AC-004.9 rather than as an authoritative empty result: the create succeeded, so an
  empty read-back contradicts it, and accepting it would restore the duplicated schedule
  this AC exists to prevent.
- **AC-OFFICE-TRIGGER-WIRE-004.7:** When two trigger syncs for the same routine
  overlap, the system shall remain deterministic rather than correct: the routine
  may end with more than one cron trigger, and REQ-003's selection shall decide
  which one every surface describes. The delete is idempotent server-side, so both
  creates succeed.
- **AC-OFFICE-TRIGGER-WIRE-004.8:** Trigger sync shall act on exactly one trigger,
  its sync target, the one REQ-003 makes every surface describe: the AC-003.1
  primary when one exists, otherwise the AC-003.3 fallback. When the routine has no
  `cron` trigger there is no sync target and AC-004.10 governs that case. Every
  singular reference in this requirement to a stored, existing or deleted trigger
  means the sync target. Sync shall not delete any other cron trigger, so a routine
  already holding duplicates under AC-004.7 keeps them until a human removes
  them.
- **AC-OFFICE-TRIGGER-WIRE-004.9:** When the AC-004.6 refetch itself fails, the
  system shall leave the created trigger out of displayed state and shall report
  that the routine's schedule could not be read back and the page should be reloaded,
  localized on the same terms as AC-004.2's messages. It shall not report that the
  routine has no cron schedule: the create succeeded, so that claim is false.
- **AC-OFFICE-TRIGGER-WIRE-004.10:** When sync runs under AC-004.11 and the routine
  has no `cron` trigger, so AC-004.8 resolves no sync target, the system shall skip
  the AC-004.4 comparison, issue no delete, and create the drafted cron trigger. An
  absent target is a routine's first schedule being armed, not a reason to do
  nothing. On success AC-004.3 applies with no deleted
  trigger to drop, and an unusable body is AC-004.6's refetch. A create that fails
  here has destroyed nothing, so it shall report the generic save failure, not
  either AC-004.2 message: both describe a schedule that was removed.
- **AC-OFFICE-TRIGGER-WIRE-004.11:** Trigger sync shall run only when the draft's
  trigger kind is `cron` and its drafted cron expression is non-empty after
  surrounding whitespace is trimmed. The trim decides only whether sync runs: the
  expression is serialized as drafted, so trimming shall not change the value
  stored. Otherwise sync shall issue neither a delete nor a create and shall leave
  displayed trigger state unchanged. Clearing the
  expression or switching the draft to `webhook` therefore leaves an existing cron
  trigger in place rather than removing it.
- **AC-OFFICE-TRIGGER-WIRE-004.12:** When the drafted timezone is the empty string,
  sync shall resolve it to the same default AC-003.7 seeds the draft with, before both
  the AC-004.4 comparison and AC-002 serialization, and shall not rewrite the drafted
  value the operator sees. The backend substitutes that default for an empty timezone
  before storing, so an unresolved empty draft can never equal the stored value and
  every later save would delete and recreate the trigger.

## Out of scope

- **Re-tagging the Go DTOs to camelCase.** Rejected. The Office backend is
  uniformly snake_case: the SQLite `db` tags mirror the `json` tags, and `agentctl`
  and the webhook fire path read the current spelling, so re-tagging would move the
  break rather than close it.
- **A global camelCase transform inside `fetchJson`.** Rejected. Several frontend
  types deliberately declare snake_case because that is their contract today:
  `CostBreakdownItem.group_key`, `InboxItem.entity_id` and `Run.agent_profile_id`
  would all silently change shape.
- **The `Routine` and `RoutineRun` wire mapping.** Owned by sibling card
  `9db55db1`, which this capability's module is shaped to accept without a second
  competing module. The audit this card was asked to perform is recorded in the
  design's sibling-card section, including its severe item, not fixed here.
- **Making trigger replacement atomic.** The delete-then-create window stays: a
  create-then-delete order would leave two armed cron triggers across a scheduler
  tick, double-firing the routine. Closing it needs a trigger update endpoint that
  replaces in one transaction, a backend contract change belonging to its own card.
  That same endpoint would serialize two concurrent syncs (AC-004.7).
- **Creating or editing webhook triggers from the UI.** The serializer covers the
  webhook fields, but no UI drives them.
- **Displaying or returning a trigger's `secret`.** The backend redacts it; AC-001.7
  only guarantees the frontend cannot surface a regression.
- **Rendering `nextRunAt` as a parsed date at the API boundary.** It stays a string;
  components parse at render time.
- **Changing the next-fire countdown's copy, its relative-time formatting, or the
  timezone display.** Only the values change.
