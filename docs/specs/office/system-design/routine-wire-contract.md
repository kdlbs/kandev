---
status: draft
system: office
requirements:
  - REQ-OFFICE-ROUTINE-WIRE-001
  - REQ-OFFICE-ROUTINE-WIRE-002
  - REQ-OFFICE-ROUTINE-WIRE-003
  - REQ-OFFICE-ROUTINE-WIRE-004
---

# Office Routine Wire Contract System Design

## Purpose and boundaries

The Office system publishes the routine HTTP contract, so it owns the
translation between that contract and the web client's model shape. This
design places that translation in one adapter at the client's API boundary and
forbids it anywhere else.

Adjacent contracts this design uses but does not own: the routine HTTP
endpoints and DTOs (`apps/backend/internal/office/routines`), the routine
models (`apps/backend/internal/office/models`), and the shared fetch helpers
(`apps/web/lib/api/client.ts`). None of them change.

## Terminology

These terms are used throughout, here and in the requirements, which reference
this section rather than restating them.

- **Wire shape:** the JSON object the backend emits or binds, snake_case, with
  `task_template` and `variables` carried as JSON-encoded strings.
- **Model shape:** the camelCase TypeScript types in
  `apps/web/lib/state/slices/office/types.ts` that components consume.
- **Adapter:** the single translation point between wire shape and model
  shape, in both directions.
- **Envelope:** the single-key object a routine endpoint wraps a resource in
  (`{"routine": ...}`, `{"trigger": ...}`, `{"run": ...}`, `{"routines": [...]}`).
- **Arm:** create a cron trigger with a satisfiable expression, so the routine
  will fire unattended.
- **Patch field:** a field the caller supplied to an update call; one the
  caller did not supply is not a patch field.
- **Effective timezone:** a trigger's `timezone` with absent, `null` and the
  empty string all read as `UTC`, matching `CreateRoutineTrigger`, which
  rewrites an empty `Timezone` to `UTC` before storing it. Two effective
  timezones are equal when their strings match exactly: no case folding, no
  alias resolution, no offset comparison.
- **Trimmed expression:** a cron expression with leading and trailing
  whitespace removed (AC-002.14).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-ROUTINE-WIRE-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow) |
