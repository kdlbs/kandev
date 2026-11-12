---
created: 2026-11-11
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003
system_design:
  - ../../specs/integrations/system-design/github-pr-ci-bubble.md
  - ../../specs/integrations/system-design/github-workflow-attention.md
legacy_specs: []
---

# Fix plan: Restore the PR conflict corner badge

## Overview

Restore the small conflict triangle in the PR icon's upper-right bubble on desktop and phones.
One sequential work order changes the shared glyph, updates affected regression coverage, and corrects public guidance.

The user requested the previous badge appearance after the placement investigation.
Conflict priority restores that previous behavior: conflicts own the corner while details retain both conflict and CI information.
The integrations system owns this repair because GitHub PR evidence defines the indicator and its disclosures.

## Evidence and contract reconciliation

PR #5244, commit `c06feb90b2e`, changed `PRStatusGlyph` on 2026-11-10.
It replaced the absolute conflict badge with a trailing flex child and reserved the corner for CI.
At the time of investigation, the glyph test explicitly required inline placement. The supplied screenshots matched that markup.

Smallest reproduction: render `PRStatusGlyph` with `hasMergeConflicts={true}`, with either glyph size and any CI state.
The triangle appears beside the icon and increases its inline width.
The investigation ran `pr-status-glyph.test.tsx`: all 11 existing tests passed because they enforce the current layout.
That baseline is diagnosis evidence, not validation of this fix.

This repair changes the intended presentation contract rather than correcting nonconforming implementation.
The CI-bubble requirement and design now make the restored placement active. The workflow-approval pair defers to the same priority.
The [original CI-bubble plan](../github-pr-ci-bubble/plan.md) remains an implemented historical record.
Its completed work orders and test counts do not prove the amended behavior.

## Scope

### In scope

- Restore the conflict corner badge for every `PRStatusGlyph` consumer, including compact and hydrated task rows.
- Keep one visible corner badge, with conflict priority over failed, approval, attention, and running CI.
- Preserve current CI meaning, accessible names, attributed details, automation dots, and PR counts.
- Prove badge geometry, resolution fallback, and existing desktop/phone disclosure behavior.
- Update the existing integration documentation during implementation.

### Out of scope

- Backend data, GitHub polling, freshness rules, merge/repair actions, new provider reads, or runtime flags.
- GitLab/plugin indicators, base PR colors, and the composer's existing non-draft primary status icons.
- New disclosure surfaces, product copy, dependencies, commits, publication, or subagent delegation.

## Technical approach

Change `apps/web/components/github/pr-status-glyph.tsx` at the shared presentation boundary.
Keep one relative 14px task or 16px top-bar container. Remove the trailing conflict child and its inline gap.
Use the existing 10px background wrapper and 8px red triangle at `-right-1 -top-1`.
Keep `pr-merge-conflict-warning`, the CI test identifiers, and the existing automation identifiers.

Render the conflict badge when `hasMergeConflicts` is true. Otherwise render `PRCIBubble` with the unchanged `ciState`.
Do not hide CI by changing classification, props, accessible-name inputs, or disclosure projections upstream.
After conflict resolution, the already selected CI state determines the next badge without another request.

Existing consumers inherit the correction: `PRTaskIcon`, `TaskContributionIcons`, `SubtaskCIGlyph`, and single/multi/per-PR top-bar entries.
The shared draft glyph inside `PRStatusChip` inherits the same priority. Its other primary status composition remains unchanged.
Modify a consumer only if focused evidence identifies a clipping, target-size, or information-loss defect caused by this change.

### Compatibility

| Source or surface | Identity and behavior | Evidence and fallback |
| --- | --- | --- |
| Compact GitHub task summary | Existing conflict and CI projection, before hydration | Compact component regression and reload. Unknown evidence creates no new state. |
| Hydrated GitHub task PRs | Any eligible open-PR conflict wins the aggregate corner | Full/mixed-PR regressions. Terminal siblings remain excluded. |
| Static fallback and subtask glyph | Existing props, same shared badge | Shared glyph matrix covers both sizes. No disclosure root is added. |
| Top bar and draft composer glyph | Existing aggregate/per-PR props | Consumer component checks and rendered top-bar geometry. |
| GitLab and plugin providers | Separate provider contracts | No shared GitHub state inference or provider changes. |

