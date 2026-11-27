---
id: "01-preserve-ignored-gitlink-evidence"
title: "Preserve recorded gitlinks under ignore preferences"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001
acceptance_criteria:
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.5
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.12
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.13
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.14
system_design:
  - ../../specs/platform/system-design/git-diff-file-metadata.md
---

# Task 01: Preserve recorded gitlinks under ignore preferences

## Summary

Independently prove the qualified ignored-gitlink defect through actual operators
and registered selected/aggregate HTTP routes, then correct the two patch argv
after exact securityutil admission. Keep parent fallback evidence available
without altering initialized-child anchors, routing, parser or Review composition.

## In scope

- Exact admission in `apps/backend/internal/common/securityutil/git.go`, with
  scoped admission and rejected-variant tests in `git_test.go`.
- ONLY the two patch argument lists in
  `apps/backend/internal/agentctl/server/process/git_log.go`.
- New independent `git_log_ignore_submodules_test.go` in process and
  `git_ignore_submodules_test.go` in API, owning their disposable Git/HTTP fixtures.
- Minimal owner-pair and this plan/work-order status/result updates.

## Out of scope

Protected discovery source and replay, shared fixture edits, parser/global
Git/status/tracker/child discovery/anchor/precedence/history/schema/API/frontend
changes, new frameworks, dependencies, broad passing replay, sibling packages,
new ADR and public docs edits. No initialized-child aggregate emptiness claim.

## Acceptance

1. Exact ignore-none and ignore-dirty are admitted, with bare/abbreviated/empty/unsupported values,
   suffixes, whitespace and control variants rejected. Commit detail retains
   none; cumulative parent comparisons use dirty to retain actual pointer changes
   without child-only tracked/untracked dirt decorations. Initialized child files
   remain in their existing child scopes and parent-recorded anchors.
2. Independent causal RED then GREEN proves committed gitlink fallback after
   real child deinit through both operators and selected/aggregate HTTP, with
   explicit old/new IDs, patch/count/metadata and routing oracles; bounded
   ordinary/default/ignore-mode/unchanged and initialized-anchor controls pass.
3. Owned config/HEAD/refs/index/worktree and absent-child state remain unchanged;
   existing targeted compatibility checks pass once, scoped lint and document
   references pass, and all original native/subprocess handles actually join
   with fresh owned absence evidence.

## Inputs and dependencies

