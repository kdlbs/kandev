---
id: "03-interactive-admission"
title: "Guard interactive compute admission"
status: done
wave: 3
depends_on:
  - "02-private-catalogue"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-OWNERSHIP-001
  - REQ-EXECUTORS-OWNERSHIP-003
  - REQ-EXECUTORS-OWNERSHIP-004
acceptance_criteria:
  - AC-EXECUTORS-OWNERSHIP-001.2
  - AC-EXECUTORS-OWNERSHIP-003.3
  - AC-EXECUTORS-OWNERSHIP-004.1
  - AC-EXECUTORS-OWNERSHIP-004.2
  - AC-EXECUTORS-OWNERSHIP-004.3
  - AC-EXECUTORS-OWNERSHIP-004.4
system_design:
  - ../../specs/executors/system-design/per-user-ownership.md
---

# Task 03: Guard interactive compute admission

## Summary

Carry an authenticated launch actor through interactive execution and enforce eligibility before any private-compute or credential side effect.

## In scope

- Cover task/session starts, resume, retry, workflow moves, runner changes, inherited/reused environments, and trusted MCP dispatch.
- Persist pending/session actor evidence and revalidate current ownership/account at common orchestrator/runtime admission after default resolution.
- Apply private-compute eligibility to prompt, terminal, command/file-write/editor operations; preserve read-only collaboration and passive recovery.

## Out of scope

Other work orders and the package exclusions. Do not add UI before the underlying authorization contract exists.

## Acceptance

- Denied operations produce no provisioning, secret reads/seeding, scripts, terminal creation, or prompt delivery for every runtime family.
- Queue/retry/ownership-transfer tests prove current actor checks; no internal missing-identity or inherited-environment bypass succeeds.
- Existing shared-resource behavior and passive reattachment remain valid without granting new work authority.

## Verification

Run from repository root. On a fresh worktree, run
`(cd apps && pnpm install --frozen-lockfile)` before the first pnpm command.
Add the planned tests with TDD; test names must contain `ExecutorOwnership`
where the Go commands filter by that pattern. Ensure new focused web tests are
added to the exact Vitest command before recording completion.

```bash
(cd apps/backend && go test ./internal/task/service ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/mcp/scope ./internal/backendapp ./internal/gateway/websocket -run 'ExecutorOwnership' -count=1)
```

Also run `bash scripts/lint-go-changed` with the explicit changed Go files,
then `git diff --check`. Run the ownership conformance scenario against both
SQLite and the repository-configured PostgreSQL test database; a skipped
PostgreSQL test is not evidence of parity. Record the database prerequisite
and exact command/output when that scenario is available.

## Files likely touched

- `apps/backend/internal/task/service/`
- `apps/backend/internal/task/models/`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/internal/orchestrator/executor/`
- `apps/backend/internal/agent/runtime/lifecycle/`
- `apps/backend/internal/mcp/scope/`
- `apps/backend/internal/backendapp/`
- `apps/backend/internal/gateway/websocket/terminal_handler.go`
- `apps/backend/internal/gateway/websocket/terminal_sessions.go`

## Dependencies

Task 02; execute sequentially after its acceptance and targeted checks pass.

## Risks

The actor must be derived from trusted identity, never from workspace ownership, client JSON, or the scheduler context. Existing exec routes need the same decision.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/per-user-ownership.md).
- [System design](../../specs/executors/system-design/per-user-ownership.md).
- [Ownership ADR](../../decisions/2026-10-15-private-executor-admission.md).
- [Plan evidence and existing code patterns](plan.md#technical-approach).
- Read scoped AGENTS.md, `/tdd`, and applicable `/mobile-parity` and `/e2e`
  instructions before implementation.

## Results

Implemented admission at session preparation, prepared launch, resume, runtime launch, process startup, profile-seeded terminal environments, and prompt dispatch. Existing environment reuse is checked before preparation, including legacy sessions whose runner is only recorded on their environment. Deferred prompts recheck after their queue barrier. Durable task/session actors prevent workspace-owner substitution during MCP and recovery.

Focused launch/resume/prompt/environment and compatibility tests pass. Full task-service, lifecycle, and orchestrator suites pass. Browser coverage launches the mock agent on its owner's profile and denies another collaborator on the same readable board. Changed-package Go lint is recorded in the plan.
