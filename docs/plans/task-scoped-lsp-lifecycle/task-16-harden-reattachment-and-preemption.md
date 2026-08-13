---
id: "16-harden-reattachment-and-preemption"
title: "Harden task-host reattachment and terminal preemption"
status: completed
wave: 8
depends_on: ["15-harden-shared-environment-recovery"]
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 16: Harden Task-Host Reattachment and Terminal Preemption

## Acceptance

- Backend startup reserves every persisted possibly-live generation before fallible task-host
  inspection; authoritative absence or confirmed stop releases the exact durable/runtime identity.
- Missing task-host cache state triggers existing-only physical reattachment before creation;
  unsupported or uncertain probes cannot launch, resume, or provision resources.
- Explicit Stop, Disabled policy, and terminal task mutation cancel blocked Start/install work
  before waiting for language or task admission.
- A selected document root's canonical identity comes from the same OS root handle that remains
  pinned; replacing its pathname between resolution and pinning cannot authorize an outside root.
- Browser task/language/session leases survive WebSocket generations and one session's reconnect
  cannot silently detach another session; final lease release drains that session's document
  references before removing routing membership.
- Task subscription acknowledgement precedes the authoritative post-subscribe refresh, closing the
  stable-state lost-event window.
- Workspace config commit and all live runtime, hub, folder-notification, and snapshot updates are
  serialized per task, with no stale refresh applying after a newer commit.
- Capacity release reserves and promotes the next task asynchronously under controller lifecycle;
  Stop does not wait for another task's launch, canceled promotion cannot leak capacity, and Close
  removes queued owned promotion work while joining any promotion already running.
- Startup inventory failure becomes a sticky fail-closed controller error; controls never observe
  an empty capacity ledger as ready after durable inventory could not be read.
- Recovery and ready-budget-reset callbacks acquire lifecycle ownership before I/O, validate their
  timer generation, retain lifecycle context through command execution, cannot outlive Close, and
  retain a recovery signal that arrives while an attempt is still completing. New crash evidence
  invalidates a fired ready-budget reset before it can commit stale Ready evidence; fired resets and
  watch-loss persistence/scheduling share the owned per-language command lane.
- Startup registers watches from post-reconcile rows, and watch loss cannot overwrite a current
  non-server phase.
- Concurrent or retried Close calls share one lifecycle completion signal; recovery, ready-reset,
  and discovery callbacks validate immutable registration identity across map deletion/recreation.
- Generation-scoped process-absence evidence survives display errors and backend restart, while a
  pre-command Restart reattachment failure retains the old server's capacity reservation.
- Task and settings reconciliation reload current durable policy inside the serialized language
  lane, so an inventory snapshot captured before an enable cannot stop the newly ready server.
- A stale global inventory row removed by task cleanup releases only its exact adopted generation,
  so delete cannot resurrect a phantom capacity occupant or erase newer generation evidence.
- The same compensation runs before task admission when cleanup has already persisted durable
  process-absence evidence but terminal deletion still owns the admission writer.

## TDD Evidence

- Root-handle replacement and identity-mismatch regressions failed before same-handle pinning.
- Multi-session reconnect, final-lease release, unavailable close, and activation retry regressions
  failed before manager-owned stable leases and reconnect.
- Concurrent workspace refresh ordering failed before the per-task ref-counted update gate.
- Equal-snapshot capacity, cleanup-error recovery, and startup-admission regressions failed before
  synchronous startup reconciliation.
- Ambiguous startup inspection and divergent durable/runtime generation regressions failed before
  conservative pre-inspection capacity reservation and identity-aware release.
- Detached/unhealthy task-host lookup and no-create executor regressions failed before physical
  existing-only reattachment.
- Existing-only cleanup with a deleted launch profile failed before reattachment stopped resolving
  launch-time profiles and environment inputs; the absent and live physical-runtime cases now prove
  cleanup independently while a later new launch still fails closed on the missing profile.
- Blocked Start versus Stop and terminal task-mutation regressions failed before command and task
  admission cancellation.
- Task subscription readiness and post-ACK hydration regressions failed before acknowledged task
  subscriptions.
