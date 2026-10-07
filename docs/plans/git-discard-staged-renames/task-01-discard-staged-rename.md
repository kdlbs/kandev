---
id: "01-discard-staged-rename"
title: "Undo a selected staged rename safely"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.44
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.47
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
---

# Task 01: Undo a selected staged rename safely

## Summary

Independently reproduce the false successful destination-only Discard with actual GitOperator,
files and index, and through registered selected-repository HTTP. Add the smallest recognized
staged-rename selection/restore path, preserving literal selections and refusing source
occupation before request mutation. Verify status-visible rename identity and post-discard
bytes/membership, alongside ordinary controls and repository isolation.

## Admission

DESIGN ONLY until a later ROOT reviewed-package implementation INTERRUPT and explicit
exclusive local-heavy lease. No code/permanent tests/install/Go/fixtures/commit/PR now.
Same primary task/session/profile/executor as the durable task plan; no delegates/new tasks/
sessions/tabs/model switch. Mark this order in_progress only after release. Critical questions
use the direct-parent barrier and end the turn; preserve system marker, identity, title
ownership, user edits and completion gates in the live task plan. Parent interactions request
interrupt and may not be acknowledged; own plan and actual primary transcript are authoritative
checkpoints. Report tool delivery rejection before any fallback, never rely on acknowledgement.

## In scope

- `GitOperator.Discard` and a small process-local staged-rename capture/preflight helper.
- Existing literal HEAD restore for eligible pairs and existing ordinary removal/restore.
- Exact `-z` and `--untracked-files=no` validator admission, without variant acceptance.
- Independent operator/registered HTTP tests with real Git, status, files, index and controls.
- Minimal public reference correction and truthful manifest/work-order results.

## Out of scope

Whole-repository reset/clean, all-deletion restoration, copy inference, new wire/schema,
tracker text-parser migration, environment framework, global mutation/concurrency audit,
rollback, frontend production/layout/copy, unrelated suites and optional polish. Do not
touch child61/62 worktrees, foreign handles/caches/refs or managed worktree/dependencies.
Never replay/copy/import/mutate/remove ROOT's protected proof candidate or receipt/logs.

## Acceptance

1. Independently authored permanent real operator and registered HTTP RED tests fail for
   missing source/residual staged deletion on the unmodified baseline. Assert actual response
   and source/destination bytes/membership, with ordinary modified/added/untracked controls.
   Capture normal status-visible rename metadata before the HTTP call. No predicate-only or
   implementation-mirror test substitutes for that evidence.
2. The implementation follows `.44`/`.47` and the selected-staged-renames design. GREEN covers
   exact pair restoration, edited/mixed/multiple selections, native filenames, truthful refusal,
   ordinary/copy controls, unselected index/worktree content and selected-repository isolation.
   HEAD/config/refs/environments stay unchanged. No frontend production edit absent ROOT
   checkpoint confirming a causal wiring need.
3. Exact scoped checks and normal hooks pass within the lease. Record actual command/handle
   closure and public guide results. Local completion does not authorize merge or persistent
   task completion; honor the separate review/merge/ROOT closure gates below.

## Implementation packet

