---
id: "02-section-header-spacing"
title: "Normalize section header spacing"
status: done
wave: 2
depends_on:
  - "01-current-divergence-evidence"
plan: "plan.md"
requirements:
  - REQ-UI-BOUNDED-CHANGES-001
acceptance_criteria:
  - AC-UI-BOUNDED-CHANGES-001.6
  - AC-UI-BOUNDED-CHANGES-001.8
system_design:
  - ../../specs/ui/system-design/bounded-changes-rendering.md
---

# Task 02: Normalize Section Header Spacing

## Summary and scope

Remove obsolete whole-section padding from virtual history headers and align
working-tree/history disclosure density. Own responsive header geometry,
estimates, and rendered spacing regressions. Do not change history classification,
global control sizes, or the working virtualizer measurement refresh algorithm.

## Acceptance

1. Add a failing `keeps collapsed history headers compact` browser regression
   using the confirmed-divergence fixture from task 01. At the standard root
   font, fine-pointer disclosures and their row wrappers measure 28px within
   1px, with no header-only 12px bottom padding or negative header margins.
   Check PR Changes, both history groups, and the working-tree header.
2. Expanded first descendants immediately follow the measured header within
   1px. Adjacent virtual wrappers remain contiguous after collapse/reopen,
   font refresh, resize, and Changes hide/reopen. Keep bounded mounting and
   stable focus/anchor behavior. Check content/control bounds as well as
   wrapper adjacency so internal whitespace cannot falsely pass.
3. At 393px and 767px phone widths, including fine-pointer emulation, and a
   coarse-pointer workbench, disclosure hit targets are at least 44px. At 768px
   fine pointer and ordinary desktop widths they return to 28px. The phone
   retains one content scroller, contained long names/actions, and no document
   horizontal overflow. Variable-height rows remain measured, not fixed.

## ASCII UI preview

UI-02/03, [full preview](plan.md#ascii-ui-preview), criteria .6/.8:

```text
Desktop: collapsed disclosure rows, 28px each
o PR CHANGES (11) >
o LOCAL CHECKOUT COMMITS (1) >
o PR #4141 VERSION (1) >

Phone: existing Changes surface, >=44px hit targets
| o PR CHANGES (11) >           |
| o LOCAL CHECKOUT COMMITS (1) >|
| o PR #4141 VERSION (1) >      |
```

Header controls and actions fit within measured rows. No added section spacers.

## Verification

Dependencies were bootstrapped by task 01. Run suites sequentially.

```bash
(cd apps/web && pnpm exec vitest run components/task/changes-timeline-history-row.test.tsx components/task/changes-timeline-working-tree.test.tsx components/task/changes-timeline-viewport.test.tsx components/task/changes-timeline-measurement.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/git/changes-history-regression.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-changes-history-regression.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/git/large-changes-virtualization.spec.ts -- --grep 'spacing')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-large-changes-virtualization.spec.ts -- --grep 'spacing')
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/changes-timeline-history-row.tsx components/task/changes-timeline-working-tree.tsx)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/changes-sidebar-history-spacing
```

Use Playwright screenshots to compare desktop/phone with the previews. Record
actual discovered tests, bounds, and results. Preserve existing translations;
any new product copy must use all locale catalogs and run i18n:check.

## Files likely touched

- `apps/web/components/task/changes-timeline-history-row.tsx`
- `apps/web/components/task/changes-timeline-working-tree.tsx`
- Corresponding `*.test.tsx` files named above
- `apps/web/e2e/tests/git/changes-history-regression.spec.ts`
- `apps/web/e2e/tests/git/mobile-changes-history-regression.spec.ts`

## Dependencies and inputs

Task 01 supplies the focused history fixtures. Read the bounded-rendering
design's spacing section and existing commit-spacing E2E helpers.
Use the shipped task mobile Changes composition and mobile-parity guidance.

## Risks and parallelism

Touch minimums and responsive estimates must agree; narrow fine pointers must
not inherit compact desktop hit targets. Wrapping/font scaling can enlarge
actual rows. Preserve positive dynamic measurements. Execution is `sequential`.

## Results

Removed history-only bottom padding and negative margins, centered timeline
dots, and set disclosure minima and row estimates to 28px desktop / 44px phone
or coarse pointer. Rows retain dynamic measurement for wrapping and font scaling.
RED: the browser observed a 24px control in a 26px working-tree wrapper with a
-2px offset, and narrow fine pointers retained 24px controls. The regression
now measures all four section kinds and their first expanded PR descendant.

GREEN: the combined eight-suite unit run passed 76 tests. The full initial
desktop regression run passed four scenarios; the final desktop spacing subset
passed compact headers, the added coarse-pointer workbench scenario, and the
existing commit-spacing regression. The phone regression suite passed all three
scenarios at 393/767px. Controls/wrappers measured 28px desktop and 44px phone;
narrow-panel wrapping to 49.5px preserved matching wrapper bounds without offsets.
Refresh, resize, Changes reopen, and expanded descendant adjacency passed.

The native touch tablet layout between 768px and 1023px shows diff review rather
than this summary. Coarse-pointer summary coverage therefore uses the existing
1040/1280px workbench. Phone checks retain one scroller and no page overflow.
Screenshots were inspected at `/tmp/kandev-history-review`. Targeted ESLint,
Prettier, typecheck, public-doc validation (47 pages), catalog validation (339
decisions / 1,282 specs), and full specification lint passed. The existing phone
commit-spacing regression passed its one scenario. The documentation coverage
preflight accepted both completed work orders and all three production paths;
diff whitespace checks passed. Publication and merge are authorized after
local verification; remote checks and review evidence are tracked with the PR.

PR review follow-up reproduced a 24px history repository disclosure on a 393px
fine-pointer phone. Applied the phone-width minimum to the shared repository
header. The regression now seeds two repository groups, checks the 44px target
and center hit test, clicks the control, and verifies collapse. The rebuilt
six-scenario desktop run passed, alongside all 76 focused unit tests, typecheck,
and targeted ESLint. Fresh desktop and narrow fine-pointer screenshots were
captured. The phone run passed four scenarios with fresh captures. Exact-head
remote checks follow the fixup push.
