---
spec: docs/specs/ui/requirements/pr-task-status-summary.md
created: 2026-08-06
status: implemented
---

# Implementation Plan: PR Task Status Summary

## Overview

Replace the shared task PR icon's pipe-delimited tooltip body with a small, localized summary
component. The component will keep existing status derivation and icon attributes authoritative,
then present each PR as a header plus separate review, CI, and merge/state rows. Focused component
tests and desktop/mobile Playwright coverage will prove readability without changing task-row
behavior or introducing a new touch surface.

## Frontend

### Structured PR summary

- Add `apps/web/components/github/pr-task-status-summary.tsx` as the presentation boundary for
  tooltip content. Its pure derivation accepts each `TaskPR` plus the already-derived
  ready-to-merge boolean, builds only rows backed by available source fields, and feeds one grouped
  rendered entry per PR.
- Separate `#<number>` and the PR title in a compact header. Use a fixed preferred width with a
  viewport-safe maximum, `text-pretty`, `break-words`, and a readable `leading-snug` treatment so a
  long title wraps without turning the disclosure into one dense paragraph.
- Render review, CI, and merge/terminal state in a two-column description layout. Each row has a
  translated category label, humanized known status, Tabler semantic icon, and restrained status
  tone. Reuse the status-copy and icon language established by
  `apps/web/components/github/my-github/pr-status-badges.tsx`; preserve unknown non-empty values as
  provider data rather than translating or omitting them.
- Use the existing `TooltipContent` surface, shadow, collision handling, and arrow. Use dividers
  only to separate multiple PR entries; do not add nested cards, animation, or interactive content.
- Add stable selectors for the summary, each PR entry, title, and status rows so tests can assert
  hierarchy rather than matching one concatenated string.

### Shared task PR icon integration

- In `apps/web/components/github/pr-task-icon.tsx`, replace both single- and multi-PR tooltip bodies
  with `PRTaskStatusSummary`. Pass `isPRReadyToMerge(pr)` into the presentation component rather
  than reimplementing merge readiness.
- Remove `getPRTooltip` and its string-concatenation tests. Keep `getPRStatusColor`,
  `aggregatePRStatusColor`, `areAllOpenPRsReadyToMerge`, `pickDefaultPR`, and all existing
  `data-pr-*` attributes unchanged.
- Give the informational trigger a localized accessible name and keyboard focus path while
  preserving its passive behavior and the containing task row's existing activation semantics.
  The shared integration intentionally updates sidebar, Kanban-card, and rich-list uses together.

### Localization

- Add only missing summary labels and known-state values to
  `apps/web/src/locales/en/github.json`, resolving them with `useTranslation()` during render.
- Regenerate `apps/web/src/locales/pseudo/github.json`; do not call `t()` at module scope or
  translate raw provider fallback values.

## Tests

- **What:** available fields become separate review, CI, and merge/state rows; ready-to-merge uses
  the supplied strict result; missing fields produce no blank rows; unknown values remain visible.
  **File:** `apps/web/components/github/pr-task-status-summary.test.ts`.
  **How:** exercise the exported pure summary derivation with table-driven single-PR states for
  approved/success/ready, failure and conflicts, draft/terminal cases, missing values, and an
  unknown provider value. Rendered hierarchy, wrapping, icons, and selectors remain Playwright
  assertions rather than component-test implementation details.
- **What:** the shared icon still guards malformed store entries, retains single/multi attributes,
  and mounts one grouped summary entry per linked PR.
  **Files:** `apps/web/components/github/pr-task-icon.render.test.tsx` and
  `apps/web/components/github/pr-task-icon.test.ts`.
  **How:** preserve the existing malformed-store and attribute cases, and replace the obsolete
  `getPRTooltip` string cases while leaving status-derivation coverage intact.

## E2E Tests

- **Scenario:** a ready PR with a long title exposes separate **Review — Approved**, **CI —
  Passed**, and **Merge — Ready to merge** rows from the sidebar icon.
  **File:** `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`.
  **What to verify:** locate the icon inside the named sidebar task row, hover it, scope through the
  open Tooltip portal, assert the structured selectors and text, and confirm the summary and
  document remain within the viewport. Keep existing ready-to-merge attribute assertions.
