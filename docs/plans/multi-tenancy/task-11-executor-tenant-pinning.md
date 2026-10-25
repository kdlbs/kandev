---
id: "11-executor-tenant-pinning"
title: "Executor tenant pinning"
status: todo
wave: 4
depends_on: ["10-per-org-agent-credentials"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 11: Executor Tenant Pinning

## Acceptance

- `executors_running.org_id` is written on every execution and checked on
  recovery. An execution row belonging to another org is not adopted.
- Docker executions are labelled `kandev.org=<org_id>`, and container names and
  volume names are namespaced by org. A container whose label does not match the
  recovering org is not adopted, not stopped, and not removed; it is logged.
- Remote and SSH executors are org-owned rows; two orgs cannot both reach one
  remote host through Kandev. The remote task directory and prepared checkout
  live under the org's remote root.
- `local_pc` executions fail closed: with more than one active org and no
  `org_os_users` row for the launching org, the launch is refused **before any
  process starts**, with an error naming the missing row and the escape-hatch
  flag. The guard is the first statement in the launch path, proven by a
  nil-dependency test that fails on a panic if the guard is placed later.
- With `features.multiTenancyTrustedStandalone` set, the launch proceeds using
  the org's redirected `HOME` and logs a warning once per org per boot.
- With a per-org OS user configured, the standalone process runs as that user
  and its working tree is not readable by another org's user.

## Verification

- `go test -race ./internal/agent/runtime/lifecycle/... ./internal/agent/docker/... ./internal/agent/executor/...`
- `go test ./internal/... -run 'TestStandaloneFailsClosed|TestForeignContainerNotAdopted|TestGuardBeforeDependencies'`
- `KANDEV_E2E_CONTAINERS=1` container project for the Docker label path.

## Files Likely Touched

- `apps/backend/internal/agent/runtime/lifecycle/{manager,executor_backend,execution_store,process_runner}.go`
- `apps/backend/internal/agent/docker/`
- `apps/backend/internal/ssh/`, remote executor preparation
- `apps/backend/internal/task/repository/sqlite/` (`executors_running.org_id`)

## Inputs

- Spec: What (runtime isolation), Failure modes (standalone refusal, foreign
  container), Scenarios.
- Patterns: ADR 0003 and ADR 0025 (`executors_running` as the source of truth
  for runtime cleanup); the lifecycle-callback identity rule and the
  `TestSessionKeyedEntryPointsGuardBeforeDependencies` shape in
  `apps/backend/AGENTS.md`.

## Output Contract

Report the recovery non-adoption test, the standalone fail-closed guard
position proof, the container-label E2E result, RED/GREEN commands, and set
this task plus its plan checkbox to done.
