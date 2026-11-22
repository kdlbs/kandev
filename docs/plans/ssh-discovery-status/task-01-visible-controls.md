---
id: "01-visible-controls"
title: "Make SSH navigation, creation and verification visible"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-001.5
  - AC-EXECUTORS-SSH-CONNECTIONS-004.6
  - AC-EXECUTORS-SSH-CONNECTIONS-004.8
  - AC-EXECUTORS-SSH-CONNECTIONS-004.11
  - AC-EXECUTORS-SSH-CONNECTIONS-004.12
  - AC-EXECUTORS-SSH-CONNECTIONS-004.13
  - AC-EXECUTORS-SSH-CONNECTIONS-004.14
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
---

# Task 01: Visible SSH controls

## Summary

Replace both far-right control clusters with content-aligned navigation and
creation. Show current verified status as green in rows and saved headers.

## In scope

Hub navigation card, manager toolbar, local shared status badge, existing
keyboard/focus/gating/state behavior, localized copy reuse, focused tests and
public navigation wording. Preserve the completed prior package results.

## Out of scope

Backend/readiness changes, global UI primitives, new navigation categories,
remote SSH changes, lab refresh, commit/push and PR/CI/review work.

## Acceptance

1. UI-01: enabled administrators find one full-width SSH destination before
   profiles, including zero profiles; member/disabled gates remain intact.
2. UI-02: tabs and New are below the title and aligned with content on desktop
   and phone, with the proposed phone action row, keyboard/focus and all saved
   navigation/continuation behavior preserved.
3. UI-03: current ready badges are green and readable in both themes at both
   sites; failed/stale/uncertain states are non-success. Required rendered,
   localization and docs checks pass without incidental remote operations.

## UI preview

