---
id: 01-multi-tag-grouping
title: Complete multi-tag combinations and real package proof
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-SIDEBAR-MULTI-TAG-001
acceptance_criteria:
  - AC-UI-SIDEBAR-MULTI-TAG-001.1
  - AC-UI-SIDEBAR-MULTI-TAG-001.2
  - AC-UI-SIDEBAR-MULTI-TAG-001.3
  - AC-UI-SIDEBAR-MULTI-TAG-001.4
  - AC-UI-SIDEBAR-MULTI-TAG-001.5
system_design:
  - ../../specs/ui/system-design/sidebar-multi-tag-grouping.md
---

# Task01: Complete Multi-tag Combinations and Real Package Proof

## Summary

Deliver the additive Multi-tag entry in the dedicated Tags plugin using the
existing complete generic grouping contract. Complete plugin/manifest action
coverage and real-package native desktop/phone proof in this one vertical slice.

## In scope

- Strict TDD: first write and run the behavioral plugin cases listed in plan Tests.
- Distinct manifest/action/reference, canonical nonempty subsets, stable IDs and
  live content revisions; checked expansion and unchanged action/SDK bounds.
- Preserve original Tag output and /tasks facet, shared-state privacy/no writes.
- Plugin version0.20.0, README, packaged artifact; generic fixture overlap cases.
- Actual package selectors, trees/counts/pagination and saved-state/touch proof.

## Out of scope

- Host production/SDK changes, tags schema migrations, new browser scan paths.
- Exact-set/nested alternatives, raised budgets or partial success fallback.
- Demo52121 mutation, publication/release/deploy or unauthorized commit/push.

## Acceptance

1. Two distinct discoverable normal saved groupings work without changing Tag;
   the real Multi-tag action deterministically classifies every nonempty subset.
2. Stable identities/revisions, pre-expansion limits, corruption and cancellation
   satisfy the paired design with failing-before/passing-after regression evidence.
3. All listed checks and actual0.20.0 desktop/phone package flows pass on exact
   owned source/artifact snapshots, including complete pre-page trees and recovery;
   limits and any non-green broad gates are reported rather than relabeled.

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

