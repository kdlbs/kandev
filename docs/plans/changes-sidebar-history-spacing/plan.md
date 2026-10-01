---
created: 2026-10-01
status: completed
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001
  - REQ-UI-BOUNDED-CHANGES-001
system_design:
  - ../../specs/tasks/system-design/remote-contribution-relation.md
  - ../../specs/ui/system-design/bounded-changes-rendering.md
legacy_specs: []
---

# Fix Plan: Changes Sidebar History and Spacing

## Overview

Uncertain provider/upstream snapshots now keep unified history and withhold
remote mutations until current evidence establishes the relation. Section
headers use compact desktop geometry and touch-sized phone controls. Both work
orders were implemented sequentially under the existing requirements.

## Evidence and root causes

The report shows one unstaged file, collapsed PR Changes, Local checkout commits,
and PR #4141 version, with large gaps between history headers.

A direct Node 24 import of the real `classifyRemoteContribution` reproduced:

| Input | Result before the repair |
| --- | --- |
| Different provider/upstream heads, local head outside the PR list, upstream ahead 2/behind 0 | Diverged, separate, replacement and restoration enabled |
| Equal provider/upstream heads, unequal local head, upstream counts 0/0 | Same incorrect result |
| Equal provider/upstream heads, upstream ahead 2/behind 1 | Same result, correctly confirmed |

The classifier's final return treats everything after a few special cases as
diverged. It does not require evidence of unique commits on both sides against
the current provider head. An existing test explicitly expects divergence from
an unrelated upstream and must be corrected to the active ancestry contract.

A subsequent disposable real-Git reproduction established a reachable false
positive, rather than relying only on hypothetical inputs. The linear graph was
`base -> A -> B -> C`, with cached upstream at A, current provider head at B,
complete provider commits A/B, and checkout at C. Git reported 2/0 against the
cached upstream and 1/0 against the current provider head. The unchanged
classifier nevertheless returned `diverged`, separate presentation, and enabled
replacement/restoration. The temporary repository was removed. This proves a
classifier defect under asynchronous snapshots, but does not establish that the
reported PR #4141 occurrence was false; genuine rewrites can correctly produce
the same groups. Intermittency alone distinguishes neither case.

`HistorySectionRow` independently carries `pb-3`, `mb-1`, and `-mt-0.5`
around a minimum-24px disclosure. Its expanded collection is now rendered as
separate virtual rows, so padding that belonged after the old whole section is
charged to the header. Working-tree section headers lack the same bottom
padding. This explains the different collapsed-header density in the screenshot.
Existing commit-spacing E2E asserts adjacent wrapper geometry, which cannot
detect excessive padding inside each wrapper.

