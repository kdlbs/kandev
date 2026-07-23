# Kandev Engineering Guide

> **Purpose**: Architecture notes, key patterns, and conventions for LLM agents working on Kandev.

## Repo Layout

```
apps/
├── backend/          # Go backend (orchestrator, lifecycle, agentctl, WS gateway)
├── web/              # Vite/React SPA frontend (Go boot payload + WS + Zustand)
├── desktop/          # Tauri desktop shell around the native runtime
├── cli/              # CLI tool (TypeScript)
└── packages/         # Shared packages/types
```

## Tooling

- **Package manager**: `pnpm` workspace (run from `apps/`, not repo root)
- **Backend**: Go with Make (`make -C apps/backend test|lint|build`)
- **Frontend**: Vite/React SPA (`cd apps && pnpm --filter @kandev/web dev|build:vite|lint`; for direct web typecheck use `cd apps/web && pnpm run typecheck`)
- **Desktop**: Tauri shell (`cd apps && pnpm --filter @kandev/desktop build|e2e`; Rust tests from `apps/desktop/src-tauri`)
- **UI**: Shadcn components via `@kandev/ui`
- **E2E**: Playwright (`cd apps/web && pnpm e2e`). The `containers` project (gated on `KANDEV_E2E_CONTAINERS=1`, formerly `docker`) covers both the Docker executor and the SSH executor — anything that needs a real Docker daemon on the host lives there. See `apps/web/e2e/README.md`.
- **GitHub repo**: `https://github.com/kdlbs/kandev`
- **Container image**: `ghcr.io/kdlbs/kandev` (GitHub Container Registry)

### Worktrees and commit hooks

A fresh git worktree shares `.git/` but **not** `apps/node_modules/`. The missing install breaks not just the commit-msg hook (`pnpm exec commitlint` → `Command "commitlint" not found` / `ERR_PNPM_RECURSIVE_EXEC_FIRST_FAIL`) but any pnpm command — `vitest` fails with `Failed to resolve import "vitest"`, eslint similarly. Run `pnpm install --frozen-lockfile` from `apps/` once after creating the worktree, before running tests/lint/commits; subsequent pnpm commands work normally.

---

## Scoped guidance

Architecture notes and per-area conventions live alongside the code they describe. Read the file in the directory you're working in:

- `apps/backend/AGENTS.md` — Go backend: package structure (incl. `internal/office/` and `internal/agent/runtime/`), key concepts (orchestrator, workflow engine, agent runtime, lifecycle manager), execution flow, provider pattern, backups, testing conventions, Go lint limits.
- `apps/backend/internal/agentctl/AGENTS.md` — agentctl HTTP server: route groups, adapter model, ACP protocol.
- `apps/backend/internal/agentctl/server/api/AGENTS.md` — reverse-proxy body rewriting (`Accept-Encoding`), iframe-blocking header stripping.
- `apps/backend/internal/integrations/AGENTS.md` — adding a new third-party integration (Jira/Linear pattern, both backend and frontend halves). The `/add-integration` skill mirrors this for scaffolding new integrations.
- `apps/desktop/AGENTS.md` — Tauri desktop app: runtime resources, Rust process lifecycle, packaging, signing, and smoke tests.
- `apps/web/AGENTS.md` — Vite/React SPA frontend: shadcn imports, Go boot-payload hydration, store slice structure (incl. `office`), WS format, component conventions, TS lint limits.

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
- **Public docs:** Website-ready user documentation lives in `docs/public/**`. Use `/docs-maintainer` when a change affects CLI commands, config keys, install/deploy flows, workflows, executors, public APIs, screenshots, or user-facing terminology.
- **Specs:** Feature specs live in `docs/specs/<slug>/spec.md` — the durable "what & why" of a feature, written before coding. Use `/spec` to write or update a spec. See `docs/specs/INDEX.md`.
- **Decisions:** Architecture decisions are recorded in `docs/decisions/`. Read `docs/decisions/INDEX.md` for an overview. When making significant architectural choices, create a new ADR via `/record decision`.
- **Plans:** Implementation plans are generated from specs via `/plan` and committed under `docs/plans/<slug>/plan.md`, with individual sibling task files named `docs/plans/<slug>/task-<NN>-<short-slug>.md`. Specs are the living requirements; plans and task files are implementation records for the current buildout.

