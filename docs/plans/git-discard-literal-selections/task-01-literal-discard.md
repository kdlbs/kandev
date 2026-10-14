---
id: "01-literal-discard"
title: "Make Discard filename selections literal"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.44
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
---

# Task 01: Make Discard filename selections literal

## Summary

Prove the mixed untracked bracket failure and native magic failure through real Discard callers,
then correct the three selected Git command sites together. Verify exact selected/unselected
index and worktree content, independent registered HTTP repository routing and inherited
selected environment controls. Keep existing behavior outside filename selection.

## Admission

DESIGN ONLY until a LATER explicit ROOT reviewed-package IMPLEMENTATION INTERRUPT plus an
explicit global local-heavy lease. Generic workflow envelopes are not admission. Work only
in this task's own worktree and existing primary session; no additional agents/tasks/sessions/
tabs/model changes. The 2026-10-05 design turn ended WAITING. ROOT subsequently reviewed all four artifacts
and explicitly released implementation and the global local-heavy lease to this primary.
Validation precision below follows that release. Merge authority is still absent.
Parent callback queue is full; checkpoint in own task plan, no callbacks/questions/retries.

## In scope

- `GitOperator.Discard`'s selected status command, `discardUntrackedFiles`'s `rm --cached`
  command, and `discardTrackedFiles`'s `restore` command in `process/git.go`.
- Reuse the existing `literalGitPathspec` and `runGitCommandWithEnvironment` convention:
  copied selected child environment overrides `GIT_LITERAL_PATHSPECS` and
  `GIT_ICASE_PATHSPECS` to `0`; never mutate inherited provider/process settings.
- Real Git fixtures and assertions below; inspect existing route registration/handler/
  `gitOpForRepo` / `Manager.GitOperatorFor`, but keep API production code unchanged.
- Minimal implementation-time existing public guide clarification; accurate own work-order/
  manifest results, normal active hooks and conditional delivery gates.

## Out of scope

Generic environment/cache frameworks, parser/rename repair, transactions/rollback, directory
remove expansion, shared policy, passing Stage/Unstage refactors, API/schema/new service,
UI layout/copy, browser/E2E/PostgreSQL, unrelated full suites, duplicate incident specs,
shared caches/foreign processes/paused oversized child/unproved volume, and proof replay.
Keep ROOT's proof artifacts until ROOT archive.

## Acceptance

1. Before production edits, permanent actual `GitOperator.Discard` tests fail on the portable
   mixed untracked bracket/dirty tracked decoy case, and supported native magic tests fail for
   the accepted wrong-file restoration. Collect actual exit and failing byte assertions;
   wrapper success never constitutes Go PASS. Independent registered HTTP tests also fail
   for affected selection/routing outcomes. Ordinary tracked bracket is a passing control.
2. All three selected command sites use the existing literal representation and selected
   environment overrides. Preserve original raw paths for filesystem deletion, `--`, empty
   list and invalid empty-entry rejection, locks, command validation/admission/deadlines,
   classification/fallback, result/error aggregation and refresh. Every test matrix row below
   passes after correction with exact index membership/content and worktree existence/bytes.
3. Exact checks and normal hooks pass under the lease/resource limits; guide/reference and
   plan/work-order results reflect actual commands. Work-order completion is local only.
   Persistent task completion requires the separate delivery gates and actual verified merge.

## Test implementation packet

Add bounded test files rather than growing the already-large `git_test.go`. Use existing
private-repo helpers, raw Git output, logger and registered-server test helpers. The
existing `runGit`/`runGitAPI` functions return raw content; keep content comparisons untrimmed.
Use exact NUL membership and exact blob reads rather than matching path substrings. No helper
concatenation tests or synthetic passing tests. Annotate `.44` coverage next to actual callers.

