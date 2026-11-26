---
created: 2026-11-25
status: complete
requirements:
  - REQ-AGENTS-OPENCODE-V2-001
system_design:
  - ../../specs/agents/system-design/opencode-v2-adoption.md
legacy_specs: []
---

# Implementation Plan: Preserve Managed OpenCode Availability Across Pin Bumps

## Overview

When Kandev has a managed OpenCode runtime selected (`family: v2` with `@opencode/cli`, or `family: v1` with `opencode-ai`) and a weekly pin update advances `managed_npm_runtime_versions.json`, `Store.reconcileOpenCodeDefault` advances `applied_default_version` on startup without pre-populating the new version's `_npx` cache directory. `OpenCodeACP.IsInstalled` then checks only `os.Lstat(<npm_cache>/_npx/<NpxExecutionCacheKey(package@newVersion)>)`, misses because only earlier versions of the same package are cached, and returns `Available: false`. `hostutility` sees `IsInstalled() == false`, logs `skipping host utility bootstrap: agent not installed`, marks `opencode-acp` as `not_installed`, and never runs the `npx` probe that would lazily cache the new pin.

This single-work-order fix adds local cache-scan helpers in `apps/backend/internal/agent/agents/opencode_acp.go`: when the exact-version `_npx/<key>` directory is missing, check whether any non-symlink execution directory in `<npm_cache>/_npx` contains a regular `node_modules/<spec.Package>/package.json`.

### Confirmed root cause

- On the live instance (`/root/.kandev/data/kandev.db`), `settings['managed_runtime.opencode.selection']` was reconciled on `2026-11-21T09:53:37Z` (after PR `#5404` bumped `@opencode/cli` from `2.3.0` to `2.3.2`) to `{"schema_version":1,"family":"v2","source":"managed","package":"@opencode/cli","applied_default_version":"2.3.2","revision":6}`.
- `/root/.npm/_npx/` contains cached `@opencode/cli` execution trees for `2.0.18` (`e47e4aac0151c97e`), `2.2.0` (`d43fa285f28e7f7a`), and `2.2.5` (`309f27d4ed984ef3`), each with `node_modules/@opencode/cli/package.json`, but not `2.3.2` (`5d65e79ca0179682`).
- `openCodeManagedRuntimeCached` in `apps/backend/internal/agent/agents/opencode_acp.go` checked only `5d65e79ca0179682`, causing `hostutility/manager.go` to skip bootstrap and publish `StatusNotInstalled` (`"agent not installed"`).

## Scope

### In scope

- Update `openCodeManagedRuntimeCached` in `apps/backend/internal/agent/agents/opencode_acp.go` with a small helper (`openCodePackageCachedInNpx`) that scans `<cacheRoot>/_npx` when the exact-version directory does not exist.
- Add regression tests in `apps/backend/internal/agent/agents/opencode_acp_test.go`.

### Out of scope

- Adding new platform-specific file-descriptor helpers in `internal/agent/managedruntime` (this check is read-only existence detection, not cache deletion).
- Changing `Store.reconcileOpenCodeDefault` or warming npm caches during startup bootstrap.
- Changing other managed npm agents or any frontend/UI files.
- Cross-family fallback (`opencode-ai` vs `@opencode/cli` remain isolated by package name).

## Technical approach

### Single-file read-only fallback (`apps/backend/internal/agent/agents/opencode_acp.go`)

- Keep the existing `os.Lstat(filepath.Join(cacheRoot, "_npx", managedruntime.NpxExecutionCacheKey(packageSpec)))` fast path so already-warmed pins stay O(1) and existing install-fixture tests (`TestOpenCodeInstallPreparesSelectedManagedRuntime`) continue to pass unchanged.
- When `os.Lstat` returns `os.ErrNotExist`, call a local helper `openCodePackageCachedInNpx(ctx, filepath.Join(cacheRoot, "_npx"), spec.Package)`:
  1. Check `ctx.Err()` before work and per entry so discovery sweep cancellation stops immediately.
  2. Reject a missing, non-directory, or symlinked `_npx` directory via `os.Lstat` (return `false, nil`).
  3. Call `os.ReadDir(npxDir)`. For each entry where `entry.IsDir()` (which is already `false` for symlinks and files):
     - `os.Lstat(filepath.Join(npxDir, entry.Name(), "node_modules", filepath.FromSlash(packageName), "package.json"))`.
     - If `err == nil && info.Mode().IsRegular()`, return `true, nil`.
  4. Skip individual invalid/missing/symlinked entries (existential predicate: *at least one* cached tree for `spec.Package` exists). Return `false, nil` if none match.
