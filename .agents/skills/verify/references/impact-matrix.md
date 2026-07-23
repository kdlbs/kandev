# Verify Impact Matrix

Collect `base...HEAD`, staged, unstaged, and untracked paths. Deduplicate the
union, then run every matching row. Prefer package/suite targets over
individual test names so changed dependents remain covered.

When the planner supplies a last verified SHA and a `/commit` hook receipt,
read [hook-evidence.md](hook-evidence.md). Eligible hook evidence removes the
duplicate formatting/lint portions below; do not rerun them, and run every
uncovered command.

| Paths | Changed-mode commands |
| --- | --- |
| `AGENTS.md`, `CLAUDE.md`, `.agents/**`, `.claude/**`, `.codex/**`, `.cursor/**`, `.opencode/**`, ADRs/plans/specs | `git diff --check` and targeted `.github/scripts/lint-harness-files.py` inputs; parse changed TOML |
| `docs/public/**` | `node --test scripts/validate-public-docs.test.mjs` plus applicable harness lint |
| `.github/workflows/**` | `python3 .github/scripts/lint-action-pinning_test.py` plus applicable harness lint |
| `scripts/**`, `.github/scripts/**` | sibling syntax/test when obvious; otherwise `make test-scripts` |
| `apps/backend/**` | `make fmt-backend`, `make test-backend`, `make lint-backend` |
| `apps/web/**` | generate web metadata, `make fmt-web`, `make typecheck-web`, `make test-web`, `make lint-web` |
| `apps/cli/**` | workspace format, CLI TypeScript check/build, `make test-cli` |
| `apps/desktop/**` excluding `src-tauri` | desktop TypeScript check and the directly affected desktop smoke/test |
| `apps/desktop/src-tauri/**` | matching Rust toolchain, `cargo fmt --check`, `cargo check`, and scoped `cargo test` |
| `apps/packages/**` or shared TypeScript config | web metadata, workspace format/typecheck, web and CLI tests, web lint, desktop typecheck |

Use `mode=full` when the base/diff cannot be established; a changed path has no
safe row; the user requests it; delivery has no PR CI; or changes touch root
build/toolchain/dependency lockfiles, Makefiles, profiles, generated contracts,
release tooling, migrations/shared schemas, or unusually broad plan
implementation. Multiple known rows are not automatically ambiguous: run their
union when ownership and dependents are clear.

Do not run E2E unless acceptance or changed behavior requires it; targeted E2E
is separate evidence. A changed-scope pass permits commit/push, while PR CI
supplies the authoritative full matrix before readiness.
