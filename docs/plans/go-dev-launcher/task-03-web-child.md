---
id: "03-web-child"
title: "Vite dev-server child and URL readiness"
status: done
wave: 1
depends_on: ["04-dev-command"]
plan: "plan.md"
spec: "../../specs/platform/requirements/go-dev-launcher.md"
---

# Task 03: Vite dev-server child and URL readiness

Port `apps/cli/src/web.ts`'s launch path plus the token-less readiness probe.

## Acceptance

- `webCommand()` returns `pnpm` with args `-C apps --filter @kandev/web dev` on Unix and
  `cmd.exe /c pnpm ...` on Windows, and the Windows branch is exercised by a test that
  does not require running on Windows.
- `buildWebEnv(ports, debug)` sets `KANDEV_API_BASE_URL` to the backend URL, `PORT` to
  the chosen web port, `HOSTNAME=127.0.0.1`, and (when debug) `KANDEV_DEBUG=true` plus
  `VITE_KANDEV_DEBUG=true`; it removes any inherited `VITE_KANDEV_API_PORT` from the
  child environment.
- `waitForURLReady(ctx, url, proc, timeout)` succeeds on any HTTP response, fails fast
  with the child's exit code if the child dies first, and times out with a message
  naming the URL and timeout. It does not consult the launcher health token.

## Verification

Use TDD. Add `apps/backend/internal/launcher/webchild_test.go`; drive the readiness probe
against an `httptest.Server` and a fake `childState`, and prefer `testing/synctest` over
`time.Sleep` for the timeout case (see `apps/backend/CLAUDE.md`, "Testing"). Then:

~~~bash
cd apps/backend && go test ./internal/launcher/ -run 'TestWebCommand|TestBuildWebEnv|TestWaitForURLReady' -race
~~~

## Files

- `apps/backend/internal/launcher/webchild.go` (new)
- `apps/backend/internal/launcher/webchild_test.go` (new)

## Inputs

- `apps/cli/src/web.ts:1-24` — `WIN_SHIM_COMMANDS` / `resolveWindowsShim` and the comment
  explaining why `cmd.exe /c` is the right call on Windows (no PATHEXT for `spawn`,
  EINVAL on direct `.cmd` spawn since Node 21, DEP0190 with `shell:true`). The Node
  specifics no longer apply, but the `.cmd`-shim resolution problem does.
- `apps/cli/src/shared.ts:136-165` — `buildWebEnv`, including why
  `VITE_KANDEV_API_PORT` is explicitly deleted.
- `apps/cli/src/health.ts` — `waitForUrlReady` semantics.
- `apps/backend/internal/launcher/health.go` — reuse `childState`, `healthPollInterval`,
  and the bounded `healthProbeClient` pattern rather than introducing new ones.

## Risks

- Reuse `startProcess` from `process.go` for spawning so the Vite child joins the same
  supervisor and process group; do not call `exec.Command` directly.
- Do not relax `waitForHealth`'s token check to share code with this probe. PR #2394
  made a token mismatch a hard failure on purpose.
