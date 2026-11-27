---
id: "01-correct-save-target"
title: "Correct repository-scoped editor save targets"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-SAVED-FILE-CONTENT-001
acceptance_criteria:
  - AC-WORKSPACES-SAVED-FILE-CONTENT-001.1
  - AC-WORKSPACES-SAVED-FILE-CONTENT-001.2
  - AC-WORKSPACES-SAVED-FILE-CONTENT-001.3
  - AC-WORKSPACES-SAVED-FILE-CONTENT-001.4
system_design:
  - ../../specs/workspaces/system-design/saved-file-content.md
---

# Task 01: Correct the Repository Save Target

## Summary and release gate

Translate the submitted file header into the already admitted workspace target
before applying the patch. Prove selected-file bytes/hash, untouched same-named
neighbors, truthful registered HTTP acknowledgements and existing failure paths.
Start only after a later explicit ROOT reviewed-package implementation interrupt
to primary session `d5da2835-1a60-4d47-9fda-f4af185170a5` and an exclusive
GLOBAL LOCAL-HEAVY ROOT lease for product checks. No delegation or new sessions.

## In scope

- Internal `ApplyFileDiff` path-coordinate argument and file-header translation,
  immediate registered-handler wiring, mechanical existing tracker test calls.
- Independent real-Git process and registered HTTP regression/compatibility
  tests, using existing fixture and admission helpers with portable cleanup.
- This package's status/results and owning pair conformance records.

## Out of scope

General patch authorization/security frameworks, arbitrary multi-file semantics,
same-file ordering, StageAll artifact handling, changing private patch ownership,
repository tracker lifecycle, transport/schema/UI/layout, persistence and
unrelated fixes. No protected-proof reuse or moving-main rebase.

## Acceptance

1. Eligible root, alpha and beta saves modify only their selected physical file,
   including genuine editor headers and a nil-desired-content control; returned
   independent SHA256 and applied result agree with actual disk bytes.
2. Registered routing/ACK and existing root-tracker write notification identify
   the requested target. Scoped symlinks remain links; fallback, cancellation,
   conflicts and shared patch isolation retain their existing outcomes.
3. Exact checks below pass with honest platform/skip/receipt evidence; no
   fallback-only success is recorded as patch-target proof.

## TDD sequence

1. Mark `in_progress` after release. Read the pair, plan, applicable scoped
   guidance, `/tdd` and its backend-test reference. Independently author new
   `workspace_file_save_target_test.go` in process and API packages. Do not read
   proof source to derive tests, or replay/copy/import it.
2. Use fresh `t.TempDir` fixtures: a plain task root with root/alpha/beta files,
   then a plain task root containing independently initialized alpha and beta
   repositories. Use existing `runGit`/`runGitAPI`, `writeFile`/`writeFileAPI` and
   logger/Manager/router patterns; no remote is needed. Put identical edit and
   three context lines at the same relative path, with distinct distant
   sentinels outside the hunk and an unrelated neighbor. Check root, alpha and
   beta on independent fixtures; add a nested relative path.
3. Author `TestApplyFileDiff_RepositoryTarget` and
   `TestHandleFileUpdate_RepositoryTarget` against existing signatures first.
   The process test supplies the admitted scoped path with a submitted relative
   patch; API supplies real `Repo` and relative `Path` through `Router`.
   Include ordinary unprefixed no-tab headers and a literal fixture faithful to
   locked diff8.0.3 `formatPatch`: Index/separator preamble, unprefixed filenames
   with empty tab suffixes, three context lines and a final newline. Verify that
   fixture against the locally inspected formatter source and
   `generateUnifiedDiff` arguments; no frontend build or install is needed for
   this oracle. Include desired-content and nil-desired-content variants.
4. Assert disk bytes of every candidate, independent `crypto/sha256` expected
   hashes, resolution, errors and HTTP200/path/success/new_hash/resolution. Use
   the real root tracker subscription (attach before action, detach in cleanup)
   to assert the immediate write event's workspace-relative selected path,
   `FileOpWrite`, and unchanged root repository-name identity. No polling tracker
   is needed. Run both RED selectors; retain the actual expected causal failure
   and passing root control. Initialization alone does not prove targeted bytes.
5. Add required internal `diffPath` string beside `reqPath` in `ApplyFileDiff`.
   API passes admitted `scopedPath` and submitted `req.Path`; existing direct
   tracker test calls pass the same path for both. After existing admission/hash
   checks, rewrite matching submitted file headers to slash-separated `reqPath`
   before existing symlink translation. Match filenames separately from their
   tab suffixes, retain suffixes and hunk text. Cover the existing a/b prefixed
   symlink forms without treating a real directory named `a` or `b` as a prefix
   before an exact filename match. Do not rewrite arbitrary mismatched paths.
   Keep root cwd/Git argv and `writeFileDiffPatch` unchanged. Run RED selectors
   again for GREEN.
