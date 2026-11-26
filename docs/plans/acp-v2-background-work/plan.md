---
created: 2026-11-25
status: implemented
requirements:
  - REQ-AGENTS-ACP-V2-002
  - REQ-AGENTS-ACP-V2-003
  - REQ-AGENTS-BACKGROUND-WORK-001
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-004
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-006
system_design:
  - ../../specs/agents/system-design/acp-v2-preview.md
  - ../../specs/agents/system-design/background-work.md
legacy_specs: []
---

# Fix ACP v2 background work projection

## Overview

Feed the existing Background Jobs chip and viewer with terminals and children
from the attached root session's live ACP v2 background list.
[GitHub issue #5507](https://github.com/kdlbs/kandev/issues/5507)

## Root cause and reproduction

In `acpv2/projectUpdate`, the root `state_update` branch calls
`observeForeground` and returns. That method reads state and stop reason.
The terminal and child handlers emit transcript entries. The adapter never
consumes `StateUpdate.Background` for workload events.

The temporary `TestWireProjectsBackgroundSnapshot` ran at `c671d8080883`
with `AgentBackgroundWorkEnabled` set to true. It used `newWireFixture` to
announce one terminal and one child and send an idle snapshot with both IDs.
Both transcript events and the ordering marker arrived. The test failed
with 0 workload updates where it expected 2. The test ran through a Go overlay.

## Technical approach

1. Read the background field alongside foreground state in
   `acpv2/updates.go`. Compare the snapshot with one map keyed by entity
   kind and wire ID for the attached native session.
2. Use the existing terminal and child metadata to emit changed
   `streams.EventTypeBackgroundWorkUpdated` observations. A matching
   terminal exit updates the same record even after foreground idle.
3. Route the events through existing workload storage and viewer code. Use
   the existing root session attachment boundary for replay.

The scope is read-only inspection of the provider's root list. A detached
child has one row for its background activity. Codex uses the same child
ID across turns.
[Wire mapping](../../specs/agents/system-design/acp-v2-preview.md#background-work-projection)
and [Codex child identity](https://github.com/agentclientprotocol/codex-acp/blob/v2.8.0/src/subagents/SubagentAssociations.ts)

## Tests

`TestWireProjectsBackgroundSnapshot` proves delivery to the workload stream.
Extend the same fixture for repeated lists, unknown and empty lists,
child removal and return, and terminal exit. The work order maps exact
checks to the existing criteria.
[Background Work requirements](../../specs/agents/requirements/background-work.md)
and [ACP v2 preview requirements](../../specs/agents/requirements/acp-v2-preview.md)

## E2E tests

Use the existing Background Work viewer specs in Chromium and Mobile Chrome
for the chip and History. An isolated Codex session checks the full path from
a provider snapshot to the chip and retained history.

## Work orders

- [x] [Task 01 · Project ACP v2 background work](task-01-project-background-work.md)

## Verification results

The wire regression failed before implementation with 0 of 2 expected workload
updates. The adapter race suite and
`TestBackgroundWorkProjectionReplayAndSuccessor` pass. `make -C apps/backend
build` passes for the backend targets. The desktop Active/History viewer test
and the phone summary viewer test each pass. Public-doc validation passes all
63 tests and validates 56 published pages. Spec catalog validation covers 464
decisions and 2114 specifications; full spec lint passes. Frozen pnpm install
and `git diff --check` pass.

The changed-package Go lint helper could not start because other concurrent
`golangci-lint` runs held the repository lock. The optional live-provider check
was unavailable because this workspace has no isolated Codex ACP profile.

Review follow-up: added wire regressions for an idle child with listed nested
work, independently base64-encoded terminal snapshots and chunks, launch
generation retained across a later list and exit, identical snapshots across
the replay marker, reference-first terminal delivery with the feature off, and
repeat snapshots. The complete ACP v2 adapter race suite, full backend build,
spec catalog validation, full spec lint, and diff check pass after these fixes.

## Risks

Codex can remove a child from the list during a foreground wait. Keep its
record available for later activity and use terminal exit evidence for shell
outcomes. In a live check, capture the list that the provider sends.
