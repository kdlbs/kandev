---
created: 2026-10-07
status: in_progress
requirements:
  - REQ-WORKSPACES-SAVED-FILE-CONTENT-001
system_design:
  - ../../specs/workspaces/system-design/saved-file-content.md
legacy_specs: []
---

# Implementation Plan: Keep Private Save Patches Out of Git

## Overview

Keep private editor-save input out of the user's working tree and real Git
index while preserving successful disk saves and ordinary staging. One
sequential work order independently establishes the current producer's RED,
changes only its patch transport, and verifies real save/staging outcomes.
The four-document package was reviewed and sealed at `905fa03c` before ROOT's
later explicit interrupt released implementation and exclusive local-heavy
ownership in the same primary conversation. Local implementation and validation
are recorded below; hosted observation and merge require separate releases.

## Ownership and assumption check

Workspaces owns filesystem mutation and truthful saved-content results. Extend
its existing saved-file-content pair rather than create an incident or UI spec.
The platform staging contract remains authoritative for selection semantics.

Confirmed intent: no private save artifact may be staged; preserve existing
stage/save callers, selected repository targeting, bytes/hash/ACK, fallback,
cancellation and portability. No atomic save-and-stage ordering is requested.
Static source facts and archived runtime evidence are distinguished below.
The local transport choice is made under ROOT's autonomous design instruction;
there is no unresolved product decision or model/delegation request.

## Source qualification and root cause

Initial clean HEAD and authoritative main both resolved to
`905fa03c5b8ae90c661dc9fec2e324d355e269aa`. Main was recovered with one
explicit second remote read after the first tool response truncated its output;
there was no remote polling, fetch, checkout or ref mutation.
Selected-repository-save dependency
`9b4af97250f491d69d14629b549b3f33ddc66c12` is an ancestor of HEAD and its
`workspace_files.go` has no diff from HEAD. ROOT independently verified and
archived that merged dependency; this turn does not replay its proof.

Qualified Git blobs at initial HEAD:

| Boundary | Blob |
| --- | --- |
| `process/workspace_files.go` | `cef590f5c17a6b4841b778526fc46461383ce613` |
| `process/git.go` | `58c465894f042d511d64b972a8f93eb7e9066b7d` |
| `common/subproc/shared.go` | `0dca1bfffee14508aaed9dbfc37c1dc875c4b2c4` |
| `api/workspace.go` | `b7536ae730e0469f34b9009c0a8a54e254b2b35b` |

Paths above are relative to `apps/backend/internal/agentctl/server/`, except
`common/subproc/shared.go`, which is relative to `apps/backend/internal/`.
The actual producer writes `os.CreateTemp(workDir, ".kandev-patch-*")` before
`RunGitCombinedAfterAcquire`. An earlier queued Stage All uses `git add -A`
and can stage that untracked file; later removal cannot remove its index entry.

Accepted ROOT diagnostic: native `68425`, chunks `b5a4f6` to `08b5ef`, baseline
`62b39941214ffe63ce72d307b6e599a7bd2a7b63`, fixed `.kandev-patch.tmp` helper.
The overlap failed on the actual index while save bytes/hash and both success
results were correct; a fresh sequential control passed. Protected candidate
`/tmp/kandev-root-stage-save-patch-next-candidate_test.go` is mode `0400`, SHA256
`0f928ca22d6f51e72ddd4cd35ad02e55258c7e1a129cb281b3f21a153f7e5111`.
Read-only receipt, classification and the receipt's actual
`/tmp/kandev-root-stage-save-patch-proof-test.log` support those facts. The
unsuffixed `...proof.log` named in the request is absent; no replacement was
created. The receipt records original joining, group teardown and scratch
removal; this turn did not re-prove their process state. No current random-helper,
browser, HTTP or commit execution is claimed. Never replay, copy, import,
mutate or delete the archive.

## Scope

In scope: `ApplyFileDiff`'s private patch input, removal of its now-unneeded
sole-use helper/defer, independently authored real-Git process regressions and
registered-route integration evidence. Preserve all existing caller contracts.

Out of scope: Stage All filtering, ignore rules, global writer locks/frameworks,
patch parser or unmatched-header authorization changes, same-target writer
ordering, frontend/layout/copy/translation/browser/build/E2E, runtime flags,
dependencies, other lifecycle defects and managed-worktree cleanup.

## Technical approach

