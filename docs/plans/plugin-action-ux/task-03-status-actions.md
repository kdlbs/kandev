---
id: "03-status-actions"
title: "Status action presentations"
status: done
wave: 3
depends_on:
  - "02-topbar-sidebar"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ACTION-UX-001
  - REQ-PLUGINS-ACTION-UX-002
  - REQ-PLUGINS-ACTION-UX-003
acceptance_criteria:
  - AC-PLUGINS-ACTION-UX-001.1
  - AC-PLUGINS-ACTION-UX-001.3
  - AC-PLUGINS-ACTION-UX-001.5
  - AC-PLUGINS-ACTION-UX-002.1
  - AC-PLUGINS-ACTION-UX-002.2
  - AC-PLUGINS-ACTION-UX-002.3
  - AC-PLUGINS-ACTION-UX-002.4
  - AC-PLUGINS-ACTION-UX-002.6
  - AC-PLUGINS-ACTION-UX-002.7
  - AC-PLUGINS-ACTION-UX-003.1
  - AC-PLUGINS-ACTION-UX-003.3
  - AC-PLUGINS-ACTION-UX-003.5
system_design:
  - ../../specs/plugins/system-design/plugin-action-ux.md
---

# Task 03: Status action presentations

## Summary

Provide compact status actions and phone drawer rows through the same public Action.
Preserve the existing visibility, density, and saved-order contracts.

## In scope

- Supply status-bar or status-drawer context from AppStatusBarPluginContribution.
- Share status styling with the native LSP status trigger without changing its data or disclosure behavior.
- Preserve orderingId, drag behavior, registration identity, and phone left-then-right order.
- Add labelled status, busy, and disabled fixture states.
- Cover mouse bar interaction, compact tablet layout, phone row activation, drawer scroll and focus return.

## Out of scope

Changing status visibility defaults, bar height, tablet routing, metric data, persistence schema, or raw legacy status widgets.

## Acceptance

1. New status controls fit the existing 24px bar and use 44px phone rows with shared native styles.
2. Reordering, reload, disable/re-enable, and presentation changes preserve the existing contribution identity and saved order.
3. Legacy status content and native LSP details retain their existing behavior.

## ASCII UI preview

UI-01 / UI-02 / UI-03: Status presentations. See the [combined previews](plan.md#ascii-ui-preview).
The frontmatter criteria apply to this excerpt. Labels and glyphs are illustrative.

```text
Non-phone 24px bar: connected | plugin 63%          LSP
Phone Status drawer:
  Status                   fixed header
  [icon] Plugin 63%        44px or taller row
  [icon] Other status      shared scrolling body
```

Keep saved left-then-right ordering, existing visibility preferences, focus return, and compact tablet geometry.

## Verification

Run from the repository root. Use TDD for new behavior and record the initial
behavioral failure. New test paths in this order are files to create.
For a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run components/app-status-bar/app-status-bar-plugin-slots.test.tsx components/app-status-bar/app-status-bar-order.test.ts components/app-status-bar/app-status-drawer.test.tsx components/app-status-bar/lsp-status-item.test.tsx components/plugins/plugin-action.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/plugins/plugin-action-ux.spec.ts --grep status)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/plugins/mobile-plugin-action-ux.spec.ts --grep status)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/plugins/mobile-status-drawer.spec.ts)
```

Run any other test file changed by this work order with the same targeted runner.
For rendered changes, inspect the focused desktop and phone screenshots against
the assigned previews. Do not infer CSS geometry from unit tests.

## Files likely touched

- `apps/web/components/app-status-bar/{app-status-bar-plugin-slots.tsx,app-status-bar-plugin-slots.test.tsx,lsp-status-item.tsx,lsp-status-item.test.tsx}`
- `apps/web/components/app-status-bar/{app-status-bar-order.test.ts,app-status-drawer.test.tsx}`
- Shared surface modules and action tests from Task 01
- Test fixture bundle and both action UX E2E files

## Dependencies

02-topbar-sidebar.

## Risks

Changing ordering IDs loses saved placement. A 44px control inside the 24px tablet bar is invalid. Retain the documented compact exception.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/plugin-action-ux.md)
- [System design](../../specs/plugins/system-design/plugin-action-ux.md)
- [Plan and source audit](plan.md)
- Existing plugin-slot, SDK consumer, and packaged plugin E2E patterns.

## Results

Done. Status slot context distinguishes inline bar and phone drawer without
changing slot identity. The native LSP trigger uses the shared compact action
shell while retaining its disclosure. Packaged fixture Actions cover value,
toggle, busy, disabled, and legacy status content.

Validation passed: focused status/plugin-action Vitest (5 files, 24 tests), web
typecheck, desktop status E2E (1 test), phone Action E2E (1 test), and the full
mobile Status drawer E2E (2 tests). The first compact tablet browser check
exposed a 44px height inherited from the default Button on coarse pointers.
The status-bar Action now selects the compact Button variant, which removes the
default variant's coarse-pointer 44px rule. The captured desktop and phone
screenshots were inspected and removed.

Review remediation (2026-09-25): browser checks now measure the SVG itself at
12px in the inline status bar and 16px in the Status drawer. The mobile status
suite passed again as part of the 5/5 mobile action/status run. The permanent
container-specific screenshot path was removed, and the neighboring drawer
spec now restores and verifies the complete prior system-metrics setting.
Follow-up CI reproduced the compact-bar sizing issue; after selecting the
compact variant, the managed mobile Status drawer spec passed 2/2 tests.

PR review remediation (2026-09-25): inline status groups now cap at 18rem and
keep all actions on the 24px row. Flex children can shrink so long values
truncate within the group rather than expanding into adjacent status items.
The style regression passed in the combined renderer suite (3 files, 19
tests):
`(cd apps/web && pnpm exec vitest run components/plugins/plugin-action.test.tsx components/kanban/main-top-bar-plugin-actions.test.tsx components/actions/surface-action-styles.test.ts)`.
Managed Chromium action UX E2E passed 3/3:
`(cd apps/web && pnpm e2e:run --project chromium --workers=1 --retries=0 e2e/tests/plugins/plugin-action-ux.spec.ts)`.
Its status case injects long values into both controls and checks their bounds
remain on the 24px row.
