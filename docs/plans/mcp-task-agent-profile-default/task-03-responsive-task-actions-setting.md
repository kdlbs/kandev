---
id: "03-responsive-task-actions-setting"
title: "Responsive Task Actions setting"
status: done
wave: 2
depends_on: ["01-backend-preference-contract"]
plan: "plan.md"
spec: "../../specs/tasks/mcp-task-agent-profile-default/spec.md"
---

# Task 03: Responsive Task Actions Setting

## Acceptance

- Task Actions displays accessible **Current task profile** and **Workspace default profile** choices, explains that workspace-default mode still honors workflow profiles first, and selects `current_task` for missing/unknown server values.
- Choosing a value updates state optimistically, sends only `mcp_task_agent_profile_default`, disables duplicate saves, and rolls back only when doing so cannot overwrite newer state.
- HTTP boot hydration, WebSocket updates, reloads, and narrow mobile layouts preserve the selected value without clipped text or horizontal page overflow.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts components/settings/mcp-task-agent-profile-default-settings.test.tsx
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ssr/user-settings.test.ts`
- `apps/web/lib/ws/handlers/users.ts`
- `apps/web/lib/ws/handlers/users.test.ts`
- `apps/web/hooks/use-ensure-user-settings.test.ts`
- `apps/web/hooks/use-user-display-settings.ts`
- `apps/web/components/settings/editors-settings-state.tsx`
- `apps/web/components/settings/mcp-task-agent-profile-default-settings.tsx`
- `apps/web/components/settings/mcp-task-agent-profile-default-settings.test.tsx`
- `apps/web/components/settings/general-settings.tsx`
- `apps/web/components/settings/general-nav.ts`

## Dependencies

- `01-backend-preference-contract` defines the wire field and enum values.

## Inputs

- Spec: What, API surface, mobile scenario, and failed-save scenario.
- Plan: Settings state and wire mapping; Task Actions control.
- Patterns: `ArchiveConfirmationSettings` for optimistic save/guarded rollback and `VoiceModeSettings` for accessible descriptive radio choices.
- Follow `apps/web/AGENTS.md` and the `/mobile-parity` skill. Keep touch targets reachable and allow option descriptions to wrap at narrow widths.

## Output Contract

Report state/wire/UI changes, responsive decisions, focused tests and typecheck results, files touched, blockers, and residual UI risks. Set this task to `done` and update its plan checkbox only after targeted verification passes.