No prior work order. Use the [manifest](plan.md) for exact evidence, source
hashes, bounded scenario matrix and delivery barriers. Follow the
[owning design](../../specs/platform/system-design/git-diff-file-metadata.md#visible-parent-gitlink-comparisons)
and [requirement](../../specs/platform/requirements/git-diff-file-metadata.md).
UI AC-UI-SUBMODULE-REVIEW-001.5 is an existing linked outcome, not a new owned ID.
The [nested-scope ADR](../../decisions/2026-08-05-nested-submodules-as-repository-scopes.md)
retains discovery and parent-recorded child comparisons.

Read scoped backend/agentctl/API guidance and TDD backend-test instructions
after implementation release. Existing `git_log_submodule_format_test.go` and
`git_submodule_format_test.go` show local operator/registered-router conventions;
do not inherit their config-sensitive oracles or change shared fixture helpers.

## Initial sequential execution (historical completed pass)

1. Await LATER ROOT reviewed-package implementation INTERRUPT plus explicit
   exclusive GLOBAL LOCAL-HEAVY release in this SAME primary. Recheck exact head
   and material source hashes; drift checkpoints ROOT without rebase. Then mark
   this one work order in_progress. Protected mode0400 source SHA256
   `1ea256ccd96290d7d8e42b4bcc7f577da182934fdd20f0c9ba7bae9cd14a0e5b`
   is never accessed in any phase.
2. Independently author exact admission/variant regressions and obtain anchored
   causal RED before securityutil production change. Author own actual operator
   and HTTP regressions; establish private home/Git environment before fixture
   creation and operator/manager capture. Obtain their causal RED before patch
   producer changes; passing controls are part of this bounded run.
3. Admit exact ignore-none first, then add it to both existing patch argv after
   subcommands and before refs. No metadata-query or parser edits. Run affected
   GREEN; process GREEN includes listed compatibility selectors once; HTTP GREEN
   includes its stable-child control once. Do not replay already passing sibling
   suites or redundant passing controls after later fixture-only repairs.
4. Run scoped lint; ordinary formatting/causal fixture fixes rerun only affected
   selectors or lint. Any resource/timeout/transport/unknown/out-of-scope failure
   checkpoints ROOT before alternatives. Persist owned command receipts and
   native joins throughout, including crash continuation.
5. Complete light checks and accurate plan/work-order results. Normal local
   publication follows the later release and standing delivery contract below.
   Return local heavy and actually END before separate hosted release. Completion
   of this work order's implementation is not task completion or merge permission.

## Fixture and oracle contract

Use actual private child commits, actual declared parent gitlinks and actual
`git submodule deinit` for fallback. Operator matrix: deinitialized forward
upgrade under default/all, BOTH methods; initialized none/untracked/dirty forward
controls; backward under all; ordinary file under all; genuinely unchanged
clean gitlink/empty commit/cumulative HEAD; cumulative dirty recorded pointer
advance. No cross-product explosion or invented dirty-child policy.

Own registered HTTP fixture: independent alpha/beta histories, matching gitlink
path but distinct parent bases and old/new child IDs. Deinitialize before manager
discovery for selected and aggregate default/all tests; verify only parent scopes
are present. Initialized-child test under all separately verifies own child file
patches and parent-recorded anchors. Selected requests pin repo; aggregate
cumulative includes required `base` query while scopes use their stored bases.
Assert NUL-qualified keys, exact path/repository_name/base_ref, is_submodule on
child files only, counts and commit/cumulative metadata. No server port/DB/browser.

Use native Git on Windows and other platforms, portable paths and scoped local
file protocol permission. Isolate HOME/USERPROFILE/XDG and inherited Git overrides
before construction; pass a copied explicit env. Oracle argv always includes
`--no-color --no-ext-diff --no-textconv --submodule=short --ignore-submodules=none`
and fixed a/b prefixes, with first-parent/format-empty for commit patches.
Explicit positive IDs, membership and +1/-1 counts prevent vacuous empty passes.
Snapshots bracket reads after setup and include config/.gitmodules bytes,
HEAD/refs/index entries, ignore-none non-mutating status and tracked worktree
content; canonical snapshot diffs are equally helper-proof. Resolve linked child
Git metadata; assert deinitialized worktree absence stays absent. Stop/join owned
managers, retain raw command bytes and prove cleanup only for owned resources.

## Initial verification commands (historical completed pass)

Execute from repo root with actual `/bin/bash`, login=false, existing PATH.
Every heavy command is serial under ROOT's exclusive lease, never an untracked
parallel call. Retain original native session/chunks, subprocess PID/PGID,
cwd/argv/environment/start/bounds/raw log/exits/ACTUALJOIN and fresh exact owned
absence. GNU outer timeout6m/kill10 and Go/CLI timeout5m apply throughout.

Independent RED before relevant production changes:

```bash
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/common/securityutil -run '^TestIsKnownSafeGitFlag(AllowsIgnoreSubmodulesNone|RejectsIgnoreSubmodulesVariants)$' -count=1 -timeout=5m -v)
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/process -run '^TestGitComparisonIgnoreSubmodules(Controls)?$' -count=1 -timeout=5m -v)
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/api -run '^TestGitComparisonIgnoreSubmodules(HTTP|MultiRepoHTTP|InitializedChildHTTP)$' -count=1 -timeout=5m -v)
```

GREEN: repeat the securityutil selector above once; use these process/API commands
instead of separately replaying new tests and then repeating them with controls:

```bash
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/process -run '^(TestGitComparisonIgnoreSubmodules(Controls)?|TestGitComparisonPlainOutput(RootAndEmpty|Merge)|TestGetCumulativeDiff_(StablePrefixesIgnoreGitDiffConfig|TruncatesLargeFile|BudgetExceeded|CapsFileCount)|TestShowCommit_NotCapped)$' -count=1 -timeout=5m -v)
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/api -run '^(TestGitComparisonIgnoreSubmodules(HTTP|MultiRepoHTTP|InitializedChildHTTP)|TestNestedSubmoduleReviewEndpointsIncludeRootAndStableChildBase)$' -count=1 -timeout=5m -v)
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run ./internal/common/securityutil ./internal/agentctl/server/process ./internal/agentctl/server/api --new-from-rev=bd63da3163271e3229bf387649135855caac300d --concurrency=2 --allow-serial-runners --timeout=5m)
```

An actual backend PR fixup requires ONE full CHANGED lint before push, against
the freshly verified exact PR base. Replace the placeholder only after refresh;
retain ROOT lease and these bounds. Docs-only fixup does not replay backend lint.

```bash
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run ./... --new-from-rev='<exact-refreshed-PR-base-sha>' --concurrency=2 --allow-serial-runners --timeout=5m)
```

Light DESIGN checks (permitted now):

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py specs --text git-diff-file-metadata --format paths
git diff --check -- docs/specs/platform/requirements/git-diff-file-metadata.md docs/specs/platform/system-design/git-diff-file-metadata.md docs/plans/git-submodule-ignore-review-evidence
git status --short -- docs/plans/git-submodule-ignore-review-evidence
```

Use `.github/scripts/pr-docs.cjs` actual `validateCoverage` with local document
contents: four-document diff must be exempt/error-free, then four docs plus the
two planned production paths must be covered/error-free by this ONE work order
and real owning pair. This is reference validation, not implementation evidence.
Resolve every REQ/AC/design and all real compatibility test names; future new
test names are an explicit authorship contract. No Go or pnpm check during DESIGN.

## Surface and public guide audit

The manifest records the actual public-guide audit and existing Review fallback
test locations. Pure-data mobile exception: no composition, copy, touch,
breakpoint, scrolling or navigation change, so process/registered HTTP evidence
is sufficient. No public edit, preview, E2E build or browser run is required.

## Standing delivery contract

After release only, inspect dependencies. If apps/node_modules is absent, ONE
`corepack pnpm@9.15.9 install --frozen-lockfile` from apps under the same lease,
actual Bash login=false and Node24.21.0 using ROOT-qualified existing PATH prefix
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin`. ROOT resolved the original
Node24.18.0 design checkpoint before the implementation release. No runtime
download, substitution or system edit is permitted. Preserve deps/cache and
normal active hooks. Conventional commit/push/ready PR uses exact repo template,
unchecked checkboxes and preserves bot additions. Follow local publication skills.

Canonical caller-bound repository ID:
`16026b06-bd79-47c0-aed1-dc7ca95f63d9`. Verify complete errors=[], actual canonical
repo/PR/head and FIVE false automation flags. Correct association means ZERO
relink/patch; known absence ONE initial link; unknown reconcile before mutation.

LOCAL-HEAVY RETURN plus actual END precede separate ROOT hosted release. One
original attached90m GNU91m kill10 cadence60 collector; six required contexts
plus actual Backend/Frontend/E2E parents SUCCESS on exact head; fresh complete
errors=[], zero actionable visible/hidden threads/changes-requested/human gates.
Authenticated configured CodeRabbit App347564 substantive FULL CURRENT ALL
actual-path source=covered-head kind=reviewed evidence; automatic completion ZERO
requests, at most ONE necessary request only for a proved completed gap. Join
original observer and prove owned absence before any ROOT-authorized replacement.
Freeze head except valid findings; no main-only rebase/synthetic merged tests/
broad passing replay/weakening/optional polish/duplicate observer/blanket rerun.

MERGE NONE until later ROOT serial static grant. Normal expected-head squash,
noadmin; verify actual merge SHA/tree/all owned blobs/remote/inclusion, all
original joins and only-owned cleanup. Preserve managed worktree/deps/caches/
foreign refs/paused resources/protected ROOT source for ROOT independent archive.
Taskcomplete requires verified merge plus joined cleanup. Persist next actions
and crash checkpoints in OWN plan; optional callback never gates and is never
repeated on queue-full. Same task/session/model through all phases.

## Risks and parallelism

Exact admission ordering, ignore-sensitive oracles, wrong child metadata paths,
missing HTTP base binding and environment capture are the concrete risks.
`sequential`: one work order; no parallel agent work.

## Initial results (historical)

Implementation acceptance passed after the later ROOT release. Independent
securityutil/process/registered HTTP RED reproduced the cause; exact admission
and two producer flags made affected tests GREEN. Raw patch, metadata/count,
read-only, default/ordinary/unchanged and initialized-child anchor checks passed.
See the manifest's released results for every original handle, terminal chunk,
package time, targeted lint correction and zero-issue scoped lint.

The one frozen pnpm9.15.9 install passed with ROOT-qualified Node24.21.0; no
lockfile change or runtime download. Original native/subprocess actual joins and
fresh owned absence are in /tmp/kandev-child97-ignore-review-20261009. Final
reference/catalog/spec36/all-spec/whitespace checks passed; actual nine-path
coverage is covered with errors=[] and this ONE work order. Normal active-hook
publication evidence is recorded in the own versioned task plan. GLOBAL LOCAL-HEAVY remains exclusively
CHILD97 until explicit RETURN/actual END; HOSTED HOLD, MERGE NONE. Work-order
completion does not mean persistent-task delivery is complete.


## Current bounded corrective execution

1. ROOT's explicit corrective release grants exclusive CHILD97 local heavy while
   retaining original read-only collector6859. Update this same owner package;
   independently add actual-Git child-dirt regressions and exact dirty admission
   coverage. Establish meaningful process/registered aggregate/securityutil RED
   before production correction; absent causal RED stops for ROOT checkpoint.
2. Admit exact dirty, retain exact none; change only cumulative none to dirty.
   Run affected GREEN plus recorded-pointer/deinitialized fallback/initialized
   anchors. Do not replay unrelated root/budget/color compatibility tests.
3. Run ONE full changed backend lint against freshly verified actual PR base,
   concurrency2/allowserial/CLI5m/GNU6m/kill10, GOMAX2/GOMEM1GiB. Resource,
   timeout, transport or unknown failure checkpoints ROOT before alternatives.
4. Record truthful current results, light references/catalog/spec36/alllint and
   whitespace; normal new conventional commit/hooks/push. Preserve body/checklist
   and bot additions. Reply/resolve the fixed CodeRabbit thread with actual
   evidence; defer optional Greptile expansion in a grounded truthful reply.
5. Explicit FIXUP LOCAL-HEAVY RETURN with original joins/raw hashes/serial
   intervals/fresh exact own absence, exempt only retained collector6859.
   CONTINUE authorized hosted on the new head in this same turn. Old semantic
   coverage is historical; accept new sufficient automatic FULL coverage with
   ZERO requests. MERGE NONE until distinct ROOT grant; no replacement observer.

Corrective checks from apps/backend with original owned wrappers and reviewed bounds:

```bash
go test -trimpath -tags fts5 -race -p=1 ./internal/common/securityutil -run '^TestIsKnownSafeGitFlag(AllowsIgnoreSubmodules(None|Dirty)|RejectsIgnoreSubmodulesVariants)$' -count=1 -timeout=5m -v
go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/process -run '^TestGitComparisonIgnoreSubmodules(ChildDirt|Controls)?$' -count=1 -timeout=5m -v
go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/api -run '^TestGitComparisonIgnoreSubmodules(ChildDirtHTTP|HTTP|MultiRepoHTTP|InitializedChildHTTP)$' -count=1 -timeout=5m -v
golangci-lint run ./... --new-from-rev=bd63da3163271e3229bf387649135855caac300d --concurrency=2 --allow-serial-runners --timeout=5m
```

RED initially selects only ChildDirt/ChildDirtHTTP and exact security admission;
GREEN adds the listed relevant pointer/fallback/initialized controls once.
Use GOMAXPROCS=2/GOMEMLIMIT=512MiB for tests; lint memory1GiB.
Corrective acceptance is complete; see the current results below. No optional test cross-product.


## Corrective resource checkpoint (historical)

Meaningful runtime RED established unchanged-pointer tracked/untracked child dirt
and contaminated pointer-plus-dirt evidence. The first HTTP RED also exposed
fixture setup after manager anchor capture; the corrected fixture-only rerun
failed solely on false parent rows while dirty child bytes/anchors passed.
Exact dirty admission RED failed while none/variant controls passed.

Affected GREEN passed: securityutil 1.013s, process 3.634s and registered HTTP
3.760s, including the relevant original pointer/fallback/initialized controls
once. The production delta remains cumulative none-to-dirty and exact dirty
admission only; ShowCommit none is preserved.

The mandatory ONE full changed backend lint at freshly verified actual base
`bd63da3163271e3229bf387649135855caac300d`, concurrency2/allowserial,
GOMAX2/GOMEM1GiB/CLI5m/GNU6m/kill10, hit the outer timeout with exit124 and
no diagnostics. Original53516/90d8a7->16ad70 and subprocess1039927 were
actually joined; fresh exact owned absence is empty. This is NOT lint success.
No retry, alternative command, commit, push or thread resolution followed.
All eight corrective local-heavy originals have verified serial intervals and
raw hashes in `/tmp/kandev-child97-ignore-review-20261009/fixup-timeout-checkpoint.json`.

Local heavy is paused for the mandatory ROOT resource checkpoint. All nine
corrective files remain unstaged/uncommitted at published head15e0e6a30.
Original collector6859/wrapper956186/subprocess956187 remains attached and
retained, never stopped/restarted/replaced. It is the only allowed overlapping
resource. ROOT must direct the next bounded step before any local alternative.
Protected source/managed worktree/deps/caches/foreign resources remain untouched.
Publication, fresh-head semantic/CI gates and delivery remain pending; MERGE NONE.


## Historical bounded corrective results

ROOT independently qualified the original timeout and authorized exactly ONE
identical warm-cache recovery with retained caches and unchanged bounds/base.
Original60157/cea5b9->883c0b returned exit0 and actual `0 issues.`; both native
and subprocess1076125 joined with fresh exact owned absence empty. Original
53516 remains FAILED exit124 with empty diagnostics and unproved cause. No
source changes or passing tests were replayed during recovery; no cache wipe,
runtime/memory adjustment or foreign-resource mutation occurred.

Actual runtime RED and affected race GREEN protect tracked/untracked child-only
dirt, real pointer changes plus dirt, parent fallback and distinct initialized
child ownership/anchors. Exact dirty admission supplements none; only cumulative
uses dirty, while ShowCommit remains none. No parser/UI/config/discovery/API
redesign or optional test-matrix/docstring expansion.

The same ONE work order's local implementation and full changed lint acceptance
are complete. Normal active-hook fixup publication, actual thread dispositions,
canonical FIVEfalse and explicit local-heavy RETURN are recorded externally in
the own versioned task plan. Original collector6859 was retained across that earlier push and later joined
terminal FAILURE; continuation70603 also joined terminal FAILURE. Both observers
are gone. Their prior-head review coverage is historical after a new publication.
Same-turn authorized hosted continuation requires all current readiness gates;
MERGE NONE until distinct ROOT serial static grant. Task delivery is incomplete.

## Scoped CI remediation: continuity test positioning

The hosted Pierre continuity case repeatedly failed its existing visible-anchor
assertion after prior preview fixtures left committed diff entries. A matched
private three-commit fixture reproduces the 14-file review inventory and line
counts without replaying preview scenarios or starting preview servers. All
three bounded repetitions fail: the initial scroll clamps against lazy
placeholders, then preceding diffs expand and leave 140 target rows below the
viewport with zero intersections until the unchanged 30-second assertion fails.

This same work order now includes a test-only CI correction in
`git-refresh-continuity-helpers.ts`, the existing desktop continuity spec and
one immediate private-history fixture helper. Preserve all existing visible
anchor, reading-state, count and refresh assertions, timeouts and strict flaky
policy. No product UI, renderer, backend, API or fixture architecture changes.

First confirm permanent matched-history RED on the original helper. Then align
using the scroll root and target's current rectangles during readiness polling,
so lazy preceding layout changes cannot leave the selected diff out of view.
Validate the corrected Pierre desktop case, the affected Monaco desktop case
and existing Pierre mobile touch case, one resource-bounded invocation at a
time. Test-only changes may use managed `--no-build` after the proven fresh
same-head backend, Vite and plugin build; never bypass freshness guards.
Run applicable changed-file lint, type, discovery, doc/reference and whitespace
checks, then active normal hooks and a new fixup commit/push. No backend test or
full-backend lint replay applies to this E2E/docs-only correction.

The matched-context diagnostic establishes causal RED; the permanent regression
and affected controls pass after the test-helper correction. Hosted CI on the
new published candidate remains a separate delivery gate. ROOT separately releases hosted observation after
explicit local-heavy return and END/WFI. Hosted retry budgets remain spent;
this correction does not authorize another rerun, review request or merge.

### CI-remediation checkpoint

Matched-context diagnostic: three causal visible-anchor failures with 14 files,
140 target metadata rows and zero viewport intersections after lazy preceding
layout expansion. Permanent stable-history regression: one causal RED on the
original helper, then three Pierre desktop GREEN repetitions after current
rectangle alignment moved into the existing readiness poll. Monaco desktop
and Pierre mobile touch controls each pass once without retries. Existing
assertions, timeouts and strict flaky policy remain unchanged.

Changed-file ESLint passes. The task-local E2E typecheck first failed because its
external configuration omitted installed Node types and existing Vite/Window
ambient declarations; that failed receipt is preserved. After correcting only
that task-local configuration, the check exposes seven diagnostics in unchanged
shared E2E dependencies: one missing store-exposure Window declaration in
`test-base.ts`, four duplicate implementations in `api-client.ts` and two in
`session-page.ts`. These files are outside the approved correction scope.
Exact source/base comparisons and original joined failure receipts are saved
in the own task plan. No errors are suppressed or treated as a passing check.

ROOT independently verified that all three diagnostic-owning files match the
actual PR base and frozen head byte-for-byte, and that normal web `tsconfig.json`
explicitly excludes E2E. This task-created expanded typecheck is not an existing
product gate. Both failed harness receipts remain FAILED; no clean typecheck,
suppression, compiler weakening or shared-fixture repair is claimed. No third
custom typecheck or unchanged production type/backend replay was run.

Formatting passes. Final scoped discovery finds exactly the Pierre desktop,
Monaco desktop and Pierre mobile cases with no discovery errors. The permanent
regression extends the existing desktop case with three private preview-history
commits and 14 diff sections; private HEAD is restored afterward. All existing
visible-anchor, count, reading-state, refresh and strict-flake assertions remain.
The shared helper now aligns the target and root rectangles inside the existing
30-second readiness poll, so lazy placeholder height changes are observed before
reading position is chosen. No product source, UI or renderer changes apply.

Local implementation is complete. Document/reference/coverage checks and normal
active-hook fixup publication finish the local handoff. Actual publication and
hook receipts are preserved in the own task plan. Hosted new-head CI and semantic
review require a later ROOT release after explicit local-heavy return and END.
No hosted retry, review request, observer or merge is authorized in this turn.

### Shared continuity cleanup correction

ROOT qualified a shared review finding: restoring private committed history
does not remove the three untracked files created by the desktop continuity
case. Apply the exact reviewed spec-only correction from the source owner:
delete only the prefix, target and unrelated paths in a nested `finally`,
including when history restoration fails. Assert restored HEAD and clean Git
status for those owned paths. Existing geometry, product assertions, timeouts
and strict flaky policy remain unchanged. The source owner's causal RED and
delivery are accepted without replay or duplicate authoring.

Validation in this work order is limited to one fresh managed Chromium/Pierre
desktop run, one worker, no retries and one repetition, followed by affected
spec lint/format, document/reference/coverage checks and normal active hooks.
No backend, expanded typecheck, Monaco or mobile passing replay applies.
The exact reviewed correction is implemented. One fresh managed Pierre desktop
run passes in 16.3 seconds with the restored-HEAD and three-owned-path Git
postconditions, one worker and no retries. Affected spec ESLint reports zero
issues, formatting passes, the catalog validates 365 decisions and 1480 specs,
all specification lint passes, and whitespace is clean. Source-owner causal
RED remains accepted without replay; no unrelated passing checks were repeated.
Normal active-hook publication evidence belongs in the own task plan.
New-head hosted delivery remains pending a later ROOT release after explicit
local-heavy return and END/WFI; prior-head CI/review evidence is historical.

### Preview-history setup failure correction

ROOT reviewed current grouped review 5470668211 and released a bounded
helper-only correction. A failure after an earlier preview commit or partial
file creation must reset the captured initial HEAD, remove only the fixed
preview file paths and rethrow the original setup error. Cleanup failures must
remain failed fixtures with the setup error retained. Preserve successful
history, restoration callback, preview contents and all product/geometry/test
assertions and timeouts. Exact owned-path staging is allowed if the real-Git
untracked sentinel control proves that broad staging captures unrelated files.

Validation is limited to independently authored real-Git failure/success and
cleanup-error controls, one fresh managed Chromium/Pierre desktop success case
with one worker, no retries and one repetition, affected helper/test lint and
formatting, document/reference/actual-scope checks and normal active hooks.
No shared GitHelper, runner configuration, product, mobile, backend or API
change applies. Five permanent real-Git cases first fail against the baseline
helper, then pass after the correction. They cover early partial creation,
failure after an earlier commit, successful restoration, reset failure and
owned-file deletion failure, including original error identity and unrelated
tracked/untracked sentinel controls. Broad staging captured the untracked
sentinel in the causal RED, so staging now names only the fixed owned paths.
Cleanup errors retain the original setup error and do not claim restored HEAD.
One fresh managed Pierre desktop success case passes in 15.2 seconds, with one
worker, no retries and one repetition. Scoped checks and active-hook publication
receipts are recorded in the own task plan. New-head hosted review and CI remain
pending a later ROOT release after explicit local-heavy return and END/WFI.

### Shared preview-path collision correction

The prior setup rollback is the baseline for a qualified shared review finding:
fixed preview paths could overwrite pre-existing files, and rollback deleted
unattempted paths. The exact source-owner helper/test correction is implemented.
It checks all twelve fixed leaf paths before the first write, records each
attempted path before creation, and deletes only attempted paths on setup failure.
It preserves the
original setup error and cleanup errors, successful history and restore callback,
preview contents, geometry, assertions, timeouts and strict flaky policy.

The source owner's four real-Git collision RED cases and five prior controls
are authoritative and accepted without duplicate authoring or replay. Local
validation is limited to the exact nine helper GREEN cases, one fresh managed
Chromium/Pierre desktop success case with one worker, no retries and one
repetition, affected helper/test lint and formatting, document/reference/actual
13-path scope checks, and normal active-hook corrective publication. No shared
GitHelper, product, backend, mobile or runner configuration change applies.
All nine actual-Git helper controls pass in 544 milliseconds. The fresh managed
Pierre desktop success control passes in 15.3 seconds with one worker, no retries
and one repetition. The initial lightweight discovery invocation failed on a
known variadic project-selector syntax mistake before any test ran; the corrected
same-case discovery finds exactly one Chromium case without errors. That failed
receipt remains failed and does not count as a product failure or causal RED.
Scoped-check and normal publication receipts are retained in the own task plan.
Hosted new-head evidence remains pending a later ROOT release after explicit
local-heavy return and actual END/WFI; prior-head CI/review evidence is historical.

## Released shared asset-directory symlink correction (local implementation complete)

ROOT validated grouped review5472731607 and released SOURCE97 after the original
hosted observer was identity-guarded, stopped and actually joined (143, no CI
terminal verdict). Reject symlinks at all three fixed asset directories before
any fixture write, preserving ordinary directories, all twelve leaf checks and
attempted-only rollback. Independently prove early/later external-directory
symlink rejection with native Git, zero create admissions and unchanged external
sentinels, link, HEAD, index and worktree; retain nine existing helper controls.
Run affected helper GREEN and one fresh strict Pierre desktop success control,
then scoped lint/format/catalog/spec/reference/coverage checks and normal hooks,
commit and push. No product or shared GitHelper changes, generalized filesystem
framework, TOCTOU guarantee, passing backend/mobile replay or hosted retry.
Public docs and mobile composition/copy/touch/breakpoints/navigation are unchanged.
Actual RED: original76909/ad40cf→027e66 exit1, two early/later native-Git
symlink cases admitted four/twelve writes and changed prepared index/worktree
state during rollback; zero-write and exact snapshot assertions failed. Nine
existing controls were intentionally skipped in this focused RED. GREEN:
original94008/b075b2→6e4b04 exit0, all eleven native-Git cases pass, including
nine prior controls and ordinary-directory success. The exact original Pierre
control61613/2786c6→601728 passed once (15.7s), strict worker1/retries0/repeat1,
fresh managed backend/Vite/plugin build, GOMAX2/-p2/GOMEM512MiB/Node4GiB,
GNU900s/kill10. Assertions, timeout and product behavior are unchanged.
Each original native/subprocess/monitor was actually joined before the next
heavy invocation, with known owned absence. Receipts and raw hashes reside in
`/tmp/kandev-child97-ignore-review-20261009/symlink-fixup`. Green unit fixture
root/environment console metadata was not captured and remains unknown; no
replay is claimed. Formatting and scoped ESLint/Prettier/catalog (365 decisions, 1480
specifications)/all-spec lint/whitespace passed, original59326/54869c→bc40b5
exit0; formatting original7dd616 actually joined0. Actual owning references and all thirteen changed paths are covered with
errors=[] and this ONE work order (preflight33962/252012→885102 exit0;
coverage originalc86496 joined0). Normal publication checks follow this pass. Hosted CI and full review at the new
corrective head remain for a separate later ROOT release. No merge authority.
