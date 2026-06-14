---
name: pr
description: Commit, push, and create a PR. Default is ready-for-review with auto-fixup. Use --draft to skip review/fixup.
---

# PR

> **Host detection:** This skill works on GitHub, GitLab, and Azure Repos. Detect the host before step 4 by inspecting `git remote get-url origin`:
> - URL contains `dev.azure.com`, `visualstudio.com`, or `ssh.dev.azure.com` → use the **Azure Repos flow** below.
> - URL contains `github.com` (or any host you have configured for GitHub) → use the **GitHub flow** below.
> - URL contains `gitlab` (e.g. `gitlab.com`, `gitlab.acme.corp`) → use the **GitLab flow** at the bottom of this file.
> - For self-managed hosts, the user's repository configuration determines the host.
>
> **GitHub tool selection:** The GitHub flow uses `gh` CLI by default. If `gh` is unavailable or fails, use any available GitHub tools in the environment (e.g. MCP GitHub tools).
> **GitLab tool selection:** The GitLab flow prefers `glab` CLI when available; otherwise it shells `curl` against the REST v4 API using `$GITLAB_TOKEN` (which the agent runtime injects from the user's secrets store).
> **Azure Repos tool selection:** The Azure flow prefers `az repos pr create` with the Azure DevOps extension. Auth can come from an existing `az login` session or `AZURE_DEVOPS_EXT_PAT`.

## Available skills

- **`/commit`** — Stage and commit changes using Conventional Commits. Runs `/verify` internally.
- **`/pr-fixup`** — Wait for CI checks and CodeRabbit, Greptile, Claude, and cubic review feedback, fix any failures or valid comments, and push.

## Context

- Current git status: !`git status`
- Current branch: !`git branch --show-current`
- Commits on this branch vs main: !`git log --oneline main..HEAD`
- Recent commit messages for style reference: !`git log --oneline -5`

## Options

- `--draft` — create the PR as draft and skip the fixup step. Use when the work is not ready for review.
- Default (no flag) — create as ready-for-review and run `/pr-fixup` to wait for CI and CodeRabbit, Greptile, Claude, and cubic review feedback, then fix issues.

## Steps

**Create a task for each step below and mark them as completed as you go.**

1. **Uncommitted changes:** If there are dirty or staged changes, run `/commit` first (it runs `/verify` internally).

2. **Branch:** If on `main`, create a new branch from the commits (use a descriptive name like `feat/short-description` or `fix/short-description`) and switch to it. If already on a feature branch, use it as-is.

3. **Push** the branch to origin with `-u` to set upstream tracking.

4. **Create the PR.** Use `--draft` flag if the user requested draft mode, otherwise create as ready-for-review.

   **PR title** must follow Conventional Commits format (see `/commit` for full rules). CI validates via `pr-title.yml` — the PR title becomes the squash-merge commit used for release notes.

   **PR body** must be built from `.github/pull_request_template.md`; fail fast if it is missing. Read the whole template before writing the body. Treat HTML comments as authoring instructions for the agent, not as output:
   - Fill the template's required sections from the actual diff, commits, and verification performed.
   - Remove optional sections that add no value for this change.
   - Preserve static required sections such as checklists exactly as the template provides them; do not pre-fill unchecked boxes.
   - Include related issue closing text only when an actual issue number is known.
   - Remove all HTML comments/placeholders from the final body.
   - Do NOT add tool attribution footers.
   - Before creating the PR, self-check that the final body has no `<!--`, no empty required sections, and no placeholder text.

   ```bash
   test -f .github/pull_request_template.md
   # Build /tmp/pr-body.md from the template, using comments as instructions
   # and removing them from the final file.
   gh pr create [--draft] --title "type: description" --body-file /tmp/pr-body.md
   ```

   Do not fall back to hand-composed `--body` prose. If creation fails, surface the exact stderr, fix the template/body-file problem, and retry with `--body-file`.

5. **If ready (not draft):** Run `/pr-fixup` to wait for CI checks and CodeRabbit, Greptile, Claude, and cubic review feedback, fix any failures or valid comments, and push.

   CodeRabbit issue comments that only report rate limits or exhausted usage credits are informational. They should not block PR completion when other review threads are resolved and checks are otherwise passing or pending.

   After pushing review fixes, interpret `scripts/pr-state --summary <PR>` thread counts carefully. The command filters thread details by the latest head commit, so `filtered_review_thread_count` can include resolved historical threads from the current filtered view. Treat `unresolved_review_thread_count` as the blocker. For example, a re-check may show `unresolved_review_thread_count: 0` and `filtered_review_thread_count: 3`; do not turn the filtered historical count into new unresolved work.

   A ready PR may still end with "CI pending" after fixup when no checks have failed and no review threads remain unresolved, especially after a late fixup push restarts CodeQL, E2E, or preview jobs. When PR checks include long E2E shard queues, continue fixing failed checks and unresolved review threads, but it is acceptable to report the PR as ready locally once full local verification is green, CodeQL/security alerts are intentionally handled, required non-E2E checks are passing, and only queued/in-progress E2E shards remain with no failures. Do not wait indefinitely; include the exact pending checks from the final re-check in the response.

6. **PR screenshots:** After creating the PR, check if `apps/web/.pr-assets/manifest.json` exists. If it does:
   - Read the manifest to list available screenshots/GIFs
   - Run `npx tsx apps/web/e2e/scripts/upload-pr-assets.ts <PR_NUMBER>` to generate embed markdown
   - If `apps/web/.pr-assets/embed.md` exists and is non-empty, append its contents to the PR body using a body file and `gh pr edit <PR_NUMBER> --body-file <file>`
   - If `gh pr edit --body-file` fails after PR creation, especially with the GitHub Projects classic deprecation GraphQL error, fall back to REST. Build the payload with `jq --rawfile`, never by hand-escaping shell strings:
     ```bash
     jq -n --rawfile body "<body-file>" '{body: $body}' > /tmp/pr-body-payload.json
     gh api --method PATCH repos/:owner/:repo/pulls/<PR_NUMBER> --input /tmp/pr-body-payload.json
     ```
   - Tell the user to drag and drop the image files from `.pr-assets/` into the PR description on GitHub for the images to render

7. **Return the PR URL** when done.

## Azure Repos flow

When `git remote get-url origin` points at Azure Repos, the steps are the same up through **Push** (1–3). For step 4, create an Azure Repos pull request instead of a GitHub PR. **Skip steps 5 and 6** — `/pr-fixup` and the PR asset upload flow are GitHub-specific.

Prefer the Azure CLI when it is on `PATH`:

```bash
# If needed once per machine / shell:
# az extension add --name azure-devops
# export AZURE_DEVOPS_EXT_PAT=...   # optional when az login is not already configured

SOURCE_BRANCH="$(git branch --show-current)"
TARGET_BRANCH="${TARGET_BRANCH:-}"   # leave empty to let Azure use the repo default branch
DRAFT_FLAG=""
[ "${DRAFT:-false}" = "true" ] && DRAFT_FLAG="--draft"

az repos pr create \
  ${TARGET_BRANCH:+--target-branch "$TARGET_BRANCH"} \
  --source-branch "$SOURCE_BRANCH" \
  --title "type: description" \
  --description "$(cat <<'EOF'
<filled PR template>
EOF
)" \
  ${DRAFT_FLAG:+$DRAFT_FLAG}
```

Notes:
- Azure DevOps CLI auto-detects organization / project / repository from the current repo in most cases, so you usually do **not** need to pass `--organization`, `--project`, or `--repository` explicitly.
- If auto-detect fails (common with unusual remotes or older CLI setups), derive them from the remote and retry with explicit flags.
- Return the PR URL and stop.

## GitLab flow (Merge Requests)

When `git remote get-url origin` points at a GitLab host, the steps are the same up through **Push** (1–3). For step 4, create a Merge Request instead of a PR. **Skip steps 5 and 6** — `/pr-fixup` is wired to GitHub CI / CodeRabbit and `gh pr edit` only works against GitHub. The GitLab equivalent is to manage the MR directly via `glab` or the REST API (see "review comments" note at the bottom). After creating the MR, return the MR URL and stop.

**MR title** still follows Conventional Commits — the squash-merge commit message is built from it the same way.

**MR description** uses the same template as the PR body above (Summary, Validation, etc.).

Prefer the `glab` CLI when it is on the agent's `PATH`:

Don't hardcode `--target-branch`: many projects ship from `master`, `develop`, or a custom default. Omit the flag so `glab` resolves the project's default branch via the API, or pass an explicit value only if the user / spec already specified one.

```bash
glab mr create [--draft] \
  --title "type: description" \
  --description "$(cat <<'EOF'
<filled template>
EOF
)" \
  --remove-source-branch \
  --yes
```

If `glab` is unavailable but `$GITLAB_TOKEN` is set, fall back to the REST API. Derive the host from the git remote — `$CI_SERVER_URL` is only set inside GitLab runners and silently falling back to `gitlab.com` from a developer's machine would target the wrong instance. Construct the JSON body with `jq` so multi-line descriptions and embedded quotes can't break the payload.

```bash
REMOTE_URL="$(git remote get-url origin)"          # any of: git@host:path.git | ssh://git@host[:port]/path.git | https://host[:port]/path.git
# Classify by scheme so we can keep an https:// port (real API endpoint)
# while dropping any ssh:// port (irrelevant to the HTTPS API).
case "$REMOTE_URL" in
  ssh://*)        URL="${REMOTE_URL#ssh://}";   FORM=ssh ;;
  http://*|https://*) URL="${REMOTE_URL#*://}"; FORM=http ;;
  *)              URL="$REMOTE_URL";            FORM=scp ;;
esac
URL="${URL#*@}"                                    # strip optional user@
case "$FORM" in
  scp)
    # scp-style "git@host:path" — no port possible.
    HOST_ONLY="${URL%%:*}"
    HOST="https://${HOST_ONLY}"
    PROJECT_PATH="${URL#*:}"
    ;;
  ssh)
    # ssh:// — port (if any) is the SSH port, not the HTTPS API port.
    HOST_PORT="${URL%%/*}"
    HOST="https://${HOST_PORT%%:*}"
    PROJECT_PATH="${URL#*/}"
    ;;
  http)
    # https://host[:port]/path — preserve the port; it IS the API endpoint.
    HOST_PORT="${URL%%/*}"
    HOST="https://${HOST_PORT}"
    PROJECT_PATH="${URL#*/}"
    ;;
esac
PROJECT="${PROJECT_PATH%.git}"                     # team/repo
SOURCE_BRANCH="$(git branch --show-current)"
PROJECT_ENC="$(printf '%s' "$PROJECT" | jq -sRr @uri)"
# Default branch via the GitLab API itself, not glab (avoids version drift
# on glab's flag surface). Fall back to "main" only if the lookup fails.
TARGET_BRANCH="$(curl --fail -s -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  "$HOST/api/v4/projects/$PROJECT_ENC" | jq -r '.default_branch // "main"')"

PAYLOAD="$(jq -n \
  --arg source "$SOURCE_BRANCH" \
  --arg target "$TARGET_BRANCH" \
  --arg title "type: description" \
  --arg description "$(cat <<'EOF'
<filled template>
EOF
)" \
  '{source_branch: $source, target_branch: $target, title: $title, description: $description, remove_source_branch: true}')"

curl --fail -X POST \
  -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  -H "Content-Type: application/json" \
  --data "$PAYLOAD" \
  "$HOST/api/v4/projects/$PROJECT_ENC/merge_requests"
```

To address review comments on a GitLab MR, use the **discussions** API rather than individual review comments — discussions are GitLab's threading primitive. List with `GET /projects/:id/merge_requests/:iid/discussions`, reply with `POST /projects/:id/merge_requests/:iid/discussions/:discussion_id/notes`, and resolve a thread with `PUT /projects/:id/merge_requests/:iid/discussions/:discussion_id?resolved=true`. The `glab` equivalent for replies is `glab mr note create --reply <discussion_id>` — bare `glab mr note` opens a new thread instead of replying to an existing one.
