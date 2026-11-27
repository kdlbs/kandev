---
id: "02-record-validated-default"
title: "Record the validated fallback version, including the default"
status: done
wave: 2
depends_on: ["01-fallback-current-version"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-001
acceptance_criteria:
  - AC-AGENTS-RUNTIME-NOTIFY-001.1
  - AC-AGENTS-RUNTIME-NOTIFY-001.8
system_design:
  - ../../specs/agents/system-design/runtime-update-notifications.md
---

# Task 02: Record the Validated Fallback Version, Including the Default

## Summary

After a successful **Use Kandev default** in fallback mode, the dialog must show
the validated default as current instead of `Unknown`. Persist the version each
successful fallback activation validated. Derive the fallback current version
from that record while it equals the effective version.

The task must keep AC-AGENTS-RUNTIME-UPDATES-001.6 true: the default is never
persisted as an operator selection. The validation record is a separate key.

## Scope

- `managedruntime.Store`: add the validation record under
  `managed_runtime.validated.<agent>` with `{package, version}`. Add
  `SaveValidated`, and `GetValidated`, which applies the same trusted-package
  and stable-version checks as `Get`. Expose it through a small optional
  interface, so the controller type-asserts it the same way it does other
  optional seams.
- `runExactCandidate` (`agent_update_job.go`): for fallback jobs, after the
  activation commits successfully, record the probed target for both selection
  and `use_default`. A failed write is logged and never fails the job.
- `managedCurrentVersion` (`agent_update.go`): for fallback, return the record
  when it names the trusted package and equals the effective version. Otherwise
  return the active selection, or `""`. The preview and the job keep sharing
  this helper.

## Exclusions

- Non-fallback runtimes, the installed-agent catalogue, and update-status
  projections.
- Frontend code and copy.
- Default reconciliation at startup. A changed default no longer matches the
  record, so it is reported as unknown without clearing anything.
- The cosmetic `x → x` label on the completed job view.
- The inherited `npm_config_prefer_offline` issue.
- The native-host dot explanation.

## Implementation acceptance conditions

1. In fallback mode, after a successful `use_default` job with default `D`, a
   fresh preview reports current `D`. It reports operation `update` for a newer
   target and `up_to_date` for `D`. This holds after the job is evicted and with
   a new controller over the same settings store, which simulates a restart.
2. If the effective version differs from the record (for example, a new default
   after an upgrade, or no activation yet), the fallback current version is `""`
   with `repair`. A selection saved before the record existed is still reported
   as current.
3. A failed record write leaves the job `succeeded`, and the selection or
   deletion stays committed. `use_default` never saves a selection. Native host
   capabilities are never reported or published as the fallback version.

## TDD sequence

1. `managedruntime`: add store tests for the record round-trip, a
   package-mismatch read, invalid versions, and independence from `Delete`.
   Confirm they fail, then implement.
2. Controller: add
   `TestManagedFallbackPreviewReportsValidatedDefaultAsCurrent` (condition 1,
   including the restart case) and the mismatch and write-failure cases
   (conditions 2 and 3). Confirm they fail on `CurrentVersion == ""` and
   `repair`, then implement.
3. Rerun the Task 01 fallback tests, the existing `use_default` tests, and the
   package suites.

## Verification

- `cd apps/backend && go test ./internal/agent/managedruntime/`
- `cd apps/backend && go test -run 'ManagedFallback|AgentUpdate|PreviewAgentUpdate|UseDefault' ./internal/agent/settings/controller/`
- `cd apps/backend && go test -race ./internal/agent/settings/... ./internal/agent/managedruntime/...`
- `make -C apps/backend fmt lint`
- `python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all`

## Files likely touched

- `apps/backend/internal/agent/managedruntime/selection.go`
- `apps/backend/internal/agent/managedruntime/selection_test.go`
- `apps/backend/internal/agent/settings/controller/agent_update.go`
- `apps/backend/internal/agent/settings/controller/agent_update_job.go`
- `apps/backend/internal/agent/settings/controller/runtime_managed_fallback_test.go`
  (or a new test file if the line limit requires it)
- the recovery selection-store test fake, which gains the record methods

## Dependencies

Task 01 (done, rebased onto `origin/main` at `824dff704`).

## Risks

- The test fake for the selection store must implement the optional
  interface. Otherwise the controller silently falls back to the Task 01
  behavior, and tests would pass for the wrong reason.
- Keep `alreadyActiveHealthyUpdate` skipped for fallback.

## Results

- RED (store): `validated_test.go` failed to build because `SaveValidated`,
  `GetValidated`, and `validatedKey` did not exist.
- RED (controller): `TestManagedFallbackPreviewReportsValidatedDefaultAsCurrent`
  failed after a successful `use_default` with `CurrentVersion: ""` and
  `ActiveVersion: ""`, which matches the user's screenshot.
- GREEN:
  - `managedruntime.Store` gains `GetValidated` and `SaveValidated` under
    `managed_runtime.validated.<agent>`, sharing record validation with
    `Get` and `Save`. A compile-time check pins the `ValidatedVersionStore`
    implementation.
  - `runExactCandidate` records the probed target for fallback jobs after
    activation commits; a write failure is only logged.
  - `managedCurrentVersion` returns the record while it names the trusted
    package and equals the effective version. Otherwise it returns the active
    selection.
- Mismatch and record-failure tests pass. The Task 01 fallback test still
  covers selections saved before the record existed.
- Commands passed:
  - the `managedruntime` tests;
  - the targeted controller tests, plus the `settings`, `managedruntime`,
    `agents`, `hostutility`, and `backendapp` packages;
  - `-race` on `settings` and `managedruntime`;
  - `go vet`, `gofmt`, and `golangci-lint --new-from-rev` (0 issues);
  - the spec validators.
- `make -C apps/backend fmt` also reformatted four files inherited from main
  that are unrelated to this task. They were restored and are not part of this
  change.
- PR review follow-up:
  - The update job sets `CurrentVersion` again before the selection read can
    fail, so a failed non-fallback job keeps the host version
    (`TestAgentUpdateJobKeepsHostCurrentVersionWhenSelectionReadFails`). A
    failed fallback read stays unknown, because the effective version is
    unknown.
  - The frontmatter now lists only the requirement that the linked design
    declares. AC-AGENTS-RUNTIME-UPDATES-001.6 is kept as a stated constraint.
- Merged `main` after OpenCode v2 adoption (#4014):
  - The fallback derivation applies on the selection-store path.
  - The new OpenCode selection path reports host capabilities, unchanged from
    `main`, because it is not a managed fallback.
  - Read failures now come wrapped from `activeRuntimeSelection`.
  - The fallback tests use `opencode-ai` major-1 versions (1.18.30, the 1.18.32
    default, and 1.19.0), because the catalogue now filters by family major.
  - With the fallback derivation disabled, the three positive fallback tests
    fail.
