---
name: pr-fixup
description: Wait for CI checks and automated reviews (CodeRabbit, Greptile, Claude, cubic) on a PR, fix failures and address comments, then push.
---

# PR Fixup

Wait for CI and code review to complete on a pull request, fix any failures or valid comments, then push.

> **GitHub tool selection:** This skill uses `gh` CLI commands by default. If `gh` is unavailable or fails, use any available GitHub tools in the environment (e.g. MCP GitHub tools) for PR checks, comments, replies, and reviews. Some operations (reactions, resolving threads, fetching CI logs) may not be available in all environments — skip gracefully.

## Available skills and subagents

- **`pr-poller` subagent (Sonnet)** — Polls CI checks and the 4 review bots until terminal, returns a compact structured report. Replaces the old steps 1-3 and the post-push re-check (step 6).
- **`verify` subagent (Sonnet)** — Run the full verification pipeline (format, typecheck, test, lint) before pushing fixes.
- **`/e2e`** — Read for debugging guidance when E2E tests fail in CI. Covers test patterns, run commands, failure triage, and local reproduction.
- **`/commit`** — Use for staging and committing fixes with Conventional Commits format.

## Context

- Current branch: !`git branch --show-current`
- Current PR: !`gh pr view --json number,url,title`

## Before anything else: create the pipeline

The first thing you do — before fetching PR state, before reading logs, before any fixes — is create a task list for the full pipeline. This is non-negotiable because it keeps you accountable to the process and lets the user see where you are.

Create these tasks immediately (use your task/todo tracking tool if available):

1. **Delegate to `pr-poller`** — Subagent gathers CI + bot review state, returns a compact report
2. **Fix failing CI checks** — Read failing run logs (via `scripts/run-quiet gh-run -- gh run view ...`), fix issues, run E2E tests locally if needed
3. **Triage review comments** — Classify each comment as valid, already addressed, nitpick, or wrong
4. **Address each comment** — Fix or reply with reasoning, resolve threads
5. **Verify, commit, push** — Delegate to `verify` subagent; commit fixes; push
6. **Re-check** — Delegate to `pr-poller` again. If new failures, loop back to task 2
7. **Summary** — Report what was done

Then start with task 1. Mark each task in_progress when you begin it and completed when you finish it.

---

## Steps

### 1. Delegate state-gathering to `pr-poller`

Mark task 1 as in_progress.

Invoke the `pr-poller` subagent with the PR number (or let it resolve via `gh pr view` against the current branch). The subagent:
- Fetches the current CI/bot/comment state once
- Polls (30s cadence, **20 min cap**) until every CI check and every bot (CodeRabbit, Greptile, Claude, cubic) reaches a terminal state
- Counts unresolved review threads and bot issue comments
- Returns a structured report between `=== pr-poller report ===` and `=== end ===` markers

**Parse the report.** The fields you care about:

- `ci_failed` — list of `{name, run_id, conclusion, url}`. Empty list ⇒ CI is green.
- `ci_pending` — anything still running when the 20-min cap hit. Decide whether to re-invoke `pr-poller` after a short delay, or proceed with what you have and re-check at step 6.
- `bots.<name>` — `done` / `rate_limited` / `pending` / `timeout`. Anything in `done` or `rate_limited` has had its chance; treat the rest as missing data, not a blocker.
- `unresolved_review_threads` and `issue_comments_from_bots` — drive steps 3-4. If both are 0 and `ci_failed` is empty, skip to step 5 (still run verify + push if you have fixes from earlier).

**Do not fetch poll output yourself** — that is what burns context. The report is the only thing that enters your context.

Mark task 1 as completed.

### 2. Fix failing CI checks

Mark task 2 as in_progress.

For each entry in the report's `ci_failed:` list:

