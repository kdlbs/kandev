---
id: "01-agent-settings-layout"
title: "Agent settings layout"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/settings-profile-layout.md"
---

# Task 01: Agent settings layout

## Acceptance

- Configured agent cards show the existing `New profile` link in their header;
  the profile body has no count or separate creation/empty-state row.
- Unconfigured agent cards show only the existing `Setup profile` header action,
  while saved profile rows and their duplicate/delete behavior remain intact.
- The installed-agent toolbar places refresh/rescan immediately before the
  rightmost Add TUI Agent action, keeps terminal available, and preserves
  touch-safe targets on mobile.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm test -- components/settings/agents/agent-profiles-section.test.tsx components/settings/installed-agent-card.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint app/settings/agents/page.tsx components/settings/agents/agent-profiles-section.tsx components/settings/installed-agent-card.tsx
cd apps/web && pnpm run i18n:check
```

The focused card test covers both unconfigured setup-link branches, while the
existing profile-section test keeps the profile-row regression coverage.

## Files likely touched

- `apps/web/app/settings/agents/page.tsx`
- `apps/web/components/settings/agents/agent-profiles-section.tsx`
- `apps/web/components/settings/agents/agent-profiles-section.test.tsx`
- `apps/web/components/settings/installed-agent-card.tsx`
- `apps/web/components/settings/installed-agent-card.test.tsx`

## Dependencies

None.

## Parallelism

Sequential. The component markup, selector IDs, and unit assertions are one
small vertical slice.

## Inputs

- Spec: `docs/specs/agents/requirements/settings-profile-layout.md`, especially What and
  the first four scenarios.
- Plan: `plan.md`, Frontend and Mobile design contract sections.
- Existing patterns: `InstalledAgentCard`, `AgentProfilesSubList`, and the
  current `agent-profiles-section.test.tsx` store read-after-write tests.

## Output contract

Report the final changed files, exact unit/typecheck/lint/i18n commands and
results, any selector or responsive decisions, blockers, risks, and synchronized
task/plan status in the same conversation.

## Results

- `cd apps && pnpm install --frozen-lockfile` passed.
- `cd apps/web && pnpm test -- components/settings/agents/agent-profiles-section.test.tsx components/settings/installed-agent-card.test.tsx` passed (2 files, 8 tests), including both setup-link routes and both empty-body guards.
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm exec eslint app/settings/agents/page.tsx components/settings/agents/agent-profiles-section.tsx components/settings/installed-agent-card.tsx` passed.
- `cd apps/web && pnpm run i18n:check` passed. It reported the repository's existing advisory catalog parity/unreferenced-key warnings; this change adds no locale keys.
- Review follow-up added setup-link coverage, the saved-empty profile-body guard, and removed a redundant toolbar-count assertion in the desktop E2E test.
