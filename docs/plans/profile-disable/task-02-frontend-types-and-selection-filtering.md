---
id: "02-frontend-types-and-selection-filtering"
title: "Frontend types and selection filtering"
status: done
wave: 2
depends_on: ["01-backend-enabled-column"]
plan: "plan.md"
spec: "../../specs/agents/requirements/profile-disable.md"
---

# Task 02: Frontend types and selection filtering

- **Acceptance:** `AgentProfile.enabled`, `AgentProfilePayload.enabled`, `AgentProfileOption.enabled`, `updateAgentProfileAction` payload, `normalizeAgentProfile` (missing → `true`), and `toAgentProfilePayload` all carry the field; `toAgentProfileOption` maps `enabled: profile.enabled ?? true`.
- **Acceptance:** A shared `isSelectableAgentProfile` predicate treats omitted `enabled` as selectable and explicit `false` as disabled; every selection path uses it. Disabled profiles are absent from: `useAgentProfileOptions` output (new task, new subtask, new session, quick chat), `useExecutorProfileCompat`'s `compatibleAgentProfiles` (autopick last-used / workspace-default / first fallback), `useHandoffProfiles`, the new-session default fallback, and the quick-chat workspace-default application. Existing session labels and the raw store list are untouched.
- **Acceptance:** Unit tests cover the predicate's legacy/enabled/disabled cases, the normalize mapping, `toAgentProfileOption`, `useAgentProfileOptions` filtering, direct executor compatibility filtering of mixed raw profiles, autopick never resolving to a disabled profile, handoff filtering, and new-session default fallback.
- **Verification:** `cd apps && pnpm --filter @kandev/web test -- task-create-dialog-computed task-create-dialog-options task-create-dialog-effects quick-chat-setup handoff-profile-menu-items new-session-dialog agent-profile-normalize settings/types`
- **Files likely touched:**
  - `apps/web/lib/types/agent-profile.ts`
  - `apps/web/lib/api/domains/agent-profile-normalize.ts`
  - `apps/web/app/actions/agents.ts`
  - `apps/web/lib/state/slices/settings/types.ts`
  - `apps/web/components/task-create-dialog-options.tsx`
  - `apps/web/components/task-create-dialog-computed.ts`
  - `apps/web/components/task/handoff-profile-menu-items.tsx`
  - `apps/web/components/task/new-session-dialog.tsx`
  - `apps/web/components/quick-chat/quick-chat-setup.tsx`
  - matching `*.test.ts(x)` files.
- **Dependencies:** Task 01 (DTO/API exposes `enabled`).
- **Parallelism:** sequential.
- **Inputs:** spec "What" bullets 4–7 and the three selection scenarios; plan "Frontend — Types & client" and "Selection filtering" sections.
- **Output contract:** Report red/green test evidence per filter, changed files, targeted test results, risks, and task/plan status update.
