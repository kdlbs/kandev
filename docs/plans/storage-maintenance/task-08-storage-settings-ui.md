---
id: "08-storage-settings-ui"
title: "Responsive Storage settings UI"
status: done
wave: 4
depends_on: ["07-system-storage-api"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 08: Responsive Storage settings UI

Build the desktop and mobile System Storage surface with safe confirmations and persisted feedback.

## Acceptance

- The page supports analyze, settings persistence, manual run, resource results, run history,
  quarantine restore, and confirmed permanent deletion without component-level direct fetching.
- Dedicated-Docker and external-Go-cache warnings are explicit; disabled/unavailable actions explain
  why, and async jobs recover through WS plus polling fallback.
- At Pixel 5 width, every action is reachable through the settings sheet, cards stack without page
  overflow, long paths wrap, and touch actions are at least 44 px high.

## Verification

```bash
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps && pnpm --filter @kandev/web test
```

## Files likely touched

- `apps/web/app/settings/system/storage/page.tsx`
- `apps/web/components/app-sidebar/sections/settings/system-group.tsx`
- `apps/web/components/settings/system/storage/*.tsx`
- `apps/web/hooks/domains/system/use-storage-maintenance.ts`
- `apps/web/lib/api/domains/system-api.ts`
- `apps/web/lib/types/system.ts`
- system Zustand slice/WS handler files
- colocated frontend tests

## Dependencies

Task 07.

## Inputs

- Spec UI/API scenarios
- `apps/web/AGENTS.md` data-flow and component rules
- Existing System cards, job progress, dialogs, settings mobile sheet, and action feedback patterns
- Mobile parity requirements

## Output contract

Report desktop/mobile layout, API/state wiring, confirmations, rendered verification, tests run,
blockers/risks, and update this task plus `plan.md` to done.
