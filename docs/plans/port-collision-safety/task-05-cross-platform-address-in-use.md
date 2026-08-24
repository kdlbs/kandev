---
id: "05-cross-platform-address-in-use"
title: "Cross-platform address-in-use handling"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/executors/requirements/port-collision-safety.md"
---

# Task 05: Cross-platform address-in-use handling

## Acceptance

- A shared internal helper recognizes wrapped Unix address-in-use errors and Windows
  WSAEADDRINUSE (10048), without relying on localized English error text.
- The agentctl instance allocator marks an occupied candidate unavailable and retries the next
  candidate on Windows; non-address-in-use errors still release and fail.
- The websocket tunnel uses the shared classifier and preserves its clear occupied-port error.
  Real double-bind regression coverage runs in the Windows-sensitive CI job.

## Verification

Use TDD with a real loopback double bind and the allocator retry path:

~~~bash
cd apps/backend
go test -tags fts5 ./internal/common/netutil ./internal/agentctl/server/instance ./internal/gateway/websocket
~~~

Build-check the Windows variants from a non-Windows host:

~~~bash
GOOS=windows GOARCH=amd64 go test -c -o /tmp/kandev-netutil-windows.test.exe ./internal/common/netutil
GOOS=windows GOARCH=amd64 go test -c -o /tmp/kandev-instance-windows.test.exe ./internal/agentctl/server/instance
GOOS=windows GOARCH=amd64 go test -c -o /tmp/kandev-tunnel-windows.test.exe ./internal/gateway/websocket
~~~

The windows-latest job must run the real tests, not only compile them.

## Files likely touched

- apps/backend/internal/common/netutil/addr_in_use.go
- apps/backend/internal/common/netutil/addr_in_use_unix.go
- apps/backend/internal/common/netutil/addr_in_use_windows.go
- apps/backend/internal/common/netutil/addr_in_use_test.go
- apps/backend/internal/agentctl/server/instance/manager.go
- apps/backend/internal/agentctl/server/instance/manager_ports_test.go
- apps/backend/internal/gateway/websocket/port_tunnel.go
- apps/backend/internal/gateway/websocket/port_tunnel_test.go
- apps/backend/Makefile
- .github/workflows/backend-tests.yml

## Dependencies

None.

## Parallelism

Parallel-safe candidate with Tasks 01 and 02; the primary conversation executes sequentially by
default. It does not share implementation files with the launcher preflight/health tasks.

## Inputs

- Spec sections: Windows address-in-use handling and Issue #2371 scenarios.
- Plan sections: Cross-platform bind errors and Address-in-use handling tests.
- Existing dependencies: golang.org/x/sys already exists in apps/backend/go.mod.

## Risks

- Preserve the error-unwrapping behavior for net.OpError and os.SyscallError on every supported OS.
- Do not use a Windows-only import from a non-Windows build; isolate x/sys/windows behind build
  tags.
- Keep port allocator release/MarkUnavailable semantics unchanged for non-collision failures.
- Keep the Makefile test-windows target and GitHub Windows job package lists aligned.

## Completion

- Behavior: shared Unix/Windows address-in-use classification drives allocator retry and tunnel
  errors without string matching.
- Files: netutil classifier, allocator/tunnel callers and tests, plus Windows Makefile/CI scope.
- Verification: real double-bind and occupied-tunnel tests pass under the race detector; the
  Windows job now runs the new collision tests separately from the incompatible broad MCP fixture.
- Platform note: the full instance package remains out of the Windows-sensitive broad list because
  its stdio fixture expects a Unix command; targeted port tests still run on Windows.
