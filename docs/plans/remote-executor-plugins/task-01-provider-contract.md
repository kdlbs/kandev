---
id: "01-provider-contract"
title: "Declare and gate remote executor providers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-001
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-001.4
  - AC-EXECUTORS-PLUGIN-001.5
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 01: Declare and gate remote executor providers

## Summary

Add the versioned manifest and SDK contract without enabling a live launch path. Establish fail-closed registration and rollout gates before provider dispatch exists.

## In scope

- Add executor_providers, capability enforcement, optional SDK interfaces, all seven provider RPCs, Host callbacks, artifact streaming messages, and generated protobuf stubs.
- Validate namespaced ownership, schema bounds, negotiated versions, and required methods. Reuse existing plugin registration and dispatch patterns.
- Add restart-required features.remoteExecutorPlugins and KANDEV_FEATURES_REMOTE_EXECUTOR_PLUGINS through the typed registry, config, profiles and web feature defaults. All shipped defaults are false.

## Out of scope

- Environment allocation, profile UI, provider-specific code.

## Acceptance

- Old SDK implementations still work; undeclared, duplicate, incompatible and incomplete providers fail registration.
- Flag-off tests prove no provider RPC or enabled-only side effect is dispatched; inventory visibility is preserved.
- The generated wire contract round-trips bounds, errors and versions without exposing secrets in errors.

## Verification

Use TDD for changed logic. Run commands from the repository root.
Before the first pnpm command in a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)`.
Tests named below are new unless an existing path is listed. Do not accept a no-tests result.

```bash
make -C apps/backend proto
(cd apps/backend && go test ./pkg/pluginsdk ./internal/plugins/manifest ./internal/plugins/... -count=1)
(cd apps/backend && go test ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1)
make -C apps/backend lint
(cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint)
```

Required evidence:

- `pkg/pluginsdk/executor_provider_test.go: TestPluginExecutorWireContract, TestPluginExecutorOldPluginCompatibility`
- `internal/plugins/executor_providers_test.go: TestPluginExecutorAdmission, TestPluginExecutorDisabledNoDispatch`

## Files likely touched

- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/pkg/pluginsdk/ (new executor_provider.go and executor_provider_test.go)`
- `apps/backend/internal/plugins/manifest/manifest.go`
- `apps/backend/internal/plugins/ (new executor_providers.go and executor_providers_test.go)`
- `apps/backend/internal/runtimeflags/registry.go and config.go`
- `apps/backend/internal/common/config/config.go`
- `profiles.yaml`
- `apps/web/lib/state/slices/features/types.ts`

## Dependencies

None.

## Risks

Generated contracts and flag completeness span several packages. Do not introduce a second capability approval model.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/remote-executor-plugins.md), using the IDs in frontmatter.
- [System design](../../specs/executors/system-design/remote-executor-plugins.md), including the named contract and flow.
- Existing source and nearby tests in the file list; follow scoped `AGENTS.md`.
- [Accepted ADR](../../decisions/2026-09-26-remote-executor-plugin-boundary.md).

## Results

Completed. The manifest declares namespaced executor providers with bounded scalar schemas, required contract/state versions, localized message references, and explicit capabilities. The plugin SDK exposes seven all-or-nothing provider RPCs and three operation-bound Host callbacks, including validated 256 KiB artifact streaming. Legacy plugins remain source-compatible; incomplete providers fail the activation contract probe. Provider RPC dispatch and contract probes remain off unless the restart-required flag is enabled.

Verification results:

- `make -C apps/backend proto`: passed.
- `cd apps/backend && go test ./pkg/pluginsdk ./internal/plugins/manifest ./internal/plugins/... -count=1`: passed.
- `cd apps/backend && env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE -u KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG go test ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1`: passed. The initial run without these unsets selected the developer's ambient config file; the isolated rerun passed.
- `make -C apps/backend lint`: passed with 0 issues.
- `cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts`: passed (5 tests).
- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps/web && pnpm run lint`: passed.
