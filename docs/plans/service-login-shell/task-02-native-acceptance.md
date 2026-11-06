---
id: "02-native-acceptance"
title: "Verify native macOS behavior and document operation"
status: in_progress
wave: 2
depends_on:
  - "01-service-shell"
plan: "plan.md"
requirements:
  - REQ-CLI-SERVICE-LOGIN-SHELL-001
acceptance_criteria:
  - AC-CLI-SERVICE-LOGIN-SHELL-001.3
  - AC-CLI-SERVICE-LOGIN-SHELL-001.4
  - AC-CLI-SERVICE-LOGIN-SHELL-001.5
  - AC-CLI-SERVICE-LOGIN-SHELL-001.6
  - AC-CLI-SERVICE-LOGIN-SHELL-001.7
  - AC-CLI-SERVICE-LOGIN-SHELL-001.8
system_design:
  - ../../specs/cli/system-design/service-login-shell.md
---

# Task 02: Verify native macOS behavior and document operation

## Summary

Establish native shell and launchd evidence after the portable implementation passes.
Document the exact supported behavior and recovery commands.

## In scope

- Real `/bin/zsh` and `/bin/bash` initialization and process replacement tests.
- A uniquely labelled temporary launchd job with owned files and cleanup.
- Disposable-account installed Kandev startup, restart, reset, and failure recovery.
- Operator instructions in CLI reference and service how-to documentation.

## Out of scope

Never change a developer's login files, default service, or Kandev database.
Do not publish packages, trigger real package upgrades, or add browser tests.

## Acceptance

1. Native tests prove exported shim discovery, a non-PATH marker, account home,
   and configured environment precedence. Interactive-only initialization stays absent.
2. launchd starts the configured Kandev entrypoint, process replacement retains
   identity, and stop/restart works. Failures never produce a false ready result.
3. Documentation covers selection, supported scope, omission, reset, updates,
   startup-file failures, and the distinction from an interactive terminal.

## Native test design

Add `TestServiceShellDarwinLoginEnvironment` and
`TestServiceShellDarwinLaunchdLifecycle` in a Darwin-only test file.
Require explicit `KANDEV_TEST_LAUNCHD_SHELL=1` for launchd mutation.
Use a disposable logged-in account with a working `gui/<uid>` launchd domain.
Do not run this gate in the user's normal account.

Use temporary fixture executables and controlled account startup files.
Zsh cases cover `.zshenv`, `.zprofile`, `.zlogin`, and an unexecuted `.zshrc` marker.
Bash cases cover its normal login-file selection and a separate interactive marker.
Startup files must export a unique shim directory and a non-sensitive marker.
Include spaces, single quotes, dollar signs, and XML metacharacters in fixture paths.
Fixtures record only selected non-sensitive values, argv, and PID.

The launchd fixture must exercise the production shell runner, not recreate its command.
Use a unique `com.kdlbs.kandev.shell-test.<suffix>` label and temporary plist/log paths.
Register teardown before bootstrap. Bound waits and remove only that job and its files.
Assert PID continuity across shell replacement, signal delivery, and full restart.
Cover startup exit, missing executable, and a hanging fixture that stops cleanly.
Do not depend on a broad `pkill` or on parsing full process environments.

## Installed-runtime acceptance

In the disposable account, use a locally built candidate with an isolated data home.
Run the commands below after setting `KANDEV_SHELL_TEST_BINARY` to its absolute path
and `KANDEV_SHELL_TEST_HOME` to an empty, disposable data directory.
The account must have no existing `com.kdlbs.kandev` service.

```bash
set -e
test "$(uname -s)" = Darwin
test -x "$KANDEV_SHELL_TEST_BINARY"
test -n "$KANDEV_SHELL_TEST_HOME"
test ! -e "$HOME/Library/LaunchAgents/com.kdlbs.kandev.plist"
if launchctl print "gui/$(id -u)/com.kdlbs.kandev" >/dev/null 2>&1; then
  exit 1
fi
"$KANDEV_SHELL_TEST_BINARY" service install --home-dir "$KANDEV_SHELL_TEST_HOME" --shell /bin/zsh --no-boot-start
"$KANDEV_SHELL_TEST_BINARY" service status
plutil -extract ProgramArguments json -o - "$HOME/Library/LaunchAgents/com.kdlbs.kandev.plist"
"$KANDEV_SHELL_TEST_BINARY" service stop
"$KANDEV_SHELL_TEST_BINARY" service start
"$KANDEV_SHELL_TEST_BINARY" service install --home-dir "$KANDEV_SHELL_TEST_HOME" --no-boot-start
"$KANDEV_SHELL_TEST_BINARY" service restart
"$KANDEV_SHELL_TEST_BINARY" service install --home-dir "$KANDEV_SHELL_TEST_HOME" --shell=none --no-boot-start
"$KANDEV_SHELL_TEST_BINARY" service uninstall
```

