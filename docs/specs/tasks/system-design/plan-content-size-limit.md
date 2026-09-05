---
status: current
system: tasks
requirements:
  - REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-001
  - REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-002
  - REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-003
created: 2026-09-02
owners:
  - kandev
---

# Task plan content size limit System Design

## Purpose and boundaries

This design adds one admission rule to the plan write path: content above a fixed
byte ceiling is refused before plan storage work happens. It changes no coalescing
rule, no revision numbering, no truncation threshold, and no read path. The
truncation guard and this ceiling never both fire on one write: an oversized write
is refused before truncation is evaluated, and a write that passes the ceiling is
evaluated for truncation exactly as it is today. They answer different questions —
how much of the prior document survived, versus how large the submitted one is.

## Where the ceiling lives

Two surfaces admit caller-supplied plan content, and both call `PlanService`:

```text
create_task_plan_kandev ─┐
update_task_plan_kandev ─┤ mcp/handlers   ─┐
                         │                 ├─► PlanService.CreatePlan / UpdatePlan
task.plan.create ────────┤ task/handlers  ─┘        │
task.plan.update ────────┘                          ▼
                                        per-task lock ─► upsertPlan ─► repository
```

The ceiling belongs in `PlanService.CreatePlan` and `PlanService.UpdatePlan`, not
in either handler. Both surfaces already share the service, and the equivalent
lesson is recorded in `REQ-TASKS-TITLE-LENGTH-LIMIT-001`: a per-surface check is
one a surface can skip, and a third write path added later inherits nothing. The
service is the seam that satisfies
`AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.3`.

`RevertPlan` does not get the check. It admits no caller-supplied content; its
content comes from a `task_plan_revisions` row this system already stored. Gating
it would strand any task whose history predates the ceiling, with no way to
restore its own plan. This is safe because the ceiling gates every path by which
content enters the revision table, so a revert can only restore something an
earlier, ungated write stored. That is the reasoning behind
`AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.9`, and the residual it accepts is that
HEAD can hold above-ceiling content after a revert.

## Placement inside the write methods

```go
func (s *PlanService) CreatePlan(ctx context.Context, req CreatePlanRequest) (PlanWriteResult, error) {
    if req.TaskID == "" {
        return PlanWriteResult{}, ErrTaskIDRequired
    }
    if err := s.authorize(ctx, req.TaskID); err != nil {
        return PlanWriteResult{}, err
    }
    if len(req.Content) > MaxPlanContentBytes {
        task, err := s.repo.GetTask(ctx, req.TaskID)
        if err != nil {
            return PlanWriteResult{}, err
        }
        if task == nil {
            return PlanWriteResult{}, repoerrors.ErrTaskNotFound
        }
    }
    if err := checkPlanContentSize(req.Content); err != nil {   // new
        return PlanWriteResult{}, err
    }
    release := s.locks.acquire(req.TaskID)
    ...
}
```

`UpdatePlan` uses the same validation helper. The helper runs authorization first.
For an oversized request it then confirms task existence before returning the size
error. This preserves the accepted task-document contract: a write for a missing
task returns `not_found` and does not expose storage constraints. The extra lookup
does not read the plan row and is only needed for an oversized request; normal
writes keep their existing access path.

The ordering in `AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.5` is deliberate. Task ID
and access checks retain their existing precedence, including the rule that a
missing task returns `not_found` without storage details. The size check then runs
before any plan-row read and before the per-task lock: an oversized write that
queued for the lock would let a flood of them serialize ahead of legitimate writes.
The task existence preflight is not plan storage work and does not serialize with
other plan writes.

The content decision itself reads no stored plan state, so two concurrent writes
for the same task cannot influence each other's size outcome and the size check
needs no lock of its own (`AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.6`). When the
request is rejected, `upsertPlan` never runs, so no HEAD row is touched, no
revision is written or coalesced, and no event is published
(`AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.4`). The backend holds no admission state
between calls, so a resubmission is judged fresh (`AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.7`).

## The constant and the unit

