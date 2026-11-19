---
created: 2026-11-16
status: done
requirements:
  - REQ-UI-SIDEBAR-PLUGIN-GROUPING-001
system_design:
  - ../../specs/ui/system-design/sidebar-plugin-grouping.md
legacy_specs: []
---

# Implementation Plan: Complete Plugin Grouping in Sidebar Views

## Overview

Deliver Tag through the left sidebar view selector as a normal saved view.
User chose complete pre-pagination grouping, not a loaded-page approximation.
Build the generic host/SDK contract and fixture first, then integrate and smoke
the dedicated real Tags plugin against that host. The user's later explicit
transfer request authorizes implementation of this imported package here.

UI owns personal sidebar organization. The existing workspace-view and bounded
pagination contracts remain authoritative; the plugin owns classification data.

## Continuation context

Original plugin layout-profile work is complete through implementation
`9ed5fa74ff` and review-closure `14204bc2f5`; do not reopen it or revive a retired
Prompt History component. Secondary acceptance instance was started from the
latter commit, with separate cache-drive state at
`/home/clem/.cache/plugin-layout-acceptance/repo/.kandev-dev`, app/backend
`http://127.0.0.1:59390`, and Vite `http://localhost:37429`.
Tags 0.16.0 is active there, with four sample tasks and three shared tags.
The actual /tasks Group menu offered Tag and rendered groups of 4, 1 and 1;
the left sidebar selector did not. This is runtime evidence of the scope gap,
not acceptance of this proposed extension. Leave primary runtime/state untouched.

## Scope

### In scope

- Generic complete grouping manifests/actions/SDK and authorized query resolution.
- Normal workspace-scoped saved views and direct Plugin views selector entries.
- Complete grouping before bounded pagination, multi-group trees and counts.
- Desktop/phone controls, persistence and provider lifecycle/failure behavior.
- Matching real Tags package and isolated consumer proof.

### Out of scope

- Page-only grouping disguised as complete grouping; whole-task browser scans.
- Plugin-specific host allowlists; direct reads of plugin databases/state schemas.
- Reworking /tasks sorting, new tag filters, manual task mutation or layout panels.
- Primary instance changes, publication, marketplace release and deployment.

## Technical approach

Follow the paired design's new `sidebar_groupings` manifest contract, verified
snapshot DTO and server-resolved plugin group reference. Use existing action
transport/identity and existing workspace view journals; no new database table
or separate plugin server. Make source revision/lifecycle part of retained query
eligibility. Preserve tree identity and parameterized SQL grouping before slicing.

| Provider shape | Behavior | Evidence |
| --- | --- | --- |
| Built-in grouping | Existing query/local path unchanged | Existing sidebar suites |
| Page-local facet only | /tasks works, no sidebar offer | Registry/selector negative control |
| Complete declared action | Saved view and full paginated grouping | Fixture + real Tags |
| Unavailable/incomplete action | Saved identity retained, explicit error | Failure/lifecycle schedules |

The generic host never recognizes a Tags ID or shared-tags JSON shape.

## ASCII UI preview

### UI-01: Desktop left Tasks header and expanded selector

```text
TASKS                 [All tasks v] [Filters]
  All tasks
  My saved view
  -------------------
  Plugin views
  Tag
  -------------------
  + New view

TASKS                       [Tag v] [Filters]
[-] Layout testing (4)     [color swatch]
    Verify saved layouts
    Check preview proportions
    ...
[-] Mobile (1)             [color swatch]
    Exercise mobile editing
[-] Untagged (2)
    ...
[Previous]   Page 1   [Next]
```

### UI-02: Phone Task views/task-switcher drawer

```text
+--------------------------------+
| Task views                Close|
| [Tag v]                [Filters]|
|--------------------------------|
| Layout testing (4)              |
|   Verify saved layouts         |
| Mobile (1)                     |
|   Exercise mobile editing      |
| Untagged (2)                   |
|       internally scrolling     |
|--------------------------------|
| [Previous] Page 1 [Next]        |
+----------- safe area ----------+
```

