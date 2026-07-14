---
id: "02-review-ui"
title: "Review status UI"
status: done
wave: 2
depends_on: ["01-status-model"]
plan: "plan.md"
spec: "../../specs/ui/review-file-status.md"
---

# Task 02: Review status UI

Carry canonical status metadata through Review and render it on desktop, mobile, and patchless files.

## Acceptance

- Review retains and normalizes every changed file, including patchless pure renames, while preserving source precedence, multi-repository identity, previous path, and explicit skip reason.
- Desktop tree rows show a fixed trailing marker without displacing reviewed, stale, or comment metadata; mobile sticky headers expose the same status without hover.
- Patchless files show status-specific explanatory text, while explicit binary/size/truncation/budget reasons take precedence.

## Verification

- `cd apps/web && pnpm test components/review/review-file-tree.test.tsx components/review/review-dialog.build-files.test.ts hooks/domains/session/use-review-sources.test.ts`
- `cd apps/web && pnpm run typecheck`

## Files likely touched

- `apps/web/components/review/types.ts`
- focused status-empty-state test beside `apps/web/components/review/types.ts`
- `apps/web/components/review/review-dialog.tsx`
- `apps/web/components/review/review-dialog.build-files.test.ts`
- `apps/web/hooks/domains/session/use-review-sources.ts`
- `apps/web/hooks/domains/session/use-review-sources.test.ts`
- `apps/web/components/review/review-file-tree.tsx`
- `apps/web/components/review/review-file-tree.test.tsx`
- `apps/web/components/review/review-diff-list.tsx`

## Inputs

- Spec: all scenarios.
- Task 01: `normalizeFileChangeStatus`, `FileChangeStatus`, and shared `FileStatusIcon`.
- Preserve `reviewFileKey`, uncommitted > cumulative > PR precedence, and current `diff_skip_reason` copy.

## Dependencies

Task 01.

## Output contract

Report the files changed, focused tests/typecheck run, blockers, and residual risks; set this task to `done` and tick it in `plan.md`.
