# parked-board-mvp — implementation notes

This directory contains the V1 specification and implementation notes for the parked-board
slice. The implementation now wires the process probe, orchestrator projection, agentctl turn
marker, runtime flag, and board-card affordance.

Reference material:
- `docs/specs/parked-board-mvp/spec.md` — the frozen V1 contract.
- `docs/plans/parked-board-mvp/reuse-map.md` — harvest / extract / new / defer map.
- `docs/plans/parked-board-mvp/split-proposal.md` — full context (v4).
- `apps/backend/internal/agentctl/server/process/probe*.go` — the platform-specific probe
  implementation and tests.

The source branch and harvested-parent references below are historical context. Current code
must follow the checked-out implementation and the spec.
