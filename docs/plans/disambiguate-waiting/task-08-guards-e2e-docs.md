---
task: 08
title: "Guards, E2E, contract amendment docs"
wave: 3
status: todo
depends_on: [03, 06, 07]
spec_acs: AC-35, AC-76, AC-77, AC-78, AC-79a, AC-82
---

# Task 08 — Guards, E2E, and contract amendment

Structural invariants, the end-to-end sequence, and the two doc amendments that close the
spec. This is the last task; it cannot start until the projection (task-07) and both
frontend halves (tasks-06, -03) are done.

## 1. AC-35 — Architecture test: recogniser seam

Write a test in `internal/orchestrator/` (or a dedicated `parked_architecture_test.go`)
that asserts the backend predicate reads only `IsDetachedBackgroundLaunch()` and
`Kind()` — never `payload.AgentID`:

```go
// TestParkedRecogniserUsesNeutralPredicate ensures no orchestrator code
// references NormalizedPayload.AgentID for the parked computation.
func TestParkedRecogniserUsesNeutralPredicate(t *testing.T) {
    // Inject a mock-agent event (agentID="mock-agent") carrying
    // Kind==BackgroundWorkKindShell and Detached==true. Verify observedDetached
    // is set. This confirms the predicate does not gate on agentID.
    ...
}
```

Also write a compile-time or runtime assertion that adding a second agent recogniser
entry (i.e., calling `stampBackgroundShellWork` in `normalize.go` with a second agentID)
requires no change to any file in `internal/orchestrator/`.

## 2. AC-76 — Notification guard

Verify that `session.turn_finished` (or the equivalent notification event) is NOT
suppressed or deferred by the parked state. The guard must confirm that notification
behaviour is unchanged by this spec:

```go
// TestParkedDoesNotAffectNotifications verifies the parked projection does not
// intercept, delay, or suppress session.turn_finished publications.
func TestParkedDoesNotAffectNotifications(t *testing.T) {
    // Run a full session settle: WAITING_FOR_INPUT transition with parked=true.
    // Verify that session.turn_finished is published immediately, not deferred.
    // Verify that no notification-related field of the session DTO changes.
}
```

This is the guard that confirms the ATTRIBUTION half ships without taking on the
notification-deferral concern (the sibling spec's domain).

## 3. AC-77/AC-78 — Epoch and restart-survivable discard

Write an orchestrator integration test:

- Start with a parked session (parked=true, epoch=E, revision=R).
- Simulate a backend restart: change `parkedEpoch` to `E+1`, reset revision to 0.
- Verify the first session notification after restart carries `epoch=E+1, revision=0,
  parked=false` (the boot state).
- Verify the frontend discard rule (tested in task-06) accepts the new epoch and applies
  the new value, discarding no fields from the restarted frame.

## 4. AC-79a — FIFO ordering guard

Add an adapter-level test that sends a detached shell tool-call frame on the notifQueue
and then a `turn_started` immediately after. Verify that the ordered consumer receives
the tool-call before the `turn_started`, so `observed_detached` is set in the turn that
precedes the `turn_marker` increment.

This is what ensures the FIFO ordering established in task-01 (both stamp and emit inside
`syncNotifQueueThen`) propagates correctly through the delivery chain.

## 5. AC-82 — No new i18n key

Verify that the parked affordance uses the existing translation keys
`task-state-background-running` and `task:backgroundWorkIsRunning` and that no new key
is introduced:

```bash
pnpm run i18n:check
pnpm run i18n:ratchet
```

Both must pass. Moving `BackgroundWorkTaskIcon` between files does not change either key.

## 6. End-to-end sequence (§J test)

Write or extend the E2E spec (`apps/web/e2e/`) to cover the §J observation sequence:

**Pre-condition**: a session with `WAITING_FOR_INPUT` and a live detached shell process.

```
E2E scenario: "parked-on-background-work"
1. Agent launches a background shell (Bash with background=true).
2. Agent settles to WAITING_FOR_INPUT.
3. Synchronous first sample fires — probe returns "live".
4. Board card shows BackgroundWorkTaskIcon.
5. Sidebar shows BackgroundWorkTaskIcon.
6. /tasks row shows BackgroundWorkTaskIcon.
7. Session icon shows BackgroundWorkTaskIcon.
8. Fake background process exits. Next sampler tick fires.
9. Probe returns "settled". parked → false.
10. Board returns to previous icon (no background icon).
```

Use an injectable probe seam so the E2E does not require a real 30-second wait:
- Steps 3–7 inject probe→`"live"`.
- Steps 8–10 inject probe→`"settled"`.

**AC-70, AC-70a, AC-71, AC-72, AC-80** (real process-tree tests from task-04) are unit
tests, not E2E. They are already written in task-04; reference them here as closed.

The `containers` project (gated on `KANDEV_E2E_CONTAINERS=1`) is not needed for this
E2E — the injectable probe means no real Docker daemon or process isolation is required.

## 7. Contract amendment docs

**`docs/specs/platform/background-work-liveness.md` line 25** — amend:

Current text:
> A settled session follows its coarse state and does not remain visually busy solely
> because detached work is still registered.

New text:
> A settled session follows its coarse state and does not remain visually busy solely
> because detached work is still registered. A session may remain visually busy — showing
> the parked-on-background-work affordance — only when a positive out-of-band liveness
> sample supports it, never on registration alone.

**`docs/specs/INDEX.md`** — verify the rows for this spec, the elicitation spec, and the
deferral spec are accurate and up to date. Update `status` for this spec to `shipped`.

**`docs/specs/disambiguate-waiting/spec.md`** — update the spec `status` frontmatter from
`draft` to `shipped` after the PR merges.

## Acceptance criteria closed

- **AC-35**: architecture test confirms neutral predicate; second-vendor extension needs no
  backend change.
- **AC-76**: notification guard confirms no suppression or deferral of `turn_finished`.
- **AC-77**: epoch in session DTO; restart produces epoch increment.
- **AC-78**: restart-survivable discard tested end-to-end.
- **AC-79a**: FIFO ordering guard for the notifQueue delivery chain.
- **AC-82**: no new i18n key; existing keys reused.
- **§J end-to-end**: observable sequence with injectable probe, no real 30s wait.
