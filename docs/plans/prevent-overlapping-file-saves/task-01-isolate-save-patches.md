---
id: "01-isolate-save-patches"
title: "Isolate overlapping file-save patches"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-SAVED-FILE-CONTENT-001
  - REQ-EXECUTORS-SURVIVAL-001
  - REQ-EXECUTORS-SURVIVAL-004
acceptance_criteria:
  - AC-WORKSPACES-SAVED-FILE-CONTENT-001.1
  - AC-WORKSPACES-SAVED-FILE-CONTENT-001.2
  - AC-WORKSPACES-SAVED-FILE-CONTENT-001.3
  - AC-WORKSPACES-SAVED-FILE-CONTENT-001.4
system_design:
  - ../../specs/workspaces/system-design/saved-file-content.md
  - ../../specs/executors/system-design/agent-survival-across-restart-03.md
---

# Task 01: Isolate Overlapping File-save Patches

## Summary and release gate

Independently reproduce the accepted distinct-file lost edit, then replace the
shared patch filename with a request-owned temporary file and cleanup. Cover
actual disk contents, truthful results, cancellation and compatibility through
the producer and registered HTTP endpoint.

ROOT's later reviewed-package implementation and exclusive local-heavy releases
are received in this same primary. No agents, extra tasks/sessions, model
switches, or automatic phase
continuation. Record every invocation's UTC, exact argv/cwd/log, PID/PGID,
native handle, cutoff and actually-joined/group-gone outcome in the platform
task plan. Resource, timeout, setup, unknown, or out-of-scope failures checkpoint
ROOT without automatic retry, cache wiping, foreign kills, or waivers.

## Selectors and resource bounds

Use exactly the anchored selectors in Verification. Process RED selects only
`TestApplyFileDiff_(ConcurrentDistinctFiles|SequentialDistinctFiles)`; final
process coverage adds `PatchCleanup`, `CancelledQueuedSave`, and the six existing
regular/symlink/conflict tests. API coverage selects concurrent/sequential saves
and the three existing file-update tests. One command at a time:
`GOMAXPROCS=2`, `GOMEMLIMIT=512MiB`, `-trimpath -tags fts5 -race -p=1
-parallel=1 -count=1`, Go90s/GNU5m/kill10s. Scoped lint concurrency2, serial,
CLI5m/GNU6m/kill10s. No broad suite, browser, E2E, or build.

## In scope

- `ApplyFileDiff` temporary patch creation, descriptor write/close, own-file
  cleanup, and absolute patch filename, using existing Go/local patterns.
- Focused real-Git outcome regressions with deliberate existing admission gates.
- Registered HTTP file-update disk/result proof using the existing API fixture.
- Keep requirement/design/plan statuses and actual results synchronized.

## Out of scope

- Same-target ordering/merging, header-target authorization, global locks,
  production test hooks, goroutines, shared frameworks, adjacent refactors.
- API/WS schemas, handlers, frontend/store changes, database persistence,
  runtime flags, harness/dependency hacks, public UI/copy changes.
- The independent ROOT-owned repository-scoped header-target candidate.

## Acceptance

1. Real-Git RED fails specifically because an overlapping distinct-file save is
   acknowledged without its desired bytes/hash; both fresh sequential controls
   pass. GREEN preserves both edits and each independent expected hash for one
   shared tracker and two independent trackers in the same workspace.
2. Applied, overwritten, error and queued-cancellation outcomes clean up only
   their request's patch and preserve neighboring bytes; cancellation with real
   desired content performs no fallback. Existing symlink/conflict paths pass.
3. The registered file-update route returns truthful status/path/success/hash/
   resolution for both overlapping and sequential saves, matching final disk;
   all exact checks pass and recorded limits/claims remain accurate.

## TDD sequence

1. Mark this work order `in_progress`. Read the owning pair, scoped backend/
   agentctl/API guidance, `/tdd` and its backend-test reference. Author permanent
   regressions independently; never copy/import/replay the ROOT archive.
2. In `workspace_file_save_test.go`, reuse `setupTestRepo`, `runGit`, `writeFile`
   and `newTestLogger`. Seed tracked alpha/beta files with distinct original and
   desired bytes, plus an untouched neighbor. Use correct original SHA256 and
   matching patches with actual non-nil desired content. Test `shared_tracker`
   and `independent_trackers` (two `NewWorkspaceTracker` instances on one root).
3. After fixture setup, hold the existing one-slot `GitInteractive` admission
   gate. Queue alpha and observe its waiter, queue beta and observe both, then
   release. Reuse `waitForAnyGitWaiter` or a bounded snapshot wait in test code.
   Never use elapsed sleeps as ordering proof or alter product code for a gate.
   Register cancel/release/drain cleanup before launching; join all request
   goroutines before restoring admission or removing fixtures. No `t.Parallel`.
