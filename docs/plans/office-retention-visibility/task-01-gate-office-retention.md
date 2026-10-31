---
id: "01-gate-office-retention"
title: "Gate Office retention surfaces"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.2
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.4
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.5
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.6
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.7
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.8
system_design:
  - ../../specs/system-page/system-design/system-data-storage-pages.md
---

# Task 01: Gate Office retention surfaces

## Summary

Apply the effective Office feature value to Storage and settings discovery.
Preserve Host access through direct and legacy retention links when Office is disabled.

## In scope

- Gate retention tabs, target mapping, panel mounting, and discovery.
- Add failing regression tests before implementation, then make the smallest correction.
- Verify desktop and phone behavior and document availability in the operations guide.

## Out of scope

Backend retention policy, flag rollout/defaults, per-agent settings, and shared tab styling.

## Acceptance

1. Disabled Office exposes Host through all listed links without mounting retention or starting retention requests.
2. Enabled Office retains its current selection, discovery, permissions, and draft behavior.
3. Reactive availability changes and phone navigation follow the same rule without blank content or horizontal overflow.

## UI preview

UI-01: Storage with Office disabled, matching the [full preview](plan.md#ui-preview).
Desktop shows Host in the existing header tab position.
Phone shows Host below the description, with the existing touch target.
Both display Host content and preserve the existing page scroll owner and primary actions.
With Office enabled, both display Host and Office retention.
Maps to criteria 002.2/.6/.8. Spacing and visual tokens remain those of the shared components.

## Files likely touched

- `apps/web/components/settings/system/storage-settings.tsx` and new `storage-settings.test.tsx`.
- `apps/web/lib/settings-discovery/{types.ts,resolve.ts,catalog/system.ts,catalog.test.ts}`.
- `apps/web/hooks/domains/settings/use-settings-discovery.ts` and its existing test.
- `apps/web/e2e/tests/system/{settings-header-tabs.spec.ts,mobile-settings-header-tabs.spec.ts}` and `apps/web/e2e/helpers/storage-office-availability.ts`.
- `docs/public/operations.md` and this package's results/status records.

## Inputs

Read the referenced requirement and design, especially **Office availability**.
Read `apps/web/AGENTS.md` and use `/tdd`, `/e2e`, `/mobile-parity`, and `/docs-maintainer`.
Use `use-settings-tab.test.ts` for query resolution tests and the coordinator discovery cases for feature-gate patterns.
Read the [header-tab ADR](../../decisions/2026-09-15-settings-header-tabs.md).

## Verification

Run from the repository root. Install dependencies once if this worktree has no workspace install.
Run the new disabled regression first and record its expected failure before editing production code.
The managed E2E runner builds current sources. Run browser commands sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/settings/system/storage-settings.test.tsx hooks/domains/settings/use-settings-tab.test.ts hooks/domains/settings/use-settings-discovery.test.ts lib/settings-discovery/catalog.test.ts src/settings-route-helpers.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/settings/system/storage-settings.tsx components/settings/system/storage-settings.test.tsx hooks/domains/settings/use-settings-discovery.ts hooks/domains/settings/use-settings-discovery.test.ts lib/settings-discovery/types.ts lib/settings-discovery/resolve.ts lib/settings-discovery/catalog/system.ts lib/settings-discovery/catalog.test.ts e2e/helpers/storage-office-availability.ts e2e/tests/system/settings-header-tabs.spec.ts e2e/tests/system/mobile-settings-header-tabs.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium -- e2e/tests/system/settings-header-tabs.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- e2e/tests/system/mobile-settings-header-tabs.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Dependencies

None.

## Risks

Retained panels and fragment precedence can bypass incomplete visibility gates.
Use independent feature values in hook test mocks so auth does not accidentally enable Office.
An unavailable retention panel must unregister its save contributor when unmounted.

## Parallelism

`sequential`

## Results

Implemented on 2026-10-29. All acceptance conditions passed.

- RED: targeted Storage and discovery tests produced eight expected assertion failures before production edits.
- GREEN: the documented Vitest command passed 33 tests across five files.
- Additional resolver coverage: `(cd apps/web && pnpm exec vitest run lib/settings-discovery)` passed 49 tests across seven files.
- Typecheck passed with `NODE_OPTIONS=--max-old-space-size=8192`; the default 4 GiB heap was insufficient.
- The documented ESLint command, including the shared browser helper, passed without warnings.
- `pnpm run i18n:ratchet` passed.
- The desktop browser suite passed 11 tests. The phone suite passed seven tests.
- Both public-doc commands passed: 63 validator tests and 54 published pages.
- Both specification commands and `git diff --check` passed.

Browser setup needed process-local loopback and Nix library overrides.
The managed runner built current sources before final browser checks.
The final suites reused those exact artifacts with `--no-build` and ran sequentially.
See [plan validation](plan.md#verification-results) for the setup failures and screenshot handling.
The production change uses existing localization keys and shared tab styling.
No new UI-polish issue was found in the inspected disabled desktop/phone surfaces or shared tab states.
The operations guide now documents Office availability.
