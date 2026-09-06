---
status: draft
system: tasks
requirements:
  - REQ-TASKS-PLAN-APPEND-006
created: 2026-09-05
owners:
  - kandev
---

# Task plan append-mode agent text System Design

## Purpose and boundaries

This design owns `REQ-TASKS-PLAN-APPEND-006`: every agent-facing string that describes a
plan write, the class rule that binds them, the truncation warning's rewrite, and the
acknowledgement. It defines no composition, locking or size-limit behavior — those stay in
[the main design](plan-write-append-mode.md), which this file was split out of on
2026-09-05 when that file reached its 32,768-byte lint ceiling. No content was dropped in
the move.

Split here rather than compressed away because the alternative was deleting Build guidance
that closed earlier review findings, which would have reopened them. The companion-file
pattern follows `external-id-idempotency-operations.md` in this same directory.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-PLAN-APPEND-006` | [Agent-facing description and acknowledgement](#agent-facing-description-and-acknowledgement) |

## Agent-facing description and acknowledgement

`REQ-TASKS-PLAN-APPEND-006` is satisfied in the tool registration in
`internal/mcp/server/server.go`, not any runtime path. The description an agent reads is
the only place the choice between modes is explained, so it is contract, not commentary.

**`AC-TASKS-PLAN-APPEND-006.9` binds a CLASS of strings, not a list.** No agent-facing
text describing a plan write may claim that no append mode exists or prescribe
read-then-resend. Two review rounds enumerated these texts and each list came up one
short, so the criterion is stated over the class: Build sweeps `origin/main` for the
claim and fixes what it finds, rather than trusting any enumeration including this one.
On 2026-09-05 the sweep matched five strings, all of which change: the
`update_task_plan_kandev` description (`server.go:1846`) and its `content` parameter
(`:1848`); `create_task_plan_kandev`'s `content` parameter (`:1832`) **and its own tool
description** (`:1830`); and both branches of the truncation warning. Site `:1830` is the
one both earlier enumerations missed — it says the tool replaces everything "same as
update_task_plan_kandev through a different door" and tells the caller to read the plan
first, so the equivalence turns false the moment `update` gains `append` and the second
clause is exactly the prescription `006.9` removes. Treat that list as of its date.

The `update_task_plan_kandev` description asserts there is no partial update, append or
section-patch mode, and that sending only a new section "will silently delete everything
else"; left alone it argues against the mode being added. Its `content` parameter also
carries the 256 KiB cap sentence from `plan-content-size-limit`, which stays and applies
to the composed document once `append` exists (`REQ-TASKS-PLAN-APPEND-007`). Rewritten,
the description must name both modes and the default (`AC-TASKS-PLAN-APPEND-006.1`),
keep the deletion warning **scoped to `replace`** rather than dropping it (`006.2`), and
state that `append` inserts a blank line before the fragment (`006.3`), is not
idempotent so a resubmission adds it again (`006.4`), and does not require reading the
plan first (`006.5`). The `mode` parameter's own description carries the two accepted
values and the default. The deletion warning must survive because replace is still the
destructive path; softening it to make room for append would trade one silent data loss
for another.

Both `create_task_plan_kandev` strings deny the mode exists and prescribe the
read-then-resend pattern this capability removes, on the same tool
`AC-TASKS-PLAN-APPEND-005.2` makes *reject* a mode by naming
`update_task_plan_kandev`'s append. `AC-TASKS-PLAN-APPEND-006.9` fixes the replacement.
The boundary Build must hold: the `mode` property, if declared, states only that its
value is rejected, per
[the two sibling surfaces](plan-write-append-mode.md#the-two-sibling-surfaces-which-behave-differently-on-purpose),
which explains why this tool's schema declares it at all — the platform's generic
argument-schema validator would otherwise shadow `createTaskPlanHandler`'s own,
correctly-worded rejection with a generic error naming neither tool.

### The truncation warning is the most urgent of them

`planTruncationWarning` in `internal/mcp/handlers/task_plan_guard.go` renders both of
its branches around this sentence:

> Plan writes REPLACE THE ENTIRE DOCUMENT — there is no partial update or append mode.

Shipping append while leaving that in place denies the mode at the one moment an agent
has just destroyed content and would most benefit from being told it exists, and steers
it back toward the replace path that caused the loss. `AC-TASKS-PLAN-APPEND-006.7` and
`006.8` require that sentence replaced by one naming `append` as the way to add a
section without resubmitting the document.

Everything else in that warning must survive, for reasons recorded in its doc comment
rather than obvious: it deliberately does not tell the caller to "recover" the content
through an MCP tool, because no plan tool reads a past revision and
`get_task_plan_kandev` would hand back the truncated document; it names where the prior
content lives; and it tells the caller not to rewrite from memory. Naming `append`
strengthens that advice rather than competing with it — the caller is told what to do
instead of what just failed. Keep the rune counts, the percentage, the two branches on
whether the prior revision number is known, and the response field.

This is Go string content, not localized UI copy, so the i18n rules do not reach it.
`AC-TASKS-PLAN-APPEND-005.1` excludes this wording from the replace-mode freeze
precisely so the two criteria do not contradict each other.

### Acknowledgement

The acknowledgement needs **no change**, verified rather than assumed. `planWriteAck`
in `internal/mcp/server/handlers.go` already prefers the stored `content` length over
what the call sent, renders it as bytes, and already appends a line stating that plan
content is omitted and should be read back with `get_task_plan_kandev`. That is exactly
`AC-TASKS-PLAN-APPEND-006.6`, satisfied by existing behavior on both modes.

Build must not add plan content to that acknowledgement. Doing so would return the
accumulated document on every append and give back the response-leg saving this
capability exists to obtain — the ack omits content deliberately, because echoing the
stored plan to the agent that just wrote it was measured as the largest single
contributor to context growth in plan-heavy sessions.

## Test strategy

**Description tests** assert the class rule by ITERATION, not by naming strings: walk
every registered plan-tool description and parameter description plus both rendered
warning branches, and assert none carries the no-append claim or the read-then-resend
prescription (`AC-TASKS-PLAN-APPEND-006.9`). A per-string test would have passed against
a spec that missed one, which is what happened twice. This must not exempt
`update_task_plan_kandev`'s own strings from the walk — the tool whose description most
recently carried the banned claim is exactly the one a special-cased test would stop
checking. Also assert the warning names `append` while still stating where the prior
content lives and that the caller must not rewrite from memory (`006.7`, `006.8`), and
that `create_task_plan_kandev`'s texts point at `update_task_plan_kandev`, and that its
`mode` property (whether or not the schema declares one) states only that the value is
rejected, never a functioning capability. Assert on substantive clauses, not whole
strings, so they do not become punctuation change-detectors.
