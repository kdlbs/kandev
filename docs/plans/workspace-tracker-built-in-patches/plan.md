---
created: 2026-10-06
status: in_progress
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
legacy_specs: []
---

# Implementation Plan: Built-in workspace patches

## Overview

Keep bounded workspace patch enrichment independent of external diff commands.
Platform owns the shared observation/detail contract; Tasks and Workspaces retain
environment and repository identity. Extend the existing owner pair with criterion
`.46`; retain all prior criteria, including `.45` plain-color behavior.
One sequential work order proves real tracker and registered HTTP failures, adds
four local options, and verifies the affected boundary after a later ROOT release.

## Design checkpoint and evidence

DESIGN ONLY in the same primary session. Four unstaged/uncommitted artifacts are
the complete package: existing requirement, existing design, this manifest and
one work order. No implementation, permanent fixtures, install, Go/product tests,
build, browser, DB, staging, commit, push, PR, agents, tasks, tabs or model switch
is authorized in this turn. End at the package handoff. Callback queue is full;
checkpoints belong to the existing MCP plan and primary conversation, without retry
or approval prompts. Implementation requires a later ROOT interrupt and exclusive
local-heavy lease. This package does not release that lease.

Task `6a424473-34af-4f9f-b363-13807d35cd06`, session
`792940c7-7041-41d6-b50d-064f6ae5bd85`, ROOT parent
`14825981-b175-411d-999a-31ddc2aa5fc3`; persistent child43 in the THREE-small-fix loop.
Actual clean design base: `5010081464673fb41c163c06a64926308cb15d43` on
`feature/keep-workspace-patch-9e3`, after predecessor workspace39 PR4242 merged.

Accepted ROOT diagnostic: `/tmp/kandev-workspace-external-diff-repro_test.go`, SHA256
`06cfd2c1021a04e9e08b2203bfecc8f442229db91dd742ca570e076be1c9134b`;
receipt `/tmp/kandev-root-workspace-external-diff-proof-receipt.json`, log
`/tmp/kandev-root-workspace-external-diff-proof.log`. ROOT actually joined handle78683,
exit **1**, package 0.314s: ordinary built-in patch and executable configured helper
positive controls passed; actual details returned ready +1/-1 with
`external-helper-output` instead of built-in hunks and recreated the helper sentinel.
This is a meaningful regression, not an infrastructure/setup failure. Receipt base
`8776877048ea029b8f861b4d658be719dfbadff2`, diff source blob
`0bf3a9540168f61b05feb38c24317e31cbac7936` are historical.
The proof remains read-only, unreplayed and unmodified.

Current source is different: blob `400b7638238ed3381ab9c77a70cf967e49ee63d0`.
Predecessor added four `--no-color` options; all four selected commands still omit
`--no-ext-diff`. ROOT's current-main audit is
`/tmp/kandev-root-workspace-external-diff-current-main-audit.json`; this design also
inspected the actual current producers, captured environment, fallback and routes.
Do not reuse the Linux-specific diagnostic as a permanent fixture.

## Scope

### In scope

- Four exact `--no-ext-diff` diff options at selected tracker patch producers.
- Portable real-Git tracker, cached fallback, helper/budget/cache controls and
  registered selected/aggregate status-details transport regressions.
- Existing color/literal-path, mixed-facet, snapshot, cancellation, budget and
  exact validator-admission compatibility checks.
- Internal owning specification and delivery records.

### Out of scope

Shared helper/global configuration/environment policy; security validator changes;
numstat or diff-driver expansion; textconv; standalone/history/cumulative producers;
parser, status, schema, API, frontend or UI-framework changes; larger budgets;
production TestMain or shared-fixture redesign; adjacent defects and broad passing
replays. No public-guide changes are justified by this repair.

## Technical approach

Add the exact option before refs and `--`, preserving each current `--no-color`
option and literal filename argv in `process/workspace_git_diff.go`:

| Producer at base | Comparison | Required causal evidence |
| --- | --- | --- |
| `enrichUnstagedFileDiff`, line 513 | Captured HEAD to worktree, flattened | Actual details read, unstaged/staged/mixed |
| `enrichMixedUnstagedFileDiff`, line 621 | Retained index to worktree | Distinct mixed unstaged hunk |
| `enrichStagedFileDiff`, line 741 | Captured HEAD to index, cached fallback | Empty flattened patch at real staged-enrichment boundary |
| `enrichMixedStagedFileDiff`, line 787 | Captured HEAD to retained index | Distinct mixed staged hunk |

