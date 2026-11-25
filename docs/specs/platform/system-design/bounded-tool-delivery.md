---
status: draft
system: platform
created: 2026-10-22
updated: 2026-10-27
requirements:
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-001
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-002
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-003
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-004
---

# Bounded tool delivery

## Boundary

Platform prepares the immutable Kandev copy before asynchronous queues and journal acceptance.
The native harness keeps its original result. This design extends the generic policy to all tool reports.

[Stream processing](durable-agent-stream-processing.md) owns persistence and acknowledgment.
[Delivery failure handling](durable-delivery-failure.md) owns recovery when persistence cannot continue.
[Incremental shell delivery](incremental-shell-delivery.md) retains its patch format, supported tails, and write-volume guarantee.

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-BOUNDED-TOOL-DATA-001 | Preparation and category policies |
| REQ-PLATFORM-BOUNDED-TOOL-DATA-002 | Retained representation and compatibility |
| REQ-PLATFORM-BOUNDED-TOOL-DATA-003 | Existing omission representation and interface |
| REQ-PLATFORM-BOUNDED-TOOL-DATA-004 | Capacity contract and replay |

## Confirmed failure

Codex ran `rg` through its shell tool. ACP classified the report as `search`, which Kandev normalized to `code_search`.
The completion contained 1,048,607 text bytes and serialized to 1,060,995 bytes before submission metadata.
The 1 MiB journal rejected it.

`BoundGenericToolEvent` returns immediately when `NormalizedPayload.Generic()` is nil.
The typed report therefore retained its oversized `ToolCallContents`.
`Manager.forwardUpdates` returned on the append error and cancelled the native turn.
Subsequent terminal output never reached the backend.

The completed generic package remains historical evidence for its narrower scope.
The [new package](../../../plans/resilient-agent-output-delivery/plan.md) owns this extension.

## Capacity contract

A proposed dependency-neutral package, `internal/agentctl/types/deliverylimits`, owns the shared defaults and their relationships.
Journal, producer, client, and replay code consume this contract. This repair adds no operator setting or runtime flag.

| Boundary | Budget | Rule |
| --- | --- | --- |
| Encoded event payload | 2 MiB | Includes the prepared event and delivery metadata allowance |
| Aggregate tool display data | 1536 KiB | Counts typed output, rich content, duplicate carriers, and escaping together |
| Delivery envelope allowance | 64 KiB | Reserved inside the event cap for delivery fields |
| Existing generic JSON value | 64 KiB | Retains the current value, node, and depth policy |
| Omission preview | 4 KiB | Encoded preview with marker overhead reserved |
| Writer queue | 4 MiB | Fits one maximum event and retains a fixed byte bound |
| Backend dispatch queue | 4 MiB | Fits the complete live envelope |
| Encoded replay response | 4 MiB | Includes base64 expansion, event envelopes, and stream metadata |
| Prompt submission | 1 MiB | Independent of the larger event limit |
| Shell stdout/stderr tails | 256 KiB each | Existing normalization and patch limits remain authoritative |

The display allowance admits the observed search result intact.
Existing generic values retain their stricter policy.
Queue limits bound queued bytes, rather than the whole process heap.

`journal.Config.MaxEventBytes` currently limits both events and submissions.
Introduce an internal `MaxSubmissionBytes` value and getter.
Its default is the smaller of 1 MiB and an explicitly smaller event limit.
Explicit internal submission overrides remain available for fixtures.
Update HTTP admission and retained payload validation to use this separate limit.

Disk write headroom accounts for the larger encoded record and bbolt/base64 overhead.
Ordinary display data cannot consume the existing reserve for bounded control and terminal records.
The separate 4 MiB SQL retention budget remains unchanged.
A maximum admitted event must fit retention proof accounting without repeated over-budget skips.

## Preparation

Generalize the shared helper into a proposed `PrepareToolEvent` operation.
ACP `sendUpdateNow` and `sendUpdateLocked`, native Codex `emit`, and direct producers use it before snapshots or queue admission.
`Manager.persistDeliveryEvent` repeats preparation with its actual journal budget.

The helper inventories known display fields using bounded byte, node, and depth accounting.
It reserves JSON escaping costs and counts duplicate carriers together.
It copies only fitting data or a bounded preview.
It does not marshal or clone the complete oversized source first.
Provider decoding remains outside the additional-memory guarantee.

Serialize the bounded candidate once. Before sequencing, validate the complete event size with reserved delivery metadata.
The journal independently checks the payload limit.
An envelope that still cannot fit returns a proposed `ErrEventTooLarge`.
This permanent size error never enters the capacity wait or ACK loop.

Preparation is immutable, deterministic, and idempotent.
The existing generic policy keeps its 256-node and depth-16 bounds.
Typed collections also use explicit bounded traversal. String copies preserve UTF-8 boundaries.

## Category policies

Classification selects a typed policy. It never bypasses common content accounting.