[Combined previews and mobile contract](plan.md#ui-previews).

| View | Desktop | Phone |
| --- | --- | --- |
| UI-01 | Title; full-width labelled SSH card; profile sections | Same order; entire card tappable |
| UI-02 | Title; leading tabs with adjacent New; list | Title; tabs; full-width New; list |
| UI-03 | Green ready badge with text; neutral non-ready states | Same semantics, wrapping text and reachable controls |

Preview maps to the ACs above. Loading/error retains the toolbar; busy state
retains the current disabled behavior. Content order/alignment is required;
exact illustrative spacing is not a replacement for shared tokens.

## Verification

Use TDD for changed rendering/state boundaries. Component markup is exempt
from artificial unit coverage; prove geometry through the browser instead.
From repo root, with workspace dependencies already installed:

```bash
pnpm --dir apps/web exec vitest run app/settings/executors/page.test.tsx components/settings/ssh-connections
pnpm --dir apps/web run typecheck
pnpm --dir apps/web exec eslint --max-warnings 0 app/settings/executors/page.tsx app/settings/executors/page.test.tsx components/settings/ssh-connections e2e/helpers/ssh-navigation.ts e2e/helpers/ssh-management-layout.ts e2e/tests/settings/ssh-management-layout.spec.ts e2e/tests/settings/mobile-ssh-management-layout.spec.ts
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web run i18n:ratchet
pnpm --dir apps/web e2e:run --host --project=chromium tests/settings/ssh-navigation.spec.ts tests/settings/ssh-management-layout.spec.ts tests/settings/ssh-connections.spec.ts tests/settings/ssh-access-setup.spec.ts tests/settings/ssh-connections-disabled.spec.ts
pnpm --dir apps/web e2e:run --host --project=mobile-chrome tests/settings/mobile-ssh-navigation.spec.ts tests/settings/mobile-ssh-management-layout.spec.ts tests/settings/mobile-ssh-connections.spec.ts tests/settings/mobile-ssh-access-setup.spec.ts
pnpm --dir apps/web e2e:run --host --project=auth tests/auth/ssh-connections-permissions.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run projects sequentially with guarded default worker limits. Use isolated
fixtures and fresh builds, never the manual lab or main instance. Run changed-
file Prettier and lint for any additional helpers/specs introduced. Re-run
local `validateCoverage` in `.github/scripts/pr-docs.cjs` with the complete
changed work order, plan and referenced requirement/design content. Record
actual commands, counts, screenshots, light/dark contrast, and any failures.
No container scenarios are required: the transport and runtime are unchanged.

## Files likely touched

Relative to `apps/web/` unless prefixed `docs/`:

- `app/settings/executors/page.tsx`, `page.test.tsx`.
- `components/settings/ssh-connections/manager-header.tsx`,
  `connection-collection.tsx`, new `status-badge.tsx` and focused badge tests.
- Existing `connection-presentation.test.ts`, `access-flow.test.tsx` as needed.
- `e2e/helpers/{ssh-navigation,ssh-management-layout}.ts` and their specs above.
- Existing context/access specs when their shared helpers change.
- Locales only if existing localized title/description/actions cannot be reused.
- `docs/public/executors.md`, this package and its paired artifacts.

## Dependencies

Committed prior UX package at `106018b68ca6`; no pending work-order dependency.

## Risks

Accidental green for stale evidence; clipped phone actions; nested interactive
card elements; navigation geometry tests tied to the previous layout; changes
to global headers; losing saved draft or secret-cleanup behavior.

## Parallelism

`sequential`

## Inputs

User screenshots and explicit green-badge request on 2026-10-28, linked owning
requirements/design, and the completed prior package. Use better-ui,
mobile-parity, TDD/e2e and docs-maintainer during implementation.

## Results

Completed locally on 2026-10-28. The Executors entry is a full-width semantic
Link before Profiles, using existing title/help translations and the original
permission/feature gates. Manager tabs and creation now occupy a leading content
toolbar, with a full-width phone create action. Shared `ConnectionStatusBadge`
uses existing `connectionState` and shows green only for current ready state.
The local dark background override avoids the shared outline badge's neutral
dark fill. Long host strings still wrap; ordinary caption words no longer split
across lines. Global UI primitives, data hooks and verification semantics did
not change. Public executor instructions now name the card and toolbar.

RED: the 767px navigation probe already passed with the old phone layout. The
768px desktop entry and ready-badge cases then failed before production edits.
The first green attempt passed two cases but sampled a fresh-failure badge
midway through its existing color transition. The assertion now polls rendered
color until the transition settles, without adding sleeps or changing product
state. The four-case retry passed. Markup is covered through actual rendered
geometry, colors and interactions; no artificial component DOM unit test was
added for the small presentation component.

Verification completed:

- Listed focused Vitest command: 48 tests in 10 files passed. Existing state
  tests cover exact route stamps, failed fresh probes and uncertain enrollment.
- Full listed Chromium set: 21 passed. Full mobile set: 13 passed. Auth: one
  administrator/member API and UI case passed. These are 35 distinct final
  browser cases. After explicit dark-fill and caption polish, one additional
  desktop palette case and one phone palette/wrapping case passed on fresh
  assets. No real SSH/container runtime claim is made for this UI-only package.
- Tests cover 390px, 767/768px, 820px coarse tablet and 1280px, whole-card touch,
  desktop 28px/phone-coarse 44px actions, create/edit/save, cancellation, focus,
  guarded return, failed reads, empty/missing states and translated overflow.
- Both row and detail badges were measured in light/dark on desktop and phone.
  Computed text contrast: 6.31:1 in light mode; 10.14:1 for dark list badges and
  10.97:1 for dark detail badges. All exceed 4.5:1. Stale, unchecked, incomplete,
  unavailable, untrusted and uncertain fixtures remain non-success. A rejected
  fresh probe changes both header and row to non-green Needs attention.
- Web typecheck/catalog, changed-file Prettier and ESLint (including the new
  `e2e/helpers/ssh-verified-status.ts`), complete i18n and new-code ratchet passed.
  No new user-facing strings or locale keys were needed.
- Public-doc checks: 63 validator tests, 54 pages passed. Specification checks:
  36 tests, catalog validation and full lint passed. Delivery-reference preflight,
  package links and tracked/untracked whitespace checks passed.

Receipt files: `/tmp/ssh-discovery-{red,green-first,green-retry,unit,type-final,lint-final,format-final,i18n,ratchet,desktop,phone,desktop-palette-final,phone-final-wrap,auth}.log`;
`/tmp/ssh-discovery-{public-tests,public-docs,catalog,spec-tests,spec-lint}.log`;
`/tmp/ssh-discovery-delivery-preflight.json`. The green-first log records the
transition-timing test failure, not a successful final run.
Detailed final browser logs:
`/tmp/kandev-run.e2e.{8OGJddWu,qgm48dyz,puH5uBr8,4wNEZlQz}.log`.
Screenshots were visually inspected and retained under
`/tmp/ssh-discovery-evidence/{desktop,phone-final,desktop-final-palette}/`.
The latter two include `ssh-status-contrast.json` measurements.

Work order done, plan implemented, requirements active and focused design current.
Same primary session; no delegation or model switch. Changes are uncommitted.
No push, PR/CI/review work, running-instance refresh or live SSH changes.
Main port 9998 remains PID 1146; the manual lab remains PID 456598 and serves
the earlier commit `106018b68ca6`. Neither runtime was modified.
