---
id: "04-automatic-launch"
title: "Load the integration automatically on supported OpenCode launches"
status: completed
wave: 4
depends_on:
  - "03-shell-outcome-delivery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-PLUGIN-001
  - REQ-AGENTS-OPENCODE-PLUGIN-004
acceptance_criteria:
  - AC-AGENTS-OPENCODE-PLUGIN-001.1
  - AC-AGENTS-OPENCODE-PLUGIN-001.2
  - AC-AGENTS-OPENCODE-PLUGIN-001.3
  - AC-AGENTS-OPENCODE-PLUGIN-001.4
  - AC-AGENTS-OPENCODE-PLUGIN-001.5
  - AC-AGENTS-OPENCODE-PLUGIN-001.6
  - AC-AGENTS-OPENCODE-PLUGIN-001.7
  - AC-AGENTS-OPENCODE-PLUGIN-004.1
  - AC-AGENTS-OPENCODE-PLUGIN-004.2
  - AC-AGENTS-OPENCODE-PLUGIN-004.3
  - AC-AGENTS-OPENCODE-PLUGIN-004.4
  - AC-AGENTS-OPENCODE-PLUGIN-004.5
  - AC-AGENTS-OPENCODE-PLUGIN-004.6
system_design:
  - ../../specs/agents/system-design/opencode-kandev-plugin.md
  - ../../specs/agents/system-design/opencode-v2-adoption.md
---

# Task 04: Load the integration automatically on supported OpenCode launches

## Summary

Wire the proved plugin into task-owned OpenCode v2 ACP launches. Supply its
artifact and private bootstrap in the executor before process start. Gate all
side effects through one typed release toggle and the proved runtime manifest.

## Scope and owned files

- Own integration preparation/lifetime files in `server/process` and
  `server/opencodeplugin`. Integrate after runtime/environment resolution and
  before `Manager.buildFinalCommand`.
- Own additive typed configuration in `internal/agentctl/types`,
  `server/config/config.go`, `server/instance`, and the lifecycle startup chain.
- Own resolved family/source/version propagation from the existing OpenCode
  selection and native detection seams. Do not parse a shell command to infer it.
- Own `profiles.yaml`, `internal/common/config/config.go`,
  `internal/runtimeflags/registry.go`, `internal/runtimeflags/config.go`, and
  their existing contract tests.
- Own the all-off key in `apps/web/lib/state/slices/features/types.ts` and
  existing localized toggle metadata required by the current Settings surface.
- Own asset/build dependencies in backend/root Makefiles and helper packaging
  tests. Every shipped agentctl variant must contain the same verified asset.

Add `features.opencodePlugin` / `KANDEV_FEATURES_OPENCODE_PLUGIN`, with
`FeaturesConfig.OpenCodePlugin` and restart-required metadata. Default it off in
prod, dev, and e2e. A backend override enables automatic preparation on the next
supported launch. Preserve the environment/SQLite/profile precedence.

Append one local-directory plugin entry through child-only inline configuration.
Use the existing `hujson` library. Preserve file configuration, inline fields,
plugin ordering/options, environment substitutions, policies, and ACP MCP
injection. Do not overwrite existing environment keys by duplicate append order.
Respect explicit stripping of the injection variable.

Start the loopback host before OpenCode. Require readiness for the shell module
before issuing claims. End preparation safely on failure without replaying a
prompt or changing runtime selection. Keep ordinary ACP available.

Use the same staging logic inside the owning local, Docker, SSH, and Kubernetes
agentctl. It must not require a host path, external callback port, package
download, or another credential. Keep v1, unknown/incompatible versions, other
providers, probes, utility sessions, and CLI passthrough on their current paths.

Retained agentctl adoption preserves the original plugin host and lease.
Backend reconnect must not reinject configuration into a live process. Older
retained agentctl peers expose no integration capability and stay legacy.

## Acceptance

1. A supported v2 ACP task launch receives one reviewed plugin automatically,
   with preserved user configuration and executor-local paths. Repeated launch
   preparation is idempotent and stopped processes lose their lease authority.
2. Every disabled, unsupported, stripped, old-peer, probe, utility, passthrough,
   and other-provider path has no integration side effects. Shared background
   disablement activates no shell module.
3. Flag/profile/frontend completeness, mixed-version adoption, helper artifact,
   source selection, and cleanup tests pass. Initial defaults remain off.

## Verification

Add `opencode_plugin_launch_test.go`, `opencode_plugin_config_test.go`,
`opencode_plugin_lifecycle_test.go`, and `opencode_plugin_adoption_test.go`
beside their respective process/configuration seams. Cover a reserved-ID
collision, malformed inline JSONC, managed policy omission, and Windows paths.

From `apps/backend/`:

```bash
go test -tags fts5 -count=1 ./internal/runtimeflags ./internal/common/config ./internal/profiles
go test -tags fts5 -count=1 ./internal/agentctl/server/config ./internal/agentctl/server/instance ./internal/agentctl/server/process ./internal/agentctl/server/opencodeplugin -run 'Test.*OpenCodePlugin'
go test -tags fts5 -count=1 ./internal/agent/agents ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl -run 'Test.*OpenCode'
```

From the repository root:

```bash
make -C apps/backend build-agentctl
make -C apps/backend build-agentctl-remote
```

From `apps/`:

```bash
pnpm --filter @kandev/opencode-plugin check:artifact
pnpm --filter @kandev/opencode-plugin test:conformance
pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts
```

When toggle metadata adds copy, update every supported catalog. From `apps/web/`:

```bash
pnpm run i18n:zh-hant
pnpm run i18n:check
pnpm run typecheck
pnpm run lint
```

Run `bash scripts/lint-go-changed` from the repository root with the exact changed
Go paths, following backend guidance. Record which helper targets were built.

## Exclusions and dependencies

Tasks 01 through 03 must pass before activation wiring. Do not promote defaults,
change OpenCode family adoption, or hot-upgrade retained peers. Do not add a
second feature registry. Do not use the generic ACP `PrepareEnvironment` as a
global configuration hook.

## Risks

The actual native executable can differ from a requested family. Use resolved
runtime evidence. Runtime configuration merging can reorder or replace user
plugins. Reuse Task 01's real configuration cases for this exact launch path.

## Results

Completed on 2026-10-29.

- Added `features.opencodePlugin` / `KANDEV_FEATURES_OPENCODE_PLUGIN`, off in
  prod, dev, and e2e, with restart-required metadata and the frontend all-off
  contract.
- The backend carries typed resolved family/source/version evidence through the
  existing lifecycle and instance configuration chain.
- Agentctl stages the embedded artifact by content hash, starts the private host,
  composes one additive `OPENCODE_CONFIG_CONTENT` layer with `hujson`, and
  preserves `OPENCODE_CONFIG`, user plugins, options, ordering, and existing
  fields. Malformed, conflicting, stripped, unsupported, non-task, probe,
  utility, and other-provider paths activate nothing.
- The launch uses executor-local files and loopback only. Retained agentctl
  processes keep their original integration and are never hot-installed.
- Tests cover supported/unsupported eligibility, idempotent preparation, every
  disabled path, reserved-ID and malformed configuration conflicts, Windows
  paths, private bootstrap permissions, cleanup, and profile defaults.
