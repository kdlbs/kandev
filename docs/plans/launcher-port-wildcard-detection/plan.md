---
status: implemented
created: 2026-08-11
spec: "../../specs/executors/requirements/port-collision-safety.md"
---

# Plan: Launcher port-availability probe detects wildcard listeners

## Root cause

`make dev` failed to bind the default backend port `38429` and did not fall back to a
random port, even though the launcher is supposed to. The launcher's port-availability
check is `canBind` in `apps/backend/internal/launcher/ports.go`:

```go
func canBind(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
```

A running Kandev backend binds the **wildcard** address (`0.0.0.0` and `[::]`). On
BSD-derived systems including macOS, a bind to the specific `127.0.0.1` address succeeds
even while a wildcard listener already holds the port, and Go's `net.Listen` sets
`SO_REUSEADDR`, which reinforces the false success. So `canBind(38429)` returns `true`
against the live backend, `pickAvailablePortExcept` keeps the occupied preferred port,
and the child backend later dies on the real wildcard bind.

Before PR #2411 moved `make dev` to the native Go launcher, `make dev` ran through the
TypeScript launcher (`apps/cli/src/ports.ts`), whose `isPortAvailable` did a **connect**
probe on both `127.0.0.1` and `::1` **and** a bind probe, and only reported a port free
when nothing answered the connect and the bind succeeded. That connect probe is exactly
what detected the wildcard listener; the Go rewrite dropped it. This is a regression from
that migration.

## Approach

Restore the dual connect-and-bind, dual-stack probe in the Go launcher's `canBind`,
mirroring the deleted TypeScript logic:

- Connect to `127.0.0.1:<port>` and `::1:<port>` with a short bounded timeout. If either
  connect succeeds, something is listening: the port is busy.
- Otherwise attempt a fresh loopback bind. Success means the port is free.

All existing callers (`pickAvailablePortExcept`, explicit backend/web preflight in
`pickDevPorts`) route through `canBind`, so the single change fixes preflight and
automatic selection together. No signature change, so no downstream caller updates.

### Design decisions

- **Keep the bind probe.** It catches Windows phantom reservations and TIME_WAIT ports a
  connect-only check misses. The spec requires both probes.
- **Bounded connect timeout.** A silently dropped SYN to an unbound loopback port (WSL2
  mirrored networking) must not hang selection; a timeout counts as "nothing listening".
  Reuse the TypeScript value of 500ms.
- **Probe both loopback families.** The backend may hold only the IPv6 wildcard socket, so
  an IPv4-only connect probe would miss it.
- **No signature change.** `canBind(port int) bool` stays; internals gain the connect step.

## Regression test

`TestCanBindDetectsWildcardListener` (new, in `ports_test.go`): stand up a wildcard TCP
listener (`net.Listen("tcp", ":0")` on the IPv6/dual wildcard, or `0.0.0.0:0`), take its
port, and assert `canBind(port)` returns `false`. This fails before the fix (bind-only
`canBind` returns `true` against the wildcard listener) and passes after. A companion
assertion confirms `pickAvailablePortExcept(port, nil)` does not return the occupied port.

## Tasks

| Task | Wave | Parallel-safe | Summary |
| --- | --- | --- | --- |
| task-01-wildcard-port-probe | 1 | no | Add failing regression test, implement connect+bind dual-stack `canBind`. |

## Validation

From `apps/backend`:

```bash
go test ./internal/launcher/ -run 'Port|CanBind|Bind' -race -count=1
go test ./internal/launcher/ -race -count=1
gofmt -l internal/launcher/ports.go internal/launcher/ports_test.go
golangci-lint run ./internal/launcher/... --new-from-rev=origin/main --timeout=5m
```

## Risks and out of scope

- **Out of scope:** changing default ports, fallback ranges, the health-token ownership
  path, the Windows address-in-use classifier, or backend runtime-state locking.
- **Risk:** a slow or firewalled loopback connect could add up to the timeout per busy
  candidate. Mitigated by the 500ms bound and by the connect only running during port
  selection, not on the hot path.