```go
// MaxPlanContentBytes bounds the plan content a single write may store.
const MaxPlanContentBytes = 256 << 10 // 256 KiB
```

An exported constant in `internal/task/service`, matching `maxUserStateBodyBytes`
in `internal/plugins/user_state_handlers.go` in both value and reasoning. It is
not read from configuration, environment, or a runtime feature toggle
(`AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.11`).

The unit is **bytes**, via `len(content)`, not runes. This differs on purpose from
`planTruncationDetected`, which counts runes because it reasons about how much of
a *document* survived and a script change can preserve bytes while destroying
characters. Here the quantity being bounded is memory and scan cost, which is
proportional to bytes, and the storage limit it backstops is a byte limit. A
comment at the constant should record this, because the neighbouring guard's
opposite choice looks like an inconsistency otherwise.

The comparison is `len(content) > MaxPlanContentBytes`, which makes exactly the
ceiling admissible and one byte more a rejection
(`AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.2`). Content at or below the ceiling
reaches `upsertPlan` on an unchanged path, including empty content *where the write
path already accepts it* (`AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.8`).

That qualifier is load-bearing, because the two write paths differ on empty content
today. The MCP handlers reject `content == ""` with a `VALIDATION_ERROR` reading
`content is required` *before* they call `PlanService`, so on the agent path empty
content never reaches the ceiling check at all. The browser WebSocket handlers have
no such pre-check, so empty content reaches the service and is admitted. This
capability changes neither. Do not remove the MCP `content is required` check in
order to make empty content reach `upsertPlan` on every path: that is a separate
contract change and is out of scope here. The ceiling only ever *rejects* a write;
it never widens what a path admits.

## Error type and message

The message must carry the submitted size, so a fixed sentinel string is not
enough. The service returns a typed error wrapping a sentinel:

```go
var ErrPlanContentTooLarge = errors.New("plan content exceeds the size limit")

type PlanContentTooLargeError struct {
    Submitted int
    Limit     int
}

func (e *PlanContentTooLargeError) Error() string { ... }
func (e *PlanContentTooLargeError) Is(target error) bool { return target == ErrPlanContentTooLarge }
```

`errors.Is` support lets `planws` and tests match the class while the rendered
text stays dynamic.

`internal/task/planws/errors.go` maps it. Its existing table pairs a sentinel with
a *fixed* message, and this is the first plan error whose message is computed from
the request, so `errorResponse` gains one branch checked before the table walk
rather than a new table entry with a placeholder message. It resolves to
`ws.ErrorCodeValidation`, which satisfies
`AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-002.4`, and is reachable from `CreateError` and
`UpdateError` only — no other plan operation can return it.

That same branch is the one place the structured `details` map described under
`## Browser surface` is populated (`reason`, `limit`, `submitted`). The rest of the
table keeps passing `nil` details, unchanged. The map is additive and the browser is
its only consumer: the agent path renders the message text and ignores `details`
entirely, so `REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-002` still depends on the message
carrying every fact it requires.

`DispatcherBackendClient.RequestPayload` already forwards a WebSocket error to the
agent verbatim as `backend error [VALIDATION_ERROR]: <message>`, and both MCP
handlers already turn that into `mcp.NewToolResultError(err.Error())`. So the
single WebSocket message *is* the agent-facing text, and it has to carry
everything `REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-002` requires: the limit and
submitted size (`002.1`), that nothing was stored and the existing plan is
unchanged (`002.2`), and an instruction to shorten the held document that neither
suggests retrying unchanged nor rebuilding from memory (`002.3`). The last clause follows `planTruncationWarning`'s recorded reasoning: an agent told only that
a write failed reaches for the tool it has, and rebuilding the plan from memory is precisely the
loss both guards exist to prevent. No further MCP-handler changes are needed.

The tool descriptions in `registerPlanTools` (`internal/mcp/server/server.go`)
gain the ceiling on the `content` parameter of both write tools
(`AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-002.5`), stated as a byte count so it matches
the enforced unit.

## Browser surface

