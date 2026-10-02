---
id: "01-classify-status"
title: "Classify file status from raw metadata"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001
acceptance_criteria:
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.1
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.2
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.4
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.5
system_design:
  - ../../specs/platform/system-design/git-diff-file-metadata.md
---

# Task 01: Classify file status from raw metadata

## Summary

Make comparison status depend only on raw per-section extended headers. Prove
the correction through focused parser and real Git/process/HTTP regressions,
then deliver the complete repair through the parent's authorized normal PR loop.

## In scope

- Mark this order `in_progress` only after the later explicit parent INTERRUPT.
- Add permanent tests first; demonstrate expected false statuses in RED.
- Replace only classification in `parseCommitDiffWithOptions` using the small
  metadata helper described by the owning design; GREEN without assertion changes.
- Assert status plus exact keys/paths, patch bytes, counts, and preserved metadata.
- Update requirement/design status when accepted, manifest/order results, and
  bounded public-doc sentence. No new ADR is needed for this existing boundary.

## Regression detail

1. Parser table: each marker in unquoted and C-quoted paths, `+`/`-`/space-prefixed
   hunk lines, leading indentation and hunk descriptions. Include marker-like
   invalid extended-header lines, metadata-looking raw lines after payload
   boundaries, and a binary summary path with markers. These remain modified.
   Positive real-format controls: complete six-octal-digit added/deleted modes,
   empty-file metadata-only sections, executable/symlink modes, pure/edited
   rename headers, renamed paths containing competing marker phrases, ordinary
   mode-only and binary sections. Use full patch assertions, not just the helper.
2. Real caller fixture: track marker-named and marker-content files in the base
   commit; modify them so added, removed, and context marker text appears in real
   Git patches. Run `ShowCommit` and `GetCumulativeDiff` over the same known base.
   Include ordinary control, actual addition/deletion/rename, and binary control;
   cover real mode-only changes where supported. Rename similarity must be
   deterministic; assert Git emitted a rename header before relying on that
   positive control. Assert complete status maps, exact counts and independent
   raw Git patch evidence. Verify index and worktree are unchanged by reads.
   Isolate empty addition/deletion controls in separate comparisons: Git can
   legitimately pair simultaneous identical empty files as a pure rename.
3. Root/empty: genuine root additions with misleading content retain added;
   an allow-empty successor returns zero files/counts. Reuse existing first-parent
   merge and uncapped detail controls instead of duplicating them.
4. Budgeted parser: status survives per-file truncation and total-budget skip;
   counts remain from full sections, patches and skip reasons match existing
   behavior. Existing cumulative byte/file-cap tests cover the production caller.
5. HTTP: use `newGitAPIFixture`/`getGitAPI` patterns for registered commit and
   cumulative routes with real Git. Establish tracked baseline before modifying
   marker files; compare decoded fields and returned SHA/base/HEAD, not just 200.
   Add two repositories beneath a task root using the teardown pattern from
   `git_multi_repo_review_test.go`. Request commit with `?repo=alpha`, selected
   cumulative with that repo/base, and aggregate cumulative using configured
   per-repo branches. Distinct sentinels for same-name files must prove correct
   selection and `<repo>\x00<path>` aggregation with exact path,
   `repository_name`, `base_ref`, and the existing omission of `is_submodule`
   for these ordinary repositories. No mocking the Git result.

## Out of scope

Command/environment changes, new schemas/enums, history providers, workspace
mutation/porcelain/NUL numstat changes, UI work, broad tests/E2E, delegation,
rebase after hosted CI starts, and optional polish.

## Acceptance

- RED reproduces incorrect classification for all marker categories; GREEN
  passes negative and genuine metadata controls without relaxing assertions.
- Actual commit/cumulative callers and real HTTP repository routing expose
  corrected status with unchanged identity, payload, budgets, and comparison semantics.
- Exact checks pass and results are recorded before normal commit/push/ready PR;
  delivery finishes only after hosted gates, normal merge and independently
  verified merged SHA/owned cleanup.

## Verification

