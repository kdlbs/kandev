---
id: "01-honest-inactivity-clock"
title: "Measure prompt progress, not traffic"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-stall-recovery.md"
system_design: "../../specs/agents/system-design/agent-stall-recovery.md"
requirements:
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
acceptance_criteria:
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.9
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.2
---

# Task 01: Measure prompt progress, not traffic

## Outcome

The stall clock advances only on prompt dispatch, a turn event, the prompt's
terminal completion, or new user input. A metadata frame no longer restarts it,
so a never-started prompt trips the watchdog five minutes after dispatch
regardless of how much metadata the adapter emits.

## In scope

- Stop `recordActivity` from advancing the activity timestamp for events outside
  `turnContentEventTypes`. The existing `agentEventSincePrompt` and
  `promptActivityEpoch` gating is already correct and is not changed.
- Keep every other effect of `recordActivity` unchanged: `firstActivityOnce`
  still fires on the first event of any type, and the wakeup-driven
  `Ready → Running` flip keeps its current turn-content gate.
- Update the `lastActivityAt` doc comment in `types.go` to state what advances
  it, so the field's contract is readable at its declaration.

## Exclusions

- No change to the five-minute threshold, the one-minute tick, the payload
  shape, or the once-per-generation rule.
- No change to `armPromptActivity`, `markAgentActivity`, or
  `recordSteerActivity`; all three already advance the clock and must keep
  doing so.
- No new field and no second clock. The completion-signal watchdog keeps reading
  the same snapshot.

## Acceptance conditions

1. A prompt that emits only `usage_update`, `context_window`,
   `available_commands_update`, and `session_info_update` frames after dispatch
   publishes exactly one stall event five minutes after dispatch, with
   `NeverStarted` true.
2. A prompt that emits a turn event and then only metadata frames reports
   `NeverStarted` false, and its clock runs from the turn event.
3. `GetPromptActivityForSession` returns the same corrected timestamp, so the
   completion-signal watchdog is not fooled by metadata frames either.

## Verification

```sh
cd apps/backend && go test ./internal/agent/runtime/lifecycle/... -count=1
cd apps/backend && go test ./internal/orchestrator/... -count=1
```

New regressions in
`apps/backend/internal/agent/runtime/lifecycle/session_test.go`:

- `TestWaitForPromptDone_MetadataOnlyStreamStillReportsNeverStarted` — drives
  metadata frames through `Manager.recordActivity` once per minute inside
  `synctest`, then asserts one `agent.stalled` event with `NeverStarted` true at
  the five-minute mark. It must first fail because no event is published: the
  metadata frames keep resetting the clock.
- `TestRecordActivity_MetadataFrameDoesNotAdvancePromptProgress` — asserts the
  snapshot timestamp directly for a metadata frame and for a turn event.

These four existing regressions must keep passing unchanged:
`TestWaitForPromptDone_PublishesSingleStall`,
`TestWaitForPromptDone_StallPayloadDiscriminatesNeverStarted`,
`TestWaitForPromptDone_StallPayloadReportsRunningAfterAgentEvent`,
`TestMarkPromptDispatchedArmsActivityAfterDispatch`.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events_test.go`

## Dependencies

None.

## Results

`recordActivity` (`manager_events.go`) now gates the `lastActivityAt` write
itself on `turnContentEventTypes`, instead of writing it unconditionally and
gating only `agentEventSincePrompt`/`promptActivityEpoch`. `firstActivityOnce`
and the wakeup-driven `Ready → Running` flip are unchanged. Updated the
`lastActivityAt` doc comment in `types.go` to state what advances it.

Added `apps/backend/internal/agent/runtime/lifecycle/stall_activity_test.go`
(new file — the two existing test files are already over the 800-line test
file convention limit) with both specified regressions:
- `TestWaitForPromptDone_MetadataOnlyStreamStillReportsNeverStarted`
- `TestRecordActivity_MetadataFrameDoesNotAdvancePromptProgress`

Both failed first for the stated reason (metadata frames kept resetting the
clock), then passed after the fix.

Also corrected a pre-existing test that pinned the defect and wasn't listed
among the "must keep passing unchanged" set:
`TestRecordActivity_MetadataDoesNotMarkPromptStarted`
(`manager_events_test.go`) asserted that a metadata event refreshes
`lastActivityAt` to "now" — exactly Defect 1. Updated its assertion to expect
the timestamp to stay unchanged; its other two assertions
(`agentEventSincePrompt` stays false, epoch stays 0) were already correct and
untouched.

The four regressions named in this work order as must-keep-passing
(`TestWaitForPromptDone_PublishesSingleStall`,
`TestWaitForPromptDone_StallPayloadDiscriminatesNeverStarted`,
`TestWaitForPromptDone_StallPayloadReportsRunningAfterAgentEvent`,
`TestMarkPromptDispatchedArmsActivityAfterDispatch`) pass unchanged.

Verification:
- `go test ./internal/agent/runtime/lifecycle/... -count=1` — passes except 7
  tests that are pre-existing, environment-caused failures unrelated to this
  change (confirmed by running the same tests against unmodified HEAD via a
  scoped `git stash`): 6 `TestWorktreePreparer_*` tests fail on worktree/git
  state in this sandbox, and
  `TestBuildAuthMethodsIdentityAgentOverridesEnvironment` fails on a Unix
  socket path length limit from this checkout's long temp-dir path. Neither
  touches `recordActivity`, `lastActivityAt`, or stall detection.
- `go test ./internal/orchestrator/... -count=1` — all packages pass.
