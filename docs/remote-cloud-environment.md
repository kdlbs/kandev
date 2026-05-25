# Remote Cloud Environment Instructions

Setup notes and caveats for developing Kandev in ephemeral cloud VMs (Cursor Cloud, Codex, GitHub Codespaces, or similar sandboxed environments).

## Runtime requirements

- **Go 1.26** — install to `/usr/local/go` and ensure `PATH` includes `/usr/local/go/bin`.
- **Node.js 24** — use `nvm install 24 && nvm use 24`.
- **pnpm 9.15.9** — matches `packageManager` in `apps/package.json`. Install with `npm install -g pnpm@9.15.9`.
- **golangci-lint v2** — required for `make lint-backend`. Install with `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`.
- **gcc** — required for CGO (SQLite FTS5). Pre-installed on most Ubuntu-based cloud VMs.
- **Azure Repos PR creation (optional)** — only needed when testing `worktree.create_pr` against Azure remotes outside the published Docker image:
  - [Azure CLI](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli) (`az`) on `PATH`
  - DevOps extension: `az extension add --name azure-devops`
  - Auth: `az login` or `export AZURE_DEVOPS_EXT_PAT=<pat>`
  - The `ghcr.io/kdlbs/kandev` image ships `gh`, `az`, and the `azure-devops` extension preinstalled (see root `Dockerfile`).
  - Host routing table: `apps/backend/internal/agentctl/AGENTS.md`.

## Generated files before typecheck

Before running `make typecheck`, you must generate two files that are gitignored:

```bash
node apps/web/scripts/generate-release-notes.mjs
node apps/web/scripts/generate-changelog.mjs
```

Without these, `tsc` fails with missing module errors for `@/generated/release-notes.json` and `@/generated/changelog.json`.

## Running dev mode

`make dev` from the repo root builds the backend and starts both the Go backend (port 38429) and Next.js frontend (port 37429). The CLI launcher sets `KANDEV_DEBUG_DEV_MODE=true` which activates the `dev` profile from `profiles.yaml`, enabling mock agent and other dev conveniences. No external services (database, message queue, Docker) are needed — SQLite is embedded and the event bus runs in-memory.

## Key commands

All documented in the root `Makefile`:
- `make dev` — build + start backend + web (dev mode)
- `make test` — backend (Go) + web (vitest) + CLI tests
- `make lint` — golangci-lint + ESLint
- `make typecheck` — TypeScript type-checking across all apps
- `make fmt` — format all code (run before lint to avoid false positives)
- `make test-e2e` — Playwright E2E tests (requires `make build-backend && make build-web` first). Uses `e2e/playwright.config.ts`. 1000+ specs.

## Firecracker / cloud-VM caveats

These apply to Cursor Cloud, Codex, and similar Firecracker-backed sandboxes:

- **golangci-lint PATH**: `go install` places the binary in `~/go/bin/`. Ensure `PATH` includes it (the update script appends to `~/.bashrc`, but in-session shells may need `export PATH="$PATH:$HOME/go/bin"`).
- **First page load**: `make dev` compiles pages on first visit via Turbopack — expect ~25 s for the initial load.
- **CLI test flake**: `src/ports.test.ts > isPortInUse > returns false within the timeout when the host black-holes packets` fails because `192.0.2.1` behaves differently inside Firecracker. This is a known environment-specific flake, not a code issue.
- **Playwright browser installation**: `playwright install chromium` hangs during zip extraction (Node.js io_uring incompatibility with the Firecracker kernel). Workaround: `scripts/install-playwright-browsers.sh` runs the install with a timeout, then falls back to extracting the already-downloaded zips with `unzip`. The update script calls this automatically. System deps can be installed via `playwright install-deps chromium`.
