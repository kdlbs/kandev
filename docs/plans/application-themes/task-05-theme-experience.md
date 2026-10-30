---
id: "05-theme-experience"
title: "Appearance, contributor guide and complete flows"
status: done
wave: 5
depends_on:
  - "04-theme-marketplace"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-THEMES-001
  - REQ-PLUGINS-THEMES-002
  - REQ-PLUGINS-THEMES-003
  - REQ-UI-APP-THEMES-001
  - REQ-UI-APP-THEMES-002
acceptance_criteria:
  - AC-PLUGINS-THEMES-001.1
  - AC-PLUGINS-THEMES-001.4
  - AC-PLUGINS-THEMES-002.1
  - AC-PLUGINS-THEMES-002.2
  - AC-PLUGINS-THEMES-002.3
  - AC-PLUGINS-THEMES-002.4
  - AC-PLUGINS-THEMES-003.1
  - AC-PLUGINS-THEMES-003.2
  - AC-PLUGINS-THEMES-003.3
  - AC-PLUGINS-THEMES-003.4
  - AC-PLUGINS-THEMES-003.5
  - AC-UI-APP-THEMES-001.1
  - AC-UI-APP-THEMES-001.2
  - AC-UI-APP-THEMES-001.3
  - AC-UI-APP-THEMES-001.4
  - AC-UI-APP-THEMES-001.5
  - AC-UI-APP-THEMES-002.1
  - AC-UI-APP-THEMES-002.2
  - AC-UI-APP-THEMES-002.3
  - AC-UI-APP-THEMES-002.4
  - AC-UI-APP-THEMES-002.5
system_design:
  - ../../specs/plugins/system-design/theme-packages.md
  - ../../specs/ui/system-design/application-themes.md
---

# Task 05: Appearance, contributor guide and complete flows

## Summary

Complete the user-facing system with generic installed theme selection and
recovery, prove interoperability and retained work across all renderers, and
publish the format/import/contribution instructions alongside the implementation.

## In scope

Appearance dynamic grouped choices, import/browse entry points, duplicate label
handling, unavailable/refetch states, preview/save/reset/discard/revision semantics,
quick toggles, locale coverage; complete desktop/mobile flow matrix and screenshots.
Write a concise appearance how-to and theme-authoring reference using real schema/
commands. Add a minimal two-variant sample to test fixtures, not production catalog.
Guide authors through their own repository/release and existing registry PR;
explain license/attribution and VS Code compatibility limits. Mark delivery records
accurately and promote specs only when implementations match every contract.
Deliver the seeded running instance described in the plan's final handoff gate:
implemented task build, sideloaded Monokai-style theme, representative editor/diff
content, explicit Tailscale-IP binding and durable process/log ownership.

## Out of scope

External theme repositories/releases/PRs, marketplace entry publication, a built-in
Monokai feature, personal install libraries and unrelated UI redesign.

## Acceptance

1. A newly authored fixture pack works without theme-name-specific code;
   installed and imported variants can be previewed/saved/restored on desktop
   and phone, and built-ins recover from stale/disabled/removed/corrupt themes
   without destroying the saved reference or unsaved editing state.
2. Every renderer and lifecycle scenario in the plan has targeted executable and
   rendered evidence, including actual worker/canvas updates, late-result fences,
   storage failures, undo/selection, terminal continuity and phone containment.
3. Author/operator documentation, schema links, a reproducible two-variant sample,
   native pack commands, VS Code mapping report and marketplace contribution
   instructions agree with shipped behavior and pass documentation validators.
   Final delivery also leaves the isolated seeded instance running at a verified
   Tailscale URL, with the imported theme saved in the verification browser and
   selection instructions for the user's fresh browser.

## ASCII UI preview