| Planned function | Cases and oracle |
| --- | --- |
| `TestGitOperatorDiscardLiteralSelections` | Ordinary tracked with separate HEAD/staged/worktree content; ordinary untracked; ordinary index-added; tracked bracket passing control; untracked `new[ab].txt` plus tracked dirty `newa.txt`; added bracket plus staged/worktree decoy; tracked `:(glob)a*.txt` / `alpha.txt` and supported `:(exclude)alpha.txt` magic; multiple selections mixing tracked/untracked/added; `nil`, empty list and `[]string{""}` rejection without unselected mutation. |
| `TestGitOperatorDiscardLiteralEnvironment` | Copied provider environment and ambient fallback. Independently exercise inherited `GIT_LITERAL_PATHSPECS=1`, `GIT_ICASE_PATHSPECS=1`, both together, `GIT_GLOB_PATHSPECS=1` and `GIT_NOGLOB_PATHSPECS=1`. Include portable mixed bracket and added bracket to cover status/removal; case-distinct sibling to cover restore. Assert provider and ambient environment unchanged afterward. Do not combine conflicting GLOB/NOGLOB settings; the inherited literal/case pair is intentionally neutralized by the selected overrides. |
| `TestHandleGitDiscardLiteralSelections` | Instantiate real Server/Manager and send JSON through registered `POST /api/v1/git/discard` with `GitDiscardRequest`. Non-Git root contains `selected` and `other` independent repos with identical names and repository-distinct HEAD/staged/worktree markers. Exercise portable mixed bracket, added bracket, multiple paths and native magic. Set inherited selected controls in `InstanceConfig.AgentEnv`, with distinct ambient settings, to exercise manager/operator environment routing. Invalid repository and empty request return existing errors and leave both repositories unchanged. |

Each fixture captures selected and unselected index membership and blob bytes, plus worktree
existence/bytes, before the call. Tracked selected files must end with the fixture's literal
HEAD content in both layers. Added/untracked selected files must be absent in both layers as
applicable. Independently staged decoys retain staged content and a different worktree value;
other repository files retain their full pre-call snapshot. This detects both restore and
`rm --cached` contamination. Keep the HTTP oracle independent of process-test helpers.

Fixture setup and oracle commands use a private filtered Git environment and exact literal
selection. Use original raw names for filesystem APIs and exact index identity; never feed
matching wildcards into fixture setup. Skip native magic names on Windows only because their
colon/star characters cannot be represented. Portable bracket cases always run. Case-only
siblings skip only after `os.SameFile` establishes actual filesystem aliasing, not by OS alone.
No `t.Parallel` in environment-sensitive or actual-Git cases. Close/join only fixture-owned
manager/tracker work through existing cleanup; never terminate foreign processes.

The accepted archived proof is read-only and is not rerun. Permanent implementation RED is
new evidence, particularly for the raw Git-only portable bracket candidate. If RED does not
prove the expected defect, checkpoint the mismatch rather than broadening scope or inventing
an alternate failure.

## Verification

All commands below run from repo root, sequentially, after both admissions. Retain every
returned handle, record inner exit status, elapsed time and scoped result, and ACTUALLY JOIN
before starting replacement work. No heavy operation overlaps a hook/install/test/lint.
A resource failure ends at a checkpoint: no automatic retry, cache wipe, foreign kill or use
of the paused oversized child/unproved volume. Use `/tdd` during implementation.

