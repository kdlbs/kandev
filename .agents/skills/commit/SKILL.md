---
name: commit
description: Stage and commit changes using Conventional Commits. Use when there are dirty/staged files to commit, the user says "commit", or before pushing a PR.
---

# Commit

## Planner Entry

Stage and commit changes in the primary conversation after the task-defined
implementation checks pass. Do not add broad post-commit `/verify` before push
by default. Preserve the hook receipt as normal commit evidence.

Create a git commit following this project's Conventional Commits convention. These messages are used by git-cliff (`cliff.toml`) to auto-generate changelogs and release notes. PRs are squash-merged, so the PR title becomes the commit on `main` — CI validates it via `pr-title.yml`.

## Available skills

- **`/pr-fixup`** — Use after the PR opens only for CI or actionable reviewer
  findings.

## Format

```
type: lowercase description
```

## Allowed Types

| Type | Use for | In changelog? |
|------|---------|---------------|
| `feat` | New features | Yes (Features) |
| `fix` | Bug fixes | Yes (Bug Fixes) |
| `perf` | Performance improvements | Yes (Performance) |
| `refactor` | Code refactoring | Yes (Refactoring) |
| `docs` | Documentation changes | Yes (Documentation) |
| `chore` | Maintenance, deps, configs | No |
| `ci` | CI/CD changes | No |
| `test` | Test-only changes | No |

## Rules

- Subject **must** start with a lowercase letter
- Scope is optional: `feat(ui): add dialog` is valid
- Include PR/issue number when relevant: `feat: add release notes (#295)`
- Breaking changes: add `!` after type: `feat!: remove legacy API`
- Keep the first line under 72 characters
- **Body lines must be ≤100 characters** (commitlint `body-max-line-length`). Hard-wrap bullet points before committing; long URLs or prose lines that exceed 100 chars will fail the hook with `body's lines must not be longer than 100 characters`. If a HEREDOC body fails, re-wrap and create a *new* commit — do not amend.

## Examples

```
feat: add release notes dialog
fix: flaky test in orchestrator (#292)
refactor: extract session handler into separate module
chore: update dependencies
ci: add PR title linting workflow
```

## Steps

Track these steps with an internal todo/checklist and mark them complete as you go.
Do not create, update, or delete Kandev subtasks for this workflow unless the user
explicitly requests task tracking.

1. **Understand changes:** Run `git status` and `git diff` to understand all changes. Review recent commits with `git log --oneline -10` to match project style.

