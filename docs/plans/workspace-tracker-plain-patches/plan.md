---
created: 2026-10-06
status: in_progress
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
legacy_specs: []
---

# Implementation Plan: Plain workspace tracker patches

## Overview

Restore plain patch production under forced Git color settings at the four selected
workspace tracker commands. One sequential work order adds causal regressions, makes
the four option additions, and runs focused checks after ROOT explicitly releases
implementation in this same primary session. ROOT accepted the design package and
released implementation after that design turn ended.

## Evidence and ownership

Current checkout, local main and origin/main resolve to
`3763bc329c258944cc82c03885efeac417cc288c`. This is a local inventory anchor, not
an assertion about a later authoritative remote head. Initial working tree was clean.

Accepted ROOT evidence was read without modification, replay, or removal:
`/tmp/kandev-workspace-patch-color-repro_test.go`, SHA256
`a8bfaf33722a721b7ebb91820bc759f77e9fe2dbb17096a5b6b66d58c49b614f`, and
`/tmp/kandev-root-workspace-patch-color-proof-receipt.json`. Receipt handle 53682
was actually joined, exit 1: ordinary control passed; local `color.diff=always`
put presentation ANSI into actual `WorkspaceTracker.GetGitStatusWithDetails(ctx,true)`
patches while readiness and +1/-1 counts remained correct. Temporary ROOT source
and private Git fixtures were cleaned. The archived proof remains ROOT-owned.

The current `workspace_git_diff.go`, `workspace_git_status.go`, and `git_pathspec.go`
blobs match the receipt exactly: `0bf3a9540168f61b05feb38c24317e31cbac7936`,
`1b57426d93d440c5ab40590642938c2039e27c92`, and
`78d8571fe018a77605a892823c76f477601842ba`. All four selected streaming patch
calls omit `--no-color`; `internal/common/securityutil/git.go` already admits it.
The cause is Git's presentation configuration entering machine-consumed patches.

Platform owns workspace observation, exact path details and their patch contract;
Tasks owns environment binding and Workspaces owns repository/comparison context.
Amend the existing requirement and existing path-details design. The main status
design is 32,685 bytes; the existing path-details design was 10,424 bytes, so it
can hold this small extension without a new supplement. Retain prior numstat,
literal-path, mixed-facet, snapshot-recovery and standalone comparison delivery history.
No new ADR or incident specification is warranted.

Confirmed intent: plain tracker patches, literal content preservation, unchanged
state/resource contracts, four command sites only. Source and accepted proof settle
the implementation boundary; there is no unresolved material design question.

## Scope

In scope: the four selected tracker patch options, actual tracker reads, cached
fallback boundary, registered selected/aggregate HTTP regressions, and spec/plan
traceability. Preserve flattened and mixed facets, captured environment/index/HEAD,
literal pathspec overrides, routing, readiness and existing budgets/cancellation.

Out of scope: escape stripping, global Git configuration, parser redesign, output
budget changes, extra `--no-ext-diff` or textconv policy, `ShowCommit` or
`GetCumulativeDiff` rewrites, UI/API/schema, broad writer audits, adjacent defects,
delegates/tasks/tabs/model changes, installs or heavy checks during design.

## Technical approach

Add the exact `--no-color` diff option before refs and `--` at these current sites
in `apps/backend/internal/agentctl/server/process/workspace_git_diff.go`:

| Site | Comparison | Causal coverage |
| --- | --- | --- |
| `enrichUnstagedFileDiff` (line 513 at anchor) | Captured HEAD to worktree, flattened | Public tracker reads for unstaged, staged-only and mixed files |
| `enrichMixedUnstagedFileDiff` (621) | Retained index to worktree | Mixed unstaged facet with distinct layer markers |
| `enrichStagedFileDiff` (741) | Captured HEAD to index, empty flattened fallback | Existing staged-enrichment boundary, real Git, no prior flattened patch |
| `enrichMixedStagedFileDiff` (787) | Captured HEAD to retained index | Mixed staged facet with distinct layer markers |

Keep the existing streaming runner and `gitCommandEnv`; do not make a generic
`capDiffOutput` policy change. Refresh these locations against the released source
before implementation. Any material routing/admission/snapshot conflict is a ROOT
checkpoint, not permission to widen the patch.

| Consumer | Identity and transport | Intended evidence |
| --- | --- | --- |
| Public tracker | `GetGitStatusWithDetails(ctx,true)`, exact repository/path | Complete ready status, plain positive patch/count/facet results |
| Selected HTTP | Registered `GET /api/v1/git/status?repo=selected&fresh=true&details=wait` | `GetWorkspaceTrackerFor`, selected repo and decoded patches |
| Aggregate HTTP | Registered `GET /api/v1/git/status/multi?fresh=true&details=wait` | `collectStatusForRepo`, independently identified repositories and patches |
| Agentctl executors | Existing manager-owned tracker implementation | No provider expansion; existing unavailable/cancellation behavior remains |

`Server.NewServer` registration in `api/server.go` routes both reads to the real
handlers. `gitStatusResult` carries accepted `FileInfo` and facet data without
patch transformation. Use `getGitAPI` and the actual manager fixture; do not call
handlers directly or fake tracker results.

## Tests

