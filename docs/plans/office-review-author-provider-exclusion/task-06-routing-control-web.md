---
id: "06-routing-control-web"
title: "Routing page control, three-part warning copy, and import submitting state"
status: done
wave: 5
depends_on:
  - "05-routing-control-backend"
plan: "plan.md"
requirements:
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-005
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-006
acceptance_criteria:
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.3
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.5
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.7
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-005.9
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.1
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.3
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.5
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-006.6
system_design:
  - ../../specs/office/system-design/review-author-provider-exclusion.md
---

# Task 06: Routing page control, three-part warning copy, and import submitting state

## Summary

Web half of system design §9 and §10: the **Review on a different provider**
section in the routing page draft, the rewritten warning copy with a
`surface`-specific fix sentence, and an import dialog that never shows an
empty review view while an auto-submitted import is in flight.

## Scope

- `useWorkspaceRouting`: `authorExclusionSteps`, `updateAuthorExclusion`.
- `AuthorExclusionStepsCard`; page draft and Save sequencing.
- `AuthorExclusionWarningList`: lead, problem, and fix sentences; `surface`.
- `ImportWorkflowsDialog`: review view only when resolving or submitting from
  review.
- Locales in all seven catalogs plus pseudo; no em dash.
- E2E: desktop routing control toggle with warning refresh; phone layout.

## Exclusions

Workflow-editor control (issue #5347). Per-agent override form warnings.

## UI-04 (from [plan](plan.md#ui-04-provider-routing-settings-review-on-a-different-provider))

```text
| Office Default workflow, Review step     [Reviewer]      ( o)   |
```

## Implementation acceptance

- Toggling a switch and saving updates the step and the banner; the banner
  and import list use the three-part copy on both surfaces.
- An import with nothing to review never renders the review view.

## Verification

```bash
cd apps/web
pnpm exec vitest run --maxWorkers=1 components/author-exclusion-warning-list.test.tsx \
  hooks/domains/office app/office/workspace/routing app/settings/workspace
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm run typecheck
pnpm e2e:run --host --shards 1 --project chromium tests/office/office-review-author-exclusion-setup.spec.ts
pnpm e2e:run --host --shards 1 --project mobile-chrome tests/office/mobile-review-author-exclusion-setup.spec.ts
```

## Likely files

- `apps/web/hooks/domains/office/use-workspace-routing.ts` (+ test)
- `apps/web/lib/api/domains/office-routing-api.ts`, `office-extended-api.ts`
- `apps/web/lib/types/author-exclusion.ts`
- `apps/web/app/office/workspace/routing/page.tsx`
- `apps/web/app/office/workspace/routing/components/author-exclusion-steps-card.tsx` (+ test)
- `apps/web/components/author-exclusion-warning-list.tsx` (+ test)
- `apps/web/app/settings/workspace/workspace-workflows-dialogs.tsx` (+ test)
- `apps/web/src/locales/*/{workflows,office}.json`
- `apps/web/e2e/tests/office/*review-author-exclusion-setup.spec.ts`

## Risks

- The routing page is near the component length limit; keep the new section
  and save sequencing in their own component and hook helper.

## Results

Implemented with TDD.

- `AuthorExclusionStepsCard` and `useAuthorExclusionDraft` keep the switches
  in the routing page draft; Save writes routing first, then the switches.
- `AuthorExclusionWarningList` renders a bold workflow-and-step lead, a
  problem sentence, and a fix sentence chosen by `surface` (`routing` on the
  routing page, `elsewhere` for import).
- The import dialog keeps the YAML view in its importing state when the
  preview has no steps, warnings, or conflicts, so an empty review view with
  disabled actions is never shown. The reported delayed dismissal after a
  confirmed import did not reproduce against the dev backend (imports
  completed within milliseconds in the backend log and in headless desktop
  and phone runs).
- Locales: all seven catalogs plus pseudo; `i18n:check` and `i18n:ratchet`
  pass.
- E2E: the desktop setup spec now toggles the switch off and on through Save
  and checks the stored step config and the banner; the phone spec checks the
  switch rows for horizontal overflow. Both pass, and the reviewer-exclusion
  spec still passes.
