---
id: "01-shared-header"
title: "Extract shared collapsible file header"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMMIT-FILE-NAV-001
acceptance_criteria:
  - AC-UI-COMMIT-FILE-NAV-001.1
  - AC-UI-COMMIT-FILE-NAV-001.7
  - AC-UI-COMMIT-FILE-NAV-001.8
system_design:
  - ../../specs/ui/system-design/commit-file-navigation.md
---

# Task 01: Extract shared collapsible file header

## Summary

Extract the review header's responsive file identity and collapse control into a caller-neutral component.
Keep review-specific state and actions in the existing adapter. This work order does not yet change commit behavior.

## In scope

- Shared header with controlled collapse, path/stats metadata, and leading/status/action slots.
- Existing review adapter integration, including test attributes and sticky offsets.
- TDD for the shared presentation contract and focused review compatibility coverage.

## Out of scope

- Commit index, toolbar changes, new review features, and review state ownership changes.

## Acceptance

- The shared header renders with no review slots and exposes an accessible controlled toggle.
- Review retains its current checkbox, stale metadata, stats, actions, selection, and mobile composition.
- Existing focused review tests pass against the extraction.

## ASCII UI preview

UI-01 and UI-02 header excerpts from the [combined preview](plan.md#ascii-ui-preview):

```text
Desktop: [caller slot] v src/a.ts +12 -3 [caller status] [caller tools]
Phone:   [caller slot] v a.ts                          [...]
                        src/                  +12 -3
```

The commit caller omits the leading review checkbox and stale slot. The review caller retains both.
Preserve mobile filename-first hierarchy and touch access. These map to AC-UI-COMMIT-FILE-NAV-001.1, .7, and .8.

## Verification

Run from the repository root. Install dependencies once if this worktree has no installation.
Run the new shared-header test in RED before adding its implementation. Record each result.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/diff/collapsible-file-header.test.tsx components/review/review-diff-header.test.tsx)
(cd apps/web && pnpm exec eslint components/diff/collapsible-file-header.tsx components/review/review-diff-header.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/review/review-file-status.spec.ts tests/review/review-markdown-preview.spec.ts tests/review/external-vcs-file-link.spec.ts tests/review/submodule-review.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/review/mobile-review-file-status.spec.ts tests/review/mobile-review-markdown-preview.spec.ts tests/review/mobile-submodule-review.spec.ts)
git diff --check
```

## Files likely touched

- `apps/web/components/diff/collapsible-file-header.tsx` (new)
- `apps/web/components/diff/collapsible-file-header.test.tsx` (new)
- `apps/web/components/review/review-diff-header.tsx`
- `apps/web/components/review/review-diff-header.test.tsx`

## Dependencies

None.

## Risks

Avoid moving review observers or state into the shared header. Preserve review selectors and caller-specific stat visibility.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/commit-file-navigation.md)
- [Design: shared header](../../specs/ui/system-design/commit-file-navigation.md#shared-header-and-caller-adapters)
- `apps/web/components/review/review-diff-list.tsx` and `review-diff-toolbar.tsx`
- `apps/web/AGENTS.md`, `/tdd`, `/mobile-parity`, and `/e2e`

## Results

Completed on 2026-09-17.

- Added the caller-neutral `CollapsibleFileHeader` with responsive path identity, controlled collapse state, status/stats slots, action slots, and accessible controls.
- Refactored `ReviewDiffHeader` to use the shared presentation while retaining review-specific state, selection, stale metadata, and actions in its adapter.
- Added focused shared-header tests and preserved the existing review test suite.
- The focused component suite passed 10 tests. Desktop review extraction guards passed 6 E2E tests, and mobile review extraction guards passed 3 E2E tests. Typecheck, touched-file ESLint, and `git diff --check` passed.

Review remediation on 2026-09-17 also verified that the shared header keeps filename-first mobile hierarchy and applies 28px fine-pointer desktop sizing versus at least 44px phone/coarse-pointer sizing. The coarse-pointer file-header regression passed with the review extraction guards.
