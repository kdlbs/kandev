---
name: pr-fixup
description: Wait for CI checks and automated reviews (CodeRabbit, Greptile, Claude, cubic) on a PR, fix failures and address comments, then push.
---

# PR Fixup

Wait for CI and code review to complete on a pull request, fix any failures or valid comments, then push.

> **GitHub tool selection:** This skill uses `gh` CLI commands by default. If `gh` is unavailable or fails, use any available GitHub tools in the environment (e.g. MCP GitHub tools) for PR checks, comments, replies, and reviews. Some operations (reactions, resolving threads, fetching CI logs) may not be available in all environments — skip gracefully.
> **Helper scripts location:** `scripts/pr-state`, `scripts/pr-resolve`, and `scripts/run-quiet` are at the worktree root (`<worktree>/scripts/...`), not under `.agents/skills/pr-fixup/scripts/`.

## Available skills and subagents

- **`pr-poller` subagent (Sonnet)** — Polls CI checks and the 4 review bots until terminal, returns a compact structured report. Replaces the old steps 1-3 and the post-push re-check (step 6).
- **`verify` subagent (Sonnet)** — Run the full verification pipeline (format, typecheck, test, lint) before pushing fixes.
- **`/e2e`** — Read for debugging guidance when E2E tests fail in CI. Covers test patterns, run commands, failure triage, and local reproduction.
- **`/commit`** — Use for staging and committing fixes with Conventional Commits format.

Prefer the `pr-poller` and `verify` helpers when the runtime supports delegated helpers. If runtime policy forbids delegated helpers/subagents unless explicitly requested by the user, treat helper delegation as unavailable and use the direct-command fallback. Do not substitute a generic/general subagent for polling; it returns too slowly for the 30s cadence and can look hung. If helper delegation is unavailable, follow the direct-command fallback sections below and keep the same output contract: compact CI state, compact review state, bounded polling, and full local verification before pushing.

## Context

- Current branch: !`git branch --show-current`
- Current PR: !`gh pr view --json number,url,title`

## Before anything else: create the pipeline

The first thing you do — before fetching PR state, before reading logs, before any fixes — is create a task list for the full pipeline. This is non-negotiable because it keeps you accountable to the process and lets the user see where you are.

Create these tasks immediately (use your task/todo tracking tool if available):

1. **Gather PR state** — Use `pr-poller` when available; otherwise gather compact CI + bot review state via `scripts/pr-state`
2. **Fix failing CI checks** — Read failing run logs (via `scripts/run-quiet gh-run -- gh run view ...`), fix issues, run E2E tests locally if needed
3. **Triage review comments** — Classify each comment as valid, already addressed, nitpick, or wrong
4. **Address each comment** — Fix or reply with reasoning, resolve threads
5. **Verify, commit, push** — Use `verify` when available; otherwise run the full verification commands directly; commit fixes; push
6. **Re-check** — Use `pr-poller` again when available; otherwise re-check directly. If new failures, loop back to task 2
7. **Summary** — Report what was done

Then start with task 1. Mark each task in_progress when you begin it and completed when you finish it.

---

## Steps

### 1. Gather PR state

Mark task 1 as in_progress.

If available, invoke the `pr-poller` subagent with the PR number (or let it resolve via `gh pr view` against the current branch). The subagent:
- Fetches the current CI/bot/comment state once
- Polls (30s cadence, **20 min cap**) until every CI check and every bot (CodeRabbit, Greptile, Claude, cubic) reaches a terminal state
- Counts unresolved review threads and bot issue comments
- Returns a structured report between `=== pr-poller report ===` and `=== end ===` markers

**Parse the report.** The fields you care about:

- `ci_failed` — list of `{name, run_id, conclusion, url}`. Empty list ⇒ CI is green.
- `ci_pending` — anything still running when the 20-min cap hit. Decide whether to re-invoke `pr-poller` after a short delay, or proceed with what you have and re-check at step 6.
- `bots.<name>` — `done` / `rate_limited` / `pending` / `timeout`. Anything in `done` or `rate_limited` has had its chance; treat the rest as missing data, not a blocker.
- `unresolved_review_threads` and `issue_comments_from_bots` — drive steps 3-4. If both are 0 and `ci_failed` is empty, skip to step 5 (still run verify + push if you have fixes from earlier).