`useTaskPlan` already catches the rejection and sets its `error` state, and
already leaves the draft alone on failure: `savePlan` returns `null` without
calling `setTaskPlan`, so the editor keeps the user's text and the panel keeps
reporting unsaved changes. `AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-003.2` and `003.3`
are therefore existing behavior to pin with tests, not behavior to build.

The gap is that `useTaskPlanPanelState` in `components/task/task-plan-panel.tsx`
does not destructure `error`, so nothing renders it. Combined with a 1500 ms
autosave, a user would type into an editor that had silently stopped persisting
and lose the work on reload — a worse outcome than the growth this capability
prevents, which is why `REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-003` exists rather than
leaving the browser path ungated. The panel must render the failure (`003.1`),
through `t()` in the `task` namespace with the limit and the submitted size as
interpolated values, added in all five shipped locales (`003.6`).

Four details a builder would otherwise have to invent, every one of which fails silently if
guessed wrong, so the reasoning is kept next to the rule.

**1. How the panel knows a *save* failed at all.** The obvious wiring is wrong.
`error` in `useTaskPlan` is a SINGLE state slot shared by five operations:
`fetchPlan`, `savePlan`, `removePlan`, `loadRevisions` and `revertTo`. Two of them run
automatically when the panel mounts, `fetchPlan` and `loadRevisions` (the latter through
`useTaskPlanRevisions`, which is handed the same `setError`). `isSaving` does not
discriminate either, because save, delete and revert all set it and neither loader does.
Nothing renders `error` today, so this capability is its first consumer and would inherit
that ambiguity whole.

Wiring the shared slot into the panel would report a plan-write rejection for a failure
that never touched the write path, and the reachable case is not exotic: a task whose stored
plan predates the ceiling (`AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.10` keeps it readable and
editable) opens the panel, `loadRevisions` fails on a transient blip, and the user is told
their plan was rejected for size when no write was attempted. That is the false save-status
signal `REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-003` exists to prevent, and it contradicts
`003.3`.

So `useTaskPlan` exposes a **save-scoped** signal distinct from the shared slot: a
`saveError`, cleared at the start of `savePlan` and set only in `savePlan`'s catch branch.
The panel renders that and only that (`003.1`, `003.7`). It starts `null`, so a panel that
has attempted no save renders no failure. The pre-existing `error` slot keeps its current
behavior and its current consumers; this capability does not change what the other four
operations write, it simply does not read them.

**2. How the panel knows the save failure was an oversize rejection.** Not by parsing the
backend's English prose — that breaks the moment the copy is reworded and cannot be
localized. And not by re-measuring the editor either: the draft is not the thing that was
rejected, and by the time the rejection arrives it may not even be the thing that was
submitted. The rejection carries its own facts, and the channel for them already exists.

`pkg/websocket.NewError(id, action, code, message, details map[string]interface{})`
already takes a structured `details` map, and the browser already preserves it:
`toWebSocketRequestError` in `lib/ws/request-error.ts` copies `code` and `details` onto a
`WebSocketRequestError`, which `client.request` rejects with and which lands in
`savePlan`'s catch branch. Nothing new has to be plumbed through the WebSocket client; the
path is wired end to end today and merely unused here.

So the size branch in `planws` emits details alongside its message:

```go
ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, sizeErr.Error(),
    map[string]interface{}{
        "reason":    "plan_content_too_large",
        "limit":     sizeErr.Limit,
        "submitted": sizeErr.Submitted,
    })
```

`reason` is the discriminator, because the error code cannot be one: `task_id is required`
and `content is required` are already `VALIDATION_ERROR` on these same actions. It is a
stable wire token compared with `===`, so it is never localized and never reworded; the
frontend comparison needs an `// i18n-exempt` line comment saying so.

`savePlan` classifies **once**, in its catch branch, and stores the result rather than the
raw message:

```ts
type PlanSaveError =
  | { kind: "content-too-large"; limit: number; submitted: number }
  | { kind: "generic"; message: string };
```

