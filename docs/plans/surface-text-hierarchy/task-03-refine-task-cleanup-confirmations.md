---
id: "03-refine-task-cleanup-confirmations"
title: "Refine task cleanup confirmations"
status: completed
wave: 2
depends_on:
  - "01-standardize-surface-typography-primitives"
plan: "plan.md"
requirements:
  - REQ-UI-SURFACE-TEXT-HIERARCHY-001
  - REQ-UI-TASK-CLEANUP-CONFIRMATION-001
acceptance_criteria:
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.2
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.3
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.1
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.2
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.3
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.4
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.5
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.6
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.7
system_design:
  - ../../specs/ui/system-design/surface-text-hierarchy.md
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
---

# Task 03: Refine Task Cleanup Confirmations

## Summary

Turn task archive/delete cleanup copy from equal-weight paragraphs into a direct
task outcome, ordered effects, and supporting reassurance. Share that model
across full dialogs and compact archive surfaces, then make the full mobile
dialogs contained and touch-safe.

## In scope

- Replace the flat cleanup `lines` shape with ordered localized effects and
  supporting notes for single and bulk executor summaries.
- Add a task-local semantic renderer shared by archive/delete dialogs and
  archive popover/inline surfaces.
- Rewrite the task outcome and cleanup catalog entries in all five locales while
  preserving every current executor-specific fact and plural contract.
- Give full task confirmation bodies dynamic-viewport containment and persistent
  footers; keep the existing centered `size="lg"` surface.
- Give phone footer actions 44px full-width targets, restore compact desktop
  sizing, and select the semantic destructive Delete variant.
- Extend cleanup-model, archive/delete, and compact archive component tests.

## Out of scope

- Cleanup behavior, executor detection, APIs, task state, warning conditions,
  focus, dismissal, callback ordering, or archive preference behavior.
- Session deletion, task detachment, and environment reset owned by Task 04.
- Shared primitive files and browser E2E files.

## Implementation acceptance

1. Every single/bulk executor path renders the same ordered effects and notes in
   full and compact surfaces, with complete five-locale key and placeholder
   parity.
2. Long or tall phone content scrolls only inside the full-dialog body while
   title and 44px actions remain reachable; desktop actions remain compact.
3. Delete has one semantic destructive color source, archive remains default,
   and all existing callbacks and safety conditions pass unchanged.

## TDD sequence

1. Change or add cleanup-summary and component expectations for the structured
   model, semantic markup, direct outcome copy, body/footer composition, and
   action variants. Record the expected RED failures.
2. Implement the model and renderer, then update `en`, `pt-pt`, and `zh-cn` in
   that order. Run `pnpm run i18n:zh-hant` after `zh-cn`; it generates `zh-tw`
   first and then `zh-hk` from that updated source catalog.
3. Run focused tests GREEN, then translation parity, typecheck, and focused
   lint. Do not hand-edit generated Traditional Chinese values after generation.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- components/task/task-cleanup-summary.test.ts components/task/task-delete-confirm-dialog.test.tsx components/task/task-archive-confirm-dialog.test.tsx components/task/task-archive-confirmation.test.tsx
pnpm --filter @kandev/web run typecheck
cd web
pnpm exec eslint --max-warnings 0 components/task/task-cleanup-summary.ts components/task/task-cleanup-consequences.tsx components/task/task-delete-confirm-dialog.tsx components/task/task-delete-confirm-dialog.test.tsx components/task/task-archive-confirm-dialog.tsx components/task/task-archive-confirm-dialog.test.tsx components/task/task-archive-confirmation.tsx components/task/task-archive-confirmation.test.tsx
pnpm run i18n:zh-hant
pnpm run i18n:check
pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/task/task-cleanup-summary.ts`
- `apps/web/components/task/task-cleanup-summary.test.ts`
- `apps/web/components/task/task-cleanup-consequences.tsx` (new)
- `apps/web/components/task/task-delete-confirm-dialog.tsx`
- `apps/web/components/task/task-delete-confirm-dialog.test.tsx`
- `apps/web/components/task/task-archive-confirm-dialog.tsx`
- `apps/web/components/task/task-archive-confirm-dialog.test.tsx`
- `apps/web/components/task/task-archive-confirmation.tsx`
- `apps/web/components/task/task-archive-confirmation.test.tsx`
- `apps/web/components/task/task-confirm-dialog-shared.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`

## Dependencies

- Task 01 must be integrated before final verification.

## Parallelism

`parallel-wave-2`

## Inputs

- `docs/specs/ui/requirements/confirmation-warning-hierarchy.md`
- `docs/specs/ui/system-design/confirmation-warning-hierarchy.md`
- `docs/specs/ui/requirements/surface-text-hierarchy.md`
- `docs/specs/ui/system-design/surface-text-hierarchy.md`
- `docs/plans/surface-text-hierarchy/plan.md`
- `apps/web/AGENTS.md`
- `docs/i18n.md`
- `.agents/skills/tdd/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`

## Results

- RED: The four focused suites failed 22 tests against the pre-change flat
  cleanup-line model, prose-only markup, unconstrained dialog body, compact
  phone actions, and non-semantic Delete styling.
- GREEN: Replaced cleanup `lines` with ordered effects and supporting notes,
  added one semantic renderer shared by full and compact archive/delete
  surfaces, and preserved every executor-specific cleanup fact and callback.
- Added dynamic-viewport body containment, persistent full-dialog footers,
  full-width 44px phone actions with compact desktop sizing, and the semantic
  destructive Delete variant.
- Updated all five task catalogs with direct outcome, ordered effect, and
  reassurance copy while preserving plural placeholders and locale parity.
- Focused verification passed with 4 test files and 63 tests. Web typecheck,
  focused ESLint with `--max-warnings 0`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` also passed on the integrated stack.
- The supported task-scoped Traditional Chinese generation command passed with
  zero residuals for `zh-tw` and `zh-hk`; it produced no owned-key difference.
  The repository-wide `pnpm run i18n:zh-hant` command still stops before any
  write on two inherited `agents:dynamicProfileSettings` residuals, reproduced
  unchanged on the exact Task 02 parent.
