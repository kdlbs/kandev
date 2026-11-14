---
id: "01-pin-build-environment"
title: "Pin production frontend compilation and guard build contracts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001
acceptance_criteria:
  - AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001.1
  - AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001.3
  - AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001.4
  - AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001.5
system_design:
  - ../../specs/platform/system-design/source-frontend-build-mode.md
---

# Task 01: Pin production frontend compilation and guard build contracts

## Summary

Correct the compilation environment at the root Makefile command boundary.
Add a focused regression to the existing shell-contract checker before changing production recipes.

## In scope

- Add `check_root_web_build_environment` coverage in `scripts/check-make-shells`.
- Execute `build-web`, `build-web-quiet`, and `build-web-e2e` with a temporary pnpm recorder.
- Cover inherited development, test, production, and unset Node environments.
- Assert the existing Vite API-port and debug clearing and the E2E build argument.
- Assert installation and `dev-web` retain their input environments.
- Assert the parent environment survives a build unchanged.
- Dry-run the Windows frontend recipe and the debug/verbose aliases with safe parse-time probe overrides.
- Pin the production value on the three POSIX build commands and the native Windows frontend command.

## Out of scope

- Global environment exports, dependency-installation changes, or package-script changes.
- Launcher environment changes, browser testing, profiling, and memory analysis.
- The quiet recipe's existing filtering and error-suppression behavior.

## Acceptance

1. The recorder regression fails before the Makefile change because it receives `development`, then passes after the command-local correction.
2. The recorder and dry-run matrix cover all affected build commands, alias forwarding, installation isolation, Vite clearing, and development compatibility.
3. The exact shell-contract and diagnostic tests pass. Native Windows generation and execution evidence remain clearly distinguished.

## Implementation notes

Reuse the existing `scripts/check-make-shells` OS/MSYSTEM assertions and add the temporary recorder beside them.
Neutralize `NULL_REDIR`, `NODE`, and build-stamp probes during Windows simulation.
Replace recursive make commands with a recorder or harmless echo during root recipe dry runs.
Do not execute cmd.exe syntax through a POSIX shell.
Record the temporary pnpm invocations in a dedicated file, so quiet-output filtering cannot hide a missing invocation.
Clean up the recorder with a trap on success and failure.

Do not set `NODE_ENV=production` before `pnpm install`.
Do not add `--prod` or alter installation's inherited environment.
Keep `build-web-e2e` on `build:e2e`, which retains the explicit pseudo-locale opt-in.

## Verification

Run from the repository root:

```bash
make -C apps/backend check-make-shells
(cd apps/backend && go test -trimpath -tags fts5 ./internal/launcher ./internal/webapp -run '^(TestResolveLogThresholdsByMode|TestBackendEnvCarriesIndependentThresholds|TestBootPayloadScriptSetsDebugGlobalBeforeBootPayload)$' -count=1)
git diff --check -- Makefile scripts/check-make-shells
```

Run the shell-contract command once for RED before editing the Makefile.
After the correction, run the complete block for GREEN.
Record actual command results and any native Windows limitation.

## Files likely touched

- `Makefile`
- `scripts/check-make-shells`

## Dependencies

None.
The existing backend CI job already runs `make check-make-shells` from `apps/backend`.
No additional workflow is necessary.

## Risks

- Command-wide environment settings can accidentally alter dependency installation.
- Windows `set` syntax can add whitespace to a value without correct quoting.
- A recorder must observe execution, rather than infer values from Makefile text.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/source-frontend-build-mode.md).
- [Compilation environment and verification design](../../specs/platform/system-design/source-frontend-build-mode.md#compilation-environment).
- [Investigation evidence and package checks](plan.md).
- Root `AGENTS.md`, `apps/backend/AGENTS.md`, and `/tdd` guidance.

## Results

Completed on 2026-11-04.

- The new `check_root_web_build_environment` assertions failed before the Makefile change:
  `build-web`, `build-web-quiet`, and `build-web-e2e` received the inherited `development`
  value, and the unset/test inputs reached the recorder unchanged.
- After the command-local correction, all checks passed for inherited development, test,
  production, and unset values.
- The recorder also confirmed the empty `VITE_KANDEV_API_PORT` and `VITE_KANDEV_DEBUG` values,
  the `build:e2e` argument, unchanged dependency installation, development command preservation,
  and parent-environment preservation.
- Windows dry-run generation confirmed `set "NODE_ENV=production"` and the verbose/debug alias
  forwarding. Native `cmd.exe` execution was not available on this host.
- `make -C apps/backend check-make-shells` and the three launcher/webapp diagnostic tests passed.
- `git diff --check` reported no whitespace errors.
