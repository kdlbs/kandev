---
id: "03-update-status-api"
title: "Add cached update status"
status: complete
wave: 3
depends_on: ["02-effective-version-selection"]
plan: "plan.md"
spec: "../../specs/agents/runtime-updates.md"
---

# Task 03: Add cached update status

## Acceptance

- One read-only batch endpoint returns structural update status for every
  available managed runtime and tolerates per-package registry failures.
- The backend performs strict stable SemVer comparison, caches successful
  checks for six hours and failures for fifteen minutes, and never starts an
  update job from a status request.
- Successful activation or return-to-default invalidates the affected cache;
  tests use injected clock/resolver seams with no real registry dependency.

## Verification

```bash
cd apps/backend && go test ./internal/agent/settings/controller ./internal/agent/settings/handlers
```

## Files likely touched

- `apps/backend/internal/agent/settings/dto/dto.go`
- `apps/backend/internal/agent/settings/controller/agent_update_status.go`
- `apps/backend/internal/agent/settings/controller/controller.go`
- `apps/backend/internal/agent/settings/controller/agent_update_job.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/settings/controller/agent_update_status_test.go`
- `apps/backend/internal/agent/settings/handlers/agent_update_handlers_test.go`

## Dependencies

Task 02.

## Parallelism

Sequential. The checker depends on final effective-version and job-invalidation
contracts.

## Inputs

- Spec: Update status API, failure behavior, cache lifetimes
- Plan: Cached update status API
- Existing pattern: `RuntimeUpdater.ResolveTarget` and handler error contracts

## Output contract

Report the endpoint shape, cache and concurrency behavior, partial-failure
evidence, exact tests/results, risks, and synchronized task/plan status.

## Results

Complete. `GET /api/v1/agent-update/status` returns one structural status per
available managed runtime, compares strict stable SemVer, tolerates individual
registry failures as `unknown`, bounds stale lookups to five concurrent
packages, and does not create update jobs. Successful checks cache for six
hours, failures for fifteen minutes, and successful activation/default reset
invalidates the affected package.

Verification: controller and handler tests passed in the post-remediation
2,646-test scoped backend run, including cache TTL, invalidation, bounded
lookups, partial failure, and read-only endpoint coverage.
