---
id: "01-native-check-contract"
title: "Native check contract, OMP adapter, and runner"
status: blocked
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-NATIVE-CHECK-001
acceptance_criteria:
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-001.2
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-001.3
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-001.4
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-001.5
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-001.6
  - AC-AGENTS-RUNTIME-NATIVE-CHECK-001.8
system_design:
  - ../../specs/agents/system-design/runtime-native-update-checks.md
---

# Task 01: Native check contract, OMP adapter, and runner

## Summary

Add the optional native update check capability, have OMP declare
`omp update --check`, and implement a pure OMP result adapter plus a bounded
direct-argv runner. Nothing calls the runner from the status service yet, so
the current status behavior does not change.

## In scope

- Capability, context, output, result, state, and reason types (including
  `cancelled`) from the design's
  [capability contract](../../specs/agents/system-design/runtime-native-update-checks.md#capability-contract).
- `ParseOmpUpdateCheck` according to the design's
  [OMP adapter](../../specs/agents/system-design/runtime-native-update-checks.md#omp-adapter)
  rules, and `ompUpdateCheckContext` (environment keys and default
  configuration file paths).
- `OmpACP.NativeUpdateCheck()` and its interface assertion.
- Extract `copilotplugins` process-tree containment into
  `internal/agent/proctree` (`Prepare`, `Capture`, `Tree.Kill`), then migrate
  `copilotplugins/runner.go` to it and delete the old files.
- `execNativeUpdateCheckRunner` according to the design's
  [execution](../../specs/agents/system-design/runtime-native-update-checks.md#execution)
  rules: `filteredInstallEnv()`, `exec.LookPath`, no added arguments,
  process-tree containment, a 30-second timeout, and a 64 KiB shared output
  budget that cancels the run on overflow. It also uses `WaitDelay`, the
  `output_limit` > `timeout` > `cancelled` reason precedence, and exit-code
  propagation.
- Native Windows coverage. Add a `native` suite step to
  `.github/workflows/backend-tests.yml` and a matching line in the backend
  `test-windows` Makefile target. Both run `./internal/agent/proctree/...`
  and `./internal/agent/settings/controller -run '^TestNativeCheckRunner'`
  on a Windows host. Runner and `proctree` helper-process tests must not be
  build-tagged away from Windows. Timeout, overflow, cancellation, and
  descendant-cleanup cases run there against the job-object branch.

## Out of scope

- Status, cache, preview, notification, and job integration (Task 02).
- DTO or web changes (Tasks 02 and 03).
- Background-lifetime cancellation wiring (Task 02); this task only honors the
  passed context.

## Acceptance

- `TestParseOmpUpdateCheck` covers stable and canary availability, up to date
  with and without a channel, both failure-prefix variants, managed
  installation, ANSI output, conflicting markers, `Switching to` and
  `Forcing reinstall` lines, empty output, an invalid version, build metadata,
  and non-zero exit with an outcome marker. Every case has exactly the state,
  reason, and version fields from the design. `TestOmpUpdateCheckContext`
  proves the declared environment keys and the resolved sources for a given
  home and working directory. The cases cover the default profile,
  `PI_CODING_AGENT_DIR`, `PI_CONFIG_DIR`, `OMP_PROFILE` precedence over
  `PI_PROFILE` (including an explicitly empty `OMP_PROFILE`, and an
  inherited profile-derived `PI_CODING_AGENT_DIR` ignored when `OMP_PROFILE`
  is empty or `default` and `PI_PROFILE` names that profile), `default`, an
  invalid profile, `PI_CONFIG_FILES` overlays with `~` and relative paths, the
  known project candidates, the `<cwd>/.omp` and `<cwd>/.claude` membership
  with its 256-entry limit, the `manager.json` path derived from a symlinked
  executable, the npm and bun configuration candidates, the startup dotenv
  files (including the named-profile config root `.env`, the base root `.env`,
  and `<cwd>/.env.*`), and case-insensitive matching of
  `npm_config_*` keys.
- Runner tests use the test binary as a helper process. They prove the timeout,
  missing-executable, and exit-code reasons. An output overflow followed by a
  sleeping process returns `output_limit` well before the timeout. A helper
  that starts a long-lived descendant leaves no descendant alive after either
  timeout or context cancellation (`cancelled`). They also prove that the child
  receives no added arguments and has no `npm_config_prefix` when the parent
  environment contains one. `copilotplugins` tests pass unchanged on the
  shared `proctree` package.
- `TestOmpHarnessUpdateContract` asserts check argv `omp update --check` and
  unchanged `HarnessUpdate`, `BuildCommand`, `Runtime().Cmd`,
  `InstallScript`, and passthrough command. `RuntimeUpdateCapabilities` for
  OMP is unchanged.

## Verification

```bash
(cd apps/backend && go test -tags fts5 -count=1 ./internal/agent/agents -run 'TestParseOmpUpdateCheck|TestOmpHarnessUpdateContract|TestOmpUpdateCheckContext')
(cd apps/backend && go test -tags fts5 -count=1 ./internal/agent/registry ./internal/agent/proctree ./internal/agent/copilotplugins)
(cd apps/backend && go test -tags fts5 -count=1 ./internal/agent/settings/controller -run 'TestNativeCheckRunner')
(cd apps/backend && GOOS=windows CGO_ENABLED=0 go vet ./internal/agent/agents ./internal/agent/proctree ./internal/agent/copilotplugins ./internal/agent/settings/controller)
(cd apps/backend && golangci-lint run ./internal/agent/agents/... ./internal/agent/proctree/... ./internal/agent/copilotplugins/... ./internal/agent/settings/controller/...)
```

Cross-compilation `go vet` only checks that the Windows branch compiles. Native
Windows evidence comes from the new `backend-tests.yml` `native` step, or from
`make -C apps/backend test-windows` on a Windows host. No local Windows host is
available, so record the step's CI result on the PR separately in Results.
Until it passes, do not mark this work order complete.

## Files likely touched

- `apps/backend/internal/agent/agents/native_update_check.go` (new)
- `apps/backend/internal/agent/agents/omp_update_check.go` (new)
- `apps/backend/internal/agent/agents/omp_update_check_test.go` (new)
- `apps/backend/internal/agent/agents/omp_acp.go`
- `apps/backend/internal/agent/agents/harness_update_test.go`
- `apps/backend/internal/agent/registry/runtime_update_coverage_test.go` (only if it enumerates harness capabilities)
- `apps/backend/internal/agent/settings/controller/native_update_check_runner.go` (new)
- `apps/backend/internal/agent/settings/controller/native_update_check_runner_test.go` (new)
- `apps/backend/internal/agent/proctree/proctree_unix.go`, `proctree_windows.go`, and `proctree_test.go` (new)
- `apps/backend/internal/agent/copilotplugins/runner.go`
- `apps/backend/internal/agent/copilotplugins/process_tree_unix.go` and `process_tree_windows.go` (deleted)
- `.github/workflows/backend-tests.yml` (Windows `native` suite step)
- `apps/backend/Makefile` (`test-windows` target)

## Dependencies

None.

## Risks

- Helper-process tests must not depend on a host `omp`. Use the test binary
  with an environment-selected mode, and use `t.Setenv` only in non-parallel
  tests.
- On Windows, `exec.LookPath` resolves `.exe` and `.cmd`. Keep direct
  execution and add no shell fallback.
- The `dupl`/`goconst` lint convention for native agent files applies to
  `omp_acp.go`. Keep the declaration to a few lines.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/runtime-native-update-checks.md)
- [System design](../../specs/agents/system-design/runtime-native-update-checks.md)
- Existing bounded-output patterns in `apps/backend/internal/agent/hostcli/runner.go`
  and `apps/backend/internal/agent/copilotplugins/runner.go`.
- Upstream `packages/coding-agent/src/cli/update-cli.ts` at commit
  `2b562c5b7e3f` and local `omp/18.13.3` output
  (`Current version: 18.13.3`, then
  `Failed to check for updates: TypeError: ECONNREFUSED: ...` on stderr, exit 1).

## Results

Local implementation and every local verification command passed on
2026-11-17. Status is `blocked` only on native Windows evidence: the new
`backend-tests.yml` `native` step must pass on the PR before this work order
is `done`.

Implementation notes:

- `NativeUpdateCheckResolver` carries the check environment as `Env` entries
  with `Getenv` (exact) and `GetenvFold` (exact first, then
  case-insensitive, matching npm). `NewNativeUpdateCheckResolver` derives the
  home directory from that environment and evaluates executable symlinks.
- `NativeUpdateCheckContext.Fingerprint` lives in `internal/agent/agents`
  next to the context declaration so the 256-entry directory limit is tested
  with the OMP sources. Task 02 calls it from the controller.
- `nativeUpdateCheckRunner` also exposes `Resolve` (LookPath only) and
  `Environment` (`filteredInstallEnv()`), which Task 02 uses for the cache key
  and fingerprint without starting a process.
- Mutation check: replacing the process-tree kill with a direct-child kill made
  `TestNativeCheckRunnerKillsDescendants` fail for both timeout and
  cancellation (heartbeat kept growing); the original code was restored.

| Command | Result |
| --- | --- |
| `(cd apps/backend && go test -tags fts5 -count=1 ./internal/agent/agents -run 'TestParseOmpUpdateCheck\|TestOmpHarnessUpdateContract\|TestOmpUpdateCheckContext')` | `ok github.com/kandev/kandev/internal/agent/agents 0.091s` |
| `(cd apps/backend && go test -tags fts5 -count=1 ./internal/agent/registry ./internal/agent/proctree ./internal/agent/copilotplugins)` | `ok .../registry 0.046s`, `ok .../proctree 0.019s`, `ok .../copilotplugins 4.685s` |
| `(cd apps/backend && go test -tags fts5 -count=1 ./internal/agent/settings/controller -run 'TestNativeCheckRunner')` | `ok .../settings/controller 3.650s` |
| `(cd apps/backend && GOOS=windows CGO_ENABLED=0 go vet ./internal/agent/agents ./internal/agent/proctree ./internal/agent/copilotplugins ./internal/agent/settings/controller)` | exit 0, no findings |
| `(cd apps/backend && golangci-lint run ./internal/agent/agents/... ./internal/agent/proctree/... ./internal/agent/copilotplugins/... ./internal/agent/settings/controller/...)` | `0 issues.` |
| Windows `native` CI step (`Test Windows native update check containment`) | Pending: no local Windows host; requires a PR run |
