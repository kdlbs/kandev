---
id: "04-policy-authority"
title: "Persist and enforce policy selections"
status: in_progress
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

Implementation is in progress. Core revisions, policy bindings, selection precedence, ceiling checks, revocation, and strict operator HTTP writes have focused passing tests. SQLite and PostgreSQL checks cover schema replay, immutable content, provenance, concurrent publication, conflicts, and transactions. Target lookup checks workspace access instead of trusting request scope.

The restart-required `features.sandboxPolicies` flag is off in prod, dev, and e2e. It gates provider dispatch and operator routes. Disabling it blocks required policy resolution while preserving ordinary wrappers. The feature registry, profile contract, and frontend contract checks pass. Backend configuration tests require a clean test environment and umask 022; inherited server settings and umask 077 caused unrelated initial failures.

Launch admission now rejects a selected policy until the runtime path is available. Discovery carries executor context and policy cache identity. Utility prompts and background probes enforce admission before process creation or cache reads. Missing tenant context cannot bypass saved requirements.

Durable task guards retain the admitted permission limit across service reloads and context changes. Tests reject expanded permissions through agent or executor switches and through captured parent context. Task creation captures policy before inserting the task and carries restrictions from the verified MCP caller even when the new task has no parent link. Focused creation/copy tests pass. Task-guard persistence and concurrent-write tests pass under the race detector on SQLite and PostgreSQL.

Logical `work_dir` grants support reusable policies without granting a parent task directory. Focused tests cover read/write ceilings, per-execution resolution, missing paths, protected roots, and symlink aliases. The import/copy integration audit, final environment enforcement, runtime compilation, and immutable attempt materialization remain. Native Nono profiles are not accepted as live authority; the current import contract accepts only canonical revisions and pinned revision sources. Do not treat these service-level results as full product acceptance.

Immutable attempt persistence now passes focused SQLite and PostgreSQL race tests. Capture rechecks the selected revision and binding version, and stores provider/runtime identity plus resolved grants. Tests cover service reload, later policy publication, stale selection, stale lifecycle writes, process identity replacement, and terminal-state revival refusal. Runtime preparation and lifecycle callers still need to adopt this store. Custom database WAL and SHM paths are also protected by passing startup-policy tests.
