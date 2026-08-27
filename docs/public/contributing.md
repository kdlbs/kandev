---
title: "Contributing to Kandev"
description: "Set up Kandev, find the owning subsystem, run focused checks, update public behavior, and prepare a reviewable change."
---

# Contributing to Kandev

Kandev combines a Go server and native launcher, a Vite/React web client, a TypeScript development supervisor and npm shim, a Tauri desktop shell, and task-environment helpers. Begin at the subsystem that owns the behavior; do not recreate its rules in a neighboring layer.

## Quick path

1. Run `make bootstrap` once in a fresh checkout.
2. Use `make dev` for normal full-stack work.
3. Find the owning backend, web, CLI, desktop, or runtime boundary before editing.
4. Open an issue before a large architectural change and discuss its direction
   with maintainers before opening a PR.
5. Keep one logical change in a small PR. Split unrelated work.
6. Run focused checks, update public docs, and leave a small reviewable diff.

## Contribution language

Use English for issues, PRs, documentation, specifications, plans, code
comments, and review discussion. Product localization values can use their
target language. Keep the surrounding explanation in English.

## Discuss large architectural changes first

Open an issue before you implement or submit a large architectural change. This
includes a new subsystem, a public API or protocol change, a persistence or
data-model change, a new execution boundary, an authentication or
permission-model change, or a cross-cutting change across subsystems.

Describe the problem, proposed direction, affected boundaries, alternatives,
and migration or compatibility risks in the issue. Wait until maintainers have
discussed the direction before opening the PR. Link the issue from the PR.
An agent must stop and report missing discussion instead of opening the PR.

## Keep PRs small

Prefer one logical change and the smallest practical diff. Split unrelated
cleanup, refactoring, documentation, and feature work into separate PRs.
Smaller PRs reduce the risk surface, make review easier, and reduce maintainer
burden.

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

This is the normal development path. The TypeScript supervisor starts the Go backend and Vite, selects available ports, points Go at Vite, and isolates application state under the checkout's `.kandev-dev/`. Use the printed URLs. Backend logs append to `.kandev-dev/logs/backend-logs.log`; startup prints the resolved path.

Automatic port selection only applies when no port was requested, and `KANDEV_BACKEND_PORT` or `KANDEV_PORT` in the environment counts as a request. An installed Kandev service that exported one therefore pins development mode to the port its own backend already occupies; the launcher rejects that request before readiness and never silently substitutes another requested port. Pass `PORT=` to override both the environment and the automatic choice, `WEB_PORT=` for the internal Vite port, and `DEV_ARGS=` for any other launcher flag.

```bash
make dev PORT=38430 WEB_PORT=37430
```

`make dev-web` starts only Vite on its fixed development port; it has no live API by itself. `make dev-backend` starts only the backend with the normal production-profile home unless you override it. For an intentionally isolated backend-only run:

```bash
KANDEV_HOME_DIR="$PWD/.kandev-dev" KANDEV_DEBUG_DEV_MODE=true make dev-backend
```

One backend owns a Kandev home at a time. Raw backend commands use the normal home by default, so a second backend with that home stops before it changes shared state. For an intentional second backend, use a separate `KANDEV_HOME_DIR`, database, and port.

Use `make build` for a production build. Use `make start` for a production-shaped local start; it installs dependencies, builds and synchronizes the embedded web application, then launches Kandev and writes `<resolved-home>/logs/backend-logs.log`.

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
- Large architectural changes have a linked issue with maintainer discussion
  before the PR opens.
- The PR contains one logical change and the smallest practical diff.
- Wire changes update Go DTOs, TypeScript types/clients, compatibility behavior, and protocol docs together.
- Persistence changes include fresh-schema and upgrade-path tests.
- Credentials, external text, shell arguments, URLs, paths, and logs respect their trust boundary.
- Exact automated and manual checks are recorded; skipped suites have a reason.
- UI changes include Playwright coverage plus screenshots or a short recording.
- Generated artifacts, local databases, recordings, and unrelated formatting are absent.
- You understand and can explain all submitted code, including agent-generated code.

The root `CONTRIBUTING.md` defines community and licensing policy. Contributions are under the repository's AGPL-3.0 license.

Continue with [Architecture](architecture.md), [Backend development](backend-development.md), [Web development](web-development.md), [Testing](testing.md), [Extending Kandev](extending-kandev.md), or [Release process](release-process.md).
