---
id: "02-status-integration"
title: "Native results in status, cache, preview, and notifications"
status: done
wave: 2
depends_on:
  - "01-native-check-contract"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-NATIVE-CHECK-001
  - REQ-AGENTS-RUNTIME-NATIVE-CHECK-002
acceptance_criteria:
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-001.1
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-001.3
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-001.7
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-001.8
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-002.1
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-002.2
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-002.3
system_design:
  - ../../specs/agents/system-design/runtime-native-update-checks.md
  - ../../specs/agents/system-design/runtime-native-update-check-omp-context.md
---

# Task 02: Native results in status, cache, preview, and notifications

## Summary

Route agents that declare a native check through the runner instead of the npm
stable lookup. Cache the results in the shared status cache, project them on
status and preview DTOs, and invalidate them when a harness job ends. Existing
notification delivery then reports native availability without notifier
changes.

## In scope

- `runtimeUpdateStatusTarget.nativeCheck` for every `NativeUpdateCheckAgent`
  (independent of `HarnessUpdateAgent`), lookup grouping by key,
  `nativeUpdateCheckCacheKey` with generation and context fingerprint, native
  cache-entry fields, TTL selection, runtime-mismatch normalization, and DTO
  mapping, as described in
  [Status integration and cache](../../specs/agents/system-design/runtime-native-update-checks.md#status-integration-and-cache).
- Shared single-flight lookup and semaphore use with flight key equal to the
  generation-bearing cache key, generation-fenced cache writes, and
  `SetNativeUpdateCheckRunner`.
- `nativeCheckLifetime` admission gate, as described in
  [Execution](../../specs/agents/system-design/runtime-native-update-checks.md#execution):
  an open lifetime at construction, and per-execution `wg.Add` under
  `lifetime.mu` before the slot wait. Stop sets `closed`, cancels, and waits.
  `StartRuntimeUpdateBackground` installs a fresh lifetime. A canceled check
  returns `unknown` / `cancelled` to waiting subscribers and is never cached.
- `InvalidateNativeUpdateCheck(agentName)` and the
  `SetStatusInvalidator(func(RuntimeStatusInvalidation))` contract in
  `agent_update_job.go`. Migrate every terminal call site.
  `finishActivatedWithRefreshError` (via `finishActivated`) and
  `finishRefresh` keep their pinned-job conditions. `finishRefresh` sends on
  every `self_update` outcome. `finishFailed`, which does not invalidate
  today, sends after a failed `self_update` job only. For `self_update` jobs
  the invalidation runs before `s.mu` is taken for the terminal transition,
  so it precedes terminal status, claim release, `complete`, and the broadcast.
  No job-store lock is held during the call. `controller.go` wires
  `invalidateRuntimeUpdateStatusForJob`.
- Registry coverage rejects a native check on an active managed npm runtime.
- Native preview branch without `resolveHarnessLatest`, as described in
  [Preview and job](../../specs/agents/system-design/runtime-native-update-checks.md#preview-and-job).
- DTO fields: `check_source`, `update_channel`, and `check_reason` on status,
  plus `check_source`, `check_state`, `latest_version`, `update_channel`, and
  `check_reason` on preview.
- Retarget OMP-specific stable-reference tests in
  `agent_update_harness_test.go` to a non-native harness fixture agent.

## Out of scope

- Web types and rendering (Task 03).
- Notification service changes. Existing delivery is reused unchanged.
- Changing `runHarness` update and probe behavior beyond terminal
  invalidation.

## Acceptance

- `agent_update_native_check_test.go` proves that a native agent never calls
  the registry resolver. It also proves that a canary latest such as
  `18.22.0-canary.3` is reported, that runtime mismatch produces `unknown`,
  and that disabled and unavailable agents do not run checks, in status and in
  direct preview requests. A discovery-unavailable fixture with a runnable
  executable is used, and preview returns `unknown` / `unavailable` without
  starting the runner. Managed and
  non-native harness statuses do not change. A checker-only fixture agent gets
  native status while its `management`, install action eligibility, and
  `auto_update_supported` are unchanged.
- Cache tests use the injected clock and fake runner. Recognized results are
  reused for six hours and failures for fifteen minutes. Concurrent
  `ListAgentUpdateStatuses` calls share one run. A changed ACP version,
  resolved path, or context fingerprint reruns the check. The fingerprint cases
  keep the executable path and version unchanged. They edit a fixture
  `config.yml` under the default, named-profile (`OMP_PROFILE`), and
  `PI_CONFIG_FILES` overlay locations; create a file in `<cwd>/.omp`; and edit
  `<cwd>/.omp/config.yml` inside a directory with more than 256 entries. They
  also create and remove the ownership `manager.json` and edit `.npmrc`. For
  `OMP_PROFILE` set to `''` and then `default`, with `PI_PROFILE=work` and
  `PI_CODING_AGENT_DIR=$HOME/.omp/profiles/work/agent`, editing
  `$HOME/.omp/agent/config.yml` reruns the check. Editing `$HOME/.env`,
  `<agentDir>/.env`, or `<cwd>/.env` also reruns it, as does editing
  `$HOME/.omp/profiles/work/.env` with `OMP_PROFILE=work` in the process
  environment. Successful and failed terminal harness jobs clear the agent's
  entries. Managed activation still invalidates only its package. In a barrier
  test, check A blocks, the agent is invalidated, a new request starts, and A
  is released; only the fresh check fills the cache. A terminal-path barrier
  test, for both success and failure, holds pre-job check A in flight and runs
  the harness job to its terminal state. As soon as `Get` reports the job
  terminal, it requests status and proves the request neither reuses nor joins
  A. All of these tests pass under `-race`.
- Lifetime barrier tests cover these cases. A request paused just before
  admission while stop runs either registers before `wg.Wait` or returns
  `cancelled` without a process. With the semaphore saturated, stop releases
  the queued slot waiters as `cancelled`. During stop, a live subscriber
  receives `cancelled`, nothing is cached, and stop returns only after the
  check ends. After stop and restart, a new lifetime runs checks again.
  `TestNativeCheckNotifiesOnlyAvailable` proves that the notifier receives the
  native latest version through `publishRuntimeStatus`. `up_to_date` and
  `unknown` do not notify. Preview returns native fields with an empty
  `stable_latest_version`, and approval still runs `omp update`.

## Verification

```bash
(cd apps/backend && go test -tags fts5 -count=1 ./internal/agent/agents ./internal/agent/registry ./internal/agent/settings/... ./internal/notifications/...)
(cd apps/backend && go test -tags fts5 -race -count=1 ./internal/agent/settings/controller -run 'NativeCheck|RuntimeStatus|HarnessUpdate|ListAgentUpdateStatuses|RuntimeBackground')
(cd apps/backend && GOOS=windows CGO_ENABLED=0 go vet ./internal/agent/settings/...)
(cd apps/backend && golangci-lint run ./internal/agent/agents/... ./internal/agent/settings/...)
```

## Files likely touched

- `apps/backend/internal/agent/settings/controller/agent_update_status.go`
- `apps/backend/internal/agent/settings/controller/native_update_check_status.go` (new)
- `apps/backend/internal/agent/settings/controller/agent_update_native_check_test.go` (new)
- `apps/backend/internal/agent/settings/controller/agent_update_harness_preview.go`
- `apps/backend/internal/agent/settings/controller/agent_update_job.go` (invalidator contract, terminal paths)
- `apps/backend/internal/agent/registry/runtime_update_coverage_test.go`
- `apps/backend/internal/agent/settings/controller/agent_update_harness_test.go`
- `apps/backend/internal/agent/settings/controller/runtime_update_background.go`
- `apps/backend/internal/agent/settings/controller/controller.go` (invalidator wiring and fields)
- `apps/backend/internal/agent/settings/dto/dto.go`

## Dependencies

Task 01.

## Risks

- `ListAgentUpdateStatuses` is near the Go function-length and complexity
  limits. Extract the lookup grouping and DTO mapping into helpers.
- Executable resolution happens before the cache lookup on every status
  request. It must remain a cheap `exec.LookPath` and never start a process.
- The status lifetime context is shared mutable controller state. Guard it with
  `runtimeBackgroundMu` and clear it on stop, so a later restart does not keep
  a canceled context.

## Parallelism

`sequential`

## Inputs

- Task 01 results and exported types.
- [System design](../../specs/agents/system-design/runtime-native-update-checks.md)
- Existing tests: `agent_update_status_test.go`,
  `runtime_status_concurrency_test.go`, `agent_update_harness_test.go`, and
  `runtime_update_notifications_test.go`.

## Results

Done on 2026-11-17.

Implementation notes:

- `native_update_check_status.go` owns `nativeCheckLifetime`, the eligibility
  guard, lookup preparation (resolve, fingerprint, generation, key), the
  single-flight execution, normalization, TTL selection, the generation-fenced
  write, `InvalidateNativeUpdateCheck`, and the shared status/preview view.
  `ListAgentUpdateStatuses` delegates grouping to `runtimeUpdateStatusEntries`
  and DTO mapping to `runtimeUpdateStatusDTO`.
- The lifetime pointer is guarded by `runtimeBackgroundMu`; stop closes the
  current lifetime (cancel and wait) and start installs a fresh one only when
  the current lifetime is closed.
- An ineligible native target reports `check_source: "native"`,
  `check_state: "unknown"`, and `check_reason: "unavailable"` in status as well
  as preview, without resolving, fingerprinting, or running anything.
- The registry coverage test also pins `omp-acp` as the only built-in native
  check declaration.
- `TestAgentUpdateStatusEndpointIsReadOnly` (handlers) now runs with an empty
  `PATH`, so the status endpoint cannot start a host agent CLI under test.
- Mutation check: removing the `finishFailed` invalidation failed
  `TestNativeCheckInvalidatedByTerminalHarnessJob/failure`,
  `TestNativeCheckTerminalJobPublishesAfterInvalidation/failed`, and
  `TestAgentUpdateJobStatusInvalidationContract`; the code was restored.

| Command | Result |
| --- | --- |
| `(cd apps/backend && go test -tags fts5 -count=1 ./internal/agent/agents ./internal/agent/registry ./internal/agent/settings/... ./internal/notifications/...)` | All packages `ok` (controller 5.702s, handlers 10.485s, notifications/* ok) |
| `(cd apps/backend && go test -tags fts5 -race -count=1 ./internal/agent/settings/controller -run 'NativeCheck\|RuntimeStatus\|HarnessUpdate\|ListAgentUpdateStatuses\|RuntimeBackground')` | `ok .../settings/controller 5.304s` |
| `(cd apps/backend && GOOS=windows CGO_ENABLED=0 go vet ./internal/agent/settings/...)` | `controller`, `dto`, `catalog`, `cliflags`, `models`, `profileconfig`: ok. `handlers` and `store` fail while compiling the dependency `internal/task/repository/sqlite` (`undefined: sqlite3.Error` without cgo). The same failure reproduces with `go vet ./internal/task/repository/sqlite`; that package and `settings/store` are unchanged from `ea6af0d500`, so this is pre-existing |
| `(cd apps/backend && golangci-lint run ./internal/agent/agents/... ./internal/agent/settings/...)` | `0 issues.` |
