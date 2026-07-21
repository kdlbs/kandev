---
name: pr
description: Create a PR from an already verified, committed, and pushed branch. Ready PRs return to the planner for delegated fixup.
---

# PR

## Planner Entry

The user-started primary session delegates
verification, commit, push, and PR creation as separate bounded implementer
assignments, then coordinates `/pr-fixup` unless draft mode was requested. It
does not run repository-host commands directly.

An explicitly assigned PR worker creates the PR only after receiving the
verified, committed, and pushed branch state. It does not spawn other workers.

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

- **`/commit`** — Planner-side prerequisite for verified, committed changes.
- **`/pr-fixup`** — Wait for CI checks and CodeRabbit, Greptile, Claude, OpenCode, and cubic review feedback, fix any failures or valid comments, and push.

## Options

- `--draft` — create the PR as draft and skip the fixup step. Use when the work is not ready for review.
- Default (no flag) — create as ready-for-review and return its URL/state to the planner for `/pr-fixup` delegation.

## Steps

Track these steps with an internal todo/checklist and mark them complete as you go.
Do not create, update, or delete Kandev subtasks for this workflow unless the user
explicitly requests task tracking.

1. **Uncommitted changes:** If there are dirty or staged changes, stop and tell
   the planner that verification and commit assignments are required first.

2. **Branch:** If on `main` or `master`, stop and tell the planner that a
   bounded branch-preparation assignment is required. Otherwise use the current
   feature branch as-is.

3. **Remote state:** Confirm the branch has an upstream and the remote contains
   the local `HEAD`. If not, stop and tell the planner that the separate push
   assignment is incomplete. Do not push from the PR-creation assignment.

4. **Create the PR.** Use `--draft` flag if the user requested draft mode, otherwise create as ready-for-review.

   **PR title** must follow Conventional Commits format (see `/commit` for full rules). CI validates via `pr-title.yml` — the PR title becomes the squash-merge commit used for release notes.

   **PR body** must be built from `.github/pull_request_template.md`; fail fast if it is missing. Read the whole template before writing the body. Treat HTML comments as authoring instructions for the agent, not as output:
   - Fill the template's required sections from the actual diff, commits, and verification performed.
   - Remove optional sections that add no value for this change.
   - Preserve static required sections such as checklists exactly as the template provides them; do not pre-fill unchecked boxes.
   - For docs-only PRs, keep code-centric checklist items unchanged when they do not apply, and list the docs-safe validation commands actually run.
   - Include related issue closing text only when an actual issue number is known.
   - Remove all HTML comments/placeholders from the final body.
   - Do NOT add tool attribution footers.
   - Before creating the PR, self-check that the final body has no `<!--`, no empty required sections, and no placeholder text.
   - If the diff touches user-visible UI, a `## Screenshots` section gets appended after the PR is created (step 6) — don't add a placeholder for it here.
   ```bash
   test -f .github/pull_request_template.md
   # Build /tmp/pr-body.md from the template, using comments as instructions
   # and removing them from the final file.
   gh pr create [--draft] --title "type: description" --body-file /tmp/pr-body.md
   ```

   Do not fall back to hand-composed `--body` prose. If creation fails, surface the exact stderr, fix the template/body-file problem, and retry with `--body-file`.

5. **If ready (not draft):** For GitHub, do not return the PR to the planner for
   `pr-poller` or `/pr-fixup` until any required screenshot capture and
   embedding in step 6 are complete. Do not poll or remediate from this
   PR-creation assignment.

