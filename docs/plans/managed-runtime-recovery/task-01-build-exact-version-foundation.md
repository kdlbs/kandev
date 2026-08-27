---
id: "01-build-exact-version-foundation"
title: "Build exact-version foundation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 01: Build exact-version foundation

## Acceptance

- A typed install-wide store reads and writes one trusted package and active
  exact version per built-in agent through `internal/system/settings`; a
  package mismatch cannot apply an old version to replacement metadata.
- Strict stable SemVer parsing rejects prereleases, tags, package specs, and
  malformed values and provides deterministic descending ordering.
- `ManagedNPMRuntimeSpec` builds launch, preparation, and execution-cache keys
  from exact `package@version` while preserving the empty-version legacy path.
- Each managed agent honors an internal exact-version command option for ACP
  commands; callers still cannot choose package identity or ACP arguments.

## TDD sequence

1. Add failing tests for selection-store round trips, missing values, invalid
   stored values, and separate-agent writes.
2. Add failing table tests for stable version validation and ordering.
3. Add failing exact command/key tests, including scoped npm packages.
4. Implement the typed store and helpers, then minimally update the five
   managed agent command builders.
5. Run focused packages, then backend lint for touched code.

## Files likely touched

- `apps/backend/internal/agent/managedruntime/selection.go`
- `apps/backend/internal/agent/managedruntime/selection_test.go`
- `apps/backend/internal/agent/managedruntime/versions.go`
- `apps/backend/internal/agent/managedruntime/versions_test.go`
- `apps/backend/internal/agent/agents/agent.go`
- `apps/backend/internal/agent/agents/managed_npm_runtime.go`
- `apps/backend/internal/agent/agents/managed_npm_runtime_test.go`
- `apps/backend/internal/agent/agents/{claude_acp,codex_acp,opencode_acp,copilot_acp,gemini}.go`
- `apps/backend/go.mod`
- `apps/backend/go.sum`

## Verification

```bash
make -C apps/backend test ARGS='./internal/agent/managedruntime ./internal/agent/agents'
make -C apps/backend lint
```

## Risks

- Use a key per trusted agent ID instead of a shared read-modify-write map.
- Do not use the update subsystem's Kandev-specific loose SemVer parser.
- Do not add a user-provided package or command field.

## Output contract

Record RED/GREEN evidence, files, checks, and risks in Results. Update this task
and `plan.md` status in the same implementation conversation.

## Results

RED covered the new selection, strict-version, exact-command, scoped-package,
and per-agent isolation contracts before their implementations were complete.
GREEN verification:

- `rtk go test ./internal/agent/managedruntime ./internal/agent/agents ./internal/agent/hostutility ./internal/agent/runtime/lifecycle ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/backendapp` — 2,321 tests passed across 7 packages.
- `rtk make -C apps/backend lint` — 0 issues.

The implementation adds the typed install-wide selection store, strict stable
catalogue helpers, exact `package@version` command/key builders, and exact ACP
command support for all five managed npm agents. The remaining risk is the
normal npm availability boundary: persisted selection prevents version drift,
but does not promise offline artifact retention.