UI-01/UI-02 cover AC-001.1/.2/.3/.6 under this package's requirement ID.
Fixed header/navigation, one internal list scroll owner and safe-area-aware
phone actions are structural; spacing/count examples are illustrative.
Selected unavailable providers remain named with an error/Retry and other-view
selection, not empty Untagged groups. Draft protection and view-limit disabled
states reuse existing explanations. Labels go through localization. No new
phone-only persistence; preserve ordinary 28px desktop/44px phone controls.

## Tests

- Generic manifest/action validation, authorization, cancellation and response bounds.
- Real SQLite pre-pagination grouping: 101+ tasks, off-page first membership,
  multi-group occurrence counts, parent context, collapse, foreign IDs and pins.
- Workspace view save/reload, invalid/missing provider refs, stale revisions and
  delayed old-workspace/plugin-generation replies.
- Mounted picker/group editor, new-view dedup/name collision/draft/50-view guards.
- Match server/local query eligibility; no plugin query uses incomplete local data.
- Tag action tests use the real shared document and mutation revisions.

## E2E tests

New host fixture files:
`apps/web/e2e/tests/plugins/sidebar-plugin-grouping.spec.ts` (chromium) and
`mobile-sidebar-plugin-grouping.spec.ts` (mobile-chrome).
Cover selector -> save -> reload/workspace switch, 101+ task pages, tag updates,
provider disable/re-enable and phone menu/list containment. Keep existing
`task-list-facet.spec.ts`, mobile facet and workspace sidebar suites as controls.
Task02 separately installs the real packaged Tags plugin and repeats the route;
fixture proof is not real-package proof.

## Work orders

- [x] [Task 01: Generic complete sidebar grouping](task-01-host-sidebar-grouping.md)
- [x] [Task 02: Tags integration and real package proof](task-02-tags-sidebar-grouping.md)

Sequential: Task02 consumes Task01's verified contract and runtime. No agent or
model assignments, parallel delegation, release or publication are implied.

## Verification results

Core implementation and exact-source real-package proof are complete in this
independent workspace. The initial main baseline was
`b4c6a197129b5b4a5e99c3e1dab132ada9fb433f`; originating layout-profile commits
were not imported. The original design bundle remains transfer history, not
acceptance evidence for this implementation.

Observed current checks:

- Required `make fmt`, complete `make lint`, SQL guard, standalone SDK tests/vet
  and final backend/UI artifact builds passed.
- The full affected SQLite repository suite passed in `945.907s` with a
  30-minute package deadline. Four complete-grouping cases also passed on both
  SQLite and real PostgreSQL 18.6, plus the verified-snapshot guard, without
  skipping the PostgreSQL cases.
- Web scoped verification passed 559 tests in 49 files and native typecheck.
  Current-artifact managed Playwright passed 7 desktop and 6 phone cases,
  including existing page-local facet and workspace controls.
- Real Tags 0.19.0 passed Go tests, vet and all 204 frontend tests. Native upload
  of the final SHA-256-identified archive returned HTTP 201 and Active status.
- Desktop and true-touch phone proof covered 103 unique tasks / 106
  occurrences, an unloaded tagged root before pagination, canonical pins,
  inherited/repeated child trees, saved-view reuse, collapse/reload/workspace
  journals, live edits and unavailable/recovery. Phone controls measured 44px
  with zero horizontal overflow, including the nested task switcher.
- `/tasks` remained page-local: 25 loaded rows in three sections, with the
  unloaded Amber group absent there and present in the complete sidebar.
  A real live rename updated both consumers.

The initial requested broad command was executed but was not green: classic
TypeScript exhausted its default 4 GiB heap, SQLite exceeded its default
10-minute package deadline, and backend failures were reported in MCP-auth
aggregation, agent-update maintenance jobs and worktree lock policy. No
baseline attribution or full-backend pass is claimed. Classic typecheck later
passed with an 8 GiB heap and one workspace at a time; the affected SQLite
deadline was resolved by its complete 30-minute run. Resource-constrained
combined browser checks were replaced by passing serial runs.

