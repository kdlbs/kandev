---
title: "Contributing to Kandev"
description: "Set up the repository, choose the right change boundary, run checks, update public docs, and prepare a reviewable pull request."
---

# Contributing to Kandev

Kandev is a Go backend, Vite/React web application, Node launcher, Tauri desktop shell, and a collection of runtime helpers. Start by understanding the user-visible behavior and the ownership boundary of the subsystem you intend to change.

## Set up the repository

The pinned toolchain is defined in `mise.toml`: Node 24, pnpm 9.15.9, Go 1.26.0, and supporting tools.

```bash
git clone https://github.com/kdlbs/kandev.git
cd kandev
make bootstrap
```

For browser end-to-end work, also run:

```bash
make bootstrap-e2e
```

`make bootstrap` installs the pinned tools when needed, installs workspace dependencies, and configures repository hooks. Run `make doctor` when local and CI behavior disagree.

## Start the development application

```bash
make dev
```

This starts the Go backend and Vite web development server with automatic port selection. The terminal output shows the actual URLs. Use `make dev-backend` or `make dev-web` when isolating one side.

The production-shaped local path is:

```bash
make build
make start
```

## Find the right layer

| Change | Start in |
|---|---|
| Task, workflow, repository, integration, persistence, or API behavior | `apps/backend/internal/<domain>/` |
| Agent process, ACP, MCP, terminal, Git, or remote runtime behavior | `apps/backend/internal/agent/`, `agentctl/`, or the executor package |
| User interface, state, routing, or browser API calls | `apps/web/` |
| CLI launch/install behavior | `apps/cli/` and `apps/backend/internal/launcher/` |
| Desktop shell behavior | `apps/desktop/` |
| Public user documentation | `docs/public/` |
| CI/release behavior | `.github/workflows/` and `scripts/` |

Read the nearest tests and existing domain types before adding a new abstraction. Prefer an established handler/service/repository or API/state/component path over a parallel framework.

## Develop with focused checks

During iteration, run the narrowest relevant command. Before opening a PR, run:

```bash
make fmt
make typecheck
make test
make lint
```

Any change under `apps/web/` must add or update Playwright coverage in `apps/web/e2e/` and run `make test-e2e`. See [Testing](testing.md) for targeted commands and fixture rules.

## Update public behavior and docs together

If a change affects commands, configuration, settings, workflows, executors, integrations, APIs, screenshots, status language, or user-facing terminology, update `docs/public/**` in the same PR.

When adding a page:

1. create a flat, stable Markdown slug in `docs/public/`;
2. add title and description frontmatter;
3. add the slug once to `docs/public/meta.json`;
4. link it from the relevant overview and related pages;
5. run `node scripts/validate-public-docs.mjs`.

The full publication contract is in the [docs contribution guide](README.md).

## Prepare the pull request

- Keep one logical change per PR.
- Explain user impact and the technical boundary.
- List exact automated and manual verification.
- Include screenshots or a short recording for UI behavior.
- Call out migrations, new credentials, security changes, or compatibility risks.
- Do not mix generated artifacts, local databases, recordings, or unrelated formatting into the diff.
- Understand and be able to explain any agent-generated code you submit.

The root `CONTRIBUTING.md` contains the community and licensing policy. By contributing, you agree to the repository's AGPL-3.0 license.

## Next guides

- [Architecture](architecture.md)
- [Backend development](backend-development.md)
- [Web development](web-development.md)
- [Testing](testing.md)
- [Extending Kandev](extending-kandev.md)
- [Release process](release-process.md)
