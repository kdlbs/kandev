---
id: "01-wildcard-port-probe"
title: "Detect wildcard listeners in the launcher port probe"
status: done
wave: 1
depends_on: []
plan: "./plan.md"
requirements:
  - REQ-EXECUTORS-CONTROL-OWNERSHIP-001
  - REQ-EXECUTORS-PORT-COLLISION-SAFETY-001
acceptance_criteria:
  - AC-EXECUTORS-CONTROL-OWNERSHIP-001.7
  - AC-EXECUTORS-PORT-COLLISION-SAFETY-001.6
  - AC-EXECUTORS-PORT-COLLISION-SAFETY-001.7
system_design:
  - ../../specs/executors/system-design/agent-survival-across-restart-01.md
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 01: Detect wildcard listeners in the launcher port probe

## Goal

Make `canBind` in `apps/backend/internal/launcher/ports.go` report a port as available
only when a dual-stack loopback connect finds nothing listening **and** a fresh loopback
bind succeeds, so `make dev` falls back to a random port when a running backend holds the
preferred port through a wildcard bind.

## Root cause (confirmed)

`canBind` bind-probes only `127.0.0.1`. A running backend binds the wildcard address, and
on macOS/BSD a specific-address bind succeeds against an active wildcard listener (Go also
sets `SO_REUSEADDR`), so `canBind` falsely returns `true`. The TypeScript launcher this
replaced (PR #2411) also ran a dual-stack connect probe; the Go rewrite dropped it.

## Files

- `apps/backend/internal/launcher/ports.go` — replace the bind-only `canBind` with a
  connect-then-bind probe: connect `127.0.0.1` and `::1` with a bounded timeout; if either
  answers, return `false`; otherwise return the result of the loopback bind. Add a
  `connectProbeTimeout` constant (500ms) and a private `portHasListener(port)` helper.
- `apps/backend/internal/launcher/ports_test.go` — add the regression test(s).

## Steps (TDD)

1. Mark this task `in_progress`.
2. Add `TestCanBindDetectsWildcardListener`: create a wildcard listener on a free port
   (`net.Listen("tcp", ":0")`), read `port := ln.Addr().(*net.TCPAddr).Port`, assert
   `canBind(port)` is `false`, and assert `pickAvailablePortExcept(port, map[int]bool{})`
   does not return `port`. Run it and confirm it FAILS for the expected reason (bind-only
   `canBind` returns `true`).
3. Implement the connect+bind dual-stack probe. Keep the exported signature
   `canBind(port int) bool` unchanged.
4. Run the targeted commands below; confirm the new test passes and existing launcher
   tests still pass.
5. Mark this task `done` and update `plan.md` status.

## Acceptance criteria

- `canBind(port)` returns `false` while a wildcard listener holds `port` (IPv4 or IPv6).
- `canBind(port)` returns `true` for a genuinely free port.
- Existing tests (`TestPickPortsRejectsOccupiedExplicitBackendPort`,
  `TestPickDevPorts*`, `TestPickAvailablePortExceptSkipsUsedPreferredPort`) still pass.
- No caller signature changes; `pickDevPorts`/`pickPorts` behavior is unchanged for free
  ports.

## Validation commands

From `apps/backend`:

```bash
go test ./internal/launcher/ -run 'CanBind|Port|Bind' -race -count=1
go test ./internal/launcher/ -race -count=1
gofmt -l internal/launcher/ports.go internal/launcher/ports_test.go
golangci-lint run ./internal/launcher/... --new-from-rev=origin/main --timeout=5m
```

## Out of scope

Default ports, fallback ranges, health-token ownership, the Windows address-in-use
classifier, and backend runtime-state locking.

## Follow-up (2026-10-01)

The Windows Firewall follow-up to PR #30 renames this task's regression test
`TestCanBindDetectsWildcardListener` to `TestCanBindDetectsOccupiedListener`. Unix keeps the
wildcard-occupant assertion, so the original contract and its failure mode are unchanged. Windows
holds the port with a loopback listener (`127.0.0.1:0`) instead, because a wildcard test listener
binds a non-loopback interface and Windows Firewall prompts for every ephemeral
`%TEMP%/go-build...` test binary. `canBind` production behavior is unchanged. See
`docs/specs/executors/requirements/port-collision-safety.md` for the probe contract.