### Plan Implementation
- After implementing a substantial plan, commit through active hooks, then
  delegate `verify` with `mode=full` before push. Full mode ignores hook
  omissions and runs `make fmt` before `make typecheck test lint`; formatting
  comes first because it may split lines and expose complexity-linter failures.

### Observability
- In dev mode (`KANDEV_MOCK_AGENT=true` or `debug.pprofEnabled`), `/debug/vars` exposes the stdlib expvar handler. Office provider-routing metrics live under `routing_*` (route attempts, fallbacks, parked runs, provider degraded/recovered counters). The metrics are also still emitted as structured `routing.metric.*` zap logs for human debugging.

### GitHub Operations
Skills use `gh` CLI by default. If a `gh` command fails (not installed, not authenticated, etc.), use whatever GitHub tools are available in the environment (MCP GitHub tools, API tools, etc.) to accomplish the same operation. The goal is the same — the tool may differ.

For multiline Markdown issue or PR bodies, write the body to a file and pass it
with the relevant `gh ... --body-file <path>` option. Do not send escaped
newlines through `--body`; GitHub will render them literally.

For PR review/fixup workflows, prefer the repo helpers before manually querying GitHub/GraphQL: `scripts/pr-state --summary <PR>` for checks and unresolved-thread state, `scripts/pr-state --comment <comment_id>` for a full review-comment body, `scripts/pr-resolve list <PR>` for actionable unresolved review threads, and `scripts/pr-resolve reply <PR> <comment_id> <thread_id> "<body>"` to reply, resolve, and react in one call.

When a Kandev system message references an MCP tool that is not visible in the active tool list, use the runtime's tool discovery mechanism, such as `tool_search` when available, before falling back to a less specific workflow. Some task messaging and platform helpers are exposed on demand.

### Planner and Worker Execution

The user-started primary session is the planner and default architect: it owns
clarification, first-pass investigation, architecture, specs, plans, task
decomposition, integration judgment, and user communication. It may directly perform small scoped work
when one clear concern touches a few localized files, has no useful isolation or
parallelism benefit, and has quick bounded verification. Follow the applicable
skill, protect unrelated dirty changes, and obtain delegated Spark `verify`
after commit and before push when code, tests, or config changed. Pass the
successful non-bypassed hook receipt so changed-scope verification skips only
equivalent hook-covered checks; PR CI remains the authoritative full matrix.

Delegate only when it has positive ROI or independent evidence is essential:
broad/unknown exploration, substantial plan tasks, large/cross-component work,
parallel packets, long/noisy E2E or debugging, exceptional specialist review,
and final change-aware `verify`. Keep long PR monitoring on cheap `pr-poller`.
Delegation is not default ceremony: weigh context reload and coordination cost.
Do not delegate routine log/session inspection, call-path mapping, reproduction,
waiting, or status reporting; investigate and report those directly first.
Each delegated worker executes one bounded packet and does not spawn agents.
Never use Kandev MCP task/session APIs as a delegation fallback. The architect
agent is only for a user-requested independent architecture second opinion.
Detailed routing lives in `planner-orchestration`.

### Kandev Task Creation

Use Kandev task/session MCP APIs only when the user explicitly asks to create or
manage persistent Kandev platform tasks or sessions. Planner-to-worker
delegation must use the active coding harness's native subagent tools; never use
`create_task_kandev`, `spawn_session_kandev`, or `message_task_kandev` as a
worker mechanism or fallback.

When the user explicitly requests related Kandev follow-up work, use
`create_task_kandev` with `parent_id: "self"`. That preserves workspace,
workflow, repository, agent profile, and executor context from the current
task. For genuinely unrelated top-level tasks, do not rely on workspace
defaults when the user expects continuity; explicitly preserve the current
task's `agent_profile_id` / `executor_profile_id`, or ask if the intended
profile is ambiguous.

