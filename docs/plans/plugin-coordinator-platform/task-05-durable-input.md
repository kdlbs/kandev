---
id: "05-durable-input"
title: "Durable ordered managed conversation input"
status: complete
wave: 5
depends_on: ["01-exact-host-foundation", "03-managed-lifetime", "04-restricted-tools"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MANAGED-COORDINATION-003
acceptance_criteria:
  - AC-PLUGINS-MANAGED-COORDINATION-003.1
  - AC-PLUGINS-MANAGED-COORDINATION-003.2
  - AC-PLUGINS-MANAGED-COORDINATION-003.3
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 05: Durable ordered managed conversation input

## Summary

Deliver accepted inputs in order through shared runtime dispatch. Expose receipts and reconcile uncertain outcomes without blind replay.

## In scope

- Implement enqueue/get/list/cancel input and separate immediate dispatch busy semantics. Persist dedup keys and FIFO sequence under one admission lock.
- Reuse shared queue/runtime paths with one durable owner; implement bounded admission, pause, explicit periodic coalescing, and exact active cancellation.
- Inject crashes before start, after launch, and after tool effect. Reconcile against execution identity before restarting accepted work.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- Human inputs queued during a running turn survive restart and complete in order without duplicate admission.
- Identical retries return one receipt; mismatched payloads conflict and uncertain execution remains visible.
- Pause and cancellation preserve unrelated inputs, and only eligible periodic inputs coalesce.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd apps/backend && go test -race ./internal/plugins ./internal/orchestrator/messagequeue/... -run 'TestManagedInputReceipts|TestManagedInputRecovery' -count=1)
(cd apps/backend && go test ./internal/task/service -run 'TestManagedConversation|TestAgentConversation' -count=1)
```

Evidence to create:

- `apps/backend/internal/plugins/host_managed_inputs_test.go`: `TestManagedInputReceipts`.
- `apps/backend/internal/orchestrator/messagequeue/managed_inputs_test.go`: `TestManagedInputRecovery`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/backend/internal/plugins/host_agent_conversations.go`
- `apps/backend/internal/task/service/agent_conversations.go`
- `apps/backend/internal/orchestrator/messagequeue/`
- `apps/backend/internal/orchestrator/`
- `apps/backend/pkg/pluginsdk/`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 01](task-01-exact-host-foundation.md)
- [Task 03](task-03-managed-lifetime.md)
- [Task 04](task-04-restricted-tools.md)

## Risks

An accepted receipt cannot be treated as model completion. After a possible tool effect, replay requires reconciliation or explicit human action.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

Implemented durable managed-input receipts, bounded FIFO admission, retry identity,
exact cancellation, explicit periodic coalescing, and recovery that does not blindly
replay uncertain execution. Immediate dispatch rechecks live task/session state at
the adapter boundary and returns `busy` without adding queue work. Periodic
coalescing moves replacement input to the FIFO tail with a new sequence, preserving
cursor order. Paused conversations do not drain accepted input.

Updated the public plugin authoring, Host API, gRPC contract, and manifest
capability documentation for receipt states, cursor bounds, FIFO order, scoped
cancellation, coalescing, pause/resume, and immediate-dispatch behavior.

Validation passed:

- `go test -race ./internal/plugins ./internal/orchestrator/messagequeue/... -run 'TestManagedInputReceipts|TestManagedInputRecovery' -count=1`
- `go test ./internal/task/service -run 'TestManagedConversation|TestAgentConversation' -count=1`
- Managed-input race tests in `internal/orchestrator/messagequeue` and
  `internal/task/service`, managed lifecycle/pause tests in `internal/orchestrator`,
  and immediate-dispatch adapter tests in `internal/backendapp`.
- `make -C apps/backend build` (multi-target backend build).
- `node --test scripts/validate-public-docs.test.mjs` (62 tests) and
  `node scripts/validate-public-docs.mjs` (47 pages).
- `git diff --check`.

Go test and build commands used the task-specific `TMPDIR` and `GOTMPDIR` because
the shared `/tmp` filesystem filled during concurrent work in another workspace.
