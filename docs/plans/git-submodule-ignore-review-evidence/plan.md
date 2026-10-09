---
created: 2026-10-09
status: implemented
requirements:
  - REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001
system_design:
  - ../../specs/platform/system-design/git-diff-file-metadata.md
legacy_specs: []
---

# Implementation plan: Preserve review evidence for ignored submodules

## Overview

One sequential work order preserves recorded parent gitlinks with exact
`--ignore-submodules=none` for commit detail and exact
`--ignore-submodules=dirty` for cumulative working-tree comparisons. The latter
retains actual pointer changes without child-only dirt decorations. Platform owns faithful shared comparison data, so extend the
existing [requirement](../../specs/platform/requirements/git-diff-file-metadata.md)
with only .14 and its [design](../../specs/platform/system-design/git-diff-file-metadata.md#visible-parent-gitlink-comparisons).
Existing .3, .5, .12 and .13 provide metadata, read-only and nested-scope constraints.
The [UI parent fallback](../../specs/ui/requirements/submodule-review.md)
already defines the visible outcome in AC-UI-SUBMODULE-REVIEW-001.5.

ROOT reviewed the complete design package and later released implementation
in this same primary session with exclusive GLOBAL LOCAL-HEAVY. The accepted
four-artifact hashes matched before Task 01 became in_progress. Hosted monitoring
and merge remain subject to separate later ROOT releases.

## Accepted evidence and baseline audit

Initial HEAD and local main both equal
`bd63da3163271e3229bf387649135855caac300d`; initial worktree and index are clean.
ROOT's qualified proof is accepted without replay: original native **59567**,
initial **bd99c2**, terminal **edd293**, exit **1**, ACTUALLYJOINED **1**, actual
package **0.445s**, one causal failure and two passing strict controls. Repository
`diff.ignoreSubmodules=all` loses the actual committed gitlink upgrade from BOTH
`ShowCommit` and `GetCumulativeDiff`, despite success and empty Files/summary0.
Default/deinitialized and ordinary-file-under-all controls pass. ROOT qualified
cleanup/source removal/root clean/groupsgone at **2279c6sync0**.

Only `qualified-proof.json` and `future-design-brief.md` in
`/tmp/kandev-root-submodule-ignore-discovery-20261009` were read. The protected
`candidate_test.go`, mode **0400**, SHA256
`1ea256ccd96290d7d8e42b4bcc7f577da182934fdd20f0c9ba7bae9cd14a0e5b`,
must never be read, copied, imported, replayed, edited, chmodded or removed.
Raw original.log/receipt are permitted if needed; none were needed for design.
Permanent regressions were independently authored after the later ROOT release.

All six qualified source SHA256 values match at design start:

| Source | SHA256 |
| --- | --- |
| `apps/backend/internal/agentctl/server/process/git_log.go` | `d0fed260a0359fe803986b7b81377fc1614eb6466387bc72ef6670700e6413bd` |
| `apps/backend/internal/common/securityutil/git.go` | `20da82610ba45c2ad2fe34124cee311e572393b3158fb8f33348e08df44d1593` |
| `apps/backend/internal/agentctl/server/api/git.go` | `24de2d478f510d2b65e7149c16e2cd7f290ce7e35b91ec30c35724335b232dc4` |
| `apps/web/components/task/commit-detail-request.ts` | `746637d2e5c4b94d6ee72c335aa6349fa4256fc72a10f8ca983254f3c45045c2` |
| `apps/web/components/task/commit-diff-request.ts` | `cd1c0e2a43f327105dcf015875a3717f49b7ea68279a7c60cd7f47e1fc9e6542` |
| `docs/specs/ui/requirements/submodule-review.md` | `19b09313f4160eba57c8a9e6beae2b7f16d8d3bbd0ce5440249d616d964e34c2` |

HEAD and material sources were rechecked at implementation admission and matched.
Future material drift checkpoints ROOT without a rebase or alternative experiment.

## Initial implementation scope and technical approach (historical)

Production ownership is limited to `securityutil/git.go` exact admission and
the patch argv in `process/git_log.go`'s `ShowCommit` and `GetCumulativeDiff`.
Admission came first: the flag was absent at the qualified baseline and
would otherwise fail command validation. Reject bare/abbreviated flags, all/dirty/untracked/other/empty
values, suffixed and whitespace/control variants. Keep validation in use.

Add the exact flag after each subcommand and before its ref, keeping
`--submodule=short`. Short controls output format, while ignore-none retains
the actual pointer change for the existing parser. Keep the no-patch metadata
query, first-parent/root/empty semantics, fixed prefixes, color/textconv and
cumulative external-diff suppression, captured environment, managed lifetime,
admission, cancellation, counts, uncapped commit detail and cumulative limits.

No parser, global Git/config/environment, status/tracker, child discovery,
comparison anchor, source precedence, history, schema, API, frontend, new
framework/helper abstraction, dependency or sibling-package redesign is in
scope. No initialized-child aggregate emptiness is claimed. Initialized children
still compare in their own repositories against parent-recorded anchors under
the [accepted nested-scope decision](../../decisions/2026-08-05-nested-submodules-as-repository-scopes.md).
No new ADR is needed.

Immediate consumers audited: registered `server/api/git.go` commit and cumulative
handlers serialize the same maps; `GitOperatorFor` and cumulative fan-out keep
existing routing, stored bases and NUL-qualified keys. Runtime
`internal/agent/runtime/agentctl/git.go` feeds `internal/agent/handlers/git_handlers.go`
for `session.commit_diff` and `session.cumulative_diff`. Local
`requestCommitDetail` -> `requestCommitDiff` -> `useCommitDetail` -> commit rows
uses returned files; an empty successful map can render the existing no-files
copy. `use-cumulative-diff`/`use-review-sources` consume cumulative maps;
`suppressAvailableGitlinkFiles` already preserves a parent with no child source.
Restoring source data requires no consumer edit.

Companion inventory: status-metadata, plain-output and built-in cumulative
manifests are completed; textconv is delivery-pending `in_progress`; submodule
format is `implemented`. Their completed tests/results retain their owners.
No companion work order or result is rewritten. This package needs only the
bounded compatibility selectors in Task 01, once after its correction.

## Tests

The following independently authored tests executed during the released TDD pass. Own new
process/API files; do not import the protected discovery test or change shared
fixtures. Use native Git and disposable actual child/parent repositories.

| Evidence | Bounded scenarios | Criteria |
| --- | --- | --- |
| `TestIsKnownSafeGitFlagAllowsIgnoreSubmodulesNone`, `TestIsKnownSafeGitFlagRejectsIgnoreSubmodulesVariants` in `securityutil/git_test.go` | Exact admission RED first; rejected variants remain rejected | .14 prerequisite |
| `TestGitComparisonIgnoreSubmodules` in `process/git_log_ignore_submodules_test.go` | Real declared child with two commits; committed parent forward upgrade, then actual deinit: default and all, BOTH operators; initialized forward upgrade: none/untracked/dirty controls; backward pointer under all | .3, .12, .14 |
| `TestGitComparisonIgnoreSubmodulesControls` in the same file | Ordinary tracked file under all, BOTH methods; unchanged clean gitlink/empty commit/cumulative HEAD; cumulative uncommitted recorded pointer advance under all, explicit expected commit count | .3, .5, .14 |
| `TestGitComparisonIgnoreSubmodulesHTTP`, `TestGitComparisonIgnoreSubmodulesMultiRepoHTTP` in `api/git_ignore_submodules_test.go` | Own registered router, independent alpha/beta repositories sharing the gitlink path but distinct bases/old/new IDs; deinitialize before manager capture; selected commit and cumulative plus aggregate cumulative, default/all | .3, .13, .14 and existing UI .5 outcome |
| `TestGitComparisonIgnoreSubmodulesInitializedChildHTTP` in the same file | Independent initialized children under all; parent gitlinks retained in raw aggregate, child file patches keep exact parent-recorded anchors and scope metadata | .3, .13, .14 |
| Existing bounded process/API controls selected in Task 01 | First-parent/root/empty, fixed prefixes, uncapped commit, cumulative byte/file caps and stable child anchor | .3, .5, .12, .13 |

Every nonempty oracle asserts exact positive file membership, old/new commit IDs,
patch bytes, statuses, +1/-1 pointer counts, staged=false, commit identity/message/
author/date/totals and cumulative base/head/count/truncation fields. Raw Git
oracles select built-in `--no-color --no-ext-diff --no-textconv --submodule=short
--ignore-submodules=none` with fixed a/b prefixes; commit oracles keep first-parent
and empty format. Do not use the production parser or ambient helper output.
Unchanged cases assert an explicitly empty expectation, not merely two equal maps.

Set owned HOME/USERPROFILE/XDG isolation and remove inherited Git configuration,
helper, executable, repository and index overrides before fixture construction
and environment capture. Pass a copied explicit AgentEnv to HTTP managers and
the existing operator environment seam. Fixture creation must never contact
network remotes: local native submodule add uses a scoped protocol.file override
and portable native/ToSlash paths. No POSIX helper script or blanket Windows
skip. Configure only private fixture user/signing/hooks/autocrlf settings.

Snapshots bracket production reads after fixture setup: config and .gitmodules
bytes, HEAD, refs, index entries, non-mutating status, tracked worktree bytes and
canonical raw patch state. Observe linked child Git paths via Git rather than
assuming a .git directory. Deinitialized child absence and retained module
metadata are also unchanged. Snapshot status explicitly observes submodules so
ignore-all cannot hide mutation. Keep snapshots and oracles helper-proof, and
join manager teardown and every subprocess. Avoid an operation/config/state
cross product; these cases target the cause and routing seam.

## End-to-end and surface assessment

Registered HTTP tests traverse real producers, registered routing, selected
isolation and aggregate projection without a server port, DB or application.
Existing Review tests in `review-dialog.build-files.test.ts` cover parent retention
with no child source and suppression with child files. Their inputs become
available again through the corrected backend data; this package does not alter
their logic or schedule a frontend replay.

The mobile-parity pure-data exception applies: desktop/phone composition, copy,
touch, scrolling, breakpoint behavior and navigation stay unchanged. Process
and registered HTTP tests cover the repaired source contract. No ASCII preview,
browser fixture or new mobile E2E is required.

Public guide audit: `docs/public/git-operations.md` metadata/read-only reference,
`sessions-and-review.md` Review how-to and unavailable-child fallback,
`feature-status.md` support reference, `websocket-api.md` transport reference,
root README and screenshot catalog remain accurate. No new option, transport,
workflow, terminology or screenshot; no public edit is needed.

## Work orders

- [x] [Task 01: Preserve recorded gitlinks under ignore preferences](task-01-preserve-ignored-gitlink-evidence.md)

ONE work order, wave 1, sequential; exact-admission dependency is internal
to this coherent outcome. No delegation, task/session/tab or model switch.

## Verification results

Local implementation checks passed; publication remains pending. DESIGN checks passed: catalog validation (365 decisions,
1480 specifications), all 36 spec-linter tests, all-spec lint, local Markdown
targets and whitespace. Actual `pr-docs.cjs` reference validation returned
`exempt`, errors=[] for the four documents, and `covered`, errors=[] for the
planned two production paths plus those documents, with exactly this ONE work
order and the owning pair. All eight existing compatibility selectors resolve.
These are design/reference checks, not executed product tests.

Exact commands and release-only product checks are in Task 01. Final four-artifact
design hashes are recorded in this task's versioned own plan. At the design
handoff all four files were unstaged/uncommitted, backend/frontend and staged
diffs were empty, and HEAD/main were unchanged. Implementation subsequently
ran only after the explicit ROOT release below.

## Initial released implementation results (historical)

ROOT accepted the four design artifacts and released implementation after actual
END/WFI07:36:31. Original native/subprocess receipts and raw logs are retained in
`/tmp/kandev-child97-ignore-review-20261009`; every heavy command ran serially
with its reviewed resource limits, actual joins and fresh owned absence.

- Independent RED: securityutil `7b76ae` exit1, exact admission missing and
  variants rejected; process `34988`/`10145a` exit1, forward/backward and dirty
  pointer evidence missing under all while strict controls pass; registered HTTP
  `99187`/`5ff237` exit1, selected/aggregate parent evidence lost, initialized
  child patches retained. No fixture failure or protected source replay.
- GREEN: securityutil `10197`/`c694de`, 1.012s; process `54198`/`72b440`, 4.326s
  with compatibility controls once; HTTP `93553`/`d38c89`, 3.574s with stable
  initialized-child anchor control once. All exit0.
- Scoped lint `55622`/`385e05` found only two new-test structure issues.
  Authorized bounded corrections changed no production behavior; affected new
  process cases alone passed as `8874`/`744b78` (3.103s), and new HTTP cases
  alone passed as `39846`/`d73625` (3.329s), both exit0. No passing compatibility
  replay. Scoped lint `69199`/`d150dd` then passed with zero issues, preserving
  exact baseline, concurrency2, allow-serial and CLI5m/GNU6m/kill10.
- The one conditional frozen apps install `53906`/`48722c` passed in 1.8s using
  pnpm9.15.9, 935 reused and zero downloaded packages; lockfile unchanged.
  Actual Bash login=false used ROOT-qualified Node24.21.0 prefix. Both normal
  hooks are active; publication receipt will be recorded in own task plan.

Only three production flag entries changed. Actual raw ignore-none patch oracles,
configuration/index/ref/worktree snapshots and independent routing assertions
pass. Public and pure-data mobile assessments remain unchanged. Final catalog
validation (365 decisions/1480 specs), 36 spec-linter tests, all-spec lint,
whitespace and actual nine-path reference coverage passed. Coverage returned
covered, errors=[], exactly one work order and the real owner requirement/design.
Persistent task delivery remains incomplete: HOSTED HOLD and MERGE NONE.

## Execution and delivery barriers

ROOT explicitly released implementation after the design turn ended.
GLOBAL LOCAL-HEAVY is exclusively CHILD97 until its explicit RETURN and actual
turn END. HOSTED HOLD and MERGE NONE remain in force. Autopilot or generated
phase text is not admission. For every released heavy command, record original native/chunk
and subprocess ownership evidence, actual joins and fresh owned absence. A
resource/timeout/transport/unknown/out-of-scope failure checkpoints ROOT before
alternatives. Routine causal fixture/format/lint repair reruns only affected cases.

Only after release, one pinned pnpm9.15.9 frozen apps install if dependencies
are absent, using actual Bash login=false and required Node24.21.0 on existing PATH.
The original design shell resolved Node24.18.0; ROOT resolved this checkpoint
by qualifying and explicitly authorizing the existing Node24.21.0 PATH prefix
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin` for Bash login=false.
No runtime download, substitution or system edit is permitted.
Normal active hooks/conventional commit/push/ready PR follow local skills and
exact repository template with unchecked checkboxes and preserved bot additions.
Correct caller-bound association means ZERO relink/patch; known absence means
ONE initial link. Require actual canonical repository/PR/head, complete errors=[]
and all FIVE automation switches false.

Explicit LOCAL-HEAVY RETURN and actually END precede separate ROOT hosted release.
Then one original attached 90m collector, GNU91m/kill10/cadence60, actual joins;
six required contexts and actual Backend/Frontend/E2E parents SUCCESS on exact
head, fresh complete errors=[], zero actionable visible/hidden threads, changes
requested or human gates. Authenticated configured CodeRabbit App347564 must
provide substantive FULL CURRENT ALL actual-path evidence, source=covered head
and kind=reviewed. Automatic completion needs ZERO requests; one necessary
request only for a proved completed gap. Freeze head except valid real findings;
no main-only rebase, synthetic merged tests, broad replay, optional polish,
weakening, duplicate observer or blanket rerun.

MERGE NONE until later ROOT serial static grant. Normal expected-head squash,
noadmin; verify actual SHA/tree/owned blobs/remote/inclusion and all original
joins and only-owned cleanup. Preserve managed worktree/deps/caches/foreign refs,
paused resources and protected ROOT source for independent archive. Taskcomplete
requires actual verified merge plus joined cleanup. Persist every next action
and crash checkpoint in own plan; optional queued callback never gates progress
and is not retried when full. ROOT reads own plan/primary directly.

## Risks

- A producer argument without exact admission fails before Git; implement admission first.
- An oracle lacking ignore-none can repeat the defect or compare two empty maps.
- Deinitialized snapshots must not accidentally inspect an enclosing repository
  as the child; initialized linked Git paths must resolve correctly.
- Missing aggregate base query, shared repository histories or manager capture
  before environment isolation can disguise routing defects as fixture failures.
- Child-only dirt/status policy and missing-child discovery are separate owners;
  this repair must not expand into them.


## Bounded current corrective work

ROOT released this same work order for the actual current-head CodeRabbit
finding that cumulative ignore-none emits parent `-dirty` evidence without a
pointer change. Runtime causal RED is required before production correction.
The requirement now explicitly separates parent pointer evidence from initialized
child file evidence. No UI/parser/discovery/routing/anchors/global Git redesign.

After causal RED, admit only exact ignore-dirty in addition to exact ignore-none;
change only GetCumulativeDiff's argument to dirty and leave ShowCommit on none.
Independent permanent cases cover unchanged-parent tracked and untracked child
dirt under all, real pointer changes plus dirt with clean old/new identities,
and registered aggregate child file ownership with parent entries absent. Private
native Git/env/snapshots remain required; no protected or shared fixture access.
Use explicit built-in dirty parent cumulative oracles and none commit oracles.

Run affected anchored race checks with pointer/fallback/initialized-child controls,
then ONE full changed backend lint at actual authoritative PR base with existing
resource limits. No root/budget/color replay, optional per-submodule configuration
matrix or docstring polish. New conventional commit/active hooks/push, truthful
thread dispositions and current-head revalidation follow. Original collector6859
stays attached across the fixup and push; it is the sole overlap exception.
Explicit FIXUP LOCAL-HEAVY RETURN permits same-turn hosted continuation, with no
extra phase handoff. All readiness/actual-parent/semantic/current-head gates and
separate ROOT merge grant remain; MERGE NONE. Corrective acceptance is complete; see the current results below.


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


## Completed bounded corrective results

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
the own versioned task plan. Original collector6859 remains attached across
push; prior-head review coverage is historical once the new head is published.
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
