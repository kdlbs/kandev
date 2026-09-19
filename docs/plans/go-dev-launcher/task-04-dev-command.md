---
id: "04-dev-command"
title: "dev command, web port, and dev env wiring"
status: done
wave: 2
depends_on: ["01-dev-paths"]
plan: "plan.md"
spec: "../../specs/platform/requirements/go-dev-launcher.md"
---

# Task 04: `dev` command, web port, and dev env wiring

Teach the launcher's arg parser, port picker, and env builder about dev mode.

## Acceptance

- `parseArgs` accepts `dev` and `--dev` (→ `CommandDev`), `--web-internal-port <n>` and
  `--web-internal-port=<n>`, and rejects `--web-port` with the existing message
  "`--web-port` has been removed; use `--web-internal-port` for dev mode" and
  `--web-internal-port` outside dev mode with "only applies to dev mode". Port values
  keep the 1–65535 validation and the existing `ParseError` exit code 2.
- `pickDevPorts` resolves the web port from the flag then `KANDEV_WEB_PORT`, defaults to
  `37429`, and guarantees backend, web, and agentctl ports are mutually distinct. An
  explicitly requested but occupied web port is a hard error naming the port and whether
  it came from the flag or the env var, matching the backend-port preflight from
  PR #2394.
- `backendEnv` sets `KANDEV_WEB_INTERNAL_URL=http://localhost:<webPort>` when a web port
  is present and `KANDEV_DEBUG_DEV_MODE=true` in dev mode, and leaves `start`/`run`
  output byte-identical to today when it is not.
- `cli.Help()` documents `dev` and `--web-internal-port`.

## Verification

Use TDD across the existing suites, then:

~~~bash
cd apps/backend && go test ./internal/launcher/ -race
~~~

## Files

- `apps/backend/internal/launcher/options.go`, `options_test.go`
- `apps/backend/internal/launcher/ports.go`, `ports_test.go`
- `apps/backend/internal/launcher/env.go`, `env_test.go`
- `apps/backend/internal/launcher/cli/help.go`
- `apps/backend/internal/launcher/launcher.go` — dispatch `CommandDev` to `runDev`
  (a stub returning "not implemented" is acceptable here; task 05 fills it in).

## Inputs

- `apps/cli/src/args.ts:22-138` — the exact flag surface and error strings to preserve.
- `apps/cli/src/cli.ts:13-58` — the help text wording for `dev` and
  `--web-internal-port`.
- `apps/backend/internal/launcher/ports.go:65-91` — `pickPorts` and the `used` map;
  extend rather than fork it.
- `apps/backend/internal/launcher/supervisor.go:169-190` — `allowedSupervisorEnv`
  already allowlists `KANDEV_WEB_INTERNAL_URL` and `KANDEV_DEBUG_DEV_MODE`; confirm, do
  not duplicate.

## Risks

- `portConfig.WebPort` must be optional. `start` and `run` have no web port and
  `backendEnv` must not emit an empty `KANDEV_WEB_INTERNAL_URL` for them — the
  TypeScript version explicitly deletes the key in that case
  (`apps/cli/src/shared.ts:85-87`).
- Watch the Go complexity limits in `apps/backend/.golangci.yml`; `parseArgAt` is
  already a dispatch chain, so add a `parseWebPortArg` helper rather than growing it.
