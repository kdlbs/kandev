---
id: "01-reusable-panel-and-settings-cleanup"
title: "Reusable PR Details panel and settings cleanup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-layout-profiles.md"
---

# Task 01: Reusable PR Details panel and settings cleanup

Remove the branch's parallel placement preference, then make canonical `pr-detail` valid reusable layout content with code-defined Default and compact placement.

## Acceptance

- No backend, boot-state, frontend HTTP, SSR, Zustand, WebSocket, or Appearance-settings contract contains `pr_panel_placement` / `prPanelPlacement`.
- `REUSABLE_PANEL_IDS` contains canonical `pr-detail`, whose editor/runtime registry title is `PR Details`; `mr-detail` and keyed review IDs remain non-reusable.
- `defaultLayout()` puts PR Details beside Agent in `CENTER_GROUP` with Agent selected, while Files and Changes remain in `RIGHT_TOP_GROUP`.
- `compactLayout()` includes PR Details in its single group.
- Layout-profile validation accepts canonical `pr-detail`, rejects keyed review IDs, and still enforces one instance per reusable panel.
- Layout editor renders a lightweight PR Details placeholder and exposes it through existing add/move/remove controls without mounting live review data.
- No schema migration or replacement user setting is added.

## TDD sequence

1. Change preset, profile-validation, and editor-action tests first; run them and confirm failures against current registry/presets.
2. Remove global-setting assertions and add a search/check proving no branch-only contract remains.
3. Implement registry, preset, profile-description, editor-placeholder, and contract cleanup.
4. Re-run focused frontend tests, backend tests affected by restored settings shapes, and typecheck.

## Files likely touched

- `apps/web/lib/state/layout-manager/constants.ts`
- `apps/web/lib/state/layout-manager/presets.ts`
- `apps/web/lib/state/layout-manager/presets.test.ts`
- `apps/web/lib/layout/layout-profiles.ts`
- `apps/web/lib/layout/layout-profiles.test.ts`
- `apps/web/components/settings/layouts/layout-editor.tsx`
- `apps/web/components/settings/layouts/layout-editor-actions.test.ts`
- `apps/web/components/settings/general-settings.tsx`
- `apps/web/components/settings/editors-settings-state.tsx`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/types/http.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ssr/user-settings.test.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/ws/handlers/users.ts`
- `apps/web/lib/ws/handlers/users.test.ts`
- `apps/web/hooks/use-user-display-settings.ts`
- `apps/web/hooks/use-ensure-user-settings.test.ts`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_state_user_settings_test.go`
- `apps/backend/internal/user/controller/controller.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/dto/dto_test.go`
- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/service/service_test.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sqlite_test.go`

## Verification

- `cd apps && pnpm --filter @kandev/web test lib/state/layout-manager/presets.test.ts lib/layout/layout-profiles.test.ts components/settings/layouts/layout-editor-actions.test.ts lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts hooks/use-ensure-user-settings.test.ts`
- `cd apps/web && pnpm run typecheck`
- `make -C apps/backend test`
- `rg -n "pr_panel_placement|prPanelPlacement" apps docs/public`

## Dependencies

None.

## Output contract

Report changed files, red/green test evidence, backend/typecheck results, blockers, and residual risks; set this task to `done` and tick it in `plan.md`.
