---
spec: docs/specs/ui/review-file-status.md
created: 2026-07-14
status: building
---

# Implementation Plan: Review File Status Cues

## Overview

First extract the Changes panel's status mapping and marker into shared frontend primitives. Then preserve status-only files through Review's source builders and render the marker on desktop and mobile with honest patchless states. Finish with focused production-build Playwright coverage. No backend or API changes are required: `FileInfo` and `PRDiffFile` already carry status and previous-path metadata.

## Frontend

### Shared status model and marker

- Add `normalizeFileChangeStatus` and the canonical `FileChangeStatus`/label mapping in `apps/web/lib/utils/file-change-status.ts`; map GitHub `removed` to `deleted`, preserve `renamed`, and fall back to `modified`.
- Move `FileStatusIcon` from `apps/web/components/task/file-status-icon.tsx` to `apps/web/components/shared/file-status-icon.tsx`. Preserve the Changes palette, use a distinct arrow for `renamed`, and expose `aria-label`, `title`, `data-file-status`, `oldPath`, and shrink-safe styling.
- Update `changes-panel-file-row.tsx`, `changes-panel-pr-files.tsx`, and `mapPRFilesToChangedFiles` in `changes-panel-helpers.ts` to use the shared primitive and normalizer without changing current Changes interactions.

### Review source completeness and UI

- Narrow `ReviewFile.status` to the shared status union and add `old_path` in `apps/web/components/review/types.ts`; add status-aware no-diff copy while keeping `diff_skip_reason` authoritative.
- Update `buildAllFiles` helpers in `apps/web/components/review/review-dialog.tsx` and `buildReviewSources` helpers in `apps/web/hooks/domains/session/use-review-sources.ts` to normalize every source, preserve `old_path`/`diff_skip_reason`, and retain status-only entries. Keep uncommitted > cumulative > PR precedence and `reviewFileKey` identity.
- Render the shared marker as the final `shrink-0` item in `review-file-tree.tsx`, after existing stale/comment metadata.
- Render a mobile-only marker in `FileDiffHeader` and status-specific patchless content in `review-diff-list.tsx`.

## Tests

- `apps/web/components/shared/file-status-icon.test.tsx`: table-driven visual and accessible contracts for added, untracked, modified, deleted, renamed, and unknown input.
- `apps/web/components/task/changes-panel-helpers.test.ts`: shared normalization and previous-path regression coverage.
- `apps/web/components/review/review-dialog.build-files.test.ts`: all PR statuses, pure rename inclusion, metadata propagation, and source precedence.
- `apps/web/hooks/domains/session/use-review-sources.test.ts`: status-only inclusion and matching normalization across Review/Changes sources.
- `apps/web/components/review/review-file-tree.test.tsx` and a focused status-empty-state test beside `types.ts`: marker semantics/layout and skip-reason precedence.

## E2E Tests

- `apps/web/e2e/tests/review/review-file-status.spec.ts`: create added, modified, deleted, and renamed files with `GitHelper`, hydrate Changes, open Review, and verify markers, narrow-sidebar layout, and the pure-rename empty state.
- `apps/web/e2e/tests/review/mobile-review-file-status.spec.ts`: open Review through the mobile Changes surface and verify the sticky-header marker remains usable without horizontal overflow.

## Implementation Waves

Wave 1:

- [x] [task-01-status-model](task-01-status-model.md) — done

Wave 2 (depends on Wave 1):

- [x] [task-02-review-ui](task-02-review-ui.md) — done

Wave 3 (depends on Wave 2):

- [x] [task-03-e2e](task-03-e2e.md) — done

## Verification

```bash
make fmt
make typecheck test lint
(cd apps && pnpm --filter @kandev/web test components/shared/file-status-icon.test.tsx components/task/changes-panel-helpers.test.ts components/review/review-file-tree.test.tsx components/review/types.test.ts components/review/review-dialog.build-files.test.ts hooks/domains/session/use-review-sources.test.ts)
(cd apps && pnpm --filter @kandev/web e2e:run tests/review/review-file-status.spec.ts)
(cd apps && pnpm --filter @kandev/web e2e:run --project mobile-chrome tests/review/mobile-review-file-status.spec.ts)
(cd apps/web && pnpm run typecheck)
```

## Risks

- Including patchless files changes review totals and empty-diff hashes; regression tests must cover marking them reviewed.
- Unknown provider statuses must degrade to modified rather than inventing categories.
- `old_path` is display metadata only; current path plus repository remains the review identity.
- Binary/large/truncated/budget messages must outrank generic patchless copy.
- The shared extraction must not change Changes rows' existing hover actions or status/stat visibility.