- **Scenario:** a linked-PR row in the phone task-switcher remains a passive indicator inside the
  existing primary task-navigation row.
  **File:** `apps/web/e2e/tests/task/mobile-task-status-summary.spec.ts`.
  **What to verify:** retain the PR-state and horizontal-overflow assertions, tap the row, and
  confirm normal task navigation without a new task-row overlay.

## Mobile Parity

- **Desktop outcome:** scan PR identity, review, CI, and merge state directly from the task icon.
- **Mobile entry point:** the task row remains the primary tap target; after navigation, the
  existing `PRStatusChipDrawer` remains the detailed touch surface.
- **Nearest shipped exemplars:** `session-task-switcher-sheet.tsx` supplies task-row navigation and
  scroll ownership; `pr-status-chip.tsx` supplies the existing coarse-pointer PR detail drawer.
- **Presentation choice and rationale:** this is a content-only desktop tooltip refinement. A new
  adjacent drawer trigger would need a 44 px hitbox and would compete with the dense row's primary
  navigation for supplemental information already available in the task view.
- **Geometry and state:** the mobile drawer keeps its existing single internal scroll owner,
  safe-area behavior, task data, selection, and row action logic. No responsive preference or
  domain state is duplicated.
- **Proof:** the focused `mobile-chrome` task-status-summary scenario verifies the shared icon,
  row navigation, and absence of document horizontal overflow.

## Verification Results

Completed on 2026-08-07.

- RED unit proof: the new summary assertion failed against the old pipe-delimited string, receiving
  `PR #2966: ... | Review: approved | CI: success | Ready to merge` instead of structured rows.
- RED browser proof: the focused Chromium test failed because `pr-task-status-summary` did not
  exist in the old tooltip.
- Focused frontend suite: 4 files and 78 tests passed, covering existing icon semantics plus ready,
  failure/conflict, blocked, draft, terminal, missing, and unknown summary states.
- Desktop Playwright: 1 Chromium scenario passed for hover hierarchy, long-title wrapping,
  viewport containment, keyboard focus, and two-PR grouping.
- Mobile Playwright: 1 `mobile-chrome` scenario passed for passive-icon row navigation and
  document overflow containment.
- `pnpm run typecheck`, targeted ESLint, `i18n:check`, and `i18n:ratchet` passed. The i18n check
  retained 918 advisory pre-existing `zh-cn` catalog parity issues and confirmed the pseudo locale
  is synchronized.
- A gated PR screenshot was captured and visually inspected at
  `apps/web/.pr-assets/pr-status-badge--readable-task-pr-summary.png`; the following normal E2E run
  cleaned the ignored `.pr-assets` directory before successful runner teardown.

No blockers or residual in-scope risks remain.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [Task 01: Build readable PR task summary](task-01-readable-pr-task-summary.md)

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run \
  components/github/pr-task-icon.test.ts \
  components/github/pr-task-icon-draft.test.ts \
  components/github/pr-task-icon.render.test.tsx \
  components/github/pr-task-status-summary.test.ts
cd apps/web && pnpm e2e:run tests/pr/pr-status-badge.spec.ts \
  -- --grep "renders readable task PR summary"
cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/task/mobile-task-status-summary.spec.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web run i18n:check
cd apps && pnpm --filter @kandev/web run i18n:ratchet
```

## Risks

- `PRTaskIcon` is shared by sidebar, Kanban, and rich task-list surfaces. The summary must improve
  all three without changing click propagation or the compact trigger geometry.
- Radix can render both an accessibility tooltip copy and a visible portal. Browser assertions
  must start at the open `[data-slot="tooltip-content"]` portal to avoid strict-locator ambiguity.
- Humanizing provider values must not create a second merge-readiness rule. Only
  `isPRReadyToMerge` may produce **Ready to merge**, and unknown non-empty values must survive.
- Multiple linked PRs increase tooltip height. Keep each entry compact and avoid adding full CI or
  review detail that belongs in the existing PR status popover/drawer.

## Documentation Impact

No public documentation change is required. This work changes an in-product informational
disclosure, not a command, setting, workflow, API, or public term. No ADR is warranted because it
adds no architectural boundary or persistent contract.