| `REQ-OFFICE-ROUTINE-WIRE-002` | [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |
| `REQ-OFFICE-ROUTINE-WIRE-003` | [Data and contracts](#data-and-contracts), [Components and responsibilities](#components-and-responsibilities) |
| `REQ-OFFICE-ROUTINE-WIRE-004` | [Data and contracts](#data-and-contracts) |

## Components and responsibilities

- **Routine wire adapter** (`apps/web/lib/api/domains/`): the only code that
  knows a routine key is spelled `catch_up_policy`. Read direction normalizes
  wire to model; write direction builds the request body from the model. It
  maps declared fields explicitly and never spreads the raw object, so an
  unmapped wire key such as `secret` cannot reach the store.
- **Routine API functions** (`office-api.ts`): call the adapter on the way out
  and on the way back, and unwrap the response envelope so callers receive a
  resource or an array.
- **Routine components** (`apps/web/app/office/routines/`): consume model shape
  only. `routine-row.tsx`'s two snake_case fallback casts and four envelope
  casts are removed, because the adapter now guarantees what they were
  defending against. The envelope casts are in `routines-content.tsx`,
  `app/office/routines/[id]/page.tsx`, `src/office-routine-client-routes.tsx`
  and `[id]/routine-detail-view.tsx`. The fourth is the one that fails silently
  if it is missed: `syncCronTrigger` ends `.trigger ?? null` where the other
  three end in a bare-resource fallback (`?? (x as Resource)`), so against an
  unwrapped return it yields `null`, the created trigger is dropped from local
  state, and the page shows no schedule after an arm that succeeded. AC-003.10's
  second negative is what catches it.
- **Office store slice**: unchanged. `setRoutines` is the only ingress and its
  only feeder is `listRoutines`; the Go boot payload ships `"routines": []`
  and no WebSocket event carries a routine, so the adapter has no bypass.

This mirrors the agent profile boundary in the same file (`normalizeAgent`,
`agentPayload`, `stringifyJSONField`) and `normalizeProject`. It follows
ADR-0036: a protocol shape is normalized
at the adapter and the frontend renders only the normalized contract.

## Data and contracts

Read direction. Left is the wire key, right is the model field.

| Routine | Model | Note |
| --- | --- | --- |
| `id`, `name`, `description`, `status` | same | single-word; `status` carries `""` unchanged (AC-003.14) |
| `workspace_id` | `workspaceId` | |
| `task_template` | `taskTemplate` | JSON string, parsed to object |
| `assignee_agent_profile_id` | `assigneeAgentProfileId` | |
| `concurrency_policy` | `concurrencyPolicy` | passthrough, no default substituted |
| `catch_up_policy` | `catchUpPolicy` | passthrough |
| `catch_up_max` | `catchUpMax` | number, `undefined` when absent, never clamped client-side |
| `variables` | `variables` | JSON string, parsed to object |
| `last_run_at` | `lastRunAt` | `null` maps to `undefined` |
| `created_at`, `updated_at` | `createdAt`, `updatedAt` | |

| Trigger | Model |
| --- | --- |
| `routine_id` | `routineId` |
| `cron_expression` | `cronExpression` |
| `public_id` | `publicId` |
| `signing_mode` | `signingMode` |
| `next_run_at` | `nextRunAt` |
| `last_fired_at` | `lastFiredAt` |
| `secret` | not mapped |
| `created_at`, `updated_at` | `createdAt`, `updatedAt` |
| `kind`, `timezone`, `enabled`, `id` | direct |

| Run | Model |
| --- | --- |
| `id`, `source`, `status` | direct |
| `routine_id`, `trigger_id` | `routineId`, `triggerId` |
| `trigger_payload` | `triggerPayload` |
| `linked_task_id` | `linkedTaskId` |
| `coalesced_into_run_id` | `coalescedIntoRunId` |
| `dispatch_fingerprint` | `dispatchFingerprint` |
| `catch_up_missed_ticks` | `catchUpMissedTicks`, `undefined` when absent |
| `catch_up_first_missed_at` | `catchUpFirstMissedAt` |
| `catch_up_truncated` | `catchUpTruncated` |
| `started_at`, `completed_at`, `created_at` | `startedAt`, `completedAt`, `createdAt` |
| `skip_reason`, `pause_id`, `causation_id` | not mapped, not rendered |

Model type changes (`types.ts`). The read mapping above cannot be written
without a cast against today's types, and AC-003.16 forbids the cast, so three
declarations widen to `string`. The server validates none of the three on
write, so each closed union is a claim the wire does not honour (AC-003.16):

- `Routine.status` becomes `string`. `models.Routine.Status` is a plain Go
  string, `applyRoutineUpdates` assigns the patch pointer unguarded, and
  `CanFire()` treats `""` as firing, so both `""` (AC-003.14) and an arbitrary
  `unknown` written by the API, agentctl or config sync are reachable.
  `RoutineStatus` stays as the union the client writes (`UpdateRoutinePatch`),
  the direction the client does control. `Routine.status` has three consumers,
  not one, and two of them need a change:
  - `isRoutineFiring(status: string)` already takes a plain string and is
    correct as written.
  - `CoordinatorRoutineHint` (`app/office/agents/[id]/layout.tsx`) tests
    `r.status === "active"` directly, bypassing `isRoutineFiring`, so a firing
    `""` routine would read as not-firing. It moves to `isRoutineFiring`
    (AC-003.18). That surface changes whether we touch it or not: the same
    expression tests `r.assigneeAgentProfileId === agentId`, `undefined` for
    every routine today (AC-003.19).
  - `buildDraft` (`routine-detail-view.tsx`) narrows with
    `(routine.status as DraftState["status"]) ?? "active"`, and `??` catches
    neither. The cast goes; AC-003.17's unset control replaces it.
- `RoutineRun.status` becomes `string` the same way (AC-004.5). Blast radius is
  nil: `RoutineRunStatus` has one reference, and `run-row.tsx`'s two lookup
  tables are typed `Record<string, string>` with an `?? run.status` fallback, so
  an unrecognized status renders as an unlabeled badge and needs no new copy.
- `RoutineTrigger.kind` becomes `string`. `CreateRoutineTrigger`
  (`routines/service.go`) validates `cron_expression` for a cron trigger and
  never validates `kind`. `RoutineTriggerKind` stays as the union the client
  writes (`CreateTriggerInput.kind`). The `kind === "cron"` comparisons in `routine-detail-view.tsx` are unaffected.

No widening changes a rendered value; each makes the declared type match what
the wire can carry. No exhaustive `switch` exists over any of the three unions.
Widening rather than casting is the point: a cast at the adapter would defeat
AC-003.9's explicit-mapping discipline, which is what keeps `secret` out of the
store.

Write-side input types (`types.ts`, next to the model types). The write mapper
takes these rather than `Partial<Routine>`: the model's `taskTemplate` is a
`Record<string, unknown>` and cannot express AC-001.5's clearing empty string,
and `RoutineTrigger` does not declare the `secret` AC-002.1 requires:

- `CreateRoutineInput`: `name`, and optional `description`, `taskTemplate`,
  `assigneeAgentProfileId`, `concurrencyPolicy`, `catchUpPolicy`, `catchUpMax`,
  `variables`. No `status` and no `workspaceId` (AC-001.6).
- `UpdateRoutinePatch`: every `CreateRoutineInput` field optional, plus
  `status`. This is the key set AC-001.9 fixes.
- `CreateTriggerInput`: `kind`, and optional `cronExpression`, `timezone`,
  `publicId`, `signingMode`, `secret`.

`taskTemplate` and `variables` are typed `Record<string, unknown> | string` on
both input types, so an operator-cleared template is an expressible `""` rather
than a cast.

Write direction. A field the caller did not supply is absent from the body;
`JSON.stringify` drops an `undefined` value, so an explicitly supplied empty
string still serializes and still clears the stored value. `task_template` and
`variables` are encoded with the existing `stringifyJSONField` shape: an object
becomes its JSON text, a string passes through, `undefined` stays `undefined`.
An empty `concurrencyPolicy` or `catchUpPolicy` is dropped rather than sent,
because `Valid()` rejects it with a 400 (AC-001.10); those two are the only
fields where empty means omit rather than clear.

Everything else `UpdateRoutineRequest` types as a pointer clears on an empty
string, and that is six fields rather than AC-001.5's three: `name`,
`description`, `task_template`, `assignee_agent_profile_id`, `status` and
`variables` (`dto.go`, applied by `applyRoutineUpdates` as an unguarded
nil-check-then-assign). `status` is the consequential one: an empty status is stored unvalidated and
`CanFire()` treats it as firing, so AC-001.11 forbids sending one and AC-003.17
keeps an unset control from producing one. `catch_up_max` is a `*int`, so `0`
and negatives are storable and distinct from absent; the client sends neither
(AC-001.11, and [Failure and recovery](#failure-and-recovery) for why the
control is not the mechanism).

Envelopes. `listRoutines` unwraps `routines`; `getRoutine`, `createRoutine` and
`updateRoutine` unwrap `routine`; `createRoutineTrigger` unwraps `trigger`;
`listRoutineTriggers` unwraps `triggers`; `runRoutine` unwraps `run`;
`listRoutineRuns` and `listAllRoutineRuns` unwrap `runs`. The unwrap reads the
named key and nothing else. There is no bare-resource tolerance: a body that is
already the resource carries no envelope key, so it takes the missing-key
branch. Tolerating one would need a shape discriminator to tell a bare routine
from a malformed body, since neither has the key, and getting it wrong is the
same silent skip the next paragraph closes. Nothing on this contract sends a
bare resource: every routine response is enveloped by `RoutineResponse`,
`RoutineListResponse`, `TriggerResponse`, `TriggerListResponse`,
`RoutineRunResponse` or `RunListResponse` (`routines/dto.go`).

The two directions fail differently. A list unwrap that finds a missing key, a
`null` payload or a non-array payload yields the empty array, which every list
caller already renders (AC-003.5). A single-resource unwrap throws on those same
three cases and on an object carrying no non-empty `id`, because its callers
read fields off the result: `createRoutine`'s caller reads the new routine's
`id` to arm the trigger, and `{}` would otherwise pass the object check, produce
`id: ""` and arm `/routines//triggers` - the silent skip AC-003.15 exists to
prevent.

Return types change from the enveloped shape to the resource shape, so the
`as unknown as` casts at the call sites listed above disappear. This is a
compile-time change inside the web app; no other package imports these
functions.

## Control flow

Create routine: dialog state, then the write mapper, then `POST
/workspaces/:wsId/routines`, then the read normalizer on the 201 body, then the
routine id for the optional trigger step. Workspace comes from the path and
status is forced server-side, so neither is in the body.

Save routine: the write mapper over the patch fields only, then `PATCH
/routines/:id`, then the cron reconciliation below. The list toggle is the same
path with a one-key patch (AC-001.8), which AC-001.9's key set admits.

Trigger state after reconciliation comes from `listRoutineTriggers`, not from
splicing the local array (AC-002.11). That is what makes AC-002.8's "the order
the list endpoint returned" true after a replacement, and how AC-002.7 learns
whether a failed delete left the superseded trigger behind. It costs one extra
GET per reconciliation that changed something; an unchanged save issues none.

Trigger equality for AC-002.4 compares the expression exactly and the effective
timezone, where absent, `null` and `""` all read as `UTC`. The server normalizes
the same way on the way in, so a stored `UTC` and an unset draft are the same
schedule and must not churn the trigger.

Cron emptiness is decided on the trimmed expression, and the trimmed string is
what is sent (AC-002.14). Three live sites disagree today:
`create-routine-dialog.tsx`'s step gate trims, `routines-content.tsx`'s arm step
gates on raw truthiness, and `syncCronTrigger` trims the gate but transmits
`draft.cronExpression` untrimmed. That disagreement is invisible while the key
is dropped and observable once it transmits: `"   "` becomes a silent no-op on
one path and a server 400 toast on another, and an untrimmed stored expression
compares unequal to the trimmed draft on every later save (AC-002.4). No stored
expression can carry stray whitespace from this UI today; one written by the API
or by config is replaced on the next save, which is correct rather than churn. One trim at the boundary settles all three sites.

Status seeding. `buildDraft` seeds the status control from `routine.status`,
where `?? "active"` catches neither `""` nor an unrepresentable stored value.
Either leaves the control unselected and contributes no `status` patch field
unless the operator picks one, which is AC-001.4's rule applied rather than a
new one (AC-003.17). A `""` routine still reads as firing everywhere firing
state is shown, because those read `isRoutineFiring`. The `<SelectValue />`
gains no placeholder text: neither value is reachable through this UI, and
inventing a placeholder would mean a new key in five locales for a state the
operator cannot create. Coercing the display to `active` instead would write
`active` back on the next save and commit a value no writer set, turning a
read-only visit into a mutation.

Cron reconciliation replaces its current delete-then-create order with
create-then-delete. The reason is a new failure mode this fix introduces: once
`cron_expression` actually transmits, the server validates it with
`shared.NextCronTime` and answers 400 on a bad expression. Under the old order
the superseded trigger is already gone by then, so a typo costs the operator a
working schedule. Under the new order a rejected create leaves the existing
trigger armed. The cost is a window in which both triggers exist, which a
delete failure leaves open and AC-002.7 requires be reported rather than
swallowed. That toast is the only signal, which is why it carries the count:
the page shows neither the duplicate nor a way out, because AC-002.8 displays
the first cron trigger by `created_at` - after a failed delete, the superseded
one - and `deleteRoutineTrigger`'s only call site is this reconciliation, which
swaps which trigger is orphaned rather than removing one. The routine fires
twice per tick until the extra is deleted through the API. Still the right
trade: a duplicate fire is recoverable, a lost schedule is not.

Reads: server component loader, client route loader and list fetch all call the
same API functions, so all three get model shape with no per-caller unwrap.

## Failure and recovery

- A non-2xx response rejects through the existing fetch helper. Callers keep
  their `try`/`catch` and their toasts. The adapter swallows nothing.
- A malformed `task_template` or `variables` string yields an empty object.
  A routine with corrupt template JSON renders with empty template fields
  instead of failing the page.
- A trigger create rejected by `ErrInvalidTrigger` surfaces the server's
  message, which names the bad expression. No local cron parser is added; the
  server owns cron validity.
- `listRoutineTriggers` already has a per-routine `.catch` returning an empty
  list. That stays: one routine's trigger fetch failing must not blank the list.
- The create-routine flow is two calls. A trigger failure after a successful
  routine create leaves an unscheduled routine, reported rather than undone
  (AC-002.10). The same split applies to the detail save (AC-002.9).
- The four partial-failure paths each have a named observable, because
  "surface the error" is not one. A rejected replacement create (AC-002.6) and
  a rejected create-flow trigger (AC-002.10) show the server's message, and
  AC-002.10 also closes the dialog and refreshes the list rather than inviting a
  second create. A failed delete (AC-002.7) and a failed trigger step after a committed routine
  update (AC-002.9) both re-list first and report what that list shows. None of
  the four shows a success toast.
- AC-002.7, AC-002.9 and AC-002.10 need copy that does not exist today: each
  path is `toast.error(err.message)` now, and the server's message cannot say
  which half of a two-call flow succeeded. That is new `office`-namespace copy,
  so it carries the five-locale obligation (`pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`
  alongside English, `pnpm run i18n:zh-hant` for the Traditional pair) and the
  new-code ratchet enforces it. AC-002.12 states it so the build does not meet
  it as a surprise.
- AC-003.15's rejection throws from the adapter, a plain `.ts` file, where
  `scripts/check-nonjsx-copy.mjs` scans internal error strings as copy. The
  message is either translated or carries an `// i18n-exempt:` line comment; a
  bare literal fails the ratchet rather than review.
- `catch_up_max` is normalized by the server at use, not by the client. The
  control's `min={1}` is not what keeps a sub-1 value off the wire. The detail
  view and the create dialog both carry this control, neither saves through a
  form submit, and both spell the handler `Number(e.target.value) || 25`, which
  catches only `0` and `NaN`. Each form supplies the coercion, using 25 below 1
  (AC-003.13), and the write mapper omits a below-1 value (AC-001.11).

## Concurrency, retry and ordering

These are properties of the server contract the client cannot change, stated so
no implementer has to invent them; each names the residual it leaves rather
than a mitigation.

- **Retry:** the adapter adds no retry. Routine calls keep `fetchJson`, not
  `fetchJsonWithRetry`.
- **Create is not idempotent:** the create endpoints carry no idempotency key,
  so a retried create produces a second routine or a second trigger. The
  adapter does not add one.
- **Last write wins, over the whole row:** routine update has no
  optimistic-concurrency token, and the server's update is read-modify-write
  rather than a column-scoped patch: `doUpdateRoutine` (`routines/handler.go`)
  reads the routine, applies the patch in memory, and calls `UpdateRoutine`,
  whose statement (`repository/sqlite/routines.go`) writes every routine column
  from that struct. Two concurrent savers therefore overwrite each other's
  fields even when their patches are disjoint, and what survives is whichever
  database write lands second, not whichever response arrives second. AC-001.4
  narrows the request body, not the write. Removing the residual needs the
  optimistic concurrency the requirements exclude.
- **Concurrent trigger creates:** two savers replacing the same cron trigger
  can leave two triggers. This is the same residual as AC-002.7 and is
  surfaced, not prevented.
- **Server ordering:** routines are `ORDER BY name`, triggers `ORDER BY
  created_at`, runs `ORDER BY created_at DESC`. None carries a tiebreak column,
  so two rows with an equal sort key have an order this contract does not
  define. The client preserves whatever order it receives (AC-003.8). For
  triggers the consequence is stronger than display order: AC-002.8 replaces the
  first cron trigger, so two stored in the same clock tick leave which one gets
  replaced undefined. Closing that needs the tiebreak column the requirements
  exclude; AC-002.11 confines it to a real endpoint response, not a local array.

## Persistence

None. This is a client-side serialization boundary: no schema, no migration, no
stored state change. The observable difference is that values the operator
already intended to store now arrive.

## Security

`secret` is redacted to `""` by `redactTriggerSecrets` before any list
response, and the model does not declare it. Explicit field mapping, rather
than an object spread, is what keeps that true if the redaction ever regresses.
No routine field is a credential; no new field is exposed.

## Observability

No new metrics or logs. The existing `routing_*` and routine counters are
backend-side and unaffected.

## Testing

Vitest at the adapter is where the contract is pinned, because that is the
boundary the defect lives at:

- serialized request body keys for routine create, routine update and trigger
  create, asserting the exact key set and that no camelCase key is present
- update omits an unsupplied field and transmits an explicitly empty string
- `task_template` and `variables` serialize as strings, never objects
- read normalization for routine, trigger and run, including JSON-string
  parsing, the malformed-JSON fallback, `null` handling, element order,
  dropped non-object elements, and `secret` exclusion
- the two unwrap failure directions: a list envelope that is missing, `null` or
  non-array yields `[]`; a single-resource envelope that is missing, `null`,
  non-object, or an object with no non-empty `id` throws (AC-003.5, AC-003.15)
- an array-valued `variables` or `task_template` string yields `{}`, not an
  index-keyed object (AC-003.4)
- run normalization carries `id`, `source` and `status` (AC-004.1), absent
  optional run fields stay `undefined`, and an absent run `status` normalizes to
  `""` without a cast (AC-004.5, AC-003.16)
- an update body built from a patch that clears `variables` or `description`
  transmits the empty string, and one with an empty policy omits the key
  (AC-001.10, AC-001.11)
- a cron expression is trimmed before it is sent (AC-002.14)

The REQ-002 orchestration behaviors are pinned by component tests over
`routine-detail-view` and the create dialog with the API module mocked, next to
the existing `create-routine-dialog.test.tsx`. Playwright cannot drive a failed
DELETE or a rejected create without a fault-injection surface the app does not
have, and these are the criteria a builder would otherwise have to invent a test
shape for:

- an unchanged save issues no trigger create and no trigger delete, including
  when the stored timezone is `UTC` and the draft's is unset (AC-002.4)
- a changed expression creates before it deletes (AC-002.5)
- a rejected create issues no delete and leaves the stored expression displayed
  (AC-002.6)
- a failed delete re-lists and reports the trigger count it found; a re-list
  that also fails reports the state as unknown (AC-002.7)
- a routine update that succeeds followed by a failing trigger step reports both
  halves and shows no success toast (AC-002.9)
- a create-flow trigger failure closes the dialog, refreshes the list, and
  reports the routine as created without a schedule (AC-002.10)
- reconciliation renders the re-listed triggers, not a spliced array (AC-002.11)
- a whitespace-only expression arms nothing in the create flow and issues
  neither create nor delete on the detail save (AC-002.14)
- a `""`-status routine leaves the status control unselected and its save patch
  omits `status`, while the page still reads the routine as firing (AC-003.17)

`CoordinatorRoutineHint` is tested where it lives, next to the agent layout,
with the store seeded: the hint is hidden for a CEO with a firing assigned
routine, including one whose status is `""`, and shown when no assigned routine
is firing (AC-003.19). The seeding is required, not convenient: the agents route
never populates `s.office.routines` (Out of scope), so this cannot honestly be
an E2E check. It is the one behavior change outside the routines directory, so it is the one place a passing routines suite
would not have covered.

AC-003.10 is checked by a scan, not by reading. A vitest case reads
`apps/web/app/office/routines/**` plus
`apps/web/src/office-routine-client-routes.tsx` and asserts three things.

- **No snake_case wire key appears.** The key set is the left columns of the
  three mapping tables above, restricted to keys containing an underscore. The
  restriction is load-bearing rather than cosmetic: those columns also hold
  `id`, `name`, `description`, `status`, `kind`, `timezone`, `enabled`,
  `variables` and `source`, which are legitimate model-shape property names
  appearing throughout these files, so a scan over the unrestricted key set must
  fail correct code and could never pass. An underscore-bearing key has no
  model-shape spelling, so a match is always a violation. Two exclusions are
  required for the scan to be able to pass, and both are decidable line by line:
  `*.test.ts`/`*.test.tsx` files, whose comments cite `catch_up_max` when they
  name the catch-up criteria they cover, and lines whose first non-whitespace
  characters are `//` or `*`, because `routine-row.tsx` explains `next_run_at`
  in prose that survives this change. A comment cannot make a component read a
  wire field, so excluding them narrows nothing the criterion claims. Run over
  the tree today with those exclusions it reports exactly the three
  `routine-row.tsx` fallback lines this capability deletes, which is what makes
  it a passing assertion afterwards rather than a permanent red.
- **No `as unknown as` appears.** This is the half no key scan can reach:
  `syncCronTrigger`'s cast contains no wire key. The bounded set holds nine
  occurrences across six sites today: two fallback casts in `routine-row.tsx`
  and four envelope unwraps, three of which spell the cast twice because they
  carry a bare-resource fallback. All nine go, so the assertion is zero rather
  than a count.
- **The consumer set of `s.office.routines` has not grown.** Scanned over
  `apps/web/app/**`, which excludes `lib/state/slices/office/office-slice.ts`,
  the slice that declares the array rather than consuming it. The set must be
  exactly the three files that read it today:
  `app/office/routines/routines-content.tsx`,
  `app/office/agents/[id]/layout.tsx` and
  `app/office/agents/[id]/runs/runs-list-view.tsx` (AC-003.18). Asserting
  AC-003.10's whole set instead could never pass: most of those files take
  routines as props. Those two are
  excluded from the key scan deliberately, not by oversight: both also consume
  `AgentRunSummary`, a snake_case wire shape owned by a different contract, so
  `run.routine_id` and `run.task_id` are correct code there and no key scan can
  tell them from a violation. Asserting the consumer set instead keeps the
  exclusion honest and self-maintaining, because a third consumer fails this
  case rather than escaping the bound in silence.

The file bound makes the negative decidable where a repo-wide claim would not
be, and the key set comes from the tables, so a key added to the contract later
is added to the scan by the same edit.

Playwright covers the round trip the wire bug currently makes impossible to
write honestly: seed a routine through the API, drive the real create and save
flows, and read the persisted value back. The user-visible surfaces are
`/office/routines` (list row, expanded detail, Runs tab), `/office/routines/:id`
(form seeding, save, schedule card) and the create dialog. Existing specs to
extend rather than duplicate: `routines-ui.spec.ts`,
`routine-catch-up-policy-ui.spec.ts` and its mobile sibling
`mobile-routine-catch-up-policy-ui.spec.ts`. No new component, layout or
interaction pattern is introduced, so the same adapter feeds both and the
mobile spec remains the check that it does.

## Out of scope

- **Changing the HTTP contract.** The backend keeps snake_case JSON tags. An
  existing wire spelling is the public contract and stays: the same keys are
  consumed by the agentctl CLI, the YAML config loader, config import/export
  and config sync. Fixing the client is the safer change.
- **Populating `s.office.routines` anywhere but `/office/routines`.** Only that
  route's two page files call `setRoutines`, so a direct visit to
  `/office/agents/:id` renders `CoordinatorRoutineHint` against an empty array
  and shows the hint whatever the assignee is. AC-003.19 is scoped to the
  component for that reason. A fix needs a second store feeder, falsifying the
  single-ingress property above, or a per-page fetch with its own loading and
  error contract. Neither is a wire-contract change.
- **Adding a tiebreak column to any routine list query.** Changing it is a
  backend ordering decision with its own compatibility surface.
- **Optimistic concurrency for routine update.** An `expected_*` guard is a
  contract change to the update DTO.
- **Making the routine update and its trigger replacement atomic.** No endpoint
  accepts both. AC-002.9 makes the split outcome explicit instead.
- **Rejecting an empty routine `name`.** `name` is single-word, so it already
  transmits and already clears; this capability neither introduces nor worsens
  it. Guarding it is a form-validation change with its own copy.
- **Switching a routine from cron to webhook in the detail view.** Selecting
  `webhook` leaves the existing cron trigger armed, so the routine keeps firing.
  This is pre-existing and not worsened by the wire fix. It needs a
  trigger-kind transition contract of its own: what happens to the cron trigger,
  whether a webhook trigger is minted, and what `public_id` and `signing_mode`
  it gets.
- **Creating or configuring webhook triggers from the UI.** `public_id`,
  `signing_mode` and `secret` have no controls. AC-002.1 makes the adapter carry
  them so a future surface needs no boundary change; no surface is added here.
- **Clearing a routine's schedule from the detail view.** Emptying the cron
  expression leaves the trigger armed (AC-002.13), so the routine keeps firing.
  This is pre-existing. Removing a schedule needs its own contract: whether
  empty means delete, whether an operator can disarm without deleting, and what
  `enabled` is for. Guessing that empty means delete would let a cleared field
  destroy a working schedule silently.
- **Clearing a routine's assignee from the detail view.** The assignee Select
  has no unassigned option. AC-001.5 makes the boundary able to express the
  clear; adding the control is a UI change this capability excludes.
- **Surfacing `skip_reason`, `pause_id` and `causation_id` on runs.** They are
  on the wire and absent from the run model. Nothing renders them, so they stay
  unmapped.
- **The divergence between the create dialog's `coalesce_if_active` default and
  the server's `skip_if_active`.** Once the field transmits, the dialog's choice
  is what persists. Which default is correct is a product decision, not a
  serialization one.

## Related decisions

- [ADR-0036: Normalize ACP shell output at the adapter boundary](../../../decisions/0036-normalize-acp-shell-output-at-adapter-boundary.md)
