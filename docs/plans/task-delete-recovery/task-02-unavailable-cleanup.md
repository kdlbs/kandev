---
id: TASK-DELETE-RECOVERY-02
title: Delete tasks when workspace inspection fails
status: done
wave: 2
depends_on:
  - TASK-DELETE-RECOVERY-01
plan: plan.md
requirements:
  - REQ-TASKS-DELETE-RECOVERY-001
  - REQ-TASKS-DELETE-RECOVERY-002
acceptance_criteria:
  - AC-TASKS-DELETE-RECOVERY-001.1
  - AC-TASKS-DELETE-RECOVERY-001.2
  - AC-TASKS-DELETE-RECOVERY-002.1
  - AC-TASKS-DELETE-RECOVERY-002.2
  - AC-TASKS-DELETE-RECOVERY-002.3
  - AC-TASKS-DELETE-RECOVERY-002.4
  - AC-TASKS-DELETE-RECOVERY-002.5
  - AC-TASKS-DELETE-RECOVERY-002.6
  - AC-TASKS-DELETE-RECOVERY-002.7
  - AC-TASKS-DELETE-RECOVERY-002.8
  - AC-TASKS-DELETE-RECOVERY-002.9
system_design:
  - ../../specs/tasks/system-design/task-delete-recovery.md
---

# Delete tasks when workspace inspection fails

## Summary

Allow explicitly acknowledged normal deletion to commit despite unavailable
physical inspection. Preserve all cleanup obligations and attempt eligible
cleanup while protecting blocked resources. Keep the retain option intact.

## In scope

Typed complete/unknown inspection, explicit consent and ticket binding,
physical preparation results, durable per-resource progress, outstanding
cleanup protection, HTTP/MCP committed outcomes, shared desktop/phone dialog,
all callback adapters, localization, and public guidance.

## Out of scope

Live task deletion, incident backup mutation, automatic Git/marker repair,
new cleanup queues or operator screens, archive behavior changes, and force
removal of resources whose ownership cannot be proved.

## Acceptance

1. Normal deletion with explicit consent removes task records after durable
   inventory capture even when physical inspection fails. Database, identity,
   admission, and confirmation errors remain hard failures before commit.
2. Independent verified resources are cleaned. Blocked resources remain in the
   durable operation and protected across restart. No cleanup bypass can erase
   retained or borrowed resources. Computed outcomes distinguish cleanup and stop.
3. Desktop, phone, HTTP, and MCP expose the same contract. Retain and healthy
   clean/dirty deletion regressions pass.

## Implementation order

1. Add failing service and real-worktree regressions for the incident. Introduce
   the typed inspection result and both consent flags through the canonical
   options and ticket. Keep each boundary typed; do not match raw error text.
2. Make acknowledged physical capture failures per-resource. Save complete
   database inventory and earlier obligations before task deletion. Preserve
   preparation rollback and cascade partial-commit handling.
3. Persist independent worker progress. Add outstanding-cleanup inventory and
   exact owning-job admission to every destructive consumer. Cover locking,
   restart, retry, resource replacement, shared references, and retain priority.
4. Propagate committed outcomes through HTTP/MCP and all UI callback chains.
   Implement UI-04/UI-05, localized text, E2E coverage, and public guidance.

## ASCII UI preview

