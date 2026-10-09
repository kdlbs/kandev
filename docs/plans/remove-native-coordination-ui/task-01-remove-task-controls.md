---
id: "01-remove-task-controls"
title: "Remove native controls and preserve layout/contracts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-004
acceptance_criteria:
  - AC-TASKS-COMPLETION-004.1
  - AC-TASKS-COMPLETION-004.2
  - AC-TASKS-COMPLETION-004.3
  - AC-TASKS-COMPLETION-004.4
system_design:
  - ../../specs/tasks/system-design/coordination-controls.md
---

# Task 01: Remove native controls and preserve layout/contracts

## Summary

Remove the native task coordination UI and restore phone geometry. Preserve the
underlying task APIs, enforcement, data, and normal task interactions.

## In scope

- Remove both rows, the phone toolbar, and their unused private component tree.
- Restore conditional phone header offset and shared-error handling.
- Remove unused locale values consistently across seven catalogs, including pseudo.
- Replace obsolete browser tests and retain domain/API recovery assertions.

## Out of scope

Other audit findings, backend behavior changes, new SDK exports, plugin implementation,
and replacement native entry points are excluded.

## Acceptance

1. Both layouts omit coordination controls for ordinary and configured tasks, without detail fetches or reserved toolbar space.
2. Phone chat, composer, errors, and navigation remain usable; desktop panels and responsive preferences remain intact.
3. Existing claims, gates, human recovery APIs, consent restrictions, and completion failures retain their behavior and regression coverage.

## ASCII UI preview

