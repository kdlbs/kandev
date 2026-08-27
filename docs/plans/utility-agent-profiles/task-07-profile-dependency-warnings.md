---
id: "07-profile-dependency-warnings"
title: "Warn on utility profile dependencies"
status: completed
wave: 2
depends_on: ["01-persist-profile-bindings"]
plan: "plan.md"
spec: "../../specs/agents/requirements/utility-agent-profiles.md"
---

# Task 07: Warn on utility profile dependencies

## Intent

Extend the existing agent-profile disable and delete dependency flows so users see utility-agent
bindings before a profile change can break unattended work.

## Acceptance

- Disable dependency checks list affected utility agents, show a warning before saving, and allow
  cancel or explicit confirmation. A confirmed disable keeps bindings unchanged.
- Delete dependency conflicts include affected utility agents in the existing profile-in-use dialog.
  The confirmed force path keeps stale utility IDs and does not auto-reassign them.
- A dependency lookup error blocks the profile change. In-flight utility calls continue with their
  start-time profile snapshot.

## Files likely touched

- `apps/backend/internal/agent/settings/controller/profile_crud.go`
- `apps/backend/internal/agent/settings/controller/profile_crud_watcher_deps_test.go`
- `apps/backend/internal/agent/settings/controller/profile_dependency_types.go`
- `apps/backend/internal/utility/service/service.go`
- `apps/web/components/settings/agent-profile-page-state.ts`
- `apps/web/components/settings/agent-profile-delete-dialog.tsx`
- `apps/web/components/settings/agent-profile-delete-dialog.test.tsx`
- `apps/web/components/settings/agent-profile-page.tsx`
- `apps/web/src/locales/en/agents.json`
- `apps/web/src/locales/zh-cn/agents.json`
- `apps/web/src/locales/pseudo/agents.json`

## Dependencies

Task 01 establishes the profile binding state and utility reference query contract.

## Parallelism

Parallel-safe with tasks 02 and 04 after task 01: this task owns profile CRUD dependency and profile
page/dialog files. Sequential execution remains the default.

## Inputs

- Spec: profile disable/delete behavior, failure modes, persistence guarantees, and dependency
  scenarios.
- ADR: `docs/decisions/2026-08-08-utility-profile-dependency-safety.md`.
- Existing profile-in-use conflict and force-delete behavior for sessions, watchers, automations,
  and routing tiers.

## Verification

```bash
cd apps/backend && go test ./internal/agent/settings/controller/... ./internal/utility/...
cd apps && pnpm --filter @kandev/web test -- components/settings/agent-profile-delete-dialog.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
```

## Output contract

Report the dependency response shape, disable/delete confirmation behavior, files changed, exact
test results, lookup-failure behavior, stale-binding behavior, and synchronized task/plan status.

## Results

Added utility dependency enumeration to profile disable/delete conflicts, force-confirmation handling, stale-binding preservation, and localized warning sections. Verified with agent settings controller/handler and frontend conflict dialog tests.
