---
created: 2026-10-03
status: completed
requirements:
  - REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001
system_design:
  - ../../specs/platform/system-design/git-diff-file-metadata.md
legacy_specs: []
---

# Implementation plan: Plain Git comparison output

## Overview

Deliver one sequential repair of the two machine-parsed comparison producers.
The parent supplied real-Git evidence at actual main
`678a4d19ad5ffd8f6f9c5f7be419bbdc02609d24`: after committing a README update
and `added.txt`, both `ShowCommit` and `GetCumulativeDiff` returned two files
with color disabled and zero files with `color.ui=always` and
`color.diff=always`. Both incorrectly reported success. The accepted proof is
`/tmp/kandev-forced-color-diff-repro.go`; parent handle 71977 joined with exit 1
and package time 0.093s. The temporary test was removed and the parent root was
clean. The earlier malformed `-p1` command was setup failure, not evidence.
Do not replay that proof before permanent RED tests.

Root cause: the patch commands omit an explicit plain-output flag. Forced color
prefixes section headers with escape sequences, while `splitDiffSections`
requires `diff --git ` at column zero. The prior metadata repair remains valid;
this repair prevents display decoration from hiding sections at the producer.

## Scope

### In scope

- Add `--no-color` to the patch-producing `ShowCommit` and `GetCumulativeDiff`
  invocations in `apps/backend/internal/agentctl/server/process/git_log.go`.
- ONLY exact `--no-color` admission in `common/securityutil/git.go`, with focused
  allow and malformed/value/suffixed rejection tests in existing `git_test.go`.
- Permanent real-Git process and registered-HTTP regression tests in two new
  bounded test files, using existing fixture and environment seams.
- Extend the existing owning Platform requirement/design and this one order.
- Preserve exact paths, literal content bytes, status/count/metadata fidelity,
  binary/root/merge/empty semantics, budgets/limits, read-only behavior and routing.

### Out of scope

Parser rewrites, ANSI stripping, configuration mutation, shared subprocess color
policy, tracker changes, operator-environment refactoring, broader Git cleanup,
new APIs/dependencies, frontend/browser/build/layout/copy work and new ADRs.
Report an actual additional comparison producer to the parent before expanding.

## Technical approach

