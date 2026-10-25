---
id: "04-profile-refresh"
title: "Compact full-profile refresh"
status: complete
wave: 4
depends_on:
  - "03-onboarding-form"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-DISCOVERY-003
acceptance_criteria:
  - AC-AGENTS-PROFILE-DISCOVERY-003.2
  - AC-AGENTS-PROFILE-DISCOVERY-003.3
  - AC-AGENTS-PROFILE-DISCOVERY-003.4
  - AC-AGENTS-PROFILE-DISCOVERY-003.5
system_design:
  - ../../specs/agents/system-design/profile-capability-discovery.md
---

# Task 04: Compact full-profile refresh

## Summary

Apply the same standard icon sizing to model-list refresh on the full agent
profile page. Align it with model/mode selectors while preserving discovery,
recovery, and full profile configuration.

## In scope

- TDD action-component coverage for the localized accessible name, invocation,
  disabled/busy state, spinner, and retained error/recovery behavior.
- Change `RefreshCapabilitiesButton` to standard `Button size="icon"`; remove
  visible text and local height/min-height/full-width/padding overrides. Preserve
  tooltip, callback, busy state, and `profile-refresh-capabilities` test ID.
- Align desktop model/mode/refresh controls in `CapabilitiesRow`. On phones use
  model plus refresh, then optional mode; retain the existing page scroller.
  Error/status content must not stretch or reposition the action.
- Extend existing desktop/mobile profile discovery E2E before production changes
  with actual dimensions, alignment/containment, mode-present/absent states,
  busy/failure, keyboard and touch activation, and no horizontal overflow.

## Out of scope

- Removing profile advanced settings, modes, fallback, permissions, or model options.
- Discovery/API changes, runtime updates, new overlays, and generic sizing sweeps.

## Acceptance

- Refresh measures 28px square within 1px on fine-pointer desktop, and at least
  44px square on phone/coarse pointer. It aligns with selectors in every tested state.
- Localized naming, keyboard activation, busy/disabled state, callback, and
  retry/recovery behavior remain intact without changing the draft.
- Full-profile option/recovery tests pass; desktop/phone screenshots match UI-05.

## ASCII UI preview

UI-05, excerpt of [the combined preview](plan.md#ascii-ui-preview):

```text
Desktop:
Start model                 Start mode
[5.5 / High            v]   [Approve for me        v] [refresh]
Refresh: 28px square, same baseline as selectors.

Phone:
Start model
[5.5 / High                    v] [refresh]
Start mode
[Approve for me                         v]
Refresh: >=44px square; status below; page owns scroll.
```

Map to PROFILE-DISCOVERY-003.2-.5. The refresh label represents an icon action
with a localized accessible name and tooltip. Mode is absent for model-only
providers. Busy/error does not change action dimensions. Other profile settings
remain accessible. Names and spacing are illustrative; alignment and sizing are required.

## Verification

Run from repository root. E2E commands execute sequentially with managed rebuilds:

```bash
(cd apps/web && pnpm exec vitest run components/settings/profile-capability-status.test.tsx components/settings/profile-form-fields.test.tsx)
(cd apps/web && pnpm exec eslint components/settings/profile-capability-status.tsx components/settings/profile-capabilities-row.tsx e2e/tests/settings/profile-capability-discovery.spec.ts e2e/tests/settings/mobile-profile-capability-discovery.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/profile-capability-discovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-profile-capability-discovery.spec.ts tests/settings/mobile-agent-profile-config-selector.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Measure both icon dimensions, selector baseline, row/viewport containment, and
document overflow at desktop, 767px/768px, phone, and coarse-pointer tablet.
Include ready, busy, failed, long-label, mode-present and mode-absent states.
Inspect focused desktop/phone screenshots against UI-05.

## Files likely touched

- `apps/web/components/settings/profile-capability-status.tsx`
- `apps/web/components/settings/profile-capability-status.test.tsx` (new)
- `apps/web/components/settings/profile-capabilities-row.tsx`
- `apps/web/e2e/tests/settings/profile-capability-discovery.spec.ts`
- `apps/web/e2e/tests/settings/mobile-profile-capability-discovery.spec.ts`
- `apps/web/components/settings/profile-form-fields.test.tsx` if action-markup
  expectations change; preserve its behavior assertions.

## Dependencies

Task 03 precedes this in the sequential package. Both surfaces use the existing
`@kandev/ui` standard icon sizing rather than local dimension overrides.

## Risks

Leftover minimum heights can defeat nominal sizing. Inspect actual bounds.
Icon-only markup must retain accessible naming and 44px touch targets.
Optional mode/error content must not stretch or reposition refresh.

## Parallelism

`sequential`

## Inputs

- [Profile refresh requirements](../../specs/agents/requirements/profile-capability-discovery.md)
- [Responsive design](../../specs/agents/system-design/profile-capability-discovery.md#responsive-behavior)
- Existing profile discovery E2E and `apps/packages/ui/src/control-sizing.tsx`.

## Results

Replaced the wide action with a standard icon button and one responsive grid
instance. Unit tests verify a single refresh with optional modes and retain
advanced settings and discovery behavior. Desktop/phone E2E assertions measure
width, height, selector alignment, and uniqueness. The combined 67-test run,
focused ESLint, and typecheck passed. Managed desktop and mobile discovery tests
passed, including touch refresh, draft option resolution, and auth recovery.
Mode-present geometry is checked with verified capabilities; stale capabilities
omit unverified modes while retaining one refresh action. Fresh desktop/phone
screenshots show the intended row composition. Actual refresh bounds are 28px
square on desktop and 44px square on the phone.
