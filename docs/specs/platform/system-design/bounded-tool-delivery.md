---
status: current
system: platform
created: 2026-10-22
requirements:
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-001
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-002
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-003
---

# Bounded generic tool delivery

## Boundary and scope

Platform owns preparation before immutable delivery acceptance. This narrows
an earlier draft to the focused implementation requested by the user.
[Durable delivery](durable-agent-delivery.md), existing shell representation,
permission and workflow authority retain their contracts.

| Requirement | Sections |
| --- | --- |
| REQ-PLATFORM-BOUNDED-TOOL-DATA-001 | Preparation and admission |
| REQ-PLATFORM-BOUNDED-TOOL-DATA-002 | Snapshot and compatibility |
| REQ-PLATFORM-BOUNDED-TOOL-DATA-003 | Existing presentation |

## Failure

Generic MCP results are assigned verbatim. A 1,048,605-byte result exceeds the
1 MiB journal event budget once serialized; duplicate content can also overflow
the 4 MiB writer queue. `Manager.forwardUpdates` returns on the write error and
cancels the current submission, so later terminal output never reaches the
backend. Request size and provider-specific limits cannot fix this boundary.

## Preparation and admission

`streams.BoundGenericToolEvent` handles only generic `tool_call`/`tool_update`
events. It copies the payload envelope and bounds generic input/output with a
conservative 64 KiB JSON budget per value, 256 visited values and depth 16.
String accounting includes JSON escaping and UTF-8 rune boundaries; opaque raw
JSON and keys use conservative escaped-byte reservations.
Wide/deep or unsupported subtrees are omitted without full traversal or marshal.
Small supported values retain JSON meaning, including typed nil containers.

An oversized value becomes ordinary JSON `{truncated: true, preview: ...}`.
Its preview has a separate 4 KiB budget and reserves nodes and depth for the
wrapper and omitted leaf on later preparation. Map traversal is deterministic
and key inventory is bounded before sorting. Byte/resource bodies are either complete within budget or omitted;
never publish a cut binary block as a valid image.

The same event's `ToolCallContents` receives a bounded preliminary size check
before serialization. If it exceeds the content budget, replace the duplicate
carrier with a small text block containing the marked JSON and mark the generic
output too. Preserve substantive-content presence, actual tool status and IDs.
When the content is the only result carrier, retain a bounded text preview from
it. Fitting content slices, blocks and mutable scalar pointers are copied.

Prepare before ACP `sendUpdateNow`/`sendUpdateLocked` snapshots and native Codex
`emit`, with a defensive call in `Manager.persistDeliveryEvent` before writer
queue accounting. This also covers direct producers and legacy delivery. The
helper never touches permission/control events or compact shell patches.

## Snapshot and compatibility

Preparation does not mutate the original normalized object or content blocks.
Existing ACP snapshots detach the prepared copy across the asynchronous queue.
Native/direct preparation detaches the selected JSON data. Small representations
and reduced wrappers are stable when prepared repeatedly.

The flag lives inside existing generic JSON input/output, not a new typed
NormalizedPayload or AgentEvent field. Existing normalized hashing already
covers its full arbitrary JSON value. Readers retain and render it without any
new protocol, journal, SQL or hash version. Historical accepted data is never
passed through the reducer on replay; existing changed-replay checks remain.
No version migration, hash backfill or forced runtime replacement is needed.

## Existing presentation

The generic tool renderer already formats arbitrary JSON on desktop and phone.
It displays the `truncated` flag and bounded `preview` using its existing output
presentation. No new layout, action, localization string or hover dependency is
introduced. The wrapper does not match complete structured-result shapes or
carry adapter-issued permission/background authority. The native agent still
receives its original result; Kandev offers no download of omitted content.

## Verification

The real process regression uses an oversized nested MCP result with duplicate
content, followed by text and a real completion. It asserts retained event order,
complete/error/cancelled tool status, healthy delivery and an unchanged source.
Shared tests cover escaping, Unicode, huge/deep/cyclic JSON-like values, nils,
small values, idempotence and trusted provenance. Adapter tests exercise both ACP
emission paths and native emission before queueing. Run targeted checks and race
coverage named by the [work order](../../../plans/bounded-tool-delivery/task-01-bounded-preparation.md).

## Decision

[Bound before acceptance](../../../decisions/2026-10-22-bounded-tool-display-data.md).
