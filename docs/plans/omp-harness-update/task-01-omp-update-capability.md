---
id: "01-omp-update-capability"
title: "Add OMP self-update capability"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-003
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-003.9
  - AC-AGENTS-RUNTIME-UPDATES-003.10
system_design:
  - ../../specs/agents/system-design/harness-self-update.md
---

# Task 01: Add OMP self-update capability

## Summary

Declare an optional built-in agent capability for trusted harness-owned updates. Implement it for OMP with the package used for stable release metadata and the direct argv `omp update`, without changing any OMP launch or installation surface.

## In scope

- Add the agent capability/spec types.
- Implement and test OMP's metadata package and trusted update argv.
- Pin unchanged launch, runtime, inference, passthrough, and install-script behavior in the agent contract test.

## Out of scope

- Controller update jobs, status, HTTP DTOs, or UI.
- Adding OMP to the managed npm catalogue or changing its `InstallScript`.

## Acceptance

- `OmpACP` implements the new self-update capability with package `@oh-my-pi/pi-coding-agent` and argv `omp update`.
- Tests prove OMP's launch/runtime/inference/passthrough commands and `bun install -g @oh-my-pi/pi-coding-agent` install script remain unchanged.
- The capability contract exposes no request-derived command construction.

## Verification

```bash
cd apps/backend
CGO_ENABLED=1 go test -tags fts5 ./internal/agent/agents
```

## Files likely touched

- `apps/backend/internal/agent/agents/agent.go`
- `apps/backend/internal/agent/agents/harness_update.go`
- `apps/backend/internal/agent/agents/omp_acp.go`
- `apps/backend/internal/agent/agents/harness_update_test.go`
- `apps/backend/internal/agent/agents/new_acp_agents_test.go`

## Dependencies

None.

## Risks

- The new capability must remain separate from `ManagedNPMRuntimeAgent`; otherwise lifecycle and host utility callers may silently switch OMP to npx commands.

## Parallelism

`sequential`

## Inputs

- Requirement AC-AGENTS-RUNTIME-UPDATES-003.9 and .10.
- [Harness self-update design](../../specs/agents/system-design/harness-self-update.md), sections Data and contracts and Security.
- Existing OMP command contract in `apps/backend/internal/agent/agents/new_acp_agents_test.go` and the other optional capability interfaces in `agent.go`.

## Results

Implemented the separate OMP `HarnessUpdateAgent` capability with trusted package and argv. RED: `TestOmpHarnessUpdateContract` failed because OMP had no capability; GREEN: focused test passed. Verified `TMPDIR=/home/clem/.kandev/tasks/omp-test-tmp CGO_ENABLED=1 GOCACHE="$PWD/.go-build-cache" go test -tags fts5 ./internal/agent/agents` from `apps/backend` (pass). `TMPDIR` avoids an unrelated pre-existing `/tmp/package.json` interfering with the npm fixture.