UI-04, desktop excerpt from the [combined preview](plan.md#ui-04-normal-deletion-unknown-inspection-desktop):

```text
Workspace changes could not be checked. [Retry]
[ ] Keep workspace files
[ ] Delete even if workspace changes cannot be checked
    Local changes may be lost. Files that cannot be
    safely removed will remain pending cleanup.
                                [Cancel] [Delete]
```

UI-05, phone excerpt:

```text
[ ] Keep workspace files
[ ] Delete even if workspace
    changes cannot be checked
    Local changes may be lost.
    Files that cannot be safely
    removed will remain pending
    cleanup.
[            Delete            ]
[            Cancel            ]
```

AC-TASKS-DELETE-RECOVERY-002.7-.9 govern this flow. Keep the existing inset
centered AlertDialog with contained body scrolling and fixed safe-area footer.
Delete remains disabled until acknowledgement and the fresh ticket resolve.
Phone hit areas are at least 44px. Desktop actions remain 28px. Loading and
transport/database errors cannot authorize the action. Retain clears removal
consent. No hover-only controls, new modal, or custom animation.

## Regression matrix

The regression matrix maps the implementation to its checks. Use prefix `TestDeleteUnavailable`
for new Go tests so the focused command selects every added regression.

| Criteria | Test and file | Required evidence |
| --- | --- | --- |
| .1, .2, .7 | `TestDeleteUnavailablePreflightConsent` in `task/service/task_delete_unavailable_test.go` | Unknown differs from clean. No acknowledgement blocks deletion. Ticket mismatch, expiry, replay, changed actor/scope/mode, and stale response fail safely. |
| .2, .3 | `TestDeleteUnavailablePreparation` in new `task/service/task_delete_unavailable_test.go` | Empty marker, empty `.git`, broken ref, inaccessible path and provider outage admit only with explicit consent. Persistence/inventory failures preserve uncommitted rows. Cancellation is not success. |
| .3, .5, .6 | `TestDeleteUnavailableMixedCleanupRestart` in that new file | Mixed healthy/dirty/corrupt selection, cascade and prior unfinished snapshots retain exact obligations. Eligible siblings complete. Reopened database resumes only unfinished effects. |
| .2, .5 | `TestDeleteUnavailableDamagedCheckoutPath` and `TestDeleteUnavailableIndependentResources` in `task/service/task_delete_unavailable_resources_test.go` | File and symlink checkout paths permit acknowledged task deletion but remain untouched. Attachments, prior containers, and SSH directories progress independently. Failed stops, incomplete inventory, stale claims, and a failed manifest save after inventory capture preserve their cleanup barriers. |
| .4, .6 | `TestDeleteUnavailableWorktreeIdentity` in new `worktree/manager_cleanup_unavailable_test.go` | Real temporary Git paths, no-follow guards, replaced path/ref, foreign marker, shared references, and unknown pre-deletion identity never authorize removal. Restoring original verified evidence allows retry. |
| .4, .6, .9 | `TestDeleteUnavailableInventoryProtection` in `backendapp/storage_inventory_test.go`, `task/retention/inventory_test.go`, and lifecycle retention tests | Maintenance, quarantine, container sweeps, stale workers, restart and terminal failure cannot bypass outstanding protection. Owning job can progress only after audits. Retain always wins. |
| .8 | `TestHTTPUnverifiedDeletePreservesCommittedResult`, `TestHTTPDeleteUnavailablePreflight`, and `TestDeleteUnavailableMCPOutcome` in handler recovery tests | Committed task removal with pending cleanup is success. Completed/no-target shutdown stays false after post-commit errors. Partial outcomes remain exact. |
| .7, .9 | Existing dialog, hook, API, action and selection unit tests | Unknown acknowledgement replaces ordinary discard option. Flag reaches all wrappers. Mode/scope/reopen resets consent. Older responses and generic 503s cannot enable deletion. |
| .8 | `tasks-page-client.delete.test.tsx`, `tasks-page-delete-cleanup.spec.ts`, and `mobile-tasks-page-delete-cleanup.spec.ts` | The Tasks page reports pending cleanup after committed deletion on desktop and phone. Real damaged files remain protected. |

Every resource read must succeed before inventory is called complete. At least
one independently eligible resource must progress in a mixed test; all-resource
success is required before reporting cleanup complete. Test missing physical
dependencies as configuration errors, not synthetic successful inspections.

## E2E tests

Extend `apps/web/e2e/helpers/task-delete-recovery.ts` without removing retain
helpers. Add normal-delete unknown-consent scenarios to:

- `e2e/tests/kanban/card-menu-delete-archive.spec.ts` (`chromium`).
- `e2e/tests/task/mobile-delete-discard-consent.spec.ts` (`mobile-chrome`).

Use a real empty marker in the disposable worktree. Keep-files stays unchecked.
Confirm through the new acknowledgement, assert successful committed response,
pending cleanup, task removal from the view, and preserved blocked directory.
Keep the existing retain E2E tests, changing their old expectation of a generic
503 to the new unknown state. Test long localized copy, keyboard focus, touch
hit targets and actual clickability at the minimum supported phone width. Do
not substitute a mocked transport 503 for the damaged-workspace scenario.

## Verification

Run from the repository root. Install `apps` dependencies only if absent in a
fresh worktree. Run Go jobs sequentially with the bounded concurrency below.

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 2 ./internal/task/retention ./internal/task/service ./internal/task/handlers ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp ./internal/worktree ./internal/agent/runtime/lifecycle -run 'Test(DeleteUnavailable|TaskDelete|Retain|Retained|OlderCleanup|ExplicitRetention|HTTPDeleteTask|HTTPTaskDeletePreflight|HandleDeleteTask|DeleteTask|PrepareTaskResourceCleanup|CleanupWorktrees|MutationLease|NestedMutation)' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/task/retention ./internal/task/service -run 'Test(DeleteUnavailable|MutationLease|NestedMutation|CleanupShutdownDoesNotHold|CleanupRechecksRetention)' -count=1)
(cd apps/web && pnpm exec vitest run hooks/use-task-delete-preflight.test.ts lib/api/domains/kanban-api.test.ts components/task/task-delete-confirm-dialog.test.tsx hooks/use-task-actions.test.ts hooks/use-task-crud.test.ts components/kanban/task-multi-select-toolbar.test.tsx components/task/task-session-sidebar-selection.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium -- e2e/tests/kanban/card-menu-delete-archive.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome -- e2e/tests/task/mobile-delete-discard-consent.spec.ts e2e/tests/task/mobile-delete-task-redirect.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The first E2E command rebuilds backend and frontend. Run changed-file Go and
web lint through the existing commit hooks and scoped lint commands. Do not
run full Go or E2E suites concurrently. Refresh affected PR screenshots after
implementation. Continue the authorized CI and review work on PR #4578, including
its outstanding retention findings. The user authorized a separate follow-up task for walkthrough authentication.

## Files likely touched

Backend paths are relative to `apps/backend/internal/`:

- `task/service/task_delete_preflight.go`, `task_delete_confirmation.go`,
  `task_delete_options.go`, `task_delete_result.go`, `service_tasks.go`,
  `handoff_cascade.go`, `handoff_service.go`, and `resource_cleanup_jobs.go`.
- `task/service/resource_cleanup_retained.go`, task cleanup models/repository
  progress writers, `task/retention/inventory.go`, and its SQLite query adapters.
- `worktree/manager_cleanup_dirty.go`, `manager_cleanup_audit.go`,
  `manager_cleanup.go`, and physical capture helpers.
- `task/handlers/task_delete_preflight.go`, `task_http_handlers.go`,
  `mcp/handlers/config_task_handlers.go`, and `mcp/server/config_handlers.go`.
- `backendapp/storage_inventory.go`, worktree retention guards, runtime lifecycle
  cleanup guards, and the matching tests named above.

Web paths are relative to `apps/web/`:

- `components/task/task-delete-confirm-dialog.tsx`, `task-cleanup-summary.ts`,
  `hooks/use-task-delete-preflight.ts`, and `lib/api/domains/kanban-api.ts`.
- All existing delete callbacks in task/sidebar, board, list, bulk toolbar,
  `use-task-actions.ts`, and `use-task-crud.ts`, plus the tests above.
- `src/locales/*/task.json`, E2E helper and specs listed above.

Public guidance: `docs/public/tasks-and-workflows.md`,
`docs/public/automation-and-mcp.md`, and `docs/public/git-operations.md`.

## Dependencies

Task 01's keep-files implementation is present. Reconcile the existing PR
retention review findings with the new outstanding-cleanup projection rather
than implementing a second inventory reader. Task 02 does not authorize
native delegation or persistent Kandev tasks/sessions.

## Risks

No surviving physical identity can mean cleanup needs intervention after the
task is deleted. State this limit accurately. Old snapshots and readers must
never interpret incomplete new envelopes as permission to delete. A stale
worker must not bypass a newer retain decision or delete a replacement path.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-delete-recovery.md).
- [Design](../../specs/tasks/system-design/task-delete-recovery.md), normal-delete section.
- [Decision](../../decisions/2026-10-19-separate-task-deletion-from-cleanup.md).
- Existing Task 01 tests and real-worktree reproduction recorded in the plan.