6. **Screenshots — required for any UI-visible change.** If the diff touches user-visible UI (typically under `apps/web/`, excluding e2e-only or backend-only edits), you must capture screenshots that show the change actually working and publish them through the host-specific flow before treating the PR as complete — do not wait to be asked. Capture both desktop and mobile viewports whenever the change is responsive.

   **Capture:**
   - The planner assigns screenshot capture to a QA or implementer worker before
     PR publication. The PR worker reuses fresh entries from
     `apps/web/.pr-assets/manifest.json`.
   - If fresh required assets are missing, stop and report the missing capture
     packet; do not run Playwright from this PR worker.
   - Screenshots must use synthetic or redacted data. Reject any asset that
     exposes secrets, authentication tokens, or personally identifiable
     information, and stop with a recapture request.
   - Compress PNGs before embedding: `pngquant --quality 65-90 --ext .png --force apps/web/.pr-assets/*.png`.

   **Embed (GitHub only — image binaries must never merge into `main`):** GitHub has no public API to upload images into a PR body (drag-and-drop is web-UI only), so publish the images on an orphan commit that can never be merged and reference them with SHA-pinned raw URLs:
   ```bash
   blob=$(git hash-object -w apps/web/.pr-assets/shot.png)
   printf '100644 blob %s\tshot.png\n' "$blob" > /tmp/tree
   # repeat the hash-object + printf lines, appending one line per file to /tmp/tree
   tree=$(git mktree < /tmp/tree)
   commit=$(git commit-tree "$tree" -m "media: screenshots for PR #<N>")
   git push origin "$commit:refs/heads/media/pr-<N>-screenshots"
   ```
   (A quoting glitch can make the first push report failure — retry with the literal commit SHA.) Reference each image in the PR body under a `## Screenshots` section using dash-named files (no spaces):
   `https://raw.githubusercontent.com/<owner>/<repo>/<media-commit-sha>/shot.png`

   Append the section to the PR body:
   ```bash
   gh pr edit <PR_NUMBER> --body-file <file>
   ```
   `gh pr edit` fails on this repo (GraphQL touches the deprecated Projects-classic API). Fall back to REST — build the payload with `jq --rawfile`, never by hand-escaping shell strings:
   ```bash
   jq -n --rawfile body "<body-file>" '{body: $body}' > /tmp/pr-body-payload.json
   gh api --method PATCH repos/:owner/:repo/pulls/<PR_NUMBER> --input /tmp/pr-body-payload.json
   ```

   Never commit the screenshot binaries to the PR branch itself — only to the throwaway `media/pr-<N>-screenshots` ref (`git rm` them from the PR branch tip if they were committed there earlier; with squash-merge, deleting at tip is enough). The `docs/screenshots/` directory is for product/docs imagery that is meant to merge — don't confuse the two. The media branch must survive branch-cleanup sweeps; deleting it 404s the images in the PR body, so don't treat "unmerged branch" as automatically safe to delete.

   If the capture worker reported that screenshots are impossible in the
   environment, return that blocker to the planner. The planner owns user
   communication and decides whether publication may proceed without them.

7. **Return the PR URL** after all applicable steps are complete. For a ready
   GitHub PR, return its URL and number to the planner, which launches
   `pr-poller` and coordinates `/pr-fixup`.

## Azure Repos flow

When `git remote get-url origin` points at Azure Repos, use the same preflight
(steps 1-3). For step 4, create an Azure Repos pull request instead of a GitHub
PR. Skip the GitHub fixup handoff and the GitHub-only embedding portion of step
6, but do not skip screenshot capture. For a UI-visible change, capture and
validate the required assets as described in step 6. Attach them to the Azure
PR when supported; otherwise return the fresh asset paths to the planner as an
explicit attachment handoff. If capture is impossible or no viable attachment
handoff exists, return that blocker instead of treating the PR as complete.

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
- Complete the screenshot capture and attachment/handoff requirements above,
  then return the PR URL and stop.

## GitLab flow (Merge Requests)

When `git remote get-url origin` points at a GitLab host, use the same preflight
(steps 1-3) and create a Merge Request for step 4. Skip the GitHub fixup
handoff and the GitHub-only orphan-ref embedding portion of step 6, but do not
skip screenshot capture. For a UI-visible change, capture and validate the
required assets, including the synthetic/redaction gate, then attach them to
the MR when supported or return fresh asset paths as an explicit attachment
handoff. If capture is impossible or no viable attachment handoff exists,
return that blocker instead of treating the MR as complete.

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
