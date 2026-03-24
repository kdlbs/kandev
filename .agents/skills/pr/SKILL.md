---
name: pr
description: Commit, push, and create a PR. Default is ready-for-review with auto-fixup. Use --draft to skip review/fixup.
---

## Context

- Current git status: !`git status`
- Current branch: !`git branch --show-current`
- Commits on this branch vs main: !`git log --oneline main..HEAD`
- Recent commit messages for style reference: !`git log --oneline -5`

## Options

- `--draft` — create the PR as draft and skip the fixup step. Use when the work is not ready for review.
- Default (no flag) — create as ready-for-review and run `/pr-fixup` to wait for CI/CodeRabbit and fix issues.

## Your task

Commit, push, and create a pull request.

### Steps

**Create a todo/task for each step below and mark them as completed as you go.**

1. **Uncommitted changes:** If there are dirty or staged changes, run `/commit` first (it runs `/verify` internally).

2. **Branch:** If on `main`, create a new branch from the commits (use a descriptive name like `feat/short-description` or `fix/short-description`) and switch to it. If already on a feature branch, use it as-is.

3. **Push** the branch to origin with `-u` to set upstream tracking.

4. **Create the PR.** Use `--draft` flag if the user requested draft mode, otherwise create as ready-for-review.

   **PR title** must follow Conventional Commits format (CI validates this via `pr-title.yml`, and it becomes the squash-merge commit used for release notes):
   - Format: `type(scope): lowercase description` or `type: lowercase description`
   - Allowed types: `feat`, `fix`, `perf`, `refactor`, `docs`, `chore`, `ci`, `test`
   - Subject starts with a lowercase letter
   - Keep under 72 characters
   - Add `!` after type for breaking changes: `feat!: remove legacy API`
   - Examples: `feat(ui): add task filter dialog`, `fix: prevent duplicate session on reconnect`

   **PR body** must follow the project's pull request template:
   - **Summary** (required): 1–2 sentences of prose, no heading. Lead with the problem/goal, end with the outcome. Say WHY, not what.
   - **Important Changes** (optional): short bullet list of significant architectural changes. Remove section if not needed.
   - **Validation** (required): list commands or checks run (e.g. `go test ./...`, `make lint`).
   - **Diagram** (optional): Mermaid diagram only for non-obvious flows. Remove section if not needed.
   - **Possible Improvements** (optional): one line on risk and what could go wrong. Remove section if not needed.
   - **Checklist**: always include as-is, do not pre-fill.
   - **Related issues**: use `Closes #N` if applicable, otherwise remove.
   - Do NOT add tool attribution footers.
   - Do NOT leave placeholder text or unfilled sections.

   ```bash
   gh pr create [--draft] --title "type: description" --body "$(cat <<'EOF'
   <filled PR template>
   EOF
   )"
   ```

5. **If ready (not draft):** Run `/pr-fixup` to wait for CI checks and CodeRabbit review, fix any failures or valid comments, and push.

6. **PR screenshots:** After creating the PR, check if `apps/web/.pr-assets/manifest.json` exists. If it does:
   - Read the manifest to list available screenshots/GIFs
   - Run `npx tsx apps/web/e2e/scripts/upload-pr-assets.ts <PR_NUMBER>` to generate embed markdown
   - Read `apps/web/.pr-assets/embed.md` and append its contents to the PR body using `gh pr edit <PR_NUMBER> --body "..."`
   - Tell the user to drag and drop the image files from `.pr-assets/` into the PR description on GitHub for the images to render

7. **Return the PR URL** when done.
