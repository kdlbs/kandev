---
status: current
system: tasks
created: 2026-10-03
requirements:
  - REQ-TASKS-PLAN-READ-001
  - REQ-TASKS-PLAN-READ-002
  - REQ-TASKS-PLAN-READ-003
owners:
  - kandev
---

# Partial task plan reads System Design

## Purpose and boundaries

Extend the existing task-plan read operation with bounded ranges. Reuse
`edit_task_plan_kandev` and `update_task_plan_kandev(mode="append")` for writes.
Their current compact acknowledgements already avoid full-plan output.
There is no new plan model, database schema, tool name, or rendered UI.

The tasks system owns the snapshot and authorization contract. Agent prompts
and frontend-generated system context consume it without taking ownership.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-PLAN-READ-001` | Read contract; Range projection |
| `REQ-TASKS-PLAN-READ-002` | Snapshot and authorization; Read/edit flow |
| `REQ-TASKS-PLAN-READ-003` | Guidance and public documentation |

## Current integration points

- `internal/mcp/server/server.go`, `registerPlanTools`: current read schema has
  only optional `task_id`.
- `internal/mcp/server/handlers.go`, `getTaskPlanHandler`: forwards the task ID
  to `mcp.get_task_plan`, then emits metadata and exact content text blocks.
- `internal/mcp/handlers/handlers.go`, `handleGetTaskPlan`: calls
  `PlanService.GetPlanSnapshot` and returns `planReadPayload`.
- `internal/task/service/plan_service.go`, `GetPlanSnapshot`: authorizes, holds
  the existing per-task writer lock, and reads one coherent row.
- `internal/task/planws/errors.go`, `GetError`: maps read failures but currently
  does not preserve conditional-version error details.

## Read contract

Extend `get_task_plan_kandev` and its existing backend action with optional
`offset`, `limit`, and `expected_version`. Preserve presence independently from
zero values. Reject null, booleans, strings, fractions, negative offset, and
limits outside 1..8,192. Offset must be a safely representable JSON integer
(at most 9,007,199,254,740,991); do not convert unchecked floating-point values
to an `int`. The service validates bounds again for direct backend calls.
Reject a supplied empty version instead of treating it as a valid token.

When neither range field is present, use the existing full-read projection.
When either is present, default offset to zero and limit to 4,096 code points.
The maximum range is 8,192 code points, bounding content to at most 32,768 UTF-8
bytes for valid stored UTF-8 text. Metadata remains a small fixed field set.
Do not publish a default range in the schema that clients might inject into
legacy full-read calls; describe the conditional default in parameter text.

Keep two text blocks for a present plan: metadata followed by exact content.
Partial-mode metadata adds:

| Field | Meaning |
| --- | --- |
| `partial` | Always true for a ranged request, including an entire short plan |
| `total_characters` | Whole snapshot's code-point count |
| `total_content_bytes` | Whole snapshot's byte count |
| `offset` | Requested zero-based code-point offset |
| `limit` | Effective requested character limit |
| `returned_characters` | Actual fragment character count |
| `content_bytes` | Fragment byte count, matching the content block |
| `has_more` | Fragment ends before total characters |
| `next_offset` | Exclusive end offset when `has_more`; otherwise null |

Preserve existing identity, title, timestamps, and `version` fields. Full-mode
metadata and exact content remain compatible with current clients. Do not
attach the full plan to structured results, metadata, or errors in partial mode.
An EOF request still returns metadata and an empty exact-content text block.

## Snapshot and authorization

Use `GetPlanRead` and result types in `internal/task/service/plan_partial_read.go`.
Presence-aware options, limits, and the strict wire parser live in the neutral
`internal/task/contract/plan_read.go`, shared by both MCP boundaries and the service.
Share
snapshot logic with `GetPlanSnapshot` through a helper rather than calling a
lock-taking method from inside the same non-reentrant lock.

After validating request-only shape, authorize the addressed task before any
state-dependent check. Under the existing per-task lock, read HEAD once,
compare a supplied version, and project the range and metadata from that row.
Do not reread the version separately or fetch counts from another snapshot.
Keep `GetPlanSnapshot` available to existing full-read consumers.

A missing plan keeps the existing empty backend object/no-plan MCP result when
no version is supplied. With `expected_version`, a missing HEAD is a version
conflict: a pinned page must not silently become a different read sequence.
Use existing `PlanSafetyError` and `plan_version_conflict` for mismatches.
Extend `planws.GetError` to preserve its typed details. Unauthorized callers
must still reveal no version or counts. Invalid state-dependent offsets return
`plan_read_offset_out_of_range` and a correction; no plan content is included.
Invalid request fields return validation errors and never become full reads.

## Range projection

Count code points with the existing Unicode convention used by
`models.PlanContentLength`. Find the start and end byte boundaries with a UTF-8
scan, then slice the original string. Do not normalize Markdown, CRLF,
whitespace, or combining sequences. A code point boundary can split a visual
grapheme; returned text is an exact substring suitable for literal matching.
Use comparisons/subtraction to avoid overflowing `offset + limit`.

An offset beyond the total is rejected; equality returns an empty final page.
Historical oversized heads remain readable because this operation applies only
the response range limit, not the new-write storage limit. Repository reads may
still load the full row into backend memory. The goal is agent token and
response reduction, not database streaming or CPU optimization.

Character ranges keep one very long line bounded and avoid line-specific
clipping/cursors. Section names and fuzzy locators need Markdown semantics and
ambiguity rules; they are outside this small extension. This routine API choice
is recorded here and does not require a separate architecture decision.

## Read/edit flow

1. Call `get_task_plan_kandev(offset=0, limit=4096)` for a bounded first page.
2. To continue, pass `offset=next_offset`, the desired limit, and the first
   page's `expected_version`. Stop or reconcile on conflict.
3. Select an exact fragment from the returned content and pass its version,
   `old_text`, and `new_text` to `edit_task_plan_kandev`.
4. Use the successful write's new version for another known fragment edit.
   Earlier pagination offsets refer to the earlier version; after a write,
   restart pagination or read a deliberately selected range of the new version.
5. Add a new section with append, which keeps its existing optional version and
   non-idempotent retry rules. Never send a range as a replacement document.

Literal uniqueness is still checked against the entire stored plan. A fragment
unique within a page can remain ambiguous globally; the edit service rejects it
and the agent fetches sufficient additional context rather than guessing.

## Guidance and public documentation

Update task and Office context, plan-mode/default-plan prompts, applicable
workflow instructions, and the active-plan system block produced by
`buildDocumentContext` in `apps/web/hooks/use-message-handler.ts`. Recommend
bounded reads for focused updates and exact edit/append for local writes.
Allow a full read when the whole document is needed. Preserve the existing
system wrapper, task/session placeholders, stop/approval behavior, and every
question, title, completion, autopilot, and delegation rule.

The frontend change affects only agent-facing prompt construction. It adds no
rendered copy, layout, viewport branch, navigation, or touch interaction.
Mobile-parity assessment therefore uses shared prompt unit tests; new mobile
Playwright scenarios or ASCII UI previews would not exercise a changed surface.
Keep the existing agent-prompt i18n exemption with its explanation.

At implementation time, update the reference/how-to sections in
`docs/public/automation-and-mcp.md`, `tasks-and-workflows.md`,
`agent-communication.md`, and the MCP action contract in `websocket-api.md`.
Use an example ranged read followed by an exact edit; document units, bounds,
conflicts, EOF, full-read compatibility, and append retry behavior.

## Persistence and observability

No migrations, new indexes, revisions, events, runtime flags, or metrics.
Never log content or fragments. Existing bounded error correlation is enough.
Reads must leave title/content/version/history/comments/implementation state
unchanged, including on failure.

## Verification

Service tests cover exact Unicode and CRLF ranges, defaults, EOF, unsafe numeric
inputs, overflow-safe bounds, oversized/long-line heads, authorization, and
pinned-version conflicts. MCP schema and forwarding tests prove presence and
metadata semantics on the real dispatcher bridge. An end-to-end MCP journey
reads a bounded fragment of a long plan, edits it, and proves the remaining
bytes survive; a browser/service write between pages must cause conflict.
Existing full-read and safe-edit tests remain required. Work orders name the
exact commands, prompt tests, public-doc validators, and lifecycle promotion.

## Related design

- [Safe agent plan edits](plan-safe-edits.md)
- [Append mode](plan-write-append-mode.md)
- [Conditional plan writes ADR](../../../decisions/2026-09-16-conditional-agent-plan-writes.md)
