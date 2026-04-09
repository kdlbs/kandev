---
name: pr-fixup
description: Wait for CI checks and automated reviews (CodeRabbit, Greptile, Claude) on a PR, fix failures and address comments, then push.
---

# PR Fixup

Wait for CI and code review to complete on a pull request, fix any failures or valid comments, then push.

> **GitHub tool selection:** This skill uses `gh` CLI commands by default. If `gh` is unavailable or fails, use any available GitHub tools in the environment (e.g. MCP GitHub tools) for PR checks, comments, replies, and reviews. Some operations (reactions, resolving threads, fetching CI logs) may not be available in all environments — skip gracefully.

## Available skills and subagents

- **`verify` subagent** — Run the full verification pipeline (format, typecheck, test, lint) before pushing fixes.
- **`/e2e`** — Read for debugging guidance when E2E tests fail in CI. Covers test patterns, run commands, failure triage, and local reproduction.
- **`/commit`** — Use for staging and committing fixes with Conventional Commits format.

## Context

- Current branch: !`git branch --show-current`
- Current PR: !`gh pr view --json number,url,title`

## Before anything else: create the pipeline

The first thing you do — before fetching PR state, before reading logs, before any fixes — is create a task list for the full pipeline. This is non-negotiable because it keeps you accountable to the process and lets the user see where you are.

Create these tasks immediately (use your task/todo tracking tool if available):

1. **Gather PR state** — Fetch checks, comments, and review status
2. **Wait for CI checks** — Poll until all checks resolve
3. **Wait for automated reviews** — Poll for CodeRabbit and Greptile
4. **Fix failing CI checks** — Read logs, fix issues, run E2E tests locally if needed
5. **Triage review comments** — Classify each comment as valid, already addressed, nitpick, or wrong
6. **Address each comment** — Fix or reply with reasoning
7. **Verify, commit, and push** — Run verification pipeline, commit fixes, push
8. **Summary** — Report what was done: CI fixes, comments addressed/skipped, pushed commit

Then start with task 1. Mark each task in_progress when you begin it and completed when you finish it.

---

## Steps

### 1. Gather PR state

Mark task 1 as in_progress.

Get the PR number from context or the user. Fetch the current state:

```bash
gh pr checks <number>
gh pr view <number> --json comments
gh api repos/:owner/:repo/pulls/<number>/comments
```

Mark task 1 as completed.

### 2. Wait for CI checks

Mark task 2 as in_progress.

If any checks are still running (`pending` / `queued` / `in_progress`), poll until they all resolve:

```bash
gh pr checks <number>
```

- Poll every **30 seconds**
- Cap at **10 minutes** (20 polls). If still running after 10 min, report which checks are stuck and proceed.
- Once done, note which checks passed and which failed.

Mark task 2 as completed.

### 3. Wait for automated reviews

Mark task 3 as in_progress.

Check if CodeRabbit, Greptile, and Claude have posted or are generating reviews.

**Bot usernames:** `coderabbitai`, `greptile-apps[bot]`, `claude[bot]`

**CodeRabbit — stop waiting if:**
- A comment contains `<!-- rate limited by coderabbit.ai -->` — rate-limited, won't review.
- A comment contains `<!-- walkthrough_start -->` — review complete.

**Greptile — stop waiting if:**
- A review from `greptile-apps[bot]` exists (posts via the GitHub review API, not issue comments).

**Claude — stop waiting if:**
- A review from `claude[bot]` exists (posts via the GitHub review API with inline review comments).
- The `claude-review` check in `gh pr checks` has completed (regardless of conclusion).

**Keep polling if:**
- A bot hasn't commented yet AND `gh pr checks` shows its check is still `pending`.

Poll every **30 seconds**, cap at **10 minutes**. Fetch both issue comments and reviews each poll:
```bash
# CodeRabbit posts issue comments
gh pr view <number> --json comments --jq '.comments[] | select(.author.login == "coderabbitai") | {author: .author.login, body: .body}'
# Greptile posts reviews (with inline review comments)
gh api repos/:owner/:repo/pulls/<number>/reviews --jq '.[] | select(.user.login == "greptile-apps[bot]") | {user: .user.login, state: .state}'
# Claude posts reviews (with inline review comments)
gh api repos/:owner/:repo/pulls/<number>/reviews --jq '.[] | select(.user.login == "claude[bot]") | {user: .user.login, state: .state}'
```

Mark task 3 as completed.

### 4. Fix failing CI checks

Mark task 4 as in_progress.

If any CI checks failed:

1. Identify the failed runs from the `gh pr checks` output (the URL column contains the run URL)
2. Fetch the failed logs:
   ```bash
   gh run view <run-id> --log-failed
   ```
3. Read the relevant source files at the failing lines
4. Fix the issues (lint errors, test failures, type errors, etc.)

**E2E test failures require special handling:**

If any failing check is an E2E test (Playwright):

1. Read the `/e2e` skill (`SKILL.md`) for debugging guidance, test patterns, and run commands
2. Follow the "Debugging failures" section — read error output, check failure screenshots in `e2e/test-results/`, classify the failure (test logic, frontend, backend)
3. Fix the root cause. **Never increase timeouts to fix flaky tests** — find the real issue
4. Confirm fixes pass locally before pushing:
   ```bash
   make build-backend build-web
   cd apps && pnpm --filter @kandev/web e2e -- tests/path/to/failing.spec.ts
   ```
   Run the specific failing test file(s), not the full suite. Only proceed to step 7 after the test passes locally.

Mark task 4 as completed.

### 5. Triage review comments

Mark task 5 as in_progress.

Fetch all review comments — human reviewers, CodeRabbit, Greptile, and Claude:

```bash
gh pr view <number> --json reviews,comments
gh api repos/:owner/:repo/pulls/<number>/comments
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

Mark task 5 as completed.

### 6. Address each comment

Mark task 6 as in_progress.

Every comment must get a response — either a fix or a reply explaining why it was skipped.

**Per-thread engagement is mandatory. Do not take shortcuts:**

- **Never post a single summary issue comment in place of individual thread replies.** A top-level summary comment leaves every inline thread unresolved and unanswered; reviewers have to hunt for your response across the diff. The only acceptable use of a summary comment is as an *addition* to per-thread replies, not a substitute.
- **Every unresolved review thread on the PR must receive a direct reply and be resolved**, even if that means 20+ thread interactions. Looping over threads programmatically is fine (and expected); batching into one summary is not.
- **Reply to the comment that started the thread**, not a random later one. Get the first-comment ID from the GraphQL `reviewThreads(first: 100) { nodes { comments(first: 1) { nodes { databaseId } } } }` query.
- **Do not mark task 6 completed until every previously-unresolved review thread is either resolved or has an explicit reason documented in a reply.** If you finish the pass and the `isResolved == false` set is still non-empty, you are not done.

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

Mark task 6 as completed.

### 7. Verify, commit, and push

Mark task 7 as in_progress.

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

Mark task 7 as completed.

### 8. Summary

Mark task 8 as in_progress.

Report what was done:
- CI checks: which failed and how they were fixed
- Comments addressed (with thumbs up)
- Comments skipped and why
- Link to the pushed commit

Mark task 8 as completed.
