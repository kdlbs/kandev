---
status: draft
system: office
requirements:
  - REQ-OFFICE-AGENT-COMMENT-READS-001
  - REQ-OFFICE-AGENT-COMMENT-READS-002
  - REQ-OFFICE-AGENT-COMMENT-READS-003
  - REQ-OFFICE-AGENT-COMMENT-READS-004
  - REQ-OFFICE-AGENT-COMMENT-READS-005
  - REQ-OFFICE-AGENT-COMMENT-READS-006
  - REQ-OFFICE-AGENT-COMMENT-READS-007
  - REQ-OFFICE-AGENT-COMMENT-READS-008
---

# Office: Agent Comment Reads System Design

## Purpose and boundaries

Office owns which tools an Office agent may call and what an Office agent is
told it can do. This design adds one read tool to that surface.

It uses, and does not own:

- the `task_comments` store and its repository, owned by Office persistence;
- the cross-task read relation, owned by the task system's handoff access
  guard and shared verbatim with the task document tools;
- the MCP profile registry that decides which surface registers which tool
  group.

One deliberate exception to that last boundary. This design does change the
shared guard's own not-found handling, in `loadAccessPair`, and claims that
change as in scope. The reason is in [Failure and recovery](#failure-and-recovery):
the guard as written today does not deny a missing task, it returns the
repository's not-found error, and that error embeds the task identifier. Fixing
that only at this tool's call site would leave the comment surface and the
document surface answering the does-this-task-exist question differently, which
`REQ-OFFICE-AGENT-COMMENT-READS-001` forbids in terms: the read relation must be
evaluated by the same shared implementation, so the two surfaces cannot diverge.
The blast radius is bounded. `loadAccessPair` has exactly two callers, both
inside the guard file, and no existing test asserts the leaking behaviour. The
change is security-positive for the document tools as well as for this one.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AGENT-COMMENT-READS-001` | [Purpose and boundaries](#purpose-and-boundaries), [Components and responsibilities](#components-and-responsibilities), [Failure and recovery](#failure-and-recovery), [Security](#security) |
| `REQ-OFFICE-AGENT-COMMENT-READS-002` | [Data and contracts](#data-and-contracts) |
| `REQ-OFFICE-AGENT-COMMENT-READS-003` | [Data and contracts](#data-and-contracts), [Ordering and windowing](#ordering-and-windowing) |
| `REQ-OFFICE-AGENT-COMMENT-READS-004` | [Data and contracts](#data-and-contracts) |
| `REQ-OFFICE-AGENT-COMMENT-READS-005` | [Data and contracts](#data-and-contracts), [Failure and recovery](#failure-and-recovery) |
| `REQ-OFFICE-AGENT-COMMENT-READS-006` | [Persistence](#persistence) |
| `REQ-OFFICE-AGENT-COMMENT-READS-007` | [The advertised-surface contract](#the-advertised-surface-contract) |
| `REQ-OFFICE-AGENT-COMMENT-READS-008` | [Control flow](#control-flow) |

## Components and responsibilities

- **MCP tool group.** A new Office-only tool group registers
  `list_task_comments_kandev`, alongside the existing office-documents and
  office-decisions groups. Its enable predicate is the Office surface alone.
- **MCP tool handler.** Reads the caller identity from the server's bound task,
  never from arguments; normalises `task_id` and `limit` by raw-type
  discrimination, not the typed argument accessors, per
  [Where argument decisions are made](#where-argument-decisions-are-made);
  forwards one payload carrying both the target and the caller.
- **Backend action handler.** Validates the payload, invokes the access-checked
  read, and maps service errors onto the existing handoff error codes.
- **Access-checked read.** A method on the cross-task handoff service that
  applies the same read guard the document reads apply, then delegates to the
  comment store. This method is the only new place the guard is called.
- **Comment store.** Supplies the windowed read and the total count.
- **Office first-turn context and role instructions.** Advertise the tool and
  direct the coordinator to use it.

## Data and contracts

The tool accepts `task_id` (optional; defaults to the caller task, and accepts
the literal `self`) and `limit` (optional).

### Where argument decisions are made

Both arguments are declared with **no JSON Schema type keyword** — the
`WithAny`-shaped declaration the MCP library already provides, which emits an
empty `{}` property schema rather than `"type": "string"` or `"type": "number"`.
That is not a stylistic choice, and it is the whole reason this subsection
exists.

Tool argument validation is a real, enforced layer that runs **before** the
handler: the server validates the incoming arguments against the declared input
schema and returns an error result without ever calling the handler when they do
not match. So a declared type is not advisory. Anything the schema rejects never
reaches the code that was supposed to decide what to do with it.

That collides with two contract requirements. `limit` must never produce an
error from any layer, yet a `"type": "number"` declaration would reject the
string, boolean, and null values the contract requires to be *defaulted*.
`task_id` must produce a handler validation error naming the field for a
wrong-typed value, and must treat null as omitted, yet a `"type": "string"`
declaration would reject both at the wrong layer, with a generic schema message
and no chance for the null case to be read as "use the caller task". Declaring
either type would make the requirement unimplementable rather than merely
untidy. Untyped declarations are what let every value reach the handler, so the
handler is the single place these decisions are made.

The handler therefore reads the raw argument map (`GetArguments()`) and
discriminates on the dynamic type itself:

- **`task_id`** — absent, or present and null, or a string that is empty or
  whitespace-only, or the exact string `self`: the caller task. Present and a
  non-empty string after trimming: that task identifier, compared to `self`
  case-sensitively. Present and any other type: a validation error naming
  `task_id`.
- **`limit`** — every value is defaulted or clamped, and no value is an error.
  JSON numbers arrive as `float64`, so "is not an integer" is a whole-number
  test on that value, never a Go `int` type assertion: treat the value as an
  integer only when it equals its own truncation, and default anything else.
  A fractional `7.5` is therefore **not** truncated to 7 — it is not an
  integer, so it takes the default of 20. Absent, null, a non-number, zero and
  negative take that same default; above 100 clamps to 100.

The typed convenience accessors must **not** be used for either argument.
`GetString` and the shared `copyOptionalStringArg` helper collapse "absent",
"null", and "wrong type" into the same fallback value, which is precisely the
distinction `task_id` depends on: a silently substituted `task_id` returns the
coordinator its own comments while it believes it is reading the child's, the
exact fan-in misread this document exists to remove. A helper cannot report a
type error it has already discarded.

`copyOptionalLimitArg` is prohibited on the same grounds and is the easier trap,
because it sits beside `copyOptionalStringArg` and already performs the raw-map
read described above — so it reads as the precedent to copy. It truncates
(`raw.(float64)` then `int(limit)`), which would turn `7.5` into 7 where the
rule above requires the default of 20. Neither helper is reused here.

The asymmetry between the two arguments is intentional. `limit` bounds a
result, so a substituted value returns the right subject in the wrong quantity,
which the window fields then report. `task_id` selects the subject, so a
substituted value returns the wrong subject silently.

Each returned comment projects exactly: identifier, task identifier,
`author_type`, `author_id`, `source`, body, creation timestamp, and — only when
the body was cut — a truncation marker plus the original body length in bytes.
`reply_channel_id` and the per-comment run lifecycle fields are not projected;
the first is internal reply routing and the second is a rendering concern for
the human comment list.

Bodies are cut with the existing shared rune-safe byte truncation helper rather
than a new cut, so the boundary rule lives in one place. The per-body cap is
8192 bytes.

Every response carries the window: `total`, `returned`, and `has_more`, which
is true exactly when `returned` is below `total`. The window describes only
omitted comments. Whether an individual body was shortened is carried on that
comment as `body_truncated`, never in the window. Two names because they are
two facts, and one name for both is how a coordinator concludes it has the
whole note when it has the first half of it.

Author fields are projected as data. No filter parameter exists for them. Two
reasons: a coordinator needs the human steer on a child task as much as the
agent note, and comment authorship is currently unreliable because reviewer
runs execute inside the author's session, so a filter keyed on authorship would
silently drop rows it should return.

## Ordering and windowing

The comment store's existing ascending list and descending recent-list both
sort on `created_at` with no tiebreak, and the comment DTO already records that
Office writes two timestamps inside the same second. The window here is
therefore explicitly two-phase and uses a named tiebreak in both directions:

1. select the newest `limit` rows ordered by `created_at` descending, `id`
   descending;
2. present that window ordered by `created_at` ascending, `id` ascending.

Using `id` in both directions is what makes the window boundary deterministic:
with a tiebreak in only one phase, two comments sharing a timestamp could
straddle the boundary differently on successive calls. `id` is a random
identifier, so this ordering is repeatable but is not insertion order; the
requirement and the exclusions both say so.

`limit` is defaulted at 20 for absent, null, zero, negative, and
non-integer values, and clamped at 100 above. Neither is an error, and neither
is invisible: `has_more` and `total` report the omission.

A second bound sits above `limit`: the response carries at most 65536 bytes of
body in total. At the clamped maximum the per-body cap alone would permit a
response near 800KB, which no consuming agent can use, and the point of this
tool is that the result is usable. When the selected window exceeds the budget,
whole comments are dropped from the oldest end, preserving the same
newest-wins rule the window selection already uses; the dropped comments leave
`total` untouched, so `has_more` reports them.

The budget never empties a non-empty window. At the stated constants this holds
by construction rather than by a special case: the per-body cap is 8192 bytes
and the budget is 65536, so the newest comment's shortened body always fits
alone and the drop loop can never reach it. The invariant is written down
anyway, because it is the load-bearing one — an empty list is read by a
coordinator as "the child produced nothing" and ends the delegation — and it is
stated independently of the constants, so that if either ever changes such that
one shortened body could exceed the budget, that comment is still returned
alone.

## Control flow

Caller agent → MCP tool handler → backend action handler → access-checked read
→ comment store, and the projection back out. The caller task identity crosses
the first boundary from server state rather than from the model's arguments;
the target identifier crosses from the arguments.

The acceptance path: a child agent posts its deliverable through the existing
signed runtime comment path; later, the parent's run calls the tool naming the
child, the descendant branch of the read guard admits it, and the body is
returned. Nothing between the two runs is human.

## Failure and recovery

Three outcomes must stay distinguishable, because conflating them is the
reported defect.

- **Accessible and empty** is a success with an empty list, an explicit empty
  array rather than a null, and a zero total.
- **Not permitted** is a forbidden error carrying the existing shared
  access-denied sentinel, whose message is the literal string `document access
  denied`. A missing task and an unrelated task must produce that same forbidden
  outcome and that same message, because distinguishing them would let a caller
  enumerate task identifiers.

  That sameness is not what the guard does today, and closing the gap is the one
  shared change this design owns. `loadAccessPair` calls the task repository for
  the caller and the target and returns the repository's error unchanged. On an
  absent identifier that repository returns a not-found error which embeds the
  identifier it was given, and the MCP handoff error mapper has no case for it,
  so it falls through to an internal error carrying that text. The guard's
  `current == nil || target == nil` denial branch below it is reachable only
  under test, because the test double returns a bare `(nil, nil)` where the
  repository returns `(nil, not-found)`, and the guard's missing-task test
  discards the error, so the divergence is invisible. Normalisation therefore
  happens inside `loadAccessPair`: a not-found from either lookup becomes a
  plain deny, with no error, before any caller sees it. The test double is
  corrected in the same change to return the repository's shape, so the guard's
  tests stop passing on a branch production never reaches.

  Renaming the `document access denied` sentinel text to something
  surface-neutral is deliberately out of scope. That both surfaces read
  identically is what matters; the wording does not, and changing it would churn
  existing assertions for nothing.
- **Dependency unconfigured or storage read failed** is an internal error. This
  path must not degrade to an empty list. The adjacent per-comment run-status
  lookup in the human comments handler does degrade to an empty map, which is
  right there and wrong here: an empty comment list is read by a coordinator as
  "the child produced nothing" and ends the delegation.

A caller task identity that resolves to empty with no `task_id` argument is a
validation error naming `task_id`.

## Persistence

Read-only. No schema change, no migration, no new index; the existing
`(task_id, created_at)` index serves the window.

Both the count and the page are taken inside one read transaction on the
read-only connection, so the returned count never exceeds the reported total. A
comment inserted concurrently either precedes that snapshot or follows it. No
lock is taken that a writer could block on.

## Security

The read relation is not re-derived here. The tool calls the same guard the
document reads call, so the two cannot drift into two answers to one question.
The relation admits self, ancestors, descendants, siblings sharing a non-empty
parent, and blockers, and denies any cross-workspace pair.

Task-identifier enumeration is closed at that guard rather than at this tool, by
the `loadAccessPair` normalisation described in
[Failure and recovery](#failure-and-recovery). Before that change an absent
identifier produced an error naming it; after it, a missing task is
indistinguishable from an existing unrelated one on every surface that uses the
guard.

The caller identity comes from the bound session, so a caller cannot claim to
be a task it is not.

The tool is registered for the Office surface only.

Out of scope and recorded here so it is not mistaken for a new hole: the
`agentctl kandev tasks conversation` command reads the dashboard comments
endpoint, which applies no cross-task relation check. That path predates this
work and is unchanged by it. This design does not route around that endpoint
and does not widen it.

## The advertised-surface contract

The Office first-turn context is asserted to advertise exactly the tool set the
Office surface registers. Registering the tool without adding it to that
context is a build failure, not a silent drift, and adding it to the context
without registering it fails the same way. The injected Office comment
reference currently documents only the write command; it gains the read. The
Coordinator role instructions — the `ceo` role, at
`apps/backend/internal/office/configloader/instructions/ceo/AGENTS.md`, which is
the role that creates subtasks and reviews their results — gain the direction to
read a delegated child's comments before concluding the child produced nothing — without that, the tool
exists and the coordinator still does not call it.

## Observability

The tool's calls are logged through the existing wrapped-handler path with the
tool name, so a coordinator's read attempts are visible next to its other tool
calls. No new metric: the failure this addresses is a missing capability, not a
rate to watch.

## Related decisions

- [ADR 0015 — explicit completion signal for auto-advance](../../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