- Cross-task Stop/promotion, canceled-reservation handoff, and controller-close regressions failed
  before lifecycle-owned asynchronous capacity promotion; the focused set passed 20 race-enabled
  repetitions.
- Final session lease/document cleanup failed before document references retained their owning
  session and were drained ahead of lease membership.
- Controller Close blocked behind a detached command before queued lifecycle-owned batches became
  cancellation-aware while running batches remained joined.
- Startup inventory failure admitted a new server against an empty capacity ledger before the
  readiness gate retained the authoritative error.
- Fired recovery and ready-budget-reset timers outlived Controller Close before callback admission,
  epoch validation, and owned recovery commands were joined to the controller lifecycle.
- Startup used pre-reconcile rows to create watches after rows had converged to non-server phases;
  the resulting watch loss could replace `waiting_for_task` with an error and schedule recovery.
- A timed-out Close made later Close calls return success before owned callbacks joined, and deleted
  timer entries could reuse epoch one so delayed callbacks consumed their replacements.
- Restart released capacity when task-host access failed before the replacement RPC even though the
  old generation could still run; proven start absence was also lost behind a generic display error
  and became a phantom capacity occupant after backend restart.
- Task and settings reconciliation stopped a newly enabled server when it entered the language lane
  with a Disabled snapshot captured before the user action completed.
- Global reconciliation could re-adopt a pre-cleanup live generation after cleanup released it and
  task deletion removed the row, permanently consuming capacity without a runtime.
- The same ghost survived while the cleanup-written Off row still existed because terminal
  admission rejection returned before missing-row compensation could run.
- Stale startup inventory could erase a newer accepted queue entry for the same task/language,
  reactivate the retired generation, and let reconciliation launch above the configured cap.
- A fired recovery inspected state and the task host outside the language lane; after explicit Start
  reached generation 2 Ready, that stale Disabled snapshot stopped generation 2 back to Off.
- A recovery signal arriving after the running command released its language lane but before the
  callback cleared `runningEpoch` was discarded, leaving keep-warm error state without the next
  bounded retry.
- A fired five-minute ready reset could read Ready, then let a crash consume the next backoff, then
  overwrite the retry attempt count with zero from its stale read.
- In the inverse split ordering, crash state could persist before the stale reset committed while
  recovery scheduling ran afterward, producing a one-second retry from an unreset budget; watch-loss
  state updates could bypass the language lane in the same way.

## Verification

Completed on 2026-08-13 after rebasing onto `origin/main`. Verification results:

- `go test -race -count=1 ./internal/lsp ./internal/agent/runtime/lifecycle
  ./internal/agentctl/server/lsp ./internal/gateway/websocket` passed.
- The ambiguous-startup reservation, mismatched-generation release, and queued-owned shutdown
  regressions passed 20 race-enabled repetitions.
- The deleted-launch-profile existing-only cleanup regression passed 20 race-enabled repetitions,
  and the production Playwright branch-split reproducer passed with its profile-deletion cleanup.
- The exact-head backend shard exposed an older creation test that treated an existing-only probe as
  a new launch. Its fixture now proves physical absence first, then verifies the real launch request;
  both the launch and deleted-profile regressions passed 20 race-enabled repetitions and the full
  lifecycle race package passed.
- `go test -race -count=20 ./internal/task/service -run
  '^TestServiceTerminalMutationCancelsActiveTaskLSPAdmission$'` passed. The full task-service race
  package also exercised the changed path; one broad run exceeded the package timeout while waiting
  in unrelated SQLite schema setup, so the deterministic changed-path race test was repeated twenty
  times instead of weakening the package timeout.
- Both GitHub backend package-shard selections, 291 packages total, passed locally under `-race`.
- The four focused frontend reconnect, task-subscription, and browser-lease files passed all 37
  tests; `pnpm run typecheck` and changed-file ESLint with zero warnings passed.
- `pnpm run build:vite`, the public-docs validator (60 tests), i18n checks, and i18n ratchet passed.
- All 10 production Playwright task-lifecycle scenarios passed against rebuilt production artifacts,
  including same-task deduplication, reload reattachment, explicit restart/stop, task cleanup, and
  active-file-independent status.
