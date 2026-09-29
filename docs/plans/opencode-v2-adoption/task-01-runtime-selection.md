---
id: "01-runtime-selection"
title: "Persist runtime family and resolve commands"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-V2-001
acceptance_criteria:
  - AC-AGENTS-OPENCODE-V2-001.1
  - AC-AGENTS-OPENCODE-V2-001.2
  - AC-AGENTS-OPENCODE-V2-001.4
  - AC-AGENTS-OPENCODE-V2-001.7
  - AC-AGENTS-OPENCODE-V2-001.9
  - AC-AGENTS-OPENCODE-V2-001.11
system_design:
  - ../../specs/agents/system-design/opencode-v2-adoption.md
---

# Task 01: Persist runtime family and resolve commands

## Summary

Make runtime selection durable and consistent before enabling migration.
Fresh installations select managed v2, while legacy installations retain their existing source and v1 choice.

## In scope

- Authoritative OpenCode settings record, validated legacy import, idempotent cleanup, and startup ordering.
- Same-family default reconciliation and use-default semantics without cross-family changes.
- Trusted v1/v2 definitions and exact pins; family-major guard in pin maintenance and catalogue resolution.
- Thread resolved source/package/version through registry, host utilities, lifecycle, preflight, installation, recovery, and interactive commands.
- Native version dispatch, managed priority over PATH and native metadata, and unsupported-major errors.
- Discovery/install readiness for a managed runtime without a native PATH executable; install execution prepares the exact managed package and private npm project prefix.
- Preserve other providers' fixed-package behavior and existing public API compatibility.

## Out of scope

- Migration submission/UI, live session-data conversion, and saved-session fallback changes.

## Acceptance

1. Fresh, native, legacy-selected, legacy-marker, ambiguous-data, and invalid-state cases select the designed source without silent v2 adoption; crash/restart preserves the result.
2. Every managed OpenCode command uses the selected exact package and appropriate arguments; native and interactive commands have their defined semantics.
3. A family-specific default change cannot change adoption; both pins and all other agents retain their intended maintenance behavior.

## Verification

Run from the repository root. Add the planned tests before implementation and establish failures first.

```bash
(cd apps/backend && go test ./internal/agent/managedruntime ./internal/agent/agents ./internal/agent/registry ./internal/agent/hostutility ./internal/agent/runtime/lifecycle ./internal/backendapp)
node --test scripts/update-agent-runtime-pins.test.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/managedruntime/selection.go`; new `opencode_selection.go` and `opencode_selection_test.go`.
- `apps/backend/internal/backendapp/managed_runtime_defaults.go` and its tests.
- `apps/backend/internal/agent/agents/opencode_acp.go`, `opencode_acp_test.go`, `managed_npm_runtime.go`, and `managed_npm_runtime_versions.json`.
- `apps/backend/internal/agent/registry/`, `hostutility/manager.go`, and `hostutility/managed_runtime_test.go`.
- `apps/backend/internal/agent/runtime/lifecycle/profile_resolver.go`, `native_binary.go`, managed-runtime startup/recovery helpers, and command tests.
- Existing executor install/preflight consumers discovered by `rg 'ManagedNPMRuntime|ManagedRuntimeVersion|InstallScript' apps/backend`.
- `scripts/update-agent-runtime-pins.mjs` and its tests; `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`.

## Dependencies

None. Task 02 must consume this resolver rather than introduce another source of truth.

## Risks

Native PATH detection and legacy evidence must not start ACP or modify user data.
Native-only `IsInstalled` detection must not hide a successfully installed managed runtime.
A generic selection reader that still filters against one fixed package can bypass the new record.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/agents/system-design/opencode-v2-adoption.md): Runtime definitions, Persistence, Bootstrap.
- Existing selection, default-reconciliation, host-utility, and remote preflight tests.
- [Plan test matrix](plan.md#tests).

## Results

Implemented install-wide OpenCode family/source persistence, fresh-install v2 bootstrap, legacy v1 import, same-family default reconciliation, exact family pins, and family-aware commands across host, executor, lifecycle, discovery, and passthrough paths.

- OpenCode-focused Go tests passed across `managedruntime`, `agents`, `registry`, `hostutility`, `lifecycle`, and `backendapp`.
- SQLite selection persistence passed after closing and reopening the settings database.
- `node --test scripts/update-agent-runtime-pins.test.mjs`: 9/9 passed.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`: passed.
- The shared Go package command reached unrelated Devin, Goose, and Muse installer tests that failed with no space left on the shared `/tmp` filesystem; the OpenCode-focused tests passed.

## Review follow-up (2026-09-28)

Fixed the review gaps in startup and the Settings install path. Startup validates an existing OpenCode selection before gathering native-runtime evidence. Settings now displays and runs the exact selected family, version, and source command: managed selections populate the selected npm execution cache, while native selections update their selected global package. The SQLite-backed regression starts from a fresh selection with no native executable and an empty npm cache, then confirms the selected v2 runtime is available after Install; it also covers managed v1 and native v1.

- `go test -race ./internal/agent/agents -run OpenCode -count=1`: passed.
- `go test -race ./internal/backendapp -run 'OpenCode|ManagedRuntimeDefault' -count=1`: passed.
- An earlier full `internal/agent/agents` race run failed three unrelated Devin, Goose, and Muse installer tests with `cat: write error: No space left on device` from shared `/tmp`. The full package suite was not rerun afterward; OpenCode-focused race tests passed when selected by name.

The Sprites preview exposed that the fresh-install job also needs to prepare the private managed npm project prefix before running its shell command. Install execution now resolves the selected command, replaces the internal prefix marker with its prepared temporary directory, and leaves the displayed command readable. The fake npm fixture rejects a missing prefix and confirms that the created directory is passed to the install. The controller regression runs through the production streaming runner with an empty HOME and explicit TMPDIR, then verifies the executed prefix exists under that temp root while HOME/.kandev remains absent.

- `go test -race ./internal/agent/settings/controller`: passed, including fresh managed v2, managed v1, and native v1 install jobs with the prefix existence check.
- The reported Sprites install failure was reproduced by the strengthened fixture before the fix and passed afterward. A live Sprites run was not available for post-fix verification.
