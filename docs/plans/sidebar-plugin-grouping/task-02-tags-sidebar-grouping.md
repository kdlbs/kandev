---
id: 02-tags-sidebar-grouping
title: Tags complete grouping and real package proof
status: done
wave: 2
depends_on:
  - 01-host-sidebar-grouping
plan: plan.md
requirements:
  - REQ-UI-SIDEBAR-PLUGIN-GROUPING-001
acceptance_criteria:
  - AC-UI-SIDEBAR-PLUGIN-GROUPING-001.1
  - AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
  - AC-UI-SIDEBAR-PLUGIN-GROUPING-001.3
  - AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
  - AC-UI-SIDEBAR-PLUGIN-GROUPING-001.5
  - AC-UI-SIDEBAR-PLUGIN-GROUPING-001.6
system_design:
  - ../../specs/ui/system-design/sidebar-plugin-grouping.md
---

# Task 02: Tags Complete Grouping and Real Package Proof

## Summary

Implement the complete grouping action in the dedicated real Tags repository
and prove it against Task01's host. Keep the installed Tags 0.16.0 package and
four seeded tasks as prior evidence, not a substitute for the new package.

## In scope

- Materialize `https://github.com/yattdev/kandev-plugin-tags` separately and read
  its instructions, manifest, source/tests and package workflow before editing.
- Add the complete grouping action and manifest declaration using Task01's SDK
  response contract over the existing shared workspace document. Use deterministic
  source revisions, deduplicate human/agent applications, preserve colors/labels
  and respect deleted values. No unrelated plugin behavior changes.
- Keep the page-local /tasks facet and older private-storage display compatibility
  intact. Document that the complete sidebar grouping uses shared workspace tags.
- Bump version/minimum-host compatibility deliberately, build a real host package,
  and install it into an isolated final-host acceptance instance.
- Record exact host/plugin commits, package version/digest, installed runtime
  state, desktop/phone proofs and any limitations in Results.

## Out of scope

Publication, remote tags/releases, marketplace submission, primary instance
mutation, raw plugin database access and changes to other plugins.

## Acceptance

1. The real action returns complete, stable-revision snapshots of shared tags;
   every mutation that changes classification changes the revision, with invalid
   or incomplete data failing explicitly and no unrelated workspace data read.
2. Selecting Tag in the left sidebar creates a normal saved view; real tagged
   groups, multi-tag roots, Untagged and 101+ task page boundaries survive save,
   reload, workspace switching, tag edits and disable/re-enable recovery.
3. Desktop and phone reach the same saved view through native controls, with
   no overflow, stale-workspace groups, token-spending agents, primary mutation
   or regression in existing /tasks grouping.

## ASCII UI preview

