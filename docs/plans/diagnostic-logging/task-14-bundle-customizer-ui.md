---
id: "14-bundle-customizer-ui"
title: "Bundle customizer UI"
status: done
wave: 11
depends_on:
  - "12-custom-bundle-contracts"
  - "13-on-demand-acp-collection"
plan: "plan.md"
spec: "../../specs/platform/diagnostic-logging.md"
---

# Task 14: Bundle customizer UI

## Acceptance

- Logs shows translated standard-source boundaries and frontend/backend event
  classes, plus **Download standard bundle** and **Customize bundle**; ACP
  controls derive only from backend capabilities.
- Debug-capable users also see **Download with ACP…**. ACP submission requires
  one to ten eligible selected sessions and replaces the standard reassurance
  with the exact raw-protocol content/secrets warning. With no eligible session,
  the action remains visible and opens a non-submittable explanatory empty state.
- Desktop uses a Dialog and phone an inset Drawer with shared source/session,
  validation, polling, partial/error, and download logic; the phone surface has
  one scroll owner, safe-area actions, ≥44px targets, and no horizontal overflow.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- \
  lib/api/domains/system-api.test.ts \
  components/settings/system/log-viewer.test.tsx
```

```bash
cd apps/web
pnpm run i18n:ratchet
pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/types/system.ts`
- `apps/web/lib/api/domains/system-api.ts`
- `apps/web/lib/api/domains/system-api.test.ts`
- `apps/web/components/settings/system/log-viewer.tsx`
- `apps/web/components/settings/system/log-viewer.test.tsx`
- `apps/web/components/settings/system/bundle-customizer.tsx`
- `apps/web/hooks/use-responsive-breakpoint.ts`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pseudo/settings.json`

## Dependencies

- Task 12 provides capabilities, candidates, runtime source, and request DTOs.
- Task 13 provides actual ACP availability/partial behavior.

## Parallelism

Sequential after Tasks 12 and 13. The component and API types must land
together so debug capability cannot be inferred client-side.

## Inputs

- Spec: System Logs page and bundle contents plus desktop/mobile scenarios.
- Plan: Logs page customization and mobile design contract.
- Mobile reference: `mobile-menu-sheet` and `mobile-picker-sheet` Drawer
  composition; existing System Logs card is the route/card baseline.

## Risks

- Standard no-transcript copy must explicitly exclude ACP-inclusive bundles and
  must not promise that incidental text already emitted into logs is redacted.
- Dialog/Drawer wrappers must not duplicate request state or let source/session
  validation drift across viewports.
- This task adds no persisted preference and no browser/database migration.

## Output contract

Report desktop/mobile interaction, translated disclosure, source/session
defaults and validation, files changed, exact tests/results, rendered mobile
evidence, blockers/risks, and synchronize this task plus `plan.md` status.

## Results

Implemented the translated standard/custom/ACP bundle workflow.

- Added backend-driven capability and session API clients/types, with standard
  frontend/backend defaults, optional runtime index, and explicit one-to-ten
  ACP session selection.
- Added a shared desktop Dialog / phone inset Drawer customizer with a single
  mobile scroll owner, safe-area footer, 44px targets, no horizontal overflow,
  empty/validation states, and the ACP content/secrets disclosure.
- Updated Logs copy to explain backend/frontend event classes, incidental text,
  and that standard bundles exclude stored chat, session, and agent messages.
- Verification: focused system API/Logs UI tests pass (28 tests); `pnpm run
  i18n:ratchet`, `pnpm run i18n:check`, `pnpm run typecheck`, and `pnpm run
  lint` all pass.