### Phone interaction

Use the shipped `PRTaskIconDrawer` and `PRStatusChipDrawer` exemplars with `useTouchDrawer`.
The entry point remains the whole PR control in Tasks or the task header. The badge has no separate action.
The existing temporary drawer explains the short status information. Its body remains the single scroll owner with current safe-area handling.
Retain shared state and translated details. Retain the effective 44px target on phones and coarse pointers despite the smaller visible group.
Keep fine-pointer desktop density. Check the canonical phone viewport, 767px, 768px, and a narrow fine-pointer viewport.

## ASCII UI preview

UI-01: Current and proposed badge composition. The same glyph arrangement applies on desktop and phones.

```text
Current: conflict plus failed CI       Proposed: conflict plus failed CI
        (x)                                      (!)
       [PR] [!] 2                               [PR] 2

After conflict clears, CI still fails  After conflict clears, CI is none
        (x)
       [PR] 2                                   [PR] 2
```

`(!)` represents the red conflict triangle. `(x)` represents the CI failure cross.
The badge overlaps the main glyph's upper-right corner. No trailing triangle or extra conflict column remains.
The automation dots retain their upper-left and lower-right positions. PR counts follow the base glyph.
The placement, priority, and allocated width are structural requirements. Sample spacing is illustrative.
UI-01 maps to CI-bubble criteria .1-.4, .7-.12, .16, and .18.

UI-02: Existing disclosures with conflict plus failed CI or approval.

```text
Desktop: hover/focus PR control    Phone: tap whole PR control
Existing status details           Existing PR drawer
  CI     Failed / Approval         Fixed header
  Merge  Conflicts                 Scroll body: CI, merge, existing details
                                  Dismiss -> same route and original trigger
```

Phone tap opens the existing drawer without selecting the task. The drawer keeps both explanations and their PR identities.
UI-02 maps to CI-bubble criteria .8, .10, .15-.16 and workflow-approval criteria 003.3, 003.6, and 003.8.

## Tests

Criterion suffixes below refer to `AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001`, unless named otherwise.

| Criteria | Regression evidence |
| --- | --- |
| .1-.4, .7, .9-.10, .18 | `pr-status-glyph.test.tsx`: both sizes, all CI states with conflicts, exactly one badge, automation, rerender after resolution. |
| .8-.13, .18; workflow 003.3, 003.6-.8 | `pr-task-icon.ci-bubble.test.tsx` and `pr-task-icon.workflow-approval.test.tsx`: compact/full, mixed PRs, retained accessible names and attributed details. |
| .11, .16 | `pr-topbar-button.ci-state.test.tsx` and `pr-status-chip.ci-state.test.tsx`: shared priority in existing consumers. |
| .13-.14, .17 | Existing conflict, negative-projection, CI-classification, automation, and refresh-route suites remain regression guards. |

The named RED regression is `renders a conflict in the upper-right slot for either glyph size` in `pr-status-glyph.test.tsx`.
Add it before production changes. Its positioning and single-badge assertions must fail against the current inline implementation.
Keep real geometry assertions in Playwright because the component environment does not perform browser layout.

## E2E tests

| Project and file | Required outcomes |
| --- | --- |
| `chromium`, `tests/pr/pr-ci-bubble.spec.ts` | Check corner overlap, unchanged inline width, and the 8px triangle inside its 10px wrapper. Keep both detail reasons and automation dots. Cover 16px top-bar geometry too. |
| `chromium`, `tests/pr/pr-status-badge.spec.ts` | Check geometry on sidebar, Home card, and pipeline row. Clear the conflict while CI remains failed and confirm the CI marker returns. Retain theme and approval scenarios. |
| `mobile-chrome`, `tests/pr/mobile-pr-ci-bubble.spec.ts` | Corner geometry before tap/reload, no failed-CI bubble during conflict, both drawer reasons, effective 44px target, route and focus restoration. |
| `mobile-chrome`, `tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts` | Conflict hides the approval lock without losing either reason. Preserve automation, multi-PR attribution, resolution, and drawer-scroll scenarios. |
| `chromium`, `tests/pr/pr-multi-popover.spec.ts` | Preserve aggregate conflict priority, identify the conflicted open PR, and check the 8px triangle inside the dropdown row. |
| `chromium`, `tests/pr/pr-topbar-popover.spec.ts` | Preserve conflict priority in the single topbar badge and retain its details. |

