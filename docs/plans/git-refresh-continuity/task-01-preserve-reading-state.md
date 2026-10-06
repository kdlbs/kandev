---
id: "01-preserve-reading-state"
title: "Preserve counts, diff content, and reading position"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.10
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.11
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.24
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.25
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.29
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.47
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.48
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.49
system_design:
  - ../../specs/platform/system-design/git-refresh-continuity.md
---

# Task 01: Preserve Counts, Diff Content, and Reading Position

## Summary

Preserve eligible previous displayed counts and patches while current details refresh.
Keep the viewer mounted for identical content and restore the visible anchor after changed content.
Deliver the shared desktop/phone behavior with store, renderer, and controlled browser regressions.

## In scope

- Runtime display companion, accepted-snapshot reconciliation, current readiness, and scope cleanup.
- Counts, mixed-layer projections, Review sources/progress, populated viewer continuity, and freshness status.
- Patch-dependent action guards, stable data/key identity, and external/provider scroll restoration.
- All tests listed in the plan, including both renderers and phone composition.

## Out of scope

Backend publication, polling/retry redesign, new dependencies, persistent caching, and unrelated editor behavior.

## Acceptance

1. Pending or failed refresh preserves eligible counts and readable diffs. Raw pending snapshots remain unenriched; initial placeholders and confirmed removal remain correct.
2. Identical completion preserves the mounted viewer and reading/view state. Changed completion follows the design's anchor/fallback policy without overriding later user scroll.
3. Desktop and phone pass the plan's controlled-enrichment matrix. Retained patches cannot submit hunk mutations or new line-based review actions.

## ASCII UI preview

UI-01 and UI-02 excerpts from the [full preview](plan.md#ascii-ui-preview), covering `.47` through `.49`:

```text
DESKTOP
| Diff: a.ts (refresh)       | Changes (spinner) |
| 40 readable line          | a.ts +18 -3 [M]   |
| <same reading anchor>     |                   |

PHONE: EXISTING FULL-HEIGHT DRAWER
| File changes       Close |
| a.ts (refresh)           |
| 40 readable line         |
| <same internal scroller> |
```

Keep fixed headers, current scroll owners, and mobile dismissal/focus behavior.
Initial loads keep the existing placeholder; failed refresh keeps readable previous content with unavailable freshness.

## Verification

Run from the repository root. New test paths below are outputs of this work order.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/state/slices/session-runtime/git-status-display-state.test.ts lib/state/slices/session-runtime/git-status-state.test.ts lib/state/slices/session-runtime/git-status-multi-repo.test.ts lib/ws/handlers/git-status.test.ts hooks/domains/session/git-change-facets.test.ts hooks/domains/session/use-session-git-derived.test.ts hooks/domains/session/use-review-sources.test.ts hooks/domains/session/use-review-sources-facets.test.ts hooks/domains/session/use-review-sources-scoped.test.ts hooks/domains/session/use-session-git-status.test.tsx components/task/changes-panel-file-row.test.tsx components/task/changes-panel-helpers.test.ts components/task/task-changes-panel-layers.test.ts components/review/review-dialog-handlers.test.ts components/review/review-diff-header.test.tsx components/review/review-file-tree.test.tsx components/review/review-file-diff-content.test.tsx components/review/review-diff-list-auto-mark.test.tsx components/review/use-review-scroll-anchor.test.ts components/editors/monaco/monaco-diff-viewer.test.tsx components/diff/diff-viewer-comment-edit.test.tsx components/diff/diff-viewer-resolver.test.tsx components/editors/monaco/use-diff-viewer-comments.test.tsx components/editors/monaco/use-diff-view-zones.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/diff/comment-form.tsx components/diff/diff-viewer-comment-edit.test.tsx components/diff/diff-viewer-resolver.test.tsx components/diff/diff-viewer-resolver.tsx components/diff/diff-viewer.tsx components/diff/use-diff-annotation-renderer.tsx components/diff/use-diff-viewer-state.ts components/editors/monaco/monaco-diff-viewer.tsx components/editors/monaco/monaco-diff-viewer.test.tsx components/editors/monaco/use-diff-viewer-comments.ts components/editors/monaco/use-diff-viewer-comments.test.tsx components/editors/monaco/use-diff-view-zones.ts components/editors/monaco/use-diff-view-zones.test.tsx components/review/line-content-anchor.ts components/review/review-dialog-handlers.test.ts components/review/review-dialog-handlers.ts components/review/review-dialog-surface.tsx components/review/review-diff-header.test.tsx components/review/review-diff-header.tsx components/review/review-diff-list.tsx components/review/review-diff-row-context.ts components/review/review-diff-state-placeholder.tsx components/review/review-file-diff-content.test.tsx components/review/review-file-diff-content.tsx components/review/review-file-tree.test.tsx components/review/review-file-tree.tsx components/review/types.ts components/review/use-review-diff-scroll-interaction.ts components/review/use-review-scroll-anchor.test.ts components/review/use-review-scroll-anchor.ts components/task/changes-panel-body-props.ts components/task/changes-panel-data.tsx components/task/changes-panel-file-row.tsx components/task/changes-panel-helpers.test.ts components/task/changes-panel-helpers.ts hooks/domains/session/git-change-facets.test.ts hooks/domains/session/git-change-facets.ts hooks/domains/session/use-review-sources.ts hooks/domains/session/use-session-git-status.test.tsx hooks/domains/session/use-session-git-status.ts hooks/domains/session/use-session-git.ts lib/state/app-state-types.ts lib/state/default-state.ts lib/state/slices/session-runtime/git-status-display-state.test.ts lib/state/slices/session-runtime/git-status-display-state.ts lib/state/slices/session-runtime/git-status-state.ts lib/state/slices/session-runtime/session-runtime-git-checkout-actions.ts lib/state/slices/session-runtime/session-runtime-slice.ts lib/state/slices/session-runtime/git-status-multi-repo.test.ts lib/state/slices/session-runtime/types.ts e2e/tests/git/diff-refresh-continuity.spec.ts e2e/tests/git/git-refresh-continuity-helpers.ts e2e/tests/git/git-status-refresh-helpers.ts e2e/tests/git/mobile-diff-refresh-continuity.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && CAPTURE_PR_ASSETS=true pnpm e2e:run --host --shards 1 --project chromium tests/git/diff-refresh-continuity.spec.ts)
(cd apps/web && CAPTURE_PR_ASSETS=true pnpm e2e:run --host --shards 1 --project mobile-chrome tests/git/mobile-diff-refresh-continuity.spec.ts)
(cd apps && pnpm --filter @kandev/web build:vite)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E runner rebuilds and owns disposable instances. Run browser commands sequentially.
If extraction adds a source or test outside this list, update this verification block before marking completion.
Record rendered desktop/phone captures from the targeted E2E run.

