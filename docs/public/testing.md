---
title: "Testing"
description: "Choose and run Kandev's Go, web, CLI, script, PostgreSQL, Playwright, agent, executor, and desktop test layers."
---

# Testing

Use the narrowest test that proves a rule, then cover the real boundary when behavior crosses a process, protocol, database, executor, or user journey.

## Standard local checks

From the repository root, format first:

```bash
make fmt
make typecheck
make test
make lint
```

`make test` runs backend, web, CLI, and repository-script tests. By default, it does **not** run browser Playwright, PostgreSQL-backed cases, real agent adapters, Sprites, real container executors, or a live Tauri launch. PostgreSQL cases do run when `KANDEV_TEST_POSTGRES_DSN` is supplied.

Use `make test-e2e` for the browser suite. It builds the backend and web app before Playwright. Every change under `apps/web/` must add or update a Playwright scenario.

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
pnpm e2e:run -- --project chromium -- tests/path/to/spec.ts
```

The managed runner builds what it needs, chooses host or the CI runtime image, enables strict WebSocket assertions, and supports `--shards N`, `--no-build`, and `--project NAME`.

Playwright uses one worker per process. Each worker fixture starts a real Go backend serving the built SPA with unique ports, a temporary `HOME`, Kandev home, SQLite database, repositories/worktrees, and agentctl port range. Each test receives a fresh browser context and resets seeded application state. Kandev process boundaries are real; external providers and the agent process are mocked unless a project says otherwise.

Projects in `apps/web/e2e/playwright.config.ts` are:

| Project | Scope |
|---|---|
| `routing` | Office routing cases that restart the backend with distinct provider configuration |
| `chromium` | Main desktop-browser journeys |
| `mobile-chrome` | Pixel 5 responsive browser journeys, not a native mobile app |
| `containers` | Real Docker executor and Docker-hosted SSH target; selectable directly and skipped without Docker |

The root `make test-e2e` target leaves project selection to Playwright and therefore runs every project whose prerequisites are available. The managed `pnpm e2e:run` command defaults to `chromium`; select another project explicitly.

Run container-backed cases explicitly:

```bash
cd apps/web
pnpm exec playwright test --config e2e/playwright.config.ts \
  --project=containers
```

E2E rules:

- seed only the story's data through fixtures or supported APIs;
- use unique IDs and repositories;
- never point tests at a developer's normal database or task workspace;
- assert persisted or user-visible outcomes, not fixed sleeps;
- preserve traces, screenshots, video, and backend logs for failures;
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
