---
spec: docs/specs/disambiguate-waiting/spec.md
created: 2026-09-04
status: in-progress
---

# Implementation Plan: Waiting Attribution (parked-on-background-work)

## How this plan was produced

The card was moved directly from Spec Review round 5 ("accept the remaining gaps
and proceed to Build") into Build without a Plan step. This plan was reconstructed
from the frozen spec (`docs/specs/disambiguate-waiting/spec.md`, 46 ACs) plus a
recon pass against the current tree (main has advanced past the spec's pinned
`bf62f39b1`; no structural drift, only line-number and minor-signature drift —
see "Recon deltas" below). No spec content is changed by this plan.

Per the round-5 human decision, 21 findings (F1-F22 in the task plan's Spec Review
history) are carried into Build OPEN. This plan makes the required build-time
calls for the five NEEDS-RETHINK-class ones and the two FIX-FIRST ones the human
flagged as most load-bearing (F6 security, F7 ordering), and records them below
rather than silently building past them.

## Recon deltas from the spec's `bf62f39b1` citations (verified against current `main`-derived tree)

- `clientCapabilitiesForAgent` builds `acp.ClientCapabilities` from the vendored
  `acp-go-sdk`, not a local struct — irrelevant to this spec (elicitation's concern).
- `agent.go` action switch has grown to 12 cases; the "completeness" test only
  covers 7. Not a blocker, but the new `agent.background.probe` case must be added
  to that test explicitly since the test will not catch its omission.
- `sendStreamRequest` lives in `client_stream.go`, not `agent.go` (same `Client` type).
- `CheckSessionAccess` lives in `manager.go:409`, not `manager_interaction.go`.
- Two parallel frontend `Task` type shapes exist with different casing:
  `lib/types/http.ts` (snake_case, wire) and
  `lib/state/slices/kanban/types.ts` (camelCase, store). `rich-task-list-row.tsx`
  uses the former, `kanban-card-content.tsx` the latter. New fields must be added
  to both, plus the wire→store normalization/hydration layer.
- Mobile session-switcher file is at
  `apps/web/components/task/mobile/mobile-sessions-section.tsx`, not
  `apps/web/components/mobile/...`.
- The `CancellationPendingSnapshot` / `EnrichCancellationPending` precedent
  (`task_operations.go:4478`, `internal/task/dto/cancellation_pending.go`) is
  exactly the shape to mirror for `parked_on_background_work` + `revision`: a
  type-asserted "snapshot provider" upgrade that degrades to `revision: 0`.

## Build-time decisions for the round-5 NEEDS-RETHINK findings (F1-F5)

These are not spec changes. They are the concrete choices Build must make because
the AC text cannot be satisfied literally, recorded here per the round-5 human
decision's explicit instruction.

- **F1 (AC-69, launch-recogniser seam).** Split into two tests: (a) a behavioural
  test that registers a second recogniser through the public registration API and
  asserts the resulting session parks and its task row renders
  `task-state-background-running` (this test may import whatever it needs —
  projection, rendering, test harness); (b) a package-graph import-direction test
  asserting the recogniser-registry package imports nothing from the probe,
  projection, or rendering packages. (b) is the mechanically-checkable form of the
  "adding a vendor changes nothing else" guarantee; (a) is the observable behaviour.
  See task-02.
- **F2 (AC-35, architecture test).** Not package-granularity (the projection lives
  in `internal/orchestrator`, the same package as the `claudeBackgroundPromptHandoff`
  accessor, per precedent). Instead: a source-scoped test naming the exact new files
  this feature adds (parked-projection, probe client, recogniser registry — listed
  explicitly in task-05/task-06) and asserting none of their source text references
  `claudeBackgroundPromptHandoffEnabled` or `features.claudeBackgroundPromptHandoff`.
  This is checkable by construction and does not vacuously pass by naming an
  unrelated package. See task-05.
- **F3 (AC-27b / AC-70 / AC-72, truncated vs. exact turn-start boundary).** The
  probe predicate, D5, and AC-80 all require comparing against the *truncated*
  turn start. AC-27b/AC-70/AC-72's GIVEN text says "strictly before the recorded
  turn start" (untruncated). Build implements the truncated comparison everywhere
  (it is the only self-consistent reading and the one AC-80 requires) and treats
  the three ACs' prose as describing the truncated boundary. See task-03.
- **F4 (AC-77, epoch reset has no publish trigger).** Chose: **the boot payload is
  the sole reset-delivery mechanism.** A client that reconnects the WebSocket
  without re-fetching boot will not receive a synthetic reset frame — no generic
  "resync on reconnect" broadcast mechanism exists in this codebase today, and
  building one is a separate architectural change outside this feature's scope.
  This narrows AC-77's WebSocket-only clause; the gap is accepted per the round-5
  note's explicit alternative ("AC-77's WebSocket-only clause is withdrawn"). See
  task-06.
- **F5 (`parked_epoch` non-monotonic across a clock step).** Kept as specified
  (Unix-nanosecond process start time). The risk (NTP step / restored snapshot /
  host migration producing an equal-or-lower epoch) is accepted per the round-5
  note's explicit allowance to "record the accepted risk" — building a persisted
  monotonic counter is a durability change this feature's "no schema migration"
  constraint forecloses. See task-06.

## Other Build-time calls (FIX-FIRST findings the human flagged as load-bearing)

- **F6 (security).** `agent.background.probe` MUST NOT ride the same unguarded
  path as `RespondToPermissionBySessionID`. Add an explicit
  `CheckSessionAccess`-equivalent guard on the new WS action before it reaches
  agentctl. See task-04.
- **F7 (ordering).** `session.turn_finished` is emitted **before** the synchronous
  probe is taken, so AC-76's "not delayed" holds unconditionally. See task-03/task-05.
- **F8/F10/F12 (cheap, closed here).** Linux boot anchor named as `/proc/uptime`
  (relative monotonic offset from process start, not `/proc/stat` `btime`); a
  non-positive `KANDEV_PARKED_PROBE_INTERVAL` is rejected at config load the same
  way as the budget; the five process-tree ACs (AC-70/70a/71/72/80) run as Go
  tests in `internal/agentctl/.../probe` guarded by `runtime.GOOS`, with the
  Darwin branch skipped in Linux CI (no macOS runner exists) and exercised only
  when run locally on `darwin`. See task-03.
- **F18 (contract amendment).** `docs/specs/platform/background-work-liveness.md:25`
  amendment is task-08, not left implicit.

## Implementation Waves

Ordered so each wave is independently testable against the `BackgroundProbe`
port / recogniser registry, per the spec's own "Notes for implementation"
guidance (land the transport before the projection; build the projection
against the port, not the real probe).

- [x] [Task 01: `turn_started` stream event and `observed_detached` clearing](task-01-turn-started-event.md) — commit `0c2253717`. Implemented as agentctl `EventTypeTurnStarted` (emitted from `beginPromptTurn`, covers both human and synthetic-wakeup dispatch) plus a backend `observedDetached` map on `orchestrator.Service`, set from the existing `normalizedIsDetachedLaunch` shell-kind branch in `trackBackgroundToolUpdate` and cleared on the new event in `handleAgentStreamEvent`. All new/updated tests green; `go build ./...`, `go vet ./...`, `golangci-lint run --new-from-rev=bf62f39b1` clean.
- [x] [Task 02: Launch-recogniser registry seam](task-02-recogniser-registry.md) — commit `2eb1aba`. Added `backgroundlaunch` (new leaf package: `Recognizer` interface, `Register`/`Lookup`/`RecognizesDetachedLaunch`, panic-on-nil/empty/duplicate registration, fail-closed on a panicking recognizer). Registered Claude's recognizer via `init()` in `background_launch_recognizer.go`; rewrote `stampBackgroundShellWork` to delegate to the registry. AC-69(a) (behavioural: a second registered agent attests through the unmodified `stampBackgroundShellWork`) and AC-69(b) (import-direction: registry package imports nothing from orchestrator/task/probe/apps-web) both covered. `go build ./...`, `go vet`, full `acp`+`orchestrator` suites, `golangci-lint run --new-from-rev=bf62f39b1` all clean.
- [x] [Task 03: Background-workload liveness probe (agentctl, cross-platform)](task-03-liveness-probe.md) — commit `52e0e4411`. New package `internal/agentctl/server/process/probe`: `ProbeBackgroundWorkloads(agentPID, turnStart)` returns `live`/`settled`/`unknown` (D5/D9). Transitive-descendant closure + start-time predicate computed against an injectable `processTableReader` seam; Linux reader parses `/proc/<pid>/stat` anchored to `/proc/uptime` (not `btime`, per F8), Darwin reader uses `sysctl kern.proc.all` (already wall-clock); Windows has no reader and always returns `unknown`. Turn start is truncated to the platform resolution before comparison, everywhere (F3). Synthetic table tests cover AC-27a/70/70a/71/72/80 deterministically; `probe_realtree_test.go` re-drives the same ACs against this test binary's real subprocess tree, GOOS-gated at runtime (compiles everywhere, explicit skip in CI) — verified passing for real on this session's Darwin host, including the own-process-group case (AC-71) and the truncation case on Linux only (AC-80; Darwin's 1µs resolution has no realistic margin for a real-tree assertion, so AC-80 is authoritatively covered by the synthetic test there). `go build`/`go vet` clean cross-compiled for linux/darwin/windows; full `probe` suite green (incl. `-race`); `golangci-lint run --new-from-rev=bf62f39b1` clean. Pre-existing, unrelated GitLab-CLI test failures observed elsewhere in the `process` package (confirmed outside this diff's scope).
- [x] [Task 04: Probe transport (`agent.background.probe` WS action) and config](task-04-probe-transport.md) — commit `cb2bf4398`. Added the `agent.background.probe` WS action on agentctl (`handleWSBackgroundProbe`), gated on the optional `adapter.TurnStartRecorder` capability interface (task-01's `RecordedTurnStart`) and `process.Manager.AgentPID()`; wired end-to-end through `agentctl-client.Client.ProbeBackgroundWorkloads` (exhaustive failure→`ProbeResultUnknown` mapping), `lifecycle.Manager.ProbeBackgroundWorkloadsBySessionID`, the `executor.AgentManager` interface, `backendapp.lifecycleAdapter`, to a new `orchestrator.Service.ProbeBackgroundWorkloads` production `BackgroundProbe` port implementation. F6 (round-5, security): added an explicit `authorizeSession` guard before the call reaches the executor chain (the spec's claimed inherited guard doesn't exist in the tree) and pinned it in `session_scope_matrix_test.go`. `KANDEV_PARKED_PROBE_BUDGET`/`KANDEV_PARKED_PROBE_INTERVAL` config added with non-positive-budget rejection (default fallback + warn log) and F10's zero-interval-is-valid/negative-interval-rejected distinction; budget applied via `context.WithTimeout` around the executor call, never baked into the transport. `executor.ProbeResult` re-exports `client.ProbeResult` so `internal/orchestrator` never imports `internal/agent/runtime/agentctl` directly (`ARCH-RUNTIME-IMPORT` architecture-lint gate — caught and fixed post-hoc, not baselined). All new unit tests green across 3 packages; `go build ./...`, `go vet ./...` (incl. cross-compiled linux/darwin/windows for touched shared code), `golangci-lint run --new-from-rev=bf62f39b1`, and `python3 scripts/lint-architecture.py --all` all clean. Broad regression run across `orchestrator`/`agentctl`/`agent`/`backendapp`/`integration` showed only the two already-known pre-existing/environmental failures (GitHub-CLI login-shell shim depending on this host's `~/.bash_profile`; GitLab-CLI remote-host tests) — no new regressions.
- [x] [Task 05: Parked projection (backend, session-level, `BackgroundProbe` port)](task-05-parked-projection.md) — commit pending. New `internal/orchestrator/parked_projection.go`: per-session `parkedSessionState` (parked, revision, lastSample, lastSampledAt) behind `parkedMu`, mirroring `CancellationPendingSnapshot`'s single-critical-section pattern; `ParkedSnapshot`/`ParkedEpoch` public accessors for task-06. Wired via one chokepoint — `onSessionStateChangedForParkedProjection`, called from `updateTaskSessionStateWithHook` (every session-state transition passes through it) — rather than each of `setSessionWaitingForInput`'s ~40 call sites individually: entering `WAITING_FOR_INPUT` triggers D2's synchronous first sample (`settleParkedProjectionSync`, skipped entirely when `observed_detached` is false per AC-40a); leaving it triggers D8's immediate clear with no new sample (`clearParkedOnSessionStateLeft`, AC-68). F7 ordering: `updateTaskSessionStateWithHook` runs downstream of `completeTurnForTaskSession` in the real turn-settle call path, so the probe always follows whatever publishes `session.turn_finished`; pinned by a source-text ordering test (same technique as F2, since a runtime reordering test would need the full task-service/notification wiring) rather than left implicit. F2's AC-35 architecture test added as `parked_projection_no_flag_test.go`, file-scoped per the round-5 disposition. Sampling loop (AC-53/54/D9): one goroutine per parked session on `KANDEV_PARKED_PROBE_INTERVAL` (interval `<=0` never starts a loop), lifecycle mirrors the existing `sendNowCtx`/`sendNowCancel`/`sendNowWorkers` pattern and is joined in `Service.Stop()`. F9 (round-5): the periodic tick re-reads session state fresh and discards a sample if the session's revision moved while the probe was in flight (a self-resume racing the sample), tested explicitly. `BackgroundProbe` port defaults to a thin adapter over `Service.ProbeBackgroundWorkloads` (task-04), overridable via `SetBackgroundProbe` for tests — every new test scripts the port rather than depending on the real transport or process walk, per the task's own guidance to build against task-04's port without depending on task-03's real probe. `publishParkedTransition` emits `task_session.activity_changed` (wire: `session.activity_changed`) with `parked_on_background_work`/`revision`/`parked_epoch` for AC-68; the conditional `task.updated` publish and full DTO/boot-payload wiring are task-06's. 16 new tests, all green (incl. `-race` and the package's `goleak.VerifyTestMain`); `go build ./...`, `go vet ./...`, `golangci-lint run --new-from-rev=bf62f39b1` clean. A same-session broad multi-package regression run hit host-level `no space left on device` linker failures (disk at 98%, unrelated build artifacts) on packages this task never touched; `internal/orchestrator` alone (the only package this task modified) was re-verified green in isolation, including with `-race`, both before and after that disk exhaustion. Also fixed in this commit: `docs/plans/disambiguate-waiting/plan.md`'s task-04 checkbox, edited during task-04 but never actually staged/committed at the time.
- [ ] [Task 06: Task-level projection, revision epoch, API surface publish rules](task-06-task-projection-api-surface.md)
- [ ] [Task 07: Frontend rendering — two task-icon resolvers and the board](task-07-frontend-task-rendering.md)
- [ ] [Task 08: Frontend rendering — `/tasks` row, session switcher, TS types, docs amendment](task-08-frontend-session-rendering-docs.md)
- [ ] [Task 09: E2E coverage](task-09-e2e.md)

## Definition of done (whole card)

`make fmt`, `make typecheck test lint`, `make lint-format`, `cd apps/web && pnpm
run i18n:ratchet` all clean; every AC in the spec has a named test (or is recorded
above as a documented Build-time reinterpretation); `docs/specs/INDEX.md` and
`docs/specs/platform/background-work-liveness.md` updated.
