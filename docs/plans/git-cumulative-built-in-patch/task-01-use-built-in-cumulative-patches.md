---
id: "01-use-built-in-cumulative-patches"
title: "Use built-in cumulative patches"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001
acceptance_criteria:
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.4
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.5
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.8
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.9
system_design:
  - ../../specs/platform/system-design/git-diff-file-metadata.md
---

# Task 01: Use built-in cumulative patches

## Summary

Ensure cumulative reads produce Git's built-in patches independently of external
helper configuration/environment. In one sequential pass, establish permanent
real-Git and HTTP RED, implement exact invocation and allowlist changes, then
run bounded checks and maintain the delivery record.

## In scope

- All new tests in the [manifest](plan.md#tests), native helper execution control
  and read-only snapshots, registered selected/aggregate repository coverage.
- Exact `--no-ext-diff` admission in common/securityutil and its variant tests,
  then the existing `GetCumulativeDiff` patch invocation only.
- Reject `--no-ext-dif`, `--no-ext-diff=true`, `--no-ext-diff=`,
  `--no-ext-diff-more`, `--no-ext-diffs`, and leading/trailing whitespace forms.
- Maintain owning requirement/design and work-order results/status.
- Later explicit release only: normal hooked commit/push/ready PR, hosted
  checks/review/finding disposition, normal merge and verified joined cleanup.

## Out of scope

All [manifest exclusions](plan.md#out-of-scope), including `ShowCommit` flags,
global policy/config/environment/parser changes, other producers, textconv,
rename/base redesign, shared fixture edits, UI/browser/full suites and delegation.

## Acceptance

1. New operator and registered-HTTP RED fail on helper-induced successful empty
   membership while explicit plain/commit and deliberate helper controls pass.
   Allowlist RED fails on absent exact admission. Add only the exact safe flag
   and cumulative invocation flag to make these regressions pass.
2. Configured/env/both helper cases retain explicit file/patch/count/metadata,
   dirty/empty/limit and independent-repository routing evidence; production
   reads never execute helpers or mutate the owned state/settings.
3. Each exact affected check and coverage/spec gate passes with recorded joined
   receipts. Task completion additionally requires the manifest's hosted review,
   required CI, independently verified actual merge and joined cleanup gates.

## Verification

From the repo root, one heavy command at a time, retain and join every handle.
After LATER explicit release, mark this order `in_progress`, write the permanent
tests and run each of the three test commands below once for RED before editing
production. Record exact expected failures and positive controls. Then add the
exact allowlist entry and cumulative flag, and run the same three commands once
for GREEN, sequentially. No previous passing regression replay or broad prefix.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/process -run '^(TestCumulativeDiffExternalHelpers|TestCumulativeDiffExternalHelperBudgets)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/api -run '^TestCumulativeDiffExternalHelpersHTTP$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/common/securityutil -run '^TestIsKnownSafeGitFlag(AllowsNoExtDiff|RejectsNoExtDiffVariants)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout 6m golangci-lint run ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/common/securityutil --new-from-rev=3e45498eb175294fc5482fb1d2f8646f6bf37e12 --concurrency=2 --allow-serial-runners --timeout=5m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/git-cumulative-built-in-patch
```

If an actual correction affects one boundary, rerun its exact affected check
only. The native helper test entry is invoked only by the explicitly guarded
helper subprocess; do not add it to the main `-run` pattern. No sleeps, argv-only
tests, private-helper mirroring, weakened assertions or environmental bypasses.
Use existing managed teardown and bound/join all fixture resources.

Run this exact documentation coverage preflight with existing Node (no install
needed), using real package contents and the two planned production paths so
documentation-only exemption is not mistaken for traceability proof:

```bash
/home/jcfs/.nvm/versions/node/v24.18.0/bin/node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/plans/git-cumulative-built-in-patch/plan.md',
  'docs/plans/git-cumulative-built-in-patch/task-01-use-built-in-cumulative-patches.md',
  'docs/specs/platform/requirements/git-diff-file-metadata.md',
  'docs/specs/platform/system-design/git-diff-file-metadata.md',
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({ filename, status: 'modified' }));
for (const filename of [
  'apps/backend/internal/agentctl/server/process/git_log.go',
  'apps/backend/internal/common/securityutil/git.go',
]) changedFiles.push({ filename, status: 'modified' });
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }));
if (!result.ok || result.status !== 'covered') process.exit(1);
NODE
```

Before publication use actual changed paths (including new tests) in that same
API, retaining requirement/design/manifest/order contents. During design run
only catalog/spec/coverage/whitespace/status checks; no Go checks or tests yet.

For an actual backend PR fixup, resolve the exact PR base SHA first into
`KANDEV_BUILTIN_PATCH_PR_BASE` and run the mandated full-backend changed lint
once, serially after affected tests, before pushing:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout 6m golangci-lint run ./... --new-from-rev="$KANDEV_BUILTIN_PATCH_PR_BASE" --concurrency=2 --allow-serial-runners --timeout=5m)
```

