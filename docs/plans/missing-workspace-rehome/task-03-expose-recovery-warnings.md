---
id: "03-expose-recovery-warnings"
title: "Expose recovery and Coder durability warnings"
status: completed
wave: 3
depends_on:
  - "02-recover-launches"
plan: "plan.md"
requirements:
  - REQ-TASKS-MISSING-WORKSPACE-REHOME-002
  - REQ-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001
acceptance_criteria:
  - AC-TASKS-MISSING-WORKSPACE-REHOME-002.1
  - AC-TASKS-MISSING-WORKSPACE-REHOME-002.4
  - AC-TASKS-MISSING-WORKSPACE-REHOME-002.5
  - AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.1
  - AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.2
  - AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.3
system_design:
  - ../../specs/tasks/system-design/missing-workspace-rehome.md
  - ../../specs/executors/system-design/coder-task-root-durability.md
---

# Task 03: Expose recovery and Coder durability warnings

## Summary

Expose stamped fresh-rehome authorization on the existing launch error card and
add a machine-readable Coder task-root warning to SSH profile health. Keep the
same capability and state on desktop and mobile and document the durable root.

## In scope

- Launch-error category/action types, localized copy, and authorization UI.
- Mobile inset recovery flow with shared state and no horizontal overflow.
- SSH Coder/mount health evidence and profile/launch warning payload.
- Responsive executor profile warning banner.
- Public SSH executor guidance for `/work/.kandev`.

## Out of scope

- Blocking profile save.
- Creating Coder mounts.

## Acceptance

- Unknown/unique work produces a prominent data-loss warning and only a stamped
  human action can proceed.
- Identified risky Coder profiles show the same warning semantics on desktop,
  mobile, API save, and launch validation.
- All new user-facing copy is localized in every required catalog.

## Verification

```bash
cd apps/backend && go test -race ./internal/task/service ./internal/agent/runtime/lifecycle -run 'Test.*Coder.*TaskRoot'
cd apps && pnpm --filter @kandev/web test -- --run components/task/simple/components/task-launch-error-entry.test.tsx components/settings/profile-edit
cd apps/web && pnpm run i18n:check
node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/task/service/service_resources.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go`
- `apps/web/lib/types/task-launch-error.ts`
- `apps/web/components/task/simple/components/task-launch-error-entry.tsx`
- `apps/web/app/settings/executors/[profileId]/page.tsx`
- `apps/web/src/locales/`
- `docs/public/executors.md`

## Dependencies

- Task 02 recovery category and action contract.

## Risks

- Remote mount evidence can be unknown; UI copy must not present unknown as
  confirmed safety or confirmed loss.
- Existing settings composition must remain touch-accessible without adding a
  second mobile scroll owner.

## Parallelism

`sequential`

## Inputs

- Existing task launch error card and mobile recovery E2E.
- Existing executor profile status/banner and SSH test connection evidence.

## Results

Implemented the stamped `rehome_fresh` action on the existing task launch-error
card for desktop and mobile. Added the always-visible SSH durable-root warning,
with Coder called out explicitly, localized interpolation for the example path,
and public operator guidance.
