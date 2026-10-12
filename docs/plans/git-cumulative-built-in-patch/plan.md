---
created: 2026-10-03
status: completed
requirements:
  - REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001
system_design:
  - ../../specs/platform/system-design/git-diff-file-metadata.md
legacy_specs: []
---

# Implementation plan: Built-in cumulative Git patches

## Overview

Repair cumulative comparisons that become successful empty file maps when a
legitimate external diff helper replaces unified patch output. Platform owns
the shared comparison-data contract. Extend its existing requirement/design
pair with .8 and .9; existing .1-.7 continue to own status, paths, counts,
budgets, routing and plain color output. One sequential work order includes
the exact safe-flag dependency before implementation validation.

Accepted parent evidence is `/tmp/kandev-external-diff-repro.go`, inspected
read-only without replay. A real committed README.md modification gives one
file through plain cumulative and `ShowCommit` controls. Configuring the owned
helper to emit `CUSTOM DIFF OUTPUT` makes `GetCumulativeDiff` return
`Success=true`, empty files. Parent's tagged resource-capped race handle 91316
joined exit 1, package 0.136s; the temporary test was removed. This is supplied
RED evidence, not this child's permanent regression run. Initial clean HEAD:
`3e45498eb175294fc5482fb1d2f8646f6bf37e12`.

## Scope

### In scope

- Add exact `--no-ext-diff` to the cumulative patch invocation and exact
  admission in `securityutil.IsKnownSafeGitFlag`, with variant rejection tests.
- Real-Git operator and registered selected/aggregate HTTP regression evidence,
  including helper execution sentinels, positive controls and read-only snapshots.
- Owning Platform pair and this one sequential delivery record.

### Out of scope

Parser changes, arbitrary helper-output support, config writes, environment
architecture or global Git policy, `ShowCommit` flags, textconv, rename/base
changes, tracker/other producers, new schemas, UI/copy/browser/build, full local
suites, optional audits/polish, new dependencies or delegation.

## Technical approach

`GetCumulativeDiff` in process `git_log.go` issues `git diff --no-color
--src-prefix=a/ --dst-prefix=b/ <base>`. `splitDiffSections` requires column-zero
`diff --git ` sections. Suppress the external helper at that invocation only.
Add the flag to the exact list in common/securityutil `git.go`; its absence is
already verified, so this is an upfront dependency rather than a later bypass.
Leave shared validation, captured environment, admission, budgets, cancellation
and managed subprocess ownership intact. `git show` is built-in by default;
`ShowCommit` provides a positive control with no production edit.

| Boundary | Identity and behavior | Evidence |
| --- | --- | --- |
| Cumulative operator | Existing base-to-working-tree patch, including dirty tracked changes | Real Git, configured/env/both helper cases, empty and limit controls |
| Commit operator | Requested SHA and existing built-in patch/metadata | Positive control under helper setup |
| Selected HTTP | Only requested repository, exact path and base/HEAD | Registered router and real Git |
| Aggregate HTTP | Independent repository bases, same path with distinct content and NUL-qualified keys | Two independent repositories, repository/base metadata and submodule omission |
| Client/WS projections | Existing fields forwarded unchanged | Operator/HTTP serialization; no projection changes |
| Provider history/live tracker/other Git | Independent existing contracts | Excluded; no new coverage claimed |

Portable helper fixture: re-execute the native test executable with a narrowly
guarded helper test entry, custom output and owned sentinel path. Invoke it
through Git's supported external-helper command syntax, quoting native paths
correctly (including spaces); retain real Git rather than replacing it. A raw
Git control without suppression must execute the helper and emit its custom
output. Clear only the owned control sentinel, then assert absence across each
production read. Cover native Windows functionality; any unsupported assertion
must be narrowly justified rather than skipping the entire regression.

Isolate owned Git configuration. Filter inherited fixture Git variables, then
explicitly inject `GIT_EXTERNAL_DIFF` through the existing operator environment
provider or `InstanceConfig.AgentEnv` before manager creation. Shared test Git
helpers filter all `GIT_*`, so raw environment controls need an explicit copied
environment; silently using those helpers does not prove env-helper behavior.
Do not mutate shared fixtures or environment after manager snapshots.

Patch oracles and snapshot diffs use exact built-in, color-disabled output with
fixed prefixes. Bracket reads after helper setup with config bytes, HEAD, refs,
index entries, worktree status and raw file-byte observations. Verify helper
settings remain intact. Observation must not run the helper or create the
sentinel. Use explicit positive expected values as well as equality; two empty
results cannot prove correctness.

## Tests

| Criteria | New test and evidence |
| --- | --- |
| .3, .5, .8, .9 | `TestCumulativeDiffExternalHelpers` in new process `git_log_external_diff_test.go`: committed/dirty tracked modifications, genuinely empty comparison; no helper/configured/env/both cases; explicit membership, exact paths, patch bytes, statuses, counts, base/HEAD/commit counts and unchanged owned state |
| .4, .8, .9 | `TestCumulativeDiffExternalHelperBudgets` in the same file: actual configured-helper cumulative reads under per-file, total-byte and file-count limits; correct metadata, skip reasons and truncation counts against a helper-free control |
| .3, .8, .9 | `TestCumulativeDiffExternalHelpersHTTP` in new API `git_external_diff_test.go`: registered single/selected/aggregate routes, plain and selected commit controls; two independent repos modifying the exact same tracked path with distinct sentinels; configured/env/both cases, exact patch/count/base/repository identities and read-only snapshots |
| .8 | `TestIsKnownSafeGitFlagAllowsNoExtDiff` and `TestIsKnownSafeGitFlagRejectsNoExtDiffVariants` in common/securityutil `git_test.go`: only exact flag admitted; abbreviation/value/suffix/whitespace variants rejected |