`WorkspaceTracker.capDiffOutput` uses `gitCommandEnv` and selected-command
literal/case overrides, then the existing capped stream. Keep that path, captured
`SetGitEnvironment` copies/generation and retained index context. Do not remove
`GIT_EXTERNAL_DIFF` from captured environments to achieve the result. The exact
flag is already in `securityutil.IsKnownSafeGitFlag`'s exact allowlist, with admitted
and rejected-variant tests. No new validator policy or extra Git command is needed.

| Boundary | Identity/transport | Intended behavior/evidence |
| --- | --- | --- |
| Public tracker | `GetGitStatusWithDetails(ctx,true)` | Complete ready built-in patches with exact repository/path and independent facets |
| Selected HTTP | Registered `/api/v1/git/status?repo=selected&fresh=true&details=wait` | Real manager `GetWorkspaceTrackerFor`, decoded selected data |
| Aggregate HTTP | Registered `/api/v1/git/status/multi?fresh=true&details=wait` | `collectStatusForRepo`, independent repositories, identical filenames with distinct markers |
| Cache/replay | Same tracker and registered status routes | Accepted data remains valid; dirty fresh observation replaces it under existing source ordering |
| Executors | Existing manager-owned tracker/environment capture | No provider change; current unavailable/cancelled results retained |

`api/server.go` registers both routes; `api/git.go` joins the same details pipeline
and `gitStatusResult` serializes accepted `FileInfo`/facets. Errors and partial
aggregate failures remain current contracts. UI diff consumers receive these
strings unchanged; repair their producer rather than introduce frontend filtering.

## Source inventory at actual design base

Paths below are repository-relative; `process/` and `api/` abbreviate
`apps/backend/internal/agentctl/server/process/` and `server/api/` respectively.
The work order owns only the four producer additions and prospective test files;
other source entries are read-only compatibility inputs.

| Path | Actual Git blob |
| --- | --- |
| `process/workspace_git_diff.go` | `400b7638238ed3381ab9c77a70cf967e49ee63d0` |
| `process/workspace_git_cmd.go` | `8389cb042533b23dd9283fa53034cafdf7c26b0a` |
| `process/workspace_tracker.go` | `c6d10c3846f9976a7f96d75c313840ba5d998df2` |
| `process/workspace_git_status.go` | `1b57426d93d440c5ab40590642938c2039e27c92` |
| `process/manager.go` | `190e70df73eb3f24876a88d1ee330a09b3982113` |
| `process/manager_submodules.go` | `d5f903d9e671a94527ff618d6cbcc26f7b927f0f` |
| `process/git_log_external_diff_test.go` | `b433ebbd4b2cd97916ad8a1f9c197c4a34b7c6ce` |
| `process/workspace_git_plain_patches_test.go` | `0cd6572e7b4ffe1fcc63b3a56bcca510ea00370e` |
| `api/git.go` | `92c72e52981189101110e7db91aff63e8976a231` |
| `api/server.go` | `15b840b07aa4caae7504592278c0fe4a66298db1` |
| `api/git_external_diff_test.go` | `b4a887241f0292ea3b95cdaeaf5daafe88cc670f` |
| `api/git_status_plain_patches_test.go` | `222b27182e22968cfe253e40b0656e1a18000403` |
| `apps/backend/internal/common/securityutil/git.go` | `9d75944cd84c9512ac0c23795a10dbab2f5ebb39` |
| `apps/backend/internal/common/securityutil/git_test.go` | `04ecde448fb954e7addc84d8374cd0b4050957a3` |
| Owning requirement, before design edits | `a7eb115d3f00791fcaa7e5a4ce8738d51d1494e7` |
| Owning design, before design edits | `527e7947e164db13f658d7051e25389dee2b2174` |

## Tests

Prospective tests below are created only after later reviewed release. Use the
existing native test-binary helper patterns, not shell scripts, fake patch output
or flag-string assertions. Actual positive controls must prove the selected
configured/env mechanism before sentinels are removed. Task 01 specifies the
assertion/configuration matrix and exact commands.

