---
id: "02-section-spacing"
title: "Verify and repair section spacing after PR 4145"
status: done
wave: 2
depends_on:
  - "01-toolbar-loading"
plan: "plan.md"
requirements:
  - REQ-UI-BOUNDED-CHANGES-001
acceptance_criteria:
  - AC-UI-BOUNDED-CHANGES-001.4
  - AC-UI-BOUNDED-CHANGES-001.6
  - AC-UI-BOUNDED-CHANGES-001.8
  - AC-UI-BOUNDED-CHANGES-001.9
system_design:
  - ../../specs/ui/system-design/bounded-changes-rendering.md
---

# Task 02: Verify and Repair Section Spacing After PR #4145

## Summary

First measure the Changes timeline with PR #4145 integrated. Repair only
remaining section separation, sibling spacing, child alignment, or expanded
commit defects. Preserve that PR's compact headers and phone actions.
The plan records the original regression and the inspected overlap.

## In scope

- Verify the colleague's header changes using their existing regression coverage.
- Repair remaining section separation, 2px list sibling gaps, and content gutters only if reproduced.
- Preserve the equivalent inner padding for controlled expanded commits.
- Account for empty/collapsed sections, repository groups, and before/after history slices.
- Keep estimates consistent with measured shells and preserve existing anchors.
- Add focused component/model regressions and desktop/phone browser geometry checks.

## Out of scope

Header padding removal, disclosure sizing/alignment, and direct first-descendant
adjacency already covered by PR #4145. Also exclude new typography, virtualizer
measurement algorithms, scroll owners, data fetching, Git mutations, or unrelated sections.

## Acceptance

1. Record the five-PR-file/two-commit fixture with PR #4145 integrated. Preserve
   its header sizing and direct first-descendant adjacency. Where still defective,
   restore 2px sibling gaps, 10px expanded-section separation, and the original
   content column. Do not introduce a second implementation of header compaction.
2. Expanded and collapsed commits retain correct padding, tree indentation,
   source identity, and the existing no-hidden-descendants contract.
3. Desktop and phone retain contiguous measured wrappers, stable anchors,
   one body scroller, and phone controls of at least 44px after refresh/resize/reopen.

## ASCII UI preview

