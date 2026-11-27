---
id: "01-prompt-lifecycle-snapshots"
title: "Serialize prompt lifecycle snapshots"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.5
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 01: Serialize prompt lifecycle snapshots

## Scope

- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/agent/runtime/lifecycle/execution_store.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_recovery_turn_outcome.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_lock_order_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/execution_store_prompt_lock_test.go`
- `apps/web/e2e/tests/task/mobile-html-preview.spec.ts`
- `apps/web/e2e/tests/task/mobile-pr-unlink-sidebar-menu.spec.ts`

## Acceptance

- Payload transitions acquire the prompt lifecycle mutex before the global
  store mutex and read the turn while both ownership boundaries are held.
- Deletion or re-registration after lookup prevents the old helper callback.
  A recovery caller retaining an earlier pointer cannot mutate a replacement,
  even when the helper captures that replacement as its own current pointer.
- Cancellation produces its ready event; passthrough returns its one-shot
  running publisher with an empty ACP turn; recovery returns ready availability.
  A late completion claims the original generation and leaves the store usable.
- Preserve existing native recovery and publication guards, persistence order,
  and synchronous/asynchronous publication boundaries.

## Verification

Hosted real-package regression command:

```sh
cd apps/backend
go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'TestMarkReadyDoesNotDeadlockWithPromptCompletion|TestWithPromptLifecycleLockRejectsRemovedOrReplacedExecution|TestPrepareRecoveredAvailabilityPreservesReplacement' -count=3
```

Broad backend, frontend and E2E suites run in hosted CI. Local proof uses a
contained extraction of exact merged-source functions with documented boundary
fakes, bounded process timeout and deliberate mutations. That proof does not
establish real-package integration or full runtime race safety.

## Results

The contained extraction passed all three race-enabled repetitions. Six deliberate
mutations failed on the intended assertions: old lock ordering, removed helper
identity check, removed stale-caller check, invalid cancellation event status,
missing passthrough publisher and missing recovery payload. Old ordering failed
all three deadlock cases within the bounded process deadline. No production
remediation beyond the contributor fix was necessary.

Documentation metadata/specification validation and exact changed-path coverage
passed. Exact-head hosted real-package and broader verification remains a
delivery gate, recorded outside this head-agnostic implementation record. Public
docs need no change: existing session recovery, payload and ownership contracts
are preserved.

The next hosted pass includes protected main's prepared-payload admission change.
Its typed callbacks, immutable SQL payload checks and Send Now claim guards remain
unchanged. Hosted E2E exposed missing synchronization in two existing phone tests:
native preview selection now waits for the inspector's capture-mode acknowledgement
before synthetic pointer input, and task-row hit-area measurement waits for the
drawer animation to finish. The same screenshot and 44px assertions remain intact.
These changes affect test scheduling only; their rendered verification remains
part of the hosted E2E delivery gate.
