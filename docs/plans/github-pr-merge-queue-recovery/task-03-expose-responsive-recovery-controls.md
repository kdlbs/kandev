---
id: "03-expose-responsive-recovery-controls"
title: "Expose responsive recovery controls"
status: done
wave: 3
depends_on:
  - "02-evaluate-repair-and-requeue"
plan: "plan.md"
requirements:
  - REQ-UI-CI-PR-MERGE-QUEUE-RECOVERY-001
acceptance_criteria:
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.1
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.2
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.3
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.4
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.5
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.6
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.7
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.8
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.9
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.10
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.11
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.12
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.13
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.14
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.15
system_design:
  - ../../specs/ui/system-design/ci-pr-merge-queue-recovery-controls.md
  - ../../specs/integrations/system-design/github-pr-merge-queue-recovery.md
---
# Task 03: Expose Responsive Recovery Controls

## Summary

Keep the two existing automation switches and add queue-aware copy and status.
Use the same state derivation in the desktop hover popover and mobile drawer.

## In scope

- Add web types for queue-recovery state.
- Update the auto-merge label.
- Adapt the title, subtitle, and supporting text to queue context.
- Use the normalized removal cause and generic unknown-cause fallback.
- Add the compact queue-recovery status line.
- Update the header information and prompt dialog copy.
- Add focused helper and component tests.
- Update all five locale catalogs.
- Update `docs/public/integrations.md`.

## Out of scope

- New navigation or overlay surfaces.
- A manual requeue button.
- GitLab copy changes.

## Acceptance

- The surface contains two switches and no queue-recovery switch.
- The queue status and same-head guard are clear on desktop and mobile.
- An already queued PR explains option adoption without implying a new enqueue.
- A removed PR explains the next repair or requeue step.
- Raw provider text does not become a localized label or accessible name.
- All copy is localized, accessible, and safe at narrow widths.

## Verification

```bash
# Run from apps/web.
pnpm test -- components/github/pr-ci-popover.automation.test.tsx components/github/pr-ci-automation-rows.test.tsx
pnpm run i18n:check

# Run from the repository root. The test harness resolves public-docs paths
# relative to the current working directory.
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Install workspace dependencies from `apps` before the first pnpm command in a
fresh worktree.

## Files likely touched

- `apps/web/lib/types/github.ts`
- `apps/web/lib/github/ci-automation.ts`
- `apps/web/lib/github/ci-automation.test.ts`
- `apps/web/components/github/pr-ci-automation-controls.tsx`
- `apps/web/components/github/pr-ci-automation-rows.tsx`
- `apps/web/components/github/pr-ci-automation-rows.test.tsx`
- `apps/web/components/github/pr-ci-automation-prompt-dialog.tsx`
- `apps/web/components/github/pr-ci-popover.automation.test.tsx`
- `apps/web/src/locales/*/github.json`
- `docs/public/integrations.md`

## Dependencies

- Task 02 supplies the final API state and retry semantics.

## Risks

- The existing compact popover has limited vertical space.
- The label change also affects selectors and public documentation.

## Parallelism

`sequential`

## Inputs

- UI requirement and system design.
- Existing PR automation rows and `PRStatusChipDrawer` mobile composition.

## Results

Implemented the queue-aware desktop hover-popover and mobile drawer state
surface while retaining exactly two automation switches. The pure helper now
derives active-queue, actionable-removal, non-actionable-removal,
repair-requested, same-head wait, and new-head wait states from durable PR and
automation state. Titles, subtitles, supporting text, status, help, and prompt
copy use localized keys; unknown, manual, and branch-protection causes remain
generic and raw provider text is not exposed as a label or accessible name.

Added focused helper and component coverage for queue context, recovery copy,
same-head waiting, accessibility, and the two-switch invariant. Updated all
five locale catalogs and `docs/public/integrations.md`.

Verification:

- `pnpm test -- components/github/pr-ci-popover.automation.test.tsx components/github/pr-ci-automation-rows.test.tsx components/github/pr-status-chip.test.tsx lib/github/ci-automation.test.ts`: 4 files, 69 tests passed.
- `pnpm run typecheck`: passed.
- `pnpm exec eslint` on the changed UI/helper files: passed with zero warnings.
- `pnpm run i18n:check`: passed; all five required catalogs complete, pseudo
  locale synchronized, and no untranslated non-JSX copy or em dashes.
- `node --test scripts/validate-public-docs.test.mjs` from the repository root:
  61 tests passed.
- `node scripts/validate-public-docs.mjs` from the repository root: 41 pages
  validated.
