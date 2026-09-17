---
status: draft
system: office
requirements:
  - REQ-OFFICE-TRIGGER-WIRE-001
  - REQ-OFFICE-TRIGGER-WIRE-002
  - REQ-OFFICE-TRIGGER-WIRE-003
  - REQ-OFFICE-TRIGGER-WIRE-004
---

# Office Routine Trigger Wire Contract System Design

## Purpose and boundaries

Office owns the routine trigger contract in both directions: the snake_case JSON
`internal/office/routines` emits and binds, and the camelCase domain shape the
web app's routine surfaces read. This design places the translation between them
and says nothing about how triggers are scheduled or fired.

Adjacent contracts used but not owned: `shared.NextCronTime` computes
`next_run_at` and decides which expressions are satisfiable; routine status
gating decides whether a routine may fire and already suppresses the displayed
countdown for a non-firing routine; the shared `fetchJson` helper performs
transport and error classification and is not modified.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-TRIGGER-WIRE-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow) |
| `REQ-OFFICE-TRIGGER-WIRE-002` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |
| `REQ-OFFICE-TRIGGER-WIRE-003` | [Components and responsibilities](#components-and-responsibilities) |
| `REQ-OFFICE-TRIGGER-WIRE-004` | [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- **Routine trigger wire adapter (web, new).** A single module holding the
  read-direction normalizer and the write-direction serializer, alongside the
  existing `office-project-normalize.ts` and `office-activity-normalize.ts`. It
  is the only place in the web app that knows both spellings. Sibling card
  `9db55db1` extends this same module with the routine and run normalizers rather
  than introducing a second one.
- **Office API layer (web, existing).** `listRoutineTriggers` and
  `createRoutineTrigger` apply the adapter at the call site, the way
  `normalizeProject` and `normalizeActivityEntry` are applied today. No consumer
  maps keys.
- **Primary cron trigger selector (web, new).** One shared function implementing
  REQ-003's ordering. The routines list row and the routine detail page's
  read-only card both call it, which is what keeps them from disagreeing, and
  trigger sync resolves its target through it (AC-004.8) so the trigger being
  replaced is the trigger being displayed. It is pure over a trigger array.
  Time validity and ordering use the shared `parseTurnTimestamp`
  (`apps/web/lib/state/slices/session/turn-actions.ts`), not `Date.parse` or
  `new Date(...)`: it returns `bigint | null`, rejecting malformed and
  calendar-invalid values rather than normalizing them, and it is already the
  house parser for untrusted RFC3339 outside the session slice.
  `apps/web/CLAUDE.md` states that rule; the `nextFireText` helper being replaced
  does not follow it, which is why AC-003.1 names the parser rather than leaving
  it to the builder. The status gates
  feeding the two surfaces stay as they are and stay different (AC-003.6); the
  selector decides which trigger, never whether the countdown is shown.
  FOUR call sites resolve a cron trigger and all four go through the selector: the
  list row's expression and countdown, the detail page's read-only card, the
  detail page's EDITABLE cron and timezone fields (AC-003.7), and trigger sync
  (AC-004.8). The editable fields are the one that is easy to miss, because they
  look like presentation rather than selection. They are not: their seeded values
  are one operand of AC-004.4's comparison against the sync target, so seeding
  them by array position while the target comes from the selector would compare
  two different triggers and let an untouched routine replace the target's
  schedule with another trigger's. A fifth site, the draft's initial trigger
  KIND, asks only whether a `cron` or `webhook` trigger exists at all. That
  question is independent of ordering, so it stays as it is; rewiring it through
  a cron-only selector would be a change with no defect behind it.
- **Routine trigger handler and service (backend, unchanged).** `CreateTriggerRequest`
  keeps `cron_expression`; `CreateRoutineTrigger` keeps rejecting an empty or
  unsatisfiable expression; `redactTriggerSecrets` keeps clearing `secret`.

## Data and contracts

The wire shape is `models.RoutineTrigger`'s JSON tags. Fields whose two
spellings differ: `cron_expression`, `next_run_at`, `last_fired_at`, `public_id`,
`signing_mode`, `routine_id`, `created_at`, `updated_at`. Fields already
identical, and therefore working today: `id`, `kind`, `timezone`, `enabled`.
`secret` exists on the wire and not in the domain type.

None of the affected Go fields carry `omitempty`, so every key is present on the
wire; the pointer fields `next_run_at` and `last_fired_at` serialize as `null`
when unset rather than being omitted. The normalizer treats `null` and absent
identically, which is what lets it also accept an already-normalized object.

The normalizer reads camelCase first and falls back to snake_case per field,
matching `normalizeProject`. That fallback is what makes it idempotent, and
idempotency is required because the detail page pushes a normalized created
trigger into the same array a later fetch repopulates.

The fallback is NULLISH, not merely key-presence: `record[camelKey] ??
record[snakeKey]` consults the snake_case key whenever the camelCase one is
absent OR holds `null`. AC-001.2 is worded to match that exactly, because the two
readings diverge on a mixed object carrying `cronExpression: null` beside a
populated `cron_expression`, and a key-presence reading would drop the value the
backend actually sent. The nullish reading is also what keeps AC-001.3 true: a
normalized trigger carries no snake_case keys at all, so every lookup falls
through to `undefined` and the camelCase value stands.

AC-001.5 keeps `""` distinct from `undefined` so AC-001.12's guarantee holds
without exception: every string-valued field except the two timestamps is a string
on every normalized trigger, which is what lets AC-004.4 compare with a bare `===`.

An earlier draft of this design justified AC-001.5 by a *stored* cron trigger whose
`timezone` is `""`. That path does not exist. `RoutineService.CreateRoutineTrigger`
substitutes the default for an empty timezone before persisting, on every kind, and
the one caller that bypasses the service (`infra/reconcile.go`) creates `manual`
triggers, which are never an AC-004.8 sync target. The reachable asymmetry runs the
other way and is the reason AC-004.12 exists: see below.

AC-004.12 exists because the comparison has a side the frontend does not control.
The timezone control is a free-text input whose `UTC` is a placeholder, not a value,
so clearing it leaves the draft holding `""`. Serialize that unresolved and the
backend stores the default instead, and the draft can never again equal the stored
value: AC-003.7 deliberately does not re-seed the draft from the trigger sync itself
creates, so the divergence is permanent for the page's lifetime and every later save
would delete and recreate the cron trigger, churning its `id` and `next_run_at`.
Resolving the drafted timezone once, on the draft side, before both the comparison
and serialization, is what removes that: the stored side stays raw, so a genuine
normalization difference is still detected, while the two operands are expressed in
the same vocabulary. Resolution never rewrites what the operator sees.

The normalizer constructs its result from an explicit field list and never
spreads the wire object. This is the one deliberate departure from
`office-task-normalize.ts`, and its reason is `secret`.

The serializer takes a create-input type declared for the purpose rather than
derived from `RoutineTrigger`, carrying exactly `kind` plus the optional strings
`cronExpression`, `timezone`, `publicId`, `signingMode` and `secret` (AC-002.7).
A derivation is not available in either direction: `secret` is on the input and
deliberately absent from the domain type, and `routineId` is required on the
domain type and absent from the input because the routine is named by the path
argument. The serializer emits only the keys the caller supplied, where supplied
means present and not `undefined`, so an explicit `""` is emitted and an omitted
field is absent (AC-002.3). `createRoutineTrigger`'s parameter changes from
`Partial<RoutineTrigger>` to that input type, which is what makes the current
camelCase call sites in `routines-content.tsx` and `routine-detail-view.tsx` fail
to compile until they are updated rather than silently continuing to send the
wrong shape.

The domain type changes in exactly one other way: `kind` widens from
`RoutineTriggerKind` to `string` so AC-001.11's pass-through of an unrecognized
value is expressible without a cast. `RoutineTriggerKind` stays exported as the
set of values consumers compare against; it is referenced nowhere else in the app
today, and every read of `kind` is an `===` comparison that is unaffected.

Every string-valued field except the two nullable timestamps coerces a missing,
`null`, or wrong-typed wire value to `""` (AC-001.12): the required `id`,
`routineId`, `createdAt` and `updatedAt`, and the optional `cronExpression`,
`timezone`, `publicId` and `signingMode` alike. The coercion matches
`office-project-normalize.ts`'s `stringField`. It is what lets AC-004.4 compare
with a bare `===` and no per-field defaulting, which would otherwise be undefined
behavior for an optional field holding `null`. A trigger whose `id` coerces to
`""` is dropped on both the list and create paths (AC-001.13), which makes `id` a
total tiebreak for the selector instead of a key that can collide across
malformed rows.

An unrecognized `kind` is carried through rather than dropped or cast
(AC-001.11): dropping would hide a row that exists in the database, and a cast
would let an unrecognized value be read as a declared one.

## Control flow

Read: handler emits snake_case, `fetchJson` parses, the API function maps the
body's `triggers` array or `trigger` object through the normalizer, components
receive the domain shape. Nothing downstream of the API function sees a wire key.

Write: a component builds a create input in domain spelling, the API function
runs it through the serializer, and the snake_case body reaches
`ShouldBindJSON`. `CronExpression` binds, `CreateRoutineTrigger` computes
`next_run_at`, and the 201 response is normalized on the way back out through the
same read path.

## Failure and recovery

Trigger sync keeps its delete-then-create order; making it atomic is out of
scope and would require a backend update endpoint. What changes is honesty about
the window. A failed create leaves the routine with no cron trigger, so the
displayed trigger state drops the deleted trigger and the error names that
outcome specifically rather than reporting a generic save failure. A failed
delete stops the sequence before the create and leaves displayed state untouched,
so the routine keeps the schedule it had.

Sync has two entry conditions and one of them is easy to drop while rewiring it.
It runs at all only when the draft is a cron draft with a non-blank expression
(AC-004.11); otherwise it touches nothing. That guard is what keeps a cleared
expression from deleting the trigger and then being refused by the backend for an
empty `cron_expression`, which would disarm the routine as a side effect of a save
the operator thought was harmless. Removing a schedule is simply not expressible
from this page, and AC-004.11 says so rather than leaving it to be inferred.

Given it runs, the routine may still have no cron trigger, so AC-004.8 resolves no
sync target. That is not a no-op: it is a routine's FIRST schedule being armed,
and it is the flow REQ-002 exists to make work. Sync skips the comparison, skips
the delete, and creates (AC-004.10). The current implementation gets this right
incidentally, by guarding the compare and the delete behind the same `existing`
binding it uses for array-position lookup; once that lookup becomes a selector
that can legitimately return nothing, the branch has to be stated rather than
inherited. A failed create on this path has deleted nothing, so it reports the
generic save failure: both AC-004.2 messages describe a schedule that was removed
and would be false here.

Sync resolves which trigger it is replacing through the same selector the
surfaces use (AC-004.8), and compares the draft against it with `===`, never
defaulting the stored side at comparison time: doing so would mask a genuine
normalization difference the comparison exists to detect. The DRAFTED timezone is
resolved first (AC-004.12), which is a different thing from defaulting the stored
value and is required for the comparison to be able to converge at all.

A create that succeeds with an unusable body is the one case the adapter cannot
resolve locally, because refusing to fabricate an entry would hide a trigger that
does exist. The detail page refetches the routine's triggers instead, which is
also what keeps a subsequent save from creating a second schedule: without the
refetch the next save would find no existing trigger, skip the delete, and create a
second one, converting one unusable response into a duplicated schedule.

The refetch has a second failure mode that is easy to miss because it arrives as a
success. AC-001.8 degrades a malformed list body to an empty array, so a 200 whose
`triggers` is absent or not an array reads exactly like a routine with no triggers.
Accepting it would drop the trigger the create just made and re-arm the duplicate
this path exists to prevent, silently and with no error. AC-004.6 therefore treats a
read-back that yields no `cron` trigger as AC-004.9's failed read-back rather than as
an authoritative empty result: the create succeeded, so an empty answer contradicts
known truth and the honest report is that the schedule could not be read back. If that
refetch itself fails the page says the schedule could not be read back and asks
for a reload (AC-004.9); it does not claim the routine has no schedule, because
the create did succeed and that reassurance would be false in the opposite
direction.

Routine creation from the dialog is a second, separate two-step write, and REQ-004's
terminology deliberately does not cover it: trigger sync is the detail page's save
path. The dialog creates the routine, then creates its trigger. If the second call
fails the routine is already persisted, and the failure is not a failed create of the
routine at all. AC-002.8 governs it, because a generic "failed to create routine"
toast on a routine that WAS created invites the operator to retry and persist a second
one, and because leaving the dialog open hides the routine that now exists. The dialog
closes, the list refreshes so the schedule-less routine is visible, and the message
says what actually happened. Arming its schedule afterwards is the detail page's
no-sync-target path (AC-004.10), which is why that branch had to be stated rather than
inherited. This is the same honesty REQ-004 requires, applied to the other write that
can half-succeed.

The error text is chosen from the state the routine is actually in, not from the
branch that failed: "no cron schedule" only when no cron trigger remains, and
"the new schedule was not saved" when duplicates leave one behind (AC-004.2).

Two overlapping syncs of one routine are not serialized. The backend delete is
idempotent, so both creates land and the routine ends with two cron triggers. The
primary-trigger selector is what contains that: every surface picks the same one
deterministically, so the outcome is a redundant trigger rather than an
inconsistent display.

Malformed read payloads degrade rather than fabricate: a non-object list element
is dropped, a non-array `triggers` yields an empty array, and a create response
without a usable `trigger` adds nothing to caller state. A missing `enabled`
resolves to `false`, so an unreadable armed state renders no countdown instead of
an unfounded one.

## Rationale relocated from the requirements

The requirements file is capped at 20480 bytes with no waiver available, so
justification prose lives here while the normative clause stays there. These are the
reasons behind clauses that now read flatly:

- **AC-001.3 (idempotency).** Trigger sync pushes an already-normalized created
  trigger into the same array a later fetch repopulates, so the normalizer must accept
  its own output.
- **AC-001.6 (`enabled` defaults false).** Absence means the armed state is unknown,
  and the UI must not promise a fire it cannot confirm.
- **AC-001.13 (drop an empty `id`).** `id` is the delete target and REQ-003's
  tiebreak column, so dropping guarantees every trigger a caller receives has a
  non-empty `id`, which makes AC-003.1's and AC-003.3's orderings total.
- **AC-002.2 (no camelCase keys).** Emitting both spellings would leave the defect
  latent: the backend ignores the unknown key, so the bug would survive the fix.
- **AC-002.6 (server-owned fields).** `enabled` is hardcoded true in the handler and
  `next_run_at` is computed from the expression.
- **REQ-003's intent.** Both surfaces render blank today, so fixing the mapping is
  what makes the row-vs-countdown disagreement visible; this capability owns it.
- **AC-003.7 (no array-position seeding).** The seeded values are one operand of
  AC-004.4's comparison against the AC-004.8 sync target. On a routine holding the
  duplicates AC-004.7 permits, seeding by position would compare two different
  triggers and let an untouched routine replace the target's schedule with another
  trigger's.
- **AC-004.7 (concurrent syncs).** Nothing serializes the two; preventing the
  duplicate needs the out-of-scope update endpoint.
- **AC-004.10 (no sync target).** A no-op would leave the detail page unable to arm a
  schedule at all, which is the defect REQ-002 exists to remove.

## Persistence

None. No schema, migration or storage change. `office_routine_triggers` and its
`db` tags are untouched, and the adapter holds no state across calls.

## Security

`secret` is redacted server-side on both the list and create responses today.
The normalizer's allow-list construction is defense in depth: a regression in
redaction cannot put a signing secret into component state or a React tree,
because the field is never copied. No trust boundary moves and no permission
changes.

## Observability

None added. The defect's signal was already visible without instrumentation: a
blank cron column and a 400 on trigger create. Regression coverage is unit tests
over the adapter and the selector, which is where a case-mapping break should
fail rather than in the running app.

AC-002.5 is the one criterion no adapter unit test can observe, because it
asserts the backend's 201 and a `next_run_at` the backend computed. It is
verified by a Playwright spec that creates a routine with a cron expression
through the create-routine dialog and asserts the resulting row renders that
expression and a countdown. That spec is the reason this capability cannot be
closed on unit tests alone: no existing office routine spec creates a cron
trigger through the UI, which is exactly why a 400 on every create reached a
shipped build unnoticed.

## Sibling card 9db55db1: the routine and run wire audit

This card was asked to audit the sibling office DTOs; the result is recorded here
rather than in the requirements, because it is scope for card `9db55db1` and not
a contract this capability implements.

`Routine` mismatches on `workspaceId`, `taskTemplate`, `assigneeAgentProfileId`,
`concurrencyPolicy`, `catchUpPolicy`, `catchUpMax`, `lastRunAt`, `createdAt` and
`updatedAt`. `RoutineRun` mismatches on `routineId`, `triggerId`, `triggerPayload`,
`linkedTaskId`, `coalescedIntoRunId`, `dispatchFingerprint`, the three `catchUp*`
fields, `startedAt`, `completedAt` and `createdAt`.

More severely, and unlike anything in this card's scope, the frontend types
`Routine.taskTemplate` and `Routine.variables` as objects while the wire carries
JSON strings, and the create dialog posts `taskTemplate: <object>`. So
`CreateRoutineRequest.TaskTemplate` binds empty and routines are created with a
blank task template. That is a distinct defect from a case mismatch: mapping the
key alone will not fix it, since the two sides disagree about the TYPE, and it
needs its own handling on that card.

## Prior art and rejected alternatives

**Two external legs were unavailable, and are recorded as unavailable rather
than as empty results.** `wiki-query @henry` is not installed (`~/.claude/skills/`
has no entry, `which wiki-query` exits 1) and no vault is configured
(`~/.obsidian-wiki/` does not exist), so no QMD collection was queried and no
grep fallback existed. No `saas-kb` MCP server is exposed and no
`search_fsm_docs` tool is callable, so no cross-product query was issued; the
gap is low-cost, since a case-mapping boundary inside one app's own API layer is
not a decision other vendors' behavior would inform.

**In-repo prior art (searched `apps/web/lib/api/domains/*-normalize.ts` and
`grep -rn "camelCase\|snake_case" docs/decisions/ docs/specs/`).** This
substitute leg decided the design and found a settled house position: normalize
on the frontend, per endpoint, in the API layer. Four normalizers already exist.
`office-project-normalize.ts` is the closest match, reading
`record[camelKey] ?? record[snakeKey]` per field into an explicit result object;
`office-task-normalize.ts` adds `canonicalStatusesToBackend` as the
write-direction inverse in the same module; `office-api.ts` applies them at the
call site. No ADR states a repo-wide JSON case rule;
`docs/specs/platform/system-design/agent-settings-domains.md` says only that
existing camelCase keys stay camelCase where that is their public contract.

**What this does differently.** Nothing structural. The one departure is
allow-list construction rather than spreading the wire object, which
`office-task-normalize.ts` does do: the wire carries a `secret` the domain type
does not declare, and a spread would leak it into component state.

## Related decisions

No ADR governs JSON case conventions across Kandev. This design follows the
established per-endpoint frontend normalizer pattern rather than establishing a
repo-wide rule, and records that choice in the requirements' out-of-scope
section.
