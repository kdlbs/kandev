---
id: "01-profile-row-actions"
title: "Profile row action presentation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/settings-profile-layout.md"
---

# Task 01: Profile row action presentation

## Acceptance

- Full desktop profile rows show compact icon-only Duplicate and Delete buttons
  inline, with translated accessible names, matching hover/focus tooltips, and
  no visible profile-actions overflow trigger.
- Compact desktop, tablet, and mobile profile rows keep the existing
  three-dots menu as the only action presentation, with both actions inside it.
- Duplicate and Delete use the existing mutation hook, confirmation dialog,
  profile-link layering, and store synchronization behavior.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test -- components/settings/agents/agent-profiles-section.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/settings/agents/agent-profiles-section.tsx components/settings/agents/agent-profiles-section.test.tsx)
(cd apps/web && pnpm run i18n:check)
```

## Files likely touched

- `apps/web/components/settings/agents/agent-profiles-section.tsx`
- `apps/web/components/settings/agents/agent-profiles-section.test.tsx`

## Dependencies

None.

## Parallelism

Sequential. The component and its responsive unit coverage form one vertical
slice.

## Inputs

- Spec: `docs/specs/agents/requirements/settings-profile-layout.md`, especially the new
  full-desktop and below-breakpoint scenarios.
- Plan: `plan.md`, Frontend and Mobile design contract sections.
- Existing patterns: `useResponsiveBreakpoint`, `useProfileDuplicate`, and
  `AgentProfileDeleteConfirmDialog`.

## Output contract

Report changed files, the exact focused test/typecheck/lint/i18n commands and
results, responsive selector decisions, and any blockers. Update this task's
status and the parent plan only after all listed checks pass.

## Results

- `cd apps && pnpm install --frozen-lockfile` passed.
- `cd apps/web && pnpm test -- components/settings/agents/agent-profiles-section.test.tsx` passed (9 tests).
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm exec eslint components/settings/agents/agent-profiles-section.tsx components/settings/agents/agent-profiles-section.test.tsx` passed with no errors or warnings.
- `cd apps/web && pnpm run i18n:check` passed with the repository's existing advisory catalog warnings.
- `cd apps/web && pnpm run i18n:ratchet` passed with 0 new-code violations.

The responsive boundary uses the canonical `isFullDesktop` value. Full-desktop
actions are compact icon-only buttons with translated accessible names and
profile-scoped test IDs. Each inline button has a tooltip using the same
translated action label; the overflow trigger and menu actions retain separate
stable IDs. Both presentations call the same duplicate hook and delete
confirmation callback.
