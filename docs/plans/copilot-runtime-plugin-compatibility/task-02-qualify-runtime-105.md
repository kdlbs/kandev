---
id: "02-qualify-runtime-105"
title: "Qualify and admit Copilot 1.0.105"
status: done
wave: 2
depends_on:
  - "01-runtime-compatibility-preview"
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-001.1
  - AC-AGENTS-COPILOT-PLUGINS-003.2
  - AC-AGENTS-COPILOT-PLUGINS-003.3
  - AC-AGENTS-COPILOT-PLUGINS-004.1
  - AC-AGENTS-COPILOT-PLUGINS-004.2
  - AC-AGENTS-COPILOT-PLUGINS-004.3
  - AC-AGENTS-COPILOT-PLUGINS-004.5
  - AC-AGENTS-COPILOT-PLUGINS-004.7
  - AC-AGENTS-COPILOT-PLUGINS-005.3
  - AC-AGENTS-COPILOT-PLUGINS-005.4
  - AC-AGENTS-COPILOT-PLUGINS-005.8
  - AC-AGENTS-COPILOT-PLUGINS-007.1
  - AC-AGENTS-COPILOT-PLUGINS-007.3
  - AC-AGENTS-COPILOT-PLUGINS-007.4
  - AC-AGENTS-COPILOT-PLUGINS-007.6
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
  - ../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md
---

# Task 02: Qualify and admit Copilot 1.0.105

## Summary

Establish compatibility for the exact managed npm package before admitting its
explicit plugin selections. Keep all other unknown versions rejected and
preserve the native-policy boundary.

## In scope

- Add a focused build-tagged `TestCopilotPluginNativeAdapterAcceptance` beside
  the existing diagnostic harness. It uses temporary native state, no model
  prompt, and the exact package from `ManagedNPMRuntimeSpec`.
- Exercise native commands and existing parser/root helpers independently of
  `RuntimeSupported`. Do not temporarily widen production admission to qualify itself.
- Verify the npm package, executable identity, and successful empty inventory.
  Verify native-written installed/live metadata, matching manifests, and repeated
  mounts with a special-character path. Keep raw state outside the repo.
- Add offline fixture/identity contract tests for the new harness. Do not
  weaken the existing full diagnostic or relabel absent controls as passed.
- Record sanitized native observations and platform limits in the sole ledger.
  Ordinary tests remain offline and credential-free.
- Write `TestCopilotPluginRuntime105Admission` against production discovery,
  selection resolution, and separate argv preparation. Its expected Red is
  `runtime_unsupported` before the map changes.
- After native evidence passes, add only `1.0.105` to the qualified map. Extend
  the existing support table and caller-boundary tests with that exact runtime.
  Keep empty, unknown-version, isolated, wrapped, and retained controls.

## Out of scope

Changing the default/pin, permitting a range, login or credential copying,
enterprise-policy changes, and claims about authenticated upstream tools.
Fixture installation commands can affect only disposable native state. They
are not production discovery operations or changes to the user's plugins.

## Acceptance

1. Exact native adapter checks pass and the ledger names actual commands,
   package identity, platform, metadata, mount observations, and limitations.
2. The permanent 1.0.105 regression fails before admission, then passes without
   weakening root, source, executor, wrapper, or manual-conflict validation.
3. Discovery, mounts, runtime preview, and caller commands agree on the exact
   selected version. Existing supported versions and negative controls pass.

## Verification

From the repository root:

```bash
npm view @github/copilot@1.0.105 version dist.tarball optionalDependencies --json
(cd apps/backend && go test -trimpath -tags 'copilot_native fts5' ./internal/agent/copilotplugins -run '^TestNative' -count=1)
(cd apps/backend && KANDEV_COPILOT_NATIVE_VERSION=1.0.105 go test -trimpath -tags 'copilot_native fts5' ./internal/agent/copilotplugins -run '^TestCopilotPluginNativeAdapterAcceptance$' -count=1 -timeout=8m -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/copilotplugins -run '^(TestCopilotPluginRuntime105Admission|TestCopilotPluginRuntimeUnsupportedAdmission|TestRuntimeSupportedVersions|TestCopilotPluginNativeAuthorityAdmission)$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/copilotplugins ./internal/agent/settings/controller -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agent/hostutility -run '^TestCopilotPlugin' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/copilotplugins/context.go apps/backend/internal/agent/copilotplugins/copilotplugins_test.go apps/backend/internal/agent/copilotplugins/runtime_compatibility_test.go apps/backend/internal/agent/copilotplugins/native_adapter_acceptance_test.go apps/backend/internal/agent/copilotplugins/native_adapter_acceptance_contract_test.go apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_launch_test.go apps/backend/internal/agent/hostutility/copilot_plugins_test.go
git diff --check
```

