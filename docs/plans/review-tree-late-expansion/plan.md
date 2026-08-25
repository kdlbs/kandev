---
spec: docs/specs/ui/requirements/submodule-review.md
created: 2026-08-15
status: implemented
---

# Implementation Plan: Review tree late expansion

## Overview

Harden the desktop Review file tree against incremental review-source updates. The existing `useTree` state expands directories present on mount, but the Review sidebar can receive parent and nested-submodule files in separate updates. The plan adds Review-specific first-seen directory reconciliation, preserves manual collapse state, and proves the behavior with a focused component regression plus the existing nested-submodule E2E.

The fix stays within the accepted nested-submodule repository-scope decision. It does not change repository discovery, source precedence, API payloads, mobile Review composition, or locator timeouts.

## Frontend

### Review file tree

- `apps/web/components/review/review-file-tree.tsx`: track directory paths already observed by the tree. When `files` introduces new directory paths, add only those paths to the expanded set. Keep existing paths unchanged so an explicit user collapse remains authoritative.
- Reuse the existing `useTree` `setExpanded` API and the same first-seen-directory behavior already used by `apps/web/components/task/changes-panel-tree.tsx`; do not broaden the shared hook contract for this narrow Review behavior.

## Tests

- **Late nested scope:** extend `apps/web/components/review/review-file-tree.test.tsx` with a rerender that adds a nested submodule file after the parent scope is present. Verify the intermediate directory and nested `submodule-node` become visible.
- **Manual collapse preservation:** retain the existing Review tree collapse coverage and ensure the new reconciliation only expands paths not observed before, not an already-seen directory.

## E2E Tests

- **Scenario:** GIVEN Review receives parent and nested submodule diffs through separate source updates, WHEN the existing desktop nested-submodule Review flow opens and expands the parent boundary, THEN the nested boundary is visible and all three `README.md` diffs remain reviewable.
- **File:** `apps/web/e2e/tests/review/submodule-review.spec.ts` (existing regression; no selector or timeout change planned).
- **Mobile note:** the desktop `ReviewFileTree` is hidden below the existing `md` breakpoint. Phone Review uses the existing sticky diff header, so no mobile interaction changes and no new mobile test are required; the existing mobile submodule Review test remains a non-targeted smoke check.

## Verification Results

Completed 2026-08-15.

- RED unit test: `cd apps && pnpm --filter @kandev/web test -- components/review/review-file-tree.test.tsx` failed at the new late-directory assertion, with 17 passing and 1 failing test before the source change.
- GREEN unit test: the same command passed with 18 tests.
- Desktop E2E: `cd apps/web && pnpm e2e:run tests/review/submodule-review.spec.ts -- --grep "shows nested scopes and commits child gitlinks through the UI" --workers=1 --retries=0` passed 1 test.
- Desktop pressure E2E: the same scenario with `--no-build --repeat-each=4` passed 4/4 runs.
- Mobile smoke E2E: `cd apps/web && pnpm e2e:run --project mobile-chrome tests/review/mobile-submodule-review.spec.ts` passed 1 test.
- TypeScript: `cd apps && pnpm --filter @kandev/web typecheck` reported no errors.
- Web lint: `cd apps && pnpm --filter @kandev/web lint` passed with zero warnings.
- PR screenshot capture used a disposable spec and synthetic submodule fixture data; the temporary spec was removed, the manifest-to-file mapping was validated, and the PNG was compressed.

## Implementation Waves And Parallel Candidates

Sequential single-task repair:

- [x] [Task 01: Expand late Review directories](task-01-expand-late-review-directories.md)

## Open Questions

None.
