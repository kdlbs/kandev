---
id: "01-canonical-path-expectations"
title: "Make plugin root expectations portable"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-003
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-003.2
  - AC-AGENTS-COPILOT-PLUGINS-003.3
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
---

# Task 01: Canonical path expectations

## Summary

Correct test expectations for valid canonical plugin roots on symlinked
temporary directories. Production directory and manifest validation remain
unchanged.

## In scope

- Reproduce the three existing failures on macOS.
- Canonicalize expected roots with checked `filepath.EvalSymlinks` results.
- Keep original installation metadata and symlink fixtures so the tests still
  exercise production resolution rather than pre-normalizing every input.

## Out of scope

Production code, broad test rewrites, runtime qualification, new dependencies,
and weakening path/symlink/manifest rejection.

## Acceptance

1. `TestInstallationMetadata`, `TestPluginRootValidation`, and
   `TestLivePluginRoot` pass with macOS's symlinked temp root and ordinary roots.
2. Missing root, traversal, volume root, manifest mismatch, and symlinked manifest
   assertions remain effective.
3. Only focused test code changes; no production canonicalization is removed.

## Verification

Run from the worktree root. The first command is the recorded pre-fix Red and
must be rerun before editing. Then run the same command Green and the package.

```bash
(cd apps/backend && go test -trimpath -count=1 ./internal/agent/copilotplugins -run 'Test(InstallationMetadata|PluginRootValidation|LivePluginRoot)$')
(cd apps/backend && go test -trimpath -count=1 ./internal/agent/copilotplugins)
bash scripts/lint-go-changed apps/backend/internal/agent/copilotplugins/copilotplugins_test.go
git diff --check
```

Record the actual OS/architecture and both command outcomes. A Linux-only pass
does not replace the known macOS regression check.

## Files likely touched

- `apps/backend/internal/agent/copilotplugins/copilotplugins_test.go`.

## Dependencies

None.

## Risks

Cleaning lexical paths is not equivalent to resolving symlinks. Do not replace
`EvalSymlinks` with string-prefix rewriting or a platform-specific `/var` rule.

## Parallelism

`parallel-safe`: this work order's test file is disjoint from WO-02.
Execute sequentially by default; do not run it alongside WO-04's later test edits.

## Inputs

- [Parent design: identity and installation adapter](../../specs/agents/system-design/copilot-profile-plugins.md#identity-and-installation-adapter).
- `validatePluginDirectory` in `copilotplugins/installation.go`.
- `.agents/skills/tdd/references/backend-tests.md`, filesystem fixture guidance.

## Results

Completed 2026-10-28 on `darwin/arm64`.

- `go env GOOS GOARCH`: `darwin`, `arm64`.
- Red: the documented focused selector exited 1 with all three expected
  canonical-root comparison failures (`/var` versus `/private/var`).
- Green: the same selector exited 0; all three regression cases passed.
- `go test -trimpath -count=1 ./internal/agent/copilotplugins`, from
  `apps/backend`: exited 0.
- `bash scripts/lint-go-changed apps/backend/internal/agent/copilotplugins/copilotplugins_test.go`:
  exited 0, zero issues.
- `git diff --check`: exited 0.

Only the owned test file changed. Checked `filepath.EvalSymlinks` values now
form the expected roots; metadata inputs and rejection fixtures remain intact.
No production path validation changed.
