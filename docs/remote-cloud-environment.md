# Remote Cloud Environment Instructions

Setup notes and caveats for developing Kandev in ephemeral cloud VMs (Cursor Cloud, Codex, GitHub Codespaces, or similar sandboxed environments).

## Runtime requirements

- **Go 1.26** — install to `/usr/local/go` and ensure `PATH` includes `/usr/local/go/bin`.
- **Node.js 24** — use `nvm install 24 && nvm use 24`.
- **pnpm 9.15.9** — matches `packageManager` in `apps/package.json`. Install with `npm install -g pnpm@9.15.9`.
- **golangci-lint v2** — required for `make lint-backend`. Install with `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`.
- **gcc** — required for CGO (SQLite FTS5). Pre-installed on most Ubuntu-based cloud VMs.

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
