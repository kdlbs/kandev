---
id: "01-replaced-session-reclaim"
title: "Stop settled replaced session runtimes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PRIMARY-SESSION-OWNERSHIP-001
acceptance_criteria:
  - AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.6
system_design:
  - ../../specs/tasks/system-design/primary-session-ownership.md
---

# Task 01: Stop settled replaced session runtimes

## Summary

With `features.replacedSessionReclaim` on, stop the execution a session held
when the user promoted another session over it, once that session settles.

## In scope

- `PromotePrimarySession`, `registerReplacedSessionReclaim`,
  `reclaimReplacedSessionsOnce`, `stopReplacedSessionIfSettled`, and
  `replacedSessionReclaimEligibility` in
  `orchestrator/replaced_session_reclaim.go`; the `session.set_primary`
  handler calls `PromotePrimarySession`.
- `SetPrimarySession` only takes `primarySessionReclaimMu` around its write and
  never registers; the idle-reaper tick performs every stop.
- Runtime flag `features.replacedSessionReclaim` /
  `KANDEV_FEATURES_REPLACED_SESSION_RECLAIM`, off in prod, dev, and e2e.

## Out of scope

Automatic promotion, session deletion, workflow profile switches and their
queue transfer, idle suspension policy, Office and coordinator lifecycle, and
queue routing.

## Verification

- `go test ./internal/orchestrator/ -run 'TestPrimaryReplacement|TestAutomaticPrimaryPromotionDoesNotReclaim|TestQueueTransferDoesNotReclaimDonor|TestProfileSwitchParkWithReclaimOnParksSource' -count=1`
- `go test ./internal/runtimeflags/ ./internal/profiles/ -count=1`
