---
spec: docs/specs/executors/requirements/port-collision-safety.md
created: 2026-08-07
status: complete
---

# Implementation Plan: Port collision safety and launcher readiness

## Overview

Repair the three reported startup/port bugs as one cross-launcher package. Add explicit backend
port preflight to the TypeScript and native Go launchers, make health polling prove ownership with
the existing per-launch token contract, and centralize address-in-use classification for agentctl
and websocket bindings. Keep PR #2368's PORT and WEB_PORT Makefile behavior unchanged.

The package has no frontend or database work. The launcher health tests use mocked HTTP responses
and Go httptest servers; the port allocator tests use real loopback listeners so the Windows
error path is exercised rather than simulated with an English error string.

## Root cause and design

- CLI explicit ports bypass the availability probe in apps/cli/src/shared.ts, and the installed
  Go launcher bypasses canBind for an explicit backend port in
  apps/backend/internal/launcher/ports.go.
- Both health pollers currently accept any successful response from the target URL. The backend
  already echoes KANDEV_DESKTOP_HEALTH_TOKEN in the
  X-Kandev-Desktop-Health-Token response header, and the desktop launcher demonstrates the
  per-launch ownership pattern. Reuse that contract and add the token to both supervisor
  environment allowlists.
- The agentctl allocator compares only against syscall.EADDRINUSE and also has an English string
  fallback. The websocket tunnel has a duplicate classifier with the same platform gap. A new
  internal common helper will handle wrapped syscall errors and Windows WSAEADDRINUSE.

## CLI launcher

### Explicit port preflight

- Carry the backend-port source selected by args.ts (CLI flag, KANDEV_BACKEND_PORT, or KANDEV_PORT)
  alongside the numeric port into the shared port-picking path.
- Add a shared explicit-port assertion in apps/cli/src/ports.ts using the existing
  isPortAvailable probe. Use it from pickPorts, pickBackendPorts, and the installed run path so
  dev, start, and run have identical behavior.
- Preserve automatic fallback only for an omitted backend port. Report the source and port in a
  stable error without changing PR #2368's Makefile flags.

### Owned health readiness

- Generate a fresh token with the Node crypto API once per CLI invocation and pass it through
  buildBackendEnv.
- Extend waitForHealth to send the launcher's expected token context and require an exact matching
  response header in addition to 2xx status.
- Add KANDEV_DESKTOP_HEALTH_TOKEN to the CLI supervisor manifest allowlist so backend restarts
  keep the same launch identity.
- Keep tokens out of startup logs, timeout text, and captured diagnostics. Keep direct backend
  health behavior unchanged when no expected token is supplied by a caller outside the launcher.

## Native Go launcher

### Explicit port preflight

- Preserve the precedence order in resolvePorts while retaining the selected source for errors.
- Make pickPorts validate an explicit backend port with the existing canBind behavior and return a
  source-aware error. Automatic backend selection keeps its current fallback loop.
- Apply the same result to both installed run and local start before their managed backend child is
  launched.

### Owned health readiness

- Generate one cryptographically random token per managed launcher invocation, not a new token for
  each supervisor restart.
- Add the token to backendEnv and to allowedSupervisorEnv, preserving it in the owner-only launch
  manifest.
- Extend waitForHealth/probeHealth to require a matching
  X-Kandev-Desktop-Health-Token response header while retaining the existing child-exit,
  timeout, and cancellation errors.
- Add a route-level backendapp regression assertion that the existing health endpoint still echoes
  the configured token; do not change the endpoint's behavior for tokenless direct callers.

## Cross-platform bind errors

- Add an internal common netutil helper with Unix and Windows platform files. Unwrap net.OpError,
  os.SyscallError, and syscall errno values as needed; recognize
  x/sys/windows.WSAEADDRINUSE on Windows.
- Replace the instance manager's syscall/string checks with the helper and leave its
  MarkUnavailable/retry behavior intact.
- Replace the websocket tunnel's local helper with the same common helper and preserve its
  existing user-facing occupied-port message.
- Add real double-bind tests and include the affected instance, tunnel, and helper packages in
  the Windows-sensitive CI test command. Keep the Makefile test-windows target and workflow in
  sync.

## Public documentation

Update only the user-facing statements that describe startup failure and Windows coverage:

- docs/public/contributing.md
- docs/public/cli.md
- docs/public/remote-cloud-environment.md
- docs/public/windows-support.md

Explain that an explicitly selected occupied port is rejected before launcher readiness and that
the launcher never silently substitutes another requested port. Do not document the internal
health-token variable as a user configuration knob. Leave the separate service-install
KANDEV_SERVER_PORT limitation unchanged.