**E2E CI outlasts the poller.** The pr-poller caps at ~20 minutes. The E2E matrix (10 shards × 2 projects) often runs longer. GitHub can expand E2E matrix jobs late; if pending checks briefly drop near zero and then jump when E2E shards appear, keep treating that as normal pending CI unless a shard reports failure. If the report shows `ci_pending` with only E2E/lint jobs and `ci_failed` is empty, re-invoke pr-poller once those jobs finish — do not spin a manual `gh pr checks` loop in the parent. If the cap hits with E2E still pending, report "CI in progress" to the user instead of blocking, and include the exact pending shard names from `ci_pending`.

**Do not fetch poll output yourself** — that is what burns context. The report is the only thing that enters your context.

#### Direct-command fallback

If delegated polling is unavailable, gather the same information directly without streaming long logs into context:

```bash
scripts/pr-state --summary <PR>
```

`scripts/pr-state` accepts flags before or after the PR (`scripts/pr-state --summary <PR>` and `scripts/pr-state <PR> --summary` both work). When parsing output with `jq`, prefer writing the JSON to a temp file first, then running `jq` against that file; this avoids `set -e` surprises and prevents stderr from corrupting a JSON pipe.

Prefer `scripts/pr-state --summary <PR>` for direct polling and `scripts/pr-resolve list <PR>` for review-thread state over raw `gh pr checks`. `gh pr checks` only reports CI/status checks; review bots can open unresolved threads after CI is green, and a checks-only poll will miss that blocker.

By default, `scripts/pr-state` returns comments, reviews, and review threads created after the latest PR head commit only. This is intentional for fixup passes: it keeps old bot summaries and already-addressed historical threads out of the working set. Use `scripts/pr-state --summary --all <PR>` only when you need to audit the full PR history.

The summary output contains:

- `failed_checks` — actionable non-green checks with `name`, `workflow`, `status`, `conclusion`, `run_id`, `job_id`, `details_url`, and `target_url`
- `pending_checks` — still-running or queued checks with `name`, `workflow`, `status`, `run_id`, `job_id`, `details_url`, and `target_url`
- `unresolved_review_thread_count` — total unresolved thread count on the PR, including older unresolved threads outside the current-head filter
- `filtered_review_thread_count` and `unresolved_threads` — compact current-head inline review state to triage in this fixup pass
- `errors` — data-gathering failures; treat affected fields as unknown instead of reconstructing them ad hoc

If `scripts/pr-state --summary <PR>` briefly returns `branch:"unknown"` or reports that `gh pr view` failed while `gh pr view <PR>` works directly, treat it as a transient state-resolution issue. Re-run the explicit PR-number command once and fall back to direct `gh pr view <PR>` / targeted `gh run view` checks for that pass instead of assuming the PR state is unavailable.

If `scripts/pr-state --summary <PR>` fails with `jq: Argument list too long`, do not debug the summary script during fixup. Split the fallback checks:

```bash
gh pr checks <PR>
scripts/pr-resolve list <PR>
```

`gh pr checks` gives CI status only; `scripts/pr-resolve list` is still required for unresolved inline review threads.

If `unresolved_review_thread_count` is nonzero but `unresolved_threads` is empty, run `scripts/pr-resolve list <PR>` before acting. `pr-state` can briefly report total unresolved count from historical state while current-head unresolved threads are empty; `pr-resolve list` is the authoritative actionable thread set for fixup work.