Run from repository root, sequentially. RED uses the first command before the
production edit; record its expected failure. After GREEN run each command once.
Retain and join every returned handle. The targeted test patterns cover only
the shared parser/callers and relevant HTTP routes, not whole packages.

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 2 ./internal/agentctl/server/process -run '^Test(ParseCommitDiffWithOptions_StatusMetadata|GitDiffStatusMetadataCallers|ShowCommit_StatusMetadataRootAndEmpty)' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/agentctl/server/process -run '^Test(ParseCommitDiff.*|ShowCommit.*|GetCumulativeDiff.*|GitDiffStatusMetadataCallers)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -race -p 2 ./internal/agentctl/server/api -run '^Test(GitDiffStatusMetadata.*|HandleGitShowCommit.*|HandleGitCumulativeDiff.*|MultiRepoReviewEndpointsUseStoredBaseBranches)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 golangci-lint run --concurrency=2 --new-from-rev=157502307f4292eb6de005b3c6317d22a45fbec2 --timeout=5m ./internal/agentctl/server/process ./internal/agentctl/server/api)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
/home/jcfs/.nvm/versions/node/v24.18.0/bin/node --test scripts/validate-public-docs.test.mjs
/home/jcfs/.nvm/versions/node/v24.18.0/bin/node scripts/validate-public-docs.mjs
git diff --check
git status --short -- docs/plans/git-diff-status-metadata
```

Also run the repository coverage API `.github/scripts/pr-docs.cjs`:
`validateCoverage({changedFiles, fileContents})` with the changed work order,
manifest, owning requirement/design, and intended changed production path.
Use actual on-disk UTF-8 documents and assert `ok`/`covered`; do not accept the
documentation-only exemption as cross-reference proof. Repeat with the actual
PR diff before delivery. No new permanent validation script is needed.

The Node path above is the verified existing executable for this child shell;
`go` and `golangci-lint` already resolve through PATH. Run the exact local design
coverage preflight below from the repository root:

```bash
/home/jcfs/.nvm/versions/node/v24.18.0/bin/node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/plans/git-diff-status-metadata/plan.md',
  'docs/plans/git-diff-status-metadata/task-01-classify-status.md',
  'docs/specs/platform/requirements/git-diff-file-metadata.md',
  'docs/specs/platform/system-design/git-diff-file-metadata.md',
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({filename, status: 'added'}));
changedFiles.push({filename: 'apps/backend/internal/agentctl/server/process/git_log.go', status: 'modified'});
const result = validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered') process.exit(1);
NODE
```

Normal hooks retain their existing gates. After ready PR creation, use the
configured authenticated full semantic-review evidence for the exact published
head, disposition all actionable findings, and wait for every actual required
hosted check. Do not request duplicate full review for an already complete
trusted exact-head full report. Follow parent instructions for normal expected-head
squash merge, verify the independently read merged SHA, and report cleanup.
Do not remove this platform-managed task worktree or unrelated branches/caches.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/git_log.go`
- `apps/backend/internal/agentctl/server/process/git_log_status_metadata_test.go` (new)
- `apps/backend/internal/agentctl/server/process/git_log_status_metadata_integration_test.go` (new)
- `apps/backend/internal/agentctl/server/api/git_status_metadata_test.go` (new)
- `docs/specs/platform/requirements/git-diff-file-metadata.md`
- `docs/specs/platform/system-design/git-diff-file-metadata.md`
- `docs/specs/platform/README.md`
- `docs/public/git-operations.md`
- This work order and sibling `plan.md`.

## Dependencies

None. Initial branch is based on verified main
`157502307f4292eb6de005b3c6317d22a45fbec2`; later explicit continuation is required.

## Risks

See the manifest. Preserve raw framing and pre-budget classification. Register
real fixture cleanup immediately; use existing manager teardown/tracker cleanup
and no arbitrary sleeps. New test files avoid growing existing large suites.

## Parallelism

`sequential`. No other workers/tasks/sessions.

## Inputs

- Owning requirement/design linked in frontmatter; parent archive and repro evidence.
- `git_log.go`, `git_log_diffstats_test.go`, `git_log_diffstats_special_test.go`,
  `git_test.go`, API `git.go`, `git_handlers_test.go`, `git_multi_repo_review_test.go`.
- Backend and agentctl/API scoped guides; `/tdd` and its backend-test reference.

## Results

Local implementation completed on 2026-10-02 after the later explicit parent
continuation. Hosted review, CI, merge and cleanup remain delivery gates tracked
in the task/session plan; this status records implementation and local validation.

- Permanent process RED: the first Verification command failed on false status
  labels, including all path/content categories and genuine metadata controls;
  package time 0.257s, handle 81635 joined with exit 1.
- Registered HTTP RED: `GOMAXPROCS=2 go test -p 2 ./internal/agentctl/server/api
  -run '^TestGitDiffStatusMetadata' -count=1` failed on wrong status through both
  routes and selected/aggregate repository reads; 0.234s, handle 39552 joined.
- GREEN: the first Verification command passed in 0.272s; handle 78386 joined.
- The exact targeted process race command passed in 2.803s; handle 63040 joined.
- The exact targeted API race command passed in 1.879s; handle 33083 joined.
- Scoped lint initially reported `dupword` in dirty-worktree fixture text.
  Replaced that marker text with another regression marker without changing
  assertions. `GOMAXPROCS=2 go test -race -p 2 ./internal/agentctl/server/process
  -run '^TestGitDiffStatusMetadataCallers$' -count=1` passed in 1.193s, handle
  69226 joined; the exact scoped lint command then passed with zero issues,
  handle 90140 joined. The failed lint handle 18958 also joined.
- The production delta changes only status classification. New tests verify
  raw full patches, exact identities/counts, repository metadata, immutable
  index/worktree observations, root/empty comparisons, and budgeted output.
- Specification/public-doc/coverage/diff gates are recorded in the sibling
  manifest. No desktop/mobile flow or broad local suite was added or run.
