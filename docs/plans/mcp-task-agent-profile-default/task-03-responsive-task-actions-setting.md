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

- Task Actions displays accessible **Current task profile** and **Workspace default profile** choices. Visible plain-language copy explains that the setting is used when an agent calls a task-creating Kandev MCP tool without `agent_profile_id`, identifies `create_task_kandev` as the only affected tool for tasks and subtasks, names `spawn_session_kandev` and UI-created tasks as unaffected, and explains the explicit-profile override. Each option describes what it does, when to choose it, and the cost risk of inheriting the current profile. Secondary tooltip help is keyboard- and touch-reachable but is not required to understand the setting. Missing or unknown server values select `current_task`.
- Choosing a value updates the local settings draft. **Save changes** sends only `mcp_task_agent_profile_default`; a failed save keeps the draft selected and leaves the stored preference unchanged.
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
- Patterns: `ArchiveConfirmationSettings` for the shared explicit-save workflow and `VoiceModeSettings` for accessible descriptive radio choices.
- Follow `apps/web/AGENTS.md` and the `/mobile-parity` skill. Keep touch targets reachable and allow option descriptions to wrap at narrow widths.

## Output Contract

Report state/wire/UI changes, responsive decisions, focused tests and typecheck results, files touched, blockers, and residual UI risks. Set this task to `done` and update its plan checkbox only after targeted verification passes.