See [full previews and scenario matrix](plan.md#ascii-ui-preview).

## Verification

Run the following complete block from the host source root after adding the
new behavioral cases. The first make test is the RED command before production
implementation; rerun it for GREEN. No broad unrelated audit is requested.

```bash
# Start from the host repository root. No output goes into the source worktrees.
set -eu
HOST_ROOT="$PWD"
PLUGIN_SOURCE="$HOST_ROOT/../yattdev-kandev-plugin-tags-main"
case "$(findmnt -no SOURCE --target "$HOME/.cache")" in /dev/sdd*) ;; *) exit 1;; esac
export TMPDIR="$HOME/.cache/tmp"
export GOTMPDIR="$TMPDIR"
export GOCACHE="$HOME/.cache/go-build"
export GOMODCACHE="$HOME/.cache/go-mod"
mkdir -p "$TMPDIR" "$GOCACHE" "$GOMODCACHE"
PLUGIN_BUILD="$HOME/.cache/sidebar-multi-tag-build/plugin"
HOST_BUILD="$HOME/.cache/sidebar-multi-tag-build/host"
mkdir -p "$PLUGIN_BUILD" "$HOST_BUILD"
rsync -a --exclude=.git --exclude=bin --exclude=.build --exclude='*.tar.gz' "$PLUGIN_SOURCE/" "$PLUGIN_BUILD/"
make -C "$PLUGIN_BUILD" test vet KANDEV_ROOT="$HOST_ROOT"
GOFLAGS=-race make -C "$PLUGIN_BUILD" test KANDEV_ROOT="$HOST_ROOT"
make -C "$PLUGIN_BUILD" package-host KANDEV_ROOT="$HOST_ROOT"
# Use a separate build mirror: the running user demo reads the older mirror.
rsync -a --exclude=.git --exclude=node_modules --exclude=bin --exclude=.build --exclude=dist --exclude=.kandev-dev --exclude=test-results "$HOST_ROOT/" "$HOST_BUILD/"
(cd "$HOST_BUILD/apps" && pnpm install --frozen-lockfile)
(cd "$HOST_BUILD/apps/web" && pnpm e2e:run --host --project chromium tests/plugins/sidebar-plugin-grouping.spec.ts tests/plugins/task-list-facet.spec.ts tests/task/workspace-sidebar-views.spec.ts --retries=0)
(cd "$HOST_BUILD/apps/web" && pnpm e2e:run --host --no-build --project mobile-chrome tests/plugins/mobile-sidebar-plugin-grouping.spec.ts tests/plugins/mobile-task-list-facet.spec.ts tests/task/mobile-workspace-sidebar-views.spec.ts --retries=0)
PYTHONDONTWRITEBYTECODE=1 python3 scripts/list-docs.py validate
PYTHONDONTWRITEBYTECODE=1 python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/sidebar-multi-tag-grouping
```

After these commands, install the resulting0.20.0 tarball into a NEW isolated
reviewed-host runtime using the normal native upload. Use cache-drive HOME,
state, ports, browser profile and evidence; prove mock routing/no real credentials.
Drive the exact real-package flows from plan E2E tests on desktop and phone;
record source SHAs, archive SHA-256, public query counts, native interactions and
screenshots. Use the existing Playwright/browser tooling, not a new permanent
external-artifact dependency in default CI. Tear down only the newly owned
runtime/browser/scratch and retain evidence. Do not touch the existing user demo.
This manual native package smoke is a required verification step, not optional
coverage replaced by the generic fixture. Update Results with exact commands.

## Files likely touched

Paths below the plugin root are in ../yattdev-kandev-plugin-tags-main, never copied
as production source into the host:

- manifest.yaml, Makefile, README.md.
- server/actions.go, server/sidebar_grouping.go, server/sidebar_grouping_test.go.
- Optional private server/sidebar_multi_grouping.go and matching *_test.go.
- Host apps/web/e2e/tests/plugins/sidebar-plugin-grouping.spec.ts and its mobile
  counterpart/helper only for generic overlapping membership consumer scenarios.
- New package docs and the paired draft requirements/design; preserve historical
  completed implementation/review evidence in the original package.

## Dependencies

None beyond the existing completed generic host/SDK grouping contract. Read
SDKconstants/manifest/action/current plugin strict-reader tests before changing
code. Existing dependency pins require no change unless actual compatibility
checks demonstrate a concrete need; stop for design reconciliation in that case.

## Risks

- Powerset expansion must not allocate from unchecked2^n or yield partial output.
- JSON-quoted labels may exceed transport text bounds even for two tags.
- Count canonical tasks separately from repeated root/descendant occurrences.
- Respect existing runtime configuration/status publication and scope fences;
  do not add separate invalidation/retry ownership for this mode.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-multi-tag-grouping.md).
- [Design](../../specs/ui/system-design/sidebar-multi-tag-grouping.md).
- Original sidebar package, SDK decoder, Tags shared-state and action tests.
- Canonical plugin authoring guide and scoped frontend/backend conventions.

## Results

Implemented on 2026-11-18 by the user-authorized Opus implementation task
(runtime `anthropic/claude-opus-5-5`, profile `3365c84c`). All changes are
unstaged and uncommitted. Source baselines: host `3bbc8367`, Tags `b10662aa`.

Tags repository: `server/sidebar_multi_grouping.go` adds
`sidebarMultiTagSnapshot`. Before enumerating, it checks each task's subset
count, computed by bounded doubling, against the remaining value and
membership budgets. It also enforces the global 1,000-value union and the
256-byte quoted label. IDs are `set-` plus SHA-256 of the JSON sorted member-ID
array. Only singletons keep a color. `sidebarGroupingAction` now takes a
builder, so both actions share the strict reader, the cancellation checks and
the encoded 1 MiB bound. Tag output is unchanged. The manifest adds `multi-tags`
/ `sidebar-multi-tags` (authenticated, workspace, body 16); the version is 0.20.0
in the manifest and Makefile; the README documents the new view. The host adds
one generic fixture scenario to `sidebar-plugin-grouping.spec.ts`; host
production code is unchanged.