2. **Ensure pre-commit hooks are wired up.** This must work in worktrees too, where `.git/` is a file (not a directory) and the real hooks path is shared with the main repo via `core.hooksPath`. Use `git rev-parse --git-path` so the check resolves correctly regardless:

   ```bash
   # Is the framework on PATH?
   pre-commit --version >/dev/null 2>&1 && echo "INSTALLED" || echo "NOT_INSTALLED"

   # Is the hook actually wired into git's hook system?
   PRE_COMMIT_HOOK_PATH=$(git rev-parse --git-path hooks/pre-commit)
   COMMIT_MSG_HOOK_PATH=$(git rev-parse --git-path hooks/commit-msg)
   test -f "$PRE_COMMIT_HOOK_PATH" && grep -q "pre-commit" "$PRE_COMMIT_HOOK_PATH" \
     && test -f "$COMMIT_MSG_HOOK_PATH" && grep -q "pre-commit" "$COMMIT_MSG_HOOK_PATH" \
     && echo "ACTIVE" || echo "INACTIVE"
   ```

   - If **NOT_INSTALLED**, tell the user once: _"⚠️ pre-commit is not on PATH. Install it with `pip install pre-commit` so format/lint runs on every commit."_ Then continue (don't block).
   - If installed but **INACTIVE**, **install it yourself** — the project ships `.pre-commit-config.yaml` and `make doctor` is a no-op-on-already-installed wrapper around the same command:
     ```bash
     pre-commit install -t pre-commit -t commit-msg --overwrite
     ```
     Mention that you wired it up. Subsequent commits will run hooks automatically.
   - If both checks pass, no output needed.

   - Before the first commit, ensure the worktree can run the commit-msg
     dependency. If `apps/node_modules/.bin/commitlint` is absent, install it
     from `apps/` before committing:
     ```bash
     cd apps && pnpm install --frozen-lockfile
     ```
     Retry the normal commit after the install; never bypass the hook.

   - After merging or rebasing the base branch, if `apps/package.json` or
     `apps/pnpm-lock.yaml` changed and a hook reports
     `ERR_MODULE_NOT_FOUND` even though `apps/node_modules/.bin/commitlint`
     exists, refresh the workspace dependencies from `apps/`:
     ```bash
     cd apps && pnpm install --frozen-lockfile
     ```
     Retry the normal commit without bypassing hooks.

   Why this matters: a missing hook lets lint regressions slip past local commits and only surface in CI (e.g. funlen / cognitive complexity on backend Go code). The hook catches them in <1s at commit time. See `Makefile`'s `doctor` target for the idempotent install command.

3. **Capture the parent SHA and preserve hook evidence:**
   ```bash
   git rev-parse HEAD
   ```
   Never use `--no-verify`, `SKIP`, or another hook-bypass option or
   environment variable. A bypassed commit is allowed only when the user
   explicitly requests it, and its receipt must say `bypass: true`; it never
qualifies as a successful hook receipt.

4. **Stage files:** Stage relevant files (prefer specific files over `git add -A`).
   - **Splitting commits with new files:** When introducing a brand-new file alongside the file that uses it, stage them together. The Go lint pre-commit hook stashes *unstaged* changes before linting but keeps *untracked* files in the working tree — so a new helper committed alone, while its (still-unstaged) caller sits in the working tree, lints as `unused` and rejects the commit.

5. **Commit:** Write a commit message following the format above. If changes span multiple concerns, consider separate commits.
   When `MERGE_HEAD` exists or a merge commit is being completed, use
   `git commit --no-edit` so a non-interactive runner does not open an editor;
   normal hooks still run. Confirm the merge commit exists and `MERGE_HEAD` is
   gone before reporting success.
   If a formatter changes files and prevents the commit, review and re-stage
   those files, then create a new commit attempt; do not use `--amend`.
   When editing harness files such as `AGENTS.md`, `CLAUDE.md`, or skills, run
   the shared validation in
   `.agents/skills/harness-improvement/references/validation.md` before
   committing.
   If a JSX layout-only edit touches an element containing an existing
   hardcoded user-facing literal, `i18n-new-code` may classify that literal as
   changed copy and fail. Localize it and add matching `en`/`pseudo` catalog
   entries before retrying; verify with `cd apps/web && pnpm run i18n:check` and
   the normal hook receipt.
   Capture the normal hook stream in a temporary log while committing and use
   that log to record each hook ID and result. Do not infer hook results from a
   condensed launcher summary. For example:
   ```bash
   COMMIT_LOG="$(mktemp "${TMPDIR:-/tmp}/kandev-commit.XXXXXX.log")"
   set -o pipefail
   git commit -m "type(scope): description" 2>&1 | tee "$COMMIT_LOG" >/dev/null
   ```
   Read the log to extract the hook receipt, rather than printing the full
   stream again. Remove the exact temporary file after copying the receipt into
   the handoff:
   ```bash
   unlink "$COMMIT_LOG"
   ```
   If the commit exits nonzero, preserve and print the full or bounded log
   before cleanup so hook diagnostics are not lost. Remove the temporary log
   only after copying a successful hook receipt or recording the failed output.

   If a hook fails only because another worktree is already running
   golangci-lint (for example, `parallel golangci-lint is running`), wait for
   that run to finish and retry the same commit. Do not bypass hooks or change
   code for this transient lock; verify the retry has a normal hook receipt and
   a clean worktree.

   If a hook fails with `ENOSPC`, load `/verify`'s Disk-constrained runners
   guidance: inspect `df`, preserve managed caches, relocate only the affected
   cache to an explicit persistent agent-owned path, and never bypass hooks or
   blindly delete shared caches.

6. **Return a hook receipt:** After a successful commit, report:
   ```text
   parent_sha: <pre-commit HEAD>
   commit_sha: <new HEAD>
   pre_commit_hook: active|inactive
   commit_msg_hook: active|inactive
   hook_results: <hook-id=passed|skipped, ...>
   bypass: false|true
   commit_result: pass
   worktree: clean|dirty
   ```
   `verify` may use the receipt only when both hooks are active, bypass is
   false, the commit succeeded, the current `HEAD` still equals `commit_sha`,
   and the worktree is clean. The commit worker does not run verification.