## Results

Implementation and local verification are complete. Normal deletion persists cleanup intent after
explicit acknowledgement of unknown workspace changes. Keep-files remains a
separate mode. Cleanup records per-worktree progress and retains blocked inventory
through restart. The indexed protection projection excludes ordinary cleanup
history and scopes malformed inventory to its task.

Current evidence:

- Service and HTTP recovery checks pass, including real database reopen,
  consent binding, incomplete configuration, bounded retries, and safe logs.
- All 79 focused web tests pass. Type checking and translation checks pass.
- Translation, public documentation, and specification checks pass.
- The original CI runtime cancellation regression passes with the lifecycle fix.
- Focused real-worktree, SQLite projection/migration, and MCP deletion tests pass.
  Service and retention race checks pass. Earlier interrupted broad runs are not
  treated as passes. Seven desktop and six phone E2E tests pass, including
  keyboard consent, touch targets at 320px, and blocked-file preservation after restart.
- Scoped Go lint passes with zero issues using a 12-minute timeout. The initial
  five-minute attempt timed out and is not a pass. Web lint passes.
- Browser runs use the task-owned Nix library paths and remove inherited Kandev
  launcher configuration. Initial host-configuration and missing-library failures
  occurred before application interaction.
- The current-base merge check passes after placing the new migration separately
  from the base branch's executor-ownership migration. Exact-head remote checks
  and final merge-result validation remain delivery gates.
