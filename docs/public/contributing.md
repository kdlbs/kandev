---
title: "Contributing to Kandev"
description: "Set up Kandev, find the owning subsystem, run focused checks, update public behavior, and prepare a reviewable change."
---

# Contributing to Kandev

Kandev combines a Go server and native launcher, a Vite/React web client, a TypeScript development supervisor and npm shim, a Tauri desktop shell, and task-environment helpers. Begin at the subsystem that owns the behavior; do not recreate its rules in a neighboring layer.

## Set up the repository

The pinned toolchain in `mise.toml` currently includes Node 24, pnpm 9.15.9, Go 1.26.0, and supporting tools.

```bash
git clone https://github.com/kdlbs/kandev.git
cd kandev
make bootstrap
```

`make bootstrap` best-effort installs host prerequisites, installs the pinned tools, runs `pnpm install --frozen-lockfile` in `apps/`, and configures hooks. For browser E2E dependencies, use `make bootstrap-e2e`. Run `make doctor` to re-install the optional pre-commit and commit-message hooks.

## Run locally

```bash
make dev
```

This is the normal development path. The TypeScript supervisor starts the Go backend and Vite, selects available ports, points Go at Vite, and isolates application state under the checkout's `.kandev-dev/`. Use the printed URLs.

`make dev-web` starts only Vite on its fixed development port; it has no live API by itself. `make dev-backend` starts only the backend with the normal production-profile home unless you override it. For an intentionally isolated backend-only run:

```bash
KANDEV_HOME_DIR="$PWD/.kandev-dev" KANDEV_DEBUG_DEV_MODE=true make dev-backend
```

Use `make build` for a production build. Use `make start` for a production-shaped local start; it installs dependencies, builds and synchronizes the embedded web application, then launches Kandev.

## Find the owner

| Change | Start here |
|---|---|
| Domain behavior, API, integration, or persistence | `apps/backend/internal/<domain>/` |
| Server construction and startup wiring | `apps/backend/internal/backendapp/` |
| Agent definitions, discovery, runtime, or executors | `apps/backend/internal/agent/` |
| agentctl binary and task-environment sidecar | `apps/backend/cmd/agentctl/`, `apps/backend/internal/agentctl/` |
| Browser routes, state, API calls, or UI | `apps/web/` |
| Shared web UI, types, or themes | `apps/packages/` |
| Development supervisor or npm runtime shim | `apps/cli/` |
| Installed native launcher behavior | `apps/backend/internal/launcher/` |
| Tauri process, updater, or native desktop boundary | `apps/desktop/` |
| Public documentation | `docs/public/` |
| CI, packaging, or release automation | `.github/workflows/`, `scripts/` |

Read the nearest types, tests, and startup registration before adding an abstraction. Backend domains do not all use one layout: follow the local handler/controller, service, repository/store, and provider pattern.

## Test while developing

Run the narrowest relevant package or workspace test during iteration. Before review, format first, then run:

```bash
make fmt
make typecheck
make test
make lint
```

`make test` covers Go, web, CLI, and repository-script tests. Browser Playwright, PostgreSQL, real-agent, container, and desktop-launch suites have separate prerequisites and commands; see [Testing](testing.md).

Repository policy requires every change under `apps/web/` to add or update a Playwright scenario in `apps/web/e2e/` and run `make test-e2e`.

## Keep public behavior current

Update `docs/public/**` in the same change when commands, configuration, settings, workflows, executors, integrations, APIs, screenshots, support status, or user terminology changes.

For a new page, create a stable Markdown slug with title and description frontmatter, add it once to `docs/public/meta.json`, link it from related pages, and run:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

The complete source and Landing build contract is in the [public docs contribution guide](README.md).

## Review checklist

- The change has one clear user impact and owning subsystem.
- Wire changes update Go DTOs, TypeScript types/clients, compatibility behavior, and protocol docs together.
- Persistence changes include fresh-schema and upgrade-path tests.
- Credentials, external text, shell arguments, URLs, paths, and logs respect their trust boundary.
- Exact automated and manual checks are recorded; skipped suites have a reason.
- UI changes include Playwright coverage plus screenshots or a short recording.
- Generated artifacts, local databases, recordings, and unrelated formatting are absent.
- You understand and can explain all submitted code, including agent-generated code.

The root `CONTRIBUTING.md` defines community and licensing policy. Contributions are under the repository's AGPL-3.0 license.

Continue with [Architecture](architecture.md), [Backend development](backend-development.md), [Web development](web-development.md), [Testing](testing.md), [Extending Kandev](extending-kandev.md), or [Release process](release-process.md).
