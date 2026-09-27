---
id: "07-task-commands"
title: "Exact task delegation and follow-up commands"
status: complete
wave: 7
depends_on: ["01-exact-host-foundation", "06-workspace-observations"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-COORDINATION-005
acceptance_criteria:
  - AC-PLUGINS-COORDINATION-005.1
  - AC-PLUGINS-COORDINATION-005.2
  - AC-PLUGINS-COORDINATION-005.3
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 07: Exact task delegation and follow-up commands

## Summary

Expose shared task commands with durable receipts, exact preconditions, and source deduplication. Preserve ordinary task workflow rules.

## In scope

- Add create/move/labels/relations/assign/archive and exact messages/directives, extending the task update from work order 01.
- Enforce source identity dedup across archived tasks and idempotent accepted messages. Validate explicit repositories, parents, profile, and executor.
- Add human-bound preview/confirm deletion through native confirmation; unrelated descendants and stale previews fail. Document public commands with concrete typed examples.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- Retried delegation creates one task and one accepted message, including after a crash.
- Moves, relations, archive, and assignment preserve shared domain preconditions and pending transitions.
- Deletion requires current host-recorded human confirmation and protects unrelated task trees.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd apps/backend && go test -race ./internal/plugins ./internal/task/service -run 'TestExactTaskCommands|TestSourceIdentityDedup' -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/managed-task-commands.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-managed-task-commands.spec.ts)
```

Evidence to create:

- `apps/backend/internal/plugins/host_exact_tasks_test.go`: `TestExactTaskCommands`.
- `apps/backend/internal/task/service/service_source_dedup_test.go`: `TestSourceIdentityDedup`.
- `apps/web/e2e/tests/plugins/managed-task-commands.spec.ts`: `delegation, retry and deletion consent`.
- `apps/web/e2e/tests/plugins/mobile-managed-task-commands.spec.ts`: `phone native deletion consent and stale preview`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/backend/internal/plugins/host_write.go`
- `apps/backend/internal/task/service/`
- `apps/backend/internal/orchestrator/messagequeue/`
- `apps/backend/internal/mcp/handlers/`
- `apps/web/components/task/`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 01](task-01-exact-host-foundation.md)
- [Task 06](task-06-workspace-observations.md)

## Risks

Task external_id is not an idempotency contract by itself. Enforce source uniqueness in the task transaction and preserve explicit source identity.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Implementation notes

If native deletion needs a new rendered control, add the affected confirmation excerpt to UI-03 and mobile E2E in this work order before implementation. Reuse the existing native preview/confirm flow where it meets the contract.

## Results

Complete. Implemented exact task create/update/move/label/relation/assignment/archive commands, durable accepted messages and directives, and source identity deduplication across active and archived tasks. Native deletion now requires a current, single-use preview confirmation bound to the Human, task roots, cascade/discard choices, and the affected task snapshot; the legacy plugin delete is denied. Updated Host/SDK/public documentation and task cleanup callers. Validation passed: plugin/task-service/handler/backend adapter race tests; 41 focused web tests, typecheck, scoped ESLint, desktop and phone deletion-consent E2E, docs/spec validation, and `git diff --check`. The phone flow verifies a settled 44-pixel action target, stale-preview rejection, and touch confirmation.

### Review remediation (2026-09-27)

Exact task updates now treat Host installation identity as authenticated attribution,
not as an ownership claim. The real Host-to-service-to-SQLite regression passed for
never-claimed and released tasks, and still rejects a competing active owner. The
Host regression suite passed with the race detector.
