---
created: 2026-10-20
status: completed
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-002
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
legacy_specs: []
---

# Implementation plan: SSH agentctl absence idle reclaim

## Overview

Restore idle reclaim for issue #4593 when the SSH controller process is confirmed absent despite a failed agentctl HTTP status request. This record captures the completed bounded repair and its parent-reviewed evidence.

## Scope and technical approach

Lifecycle consults an optional runtime absence capability only after failed HTTP status. SSH uses the tracked instance's positive PID and existing connection, the existing precise remote probe, a five-second budget, and a final instance/client/PID/transport check. Unknown evidence preserves the original HTTP error. Existing reaper eligibility, cleanup identity fences, stop ownership, and resume-record rules remain authoritative.

Redial policy, transport-loss recovery, new runtime support, persistence changes, and UI changes are outside this repair.

## Tests

`AC-EXECUTORS-IDLE-PARKING-002.3` maps to real lifecycle/fake SSH probe and cleanup tests, plus periodic reaper/real lifecycle/SQLite integration with a capability-complete executor fake. The latter proves confirmed absence reaches reclaim and preserves resume identity; unknown/error evidence preserves execution ownership. The SSH tests separately cover actual command evidence and concurrent invalidation.

## Work orders

- [x] [Task 01: Confirm SSH controller absence before idle reclaim](task-01-confirm-absence.md)

## Verification results

The work order records the exact commands and passing results. Parent independently reran the focused race suite and approved the seven-file implementation candidate before delivery. These two delivery records were added afterwards to satisfy PR traceability; they do not change the approved implementation.

## Risks and limits

All SSH evidence uses a local fake SSH server. The reaper integration replaces only the executor boundary. No live host, complete backend suite, browser, or release validation is claimed. Delivery and current-head CI/review state are tracked in the task's live plan.