- The agentctl LSP package cross-compiled for Windows amd64. GitHub's native Windows containment
  check passed the selected-root junction and hostile-UNC tests after the test was updated to use a
  real pinned root handle.
- The sticky startup-inventory failure and fired recovery/reset callback ownership regressions
  passed 20 race-enabled repetitions; the complete `internal/lsp` race package passed once.
- Post-reconcile watch selection, non-server watch-loss preservation, shared Close joining, and
  recovery/ready/discovery registration-identity regressions each failed before their production
  repairs, then passed 20 race-enabled repetitions together.
- Pre-command Restart reservation and durable process-absence regressions failed before their
  repairs, then passed with the SQLite round-trip and migration-backfill coverage.
- Concurrent SetPolicy/ReconcileTask and ApplySettings/ReconcileAll regressions failed before
  reconciliation reloaded durable state inside the language lane, then passed 20 race-enabled
  repetitions. PostgreSQL replay coverage now rewinds the live table to the pre-absence-evidence
  schema, verifies selective legacy backfill, replays idempotently, and verifies allocation clears
  the proof. Its focused race-enabled test passed against a disposable PostgreSQL 16 container.
- The stale inventory/delete regression failed with one phantom active slot before the fix, then
  passed 20 race-enabled repetitions with exact-generation release inside the language lane.
- Its durable-Off/admission-blocked variant also failed with one phantom slot before compensation
  moved ahead of admission inspection, then passed 20 race-enabled repetitions.
- The stale-inventory/newer-queue unit and controller regressions failed with the legitimate queue
  removed, two active slots at a limit of one, and one unauthorized launch before adoption became
  generation ordered; both passed 20 race-enabled repetitions after the fix.
- The fired-recovery/explicit-Start regression failed with generation 2 stopped by stale recovery,
  then passed 20 race-enabled repetitions with recovery epoch validation, state reload, inspection,
  host recovery, and relaunch held inside one controller-owned language command.
- The running-recovery lost-wakeup regression failed without a next timer, then passed 100
  race-enabled repetitions after concurrent recovery demand was retained for the next bounded
  backoff; the complete `internal/lsp` race package passed.
- The fired-ready-reset/crash regression failed with a 30-second timer paired to an impossible zero
  attempt count, then passed 100 race-enabled repetitions together with ready-reset cancellation,
  epoch replacement, and normal five-minute budget-reset coverage.
- The inverse error-persist/reset/schedule ordering and watch-loss lane regressions failed before
  ready-reset and watch-loss work joined the owned language lane, then passed 100 race-enabled
  repetitions with the broader recovery and controller-close suite.
- The dependent backend race run passed LSP (including 20 complete-package repetitions), agentctl
  LSP, gateway, lifecycle, task service, and SQLite. An earlier unrelated lifecycle run hit the
  existing SSH fake-server session-open timeout once; that isolated test immediately passed three
  race-enabled repetitions, and the final exact-head local rerun passed.
- `go test -count=1 ./...` passed across the complete backend after the callback-ownership repair;
  changed-code `golangci-lint` reported zero issues and the architecture linter passed.
- Commit hooks passed architecture, formatting, changed-code Go/Web lint, i18n, public-copy, and
  Conventional Commit checks.
- Independent review findings for root identity, browser lease recovery, workspace refresh ordering,
  detached task-host recovery, startup capacity adoption, terminal preemption, task subscription
  readiness, asynchronous capacity promotion, startup admission failure, final-lease document
  cleanup, queued-owned shutdown, and E2E strength were reproduced and closed with deterministic
  regressions. PR-wide exact-head CI and read-only review remain delivery gates rather than
  implementation-task acceptance criteria.

## Files

- `apps/backend/internal/lsp/**`
- `apps/backend/internal/task/service/**`
- `apps/backend/internal/agent/runtime/lifecycle/**`
- `apps/backend/internal/agentctl/server/lsp/**`
- `apps/backend/internal/backendapp/task_lsp.go`
- `apps/web/lib/lsp/**`
- `apps/web/lib/ws/**`
- `apps/web/hooks/use-lsp*`
- `apps/web/hooks/domains/lsp/**`
- `docs/decisions/2026-08-05-task-scoped-lsp-ownership.md`
- `docs/specs/lsp-file-intelligence/spec.md`
