---
name: push-pr
description: Push the current branch and open a draft PR. Use when the user wants to open a pull request or push their work for review.
---

## Context

- Current git status: !`git status`
- Current branch: !`git branch --show-current`
- Commits on this branch vs main: !`git log --oneline main..HEAD`
- Recent commit messages for style reference: !`git log --oneline -5`

## Your task

Push the current branch and open a **draft** pull request.

### Rules

1. **Uncommitted changes:** If there are dirty or staged changes, run the `/commit` skill first to commit them before proceeding.

2. **Branch:** If on `main`, create a new branch from the commits (use a descriptive name like `feat/short-description` or `fix/short-description`) and switch to it before pushing. If already on a feature branch, use it as-is.

3. **Push:** Push the branch to origin with `-u` to set upstream tracking.

4. **PR title** must follow Conventional Commits format (CI validates this via `pr-title.yml`, and it becomes the squash-merge commit used for release notes):
   - Format: `type(scope): lowercase description` or `type: lowercase description`
   - Allowed types: `feat`, `fix`, `perf`, `refactor`, `docs`, `chore`, `ci`, `test`
   - Subject starts with a lowercase letter
   - Keep under 72 characters
   - Add `!` after type for breaking changes: `feat!: remove legacy API`
   - Examples: `feat(ui): add task filter dialog`, `fix: prevent duplicate session on reconnect`

5. **PR body** must follow the project's pull request template. Fill in each section using these rules:
   - **Summary** (required): 1–2 sentences of prose, no heading. Lead with the problem/goal, end with the outcome. Say WHY, not what.
   - **Important Changes** (optional): short bullet list of significant architectural changes. Remove section if not needed.
   - **Validation** (required): list commands or checks run (e.g. `go test ./...`, `make lint`).
   - **Diagram** (optional): Mermaid diagram only for non-obvious flows. Remove section if not needed.
   - **Possible Improvements** (optional): one line on risk and what could go wrong. Remove section if not needed.
   - **Checklist**: always include as-is, do not pre-fill.
   - **Related issues**: use `Closes #N` if applicable, otherwise remove.
   - Do NOT add tool attribution footers.
   - Do NOT leave placeholder text or unfilled sections.

6. **Always create as draft:** Use `gh pr create --draft`.

7. **Execute in a single message.** Push and create the PR using parallel tool calls where possible. Do not read files or do anything else beyond what's listed here.

### Command

```
gh pr create --draft --title "type: description" --body "$(cat <<'EOF'
<filled PR template>
EOF
)"
```

8. **Return the PR URL** when done.
