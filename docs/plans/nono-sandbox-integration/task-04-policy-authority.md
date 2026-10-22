---
id: "04-policy-authority"
title: "Persist and enforce policy selections"
status: pending
wave: 4
depends_on:
  - "03-plugin-capability"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-001
  - REQ-AGENTS-SANDBOX-002
acceptance_criteria:
  - AC-AGENTS-SANDBOX-001.1
  - AC-AGENTS-SANDBOX-001.2
  - AC-AGENTS-SANDBOX-001.3
  - AC-AGENTS-SANDBOX-001.4
  - AC-AGENTS-SANDBOX-002.1
  - AC-AGENTS-SANDBOX-002.2
  - AC-AGENTS-SANDBOX-002.3
  - AC-AGENTS-SANDBOX-002.4
  - AC-AGENTS-SANDBOX-002.5
  - AC-AGENTS-SANDBOX-002.6
system_design:
  - ../../specs/agents/system-design/sandbox-policies.md
---

# Task 04: Persist and enforce policy selections

## Summary

Implement core policy revisions, executor ceilings, and reference selection at authorized launch admission.

## In scope

Cover SQLite/PostgreSQL persistence, executor and agent defaults, workflow/task references, imports, draft probes, canonical grants, and environment validation.

## Out of scope

No silent inheritance fallback, agent-authored policy authority, or synthetic-admin activation.

## Acceptance

- Task, workflow, agent, and executor references resolve in order and always satisfy the executor ceiling.
- HTTP, WS, tools, imports, and profile environment cannot weaken or disable required confinement.
- Revisions survive reload and concurrent updates reject stale writes without changing existing attempts.

## Verification

Commands run from the Kandev repository root unless a subshell selects the dedicated plugin checkout.
New test and runner paths below are implementation deliverables, not existing passing evidence.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/sandbox/... ./internal/agent/settings/handlers ./internal/task/handlers ./internal/workflow/... -run 'SandboxPolicy|SandboxAuthority' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/sandbox/... -run 'SandboxPolicy.*SQLite' -count=1)
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN="${KANDEV_TEST_POSTGRES_DSN:?Supply a disposable PostgreSQL DSN privately}" go test -trimpath ./internal/agent/sandbox/... -run 'SandboxPolicy.*Postgres' -count=1)
```

Use TDD for changed logic. Before the first pnpm command in this workspace, run `(cd apps && pnpm install --frozen-lockfile)`.
Run each listed suite after its files exist. Do not count a zero-test filter as a pass.

## Files likely touched

- `apps/backend/internal/agent/sandbox/ (new)`
- `apps/backend/internal/common/sandbox/ (new)`
- `apps/backend/internal/agent/settings/handlers/`
- `apps/backend/internal/agent/executor/`
- `apps/backend/internal/task/models/`
- `apps/backend/internal/task/handlers/`
- `apps/backend/internal/workflow/`
- `apps/backend/internal/agent/sandbox/policy_test.go (new)`

## Dependencies

03-plugin-capability

## Risks

Nono inheritance can add access. Unknown subset relationships must refuse activation.

## Parallelism

`sequential`

## Inputs

- [Evidence](investigation.md).
- [Design](../../specs/agents/system-design/sandbox-policies.md).

## Results

Pending. No production implementation is authorized during the design turn.
