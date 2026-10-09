---
created: 2026-10-05
status: in_progress
requirements:
  - REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001
system_design:
  - ../../specs/platform/system-design/git-diff-file-metadata.md
legacy_specs: []
---

# Implementation plan: Preserve actual-byte comparison patches

## Scope and dependency

One sequential work order adds exact `--no-textconv` to the TWO existing patch
argument lists in `apps/backend/internal/agentctl/server/process/git_log.go`
(`GitOperator.ShowCommit` and `GitOperator.GetCumulativeDiff`). The upfront,
already scoped dependency is exact flag admission in
`apps/backend/internal/common/securityutil/git.go`; it is confirmed absent.
No later permission gate, prefix expansion or validation bypass applies.
Platform owns the shared comparison source-data contract, so extend its
existing [requirement](../../specs/platform/requirements/git-diff-file-metadata.md)
and paired design with only missing .10/.11 criteria.

Exclude parser/count/budget/ref/environment/global helper policy/command
framework changes, other Git reads/writes, UI/copy/schema, ADRs, dependency
changes and sibling artifacts. Siblings own workspace_files.go/content-search
and git.go/literal Discard/workspace Git. Their files and plans remain separate.
Prior [status](../git-diff-status-metadata/plan.md),
[color](../git-comparison-plain-output/plan.md), and
[external-diff](../git-cumulative-built-in-patch/plan.md) packages are existing
completed delivery records; no work-order/status edits are required there.

## Accepted evidence and root cause

Read-only proof archive `/tmp/kandev-comparison-textconv-repro_test.go` has
verified SHA256 `c18bcdf8617d15ee9d8dc34c71e84d7b8f33bcf91f425991407f577b3cb0b336`.
Receipt `/tmp/kandev-root-comparison-textconv-proof-receipt.json` and log
`/tmp/kandev-root-textconv-repro.log` record actual production
`TestRootComparisonTextconvProof`, handle 90819 actually joined exit 1, expected
RED, package 0.226s, no timeout. No-driver same-API controls pass exact path
and old/new byte checks. A configured attribute driver producing identical
constant text makes both methods return `Success=true` without the real file;
commit totals become 0 instead of 1 +1/-1. Each method invokes the converter
twice. ROOT removed temporary source and cleaned private repositories.

Proof base is `eb589f279dc8527101b71fdaf99ce523909a5299`. This worktree's
design HEAD is `dd7dfa81634236cfeb0df6fd7fac4e005d08d2f3`; one lightweight blob
check confirmed process `git_log.go` = `9e5a05cca8fb499491da66fc9934c5cf242c47f1`
and securityutil `git.go` = `bcae0b82ec8897627e5de5b64edfcccce73176f8`, both
matching proof. Do not replay that accepted proof.

Supporting `/tmp/kandev-root-textconv-raw-probe.json` is RAW Git-only evidence,
not another production verdict. Exact current show/diff argv runs the converter
and omits patch sections; adding `--no-textconv` restores actual file patches
without helper execution. `--no-ext-diff` does not disable text conversion.
Stat/numstat can describe a real change while the parser receives no section.
This repair selects actual-byte patches at the producers, leaving parsing intact.

## Test design

Use actual Git, public operators, and only the necessary registered HTTP seam.
Follow the native test-binary helper pattern in `git_log_external_diff_test.go`
and API `git_external_diff_test.go`; new local helpers live in new test files.
Guard helper dispatch with private env/argv and anchor its test name, quote paths
including spaces, and preserve native Windows coverage without shell scripts.
Install guards before captured environments/managers. No shared fixture rewrite.

Raw Git fixture controls with `--textconv` must prove converter execution and
changed/suppressed output, then remove only the owned sentinel. Oracles and
snapshot diffs use `--no-textconv --no-ext-diff --no-color`, fixed prefixes and
the correct first-parent/base semantics. Assert explicit nonempty membership,
old/new bytes and positive counts, as well as equality to raw Git patches; never
parse expected results with the production parser. Snapshot config and
info/attributes bytes, HEAD, refs, index entries and content/status after setup
and around each read. Converter cache/config writes must not arise from reads.

| Criteria | Exact evidence |
| --- | --- |
| .3, .5, .10, .11 | New process `git_log_textconv_test.go`, `TestGitComparisonTextconv`: no-driver, changed-output converter, constant-output converter; both methods, committed changes, dirty cumulative tracked changes; raw bytes/path/status/counts, commit metadata/totals, cumulative base/HEAD/count and absent sentinel |
| .2, .5, .10, .11 | Same file, `TestGitComparisonTextconvControls`: converter-selected binary modification, empty-file change, empty commit/cumulative result; binary status and 0/0 counts rather than converted text; explicit membership and read-only snapshots |
| .3, .10, .11 | New API `git_textconv_test.go`, `TestGitComparisonTextconvHTTP`: two independent repositories, same path but distinct bytes/bases/driver modes; selected commit and cumulative routes, cumulative aggregation, no-driver positives before configuration; selected reads leave the unselected repository sentinel absent too |
| .10 | Existing securityutil `git_test.go`, new `TestIsKnownSafeGitFlagAllowsNoTextconv` and `TestIsKnownSafeGitFlagRejectsNoTextconvVariants`: exact acceptance; abbreviation, value, suffix and leading/trailing whitespace rejection |
| .4, .5 | Existing focused `TestCumulativeDiffExternalHelperBudgets`, `TestShowCommit_NotCapped`, `TestGitComparisonPlainOutputRootAndEmpty`, `TestGitComparisonPlainOutputMerge`: unchanged limits, prefixes, first-parent/root/empty behavior |

