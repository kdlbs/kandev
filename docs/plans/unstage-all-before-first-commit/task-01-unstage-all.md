---
id: "01-unstage-all"
title: "Restore first-commit Unstage all"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.37
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.38
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
---

# Task 01: Restore first-commit Unstage all

## Summary

Make empty-list Unstage work before the first commit using the smallest unconditional
Git argv correction. Independently authored real operator and registered HTTP regressions
must establish causal RED, then GREEN with selected-file and committed compatibility.

## In scope

- Add bounded initial/all and committed/all operator/HTTP regressions from the plan matrix.
- Change only empty-list `GitOperator.Unstage` argv from `reset HEAD` to `reset --`.
- Correct the process/runtime command comments and the public reference Unstage row.
- Update this order, manifest and version-safe task plan with actual command outcomes.

## Out of scope

HEAD detection, history/tracker, Discard/Revert, literal-path helper/admission, environment,
command/resource policy, transport/schema, UI/copy/layout/browser/build/E2E, PostgreSQL,
broad local suites, native delegates, recursive tasks, extra sessions or model changes.
Do not access/replay/copy/alter the protected ROOT proof.

## Acceptance

1. Production Stage all followed by Unstage all in an actually unborn private repository
   clears every index entry and preserves exact working bytes/permissions, symbolic HEAD,
   refs and config; permanent RED proves the causal failure before the correction.
2. Committed all-files, explicit literal selection/sibling preservation, staged
   additions/mixed edits/deletion and invalid empty-entry behavior remain correct through
   production operator and actual registered HTTP requests. Selected independent
   multi-repository routing preserves every other repository's index and bytes.
3. Task-defined GREEN, scoped lint and documentation gates pass with every original
   process handle actually joined; update only causal comments/public reference and
   record evidence. Return the local-heavy lease explicitly before hosted collection.

## Inputs and files likely touched

- [Manifest](plan.md), full regression matrix and supplied proof receipts.
- [Owning requirement](../../specs/platform/requirements/workspace-git-status.md), AC.37/.38.
- [Owning design](../../specs/platform/system-design/workspace-git-path-details.md), Literal selected paths.
- `apps/backend/internal/agentctl/server/process/git.go`: Unstage empty branch/comment.
- `apps/backend/internal/agentctl/server/process/git_unstage_initial_test.go`: new tests.
- `apps/backend/internal/agentctl/server/api/git_unstage_initial_test.go`: new tests.
- `apps/backend/internal/agent/runtime/agentctl/git.go`: command comment only.
- `docs/public/git-operations.md`: Everyday operations Unstage row only.
- Existing patterns: `process/git_pathspec_test.go`, `api/git_literal_paths_test.go`,
  `api/git_handlers_test.go`. Reuse real-Git helpers only when they preserve unborn setup.
- Guidance: backend, agentctl and API AGENTS; TDD backend test reference; fix/spec/plan,
  docs-maintainer and pure-data mobile exception.

## Dependencies and release barrier

No preceding work orders. ROOT must review the completed design package and later send an
explicit implementation INTERRUPT to this SAME primary session, with ONE GLOBAL
LOCAL-HEAVY lease. Design artifacts alone are not release. Mark this order in_progress
only after that release; no permanent tests, production edits, install, product check
or commit before it.

## Implementation sequence

1. Read current task-plan version, release message and owned diff. Confirm no original
   process handle from an earlier phase remains unjoined; record lease.
2. Add the four planned new test functions in two new files, with sequential subtests.
   For initial fixtures use real `git init` without committing, Stage through production,
   edit one added file after staging, snapshot before Unstage and compare afterward.
   Cover nil/empty all, literal selected name/sibling and invalid empty entry.
   Assert actual index membership/blob/mode/stage and exact file bytes with distinct
   sentinels. Capture HEAD/config/ref and environment/permission preservation.
3. Use actual `Server.Router().ServeHTTP` and manager/operator lookup, not direct handler
   invocation or mocked success. Exercise root unborn and two independent repositories
   under a non-repository root. Request selected repo through the registered endpoint,
   prove selected/all success and untouched other repo. Do not require unrelated
   unborn tracker enrichment or manufacture browser evidence. Join all fixture owners.
4. Run the single anchored permanent RED command below and actually join it. Expected
   Go exit 1 must be confined to unborn/all operator/HTTP assertions; selected and
   committed controls PASS. Save real command exit separately from wrapper exit.
   Any other failure checkpoints ROOT; no automatic retry.
5. Correct the empty argv unconditionally to `{"reset", "--"}`. Preserve explicit
   selectors, invalid empty entries, environment overrides, lock, runner and refresh.
   Run the single anchored GREEN command with the existing controls, actually join,
   then run scoped lint serially. Fix only grounded in-scope failures; checkpoint
   resource/timeout/transport/unknown failures without retry.
6. After GREEN, make the minimal public row and comment corrections. Run documentation
   gates, coverage preflight and diff checks. Record true results and statuses, return
   lease explicitly, then continue authorized normal publication using the task-plan
   hosted gates. Do not merge until separate serial ROOT grant.

## Verification

Run from the repository root under bash with login=false. Commands below are executable
bodies; launch each heavy body through a private owned receipt wrapper that records exact
PID/process-group, UTC start/cutoff, GNU timeout and actual exit. Retain every returned
session handle and poll that SAME handle to completion. One heavy process at a time.
No repeated passing test run and no duplicate launch to infer an earlier process outcome.

Use explicit installed tools:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
```

Permanent RED, after writing tests and before production change (expected exit 1):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 11m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^(TestGitOperatorUnstageBeforeFirstCommit|TestGitOperatorUnstageAllCommitted|TestHandleGitUnstageBeforeFirstCommit|TestHandleGitUnstageAllCommitted)$' ./internal/agentctl/server/process ./internal/agentctl/server/api)
```

