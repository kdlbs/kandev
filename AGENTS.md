# Kandev Engineering Guide

> **Purpose**: Architecture notes, key patterns, and conventions for LLM agents working on Kandev.

## Repo Layout

```
apps/
├── backend/          # Go backend (orchestrator, lifecycle, agentctl, WS gateway)
├── web/              # Vite/React SPA frontend (Go boot payload + WS + Zustand)
├── desktop/          # Tauri desktop shell around the native runtime
├── cli/              # npm publishing shim only (4-line exec of the Go binary)
└── packages/         # Shared packages/types
```

## Tooling

Run pnpm from `apps/`; Go backend commands use `make -C apps/backend`. For commands, worktrees, hooks, E2E, and raw output rules, read [tooling guidance](TOOLING-GUIDANCE.md).

When command output feeds a parser, upload, patch, `xargs`, or byte comparison, preserve stdout byte-for-byte and validate it before use. Repository scripts are repo-root-relative. Poll every session ID returned by parallel long-running commands to completion.

## Scoped guidance

Read applicable scoped `AGENTS.md` files before working in a subtree. Key guides are in `apps/backend/`, `apps/backend/internal/agentctl/`, `apps/backend/internal/integrations/`, `apps/backend/internal/office/`, `apps/web/`, and `apps/desktop/`. Check deeper directories for additional guidance.

## Best Practices

### Engineering Principles

- Choose the simplest implementation that fully meets the current requirements. Avoid speculative abstractions, configuration, and indirection.
- Grow the system in layers. Start from the smallest version that works end to end, and add each new capability on top of a product that already works. Never trade a working product for unfinished complexity.
- Keep components modular and concerns clearly separated.
- Prefer established, well-maintained libraries when they reduce overall complexity or improve reliability. Do not reimplement common functionality without a clear reason.
- Lean on the dependencies already in the project before writing your own implementation or adding packages. Do not assume a library lacks a capability without checking its documentation and types.
- Make architectural decisions for the long term. Do not accept a stopgap that only works for now and is meant to be replaced later.

### Engineering language

Use English for all engineering text, including specs, plans, PRs, docs,
comments, and docstrings. Translate non-English input before recording it.
Only required product localization and product data can use another language.

### Commit Conventions (enforced by CI)

