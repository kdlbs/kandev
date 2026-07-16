---
title: "Testing"
description: "Choose the right Kandev test layer and run backend, web, CLI, script, desktop, and end-to-end checks."
---

# Testing

Kandev tests behavior at several boundaries. Use the narrowest test that proves a rule, then add an end-to-end scenario when the user-visible path crosses processes or protocols.

## Standard checks

From the repository root:

```bash
make fmt
make typecheck
make test
make lint
```

`make test` includes backend, web, CLI, and repository-script tests. The browser E2E suite is separate:

```bash
make test-e2e
```

Every change under `apps/web/` must add or update Playwright coverage.

## Backend tests

Go unit and integration tests live next to their packages. Use `-tags fts5` to match the SQLite build path:

```bash
cd apps/backend
go test -tags fts5 ./internal/workflow/...
go test -tags fts5 ./internal/agent/runtime/lifecycle -run TestName
```

Prefer table tests for validation and state transitions. Use temporary databases, fake clocks/providers, `httptest`, and the existing test helpers. A persistence change needs repository coverage; a handler change needs malformed-input and transport coverage; a background flow needs retry/cancellation/status coverage.

Agent adapter E2E tests under `internal/agentctl/server/adapter/e2e` exercise protocol lifecycle behavior without requiring the full browser stack.

## Web unit tests

Web tests run with Vitest:

From a fresh worktree, install the locked workspace dependencies first:

```bash
cd apps
pnpm install --frozen-lockfile
cd ..
```

Then run the full web test target or a focused test from the repository root:

```bash
make test-web
cd apps && pnpm --filter @kandev/web test -- path/to/file.test.tsx
```

Test state transitions, interaction, accessibility semantics, and failure paths rather than snapshots of incidental markup.

## Playwright E2E

The suite under `apps/web/e2e/` starts production-shaped backend/web services and uses isolated seed APIs and test fixtures. Setup:

```bash
make bootstrap-e2e
make test-e2e
```

Useful modes include `make test-e2e-headed` and `make test-e2e-ui`.

- Seed only the data the story needs.
- Use unique test identifiers and repositories.
- Do not connect to a developer's normal Kandev database or task workspace.
- Assert persisted/user-visible outcomes, not fixed sleeps.
- Preserve traces/screenshots for failures.
- Cover native mobile routes when the mobile experience differs from desktop.

The internal `docs/test_e2e_web.md` contains fixture and debugging detail for contributors working directly on the suite.

## Agent and executor E2E

Agent tests use mock agents and executor-specific harnesses to cover launch, permission, MCP, resume, terminal, and lifecycle behavior. Docker and Sprites suites have additional prerequisites and are not implied by a fast local unit run.

When changing runtime code, test the relevant local path and a remote/container-shaped path. Include failure during prepare, process start, reconnect, and cleanup where the change can affect them.

## CLI, desktop, and scripts

```bash
make test-cli
make test-scripts
```

Desktop tests live under `apps/desktop/`; the script suite also includes a desktop launch smoke test. Shell, Python, workflow-lint, release, and documentation validators are part of `make test-scripts` because failures there can block packaging or deployment even when application tests pass.

## Windows coverage

`make test-windows` runs the curated cross-platform subset used by Windows CI. Do not assume POSIX shell commands, symlink behavior, process signals, or delete-while-open semantics apply on Windows. Add platform guards only for genuinely platform-specific behavior; prefer portable fixtures where possible.

## Before requesting review

Record:

- exact commands and test counts;
- manual paths, viewport/platform, and executor used;
- any suite not run and why;
- known flaky or dependency-bound behavior;
- screenshots, traces, or logs needed to understand a failure.

A green unit test does not replace manual review of a generated diff, provider side effect, migration, or security boundary.
