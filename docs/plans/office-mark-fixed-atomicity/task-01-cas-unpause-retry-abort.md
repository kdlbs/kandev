---
id: "01-cas-unpause-retry-abort"
title: "CAS unpause write and retry-abort guard"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-RUNTIME-001
acceptance_criteria:
  - AC-OFFICE-RUNTIME-001.9
  - AC-OFFICE-RUNTIME-001.10
system_design:
  - ../../specs/office/system-design/runtime-01.md
  - ../../specs/office/system-design/runtime-02.md
---

# Task 01: CAS unpause write and retry-abort guard

## Summary

Close the TOCTOU window in **Mark fixed** on `agent_paused_after_failures`:
a second, unrelated auto-pause landing between the read and the write could
previously be silently clobbered, letting the caller reset the counter and
recover tasks against the wrong pause. Gate the unpause write on the
observed `pause_reason`, abort the service-layer retry instead of adopting
a newer reason it never observed, and dismiss the inbox entry only after a
successful clear.

## In scope

- `UnpauseAgentIfCurrent`'s compare-and-set predicate on `pause_reason` in
  addition to `status`.
- `clearAutoPause`'s retry-abort branch: still-paused-with-a-different-reason
  on reload aborts rather than retrying.
- Reordering `MarkAgentPausedFixed` to dismiss the inbox entry after the
  clear succeeds, not before.
- Regression tests for the CAS refusal (repository layer) and the abort
  branch (service layer, both the abort case and a matching-reason control).

## Out of scope

- `agent_run_failed`'s dismiss-then-retry ordering (unaffected - it clears a
  session, not a shared `pause_reason` field).
- Reassignment, the counter-reset UX, or any change to the transition table
  the unrelated `office-agent-recovery` control uses.
- A third retry attempt beyond `clearAutoPause`'s existing two-attempt cap.

## Acceptance

- `UnpauseAgentIfCurrent` returns `changed = false` and leaves the row
  untouched when `pause_reason` no longer matches the caller's value, even
  though `status` is still `paused`.
- `clearAutoPause` returns an error, leaves `pause_reason` at the newer
  value, and does not proceed to reset `consecutive_failures` or dismiss the
  inbox entry when a reload shows the agent still paused with a reason the
  call never observed.
- `clearAutoPause` still succeeds and clears the pause when the reload's
  reason matches what the call started with.

## Verification

```bash
(cd apps/backend && go test -tags fts5 -count=1 -run TestUnpauseAgentIfCurrent_RefusesWhenPauseReasonChanged ./internal/office/repository/sqlite/)
(cd apps/backend && go test -tags fts5 -count=1 -run TestClearAutoPause ./internal/office/service/)
(cd apps/backend && go test -tags fts5 -count=1 -run 'TestMarkAgentPausedFixed|TestMarkAgentRunFailedFixed' ./internal/office/service/)
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/office/repository/sqlite/agents.go`
- `apps/backend/internal/office/repository/sqlite/agents_test.go`
- `apps/backend/internal/office/service/failure.go`
- `apps/backend/internal/office/service/pause_reason_cas_internal_test.go`

## Dependencies

None.

## Risks

- The abort branch is only reachable when the agent is still `paused` with
  a changed reason; a concurrent transition to `stopped` or `idle` takes the
  "no longer auto-paused, treat as done" branch instead, which this task
  does not change.
- `clearAutoPauseAttempt`'s two code paths (`unpauseAgentIfCurrent` for a
  `paused` agent, `clearPauseReasonIfCurrent` for anything else) only the
  first is exercised by the abort scenario, since a currently-`paused` agent
  is the precondition for `MarkAgentPausedFixed`'s auto-pause branch.

## Parallelism

`parallel-safe`

Touches only `internal/office/repository/sqlite/agents.go` and
`internal/office/service/failure.go`; disjoint from any other in-flight
Office work order.

## Inputs

- `docs/specs/office/requirements/runtime.md`, `REQ-OFFICE-RUNTIME-001`
  (`AC-OFFICE-RUNTIME-001.9`, `.10`).
- `docs/specs/office/system-design/runtime-01.md`, "Recovery hooks > Mark
  fixed = dismiss + retry" and "Failure modes".
- `docs/specs/office/system-design/runtime-02.md`, "Migrated source detail >
  Scenarios".
- `apps/backend/internal/office/service/failure.go`,
  `MarkAgentPausedFixed` / `clearAutoPause` / `clearAutoPauseAttempt`.
- `apps/backend/internal/office/repository/sqlite/agents.go`,
  `UnpauseAgentIfCurrent`.

## Results

Implemented in commits `9d1f1975e` (CAS unpause write on `pause_reason`,
plus `TestUnpauseAgentIfCurrent_RefusesWhenPauseReasonChanged`) and
`4bcdbef71` (retry-abort guard and dismiss-after-clear reorder) on PR
[#3734](https://github.com/kdlbs/kandev/pull/3734). The retry-abort branch
shipped in `4bcdbef71` without a dedicated regression test; this task adds
`TestClearAutoPause_AbortsWhenNewerAutoPauseLands` (verified to fail against
the pre-guard code) and its matching-reason control
`TestClearAutoPause_SucceedsWhenPauseReasonStillMatches`, both driving the
unexported `clearAutoPause` directly from an internal-package test against a
minimal in-memory repo, since the exported `MarkAgentPausedFixed` always
re-reads the agent fresh and cannot reproduce the read-then-CAS window from
the outside.
