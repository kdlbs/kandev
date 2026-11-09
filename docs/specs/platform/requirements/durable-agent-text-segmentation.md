---
status: draft
system: platform
created: 2026-10-26
updated: 2026-10-26
owners:
  - kandev
---

# Durable agent text segmentation requirements

## Overview

Durable delivery must preserve assistant text boundaries around tool use in the
canonical transcript. This extends [idempotent backend projection](durable-agent-delivery.md#req-platform-durable-agent-delivery-005).

## Requirements

### REQ-PLATFORM-DURABLE-AGENT-SEGMENTATION-001: Durable text segments

**Intent:** ID-less assistant text remains coalesced within a segment and is
separate across top-level tool calls.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-AGENT-SEGMENTATION-001.1:** In durable v1, adjacent ID-less assistant text chunks in one segment shall append to one transcript message. A top-level tool call shall end that segment; subsequent text shall begin a new message. Nested tool calls and tool updates shall not create additional boundaries. Native protocol IDs and reasoning messages shall retain their existing identity and type.
- **AC-PLATFORM-DURABLE-AGENT-SEGMENTATION-001.2:** Segment identity shall remain deterministic through duplicate delivery, batching, replay, partial projection, reconnect, and backend or agentctl restart. Projection shall derive from a stable base identity, persist the exact committed ID in the authoritative `canonical_message_id` inbox column, and return that stored ID for duplicates after payload pruning. The projection-only field shall be excluded from immutable transport identity and payload-hash comparisons, and duplicate admission shall preserve its stored value. Projection shall leave caller-owned event data unchanged before commit. A rollback shall not advance identity, message effect, boundary, or cursor. For an older projected row with an empty column, retained payload metadata may supply its stored ID; if the payload is pruned, projection shall return no ID, shall not reconstruct one from incoming metadata or the current boundary, and shall not notify or rewrite a transcript row without a committed ID. Whole-row receipt retirement shall retain existing expired-replay behavior. Existing projected transcript rows shall not be rewritten.
- **AC-PLATFORM-DURABLE-AGENT-SEGMENTATION-001.3:** Every route that projects a top-level tool start shall advance the boundary atomically with its effect and projected cursor, in sequence order, including normal event projection, retained replay, and canonical shell-start projection. Upgrade initialization shall occur once for existing cursor rows, account for a nonzero projected sequence and pruned payloads, and preserve a valid boundary through reopen and owner/generation/cursor updates.

## Implementation records

- [Durable agent text segmentation fix](../../../plans/durable-agent-text-segmentation/plan.md).