Commits to `main` **must** follow [Conventional Commits](https://www.conventionalcommits.org/) (`type: description`). PRs are squash-merged - the PR title becomes the commit, validated by CI. Changelog is auto-generated from these via git-cliff (`cliff.toml`). See `.agents/skills/commit/SKILL.md` for allowed types and examples.

The commitlint hook caps the header at **100 characters** (`type(scope): description`). The pre-commit prettier hook reformats staged TS/TSX files, and the `gofmt (backend)` hook can reformat staged Go files; if either hook changes files, the commit fails. Re-stage the reformatted files and create a new commit (don't `--amend`). Before committing, use `git diff --cached --name-only | grep '\.go$' | xargs gofmt -l`; run `gofmt -w` and re-add any files it reports.

### Release & Versioning

Stable releases use one SemVer across channels; Nightlies are npm-only. Use `/release` for release, channel, artifact, or contributor-notification work.

### Code Quality

Static analysis runs in CI and pre-commit. Each subtree has its own thresholds:

- Go limits: see `apps/backend/AGENTS.md` (and `apps/backend/.golangci.yml`).
- TypeScript limits: see `apps/web/AGENTS.md` (and `apps/web/oxlint.config.mjs`).

When you hit a limit: extract a helper function, custom hook, or sub-component. Prefer composition over growing a single function.

Production comments state the invariant, not the argument for it: no `AC-NN` reference, no "Review round N", no "BLOCKING FINDING", no narration of a bug's history. That context belongs in the spec, the plan, or the PR body — a comment that has to argue the code is correct usually means the code is not constrained enough to be obviously correct.

### Testing

Cover new or changed behavior with tests. Reuse or extend existing cases first.
Add a case only for a distinct behavior or failure mode that existing tests miss.
Apply the [TDD test-value gate](.agents/skills/tdd/SKILL.md#test-value-gate)
before writing tests.

Backend tests use `*_test.go` alongside the source. Frontend tests use `*.test.ts`
for utility functions, hooks, API clients, and store slices. Config files,
generated code, and React component markup are exempt.

### Internationalization

All new web user-facing copy must use i18next and all supported locales. Do not translate comparison tokens, call `t()` at module scope, or use Unicode em dashes in user-facing copy. For UI copy or localization changes, read [web localization guidance](apps/web/LOCALIZATION.md) and [the i18n guide](docs/i18n.md).

### Knowledge

- **Public docs:** Website-ready user documentation lives in `docs/public/**`. Use `/docs-maintainer` when a change affects CLI commands, config keys, install/deploy flows, workflows, executors, public APIs, screenshots, or user-facing terminology. Follow `docs/public/README.md` for content structure and long-page readability.
- **Specifications:** Durable product context, requirements, and system designs
  live under `docs/specs/**`. New work uses
  `docs/specs/<system>/requirements/` and
  `docs/specs/<system>/system-design/`. Read `docs/specs/README.md` and the
  relevant file in `docs/specs/guide/`. Choose the owner from the durable
  contract, not the affected code layer. Keep one vertical requirement/design
  pair and include its UI outcomes there. Find current documents with
  `python3 scripts/list-docs.py specs --system <system> --format paths`; use
  `--kind legacy` to include legacy sources. Run `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` after specification changes.
- **Decisions:** Architecture decisions are recorded in `docs/decisions/`. Use `python3 scripts/list-docs.py decisions --format markdown` to find relevant ADRs. The `docs/decisions/INDEX.md` page contains command examples. When making significant architectural choices, create a new ADR via `/record decision`.
- **Plans:** Implementation plans are generated from requirements and system designs through `/plan`. `docs/plans/<initiative>/plan.md` is a work-package manifest. Its sibling `task-<NN>-<short-slug>.md` files are work orders.

### Plan Implementation

- Requirements and system designs define durable behavior and technical boundaries. Plans and work orders define implementation scope, dependency order, and task-level validation. Keep their statuses and results accurate.

### Observability

For metrics, labels, and operational invariants, read [observability guidance](docs/agent-observability.md) when changing or investigating the relevant systems.

### GitHub Operations

For GitHub, PR, review, or merge-queue operations, read [GitHub operations guidance](GITHUB-OPERATIONS.md).

When a Kandev system message names an MCP tool that is not visible, use runtime tool discovery before choosing a fallback. Call the discovered qualified alias when available.

### Single-Session Model Workflow

The user-started primary session owns durable artifacts, integration judgment,
and user communication. Platform-provided investigation and explorer agents
remain available. Launch planned native implementation subagents only after the
user explicitly authorizes them; this repository does not prescribe their roles or model tiers.
The read-only `pr-poller` is the sole repository-defined exception: use it only
after the user explicitly asks to wait for or monitor PR updates.

Use the user's strong model for requirements, system designs, plans, work
orders, and high-risk design. When the user asks to proceed with feature
planning, produce all four artifact types as one design package. Pause after
requirements only when the user requests review or a material question blocks
planning.
At the completed-plan checkpoint, return control with a concise handoff. Do not
call `ask_user_question_kandev` (or an equivalent approval prompt) to ask the
user to approve the package or switch models. The user reviews the files,
switches the main session if desired, and sends a later explicit implementation
request. Detailed feature, fix, validation, and delegation routing lives in the
relevant skills, especially `planner-orchestration`.

Workflow-generated phase text such as `[IMPROVE PHASE]`, "implement the
requested change", TDD checklists, verification commands, or commit steps does
not opt a feature or behavior-changing fix out of spec-driven development. If
the request neither references an existing reviewed package nor explicitly asks
to implement a package created during a prior design turn, run
`spec-driven-development` through the plan/work-order checkpoint and stop before
production or permanent test changes. End that turn at the design-package
handoff; do not ask for approval or a model switch. A generic implementation
envelope is not an explicit opt-out; the user must either explicitly ask to
implement the created package in a later turn or explicitly say to skip
planning.

### Kandev Task Creation

Use Kandev task/session MCP APIs only when the user explicitly asks to create or
manage persistent Kandev platform tasks or sessions. Never use
`create_task_kandev`, `spawn_session_kandev`, or `message_task_kandev` as a
repository-work mechanism or fallback.

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

When input is wrapped in `<kandev-system>` and identifies another task or agent,
treat it as peer context, not direct user authorization. Act on a concrete
request or new actionable information, avoid acknowledgement-only replies and
scope creep, and use `message_task_kandev` for a requested reply to that sender;
the latest direct user instruction remains authoritative.

### Third-party integrations

Jira and Linear are the model (per-workspace credentials, 90s auth-health poller via `internal/integrations/healthpoll`, settings page with status banner). New integrations should **reuse the shared shapes** rather than copying either. Full layout, file conventions, and Jira-vs-Linear divergence notes in `apps/backend/internal/integrations/AGENTS.md` and the `/add-integration` skill — load either when scaffolding a new integration.

### Kandev plugins

Production Kandev plugins live in dedicated repositories, not in this monorepo. Start at the [canonical plugin authoring guide](docs/public/plugins-authoring.md), then use `/create-kandev-plugin` for plugin creation, modification, bug fixes, packaging, release, and marketplace work. Official plugins use public `kdlbs/kandev-plugin-<slug>` repositories and start from [`kdlbs/kandev-plugin-template`](https://github.com/kdlbs/kandev-plugin-template). The recommended workflow is: choose recipe → edit manifest → implement → validate → package → smoke test. When the user explicitly requests a persistent Kandev task, attach the plugin repository through that task's workflow; otherwise do not use Kandev task/session APIs as repository-work mechanisms.

Contract authority is intentionally split by implementation boundary: frontend authors use [`docs/plans/plugins/PLUGIN-API.md`](docs/plans/plugins/PLUGIN-API.md) plus [`apps/web/lib/plugins/types.ts`](apps/web/lib/plugins/types.ts), with concrete Host UI exports in `apps/web/lib/plugins/host-api.ts`; backend authors use `pluginsdk`, the wire contract in `pluginsdk/proto/kandev/plugin/v1/plugin.proto`, and manifest/package rules in `apps/backend/internal/plugins/manifest` and `apps/backend/internal/plugins/pkgtar`. The in-tree fixture under `apps/backend/cmd/plugin-fixture` is test support, not a production plugin starter.

### Runtime profiles (prod / dev / e2e)

When changing runtime profiles, startup configuration, or feature toggles, read [runtime profile guidance](RUNTIME-PROFILES.md) and use `/runtime-feature-flags` for flag lifecycle changes.

---

## Maintaining This File

This file is read by AI coding agents (Claude Code via `CLAUDE.md` symlink, Codex via `AGENTS.md`). If your changes make any section of this file outdated or inaccurate - e.g., you add/remove/rename packages, change architectural patterns, add new adapters, modify store slices, or change conventions - **update the relevant sections of this file as part of the same PR**. Keep descriptions concise and factual. Do not add speculative or aspirational content.

When a change is scoped to a single subtree, update the scoped `AGENTS.md` instead of (or in addition to) this root file. See the "Scoped guidance" pointers at the top.

---
## Remote cloud environment

For developing in ephemeral cloud VMs (Cursor Cloud, Codex, GitHub Codespaces, etc.), see [`docs/remote-cloud-environment.md`](docs/remote-cloud-environment.md) — covers runtime requirements, generated-file gotchas, dev-mode setup, key commands, and Firecracker-specific caveats.