When the user requests a persistent remediation task discovered during PR
review that must start after merge, create it with `parent_id: "self"`,
`workspace_mode: "new_workspace"`, and the reviewed PR's base branch; otherwise
a same-repository subtask inherits the reviewed branch. Set `start_agent: false`
when the follow-up is intentionally queued until merge.

### Third-party integrations

Jira and Linear are the model (per-workspace credentials, 90s auth-health poller via `internal/integrations/healthpoll`, settings page with status banner). New integrations should **reuse the shared shapes** rather than copying either. Full layout, file conventions, and Jira-vs-Linear divergence notes in `apps/backend/internal/integrations/AGENTS.md` and the `/add-integration` skill — load either when scaffolding a new integration.

### Kandev plugins

Production Kandev plugins live in dedicated repositories, not in this monorepo. Official plugins use public `kdlbs/kandev-plugin-<slug>` repositories and start from [`kdlbs/kandev-plugin-template`](https://github.com/kdlbs/kandev-plugin-template). Use `/create-kandev-plugin` for plugin creation, modification, bug fixes, packaging, release, and marketplace work. When a Kandev Worktree task needs the plugin repository, attach it with `add_branch_to_task_kandev` instead of cloning inside this worktree. Keep host API, SDK, loader, registry, and the in-tree test fixture in this repository.

### Runtime profiles (prod / dev / e2e)

**`profiles.yaml` at the repo root** is the single source of truth for env-driven runtime defaults — feature flags, mock providers (agent / GitHub / Jira / Linear), debug switches, and e2e tuning knobs. The backend embeds it (`//go:embed` via `apps/backend/internal/profiles/`) and at startup calls `profiles.ApplyProfile()` to write the matching profile's env vars onto its own process, *only when each var is not already set* — so launchers, shells, and per-spec overrides still win.

Runtime feature toggles add a SQLite-backed override tier managed through `Settings > System > Feature Toggles`. Effective values use this precedence: explicit environment variable > SQLite override > profile default. The typed runtime flag registry lives in `apps/backend/internal/runtimeflags/registry.go`; add or update registry metadata when exposing a flag in the UI.

Profile selection: `KANDEV_E2E_MOCK=true` → `e2e`, `KANDEV_DEBUG_DEV_MODE=true` → `dev`, otherwise `prod`. `apps/cli/src/dev.ts` and `apps/web/e2e/fixtures/backend.ts` set only the selector — they no longer hardcode the underlying values.

To flip a feature on for every user: change its `prod:` to `"true"` in `profiles.yaml`. To add a new feature flag: 1 line in `profiles.yaml` + 1 `FeaturesConfig` field + the runtime flag registry entry when it should be user-toggleable + the gate at the call site + the frontend additions (`FeatureFlags` type, `useFeature` checks, `notFound()` from a server-side layout). Full pattern in `docs/decisions/0007-runtime-feature-flags.md`; runtime overrides and restart support are documented in `docs/decisions/0018-runtime-settings-overrides.md` and `docs/decisions/0019-restart-supervisor.md`.

---

## Maintaining This File

This file is read by AI coding agents (Claude Code via `CLAUDE.md` symlink, Codex via `AGENTS.md`). If your changes make any section of this file outdated or inaccurate - e.g., you add/remove/rename packages, change architectural patterns, add new adapters, modify store slices, or change conventions - **update the relevant sections of this file as part of the same PR**. Keep descriptions concise and factual. Do not add speculative or aspirational content.

When a change is scoped to a single subtree, update the scoped `AGENTS.md` instead of (or in addition to) this root file. See the "Scoped guidance" pointers at the top.

---

## Remote cloud environment

For developing in ephemeral cloud VMs (Cursor Cloud, Codex, GitHub Codespaces, etc.), see [`docs/remote-cloud-environment.md`](docs/remote-cloud-environment.md) — covers runtime requirements, generated-file gotchas, dev-mode setup, key commands, and Firecracker-specific caveats.

---

**Last Updated**: 2026-07-21