Record service readiness from the logged URL after each start, plus tool discovery
and a local fixture-agent run. Record plist and metadata selection after reinstall
and reset. Exercise Apply regeneration with stub package-manager commands through
Task 01's complete intent path, without downloading a release.
Inspect the macOS background-item presentation separately and record the OS version.
Do not claim that plist argv alone proves the displayed attribution.
Remove the disposable account's fixture files and runtime data after evidence capture.

## Verification

Run from the repository root on the disposable macOS host:

```bash
set -e
test "$(uname -s)" = Darwin
(cd apps/backend && KANDEV_TEST_LAUNCHD_SHELL=1 go test -tags fts5 ./internal/launcher -run '^TestServiceShellDarwin' -count=1 -v)
(cd apps/backend && go test -tags fts5 ./internal/launcher ./internal/system/updates -count=1)
git ls-files -m -o --exclude-standard -- 'apps/backend/*.go' | xargs bash scripts/lint-go-changed
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The platform guard is mandatory. A skip is not a pass for native acceptance.
The repository has no equivalent browser check for this OS boundary.
Review public examples against the implemented help and native command evidence.

## Files likely touched

- `apps/backend/internal/launcher/service_shell_darwin_test.go` (new)
- `docs/public/cli.md` (reference)
- `docs/public/run-as-a-service.md` (how-to)
- This work order and `plan.md` for native evidence and final results.

## Dependencies

Task `01-service-shell` and access to a disposable macOS login account.

## Risks

An unavailable macOS host blocks this acceptance task, not the portable implementation.
OS background-item wording can differ despite a correct configured entrypoint.
If observed attribution differs, record it and resolve the requirement before completion.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/cli/requirements/service-login-shell.md)
- [Design](../../specs/cli/system-design/service-login-shell.md)
- Task 01 results and the [plan](plan.md).
- Existing `service_survival_linux_test.go` for bounded OS-process fixture conventions.
- Official Bash/Zsh startup documentation and the installed macOS shell manuals.

## Results

Public CLI, service, and update documentation is implemented. Darwin-only tests
cover Bash/Zsh initialization, exported tool discovery, process replacement,
launchd signal/restart behavior, and startup failures. Linux package tests and
the backend build pass.

Source-review remediation adds explicit migration and reset coverage for the
reported Zsh login-wrapper plist, strict no-write rejection for manager, mode,
service-path, and home-directory metadata mismatches, and legacy Node/CLI
LaunchAgent recognition for service updates. The launchd fixture now records
the complete argv atomically, clears stale results before restart, and checks
that kickstart produces a new PID. The Linux-only rejection test asserts that
no service command or definition/metadata write occurs.

Post-review verification passed:

- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/launcher ./internal/system/updates -count=1)`
- `make -C apps/backend build`
- `git ls-files -m -o --exclude-standard -- 'apps/backend/*.go' | xargs -r bash scripts/lint-go-changed` (0 issues)
- `python3 scripts/list-docs.py validate` (404 decisions, 1,742 specifications)
- `python3 scripts/lint-spec-files.py --all`
- `node --test scripts/validate-public-docs.test.mjs` (63 passed)
- `node scripts/validate-public-docs.mjs` (54 pages)
- `git diff --check`
- PR follow-up: the Git admission audit now distinguishes direct Git exec from
  the non-Git login-shell process replacement; `go test ./internal/common/subproc`
  passes. Reset examples in the service and operations guides preserve an
  installed custom `--home-dir`; public-doc tests and page validation pass.

The Darwin-tagged source and lifecycle tests also passed an isolated cross-target
compile check with dependency stubs. This was compile-only and provides no
native launchd runtime evidence.

Native macOS shell, launchd, background attribution, and disposable-account
installed-runtime acceptance remain pending because this workspace runs Linux.
The attempted Darwin cross-compile is blocked by existing CGO-disabled SQLite
type errors in `internal/task/repository/sqlite`, before the launcher package
can compile. A skip or cross-compile failure is not native acceptance.