Poll at a 30s cadence with a **20 min cap**. Prefer one-shot `scripts/pr-state --summary <PR>` checks, or a bounded command that can finish naturally, over long inline shell loops. In this environment, a running non-TTY loop may not receive Ctrl-C through `write_stdin`, so avoid `while sleep ...; do ...; done` polling in the main session. Stop early if any required check fails. If the cap hits and only E2E shards are still pending with no failures or unresolved comments, report "CI in progress" instead of continuing to watch indefinitely, and include the exact pending shard names from `pending_checks`. Do not run `gh pr checks --watch` in the main session unless the runtime can keep the watcher isolated and automatically clean it up. If you do use `gh pr checks --watch`, keep watching until the command exits; GitHub can expand matrix jobs after an initial aggregate "Build" check passes, so the first green build/lint/test rows are not necessarily terminal.

If a user interrupts a long manual poll loop (`sleep`, `gh pr checks`, or `scripts/pr-state`), check for leftover polling processes before switching tasks and terminate only the processes you started.

Use raw mode only when debugging an odd GitHub state:

```bash
scripts/pr-state <PR>
```

Mark task 1 as completed.

### 2. Fix failing CI checks

Mark task 2 as in_progress.

**Sanity-check the poller's `ci_failed` before fixing anything.** Confirm each reported check `name` actually appears in `gh pr checks <PR>` output and its `run_id` resolves (`gh run view <run_id>` must not 404). If the report cites checks the repo doesn't have, discard it and re-gather state directly before touching code.

Prefer `scripts/pr-state <PR>` for this cross-check before falling back to raw `gh` output. It already gives you raw checks with normalized names plus extracted `run_id`s in one JSON snapshot.

For each entry in the report's `ci_failed:` list:

