---
id: "01-align-pr-status-precedence"
title: "Align compact PR status precedence"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
system_design:
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
  - ../../specs/ui/system-design/pr-task-status-summary.md
---

# Task 01: Align Compact PR Status Precedence

## Acceptance

- An open PR with pending CI and blocked mergeability has a `pending` compact
  aggregate and a yellow icon.
- A stronger failure signal is not hidden by blocked mergeability.
- A stronger failure, pending, or review signal is not hidden by dirty
  mergeability.
- A pending review without a passed CI signal remains yellow.
- Pending and awaiting-review PRs outrank blocked PRs when a task has multiple
  open PRs.
- Terminal, queued, draft, review, ready, and passing behavior remains
  unchanged.
- An inactive task shows the same yellow icon in the desktop sidebar, Kanban
  card, and mobile task switcher after a page reload.
- Hydrating the full PR record does not change the icon color.

## TDD sequence

1. RED: add table-driven backend cases for pending CI plus blocked
   mergeability and failure plus blocked mergeability. Run the focused test and
   record the expected failure.
2. RED: extend the existing desktop and mobile Playwright specs with the
   combined PR state. Confirm that the compact desktop state fails on gray.
3. GREEN: reorder only the compact aggregate checks needed to match the
   existing full PR attention precedence, including dirty mergeability,
   no-signal pending review, and multi-PR ranking.
4. REFACTOR: name the precedence cases clearly and keep the projector branch
   order readable.
5. GREEN: rebuild the production web and backend artifacts through the managed
   E2E runner. Confirm yellow before reload, after reload, and after hydration.

## Verification

```bash
cd apps/backend && go test -v -run TestPullRequestAggregateStatePrecedence ./internal/task/statussummary
cd apps/backend && go test ./internal/task/statussummary
cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-sidebar-hover-hydration.spec.ts -- --grep "keeps pending CI yellow"
cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-status-badge.spec.ts -- --grep "keeps pending CI yellow"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-status-summary.spec.ts -- --grep "keeps pending CI yellow"
```

## Files likely touched

- `apps/backend/internal/task/statussummary/projector_pr.go`
- `apps/backend/internal/task/statussummary/projector_pr_precedence_test.go`
- `apps/web/components/kanban-card.tsx`
- `apps/web/components/kanban-card-content.tsx`
- `apps/web/e2e/tests/pr/pr-sidebar-hover-hydration.spec.ts`
- `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-status-summary.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. The browser assertions depend on the corrected backend summary,
and all files describe one precedence rule.

## Inputs

- `docs/specs/platform/requirements/bounded-task-status-delivery.md`,
  especially acceptance criteria 001.1 and 001.2.
- `docs/specs/ui/requirements/pr-task-status-summary.md`, especially
  acceptance criteria 001.5 and 001.8.
- `docs/plans/pr-status-summary-color-parity/plan.md`.
- The full-record precedence in
  `apps/web/components/github/pr-task-icon.tsx`.
- The compact-summary projection in
  `apps/backend/internal/task/statussummary/projector_pr.go`.

## Output contract

Report the RED failures, the final precedence order, all changed files, the
focused backend and browser results, and the work-order status update. Include
the observed icon color before reload, after reload, and after full-record
hydration.

## Result

The RED projector cases returned `blocked` for pending CI, failed CI, and
pending review when mergeability was blocked. The desktop sidebar and native
mobile switcher consequently rendered the compact icon gray. The Kanban reload
case additionally showed that the card did not pass its compact task summary to
the PR icon while full PR hydration was pending.

The projector now orders failed checks, pending checks, and pending review
ahead of general blocked or dirty mergeability. Awaiting-review requires passed
checks, while a pending review without a passed CI signal remains yellow. The
multi-PR rank table follows the same ordering. Kanban cards pass their
persisted task summary through the existing PR icon fallback. The icon is
yellow before reload, after reload, and after full-record hydration on the
covered desktop and mobile surfaces. The preview deploy command also accepts
the `--skip-description` flag used by the current default-branch workflow.

Verification completed with 10 focused projector cases, all 90 status summary
package tests, 14 Kanban component tests, 2 desktop browser regressions, and 1
mobile browser regression passing. Backend and web production builds,
preview command tests, frontend typecheck, and `git diff --check` also passed
before this remediation.
