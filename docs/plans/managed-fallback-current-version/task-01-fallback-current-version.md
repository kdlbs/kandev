---
id: "01-fallback-current-version"
title: "Report validated fallback version as current"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-001
acceptance_criteria:
  - AC-AGENTS-RUNTIME-NOTIFY-001.1
  - AC-AGENTS-RUNTIME-NOTIFY-001.8
system_design:
  - ../../specs/agents/system-design/runtime-update-notifications.md
---

# Task 01: Report Validated Fallback Version as Current

## Summary

In managed-fallback mode, derive the fallback's current version from the
validated active selection for both the update preview and the job's operation
classification. After a successful repair, a reopened dialog then shows
`<active> → <target>` with the correct operation instead of
`Unknown → <target>` with **Repair runtime**.

## Scope

- Add one controller helper that returns the managed current version:
  - non-fallback: the host capability observation;
  - fallback: the active selection, or `""` when there is none.
- Use it in `previewAgentUpdate`
  (`apps/backend/internal/agent/settings/controller/agent_update.go`) and in
  `AgentUpdateJobStore.run` (`agent_update_job.go`), after the selection read
  and before classification.
- Update the existing fallback test assertion, and add a reopen regression
  test.

## Exclusions

- `buildRuntimeUpdateDTO` and runtime update-status projections.
- Frontend code, copy, and locales.
- Native host capability publication.
- The no-selection case. It stays unknown with `repair`.

## Implementation acceptance conditions

1. With a native host binary on PATH and a saved fallback selection `8.0.0`,
   the preview for target `9.0.0` reports `current_version: "8.0.0"` and
   operation `update`. Target `8.0.0` reports `up_to_date`. After a fallback
   job activates `9.0.0` and terminates, a fresh preview for `9.0.0` reports
   current `9.0.0` and `up_to_date`.
2. With no saved selection in fallback mode, the preview current version stays
   `""` and the operation stays `repair`. Native host capabilities are never
   reported as the fallback current version.
3. Non-fallback preview and job behavior is unchanged, and the existing
   controller tests pass.

## TDD sequence

1. Add `TestManagedFallbackPreviewReportsValidatedSelectionAsCurrent`. Run it
   and confirm that it fails on `CurrentVersion == ""` and operation `repair`.
2. Update the assertion in `TestNativeHostPreservesManagedFallbackSelectionAndRecovery`
   that expects an empty fallback preview current version while selection
   `8.0.0` exists.
3. Implement the helper and wire both call sites. Rerun the tests until they
   are green.

## Verification

- Targeted:
  `cd apps/backend && go test -run 'ManagedFallback|AgentUpdate|PreviewAgentUpdate' ./internal/agent/settings/controller/`
- Package:
  `cd apps/backend && go test ./internal/agent/settings/controller/ ./internal/agent/managedruntime/`
- Lint and format:
  `make -C apps/backend fmt lint`
- Specs:
  `python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all`

## Files likely touched

- `apps/backend/internal/agent/settings/controller/agent_update.go`
- `apps/backend/internal/agent/settings/controller/agent_update_job.go`
- `apps/backend/internal/agent/settings/controller/runtime_managed_fallback_test.go`

## Dependencies

None.

## Risks

- `run` reads the selection after it computes `currentVersion` today. Reorder
  the reads so classification uses the derived value without a second store
  read.
- Keep `alreadyActiveHealthyUpdate` skipped for fallback. Its native-capability
  check must not be satisfied by the selection.

## Results

- RED: `TestManagedFallbackPreviewReportsValidatedSelectionAsCurrent` failed with
  `CurrentVersion: ""`, `ActiveVersion: 8.0.0`, and `Operation: repair`. This
  reproduces the reported dialog.
- GREEN: `managedCurrentVersion` in `agent_update.go` is shared by
  `previewAgentUpdate` and `AgentUpdateJobStore.run`. The fallback observation
  is the active selection, and the job sets `CurrentVersion` from it before
  classification.
- The fallback preview assertion in
  `TestNativeHostPreservesManagedFallbackSelectionAndRecovery` now expects the
  validated selection `8.0.0`. The native `1.0.0` observation is never reported
  and never published.
- Commands passed:
  - targeted `go test -run 'ManagedFallback|AgentUpdate|PreviewAgentUpdate'`;
  - the package tests for `settings`, `managedruntime`, `agents`, and
    `hostutility`;
  - `-race` on the update tests;
  - `gofmt`;
  - `golangci-lint --new-from-rev` (0 issues);
  - the spec validators.
