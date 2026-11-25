---
id: "04-sidebar-entry"
title: "Direct SSH manager entry under Executors"
status: done
wave: 4
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-004.4
  - AC-EXECUTORS-SSH-CONNECTIONS-004.6
  - AC-EXECUTORS-SSH-CONNECTIONS-004.8
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
---

# Task 04: Direct SSH manager entry under Executors

## Summary

Add SSH connections as the first direct child of Executors in tree navigation.
Open the existing manager directly and reuse the same entry on phone Settings.

## In scope

Static branch node, key icon, translated label, feature/admin gate, active-route
selection, zero-profile visibility, existing count semantics and navigation modes.
Keep the Executors page card. Update public navigation guidance when implemented.

## Out of scope

A new top-level settings section, SSH record children/counts, global sidebar
restyling, flat-mode changes, backend fetches, and live instance deployment.

## Acceptance

1. Eligible users see SSH connections first under Executors in accordion and
   persistent modes, even without profiles. Clicking/tapping opens the canonical
   manager. No chevron, profile-count increment, or extra data request is added.
2. The SSH leaf is selected on all manager views and direct links reveal its
   parent through existing expansion behavior. Flag/role changes remove it.
   Other executor/profile routes, ordering, counts and flat mode stay intact.
3. Desktop keyboard and phone navigation work with the shared SettingsTree.
   Existing touch targets, focus, translated text and no-overflow behavior pass.

## UI preview

[UI-04 full preview](plan.md#ui-04-direct-sidebar-entry). Structured preview
honors the task's no-ASCII instruction. Covers AC-004.4/6/8.

| Position | Content | Interaction |
| --- | --- | --- |
| Parent | Executors, unchanged count | Existing expansion/navigation |
| First child | Key icon, SSH connections | One direct link, active throughout manager |
| Other children | Existing executor rows | Existing order and expandable profiles |

Phone uses the existing Settings index tree and direct page navigation. It does
not gain a persistent sidebar or an extra action sheet. Flat mode keeps the
Executors page card as its discovery path.

## Verification

Use TDD in `settings-menu-branches.test.ts`, `use-settings-menu-branches.test.ts`
and `settings-tree-render.test.tsx`. Cases: gated static node first, zero profiles,
unchanged count/order, no SSH API fetch, active manager identity/connection paths,
flag/admin changes, both tree modes and unchanged flat mode.
Extend existing SSH navigation helpers/specs for keyboard desktop and phone tap.
Use semantic navigation scoping: the page card and sidebar now share a label,
so a global `getByRole(link, name: SSH connections)` can become ambiguous.
Retain breadcrumb, search, redirect and page-card assertions.

Run from repo root with fresh builds and isolated browser fixtures:

```bash
(cd apps/web && pnpm exec vitest run components/app-sidebar/sections/settings/settings-menu-branches.test.ts components/app-sidebar/sections/settings/use-settings-menu-branches.test.ts components/app-sidebar/sections/settings/settings-tree-render.test.tsx lib/settings-discovery/catalog.test.ts lib/settings/ssh-connection-settings-routes.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/app-sidebar/sections/settings/settings-menu-branches.ts components/app-sidebar/sections/settings/use-settings-menu-branches.ts e2e/helpers/ssh-navigation.ts e2e/tests/settings/ssh-navigation.spec.ts e2e/tests/settings/mobile-ssh-navigation.spec.ts)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
make build-backend build-web
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/ssh-navigation.spec.ts tests/settings/ssh-management-layout.spec.ts
pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/settings/mobile-ssh-navigation.spec.ts tests/settings/mobile-ssh-management-layout.spec.ts
pnpm --dir apps/web e2e:run --host --project auth tests/auth/ssh-connections-permissions.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Inspect desktop/phone screenshots, including active manager and empty executors.
Measure touch hit areas and check long labels/pseudo locale. Run other helper
consumers if selectors change. Record commands and actual results, not old counts.

## Files likely touched

Under `apps/web/`: `components/app-sidebar/sections/settings/settings-menu-branches.ts`,
`use-settings-menu-branches.ts`, their tests, `settings-tree-render.test.tsx`;
existing SSH navigation/layout E2E helpers and specs; optional settings-tree
wiring only if required. Reuse current locale keys and canonical route constants.
Also `docs/public/executors.md`, this package and its linked specifications.

## Dependencies

No hard dependency. Execute after Tasks 01-03 in this package's sequential queue.
Do not change those tasks' pending status while implementing this one.

## Risks

Duplicate accessible link names between sidebar and page content; hardcoding
routes; incrementing executor profile count; missing flag/admin memo dependencies;
changing the user's chosen menu mode to force discovery.

## Parallelism

`sequential`

## Inputs

User sidebar screenshot and explicit request, AC-004.6, owning interaction
design Navigation compatibility, existing branch builder and mobile SettingsTree.
Apply mobile-parity, better-ui, TDD/e2e and docs-maintainer during implementation.

## Results

Implemented a leading key-icon SSH connections leaf through the shared settings tree. Feature/admin changes are reactive; empty record lists still expose the eligible leaf. Profile counts and flat mode remain unchanged. Builder, hook, rendered-tree, desktop/phone navigation and member-permission checks passed. Inspected the active sidebar entry and phone settings navigation. See [verification](verification.md).