The new native test owns fresh HOME/config/work directories through its child
environment. It preserves inherited proxy and CA trust. All waits/output are
bounded, and exact owned processes/directories are cleaned on every exit path.
Do not run the existing macOS-only full authenticated test on Linux and call
its platform rejection adapter evidence. Its diagnostic result remains separate.

The planning lookup returned E404. The package later became available, so
admission was added after the native evidence passed. If an exact package were
unavailable, admission would stay unchanged and the external blocker recorded. A downloaded standalone
binary alone does not establish managed npm identity. If metadata differs from
the bounded adapter, stop admission and amend the owning design first.
Task 03 can still proceed after task 01.

## Files likely touched

- `apps/backend/internal/agent/copilotplugins/context.go`, `copilotplugins_test.go`, and task 01's `runtime_compatibility_test.go`.
- New `apps/backend/internal/agent/copilotplugins/native_adapter_acceptance_test.go` and `native_adapter_acceptance_contract_test.go`.
- `apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_launch_test.go`.
- `apps/backend/internal/agent/hostutility/copilot_plugins_test.go`.
- `docs/plans/copilot-profile-plugins/compatibility-evidence.md`.
- Native-authority design evidence baseline after successful qualification.

## Dependencies

Task 01 supplies the shared compatibility projection. Native artifact access
is an external prerequisite; it is not permission to change another runtime.

## Risks

Native metadata is not a stable public schema. A synthetic compatible document
cannot replace native-written evidence. Missing enterprise credentials are not
a separate admission gate under the accepted ADR.

## Parallelism

`sequential`

## Inputs

- Requirements 001, 003, 004, 005, and 007.
- Both plugin designs and the native-policy-authority ADR.
- Existing native diagnostic fixtures and sole compatibility ledger.
- Production `Discover`, `ResolveLaunchSelection`, and actual lifecycle and
  hostutility caller tests. Use one captured profile/version/environment.

## Results

The configured npm registry now publishes `@github/copilot@1.0.105`. A new
focused `copilot_native` test qualified the exact package on Linux x64 with
disposable home, config, npm cache, work, marketplace, and plugin directories.
It observed exact package/binary identity, a successful fresh inventory, a
native-written remote marketplace install with `config.json`
`installedPlugins`/`cache_path` metadata, a `source=installed` row, a
`source=live` directory-marketplace control, matching manifests, production
`Discover` and `ResolveLaunchSelection` admission for both shapes, and two
external rows from repeated `--plugin-dir` arguments with special-character
paths.

Command:

```bash
(cd apps/backend && KANDEV_COPILOT_NATIVE_VERSION=1.0.105 \
  go test -trimpath -tags 'copilot_native fts5' ./internal/agent/copilotplugins \
  -run '^TestCopilotPluginNativeAdapterAcceptance$' -count=1 -timeout=8m -v)
```

All five cases passed, including production installed-plugin admission and the
live control. Offline `TestCopilotPluginRuntime105AdapterContract` covers the
exact managed package identity and platform-scoped discovery/mount predicate.
`1.0.105` was then added to the qualified-version map with linux/amd64
platform scope. `TestCopilotPluginRuntime105Admission`, the extended support
table, the lifecycle 1.0.105 preparation test, and the hostutility 1.0.105
inventory test pass on qualified hosts. Other platforms remain unsupported
until separately qualified. The existing full macOS diagnostic remains
separate and was not rerun on Linux. No authentication, enterprise-policy
enforcement, model behavior, or other runtime version is claimed. The sole
ledger records the sanitized result.
