---
id: "27-work-item-detail-ui"
title: "Responsive work-item detail"
status: completed
wave: 13
depends_on:
  [
    "19-browse-preferences",
    "20-work-item-detail",
    "21-work-item-mutations",
    "22-task-work-item-links",
    "23-provider-presets",
  ]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 27: Responsive Work-Item Detail

## Acceptance

- `PATCH /api/v1/azure-devops/work-items/:id` supports only Assign to me and
  Unassign with `project` plus displayed revision. It resolves the stored PAT
  identity, emits an allowlisted `/rev`-guarded patch, maps stale revision to
  409, and returns the normalized updated item without needing board context.
- Board cards and work-item rows open shared read-only detail showing core
  fields, sanitized description, planning values, discussion paging/retry,
  linked tasks, Azure link, and a visible quick-action Task menu.
- Core detail and discussion load independently. Comments are newest-first,
  older pages append by opaque continuation token without duplicates, and a
  failed page retries without discarding the item or prior pages. Provider HTML
  uses `rehype-raw` plus `rehype-sanitize`; unsafe markup/URLs never mount.
- All contexts expose Assign to me/Unassign. Only detail opened with board
  context exposes column/split controls, using the existing board mutation
  endpoint and current revision. Failed mutations keep detail open and restore
  the last confirmed fields; conflict refreshes the board/detail.
- Quick actions open task creation with provider URL, title, description, and
  selected prompt. Success persists the task/work-item association before
  invalidating linked-task state; failure is visible and retryable.
- Desktop uses a modal `Dialog`; phone uses one safe-area-aware full-height
  `Drawer` with fixed context/actions, one internal scroll owner, 44px controls,
  focus return, and no document horizontal overflow. Closing either surface
  leaves the originating board/search filters and scroll position intact.

## TDD Sequence

1. Add backend controller/service tests for direct assignment/unassignment,
   arbitrary-assignee rejection, revision conflict, PAT identity failure, and
   workspace authorization; run red before adding the route.
2. Add hook tests for independent loads, continuation append/dedup, section
   retries, linked-task lookup/invalidation, mutation rollback/conflict, and
   quick-action association failure; run red before implementing the hook.
3. Add component tests for sanitized content, read-only fields, board-only
   controls, desktop dialog, and phone drawer structure. Implement the shared
   domain hook and responsive compositions with all focused tests green.
4. Add the initial failing desktop/mobile Playwright detail assertions in the
   existing Azure specs; Task 29 completes the integrated flows and fixtures.

## Verification

- `go test ./internal/azuredevops ./internal/orchestrator` from `apps/backend` — passed (1,455 tests).
- `pnpm test -- --run components/azure-devops/azure-devops-work-item-detail.test.tsx components/azure-devops/azure-devops-task-launcher.test.tsx hooks/domains/azure-devops/use-azure-devops-work-item-detail.test.ts hooks/domains/azure-devops/use-azure-devops-watches.test.ts` from `apps/web` — passed (7 tests).
- `pnpm --filter @kandev/web typecheck` from `apps` — passed.

## Files Likely Touched

- `apps/backend/internal/azuredevops/client.go`
- `apps/backend/internal/azuredevops/rest_client.go`
- `apps/backend/internal/azuredevops/service_work_item_mutations.go`
- `apps/backend/internal/azuredevops/service_work_item_mutations_test.go`
- `apps/backend/internal/azuredevops/controller.go`
- `apps/backend/internal/azuredevops/controller_test.go`
- `apps/web/lib/types/azure-devops.ts`
- `apps/web/lib/api/domains/azure-devops-api.ts`
- `apps/web/hooks/domains/azure-devops/use-azure-devops-work-item-detail.ts`
- `apps/web/hooks/domains/azure-devops/use-azure-devops-work-item-detail.test.tsx`
- `apps/web/hooks/domains/azure-devops/use-azure-devops-board.ts`
- `apps/web/components/azure-devops/azure-devops-board.tsx`
- `apps/web/components/azure-devops/azure-devops-results.tsx`
- `apps/web/components/azure-devops/azure-devops-work-item-detail.tsx`
- `apps/web/components/azure-devops/azure-devops-task-actions.tsx`
- `apps/web/components/azure-devops/azure-devops-task-launcher.tsx`
- `apps/web/app/azure-devops/azure-devops-page-client.tsx`

## Dependencies

Tasks 19-23.

## Parallelism

Sequential. Detail, board state, task launch, and association cache share one
frontend interaction model.

## Inputs

- Spec: work-item detail/mutation/quick-action scenarios.
- Mobile exemplar: `task-layout.tsx` and
  `session-task-switcher-sheet.tsx` for dedicated full-height detail and
  safe-area-aware drawer geometry.
- Desktop exemplar: existing Kandev detail dialogs; use `@kandev/ui/dialog`.
- Required skills during implementation: `/tdd`, `/mobile-parity`, `/e2e`.

## Mobile Design Contract

- Desktop outcome: inspect one item without losing board context, take the
  allowed mutations, or launch a task.
- Phone entry point: tapping the visible card body opens detail directly from
  the focused column; no intermediate menu.
- Hierarchy: fixed title/type/status header, scrollable description/planning/
  discussion body, then persistent explicit Task and assignment/move actions.
- Surface rationale: detail is dense primary content, so a full-height Drawer
  is preferable to an inset temporary picker; temporary column/action choices
  may use the shared responsive menu treatment.
- Shared hooks own data, paging, mutations, linked tasks, and launch payloads;
  only desktop/mobile composition differs.
- Use `100dvh`, one `min-h-0 overflow-y-auto` body, bottom safe-area clearance,
  focus return, and at least 44px touch targets.

## Risks

- Sanitize Azure HTML before rendering and do not let comment failures close or
  replace already loaded core detail.
- Search results have no authoritative board selection. Never infer a column
  mutation context from the work item's state; hide board controls there while
  retaining direct assignment.

## Output Contract

Report responsive compositions, RED/GREEN commands, rendered desktop/mobile
inspection, files changed, risks, and update task/plan status.