6. Add `TestApplyFileDiff_RepositoryTargetCompatibility` and
   `TestHandleFileUpdate_RepositoryTargetCompatibility`. Include nested paths,
   a selected path under a real `a/` directory, a save whose hunk body resembles
   `---`/`+++` headers, and a scoped symlink to a workspace-admitted real target.
   Check link identity and exact target/neighbor bytes. Skip only unsupported
   native symlink setup with an explicit reason. Include stale hash and invalid
   patch outcomes with nil, empty and nonempty desired content. Invalid
   repository/path requests must fail without writes. Assert resolution/hash
   for overwrites, failure with empty success fields for nil fallback.
7. For scoped queued cancellation, use existing one-slot `GitInteractive`
   admission and portable `AdmissionSnapshot().Waiters` observation. Register
   cancel/release/drain before launches; join requests/subprocesses before cap
   restoration. Include a distinct successful peer, unchanged cancelled target,
   context error/HTTP failure, untouched neighbors and no newly owned patch
   artifacts after settlement. Never add `t.Parallel`, sleeps as ordering proof,
   mocks for Git or production test hooks.
8. Run final anchored tests, exact-base scoped lint and document/reference
   preflight. Record actual results and missing metadata. Mark work order done
   and plan implemented only after conformance; active/current pair statuses
   already describe the accepted contract. No second broad passing replay.

## Verification

Commands run individually from repo root, serially through a receipt-producing
wrapper retaining native original handles/chunks, UTC argv/cwd/log/PID/PGID and
cutoffs, actual joins and group disappearance. See the platform plan for failure
checkpoint rules and later publication gates. No commands below authorize heavy
execution during DESIGN.

```bash
# Each RED before production edits, each GREEN after the correction:
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=1 ./internal/agentctl/server/process -run '^TestApplyFileDiff_RepositoryTarget$' -count=1 -timeout=90s -v)
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=1 ./internal/agentctl/server/api -run '^TestHandleFileUpdate_RepositoryTarget$' -count=1 -timeout=90s -v)

# Final process outcomes, every mechanically changed caller and compatibility:
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=1 ./internal/agentctl/server/process -run '^(TestApplyFileDiff_(RepositoryTarget|RepositoryTargetCompatibility|ConcurrentDistinctFiles|SequentialDistinctFiles|PatchCleanup|CancelledQueuedSave|RegularFile|Symlink|ConflictDetection|ConflictWithDesiredContent|ConflictWithoutDesiredContent|SymlinkConflictWithDesiredContent)|TestRewriteDiffPaths|TestWorkspaceFileOperationsAllowRegisteredLinkedSource|TestWorkspaceFileMutationsRejectDescendantSymlinkSwap|TestRescanRepositories_SourceRootsApplyToCurrentRootAppends)$' -count=1 -timeout=90s -v)
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=1 ./internal/agentctl/server/api -run '^TestHandleFileUpdate_(RepositoryTarget|RepositoryTargetCompatibility|ConcurrentDistinctFiles|SequentialDistinctFiles|AppliesDiff|ReportsHashConflict|Rejections)$' -count=1 -timeout=90s -v)

(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m golangci-lint run ./internal/agentctl/server/process ./internal/agentctl/server/api --new-from-rev=1b70c91a412361458a640b9fe25a8093ebc043f6 --concurrency=2 --allow-serial-runners --timeout=5m)

python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/workspaces docs/plans/repository-save-target apps/backend/internal/agentctl/server
git status --short -- docs/specs/workspaces docs/plans/repository-save-target apps/backend/internal/agentctl/server
```