UI-04 is specified in the [combined preview](plan.md#ascii-ui-preview).

```text
BEFORE #4145                   PROPOSED REMAINING REPAIR
| PR CHANGES (5)            |   | PR CHANGES (5)            |
|       large blank gap    |   |   file                    |
| file                     |   |   file                    |
| file                     |   |       2px sibling gap     |
| file                     |   |   file                    |
| COMMITS (2) touches files |   |       10px section gap    |
|       large blank gap    |   | COMMITS (2)               |
| commit                   |   |   commit                  |
| commit                   |   |   commit                  |
                               |       2px sibling gap     |
                               |   commit                  |

PHONE AFTER
| [route] [eye] (status)                    ... |
| PR CHANGES (5)                               |
|   wrapped files with touch-sized actions     |
|              section separation             |
| COMMITS (2)                                  |
|   wrapped commit rows / expanded files        |
|             one content scroller             |
| existing bottom navigation                   |
```

The remaining repair is conditional on rendered evidence after PR #4145.
Preserve its compact headers and direct-child adjacency. Desktop numbers are
reference inner content gaps. Row heights remain measured.
The 16px section gutter is additional to existing tree depth indentation.
Phone rows retain their current minimum touch size and wrapping.
This preview maps to AC-UI-BOUNDED-CHANGES-001.6 and `.8`.

## Implementation approach

PR #4145 is merged. Verify its existing header regression before changing
residual spacing. Task 01 may proceed independently. If no residual defect
reproduces, record verification and omit a spacing patch.

Use the existing bounded-rendering design's Section and child geometry rules.
Derive section/list/expanded-commit boundaries alongside semantic descriptors,
or in one equivalent local history-shell projection. Avoid rebuilding the full
model on scroll. Compute footer placement from the final visible row, including
the header when a section has no visible descendants.

Keep PR #4145's headers unchanged. Place any required expanded-section separation
after the final visible child, retaining compact collapsed headers.
Restore remaining list sibling gaps inside measured shells; singleton `space-y-0.5` lists
cannot create those gaps. Restore the section content gutter before tree depth.
Do not add padding to absolute group wrappers or introduce a global virtualizer gap.

Preserve standalone `CommitRow` behavior while correcting controlled expanded
rows. Compare its original normal-flow child placement with the new measured
shells. Avoid counting a commit's trailing padding both before its first file
and after its last file. Retain collapsed-header padding and error/empty content.

Include assigned spacing in estimates and positive measurements. Leave the
existing presentation measurement refresh and anchor restoration intact.
The before/after history slices must preserve section separation when no working
tree rows are present. Apply this after Task 01 removes passive loading descriptors.

## Files likely touched

- `apps/web/components/task/changes-timeline-history-model.ts`
- `apps/web/components/task/changes-timeline-history-row.tsx`
- `apps/web/components/task/changes-timeline-working-tree.tsx`
- `apps/web/components/task/changes-panel-timeline-history.tsx` (if needed for slice boundaries)
- `apps/web/components/task/commit-row.tsx` (controlled expanded padding only)
- Corresponding existing model, history-row, working-tree, history, and commit-row tests.
- `apps/web/e2e/tests/git/changes-commit-spacing-helpers.ts`
- `apps/web/e2e/tests/git/large-changes-virtualization.spec.ts`
- `apps/web/e2e/tests/git/mobile-large-changes-virtualization.spec.ts`

## TDD and browser evidence

First verify the integrated header repair. For residual defects, add failing
model/component assertions for final-row spacing allocation,
including collapse, empty sections, multiple repositories, flat/tree mode,
expanded/error/empty commits, and PR-before/Commits-after with an empty working tree.
Keep keys, source targets, and existing controlled-expansion behavior covered.
Assert estimates include the same deliberate space as the rendered shell.

Extend the existing browser suites with a small five-file/two-commit fixture.
Test names must contain `spacing` so the focused command below includes them.
Measure actual header and inner row rectangles after font readiness and settled
layout. Assert the original desktop gaps with at most 1px rounding tolerance.
Check the original content-column alignment, not merely the wrapper's left edge.
Reuse the colleague's `changes-history-regression` suites for header geometry.
Do not duplicate their header-sizing or direct-adjacency test cases.
Keep the existing adjacent-wrapper contiguity assertions unchanged.

Measure expanded commit header/file/footer boundaries and section collapse/reopen.
Exercise list/tree and multiple repository groups. Repeat relevant assertions
after presentation refresh, panel resize, and reopening. Retain the visible
anchor and bounded mounted-row checks from the existing measurement regression.

Run phone checks at 393px and 767px, using actual pointer mode and wrapped names.
Assert preserved section grouping, content gutter, touch targets, one scroller,
and zero document overflow. Do not apply fixed desktop row heights to phone rows.
Capture desktop/phone after-state screenshots through the existing `prCapture` fixture.

## Verification

Run from the repository root. Install frozen workspace dependencies from `apps/`
first if absent. Execute the managed desktop and phone runners sequentially.

```bash
(cd apps/web && pnpm exec vitest run components/task/changes-timeline-model.test.ts components/task/changes-timeline-history-row.test.tsx components/task/changes-timeline-working-tree.test.tsx components/task/changes-panel-timeline-history.test.tsx components/task/changes-timeline-viewport.test.tsx components/task/changes-timeline-measurement.test.ts components/task/commit-row.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/changes-timeline-history-model.ts components/task/changes-timeline-history-row.tsx components/task/changes-timeline-working-tree.tsx components/task/changes-panel-timeline-history.tsx components/task/commit-row.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/git/changes-history-regression.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-changes-history-regression.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/git/large-changes-virtualization.spec.ts --grep spacing)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-large-changes-virtualization.spec.ts --grep spacing)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If implementation changes additional files, include them in the focused checks.
Record measurements and screenshots in Results. Mark this work order complete
and the combined plan implemented after both work orders pass their gates.

## Dependencies

Task 01 settles loading-row removal before this work order assigns row boundaries.
PR #4145 supplies the header baseline and its existing regression suites.
The existing measurement-refresh repair remains the current measurement algorithm.

## Risks

- A last-row marker can become stale after collapse or a partial accepted update.
- Singleton lists and negative margins can conceal incorrect padding in mocked DOM tests.
- Expanded commits need distinct section and commit-footer boundaries.
- Unmeasured CSS space would recreate scroll drift and gaps after refresh.
- Reapplying legacy header margins/padding would conflict with PR #4145's tested geometry.

## Parallelism

`sequential`

## Inputs

- [Existing bounded-rendering requirement](../../specs/ui/requirements/bounded-changes-rendering.md)
- [Owning design](../../specs/ui/system-design/bounded-changes-rendering.md)
- [Plan, source evidence, and measured comparison](plan.md)
- [Previous measurement repair](../changes-timeline-measurement-refresh/plan.md)

## Results

Planning diagnostic: the disposable real-component Chromium comparison reproduced
the lost section/sibling spacing and 16px child alignment shift. Its server,
browser, and temporary files were removed. PR #4145 later landed at head
`c5038084dc3d2a3f05294c0797e48905d5fb8953` in merge commit
`0ec0538aa038f2e8b8617fb4f5be6128f0cedbac`; its header work remains unchanged.

The integrated five-file/two-commit fixture measured 0px sibling gaps, 0px
expanded-section separation, and a 20px leftward content shift at 1280px before
the repair. After the repair, it measures 2px between PR files, 10px before the
next section, and a -4px file-to-header offset. PR #4145's 28px desktop headers,
44px phone controls, and direct first-descendant adjacency remain intact.

The expanded commit fixture measures 0px from the commit header wrapper to its
first detail file, 2px between sibling detail files, and 6px after the final
detail file before the next commit (4px commit footer plus 2px list gap).
The virtualizer estimate includes each shell's measured spacing and removes the
expanded header's displaced footer. Phone geometry passed at 393px and 767px
with one scroll owner and no document overflow.

Verification passed: seven focused Vitest suites (46 tests), desktop history
regressions (6 tests), phone history regressions (4 tests), desktop and phone
commit-spacing regressions (1 each), and expanded commit file navigation (1).
The managed production Vite build, web typecheck, targeted ESLint, documentation
catalog/specification checks, and `git diff --check` passed. The combined plan
remains in progress while PR #4155's fixup and review continue.