The kind is `content-too-large` when the caught value is a `WebSocketRequestError` whose
`details.reason` is exactly `plan_content_too_large` and whose `details.limit` and
`details.submitted` are both finite numbers. In every other case — a different `reason`, a
missing or malformed `details`, or a transport failure that never reached the backend at
all — the kind is `generic` and carries `err.message`. No version-skew fallback is needed
on top of that: `internal/webapp/embedded` embeds the built SPA into the backend binary, so
a shipped build cannot pair a frontend that expects `reason` with a backend that predates
it. `ErrorPayload.Details` is already `json:"details,omitempty"`, so the field simply does
not appear on the errors that do not set it.

**What the panel renders for each kind, and why `003.6` still holds.** Both kinds render
localized copy from the `task` namespace; neither renders the backend's string. The
`content-too-large` kind renders a size-specific message interpolating `limit` and
`submitted` (`003.1`). The `generic` kind renders one localized "could not save your plan"
message. `err.message` is retained on the variant for DIAGNOSTICS ONLY — it is what
`savePlan` already hands to `console.error` — and is never the rendered string. This is what
keeps `003.5` and `003.6` from colliding. `003.5` requires that the failure the panel
reports CORRESPOND to the attempt that failed rather than to a size rejection that did not
happen; a localized generic message satisfies that, because what `003.5` is protecting
against is telling the user to shorten a document when nothing was rejected for size.
`003.6` requires that whatever is displayed be localized copy in all five shipped locales,
which a raw backend or transport string can never be. So: do NOT render `err.message`, and
do NOT append it to the localized frame. It is English, it is written for an agent audience
(see `## Error type and message`), and in the transport-failure case it is not a backend
message at all.

Deciding once, at the moment the attempt resolves, is what makes `003.1` and `003.5` hold
together. An edit that lands while the request is in flight cannot change either number or
the kind, and an edit that lands between one failure and the next attempt cannot flip the
rendered copy. Re-measuring the editor instead breaks all three cases; they are enumerated once under
`## Rejected alternatives`.

One consequence worth naming: the frontend needs no mirrored copy of the ceiling, because it
displays the limit the backend sent. A `lib/plan-content.ts` module and a
`MAX_PLAN_CONTENT_BYTES` constant are deliberately **not** part of this design — a mirrored
constant earns its drift risk only when the frontend must decide something before it can ask
the backend, and here it never decides anything.

**3. How the panel stops resubmitting a draft that was just rejected.** This is the trap
that matters most, because the plain reading of the autosave effect is wrong and the
failure mode is the exact resource problem this capability exists to prevent.

The effect's dependency array is `[draftContent, plan, isSaving, attemptSave, saveError, taskId]`, and `isSaving` is
not inert. `savePlan` sets it true before its `await` and false in its `finally`, so every
attempt flips it twice and re-runs the effect twice. The `false → true` run returns early on the
`isSaving` guard; the `true → false` run finds `hasChanges` still true — a failed save never
calls `setTaskPlan`, so `plan.content` still differs from `draftContent` — and arms a fresh
1500 ms timer carrying byte-identical content. `savePlan`'s identity is stable across a failed
save, so nothing breaks the cycle: left alone, a rejected 256 KiB draft is resubmitted every
1.5 seconds for as long as the panel stays open. `003.4` is not free, and no existing test
would catch it.

The suppression is a ref in `usePlanDraft` holding the content of the most recent autosave
attempt. It starts `null`, which no draft is equal to, so a panel that has attempted no
save never suppresses its first autosave. The ordering is part of the rule:

- It is assigned **immediately before** `savePlan` is called, never in the promise's
  failure branch. `setTaskPlanSaving(taskId, false)` runs in `savePlan`'s `finally`, which
  fires before the awaiting caller's continuation, so a ref written from that continuation
  could lose the race with the very re-render it is meant to guard. Written first, the
  guard is correct regardless of scheduling.
- The autosave effect refuses to arm a timer while `draftContent` is identical to that ref.
  With the existing `hasChanges` and `isSaving` guards, this is what makes `003.4` true.
