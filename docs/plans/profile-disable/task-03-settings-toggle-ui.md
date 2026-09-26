---
id: "03-settings-toggle-ui"
title: "Settings toggle UI"
status: done
wave: 3
depends_on: ["02-frontend-types-and-selection-filtering"]
plan: "plan.md"
spec: "../../specs/agents/requirements/profile-disable.md"
---

# Task 03: Settings toggle UI

- **Acceptance:** The profile settings page header (`/settings/agents/<agent>/profiles/<id>`) renders an Enabled `Switch` reflecting `savedProfile.enabled`; flipping it marks the page dirty and the save PATCH includes `enabled`; after save the header matches the persisted value.
- **Acceptance:** The `/settings/agents` "Manage existing profiles by agent" list renders an Enabled `Switch` per profile row; flipping it saves immediately via `updateAgentProfileAction(profile.id, { enabled })`, updates the store (`settingsAgents` + `agentProfiles`), does not navigate the row link, and reflects the new state without reload.
- **Acceptance:** Component tests cover the header toggle dirty/save payload and the list-row toggle action + store sync.
- **Verification:** `cd apps && pnpm --filter @kandev/web test -- agent-profile-page app/settings/agents`
- **Files likely touched:**
  - `apps/web/components/settings/agent-profile-page.tsx`
  - `apps/web/app/settings/agents/page.tsx`
  - `apps/web/components/settings/agent-profile-page.test.tsx`
  - `apps/web/app/settings/agents/page.test.tsx` (or a new `ProfileListItem` component test)
- **Dependencies:** Task 02 (types carry `enabled`; store options carry it).
- **Parallelism:** sequential.
- **Inputs:** spec "What" bullets 2–3 and the list-toggle + header-toggle scenarios; plan "Frontend — Settings UI" section. Reuse the `useSyncAgentsToStore` pattern from `agent-profile-page.tsx` for the list-row store sync.
- **Output contract:** Report red/green test evidence, changed files, targeted test results, risks, and task/plan status update.