- Why this is the simplest sufficient solution: `@opencode/cli` (v2) and `opencode-ai` (v1) have distinct package names, so checking `node_modules/<spec.Package>/package.json` naturally enforces family isolation, works across Unix and Windows via standard `os` calls, stays immune to `@opencode/cli`'s multi-bin layout, and requires no new persisted state or startup network I/O.

### Compatibility matrix

| Provider / source | Transport and identity | Intended behavior | Verification evidence | Unsupported / negative case |
| --- | --- | --- | --- | --- |
| Managed OpenCode v2 (`@opencode/cli`) | ACP (`opencode-acp`), exact new pin on launch/probe | Reports installed when exact pin OR any cached `@opencode/cli` tree exists in `_npx`; `hostutility` probes and materializes the new pin | `opencode_acp_test.go` | Fresh empty cache, only v1 `opencode-ai`, or only symlinked/incomplete `_npx` entries → not installed |
| Managed OpenCode v1 (`opencode-ai`) | ACP (`opencode-acp`), exact new pin on launch/probe | Reports installed when exact pin OR any cached `opencode-ai` tree exists in `_npx` | `opencode_acp_test.go` | Fresh empty cache or only v2 `@opencode/cli` → not installed |
| Native OpenCode (`source: native`) | ACP (`opencode-acp`), PATH binary | Unchanged native detection first | Existing `opencode_acp_test.go` + `agent_install_test.go` | Unsupported native major continues to fail closed |
| Other managed providers | Existing PATH / `npx` detection | Unchanged | Existing suites untouched | Not applicable |

## Tests

| Acceptance criteria | File and test method |
| --- | --- |
| AC-AGENTS-OPENCODE-V2-001.4, .9, .13 | `apps/backend/internal/agent/agents/opencode_acp_test.go`: `TestOpenCodeACPManagedIsInstalledSurvivesPinBump` (managed v2 with bumped default and only prior `@opencode/cli` cached + native v1 on PATH + mixed-state invalid/unrelated/cross-family/symlinked siblings -> `Available: true`; managed v1 with prior `opencode-ai` cached -> `Available: true`) |
| AC-AGENTS-OPENCODE-V2-001.13 | `apps/backend/internal/agent/agents/opencode_acp_test.go`: `TestOpenCodeACPManagedIsInstalledRejectsFreshAndInvalidCaches` (fresh empty cache, cross-family-only cache, symlinked `_npx` or entry, and incomplete dir without `package.json` -> `Available: false, err: nil`; canceled context -> `context.Canceled`) |
| AC-AGENTS-OPENCODE-V2-001.4 | `apps/backend/internal/agent/settings/controller/agent_install_test.go`: existing `TestOpenCodeInstallPreparesSelectedManagedRuntime` confirms exact-key install flow still passes |

No rendered UI changes or new browser flows are introduced, so browser E2E tests are not required.

## Work orders

- [x] [Task 01: Recognize cached same-family managed OpenCode runtimes after pin bumps](task-01-managed-opencode-pin-bump-detection.md)

Dependency order: Task 01 (single sequential work order).

## Verification results

- RED confirmed: the new managed v1 and v2 pin-bump regression cases reported unavailable before the cache scan.
- `go test -trimpath -race ./internal/agent/agents -run 'TestOpenCode' -count=1` — passed.
- `go test -trimpath -race ./internal/agent/settings/controller -run 'TestOpenCodeInstallPreparesSelectedManagedRuntime' -count=1` — passed.
- `golangci-lint run ./internal/agent/agents/...` — passed.
- `make -C apps/backend build` — passed. The cross-platform agentctl builds printed expected warnings because no macOS signing tool is installed.
- Review follow-up corrected symlink fixtures to target execution directories, ordered invalid siblings before the valid tree, and simplified the cache scan.
- `go test -trimpath -race ./internal/agent/agents -run 'TestOpenCode' -count=1` — passed after the review fixes.
- `go test -trimpath -race ./internal/agent/settings/controller -run 'TestOpenCodeInstallPreparesSelectedManagedRuntime' -count=1` — passed after the review fixes.
- `golangci-lint run --allow-serial-runners ./internal/agent/agents/...` — passed after the review fixes; the standard invocation was blocked by another active lint run.
- `make -C apps/backend build` — passed after the review fixes; macOS signing tools were unavailable, so the expected unsigned-binary warnings were printed.

## Risks

- **Cleared npm cache (`npm cache clean --force`):** If the entire npm `_npx` cache is wiped, no `@opencode/cli` tree remains on disk and Kandev reports `not_installed` until re-installed (consistent with the fresh-install boundary).
- **Cold fetch during first post-bump probe:** Once `IsInstalled` returns `true`, `hostutility` probes `@opencode/cli@<new-pin>` within `probeTimeout` (60s); offline failures surface as probe `failed` instead of `not_installed`.