## Tests

### CLI

- Add unit coverage for source precedence and occupied explicit ports from --port,
  KANDEV_BACKEND_PORT, and KANDEV_PORT.
- Add health tests for matching, missing, and mismatched response headers, including the
  no-early-success guarantee.
- Add manifest/env tests proving the token is passed to the child and retained for supervisor
  restarts without logging it.

### Go launcher/backend

- Add launcher health tests for matching, missing, and mismatched response headers, plus child
  exit and timeout behavior.
- Add env/manifest tests proving the generated token is retained across restart.
- Add a backendapp health route test for the existing echo header.

### Address-in-use handling

- Add a netutil test that binds a real loopback listener, attempts a second bind, and verifies the
  wrapped error is classified as address-in-use on the host platform.
- Add an instance allocator test that occupies the first configured candidate and verifies the
  next candidate is returned.
- Keep non-address-in-use errors on the release/fail path and retain a focused websocket occupied
  port assertion where the package test seams permit it.
- Run the affected packages on windows-latest and keep the local cross-compile checks available.

No browser E2E test applies because this repair changes launcher/backend startup ownership, not
the SPA UI.

## Implementation waves and parallel candidates

Wave 1:

- [x] [Task 01: CLI explicit port preflight](task-01-cli-explicit-port-preflight.md)
- [x] [Task 02: Native launcher port preflight](task-02-native-launcher-port-preflight.md)
- [x] [Task 05: Cross-platform address-in-use handling](task-05-cross-platform-address-in-use.md)

These are parallel-safe candidates because they own separate source areas. The primary
conversation executes them sequentially unless the user explicitly authorizes subagents.

Wave 2:

- [x] [Task 03: CLI health ownership](task-03-cli-health-ownership.md), depends on Task 01.
- [x] [Task 04: Native health ownership](task-04-native-health-ownership.md), depends on Task 02.

Tasks 03 and 04 are separate-language parallel-safe candidates after their respective preflight
contracts are stable; execute sequentially by default.

Wave 3:

- [x] [Task 06: Update launcher documentation](task-06-update-launcher-documentation.md), depends on
  Tasks 01–05.

## Risks and mitigations

- A preflight probe can race with another process. Token-matched readiness prevents a stranger's
  2xx response from producing a false ready state; child exit still reports the actual bind
  failure.
- The existing token name is desktop-specific. Reusing it avoids a backend contract migration;
  renaming it is explicitly out of scope.
- Persisting the token in a supervisor manifest is necessary for restart ownership. The existing
  manifest is owner-only; implementation must not add token values to logs or user-facing errors.
- Windows test packages include platform-sensitive fixtures. Keep the new regression narrow and
  preserve existing skips for tests that cannot run under Windows ACL/socket rules.

## Open questions

None blocking. Exact error wording may follow each launcher’s current error style, provided the
port and selected source are present.

## Verification Results

Implementation is complete. The following checks passed:

- `make -C apps/backend test` — all backend packages passed.
- `pnpm --filter kandev test` — 30 files and 297 CLI tests passed.
- `pnpm --filter kandev exec tsc --noEmit` — passed.
- `go test -race -tags fts5 ./internal/launcher ./internal/backendapp ./internal/common/netutil ./internal/agentctl/server/instance ./internal/gateway/websocket` — affected packages passed.
- Windows cross-compilation checks passed for netutil, instance, and websocket packages.
- `make -C apps/backend test-windows` — original Windows-sensitive packages plus targeted
  netutil, instance allocator, and websocket tunnel tests passed.
- Public-doc tests passed (58 tests); `validate-public-docs.mjs` validated 41 pages.
- `git diff --check`, Go formatting, and Prettier checks passed.

Reference commands from the repository root:

~~~bash
make -C apps/backend test
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
~~~

From apps/ after installing the missing worktree dependencies when necessary:

~~~bash
pnpm --filter kandev test -- src/args.test.ts src/ports.test.ts src/shared.test.ts src/health.test.ts src/supervisor/manifest.test.ts
~~~

For the focused Go loop:

~~~bash
cd apps/backend
go test -tags fts5 ./internal/launcher ./internal/backendapp ./internal/common/netutil ./internal/agentctl/server/instance ./internal/gateway/websocket
GOOS=windows GOARCH=amd64 go test -c -o /tmp/kandev-instance-windows.test.exe ./internal/agentctl/server/instance
GOOS=windows GOARCH=amd64 go test -c -o /tmp/kandev-tunnel-windows.test.exe ./internal/gateway/websocket
~~~
