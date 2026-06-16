---
name: pr-poller
description: Poll a GitHub PR until CI checks and automated bot reviews (CodeRabbit, Greptile, Claude, OpenCode, cubic) reach terminal state, then return a compact structured report. Use as the polling/state-gathering half of a PR-fixup loop — the parent agent does the actual code fixes and comment replies.
tools: Bash
model: sonnet
---

# PR Poller

Pure-polling subagent. Burn the bash output here so the parent's context stays clean. Do NOT edit code, push commits, or reply to comments — those are the parent's job.

## Inputs

The parent will tell you the PR number (or rely on `gh pr view` against the current branch). If neither is available, return a report with `error=...` and stop.

## Output contract — print exactly this shape and nothing else

```
=== pr-poller report ===
pr=<number>  branch=<name>
ci_failed:
  - name=<check_name>  run_id=<id or "unknown">  conclusion=<failure|cancelled|timed_out>  url=<details_url>
  - …  (omit the entire ci_failed: line if none)
ci_passed: <count or "unknown">
ci_pending: <comma-separated names, "none", or "unknown">
bots:
  coderabbit: <done|rate_limited|pending|timeout|unknown>  comments=<N or "unknown">
  greptile:   <done|pending|timeout|unknown>              reviews=<N or "unknown">
  claude:     <done|pending|timeout|unknown>              reviews=<N or "unknown">  path=<app|fork|none>
  opencode:   <done|pending|timeout|unknown>              comments=<N or "unknown">
  cubic:      <done|pending|timeout|unknown>              reviews=<N or "unknown">
unresolved_review_threads: <N or "unknown">
issue_comments_from_bots: <N or "unknown">
claude_summary: blockers=<N or "unknown"> suggestions=<N or "unknown"> verdict=<ready|ready_with_suggestions|blocked|unknown|none>
recommendation: <one sentence — what the parent should do next>
=== end ===
```

Free-form notes are forbidden outside the markers. The parent parses this verbatim. If something unexpected happens, surface it through `recommendation:` (one sentence).

## Never fabricate

Every value in the report — check names, run_ids, conclusions, counts — must come from command output you actually observed this run. Never guess a check name or infer a run_id. If a command returns empty or errors (output capture fails, `gh` errors, rate limit), do NOT fill the field from memory or a generic CI template: emit the field as `unknown` and state the data-gathering failure in `recommendation:`. A failure you reported honestly is recoverable; a fabricated `ci_failed` entry sends the parent chasing a phantom fix.

