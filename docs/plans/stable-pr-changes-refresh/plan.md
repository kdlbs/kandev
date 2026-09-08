---
spec: docs/specs/tasks/requirements/multi-branch.md
created: 2026-08-12
status: complete
---

# Implementation Plan: Stable PR Changes Refresh

## Overview

The PR Changes row flickers because `useActiveTaskPRsWithFiles` includes `last_synced_at` in its request key and prunes the prior key before the replacement request resolves. Preserve the last resolved files for the same logical workspace/task/PR while refreshing, then replace them atomically when the current response lands. Scope changes and removed PRs must continue to clear stale data.

## Frontend

### PR file refresh cache

- Update `apps/web/hooks/domains/github/use-active-task-pr-files.ts` so request invalidation and display identity are separate: `last_synced_at` still triggers one new request, while the last resolved files remain readable under the current PR until that request settles.
- Preserve the existing workspace response guard and garbage collection for removed PRs. Do not retain files across workspace changes, task changes, PR removal, or a different logical PR.
- Keep `apps/web/components/task/changes-panel-body.tsx` and its desktop/mobile composition unchanged. Stabilizing hook output prevents `hasPRFiles` from oscillating, so the existing `PRFilesSection` stays mounted and preserves user collapse state.

## Tests

- **What:** After an initial PR file response resolves, advancing `last_synced_at` starts one refresh and leaves the prior files observable until the replacement resolves.
- **File:** `apps/web/hooks/domains/github/use-pr-workspace-scope.test.ts`
- **How:** Render the hook with a deferred second WebSocket response; assert the old file remains during the pending interval and is atomically replaced afterward.
- **What:** Workspace/task/PR scope changes and PR removal never reuse retained files.
- **File:** `apps/web/hooks/domains/github/use-pr-workspace-scope.test.ts`
- **How:** Extend the existing deferred-response coverage with same-workspace task/PR removal cases as needed by the chosen cache shape.
- **What:** A failed background refresh retains resolved files, while an initial failure remains empty.
- **File:** `apps/web/hooks/domains/github/use-pr-workspace-scope.test.ts`
- **How:** Reject a controlled replacement request after the initial response resolves and assert the stable list remains observable.

## E2E Tests

No new Playwright scenario is planned. This repair changes only hook-level cache normalization and deliberately leaves layout, touch behavior, scrolling, navigation, and viewport composition unchanged. The existing `apps/web/e2e/tests/task/mobile-changes-panel.spec.ts` covers the mobile PR Changes surface; the deferred refresh race is deterministic and more reliably proven at hook level than through the polling fixture.

## Verification Results

- `cd apps && pnpm install --frozen-lockfile` passed and installed the fresh-worktree dependencies.
- RED: the deferred timestamp-refresh test failed because `filesByPRKey` became empty before the replacement response resolved.
- RED: the controlled refresh-rejection test failed because the settled error replaced retained files with an empty list.
- `cd apps && pnpm --filter @kandev/web test hooks/domains/github/use-pr-workspace-scope.test.ts && pnpm --filter @kandev/web lint && pnpm --filter @kandev/web typecheck` passed: 5 tests, zero lint warnings, and clean TypeScript compilation.
- Review remediation: a reverse-order T2/T3 regression failed before the desired-key guard because late T2 replaced T3, then the same test passed after superseded success/failure handlers were rejected. The focused suite now passes 6 tests with lint and typecheck clean.

## Implementation Waves And Parallel Candidates

Sequential:

- [x] [task-01-stabilize-pr-file-refresh](task-01-stabilize-pr-file-refresh.md)

No parallel candidates. The implementation and regression test exercise one hook contract.

## Risks

- Retaining by an identity that is too broad can leak files across task, workspace, repository, or PR switches. The regression test must cover those invalidation boundaries.
- Retaining by the timestamped request key recreates the flicker. Request freshness and displayed-data identity must be modeled separately.
