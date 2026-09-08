---
id: "02-profile-actions-e2e"
title: "Profile action responsive E2E coverage"
status: done
wave: 2
depends_on: ["01-profile-row-actions"]
plan: "plan.md"
spec: "../../specs/agents/requirements/settings-profile-layout.md"
---

# Task 02: Profile action responsive E2E coverage

## Acceptance

- Desktop E2E proves that a full desktop profile row exposes Duplicate and
  Delete inline, shows both action tooltips on hover, and duplicates the seeded
  profile through the direct button.
- Tablet-width E2E proves that inline controls are absent, the overflow menu
  contains both actions, and the document remains horizontally contained.
- Mobile E2E keeps the touch-based overflow path, proves inline controls are
  absent, and preserves the existing duplicate outcome and 44px geometry.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-profile-duplicate.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-profile-duplicate.spec.ts)
```

The managed runner must rebuild the production Vite assets before the tests.
Run headless only. Record the exact test counts and any failure-artifact or
cleanup results below.

## Files likely touched

- `apps/web/e2e/tests/settings/agent-profile-duplicate.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-profile-duplicate.spec.ts`

## Dependencies

Task 01 must land first because the E2E selectors and responsive branches
depend on the final profile-row markup.

## Parallelism

Sequential. Desktop, tablet, and mobile assertions share the same responsive
action contract and managed fixture state.

## Inputs

- Spec: `docs/specs/agents/requirements/settings-profile-layout.md`, responsive action
  scenarios.
- Plan: `plan.md`, E2E Tests and Mobile design contract sections.
- Existing patterns: `agent-profile-duplicate.spec.ts`,
  `mobile-agent-profile-duplicate.spec.ts`, and the `tabletTestPage` fixture.

## Output contract

Report changed test files, exact managed-runner commands and test counts,
screenshots or failure artifacts if produced, cleanup evidence, and
synchronized task/plan status.

## Results

- `cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-profile-duplicate.spec.ts` passed (3 tests).
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-profile-duplicate.spec.ts` passed (1 test).
- Final desktop/tablet selector assertion rerun with `cd apps/web && pnpm e2e:run --no-build --project chromium tests/settings/agent-profile-duplicate.spec.ts` passed (3 tests).

The managed runner rebuilt the backend and pseudo-locale Vite assets for both
runs, started from cleaned E2E artifact directories, and left no failure
artifacts. Desktop coverage proves the compact icon-only inline path, the 900px
tablet fixture proves the overflow path and document containment, and the
mobile coverage keeps the touch menu and 44px trigger check.

The desktop flow also verifies that hovering the icon-only Duplicate and Delete
buttons exposes their matching translated action names.

Three fresh synthetic PR screenshots were captured and visually inspected for
full desktop, tablet, and mobile states. They were compressed, validated
against `.pr-assets/manifest.json`, and the disposable capture spec was
removed. The ignored asset files are ready for PR media publication.