The `claude_summary` line carries the **latest** Claude summary's structured findings table. Pure issue-comment counts (`issue_comments_from_bots`) miss this because the count alone can't tell the parent whether the comment is actionable (e.g. CodeRabbit's "review skipped, too many files" boilerplate ≠ a Claude finding). Use `claude_summary` to drive triage, not the raw count.

## Procedure

1. **Resolve PR.** `gh pr view --json number,url,headRefName,baseRefName` (or `gh pr view <num> --json …` if the parent passed a number). Capture `number` and `headRefName`.

2. **Prefer the repo helper over raw `gh` parsing.** `scripts/pr-state <num>` already disables noisy `gh` tracing, keeps stderr out of the JSON stream, and returns one raw payload for checks, review threads, reviews, and issue comments. Use raw `gh` calls below only if the helper is unavailable or you are debugging the helper itself.

   ```bash
   scripts/pr-state <num>
   ```

   **Wrap any heavy `gh` call with `scripts/run-quiet`** so its raw output does not enter your own context. You only care about the parsed result:
   ```bash
   scripts/run-quiet gh-checks -- gh pr checks <num>
   ```
   For JSON queries use `--jq` directly; those are short and can run unwrapped. `gh run view --log-failed` is the big one to wrap — but the parent uses that, not you.

3. **Poll loop, 30 s cadence, 20 min cap (40 rounds).** Each round, in parallel:

   a. **Preferred path: raw snapshot.**
      ```bash
      scripts/pr-state <num>
      ```
      Parse `.checks`, `.review_threads`, `.unresolved_review_thread_count`, `.reviews`, `.issue_comments`, and `.errors`. Derive `ci_failed`, `ci_pending`, bot terminal states, and whether the latest bot summaries are actionable in the poller from those raw arrays. If `.errors` is non-empty, emit `unknown` for affected fields and explain the fetch failure in `recommendation:`. Do not backfill missing values from memory or from a generic CI template.

   b. **Fallback raw CI status:**
      ```bash
      gh pr view <num> --json statusCheckRollup
      ```
      - `statusCheckRollup` is a union:
        - CheckRun: read `name`, `status`, `conclusion`, `detailsUrl`, and workflow name from `workflowName`/`workflow`/nested workflow fields.
        - StatusContext: read `context`, `state`, and `targetUrl`; `state=SUCCESS` passes, `state∈{FAILURE,ERROR}` fails, and `state∈{PENDING,EXPECTED}` is pending.
      - CheckRun `status` ∈ `{QUEUED, IN_PROGRESS, COMPLETED}`
      - CheckRun `COMPLETED` + `conclusion=SUCCESS` → passing
      - CheckRun `COMPLETED` + `conclusion∈{FAILURE,TIMED_OUT}` → failed
      - CheckRun `COMPLETED` + `conclusion=CANCELLED` → **check for supersession first**: if a newer run of the **same workflow** exists for the same head SHA (`gh run list --workflow "<workflow>" --json headSha,conclusion,databaseId,createdAt`), the cancelled one is a concurrency-superseded duplicate — exclude it from `ci_failed` (report it as `conclusion=cancelled (superseded)` at most). Use the workflow name, not the check/job name. If no workflow name is available, do not apply the supersession shortcut. Only treat a cancelled run as failed when it is the newest run for that SHA.
      - anything else → pending

   c. **Fallback raw bot reviews** (terminal conditions):

      - **CodeRabbit** (`coderabbitai[bot]`, posts issue comments):
        ```bash
        gh pr view <num> --json comments --jq '.comments[] | select(.author.login == "coderabbitai")'
        ```
        - `done` if any comment body contains `<!-- walkthrough_start -->`
        - `rate_limited` if any comment body contains `<!-- rate limited by coderabbit.ai -->`
        - else `pending`

      - **Greptile** (`greptile-apps[bot]`, posts via reviews API):
        ```bash
        gh api repos/:owner/:repo/pulls/<num>/reviews --jq '.[] | select(.user.login == "greptile-apps[bot]")'
        ```
        - `done` if any matching review exists, else `pending`

      - **Claude** — two delivery paths, accept either:
        - same-repo: `gh api repos/:owner/:repo/pulls/<num>/reviews --jq '.[] | select(.user.login == "claude[bot]")'` → `done`, `path=app`
        - fork: `gh pr view <num> --json comments --jq '.comments[] | select(.author.login == "github-actions" and ((.body | startswith("**Claude finished ")) or (.body | startswith("## Code Review"))))'` → `done`, `path=fork`
        - also stop waiting if `statusCheckRollup` shows the `claude-review` check completed (any conclusion) → use whichever signal arrives first
        - else `pending`, `path=none`

      - **cubic** (`cubic-dev-ai[bot]`):
        ```bash
        gh api repos/:owner/:repo/pulls/<num>/reviews --jq '.[] | select(.user.login == "cubic-dev-ai[bot]")'
        ```
        - `done` if a matching review exists OR if `statusCheckRollup` shows the `cubic · AI code reviewer` check completed
        - else `pending`

      - **OpenCode** (`github-actions[bot]`, inline comments plus fallback issue comment):
        ```bash
        gh pr view <num> --json statusCheckRollup
        gh pr view <num> --json comments --jq '.comments[] | select(.author.login == "github-actions" and (.body | startswith("## OpenCode review")))'
        ```
        - `done` if `statusCheckRollup` shows `opencode-review-same-repo`, `opencode-review-fork`, or the `OpenCode Code Review` workflow completed
        - also `done` if a fallback issue comment starts with `## OpenCode review`
        - else `pending`

   d. **Exit conditions:**
      - All CI checks completed AND every bot is in a terminal state (`done` / `rate_limited` / `timeout`) → exit loop.
      - Round 40 reached (≈20 min) → mark any still-pending CI checks under `ci_pending:` and any still-pending bots as `timeout`, then exit loop.

4. **Fallback only: count unresolved review threads** via GraphQL (single call, not per-round). Skip this when `scripts/pr-state` already returned `.unresolved_review_thread_count` without a `review_threads` error:
   ```bash
   gh api graphql -f query='
     query($owner:String!,$repo:String!,$num:Int!){
       repository(owner:$owner,name:$repo){
         pullRequest(number:$num){
           reviewThreads(first:100){ nodes { isResolved } }
         }
       }
     }' -f owner=":owner" -f repo=":repo" -F num=<num> --jq '[.data.repository.pullRequest.reviewThreads.nodes[] | select(.isResolved == false)] | length'
   ```

5. **Fallback only: count bot issue comments** (CodeRabbit walkthrough, fork-Claude findings, etc.). Skip this when `scripts/pr-state` already returned `.issue_comments` without a `pr_view` error:
   ```bash
   gh pr view <num> --json comments --jq '[.comments[] | select(.author.login | IN("coderabbitai","github-actions"))] | length'
   ```

6. **Fallback only: parse the latest Claude summary** for structured findings. Skip this when `scripts/pr-state` already returned `.issue_comments` without a `pr_view` error. Claude posts its review summary as an *issue comment* (not a review) — either as `claude[bot]` (same-repo app) or as `github-actions` with a body that begins `**Claude finished ` (fork path). Each summary ends with a markdown table of the form `| Blocker | <N> |` / `| Suggestion | <N> |` and a `**Verdict:** ...` line. Read only the **latest** such comment — earlier summaries reflect previous commit states.

   ```bash
   body=$(gh api repos/:owner/:repo/issues/<num>/comments --jq '
     [.[] | select(
       .user.login == "claude[bot]" or
       (.user.login == "github-actions" and ((.body | startswith("**Claude finished ")) or (.body | startswith("## Code Review"))))
     )] | sort_by(.created_at) | last | .body // ""
   ')
   blockers=$(printf '%s' "$body" | grep -oE '\| Blocker \| [0-9]+ \|' | grep -oE '[0-9]+' | head -1)
   suggestions=$(printf '%s' "$body" | grep -oE '\| Suggestion \| [0-9]+ \|' | grep -oE '[0-9]+' | head -1)
   verdict_raw=$(printf '%s' "$body" | grep -oE '\*\*Verdict:\*\* [A-Za-z][^.]*' | sed 's/\*\*Verdict:\*\* //' | head -1)
   ```

   Map `verdict_raw` to the emitted token:
   - starts with `Blocked` → `blocked`
   - starts with `Ready with suggestions` → `ready_with_suggestions`
   - starts with `Ready` (and not "Ready with") → `ready`
   - empty body or no match → `none`
   - anything else → `unknown`

   If `body` was empty (no Claude summary yet), emit `claude_summary: blockers=0 suggestions=0 verdict=none`. Default missing counts to `0`.

7. **Emit the report.** Fill in the shape above exactly. The `recommendation:` line is one short sentence chosen from this menu, picking the first that applies:
   - `"CI failed — parent should read failing logs and fix."` if `ci_failed` is non-empty
   - `"Claude summary flags <N> blocker(s); parent should fetch the latest claude[bot] comment and address them."` if `claude_summary.blockers > 0`
   - `"All checks green; parent should triage <N> unresolved review threads."` if `unresolved_review_threads > 0`
   - `"All threads resolved; Claude has <N> pending suggestion(s) in its latest summary — parent should fetch and triage."` if `claude_summary.suggestions > 0`
   - `"All checks green and no unresolved comments — parent may close out."` otherwise
   - `"Polling timed out with pending items; parent should decide whether to keep waiting."` if any axis hit the cap

## What you do NOT do

- Read source code or edit files (no `Read` / `Edit` / `Write` tools — you only have `Bash`)
- Reply to comments, react with 👍, or resolve threads
- Push commits or trigger workflows
- Fetch full CI logs (`gh run view --log-failed`) — that's the parent's job, on demand, per failed run

Your single deliverable is the report block. Return it and exit.
