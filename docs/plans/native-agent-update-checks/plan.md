---
created: 2026-11-17
status: in_progress
requirements:
  - REQ-AGENTS-RUNTIME-NATIVE-CHECK-001
  - REQ-AGENTS-RUNTIME-NATIVE-CHECK-002
system_design:
  - ../../specs/agents/system-design/runtime-native-update-checks.md
  - ../../specs/agents/system-design/runtime-native-update-check-omp-context.md
legacy_specs: []
---

# Implementation Plan: Native agent update checks

## Overview

Add an optional per-agent native update check, declare `omp update --check`
for OMP, and use its normalized result in the existing runtime status, cache,
notification, Settings, and update-dialog paths. Installation remains the
existing user-approved `omp update` maintenance job.

The work proceeds bottom-up because each layer depends on the previous one:

1. The capability contract, OMP adapter, and bounded runner can be tested
   without the status service.
2. The status, cache, preview, and invalidation integration uses that contract
   and changes the backend wire format.
3. Web presentation, locales, E2E tests, and public docs use the new wire
   fields.

This package follows the maintainer direction in
[kdlbs/kandev#5156](https://github.com/kdlbs/kandev/issues/5156#issuecomment-6920992319)
and [ADR-2026-11-17](../../decisions/2026-11-17-native-agent-update-checks.md).
It is based on `upstream/main` `ea6af0d500` on branch
`feature/omp-native-update-check`. The rejected automatic in-place update
implementation on `feature/add-automatic-update-c42` is not carried forward.

## Scope

### In scope

- `NativeUpdateCheckAgent` / `NativeUpdateCheckSpec` capability, result type,
  and closed reason set in `internal/agent/agents`.
- `ParseOmpUpdateCheck` and OMP's `omp update --check` declaration.
- A bounded direct-argv runner with a 30-second timeout, a 64 KiB output limit
  that stops the run, process-tree containment, shutdown cancellation, and the
  trusted updater's environment. It uses a shared `internal/agent/proctree`
  package extracted from `copilotplugins`.
- An update-configuration context fingerprint (declared environment values and
  configuration-file metadata) in the native cache identity.
- Native lookup keys, TTL selection, single-flight lookup, runtime-mismatch
  normalization, terminal-job invalidation, and preview fields in
  `internal/agent/settings/controller`.
- Status and preview DTO fields: `check_source`, `update_channel`, and
  `check_reason` on status, plus `check_state` and `latest_version` on preview.
- Settings row and update-dialog presentation on desktop and phone, all seven
  locales, Playwright coverage, and the OMP section of
  `docs/public/agents-and-profiles.md`.

### Out of scope

- Automatic or scheduled installation, rollback, version selection, or channel
  switching.
- Native checks in executor environments other than the Kandev host.
- Native checks for custom agents or other built-in agents. This package adds
  the extension point and the OMP declaration only.
- Any change to the managed npm runtime, GitHub release lookup, or
  non-native harness behavior.

## Technical approach

### Capability and adapter (Task 01)

- `apps/backend/internal/agent/agents/native_update_check.go`: types
  `NativeUpdateCheckSpec`, `NativeUpdateCheckContext`,
  `NativeUpdateCheckAgent`, `NativeUpdateCheckOutput`,
  `NativeUpdateCheckResult`, `NativeUpdateCheckState`, and
  `NativeUpdateCheckReason`. The constants are the closed reason set from the
  design, including `cancelled`.
- `apps/backend/internal/agent/agents/omp_update_check.go`:
  `ParseOmpUpdateCheck`, with ANSI stripping, line-marker recognition, and
  `semver.NewVersion` validation without build metadata. It also contains
  `ompUpdateCheckContext` (environment keys and default configuration file
  paths).
- `apps/backend/internal/agent/agents/omp_acp.go`: `NativeUpdateCheck()`
  returning `NewCommand("omp", "update", "--check")`, the parser, and the
  context. Add the interface assertion.
- `apps/backend/internal/agent/proctree/` (new): `Prepare`, `Capture`, and
  `Tree.Kill` for Unix and Windows. These move from
  `copilotplugins/process_tree_{unix,windows}.go`, and `copilotplugins`
  migrates to them.
- `apps/backend/internal/agent/settings/controller/native_update_check_runner.go`:
  `nativeUpdateCheckRunner` interface, `execNativeUpdateCheckRunner`,
  `nativeUpdateCheckTimeout`, process-tree containment, and a 64 KiB shared
  output budget that cancels on overflow. It also uses `filteredInstallEnv()`,
  `exec.LookPath`, and `WaitDelay`. It returns output, exit code, resolved
  path, and the timeout, cancellation, start, or limit reason.

### Status integration (Task 02)

- `agent_update_status.go`: `runtimeUpdateStatusTarget.nativeCheck` for every
  `NativeUpdateCheckAgent`; group lookups by key; `nativeUpdateCheckCacheKey`
  with generation and context fingerprint; native entry fields; TTL selection;
  runtime-mismatch normalization; DTO mapping.
- `native_update_check_status.go`: native lookup through
  `runtimeUpdateStatusFlight` and the shared semaphore; generation-fenced cache
  writes; context fingerprinting; background-lifetime cancellation through
  `context.AfterFunc`; in-flight `sync.WaitGroup`; `SetNativeUpdateCheckRunner`;
  and `InvalidateNativeUpdateCheck`.
- `runtime_update_background.go`: publish the worker context as the status
  lifetime context. On stop, cancel it, wait for in-flight native checks, and
  clear it.
- `agent_update_harness_preview.go`: native branch without
  `resolveHarnessLatest`.
- `agent_update_job.go`: `SetStatusInvalidator(func(RuntimeStatusInvalidation))`.
  Migrate every terminal call site. `finishActivatedWithRefreshError` and
  `finishRefresh` keep their pinned-job conditions. `finishRefresh` sends on
  every `self_update` outcome. `finishFailed`, which does not invalidate
  today, sends after a failed `self_update` job only. For `self_update` jobs,
  the invalidation runs before the terminal state or claim release is
  published, with no job-store lock held.
- `controller.go`: wire `invalidateRuntimeUpdateStatusForJob` to call
  package and native invalidation.
- `dto/dto.go`: `AgentUpdateCheckSource`, `UpdateChannel`, `CheckReason`, and
  `CheckSource` on `AgentUpdateStatusDTO`; preview fields on
  `AgentUpdatePreviewDTO`.

### Web, docs (Task 03)

- `apps/web/lib/api/domains/agent-update-api.ts`: optional fields and
  `AgentUpdateCheckReason`.
- `apps/web/components/settings/agent-runtime-update-rows.tsx`: channel label
  and reason-specific `rowDescription`.
- `apps/web/components/settings/runtime-version-summary.tsx`: native
  self-update branch.
- `apps/web/src/locales/*/agents.json` (seven locales): native notice, result,
  channel, and reason keys. Run `pnpm run i18n:zh-hant` for zh-hk and zh-tw.
- `apps/web/e2e/tests/settings/agent-self-update.spec.ts` and
  `mobile-agent-self-update.spec.ts`, plus native status and preview options in
  `agent-runtime-update-helpers.ts`.
- `docs/public/agents-and-profiles.md` "Update Oh My Pi on the Kandev host".

### Compatibility matrix

| Agent | Check source | Status semantics | Evidence | Unsupported-shape fallback |
| --- | --- | --- | --- | --- |
| OMP (`omp-acp`) | `omp update --check` | Native channel-aware result | Task 01 adapter and runner tests, Task 02 controller tests, Task 03 E2E | `unknown` with a reason |
| Harness without a native check (test fixture only today) | npm stable reference | Unchanged | Existing harness tests retargeted to a fixture agent | Unchanged |
| Checker-only built-in (test fixture only) | Declared native check | Native result; install and automatic eligibility unchanged | Task 02 fixture test | `unknown` with a reason |
| Managed npm and GitHub release agents | npm or GitHub metadata | Unchanged | Existing status tests | Unchanged |
| Custom, virtual, and mock agents | None | Unchanged | Existing capability tests | Unchanged |

## ASCII UI preview

Structural requirements: one native result line replaces the stable reference.
The channel appears only when reported. Unknown results show one reason
sentence. Existing controls, order, and test IDs are unchanged. Wording and
spacing are illustrative.

UI-01: Settings > Agents > runtime manager row (desktop), native OMP states
(AC-AGENTS-RUNTIME-NATIVE-CHECK-002.4)

```text
+---------------------------------------------------------------------------+
| omp   18.13.3 -> 18.22.0-canary.3                      [ Review update ]  |
|       Channel: canary                                                     |
+---------------------------------------------------------------------------+
| omp   Effective 18.22.0-canary.3                            [ Update ]    |
|       Channel: canary                                                     |
|       Up to date.                                                         |
+---------------------------------------------------------------------------+
| omp   Effective 18.13.3                                     [ Update ]    |
|       OMP is managed by another installer. Update it there.               |
+---------------------------------------------------------------------------+
```

UI-02: Update dialog summary (desktop dialog), native available state
(AC-AGENTS-RUNTIME-NATIVE-CHECK-002.4)

```text
+----------------------------------------------------------+
| Update omp                                           [x] |
| Update runtime                                           |
| Status comes from omp's own update check for its         |
| configured channel.                                      |
| Installed version: 18.13.3   Latest: 18.22.0-canary.3    |
| Channel: canary                                          |
| Command: omp update                                      |
|                                  [ Cancel ] [ Update ]   |
+----------------------------------------------------------+
```

UI-03: Update drawer summary (phone), native unknown state
(AC-AGENTS-RUNTIME-NATIVE-CHECK-002.4)

```text
+------------------------------+
| Update omp                   |
| Repair runtime               |
| Status comes from omp's own  |
| update check for its         |
| configured channel.          |
| Installed version: 18.13.3   |
| The update check failed.     |
| Command: omp update          |
| [         Update          ]  |  <- full width, >= 44px
| [         Cancel          ]  |
+------------------------------+
```

Phone uses one column. Version, channel, and reason text wrap. The drawer body
scrolls, and no horizontal overflow is allowed.

## Tests

| AC | Evidence |
| --- | --- |
| 001.1 | `agent_update_native_check_test.go` `TestNativeCheckStatusBypassesRegistryLookup`; existing `agent_update_status_test.go` suites unchanged |
| 001.2 | `harness_update_test.go` `TestOmpHarnessUpdateContract` (exact argv, unchanged commands); `native_update_check_runner_test.go` `TestNativeCheckRunnerUsesUpdaterEnvironmentWithoutAddedArgs` |
| 001.3 | `native_update_check_runner_test.go` `TestNativeCheckRunnerTimeout`, `TestNativeCheckRunnerOutputLimitStopsEarly`, `TestNativeCheckRunnerKillsDescendants`, `TestNativeCheckRunnerMissingExecutable`; `agent_update_native_check_test.go` `TestNativeCheckBackgroundStopCancelsProcess` (live subscriber receives `cancelled`, nothing cached); `proctree` tests and existing `copilotplugins` runner tests |
| 001.4, 001.5, 001.6 | `omp_update_check_test.go` `TestParseOmpUpdateCheck` table |
| 001.7 | `agent_update_native_check_test.go` `TestNativeCheckRuntimeMismatchIsUnknown` |
| 001.8 | `TestOmpHarnessUpdateContract`; `TestNativeCheckDoesNotEnableAutomaticUpdates`; `TestNativeCheckCheckerOnlyAgentKeepsManualCapability` |
| 002.1 | `TestNativeCheckCacheTTLsAndSingleFlight` |
| 002.2 | `TestNativeCheckRechecksOnVersionPathOrConfigChange`, `TestNativeCheckInvalidatedByTerminalHarnessJob` (success and failure), `TestNativeCheckStaleFlightCannotRepopulateAfterInvalidation` (barrier) |
| 002.3 | `TestNativeCheckNotifiesOnlyAvailable`, `TestNativeCheckSkipsDisabledAndUnavailable` (status and direct preview; unavailable fixture with a runnable executable never starts the runner) |
| 002.4 | `TestNativeCheckPreviewSkipsStableReference`; `agent-runtime-update-manager.test.tsx` and `agent-runtime-update-control.test.tsx` native cases |

Existing OMP-specific stable-reference tests in
`agent_update_harness_test.go`, such as
`TestHarnessUpdatePreviewKeepsNewerCanaryActionable` and
`TestHarnessUpdateStatusDoesNotCallNewerCanaryUpToDate`, are retargeted to a
non-native harness fixture agent so the generic flavor stays covered. OMP
assertions move to the native tests. Do not keep tests that pin OMP to the
stable reference.

## E2E tests

| Flow | AC | File | Project |
| --- | --- | --- | --- |
| Native canary update is available, the channel is shown, the dialog shows the native latest without a stable reference, and the empty approval body and streamed job are unchanged | 002.3, 002.4 | `e2e/tests/settings/agent-self-update.spec.ts` | `chromium` |
| Native unknown reason explanation in the row and dialog | 001.6, 002.4 | `e2e/tests/settings/agent-self-update.spec.ts` | `chromium` |
| Phone drawer shows the native result, channel, and full-width action without horizontal overflow | 002.4 | `e2e/tests/settings/mobile-agent-self-update.spec.ts` | `mobile-chrome` |

## Work orders

- [ ] [Task 01: Native check contract, OMP adapter, and runner](task-01-native-check-contract.md) (blocked: Windows `native` CI evidence)
- [x] [Task 02: Native results in status, cache, preview, and notifications](task-02-status-integration.md)
- [x] [Task 03: Native check presentation, locales, E2E, and docs](task-03-settings-presentation.md)

## Verification results

Local implementation finished on 2026-11-17. Each work order records its exact
commands. The implementation was checked against
REQ-AGENTS-RUNTIME-NATIVE-CHECK-001 and -002 and the system design; the
deviations listed below do not change a contract boundary.

| Check | Result |
| --- | --- |
| Task 01 verification block | All local commands pass; Windows `native` CI step pending |
| Task 02 verification block | Pass; Windows cross-vet of `settings/handlers` and `settings/store` fails in the unchanged `internal/task/repository/sqlite` package without cgo (pre-existing) |
| Task 03 verification block | Pass, including `chromium` (4 passed) and `mobile-chrome` (2 passed) E2E |
| `make fmt` | No changes to the working tree |
| `make typecheck` | Pass with `NODE_OPTIONS=--max-old-space-size=10240`; the default heap ran out of memory in classic `tsc` on Node 22 |
| `make lint` | `0 issues.`, `Linting complete!` |
| `make test` (backend) | Fails only in packages unrelated to this change: `common/config`, `launcher`, `system/updates` pass with a clean `env -i` environment (ambient `KANDEV_*` variables); `task/repository/sqlite` passes alone (`ok ... 978.385s`, 10-minute timeout in the full run); `improvekandev` (`workflow step lister is not configured`) and `worktree` (`TestCreateWorktree_RefreshLockPolicy`, no dependency on changed packages) also fail in a clean environment |
| `make test-web` | Full run: 29 files failed under load. Re-running those files alone: all `agent-runtime-update-*` files pass; 7 tests in unrelated GitLab, Linear, Sentry, Git server, and seed-origin files still fail |
| `make test-cli` / `make test-scripts` | CLI 5 of 5 pass; one unrelated `pr-await` script case fails (`strict-deadline terminal findings should exit 1, got 2`) |
| Windows `native` CI step | Pending PR |
| PR documentation coverage (`.github/scripts/pr-docs.cjs`) | Pending PR |

Implementation deviations from the design text:

- `NativeUpdateCheckContext.Fingerprint` and `NewNativeUpdateCheckResolver`
  live in `internal/agent/agents`; the resolver carries the check
  environment (`Env`, `Getenv`, `GetenvFold`) instead of a lookup function.
  The controller calls them to build the cache key.
- `nativeUpdateCheckRunner` also has `Resolve` and `Environment`, so the key
  is computed without starting a process.
- Status reports `check_source: "native"` with `check_reason: "unavailable"`
  for disabled or discovery-unavailable native agents, matching preview. No
  lookup runs for them.
- Native dialog unit tests are in
  `components/settings/agent-runtime-update-native-check.test.tsx` because
  the control test file is at the Oxlint 600-line limit.

Requirement and system-design promotion waits for Task 01, which stays
`blocked` until the Windows `native` CI step passes on the PR.

## Risks

- OMP output is human-readable. Upstream wording changes produce `unknown`
  with `unrecognized_output` until the adapter changes. Adapter tests record
  the upstream commit and local `omp/18.13.3` evidence.
- The installed `omp/18.13.3` prints `Failed to check for updates: ...` without
  the `Error: ` prefix that upstream source suggests. The adapter accepts both
  forms.
- Status GET can start a host process on a cache miss. The shared semaphore,
  single flight, and six-hour cache bound this. Disabled and unavailable agents
  never run checks.
- The ACP version and check version can differ when PATH differs between the
  host utility and backend environments. This produces `runtime_mismatch`
  rather than a misleading result.
- `omp update` still fails the job when OMP is already current, because the ACP
  version does not change. This existing behavior is not changed here.
  Settings shows "Up to date" first.
- The OMP context declaration mirrors upstream sources at one commit, so a
  configuration input added upstream later is picked up only when the cache
  entry expires. `TestOmpUpdateCheckContext` pins the list, and the adapter's
  upstream reference shows where to re-check.
- Moving `copilotplugins` process-tree code into `internal/agent/proctree`
  touches a working Windows job-object path. The migrated `copilotplugins`
  tests guard it, and a new Windows `native` CI step runs the `proctree` and
  runner tests on a real Windows host. Task 01 stays incomplete until that CI
  step passes on the PR, so the work order cannot be closed before the PR is
  open.
