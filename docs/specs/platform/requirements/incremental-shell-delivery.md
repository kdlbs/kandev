---
status: active
system: platform
created: 2026-10-19
owners:
  - kandev
---

# Incremental shell delivery requirements

## Overview

Platform owns normalized event delivery, replay evidence, and operational storage
bounds. Agents owns upstream ACP interpretation. Tasks and the existing shell
output disclosure consume the resulting bounded output. This capability extends
[digest-backed storage](pr-watch-and-bounded-storage.md) with a lifetime byte-volume
bound; its existing cadence guarantee alone does not provide that bound.

This extension preserves the existing
[UI output contract](../../ui/requirements/acp-shell-command-output.md), including
live availability and final result semantics, remains the observable baseline.
The version-2 storage representation replaces that document's historical
no-new-table implementation scope, without adding an interaction or UI requirement.

## Terminology

- **Supported output:** The latest normalized UTF-8-safe stdout/stderr tails,
  each at most 256 KiB, including explicit empty values, truncation and nullable
  exit status. This does not promise an unlimited historical transcript.
- **Changed bytes:** Bytes added to a normalized tail or supplied by an authoritative
  replacement. Unchanged prefixes in repeated cumulative snapshots do not count.
- **Encoding epoch:** A tool's baseline established at first admission or after
  verified loss of encoder state. A different tool, parent, prompt generation,
  session incarnation or harness generation cannot reuse that baseline.

## Requirements

### REQ-PLATFORM-INCREMENTAL-SHELL-001: Bounded shell delivery write volume

**Intent:** A quiet or slowly streaming hours-long command shall not repeatedly
persist its entire accumulated tail to keep live output and recovery current.

#### Acceptance criteria

- **AC-PLATFORM-INCREMENTAL-SHELL-001.1:** On compatible runtimes, serialized
  accepted shell-output bodies in both journal and inbox shall grow with changed
  output bytes and bounded per-event metadata/checkpoint overhead, rather than
  retained-tail size times update count. This shall hold before and after tail
  saturation, for fast bursts, one update per second, sparse updates and
  interleaved tools. Unchanged output-only snapshots shall cause no new output
  body write. Identical text with changed truncation, status or exit metadata
  shall still preserve that change.
- **AC-PLATFORM-INCREMENTAL-SHELL-001.2:** Reconstructed live and terminal output
  shall equal existing normalization for every supported carrier, including
  deltas, cumulative snapshots, authoritative replacement, omitted or explicitly
  empty streams, command-echo normalization, multibyte boundaries, nonzero,
  unknown and cancelled results. Existing first/trailing publication and
  250 ms cadence requirements continue to apply when consumers are available.
- **AC-PLATFORM-INCREMENTAL-SHELL-001.3:** Accepted output shall survive the
  existing supported disconnect/restart boundaries and replay in order exactly
  once into canonical output. Duplicate replay shall not append twice; changed
  replay or a missing/wrong baseline shall fail without advancing projection,
  acknowledgment or dispatch authority. Missing recovery evidence shall remain
  an explicit recovery failure, never an empty successful result.
- **AC-PLATFORM-INCREMENTAL-SHELL-001.4:** Incremental canonical persistence shall
  avoid creating a new full retained-tail blob for each live change. The
  existing lightweight browser summary and authorized output-detail endpoint
  shall expose the latest committed result during execution and after reload,
  terminal completion and backend recovery on desktop and phone.
- **AC-PLATFORM-INCREMENTAL-SHELL-001.5:** A new backend shall continue to adopt
  supported retained runtimes using the old full-snapshot contract. Compact
  output shall require mutually supported delivery semantics; incompatible
  readers shall not reinterpret it as empty output or trigger replacement work.
  Upgrade shall preserve identities, accepted evidence and active submissions.
  Limits of old-runtime improvement and rollback shall be documented.
- **AC-PLATFORM-INCREMENTAL-SHELL-001.6:** Isolated verification shall record new
  output bytes, event counts, serialized journal/inbox bytes, canonical payload
  bytes submitted to storage and physical fixture journal/DB/WAL sizes
  separately. Slow and fast saturated fixtures shall demonstrate a byte bound
  and correct final output, including restart and duplicate replay, without
  shortening retention or deleting unacknowledged evidence.

## Out of scope

- Provider output never supplied to Kandev, or retaining more than supported tails.
- Conversation-fetch volume, manual VACUUM, live database cleanup and instance restarts.
- Changing unrelated tools, background-work provenance, permission or turn authority.
- Claiming the improvement for old runtime producers or unrecognized carrier shapes.

## Implementation plan

[Shell-output write amplification](../../../plans/shell-output-write-amplification/plan.md).
