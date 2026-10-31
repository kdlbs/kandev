---
created: 2026-10-22
status: implemented
requirements:
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-001
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-002
  - REQ-PLATFORM-BOUNDED-TOOL-DATA-003
system_design:
  - ../../specs/platform/system-design/bounded-tool-delivery.md
legacy_specs: []
---

# Implementation plan: bounded tool delivery

## Overview

Fix the oversized generic tool-result rejection directly. The user requested
implementation with minimal surrounding work on 2026-10-22, superseding the
broader draft package. One work order replaces the proposed protocol, schema
and UI work: retain a bounded preview inside existing generic JSON values and
keep the existing renderer, journal format and normalized hash contract.

## Scope

Bound generic input/output and their duplicated content before adapter cloning,
queueing and journal acceptance. Preserve the native result, tool identities,
status and trusted provenance. An oversized result must not stop forwarding the
following text or actual terminal. Small values remain unchanged.

Full-output storage/download, new typed event fields, protocol/schema/hash
versions, UI components/locales, logging redesign, non-generic tool policy,
permission changes and repair of the live affected task are excluded.

## Evidence

Two GitHub MCP results each contained 1,048,605 text bytes. The 1 MiB event cap
rejected the serialized event; forwarding stopped and cancellation followed.
The backend received no terminal. Baseline: `10f5852d8362218767b7471dfa2543ae156c2bd3`.
A synthetic real-forwarder regression reproduces the failure without private data.

## Technical approach

A shared streams helper bounds JSON-compatible generic values with encoded-byte,
depth and node budgets. Oversized values become ordinary JSON objects containing
`truncated: true` and a bounded `preview`. These live inside known generic
input/output fields, so existing marshaling, hashes, persistence and rendering
already understand the representation. No typed marker or version migration.

Apply before ACP Snapshot and native Codex emission; guard direct process
producers too. Bound duplicate tool content without emitting partial binary
blocks. Do not touch permission/control events or compact shell patches.

## Tests

- Real process forwarding: oversized nested MCP result and duplicate content,
  then ordinary text and the real terminal. Preserve complete/error/cancelled
  tool statuses, ordered replay, a healthy delivery path and the source result.
- Shared value/snapshot tests: encoded escaping, Unicode, huge collections,
  depth, unchanged small values, bounded preview and repeat preparation.
- Adapter tests: real ACP and native emission seams retain only bounded values.

## Compatibility

Generic JSON is already opaque data inside a known field. Old readers hash and
render the shortened object as ordinary tool output. Existing raw and compacted
records are not normalized again or rewritten. Retained old runtimes keep their
producer behavior; adoption does not forcibly replace them.

## Work orders

- [x] [Task 01: Fix oversized generic tool delivery](task-01-bounded-preparation.md)

## Verification results

Implemented and verified on 2026-10-22. Task 01 records the red reproduction,
passing targeted/race checks across four packages, complete adapter/streams test
runs, clean changed-file lint, documentation gates and scoped exclusions.
Requirements are active and the paired design is current for this narrowed fix.
No commit, push or live-instance change was performed.

## Risks

The preview is intentionally incomplete. It must not be interpreted as a
complete structured tool result or confer control authority. Queue memory is
bounded after source decoding; upstream provider/decoder memory remains outside
this change. Genuine storage failures keep their existing recovery behavior.

## Follow-up scope

The [resilient output package](../resilient-agent-output-delivery/plan.md) extends the implemented generic policy.
It owns typed reports, the 2 MiB event cap, byte-bounded replay, and independent writer-failure recovery.
The completed work order and verification results here retain their original narrower scope.