1. Use the `run_id` from the report (the poller already extracted these — don't re-run `gh pr checks`).
2. Fetch the failed logs via `scripts/run-quiet` — `gh run view --log-failed` dumps thousands of lines and will blow your context if it goes straight to stdout. The wrapper redirects to `/tmp/kandev-run.gh-run.<random>.log` and auto-greps for the relevant error lines:
   ```bash
   scripts/run-quiet gh-run -- gh run view <run-id> --log-failed
   ```
   If the printed summary is enough, stop. Only `Read` specific line ranges from the printed log path if you need surrounding context.
   If `gh run view --log-failed` returns only GitHub request metadata and no failure output, immediately fetch the failed job log directly and search the saved file for failing specs/errors:
   ```bash
   job_id="<failed job id from scripts/pr-state or gh run view --json jobs>"
   gh api repos/:owner/:repo/actions/jobs/"$job_id"/logs > /tmp/kandev-job.log
   rg -n "(Error:|FAIL|Failed|Timed out|Timeout|\\.spec\\.ts)" /tmp/kandev-job.log
   ```
   Then inspect targeted line ranges or downloaded artifacts rather than streaming the full log into context.
   If `gh run view --log-failed` says logs are unavailable because the workflow is still running, wait for the workflow/report job to finish before retrying.
3. Read the relevant source files at the failing lines (use `Read` with `offset`/`limit`, not `cat`)
4. Fix the issues (lint errors, test failures, type errors, etc.)

**If the failure looks unfamiliar or the cause isn't obvious from the log, check CI history on the branch before diving into the code:**

```bash
gh run list --branch <branch> --workflow "<workflow name>" --limit 10 --json conclusion,headSha,createdAt,databaseId
```

On long-lived PRs that get rebased/squashed, prior SHAs on the same branch often passed the same workflow. A `passing → failing` boundary tells you the regression is isolated to the most recent rework — diff against the last passing SHA (`git diff <last-passing-sha>..HEAD`) instead of against `main` to narrow the search dramatically.

**Recognize a cancelled concurrency-duplicate before reading any logs.** A required check with `conclusion=cancelled` — often annotated *"Canceling since a higher priority waiting request … exists"*, with 0s job durations and **unexpanded** `${{ matrix.* }}` job names (a "Merge reports" job may exit 1 for missing blobs) — is a concurrency-group artifact from a superseded run, **not a real failure**. GitHub re-triggers PR workflows when the base branch moves (e.g. a release lands on main), cancelling the in-flight run. Confirm the *non-cancelled* run for the **same head SHA** passed (`gh run list --workflow "<name>" --json headSha,conclusion,databaseId`), then trigger a single clean run (rebase onto main + force-push, or `gh run rerun <id>`) instead of debugging code.

**E2E test failures require special handling:**

If any failing check is an E2E test (Playwright):

1. Read the `/e2e` skill (`SKILL.md`) for debugging guidance, test patterns, and run commands
2. Follow the "Debugging failures" section — read error output, check failure screenshots in `e2e/test-results/`, classify the failure (test logic, frontend, backend)
3. For failed shards, identify the exact failing spec/test from logs before making changes.
4. Fix the root cause. **Never increase timeouts to fix flaky tests** — find the real issue
5. Confirm fixes pass locally before pushing. If CI logs name a specific failed test, run that exact spec/test first before any full shard run; shard-level runs can fail on unrelated existing flakes and obscure whether the reported PR failure reproduces. Build required artifacts first if global setup needs them, then run the targeted Playwright command. Wrap with `scripts/run-quiet`:
   ```bash
   scripts/run-quiet build -- make build-backend build-web
   scripts/run-quiet e2e -- bash -c 'cd apps && pnpm --filter @kandev/web e2e -- tests/path/to/failing.spec.ts'
   scripts/run-quiet e2e -- bash -c 'cd apps && pnpm --filter @kandev/web e2e -- tests/path/to/failing.spec.ts -g "exact failing test title"'
   ```
   Run the specific failing test file(s) or test title first, not the full suite or full shard. Only proceed to the verify/commit/push step after the reported failure passes locally.

If the failed E2E spec is unrelated to the PR diff, the exact failing test passes locally via `pnpm e2e:run`, and there are no unresolved review threads or other failed checks, rerun the failed GitHub job once:

```bash
gh run rerun <run-id> --failed
scripts/pr-state --summary <PR>
```

After rerunning, poll `scripts/pr-state --summary <PR>` until terminal using the bounded polling rules above.

If the failed specs are in unrelated existing areas and no changed code plausibly affects that surface, record the failure as unrelated in the PR fixup summary instead of changing unrelated tests.

**Don't dismiss a repeated failure as "flaky".** If the same shard or test fails 2+ poll iterations in a row, stop polling and do two cheap checks instead:

- **Compare per-shard runtime vs `main`.** `gh run list --branch main --workflow "<name>" --limit 5 --json databaseId` then `gh run view <id> --json jobs` and diff started/completed timestamps against the PR's run. A shard that takes e.g. 216s on main and 616s on the PR is real test failures + retries pushing past the job's `timeout-minutes` cap, not infrastructure variance. "Cancelled" at exactly the timeout boundary almost always means this.
- **Reproduce the failing spec locally** (step 4 above). CI logs hide in-DOM React render errors that show up immediately in the local `e2e/test-results/<test>/error-context.md`. A single local run (~5-10 min) routinely unlocks fixes that would otherwise burn 3+ CI cycles of speculative "rerun and hope".

Recommend a merge over green-pending-flake-rerun only after both checks pass.

Mark task 2 as completed.

### 3. Triage review comments

Mark task 3 as in_progress.

Use the report's `unresolved_review_threads` and `issue_comments_from_bots` counts to know whether there's anything to triage. If both are 0, mark this step completed and move on.

Otherwise, fetch the actual comment bodies on demand — one bot or one set at a time, not all at once:

```bash
# Inline review threads (humans, Greptile, Claude same-repo, cubic):
gh api repos/:owner/:repo/pulls/<number>/comments
# Issue comments (CodeRabbit walkthrough, Claude fork findings):
gh pr view <number> --json comments
```

When piping `gh api` or `gh api graphql` to `jq`, never use `2>&1`. `gh` writes diagnostics to stderr, and merging them corrupts the JSON stream (`jq: parse error: Invalid numeric literal`). Use `2>/dev/null` or `gh`'s built-in `--jq`.

Bot issue comments such as CodeRabbit walkthroughs, Claude summaries, Greptile summaries, and historical "actionable comments posted" notices are informational once `scripts/pr-resolve list <PR>` is empty and the latest bot check is passing. Do not reply to old top-level summaries unless they contain a current unresolved request.

**Verify before implementing.** Do not blindly accept review feedback — evaluate each comment technically:

For each comment:
1. Restate the requirement in your own words — if you can't, ask for clarification
2. Check against the codebase: is the suggestion correct for THIS code?
3. Check if it breaks existing functionality or conflicts with architectural decisions
4. YAGNI check: if the suggestion adds unused features ("implement properly"), grep for actual usage first

Then classify:
- **Valid and actionable** — real issue (bug, missing edge case, naming, architecture, code quality). Fix it.
- **Already addressed** — the code already handles what the comment suggests. Skip.
- **Nitpick or preference** — subjective style not covered by linters. Skip unless the reviewer insists.
- **Wrong or outdated** — misunderstands the code, refers to old state, or is technically incorrect. Push back with reasoning.

**Push back when:**
- The suggestion breaks existing functionality
- The reviewer lacks full context (explain what they're missing)
- It violates YAGNI (the feature is unused)
- It's technically incorrect for this stack
- It conflicts with architectural decisions

Mark task 3 as completed.

### 4. Address each comment

Mark task 4 as in_progress.

Every comment must get a response — either a fix or a reply explaining why it was skipped.

**Per-thread engagement is mandatory. Do not take shortcuts:**

- **Never post a single summary issue comment in place of individual thread replies.** A top-level summary comment leaves every inline thread unresolved and unanswered; reviewers have to hunt for your response across the diff. The only acceptable use of a summary comment is as an *addition* to per-thread replies, not a substitute.
- **Every unresolved review thread on the PR must receive a direct reply and be resolved**, even if that means 20+ thread interactions. Looping over threads programmatically is fine (and expected); batching into one summary is not.
- **Reply to the comment that started the thread**, not a random later one. Get the first-comment ID from the GraphQL `reviewThreads(first: 100) { nodes { comments(first: 1) { nodes { databaseId } } } }` query.
- **Do not mark task 4 completed until every previously-unresolved review thread is either resolved or has an explicit reason documented in a reply.** If you finish the pass and the `isResolved == false` set is still non-empty, you are not done.

**Important: issue comments vs review comments use different APIs:**
- **Review comments** (inline, from `gh api repos/:owner/:repo/pulls/<number>/comments`) — use `scripts/pr-resolve` (below).
- **Issue comments** (conversation timeline, from `gh pr view --json comments` — e.g., CodeRabbit walkthrough) — reply by posting a new comment via `gh pr comment <number> --body "..."`, react via `gh api repos/:owner/:repo/issues/comments/<comment_id>/reactions -f content="+1"`. There's no "resolve" concept for issue comments.

### Review comments: `scripts/pr-resolve`

Use the script for every review thread, not just batches — it collapses reply + resolve + +1 reaction into a single call so you don't re-derive the graphql mutation each session.

```bash
# Dump every unresolved thread, TAB-separated (tid, cid, author, path, body_first_120):
scripts/pr-resolve list <PR>

# Show full thread/comment details before deciding how to respond:
scripts/pr-resolve show <PR> <THREAD_ID>
scripts/pr-resolve show <PR> <COMMENT_ID>

# Reply syntax is: scripts/pr-resolve reply <PR> <comment_id> <thread_id> <body>
#
# Reply + resolve + +1 (same call whether you're agreeing or pushing back —
# the body text conveys which):
scripts/pr-resolve reply <PR> <COMMENT_ID> <THREAD_ID> "Fixed — monotonic counter via useRef. See commit abc1234."
scripts/pr-resolve reply <PR> <COMMENT_ID> <THREAD_ID> "Acknowledged; the strict source check was relaxed for E2E. Tracking as a follow-up."
```

`scripts/pr-resolve list` only prints previews. Before deciding whether a thread is valid, stale, duplicate, or already fixed, fetch the full comment/thread body:

```bash
scripts/pr-resolve show <PR> <THREAD_ID_OR_COMMENT_ID>

# Raw fallback for a single review comment:
gh api repos/:owner/:repo/pulls/comments/<comment_id> --jq '{body,path,line,commit_id,html_url}'

# Raw fallback for all PR review comments when the helper only supports list/reply:
gh api repos/:owner/:repo/pulls/<PR>/comments --paginate

# Use GraphQL reviewThreads when you need thread IDs, resolution state,
# or all comments in a review thread.
```

Bots may post duplicate or stale review threads against the previous head immediately after a fix push. Check `commit_id` on the full comment. If the current branch already contains the fix, reply and resolve with wording like "Fixed in <new commit>."

For valid comments: read the file, implement the fix, then call `pr-resolve reply` with a body that names the commit or the file:line of the fix.

For skipped comments (already addressed, nitpick, wrong, outdated): call `pr-resolve reply` with a body that explains why. Examples:
- "This is already handled by X on line Y."
- "This is a style preference not enforced by our linters — keeping as-is."
- "Refers to code that was changed in a later commit."

For dozens of threads grouped by topic, declare a bash associative array mapping thread IDs → category, then a `reply_for` case that returns the right body per category. Avoids retyping the same explanation across duplicate threads from multiple bots:

```bash
declare -A CAT=(
  [PRRT_xxx1]=fixed_counter
  [PRRT_xxx2]=fixed_counter
  [PRRT_xxx3]=skipped_source_guard
)
declare -A COMMENT_ID=(
  [PRRT_xxx1]=3253164429
  [PRRT_xxx2]=3253168996
  [PRRT_xxx3]=3253164669
)
reply_for() {
  case "$1" in
    fixed_counter) echo "Fixed — monotonic counter via useRef. See commit abc1234." ;;
    skipped_source_guard) echo "Acknowledged; the strict source check was relaxed for E2E. Tracking as a follow-up." ;;
  esac
}
for THREAD_ID in "${!CAT[@]}"; do
  scripts/pr-resolve reply <PR> "${COMMENT_ID[$THREAD_ID]}" "$THREAD_ID" "$(reply_for "${CAT[$THREAD_ID]}")"
done
```

### Verify resolution before moving on

Run `scripts/pr-resolve list <PR>` — output must be empty. Run it again after pushing fixes and resolving threads; automated reviewers may add duplicate or new threads on the latest pushed SHA.

Or confirm via GraphQL that unresolved thread count is 0:

```bash
gh api graphql -f query='query { repository(owner:"kdlbs", name:"kandev") { pullRequest(number:<PR>) { reviewThreads(first:100) { nodes { isResolved } } } } }' --jq '[.data.repository.pullRequest.reviewThreads.nodes[] | select(.isResolved == false)] | length'
```

Manual fallback when you reply outside `scripts/pr-resolve`:

```bash
set -euo pipefail

# Get the first comment's REST databaseId for one thread:
db_id="$(gh api graphql -f query='query { node(id:"PRRT_xxx") { ... on PullRequestReviewThread { comments(first:1){ nodes{ databaseId } } } } }' --jq '.data.node.comments.nodes[0].databaseId')"
if [[ -z "$db_id" || "$db_id" == "null" ]]; then
  echo "failed to fetch first comment databaseId for PRRT_xxx" >&2
  exit 1
fi

# Reply to THAT id (a guessed id 404s with "Parent comment not found"):
gh api --method POST repos/:owner/:repo/pulls/<PR>/comments/"$db_id"/replies --input reply.json >/dev/null

# Resolve:
resolved="$(gh api graphql -f query='mutation { resolveReviewThread(input:{threadId:"PRRT_xxx"}){ thread { isResolved } } }' --jq '.data.resolveReviewThread.thread.isResolved')"
if [[ "$resolved" != "true" ]]; then
  echo "failed to resolve PRRT_xxx" >&2
  exit 1
fi
```

Informational threads (acknowledged, no code change) still need `scripts/pr-resolve reply` + resolve — skipping them leaves the PR blocked.

Mark task 4 as completed.

### 5. Verify, commit, and push

Mark task 5 as in_progress.

1. Use the **`verify` sub-agent** when available to run the full verification pipeline (format, typecheck, test, lint). It will fix any issues it finds. Wait for it to complete.

   If delegated verification is unavailable, run the full verification pipeline directly:
   ```bash
   make fmt
   make typecheck
   make test
   make lint
   ```

   If formatting changes files, re-run the affected checks after reviewing the diff.

2. Stage and commit the fixes directly. Use a descriptive Conventional Commits message, e.g.:
   ```
   fix: address PR review feedback
   fix: resolve CI lint failures
   fix: address review feedback and fix CI failures
   ```

   For conflict-fix PRs, after merging or rebasing `origin/main`, check unresolved review threads again before pushing because conflict-resolution diffs can trigger new comments:
   ```bash
   scripts/pr-resolve list <PR>
   scripts/pr-resolve reply <PR> <COMMENT_ID> <THREAD_ID> "Fixed in the conflict resolution."
   ```

3. Push:
   ```bash
   git push
   ```

   If verification or conflict resolution rebased the branch onto `origin/main`, local commit SHAs changed and a plain push may fail non-fast-forward. In that case, push with:
   ```bash
   git push --force-with-lease
   ```

Mark task 5 as completed.

### 6. Re-check PR state

Mark task 6 as in_progress.

After the push, CI restarts and bots may re-review. Use `pr-poller` again when available — same helper, same contract, same 20-min cap. If delegated polling is unavailable, use the direct-command fallback from step 1. Parse the new report:

- Before waiting on CI, run `scripts/pr-resolve list <PR>` and address any newly opened review threads. Review bots may add new unresolved threads after earlier threads were resolved.
- The final re-check must include `scripts/pr-state --summary <PR>` (or a fresh pr-poller report), not only CI status. New review threads can arrive after a fresh push even when the previous unresolved count was zero.
- If `ci_failed:` is empty AND `unresolved_review_threads: 0` AND `issue_comments_from_bots: 0` (no new bot comments to address) → mark task 6 completed and proceed to summary.
- If a pushed fix restarts CI while previous checks were already in progress, continue using `scripts/pr-state --summary <PR>`. Treat a stable state of `failed_checks: []` plus `unresolved_review_thread_count: 0` as the actionable fixup completion point once all current review threads have been resolved. Current-head CI can legitimately reset to queued after a push, especially for long E2E, CodeQL, or preview jobs. If checks remain pending after several clean polls, report the exact pending checks instead of waiting indefinitely unless the user explicitly asks to wait for all CI.
- If new CI failures appeared from the latest commit → loop back to task 2 and reset task 2-5 to `in_progress` as needed.
- If new review comments appeared after the push → loop back to task 3.
- If the poller hit its cap (`recommendation:` mentions "timed out") → surface the remaining pending items to the user and stop.

Cap re-check loops at **3 iterations** to prevent runaway sessions. After 3, surface the remaining state to the user and stop.

Mark task 6 as completed.

### Multi-round bot reviews

**Expect new threads after every push.** CodeRabbit, Greptile, Claude, and cubic often re-review the latest commit and open fresh inline threads even when earlier ones were resolved. On cross-cutting changes (backend event payloads + frontend WS handlers + E2E), plan for 2–3 fixup rounds. After each push, always run `scripts/pr-state --summary <PR>` plus `scripts/pr-resolve list <PR>` before declaring done — do not rely on the prior round's zero count or CI status alone.

**Stop when green unless the next comment is clearly worth another cycle.** Tiny review-only commits restart the full CI and bot-review stack. Once the PR is green with no unresolved threads, avoid nonessential cleanup that would trigger another round unless the comment is blocking, clearly valid, or requested by the user.

### 7. Summary

Mark task 7 as in_progress.

Report what was done:
- CI checks: which failed and how they were fixed
- Comments addressed (with thumbs up)
- Comments skipped and why
- Link to the pushed commit
- Re-check iteration count

Mark task 7 as completed.
