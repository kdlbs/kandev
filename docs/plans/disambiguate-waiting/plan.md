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

- [ ] [Task 01: `turn_started` stream event and `observed_detached` clearing](task-01-turn-started-event.md)
- [ ] [Task 02: Launch-recogniser registry seam](task-02-recogniser-registry.md)
- [ ] [Task 03: Background-workload liveness probe (agentctl, cross-platform)](task-03-liveness-probe.md)
- [ ] [Task 04: Probe transport (`agent.background.probe` WS action) and config](task-04-probe-transport.md)
- [ ] [Task 05: Parked projection (backend, session-level, `BackgroundProbe` port)](task-05-parked-projection.md)
- [ ] [Task 06: Task-level projection, revision epoch, API surface publish rules](task-06-task-projection-api-surface.md)
- [ ] [Task 07: Frontend rendering — two task-icon resolvers and the board](task-07-frontend-task-rendering.md)
- [ ] [Task 08: Frontend rendering — `/tasks` row, session switcher, TS types, docs amendment](task-08-frontend-session-rendering-docs.md)
- [ ] [Task 09: E2E coverage](task-09-e2e.md)

## Definition of done (whole card)

`make fmt`, `make typecheck test lint`, `make lint-format`, `cd apps/web && pnpm
run i18n:ratchet` all clean; every AC in the spec has a named test (or is recorded
above as a documented Build-time reinterpretation); `docs/specs/INDEX.md` and
`docs/specs/platform/background-work-liveness.md` updated.
