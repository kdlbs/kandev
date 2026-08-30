---
id: "01-backend-replay-catalog-filter"
title: "Backend: filter runtime config replay against current agent catalog"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/session-config-cross-agent-reconcile.md"
parallelism: sequential
---

# Task 01: Backend replay catalog filter

Filter persisted runtime config options against the currently running agent's
advertised option catalog on the startup/resume replay path, so options a prior
agent wrote are neither applied nor reported as startup failures.

## Context / root cause

`applyRuntimeSessionLayers`
(`apps/backend/internal/agent/runtime/lifecycle/session.go:602-651`) sanitizes
persisted options with `profileconfig.SanitizeConfigOptions`, which only strips
`model`/`mode`/blank keys. Prior-agent keys (`effort`, `thinking`) survive, are
sent via `SetConfigOption`, fail, and land in `failed` -> user warning
"Some session settings could not be applied at startup: effort, thinking."

The catalog-aware helper already exists and is used on the reset/restore path:
`sanitizeRuntimeConfigOptionsWithCatalog(options, execution.GetModelState())`
(`apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go:627-648`,
called at line 696). Both files are in package `lifecycle`.

## Steps (TDD)

1. Set `status: in_progress`.
2. Write the regression test FIRST and confirm it fails for the expected reason
   (stale keys reach `SetConfigOption` / appear in `failed`).
3. In `applyRuntimeSessionLayers`, replace the line
   `sanitizedOptions := profileconfig.SanitizeConfigOptions(runtimeConfigOptions)`
   (session.go:638) with
   `sanitizedOptions := sanitizeRuntimeConfigOptionsWithCatalog(runtimeConfigOptions, execution.GetModelState())`.
   Leave the `SetConfigOption` loop, `failed` handling, and `applyProfileSessionLayers`
   unchanged.
4. Run the targeted test; confirm it passes.
5. Reconcile files-touched, set `status: done`, update `plan.md` checkbox and
   `## Verification Results`.

## Acceptance

- Given persisted options `{reasoning, fast, effort, thinking}` and a current
  agent catalog advertising `{reasoning, fast, context}`, replay sends only
  `reasoning` and `fast`; `effort`/`thinking` are not sent and not in `failed`.
- Given an advertised key whose value the agent rejects, that key still appears
  in `failed` (real failure preserved).
- Given a catalog that is not yet known (unsettled), behavior matches today's
  restorable-set behavior (no regression for same-agent resume).

## Verification

```bash
cd apps/backend && go test -run TestApplyRuntimeSessionLayers ./internal/agent/runtime/lifecycle/...
```

If the new test uses a different name, substitute it in the `-run` filter. Also
run the changed-file linter before pushing (see backend AGENTS.md):

```bash
cd apps/backend && golangci-lint run ./internal/agent/runtime/lifecycle/... --new-from-rev="<base-sha>" --timeout=5m
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_test.go` (new test;
  put in a new `*_test.go` file if the existing one is near the 800-line limit)

## Dependencies

None.

## Parallelism

`parallel-safe` with task-02 (disjoint Go vs TS files). Default sequential.

## Inputs

- Spec: "Desired behavior" (Backend), "Failure modes", first and fourth scenarios.
- Plan: Backend section.
- Existing helper: `sanitizeRuntimeConfigOptionsWithCatalog`,
  `capturedRuntimeConfigOptionCatalog`, `isRestorableRuntimeConfigOption`
  (`manager_interaction.go:627-679`).

## Output contract

Summary, files changed, exact test command + result, blockers, risks, and
task/plan status update in the same conversation.

## Results

Done.

- Change: `applyRuntimeSessionLayers` (`session.go:638`) now calls
  `sanitizeRuntimeConfigOptionsWithCatalog(runtimeConfigOptions, execution.GetModelState())`
  in place of `profileconfig.SanitizeConfigOptions(runtimeConfigOptions)`. The
  `profileconfig` import is retained (still used at `session.go:585` and `:794`).
- Test (TDD): added `session_runtime_layers_test.go` with
  `TestApplyRuntimeSessionLayersFiltersUnadvertisedOptions`. Red first: stale
  `effort`/`thinking` reached `SetConfigOption` and landed in `failed`
  (`got [effort fast reasoning thinking]`). Green after fix.
- Commands:
  - `go test -run TestApplyRuntimeSessionLayersFiltersUnadvertisedOptions ./internal/agent/runtime/lifecycle/` → ok
  - `go test ./internal/agent/runtime/lifecycle/` → ok (69.9s, full package)
  - `gofmt -l` on changed files → clean
  - `golangci-lint run ./internal/agent/runtime/lifecycle/... --new-from-rev=1648cb8011b10cec0085c5fc3c2aea314404272e --timeout=5m` → 0 issues
- External side-effect boundaries: None (in-process WS mock only).