| Category | Display data | Preserved semantics |
| --- | --- | --- |
| Generic/MCP | Input, output, duplicate content | Name, actual status, lineage, provenance |
| Code search | Files, rich/text output | Query/path identity, supplied totals, actual status |
| File read | Read output, duplicate text | File identity, offset, supplied line count, explicit empties |
| File edit | Mutation bodies, duplicate diffs | Operation/path identity and the real repository mutation |
| Shell | Extra display carriers and labels | Exit status, tails, truncation, patch epochs and baselines |
| HTTP | Response and duplicate content | Request identity and actual error status |
| Subagent | Prompt/result display | Child identity, lifecycle, lineage, trusted background evidence |
| Plan/todos/task display | Text and display collections | Stable item identity and actual item state |
| Unknown tool | Known common content and generic presentation | No inferred control authority |

Retain a bounded collection subset and an explicit omission marker.
Keep supplied total counts. Do not invent unknown totals.
Keep fitting binary blocks whole. Replace oversized binary blocks with a text notice.
Never publish a cut image, audio block, resource, or diff as valid structured content.

Remove oversized mutation bodies as whole display fields.
The preview cannot become an executable patch or complete-file representation.
Native edits remain independent of the display copy.

Identity, permissions, workflow signals, and background-work provenance are not display fields.
Unknown large control extensions produce a typed error instead of arbitrary JSON reduction.
Assistant/reasoning text keeps its existing chunk path.

Compact shell patches retain their existing format and encoder.
Bound extra carriers before patch encoding without changing supported output or explicit empty replacements.
Validate the final patch envelope without changing revisions or digests.

## Existing omission representation

Reuse generic `{truncated: true, preview: ...}` values and existing typed `Truncated` fields.
Reuse the current rich-content fallback: a bounded text block containing a truncation marker and preview.
Do not introduce a reserved `Data` namespace, new message metadata, or a common projection layer.

Count these existing representations inside the display budget.
When a typed body cannot express partial content safely, omit that body as a whole.
For file mutations, omit oversized body-bearing entries rather than constructing a misleading empty file or partial patch.
Preserve the tool category, call identity, target path, and actual status.
Keep the bounded marker in an existing content carrier.

Existing renderers retain their current behavior and supported indicators.
An internal marker does not require a new visible control when the current renderer does not show that carrier.
Do not add a tool-result notice, preview component, or new output-detail endpoint.
Native results remain unchanged.

Known event fields preserve current serialization and normalized hashing.
No new event field, projection contract, hash version, journal format, or SQL column is required.
Canonical and identified legacy delivery continue through existing message paths.
Accepted records never pass through preparation again.

## Replay and compatibility

`Journal.Replay` currently limits record count alone.
Add a byte ceiling before materializing a full page.
Count encoded journal records, including base64 payload expansion, and reserve response-envelope space.
The first next event must fit the configured response budget.
A historical event that cannot fit returns a typed error without cursor advancement.

Apply this budget to HTTP replay, WebSocket catch-up, and retained inspection.
Callers continue to the captured high-water mark after short byte-limited pages.
The record limit remains an additional bound.
An empty page before that mark remains a sequence/recovery error.

Advertise optional event/replay budget metadata through the authenticated delivery capability.
These fields describe limits and grant no ownership or dispatch authority.
For older peers without replay-byte metadata, request one event per HTTP page.
Bound response reads and report unsupported oversized responses explicitly.

| Backend / producer | Behavior |
| --- | --- |
| New / new | 2 MiB admission, all-category preparation, byte-bounded replay |
| New / retained old v2 | Unchanged records, single-event HTTP replay, reported delivery-health checks |
| Supported old v2 / new | Known fields remain readable, subject to real replay/hash compatibility fixtures |
| Identified legacy | New producers prepare display data before the existing transport boundary |
| Unknown control shape | Typed refusal and visible recovery, without inferred capability |

A smaller append limit must not reject supported historical reads.
Do not replace a retained agent to obtain new producer limits.
Existing pre-v2 rollback restrictions remain in force.

## Existing interface

Keep current tool renderers, output disclosures, recovery copy, and action eligibility.
This package creates no visible output widget, new control, layout, drawer, or route.
It requires no new locale keys or renderer metadata.

Delivery failures reach the existing recovery surface through the current backend recovery model.
Desktop actions retain their current composition. Phone actions retain their current touch behavior.
Tests verify existing controls, draft retention, and reload behavior without redesigning them.

## Verification

A synthetic search fixture contains 1,048,607 ordinary text bytes.
It crosses the real forwarder, bbolt replay, backend projection, actual terminal settlement, and next-prompt admission.
Larger fixtures exercise reduction for every category and duplicate carrier.

Shared tests cover escaping, UTF-8, nulls, explicit empties, bounded work, immutable snapshots, and repeat preparation.
Replay tests cover near-limit records, short pages, restart, duplicate receipt, and old-reader normalization.
Shell tests preserve epochs, baselines, digests, supported tails, and write-volume bounds.
Exact commands belong to the [work orders](../../../plans/resilient-agent-output-delivery/plan.md).

## Decisions

The [all-tool boundary decision](../../../decisions/2026-10-27-all-tool-delivery-boundary.md) extends
[the original generic decision](../../../decisions/2026-10-22-bounded-tool-display-data.md).
