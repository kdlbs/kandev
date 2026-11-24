# Backend (Go) — architecture and conventions

Scoped guidance for `apps/backend/`. Repo-wide rules (commit format, code-quality limits, etc.) live in the root `AGENTS.md`. For plugin work, start at the [canonical plugin authoring guide](../../docs/public/plugins-authoring.md), follow choose recipe → edit `manifest.yaml` → implement → validate → package → smoke test, and treat `../../pluginsdk`, `../../pluginsdk/proto/kandev/plugin/v1/plugin.proto`, `internal/plugins/manifest`, and `internal/plugins/pkgtar` as authoritative; `cmd/plugin-fixture` is test support, including the remote executor provider and fake HTTPS service, and plugins must not access databases or `internal/...` packages.

## Package Structure

For the backend package map and canvas ownership details, read [package structure](PACKAGE-STRUCTURE.md).

## Key Concepts

Before changing backend subsystem behavior, read the relevant sections of [backend key concepts](KEY-CONCEPTS.md), including the authorization and lifecycle invariants that apply to the change.

## Execution Flow

When changing task execution or lifecycle, read [execution flow](EXECUTION-FLOW.md).

## Conventions

Before changing backend code, read the relevant [backend conventions](CONVENTIONS.md), including goroutine ownership and test requirements.

## Backups and retention

For backups, retention, database schema, or migrations, read [persistence guidance](PERSISTENCE-GUIDANCE.md).

## Internationalization

`internal/i18n` renders only browser-facing copy: SPA-unavailable pages and shared-task artifacts. Diagnostics, logs, agent/ACP output, and CLI output remain English. Use `i18n.T`/`i18n.Tf` with explicit locale threading (including interpolation/plurals); resolve artifact locale at creation. Catalogs are embedded in `internal/i18n/locales/`; regenerate `pseudo` with `pnpm run i18n:pseudo`.

**Table-rebuild migrations:** When a legacy or constraint migration recreates a table, mirror every new column in the replacement `CREATE TABLE` and `INSERT ... SELECT` copy list; add a replay regression test proving values, including timestamps, survive. **Destructive cutover migrations:** Build a legacy schema with `NewWithDB` on a fresh database, replace final-shaped tables with legacy-shaped ones, inject a test-only failpoint after each cutover step, and assert byte-equivalent rollback; run the same matrix with `KANDEV_TEST_POSTGRES_DSN`, looking up PostgreSQL constraint names dynamically because they truncate at 63 bytes.

Every built-in SQL schema owner needs a descriptor in `internal/persistence/requiredstores` and a fixed adapter in `internal/persistence/storeconformance`. Bootstrap records each constructor through the tracker; missing required schema fails before readiness. Provider credentials and remote probes remain independently degradable.
For persistence changes, run `go run ./cmd/sqlguard ./internal` and `go test -race ./internal/persistence/storeconformance -count=1`; set `KANDEV_TEST_POSTGRES_DSN` for PostgreSQL coverage and update the explicit-tag upgrade fixture and manifest when schema history changes.

## Code-quality limits

Enforced by `apps/backend/.golangci.yml` (errors on new code only):
- Functions: ≤80 lines, ≤50 statements · Cyclomatic complexity: ≤15 · Cognitive complexity: ≤30 · Nesting depth: ≤5 · Naked returns only in functions ≤30 lines · No duplicated blocks (≥150 tokens) · Repeated strings → constants (≥3 occurrences) · Revive's 800-effective-line file limit also applies to test files; put new tests in a new file instead of appending to an already-large test file.

For local backend checks and PR fixups, use `bash scripts/lint-go-changed <changed-go-files>` from the repo root. Follow the [lint design](../../docs/specs/platform/system-design/go-pre-commit-lint.md) for file/base selection, resource defaults, and sequential validation; CI owns full-backend lint.
- **Build cache reuse:** Make build/test targets use `-trimpath`. Include it in direct `go build` and `go test` commands, including race/coverage runs, so identical packages share artifacts across worktrees. Diagnostic source paths use module paths instead of absolute worktree paths.
- `internal/launcher/` — native launcher owning every entrypoint (`dev`, `start`, `run`, `service`); `dev` runs `make -C apps/backend dev` with Vite as a supervised child, state under `<repoRoot>/.kandev-dev/`. The root `make dev` prebuilds only the copied launcher; the backend dev target builds the native agentctl and a linux/amd64 helper when the host is not Linux/amd64 (`docs/plans/go-dev-launcher/`).
- `internal/agentctl/AGENTS.md` — server routes, adapters, ACP; `cmd/mock-agent/AGENTS.md` — E2E scenario patterns and rebuild requirements
- `internal/agentctl/server/api/AGENTS.md` — reverse-proxy body rewriting (`Accept-Encoding`), iframe-blocking header stripping
- `internal/integrations/AGENTS.md` — playbook for adding a new third-party integration (Jira/Linear pattern)
- `docs/i18n.md` ("Backend") — `internal/i18n` covers only what Go renders straight to a browser: the SPA-unavailable error pages and the shared-task artifacts (`share.html`, gist README and description). Both are complete. Everything else stays English by design; for new user-facing output prefer a stable error code the frontend translates. Use `i18n.Tf` for anything carrying a value — never `fmt.Sprintf` a translated string, and never build a plural in Go. A locale for output that outlives the request is resolved once at write time and threaded as an argument, not a context value (ADR `2026-08-01-share-artifact-locale.md`).
