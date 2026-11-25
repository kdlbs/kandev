---
created: 2026-11-17
status: done
requirements:
  - REQ-UI-SIDEBAR-MULTI-TAG-001
system_design:
  - ../../specs/ui/system-design/sidebar-multi-tag-grouping.md
legacy_specs: []
---

# Implementation Plan: Multi-tag Sidebar Combinations

## Overview

Add one plugin-provided Multi-tag grouping over all nonempty subsets. The user
explicitly selected this behavior after seeing exact-set and nested alternatives.
Keep the shipped Tag mode and completed sidebar package unchanged. One vertical
work order delivers the dedicated plugin action, manifest, bounds, regressions,
package and native consumer proof; no artificial host/backend/frontend split.

## Scope

### In scope

- Dedicated Tags complete combination action and distinct saved reference.
- Canonical subset identity, labels, revisions and existing strict read path.
- Pre-allocation output budgets and unchanged generic unavailable handling.
- Existing desktop/phone native selectors, saved state and complete pagination.
- Package0.20.0 smoke in a separate owned instance, not the user's running demo.

### Out of scope

- Exact-set grouping, nested tag trees, board /tasks facet changes.
- Host or SDK production changes, host Tags allowlists, new database readers.
- Raising limits, silent truncation, new settings or telemetry.
- Release, deployment, commit/push, persistent tasks/workers or demo upgrade.

## Technical approach

Reuse the public generic contract from the completed
[original package](../sidebar-plugin-grouping/plan.md). Add manifest multi-tags
and authenticated workspace action sidebar-multi-tags in the separate Tags repo.
Implement sidebarMultiTagSnapshot beside sidebarTagSnapshot, using existing
strict reader and text/color/revision helpers without copying whole snapshots.
Preserve the existing Tag action; use checked output growth and deterministic
ID-set hashing. Update plugin README and local package version to0.20.0.
Host production code is intentionally unchanged. Extend the generic host
browser fixture tests with overlapping combination memberships where useful;
real package smoke proves actual Multi-tag discovery rather than a fixture label.

| Provider shape | Behavior | Verification |
| --- | --- | --- |
| Existing Tag | Same output/reference | Existing plugin and page-local regressions |
| Complete Multi-tag | Every nonempty subset before page slicing | Powerset tests, generic fixture, real package |
| Combination-only overflow | Explicit unavailable; Tag remains usable | Group/pair/label/byte boundary tests and recovery |
| Old/missing plugin | No new offer, saved reference retained if unavailable | Existing generic lifecycle contract and smoke |

## ASCII UI preview

### UI-01: Desktop Tasks grouping picker

```text
TASKS                       [Multi-tag v] [Filters]
  Plugin groupings
    Tag
    Multi-tag

[expanded] "Backend"                     2
  Task A (Backend, Frontend)
  Task B (Backend)
[expanded] "Frontend"                    1
  Task A (Backend, Frontend)
[expanded] "Backend" + "Frontend"        1
  Task A (Backend, Frontend)
[expanded] Untagged                       1
  Task C
[Previous]  3 unique tasks / occurrences  [Next]
```

### UI-02: Phone native grouping sheet

```text
Tasks drawer (underlying scroll owner)
+---------------------------------------+
| Plugin groupings                Close |
| Tag                                   |
| Multi-tag                             |
+---------------------------------------+
```

The sheet's choice list scrolls internally with 44px targets and safe-area
padding. Selection closes the inner sheet, restores its parent drawer geometry
and focus, and selects the normal saved Multi-tag view. Existing group list,
pager and disclosure controls remain native phone surfaces. Labels shown above
are illustrative; canonical tag-ID ordering decides actual component order.
Loading/unavailable/retry use current shared controls, not empty Untagged output.
UI-01/UI-02 map to AC-001.1 through AC-001.5 of the new requirement.

## Tests

All criterion suffixes below refer to AC-UI-SIDEBAR-MULTI-TAG-001.

- .1: new authenticated action/manifest and normal native saved view selection.
- .2: plugin sidebar_grouping_test.go new TestSidebarMultiTagPowersetMemberships:
  A/B/C gives exactly7pairs; A/B and B/C share B but not unrelated combinations;
  empty sets, duplicate human/agent rows, deleted/inactive tags and v1 inputs.
- .3: TestSidebarMultiTagStableIdentityAndRevision: input permutations, rename,
  singleton recolor, timestamps/notes, deletion and reapplication; zero Host writes.
- .4: host sidebar-plugin-grouping.spec.ts overlapping subset fixture with101+
  roots, repeated child trees across100row window, collapse, pins, canonical task
  actions, unique totals, saved copy/reload and a second workspace.
- .5: TestSidebarMultiTagExpansionBounds: 10distinct tags fail before expansion;
  many small sets cross global1000values/10000pairs independently; exact-at-bound
  and just-over cases; UTF-8/quoted label256bytes and actual encoded1MiB boundary;
  cancellation returns no successful snapshot. Existing corrupt/raw-before-dedup
  tests remain applicable, and combination-only failures do not alter Tag output.

## E2E tests

Use existing host desktop and mobile grouping specs for generic overlapping
values and leave default CI independent of the separate plugin checkout.
Additionally perform recorder-free native smoke against the actual0.20.0 package:
select Multi-tag, verify A/B/A+B repetitions, three-tag7occurrences, tree/pager
boundaries, saved selection/disclosure/reload/workspace switch, live tag mutations,
provider unavailable/recovery and keyboard/focus. Phone at320/393 uses real touch,
44px choices, internal scrolling and no horizontal document overflow. Inspect PNGs.

## Work orders

- [x] [Task01: Complete plugin combinations and package proof](task-01-multi-tag-grouping.md)

## Verification results

Implemented by Opus with failing-first plugin regressions and real 0.20.0
package smoke. Targeted plugin test/vet/race, desktop 11/11 and mobile 6/6
E2Es passed. The separate plan and source Sol review gates each reached
three consecutive clean rounds. See the work order Results for exact source,
archive and retained evidence; historical broad gates remain non-green.

The later post-rebase review gate remains open. Round 3's eight findings now
have corrections and regressions; see the work order's corrective receipt.
Rebuilt desktop acceptance passed 11 cases, then the final rebuilt three
boundary cases and four phone cases passed after lint cleanup. Fresh real
0.20.0 installation, desktop and 320/393 touch smoke passed, including readable
control names and a different-group draft. This does not advance the source
clean streak; Sol round 4 reviews the corrected commits independently.

Round 4's eight corrections have targeted regressions. Rebuilt desktop
acceptance passed 17/17 and true-touch phone emulation passed 8/8. A fresh
actual 0.20.0 install passed separate desktop/320/393 smoke, including
persisted changed-group previews and distinct escaped headings. The work order
records failing attempts and the final receipts; this does not advance the
independent source-clean streak. Round 5 reviews the published corrective heads.


## Risks

- Exponential growth is intentional; ten tags already exceed the existing value
  budget. Fail closed before expansion, without weakening the requested semantics.
- Quoted combined labels can exceed256UTF-8bytes; reject, do not silently shorten.
- Keep subset identity stable when names/colors change; avoid delimiter-based IDs.
- Existing SDK pins/host floor are reused, not a reason to publish a new SDK.
- The current persistent user demo at52121 must remain unchanged. Use fresh
  cache-drive build/runtime/browser state; no automatic restart/upgrade.