- A successful save clears it to `null`.
- A task change clears it too. `usePlanDraft` receives `taskId`, resets the
  attempt ref, and replaces the local draft with that task's stored content (or
  an empty string) before the autosave effect can run. This is required even
  when both tasks have no plan, because their `plan?.content` values are both
  `undefined`.
- A generic save failure does not suppress unchanged content. The hook receives
  the save-scoped error and suppresses only a matching `content-too-large`
  rejection; transport or server failures can retry after the saving state
  returns to idle.
- An explicit user-requested save proceeds unconditionally. That is `003.4`'s
  "or the user explicitly requests a save", and it is the user's escape hatch
  from a suppressed autosave. The explicit path and the autosave timer both use
  the hook's `attemptSave` entry point, while only the autosave effect applies
  the suppression guard.
- It is a ref rather than state on purpose: assigning it must not itself re-run the effect.

Two consequences to state rather than let Build discover. The guard is on content identity,
not edit events, so a user who edits away from rejected content and back byte-for-byte stays
suppressed until they save explicitly — correct, because resubmitting those exact bytes would
fail identically. And because the ref is consulted before arming, it also closes detail 4's
overlapping-save re-arm: when the earlier of two in-flight saves clears `isSaving` and re-runs
the effect, the ref still holds the later save's content, so no third attempt is armed.

**4. Which task and which attempt a rejection belongs to.** `saveError` has to answer "did
*this* task's *latest* write fail", and neither half comes for free.

It cannot simply copy the existing `error` slot's shape. `error` is a plain `useState` local
to the hook, unlike `plan`, `isLoading` and `isSaving`, which are read per task from the store.
Nothing resets a `useState` when `taskId` changes, and there is no remount to do it: `PlanContent`
(`components/task/dockview-panel-content.tsx`) renders `<TaskPlanPanel taskId={taskId} visible />`
with no `key={taskId}`, so one instance survives every active-task switch. Reject a save on task
A, switch to B, and A's banner is displayed against B's plan.

Three rules, and together they are `003.8`:

- **Reset on task change.** `useTaskPlan` (`hooks/domains/session/use-task-plan.ts`)
  clears `saveError` in an effect keyed on `taskId`. `usePlanDraft`
  (`hooks/domains/session/use-plan-draft.ts`) resets its last-attempt ref and
  synchronizes the local editor draft in an effect keyed on `taskId`. The sync
  marks the render as external so it cannot autosave the previous task's draft,
  including when both tasks have no plan. `taskId` is nullable, and a change to
  no task is a change like any other. The sequence counter is deliberately *not*
  reset, because it only needs to stay monotonic.
- **Discard stale results.** `savePlan` compares the `taskId` it was invoked for against a
  **`currentTaskIdRef` that the hook assigns on every render**, and writes `saveError` only
  while the two are equal; otherwise it drops the result. The ref is load-bearing, not
  bookkeeping. `savePlan` is a `useCallback` with `taskId` in its dependency array, so an
  in-flight attempt's continuation runs inside the closure that captured the OLD `taskId`;
  comparing that captured value against the same closure's `taskId` compares a variable with
  itself, always passes, and discards nothing. Reading the CURRENT task through a ref is the
  only way a stale closure can see what the live render thinks the task is. Assign it in the
  hook body on every render, NOT in an effect: an effect commits after the render, so a save
  that resolves in between would compare against the previous task's id and the guard would
  be wrong in exactly the window it exists to cover. Its initial value is the first render's
  `taskId`, so it is never `undefined` when a continuation reads it. This covers what the
  reset effect cannot — a write for task A that fails *after* the switch to task B.
- A task ID can appear again after an A → B → A switch. The hook therefore
  increments a task-instance generation synchronously when `taskId` changes;
  each attempt captures that generation, and a continuation must match both the
  current task ID and the current generation before it can set `saveError`.