1. Use the `run_id` from the report (the poller already extracted these — don't re-run `gh pr checks`).
2. Fetch the failed logs via `scripts/run-quiet` — `gh run view --log-failed` dumps thousands of lines and will blow your context if it goes straight to stdout. The wrapper redirects to `/tmp/kandev-run.gh-run.<random>.log` and auto-greps for the relevant error lines:
   ```bash
   scripts/run-quiet gh-run -- gh run view <run-id> --log-failed
   ```
   If the printed summary is enough, stop. Only `Read` specific line ranges from the printed log path if you need surrounding context.
3. Read the relevant source files at the failing lines (use `Read` with `offset`/`limit`, not `cat`)
4. Fix the issues (lint errors, test failures, type errors, etc.)

**If the failure looks unfamiliar or the cause isn't obvious from the log, check CI history on the branch before diving into the code:**

```bash
gh run list --branch <branch> --workflow "<workflow name>" --limit 10 --json conclusion,headSha,createdAt,databaseId
```

On long-lived PRs that get rebased/squashed, prior SHAs on the same branch often passed the same workflow. A `passing → failing` boundary tells you the regression is isolated to the most recent rework — diff against the last passing SHA (`git diff <last-passing-sha>..HEAD`) instead of against `main` to narrow the search dramatically.

**E2E test failures require special handling:**

If any failing check is an E2E test (Playwright):

1. Read the `/e2e` skill (`SKILL.md`) for debugging guidance, test patterns, and run commands
2. Follow the "Debugging failures" section — read error output, check failure screenshots in `e2e/test-results/`, classify the failure (test logic, frontend, backend)
3. Fix the root cause. **Never increase timeouts to fix flaky tests** — find the real issue
4. Confirm fixes pass locally before pushing. Wrap with `scripts/run-quiet`:
   ```bash
   scripts/run-quiet build -- make build-backend build-web
   scripts/run-quiet e2e -- bash -c 'cd apps && pnpm --filter @kandev/web e2e -- tests/path/to/failing.spec.ts'
   ```
   Run the specific failing test file(s), not the full suite. Only proceed to step 5 after the test passes locally.

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
- **Review comments** (inline, from `gh api repos/:owner/:repo/pulls/<number>/comments`) — reply via `/pulls/<number>/comments/<comment_id>/replies`, react via `/pulls/comments/<comment_id>/reactions`
- **Issue comments** (conversation timeline, from `gh pr view --json comments` — e.g., CodeRabbit walkthrough) — reply by posting a new comment via `gh pr comment <number> --body "..."`, react via `/issues/comments/<comment_id>/reactions`

**For valid comments:**
1. Read the file at the referenced line
2. Implement the fix
3. React with thumbs up:
   ```bash
   # For review comments:
   gh api repos/:owner/:repo/pulls/comments/<comment_id>/reactions -f content="+1"
   # For issue comments:
   gh api repos/:owner/:repo/issues/comments/<comment_id>/reactions -f content="+1"
   ```
4. Resolve the review thread (see below for thread ID retrieval)

**For skipped comments** (already addressed, nitpick, wrong, or outdated):
1. Reply to the comment explaining why it was skipped:
   ```bash
   # For review comments:
   gh api repos/:owner/:repo/pulls/<number>/comments/<comment_id>/replies -f body="<explanation>"
   # For issue comments:
   gh pr comment <number> --body "<explanation>"
   ```
   Examples:
   - "This is already handled by X on line Y."
   - "This is a style preference not enforced by our linters — keeping as-is."
   - "This refers to code that was changed in a later commit."
2. Resolve the review thread

**Resolving threads:** First fetch thread node IDs to map comment IDs to threads:
```bash
gh api graphql -f query='
query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      reviewThreads(first: 100) {
        nodes {
          id
          comments(first: 1) {
            nodes { databaseId }
          }
        }
      }
    }
  }
}' -f owner=":owner" -f repo=":repo" -F number=<number>
```
Then resolve using the thread `id`:
```bash
gh api graphql -f query='mutation { resolveReviewThread(input: {threadId: "<thread_node_id>"}) { thread { isResolved } } }'
```

Mark task 4 as completed.

### 5. Verify, commit, and push

Mark task 5 as in_progress.

1. Delegate to the **`verify` sub-agent** to run the full verification pipeline (format, typecheck, test, lint). It will fix any issues it finds. Wait for it to complete.

2. Stage and commit the fixes directly. Use a descriptive Conventional Commits message, e.g.:
   ```
   fix: address PR review feedback
   fix: resolve CI lint failures
   fix: address review feedback and fix CI failures
   ```

3. Push:
   ```bash
   git push
   ```

Mark task 5 as completed.

### 6. Re-check via `pr-poller`

Mark task 6 as in_progress.

After the push, CI restarts and bots may re-review. Delegate to `pr-poller` again — same subagent, same contract, same 20-min cap. Parse the new report:

- If `ci_failed:` is empty AND `unresolved_review_threads: 0` AND `issue_comments_from_bots: 0` (no new bot comments to address) → mark task 6 completed and proceed to summary.
- If new CI failures appeared from the latest commit → loop back to task 2 and reset task 2-5 to `in_progress` as needed.
- If new review comments appeared after the push → loop back to task 3.
- If the poller hit its cap (`recommendation:` mentions "timed out") → surface the remaining pending items to the user and stop.

Cap re-check loops at **3 iterations** to prevent runaway sessions. After 3, surface the remaining state to the user and stop.

Mark task 6 as completed.

### 7. Summary

Mark task 7 as in_progress.

Report what was done:
- CI checks: which failed and how they were fixed
- Comments addressed (with thumbs up)
- Comments skipped and why
- Link to the pushed commit
- Re-check iteration count

Mark task 7 as completed.
