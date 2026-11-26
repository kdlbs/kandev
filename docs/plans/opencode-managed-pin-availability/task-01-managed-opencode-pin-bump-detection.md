---
id: "01-managed-opencode-pin-bump-detection"
title: "Recognize cached same-family managed OpenCode runtimes after pin bumps"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-V2-001
acceptance_criteria:
  - AC-AGENTS-OPENCODE-V2-001.4
  - AC-AGENTS-OPENCODE-V2-001.9
  - AC-AGENTS-OPENCODE-V2-001.13
system_design:
  - ../../specs/agents/system-design/opencode-v2-adoption.md
---

# Task 01: Recognize cached same-family managed OpenCode runtimes after pin bumps

## Summary

Keep managed OpenCode runtimes detected as installed after a same-family default pin bump by adding a small `_npx` scan fallback in `openCodeManagedRuntimeCached` when the exact-version cache key is absent. Preserve the exact-key fast path and ensure unprepared fresh installs and cross-family caches continue to report unavailable without triggering downloads during discovery.

## In scope

- Add a local `openCodePackageCachedInNpx(ctx context.Context, npxDir, packageName string) (bool, error)` helper in `apps/backend/internal/agent/agents/opencode_acp.go` and call it from `openCodeManagedRuntimeCached` when `os.Lstat` on the exact `_npx/<key>` directory returns `os.ErrNotExist`.
- Add TDD regression tests in `apps/backend/internal/agent/agents/opencode_acp_test.go` covering pin-bump detection for managed v2 and managed v1, mixed-state `_npx` directories (valid sibling alongside unrelated, cross-family, incomplete, and symlinked siblings), fresh/invalid negative cases, and context cancellation.

## Out of scope

- Modifying `internal/agent/managedruntime`, `Store.reconcileOpenCodeDefault`, startup bootstrap, or any other agent's `IsInstalled`.
- Pre-warming npm caches during discovery or startup.
- Any frontend/UI changes.

## Acceptance

1. With a persisted managed OpenCode selection (`v2`/`@opencode/cli` or `v1`/`opencode-ai`) whose `applied_default_version` has advanced beyond the cached `_npx` version, `OpenCodeACP.IsInstalled` returns `Available: true` and `MatchedPath: "managed OpenCode runtime"` when at least one non-symlink `_npx` directory contains a regular `node_modules/<package>/package.json` for that package, even alongside unrelated, cross-family, incomplete, or symlinked sibling entries and native v1 on `PATH`.
2. With an empty `_npx` cache, a cache containing only the opposite family's package, a symlinked `_npx` directory, or only incomplete/symlinked entries, `OpenCodeACP.IsInstalled` returns `Available: false` with `err == nil` and triggers no download; caller context cancellation returns `ctx.Err()`.
3. Existing exact-key installation flows (`TestOpenCodeInstallPreparesSelectedManagedRuntime`) continue to pass unchanged.

## Verification

```bash
(cd apps/backend && go test -race ./internal/agent/agents -run 'TestOpenCode')
(cd apps/backend && go test -race ./internal/agent/settings/controller -run 'TestOpenCodeInstallPreparesSelectedManagedRuntime')
(cd apps/backend && golangci-lint run ./internal/agent/agents/...)
make -C apps/backend build
```

## Files likely touched

- `apps/backend/internal/agent/agents/opencode_acp.go`
- `apps/backend/internal/agent/agents/opencode_acp_test.go`

## Dependencies

None.

## Risks

- Clearing the entire npm `_npx` cache leaves no same-family tree on disk, returning `not_installed` until re-installed (intentional fresh-install boundary).

## Parallelism

`sequential`

## Inputs

- `docs/specs/agents/requirements/opencode-v2-adoption.md` (`REQ-AGENTS-OPENCODE-V2-001`, `AC-AGENTS-OPENCODE-V2-001.4`, `.9`, `.13`)
- `docs/specs/agents/system-design/opencode-v2-adoption.md` (`Runtime definitions and command resolution`)
- `apps/backend/internal/agent/agents/opencode_acp.go` (`IsInstalled`, `detectManagedRuntime`, `openCodeManagedRuntimeCached`)
- `apps/backend/internal/agent/settings/controller/agent_install_test.go` (`TestOpenCodeInstallPreparesSelectedManagedRuntime`)

## Results

- RED confirmed: managed v1 and v2 selections were unavailable when only an older same-family npm execution tree was cached.
- `go test -trimpath -race ./internal/agent/agents -run 'TestOpenCode' -count=1` — passed.
- `go test -trimpath -race ./internal/agent/settings/controller -run 'TestOpenCodeInstallPreparesSelectedManagedRuntime' -count=1` — passed.
- `golangci-lint run ./internal/agent/agents/...` — passed.
- `make -C apps/backend build` — passed.
- Review follow-up corrected both symlink fixtures to point to the execution tree, ensured mixed invalid entries are visited before the valid tree, and simplified context checks.
- `go test -trimpath -race ./internal/agent/agents -run 'TestOpenCode' -count=1` — passed after review fixes.
- `go test -trimpath -race ./internal/agent/settings/controller -run 'TestOpenCodeInstallPreparesSelectedManagedRuntime' -count=1` — passed after review fixes.
- `golangci-lint run --allow-serial-runners ./internal/agent/agents/...` — passed after review fixes. The standard lint command reported a concurrent linter, so the serial-runner override was used.
- `make -C apps/backend build` — passed after review fixes; macOS signing tools were unavailable, so the expected unsigned-binary warnings were printed.