- **Latest started attempt wins.** `savePlan` takes a monotonically increasing sequence
  number from a ref at the top of each attempt — the ref starts at 0 and is incremented
  before the number is read, so the first attempt is 1 and 0 is never a live attempt — and
  writes `saveError` only while its own number is still the highest issued. "Last to complete" is not a usable rule:
  `savingByTaskId` is one boolean per task in `session-slice.ts`, not a refcount, so with
  two writes in flight the first to finish clears it while the second is still running, and
  completion order carries no information about which attempt is more recent. Ordering by
  start is decided by a counter this code owns rather than inferred from a flag it does not.

`003.5` is deliberately worded around *when* the rejection clears rather than its outcome:
`savePlan` clears its error state at the top, before the request goes out, so a displayed
rejection disappears when the next attempt begins, not when that attempt succeeds. Everything
else the AC requires — no rejection returning for an at-or-below-ceiling submission, an
unrelated failure surfacing its own reason, and editing between attempts not changing which
failure is shown — follows from detail 2's decide-once classification. A test written against
"clears on success" would pass without exercising any of it.

## Rejected alternatives

- **Clamp instead of reject.** Silently storing the first 256 KiB is the exact
  whole-document destruction `planTruncationDetected` exists to warn about, and it
  would destroy the tail with no revision holding it. Rejection keeps the caller's
  copy authoritative.
- **Enforce in the two MCP handlers.** Leaves the browser path open, and a future
  write path inherits nothing. Rejected for the reason recorded in
  `REQ-TASKS-TITLE-LENGTH-LIMIT-001`.
- **A per-message limit on the WebSocket gateway.** The gateway's 32 MiB
  `SetReadLimit` is transport-wide; lowering it to bound plans would cap every
  other action, and it does not cover a non-WebSocket caller.
- **A configurable ceiling.** Makes the published agent guidance
  install-dependent and gives an operator a way to switch the bound off.
- **Gate reverts too.** Strands tasks whose history predates the ceiling.
- **Classify the browser rejection by measuring the draft against a mirrored
  ceiling.** The draft is not what was rejected, and by the time the rejection lands
  it may not be what was submitted either. Measuring it renders a size rejection that
  never happened when an unrelated failure coincides with an oversized paste, hides a
  real one when the user trims while the request is in flight, and silently swaps the
  copy under an unchanged failure when the user trims afterwards. Replaced by the
  decide-once classification from the error's `details`. This also removes the reason
  for a mirrored `MAX_PLAN_CONTENT_BYTES` constant on the frontend.
- **Rely on the autosave effect not re-firing after a failed save.** It does re-fire:
  `isSaving` is in its dependency array and every attempt flips it twice. Believing
  otherwise ships an indefinite 1.5-second resubmit loop for the exact oversized
  document this capability refuses. Replaced by the last-attempt ref in detail 3.
- **`key={taskId}` on `<TaskPlanPanel>` to scope the rejection to its task.** A remount would
  reset both pieces of state and drop a stale attempt's continuation, so it does solve the TASK
  half of `003.8`. Not chosen because it does not touch the ATTEMPT half: two writes in flight
  for the SAME task share one key and one mount, so the sequence ref is required either way,
  and once it exists, resetting the named state and the draft in a task-aware
  sync beats remounting the panel and re-running its load effects on every task
  switch. `PlanEditor` is already keyed ``key={`${taskId}-${state.editorKey}`}``;
  the draft hook now resets its local value even when both plans are absent.

## Testing

Backend, in `internal/task/service`:

- Boundary: exactly 262,144 bytes accepted, 262,145 rejected, on both `CreatePlan`
  and `UpdatePlan`.
- Rejection persists nothing: HEAD unchanged, revision count unchanged, no event
  published on the bus.
- Ordering: an oversized request with an empty `TaskID` returns
  `ErrTaskIDRequired`; an oversized request for a missing task returns
  `not_found` without size details; an inaccessible task is rejected by the
  authorizer before the size response; and an oversized write for a valid task
  does not wait behind its plan lock.
- No lock contention: an oversized write returns while another write for the same
  task holds the lock.
- `RevertPlan` restores an above-ceiling stored revision successfully.
- A multi-byte-character document just under the ceiling in bytes is accepted even
  though its rune count is far lower, pinning the byte unit.

