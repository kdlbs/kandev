---
id: "04-legacy-v1-serialization"
title: "Preserve legacy v1 event serialization"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-006
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-002.1
  - AC-PLATFORM-DELIVERY-RETENTION-006.1
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention-payloads.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 04: Preserve legacy v1 event serialization

## Outcome and owned change

Retained events can contain `tool_output_bytes` from an older producer. The v1
recipe decodes a typed `streams.AgentEvent` and re-encodes it. A backend that did
not know this field stored a payload and receipt hash without it. A newer backend
must accept that receipt when it replays the original field-bearing journal event.

Keep the legacy read-compatibility `*uint64` field in
`apps/backend/internal/agentctl/types/streams/agent.go`, after `ToolStatus` and
before `NormalizedPayload`. Current producers do not set it. Receipt comparison
must validate the stored v1 hash against a supported canonical form, then compare
the incoming event against the exact form or the form with only this field
omitted. Keep the stored payload, hash, envelope checks and lifecycle-field
stripping unchanged.

## Acceptance and exclusions

1. Absent/nil keeps current serialized bytes unchanged. Present zero, nonzero and
   the maximum unsigned value retain the historical key and serialization order.
2. Decode/encode round trips preserve the field; invalid field types remain normal
   decoding errors.
3. Fixed v1 hash cases cover absent, null, zero, nonzero, maximum and invalid
   values. Receipt tests cover field-less historical hashes, field-bearing hashes,
   compacted receipts, replay, corruption and unchanged envelope/content guards.
4. No progress/stall producer, format migration, permission, new setting, database
   write, prompt replay or verification bypass is added.

## Regression coverage and current results

Permanent cases extend the existing stream payload and SQLite hash-comparison
test files. They pin JSON
serialization and order, fixed v1 digests, old-reader replay, exact field-bearing
receipt checks, compaction hash preservation and replay after pruning. The tests
also reject invalid field types, changed envelope/content and corrupt stored hashes.

The proposed fixtures initially omitted the four capability booleans that the
unchanged parent AgentEvent already serializes. Expected JSON and fixed v1
vectors now include those existing false defaults. The existing hash framing was
checked independently against those explicit bytes; the algorithm and runtime
wire fields are unchanged. Capability changes remain conflicting normalized
content, covered for all four fields.

With valid fixtures, the original replay comparison rejects all three mixed-reader
numeric cases (0.079s, no corrupt-receipt error). The corrected serialization,
fixed-vector, replay, corruption and compaction controls pass under fts5/race
detection (streams1.046s, SQLite1.908s). Hosted CI is separate. No stored receipt
is rewritten, and this change does not perform a database repair or trigger
runtime publication. The historical operating receipt failure remains unresolved
until the corrected reader validates its separately owned sample; these fixtures
are source compatibility evidence, not an operating repair result.

The raw Windows conformance run fails its existing POSIX-only temporary-path
fixture; the old replay comparison reproduces that same storage CRUD failure
(0.234s). With the prior disposable fixture changing only two path literals to
filepath.Join(os.TempDir(), ...), full store conformance passes (56.795s).
Product path validation and conformance assertions are unchanged. PostgreSQL
test DSN is not configured in this execution environment, so this records SQLite
coverage rather than a PostgreSQL run.
The unchanged full SQL guard also passed for this candidate.

## Current upstream CI baseline

A normal main merge adopts the upstream version-scoped update-notification
localization oracle after CI found two matches under one generic-title
locator. The upstream oracle exercises distinct versions and preserves the
localization assertion. The incoming
CI/cache changes keep their existing budgets; the old Windows persistence
compile-only job had missed both caches and hit its 10-minute deadline.

On the merged source, legacy output-byte/mixed-reader/receipt controls passed
with race detection (streams 1.059s, SQLite 1.180s); existing normalization,
snapshot and ordinary hash controls passed separately (1.052s/1.168s). The
normal merge hooks remain required before publishing this baseline update.
