---
id: "02-hold-restricted-profiles"
title: "Hold restricted profiles"
status: done
wave: 2
depends_on:
  - "01-classify-restricted-accounts"
plan: "plan.md"
requirements:
  - REQ-AGENTS-ACCOUNT-BLOCK-001
acceptance_criteria:
  - AC-AGENTS-ACCOUNT-BLOCK-001.4
  - AC-AGENTS-ACCOUNT-BLOCK-001.5
  - AC-AGENTS-ACCOUNT-BLOCK-001.6
system_design:
  - ../../specs/agents/system-design/account-blocked-classification.md
---

# Task 02: Hold restricted profiles

## Summary

Hold the failed execution profile in the circuit registry with no expiry and no
probe, skip it during selection, and report the failed profile from the
orchestrator.

## In scope

- `Hold`, `ReleaseHold`, and `IsHeld` on the circuit registry.
- Engine selection, pending-retry resume, failure handling, and resolver
  methods.
- Orchestrator hold for every dynamic-route failure, before the checks that
  decide whether the failure may route.

## Out of scope

The settings release route and any change to candidate order.

## Acceptance

`AC-AGENTS-ACCOUNT-BLOCK-001.4` through `001.6`.

## Verification

```bash
cd apps/backend
go test -trimpath ./internal/agent/runtime/dynamic -count=1
go test -trimpath ./internal/agent/runtime -run AccountBlocked -count=1
go test -trimpath ./internal/orchestrator -run AccountBlocked -count=1
```

## Results

- RED: the orchestrator test failed with the hold call removed.
- GREEN: `dynamic`, `agent/runtime`, and the orchestrator `AccountBlocked`
  tests passed, also after rebuilding the branch on the current `main`.
- Review fixes, each failing first and then passing:
  - a repeated hold retries a hold save that failed, so the hold survives a
    restart;
  - a pending retry, scheduled or manual, refuses a profile held after the
    route started waiting and returns the route to manual recovery;
  - a predecessor still serving the session holds its own profile although
    its failure cannot advance the route.
- The orchestrator test for a restriction before any output holds the profile
  and moves the route to the next candidate.
- Review remediation adds refusal at existing-route resolution and the
  downstream launch/resume boundary, including a hold established after
  selection. Release preserves the retained route attribution. Exact-source
  contained proofs reproduced each bypass before the fix; genuine package
  and integration results are recorded in the PR's hosted CI evidence.
