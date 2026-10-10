---
id: "01-contracts-and-rollout"
title: "Normalized contract and rollout gate"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-001
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-001.1
  - AC-AGENTS-BACKGROUND-WORK-001.2
  - AC-AGENTS-BACKGROUND-WORK-001.3
  - AC-AGENTS-BACKGROUND-WORK-001.4
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 01: Normalized contract and rollout gate

## Summary

Define the shared workload/run DTOs, typed observations and optional adapter/runtime capability interfaces. Add the independent rollout gate without changing existing adapter requirements or admission semantics.

## In scope

- Shared Go contracts in streams and runtime, matching additive TS DTOs, typed capability absence and control errors. Add a provider-neutral conformance fixture; it is not a new production protocol.
- New `features.agentBackgroundWork` / `KANDEV_FEATURES_AGENT_BACKGROUND_WORK`, restart-required and off in all three profiles. Use the current registry read/apply closures and explicit config tags; sync the embedded profile through the existing mechanism.
- Gate the producer/service composition and all subsequently introduced read/control/output entry paths through one server-owned policy. Preserve existing inline messages and attestation when off.
- Initial conformance tests for no optional interface, omitted/new fields, unknown kind/action, and connection-scoped capabilities. Confirm existing Codex and Claude handoff gates remain independent.

## Out of scope

New native providers, generic scheduling, host PID control, and production changes owned by later work orders. Preserve the design's explicit exclusions.

## Acceptance

1. `TestBackgroundWorkContractCompatibility` exercises a synthetic provider without provider-name switches, and old adapters still compile unchanged.
2. `TestBackgroundWorkFeatureDisabled` proves no provider calls or new projection writes, while existing background accounting and messages continue.
3. Registry/profile/frontend-contract tests establish matching names and all-off defaults; no unverified upstream Codex feature keys are injected.

## TDD and verification

Write the named behavioral tests first and observe the relevant failure before implementation. Proposed file/test names below are implementation targets, not existing coverage. Run from repository root; fresh worktrees first install with `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test ./internal/agentctl/types/streams ./internal/agentctl/server/adapter ./internal/agent/runtime ./internal/runtimeflags ./internal/common/config ./internal/profiles)
(cd apps/web && pnpm test -- lib/state/slices/features/features-contract.test.ts)
(cd apps/web && pnpm run typecheck)
git diff --check
```

## Files likely touched

- Existing: `apps/backend/internal/agentctl/types/streams/background_work.go`, `agent.go`; `internal/agentctl/server/adapter/adapter.go`; `internal/agent/runtime/runtime.go`.
- Proposed: `apps/backend/internal/agentctl/types/streams/background_work_contract.go` and `_test.go`; `apps/backend/internal/agent/runtime/background_work.go` and `_test.go`; adapter conformance test helpers.
- Existing config, runtimeflags registry, root/embedded profiles, backend composition; web feature defaults and `apps/web/lib/types/session-events.ts`.
- Proposed: `apps/web/lib/types/background-work.ts`.

## Dependencies

None.

## Risks

A mandatory adapter method would break unrelated protocols; a frontend-only flag would leave controls reachable.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/background-work.md), criteria listed in frontmatter.
- [System design](../../specs/agents/system-design/background-work.md), corresponding mapped sections.
- [ADR](../../decisions/2026-09-27-provider-neutral-background-work.md).
- [Plan baseline and existing test patterns](plan.md#baseline-and-integration-points).

## Results

Pending.