The complete supported one-worker web rerun finished: 30,898 tests passed,
16 failed and 4 were skipped across 3,390 files. The five failing files cover
HTTP Git/Docker fixtures, seed-origin Git error evidence and GitLab/Linear/Sentry
dialog lifetimes. These are not sidebar acceptance passes and are not asserted
to be baseline-only failures.

Final classic typecheck passed for all five TypeScript workspaces with an 8 GiB
heap and workspace concurrency one. CLI tests passed. Complete `make test-scripts`
passed after using the existing Rust 1.92 toolchain and building the desktop
startup Vite prerequisite; signed-updater verification and rendered startup
checks were exercised, not skipped. Specs, all 55 published docs pages and
complete locale checks passed.

The requested broad test gate remains non-green for the backend and web failures
above; focused passes do not replace that result.

## Delivery checkpoint

Host: `c42775f671cef7532065c054669c62dbd5970275`, parent
`b4c6a197129b5b4a5e99c3e1dab132ada9fb433f`.
Tags: `2a122af6b5eb617b9e313c34b591cb2c15ef73da`, parent
`61a7e99770801d7e4f027bbf1cee2d2154cee0d7`.
Both are normal signed commits under the user's configured identity. All
applicable host hooks passed; Tags has no active repository hooks. Host auto-GC
reported shared remote-ref lock contention after commit success; no lock or
remote ref was removed.

The earlier 91% cache blocker cleared on November 17 (31% observed before
review); no active runtime files or today's caches were deleted to resolve it.
Sol review round 1 reported three defects: transient catalog failure erased
eligible metadata, nullable application rows looked empty, and raw duplicate
applications bypassed the limit. All three now have failing-before/passing-after
regressions. The native desktop failure was also reproduced against a fresh
pre-fix build; final corrected browser/package acceptance is being completed.

The user authorized commit, rebase and push before continuing the Sol loop.
Original port 59390 and primary instances remain untouched. Publication is
authorized for the feature branches only, not a release or deployment.

## Adversarial review closure

The requested Sol review/fix loop stopped after rounds 12, 13 and 14 each
reported CLEAN with zero findings on the same immutable source snapshots:
host `763babaf87d29b4e767f7285b409a80f792b713d` and Tags
`b10662aa551dcd37d7186385cc378869a06c2149`. Each reviewer verified the exact
Sol runtime and clean initial/final worktrees. Earlier findings were corrected
with failing-first coverage; the work orders record the individual corrections.
Both feature branches are pushed to the contributor forks. This closure changes
documentation only; it does not extend source-review certification to new code.

Latest affected-package verification passed full plugin `-race` (`26.695s`),
required formatting, Go lint, document validation and normal signed hooks.
Rebuilt native acceptance passed seven desktop and five phone scenarios with
one worker and no retries. Corrected real Tags package installation and touch
proof are recorded separately in Task 02, not relabeled as a new-head reinstall.
Implementation and the requested review loop are complete; the historical broad
backend/web gates remain non-green. No physical-phone, remote-CI, release or
deployment certification is claimed. The user retains the Test-step transition.

## Post-rebase round 3 checkpoint

The subsequent post-rebase review gate is separate from the historical closure
above. Round 3 reported eight findings; corrections and failing-first coverage
are recorded in [the Multi-tag work order](../sidebar-multi-tag-grouping/task-01-multi-tag-grouping.md#post-rebase-round-3-corrections).
Complete provider identities now reconcile disclosure, normal saved-view writes
use bounded CAS deltas, and selection ranges retain rendered occurrence identity.
Post-RPC workspace authorization and committed record/epoch captures fence reads.
The new review streak remains zero pending independent review of these changes.


## Risks

- Group/task occurrence keys must not collide in virtual lists or tree actions.
- Complete snapshots must be generation/revision-fenced, never silently truncated.
- Public SDK, manifests, boot metadata and backend settings normalization must agree.
- Keep Tags production source in its materialized dedicated repository. Installed
  package versions are immutable and are not an editable production source.
- Existing unavailable-provider page retention must not leak across contexts.
- New SDK/manifest compatibility must be checked against the actual plugin minimum
  host version; publishing a version is a separate authorized operation.