Permanent tests must independently fail on the accepted empty-map defect before
production edits. Prove each fixture helper works; production helper sentinels
must stay absent. Commit and plain cumulative controls are required, not claims
of another defect. Preserve .1/.2/.6/.7 behavior with representative metadata
and plain literal content in the new fixtures, without replaying prior passing
regression suites. New budget cases are required coverage for this invocation.

## End-to-end and surface assessment

Actual Git -> public operator -> registered router -> decoded JSON is the
end-to-end boundary. Desktop and phone receive the same corrected comparison
data. No layout, touch, scrolling, navigation or viewport-dependent behavior
changes, so no mobile/browser E2E or ASCII UI preview applies. The existing
Git operations how-to/reference subsection already explains faithful metadata
and read-only comparison actions; README/screenshot references add no conflicting
promise. No public-doc/copy change is needed. No material architectural
alternative requires an ADR or another incident requirement.

## Work orders

- [x] [Task 01: Use built-in cumulative patches](task-01-use-built-in-cumulative-patches.md)

## Execution and delivery gates

Task `27d98911-a7b6-4fc1-92bf-63e0f2a33212`, primary session
`6b705579-d3ea-4e21-9064-b7a1ed0b9aff`, is the sole child 16. Parent
`14825981-b175-411d-999a-31ddc2aa5fc3`, session
`4b15fc37-c487-4e2b-b4a2-2833edf18794`, owns package review. The design turn ended
with four unstaged/uncommitted artifacts and a queued parent handoff, then waited
for a later explicit parent implementation INTERRUPT before production or permanent
test changes. That later release is recorded below. No operator/model/profile-switch question or additional
agent/task/session is authorized. Preserve the external task plan marker,
identities, user edits, question barriers and stop/completion gates.

After release, execute TDD and exact work-order checks sequentially. ONE heavy
command at a time, retain every handle and join before the next, including
install/hooks. Reconcile ownership/receipts after a crash before replacements;
resource limits do not guarantee crash immunity. Preserve foreign processes,
workspaces and caches. No broad suite/replay/build/browser/audit or automatic
resource retry. A failure outside scope needs exact leaf/log/artifact evidence
and a queued parent report for bounded direction.

Later delivery requires normal hooks, commit/push/ready PR, disposition of every
actual finding, terminal exact-head required CI, and authenticated configured
CodeRabbit App 347564 FULL substantive semantic review of all changed files.
ACK0s/skipped reports are insufficient. A completed current-head full report
needs no duplicate full or optional second-Claude wait. With incremental review
disabled, permit at most one necessary full request for a corrected head.
One owned `scripts/pr-await` monitor must be joined before any replacement;
persist identity/resources if interrupted.

Freeze published SHA except valid corrections; no moving-main rebase or
synthetic compatibility tests. Cheap exact blob/merge-tree evidence is allowed
near merge. Normal expected-head squash must be independently verified by
actual merged SHA/tree/blobs/remote. JOIN owned cleanup before COMPLETE;
preserve this platform-managed worktree for parent archive. Parent archives and
finds the next task. Child-to-parent messages use queued delivery, never
interrupt: concrete design handoff, blocker/recovery needing coordination or
verified merge/cleanup receipt. Final chat alone is insufficient.

## Verification results

Design validation on 2026-10-03:

- `python3 scripts/list-docs.py validate`: passed, 343 decisions and 1321 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Work-order Node `validateCoverage` preflight with all four real documents and
  both intended production paths: `ok: true`, `status: covered`, no errors.
  This proves package links, not implementation or hosted PR coverage.
- `git diff --check` and package inventory: passed; both new package files exist.

All required design validation commands terminated with exit 0; no outstanding owned handle.
Four artifacts remain unstaged/uncommitted. No production/permanent tests,
Go checks, dependency install, commit or publication performed. Implementation
remains unreleased; end after queued parent handoff.

## Risks

- Missing exact flag admission rejects the corrected invocation before Git runs.
- Filtered fixture environments can silently omit the env-helper case.
- Helpers can corrupt oracles/snapshots or create false execution sentinels.
- Captured manager environments require setup before construction.
- Identical paths or empty-equality assertions can conceal routing/membership loss.
- Native helper path quoting and test-binary dispatch must be portable and guarded.

## Implementation release

The later explicit parent INTERRUPT on 2026-10-03 released this reviewed package through actual normal merge and joined cleanup in the existing primary session. The design stop barrier above is historical and satisfied. Task 01 is in progress; no other agent, task or session is authorized.

## Local implementation results

Permanent exact process/API/securityutil RED and GREEN are recorded in
[Task 01](task-01-use-built-in-cumulative-patches.md#results). All new regression
checks pass after the two-line production correction. Scoped lint replacement
99975 joined exit 0 with zero issues. The aborted overlapping attempt 93084
joined exit 143 and is not lint evidence; the parent explicitly released the
one serial replacement. No passing test was replayed. One pinned frozen pnpm
9.15.9 install joined exit 0, 1.9s; all 923 packages reused, no downloads.
Final document gates precede the normal hooked commit/publication. Hosted
semantic review/required CI, actual normal merge and joined cleanup remain
external task completion gates.

Final local document gates passed: catalog343 decisions/1321specifications,
spec-linter36 tests and all-file lint, actual nine-file `validateCoverage`
(`covered`, no errors), whitespace/package inventory. Task 01 is done and
this plan's status records local implementation only. Normal hooked publication,
current-head hosted review/CI, actual merge and joined cleanup remain task gates.
