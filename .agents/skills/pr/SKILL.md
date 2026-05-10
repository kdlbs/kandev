---
name: pr
description: Commit, push, and create a PR. Default is ready-for-review with auto-fixup. Use --draft to skip review/fixup.
---

# PR

> **Host detection:** This skill works on both GitHub and GitLab repositories. Detect the host before step 4 by inspecting `git remote get-url origin`:
> - URL contains `github.com` (or any host you have configured for GitHub) → use the **GitHub flow** below.
> - URL contains `gitlab` (e.g. `gitlab.com`, `gitlab.acme.corp`) → use the **GitLab flow** at the bottom of this file.
> - For self-managed hosts, the user's repository configuration determines the host.
>
> **GitHub tool selection:** The GitHub flow uses `gh` CLI by default. If `gh` is unavailable or fails, use any available GitHub tools in the environment (e.g. MCP GitHub tools).
> **GitLab tool selection:** The GitLab flow prefers `glab` CLI when available; otherwise it shells `curl` against the REST v4 API using `$GITLAB_TOKEN` (which the agent runtime injects from the user's secrets store).

## Available skills

- **`/commit`** — Stage and commit changes using Conventional Commits. Runs `/verify` internally.
- **`/pr-fixup`** — Wait for CI checks and CodeRabbit review, fix any failures or valid comments, and push.

## Context

- Current git status: !`git status`
- Current branch: !`git branch --show-current`
- Commits on this branch vs main: !`git log --oneline main..HEAD`
- Recent commit messages for style reference: !`git log --oneline -5`

## Options

- `--draft` — create the PR as draft and skip the fixup step. Use when the work is not ready for review.
- Default (no flag) — create as ready-for-review and run `/pr-fixup` to wait for CI/CodeRabbit and fix issues.

## Steps

**Create a task for each step below and mark them as completed as you go.**

1. **Uncommitted changes:** If there are dirty or staged changes, run `/commit` first (it runs `/verify` internally).

2. **Branch:** If on `main`, create a new branch from the commits (use a descriptive name like `feat/short-description` or `fix/short-description`) and switch to it. If already on a feature branch, use it as-is.

3. **Push** the branch to origin with `-u` to set upstream tracking.

4. **Create the PR.** Use `--draft` flag if the user requested draft mode, otherwise create as ready-for-review.

   **PR title** must follow Conventional Commits format (see `/commit` for full rules). CI validates via `pr-title.yml` — the PR title becomes the squash-merge commit used for release notes.

   **PR body** must follow the project's pull request template:
   - **Summary** (required): 1-2 sentences of prose, no heading. Lead with the problem/goal, end with the outcome. Say WHY, not what.
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

## GitLab flow (Merge Requests)

When `git remote get-url origin` points at a GitLab host, the steps are the same up through **Push** (1–3). For step 4, create a Merge Request instead of a PR:

**MR title** still follows Conventional Commits — the squash-merge commit message is built from it the same way.

**MR description** uses the same template as the PR body above (Summary, Validation, etc.).

Prefer the `glab` CLI when it is on the agent's `PATH`:

```bash
glab mr create [--draft] \
  --title "type: description" \
  --description "$(cat <<'EOF'
<filled template>
EOF
)" \
  --target-branch main \
  --remove-source-branch \
  --yes
```

If `glab` is unavailable but `$GITLAB_TOKEN` is set, fall back to the REST API. Use `$CI_SERVER_URL` if defined (set by GitLab runners), otherwise default to `https://gitlab.com`. The project path is `<namespace>/<repo>` — read it from the remote URL.

```bash
PROJECT="namespace/repo"   # adjust from `git remote get-url origin`
HOST="${CI_SERVER_URL:-https://gitlab.com}"
SOURCE_BRANCH="$(git branch --show-current)"
TARGET_BRANCH="main"
PROJECT_ENC="$(printf '%s' "$PROJECT" | jq -sRr @uri)"

curl --fail -X POST \
  -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  -H "Content-Type: application/json" \
  --data @- \
  "$HOST/api/v4/projects/$PROJECT_ENC/merge_requests" <<EOF
{
  "source_branch": "$SOURCE_BRANCH",
  "target_branch": "$TARGET_BRANCH",
  "title": "type: description",
  "description": "<filled template>",
  "remove_source_branch": true
}
EOF
```

To address review comments on a GitLab MR, use the **discussions** API rather than individual review comments — discussions are GitLab's threading primitive. List with `GET /projects/:id/merge_requests/:iid/discussions`, reply with `POST /projects/:id/merge_requests/:iid/discussions/:discussion_id/notes`, and resolve a thread with `PUT /projects/:id/merge_requests/:iid/discussions/:discussion_id?resolved=true`. The `glab` equivalent is `glab mr note` for replies.