Supply the final rewritten diff with `strings.NewReader` as `cmd.Stdin` in
the existing post-admission builder. Replace only the patch filename argument
with Git's documented `-` stdin input. Remove `writeFileDiffPatch` and the
patch removal defer. Keep Git flags, working directory, admission class,
execution timeout, managed lifecycle and result/fallback branches.

The helper has one producer caller and one Git consumer; no external filename
contract exists. Managed Git preserves stdin on Unix/Windows. Native temp
overrides can point inside a checkout, so merely using `os.CreateTemp("", ...)`
would not establish the required boundary. This correction creates no patch
file or manual pipe lifecycle. The unrelated Git-metadata index snapshot path
is unchanged. No new ADR is needed for this local input choice; the owning
design records the constraint and alternatives.

## Tests

All criteria use prefix `AC-WORKSPACES-SAVED-FILE-CONTENT-001`.

| Criteria | Planned evidence |
| --- | --- |
| .2, .5 | New `TestApplyFileDiff_StageAllOverlap`: held cap one, actual Stage All first, actual save second, exact real-index paths/blobs, final bytes/hash/applied result and success for both operations |
| .2, .5 | New `TestApplyFileDiff_StageAllSequentialControl`: fresh fixture, saved bytes staged; genuine patch-like dotfiles/additions/deletion remain ordinary Stage All input |
| .1-.5 | Existing distinct-file, cleanup, cancellation, regular/symlink/conflict and repository-target controls; new overlap subcase with native temp variables inside checkout and a large non-ASCII patch without overwrite fallback |
| .2, .5 | New `TestHandleFileUpdate_StageAllOverlap` and `StageAllSequentialControl`: registered save/stage routes, real Manager/Git/index and truthful HTTP result; named-repository save verifies nonselected same-name files |
| .2-.4 | Existing registered-route distinct-file and repository-target compatibility tests |

The permanent tests were independently authored after implementation release.
RED exercised the actual random-helper producer before changing production;
the protected fixed-name diagnostic was never imported.
The anchored selectors were source-audited: they match exactly 11 existing
process controls and four existing registered API controls. Naming the conflict
and repository controls explicitly preserves the former selector's match set;
the two new names per package were planned and absent at the design audit. No
suite was broadened or product test executed during that audit.

## End-to-end operation evidence

Registered `POST /api/v1/workspace/file/content` and `/api/v1/git/stage`, backed
by real Git, prove disk, real index and caller ACK outcomes. These constitute
the causal end-to-end operation check. No artificial Playwright flow is added.
Mobile-parity does not apply: this changes backend patch input only, with no
rendered surface, shared frontend state, responsive, touch or copy change.
Desktop and mobile continue using the same existing save/staging contracts.

Public docs audit covered `developer-tools.md`, `sessions-and-review.md`,
`git-operations.md`, root README and screenshot catalog. They describe existing
editing and repository-scoped staging, without a private patch-file contract.
No command, workflow, schema, terminology or screenshot changes warrant public
guidance edits. Recheck only if implementation uncovers a material change.

## Work orders

- [ ] [Task 01: Use private patch input for file saves](task-01-private-patch-input.md)
  (`in_progress`, sequential, no internal dependency; local checks passed,
  hosted/native-platform delivery gates pending)

## Verification results

The original coverage attempt in native chunk `30a0eb` did not start because
`node` was unavailable on the login shell's PATH. Its later Git status command
yielded overall exit zero; that is not coverage success. The original remains
unrun, with no installation or automatic retry.

ROOT then explicitly authorized lightweight design preflight only, using
existing Node `24.21.0` and Go `1.26.0` via explicit PATH under `/bin/bash`,
`login:false`. One corrected known-runtime coverage invocation and the following
document gates passed:

- Catalog validation: 357 decisions and 1416 specifications.
- Specification-linter tests: all 36 passed; full specification lint passed.
- Exported `validateCoverage`: actual four-document diff `exempt`, errors `[]`;
  prospective producer package `covered`, errors `[]`, with the one work order
  and owning requirement/design references accepted. This is reference coverage,
  not runtime or production-code evidence.
- Local links, untracked-document whitespace, tracked diff whitespace,
  anchored selector/source audit and inventory passed. Exactly the four design
  documents are changed; the index is empty and HEAD remains `905fa03c`.