4. Assert after both settle: each exact expected disk content, independent
   `crypto/sha256`/hex hash, and applied resolution. Assert neighbor bytes remain.
   Fresh sequential controls must pass on unfixed source; use their own fixtures
   so the failing overlap cannot contaminate them. Run the RED command below
   before any production change and retain the causal assertion output.
5. In `ApplyFileDiff` only, use `os.CreateTemp` in an absolute workspace directory,
   private permissions, immediate owned cleanup, checked write/close, and a
   closed patch descriptor before existing Git execution. Preserve the current
   fallback/cancellation, hash, symlink, Git and notification branches. Run RED's
   selector again for GREEN.
6. Add `TestApplyFileDiff_PatchCleanup` for applied, invalid-patch overwritten
   fallback (nonempty and explicitly empty desired content), and invalid-patch
   failure without desired content.
   Supplement disk/hash/error assertions with no new patch remnants after
   settlement; preserve an unrelated sentinel and a pre-existing legacy fixed
   patch file. Existing concurrent evidence proves one request's cleanup cannot
   delete the other's pending patch. Add `TestApplyFileDiff_CancelledQueuedSave`
   with desired content and a distinct successful peer; inspect wrapped context
   cancellation, empty success fields, unchanged cancelled target, peer bytes,
   and no remaining request patch after joining.
7. In `workspace_file_save_test.go` in the API package, reuse `newGitAPIFixture`,
   `workspaceRequest`, `decodeWorkspaceBody`, `runGitAPI`, and `writeFileAPI`.
   Seed/commit distinct files before gating. Drive the actual registered
   `POST /api/v1/workspace/file/content` router with correct original hashes and
   desired-content pointers using the same queue/release interleaving; use only
   test-local bounded gate/drain helpers. Check HTTP200, requested path,
   success=true, applied resolution, empty error, independent desired SHA256 and
   both final disk contents. Add fresh sequential route controls. Do not replace
   Git, bypass the real handler, or build a new fixture framework.
8. Run final anchored checks, scoped lint, document checks and coverage preflight.
   Mark this work order done and plan implemented only after actual results;
   promote draft requirement/design to active/current only if conformance holds.
   Leave delivery to the already authorized ROOT release/checkpoint sequence.

## Verification

Run commands individually from repo root, under a receipt-producing invocation
wrapper; retain and join the original native handles. The test timeouts below
include no permission to repeat a timed-out/setup-failed run automatically.

```bash
# RED, then GREEN after the minimum production correction:
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=1 ./internal/agentctl/server/process -run '^TestApplyFileDiff_(ConcurrentDistinctFiles|SequentialDistinctFiles)$' -count=1 -timeout=90s -v)

# Final process outcomes and existing compatibility:
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=1 ./internal/agentctl/server/process -run '^TestApplyFileDiff_(ConcurrentDistinctFiles|SequentialDistinctFiles|PatchCleanup|CancelledQueuedSave|RegularFile|Symlink|ConflictDetection|ConflictWithDesiredContent|ConflictWithoutDesiredContent|SymlinkConflictWithDesiredContent)$' -count=1 -timeout=90s -v)

# Registered route outcomes and existing failures:
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=1 ./internal/agentctl/server/api -run '^TestHandleFileUpdate_(ConcurrentDistinctFiles|SequentialDistinctFiles|AppliesDiff|ReportsHashConflict|Rejections)$' -count=1 -timeout=90s -v)

# Scoped lint against the admitted exact base:
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m golangci-lint run ./internal/agentctl/server/process ./internal/agentctl/server/api --new-from-rev=62b39941214ffe63ce72d307b6e599a7bd2a7b63 --concurrency=2 --allow-serial-runners --timeout=5m)

python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/workspaces docs/plans/prevent-overlapping-file-saves apps/backend/internal/agentctl/server
git status --short -- docs/specs/workspaces docs/plans/prevent-overlapping-file-saves apps/backend/internal/agentctl/server
```

