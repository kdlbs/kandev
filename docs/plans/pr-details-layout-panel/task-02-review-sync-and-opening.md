---
id: "02-review-sync-and-opening"
title: "Review synchronization and opening behavior"
status: done
wave: 2
depends_on: ["01-reusable-panel-and-settings-cleanup"]
plan: "plan.md"
spec: "../../specs/ui/requirements/task-layout-profiles.md"
---

# Task 02: Review synchronization and opening behavior

Keep canonical PR Details content current without changing layout, and anchor explicit keyed review tabs to the canonical panel's configured group.

## Acceptance

- One focused review-sync hook updates params only when canonical `pr-detail` exists; it never adds, removes, moves, or activates any panel.
- GitHub PR remains preferred when both providers have linked reviews. Task/provider switches clear stale opposing params, and no-review state clears both identities while leaving the panel present.
- Closing/removing canonical PR Details is respected across later review-data updates; obsolete PR-panel-offered session storage is removed.
- Agent restoration no longer treats `pr-detail`, `mr-detail`, or keyed review panels as proof that their group belongs to Agent.
- Explicit PR/MR opening focuses an exact existing tab in its current group.
- A matching canonical panel is reused when it already renders the requested key. A new keyed panel joins canonical `pr-detail`'s group; if canonical is absent, it uses `centerGroupId`. No path creates a new split or moves an existing tab.
- Topbar and add-panel callers no longer pass global placement options or use the active Agent as a spatial policy.

## TDD sequence

1. Add pure sync tests for GitHub, GitLab, provider switch, empty review, and missing canonical panel; confirm current auto-add/remove code fails the contract.
2. Rewrite panel-action tests for canonical-group anchoring, fallback, reuse, and focus-in-place; confirm current relocation behavior fails.
3. Add an Agent-restoration regression where canonical PR Details lives in the right group.
4. Implement the unified hook and action changes; remove auto-panel and offered-storage code.
5. Re-run focused tests and typecheck before touching E2E.

## Files likely touched

- add `apps/web/components/task/dockview-review-panel-sync.ts`
- add `apps/web/components/task/dockview-review-panel-sync.test.ts`
- `apps/web/components/task/dockview-desktop-layout.tsx`
- `apps/web/components/task/dockview-session-tabs.ts`
- `apps/web/components/task/dockview-session-tabs.test.ts`
- remove `apps/web/components/task/dockview-auto-mr-panel.ts`
- remove `apps/web/components/task/dockview-auto-mr-panel.test.ts`
- replace/remove `apps/web/components/task/__tests__/use-auto-pr-panel.test.ts`
- `apps/web/lib/state/dockview-panel-actions.ts`
- `apps/web/lib/state/dockview-panel-actions.test.ts`
- `apps/web/lib/state/dockview-store.ts`
- remove `apps/web/lib/state/pr-panel-placement.ts`
- remove `apps/web/lib/state/pr-panel-placement.test.ts`
- `apps/web/lib/local-storage.ts`
- `apps/web/components/github/pr-topbar-button.tsx`
- `apps/web/components/gitlab/mr-topbar-button.tsx`
- `apps/web/components/task/dockview-add-panel-items.tsx`

## Verification

- `cd apps && pnpm --filter @kandev/web test components/task/dockview-review-panel-sync.test.ts components/task/dockview-session-tabs.test.ts lib/state/dockview-panel-actions.test.ts components/task/review-panel-provider.test.ts`
- `cd apps/web && pnpm run typecheck`
- `rg -n "useAutoPRPanel|useAutoMRPanel|PR_PANEL_OFFERED|resolvePRPanelTargetGroup|prPanelPlacement" apps/web`

## Dependencies

Task 01.

## Output contract

Report changed files, red/green test evidence, typecheck result, blockers, and residual risks; set this task to `done` and tick it in `plan.md`.