UI-01 and UI-04 from [the plan](plan.md#ascii-ui-preview):

```text
Appearance: [Package / Variant v] [Import theme] [Browse themes]
             Compatibility or unavailable status / Retry
             Unsaved changes [Reset] [Save changes]
Desktop task: Sidebar | Toolbar | File editor | Diff | Terminal / Chat
Phone task:   Header -> Current content -> Existing bottom navigation
```

Keep existing settings and task scroll owners. No viewport fallback writes to
storage. Phone controls retain >=44px actual targets and safe-area clearance;
keyboard and focus return remain functional. Full state/AC mapping is in UI-01/UI-04.

## Verification

Run from repository root with Node 24. E2E runner rebuilds the production frontend
and backend before the desktop run; suites run serially.

```bash
(cd apps/web && pnpm exec vitest run components/settings/application-themes.test.tsx components/settings/appearance-settings-state.test.ts components/theme-toggle.test.tsx components/theme/sonner-theme.test.tsx lib/theme hooks/domains/plugins/use-theme-inventory.test.ts lib/state/slices/plugins)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec eslint components/settings/general-settings.tsx components/theme lib/theme components/settings/plugins components/editors components/diff/use-diff-options.tsx components/task/use-terminal-theme.ts)
(cd apps/web && pnpm e2e:run --project chromium -- tests/settings/application-themes.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- tests/settings/mobile-application-themes.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Also run the published packaging/validation command against the sample using the
real backend pack/verify tools and record exact paths/output. Do not publish it.
Task 04 owns marketplace/import suites; rerun those only if final integration
changes their behavior. Remove --no-build if sources changed. Screenshots and
computed style/token assertions are required evidence; a passed prop mock alone
is insufficient. Report any unrun command and its actual blocker.

After these checks, execute the plan's final running-instance handoff. Record the
actual launch/seed/import commands, build identity, theme provenance/digest,
listener inspection, health/page/WebSocket checks and reload evidence in Results.
Discover the address with `tailscale ip -4`; verify the selected address belongs
to this host before binding. Keep data and processes separate from existing user
instances, and leave the delivered process running. Provide the exact URL, sample
task route, browser selection step, logs and stop/cleanup commands. Do not mark
this delivery gate complete based only on an automated E2E server that tears down.

## Files likely touched

- `apps/web/components/settings/general-settings.tsx`, appearance state tests,
  new application-themes.test.tsx, theme toggle/command tests.
- `apps/web/e2e/tests/settings/{application-themes,mobile-application-themes}.spec.ts`
  and scoped disposable theme/rendering helpers.
- `apps/web/src/locales/` all supported host-copy catalogs, pseudo and generated
  Traditional Chinese using repository scripts.
- `docs/public/appearance.md`, `themes-authoring.md` (new), meta.json;
  `plugins.md`, `plugins-authoring.md`, `plugins-manifest.md` for theme-profile
  exception links; `plugin-registry/README.md`.
- Backend theme fixture sample and CLI integration test if needed for documented commands.
- This plan/work orders and paired specs for final results/lifecycle promotion.

## Dependencies

Task 04 supplies marketplace/import destinations; Task 03 supplies generic rendering;
Task 02 owns current eligibility. Read public-doc README, /docs-maintainer, /e2e,
/mobile-parity and the localization guide before their respective implementation.

## Risks

Mock-only evidence, undocumented conversion limitations, dynamic names treated as
translation keys, a missing recovery route, and accidental publication of test data.

## Parallelism

`sequential`

## Inputs

Every plan criterion/scenario, paired designs, ADR, real Appearance/coordinator
patterns and registry authoring procedures. No scope remains from the rejected
built-in-Monokai-only package.

## Results

Implemented Appearance selection/recovery and import/browse entry points,
localized all new copy, and added user/contributor documentation with a shared
MIT-licensed two-variant fixture. Desktop and mobile production-build E2Es passed;
mobile checks cover touch target height, viewport containment and overflow.

The disposable seeded instance is running at http://100.122.240.70:36693 with the
Monokai study package sideloaded through browser review. Health, WebSocket traffic,
saved custom-theme reload, actual viewer colors and restart restoration passed.
Source, license, digest, isolation, logs and stop/restart commands are recorded
in [delivery evidence](delivery.md). No marketplace publication was performed.
