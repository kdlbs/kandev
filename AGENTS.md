# Kandev Engineering Guide

> **Purpose**: Architecture notes, key patterns, and conventions for LLM agents working on Kandev.

## Repo Layout

```
apps/
├── backend/          # Go backend (orchestrator, lifecycle, agentctl, WS gateway)
├── web/              # Next.js frontend (SSR + WS + Zustand)
├── cli/              # CLI tool (TypeScript)
├── landing/          # Landing page
└── packages/         # Shared packages/types
```

## Tooling

- **Package manager**: `pnpm` workspace (run from `apps/`, not repo root)
- **Backend**: Go with Make (`make -C apps/backend test|lint|build`)
- **Frontend**: Next.js (`cd apps && pnpm --filter @kandev/web dev|lint|typecheck`)
- **UI**: Shadcn components via `@kandev/ui`
- **GitHub repo**: `https://github.com/kdlbs/kandev`
- **Container image**: `ghcr.io/kdlbs/kandev` (GitHub Container Registry)

### Worktrees and commit hooks

The commit-msg hook runs `cd apps && pnpm exec commitlint --edit "$1"`. A fresh git worktree shares `.git/` but **not** `apps/node_modules/`, so the first commit attempted in a worktree fails with `Command "commitlint" not found` / `ERR_PNPM_RECURSIVE_EXEC_FIRST_FAIL`. Run `pnpm install --frozen-lockfile` from `apps/` in the worktree once before the first commit; subsequent commits work normally.

---

## Scoped guidance

Architecture notes and per-area conventions live alongside the code they describe. Read the file in the directory you're working in:

- `apps/backend/AGENTS.md` — Go backend: package structure (incl. `internal/office/` and `internal/agent/runtime/`), key concepts (orchestrator, workflow engine, agent runtime, lifecycle manager), execution flow, provider pattern, backups, testing conventions, Go lint limits.
- `apps/backend/internal/agentctl/AGENTS.md` — agentctl HTTP server: route groups, adapter model, ACP protocol.
- `apps/backend/internal/agentctl/server/api/AGENTS.md` — reverse-proxy body rewriting (`Accept-Encoding`), iframe-blocking header stripping.
- `apps/backend/internal/integrations/AGENTS.md` — adding a new third-party integration (Jira/Linear pattern, both backend and frontend halves). The `/add-integration` skill mirrors this for scaffolding new integrations.
- `apps/web/AGENTS.md` — Next.js frontend: shadcn imports, SSR-hydrate-store data flow, store slice structure (incl. `office`), WS format, component conventions, TS lint limits.

---

## Best Practices

### Commit Conventions (enforced by CI)

