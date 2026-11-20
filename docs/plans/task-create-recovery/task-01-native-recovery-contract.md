---
id: "01-native-recovery-contract"
title: "Add the native recovery contract and receipt"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CREATE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-CREATE-RECOVERY-001.1
  - AC-TASKS-CREATE-RECOVERY-001.2
  - AC-TASKS-CREATE-RECOVERY-001.3
  - AC-TASKS-CREATE-RECOVERY-001.4
  - AC-TASKS-CREATE-RECOVERY-001.5
  - AC-TASKS-CREATE-RECOVERY-001.6
  - AC-TASKS-CREATE-RECOVERY-001.7
  - AC-TASKS-CREATE-RECOVERY-001.8
  - AC-TASKS-CREATE-RECOVERY-001.10
  - AC-TASKS-CREATE-RECOVERY-001.11
system_design:
  - ../../specs/tasks/system-design/task-create-recovery.md
---

# Task 01: Add the native recovery contract and receipt

## Summary

Provide a tested native creation owner that retains identity through uncertainty.
Add definitive rejection evidence and the existing external-ID lookup to the browser API.
Leave product dialog wiring for task 02.

## In scope

- Native `createTask` input support for `external_id`, required metadata decoding, and lookup response types.
- A versioned captured receipt and sessionStorage persistence scoped to user, workspace, and draft.
- A synchronous guard, exact payload replay, sticky uncertainty, and result ownership checks.
- `check` and explicit `retry` operations using the existing read-only lookup before same-key POST.
- Known-task hydration, incomplete outcomes, identity-lost outcomes, and deletion/archive/access fences.
- Descriptor-only persistence, legacy inline-data replay blocking, and observable storage errors.
- Optional `creation_rejected: true` from proven native pre-allocation errors only.
- Preservation of existing error statuses, codes, and branch-policy details.

## Out of scope

- Rendered recovery controls, translations, or production dialog integration.
- Database/schema changes, server recovery workers, or generalized creation frameworks.
- Changes to Quick Chat, plugin, WS, edit, or additional-session contracts.
- Treating all 4xx errors or lookup misses as proof that no task exists.

## Acceptance

1. Receipt/hook tests prove stable identity, captured payload, scoped restoration, and one admitted operation across retries and late responses.
2. Handler tests prove rejection evidence appears only before allocation. A later rejection after uncertainty never retires the original browser identity.
3. Found/incomplete, known-task absence, malformed responses, failed storage, and missing inline bytes retain safe recovery without another identity or launch.

## TDD sequence

1. Add `TestHTTPCreateTask_CreationRejectionEvidence` and record its missing-marker RED result.
2. Add receipt and hook behavior tests before their minimal implementation.
3. Cover a retry rejection while the original request remains unresolved, then resolve the original task.
4. Run the exact checks below and record actual results.

The hook tests need a captured server identity map, not a mock that always returns one arbitrary task.
Prove one task ID for one key and a different ID for an independent submission.
All proposed modules below are new files. Existing named symbols remain source dependencies.

## Verification

Run from the repository root. If dependencies are absent, first run `(cd apps && pnpm install --frozen-lockfile)` once.

```bash
(cd apps/backend && go test ./internal/task/handlers -run 'TestHTTPCreateTask_(CreationRejectionEvidence|PostAllocationErrorIsUncertain|ExternalIDGoldenPath|FoundSettledSkipsPostCreateWork|FoundOutcomeIgnoresNonexistentRepositoryInRetry|FoundUnsettledReportsIncomplete)|TestHTTPGetTaskByExternalID_' -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/handlers/task_http_handlers.go apps/backend/internal/task/handlers/task_http_create_rejection.go apps/backend/internal/task/handlers/task_http_create_rejection_test.go
(cd apps/web && pnpm exec vitest run lib/task-creation-receipt.test.ts components/task-create-dialog-recovery.test.tsx lib/api/domains/kanban-api.test.ts lib/api/domains/kanban-api-creation.test.ts)
(cd apps/web && pnpm exec eslint lib/task-creation-receipt.ts components/task-create-dialog-recovery.ts lib/api/domains/kanban-api.ts lib/api/domains/kanban-api-creation.test.ts lib/types/http.ts)
(cd apps/web && pnpm run typecheck)
```

