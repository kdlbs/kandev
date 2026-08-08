---
id: "01-snapshot-acp-event-payloads"
title: "Snapshot mutable ACP event payloads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/agent-runtime-availability.md"
---

# Task 01: Snapshot mutable ACP event payloads

## Intent

Make every queued ACP event own an immutable normalized payload so later tool
updates cannot race serialization or retroactively alter earlier events.

## Acceptance

- `NormalizedPayload.Snapshot` detaches every pointer, typed slice, nested map,
  nested slice, and `json.RawMessage` that can be observed through JSON.
- Both `sendUpdate` and `sendUpdateLocked` snapshot while `Adapter.mu` is held
  and before queue ownership transfers.
- Generic, miscellaneous, subagent, monitor, and typed tool payloads follow one
  snapshot rule; the subagent-only clone is removed or delegates to it.
- Mutating an adapter-owned payload after enqueue does not alter queued JSON.
- A concurrent update/marshal regression passes under `go test -race`.

## TDD sequence

1. Add a table test that snapshots each mutable payload category, mutates the
   original deeply, and confirm the snapshot JSON remains unchanged; observe RED
   because no complete snapshot exists.
2. Add an adapter regression that enqueues a generic payload, performs later
   nested updates, and marshals the captured event concurrently; confirm RED or
   a race report on the current aliasing path.
3. Implement the smallest complete snapshot helper and call it at both event
   emission boundaries under the adapter lock.
4. Replace the special-purpose subagent clone, rerun under `-race`, and refactor
   duplicated copy logic without changing the wire shape.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/tool_payload.go`
- `apps/backend/internal/agentctl/types/streams/tool_payload_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/subagent_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/conversion_test.go`

## Dependencies

None.

## Parallelism

`parallel-safe` with Task 02. This task owns normalized stream and ACP adapter
files; Task 02 owns launcher, backend composition, events, and gateway files.

## Verification

- `cd apps/backend && go test -race ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp`

## Inputs

- Confirmed crash stack through `NormalizedPayload.MarshalJSON`.
- `convertToolCallResultUpdate`, `sendUpdate`, and `sendUpdateLocked` ownership
  paths.
- Existing subagent snapshot tests as the nearest regression exemplar.

## Output contract

Record the failing test/race evidence, the final focused race-test result, and
any normalized payload types deliberately excluded because they are immutable.

## Results

- RED: `TestSendUpdateSnapshotsNormalizedPayloadBeforeQueueing` failed because
  the queued generic payload still aliased the adapter-owned nested map.
- Added `NormalizedPayload.Snapshot`, which copies all typed payload pointers
  and slices, recursively clones JSON-compatible maps/slices and
  `json.RawMessage`, and preserves adapter-only provenance markers.
- Both `sendUpdate` and `sendUpdateLocked` snapshot while the adapter lock is
  held. `cloneSubagentPayload` now delegates to the common snapshot contract.
- Focused verification: `go test -run
  'TestSendUpdate(Snapshots|LockedSnapshots)|TestQueuedNormalizedPayloadCanMarshalDuringSourceMutation|TestNormalizedPayloadSnapshotDetachesMutablePayloads'
  ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp`
  — passed (16 tests).
- Required verification: `go test -race
  ./internal/agentctl/types/streams
  ./internal/agentctl/server/adapter/transport/acp` — passed (803 tests).
