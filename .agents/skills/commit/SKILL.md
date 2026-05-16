---
name: commit
description: Stage and commit changes using Conventional Commits. Use when there are dirty/staged files to commit, the user says "commit", or before pushing a PR.
---

# Commit

Create a git commit following this project's Conventional Commits convention. These messages are used by git-cliff (`cliff.toml`) to auto-generate changelogs and release notes. PRs are squash-merged, so the PR title becomes the commit on `main` — CI validates it via `pr-title.yml`.

## Available skills and subagents

- **`verify` subagent** — Run fmt, typecheck, test, and lint. Delegate to this before committing to catch issues early.

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

## Examples

```
feat: add release notes dialog
fix: flaky test in orchestrator (#292)
refactor: extract session handler into separate module
chore: update dependencies
ci: add PR title linting workflow
```

## Steps

**Create a task for each step below and mark them as completed as you go.**

1. **Understand changes:** Run `git status` and `git diff` to understand all changes. Review recent commits with `git log --oneline -10` to match project style.

2. **Ensure pre-commit hooks are wired up.** This must work in worktrees too, where `.git/` is a file (not a directory) and the real hooks path is shared with the main repo via `core.hooksPath`. Use `git rev-parse --git-path` so the check resolves correctly regardless:

   ```bash
   # Is the framework on PATH?
   pre-commit --version >/dev/null 2>&1 && echo "INSTALLED" || echo "NOT_INSTALLED"

   # Is the hook actually wired into git's hook system?
   HOOK_PATH=$(git rev-parse --git-path hooks/pre-commit)
   test -f "$HOOK_PATH" && grep -q "pre-commit" "$HOOK_PATH" && echo "ACTIVE" || echo "INACTIVE"
   ```

   - If **NOT_INSTALLED**, tell the user once: _"⚠️ pre-commit is not on PATH. Install it with `pip install pre-commit` so format/lint runs on every commit."_ Then continue (don't block).
   - If installed but **INACTIVE**, **install it yourself** — the project ships `.pre-commit-config.yaml` and `make doctor` is a no-op-on-already-installed wrapper around the same command:
     ```bash
     pre-commit install -t pre-commit -t commit-msg --overwrite
     ```
     Mention that you wired it up. Subsequent commits will run hooks automatically.
   - If both checks pass, no output needed.

   Why this matters: a missing hook lets lint regressions slip past local commits and only surface in CI (e.g. funlen / cognitive complexity on backend Go code). The hook catches them in <1s at commit time. See `Makefile`'s `doctor` target for the idempotent install command.

3. **Run verify (MANDATORY — do NOT skip):** Delegate to the `verify` subagent to run the full verification pipeline (rebase, format, typecheck, test, lint). It will fix any issues it finds. **Wait for it to complete before proceeding.** Do NOT proceed to step 4 until verify passes. If verify cannot fix the failures, stop and surface the errors to the user — do not commit. Do NOT substitute this with a partial check (e.g. running only the changed package's tests).

4. **Stage files:** Stage relevant files (prefer specific files over `git add -A`).
   - **Splitting commits with new files:** When introducing a brand-new file alongside the file that uses it, stage them together. The Go lint pre-commit hook stashes *unstaged* changes before linting but keeps *untracked* files in the working tree — so a new helper committed alone, while its (still-unstaged) caller sits in the working tree, lints as `unused` and rejects the commit.

5. **Commit:** Write a commit message following the format above. If changes span multiple concerns, consider separate commits.
