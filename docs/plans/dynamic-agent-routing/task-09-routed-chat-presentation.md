---
id: "09-routed-chat-presentation"
title: "Routed chat presentation"
status: in_progress
wave: 7
depends_on: ["06-logical-session-integration", "08-dynamic-profile-settings"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 09: Routed chat presentation

- **Acceptance:** Apply route/capability events by generation, preserve logical
  tab and author identity, localize reason enums, render immutable concrete
  badges, replace controls without remounting the composer, and render waiting
  or action-required recovery actions with expected generations.
- **Files likely touched:** `apps/web/lib/state/slices/session/**`,
  `apps/web/hooks/domains/session/**`, `apps/web/components/task/chat/**`,
  `apps/web/components/task/dockview-{shared,panel-content}.tsx`, locale catalogs.
- **Dependencies:** Tasks 06 and 08.
- **Parallelism:** parallel-safe with Task 07. This task owns only frontend
  session and chat files after Task 06 fixes the event and action contracts.
- **Inputs:** Spec Route and capability events, Settings interaction and mobile
  parity, and API surface, Tasks 06 and 08, existing session action hooks and
  mobile task chat composition.
- **Output contract:** Report generation handling, localized route reasons,
  mobile recovery controls, files changed, exact commands and results,
  blockers, risks, and synchronized task and plan status.
- **Verification:** `cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- lib/state/slices/session components/task/chat hooks/domains/session && cd web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet`
- **Risks:** Late frames cannot overwrite new provider controls or historical attribution.

## Results

In progress. Added generation-aware session state updates, route fields in
session events, localized waiting/action-required recovery UI, and desktop and
mobile recovery controls that keep the composer mounted. The rollout-blocker
repair adds one-request route recovery, restores the Dynamic profile list and
dedicated profile navigation, and filters disabled dynamic profiles from new
selection surfaces.

Verification:

- `pnpm --filter @kandev/web test -- --run lib/state/slices/session components/task/chat hooks/domains/session`
- `pnpm --filter @kandev/web run typecheck`
- `pnpm --filter @kandev/web run i18n:check`

The focused frontend checks passed. Immutable per-turn provider badges,
capability replacement, and routed-session Playwright coverage remain open.