HTTP assertions include exact selected membership and totals/metadata; aggregate
NUL-qualified keys, `repository_name`, each repository's `base_ref` and ordinary
repository `is_submodule` omission. Independent repositories use independently
resolvable refs; passing one repository's SHA to another is not an oracle.
Keep the converter matrix in process tests; HTTP needs one mixed configured
pair and no-driver controls, not a duplicated Cartesian matrix.

## Surface assessment

Actual Git -> public operator -> registered router -> decoded JSON supplies
end-to-end evidence. Shared data correction changes no layout, touch behavior,
scrolling, navigation or viewport interaction; `/mobile-parity`'s pure-data
exception applies, with no browser/E2E or ASCII UI preview. `/docs-maintainer`
audit of the Git operations how-to/reference subsection, README and screenshot
catalog found accurate read-only comparison guidance and no converter promise
to amend. No public guide edit or new user-facing copy is needed.

## Work orders

- [x] [Task 01: Preserve actual-byte patches](task-01-preserve-actual-byte-patches.md)

## Execution and delivery barriers

Task `16caca17-8c67-49c6-b4f5-19e6ccdbb333`, primary session
`e6b60395-c663-4c35-a9f4-9d4b2abda7a3`, parent ROOT
`14825981-b175-411d-999a-31ddc2aa5fc3`. Existing worktree/branch and session only;
no agents/tasks/sessions/tabs/model changes. DESIGN holds no local-heavy lease:
no install, Go/Node tests, product lint/typecheck/build or heavy hooks. Leave the
four artifacts unstaged/uncommitted and end WAITING. Implementation requires a
LATER explicit ROOT reviewed-package interrupt AND global local-heavy lease.
Parent callback queue is full: persist checkpoint, never retry messages/questions.

After release, execute the work order with TDD, one heavy operation at a time.
Retain and ACTUALLY JOIN every handle before another operation or replacement.
Preserve shared caches, foreign processes, paused oversized child and unproved
volume. Failed resource gates checkpoint exact logs/handles for bounded ROOT
direction; no automatic retry, cache wipe or foreign kill.

Normal ready PR/full delivery is authorized only after release and lease. Use
normal active hooks; if dependencies are absent, one pinned pnpm 9.15.9 frozen
install. Backend-code fixup requires the exact-base full CHANGED lint command in
the work order once; do not substitute package-only lint for that fixup gate.
Freeze published SHA except valid findings, with no moving-main rebase,
synthetic compatibility tests or optional polish. Retain one all-terminal CI
collector and join it before any specifically authorized replacement. Require
all six actual required contexts successful, all CI parents terminal and
complete error-free current-head snapshots, with no hidden/actionable findings.
Inspect authenticated CodeRabbit App 347564 automatic all-file coverage first;
at most ONE necessary full request after proven skip/gap. Require substantive
FULL current-head report, not ACK/progress; no optional second-review wait.
Ground every finding disposition.

After clean checkpoint and all joins, obtain separate ROOT MERGE LEASE. Merge
promptly by normal expected-head squash, without admin/bypass; independently
verify actual merged SHA/tree/owned blobs/remote inclusion and join only-owned
cleanup. Preserve managed worktree/deps and ROOT proof for archival. Persistent
task completion requires verified merge, not just local work-order completion.

## Verification results

Design checks on 2026-10-05:

- `python3 scripts/list-docs.py validate`: exit 0, 351 decisions and 1357 specifications.
- `python3 scripts/lint-spec-files.test.py`: exit 0, 36 lightweight Python tests.
- `python3 scripts/lint-spec-files.py --all`: exit 0; both owning files remain below size limits.
- Catalog search and package inventory: owning pair and exactly one work order present.
- `validateCoverage` from `.github/scripts/pr-docs.cjs`: four actual documentation
  files `ok=true`, `status=exempt`; the same documents plus both planned
  production paths `ok=true`, `status=covered`, no errors. This proves design
  references, not implementation or hosted PR coverage. Bare `node` first failed
  to start, exit 127; the existing executable at
  `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node` ran this lightweight
  preflight successfully, with no install or cache mutation.
- `git diff --check`, empty staged diff and unchanged backend diff: passed.

