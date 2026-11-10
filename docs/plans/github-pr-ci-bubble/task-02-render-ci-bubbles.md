---
id: "02-render-ci-bubbles"
title: "Render shared CI bubbles and skipped details"
status: done
wave: 2
depends_on:
  - "01-project-ci-state"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.1
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.2
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.3
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.4
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.5
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.6
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.7
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.8
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.9
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.10
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.11
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.12
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.13
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.14
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.15
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.16
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.17
system_design:
  - ../../specs/integrations/system-design/github-pr-ci-bubble.md
---

# Task 02: Render shared CI bubbles and skipped details

## Summary

Connect the new projection to existing task PR indicators and disclosures.
Keep successful and all-skipped results free of bubbles. Preserve CI, conflicts, and automation as separate visible facts.

## In scope

- Use `/tdd` for the display logic and component regressions. Use `/mobile-parity` for the shared glyph and touch disclosure.
- Add the upper-right marker to `PRStatusGlyph` and move conflicts to the inline trailing slot.
- Update `PRTaskIcon`'s view model, disclosure props, compact/full authority selection, accessible names, and static fallback.
- Update single/multi/per-PR top-bar entries and presentational subtask glyphs. Do not nest new tooltip roots.
- Give the composer draft state the shared PR glyph and bubble. Preserve its existing non-draft primary glyph composition.
- Add skipped-only and mixed-skipped explanations to task summaries and check popovers, including authoritative empty-feedback behavior.
- Qualify retained metadata as the last detailed observation. Explicit failures, running work, and workflow attention remain visible.
- Add complete translated copy and count-based plural keys. Generate Traditional Chinese through the existing script.
- Reconcile the active approval requirement 003 placement/priority clauses and its design section with the new CI position.
  Keep eligibility and stale-evidence clauses intact. Link this package instead of duplicating the CI contract.

## Out of scope

- Changes to the data contract already delivered by Task 01, except a demonstrated integration defect.
- New overlays, provider reads, merge/repair actions, public docs, or publication.

## Acceptance

1. Every scoped status surface renders the shared CI meaning, with no success/skipped bubble and with an independent conflict triangle and automation dots.
2. Freshness, legacy omission, negative projection, and head/terminal cleanup preserve one coherent current display and per-PR explanations.
3. Skipped details, accessible names, localization, existing desktop disclosures, and actual phone drawers pass the targeted component checks.

## ASCII UI preview

