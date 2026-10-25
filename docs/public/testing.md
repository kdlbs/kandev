---
title: "Testing"
description: "Choose and run Kandev's Go, web, CLI, script, PostgreSQL, Playwright, agent, executor, and desktop test layers."
---

# Testing

Use the narrowest test that proves a rule, then cover the real boundary when behavior crosses a process, protocol, database, executor, or user journey.

## Quick path

1. Format the changed files and run the focused package or Vitest test for the changed rule.
2. For Go lint, use the changed-package hook below. Use broader local checks when the affected boundary requires them.
3. Add Playwright coverage for every `apps/web/` behavior change.
4. Run PostgreSQL, container, real-agent, or desktop layers only when the change crosses that boundary.

## Standard local checks

For an explicitly selected full local audit, run these checks sequentially from the repository root:

```bash
make fmt
make typecheck
make test
make lint
```

`make test` runs backend, web, CLI, and repository-script tests. By default, it does **not** run browser Playwright, PostgreSQL-backed cases, real agent adapters, Sprites, real container executors, or a live Tauri launch. PostgreSQL cases do run when `KANDEV_TEST_POSTGRES_DSN` is supplied.

Use `make test-e2e` for the browser suite. It builds the backend and web app before Playwright. Every change under `apps/web/` must add or update a Playwright scenario.

## Local Go lint

The commit hook lints the complete packages containing staged Go files. To check specific changed files from the repository root:

```bash
bash scripts/lint-go-changed apps/backend/internal/common/gitref/gitref.go
```

For a committed PR diff, use `pre-commit run go-lint --from-ref <base-sha> --to-ref HEAD`. If the PR targets another branch, set `kandev.lintBaseRef` to its local remote-tracking ref so the wrapper uses the correct comparison base.

The wrapper defaults to `GOMEMLIMIT=3GiB`, `GOMAXPROCS=2`, two lint CPUs, and two Go build workers. Explicit environment values and `GOFLAGS=-p=N` override those defaults; other Go flags are preserved. The memory setting is a soft target for each Go process, not a limit on the combined process tree.

Run lint and race/build suites sequentially. CI retains full-backend lint coverage. `make lint` and `make lint-backend` still select the entire backend; use OS-level resource limits covering subprocesses when running a full local scan. `--new-from-rev` filters reported findings and does not narrow the packages analyzed.

## Backend tests

Go tests are colocated with packages. Match the supported local SQLite path with CGO and FTS5:

```bash
cd apps/backend
CGO_ENABLED=1 go test -tags fts5 ./internal/workflow/...
CGO_ENABLED=1 go test -tags fts5 ./internal/agent/runtime/lifecycle -run TestName
```

Use temporary databases and directories, `httptest`, fake clocks/providers, and existing fixtures. Test validation and state transitions at the service layer, malformed transport input at handlers, and retry/cancellation/status at background boundaries.

A persistence change needs fresh-schema, replay, and representative old-row coverage. PostgreSQL tests read `KANDEV_TEST_POSTGRES_DSN`; CI starts PostgreSQL 16 and gives tests isolated schemas. Run that path when SQL, rebinding, migrations, or backend startup changes.

## Web and CLI tests

Install locked workspace dependencies from `apps/` in a fresh checkout:

```bash
cd apps
pnpm install --frozen-lockfile
cd ..
make test-web
make test-cli
```

For one Vitest file:

```bash
cd apps
pnpm --filter @kandev/web test -- path/to/file.test.tsx
```

Prefer assertions on state transitions, interaction, accessibility, error handling, and command selection over snapshots of incidental markup.

## Isolated browser E2E

Bootstrap once, then use the root target for the complete production-shaped build:

```bash
make bootstrap-e2e
make test-e2e
```

`make test-e2e-headed` and `make test-e2e-ui` support visual debugging. For a focused, managed run:

```bash
cd apps/web
pnpm e2e:run --project chromium tests/path/to/spec.ts
```

The managed runner builds what it needs, chooses host or the CI runtime image, enables strict WebSocket assertions, and supports `--shards N`, `--no-build`, and `--project NAME`.

Set `KANDEV_E2E_DOCKER_PROBE_TIMEOUT` to change the Docker availability limit in automatic mode. The default is 10 seconds. Set it to `0` to skip the probe and use host mode.

Playwright uses one worker per process. Each worker fixture starts a real Go backend serving the built SPA with unique ports, a temporary `HOME`, Kandev home, SQLite database, repositories/worktrees, and agentctl port range. Each test receives a fresh browser context and resets seeded application state. Kandev process boundaries are real; external providers and the agent process are mocked unless a project says otherwise.

Projects in `apps/web/e2e/playwright.config.ts` are:

| Project | Scope |
|---|---|
| `routing` | Office routing cases that restart the backend with distinct provider configuration |
| `chromium` | Main desktop-browser journeys |
| `mobile-chrome` | Pixel 5 responsive browser journeys, not a native mobile app |
| `containers` | Real Docker executor and Docker-hosted SSH target; selectable directly and skipped without Docker |

The root `make test-e2e` target runs the managed runner once per declared project (`routing`, `auth`, `chromium`, `mobile-chrome`, `containers`), so it covers the same projects Playwright would run unscoped. The managed `pnpm e2e:run` command defaults to `chromium`; select another project explicitly.

Run container-backed cases explicitly:

```bash
cd apps/web
KANDEV_E2E_CONTAINERS=1 pnpm e2e:raw --project=containers
```

E2E rules:

- seed only the story's data through fixtures or supported APIs;
- use unique IDs and repositories;
- never point tests at a developer's normal database or task workspace;
- assert persisted or user-visible outcomes, not fixed sleeps;
- preserve traces, screenshots, video, and backend logs for failures;
- use the guarded `e2e:raw` or `e2e:run` commands. Both enforce one
  Playwright worker per process; the managed runner also enforces a
  memory-aware local shard limit;
- cover reconnect, multi-repository, and mobile behavior when the feature depends on them.

The internal `docs/test_e2e_web.md` contains fixture and debugging detail.

## Agent, executor, and desktop E2E

Do not confuse the browser target with real-agent tests:

```bash
make -C apps/backend test-e2e
```

Backend adapter E2E uses the `e2e` Go build tag and may invoke installed third-party agents, consume paid usage, or require login. Run only the affected agents intentionally. Sprites has a separate `make test-sprites-e2e` target and credential requirements.

The Playwright `containers` project covers real Docker and SSH executor paths. Runtime changes should also test prepare failure, readiness, reconnect, cancellation, and cleanup rather than only the happy local path.

`make test-scripts` unit-tests the desktop launch-smoke harness; it does not launch Tauri. The scoped desktop smoke is:

```bash
cd apps
pnpm --filter @kandev/desktop e2e
```

Run Rust unit tests with the desktop runtime enabled:

```bash
cd apps/desktop/src-tauri
cargo test --features desktop-runtime
```

Desktop E2E CI supplies the platform dependencies and bundled runtime.

## Script and platform checks

`make test-scripts` covers action pinning, repository helper scripts, release-desktop logic, the desktop-smoke harness, and public-doc validator unit tests. Run the live docs validator separately with `node scripts/validate-public-docs.mjs`.

`make test-windows` is a curated Windows-safe subset for process and agentctl launcher code plus web and CLI tests. It is not the complete Linux/macOS suite. Avoid assumptions about POSIX signals, symlinks, shells, path separators, or deleting open files.

## Report evidence

Record exact commands and results, manual routes/viewports/platforms/executors, suites not run and why, and any relevant traces or logs. A green unit test does not replace review of a generated diff, provider side effect, migration, credential boundary, or release artifact.
