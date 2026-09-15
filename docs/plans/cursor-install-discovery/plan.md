---
created: 2026-09-15
status: done
requirements:
  - REQ-PLATFORM-LOCAL-AGENT-PATH-001
system_design:
  - ../../specs/platform/system-design/local-agent-path.md
legacy_specs: []
---

# Implementation plan: Cursor install discovery

## Overview

Repair the shared native launcher's backend PATH so agents installed in the
runtime user's `.local/bin` can be discovered and executed. One sequential work
order covers the environment producer, its consumers, regression tests, and docs.

## Evidence and diagnosis

- User reports macOS with regular `npx` and service launch. Terminal resolves
  both `agent` and `cursor-agent` under `/Users/ram58/.local/bin`.
- `agents/cursor_acp.go` checks only `cursor-agent` through process PATH.
  Its installer exports `.local/bin` in its child shell and appends to `.bashrc`;
  neither changes a running parent's environment.
- `launcher/service.go` omits `.local/bin` from `launchdServicePath`.
  `backendEnvForConfig` currently forwards PATH without supplementing it.
- Post-install cache invalidation and capability refresh already exist in
  `controller.SetJobBroadcaster`; rescanning alone cannot repair the environment.
- The regular-launch failure is reproducible when its inherited PATH omits the
  install directory. The user's historical regular-launch process PATH has not
  been captured; do not claim that specific process environment was verified.
- A Linux diagnostic bundle was collected before the macOS clarification and
  deleted. It is not evidence about the affected Mac.

## Scope

In scope: shared Unix child environment normalization, actual Cursor discovery
and subprocess execution, launchd-input coverage, late installation, precedence,
restart propagation, and CLI documentation.

Out of scope: alias expansion, authentication, installer replacement, remote
executor changes, frontend code, service-account changes, and live-instance
mutation. Both Cursor binary names already exist in the reported installation.

## Technical approach

Follow [the design](../../specs/platform/system-design/local-agent-path.md).
Add the effective user's `.local/bin` once to the end of the final child PATH
in `backendEnvForConfig`. Apply before subprocess creation and retain the path
even when the directory does not yet exist. Keep service definitions compatible;
fixing only the generated plist would miss regular and existing service launches.

## Tests and end-to-end evidence

Use `internal/launcher/local_agent_path_test.go` for the acceptance matrix.
`TestLocalAgentPathPostInstall` must create an isolated home, obtain the real
launcher-produced environment before installation, create fake `agent` and
`cursor-agent` executables afterward, and drive actual Cursor discovery and
subprocess execution under that environment. Expect failure before the patch.

`TestLocalAgentPathLaunchd` must parse PATH from `renderLaunchdPlist` and pass it
through the actual backend environment producer. Include a distinct Kandev data
home. Add cases for existing PATH precedence, duplicate normalization, empty
PATH, unavailable home, missing/non-executable files, and Windows preservation.
Cover backend child restart propagation using the existing supervisor test seam.
This is process-level end-to-end evidence; no browser layout changes are planned.

On an isolated macOS runtime, confirm installation followed by rescan exposes
Cursor and a local probe executes for regular and service launches. Record this
as pending if no macOS runner is available; Linux fixture execution is not a
native macOS smoke test.

## Work orders

- [x] [Task 01: Preserve local agent PATH](task-01-local-agent-path.md)

## Verification results

Temporary `TestReproCursorPostInstallPath` created both executable names under
an isolated home after obtaining `backendEnv` with `/usr/bin:/bin` PATH. Actual
`CursorACP.IsInstalled` returned unavailable. Command:
`GOCACHE=/tmp/kandev-cursor-go-cache go test ./internal/launcher -run '^TestReproCursorPostInstallPath$' -count=1`
failed with `installed Cursor is invisible under launcher-produced PATH`, as
expected. The temporary test was removed at the design checkpoint. This ran on
Linux and verifies the shared Go path; native macOS execution remains pending.

`python3 scripts/list-docs.py validate` passed (272 decisions, 936 specifications).
`python3 scripts/lint-spec-files.py --all` passed.
Implemented in `env.go` and `local_agent_path.go`; regression coverage is in
`local_agent_path_test.go`. Public CLI documentation updated.

- `GOCACHE=/tmp/kandev-cursor-go-cache go test ./internal/launcher -run '^TestLocalAgentPath' -count=1`: passed after failing on missing discovery before the patch.
- `GOCACHE=/tmp/kandev-cursor-go-cache go test -race ./internal/launcher ./internal/agent/agents ./internal/agent/runtime/agentctl/launcher -count=1`: both agent packages passed; launcher required the isolated rerun below because the sandbox forbids sockets and inherited runtime settings conflict with fixture values.
- Launcher rerun outside the socket-restricted sandbox: Python copied `os.environ` excluding keys starting with `KANDEV_`, set `GOCACHE=/tmp/kandev-cursor-go-cache`, and ran `go test -race ./internal/launcher -count=1`. Passed (14.310s).
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Final lint correction reused the existing `goosWindows` constant; the focused regression suite was rerun with `-race` afterward.
- `pnpm install --frozen-lockfile` from `apps/`: passed for commit-hook dependencies.

Native macOS regular/service smoke testing was not available on this Linux
runner. The generated launchd environment, actual discovery, executable
precedence, late installation, absence behavior, and supervised restart are
covered by process-level tests. Windows preservation is included as a
platform-conditional test but was not executed on Windows here.


## Risks

The fix must use the runtime account home, preserve PATH precedence, cover late
installation, and propagate to subprocesses. An already-running old launcher or
agentctl may retain its old environment until the runtime is restarted. If the
regular-launch symptom persists with a verified correct PATH, investigate that
separately instead of expanding this repair without evidence.

## PR review remediation

Codex identified that a system LaunchDaemon can inherit an absent or incorrect
HOME after selecting a non-root account. System-service local executable PATH
now uses the effective UID's account home and preserves PATH when lookup fails.
Added `local_agent_service_path_test.go` with a generated non-root LaunchDaemon
fixture, actual discovery/execution, and failed/invalid account lookup coverage.
Regular launches retain final child HOME precedence.

CodeRabbit's summary suggestion is addressed by documenting the usable absolute
home condition in the requirement and public CLI guide.

Remediation validation:
- `GOCACHE=/tmp/kandev-cursor-go-cache go test -race ./internal/launcher -run '^TestLocalAgentPath' -count=1`: passed.
- `go test -race ./internal/launcher -count=1` with inherited `KANDEV_*` settings removed, writable GOCACHE, and local socket access: passed (16.928s).
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`: passed.

Remote CI/review evidence remains pending for the remediation head.
