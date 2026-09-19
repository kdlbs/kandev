---
spec: docs/specs/tasks/requirements/multi-branch.md
created: 2026-08-21
status: complete
---

# Implementation Plan: Stable Review Dialog Refresh

## Overview

The Review dialog reloads because `usePRDiff` uses one timestamped `sourceKey` for both request freshness and displayed-data identity. When `last_synced_at` advances for the same PR, `resolvePRDiffView` masks the resolved files and `fetchPRFiles` writes an empty loading state, causing `ReviewPRDiffBoundary` to unmount `ReviewDiffList` and discard its scroll container. Separate logical PR identity from the timestamped request key so same-PR refreshes retain resolved content while different PRs remain isolated.

## Frontend

### PR diff request and display identity

- Update `apps/web/hooks/domains/github/use-pr-diff.ts` to derive:
  - a logical identity for the workspace, owner, repository, and PR number; and
  - a timestamped request key that still includes `refreshKey` and triggers replacement fetches.
- During a same-identity refresh, keep the last resolved files in state and keep the returned loading state non-blocking. `ReviewPRDiffBoundary` treats `loading: true` as an initial-load replacement even when retained PR-only files exist, so only an identity without a resolved diff may report blocking loading. Replace retained files atomically only when the current request succeeds.
- If a background refresh fails after files resolved, retain those files and avoid replacing the usable review with the initial-load error boundary. Preserve the existing empty/error/retry behavior when no diff has resolved.
- Keep immediate masking and request ownership for a different workspace or PR. A late response from an older request must not overwrite the current selection.
- Leave `ReviewDialog`, `ReviewDialogSurface`, and desktop/mobile composition unchanged. Keeping `ReviewDiffList` mounted preserves its scroll container and shared transient review state.

### Mobile parity

The repair is shared hook-level state normalization consumed by desktop, phone, and tablet Review mounts. It changes no entry point, layout, overlay, touch target, hierarchy, or scroll owner. Existing full-height mobile Review composition remains the closest shipped exemplar; the same retained diff contract applies without viewport-specific code.

## Tests

- **What:** A newer sync timestamp for the same PR starts a request while keeping resolved files observable.
  - **File:** `apps/web/hooks/domains/github/use-pr-diff.test.ts`
  - **How:** Keep the replacement WebSocket response deferred and assert the prior file remains while the hook does not re-enter its blocking loading state.
- **What:** A current same-PR response replaces retained files atomically, while a different PR masks prior files and ignores late responses.
  - **File:** `apps/web/hooks/domains/github/use-pr-diff.test.ts`
  - **How:** Resolve controlled requests in normal and reverse order and assert only the current logical identity is exposed.
- **What:** Background failure retains resolved files, while initial failure remains empty and exposes the error.
  - **File:** `apps/web/hooks/domains/github/use-pr-diff.test.ts`
  - **How:** Reject controlled initial and replacement requests and assert the two failure contracts separately.

## E2E Tests

No new polling-based Playwright spec is planned. The timing boundary is deterministic in the hook test, while reproducing it through the mock provider would require unrelated backend delay controls. Run the existing desktop and mobile multi-PR Review specs after the hook fix to prove rendered Review remains functional and different-PR isolation still holds:

- `apps/web/e2e/tests/review/review-multi-pr.spec.ts`
- `apps/web/e2e/tests/review/mobile-review-multi-pr.spec.ts`

## Verification Results

- RED confirmed before planning: `cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/github/use-pr-diff.test.ts` failed because the same-PR refresh returned `undefined` instead of retaining `src/first-pr.ts`.
- Expanded RED: three of eight focused tests failed while a pending or failed same-PR refresh cleared the resolved file.
- QA RED: after initial retention was implemented, one of eight tests still failed because the hook returned blocking `loading: true`; tracing `ReviewPRDiffBoundary` proved that state would still unmount a PR-only diff.
- GREEN: `cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/github/use-pr-diff.test.ts --reporter=dot` passed all 8 tests after resolved same-PR refreshes became non-blocking.
- `cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/github/use-pr-workspace-scope.test.ts --reporter=dot` passed all 6 adjacent workspace-scope tests.
- `cd apps && pnpm --filter @kandev/web lint` passed with zero warnings, and `cd apps && pnpm --filter @kandev/web typecheck` passed.
- Fresh production-build desktop E2E passed 1 test: `cd apps/web && pnpm e2e:run tests/review/review-multi-pr.spec.ts`.
- Mobile parity E2E passed both phone and coarse-pointer tablet tests: `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/review/mobile-review-multi-pr.spec.ts`.

## Implementation Waves And Parallel Candidates

Sequential:

- [x] [task-01-preserve-review-diff-refresh](task-01-preserve-review-diff-refresh.md)

No parallel candidates. Production state and regression coverage share one hook contract.

## Risks

- A display identity that is too broad can expose files from another workspace or PR; retain workspace, owner, repository, and PR number in the logical identity.
- Treating every failure as a background failure can hide an initial-load error; retain content only when that logical identity already has resolved files.
- A genuinely changed replacement diff can alter document height after it lands. This repair guarantees no pending-refresh unmount or reset, not a fixed visual position after remote content itself changes.

## Out of Scope

- Changes to Review dialog layout, file ordering, comment persistence, or reviewed-file persistence.
- Changes to synchronization cadence or the `TaskPR.last_synced_at` contract.
- New backend delay controls solely for Playwright timing injection.
