---
id: "06-repository-intent"
title: "Repository intent and provider-prepared workspace (R7)"
status: pending
wave: 4
depends_on:
  - "05"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-003
acceptance_criteria:
  - AC-EXECUTORS-NATIVE-WORKER-003.1
  - AC-EXECUTORS-NATIVE-WORKER-003.2
  - AC-EXECUTORS-NATIVE-WORKER-003.3
  - AC-EXECUTORS-NATIVE-WORKER-003.4
  - AC-EXECUTORS-NATIVE-WORKER-003.5
  - AC-EXECUTORS-NATIVE-WORKER-003.6
  - AC-EXECUTORS-NATIVE-WORKER-003.7
  - AC-EXECUTORS-NATIVE-WORKER-003.8
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
---

# Task 06: Repository intent and provider-prepared workspace (R7)

## Summary

Carry repository intent and filtered agent environment in the provision request and verify provider-returned repository inventory before the host publishes.

## Size

M: ~300 source, ~500 test lines.

## In scope

- Carry repository intent and filtered agent environment in the provision request and verify provider-returned repository inventory before the host publishes.

## Out of scope

- Per-node mirror; managed credentials with provider preparation.

## Acceptance

- Intent and agent_env reach the provider with no credential of any class.
- Inventory mismatch cleans up; core mode is unchanged.

## Failing-first tests

- Seeded secrets of every class never reach agent_env or intent (fails: field absent).
- Digest includes intent; changed intent under one operation ID conflicts.
- Inventory with missing, extra, duplicate, or out-of-workspace repository: cleanup with named reason.
- Managed credentials plus provider preparation rejected before allocation.
- Empty repository list is valid.
- Ordering by position then repository ID.

## Verification

Run from the repository root:

```bash
go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1
```

## Likely files and risks

- `plugin.proto`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin_validation.go`
- Risk: concurrency on both dialects; add the PostgreSQL race run before marking done.