All invoked commands returned terminal results, with no owned running handle.
Four artifacts remain unstaged/uncommitted. No production/permanent test edits,
Go/Node tests, product lint/typecheck/build, install, commit or PR. Product checks
are deferred by the design/lease barriers. End DESIGN turn **WAITING** for the
later ROOT reviewed-package interrupt AND global local-heavy lease.

## Risks

- Missing exact flag admission prevents the production argv from reaching Git.
- Converter-sensitive oracles/snapshots can execute helpers and hide real bytes.
- Empty-result equality and identical repository data can mask missing patches
  or wrong selection; require positive values and independent fixture identity.
- Native helper dispatch/quoting and environment capture must remain portable.

## Initial failure checkpoint: WAITING ROOT (historical)

ROOT subsequently granted child36 the exclusive global LOCAL-HEAVY lease.
All three new anchored RED commands ran sequentially with the required race,
memory/concurrency and wall bounds, and all ACTUALLY JOINED. Securityutil RED
exited 1 (0.012s) for the missing exact flag; process RED handle 63254 exited 1
(25.867s) for actual transformed/suppressed patches and converter execution,
with no-driver controls passing. HTTP RED handle 76166 exited 1 (13.726s):
selected no-driver controls passed and configured selected reads showed causal
converter failures, but both aggregate controls also returned HTTP 400 because
the new test omitted the required `base` query parameter. Aggregate RED is
therefore unproved, and this unexpected fixture failure triggers WAITING ROOT.

Receipts are `/tmp/kandev-child36-textconv-{securityutil,process,api}-red.log`;
the work order records the exact one-line test-only repair and next HTTP RED
command for later bounded ROOT direction. No automatic repair/retry, production
edit, GREEN, lint, install, hook, commit, push or PR followed the failure.
All local handles actually joined, zero live owned handles, LOCAL-HEAVY returned.
No merge lease or callback. Task01 remains in_progress; persistent task is not
complete. Earlier design/authoring checkpoints below are historical.


## Staged implementation continuation

ROOT reviewed and accepted all four concrete artifacts, then released only
NON-HEAVY permanent regression-source authoring in the existing primary session.
Task01 is in_progress. child35 owns the global LOCAL-HEAVY lease. Commands were
refined first: GNU outer 6m/kill-after 10s around each Go test with inner 5m,
using valid `-p=1` for one-package concurrency (the prior package records
`-p1` as invalid);
RED includes new regressions only, GREEN includes necessary existing compatibility
controls once. Initial scoped three-package lint uses reviewed base
`dd7dfa81634236cfeb0df6fd7fac4e005d08d2f3`; full CHANGED lint applies only to an
actual backend-code PR fixup. No test, lint, install, hook, commit, push or PR is
permitted during this authoring stage. Production remains unchanged until the
subsequent explicit LOCAL-HEAVY grant and actually joined meaningful RED.


### Authoring checkpoint

Three regression source files are prepared: new process `git_log_textconv_test.go`,
new API `git_textconv_test.go`, modified securityutil `git_test.go`. Test sources
use guarded native helpers, raw positive execution controls, converter-disabled
actual-Git patch oracles, read-only snapshots and selected-repository isolation.
No tests or lint have run; no RED/GREEN verdict is claimed. Production remains
unchanged. End this staged turn WAITING for explicit LOCAL-HEAVY grant; the exact
next command is work-order securityutil RED. Zero live owned handles, no install,
helper execution, private repository creation, hooks, commits, pushes or PR.


## Reviewed regrant and implementation

ROOT regranted exclusive LOCAL-HEAVY to child36 after the initial joined failure
checkpoint. The one-line aggregate test URL repair preserves independently
configured comparison refs and per-repository raw oracles. Corrected HTTP RED
handle 71117 joined causally, exit 1 (15.798s), with no-driver aggregate passing.
Securityutil/process RED was not replayed. The approved production diff is
exactly three added argument entries: two producer flags, one exact admission.

All affected GREEN joined successfully: securityutil 14163/1.014s, process
48364/13.477s with required existing compatibility/budgets once, and HTTP
96081/6.049s. Initial scoped lint 11977 joined exit 1 for a routine ifElseChain
style finding in the new API native helper. It now uses switch; only affected
HTTP GREEN and API lint are rerun under the same bounds and exact reviewed base.
ROOT explicitly authorizes such routine in-scope repair without another handoff.
Resource, transport, unknown-causality, scope and hosted failure barriers remain.


### Local implementation complete; delivery pending

Task01 acceptance is done. Affected HTTP GREEN after the helper dispatch
correction joined exit 0 (89295, 6.043s); affected API exact-base lint joined
exit 0 (46297, zero issues). All process/securityutil checks were already
joined GREEN, including existing compatibility/budgets once. Catalog, all-spec
lint, actual-path one-work-order documentation coverage, whitespace and gofmt
checks passed. Three production flag entries are the complete production diff.
Normal hooked ready PR, terminal CI/current-head FULL review, separate ROOT merge
lease and independently verified actual merge remain persistent delivery gates.
