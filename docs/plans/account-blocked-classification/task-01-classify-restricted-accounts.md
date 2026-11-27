---
id: "01-classify-restricted-accounts"
title: "Classify restricted accounts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-ACCOUNT-BLOCK-001
acceptance_criteria:
  - AC-AGENTS-ACCOUNT-BLOCK-001.1
  - AC-AGENTS-ACCOUNT-BLOCK-001.2
  - AC-AGENTS-ACCOUNT-BLOCK-001.3
system_design:
  - ../../specs/agents/system-design/account-blocked-classification.md
---

# Task 01: Classify restricted accounts

## Summary

Add the `account_blocked` code, the anchored gateway rule, its recovery flags,
and the policy rule that forbids same-candidate retry and reset waiting.

## In scope

- `routingerr` rule, code, class, invariants, and `HoldsUntilRelease`.
- `routingpolicy` evaluation for a held code.
- Tests for positive envelopes, prose, plain 403, and status boundaries.

## Out of scope

Holding profiles, release, and other 403 or authentication failures.

## Acceptance

`AC-AGENTS-ACCOUNT-BLOCK-001.1` through `001.3`.

## Verification

```bash
cd apps/backend
go test -trimpath ./internal/agent/runtime/routingpolicy -count=1
go test -trimpath ./internal/agent/runtime/routingerr -run AccountBlocked -count=1
```

## Results

- RED: on `main`, `account_blocked` was undefined and the diagnostic
  classified as `agent_runtime_error`.
- GREEN: `routingerr` and `routingpolicy` passed, also after rebuilding the
  branch on the current `main`.
- `TestRemediateNpxCache_RemovesHashDirAndPreservesParent` in `routingerr`
  also fails on `main` on Windows, because it overrides only `HOME`. It was
  skipped in the local run.