| Criteria | Prospective/existing evidence |
| --- | --- |
| `.46`, `.9`, `.36`, `.45` | `process/workspace_git_external_diff_test.go`: `TestWorkspaceGitExternalHelpers`, ordinary/configured/environment/both, unstaged/staged/mixed, exact hunks/status/facets/readiness/read-only evidence |
| `.46`, `.36`, `.45` | Same: `TestWorkspaceGitExternalHelperCachedFallback`, actual fourth producer under every helper mode |
| `.46`, `.1`, `.2`, `.21` | Same: `TestWorkspaceGitExternalHelperCacheAndDirtyReads`, accepted cached patch and changed-worktree fresh patch |
| `.46`, `.7`, `.8`, `.9`, `.31` | Same: `TestWorkspaceGitExternalHelperBudgets`, actual built-in truncation/total-budget behavior with helpers configured and captured |
| `.46`, `.9`, `.36`, `.45` | `api/git_status_external_diff_test.go`: `TestGitStatusHTTPExternalHelpers`, real selected/aggregate routes with independent repositories and decoded built-in patches |
| `.7`, `.9`, `.31`, `.33`, `.36`, `.45`, `.46` | Existing anchored tests in Task 01: streaming/carry-forward/mixed budgets, cancellation, snapshot replacement/recovery, color/literal-env and exact validator admission |

## End-to-end evidence and mobile exception

Real index/worktree to tracker publication and registered HTTP decoding is the
changed end-to-end boundary. Existing `ReviewFileDiffContent`, file-diff viewer,
shared Git status projection and desktop/phone Changes surfaces use the same data.
No rendered layout, touch, scrolling, navigation, copy or viewport behavior changes.
The `mobile-parity` pure-data exception applies; tracker/HTTP tests are sufficient.
No ASCII UI preview, browser, web build, typecheck or Playwright/E2E is required.

## Documentation impact

Audited `docs/public/git-operations.md`: Changes inspection, preserved data during
refresh and internal agentctl HTTP guidance remain accurate. This restores existing
patch data without a new user action, key, public API, terminology or screenshot.
Internal docs only; no fabricated public edit or public validation run. Existing
plain-patch and exact-path packages remain historical and are not reopened.
This is a local producer constraint adequately recorded in the existing design;
no new ADR or system boundary is needed.

## Work orders

- [ ] [Task 01: Produce built-in selected workspace patches](task-01-built-in-selected-patches.md)

## Verification results

Design checks on 2026-10-06:

- `python3 scripts/list-docs.py validate`: passed, 351 decisions and 1369 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- Task 01's exported `validateCoverage` preflight: `covered`, no errors, all four
  actual artifacts plus prospective production/test paths.
- `git diff --check` and direct whitespace checks on all four files: passed.
  Status shows two modified owner documents and two untracked package files;
  nothing staged, no production/permanent-test changes.
- Cheap source checks: actual base and every inventoried blob match; exactly four
  current selected commands retain `--no-color`/literal paths and omit
  `--no-ext-diff`; all declared ACs, local links and existing anchored test names
  resolve. Both prospective test files are absent. Catalog discovers the owner pair.
- ROOT archive checksum matches the accepted SHA256; proof was not replayed or
  modified. All owned cheap commands returned terminal results, with no live handles.

Requirement/design are below their 20/32 KiB limits. No heavy lease is held;
implementation, permanent tests, installs and heavy verification have not run.
Task 01 remains pending. The next step is a later ROOT same-session implementation
interrupt with the exclusive heavy lease, not automatic continuation from this handoff.

## Implementation checkpoint

ROOT accepted the actual four-artifact package, then later released execution in
the same primary session with the sole global local-heavy lease. Task 01 began.
Pinned pnpm9.15.9 frozen apps install passed, 933 reused/zero downloads, joined
session59732/PID321807 gone. Native process and registered HTTP regressions are
written; production `workspace_git_diff.go` still matches the design-base blob.

The reviewed process RED command, session56174/PID337807, joined exit1 with a Go
two-minute timeout, package 120.045s. Meaningful completed main/fallback cases show
positive helper controls, actual unwanted execution and replaced patch hunks;
the full RED gate did not complete. Some mixed fixture reads also reached their
15-second context deadlines. Logs/receipt are `/tmp/kandev-child43-process-red.log`
and `/tmp/kandev-child43-process-red.receipt`. No retry or larger timeout was used.