Retained original: native `56219`, initial chunk `a694bc`, terminal chunk
`e3660e`, joined exit zero. UTC interval
`2026-10-07T06:56:36.005045Z` to `2026-10-07T06:56:37.728933Z`.
All 11 direct commands were joined with observed absent groups. Wrapper
PID/PGID `1263341` and every command group were independently observed gone
at `2026-10-07T06:56:54.665884Z` (native chunk `3941be`, exit zero).
Exact argv/cwd/PID/PGID/UTC/60-second immutable cutoffs and per-command logs:
`/tmp/kandev-child67-private-save-patches/design-preflight-61x11k3s/receipt.json`;
the native terminal receipt is its sibling `native-terminal.json`.
No live native handles remain. No protected ROOT proof was modified.

No product checks, production/permanent-test edits, installation, product lint,
heavy lease, commit, PR, hosted observer or merge ran. Prior delivery packages
remain historical, completed records and are not reopened or relabelled here.

### Later implementation release and local results

ROOT reviewed the amended package in native `97fd56`, sealed its four hashes
and explicitly released implementation plus GLOBAL LOCAL-HEAVY EXCLUSIVE67.
The permanent independently authored tests reproduced four actual random-helper
index leaks; both sequential controls passed, and save/stage success, bytes and
hash remained correct. Original RED `12205` / `4b5c88` to `ee8487` joined exit
one at `2026-10-07T07:04:33.572110Z`. Its wrapper/group were observed gone.

The minimal producer correction removes the helper/defer and attaches the
rewritten string reader to the existing Git command with explicit `-` input.
No staging/caller, flags, parser, API, event or fallback/cancellation changes.

- Process GREEN: `84121` / `6ca787` to `fc4fd5`, joined exit zero at
  `2026-10-07T07:05:32.818585Z`: all 13 top-level tests and 60 cases passed,
  zero skips.
- Registered API GREEN: `35404` / `5a4743` to `c02078`, joined exit zero at
  `2026-10-07T07:06:16.224402Z`: all six top-level tests and 47 cases passed,
  zero skips.
- Scoped lint: `58544` / `d2c675` to `714d3a`, joined exit zero at
  `2026-10-07T07:09:04.874641Z`, zero issues. Exact initial base, serial runners,
  concurrency two, CLI five-minute/GNU six-minute bounds were retained.

Receipts, original command logs and native terminal/gone observations are in
the exact owned `/tmp/kandev-child67-private-save-patches/` directory, in
`RED-4y21r4bt`, `GREEN-process-zs8kbqx9`, `GREEN-api-9iojiagt` and
`scoped-lint-l3agd4eh`. Every original command is joined; nested process IDs
not independently observed are explicitly unknown. No production or test edit
followed GREEN. Linux evidence does not replace the later hosted native gate.

Final document gates passed: catalog 357/1416, all 36 spec-linter tests,
all-spec lint, actual changed-path `validateCoverage` (`covered`, errors `[]`)
and whitespace/inventory. Original `2986` / `05fba1` to `08169b` joined exit
zero at `2026-10-07T07:11:48.676649Z`, with all six inner command joins/groups
recorded in `document-gates-mjkzr77y/receipt.json` and its supervisor in
`document-gates-7x__dwjv/receipt.json`, under the same exact owned directory.

Because worktree dependencies were absent, the single authorized
`corepack pnpm@9.15.9 install --frozen-lockfile` from `apps/` passed, original
`15982` / `254473` to `f6c931`, joined exit zero at
`2026-10-07T07:09:53.719361Z`. It reused 935 packages, downloaded none, and
reported pnpm 9.15.9. Receipt `frozen-install-y53c11hg`; dependencies remain
in the managed worktree. Normal active-hook publication follows local checks.
The platform plan owns exact final publication/physical-return receipts;
hosted observation and merge are not released. The order remains in progress
for the independently required hosted/native-platform evidence.

## Risks and delivery gates

Pipe transport must preserve byte content, managed cancellation and native
Windows behavior. Large valid input without desired-content fallback detects
application/pipe errors; Linux success or cross-compilation cannot replace
actual hosted native-platform execution. Every started test request must be
joined before restoring global admission, including failure paths.

ROOT's global local-heavy lease, physical cleanup receipts, hosted-observer
release, substantive exact-head CodeRabbit review and separate serial merge
grant remain necessary. Their full constraints and task/session identity live
in this task's version-safe platform plan. Resource/timeout/transport/unknown
or out-of-scope findings require an actionable ROOT checkpoint in that plan
and the primary conversation, with no automatic retry. Do not proceed from
this design handoff without the later explicit interrupt implementation release.