UI-01 and UI-02 from the [full preview](plan.md#ascii-ui-preview):

```text
Desktop                         Phone
+--------------------------+    +----------------------+
| Existing task header     |    | Fixed task header    |
| Existing error, if any   |    | Existing error, if any|
| Chat       | Work panels |    | Active panel         |
+--------------------------+    | Composer             |
                                | Bottom navigation    |
                                +----------------------+
```

No replacement row or blank strip. Applicable criteria: 004.1, 004.2, 004.4.
Phone scroll, safe areas, and touch behavior follow the full preview.

## Verification

Run from the repository root. In a fresh worktree, first install dependencies:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Use TDD for the layout behavior and any missing recovery regression. First prove
the new browser cases fail because the current rows still render. Then implement.
The four replaced browser files are listed below; remove them only after their
retained domain assertions have explicit coverage.

```bash
(cd apps/web && pnpm exec vitest run components/task/mobile/session-mobile-layout.test.tsx lib/api/domains/task-management-claims-api.test.ts lib/api/domains/task-completion-gates-api.test.ts lib/api/domains/kanban-api.test.ts lib/plugins/host-api.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts)
(cd apps/backend && go test ./internal/task/handlers ./internal/task/service -run 'Test(HTTP.*TaskManagementClaim|TaskManagementClaim|CompletionGate|HTTPCompletionGate)' -count=1)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-coordination-ui-removal.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-coordination-ui-removal.spec.ts tests/task/mobile-launch-failure-recovery.spec.ts tests/chat/mobile-clarification.spec.ts)
git diff --check
```

Inspect a rendered desktop and phone screenshot from those runs. Record geometry
results and the actual discovered test counts. Run the commands sequentially.
If new HTTP tests are needed, use `TestHTTPCompletionGate...` names so the command
includes them. Use scoped backend guidance before editing tests.

## Files likely touched

- `apps/web/components/task/task-page-inner.tsx`
- `apps/web/components/task/task-layout.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx` and `.test.tsx`
- `apps/web/components/task/task-management-claim-{row,surface,details}.tsx`
- `apps/web/components/task/task-completion-gate-{row,surface,details,sections,criteria}.tsx`
- `apps/web/components/task/use-task-completion-gate-actions.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,pseudo}/task.json` and affected `_verbatim.json` entries
- `apps/web/e2e/tests/plugins/task-management-claims.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-task-management-claims.spec.ts`
- `apps/web/e2e/tests/plugins/task-completion-evidence.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-task-completion-evidence.spec.ts`
- `apps/web/e2e/tests/plugins/task-completion-gate-e2e-helpers.ts` if its remaining consumers change
- New `apps/web/e2e/tests/task/task-coordination-ui-removal.spec.ts` and `mobile-task-coordination-ui-removal.spec.ts`
- Existing `apps/backend/internal/task/handlers/task_management_claims_http_test.go`
- New `apps/backend/internal/task/handlers/completion_gate_handlers_test.go` if coverage is missing

## Dependencies

None.

## Risks

Read the plan risks. Preserve API types, status DTOs, completion flags, and human
identity checks even when their native UI consumer disappears.

## Parallelism

`sequential`

## Inputs

- [Requirement amendment](../../specs/tasks/requirements/task-completion.md#req-tasks-completion-004-plugin-coordination-without-native-task-controls)
- [Design](../../specs/tasks/system-design/coordination-controls.md#ui-and-contracts)
- [Audit](pr-3994-ui-audit.md) and parent of commit `f24a785cd30` for the old offset contract
- Current mobile layout, launch-failure E2E, and completion-gate service tests

## Results

Completed on September 28, 2026. The native rows, phone toolbar, and private
component tree are removed. Task APIs, data, enforcement, workflow status, and
human authorization remain. Phone header spacing now matches the pre-#3994
contract, including the shared-error offset.

TDD evidence: the new offset unit case failed before implementation. The new
desktop and phone E2E cases first failed because the removed controls rendered.

Validation passed:

- The focused frontend Vitest command passed 108 tests across 6 files.
- The focused Go command passed in `internal/task/handlers` and `internal/task/service`.
- `pnpm run typecheck` and `pnpm run i18n:check` passed.
- The desktop Playwright spec passed 1 test. It also submits a blocked completion
  through the workflow stepper and confirms the existing task error banner.
- The mobile Playwright command passed 14 tests across the task layout,
  launch-failure recovery, and clarification specs. The configured task shows no
  controls or detail requests. The composer remains above bottom navigation,
  and the page does not overflow horizontally.
- Desktop and phone screenshots were inspected. Desktop content starts within
  8px of the task header; the phone task panel starts within 24px of its header.
- `git diff --check` passed for tracked changes. Documentation checks cover the
  untracked plan files separately.

### Review follow-up: phone feedback clearance

The review found that page-level ensure and session-recovery feedback rendered
before `TaskLayout`, so restoring padding inside the mobile panel did not keep
those earlier siblings below the fixed phone header. `TaskPageInner` now gives
the shared feedback-and-content parent one header plus safe-area offset when a
page-level error or recovery notice is visible. The ordinary phone panel and
shared-task-error-only layout retain their existing clearance ownership.

A later PR review found that the panel still added a safe-area inset after the
parent reserved it, and that the recovery card could use a newer live task
status than the offset predicate. Page-level feedback now sets panel top padding
to zero, including the inset. The task page resolves the bootstrap recovery
error from the live task-status summary once and uses that value both to show
the recovery card and to reserve header clearance.

Regression and validation:

- The final focused frontend Vitest run passed 81 tests across four files. The
  cases cover ensure failure without a task-summary error, each page-feedback
  branch, live bootstrap status, duplicated safe-area clearance, ordinary
  pages, shared errors, and desktop behavior.
- The managed mobile E2E passed six targeted tests across coordination removal
  and launch recovery. The new
  ensure-session failure case has no task-summary error, measures the banner
  and retry against the fixed header, and taps retry after recovery. The
  status-unavailable notice has the same geometry and interaction checks.
- The desktop coordination-removal E2E passed 1 test on the reviewed changes.
  Web typecheck and scoped ESLint passed.
- `pnpm run i18n:check`, documentation catalog validation, all specification
  lint, and `git diff --check` passed.
