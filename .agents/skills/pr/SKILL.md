---
name: pr
description: Create a PR from a committed and pushed branch after task-defined checks pass. Ready PRs continue to fixup in the primary conversation.
---

# PR

## Planner Entry

Create the PR directly in the primary conversation after commit, task-defined
checks, and push. Ready PR monitoring and remediation continue through
`/pr-fixup` in the same conversation.

> **Host detection:** This skill works on GitHub, GitLab, and Azure Repos. Detect the host before publication by inspecting `git remote get-url origin`:
> - URL contains `dev.azure.com`, `visualstudio.com`, or `ssh.dev.azure.com` → use the **Azure Repos flow** below.
> - URL contains `github.com` (or any host you have configured for GitHub) → use the **GitHub flow** below.
> - URL contains `gitlab` (e.g. `gitlab.com`, `gitlab.acme.corp`) → use the **GitLab flow** at the bottom of this file.
> - For self-managed hosts, the user's repository configuration determines the host.
>
> **GitHub tool selection:** The GitHub flow uses `gh` CLI by default. If `gh` is unavailable or fails, use any available GitHub tools in the environment (e.g. MCP GitHub tools).
> **GitLab tool selection:** The GitLab flow prefers `glab` CLI when available; otherwise it shells `curl` against the REST v4 API using `$GITLAB_TOKEN` (which the agent runtime injects from the user's secrets store).
> **Azure Repos tool selection:** The Azure flow prefers `az repos pr create` with the Azure DevOps extension. Auth can come from an existing `az login` session or `AZURE_DEVOPS_EXT_PAT`.

## Available skills

- **`/commit`** — Creates the artifact after task-defined checks pass.
- **`/pr-fixup`** — Wait for CI checks and CodeRabbit, Greptile, Claude, OpenCode, and cubic review feedback, fix any failures or valid comments, and push.

## Options

- `--draft` — create the PR as draft and skip the fixup step. Use when the work is not ready for review.
- Default (no flag) — create as ready-for-review and continue with `/pr-fixup` in the same conversation.

**Publishing-state precedence:** explicit user state (`--draft` or an explicit
ready-for-review request) wins. If the `github:yeet` plugin is explicitly
selected, follow its draft default only when the user did not request either
state; otherwise `/pr` defaults to ready-for-review. Always report the result.

## Steps

Track these steps with an internal todo/checklist and mark them complete as you go.
Do not create, update, or delete Kandev subtasks for this workflow unless the user
explicitly requests task tracking.

1. **Uncommitted changes:** If there are dirty or staged changes, stop: commit
   and the affected task checks are required first.

2. **Branch:** If on `main` or `master`, stop and ask the user for a feature
   branch. Otherwise use the current feature branch as-is.

3. **Remote state:** Confirm the branch has an upstream and the remote contains
   the local `HEAD`. If not, run `/push` before creating the PR.

   **CI artifact bootstrap:** If the diff introduces a CI-consumed registry tag
   or artifact that a workflow on this branch must publish first, follow that
   publisher's documented bootstrap path before rerunning consumer checks.
   Request explicit user approval before a `workflow_dispatch` or other action
   writes a shared registry, and verify the target tag/digest exists.

4. **Screenshots — capture and validate before publication.** For a UI-visible
   change, capture fresh screenshots for every affected viewport before creating
   the PR. When the changed surface is structurally absent on another viewport,
   record that rationale instead of capturing an unrelated screen.
   Use synthetic or redacted data, validate the assets, and compress PNGs using
   the recipe in step 7. If capture is impossible, report the concrete blocker
   and stop before PR publication. For non-UI changes, record that screenshots
   are not required and continue.

5. **Create the PR.** Before creating, check open PRs for the current branch and
   inspect task-linked PR metadata. Reuse an existing PR; create a duplicate only
   when the user explicitly requests separate PRs. Use `--draft` if requested,
   otherwise create as ready-for-review.

   **Architecture and scope gate:** Before running any PR creation command, verify
   the authenticated actor's repository permission. Do not treat a user statement
   as proof of maintainer status. On GitHub, query
   `gh api repos/{owner}/{repo}/collaborators/{login}/permission`; `push`,
   `maintain`, and `admin` permissions are write-authorized, so those actors may
   open large or architectural PRs directly. On other hosts, use the equivalent
   repository permission check. When the host confirms write access, the
   permission itself satisfies this gate and no linked issue is required solely
   for this purpose. If permission cannot be verified, or the actor has no write
   access, require a linked issue with maintainer discussion before opening a
   large or architectural PR. If that issue or discussion is missing, stop and
   report the blocker. Do not create an issue or open a PR solely to start the
   discussion. Prefer one logical change and the smallest practical diff; split
   unrelated cleanup, refactoring, and feature work into separate PRs.

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
   - If the diff touches user-visible UI, append the already captured assets in a `## Screenshots` section after the PR is created (step 7) — don't add a placeholder for it here.
   ```bash
   test -f .github/pull_request_template.md
   # Build /tmp/pr-body.md from the template, using comments as instructions
   # and removing them from the final file.
   gh pr create [--draft] --title "type: description" --body-file /tmp/pr-body.md
   ```

   Do not fall back to hand-composed `--body` prose. If creation fails, surface the exact stderr, fix the template/body-file problem, and retry with `--body-file`.

   If `gh pr create` fails after the branch is pushed with a credential-lease
   or repository-scope error, use the REST fallback with the same template body.
   Build the payload with `jq --rawfile` and submit it with
   `gh api --method POST repos/<owner>/<repo>/pulls --input <payload-file>`;
   preserve the validated title, head, base, and body, and never hand-escape
   Markdown or JSON.

6. **If ready (not draft):** For GitHub, do not begin `/pr-fixup` until any
required screenshot embedding in step 7 is complete.

7. **Screenshots — publish already captured assets.** If the diff touches user-visible UI (typically under `apps/web/`, excluding e2e-only or backend-only edits), publish the affected-viewport assets captured and validated in step 4 through the host-specific flow before treating the PR as complete — do not wait to be asked. Preserve any structural-absence rationale recorded in step 4.

   **Capture prerequisite:**
   - If `pnpm --dir apps exec playwright-cli list` has no local browser, use the
     managed `apps/web` E2E runner with a disposable capture spec instead of
     treating capture as blocked. Name mobile specs `mobile-*.spec.ts`, write
     assets to ignored `apps/web/.pr-assets`, inspect/compress them, then remove
     the temporary spec and confirm `git status` is clean.
   - After opening a popover or dialog, assert that the intended surface is
     visible and await finite active CSS animations (`element.getAnimations().finished`)
     before capture; do not publish a mid-transition asset.
   - For disposable capture specs, prefer the existing `prCapture` fixture from
     `apps/web/e2e/fixtures/test-base.ts`; it writes the expected filenames and
     manifest for PR assets.
   - `apps/web/e2e/scripts/run-e2e.sh` clears `.pr-assets` at the start of each
     managed-runner invocation. Capture desktop and mobile assets in one
     invocation when possible; if separate runs are necessary, preserve/merge
     the prior assets and revalidate the complete manifest afterward.
   - Reuse only fresh entries from `apps/web/.pr-assets/manifest.json`. After
     every capture, require a non-empty manifest with the intended fresh asset
     entries: `test -s apps/web/.pr-assets/manifest.json`. If it is absent or
     lacks the capture, do not treat the run as successful; rerun with `--host`
     and report the managed-runner gap.
   - Before inspecting, compressing, or publishing the assets, verify that every
     manifest entry maps to an existing file under `apps/web/.pr-assets`. If any
     entry is missing, treat the capture as incomplete and restore or recapture
     the asset; revalidate the complete mapping immediately before publication.
   - If required assets are missing, run the Playwright capture before
     publication; do not create the PR first.
   - For a mobile capture through `pnpm e2e:run`, select the runner project
     before the Playwright separator and use a `mobile-*.spec.ts` filename:
     `pnpm e2e:run --project mobile-chrome e2e/tests/<area>/mobile-<capture>.spec.ts`.
     `--project` after `--` is only a Playwright argument and leaves the
     runner on its default Chromium project.
   - In managed Docker E2E, write capture files directly to the mounted
     workspace path `/work/apps/web/.pr-assets/<name>.png`, not
     `testInfo.outputPath(...)`, which is container-local. After the run,
     confirm the host has the files before inspecting or compressing them.
   - Reject any asset that exposes secrets, authentication tokens, or personally
     identifiable information and stop for recapture.
   - Compress PNGs before embedding. Prefer a system `pngquant`; when it is
     unavailable, use the supported ephemeral fallback rather than skipping
     compression:
     ```bash
     if command -v pngquant >/dev/null 2>&1; then
       pngquant --quality 65-90 --ext .png --force apps/web/.pr-assets/*.png
     else
       (
         cd apps
         pnpm dlx pngquant-bin@9.0.0 --quality 65-90 --ext .png --force web/.pr-assets/*.png
       )
     fi
     ```

   **Embed (GitHub only — image binaries must never merge into `main`):** GitHub has no public API to upload images into a PR body (drag-and-drop is web-UI only), so publish the images on an orphan commit that can never be merged and reference them with SHA-pinned raw URLs:
   ```bash
   blob=$(git hash-object -w apps/web/.pr-assets/shot.png)
   printf '100644 blob %s\tshot.png\n' "$blob" > /tmp/tree
   # repeat the hash-object + printf lines, appending one line per file to /tmp/tree
   tree=$(git mktree < /tmp/tree)
   commit=$(git commit-tree "$tree" -m "media: screenshots for PR #<N>")
   git push origin "$commit:refs/heads/media/pr-<N>-screenshots"
   ```
   (A quoting glitch can make the first push report failure — retry with the literal commit SHA.) Reference each image in the PR body under a `## Screenshots` section using GitHub-Flavored Markdown image syntax and dash-named files (no spaces):
   `![Short descriptive caption](https://raw.githubusercontent.com/<owner>/<repo>/<media-commit-sha>/shot.png)`
   Bare image URLs render as clickable links instead of previews, so never add a screenshot URL without the surrounding `![alt text](...)` wrapper.

   Append the section to the PR body:
   ```bash
   gh pr edit <PR_NUMBER> --body-file <file>
   ```
   `gh pr edit` fails on this repo (GraphQL touches the deprecated Projects-classic API). Fall back to REST — build the payload with `jq --rawfile`, never by hand-escaping shell strings:
   ```bash
   set -euo pipefail
   PAYLOAD="/tmp/pr-body-<PR_NUMBER>-payload.json"
   jq -n --rawfile body "<body-file>" '{body: $body}' > "$PAYLOAD"
   jq empty "$PAYLOAD"
   gh api --method PATCH repos/:owner/:repo/pulls/<PR_NUMBER> --input "$PAYLOAD" --silent
   ```
   For a write-only PATCH, `--silent` prevents `gh` from decoding a response
   body that the caller does not consume. Redirecting stdout to `/dev/null`
   does not prevent a truncated or empty JSON response from making `gh` report
   an error after the mutation succeeds. Read the resource back with a
   separate GET and verify the intended change.
   **JSON payloads:** Use `jq` (or an equivalent JSON tool) to build and
   validate payloads without interpolating untrusted Markdown into shell
   syntax. When command output is redirected, piped, or consumed by `jq` or
   another parser, run the command through `rtk proxy` so stdout is
   byte-preserving; normal RTK filtering can truncate or annotate machine-readable
   output. Keep the REST fallback byte-preserving for command substitutions,
   `xargs`, and any other consumer that expects unmodified Git or JSON output.

   **Preserve the existing PR description:** The PR body is a shared, mutable
   document. Preview automation and other bots may add sections after the PR
   is created, so never PATCH a body reconstructed from the creation-time
   template or a stale `/tmp/pr-body.md`. Before every post-creation update:

   1. Fetch the current body from GitHub and keep that pristine live response
      as the merge base:
      `gh pr view <PR_NUMBER> --json body --jq .body > /tmp/pr-body-latest.md`.
   2. Change only the section owned by this operation. For screenshots, wrap
      the section in `<!-- kandev-screenshots-start -->` and
      `<!-- kandev-screenshots-end -->` markers (or replace the existing
      `## Screenshots` block if those markers are not present). Preserve every
      byte outside that range, including
      `<!-- kandev-preview-start --> ... <!-- kandev-preview-end -->`.
   3. Build a separate merged candidate, then re-fetch the live body immediately
      before PATCH and compare the two live snapshots (not the candidate with a
      snapshot). Use PR-scoped temporary filenames, fail immediately on a
      differing `cmp`, and discard any payload on failure; never reuse a prior
      `/tmp` payload. Extract JSON bodies byte-for-byte with `jq -j .body` before
      `cmp`; `jq -r .body` appends a newline and can report a false mismatch.
      Verify the payload contains this PR's current body and required sentinels
      before PATCH. If it changed, re-fetch and merge again; do not overwrite
      the newer body.
   4. After PATCH, read the body back and verify both the intended change and
      all previously present sentinel sections are still present.

   The REST PATCH endpoint replaces the complete body and does not provide a
   convenient description-level compare-and-swap, so this fetch/merge/check
   sequence is required even when the edit appears small.

   Before treating screenshot publication as complete, inspect the submitted body and verify every screenshot entry is a Markdown image embed (`![...](https://raw.githubusercontent.com/.../*.png)`) rather than a bare URL. Use `gh pr view <PR_NUMBER> --json body --jq .body` for this check.

   Never commit the screenshot binaries to the PR branch itself — only to the throwaway `media/pr-<N>-screenshots` ref (`git rm` them from the PR branch tip if they were committed there earlier; with squash-merge, deleting at tip is enough). The `docs/screenshots/` directory is for product/docs imagery that is meant to merge — don't confuse the two. The media branch must survive branch-cleanup sweeps; deleting it 404s the images in the PR body, so don't treat "unmerged branch" as automatically safe to delete.

8. **Report the PR URL** after all applicable steps are complete. For a ready
GitHub PR, continue with `/pr-fixup` in the same conversation when requested.

## Azure Repos flow

When `git remote get-url origin` points at Azure Repos, use the same preflight
(steps 1-4). For step 5, create an Azure Repos pull request instead of a GitHub
PR. Skip the GitHub fixup handoff and the GitHub-only embedding portion of step
7, but do not skip screenshot capture. For a UI-visible change, capture and
validate the required assets as described in step 4. Attach them to the Azure
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
(steps 1-4) and create a Merge Request for step 5. Skip the GitHub fixup
handoff and the GitHub-only orphan-ref embedding portion of step 7, but do not
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