Read [Selected staged renames](../../specs/platform/system-design/workspace-git-path-details.md#selected-staged-renames)
and the manifest's evidence/identity before edits. Use `/tdd` plus its backend-tests reference
after release. New tests use existing private-repo helpers and raw outputs without trimming
blob bytes. API oracle/fixtures remain independent of process-test helpers. Mock only external
boundaries, such as a scoped test Git executable emitting failed or malformed status; use
real Git for all substantive selected/unselected file outcomes. Test-owned processes and
trackers must stop through their actual cleanup owners.

Read fresh full tracked porcelain with NUL framing before any mutation. Do not pass the
destination pathspec to pair discovery or use cached UI `old_path`. Consume R/C pairs in
destination/source order; only staged R plus eligible unique endpoints can expand selection.
No forced rename/copy configuration. Use literal HEAD tree inspection and source `Lstat`
preflight for every selected pair; reject occupied/unsupported/ambiguous pairs and failed
evidence before ordinary neighbors mutate. Recheck source absence at the rename boundary.
Restore both endpoints with existing HEAD/staged/worktree command, excluding the destination
from removal first. Keep copied command environments, operator lock, admission/deadlines,
result/error aggregation and refresh. Empty list/entry and invalid repository remain rejected.

### Permanent test matrix

| Function | Required real outcomes |
| --- | --- |
| `TestGitOperatorDiscardStagedRenames` | Pure `git mv` destination-only; rename with staged edits still recognized by Git; rename with different unstaged edits; two independent renames plus ordinary tracked/added/untracked selections; repeated endpoint selection deduplicated; source-only selection remains literal; ordinary modified, staged-added, untracked and explicit copy controls (source retained); unselected deleted file remains deleted/staged, proving no all-deletion restore. |
| Same operator function, native subcases | Exact Unicode, whitespace, tabs/newlines, quotes, literal ` -> ` and supported bracket/glob/pathspec-magic names at both endpoints. Verify raw Git recognition and exact index/file identity without arrow/line parsing. Exercise copied provider and ambient literal/icase settings with case-distinct decoys where native filesystem supports them; glob/noglob separately if affected. Skip only names physically unsupported on native Windows. |
| `TestGitOperatorDiscardStagedRenameRefusals` | Recreate source as ordinary file, directory or dangling symlink after staged rename; source/destination shared-endpoint chain or unsupported recognized shape; source/committed-destination preflight rejection where Git produces it; failed/malformed external status framing including incomplete second path. Include an otherwise valid ordinary selected neighbor and assert entire preflight rejection preserves all selected/unselected state. No fabricated parser predicate alone; enter Discard. |
| `TestHandleGitDiscardStagedRenames` | Real Server/Manager, non-Git root with independent selected/other repositories containing same endpoint names and repo-distinct markers. GET registered fresh status, decode destination and `old_path` (staged facet for mixed rename edits); send only destination via registered POST discard with `Repo:selected`; GET status again and prove no renamed/staged-deleted endpoint. Include pure and edited rename, multiple selection, independent staged/worktree neighbors, ordinary positive control. |
| `TestHandleGitDiscardStagedRenameRefusals` | Registered API occupied-source failure plus mixed ordinary selection, invalid/escaping/missing repository, nil/empty paths and empty filename; no request or other-repository mutation. |
| `TestIsKnownSafeGitFlagDiscardStatus` | Exact required flags admitted; e.g. `-zz`, `--untracked-files=all`, `--untracked-files=no-extra` rejected. This execution-boundary check complements the real regressions. |

Choose rename+edit content with high enough retained similarity to require actual R status;
assert R rather than hoping the fixture was recognized. For ordinary copies, assert the source
is not a deleted rename origin and remains untouched. A legitimate Git C record consumes its
origin field without expanding authority. Do not manufacture status-visible metadata in the
HTTP tests. If current tracker parsing blocks the exact status-visible scenario, checkpoint
ROOT and retain source identity; no silent tracker/UI expansion.

Capture exact index membership, mode/blob bytes and filesystem type/existence/content at
both endpoints and neighbors. Refusal compares complete pre-call content state. Successful
rename compares source to literal committed bytes and destination absence, then checks the
literal cached deletion set. Capture HEAD OID, symbolic HEAD/refs, local config bytes and
captured/process environments around the calls. Pair/status reads preserve content/refs/config;
raw index-file stat-cache bytes are not the read-only oracle. An ordinary read can update
Git bookkeeping. Keep fixture setup/oracles in private filtered environments with literal
arguments, never wildcard setup or ambient identity changes.

## Verification

Run commands individually, sequentially, only under the ROOT lease. Resolve the existing
Node 24.21.0 bin directory in PATH with `bash`, `login:false`; do not install a runtime.
If `apps/node_modules` is absent, perform exactly one conditional pinned frozen install
after lease, before pnpm checks/hooks:

```bash
(cd apps && PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" timeout --kill-after=10s 6m corepack pnpm@9.15.9 install --frozen-lockfile)
```

RED before production changes (expected causal assertions, actual nonzero Go exit):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 3m go test -trimpath -tags fts5 -race -p=1 -parallel=2 ./internal/agentctl/server/process -run '^TestGitOperatorDiscardStagedRenames$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 3m go test -trimpath -tags fts5 -race -p=1 -parallel=2 ./internal/agentctl/server/api -run '^TestHandleGitDiscardStagedRenames$' -count=1 -v)
```

GREEN and exact affected compatibility checks:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=2 ./internal/agentctl/server/process -run '^(TestGitOperatorDiscardStagedRenames|TestGitOperatorDiscardStagedRenameRefusals|TestGitOperatorDiscardLiteralSelections|TestGitOperatorDiscardLiteralEnvironment)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=2 ./internal/agentctl/server/api -run '^(TestHandleGitDiscardStagedRenames|TestHandleGitDiscardStagedRenameRefusals|TestHandleGitDiscardLiteralSelections|TestHandleGitDiscard_RestoresTrackedFile)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 3m go test -trimpath -tags fts5 -race -p=1 -parallel=2 ./internal/common/securityutil -run '^TestIsKnownSafeGitFlagDiscardStatus$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./internal/agentctl/server/process/... ./internal/agentctl/server/api/... ./internal/common/securityutil/... --new-from-rev=a5f6f7722ee0ebf6ae966f96a7d9c8a17ce6f8fc --timeout=5m --concurrency=2 --allow-serial-runners)
```

Record every original native handle/chunk, UTC start/end, exact argv/cwd/log, PID/PGID and
terminal exit. Retain and actually join each original handle; confirm owned PID/groups gone
before the next heavy operation. No wrapper PASS, discarded sessions, duplicate runs or
physical-cleanup inference. No new check broadening absent changes/failure. Setup/resource/
timeout/unknown/out-of-scope failure checkpoints ROOT, with no auto retry, cache wipe,
foreign kill or weakened assertion/timing/race check. Source changes justify only relevant
task reruns. One heavy at a time; lease return requires clean exact local/upstream/remote
OPEN-ready head and every actual original process joined/gone.

Lightweight root checks (design now; after spec changes later as needed):

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/git-discard-staged-renames
git status --short -- docs/specs docs/plans/git-discard-staged-renames
```

Use `.github/scripts/pr-docs.cjs`'s `validateCoverage` with actual changed work-order/plan/
owner/design contents and the planned runtime paths during design, then actual published
changes during delivery. Require status covered, ok true, errors[] and linked AC/REQ/design
coverage. The planned-path preflight proves linkage only, not production execution.

During implementation minimally edit the two Discard rows of the existing public reference,
then run from repo root using the existing Node PATH:

```bash
PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" node --test scripts/validate-public-docs.test.mjs
PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH" node scripts/validate-public-docs.mjs
git diff --check
```

An actual backend-code PR fixup additionally requires full CHANGED lint once against the
PR's exact API-reported base SHA, after recording it as `PR_BASE_SHA`; no moving-main base,
synthetic merged tree or automatic retry:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./... --new-from-rev="$PR_BASE_SHA" --timeout=5m --concurrency=2 --allow-serial-runners)
```

### Grounded relative-spelling fixup

Greptile finding 4202800563 identified a real remaining failure: `./new.txt` is accepted by
literal Git selection but missed the exact rename map and endpoint exclusion. ROOT released
an exclusive fixup lease for this correction in the same order. Audit raw admission first:
the API passes raw paths, and the existing runner defers arguments after `--` to Git/process
execution. A cleaned candidate must never replace that admission. Verify the original literal
spelling with fresh NUL status before matching an eligible endpoint; keep ordinary raw
arguments unchanged. Canonical and verified alias endpoints share selection/dedup/exclusion.
Do not broaden accepted paths, trim bytes, or reinterpret literal POSIX backslashes.

Independently authored additional operator and registered HTTP tests reproduce `./destination`
on the published implementation before correction, including status-visible rename origin,
source restoration/residual deletion, canonical plus alias duplicates and recreated-source
refusal with ordinary selected neighbors. Raw NUL arguments rejected before cleaning must
remain rejected. Alias source-only selection stays literal; portable alias paths and native
separator/literal-backslash controls remain enabled where representable.

Exact additional RED selectors (before correction), with the same resource envelope:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 3m go test -trimpath -tags fts5 -race -p=1 -parallel=2 ./internal/agentctl/server/process -run '^TestGitOperatorDiscardStagedRenameRelativeSelections$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 3m go test -trimpath -tags fts5 -race -p=1 -parallel=2 ./internal/agentctl/server/api -run '^TestHandleGitDiscardStagedRenameRelativeSelections$' -count=1 -v)
```

Affected GREEN selectors add each relative-selection function to its process/API command
above. Process compatibility passed before adding source-only/native-Windows test inputs;
rerun only the exact relative-selection function for those new inputs. No securityutil,
public-doc validator, installation or broad initial suite replay is justified. Follow with
scoped changed process/API lint against the published parent and exactly one mandatory full
CHANGED lint against the API-reported PR base:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./internal/agentctl/server/process/... ./internal/agentctl/server/api/... --new-from-rev=35610f31eb8f51cbf418f785a8afb1a4cc00c492 --timeout=5m --concurrency=2 --allow-serial-runners)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./... --new-from-rev="$PR_BASE_SHA" --timeout=5m --concurrency=2 --allow-serial-runners)
```

Retain the original observer and cutoffs through corrected-head publication. Normal active
hooks/fixup commit/push, accurate package results and actual-finding disposition remain
required; no optional polish, rebase, hosted retry or successor observer.

The first full CHANGED invocation ended at the GNU six-minute timeout with exit 124 and an
empty log, so it supplied no lint verdict and its cause remains unknown. Its original native
handle was actually joined and wrapper/process groups verified gone. ROOT then explicitly
authorized one identical bounded recovery with retained caches and the same exact API base.
This is the only recovery exception; no third invocation, resource increase or gate
substitution is authorized. Both original receipts/logs remain part of delivery evidence.

## Delivery gates after later release

Normal active hooks and standing-authorized Conventional Commit, push and ready PR follow
scoped checks. Do not disable hooks or leave a dirty hook reformatted tree. Freeze published
SHA except actual findings; no rebase onto moving main, optional polish or hosted retry.
Load local commit/push/pr/pr-fixup skills only when their phase is reached, honoring these
explicit standing overrides. Publication alone cannot mark this persistent task COMPLETE.

Use one original all-terminal waiter, once, with its original cutoff and retained handle
across findings:

```bash
timeout --kill-after=10s 91m scripts/pr-await "$PR_NUMBER" --mode all-terminal --deadline-min 90 --format json
```

The design source audit confirms these existing helper options. Record the concrete argv
and original deadline when running. Do not launch a self-successor waiter. Keep primary commentary/checkpoints
responsive while retaining original handle ownership. Require authenticated configured
CodeRabbit App 347564 substantive FULL current-head review of every changed file. Inspect
skipped/running/completed evidence before at most one necessary review-gap request; ACK is
not semantic review and no optional Claude wait is required by ROOT.

Ground and disposition every actual finding. Fresh complete known six required checks and
actual Backend/Frontend/E2E parent jobs must all be SUCCESS, with errors[] and visible,
hidden and human findings clear. Report MERGE READY END and await a separate ROOT serial
MERGE INTERRUPT. Only then perform normal expected-head squash, no admin bypass. Independently
verify actual merged SHA/tree/all owned blobs and remote-main inclusion, plus every owned
original handle joined/gone. Preserve managed worktree/dependencies and all foreign state.
ROOT independently verifies/archive/proof release/refill; no premature completion.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/git.go` (Discard only).
- `apps/backend/internal/agentctl/server/process/git_discard_staged_renames.go` (new bounded helper if needed).
- `apps/backend/internal/agentctl/server/process/git_discard_staged_renames_test.go` (new).
- `apps/backend/internal/agentctl/server/api/git_discard_staged_renames_test.go` (new).
- `apps/backend/internal/common/securityutil/git.go`, `git_test.go` (exact flags only).
- `docs/public/git-operations.md` (two reference rows).
- This work order, manifest and owning requirement/design (accurate results/lifecycle).

No API production, frontend, generated types, package config, lockfile or AGENTS.md change is
currently indicated. Existing conventions stay accurate.

## Dependencies

None besides the later reviewed-package release and exclusive local-heavy lease. Existing
literal Discard implementation is part of the baseline, not an unmerged dependency.

## Risks

Git may expose A/D instead of R based on similarity/configuration; no guessed restoration.
Occupation/refusal must precede removal, including mixed selections. External mutation is
not atomic with preflight. Text tracker parser limits can require a ROOT checkpoint for
status-visible special names. Do not broaden the parser/runtime/API policy to hide a failed
fixture or unsupported case. Exact flag admission must remain bounded.

## Parallelism

`sequential`

## Inputs

- [Manifest](plan.md), source hashes and accepted read-only ROOT proof classification.
- [Owning requirement](../../specs/platform/requirements/workspace-git-status.md), `.44`/`.47`.
- [Path-details design](../../specs/platform/system-design/workspace-git-path-details.md).
- Current `git_discard_literal_paths_test.go` in process/API and `git_handlers_test.go`
  helpers; source-status capture/parser, API registration/repository resolution.
- [Completed literal-selection package](../git-discard-literal-selections/plan.md) for
  unchanged compatibility scope and historical evidence, not test copying.
- Public Git operations reference and scoped backend/agentctl/API guidance.

## Results

ROOT reviewed the four-artifact design package and released this order under the exclusive
local-heavy lease in the same primary session. No code/tests/fixture/install/Go work ran during
the historical design turn. Implementation and local checks are complete; hosted CI/review
and separate ROOT merge authorization remain delivery gates.

- Independent real operator RED: pure/staged/mixed rename restoration assertions failed;
  ordinary controls and unrelated bytes stayed valid. Registered HTTP RED proved actual
  rename visibility before Discard and a residual staged deletion after the false success.
- Independent refusal RED: recreated file/directory/dangling-link sources, empty entry and
  six scoped external failed/malformed/unsupported/overlapping evidence shapes failed
  causally. Fault injection is limited to full tracked NUL status; real ordinary operations
  remain enabled. Both exact required flags failed baseline validator admission.
- Final exact GREEN race commands above passed: process 7.087s, API 7.809s, securityutil 1.013s.
  Operator coverage includes native names, edited/mixed/multiple/deduplicated/source-only
  selections, copies and ordinary controls, unselected staged deletion, index mode/blob/file
  bytes and occupied-source preflight. Registered fresh status → Discard → fresh status proves
  selected-repository isolation and no residual rename/deletion; no mocked metadata or Git.
- Exact scoped lint passed with 0 issues. The ordinary classification was extracted unchanged
  into the bounded helper for the existing function limit, followed by the final process rerun.
- Two public Discard reference rows updated; public tests 62/62 and validator 47 pages passed.
  Catalog, full specification lint and whitespace checks passed. Structural coverage is
  recorded in the manifest/durable task plan using actual changed paths before publication.

All originals were retained and actually joined before each next heavy command; exact argv,
cwd, logs, UTC, native chunks and PID/PGID/physical closure are in the own task plan and
`/tmp/kandev-child63-d6c521cc/` receipts. Normal active hooks/pinned frozen dependencies and
published-head monitoring remain delivery prerequisites. No frontend production or browser
execution was needed under the mobile pure-data exception. No universal external-writer
atomicity, inferred A/D restoration, full-repo mutation or ROOT proof replay is claimed.

Relative-spelling fixup results: independent operator and registered HTTP RED failed for
accepted aliases, duplicate endpoints and occupied-source mutation; raw-NUL rejection stayed
valid. Affected operator GREEN passed in 7.669s, followed by a narrow new-input rerun in
2.200s; registered HTTP GREEN passed in 9.798s. Scoped changed lint passed with 0 issues.
The first mandatory full CHANGED run timed out (exit 124, empty log, cause unknown). After
its actual join/physical closure, the one identical ROOT-authorized recovery passed with
0 issues in 252.938s and was also joined/gone. Exact originals are retained, including the
failed timeout. No passing initial securityutil/public-doc/install checks were replayed.
Normal active hooks and corrected-head publication follow; current-head hosted CI, substantive
full-file review and separate ROOT merge authorization remain pending.

### Grouped copy-endpoint review correction

CodeRabbit review 5437366168 identified that parsed C endpoints did not contribute to the
shared-endpoint refusal count. ROOT confirmed this violates the existing `.47` contract and
released the smallest accounting correction in this order. C records remain ordinary copy
evidence, with no pair restoration or copy-discard feature. Alias discovery stays limited
to recognized R endpoints so ordinary copy-source spellings retain literal behavior.

Before production edits, add actual GitOperator refusal RED using correctly framed R/C
shared-source and shared-destination evidence at the existing controlled full-status command
boundary. All filesystem/index/ordinary-neighbor snapshots and mutation commands remain real
Git. Label this as boundary-fault evidence, not native-generated copy evidence. Include
independent-copy and standalone-copy raw/alias controls, HEAD/refs/config and environments.
The new exact selector is `^TestGitOperatorDiscardCopyEndpointEvidence$`, using the same
trimpath/fts5/race/p1/parallel2/GOMAX2/GOMEM512MiB/3mkill10 envelope. No API production or
transport branch changes; operator evidence owns the new parser boundary. Run only the new
anchored operator GREEN, scoped changed process lint against parent f08285d, then one new
full `./...` CHANGED lint against the freshly API-confirmed a5f6 base with the existing
GOMAX2/GOMEM1GiB/concurrency2/serial/CLI5m/GNU6mkill10 bounds. This is a new actual-code
correction allowance, not a third invocation of the prior unchanged recovery. Failure
requires ROOT checkpoint, with no automatic retry. No passing aliases/security/public-doc/
install/broad suite replay. Normal active hooks, corrective commit/push, grouped finding and
three informational dispositions, final-head FULL review and physical heavy return follow.

The new copy-endpoint operator RED failed causally for shared source/destination mutation;
independent and standalone-copy alias controls passed. GREEN passed all four cases in 1.586s,
and scoped changed process lint returned 0 issues. The new mandatory full CHANGED original
then timed out at six minutes (exit 124, empty log, cause unknown), supplying no lint verdict.
It was actually joined and its wrapper/PID/process group verified gone. Current production,
test and documentation changes stay uncommitted at published parent f08285d; no automatic
retry, commit, push, review-gap request or gate substitution follows this failure. ROOT
timeout/resource decision is required before further local-heavy work. The same observer
and original cutoffs remain live. No physical-return or merge-ready claim is made.

After reviewing that failed original and physical closure, ROOT explicitly authorized one
identical bounded recovery for this copy correction, retaining caches and all original
limits. The earlier failed run remains a timeout with no verdict and unknown cause. No
cache-population explanation, affected-test replay, resource increase or third invocation
is authorized. A clean recovery permits normal hooks and publication; another failure
requires a ROOT checkpoint without publication or a hosted substitute.

The one identical ROOT-authorized copy recovery passed with 0 issues in 191.081s. Its original
native handle was joined and wrapper/PID/process groups verified gone. The original exit-124
timeout remains recorded separately with unknown cause. Together with new operator GREEN
and scoped lint, this completes local checks for the five-path correction. Normal active
hooks/publication and grouped review disposition follow; no prior passing checks were
replayed. Final-head hosted CI/FULL review and separate ROOT merge authorization remain gates.