ROOT subsequently authorized bounded continuation in this same primary. Task 01
is in progress and retains the exclusive local-heavy lease; merge remains ungranted.
Session56174 is terminal and joined, wrapper337807 and owned fixture processes are
gone, recorded in `/tmp/kandev-child43-process-red-terminal.json`. The completed
cached fallback took 20.20s and independently failed exact built-in hunks and helper
suppression in configured/environment/both modes, so it will not be replayed.
Only ordinary/configured mixed HTTP RED is authorized before the four options;
full permanent process/HTTP GREEN matrices and original bounds remain required.

## Remaining design risks

- Configured positive controls that silently override with `GIT_EXTERNAL_DIFF`
  do not prove repository configuration works. Prove each mechanism independently.
- Normal staged reads can fill flattened data before fallback. The separate real
  staged-enrichment boundary is essential to prove all four producer sites.
- Helper/env fixtures must remain isolated and sequential. Native Windows support
  cannot be dropped for portable scenarios; only demonstrated filename restrictions
  may be scoped. Preserve active package teardown and leak checks.
- Actual source changed after the diagnostic. Audit current base and captured-index
  semantics again at later release; do not rebuild production from archived source.
- Resource/timeouts/transport/unknown/out-of-scope failures checkpoint ROOT without
  automatic retry. Routine causal own-scope fixture/lint correction is allowed later.

HTTP narrow RED session11127/PID368123 joined exit1, package35.826s, without
timeout. Ordinary mixed passed; configured mixed executable controls passed and
registered selected/aggregate independent repositories failed helper suppression
and exact built-in hunks. Logs/receipts: `/tmp/kandev-child43-http-red.log`,
`/tmp/kandev-child43-http-red.receipt`, `/tmp/kandev-child43-http-red-terminal.json`.
Wrapper/owned fixture processes are gone. The four reviewed producer-local
`--no-ext-diff` options are now applied; full permanent GREEN is next.

## Implementation validation

Production is exactly four `--no-ext-diff` additions beside existing `--no-color`
options; a byte comparison against design-base source proves no other changes.
Permanent test matrices and native helper controls remain intact. No global
configuration, validator, shared fixture, runtime, parser, API or frontend change.

| Check | Actual joined handle / wrapper PID | Result |
| --- | --- | --- |
| Full process GREEN | 31625 / 373253 | PASS, package 37.780s |
| Full registered HTTP GREEN | 98165 / 379686 | PASS, package 33.953s |
| Original process compatibility | 95160 / 387307 | PASS, package 9.032s |
| Original HTTP compatibility | 31827 / 393151 | PASS, package 8.687s |
| Exact security flag/variant compatibility | 55221 / 399217 | PASS, package 1.012s |

All ran sequentially using the exact commands above: trimpath, fts5, race, p=1,
GOMAXPROCS=2, GOMEMLIMIT=512MiB, Go2m/GNU4m/kill10. Every handle is terminal and
actually joined, each wrapper gone before the next heavy operation. Receipts and
logs are `/tmp/kandev-child43-{process-green,http-green,process-compat,http-compat,security-compat}.{log,receipt}`.
No passing replay, bounds increase, GORACE adjustment or reinstall was used.
The failed partial process RED timeout remains preserved and is not a passing
suite. The ROOT-authorized narrow HTTP RED proves the transport regression.

Catalog validation passed (351 decisions, 1369 specifications); all-spec lint
passed. Actual seven-file documentation coverage passed, covered/errors0, receipt
`/tmp/kandev-child43-docs-coverage.json`. gofmt and whitespace checks passed.
Public Git guide audited: no public controls/API/copy/workflow change justifies an
edit. Backend patch data preserves desktop/mobile consumers; no browser, build,
E2E, typecheck or frontend test is required for this pure-data fix.
Scoped three-package lint passed with zero issues, original concurrency2/serial/
CLI5m/GNU6m/GOMAX2/GOMEM512 bounds: session26580/PID399917 actually joined exit0
at04:38:48Z, wrapper/owned descendants gone. Log/receipt:
`/tmp/kandev-child43-scoped-lint.log`, `/tmp/kandev-child43-scoped-lint.receipt`.
Normal active hooks and publication follow. Exclusive local-heavy lease is retained
through joined publication, then returned before the sole hosted collector.
Task 01 remains in progress until actual separately authorized merge and all joins.