Commits to `main` **must** follow [Conventional Commits](https://www.conventionalcommits.org/) (`type: description`). PRs are squash-merged - the PR title becomes the commit, validated by CI. Changelog is auto-generated from these via git-cliff (`cliff.toml`). See `.agents/skills/commit/SKILL.md` for allowed types and examples.

The commitlint hook caps the header at **100 characters** (`type(scope): description`). The pre-commit prettier hook also reformats staged TS/TSX files - if it does, the commit fails. Re-stage the reformatted files and create a new commit (don't `--amend`).

### Release & Versioning

Kandev uses a **single SemVer** `X.Y.Z` across npm, Homebrew, and GitHub release; release flow runs entirely in CI via `.github/workflows/release.yml`. Full details in the `/release` skill — load it when cutting a release or debugging release artifacts.

### Code Quality

Static analysis runs in CI and pre-commit. Each subtree has its own thresholds:

- Go limits: see `apps/backend/AGENTS.md` (and `apps/backend/.golangci.yml`).
- TypeScript limits: see `apps/web/AGENTS.md` (and `apps/web/eslint.config.mjs`).

When you hit a limit: extract a helper function, custom hook, or sub-component. Prefer composition over growing a single function.

### Testing

Every code change must include tests for new or changed logic. Backend: `*_test.go` files alongside the source. Frontend: `*.test.ts` files for utility functions, hooks, API clients, and store slices. Exceptions: config files, generated code, React component markup. Use `/tdd` for test-driven development.

### Knowledge
- **Specs:** Feature specs live in `docs/specs/<slug>/spec.md` — the "what & why" of a feature, written before coding. Optional siblings: `plan.md` (how), `notes.md` (post-ship). Use `/spec` to write or update a spec. See `docs/specs/INDEX.md`.
- **Decisions:** Architecture decisions are recorded in `docs/decisions/`. Read `docs/decisions/INDEX.md` for an overview. When making significant architectural choices, create a new ADR via `/record decision`.
- **Plans:** Implementation plans are generated from specs via `/plan` and saved to `docs/specs/<slug>/plan.md`. Plans are gitignored (working files, not committed) — only specs and ADRs are tracked.

### Plan Implementation
- After implementing a plan, run `make fmt` first to format code, then run `make typecheck test lint` to verify the changes. Formatting must come first because formatters may split lines, which can trigger complexity linter warnings.

### Observability
- In dev mode (`KANDEV_MOCK_AGENT=true` or `debug.pprofEnabled`), `/debug/vars` exposes the stdlib expvar handler. Office provider-routing metrics live under `routing_*` (route attempts, fallbacks, parked runs, provider degraded/recovered counters). The metrics are also still emitted as structured `routing.metric.*` zap logs for human debugging.

### GitHub Operations
Skills use `gh` CLI by default. If a `gh` command fails (not installed, not authenticated, etc.), use whatever GitHub tools are available in the environment (MCP GitHub tools, API tools, etc.) to accomplish the same operation. The goal is the same — the tool may differ.

### Third-party integrations

Jira and Linear are the model (per-workspace credentials, 90s auth-health poller via `internal/integrations/healthpoll`, settings page with status banner). New integrations should **reuse the shared shapes** rather than copying either. Full layout, file conventions, and Jira-vs-Linear divergence notes in `apps/backend/internal/integrations/AGENTS.md` and the `/add-integration` skill — load either when scaffolding a new integration.

### Runtime profiles (prod / dev / e2e)

**`profiles.yaml` at the repo root** is the single source of truth for env-driven runtime defaults — feature flags, mock providers (agent / GitHub / Jira / Linear), debug switches, and e2e tuning knobs. The backend embeds it (`//go:embed` via `apps/backend/internal/profiles/`) and at startup calls `profiles.ApplyProfile()` to write the matching profile's env vars onto its own process, *only when each var is not already set* — so launchers, shells, and per-spec overrides still win.

Profile selection: `KANDEV_E2E_MOCK=true` → `e2e`, `KANDEV_DEBUG_DEV_MODE=true` → `dev`, otherwise `prod`. `apps/cli/src/dev.ts` and `apps/web/e2e/fixtures/backend.ts` set only the selector — they no longer hardcode the underlying values.

To flip a feature on for every user: change its `prod:` to `"true"` in `profiles.yaml`. To add a new feature flag: 1 line in `profiles.yaml` + 1 `FeaturesConfig` field + the gate at the call site + the frontend additions (`FeatureFlags` type, `useFeature` checks, `notFound()` from a server-side layout). Full pattern in `docs/decisions/0007-runtime-feature-flags.md`.

---

## Maintaining This File

This file is read by AI coding agents (Claude Code via `CLAUDE.md` symlink, Codex via `AGENTS.md`). If your changes make any section of this file outdated or inaccurate - e.g., you add/remove/rename packages, change architectural patterns, add new adapters, modify store slices, or change conventions - **update the relevant sections of this file as part of the same PR**. Keep descriptions concise and factual. Do not add speculative or aspirational content.

When a change is scoped to a single subtree, update the scoped `AGENTS.md` instead of (or in addition to) this root file. See the "Scoped guidance" pointers at the top.

---

**Last Updated**: 2026-05-16