No automatic resource retry. Report exact failing leaf/log/artifact to parent
for bounded direction if necessary. Normal hooks stay active; if
`apps/node_modules` is absent, use one pinned project/mise pnpm 9.15.9 frozen
install from `apps/` with existing Node runtime on PATH before hooks. Retain and
join installation/hooks handles too. No dependency/lockfile or foreign-cache
changes. Hosted delivery follows the [manifest gates](plan.md#execution-and-delivery-gates).

## Files likely touched

- `apps/backend/internal/agentctl/server/process/git_log.go`
- `apps/backend/internal/agentctl/server/process/git_log_external_diff_test.go` (new)
- `apps/backend/internal/agentctl/server/api/git_external_diff_test.go` (new)
- `apps/backend/internal/common/securityutil/git.go`
- `apps/backend/internal/common/securityutil/git_test.go`
- `docs/specs/platform/requirements/git-diff-file-metadata.md`
- `docs/specs/platform/system-design/git-diff-file-metadata.md`
- This order and sibling `plan.md`.

## Dependencies

No preceding work order. Exact `--no-ext-diff` registration is required by the
existing validator; it belongs to this same sequential order upfront. Prior
PR #4179 is parent-confirmed actually merged/verified/cleaned. Baseline is pinned
above. The later explicit parent implementation release satisfied the design stop barrier.

## Risks

Use the [manifest risks](plan.md#risks). Do not use plain-output snapshot helpers
unchanged: their raw diffs currently allow external helpers. Keep new observers
and oracles scoped to this fixture; preserve existing shared test helpers.
Raw helper positive controls need explicitly injected env, since `runGit` and
`runGitAPI` filter all `GIT_*`. Snapshot `AgentEnv` before manager construction.

## Parallelism

`sequential`. This primary session owns every file and gate; no other
agents/workers/tasks/sessions or recursive delegation.

## Inputs

- [Requirement](../../specs/platform/requirements/git-diff-file-metadata.md), .3-.5 and .8-.9.
- [Design](../../specs/platform/system-design/git-diff-file-metadata.md), built-in cumulative patches and preserved contracts.
- Read-only parent archive `/tmp/kandev-external-diff-repro.go` and its accepted
  proof in the manifest; do not replay or modify the parent archive.
- `git_log.go`, `git.go`, `git_test.go`, process
  `git_log_plain_output_test.go`, `git_log_status_metadata_integration_test.go`,
  `workspace_tracker_test.go`; API `git.go`, `git_plain_output_test.go`,
  `git_status_metadata_test.go`, `git_handlers_test.go` and `runGitAPI`.
- Existing native-test-executable process fixtures in
  `git_contribution_history_edge_test.go` and `manager_comparison_targets_test.go`.
- Backend/agentctl/API guides, `/fix`, `/planner-orchestration`, and later `/tdd`.

## Results

The later explicit parent INTERRUPT on 2026-10-03 released this reviewed package.
Local implementation and required document gates are complete.
Hosted review/CI, actual merge and joined cleanup remain task completion gates.

- Exact process RED: handle 58562 joined exit 1, package 63.360s. Configured,
  environment and combined helpers hid committed/dirty changes and executed
  helpers. Limit reads lost their positive file sets. Plain/commit/empty and
  deliberate helper controls passed. File-count RED exercised real helper
  subprocesses under the existing Git execution budget; it was not replayed.
- Exact API RED: handle 83328 joined exit 1, package 22.516s. Single, selected
  and aggregate routes returned successful empty files under helpers; helper
  sentinels fired. Plain and selected commit controls passed.
- Exact securityutil RED: direct terminal exit 1, package 0.012s. Exact flag
  admission failed as expected; all malformed variant controls passed.
- Production change: only cumulative `--no-ext-diff` and its exact allowlist
  entry. No ShowCommit, parser, environment or shared-fixture change.
- Exact process GREEN: handle 4286 joined exit 0, 14.666s.
- Exact API GREEN: handle 73275 joined exit 0, 15.832s.
- Exact securityutil GREEN: handle 89123 joined exit 0, 1.013s.
- Initial scoped lint 93084 was mistakenly launched before joining 89123.
  Stopped only its identified timeout process; 93084 joined exit 143, no verdict.
  Remaining owned heavy-process scan was empty. Parent explicitly released
  one serial replacement, without passing-test replay or scope expansion.
- Replacement exact scoped lint: handle 99975 joined exit 0, zero issues.
  GNU six-minute deadline, CLI five-minute timeout, concurrency 2 and serial
  runners at the reviewed base; no automatic resource retry.
- One pinned pnpm 9.15.9 frozen install from apps: handle 31447 joined exit 0,
  1.9s, all 923 packages reused, zero downloads. Existing Node runtime used;
  no dependency/lockfile changes. Framework and both normal hooks are active.

Native helpers re-execute each owning package's test binary under explicit
private environment and argument guards. Tests retain real Git and use built-in
oracles/snapshot diffs; no Windows-wide skip or production helper mirroring.
No old passing regression, full local suite, browser, build or audit was run.

Final document gates passed: catalog (343 decisions, 1321 specifications),
36 spec-linter tests, all-file spec lint, actual nine-file documentation coverage
(`covered`, no errors), whitespace and package inventory. Both work-package
files exist. This order is done for local implementation; hosted delivery
remains external and cannot be inferred from this status.