Follow the [owning design](../../specs/platform/system-design/git-diff-file-metadata.md#plain-comparison-output).
The two patch argv lists change, with one exact safe-flag registration in
`apps/backend/internal/common/securityutil/git.go`. Reject malformed, value and
suffixed variants through focused tests in its existing `git_test.go`. Keep the metadata-only `git show` query,
validation, operator environment provider, `RunGitAfterAcquire`, admission class,
timeouts, cancellation and managed subprocess ownership intact.

Immediate inventory: `parseCommitDiff`/`parseCommitDiffWithOptions` have these
two production comparison callers. Registered commit HTTP uses `ShowCommit`;
all resolved cumulative paths use `GetCumulativeDiff`. Client and WS projections
forward existing result fields. No additional producer requiring expansion was
found in this inventory. `GetLog` shortstat and mutation `ShowCommitStats` are
separate operations and not part of this patch-parser contract.

| Surface | Identity and expected result | Evidence |
| --- | --- | --- |
| Local commit | Requested SHA; first-parent/root semantics; uncapped patch and metadata | Real `ShowCommit` regressions |
| Local cumulative | Existing base-to-worktree comparison including dirty tracked content | Real `GetCumulativeDiff` regressions and existing caps |
| Registered commit / selected cumulative HTTP | Selected repository only, exact path and plain content | Real registered-router tests |
| Aggregate cumulative HTTP | Independent repo bases, NUL-qualified identical paths, distinct sentinels and repository metadata | Two-repository registered-router test |
| Existing client / WS projections | Same field/status shapes pass through | Producer and HTTP evidence; no projection changes |
| Provider-only history / live tracker | Existing independent source contract | No changes or claimed new coverage |

## Tests

| Criteria | Test and evidence |
| --- | --- |
| .1, .2, .3, .6 | `TestGitComparisonPlainOutput` in process `git_log_plain_output_test.go`: unset defaults, UI always alone, UI false plus diff always, both always, UI always plus diff false; explicit file keys/status/counts/metadata and exact plain patch oracle |
| .3, .6, .7 | Same test includes literal ANSI source content, an escape-containing path on filesystems that support it, binary modification and empty-file addition/deletion controls, plus dirty cumulative reads; compare config/HEAD/refs/index/status/content before and after each read |
| .5, .6 | `TestGitComparisonPlainOutputRootAndEmpty` and `TestGitComparisonPlainOutputMerge` in the same process file: forced color through the distinct root/empty/first-parent branches |
| .4 | Existing `TestGetCumulativeDiff_TruncatesLargeFile`, `_BudgetExceeded`, `_CapsFileCount`, `TestShowCommit_NotCapped`, and parser status-budget tests; keep budgets and skip reasons intact |
| .3, .6, .7 | `TestGitComparisonPlainOutputHTTP` and `TestGitComparisonPlainOutputMultiRepoHTTP` in API `git_plain_output_test.go`: actual router, selected/aggregate identity and per-repo bytes, counts and status, read-only snapshots |

Use explicit expected values and a raw `git ... --no-color` patch oracle, not
argv-only assertions or comparisons between two potentially empty results.
No blanket assertion that every ESC is absent: literal ANSI is legitimate data.
Isolate fixture Git configuration with the existing test/environment seams
before operator or manager creation. Do not patch process environment after a
manager snapshot or run environment-changing tests in parallel. Keep portable
tests on Windows; scope only unsupported filesystem names, if demonstrated.

Focused securityutil evidence: `TestIsKnownSafeGitFlagAllowsNoColor` and
`TestIsKnownSafeGitFlagRejectsNoColorVariants` prove exact flag admission and
rejection of malformed/value/suffixed forms. The order includes that package
in scoped race and lint commands.

## End-to-end and surface assessment

Real Git -> operator -> registered router -> decoded JSON is the end-to-end
boundary. This is source-data normalization with no layout, touch, scrolling,
navigation or viewport changes. The mobile-parity pure-data exception applies;
no browser, E2E shard or ASCII UI preview is required. Public-docs assessment:
the existing Git operations reference already states faithful read-only
comparisons; this repair adds no user option, API field or workflow. No public
documentation change is needed.

## Work orders

- [x] [Task 01: Produce plain comparison patches](task-01-produce-plain-comparisons.md)

## Delivery gates

Finish the design turn with these four artifacts unstaged and uncommitted.
Send a queued handoff to parent `14825981-b175-411d-999a-31ddc2aa5fc3` and end
the turn. Await a later explicit parent INTERRUPT implementation release in
task `2d3cd45e-8d4e-4996-91d3-14bdeaa88a5f`, session
`fccd06a9-126d-45e1-ab32-13504ff36364`. No operator approval or model-switch
question, additional agent, worker, task or session is authorized.

After release, implement with permanent RED/GREEN and the exact work-order
checks, normal hooks, commit/push and a ready PR promptly. Keep one heavy local
command at a time; retain and join every handle. Preserve others' edits,
processes, worktrees and caches. If dependencies are absent, one project-pinned
pnpm 9.15.9 frozen install from `apps/` precedes hooks; no lockfile change.

Delivery requires one owned `scripts/pr-await` monitor joined before any
replacement, current-head terminal required CI and an authenticated configured
CodeRabbit App 347564 full all-file semantic report with every actual finding
dispositioned. No ACK, skip, success check or duplicate optional review suffices.
Freeze the published SHA except for valid corrections; never rebase solely for
moving main. Use cheap merge-tree/blob compatibility evidence without synthetic
tests. Merge by normal expected-head squash without admin/bypass, independently
verify actual merged SHA/content/remote and join all owned handles before owned
cleanup. Parent handles archive and the next child. Send queued callbacks for
PR publication, concrete blockers/recovery, and verified merge with cleanup.

Unrelated CI needs exact failing leaf/log evidence and a queued parent report,
not a blind retry loop or scope expansion. If the known mobile history geometry
failure recurs, consult the parent; prior passing retries are not this head's
evidence. No local geometry build or broad audit is authorized.

## Verification results

Design checks on 2026-10-03:

- `python3 scripts/list-docs.py validate`: passed (343 decisions, 1321 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Exported `.github/scripts/pr-docs.cjs` `validateCoverage`, using the four
  actual package documents plus intended runtime `git_log.go`: `ok: true`,
  `status: covered`, no errors; requirement/design/manifest/order links accepted.
  This is design coverage, not hosted PR evidence or an implementation pass.
- `git diff --check` and package status: passed; both work-package files exist.

All four owned artifacts remain unstaged/uncommitted. No production/permanent
test change, Go test/lint/build or dependency install occurred. No owned live
handle exists. Next action is the queued parent handoff and end of design turn;
implementation remains pending a later explicit parent INTERRUPT release.

Implementation results: permanent process and registered-HTTP RED failed on
forced-color empty membership while controls passed. After the parent-released
exact allowlist dependency, scoped process/API/securityutil race GREEN passed
in 3.067s/1.867s/1.012s, all handles joined. Scoped changed lint passed with zero
issues. Catalog/spec lint, actual changed-file documentation coverage and
whitespace checks passed. One pinned pnpm 9.15.9 frozen install completed with
no tracked dependency changes. No full suite/browser/E2E/build was run.
Local order is complete; normal hooks, publication, hosted gates, actual merge
and joined owned cleanup remain external task completion gates.

## Bounded validation dependency

The first GREEN attempt was rejected by the existing safe-flag validator. The
parent explicitly released one exact `--no-color` allowlist entry plus focused
allow/variant tests in common/securityutil, within this same sequential order.
No prefix widening, bypass, new producer or shared color policy is authorized.
The work order preserves all failed-run and joined-handle receipts.

## Risks

- A generic escape stripper would corrupt real source bytes.
- A color-disabled UI setting alone cannot control `color.diff=always`.
- Two empty results can falsely appear equivalent; assert positive membership.
- Configuration snapshots must bracket reads after disposable fixture setup.
- Root/merge/empty branches and binary metadata need positive controls independent
  of the initial two-text-file proof.
- Broader Git configuration behavior and unrelated producers stay outside scope.