`internal/task/planws`: the typed error maps to `VALIDATION_ERROR` with a message
containing both numbers, from `CreateError` and `UpdateError`.

`internal/mcp/handlers`: the oversized write returns an error response, not a
success payload with a warning.

Frontend, all with fake timers where a debounce is involved:

- `use-task-plan`: a rejection sets `saveError` to kind `content-too-large` carrying the
  `limit` and `submitted` values from the error's `details`, and the draft is retained.
- `use-task-plan`: a failure whose `details` is absent, or whose `reason` is anything
  other than `plan_content_too_large`, sets kind `generic` — this is the branch that
  keeps a transport failure from being reported as a size rejection.
- `use-task-plan`: a save that is rejected for size while the draft is edited *below* the
  ceiling before the rejection resolves still reports `content-too-large` with the
  originally submitted size; and a save that fails for an unrelated reason while the draft
  is edited *above* the ceiling still reports `generic`. These two are the whole point of
  deciding once, and a test that measures the draft instead would pass in neither.
- `task-plan-panel`: the rejection renders, with the limit and submitted size from the
  attempt.
- `task-plan-panel` (`003.5` + `003.6`): a `generic` save failure renders the localized
  generic copy and NOT the backend's message string. Assert both halves — that the
  translated generic text is present, and that the text of `err.message` does NOT appear
  anywhere in the rendered output. A test that only asserts "some error is shown" passes
  with the raw backend string rendered verbatim, which is the exact thing `003.6` forbids.
- `task-plan-panel` (`003.4`): after a rejected autosave, advance fake timers **through
  the full `isSaving` true → false round trip and well past `AUTO_SAVE_DELAY`**, and
  assert the save function was called exactly once. Advancing a single debounce interval
  is not sufficient — the re-arm happens on the `isSaving` false transition, which a
  one-interval test never reaches, so that version of the test passes against the
  infinite-retry bug. Then assert that editing the draft re-arms it, and that an explicit
  save resubmits even when the content is unchanged.
- `task-plan-panel` (`003.7`): with a draft above the ceiling, fail each of the four
  operations the AC enumerates — `fetchPlan`, `loadRevisions`, `revertTo` and
  `deletePlan` — one at a time, and assert the plan-write rejection does NOT render in any
  of them. This is the regression the shared `error` slot would otherwise ship, so it is
  the case that must not be dropped for being awkward to set up.
- `task-plan-panel` (`003.8`): reject a save on task A, change `taskId` to task B, and
  assert the rejection is no longer displayed. With no plan on either task, assert
  the local draft resets to empty and no autosave sends task A's rejected draft to
  task B. Separately, resolve a *task A* rejection after the switch and assert
  nothing is displayed for task B.
- `use-task-plan` (`003.8`): with two saves in flight, resolve the earlier-started one
  last and assert the later-started one's outcome is what remains displayed.
- `use-task-plan` (`003.8`, stale-task discarding): this test must be written so it FAILS
  against a guard that compares the captured `taskId` with the closure's own `taskId`. Start
  a save for task A, change the hook's `taskId` to B, THEN reject A's request, and assert
  nothing is displayed. The tautological guard passes A's rejection through and renders it
  against B, so a test that never changes `taskId` between the call and the rejection cannot
  tell the two implementations apart.

There is no `lib/plan-content` test because there is no such module: the frontend takes the
limit from the rejection rather than mirroring it. If a later change reintroduces a mirrored
constant it needs a UTF-8-versus-UTF-16 measurement test, because `draft.length` under-reports
every non-ASCII document and Kandev ships zh-cn, zh-hk, zh-tw and pt-pt.

The PR includes focused desktop and mobile E2E checks. The mobile case is
important because the plan panel has a separate touch entry point and must keep
the same rejection, draft-retention, and recovery behavior. E2E checks cover the
browser/WebSocket boundary that unit tests cannot exercise; service and hook tests
remain the precise checks for admission ordering and state ownership.