First mark this work order `in_progress`, add permanent tests, then collect RED while
production stays unchanged. The environment-only RED below additionally protects the selected
override behavior. These commands are expected to exit 1 for the stated defect,
so collect each result independently rather than treating the wrapper as a PASS:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 3m go test -trimpath -tags fts5 -race -p=1 -parallel=2 ./internal/agentctl/server/process -run '^TestGitOperatorDiscardLiteralSelections$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 3m go test -trimpath -tags fts5 -race -p=1 -parallel=2 ./internal/agentctl/server/api -run '^TestHandleGitDiscardLiteralSelections$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 3m go test -trimpath -tags fts5 -race -p=1 -parallel=2 ./internal/agentctl/server/process -run '^TestGitOperatorDiscardLiteralEnvironment$' -count=1 -v)
```

After the minimal correction, run the new Discard callers and the existing ordinary HTTP
Discard control once. `.37`/`.38` and the prior Stage/Unstage package remain historical context;
untouched Stage/Unstage suites are not rerun or claimed as coverage. Hosted CI supplies other gates:

```bash
set -euo pipefail
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=2 ./internal/agentctl/server/process -run '^(TestGitOperatorDiscardLiteralSelections|TestGitOperatorDiscardLiteralEnvironment)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=2 ./internal/agentctl/server/api -run '^(TestHandleGitDiscardLiteralSelections|TestHandleGitDiscard_RestoresTrackedFile)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./internal/agentctl/server/process/... ./internal/agentctl/server/api/... --new-from-rev=eb589f279dc8527101b71fdaf99ce523909a5299 --timeout=5m --concurrency=2 --allow-serial-runners)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" node --test scripts/validate-public-docs.test.mjs
PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" node scripts/validate-public-docs.mjs
git diff --check
git status --short -- docs/plans/git-discard-literal-selections
```

Catalog/specification/coverage and diff checks are lightweight and may run during design;
Go/Node tests, product lint/build/typecheck/browser and installs may not. Public-doc validator
tests above are deferred to implementation under the heavy lease. No additional full suite.

Run this lightweight PR-documentation coverage preflight at design and implementation
checkpoints. Prospective production paths test traceability, not implementation success:

```bash
PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" node <<'JS'
const fs = require('node:fs');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/plans/git-discard-literal-selections/plan.md',
  'docs/plans/git-discard-literal-selections/task-01-literal-discard.md',
  'docs/specs/platform/requirements/workspace-git-status.md',
  'docs/specs/platform/system-design/workspace-git-path-details.md',
];
const source = ['apps/backend/internal/agentctl/server/process/git.go'];
const result = validateCoverage({
  changedFiles: [...docs, ...source],
  fileContents: Object.fromEntries(docs.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify({ok: result.ok, status: result.status, errors: result.errors}));
process.exitCode = result.ok ? 0 : 1;
JS
```

Before normal active commit hooks, install workspace dependencies once only if absent. Use
pinned pnpm 9.15.9; do not mutate the lockfile or shared caches. Under the heavy lease:

```bash
if [ ! -d apps/node_modules ]; then
  (cd apps && PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" timeout --kill-after=10s 6m corepack pnpm@9.15.9 install --frozen-lockfile)
fi
```

Normal active hooks are required; do not disable them. Respect their own resource admission
and retain/join the commit handle. If the install or hooks cannot finish within the admitted
budget, checkpoint for ROOT; no automatic repeat or bypass. Re-stage hook formatting only
in the later authorized implementation/delivery turn, then create a new conventional commit.

For every actual backend-code PR fixup, obtain the exact live PR base SHA and validate the
local commit exists. Run full CHANGED lint once, with GNU outer 6m, CLI 5m, kill-after 10s,
GOMAXPROCS 2, GOMEMLIMIT 1GiB, concurrency 2 and allow-serial once; do not substitute a moving
main ref or a package-only scan. With `PR_BASE_SHA` set from the retained exact PR snapshot:

```bash
: "${PR_BASE_SHA:?Set the exact live PR base SHA from retained PR evidence}"
git cat-file -e "${PR_BASE_SHA}^{commit}"
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./... --new-from-rev="$PR_BASE_SHA" --timeout=5m --concurrency=2 --allow-serial-runners)
```

Rerun only affected caller checks after a real correction. Never create synthetic tests or
rebase solely for main drift. Record resource/command failures before further work.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/git.go` (only three scoped Discard sites).
- `apps/backend/internal/agentctl/server/process/git_discard_literal_paths_test.go` (new).
- `apps/backend/internal/agentctl/server/api/git_discard_literal_paths_test.go` (new).
- `docs/public/git-operations.md` (minimal reference clarification, implementation only).
- `docs/specs/platform/requirements/workspace-git-status.md` (minimal `.44`).
- `docs/specs/platform/system-design/workspace-git-path-details.md` (existing selector design).
- `docs/plans/git-discard-literal-selections/plan.md` and this work order.

Read, reuse and preserve `git_pathspec.go`, Stage/Unstage tests, handler/registration, manager
routing and prior literal-selection package. No production edit to those passing paths.

## Dependencies

None. Existing literal helper and selected environment seam are already present in this
checkout. No dependency work, subagents or new persistent tasks are authorized.

## Risks

Unqualified status can choose a decoy category before either mutation. Fixing restore alone
will not fix untracked removal. Empty entries must never become all-tree literal selectors.
Index absence must be asserted independently of worktree removal. Filename support and
case aliasing need narrow skips, preserving the portable core. Partial-error semantics and
directory behavior are intentionally unchanged.

## Parallelism

`sequential`

## Inputs

- Existing Platform requirement `.44` and path-details Literal selected paths section.
- This plan's accepted actual production proof, separate raw Git-only candidate and blob receipt.
- `git.go`, `git_pathspec.go`, process raw Git helpers and existing literal tests.
- API `server.go`, `git.go`, `git_handlers_test.go`, `git_literal_paths_test.go` and
  `Manager.GitOperatorFor` / `gitEnvironment`.
- Scoped backend/agentctl/API guidance; `/tdd`, `/mobile-parity` no-UI assessment and
  `/docs-maintainer`. Public guide is a reference; no root README/screenshots change.

## Conditional delivery and completion

Standing delivery starts only after ROOT's reviewed-package implementation interrupt and
explicit global local-heavy lease. Use normal `/commit`, `/push`, `/pr` workflows to create
ready PR after checks. Preserve immutable head except real corrections.
Retain ONE hosted all-terminal collector and ACTUALLY JOIN before replacement. Require six
known required checks at actual terminal success, CI parent terminal, complete error-free
exact-head snapshots, and authenticated configured CodeRabbit App 347564 FULL substantive
current-head all-file review. Resolve required-check names from actual PR evidence, not guesswork.
Inspect sufficient automatic report before ONE necessary full-review request; ACK/skipped/
progress is insufficient and no optional second review wait is authorized. Ground and
record disposition of every actual finding; defer optional polish and head churn.

After clean checkpoint/all joins, await separate ROOT MERGE LEASE. Use normal expected-head
squash, no administrator bypass. Independently verify actual merged SHA/tree/owned blobs and
remote state; clean only owned joined resources. Preserve ROOT proof until ROOT archive.
Marking this work order `done` and manifest `implemented` never completes the persistent task;
that happens only after actual verified merge. Persist task/session/system marker, user edits,
handles, leases, barriers, receipts and next action in own platform task plan throughout.

## Results

ROOT reviewed-package implementation release and global local-heavy lease granted on
2026-10-05. Validation narrowed to `.44`, targeted race tests at GOMAXPROCS=2,
GOMEMLIMIT=512MiB and -p=1, one heavy command at a time.

- Final permanent process selection RED: exit 1, package 0.974s; portable mixed untracked
  bracket, native glob/exclude and multiple selections fail for selected/unselected bytes.
  Ordinary tracked/untracked/added, original unstaged tracked-bracket control and exact empty
  inputs pass. Initial strengthened staged bracket setup was aligned with the original
  passing control before collecting this final RED; production remained unchanged.
- Independent registered HTTP RED: exit 1, package 5.599s; wrong selected/decoy bytes in the
  selected repository and inherited selection failures. Other repository bytes are
  independently checked. Invalid request guards pass. Cold compilation completed within
  the documented three-minute cap. Handle 88554 actually joined.
- Exact environment-only RED: exit 1, package 2.298s, actual Git with inherited literal/case
  settings. Supported glob/noglob controls include passing cases. Handle 90849 joined.
- Process GREEN: exit 0, package 4.196s, exactly the two new Discard functions, including
  ordinary/control, portable/native/multiple, empty and 30 environment/provider subcases.
  Handle 97978 joined.
- API GREEN: exit 0, package 6.798s, exactly the new routing function plus existing ordinary
  `TestHandleGitDiscard_RestoresTrackedFile`. Handle 79656 joined.
- Catalog validation: PASS, 351 decisions and 1355 specifications; specification lint PASS.
- Initial scoped process/API lint: exit 0, zero issues, exact starting-base SHA,
  concurrency 2, GOMAXPROCS=2/GOMEMLIMIT=1GiB, GNU outer six minutes / CLI five minutes
  / kill-after ten seconds / allow-serial once. Handle 90213 actually joined.
- Public-doc validator tests: 62 PASS; live validator: 47 published pages PASS.
  Documentation coverage preflight includes the actual eight changed paths: covered,
  ok:true, no errors. Whitespace checks PASS, including new files.
- The ONE pinned pnpm 9.15.9 frozen install: exit 0, 2.2s; handle 68988 actually joined.
  Package manifest and lockfile unchanged. No cache wipe or foreign-process mutation.
- Normal active hook receipt and exact-head ready-PR publication are recorded in the
  own platform task plan as delivery proceeds. Local work-order verification is done;
  persistent task completion and merge remain separately gated.

No unchanged Stage/Unstage suite reran, and none is claimed as coverage. Production edits
remain limited to the three scoped commands. ROOT proof was not replayed. Hosted CI, full
configured review, separate ROOT merge lease and actual verified merge remain external gates.
