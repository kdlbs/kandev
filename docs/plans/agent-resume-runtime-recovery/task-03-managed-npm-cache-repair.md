---
id: "03-managed-npm-cache-repair"
title: "Repair corrupt managed npm execution caches"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-resume-runtime-recovery.md"
---

# Task 03: Repair corrupt managed npm execution caches

## Acceptance

- An explicit managed-runtime update that fails once invalidates only the
  selected built-in package's deterministic `_npx` execution directory and
  retries exactly once.
- Cache-root and target validation prevent caller-controlled or broad
  deletion; no global npm cache clean is executed.
- A repaired update is successful only after the existing ACP refresh succeeds,
  while repair or retry failure is terminal and visible.

## Verification

- `cd apps/backend && go test ./internal/agent/agents -run 'TestManagedNPMRuntime'`
- `cd apps/backend && go test ./internal/agent/settings/controller -run 'TestAgentUpdate'`

The update-job regression must first fail because no repair/retry call occurs,
then pass with the exact call order `update -> repair -> update -> refresh`.

## Files likely touched

- `apps/backend/internal/agent/agents/managed_npm_runtime.go`
- `apps/backend/internal/agent/agents/managed_npm_runtime_test.go`
- `apps/backend/internal/agent/settings/controller/agent_update.go`
- `apps/backend/internal/agent/settings/controller/agent_update_job.go`
- `apps/backend/internal/agent/settings/controller/agent_update_test.go`
- `docs/public/agents-and-profiles.md`

## Dependencies

None.

## Parallelism

`parallel-safe` relative to Tasks 01 and 02: it touches disjoint agent settings
and runtime files, with no shared schema, migration, generated contract,
lockfile, or package configuration.

## Inputs

- Repair spec final two regression scenarios and cache safety constraints
- `docs/specs/agents/requirements/runtime-updates.md` failure mode and corrupt-cache scenario
- Plan section `Repair one managed npm execution cache`
- Existing direct-argv `RuntimeUpdater`, job output ring, and update retry
  patterns

## Output contract

Report the red/green test evidence, npm key derivation, deletion guards, retry
ordering, public-doc change, exact test results, files changed, blockers, and
risk notes. Mark this task `done` and update its plan checkbox in the same
conversation.
