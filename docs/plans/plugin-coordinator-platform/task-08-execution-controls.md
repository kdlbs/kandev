---
id: "08-execution-controls"
title: "Guarded execution, recovery, and human interactions"
status: complete
wave: 8
depends_on: ["01-exact-host-foundation", "04-restricted-tools", "06-workspace-observations", "07-task-commands"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-COORDINATION-006
acceptance_criteria:
  - AC-PLUGINS-COORDINATION-006.1
  - AC-PLUGINS-COORDINATION-006.2
  - AC-PLUGINS-COORDINATION-006.3
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 08: Guarded execution, recovery, and human interactions

## Summary

Expose exact execution controls and interaction response adapters. Use native consent and provider capability checks.

## In scope

- Implement ensure/stop/recover run and exact pending-transition cancellation using normal orchestrator and runtime lifecycle services.
- Implement exact permission/clarification response with host-issued single-use human response receipts. First valid response wins.
- Expose supported session modes only; exclude permission bypass and provider credential-lock repair. Return typed unavailable/unsupported states.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- A stop or recovery request cannot affect a replacement execution or a newer pending transition.
- Only an exact human-authorized response can approve a permission once; stale/duplicate/forged responses fail.
- Recovery and mode changes preserve restricted tool policy and normal task workflow admission.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd apps/backend && go test -race ./internal/plugins -run 'TestExactExecutionControls|TestExactHumanInteraction' -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'Test.*PendingMove|Test.*Recover|Test.*Stop' -count=1)
```

Evidence to create:

- `apps/backend/internal/plugins/host_exact_execution_test.go`: `TestExactExecutionControls`.
- `apps/backend/internal/plugins/host_exact_interactions_test.go`: `TestExactHumanInteraction`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/backend/internal/plugins/host_interactions.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/agent/runtime/`
- `apps/backend/internal/task/service/`
- `apps/backend/pkg/pluginsdk/`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 01](task-01-exact-host-foundation.md)
- [Task 04](task-04-restricted-tools.md)
- [Task 06](task-06-workspace-observations.md)
- [Task 07](task-07-task-commands.md)

## Risks

Current MCP parent-only stop helpers are not general plugin authority. Adapters must call shared domain services with the exact caller policy.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

Completed 2026-09-26. Added exact protobuf and Go SDK methods for guarded run
ensure/stop/recovery, pending-transition cancellation, supported mode queries and
updates, and permission/clarification replies. Backend adapters use the normal
orchestrator and lifecycle services, fence observed execution generations, refuse
recovery of manually cancelled sessions, and filter permission-bypass or
credential-repair modes. Human responses require a five-minute, single-use Host
receipt issued by the authenticated native UI, bound to the workspace, pending
interaction revision, response kind, and exact payload. Receipt issuance uses the
native session-control authorization. Legacy v1 interaction mutation methods
remain in the wire schema but return PermissionDenied; reads remain available.

Reconciled the public SDK/authoring docs, gRPC contract, managed-coordination
design, and earlier interaction and Host decisions with the consent boundary.
Protobuf regeneration passed. Required race tests passed for exact execution and
human interaction controls; orchestrator pending-move, recovery, and stop
regressions passed. Receipt handler, adapter-fencing, gRPC round-trip, plugin
Host, SDK, and backendapp tests passed. The broader lifecycle run exposed an
outdated path-redaction assertion; it now checks the sanitizer's path-redacted
marker, and that regression plus managed-policy admission tests pass. Public docs
validation (62 tests, 47 pages), docs/spec catalog and lint, spec-linter tests
(36), and whitespace checks passed. Changes are carried on the feature branch.

### Review remediation (2026-09-27)

Pending EnsureTaskRun and RecoverSession replays now preserve the uncertain result
when no receipt exists; stored successful run/recovery and stop results replay with
their original payload. Exact execution controls carry the observed management
instance and generation, and the backend checks all six control operations under
the shared task-claim fence. Pending-replay and Host execution tests passed with
the race detector; backend ownership-admission tests passed with the race detector,
and the service transfer/release effect-boundary race regression passed.