## Files likely touched

- `apps/web/lib/state/slices/session-runtime/git-status-display-state.ts` and its new tests.
- `apps/web/lib/state/slices/session-runtime/{git-status-state,session-runtime-slice,types}.ts` and existing tests.
- `apps/web/hooks/domains/session/{use-session-git-derived,use-review-sources,git-change-facets}.ts` and related tests.
- `apps/web/components/task/{changes-panel-file-row,changes-panel-touch-file-row}.tsx` and `changes-panel-helpers.ts`.
- `apps/web/components/task/task-changes-panel-state.ts` and layer tests if needed for separate display/review state.
- `apps/web/components/review/types.ts`, `review-file-diff-content.tsx`, and `review-diff-list.tsx`.
- New `apps/web/components/review/use-review-scroll-anchor.ts` and its tests.
- New `apps/web/components/review/line-content-anchor.ts` shared by Pierre and Monaco.
- `apps/web/components/editors/monaco/monaco-diff-viewer.tsx`, comment hooks/view zones, and provider tests.
- `apps/web/lib/state/slices/session-runtime/session-runtime-git-checkout-actions.ts` and checkout invalidation tests.
- `apps/web/components/diff/comment-form.tsx`, resolver, annotation renderer, and Pierre comment tests.
- `apps/web/components/diff/{file-diff-viewer,diff-viewer-resolver,diff-viewer}.tsx` if readiness props need propagation.
- New desktop/mobile E2E specs and focused helpers beside `git-status-refresh-helpers.ts`.
- Locale catalogs only if existing status keys cannot express the needed freshness.

## Dependencies

None. Read the existing progressive-refresh and compact-toolbar contracts; preserve their current behavior outside this scope.

## Risks

See the plan. Retention eligibility, stale patch actions, renderer layout timing, and user-scroll cancellation require explicit tests.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/workspace-git-status.md), especially `.21`, `.25`, `.27`, and `.47` through `.49`.
- [Design](../../specs/platform/system-design/git-refresh-continuity.md) and its linked decision.
- Existing raw-store, layer-selection, auto-mark, and enrichment-gate tests.
- `apps/web/AGENTS.md`, `/tdd`, `/mobile-parity`, and `/e2e` guidance.

## Results

Follow-up review fixes preserve compact staged/unstaged cache identity, prevent stale retained patches from authorizing new line comments in both providers, and map changed-content anchors by surviving line content and same-side context. Existing drafts remain available but cannot submit until the current detail is ready. Monaco maps both its viewport and selection; desktop and phone scenarios shift the anchor with inserted and deleted lines. The original accepted-snapshot boundary and raw pending state remain intact.

The focused Vitest suite passed 21 files / 175 tests; the final Monaco view-state regression rerun passed 4 tests. Both the desktop Chromium and mobile Chrome continuity suites passed 2 tests with capture enabled, and all 16 renderer/surface captures are listed in `apps/web/.pr-assets/manifest.json`. TypeScript typecheck, the production Vite build, i18n checks, documentation validation, full specification lint, and `git diff --check` passed. Strict touched-file ESLint passed with zero warnings.
The PR fixup regression suite passed 5 files / 55 tests, covering checkout-scope invalidation, pending and unavailable review controls on desktop and phone layouts, guarded review-state submission, and scroll-cancellation suppression restoration.
After the fixup, web typecheck, the production Vite build, strict touched-file ESLint, documentation catalog validation, and `git diff --check` passed again.
