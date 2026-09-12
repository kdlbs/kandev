---
id: "02-bottom-filter-hosts"
title: "Bottom-hosted saved-query filters"
status: done
wave: 2
depends_on:
  - "01-compact-drawer-hosts"
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-CONFIRMATION-001
  - REQ-UI-MOBILE-CONFIRMATION-002
  - REQ-UI-MOBILE-CONFIRMATION-003
acceptance_criteria:
  - AC-UI-MOBILE-CONFIRMATION-001.2
  - AC-UI-MOBILE-CONFIRMATION-001.4
  - AC-UI-MOBILE-CONFIRMATION-001.6
  - AC-UI-MOBILE-CONFIRMATION-001.8
  - AC-UI-MOBILE-CONFIRMATION-002.3
  - AC-UI-MOBILE-CONFIRMATION-002.4
  - AC-UI-MOBILE-CONFIRMATION-003.1
  - AC-UI-MOBILE-CONFIRMATION-003.6
system_design:
  - ../../specs/ui/system-design/mobile-action-confirmations.md
---

# Task 02: Bottom-Hosted Saved-Query Filters

## Summary

Move the GitHub and GitLab phone filter hosts from their existing right-hand
Sheet to an inset bottom Drawer. Use work order 01's compact confirmation mode
inside that host without changing saved-query semantics.

## In scope

- Change only the phone filter shell in both provider page clients.
- Preserve IDs, entry triggers, filtering/selection/default state, cancellation
  and each provider's current close-on-select/save/delete timing.
- Add actual bottom-direction and compact geometry assertions to their existing
  mobile saved-query flows; retain desktop/tablet compatibility.
- Update the mobile how-to with compact hosted behavior after implementation.

## Out of scope

Provider APIs, persistence, query semantics, broad dashboard navigation, plugin
surfaces, new translations, global Sheet styling and main :9998.

## Acceptance

1. On phones both filter panels open from the bottom and expose all current
   choices through one viewport-contained scroller. Confirming saved-query
   deletion switches that same host to compact geometry, never another modal.
2. Cancel/Back restores filter state, scroll and focus. Explicit deletion uses
   the captured ID once and does not select the row or toggle its default.
3. Phone-boundary changes cancel pending decisions; non-phone layouts and saved
   preferences remain unchanged. The public phone how-to describes the shipped
   geometry accurately.

## Verification

Use `/tdd`, `/e2e` and `/docs-maintainer`. First assert bottom-drawer direction in
the existing GitLab saved-query test; current code must fail because it renders
a right-hand Sheet. Extend the equivalent GitHub saved-query flow and keep its
default-action isolation assertions. From `apps/web`:

```bash
pnpm run build:e2e
pnpm e2e:run --project mobile-chrome tests/gitlab/mobile-gitlab-parity.spec.ts --grep 'saved-query deletion'
pnpm e2e:run --project mobile-chrome tests/github/mobile-github-sidebar.spec.ts
pnpm e2e:run --project mobile-chrome tests/gitlab/mobile-gitlab-parity.spec.ts --grep 'saved-query deletion'
pnpm e2e:run --project chromium tests/github/github-scope-bar.spec.ts tests/gitlab/gitlab-issue-milestone-filter.spec.ts
pnpm exec vitest run components/confirmation/mobile-confirmation-host.test.tsx components/confirmation/mobile-action-confirmation.test.tsx
pnpm run typecheck
pnpm exec eslint --max-warnings 0 app/github/github-page-client.tsx app/gitlab/gitlab-page-client.tsx
```

The first GitLab command is RED before conversion; the later runs use a fresh
build after implementation. Inspect one provider's phone filter/confirmation
capture. From the repository root, validate documentation:

```bash
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

After checks, the existing isolated test instance may serve the rebuilt web
assets without restarting its backend. Preserve its data and Tailscale route;
do not reset the seed or touch main :9998. Report its refresh URL, not a claim
that the user's live main installation was updated.

## Files likely touched

- `apps/web/app/github/github-page-client.tsx`.
- `apps/web/app/gitlab/gitlab-page-client.tsx`.
- `apps/web/components/integrations/integration-filters-sheet.tsx` (shared shell).
- `apps/web/e2e/tests/github/mobile-github-sidebar.spec.ts`.
- `apps/web/e2e/tests/gitlab/mobile-gitlab-parity.spec.ts` and relevant existing
  desktop saved-query cases if needed for the phone-boundary check.
- `docs/public/mobile-remote-access.md` (existing how-to), plan/work-order results.

## Dependencies

Work order `01-compact-drawer-hosts` must be done.

## Risks

Replacing a primitive changes focus/close callbacks and scroll containment.
Keep the form/list owner stable and verify selection/default isolation. Do not
turn a formerly phone-only entry point into a desktop drawer.

## Parallelism

`sequential`

## Inputs

The design's Other inline consumers and Hosted content sections; both current
provider filter roots and saved-query mobile tests; the mobile UI language's
bottom-picker precedent and `apps/web/AGENTS.md`.

## Results

Initial implementation verified on 2026-09-11. RED reproduced the missing bottom direction on the
GitLab phone filter: the current root had `data-side="right"` and no Vaul
direction. Both providers now use a shared presentation-only
`IntegrationFiltersSheet`, with an existing Drawer on phones and the retained
right Sheet above the phone boundary. No query or mutation hooks moved.

The fresh frontend build and changed-file lint pass. All five provider mobile
cases passed, including scroll restoration and crossing 768px during a pending
confirmation. Desktop GitHub scope/saved-query checks and all three GitLab
milestone/saved-query checks passed.

A later combined run opened Save query instead of the GitHub delete step once
during sheet entry. Its geometry/tap check now settles the existing finite
entry animation first; no fixed delay or retry was added. The final run also
includes work order 01's outer-scroll containment correction and stronger
heading/action visibility assertions. All 16 targeted mobile cases passed on
the final build, followed by four clean desktop confirmation/persistence checks
through the guarded raw runner. Full results and actual bounds are in the
parent plan. The existing mobile how-to was updated, and its validators pass.

PR integration with the newer base moves GitHub hosting to its extracted
`MobileViewsPicker`, preserving its fixed save action and delayed focus handoff.
GitLab retains `IntegrationFiltersSheet`. The full seven-case mobile GitHub
sidebar spec passes after integration, including save retry, kind switching,
saved defaults, compact deletion and Cancel restoration. Current combined
verification is recorded in the parent plan.
