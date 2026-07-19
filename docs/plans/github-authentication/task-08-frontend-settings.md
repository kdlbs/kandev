---
id: "08-frontend-settings"
title: "Workspace and personal GitHub settings"
status: completed
wave: 6
depends_on: ["07-http-health-mocks"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 08: Workspace And Personal GitHub Settings

## Inputs

- Spec: What, Identity And Routing, state machines, failure modes, and desktop/mobile scenarios.
- Task 07 API contract.
- `apps/web/AGENTS.md` and the mobile-parity skill.

## Acceptance

- GitHub status/state is keyed by workspace and refetched on workspace changes without showing a
  previous workspace's actor, repositories, diagnostics, permissions, or rate limits.
- Settings expose feature-complete Workspace automation and My GitHub identity sections for legacy,
  PAT, named CLI, App installation, personal connect/reconnect, capability, and actor states.
- App-only `My GitHub` gating and user-triggered mutation attribution match the spec on desktop and
  mobile; all controls fit and remain keyboard/screen-reader accessible.

## Files Likely Touched

- `apps/web/lib/api/domains/github-api.ts`
- `apps/web/lib/types/github.ts`
- `apps/web/lib/state/slices/github/github-slice.ts`
- `apps/web/lib/state/slices/github/github-slice.test.ts`
- `apps/web/hooks/domains/github/use-github-status.ts`
- `apps/web/hooks/domains/github/use-github-status.test.tsx` (new)
- `apps/web/components/github/github-status.tsx`
- `apps/web/components/github/github-settings.tsx`
- `apps/web/components/integrations/integrations-menu.tsx`
- `apps/web/app/github/github-page-client.tsx`
- GitHub PR review/merge components that display the effective actor

## Verification

```bash
cd apps/web && rtk pnpm run typecheck
cd apps && rtk pnpm --filter @kandev/web test -- github
cd apps && rtk pnpm --filter @kandev/web lint
```

## Output Contract

Report states and responsive behavior implemented, accessibility checks, tests run, screenshots
inspected, files touched, blockers, and risks.