Use `.github/scripts/pr-docs.cjs`'s local `validateCoverage` API with actual
changed-file paths and these four document contents to verify REQ/AC/design/
plan/work-order links. Record whether evidence is design-only prospective
coverage or actual implementation-diff coverage. Do not publish a check from
this local preflight. Backend PR fixup additionally requires ROOT's one full
CHANGED exact-base lint (`GOMAXPROCS=2`, `GOMEMLIMIT=1GiB`, concurrency2, serial,
CLI5m/GNU6m/kill10s); it is not authorized during design.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_files.go`
- `apps/backend/internal/agentctl/server/process/workspace_file_save_test.go` (new)
- `apps/backend/internal/agentctl/server/api/workspace_file_save_test.go` (new)
- The four owning design/delivery artifacts in this package, for status/results.

## Dependencies and parallelism

None within the package. `sequential`; a later ROOT implementation/heavy release
was received before starting. No implementation delegation was authorized.

## Risks and inputs

- Global admission test state needs bounded joins and restoration after drain.
- Write/close/remove ordering affects Windows; report the platform actually run.
- Best-effort removal is retained; same-file races and post-write rollback remain
  outside the tested guarantee. Preserve existing notification behavior.
- Inputs: [requirements](../../specs/workspaces/requirements/saved-file-content.md),
  [design](../../specs/workspaces/system-design/saved-file-content.md), existing
  `workspace_tracker_test.go`, `workspace_git_cmd_test.go`, `workspace_git_index.go`,
  `workspace_handlers_test.go`, and `git_handlers_test.go` fixtures.
- See [plan delivery guidance](plan.md#risks-and-delivery) and the platform task
  plan for ROOT's scheduling, hooks, review, and merge gates. A missing hook
  install permits only the separately scheduled single pinned pnpm9.15.9 frozen
  install from `apps`; never bypass hooks or change runtime dependencies.

## Results

Executed sequentially on Linux with Go 1.26.0 under ROOT's exclusive local-heavy
release. Exact product commands are the unchanged selectors in Verification;
scoped lint includes the explicitly released `--allow-serial-runners` option.

| Verification | Actual result |
| --- | --- |
| RED process selector | Expected exit1: shared and independent trackers both falsely acknowledged unchanged alpha content/hash; fresh sequential alpha/beta controls passed |
| GREEN process selector after private patch correction | Exit0; both tracker variants and sequential controls passed |
| Final process selector with race | Exit0; all ten top-level tests passed, including four cleanup outcomes and cancelled request with successful peer |
| Registered HTTP selector with race | Exit0; all five top-level tests passed, including concurrent/sequential disk and exact response outcomes |
| Exact-base scoped lint, concurrency2, serial | Exit0, zero issues |
| `list-docs.py validate`, `lint-spec-files.test.py`, `lint-spec-files.py --all` | Exit0; 357 decisions/1414 specs validated, 36 tests passed, full spec lint passed |
| `git diff --check` and exported `validateCoverage` on actual changed paths | Exit0; whitespace clean, coverage covered with errors[] |

Receipts: `/tmp/kandev-child61-implementation-receipts.jsonl`, with per-original
logs and native-handle join records in the platform task plan. RED/GREEN/process/
API/lint originals are actually joined and their groups gone. These results describe the initial Linux implementation checks. No broad suite, build, browser, E2E, network save,
WebSocket, Windows execution, or backend database persistence claim.

Design originally encountered `node` missing from PATH. ROOT's single bounded
recovery using existing Node 24.21.0 passed prospective `validateCoverage` with
no install or runtime/harness edits. That original failure and successful
recovery are both retained; actual implementation-diff coverage is a separate
final gate. Normal hooks were confirmed active; the single allowed pnpm 9.15.9
frozen workspace install was required because this worktree lacked commitlint.
Publication and hosted review remain governed by ROOT's separate delivery
checkpoints; no merge authorization was granted.

## Windows fixture correction

Hosted job `112569366176` at initial head `b96495a` failed compilation with
four undefined `waitForAnyGitWaiter` references. Its existing definition is in
a `!windows` fixture. The correction adds the save fixture's own portable
context-bound `AdmissionSnapshot().Waiters` ticker helper and replaces only
the four calls. The real Git admission, disk and hash assertions remain intact.

ROOT released a second exclusive local-heavy slot for these two serial commands
from `apps/backend`; no unchanged selector or install is replayed:

```bash
env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=1 ./internal/agentctl/server/process -run '^TestApplyFileDiff_(ConcurrentDistinctFiles|CancelledQueuedSave)$' -count=1 -timeout=90s -v
env GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./... --new-from-rev=62b39941214ffe63ce72d307b6e599a7bd2a7b63 --concurrency=2 --allow-serial-runners --timeout=5m
```

The two affected race-enabled tests passed on Linux, including both tracker
variants and queued cancellation. The single full changed-code lint invocation
ended at its six-minute outer timeout with exit124 and no diagnostic output;
its process was actually joined and its group was gone. ROOT was checkpointed
without an automatic retry, commit or push. ROOT then explicitly authorized
exactly one identical recovery: it passed with exit0 and zero issues in
236.839s, actually joined with its group gone. The original timeout remains
failed with cause unproved. The previous Linux checks did not prove Windows
compilation or execution; the actual corrected-head hosted Windows process job
must pass. Corrective catalog validation, full specification lint, whitespace
checks and exported actual-diff coverage also passed with errors[]. Preserve the
original hosted observer and deadline across
the normal corrective commit/push. Merge remains separately gated by ROOT.

## Retained-outcome shutdown correction

ROOT separately released this same order for a concrete lifecycle lock inversion
exposed while investigating current-head backend failure. The failed API test
was `TestHandleWSInitializeCarriesExactProcessEvidence`: stopping its first child
reported manager goroutines not reaped. Its artifact does not establish the
actual interleaving. Source establishes a reachable cycle: Stop holds `m.mu`
while waiting on `m.wg`; the exit waiter publishes evidence, then terminal
retention takes `m.mu.RLock` before its deferred `wg.Done`.

Reuse the existing [retained-outcome requirements](../../specs/executors/requirements/agent-survival-session-state.md)
and [design](../../specs/executors/system-design/agent-survival-across-restart-03.md#turn-outcome-across-the-detached-gap).
This correction changes synchronization only, preserving terminal filtering,
instance identity, retained copies, callback behavior and delivered turn stamps.
No new owner pair, shutdown framework, deadline relaxation or assertion removal.

Sequential correction:

1. Add `TestSendUpdateBlockingDoesNotWaitForLifecycleLock` to
   `process/turn_outcome_test.go`. Hold the actual lifecycle mutex before launching
   terminal publication; require publication and its wait-group completion while
   that mutex remains held, with nil and wired recorders. Assert actual delivered
   content and wired retention/stamp. Use channels and a five-second deadlock
   guard, with unlock and bounded join cleanup; no sleep-derived pass or product
   injection. RED must fail on blocked publication before production changes.
2. Give only recorder/instance-ID wiring a private zero-value mutex; consistently
   use it in Set, Clear and record, releasing it before recorder callbacks.
   Leave lifecycle locking and all retention/copy/stamp semantics intact.
3. Run the affected outcome selector and the exact failed API test with race,
   count10 and serial resource bounds. No saved-file suite, reinstall or browser
   replay. Run scoped lint, then exactly one full changed-code lint at the admitted
   base. Record failures without automatic retry. Update results and normal
   active-hook corrective delivery only after gates pass; retain the hosted
   original and its original deadline across the push.

From `apps/backend`, one command at a time:

```bash
# RED only, count1; then GREEN includes it in the affected selector below.
env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=1 ./internal/agentctl/server/process -run '^TestSendUpdateBlockingDoesNotWaitForLifecycleLock$' -count=1 -timeout=90s -v
# Affected outcome behavior, including the new regression:
env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=1 ./internal/agentctl/server/process -run '^Test(ClearTurnOutcomeForwardsPromptGenerationToRecorder|RecordTerminalOutcome(RetainsCompleteAndErrorEvents|PreservesRetainedPromptFailureDisposition|CopiesCapacityContinuationSnapshot|IgnoresNonTerminalEvents|NoopWithoutRecorder)|SendUpdateBlocking(DoesNotWaitForLifecycleLock|RecordsTerminalOutcomeOnDelivery|StampsControlTurnIDOnDeliveredCopy)|ForwardUpdates(RecordsTerminalOutcomeForAdapterOriginatedEvents|StampsControlTurnIDOnDeliveredCopy))$' -count=10 -timeout=90s -v
# Exact failed API test; no passing save/API suite replay:
env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p=1 -parallel=1 ./internal/agentctl/server/api -run '^TestHandleWSInitializeCarriesExactProcessEvidence$' -count=10 -timeout=90s -v
# Scoped, then ONE mandatory full changed-code lint:
env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m golangci-lint run ./internal/agentctl/server/process ./internal/agentctl/server/api --new-from-rev=62b39941214ffe63ce72d307b6e599a7bd2a7b63 --concurrency=2 --allow-serial-runners --timeout=5m
env GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./... --new-from-rev=62b39941214ffe63ce72d307b6e599a7bd2a7b63 --concurrency=2 --allow-serial-runners --timeout=5m
```

Additional files:
`apps/backend/internal/agentctl/server/process/manager.go`,
`apps/backend/internal/agentctl/server/process/turn_outcome.go`,
`apps/backend/internal/agentctl/server/process/turn_outcome_test.go` and the
existing Executors design reference.

Independent RED failed both nil and wired cases on terminal publication blocked
behind the held lifecycle lock; cleanup released the lock and joined both
publishers. After the isolated recorder mutex correction, all 11 selected outcome
tests passed ten times with race enabled, including both regression variants.
The exact failed API test also passed ten race-enabled runs. Scoped and mandatory
full changed-code lint both passed with zero issues; the single full invocation
completed in 288.276s. All originals were actually joined with process groups
gone. These checks independently prove the removed lock dependency; they do not
prove the original hosted artifact's unobserved interleaving. Prior Windows
execution passed after the portable waiter correction; this additional backend
correction still requires fresh hosted native Windows and parent workflow success.
