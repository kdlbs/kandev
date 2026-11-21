---
id: "03-blocker-restart-proof"
title: "Prove blocker replay after restart"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-BLOCKER-POLICY-002
acceptance_criteria:
  - AC-OFFICE-BLOCKER-POLICY-002.4
  - AC-OFFICE-BLOCKER-POLICY-002.6
  - AC-OFFICE-BLOCKER-POLICY-002.8
system_design:
  - ../../specs/office/system-design/blocker-terminal-policy.md
---

# Task 03: Prove blocker replay after restart

## Summary

Prove the existing blocker-alert durability contract with fresh services over a closed and reopened database.

## In scope

- Add an owned on-disk fixture with real repositories, publisher, pause service, dispatcher, and recovery path.
- Exercise pause/restart/resume and pause release without replay followed by restart/recovery.
- Assert durable intent, one eventual effect, repeated-replay idempotency, stale discard, and retained context.
- Reuse existing occurrence, merge, prompt, and subscriber tests.

## Out of scope

Unrelated behavior changes, generic verification passes, and automatic publication.

## Acceptance

1. TestOfficeBlockerFailureRestartReplay closes the original store and reconstructs services without reseeding pending work.
2. TestOfficeBlockerFailureRestartStaleDiscard emits no alert after blocker recovery. A positive control proves delivery remains reachable.
3. Resume-hook and missed-hook recovery retain the occurrence and deliver at most once. Existing merge and prompt regressions remain green.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/office/service ./internal/office/wakeup -run 'TestOfficeBlockerFailure|TestBuildPrompt_(BlockerFailed|IncludesEveryCoalescedBlockerFailure)|TestDispatch_BlockedByPause' -count=1)
(cd apps/backend && go test -trimpath ./internal/backendapp -run '^TestOfficeActivationRegistersBlockerSubscribers$' -count=1)
bash scripts/lint-go-changed apps/backend/internal/office/service/blocker_failure_restart_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/office/service/blocker_failure_restart_test.go (new)`
- `apps/backend/internal/office/service/blocker_failure_escalation_test.go (fixture patterns)`
- `apps/backend/internal/office/service/assignment_blocker_admission_test.go (fixture patterns)`

## Dependencies

None. Execute after the prompt work by default, as listed in the plan.

## Risks

The existing fixture is in-memory. Recovery over its original store cannot establish restart survival. Drain owned buses and services before database close.

## Parallelism

`sequential`

## Inputs

Read the Office blocker requirement/design, QueueBlockerFailureWakeup, paused_assignment_replay.go, scheduler_recovery.go, and the escalation fixture.

## Execution notes

This requested contract coverage can pass against current code.
Demonstrate test value with a temporary mutation that drops queued intent or omits replay, then restore the source.
Do not add a permanent production test hook or manufacture a failure by changing the contract.

## Results

Completed. Added an on-disk Office fixture that reconstructs repositories, event
subscribers, dispatch, pause handling, and replay services after closing the
original store. Both resume-hook and missed-hook recovery retain the original
failure occurrence, deliver exactly one run after repeated replay, and mark the
wakeup claimed. Recovery after the blocker returns to an active state marks the
durable request skipped without delivery.

The required mutation check temporarily removed the recovery-tick replay call.
`TestOfficeBlockerFailureRestartReplay/missed_resume_hook_recovery_tick` then
failed because no run was delivered. The production source was restored and
verified byte-for-byte against its original content.

Verification passed:

- `(cd apps/backend && go test -trimpath -race ./internal/office/service ./internal/office/wakeup -run 'TestOfficeBlockerFailure|TestBuildPrompt_(BlockerFailed|IncludesEveryCoalescedBlockerFailure)|TestDispatch_BlockedByPause' -count=1)`
- `(cd apps/backend && go test -trimpath ./internal/backendapp -run '^TestOfficeActivationRegistersBlockerSubscribers$' -count=1)`
- `bash scripts/lint-go-changed apps/backend/internal/office/service/blocker_failure_restart_test.go`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
