---
id: "03-present-submodule-review-hierarchy"
title: "Present submodule review hierarchy"
status: done
wave: 2
depends_on: ["02-aggregate-submodule-git-data"]
plan: "plan.md"
spec: "../../specs/ui/requirements/submodule-review.md"
---

# Task 03: Present submodule review hierarchy

## Acceptance

- Review retains mixed root and named statuses, keeps `(repository_name, path)` identities distinct, and applies identical source precedence in the Changes panel and expanded dialog.
- Desktop builds the task-root directory hierarchy with pinned, accessible submodule boundary nodes; mobile sticky headers expose the same scope without adding a second navigator or horizontal page overflow.
- Parent gitlink rows are hidden only when matching child file diffs are available, and remain visible for unavailable children.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- hooks/domains/session/use-review-sources.test.ts components/review/review-dialog.build-files.test.ts components/review/types.multi-repo.test.ts components/review/review-file-tree.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web i18n:check
```

## Files likely touched

- `apps/web/hooks/domains/session/use-review-sources.ts`
- `apps/web/hooks/domains/session/use-review-sources.test.ts`
- `apps/web/components/task/use-review-dialog.ts`
- `apps/web/components/task/use-review-dialog.test.ts`
- `apps/web/components/review/review-dialog.tsx`
- `apps/web/components/review/review-dialog.build-files.test.ts`
- `apps/web/components/review/types.ts`
- `apps/web/components/review/types.multi-repo.test.ts`
- `apps/web/components/review/review-file-tree.tsx`
- `apps/web/components/review/review-file-tree.test.tsx`
- Repository/diff group header component and focused test identified during implementation
- `apps/web/src/locales/en/common.json`
- `apps/web/src/locales/pseudo/common.json`

## Dependencies

Task 02.

## Parallelism

`parallel-safe` with Task 04 after Task 02: this task owns Review source/presentation files and locale catalogs; Task 04 owns Git operation derivation/dispatch files.

## Mobile design contract

- Desktop outcome: the existing Review tree embeds submodule boundaries in the file hierarchy and the diff list remains the single scroll owner.
- Mobile entry point: the existing task Changes/Review control opens the existing full-height Review dialog.
- Nearest shipped exemplar: `apps/web/e2e/tests/review/mobile-review-multi-pr.spec.ts` for the focused phone Review surface and sticky repository context.
- Presentation: keep direct full-height Review; do not introduce a drawer because reviewing a dense diff is primary, deep content rather than a temporary choice.
- Shared logic: source identity, filtering, selection, review hashes, and comments remain common; only the already-existing desktop tree versus mobile sticky-header presentation differs.
- Mobile proof: Task 05 verifies the visible scope label, diff access, touch path, containment, and horizontal-overflow contract.

## Inputs

- Spec **What**, mobile scenario, duplicate-path scenario, and gitlink fallback.
- `/mobile-parity` full-height dense-content guidance and existing `ReviewDialogSurface` composition.
- Existing composite review keys and multi-repository tree tests.

## Output contract

Report hierarchy semantics, accessibility/i18n keys, mobile parity, files changed, exact tests and counts, rendered-check evidence or blocker, risks, and synchronized task/plan status.

## Results

Implemented mixed root/named source normalization, nested hierarchy construction, gitlink suppression with unavailable-child fallback, composite review identities, visible/accessibility-aware submodule markers, and localized desktop/mobile scope labels. Added a dedicated repository-summary helper to keep the Changes controls within lint thresholds.

Verification:

- Focused Vitest command covering the 9 changed Review/source/order files — 118 passed.
- `pnpm run typecheck` — passed; `pnpm run lint` — passed with zero warnings; `pnpm run i18n:check` and `pnpm run i18n:ratchet` — passed.
