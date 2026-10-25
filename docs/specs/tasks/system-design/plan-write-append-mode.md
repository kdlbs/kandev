---
status: draft
system: tasks
requirements:
  - REQ-TASKS-PLAN-APPEND-001
  - REQ-TASKS-PLAN-APPEND-002
  - REQ-TASKS-PLAN-APPEND-003
  - REQ-TASKS-PLAN-APPEND-004
  - REQ-TASKS-PLAN-APPEND-005
  - REQ-TASKS-PLAN-APPEND-006
  - REQ-TASKS-PLAN-APPEND-007
created: 2026-09-01
owners:
  - kandev
---

# Task plan append-mode write System Design

## Purpose and boundaries

This design defines where an append composes the stored document, how the mode travels
from the MCP tool call to that point, what the separator is, and where the existing
plan size limit is evaluated for a composed write. It does not define the limit's value,
revision compaction, or any section-patch mode.

**Base state (verified 2026-09-05 against `origin/main`).** The
`plan-write-consistency` precondition has **landed**: `PlanService` holds a
`planLockTable` (`internal/task/service/plan_lock.go`) and both `CreatePlan` and
`UpdatePlan` acquire it around `upsertPlan`. Truncation detection moved into the
service (`internal/task/service/plan_truncation.go`); `evaluatePlanWriteGuard` no
longer exists, and `internal/mcp/handlers/task_plan_guard.go` now only renders the
warning text. `plan-content-size-limit` has also landed, which
[Size limit placement](#size-limit-placement) exists to account for.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-PLAN-APPEND-001` | [Mode parameter and validation](#mode-parameter-and-validation) |
| `REQ-TASKS-PLAN-APPEND-002` | [Separator normalization](#separator-normalization) |
| `REQ-TASKS-PLAN-APPEND-003` | [Where composition happens](#where-composition-happens), [Serialization dependency](#serialization-dependency) |
| `REQ-TASKS-PLAN-APPEND-004` | [Guard placement](#guard-placement) |
| `REQ-TASKS-PLAN-APPEND-005` | [Unchanged surfaces](#unchanged-surfaces) |
| `REQ-TASKS-PLAN-APPEND-006` | [Agent text design](plan-write-append-mode-agent-text.md) |
| `REQ-TASKS-PLAN-APPEND-007` | [Size limit placement](#size-limit-placement) |

## Prior art

**Our own prior reasoning.** Searched the `henry` vault (local path redacted)
through QMD, collection
`wiki`, with lexical, semantic and hypothetical-document sub-queries on
append-versus-replace write APIs, the context cost of read-modify-write, and durable agent
memory across context resets. Eight candidates, three read; each position is argued where
applied:

- `concepts/agents-md-pattern.md` — "compaction is not the fix, promotion is": periodic
  full rewrites of a durable agent-facing file cause measured **context collapse**. A
  replace-only API makes every write such a rewrite, so the erosion is structural, not a
  token-cost inconvenience. Applied in `## Out of scope`.
- `concepts/optimistic-vs-pessimistic-concurrency.md` — Agrawal, Carey and Livny (ACM
  TODS 12(4), 1987): blocking beats optimistic restart at medium-to-high contention
  under finite resources. Applied in
  [What the serialization must cover](#what-the-serialization-must-cover).
- `concepts/agent-replay-non-idempotence.md` — re-running an author is not idempotent.
  Applied in `AC-TASKS-PLAN-APPEND-003.6`.

**What other products shipped.** Searched `saas-kb` via `search_fsm_docs`,
`category: "ai_sdlc"`, on two queries: agent plan documents with append versus replace edit
tools and their context cost, and file-edit tools that insert rather than rewrite a whole
file. Both returned only Warp and Augment Code tool-permission, hook and quickstart pages
at relevance around 0.01, none describing a plan-document write mode. **This leg returned
nothing useful.**

**What we are doing differently.** Neither leg supplied a partial-write contract to copy,
so this capability takes its shape from the two positions above: blocking serialization
rather than optimistic retry, and a stated non-idempotence rather than an assumed-safe
retry.

*(Moved from the requirements document at its lint ceiling and compressed at this file's
own. No position or receipt was dropped.)*

## Components and responsibilities

- `internal/mcp/server` registers the tool parameter, validates the mode and forwards
  it. It does not compose content.
- `internal/mcp/handlers` unmarshals the mode from the WebSocket payload, re-validates
  it, and passes it to the plan service. It neither composes content nor reads the plan
  for composition.
- `internal/task/service.PlanService` owns composition: it reads the stored content and
  produces the composed content inside the same serialized region that commits the write.
- `internal/task/repository/sqlite.Repository` is unchanged. `WritePlanRevision`
  already commits the HEAD upsert and the revision write in one transaction.

## Where composition happens

Composition belongs in `PlanService`, not the MCP handler.

An append is a read-modify-write. Its read must be ordered against other writes to the
same task, or two concurrent appends read the same stored content, the second overwrites
the first, and a section is lost while both callers receive success —
`AC-TASKS-PLAN-APPEND-003.1` and `003.2` forbid exactly that. The MCP handler sits
outside every mechanism that orders plan writes: a handler calling `GetPlan`,
concatenating, then calling `UpdatePlan` reads strictly before the write path begins, so
that interleaving is not merely possible there, it is unprotected by construction.
Passing the mode and fragment down to the service is what makes `003.1` achievable at all.

This departs deliberately from the originating task description, which proposed composing
in `handleUpdateTaskPlan` before calling `planService.UpdatePlan`. That placement cannot
satisfy `REQ-TASKS-PLAN-APPEND-003`, and contradicts the same description's own
requirement that the append run under the per-task lock — the lock is inside the service,
that site above it. This design resolves the conflict in favor of the guarantee.

`UpdatePlanRequest` carries the mode.

Which read composes matters. `upsertPlan` already performs exactly one HEAD read —
`readPlanHead`, inside the lock — and `resolveHeadFallbacks` reuses that value for
title, author and missing-plan resolution. **Compose from that read.** It sits in the
serialized region and observes the HEAD row the write then commits against, which is what
`003.1` requires. A second read would reintroduce the straddling this section exists to
prevent and must not be added; title, author and missing-plan resolution keep today's
values, which `AC-TASKS-PLAN-APPEND-002.6` and `005.1` fix.

## Serialization dependency

An append is a read-modify-write, so `REQ-TASKS-PLAN-APPEND-003` holds only if plan
writes for a task are serialized. This section states what that must cover and why this
capability does not build it.

### What the serialization must cover

**Every plan write for that task id must take the same per-task lock, not appends
alone.** An append-only lock cannot satisfy `AC-TASKS-PLAN-APPEND-003.4`: a replace not
taking the lock can commit between an append's read and its commit, after which the
append writes stored-plus-fragment and the replace is discarded. Both callers are told the
write succeeded and the surviving content matches neither serial order — a lost update,
with a replace rather than a second append as the other party.

The lock already covers the replace path, which does not conflict with
`AC-TASKS-PLAN-APPEND-005.1`: that freezes replace's **observable** behavior (content,
title, revision numbering and coalescing, truncation reporting, events, response shape),
and internal serialization changes none of them.

The mechanism is blocking, not optimistic, per
`concepts/optimistic-vs-pessimistic-concurrency.md` (Agrawal, Carey and Livny 1987:
blocking beats optimistic restart at medium-to-high contention under finite resources).
Compare-and-retry also fails for a second reason: a retry would re-append, and
`AC-TASKS-PLAN-APPEND-003.6` states an append is not idempotent.

### The primitive already exists — do not rebuild it

`plan-write-consistency` landed: `internal/task/service/plan_lock.go` provides
`planLockTable`, a per-task-id mutex table retiring entries by waiter count and returning
an idempotent release, held by `PlanService`. **Build must not implement a second lock.**
Two invariants are non-obvious: the waiter count is incremented while the outer map mutex is still
held, so a concurrent release cannot delete an entry from under an acquirer en route to
it; and release is idempotent, so a deferred panic-safety release can coexist with the
explicit one on the success path. Getting either wrong fails rarely and silently — the
worst outcome for a concurrency primitive.

### The precondition, and how Build confirms it

**Met on `origin/main` as of 2026-09-05.** Build confirms it against a closed list of
paths, not the open phrase "all of its write paths", which is not decidable as written.
Serialization is present when every path performing a *Plan write* — per
`## Terminology`, upserting HEAD **and** writing or merging a revision — takes the
per-task lock around both its read and its commit. There are exactly four, all taking it
today: `PlanService.CreatePlan`, `UpdatePlan`, `DeletePlan`, and the revision revert path.

`PlanService.MarkImplementationStarted` is deliberately **not** on that list and its
absence does not fail the check. It issues a targeted `UPDATE task_plans` under no lock,
but it is not a *Plan write*: it writes no revision and touches neither `content` nor
`title`, setting only `implementation_started_at`, `..._session_id`, `..._by` and
`updated_at`. Only the last overlaps the HEAD upsert's `ON CONFLICT` set, so the worst a
race does is leave `updated_at` reflecting whichever statement committed second — a
timestamp, not content, so no composed section is lost. Build records this path as
unlocked and proceeds. The rule generalizes: a `PlanService` method writing no revision
and no content-bearing column is out of scope even though it writes `task_plans`.

If a future base fails this check, **stop and route for a scheduling decision**: do not
build a temporary lock, and do not ship the append path unserialized.

### Alternative considered and rejected

Composing inside `WritePlanRevision`'s write transaction would let SQLite's transaction
serialization stand in for the lock, removing the precondition. Rejected: it moves
Markdown composition policy into the SQL repository layer, which this design assigns to
`PlanService`; it changes the repository interface the precondition branch also changes,
trading one collision for another; and it substitutes a different
concurrency mechanism for the blocking one `## Prior art` reasoned about. Recorded so
it is not re-derived.

## Mode parameter and validation

The mode is an optional string. Its description names `replace` and `append`. The
registration does not use `mcp.Enum`, because the generic argument validator would
intercept an invalid value before the handler can return the required error. The
handler performs exact, case-sensitive validation and names both accepted values.

Validation is an exact, case-sensitive match against the two values. An empty or absent
value means `replace`. Any other is a validation error naming both accepted values, and
no write occurs. The handler must not fall back to `replace`: that is the path by which
a typo overwrites a plan with a fragment.

`AC-TASKS-PLAN-APPEND-001.7` puts mode validity **before** task-reach authorization,
inverting the order `validatePlanWrite` uses for the size limit, where authorization
runs first so an unreachable task's write is refused as unreachable, not as oversized.
The mode differs in kind: its accepted values are published in the tool's own schema,
so rejecting an unrecognized one discloses nothing about the target task and belongs at
the edge. Every later check keeps the existing order, so the size limit's placement
relative to authorization is unchanged.

Three sites carry the value, and the second is not named in the originating task
description:

1. `internal/mcp/server/server.go` — the `update_task_plan_kandev` registration.
2. `internal/mcp/server/handlers.go` — `updateTaskPlanHandler`, which builds the
   WebSocket payload. It copies `task_id`, `content`, `created_by` and an optional
   `title`. A mode not copied here never reaches the handler, and the tool silently
   replaces instead of appending — the destructive default. `copyOptionalStringArg` is
   the existing helper.
3. `internal/mcp/handlers/handlers.go` — `handleUpdateTaskPlan`, whose request struct
   has no `Mode` field today, so an unmarshalled mode is silently dropped until one is
   added. It then calls the service.

### The two sibling surfaces, which behave differently on purpose

`AC-TASKS-PLAN-APPEND-005.2` and `AC-TASKS-PLAN-APPEND-005.3` resolve a hazard that
exists today: both sibling write surfaces silently drop an unknown field, and one of
them is destructive if it does.

- **`create_task_plan_kandev` rejects a mode.** `createTaskPlanHandler` builds an
  explicit payload map, so a supplied `mode` is dropped before it travels, and
  `CreatePlan` upserts. An agent typing `mode: "append"` on this tool would have its
  fragment committed as the entire plan, with a success response — precisely what
  `AC-TASKS-PLAN-APPEND-001.3` rejects on the sibling tool. The fix is a validation
  error at `createTaskPlanHandler` naming `update_task_plan_kandev`, before any payload
  is sent.

  **The schema must declare `mode` after all, narrowly.** The server's generic MCP
  argument-schema validator (`internal/mcp/server/tool_argument_validation.go`) compiles
  every tool's schema with `additionalProperties:false`, derived from the exact same
  `Tool.InputSchema` object `ListTools()` exposes to clients, and runs it **before** any
  handler body executes. If `create_task_plan_kandev`'s schema does not declare `mode`,
  a call supplying `mode: "append"` never reaches `createTaskPlanHandler`'s own
  rejection at all: the generic validator intercepts it first with a schema-keyword
  error that names neither the tool nor `update_task_plan_kandev`, which fails
  `AC-TASKS-PLAN-APPEND-005.2`'s literal requirement. There is no code-only way to let
  an undeclared top-level key reach a handler under this validator. The resolution
  (`AC-TASKS-PLAN-APPEND-006.9`) is to declare `mode` in the schema with a description
  stating only that the value is rejected — advisory to a well-behaved client, not an
  advertised capability — so `createTaskPlanHandler`'s own, correctly-worded rejection
  is what the caller actually sees.
- **The browser path ignores it.** `wsUpdateTaskPlan` unmarshals into a struct with no
  `Mode` field, and `json.Unmarshal` ignores unknown fields — the path's existing
  behavior for *every* unknown field, so leaving it is what keeps
  `AC-TASKS-PLAN-APPEND-005.3`'s "unchanged" true. Build adds nothing here. The
  asymmetry with the bullet above is deliberate: a mistyped mode is possible on an
  agent-facing tool and not on this transport. Recorded so it is not "corrected" into
  symmetry later.

## Separator normalization

Defined so that a fragment beginning with a Markdown heading always begins a line,
whatever the stored content ends with. `AC-TASKS-PLAN-APPEND-002.2` requires exactly
one blank line between the two bodies.

"Whitespace" here is the requirements' normative definition: a character whose Unicode
`White_Space` property is true. In Go that is `unicode.IsSpace`, and so
`strings.TrimSpace` plus `strings.TrimLeftFunc`/`TrimRightFunc` with a
`unicode.IsSpace` predicate. A hand-rolled `" \t\r\n"` cutset does **not** satisfy the contract: it
misses U+0085 NEL and U+00A0 no-break space, so it would classify the same fragment
differently from `AC-TASKS-PLAN-APPEND-001.5`'s validation, which uses this definition.
Use one helper for both so they cannot drift.

Given stored `S` and fragment `F`:

1. Let `S'` be `S` with trailing whitespace removed.
2. Let `F'` be `F` with its leading empty and whitespace-only **lines** removed.
   Leading spaces and tabs on the fragment's first *non-empty* line are preserved,
   because they can be significant in Markdown — inside a list item, or an indented
   code block.

   Removing lines rather than only leading line-ending characters is what
   `AC-TASKS-PLAN-APPEND-002.2` and `AC-TASKS-PLAN-APPEND-002.3` require, and the two
   differ. For `S = "previous"` and `F = "   \n## New section"` — valid, since
   `AC-TASKS-PLAN-APPEND-001.5` rejects only fragments that are whitespace *throughout*
   — stripping leading line endings removes nothing, because `F` begins with spaces.
   The result would carry both the inserted empty line and the fragment's own
   whitespace-only first line: two blank lines between the bodies, breaking "exactly
   one empty line". Deciding per line removes that first line while leaving any indent
   on `## New section` untouched.
3. If `S'` is empty, the composed content is `F'`.
4. Otherwise the composed content is `S'` + `"\n\n"` + `F'`.

Trailing characters of `F` are preserved exactly. Nothing is appended after the
fragment, so a caller ending without a newline gets a document ending without one; a
later append still separates correctly because step 1 normalizes the tail then.

Both steps act on the outer edges only: no interior character of either body is
examined or altered, which is what `AC-TASKS-PLAN-APPEND-002.5` requires.

This algorithm composes `S'` and `F'`, not `S` and `F`, which is exactly what
`AC-TASKS-PLAN-APPEND-002.1` permits — it forbids insertion, reordering and removal
*beyond* the edge whitespace its siblings mandate, and steps 1 and 2 are those mandated
removals. A test written against `AC-TASKS-PLAN-APPEND-002.1` must therefore assert
`composed == S' + "\n\n" + F'`, never `composed == S + "\n\n" + F`; the latter
contradicts `AC-TASKS-PLAN-APPEND-002.2` for the example above and would be a wrong
test, not a found bug.

Line endings: a line is terminated by `\n` or `\r\n`, and a carriage return counts as
whitespace for steps 1 and 2, so a CRLF document or fragment is trimmed correctly at
both edges. The separator at step 4 is always `\n\n`, so appending to a CRLF document
yields mixed line endings. That is accepted: plans are agent-written Markdown, every
consumer reads them as text, and normalizing a document's line endings would rewrite
interior characters `AC-TASKS-PLAN-APPEND-002.5` forbids touching.

## Size limit placement

`plan-content-size-limit` landed first, and its admission check sits in the one place an
append makes wrong. `PlanService.validatePlanWrite` runs from both `CreatePlan` and
`UpdatePlan` **before** `s.locks.acquire`, measuring `len(req.Content)` against
`MaxPlanContentBytes`. For a replace that is correct and deliberate: the submitted content
*is* what will be stored, so the cheap check runs before the lock and an oversized write
never queues. For an append it is wrong twice over. The submitted content is the fragment,
so a 2 KiB fragment onto a 255 KiB plan passes a check the 257 KiB result would fail — the
limit is silently bypassed. And the composed content does not exist at that point, the
stored content not having been read.

The resolution is a split, not a move:

- **Replace keeps the pre-lock check exactly as it is.** `AC-TASKS-PLAN-APPEND-007.5`
  freezes its value, position and error. Moving it inside the lock for both modes would
  make every oversized replace take the lock first — a regression in a path this
  capability is not here to change.
- **Append is measured after composition, inside the lock.** The composed content is
  produced in `upsertPlan`, already under the per-task lock, so the check belongs
  immediately after composition and before `WritePlanRevision`. That satisfies
  `AC-TASKS-PLAN-APPEND-007.4`: the value admitted is the value committed, against the
  same stored content the write composed from.
- **Build must not also leave the pre-lock check active for `append`.** The limit is
  evaluated exactly ONCE on that path. Kept as a cheap early reject it measures
  `req.Content` — the fragment — so a fragment that alone exceeds the limit is refused
  before any composed content exists and the caller is shown the fragment's size, which
`AC-TASKS-PLAN-APPEND-007.3` forbids without qualification. Skip only the size comparison:
`validatePlanWrite`'s authorization still runs for both modes, and its `GetTask` lookup
sits inside the oversized branch, so skipping the comparison skips that too. Order per
`AC-TASKS-PLAN-APPEND-001.7`.

Reuse `checkPlanContentSize` rather than re-deriving it, so both modes share one limit
and one error type. The rejection must report the **composed** size
(`AC-TASKS-PLAN-APPEND-007.3`): `PlanContentTooLargeError.Submitted` carries the number
the agent sees, and reporting the fragment's would tell a caller that 2 KiB exceeded
256 KiB, which is unactionable.

One consequence Build must not miss: the append's check runs after the lock is taken, so
an oversized append returns having read and locked, and must still leave the stored plan
untouched and write no revision (`AC-TASKS-PLAN-APPEND-007.2`). Returning the error before
`WritePlanRevision` makes that true, since HEAD and the revision are written in that one
call. An oversized append is not a truncation; the detector is already skipped here per
[Guard placement](#guard-placement).

## Guard placement

Truncation detection runs on the replace path only. It must not run for an append.

The site is `planTruncationDetected` in `internal/task/service/plan_truncation.go`,
reached from `PlanService.UpdatePlan` **inside** the per-task lock, gated by the
request's `EvaluateTruncation` flag. `evaluatePlanWriteGuard` no longer exists;
`internal/mcp/handlers/task_plan_guard.go` now only renders `planTruncationWarning`
from the returned `ReplacedRunes`/`NewRunes`. **Skip the detector at the service, not
the handler.**

Skipping it is a correctness requirement, not an optimization. The guard flags a write
retaining less than half the stored document's characters, and an append's fragment is
routinely a small fraction of the stored plan, so it would fire on the common case, warn
about content that was not lost, and force a revision split. The composed content is
never a truncation of the stored content, because it contains it in full.

Two consequences. The append path must not set `ForceNewRevision` on the guard's
account, which keeps appends eligible for the existing coalescing rules
(`AC-TASKS-PLAN-APPEND-004.2`); and its response must not be wrapped by
`planWritePayload`, so the shape stays the unwrapped plan DTO a non-truncating write
returns today (`AC-TASKS-PLAN-APPEND-004.3`). Those criteria are stated as observable
outcomes rather than as a named call, so they hold wherever the detector lives.

## Failure and recovery

A failed read of the stored content is fatal to an append and must abort the write with
the stored plan untouched (`AC-TASKS-PLAN-APPEND-003.5`). This differs deliberately from
replace, where a failed read is non-fatal: there the caller supplied the whole document,
so the read only judges the write. On append the read is an input to the content itself,
and proceeding without it would commit the fragment as the entire document — the
destruction the mode exists to prevent.

`AC-TASKS-PLAN-APPEND-003.5` also fixes how that failure is *reported*: distinguishably
from plan-not-found, and with no storage detail. The state Build needs already exists —
`readPlanHead` returns `planHeadUnknown` when the read fails and `planHeadAbsent` when
there is no plan, and its doc comment says that split is deliberate — but `upsertPlan`
acts only on `planHeadAbsent` today, so `planHeadUnknown` falls through into the write.
Build adds an append branch on `planHeadUnknown` and **must not report it as
`ErrTaskPlanNotFound`**. That is the nearest error to hand and
`AC-TASKS-PLAN-APPEND-001.6` pins it to the adjacent state, which is exactly the trap: a
caller told its plan is missing calls `create_task_plan_kandev`, which upserts and
commits the fragment as the entire plan — the destruction this mode exists to prevent,
reached through the error message. Use a distinct sentinel and map it in
`planws.UpdateError`, which otherwise falls back to a generic internal error.

`PlanService.UpdatePlan` authorizes the task before it reads anything, so composing in
the service keeps the append's read behind the same task-reach check that guards a
replace (`AC-TASKS-PLAN-APPEND-005.6`). Composing in the handler would put a
plan-content read on a path that must re-derive that check for itself.

Appending to a task with no plan is rejected with the existing plan-not-found outcome:
`UpdatePlan` already returns `ErrTaskPlanNotFound` when HEAD is absent and
`planws.UpdateError` maps it. Append does not create a plan, so
`create_task_plan_kandev` remains the only way to make one. The missing-task case is
unchanged and stays owned by the plan write lifecycle design: the foreign key on the
HEAD upsert produces `ErrTaskNotFound` and the transaction rolls back.

## Unchanged surfaces

The browser write path (`wsCreateTaskPlan`, `wsUpdateTaskPlan`), the revert path, the
plan DTO, the plan editor and revision history keep their current behavior. The browser
path reaches `PlanService` through different handlers and never supplies a mode, so it
takes the `replace` default. `create_task_plan_kandev` is **not** on this list: what it
stores is unchanged, but it gains a mode rejection (`AC-TASKS-PLAN-APPEND-005.2`) and a
corrected `content` description (`006.9`).

## Agent-facing description and acknowledgement

`REQ-TASKS-PLAN-APPEND-006` — which agent-facing strings must change, the class rule
`AC-TASKS-PLAN-APPEND-006.9` binds them under, the truncation warning's rewrite, the
acknowledgement's unchanged behavior and the description tests that pin all of it — is
specified in [the agent text design](plan-write-append-mode-agent-text.md). It was split
out when this file reached its lint ceiling; no content was dropped in the move.

## Test strategy

Backend tests alongside the sources. Service tests cover composition and are where the
separator table belongs: a stored document ending with no newline, with one, and with
several, each against a fragment beginning with a heading; an empty and a
whitespace-only stored document; a fragment with leading blank lines and one with a
leading indent; and preservation of the fragment's trailing characters.

**Two concurrency tests**, both driving overlapping writes from separate goroutines
rather than in sequence, both under the race detector.

- Two appends to one task: both fragments present afterwards. Must fail against an
  implementation that composes outside the serialized region.
- An append concurrently with a **replace**: the result equals one of the two serial
  outcomes — the replace's content alone, or that content followed by the appended
  fragment — and never the pre-replace content plus the fragment, the lost update
  `AC-TASKS-PLAN-APPEND-003.4` forbids. Without it an append-only lock passes the whole
  suite while violating that criterion, so this is the test that pins the
  serialization's scope rather than merely its existence.

**Handler tests** cover mode validation: absent, empty, both accepted values, an
unknown value, and a case variant. Rejections assert the stored plan and revision count
are unchanged, not merely that an error was returned. A further test pins
`AC-TASKS-PLAN-APPEND-001.7`: a request invalid on two counts reports the earlier
failure in that criterion's order.

**A read-failure test** forces the stored-content read to fail and asserts the append is
rejected with an error the caller can tell apart from plan-not-found, that no revision is
written and the stored plan is byte-identical afterwards
(`AC-TASKS-PLAN-APPEND-003.5`). **A guard test** asserts an append of a small fragment onto a plan large enough to trip
the truncation thresholds emits no warning, forces no new revision, and returns the
unwrapped shape. **A bridge test** asserts a mode supplied to the MCP tool
reaches the WebSocket payload, so site 2 cannot be omitted silently.

**Size-limit tests**, against `MaxPlanContentBytes` rather than a copied literal:

- an append whose fragment is far below the limit but whose **composed** content
  exceeds it is rejected, with the stored plan and revision count unchanged
  (`AC-TASKS-PLAN-APPEND-007.1`, `007.2`). This fails against an implementation that
  measures the fragment, and is the reason that section exists.
- the error reports the composed size, not the fragment's (`007.3`).
- an append whose **fragment alone** exceeds the limit is likewise rejected reporting
  the composed size (`007.3`). This is the test that fails against an implementation
  which adds the post-composition check but also leaves the pre-lock one active for
  `append`; the first bullet passes there.
- an oversized **replace** is still rejected before the lock, with its existing error
  (`007.5`).

**Sibling-surface tests:** `create_task_plan_kandev` with any mode is rejected and
stores nothing (`AC-TASKS-PLAN-APPEND-005.2`); the browser update path with an unexpected
`mode` field behaves as without it (`005.3`).

**A whitespace-boundary test** covers a fragment whose only non-`" \t\r\n"` characters
are U+00A0 or U+0085: `AC-TASKS-PLAN-APPEND-001.5` must reject it, pinning the Unicode
definition against a hand-rolled cutset.

Replace-mode regression tests already exist and must continue to pass **unmodified**,
including the truncation-guard tests. A change to one signals that
`AC-TASKS-PLAN-APPEND-005.1` has been broken.
