---
id: "01-local-agent-path"
title: "Preserve local agent PATH"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-LOCAL-AGENT-PATH-001
acceptance_criteria:
  - AC-PLATFORM-LOCAL-AGENT-PATH-001.1
  - AC-PLATFORM-LOCAL-AGENT-PATH-001.2
  - AC-PLATFORM-LOCAL-AGENT-PATH-001.3
  - AC-PLATFORM-LOCAL-AGENT-PATH-001.4
  - AC-PLATFORM-LOCAL-AGENT-PATH-001.5
system_design:
  - ../../specs/platform/system-design/local-agent-path.md
---

# Task 01: Preserve local agent PATH

## Summary

Make the shared Unix backend child environment include the runtime user's local
executable directory. Prove newly installed Cursor is discovered and executable
without restarting solely to refresh PATH.

## Scope and owned files

- `apps/backend/internal/launcher/env.go`: invoke the normalizer after overrides.
- `apps/backend/internal/launcher/local_agent_path.go` and
  `local_agent_path_test.go`: focused helper and producer/consumer regressions.
- Existing launcher supervisor tests if needed to verify restart propagation.
- `docs/public/cli.md`: describe supported local agent discovery after the fix.

Do not change Cursor identity, permissions, shell profiles, UI markup, remote
execution, service accounts, or npm shim behavior.

## Inputs

Read the linked requirement/design and plan evidence. Use
`service_npm_path_test.go` as a subprocess fixture pattern. Trace `start.go`,
`dev.go`, `supervisor.go`, and local agentctl environment inheritance before edits.

## Acceptance

1. The post-install and launchd regression tests fail for missing discovery
   before the patch, then pass through the real environment producer.
2. Preserve ordering, effective home, idempotence, Windows behavior, and absence
   semantics; cover all criteria in the plan's test matrix.
3. Actual child execution and restart propagation pass; record isolated macOS
   regular/service smoke results or explicitly retain that validation gap.

## Verification

From the repository root, run the focused regression first for red/green, then
the package checks:

```sh
(cd apps/backend && go test ./internal/launcher -run '^TestLocalAgentPath' -count=1)
(cd apps/backend && go test -race ./internal/launcher ./internal/agent/agents ./internal/agent/runtime/agentctl/launcher -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use a writable GOCACHE override when the default cache is read-only. macOS smoke
steps: start an isolated owned regular runtime with `.local/bin` absent from
inherited PATH; install Cursor through settings; rescan and verify detection and
local probe execution. Repeat with an isolated owned launchd service. Never
replace or restart the user's main service for this check. Remove owned fixtures.

## Risks

Do not use Kandev data home as user home or normalize before final HOME/PATH
overrides. Preserve configured executable precedence and do not rely on the
directory existing at startup. Keep auth failure separate from binary discovery.

## Parallelism

`sequential`

## Results

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

