---
id: "08-dynamic-profile-settings"
title: "Dynamic profile settings"
status: completed
wave: 4
depends_on: ["03-dynamic-profile-management"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 08: Dynamic profile settings

- **Acceptance:** Add typed API/state support and desktop/mobile dynamic profile
  creation, candidate ordering, validation, dependency dialogs, and core error
  actions behind the feature flag.
- **Files likely touched:** `apps/web/lib/types/agent-profile.ts`,
  `apps/web/lib/api/domains/agent-profile-normalize.ts`,
  `apps/web/components/settings/agents/agent-profiles-section.tsx`,
  `apps/web/components/settings/agent-profile-page*.tsx`, locale catalogs.
- **Dependencies:** Task 03.
- **Parallelism:** parallel-safe with Task 04. This task owns only web profile
  settings, state, and locale files after Task 03 fixes the API contract.
- **Inputs:** Spec Settings interaction and mobile parity, Task 03 API contract,
  `/mobile-parity`, `agent-profiles-section.tsx`, `MobilePickerSheet`, and the
  Kandev mobile UI language reference.
- **Output contract:** Report desktop and phone entry points, surface choice,
  scroll owner, touch targets, files changed, exact commands and results,
  blockers, risks, and synchronized task and plan status.
- **Verification:** `cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- components/settings lib/api/domains/agent-profile-normalize.test.ts && cd web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet`
- **Risks:** Phone uses direct navigation, one scroll owner, 44px reorder actions, and no drag-only behavior.

## Results

Completed. Added typed dynamic profile API/state normalization, desktop
settings editing, direct mobile profile navigation, ordered candidate editing,
validation, localized copy, and recovery-state support.

Verification:

- `pnpm --filter @kandev/web test -- --run lib/api/domains/agent-profile-normalize.test.ts lib/state/slices/features/features-contract.test.ts components/settings/agents/agent-profiles-section.test.tsx lib/ws/client.test.ts`
- `pnpm --filter @kandev/web run typecheck`
- `pnpm --filter @kandev/web run i18n:check`
- `pnpm --filter @kandev/web run i18n:ratchet`

All commands passed. Dedicated settings Playwright coverage remains pending.
The separate Provider Error Policies package owns one-draft creation and the
transient/hard retry, reset-wait, skip, and stop editor that supersedes the
generic action control delivered here.
