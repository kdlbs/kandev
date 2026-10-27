# Worker node policy follow-up: local validation

Recorded 2026-10-28, Asia/Singapore. Added to the existing draft PR at the owner's explicit request, after the original executor closeout at `f92cb324ab`. Earlier Testing/Review verdicts apply to their recorded SHAs, not this follow-up.

## Change

Administrator-owned `KANDEV_NODE_LAUNCH_WRAPPER_FILE` enforces an outer wrapper independently of optional profile wrappers on session, continuation, interactive, ACP utility and native Codex utility launches. The allocation reference comes from the worker process environment. Required-policy bootstrap preserves the node identity across private env sourcing and rejects an incompatible agentctl. Policy validation checks root ownership, canonical path, trusted parent chain, permissions, size, strict JSON and an absolute executable. Unconfigured workers retain existing behavior.

## Verified locally

- After fast-forward to `f92cb324ab`, targeted Go race checks passed in all four packages: `internal/common/launchwrapper`, `internal/agent/nomad`, `internal/agentctl/server/process`, `internal/agentctl/server/utility`, with `-run 'NodePolicy|NodeAllocation|GenericWrapper|Bootstrap' -count=1`.
- Earlier full launchwrapper and Nomad package tests, affected-package vet/lint, and an agentctl build passed. The full backend suite has not been rerun for this addition.
- Separate FleetFoundry fixtures passed 55 tests / 35 subtests and actual pinned nono process checks for linked worktree and fresh SSH-style checkout shapes. Those are external integration checks; their implementation remains in FleetFoundry, not this repository.

## Still unproven

Actual SSH transport, installed native-client credentials/model/tool/telemetry flow, new-policy restart/cancellation/GC, and Tank/Neo acceptance remain open. A supplementary unfiltered process/utility package run was stopped without a result and is not counted as passing. Existing executor closeout failures and exclusions remain documented separately. This change does not implement scheduler wrapper-capability admission or confine control-plane APIs. No live deployment or service change was performed; PR #4835 remains draft.
