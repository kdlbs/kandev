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

2. **Check pre-commit hooks:** Run:
   ```bash
   pre-commit --version 2>/dev/null && echo "INSTALLED" || echo "NOT_INSTALLED"
   ```
   - If **not installed**, warn the user: _"⚠️ pre-commit is not installed. Install it with `pip install pre-commit && pre-commit install` for automatic formatting and lint checks on every commit."_
   - If installed, check hooks are active:
     ```bash
     test -f .git/hooks/pre-commit && grep -q "pre-commit" .git/hooks/pre-commit && echo "ACTIVE" || echo "INACTIVE"
     ```
   - If **inactive**, warn the user: _"⚠️ pre-commit hooks are not active. Run `pre-commit install` to enable them."_
   - **Do not block** — continue with the remaining steps regardless.

3. **Run verify (MANDATORY — do NOT skip):** Delegate to the `verify` subagent to run the full verification pipeline (rebase, format, typecheck, test, lint). It will fix any issues it finds. **Wait for it to complete before proceeding.** Do NOT proceed to step 4 until verify passes. If verify cannot fix the failures, stop and surface the errors to the user — do not commit. Do NOT substitute this with a partial check (e.g. running only the changed package's tests).

4. **Stage files:** Stage relevant files (prefer specific files over `git add -A`).

5. **Commit:** Write a commit message following the format above. If changes span multiple concerns, consider separate commits.