GREEN with the focused existing controls, after correction (expected exit 0):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 11m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^(TestGitOperatorUnstageBeforeFirstCommit|TestGitOperatorUnstageAllCommitted|TestHandleGitUnstageBeforeFirstCommit|TestHandleGitUnstageAllCommitted|TestGitOperatorLiteralSelections|TestGitOperatorLiteralPathspecEnvironment|TestGitOperatorLiteralCaseSelection|TestHandleGitLiteralSelections|TestHandleGitStageAndUnstage)$' ./internal/agentctl/server/process ./internal/agentctl/server/api)
```

Scoped initial lint, serial, exact starting base:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run --concurrency=2 --allow-serial-runners --timeout=5m --new-from-rev=202d48bceb50ff839e1834d928d1677337fda672 ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl)
```

Documentation/format gates (only these are authorized in the design turn; public-doc
checks run after the implementation row correction):

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short
```

Coverage preflight: invoke the repository `.github/scripts/pr-docs.cjs`
`validateCoverage({changedFiles, fileContents})` export with this changed work order,
manifest, owning requirement and design, plus prospective owned production/test/doc
paths. Require `ok=true`, `status=covered`, `errors=[]`; use actual file contents,
not fabricated reference documents. This is a documentation gate, not a product run.

No package installation is needed for these Node/Python gates. After release only,
if apps/node_modules is absent, perform at most one pinned pnpm 9.15.9
`pnpm install --frozen-lockfile` from apps before active commit hooks.
No hook bypass or amend. No optional browser/build/E2E/PG or broad local verification.

A backend review fixup requires ONE full CHANGED lint from apps/backend against the
then exact PR base: `GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run ./... --new-from-rev="<exact-pr-base-sha>" --concurrency=2 --allow-serial-runners --timeout=5m`.
No backend replay for a docs-only fixup and no retry after a resource failure.

## Risks

- Fixture helpers that commit during setup can conceal the defect.
- Whole reset's existing ORIG_HEAD/reflog bookkeeping is a compatibility behavior;
  this local correction must not become a history-policy redesign.
- Assert Git index content, not byte-identical stat bookkeeping. Preserve all
  working bytes, including later unstaged edits to additions and existing files.
- Real HTTP manager fixtures own background work and must drain even on test failure.
- Unexpected Git/platform/resource behavior requires ROOT checkpoint before expansion.

## Parallelism

`sequential`. Same primary session owns all phases; no delegates.

## Results

ROOT released this reviewed package in the same primary after design END 02:15:53 /
WAITING 02:16:04 and granted the one local-heavy lease. Qualification is in
`/tmp/kandev-root-child77-reviewed-design-20261008.json`. The independently authored
permanent tests establish the supplied cause without accessing the protected proof.

The RED/GREEN and initial scoped-lint commands above ran exactly as documented.
Only initial all-files operator/HTTP subtests failed in RED; explicit, invalid and
committed controls and preservation assertions passed. GREEN passed all nine named
functions. Initial lint found one QF1003 in the new API test; changing its scenario
branch to a tagged switch required only these affected checks:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 11m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestHandleGitUnstageBeforeFirstCommit$' ./internal/agentctl/server/api)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run --concurrency=2 --allow-serial-runners --timeout=5m --new-from-rev=202d48bceb50ff839e1834d928d1677337fda672 ./internal/agentctl/server/api)
```

All original native handles were actually joined, not replaced or inferred. Detailed
argv, environment, UTC cutoffs and fresh empty-group observations are in the owned
`/tmp/kandev-child77-unstage-20261008/<job>.receipt.json` files; native join receipts
are sibling `<job>.native.json` files and complete command streams are `<job>.log`.

| Job | Original native / actual join | PID/group | UTC start / cutoff | True exit / result |
| --- | --- | --- | --- | --- |
| red | 48487 / f1b3c5 | 1454939 | 02:21:15.145766 / 02:32:15.145766 | 1 expected; process 0.200s / API 0.522s; wall 47.301s |
| green | 18322 / 3b2a5c | 1459788 | 02:22:27.963898 / 02:33:27.963898 | 0; process 3.823s / API 2.412s; wall 29.081s |
| lint | 48848 / a0f838 | 1465065 | 02:23:07.295886 / 02:29:07.295886 | 1; one API QF1003; wall 67.369s |
| http-after-lint | 3402 / ba0c2e | 1470563 | 02:24:31.775496 / 02:35:31.775496 | 0; API 1.309s; wall 9.109s |
| api-lint | 59648 / d14501 | 1472818 | 02:24:56.769542 / 02:30:56.769542 | 0; 0 issues; wall 5.809s |
| docs | 42094 / aa0f99 | 1475147 | 02:25:47.278781 / 02:30:47.278781 | 0; wall 2.025s |
| install | 37590 / 03f521 | 1476963 | 02:26:08.222443 / 02:36:08.222443 | 0; pnpm 9.15.9; wall 1.997s |

All dates are 2026-10-08; all command groups were empty after joining. Documentation
commands passed: catalog 363 decisions/1437 specs; spec tests 36; full spec lint;
public-doc tests 62; public pages 47; actual-path coverage covered/errors empty;
whitespace check. The install was conditional on missing apps/node_modules and used
`corepack pnpm@9.15.9 install --frozen-lockfile` from apps, once, without a lockfile edit.
Production changed only empty-list argv and matching comments; public reference changed
only the Unstage row. No frontend/browser/E2E/PG or broad product checks were run.

Task implementation is done. Normal hooks/publication, explicit lease return and hosted
merge-ready evidence remain delivery gates tracked in the version-safe task plan.
