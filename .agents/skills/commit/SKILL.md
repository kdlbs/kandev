---
name: commit
description: Stage and commit changes using Conventional Commits. Use when there are dirty/staged files to commit, the user says "commit", or before pushing a PR.
---

# Commit

Create a git commit following this project's Conventional Commits convention. These messages are used by git-cliff (`cliff.toml`) to auto-generate changelogs and release notes. PRs are squash-merged, so the PR title becomes the commit on `main` — CI validates it via `pr-title.yml`.

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

**Create a todo/task for each step below and mark them as completed as you go.**

1. Run `git status` and `git diff` to understand all changes
2. Review recent commits with `git log --oneline -10` to match project style
3. Run `/verify` as a **sub-agent** to ensure formatters, linters, typechecks, and tests all pass. Fix any issues before committing.
4. Stage relevant files (prefer specific files over `git add -A`)
5. Write a commit message following the format above
6. If changes span multiple concerns, consider separate commits
