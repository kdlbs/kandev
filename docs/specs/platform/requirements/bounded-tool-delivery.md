---
status: active
system: platform
created: 2026-10-22
owners:
  - kandev
---

# Bounded generic tool delivery requirements

## Overview

Platform owns normalized delivery before journal acceptance. Generic tool input,
results and duplicate content can be arbitrarily larger than a storage record.
The user selected a marked, bounded Kandev copy and then requested a focused fix
without the surrounding protocol, database or UI work proposed in the first draft.
This contract preserves the agent's own result and the existing
[durable-delivery contract](durable-agent-delivery.md).

## Requirements

### REQ-PLATFORM-BOUNDED-TOOL-DATA-001: Size-safe generic tool data

**Intent:** Large generic results must not interrupt valid turn delivery.

#### Acceptance criteria

- **AC-PLATFORM-BOUNDED-TOOL-DATA-001.1:** For a generic tool result exceeding
  the inline budget, Kandev shall retain a bounded copy and continue delivering
  subsequent events and the actual terminal outcome without size-induced
  cancellation, replacement or storage-error state.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-001.2:** The same behavior shall apply across
  providers and generic tool names, including arguments, nested JSON, Unicode,
  escaped text, large collections and duplicate content carriers.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-001.3:** Reduction shall preserve identity,
  actual tool status and trusted adapter provenance. It shall not change native
  agent results, issue tools/prompts or manufacture permission/control evidence.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-001.4:** Additional preparation memory and
  traversal shall be bounded by byte, node and nesting budgets after source
  decoding, without serializing or cloning the complete oversized JSON tree.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-001.5:** Small supported JSON values shall
  preserve their JSON meaning, including nulls. Preparing an already bounded
  representation again shall preserve its bytes and truncation marker.

### REQ-PLATFORM-BOUNDED-TOOL-DATA-002: Immutable retained representation

**Intent:** The shortened copy remains ordinary durable tool evidence.

#### Acceptance criteria

- **AC-PLATFORM-BOUNDED-TOOL-DATA-002.1:** The bounded JSON copy and explicit
  truncation marker shall be journaled before publication and survive normal
  replay/persistence through the existing generic data fields. Previously
  accepted payloads and hashes shall not be rewritten or reduced during replay.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-002.2:** Preparing an emitted event shall not
  mutate the original provider payload or its cumulative content. Source
  mutation shall not change the prepared retained copy.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-002.3:** Existing readers shall accept the
  shortened copy as ordinary generic JSON without a protocol/schema/hash
  upgrade. Existing store, adoption and genuine persistence-failure behavior
  shall remain unchanged.

### REQ-PLATFORM-BOUNDED-TOOL-DATA-003: Explicitly incomplete copy

**Intent:** Omitted data must not appear to be a complete result.

#### Acceptance criteria

- **AC-PLATFORM-BOUNDED-TOOL-DATA-003.1:** A shortened generic input/output shall
  contain an explicit `truncated: true` flag and bounded `preview` in the existing
  JSON presentation. The preview shall not be a partial image or other malformed
  rich block, confer control authority, or offer unavailable full-output retrieval.

## Out of scope

New UI/copy, full original-output storage/download, protocol/schema/hash
migrations, logging redesign, non-generic tool representation, assistant content
limits, shell patches, permissions, workflow changes and live-task repair.
Unsupported non-JSON runtime values can be omitted in the marked preview.

## Related documents

- [System design](../system-design/bounded-tool-delivery.md).
- [Implementation package](../../../plans/bounded-tool-delivery/plan.md).
- [Decision](../../../decisions/2026-10-22-bounded-tool-display-data.md).
