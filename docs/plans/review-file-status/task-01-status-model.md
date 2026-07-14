---
id: "01-status-model"
title: "Shared file status model"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/review-file-status.md"
---

# Task 01: Shared file status model

Extract the existing Changes status language into a domain-neutral, accessible primitive.

## Acceptance

- `normalizeFileChangeStatus` preserves local statuses, maps GitHub `removed` to `deleted`, preserves `renamed`, and maps unknown values to `modified`.
- `FileStatusIcon` renders plus/dot/minus/arrow markers with semantic labels and optional previous-path context; untracked remains semantically distinct from added.
- Both Changes file-row variants use the shared component without changing row actions or layout.

## Verification

- `cd apps/web && pnpm test components/shared/file-status-icon.test.tsx components/task/changes-panel-helpers.test.ts`
- `cd apps/web && pnpm run typecheck`

## Files likely touched

- `apps/web/lib/utils/file-change-status.ts`
- `apps/web/components/shared/file-status-icon.tsx`
- `apps/web/components/shared/file-status-icon.test.tsx`
- `apps/web/components/task/changes-panel-file-row.tsx`
- `apps/web/components/task/changes-panel-pr-files.tsx`
- `apps/web/components/task/changes-panel-helpers.ts`
- `apps/web/components/task/changes-panel-helpers.test.ts`
- remove `apps/web/components/task/file-status-icon.tsx`

## Inputs

- Spec: `What`, marker semantics and accessibility.
- Pattern: current `FileStatusIcon` and its call sites in the Changes panel.
- Constraint: preserve Changes hover/action behavior.

## Dependencies

None.

## Output contract

Report the files changed, focused tests/typecheck run, blockers, and residual risks; set this task to `done` and tick it in `plan.md`.
