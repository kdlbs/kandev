---
status: draft
system: platform
created: 2026-10-22
updated: 2026-10-27
owners:
  - kandev
---

# Bounded tool delivery requirements

## Overview

Platform owns preparation of tool reports before durable acceptance. Large display data must not interrupt an otherwise valid agent turn.
This contract covers every tool category and provider. The native agent keeps its original tool result.

The [durable delivery requirements](durable-agent-delivery.md) own persistence, uncertainty, recovery, and admission.
The [shell delivery requirements](incremental-shell-delivery.md) retain their output and write-volume guarantees.

## Requirements

### REQ-PLATFORM-BOUNDED-TOOL-DATA-001: Size-safe tool reports

**Intent:** Large tool reports must preserve subsequent output and the actual terminal outcome.

#### Acceptance criteria

- **AC-PLATFORM-BOUNDED-TOOL-DATA-001.1:** When tool display data exceeds its inline budget, Kandev shall retain a bounded copy.
  Delivery shall continue without size-induced cancellation, replacement, or storage-error state.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-001.2:** This guarantee shall cover every tool category, provider, and supported result carrier.
  Coverage shall include arguments, nested JSON, typed output, rich content, collections, Unicode, escaping, and duplicate carriers.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-001.3:** Preparation shall preserve tool identity, category, actual status, and trusted adapter provenance.
  It shall not change native results, execute tools, dispatch prompts, or manufacture permission or control evidence.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-001.4:** Preparation shall bound its additional memory, traversal, and nesting after source decoding.
  It shall not serialize or clone a complete oversized tree before reduction.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-001.5:** Fitting supported values shall preserve their meaning, including nulls and explicit empty values.
  Repeated preparation shall preserve the prepared bytes and omission markers.

### REQ-PLATFORM-BOUNDED-TOOL-DATA-002: Immutable retained representation

**Intent:** Prepared reports remain ordinary durable evidence.

#### Acceptance criteria

- **AC-PLATFORM-BOUNDED-TOOL-DATA-002.1:** Kandev shall commit the prepared representation before publication.
  Replay shall preserve accepted payloads, hashes, ordering, identity, and actual status without further reduction.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-002.2:** Preparation shall not mutate provider objects or cumulative output.
  Later source mutation shall not alter the retained copy.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-002.3:** Existing supported readers shall retain the prepared report through known JSON fields.
  This change shall not require a journal, SQL, or payload-hash format migration.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-002.4:** Compatible adoption shall preserve existing streams and submissions.
  Older producers shall retain their supported behavior until normal replacement. Adoption shall not rewrite history or force replacement to obtain new limits.

### REQ-PLATFORM-BOUNDED-TOOL-DATA-003: Existing incomplete-output representation

**Intent:** Existing output representations remain honest without new interface controls.

#### Acceptance criteria

- **AC-PLATFORM-BOUNDED-TOOL-DATA-003.1:** Every reduced report shall carry an explicit truncation marker and a bounded preview.
  Existing generic `truncated: true`, `preview`, and typed truncation fields shall remain supported.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-003.2:** Existing tool rows, disclosures, labels, and recovery controls shall retain their layout and actions on desktop and phone.
  Existing truncation indicators shall remain usable. This repair shall not add an omission widget or preview control.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-003.3:** A preview shall not appear as a complete image, executable patch, complete file, or permission decision.
  The interface shall not offer retrieval of omitted output that Kandev does not retain.

### REQ-PLATFORM-BOUNDED-TOOL-DATA-004: Aligned delivery capacity

**Intent:** The event limit provides useful headroom without creating another delivery bottleneck.

#### Acceptance criteria

- **AC-PLATFORM-BOUNDED-TOOL-DATA-004.1:** The default event payload limit shall be 2 MiB of encoded JSON.
  Admission shall account for escaping, duplicate fields, and delivery metadata before publication.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-004.2:** A valid typed search report containing 1,048,607 ordinary text bytes shall fit without display reduction.
  A report exceeding the display budget shall use the bounded representation instead.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-004.3:** Every admitted event shall fit the supported live transport and replay path.
  Replay shall bound bytes and record count, preserve contiguous order, and make progress across a short byte-limited page.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-004.4:** The event increase shall preserve the existing prompt-size limit and supported shell tails.
  It shall not introduce an operator setting or change retained evidence.
- **AC-PLATFORM-BOUNDED-TOOL-DATA-004.5:** A remaining oversized control envelope shall produce a distinct size error.
  Kandev shall not classify that error as storage exhaustion or wait for an acknowledgment to make it fit.

## Out of scope

- New UI components, tool-output notices, preview controls, labels, or localization work.

- Full original-output storage, download, or artifact references.
- Increasing prompt limits or the supported shell tails.
- Reducing permission, workflow, ownership, or background-work authority.
- Changing accepted events during replay or projection.
- Repairing a live session or rewriting its database records.

## Related documents

- [System design](../system-design/bounded-tool-delivery.md).
- [Fix package](../../../plans/resilient-agent-output-delivery/plan.md).
- [Original generic implementation](../../../plans/bounded-tool-delivery/plan.md).
- [Boundary decision](../../../decisions/2026-10-27-all-tool-delivery-boundary.md).
