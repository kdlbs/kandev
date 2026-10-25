---
id: "04-autonomous-attribution"
title: "Attribute autonomous execution"
status: done
wave: 4
depends_on:
  - "03-interactive-admission"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-OWNERSHIP-001
  - REQ-EXECUTORS-OWNERSHIP-004
  - REQ-EXECUTORS-OWNERSHIP-005
acceptance_criteria:
  - AC-EXECUTORS-OWNERSHIP-001.1
  - AC-EXECUTORS-OWNERSHIP-001.4
  - AC-EXECUTORS-OWNERSHIP-004.3
  - AC-EXECUTORS-OWNERSHIP-005.1
  - AC-EXECUTORS-OWNERSHIP-005.2
  - AC-EXECUTORS-OWNERSHIP-005.3
  - AC-EXECUTORS-OWNERSHIP-005.4
system_design:
  - ../../specs/executors/system-design/per-user-ownership.md
---

# Task 04: Attribute autonomous execution

## Summary

Persist and restore each autonomous source creator so private executor use is checked as that current user through dispatch, retries, and restarts.

## In scope

- Inventory/create migrations for Kandev creator attribution on automations/workflows and all provider-watch families; preserve unknown legacy rows.
- Wire current identity through scheduler, webhook, provider poller, workflow engine, run queues and retries; imports bind to importer.
- Authorize editors and interactive triggers against resulting private executor choices to prevent creator authority being borrowed.

## Out of scope

Other work orders and the package exclusions. Do not add UI before the underlying authorization contract exists.

## Acceptance

- Provider/source matrix proves owner/admin success and other/disabled/missing creator denial before runtime or credential calls.
- Creator survives edits/restart/retry and is not accepted from exports, provider author filters, or forged payloads; legacy shared sources still run.
- Shared-workspace edits/manual triggers cannot cause another creator to run arbitrary work on private compute.

## Verification

Run from repository root. On a fresh worktree, run
`(cd apps && pnpm install --frozen-lockfile)` before the first pnpm command.
Add the planned tests with TDD; test names must contain `ExecutorOwnership`
where the Go commands filter by that pattern. Ensure new focused web tests are
added to the exact Vitest command before recording completion.

```bash
(cd apps/backend && go test ./internal/automation ./internal/workflow/... ./internal/github ./internal/gitlab ./internal/jira ./internal/linear ./internal/sentry ./internal/azuredevops ./internal/runs/... ./internal/backendapp -run 'ExecutorOwnership' -count=1)
```

Also run `bash scripts/lint-go-changed` with the explicit changed Go files,
then `git diff --check`. Run the ownership conformance scenario against both
SQLite and the repository-configured PostgreSQL test database; a skipped
PostgreSQL test is not evidence of parity. Record the database prerequisite
and exact command/output when that scenario is available.

## Files likely touched

- `apps/backend/internal/automation/models.go`
- `apps/backend/internal/automation/service.go`
- `apps/backend/internal/automation/store.go`
- `apps/backend/internal/automation/scheduler.go`
- `apps/backend/internal/automation/retry_*.go`
- `apps/backend/internal/workflow/models/models.go`
- `apps/backend/internal/workflow/repository/`
- `apps/backend/internal/workflow/engine/`
- `apps/backend/internal/github/`
- `apps/backend/internal/gitlab/`
- `apps/backend/internal/jira/`
- `apps/backend/internal/linear/`
- `apps/backend/internal/sentry/`
- `apps/backend/internal/azuredevops/`
- `apps/backend/internal/runs/`
- `apps/backend/internal/backendapp/turn_adapters.go`
- `apps/backend/internal/backendapp/executor_ownership_wiring_test.go (new)`

## Dependencies

Task 03; execute sequentially after its acceptance and targeted checks pass.

## Risks

Provider CreatorID often means a remote issue filter. Some sources have no durable Kandev creator. Fail closed for private targets rather than inventing one.

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

Persisted server-derived creator identity on GitHub/GitLab review and issue watches, Jira/Linear/Sentry issue watches, Azure DevOps work-item/pull-request watches, automations, and workflows. Passive PR/MR tracking retains the existing session actor. Source edits and manual triggers validate the actual caller and immutable creator, including a private workspace default. Internal events carry trusted creator attribution through dispatch; retries reload the current source creator. Unknown legacy creators cannot borrow private authority. Export/import excludes local creator authority.

Creator persistence/forgery, inactive creator, interactive deputy, export disposition, workflow/task edit, and production wiring checks pass. Full provider suites, including the Azure DevOps and automation reruns with the isolated Go cache, pass. Changed-package Go lint is recorded in the plan.
