---
id: "13-launch-policy"
title: "Launch-wrapper and sandbox requirement (M5)"
status: pending
wave: 8
depends_on:
  - "12"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-LAUNCHPOLICY-001
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-LAUNCHPOLICY-001.1
  - AC-EXECUTORS-PLUGIN-LAUNCHPOLICY-001.2
  - AC-EXECUTORS-PLUGIN-LAUNCHPOLICY-001.3
  - AC-EXECUTORS-PLUGIN-LAUNCHPOLICY-001.4
  - AC-EXECUTORS-PLUGIN-LAUNCHPOLICY-001.5
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-operations-migration.md
---

# Task 13: Launch-wrapper and sandbox requirement (M5)

## Summary

Add the profile `launch_policy`, validation before allocation, the applied policy on the descriptor, and fail-closed enforcement in agentctl.

## Size

M: ~350 source, ~600 test lines. Ports the node-policy files from the read-only native reference.

## In scope

- Profile field and validation; descriptor `applied_launch_policy`; bootstrap fields.
- Agentctl refusal for `required` without a wrapper (ported test-first from the native branch node-policy files).

## Out of scope

- Wrapper discovery and placement (provider-side).

## Acceptance

- `required` never starts an unwrapped process; a weaker applied policy is rejected before ready.

## Failing-first tests

- Agentctl process test: `required` with no wrapper refuses and reports; `preferred` starts unwrapped and reports it.
- Unsupported policy rejected before allocation; weaker descriptor rejected with cleanup.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/common/launchwrapper ./internal/agentctl/server/process -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

## Likely files and risks

- `apps/backend/internal/common/launchwrapper`
- `apps/backend/internal/agentctl/server/process`
- Risk: concurrency on both dialects; add the PostgreSQL race run (`KANDEV_TEST_POSTGRES_DSN`) before marking done.
