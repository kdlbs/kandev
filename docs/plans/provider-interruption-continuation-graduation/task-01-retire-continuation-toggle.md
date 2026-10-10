---
id: "01-retire-continuation-toggle"
title: "Retire the continuation toggle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
  - REQ-PLATFORM-TURN-CONTINUITY-003
acceptance_criteria:
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.6
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.7
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.6
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.2
  - AC-PLATFORM-TURN-CONTINUITY-003.1
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
---

# Task 01: Retire the continuation toggle

## Summary

Make supported continuation unconditional across the backend and agentctl,
including standalone startup. Remove the public feature identity and propagated
booleans while preserving native capability and prompt safety as the admission
boundary.

## In scope

- Use `/tdd` to add `TestCursorContinuationEvidenceWithoutOptIn` and
  `TestInterruptionContinuationAdmissionWithoutOptIn` against default fixtures
  before production edits. Record the expected red failures for absent evidence
  and refused admission.
- Replace `TestContinuationConfigFlagContract` with
  `TestInterruptionContinuationGraduatedFlagContract`. Verify retired identity,
  active registry/features omission, actual config loading with stale env/YAML,
  and stale false override handling through the real runtime flag store/service.
- Remove the field from every startup/config layer listed in the plan. Replace
  flag-propagation tests with managed/standalone no-opt-in behavior and ignored
  legacy fields. Do not retain an always-true configuration field.
- Remove flag conditions from ACP outcome/permission tracking, safety poisoning,
  continuation admission/refusal, and final owner validation. Preserve all
  capability, loading, generation, queue, and policy distinctions.
- Convert disabled-only test cases into support/evidence negatives or remove
  obsolete toggle assertions while retaining the permanent safety matrix.
- Remove the frontend default property; update feature-contract tests for omission
  and exact backend shape. Historical disabled failure records remain displayable.

## Out of scope

No live runtime mutation, new flag, schema change, provider expansion, altered
retry schedule, relaxation for cancelled tools, or historical redispatch.

## Acceptance

- Default-config supported Cursor evidence and continuation admission pass after
  failing on the original gated code; hidden payload, native identity, and replay
  fence remain unchanged.
- Active registry, profile/default, config/startup, and frontend feature contracts
  omit the live flag; its retired identity is reserved and stale values are inert.
- Existing uncertainty, version-skew, permissions, cancellation, human-priority,
  retained-runtime, capacity, and restart regressions pass.

## ASCII UI preview

UI-01 from [the package preview](plan.md#ascii-ui-preview), `002.6`:

```text
Settings > System > Feature Toggles (desktop and phone)
  Other active feature toggles
  [No Interrupted conversation continuation row]
```

The registry controls omission; keep the existing settings composition. Task 02
proves rendered omission and unchanged Chat recovery controls on both viewports.

## Verification

Run from repository root. Install workspace dependencies first only for a fresh
worktree. The first two commands capture the red phase; their failures before
gate removal and green results after implementation are recorded below.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestCursorContinuationEvidenceWithoutOptIn$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestInterruptionContinuationAdmissionWithoutOptIn$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/runtimeflags ./internal/common/config ./internal/profiles ./internal/agentctl/server/config ./internal/agentctl/server/adapter/transport/acp -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agentctl/server/process ./internal/agentctl/server/adapter ./internal/agentctl/server/adapter/transport/shared ./internal/backendapp -run 'Continuation|RuntimeFlag|Feature' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'InterruptionContinuation|Continuation|TransientFailure|RetainedRuntime|ReplaySafety|Capacity' -count=1)
(cd apps/web && pnpm exec vitest run lib/state/slices/features/features-contract.test.ts components/task/chat/messages/interruption-recovery-feedback.test.ts)
make -C apps/backend lint
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
git diff --check
```

Run each command once at the documented working directory after final edits.
Record actual failures or unavailable checks, and never infer product behavior
from a compile-only or skipped test.

## Files likely touched

- `profiles.yaml` / `apps/backend/internal/profiles/profiles.yaml`
- `apps/backend/internal/common/config/config.go`, `agentctl.go`, and their tests
- `apps/backend/internal/runtimeflags/registry.go`, `provider_continuation_test.go`, `registry_test.go`
- `apps/backend/internal/profiles/profiles_test.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/orchestrator/service.go`, `provider_interruption_continuation.go`, `provider_interruption_dispatch.go`
- `apps/backend/internal/orchestrator/provider_interruption_continuation_test.go`, `provider_interruption_refusal_test.go`, `provider_interruption_capacity_test.go`
- `apps/backend/internal/agentctl/server/config/config.go`, `continuation_config_test.go`
- `apps/backend/internal/agentctl/server/process/manager.go`, `continuation_config_test.go`
- `apps/backend/internal/agentctl/server/adapter/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/shared/config.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_continuation.go`, `adapter_continuation_permissions.go`
- ACP Cursor/mock continuation and permission test files adjacent to those sources
- `apps/web/lib/state/slices/features/types.ts`, `features-contract.test.ts`

## Dependencies

None. Read the linked requirements/designs and graduation ADR. Reuse the existing
retired-identity infrastructure and Office-session-identity graduation test pattern.

## Risks

Unsupported transports must not gain authority from unconditional evidence
collection. Default fixtures can expose previously hidden uncertain outcomes;
preserve the safety contract rather than forcing tests to attest success.

## Parallelism

`sequential`

## Inputs

- [Interruption requirements](../../specs/platform/requirements/provider-interruption-continuation.md)
- [Interruption design](../../specs/platform/system-design/provider-interruption-continuation.md)
- [Capacity design](../../specs/platform/system-design/transient-turn-runtime-continuity.md)
- [Graduation ADR](../../decisions/2026-10-08-unconditional-interruption-continuation.md)
- `/runtime-feature-flags`, `/tdd`, scoped backend/agentctl/web guidance

## Results

Completed 2026-10-08.

- `TestCursorContinuationEvidenceWithoutOptIn` and `TestInterruptionContinuationAdmissionWithoutOptIn` failed before gate removal and passed after.
- Runtime-flag, common-config, profile, agentctl-config, and full ACP package tests passed.
- Race tests for agentctl process/adapter/shared config/backend composition and orchestrator/lifecycle continuation passed.
- Frontend feature-contract/recovery-feedback tests passed (2 files, 14 tests); backend lint, web typecheck, web lint, and `git diff --check` passed.
