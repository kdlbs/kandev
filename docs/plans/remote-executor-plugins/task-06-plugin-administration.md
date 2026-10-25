---
id: "06-plugin-administration"
title: "Protect resources during plugin lifecycle changes"
status: done
wave: 6
depends_on:
  - "05-recovery-cleanup"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-001
  - REQ-EXECUTORS-PLUGIN-005
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-005.1
  - AC-EXECUTORS-PLUGIN-005.2
  - AC-EXECUTORS-PLUGIN-005.3
  - AC-EXECUTORS-PLUGIN-005.4
  - AC-EXECUTORS-PLUGIN-001.5
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 06: Protect resources during plugin lifecycle changes

## Summary

Make disable, uninstall, upgrade and rollback respect retained executor resources. Keep these guards active even when the feature flag is off.

## In scope

- Close admission and cancel/drain dispatch before disable; preserve profile state, credentials and allocation identities.
- Guard uninstall before stopping runtime or deleting secrets; reject retained and unresolved inventory under the same admission barrier.
- Check provider IDs and state-version compatibility before upgrade and rollback activation; fence old callbacks and reuse existing version rollback behavior.

## Out of scope

- Force forget, out-of-band resource adoption, new capability approval framework.

## Acceptance

- Concurrent launch versus uninstall cannot allocate an untracked resource or delete needed credentials.
- Disable/re-enable and crash/restart preserve ownership; compatible upgrade succeeds and incompatible upgrade leaves the old provider usable.
- Flag-off administrative guards preserve retained inventory without dispatching provider RPCs.

## Verification

Use TDD for changed logic. Run commands from the repository root.
Tests named below are new unless an existing path is listed. Do not accept a no-tests result.

```bash
(cd apps/backend && env TMPDIR=/root/kandev-go-tmp GOTMPDIR=/root/kandev-go-tmp go test -race ./internal/plugins/... -count=1)
```

Required evidence:

- `internal/plugins/executor_lifecycle_guard_test.go: TestPluginExecutorDisableRetention, TestPluginExecutorUninstallRace, TestPluginExecutorUpgradeCompatibility, TestPluginExecutorFlagOffRetention`

## Files likely touched

- `apps/backend/internal/plugins/service_lifecycle.go`
- `apps/backend/internal/plugins/service_install.go`
- `apps/backend/internal/plugins/executor_providers.go`
- `apps/backend/internal/plugins/executor_provider_host.go`
- `apps/backend/internal/plugins/runtime/manager.go`
- `apps/backend/internal/plugins/ (new executor_lifecycle_guard.go and executor_lifecycle_guard_test.go)`
- `apps/backend/internal/plugins/handlers.go and handlers_test.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/web/components/settings/plugins/use-plugin-actions.ts`
- `apps/web/lib/api/domains/plugins-api.ts`
- `apps/web/src/locales/*/plugins.json`
- `apps/web/e2e/tests/settings/mobile-plugin-executor-lifecycle.spec.ts`

## Dependencies

[Task 05](task-05-recovery-cleanup.md).

## Risks

Existing lifecycle/dispatch lock order can deadlock if a guard performs nested dispatch. Add concurrency tests with explicit synchronization.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/remote-executor-plugins.md), using the IDs in frontmatter.
- [System design](../../specs/executors/system-design/remote-executor-plugins.md), including the named contract and flow.
- Existing source and nearby tests in the file list; follow scoped `AGENTS.md`.
- [Accepted ADR](../../decisions/2026-09-26-remote-executor-plugin-boundary.md).

## Results

Lifecycle transitions now close provider admission, cancel admitted call contexts, and drain calls plus host callbacks before stopping or replacing a plugin. Disable preserves provider declarations, secret credentials, and allocation inventory, and its API response drives a localized warning that remote environments may keep running.

Uninstall checks durable inventory under the same dispatch barrier before runtime stop or secret deletion. It rejects retained or unresolved resources with the normal task-cleanup path, including while the rollout flag is off. Upgrade checks provider identity, stable installation identity, contract version, and recorded state versions before replacing the working installation. A failed inventory read fails closed. Existing compatible-version rollback paths remain available, and callback leases drain before the old provider can be replaced.

Validation passed:

- `(cd apps/backend && env TMPDIR=/root/kandev-go-tmp GOTMPDIR=/root/kandev-go-tmp go test -race ./internal/plugins/... -count=1)`
- `pnpm exec vitest run components/settings/plugins/use-plugin-actions.test.tsx lib/api/domains/plugins-api.test.ts`: 38 tests passed.
- `pnpm run i18n:check` and `pnpm run typecheck` passed.
- `TMPDIR=/root/kandev-go-tmp GOTMPDIR=/root/kandev-go-tmp make -C apps/backend build` passed.
- `pnpm run build:e2e` and `make -C apps/backend e2e-plugin-ui e2e-plugin-package` passed.
- `pnpm e2e:run --project=mobile-chrome e2e/tests/settings/mobile-plugin-executor-lifecycle.spec.ts`: 1 test passed.
