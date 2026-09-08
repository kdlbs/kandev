---
id: "02-effective-version-selection"
title: "Resolve and persist effective versions"
status: complete
wave: 2
depends_on: ["01-default-pins"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 02: Resolve and persist effective versions

## Acceptance

- Effective version resolution uses a matching validated DB selection first and
  the reviewed default otherwise across host, container, and SSH managed ACP
  commands.
- Runtime preview and job contracts expose default, optional active selection,
  and effective version without changing trusted package identity.
- **Use Kandev default** validates the exact default and deletes the selection
  only after a successful candidate probe; failures retain the prior selection.

## Verification

```bash
cd apps/backend && go test ./internal/agent/managedruntime ./internal/agent/hostutility ./internal/agent/runtime/lifecycle ./internal/agent/settings/controller ./internal/agent/settings/handlers
```

## Files likely touched

- `apps/backend/internal/agent/managedruntime/selection.go`
- `apps/backend/internal/agent/managedruntime/selection_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/hostutility/manager.go`
- `apps/backend/internal/agent/settings/dto/dto.go`
- `apps/backend/internal/agent/settings/controller/agent_update.go`
- `apps/backend/internal/agent/settings/controller/agent_update_job.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- Focused controller, handler, host utility, and lifecycle tests

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Task 04 after Task 01. This task owns runtime selection and
mutation contracts; Task 04 owns automation files.

## Inputs

- Spec: version semantics, activation lifecycle, command routing, persistence
- ADR: install-wide selection precedence and accepted npm boundary
- Existing patterns: `managedruntime.Store`, `PreviewAgentUpdate`, update jobs

## Output contract

Report API changes, persistence behavior, execution surfaces covered, exact
tests and results, blockers/risks, and synchronized task/plan status.

## Results

Complete. Matching install-wide selections now take precedence over reviewed
defaults across host utility, standalone, container, and SSH command builders.
Runtime catalogue, preview, job, and executor contracts expose default,
active, and effective versions. Structural `use_default` previews and jobs
validate and probe the exact default before deleting the stored selection;
failed candidates retain the previous selection.

Verification: the post-remediation command `go test ./internal/agent/agents
./internal/agent/registry ./internal/agent/runtime/lifecycle
./internal/agent/managedruntime ./internal/agent/hostutility
./internal/agent/settings/controller ./internal/agent/settings/handlers -count=1`
passed 2,646 tests.

Follow-up review verification added terminal assertions for normal update,
rollback, and return-to-default jobs. Successful capability probes now update
the terminal `current_version` before publication; the focused follow-up
backend command passed 2,594 tests.
