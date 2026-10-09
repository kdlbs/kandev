---
id: "01-retire-flag"
title: "Retire the flag and keep provider admission"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-001
  - REQ-EXECUTORS-PLUGIN-004
  - REQ-EXECUTORS-PLUGIN-005
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-001.1
  - AC-EXECUTORS-PLUGIN-001.3
  - AC-EXECUTORS-PLUGIN-001.4
  - AC-EXECUTORS-PLUGIN-001.8
  - AC-EXECUTORS-PLUGIN-004.1
  - AC-EXECUTORS-PLUGIN-005.2
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 01: Retire the flag and keep provider admission

## Summary

Remove the temporary runtime flag and make provider support unconditional at the host boundary.
Keep all provider eligibility, resource inventory, recovery, and plugin administration checks.

## In scope

- Remove the live config field, profile entries, runtime registration, plugin service toggle state,
  setter, and `feature_disabled` branch. Retire the exact key and environment variable.
- Remove the frontend default key and update runtime feature contract tests.
- Replace flag-off unit tests with tests for absent, disabled, incompatible, and unavailable providers.
  Include a built-in executor regression and retained-resource guard coverage.

## Out of scope

- Provider SDK or wire changes, production providers, and browser E2E updates.

## Acceptance

1. A clean install with no provider plugin exposes only built-in executors; an eligible installed
   provider is available without configuration or restart for a flag.
2. Disabled, incompatible, or stopped plugins cannot accept new provider operations; saved profiles,
   inventory, and uninstall/upgrade protections remain intact.
3. The old flag key and environment variable are absent from live contracts, present in the retired
   identity list, and stale overrides remain inert.

## Verification

```bash
(cd apps/backend && go test ./internal/runtimeflags ./internal/common/config ./internal/profiles ./internal/plugins ./internal/task/service ./internal/agent/runtime/lifecycle)
(cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint)
git diff --check
```

In a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)` before the pnpm commands.
The repository runtime-feature-flags gate also requires `make -C apps/backend lint` before delivery.

## Files likely touched

- `profiles.yaml`, `apps/backend/internal/profiles/profiles.yaml`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/runtimeflags/registry.go`, `registry_test.go`
- `apps/backend/internal/plugins/provider.go`, `service.go`, `executor_providers.go`
- `apps/backend/internal/plugins/executor_providers_test.go`, `executor_profiles_test.go`, `executor_lifecycle_guard_test.go`
- `apps/backend/internal/task/service/executor_provider_catalog.go` and adjacent tests if a missing-provider regression needs coverage
- `apps/web/lib/state/slices/features/types.ts`, `features-contract.test.ts`

## Dependencies

None.

## Risks

Provider cleanup must still be guarded by durable resource identity when a plugin is unavailable.

## Parallelism

`sequential`

## Inputs

- [Executor requirements](../../specs/executors/requirements/remote-executor-plugins.md),
  [system design](../../specs/executors/system-design/remote-executor-plugins.md), and
  [provider-boundary ADR](../../decisions/2026-09-26-remote-executor-plugin-boundary.md).
- Existing plugin admission, runtime flag retirement, provider catalog, and lifecycle tests.

## Results

Targeted Go tests passed for runtime flags, profiles, plugins, task service, and agent lifecycle. The
common-config package passed when rerun with the session's pinned internal config environment
variables unset. Backend lint and build passed. The web feature-contract test, typecheck, lint, and
i18n validation passed. `git diff --check` passed.
