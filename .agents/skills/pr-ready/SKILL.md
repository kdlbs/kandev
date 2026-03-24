---
name: pr-ready
description: Commit, push, create a ready-for-review PR, wait for CI and CodeRabbit, fix issues, and push.
---

## Context

- Current git status: !`git status`
- Current branch: !`git branch --show-current`
- Commits on this branch vs main: !`git log --oneline main..HEAD`
- Recent commit messages for style reference: !`git log --oneline -5`

## Your task

Commit, push, create a **ready-for-review** pull request, then wait for CI and code review and fix any issues.

### Steps

1. **Uncommitted changes:** If there are dirty or staged changes, run `/commit` first.

2. **Verify:** Run `/verify` to ensure formatters, linters, typechecks, and tests all pass. Fix any issues and commit before proceeding.

3. **Branch:** If on `main`, create a new branch (e.g. `feat/short-description` or `fix/short-description`) and switch to it. If already on a feature branch, use it.

4. **Push** the branch to origin with `-u` to set upstream tracking.

5. **Create the PR as ready** (no `--draft` flag). Follow the same rules as `/pr-draft` for title and body:

   **PR title** — Conventional Commits format:
   - Format: `type(scope): lowercase description` or `type: lowercase description`
   - Allowed types: `feat`, `fix`, `perf`, `refactor`, `docs`, `chore`, `ci`, `test`
   - Subject starts with a lowercase letter, under 72 characters
   - Add `!` after type for breaking changes

   **PR body** — project template:
   - **Summary** (required): 1–2 sentences, lead with problem/goal, say WHY.
   - **Important Changes** (optional): bullet list of architectural changes. Remove if not needed.
   - **Validation** (required): list commands/checks run.
   - **Diagram** (optional): Mermaid diagram for non-obvious flows. Remove if not needed.
   - **Possible Improvements** (optional): risks. Remove if not needed.
   - **Checklist**: include as-is, do not pre-fill.
   - **Related issues**: `Closes #N` if applicable, otherwise remove.
   - No tool attribution footers. No placeholder text.

   ```bash
   gh pr create --title "type: description" --body "$(cat <<'EOF'
   <filled PR template>
   EOF
   )"
   ```

6. **Run `/pr-fixup`** to wait for CI checks and CodeRabbit review, fix any failures or valid comments, and push.

7. **Return the PR URL** when done.