All tests below are prospective permanent tests, not files created in this design
turn. Use small disposable real repositories, isolated system/global Git settings,
sequential subtests, bounded contexts and joined tracker/manager teardown.

| Criteria | File and exact proposed test | Positive and preservation evidence |
| --- | --- | --- |
| `.45`, `.36`, `.9` | `process/workspace_git_plain_patches_test.go`: `TestWorkspaceGitPlainPatches` | Actual details read, ordinary/explicit-disable/forced settings, unstaged/staged/mixed; exact keys, status, count, readiness, flattened and distinct facets, selected marker exclusion |
| `.45`, `.36` | Same: `TestWorkspaceGitPlainCachedFallback` | Empty flattened staged boundary executes real cached patch; exact content/counts and selected sibling exclusion |
| `.45`, `.9`, `.36` | `api/git_status_plain_patches_test.go`: `TestGitStatusHTTPPlainPatches` | Real selected and aggregate registered HTTP routes; decode ready patch data and exact repository/path/facet results |
| `.7`, `.31`, `.33` | Existing exact test selections in Task 01 | Streaming cap, byte budget, cancellation, transient failure, literal env and snapshot ownership controls |

The work order defines the configuration and assertion matrix. New behavioral tests
must fail before the four option additions for Git-generated presentation bytes,
while ordinary positive controls pass. No flag-string mocks or ANSI-wide rejection
on literal-content fixtures. Preserve existing config/ref/index/worktree semantics.

## End-to-end evidence and mobile exception

Real filesystem/index to actual tracker publication and registered HTTP decoding is
the affected end-to-end boundary. Desktop and phone consume the same data. This is
a pure-data producer flag correction with no frontend or rendered interaction change;
`mobile-parity`'s no-UI exception applies. No ASCII UI preview, browser, build,
typecheck or Playwright/E2E run belongs to this work order.

## Documentation impact

Audited `docs/public/git-operations.md`, `tasks-and-workflows.md`, `use-kandev.md`,
root README and `docs/screenshots.md`. Existing Changes/Review guidance and internal
agentctl-route reference remain accurate. The fix restores existing patches without
a user control, configuration key, terminology, screenshot or public API change.
Internal docs only; no public edits or public validator run is needed here.

## Work orders

- [ ] [Task 01: Produce plain selected workspace patches](task-01-plain-selected-patches.md)

## Verification results

Design checks on 2026-10-06:

- `python3 scripts/list-docs.py validate`: passed, 351 decisions and 1368 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- Exported `validateCoverage` preflight in Task 01: `covered`, no errors, including
  all four actual artifacts and prospective production/test paths.
- `git diff --check`: passed. Status confirms only two spec amendments and two
  untracked package files; nothing staged. Requirement/design sizes are 18,014 and
  12,585 bytes, below their limits.
- All cheap command calls returned terminal exit 0; no outstanding owned handles.
  Installed Node and cached pnpm 9.15.9 located; `apps/node_modules` is absent.
  The one frozen apps install is deferred until later ROOT release and lease.

That design turn ended before production/permanent test changes, installs, heavy
checks, commit, push or PR. Earlier packages' results are historical and unchanged.

Implementation checks on 2026-10-06 after ROOT release:

- Process RED: exit 1, package 3.467s. Nine forced-color tracker cases and three
  forced-color fallback cases failed on actual Git presentation bytes; twelve
  ordinary/disable control cases passed. All four patch sites were exercised.
- Registered selected/aggregate HTTP RED: exit 1, package 5.834s. Nine forced-color
  cases failed on actual decoded presentation bytes; nine controls passed.
- Added only four `--no-color` options. New process GREEN: exit 0, package 5.033s,
  24 sequential color/layer/fallback cases. New HTTP GREEN: exit 0, package 7.641s,
  18 sequential cases covering both real registered transports. Exact patch hunks,
  statuses/counts/facets, literal ANSI content and supported filenames, read-only
  repository evidence and captured/live environment assertions pass.
- Reviewed 12-function process compatibility selection: exit 0, package 5.907s.
  Reviewed three-function HTTP compatibility selection: exit 0, package 2.090s.
  No existing fault fixture adjustment was necessary.
- Reviewed scoped process/API lint: exit 0, zero issues, concurrency 2, serial runner,
  CLI 5m/GNU 6m bounds. Catalog/spec-lint, actual seven-file coverage preflight and
  whitespace passed. Every test/lint handle was actually joined before the next
  heavy command. Exact commands are in Task 01; logs/handles in the Kandev plan.

One pinned frozen apps install and normal hook/publication receipts remain in the
durable Kandev plan. Delivery is pending actual hosted checks/review and the later
ROOT merge lease; this record does not imply a merge.

## Risks

- Ordinary staged-only reads often fill the flattened patch before cached fallback;
  a separate real staged-enrichment boundary test is necessary to cover that site.
- Literal ANSI content must survive; absence assertions apply only to ASCII-source
  fixtures. Filename controls on Windows exclude only demonstrated invalid names.
- Snapshot/environment/admission work recently changed. Preserve it and use current
  captured-environment controls; do not reconstruct commands from the ROOT archive.
- Resource, timeout, transport, unknown or unrelated failures require ROOT direction.
  No automatic recovery, wider audit or passing broad replay is authorized.
