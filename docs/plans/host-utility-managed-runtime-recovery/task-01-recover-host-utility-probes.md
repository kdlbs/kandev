---
id: "01-recover-host-utility-probes"
title: "Recover host utility probes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.8
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.5
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 01: Recover host utility probes

## Summary

Add a safe structured npm-resolution failure signal to capability probes. Use
it to perform one exact cache repair and online retry inside the host utility
manager before capability failure is published.

## In scope

- Add the failing host utility regression test first.
- Classify strict top-level package `ETARGET` evidence inside agentctl.
- Keep raw stderr out of the probe response.
- Route repair through the failed probe's warm agentctl client.
- Rebuild the retry command from the registered managed-runtime spec and effective version.
- Preserve one-retry, cancellation, shutdown, registry, package, and version invariants.
- Prove profile reconciliation does not mutate persisted profile selection fields on probe failure.

## Out of scope

- Frontend changes or tooltip behavior.
- Automatic rollback or another package version.
- Cache repair for unrelated or transitive npm failures.
- Capability catalogue persistence.

## Acceptance

- A matching host probe failure performs one scoped repair and one online retry.
- A mismatch, unrelated failure, cancellation, or second failure does not start another replacement probe.
- Success publishes recovered capabilities; failure preserves profile selection data and publishes the final error.

## Verification

```bash
go test ./internal/agentctl/server/utility ./internal/agent/hostutility ./internal/agent/settings/controller -count=1
```

## Files likely touched

- `apps/backend/internal/agentctl/server/utility/types.go`
- `apps/backend/internal/agentctl/server/utility/acp_executor.go`
- `apps/backend/internal/agentctl/server/utility/acp_executor_test.go`
- `apps/backend/internal/common/npmresolution/matcher.go`
- `apps/backend/internal/agent/runtime/routingerr/npm_resolution.go`
- `apps/backend/internal/agent/hostutility/manager.go`
- `apps/backend/internal/agent/hostutility/manager_test.go`
- `apps/backend/internal/agent/settings/controller/reconciler_test.go`

## Dependencies

None.

## Risks

- Probe commands can contain trusted prefixes and ACP arguments, so package extraction must not accept arbitrary request data.
- Host utility bootstrap runs concurrently across agents; retry state must remain per probe operation.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001`
- `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002`
- Host capability-probe flow in the system design.
- Existing lifecycle `ETARGET` classifier and agentctl cache-repair patterns.

## Results

- Added a stable probe failure code for strict top-level npm `ETARGET` evidence while keeping raw stderr private.
- Added one exact cache repair and online-preferred retry on the same warm agentctl instance.
- Added coverage for recovery success, retry bounds, cancellation, untrusted commands, and persisted profile preservation.
- Passed `go test ./internal/agentctl/server/utility ./internal/agent/hostutility ./internal/agent/settings/controller -count=1`.
- Passed `go test ./internal/agent/runtime/routingerr ./internal/common/npmresolution -count=1`.