Measure the 10px warning wrapper and 8px triangle against the main PR SVG: the wrapper overlaps the SVG's right edge and extends above its top.
Check the triangle under topbar-button and dropdown-menu item sizing rules. Also assert that the glyph's allocated inline width matches the base icon width.
Visibility alone cannot prove placement or symbol size.
Check light/dark rendering, row containment, both sides of 768px, and no document horizontal overflow.
Use each viewport's real entry point. Measure the effective touch area, including an existing pseudo-element, and exercise its edge with a tap.
Use provider-backed fixtures and persisted API convergence. Do not inject desired badge state into the browser store.
Capture and inspect the existing `prCapture` desktop/phone images against UI-01 and UI-02.

## Work orders

- [x] [Task 01: Restore and prove the shared conflict corner badge](task-01-restore-conflict-corner-badge.md).

The work order is sequential. Its exact commands and owned files are in [Verification](task-01-restore-conflict-corner-badge.md#verification).

## Verification results

### Planning validation

Planning validation passed on 2026-11-11:

- `python3 scripts/list-docs.py validate`: validated 449 decisions and 1,984 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- The work order's `validateCoverage` preflight accepted both requirement/design chains and the single work order.
  This preflight used the planned glyph path to validate traceability. It did not modify or validate production code.
- Local document links, verification-shell syntax, and `git diff --check -- docs/specs docs/plans` passed.
- Prose checks found no plan/work-order sentence over 25 words after technical identifiers were treated as single terms.
- All seven changed or new files are internal documentation, unstaged and uncommitted.

### Implementation validation

Completed on 2026-11-11:

- The glyph regression failed before the production edit, then the targeted unit command passed all 80 tests in 10 files.
- Web typecheck and lint passed. The managed desktop build passed, then desktop E2E passed 16/16. The managed phone build passed, then phone E2E passed 5/5.
- The additional aggregate and topbar E2E suites passed 17/17. The focused desktop capture case passed 1/1.
- Browser geometry matched the 14px task glyph and 16px topbar glyph. The 10px conflict wrapper overlapped the PR icon's right edge and extended above it. The inline allocation matched the base glyph width.
- A follow-up review found that host button and menu rules enlarged the nested triangle. The explicit `size-2` class keeps it at 8px. The topbar and multi-PR menu regression cases pass after the correction.
- The phone pseudo-element provided a 44px touch target. Its outer edge opened the existing drawer and returned focus to the trigger after dismissal.
- Inspected desktop and phone captures matched UI-01 and UI-02: conflict owns the corner, automation markers remain visible, and existing details retain both reasons. No preview mismatch remains.
- Public documentation and the amended specifications passed the validation commands in the work order. The documentation coverage preflight accepted both requirement/design chains and one work order.
- PR review clarified that failed-check crosses outrank approval locks without conflicts, while conflict triangles outrank both. The existing design already defines this priority, so no product behavior or spec contract changed.
- After the public-doc correction, `node --test scripts/validate-public-docs.test.mjs` passed 63 tests and `node scripts/validate-public-docs.mjs` validated 55 pages. `git diff --check` passed.
- `git diff --check` passed. No dependencies, provider behavior, or API requests changed.

## Risks

- Keeping both markers in the same corner can obscure one. Render exactly one badge and preserve both facts in details.
- Removing the inline span can reduce an effective touch target or expose clipping. Measure geometry and actual edge taps.
- Existing tests require simultaneous CI/conflict markers. Update those assertions while retaining their interaction and identity scenarios.
- Mixed PRs can place conflict and CI on different siblings. Keep existing attribution and terminal filtering.

## Public documentation and lifecycle

The existing status explanation in `docs/public/integrations.md` now describes conflict priority, CI fallback after resolution, and both retained detail reasons. No screenshot catalog or README update was needed.
The amended requirements are `active`, the designs are `current`, this plan is `implemented`, and its work order is `done`.
Earlier delivery statuses and results remain historical evidence.
