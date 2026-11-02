---
id: "03-probe-recovery"
title: "Recover host and profile capability probes"
status: pending
wave: 3
depends_on:
  - "01-executor-local-preparation"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-005
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.8
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.9
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 03: Recover host and profile capability probes

## Summary

Reuse one bounded retry helper for agent-wide host capabilities, concrete
profile baseline discovery, and dependent model-option resolution. A successful
private preparation becomes the result for exactly the authorized context
that failed.

## In scope

- Stable missing-dependency probe code and one opt-in repair retry using Task 01.
- Existing instance operation admission, single-flight, secret/launch context,
  profile/runtime generations, actual runtime observations, and publication.
- Cancellation of obsolete preparation, preservation of draft/saved selections,
  and final failure propagation without default-context substitution.
- Update the profile probe's no-install comment to state the narrow exception.

## Out of scope

- Changing other probe failure eligibility, general runtime updates, writing
  profile data, agent-wide publication of profile results, and frontend changes.

## Acceptance

1. `TestManagedDependencyHostProbe` first fails because the missing-dependency
   code is not recovered, then publishes a validated catalogue after exactly
   one private retry, preserving the effective selected package and environment.
2. `TestManagedDependencyProfileProbe` and
   `TestManagedDependencyProfileModelOptions` prove both flights preserve
   complete contexts, mode/model/options, private observations, and saved/draft
   values without cross-profile or agent-wide cache pollution.
3. Context changes, shutdown, deadline, permanent errors, prior ETARGET retry,
   unsupported runtime shapes, and old peers prevent stale or extra replacement;
   final preparation/replacement failures retain their real sanitized cause.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agentctl/server/utility -run 'ManagedDependency|ManagedRuntime|ProfileProbe|ProfileCapabilities|ProfileModel|ProfileContext|RuntimeObservation' -count=1)
(cd apps/backend && go test -race ./internal/agent/hostutility -run 'ManagedDependency' -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/hostutility/manager.go`.
- `apps/backend/internal/agent/hostutility/profile_probe.go`.
- `apps/backend/internal/agent/hostutility/managed_runtime_recovery_test.go`.
- New `apps/backend/internal/agent/hostutility/managed_dependency_recovery_test.go`.
- Existing profile probe/context-generation and runtime-observation tests.
- `apps/backend/internal/agentctl/server/utility/acp_executor.go` and `types.go`.
- Profile discovery tests in `apps/backend/internal/agent/settings/controller`.

## Dependencies

Task 01. Task 02 is not a technical prerequisite, but execute this work order
after Task 02 by default to avoid concurrent shared wire/policy changes.

## Risks

- `ProbeProfileCapabilities` currently bypasses the agent-wide retry helper.
- `resolveProfileModelConfigFlight` is a separate provider launch and must not
  remain broken after baseline discovery begins recovering.
- `context.WithoutCancel` must not allow an obsolete repair to publish or start
  a replacement after the launch context changed.

## Parallelism

`sequential`

## Inputs

- Requirement 005; existing profile-mutation prohibition in 001.7.
- Design: Host and profile discovery.
- Linked profile capability discovery design for existing authority and
  generation contracts, without copying that contract into this package.
- Existing `managed_runtime_recovery_test.go` and profile context tests.

## Results

Pending.