UI-01 and UI-02 excerpts from the [combined previews](plan.md#ascii-ui-preview):

```text
Draft pass/skipped: [PR]             (no upper-right bubble)
Draft failed:      [PR+x] [!]        (CI cross plus separate conflict)
Draft approval:    [PR+L] [!] 2      (CI lock, conflict, then PR count)

Details:
CI     All checks skipped
       Last detailed observation    (persisted fallback only)
Merge  Draft
```

UI-03 phone behavior:

```text
Tasks: Fix documentation checks [PR+L] [!]
Tap whole PR control -> existing PR drawer
  CI     Awaiting maintainer approval
  Merge  Conflicts
Dismiss -> original trigger, same task route
```

The full trigger owns touch input and the existing drawer owns scrolling.
Keep the current dynamic height and safe-area clearance. Account for the wider inline conflict group in the actual hitbox.
The nearest exemplars are `PRTaskIconDrawer` and `PRStatusChipDrawer`; do not add a new surface.
Placement and interaction are structural. Sample spacing is illustrative.
These excerpts map to criteria .1-.16. Task 03 supplies the rendered geometry and pointer evidence.

## Verification

Run from the repository root. Task 01 supplies workspace dependencies.
Write new component cases before production logic. Keep older scenarios and change their placement assertions to the new contract.

```bash
(cd apps/web && pnpm exec vitest run components/github/pr-status-glyph.test.tsx components/github/pr-topbar-button.ci-state.test.tsx components/github/pr-ci-state.test.ts components/github/pr-task-icon.test.ts components/github/pr-task-icon-draft.test.ts components/github/pr-task-icon.render.test.tsx components/github/pr-task-icon.workflow-approval.test.tsx components/github/pr-task-icon-conflicts.test.ts components/github/pr-task-icon.automation-render.test.tsx components/github/pr-task-icon.hover-render.test.tsx components/github/pr-task-icon.negative-projection.test.tsx)
(cd apps/web && pnpm exec vitest run components/github/pr-task-status-summary.test.ts components/github/pr-ci-popover.checks.test.tsx components/github/pr-status-chip.test.tsx components/github/pr-workflow-attention-chip.test.tsx components/github/pr-workflow-attention-summary.test.ts lib/github/check-buckets.test.ts lib/task-pr-info.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

Run every additional changed test file by exact path. Record the complete command and result.
Task 03 immediately follows with production-build desktop/phone E2E and captures before this UI package is accepted.

## Files likely touched

- `apps/web/components/github/pr-status-glyph.tsx` and new `pr-status-glyph.test.tsx`
- `apps/web/components/github/pr-task-icon.tsx`, `pr-task-icon-disclosure.tsx`, and `pr-task-icon-aria.ts`
- `apps/web/components/github/pr-task-workflow-projection.ts` and its existing negative/freshness tests
- `apps/web/components/github/pr-topbar-button.tsx` and new `pr-topbar-button.ci-state.test.tsx`
- `apps/web/components/github/pr-status-chip.tsx`, `pr-task-status-summary.tsx`, and `pr-ci-popover.tsx`
- `apps/web/components/task/task-contribution-icons.tsx` and `task-subtask-row.tsx`
- `apps/web/components/integrations/change-request-task-status-summary.tsx`
- Existing component/helper tests named in Verification
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/github.json` and any shared namespace used by the summary
- `docs/specs/integrations/requirements/github-workflow-attention.md` and its paired design

## Dependencies

Task 01 supplies the compact CI projection, selected metadata, and pure classifier.

## Risks

Fixed-width wrappers can clip the conflict triangle or shrink a touch target.
An old compact positive must not restore a bubble after a newer full negative.
All-skipped metadata must not hide a newer active run or falsely report cancelled-only checks as skipped.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-ci-bubble.md)
- [Presentation design](../../specs/integrations/system-design/github-pr-ci-bubble.md#shared-presentation)
- [Phone composition](../../specs/integrations/system-design/github-pr-ci-bubble.md#desktop-and-phone-composition)
- Plan previews UI-01 through UI-03 and Task 01's completed results

## Results

Completed on 2026-11-10.

- Extended `PRStatusGlyph` with an explicit `ciState` upper-right bubble and moved the conflict triangle to an inline trailing slot beside the PR glyph. Automation dots remain in their existing corners.
- Wired the shared CI state through `PRTaskIcon`, its disclosure projection, `TaskContributionIcons`, `PRTopbarButton`, `SubtaskCIGlyph`, and the composer `PRStatusChip` draft glyph.
- Added coherent compact/full CI authority. A valid compact summary supersedes full records only when strictly newer than every full sync and workflow observation. A newer compact `none` clears an obsolete full bubble, and an older compact positive cannot restore one.
- Added the muted `all_skipped` summary row, separate skipped counts, and all-skipped check-popover copy. Loaded selected feedback takes priority over persisted metadata, and cancelled-only results keep their existing explanation.
- Added English, Portuguese, Simplified Chinese, Japanese, and Korean copy, then generated Traditional Chinese catalogs with `pnpm run i18n:zh-hant`.
- Updated approval placement assertions so the conflict triangle and approval lock are proven as independent indicators.

Review remediation on 2026-11-10:

- Reordered the CI summary decision so explicit live failure, pending, and unstable/attention stay primary. Retained all-skipped metadata is now a qualified `lastDetailedObservationAllSkipped` detail, never the primary row, when a newer rollup exists.
- Changed compact CI disclosure replacement to remove every superseded check-outcome row for the selected PR, including `passed` and `all_skipped`. Independent workflow reasons remain, and a newer approval replaces older workflow-attention rows.
- Carried each full PR's repository identity into the disclosure entries. Compact CI now matches the complete repository plus number key, refuses to guess an ambiguous number, and keeps repository attribution on synthetic entries.
- Passed compact CI authority into the no-full-record disclosure. The known CI row and selected identity now render beside loading or unavailable hydration copy on desktop and in the touch drawer.
- Added focused regressions for retained all-skipped history, stale outcome replacement, two-repository same-number attribution, ambiguous numbers, and compact CI disclosure during loading/unavailable hydration.

Verification:

- `pnpm exec vitest run` on the task 02 component, helper, and regression list, plus the new `pr-status-glyph.test.tsx`, `pr-topbar-button.ci-state.test.tsx`, `pr-task-icon.ci-bubble.test.tsx`, `pr-task-icon.compact-ci.test.tsx`, `pr-task-workflow-projection.ci-disclosure.test.ts`, and `pr-status-chip.ci-state.test.tsx`: 23 files, 311 tests passed.
- `pnpm run typecheck`: passed.
- `pnpm run lint`: passed.
- `pnpm run i18n:check`: passed with every locale complete and the pseudo locale in sync.
- `pnpm run i18n:ratchet`: passed with 0 new literals.