- HTTP, service, hook, and API tests provided the failing behavioral checks.
  Browser scenarios require the new wire contract and are final integration checks.

Design validation on 2026-10-19:

- `python3 scripts/list-docs.py validate`: passed, 386 decisions and 1618 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: passed for this work order,
  its plan, both requirement IDs, and the referenced design. This was a local
  coverage preflight using the planned runtime path, not a published CI result.
- `git diff --check`: passed. This design-only result predates implementation.

PR #4578 passed 89 checks before the latest Greptile review. That review found
two remaining gaps: file/symlink checkout errors blocked task deletion, and a
blocked worktree prevented independent resource cleanup. The remediation
classifies physical path errors and separates resource failures from errors
that must stop the cleanup operation. It preserves the existing ownership,
runtime-stop, inventory, and progress-persistence checks.

Remediation verification on 2026-10-20:

- The new file/symlink deletion and independent-resource tests failed before
  the fix, then passed after it. The failed-stop fixture explicitly returns
  a stop error. Incomplete-inventory and stale-claim cases preserve resources.
- `cd apps/backend && GOMAXPROCS=2 go test -p 2 -parallel 2 ./internal/task/service ./internal/worktree -run 'Test(DeleteUnavailable|CaptureCleanupHeadOIDs|TaskLifecycleCleanup|Cleanup|ExecuteTaskResourceCleanupJob|PerformTaskCleanup|Retain|TaskResourceCleanup)' -count=1`: passed.
- Documentation catalog validation and full specification lint: passed.
- Merged `main` at `e29a5b36448892d12b9eac9cb75d837007fb55f4`. Resolved
  the helper extraction by applying error classification in
  `manager_cleanup_identity.go`. Repeated the focused Go command with
  `-trimpath` on the combined tree: passed. Harness tests and lint passed.

A subsequent review identified a failed manifest save that still permitted
independent cleanup, and a missing cleanup-pending toast on the Tasks page.
Manifest-save failures now use the fatal result, including a lost worker claim.
The Tasks page uses the same localized notice as the shared task actions.
Both cases have failing-before-fix regression evidence. The service
`TestDeleteUnavailable` tests and 20 focused frontend tests pass.

Additional verification on 2026-10-20:

- `cd apps/backend && GOMAXPROCS=2 go test -trimpath -p 2 -parallel 2 ./internal/task/service -run '^TestDeleteUnavailable' -count=1`: passed.
- `cd apps/web && pnpm exec vitest run app/tasks/tasks-page-client.delete.test.tsx app/tasks/tasks-page-client.facet-selection.test.tsx hooks/use-task-actions.test.ts`: 20 passed.
- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps/web && pnpm e2e:run --host --shards 1 --project chromium e2e/tests/task/tasks-page-delete-cleanup.spec.ts`: one passed after reproducing the missing notice.
- The matching `mobile-chrome` run of `mobile-tasks-page-delete-cleanup.spec.ts`: one passed at 320px. Both browser runs assert the visible notice, removed task row, preserved damaged files, and no horizontal document overflow.

Fresh CI and review evidence for this remediation remain delivery gates.