Include any additional changed Go file in the changed-file lint invocation before recording completion.

## Files likely touched

- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_create_rejection.go` (new create-only response helper)
- `apps/backend/internal/task/handlers/task_http_create_rejection_test.go` (new)
- `apps/backend/internal/task/handlers/task_http_handlers_external_id_test.go`
- `apps/web/lib/api/domains/kanban-api.ts`
- `apps/web/lib/api/domains/kanban-api.test.ts`
- `apps/web/lib/api/domains/kanban-api-creation.test.ts` (new)
- `apps/web/lib/types/http.ts`
- `apps/web/lib/task-creation-receipt.ts` and `apps/web/lib/task-creation-receipt.test.ts` (new)
- `apps/web/components/task-create-dialog-recovery.ts` and `apps/web/components/task-create-dialog-recovery.test.tsx` (new)

Read `PreAllocationTaskCreateError` in `service/preallocation_error.go`.
Reuse that evidence without changing service allocation or rollback behavior.
Keep new recovery modules within the scoped complexity limits.

## Dependencies

None. Reuse shipped external-ID uniqueness, settlement, lookup, and existing browser storage primitives.
The draft aggregate-creation protocol is not a prerequisite.

## Risks

- A first request can remain live after its browser response fails. The sticky uncertainty test is mandatory.
- `dto.FromTask` can omit session fields in a found response.
- Generic storage wrappers silently ignore write failures. The receipt needs an observable result.
- Native response typing must not break custom adapter contracts.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/task-create-recovery.md), criteria .1 through .8, .10, and .11.
- [System design](../../specs/tasks/system-design/task-create-recovery.md), submission identity, REST contract, and receipt ownership sections.
- `task_http_handlers_external_id_test.go`: existing native found/lookup behavior.
- `service/preallocation_error.go` and `handlers/quick_chat_creation.go`: allocation evidence precedent.
- `components/quick-chat/use-quick-chat-creation.ts`: owner fencing precedent, without its endpoint-specific resume behavior.

## Results

Completed on 2026-10-28.

- Added native `external_id`, the required `deduplicated` and `creation_complete` response metadata, and `fetchTaskByExternalId` to the browser API.
- Added `lib/task-creation-receipt.ts` for the versioned receipt, descriptor-only persistence, response decoding, and identity-checked lookup classification.
- Added `components/task-create-dialog-recovery.ts` for the in-memory guard, scoped restoration, sticky uncertainty, owner fencing, and read-only hydration.
- Added `creation_rejected` to proven pre-allocation create failures only. Post-allocation attachment and settlement failures carry no marker.
- Handler tests: `TestHTTPCreateTask_CreationRejectionEvidence`, `TestHTTPCreateTask_PreAllocationServiceFailureCarriesRejection`, `TestHTTPCreateTask_PostAllocationErrorIsUncertain`, and `TestHTTPCreateTask_ExternalIDValidationStaysUncertain` pass.
- Receipt and hook tests: `lib/task-creation-receipt.test.ts` and `components/task-create-dialog-recovery.test.tsx` pass.
- `(cd apps/backend && go test ./internal/task/handlers -count=1)`: passed.
- `bash scripts/lint-go-changed` on the changed Go files: 0 issues.
- `(cd apps/web && pnpm run typecheck)`: passed.
- Focused ESLint on the changed modules: passed.
- Remediation: native response decoding rejects a foreign workspace, external ID, or conflicting task ID without adopting that task; a valid identity-lost response still keeps its verified task. Lookup requires matching workspace and external identity. Stale branch-policy failures are allowlisted as proven pre-allocation rejections with their error code preserved.