RED: `make test` failed 12 cases with `unknown action: sidebar-multi-tags`,
before any production edit. GREEN, on the final source: `make fmt` clean,
`make test vet` passed (Go plus 204 UI tests), and `GOFLAGS=-race make test`
passed. New cases: powerset memberships, stable identity/revision, expansion
bounds (9/10/64 tags, 1,000/1,001 values, 10,000/10,001 memberships, 256/257
byte labels), exact 1 MiB encoded response, cancellation, plus the existing
corrupt-state and raw-bound cases now run through both actions.

Workorder commands from the owned mirror under `~/.cache/sidebar-multi-tag-build`:
desktop chromium specs 11/11 passed; mobile-chrome specs 6/6 passed;
`list-docs.py validate`, `lint-spec-files.py --all` and `git diff --check` passed.

Package `kandev-plugin-tags-0.20.0.tar.gz`, SHA-256
`29c1b8fc589365adbf42121523647d751d1a8b3e21b8e8feffdc2dfb119c6a39`.
It was installed by native upload (HTTP 201, active) into a new owned runtime
(loopback port 53886, cache-drive HOME/state/database, `KANDEV_E2E_MOCK=true`,
onboarding listed only Mock agents, no credentials). The scripted smoke passed
67 checks, including:

- The real action returned all seven B/F/O subsets; memberships were 7/3/1/0
  for 3/2/1/0-tag roots, 111 in total. Tag output was unchanged at 4 values
  and 105 memberships.
- 103 unique tasks and 119 occurrences across two pages. The root and its
  child appeared in all 7 subsets. Selecting the root marked every occurrence.
- Keyboard selection created a normal saved view and Escape restored focus.
  Reselection reused the view. Save as copied collapsed state, and the copy's
  disclosure stayed independent after reload. Workspace journals stayed
  separate.
- Live rename and recolor kept the collapsed combination's identity. Removing
  one application and deleting a tag updated only the affected subsets.
- Ten tags on one root: Multi-tag returned 503 with an explicit
  unavailable/Retry state and no Untagged fallback, while Tag stayed usable.
  Dropping to nine tags recovered with 511 subsets. Disable/enable kept the
  saved view.
- At 320 and 393 px with true-touch emulation: trigger, choices and pager were
  at least 44 px; the choices were inside the viewport; Escape restored focus;
  the parent drawer geometry was restored; there was no horizontal overflow;
  touch pagination worked.

PNGs were inspected. This is emulation, not a physical-device test. Evidence
is in `~/.cache/sidebar-multi-tag-build/evidence`. The owned runtimes were
stopped and their state was removed. The demo on port 52121, the build mirror
it reads, and every other instance were untouched. Historical broad web and
backend failures were neither rerun nor relabeled.

Independent Sol source reviews 1, 2 and 3 reported CLEAN with zero findings on
the same implementation and all 13 working-tree file hashes. The source review
gate reached three consecutive clean rounds after the separate three-clean
plan review gate and Opus implementation. No source corrections were required.
These are read-only reviews of uncommitted changes, not fresh runtime checks
or broad-suite certification. Parent closure changes only execution status and
this receipt after the reviewed snapshot.

The recovered desktop screenshot captures loading, not a completed recovery
render. Recovery evidence is the retained action assertions and generic
recovery coverage. The 67 smoke records include screenshots and narrative
entries, not 67 independent assertions. Physical-phone and remote-CI proof
remain absent; historical broad gates remain non-green. No new commit, push,
release, deployment or user-demo upgrade was performed.