Run `.github/scripts/pr-docs.cjs` exported `validateCoverage` with all four
document contents and prospective production paths during design, and actual
changed paths after implementation. Use existing Node24.21.0 explicitly through
Bash loginfalse; no install for this lightweight preflight. Save errors/status
and distinguish prospective coverage from product evidence. A backend PR fixup
requires the single actual-PR-base full changed-code lint specified in the
platform plan; it is not this scoped command or a design gate.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_files.go`
- `apps/backend/internal/agentctl/server/api/workspace.go`
- `apps/backend/internal/agentctl/server/process/workspace_file_save_target_test.go` (new)
- `apps/backend/internal/agentctl/server/api/workspace_file_save_target_test.go` (new)
- Existing process callers in `workspace_tracker_test.go`,
  `workspace_file_save_test.go`, `workspace_files_test.go` and
  `manager_rescan_test.go` (argument wiring only).
- The four owning requirement/design/plan/work-order files, for delivery links,
  conformance and results. Preserve the previous completed package's history.

## Dependencies and parallelism

No internal dependency. PR4280 is actually merged at admission. `sequential`.
ROOT's later implementation interrupt and heavy lease are external prerequisites.

## Inputs and risks

[Requirements](../../specs/workspaces/requirements/saved-file-content.md),
[design](../../specs/workspaces/system-design/saved-file-content.md), existing
save tests, `workspace_handlers_test.go` router fixtures, `manager.go` admission
and `workspace_monitor.go` notification contract. Review transport and Canvas
consumers remain source-only compatibility context, with no changes scheduled.

Tab suffixes and header-like hunk bytes can defeat naive replacement. Scoped
symlink translation must occur after coordinate normalization. Native Windows
path separators/fixture helpers need actual hosted coverage; Linux results do
not establish it. No wider boundary change is authorized: checkpoint ROOT first
if the independent regression shows one is required.

## Results

ROOT released a bounded corrective fixture update after native Windows process
CI failed at published `f3003d34409607f9444e41d81931f4914dd171e2`.
All 24 matrix cases (including root controls) and five compatibility cases
received CRLF after Git apply; exact LF byte/hash assertions failed. Event
envelopes hid the received fields, so their cause was not established.
The fixtures now provide an owned Git config with `core.autocrlf=false` to
initialization and the real tracker/Manager Git environment, canonicalize their
temporary root as existing save tests do, and expose received event fields.
Strict path, byte, hash, neighbor and native symlink assertions remain.
Production behavior is unchanged. Corrective anchored race-enabled process and
registered API regressions each passed all 24 target cases and compatibility
controls without Linux skips. Scoped lint passed with zero issues against live
PR base `aad793bfa93e637e74d5d74d4cf194bc3d02f23b`. The one full changed-code
lint reached its six-minute outer timeout, exit124, without diagnostic output;
native46243/334336→56d8ff was actually joined at 2026-10-07T05:27:06.499502Z,
and its command group and wrapper were observed gone. No passing lint verdict,
commit or push followed. ROOT subsequently authorized exactly one identical
recovery using retained caches, the same captured base and resource/time bounds.
That recovery, native13207/bc4270→6447ab, passed with zero issues and joined
exit0 at 2026-10-07T05:34:49.992631Z (273.541s); its group and wrapper were
observed gone. The original timeout remains failed with unknown cause.
No third attempt ran. Normal-hook fixup publication is authorized; corrected
native Windows and hosted gates remain required.
Current-head native Windows gates remain pending; the old hosted failure
remains a failure.

Executed sequentially on Linux after ROOT reviewed the SHA256-sealed package and
released implementation plus the exclusive local-heavy lease. Commands use the
exact Verification selectors and resource bounds. Permanent tests were authored
independently; the immutable ROOT proof was not read to derive or replay them.

| Check | Actual result |
| --- | --- |
| Process RED | Exit1; 16 alpha/beta cases falsely acknowledged unchanged selected content/hash while root bytes changed; eight fresh root controls passed |
| Registered API RED | Exit1; same 16 scoped failures and eight passing root controls, including genuine empty-tab headers and nil fallback |
| Additional compatibility RED | Exit1 in both packages for nested, literal-a, scoped symlink and header-like hunk outcomes |
| Process GREEN | Exit0; all 24 matrix cases passed |
| API GREEN | Exit0; all 24 matrix cases passed |
| Final process race selector | Exit0; all 16 top-level tests passed, no skips |
| Final API race selector | Exit0; all seven top-level tests passed, no skips |
| Scoped lint | Exit0; zero issues against admitted base |
| Document/reference checks | Exit0; catalog357/1416, all36 spec-linter tests, full spec lint, actual changed-path coverage covered/errors[], whitespace |

Original handles and per-command UTC/argv/cwd/log/PID/PGID/cutoffs are retained
in `/tmp/kandev-child64-implementation-receipts.jsonl` and the platform task
plan. Each completed test original is actually joined and its command group
observed gone. Linux results do not establish native Windows execution, browser
or WebSocket-network behavior. No public-doc change is needed: existing editing
and saving instructions stay accurate; only internal docs changed. Normal hook
publication completed at the initial head; corrected-head publication and
hosted current-head gates remain pending.
