---
id: "01-apprise-rescan"
title: "Add Apprise rescan"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-APPRISE-RESCAN-001
acceptance_criteria:
  - AC-PLATFORM-APPRISE-RESCAN-001.1
  - AC-PLATFORM-APPRISE-RESCAN-001.2
  - AC-PLATFORM-APPRISE-RESCAN-001.3
  - AC-PLATFORM-APPRISE-RESCAN-001.4
  - AC-PLATFORM-APPRISE-RESCAN-001.5
system_design:
  - ../../specs/platform/system-design/apprise-rescan.md
---

# Task 01: Add Apprise Rescan

## Summary

Add the Notifications settings rescan action using the existing provider-list
API. Apply only availability updates and preserve editable settings.

## In scope

- Domain hook, narrow store setter, availability hydration, and control wiring.
- Loading, result, error/retry, draft preservation, and translations.
- Targeted tests, mobile geometry, and a public Notifications how-to section.

## Out of scope

Installation, new API contracts, automatic polling, sending notifications,
schema changes, and broad settings refactoring.

## Acceptance

1. The labeled action updates detection in both directions; duplicate calls
   are blocked and a failed request retains status until a successful retry.
2. Dirty settings and open forms survive every result; a delayed rescan cannot
   overwrite provider data saved while it was pending.
3. Desktop and mobile flows pass targeted tests, all copy is localized, and the
   public instructions explain the backend environment boundary.

## Verification

Use TDD for changed logic. From `apps/`, install once before package commands:

```bash
rtk pnpm install --frozen-lockfile
```

From `apps/web/`:

```bash
rtk pnpm exec vitest run hooks/domains/settings/use-notification-providers.test.tsx components/settings/notifications-settings-actions.test.tsx components/settings/notifications-settings.test.tsx
rtk pnpm run typecheck
rtk pnpm exec eslint hooks/domains/settings/use-notification-providers.ts components/settings/notifications-settings-actions.ts components/settings/notifications-settings.tsx lib/state/slices/settings/settings-slice.ts lib/state/slices/settings/types.ts
rtk pnpm run i18n:zh-hant
rtk pnpm run i18n:check
rtk pnpm e2e:run --project chromium tests/settings/apprise-rescan.spec.ts
rtk pnpm e2e:run --project mobile-chrome tests/settings/mobile-apprise-rescan.spec.ts
```

Include any extracted production component in the targeted lint command.
Use normal managed builds and inspect rendered desktop/mobile captures.
From the repository root:

```bash
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

## Files likely touched

- `apps/web/hooks/domains/settings/use-notification-providers.ts` and new test.
- `apps/web/lib/state/slices/settings/settings-slice.ts` and `types.ts`.
- `apps/web/components/settings/notifications-settings-actions.ts` and its test.
- `apps/web/components/settings/notifications-settings.tsx`, the extracted
  external-provider section, and its test.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/settings.json`.
- `apps/web/e2e/tests/settings/{apprise-rescan,mobile-apprise-rescan}.spec.ts`
  and a shared helper if needed.
- `docs/public/developer-tools.md`.

## Dependencies

None.

## Risks

See the plan. Keep create-form input mounted when availability becomes false.
Use the narrow store setter so captured provider arrays cannot revert a save.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/apprise-rescan.md).
- [System design](../../specs/platform/system-design/apprise-rescan.md).
- Existing `notification-permission-section.tsx` refresh control.
- Existing `mobile-notification-events.spec.ts` for settings fixture and geometry.
- Repository `/tdd`, `/mobile-parity`, `/e2e`, and `/docs-maintainer` guidance.

## Results

Implemented with an availability-only rescan path and localized pending,
result, and retry feedback. The store setter updates only Apprise availability,
so provider drafts, removals, open forms, and concurrent provider saves are not
replaced.

Verification completed:

- `pnpm install --frozen-lockfile` completed from `apps/`.
- Targeted Vitest passed: 4 files, 37 tests.
- Typecheck passed.
- Targeted ESLint passed with `--max-warnings 0`.
- Traditional Chinese generation and the complete i18n gate passed.
- Chromium and mobile-chrome managed E2E tests passed, including desktop and
  mobile geometry/overflow assertions and fresh PR captures.
- Public-doc test suite passed (61 tests), public-doc validation passed (46
  pages), specification lint passed, and `git diff --check` passed.
