---
id: "01-exact-host-foundation"
title: "Exact Host authorization and durable command receipts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MANAGED-COORDINATION-001
acceptance_criteria:
  - AC-PLUGINS-MANAGED-COORDINATION-001.1
  - AC-PLUGINS-MANAGED-COORDINATION-001.2
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 01: Exact Host authorization and durable command receipts

## Summary

Introduce the common v2 envelope, capability discovery, and durable command admission. Reuse the implemented approval ledger and prove one real task update through the shared command path.

## In scope

- Add exact method/resource registry, typed results, SDK helpers, resource preconditions, and live authorization guards. Do not rebuild approval identity or its JSON ledger.
- Add additive SQL migrations for command intents/receipts and a shared domain operation identity. Implement GetCapabilityContext and UpdateTaskExact as the first vertical command.
- Exercise restart between task commit and receipt completion, retry payload mismatch, revocation races, and stale workspace/resource versions. Update protocol and authoring contract documentation for the shipped subset.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- A capability-approved exact task update commits once across retry and crash; a stale or revoked request has no new effect.
- Capability discovery and both SDKs distinguish supported v2 operations from unchanged v1 APIs.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
make -C apps/backend proto
(cd apps/backend && go test -race ./internal/plugins ./internal/task/service -run 'TestExactHostAdmission|TestExactTaskUpdateRecovery' -count=1)
(cd apps/backend && go test ./pkg/pluginsdk/... ./internal/plugins/manifest/...)
(cd apps && pnpm --filter @kandev/plugin-sdk test && pnpm --filter @kandev/plugin-sdk typecheck)
```

Evidence to create:

- `apps/backend/internal/plugins/host_exact_test.go`: `TestExactHostAdmission`.
- `apps/backend/internal/task/service/service_exact_operation_test.go`: `TestExactTaskUpdateRecovery`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/backend/internal/plugins/approval_api.go`
- `apps/backend/internal/plugins/host_write.go`
- `apps/backend/internal/plugins/manifest/capability.go`
- `apps/backend/internal/task/service/`
- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/pkg/pluginsdk/`
- `apps/packages/plugin-sdk/src/index.ts`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

None.

## Risks

Receipt persistence after a domain effect is not sufficient deduplication. Couple the operation identity to the domain commit or reconcile from an authoritative record.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

Implemented additive Host v2 capability discovery and exact task-update contracts
in the protobuf API, Go and TypeScript SDKs. Exact updates use live workspace
approval, a durable intent/receipt store, and a task-repository operation identity
committed in the same transaction as the task mutation. Retries recover the same
result; changed payloads and stale versions conflict. Revocation is serialized
against effect admission. Updated plugin authoring and contract documentation.

Validation passed: `make -C apps/backend proto`; focused race tests for plugin
admission and task recovery; `go test ./pkg/pluginsdk/... ./internal/plugins/manifest/...`;
plugin SDK tests and typecheck; public documentation validation (62 tests, 47
pages); and `git diff --check`. The exact task recovery test also verifies one
domain operation record and that replay does not update the task again.