Git history attributes the unconditional divergence fallback to `c4b4791de`
(PR #2509, 2026-08-12). `8ca01e0f7` (PR #2560, the same day) subsequently
enabled replacement/restoration for that classification. The spacing change
comes from `0be0fd65a` (PR #4071, 2026-09-30): it copied pre-existing section
padding into the virtual header and added a minimum-24px disclosure where the
old desktop header had no minimum height. The padding itself predates that PR;
its header-only ownership and increased collapsed height are the regression.

The complete requested-source frontend diagnostic bundle did not contain the
Git heads/counts for PR #4141. Its exact relation is unverified. Source and the
classifier reproduction establish the defects, but do not prove that this PR
has no real divergence. No live browser interaction or Git mutation occurred.

## Ownership and scope

Tasks owns version classification under AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.4
and retained provenance under .7. UI owns reusable contiguous geometry under
AC-UI-BOUNDED-CHANGES-001.6 and phone access under .8. No new product requirement
or ADR is needed.

In scope: the classifier fallback, tests through the hook and rendered groups,
working-tree/history disclosure geometry, estimates, and focused desktop/phone
regressions. Excluded: automatic fetch or Git recovery, provider expansion,
changing real divergence semantics, global control sizing, and another generic
virtualizer measurement rewrite.

## Technical approach

1. In `remote-contribution-relation.ts`, classify divergence only when the
   upstream snapshot equals the current provider head and both upstream-relative
   counts are positive. Return unknown for unsupported/inconsistent evidence.
   Preserve earlier aligned, provider-ahead, and local-ahead proofs and the
   existing action-policy gate. Update the old mismatch expectations.
2. In `changes-timeline-history-row.tsx` and
   `changes-timeline-working-tree.tsx`, give section disclosures one density:
   28px on fine-pointer desktop and at least 44px on phone/coarse pointers.
   Remove history-only bottom padding and header negative margins. Keep dots,
   counts, labels, and actions inside the measured row. Update the matching
   `section` and `history-section` estimates to 28/44. Let positive measured
   heights handle wrapping and font scaling. No extra section spacer is needed
   for this repair.

### Compatibility matrix

| Surface / evidence | Transport / identity | Behavior | Evidence |
| --- | --- | --- | --- |
| GitHub Changes | Provider commits resource + selected repository/branch status | Current head governs classification | Pure, hook, desktop/phone E2E |
| Ordinary repository | No selected PR | Existing local commit behavior | Classifier control |
| Provider failure or stale upstream | Existing unavailable-evidence policy | Unified usable history, remote mutation gated | Pure and rendered tests |
| GitLab/plugin without complete supported head evidence | Existing capability boundary | No new drift UI support | Preserve existing exclusions |

### Mobile contract

Entry: task bottom navigation's Changes action. Exemplar:
`mobile-large-changes-virtualization.spec.ts` and the shipped task mobile layout.
Keep its full-height focused Changes surface because files and history are
primary dense content. Fixed header/navigation, one `PanelBody` scroller,
dynamic height, safe-area behavior, and shared classification remain in place.
Header taps expand groups; file taps open diffs; visible actions remain reachable.
Phone widths below 768px and coarse pointers require 44px header targets,
including a narrow fine-pointer viewport. Long labels may wrap within the
surface. Resizing never overwrites the saved desktop layout preference.

## ASCII UI preview

UI-01: Changes list, desktop, uncertain relation:

```text
o UNSTAGED (1) v
  docs/plans/...  task-01-...
o PR CHANGES (11) >
o COMMITS (2) >
```

UI-02: Changes list, desktop, confirmed divergence:

```text
o UNSTAGED (1) v
  docs/plans/...  task-01-...
o PR CHANGES (11) >
o LOCAL CHECKOUT COMMITS (1) >
o PR #4141 VERSION (1) >
```

Each collapsed desktop disclosure occupies 28px at the standard root font;
expanded descendants follow without section-bottom padding before the first row.
Counts and filenames are illustrative, not identity equivalence.

UI-03: Phone, Changes selected, same relation states:

```text
| Changes                 [...] | fixed
| o UNSTAGED (1) v              |
|   task-01-long-file-name.md    | scroll body
|   docs/plans/...              |
| o PR CHANGES (11) >           |
| o COMMITS (2) >              |
| Chat   Changes   Files       | fixed
```

Confirmed divergence substitutes the two history groups from UI-02. Phone
headers have at least 44px active height, with no hidden actions or horizontal
page overflow. UI-01/02 map to 001.4 and UI geometry .6; UI-03 also maps to .8.
Existing localized labels remain authoritative. ASCII gaps are illustrative.

## Tests and E2E

Task 01 covers the classifier table, hook scope, retained provenance, and
desktop/phone group/action behavior. Task 02 adds rendered bounds for collapsed
PR and commit headers, expanded first descendants, resizing, and remounting.
New focused suites are `changes-history-regression.spec.ts` and
`mobile-changes-history-regression.spec.ts`; each work order owns its scenario.
Run existing virtualizer/measurement tests to preserve bounded rendering.

## Work orders

- [x] [Task 01: Require current divergence evidence](task-01-current-divergence-evidence.md)
- [x] [Task 02: Normalize section header spacing](task-02-section-header-spacing.md)

## Verification results

Classifier reproduction: three cases executed against the unchanged real
function with Node 24. Documentation validation passed: catalog validation
(339 decisions, 1,282 specifications), 36 specification-linter tests, full
specification lint, and diff whitespace checks. The repository's
`validateCoverage` preflight passed for both work orders with their planned
runtime paths, and every existing unit-test path in the commands was checked.
Workspace dependencies were installed from the frozen lockfile without manifest
or lockfile changes. The combined eight-suite unit run passed 76 tests. Five
desktop scenarios (including narrow fine-pointer and coarse-pointer controls)
and three phone scenarios passed across the focused runs. Existing desktop
and phone commit-spacing regressions each passed, for ten distinct browser
scenarios. Typecheck, targeted ESLint, and formatting checks passed. Public docs
validation passed for all 47 pages. Desktop and phone screenshots were inspected
against the previews; captured review files are in `/tmp/kandev-history-review`.

At 1040px, a narrow desktop panel wrapped the long local-history label to
49.5px. The regression permits measured growth while requiring the wrapper to
match the control and have no leading offset. Coarse-pointer coverage uses the
1040/1280px workbench; at 768-1023px the existing touch tablet composition exposes
diff review rather than this history summary. No tablet navigation change was
introduced by this repair. The user authorized PR publication, CI/review
remediation, and merge after local verification.

Final `validateCoverage` preflight passed with both completed work orders and
the three changed production paths. Catalog/specification lint and diff
whitespace checks passed after the delivery-status updates.

Publication preflight updated the branch to the latest `main` without conflicts
or overlap with the repair. The focused unit run again passed 76 tests, and
typecheck and targeted ESLint passed. The rebuilt managed Chromium run passed
all six desktop scenarios; the matching phone run passed four scenarios.
Fresh desktop/phone screenshot manifests were captured. The current catalog validates 339 decisions and
1,284 specifications; public-doc and specification validation passed.

## PR review follow-up

Repository sub-header disclosures also use the phone-width minimum. A rendered
multi-repository fixture reproduced the missing minimum at 24px; the regression
now verifies at least 44px, the center hit target, and collapse clickability.
The selected-repository hook fixture now replaces its status array immutably
before rerendering. All 76 focused unit tests, six desktop and four phone
scenarios, typecheck, and lint checks passed. Exact-head remote evidence is
tracked after the fixup push.

## Risks

- Stale upstream evidence can legitimately postpone comparison until ordinary
  status/provider refresh provides a matching snapshot. Do not hide this by
  weakening action gates or silently fetching.
- Real force-push/local-rebase cases must remain separate when proven.
- Narrow translated labels and root-font scaling must remain dynamically
  measured; fixed estimates are not fixed row heights.
- The screenshot's PR may be truly diverged; the repair removes false proofs
  and excessive header padding, not all appearances of the local-history group.
