---
id: "01-normalize-npm-version-metadata"
title: "Normalize npm version metadata"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/runtime-updates.md"
---

# Task 01: Normalize npm version metadata

The preview currently fails on a Sprite because npm 12 returns a one-element
array for `npm view <package> versions dist-tags --json`, while the resolver
expects an object. Implement the smallest parser change after proving the
failure with a regression test.

## Acceptance

- Object-shaped npm metadata continues to resolve unchanged.
- npm 12's one-element array-shaped metadata resolves to a stable catalogue
  and latest version.
- Empty, multi-entry, malformed, and missing-latest metadata still fails
  closed through the existing preview error path.

## Verification

```bash
cd apps/backend && go test -run 'TestHostRuntimeUpdaterResolves|TestPreviewAgentUpdate' ./internal/agent/settings/controller ./internal/agent/settings/handlers
```

## Files likely touched

- `apps/backend/internal/agent/settings/controller/agent_update.go`
- `apps/backend/internal/agent/settings/controller/agent_update_test.go`
- `docs/specs/agents/runtime-updates.md`
- `docs/plans/managed-runtime-npm-metadata-compat/plan.md`
- `docs/plans/managed-runtime-npm-metadata-compat/task-01-normalize-npm-version-metadata.md`

## Dependencies

None.

## Parallelism

Sequential.

## Inputs

- The managed runtime preview contract and the npm metadata compatibility
  scenario in `docs/specs/agents/runtime-updates.md`.
- The confirmed Sprite reproduction: npm `12.0.2` returns a top-level array
  for the exact multi-field query.

## Output contract

Update the parser and regression coverage, record exact test results in this
task and `plan.md`, and report changed files, risks, and any remaining
environment-specific unknowns.

## Results

- Added a dedicated resolver regression test for npm 12's one-element array
  response and rejection coverage for empty, multi-entry, and malformed
  metadata.
- Updated the resolver to normalize object and one-element array payloads while
  preserving the existing exact npm argv and latest-version validation.
- `cd apps/backend && go test -run 'TestHostRuntimeUpdaterResolves|TestHostRuntimeUpdaterRejectsAmbiguousNPMRuntimeMetadata' ./internal/agent/settings/controller`: 7 tests passed.
- `cd apps/backend && go test ./internal/agent/settings/controller ./internal/agent/settings/handlers`: 350 tests passed.
- `cd apps/backend && go test -race ./internal/agent/settings/controller ./internal/agent/settings/handlers`: 350 tests passed.
- `cd apps/backend && golangci-lint run ./... --new-from-rev="032ea05bc8997028b1690f5f351939c83a6f77c2" --timeout=5m`: no issues.
- `git diff --check`: passed. No temporary diagnostic state was created.