UI-01/UI-02 from [the plan](plan.md#ascii-ui-preview), AC-001.1/.3/.6:

```text
Left Tasks selector              Phone Task views drawer
  Plugin views                     [Tag v]        [Close]
    Tag                            Layout testing (4)
                                   Mobile (1)
Tag view                           Untagged (2)
  Layout testing (4)                [Previous] [Next]
  Mobile (1)                       internal list + safe area
  Untagged (2)
```

Labels/colors come from real plugin state. Repeated tasks in matching groups
are intentional display occurrences, not fabricated duplicate tasks.

## Verification

Task01 must be complete first. From the materialized Tags repository, use the
actual Makefile entry points and a matching host checkout. Put the working
build mirror, TMPDIR, Go caches and package output on the verified cache drive:

```bash
gofmt -l ./server
make test vet package-host KANDEV_ROOT=/path/to/matching/kandev
```

Confirm `package-host` exists in the materialized repository before execution;
if the repository documents a different packaging entry point, use that real
entry point and record its exact command rather than inventing a new target.
Run existing plugin frontend tests when its repository defines them. Tests must
cover revision transitions, duplicate human/agent presence, removed tags,
empty catalog, invalid state and workspace isolation through real handlers.

Host runtime proof, after final artifacts are rebuilt:

1. Start a disposable secondary instance using automatic ports and owned
   cache-drive HOME/state; do not reuse/copy the primary database.
2. Install the real package through supported plugin installation, confirm active
   status and a visible Tag entry in the left selector, and save/select it.
3. Seed 101+ tasks without starting agents. Ensure at least one tag first occurs
   beyond the old first page, and include multiple tags and untagged roots.
   Verify group order/counts, Next/Previous, collapse and parent context.
4. Rename/recolor/apply/remove shared tags; check live sidebar and existing
   /tasks groups, then reload and switch workspace away/back.
5. Disable/re-enable the plugin and verify explicit unavailable/recovery without
   rewriting saved settings. Repeat native phone selection and pagination;
   measure menu targets >=44px and document horizontal overflow ==0.
6. Capture actual rendered desktop/phone surfaces and the installed package
   version/checksum. Leave user acceptance instance running only when requested;
   otherwise tear down owned instances on success or failure.

Re-run Task01's managed host fixture E2E commands for any host edits. The real
package smoke is mandatory even when every fixture test passes. No full-suite,
provider, publication or primary-runtime claims follow from these checks.

## Files likely touched

- Dedicated Tags repository manifest, shared tag action implementation and tests,
  frontend initialization metadata if needed, package configuration and README.
  Resolve their exact paths from that repository before editing.
- This work order Results and host public authoring/sidebar guide only if the
  contract documentation needs reconciliation; no in-tree production plugin.

## Dependencies

Task01's host/SDK action contract, final backend/UI build and fixture proof.
The dedicated repository is attached separately as
`yattdev-kandev-plugin-tags-main`. Its reconciled baseline was 0.18.0; the local
integration package is 0.19.0 with minimum host 0.102.0. Imports and package
tooling use standalone `pluginsdk/` plus `apps/backend/` from the matching host
checkout. Make targets use alternate module files rather than mutating the
checked-in module metadata. The origin runtime remains untouched.

## Risks

A workspace tag document is not the old private per-user storage layer.
Classification completeness and changes must be explicit; an error or missing
state may not silently label real tagged tasks Untagged. Validate minimum-host
compatibility without publishing an unapproved release.

## Parallelism

sequential

## Inputs

- [Requirement](../../specs/ui/requirements/sidebar-plugin-grouping.md)
- [Design](../../specs/ui/system-design/sidebar-plugin-grouping.md)
- Task01 verified contracts and dedicated plugin repository instructions.

## Results

Implemented separately in
`yattdev-kandev-plugin-tags-main`, from the dedicated repository's 0.18.0
baseline. The local integration package is 0.19.0, minimum host 0.102.0.
Its standalone SDK imports, matching-host package tooling and existing
frontend behavior are kept in that repository, not copied into this monorepo.

`sidebar-tags` reads the shared workspace document through Host KV. It validates
complete v1/v2 state, reads legacy shared v1 without migration writes, omits
deleted values, deduplicates human/agent applications and hashes canonical
classification fields for the revision. It has no private-storage fallback,
200-task truncation, host allowlist or direct plugin database access.

The formatted package passed `make test vet package-host` against the matching
host: Go tests, vet and all 204 existing frontend tests passed.
The CI sibling-checkout layout also passed `go mod tidy -diff` against the
matching standalone SDK, without module-file changes.

Final archive: `kandev-plugin-tags-0.19.0.tar.gz`, SHA-256
`18894c74969da653ae4c16adc62ad3b3d7e294d5616943ffad53fbdf998e4d76`.
Supported native file upload returned HTTP 201 and Active status in fresh
owned state. An earlier same-version reinstall correctly returned HTTP 409;
installed files were not edited and uninstall/data purge was not used.

The final host ran with development compatibility stamping from this task's
exact rebuilt source. Its own state was
`~/.cache/tmp/sidebar-tags-smoke-9e23j7wj/final-instance`, on the verified cache
drive, at loopback port 53049. Public APIs seeded 101 roots, one child, one
untagged task and an isolated second workspace, with agents disabled.
The final unloaded root was `Tags proof root 001`, absent from the same-sort
plain first 100 task rows before its tag was grouped.

Observed desktop and true-touch phone outcomes:

- 103 canonical tasks and 106 occurrences, with the unloaded tagged root first,
  inherited child trees, multi-tag counts, colors and canonical pin ordering.
- Native Tag selection created one normal saved view; re-selection reused
  `view-mwjo03mu-f1aflz`. Collapse, reload and workspace A/B journals retained it.
- Rename/recolor and create/apply/remove/delete updated the live sidebar.
  Restoring classification restored the same revision, excluding timestamps.
- Disable retained saved identity and displayed explicit unavailable state;
  re-enable recovered with a new generation and the same classification revision.
- `/tasks` still grouped only its 25 loaded rows: Child only 1, Zulu bulk 23,
  Unassigned 1. The unloaded Amber group remained absent there and present in
  the complete sidebar. A live rename updated both consumers and was restored.
- Phone emulation reported native touch and coarse pointer. Trigger/choice/pager
  heights were at least 44px; 320px containment, Escape/focus, second-page
  navigation and the nested task-switcher parent geometry passed with zero
  horizontal document overflow.

PNGs were visually inspected. Task-route proof covers the sidebar and task
switcher, not agent startup: this repository-less fixture rejects automatic
session creation, and the canonical task's workflow step stayed unchanged.
Owned runtime, browser tab and PostgreSQL server were stopped after proof.
The originating port 59390 instance and every primary instance were untouched.
At the initial checkpoint no publication, release, deployment or push occurred.
Original dedicated signed commit:
`2a122af6b5eb617b9e313c34b591cb2c15ef73da`.
This repository has no active commit hooks; tests, vet, module-layout tidy and
real package proof above were run separately. Initial workflows pinned
host commit `c42775f671cef7532065c054669c62dbd5970275`.

### Post-review corrected package smoke

The three workflow pins now use rebased host SDK commit
`d61508aa28568cc3a68ccc079004fe9b2f1b65c2`; Tags source is
`b10662aa551dcd37d7186385cc378869a06c2149`. Both branches were pushed to
the contributor forks, without release, publication, deployment or remote CI proof.
The rebuilt local 0.19.0 archive SHA-256 is:
`5eb61b13e85806054de6bbc1d5b8cf442213025f8a98b998c2bab8a9e4c664b2`.

A fresh isolated runtime accepted this archive through native file upload
with HTTP 201 and active status. Public tag actions produced three memberships;
the complete sidebar returned three canonical tasks and four tree occurrences.
The real provider's native Save as retained collapsed disclosure through reload,
and expanding the copy did not expand its source.
True-touch emulation at 320px and 393px exposed the real Tag choice at 44px,
with in-viewport bounds, persisted selection after reload and no document overflow.
The desktop collapsed-copy and both phone chooser PNGs were visually inspected.
Evidence is under the cache-drive `sidebar-grouping-review2-rvznjn03` smoke root.
This is fresh corrected-package proof, not a relabeling of the earlier 103/106
scenario. It is phone emulation, not physical-device verification.

Review closure: rounds 12/13/14 independently reported CLEAN with zero findings
on Tags `b10662aa551dcd37d7186385cc378869a06c2149` and host
`763babaf87d29b4e767f7285b409a80f792b713d`. No later plugin source or SDK change
required a new package. Recorded package/runtime proof remains dated evidence,
not a claim of fresh installation on every host-only review correction.
