---
id: "02-frontend-duplicate-ui"
title: "Add the frontend duplicate UI"
status: done
wave: 2
depends_on: ["01-backend-duplicate-endpoint"]
plan: "plan.md"
spec: "../../specs/agents/requirements/profile-duplicate.md"
---

# Task 02: Frontend duplicate UI

## Acceptance

- `duplicateAgentProfileAction(profileId)` POSTs to
  `/api/v1/agent-profiles/:id/duplicate` and returns the normalized
  `AgentProfile`.
- Each profile row on `/settings/agents` shows a Duplicate icon button
  (outside the row link, beside the enabled switch) with an accessible label
  `Duplicate <name>`, `data-testid="duplicate-profile-<id>"`, and a touch
  target of at least 44×44px. Clicking it creates the copy and adds the new
  row to the store immediately (toast on success/failure). No navigation.
- The profile settings page header (`/settings/agents/<agent>/profiles/<id>`)
  shows a Duplicate button (`data-testid="duplicate-profile-header"`,
  `min-h-11` touch target). On success it toasts and navigates to the copy's
  settings page.
- All new user-facing copy goes through `t()`; the persisted copy name stays
  server-side (`<source> Copy`) and is not localized.
- New en keys exist in `agents.json`; the pseudo locale is regenerated;
  `i18n:check` and `i18n:ratchet` pass for changed lines.

## Verification

- Component test:
  `cd apps/web && pnpm vitest run app/settings/agents/profile-list-item.test.tsx`
- i18n:
  `cd apps/web && pnpm run i18n:pseudo && pnpm run i18n:check`
- Typecheck + lint of changed files:
  `cd apps/web && pnpm run typecheck`
  `cd apps && pnpm --filter @kandev/web lint` (or the repo's configured lint command)

## Files likely touched

- `apps/web/app/actions/agents.ts`
- `apps/web/app/settings/agents/profile-list-item.tsx`
- `apps/web/app/settings/agents/profile-list-item.test.tsx`
- `apps/web/app/settings/agents/page.tsx`
- `apps/web/components/settings/agent-profile-page.tsx`
- `apps/web/src/locales/en/agents.json`
- `apps/web/src/locales/pseudo/agents.json` (regenerated)

## Dependencies

Task 01 (endpoint must exist for the action to hit).

## Parallelism

Sequential by default; may run in parallel with Task 03 only with explicit
user authorization.

## Inputs

- Spec duplicate scenarios and API surface
- `useProfileEnabledToggle` / `applyEnabledProfileUpdate` store-merge
  pattern; `agent-profile-page-state.ts` delete/navigate flow; `normalizeAgentProfile`;
  `ws/handlers/agents.ts` `handleProfileCreated` upsert-by-ID behaviour
  (verify no double-row when the direct merge and WS both insert).

## Output contract

Report RED and GREEN results, store-merge approach, toast keys, changed
files, and update this task plus `plan.md` status.
