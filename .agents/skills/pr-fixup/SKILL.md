---
name: pr-fixup
description: Wait for CI checks and automated reviews (CodeRabbit, Greptile) on a PR, fix failures and address comments, then push.
---

# PR Fixup

Wait for CI and code review to complete on a pull request, fix any failures or valid comments, then push.

## Context

- Current branch: !`git branch --show-current`
- Current PR: !`gh pr view --json number,url,title`

## Steps

**Create a todo/task for each step below and mark them as completed as you go.**

### 1. Gather PR state

Get the PR number from context or the user. Fetch the current state:

```bash
gh pr checks <number>
gh pr view <number> --json comments
gh api repos/:owner/:repo/pulls/<number>/comments
```

### 2. Wait for CI checks

If any checks are still running (`pending` / `queued` / `in_progress`), poll until they all resolve:

```bash
gh pr checks <number>
```

- Poll every **30 seconds**
- Cap at **10 minutes** (20 polls). If still running after 10 min, report which checks are stuck and proceed.
- Once done, note which checks passed and which failed.

### 3. Wait for automated reviews

Check if CodeRabbit and Greptile have posted or are generating reviews.

**Bot usernames:** `coderabbitai`, `greptile-apps[bot]`

**CodeRabbit — stop waiting if:**
- A comment contains `<!-- rate limited by coderabbit.ai -->` — rate-limited, won't review.
- A comment contains `<!-- walkthrough_start -->` — review complete.

**Greptile — stop waiting if:**
- A review from `greptile-apps[bot]` exists (posts via the GitHub review API, not issue comments).

**Keep polling if:**
- A bot hasn't commented yet AND `gh pr checks` shows its check is still `pending`.

Poll every **30 seconds**, cap at **10 minutes**. Fetch both issue comments and reviews each poll:
```bash
# CodeRabbit posts issue comments
gh pr view <number> --json comments --jq '.comments[] | select(.author.login == "coderabbitai") | {author: .author.login, body: .body}'
# Greptile posts reviews (with inline review comments)
gh api repos/:owner/:repo/pulls/<number>/reviews --jq '.[] | select(.user.login == "greptile-apps[bot]") | {user: .user.login, state: .state}'
```

### 4. Fix failing CI checks

If any CI checks failed:

1. Identify the failed runs from the `gh pr checks` output (the URL column contains the run URL)
2. Fetch the failed logs:
   ```bash
   gh run view <run-id> --log-failed
   ```
3. Read the relevant source files at the failing lines
4. Fix the issues (lint errors, test failures, type errors, etc.)

### 5. Triage review comments

Fetch all review comments — human reviewers, CodeRabbit, and Greptile:

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

### 6. Address each comment

Every comment must get a response — either a fix or a reply explaining why it was skipped.

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

### 7. Commit and push

Run `/commit` to stage and commit fixes (it runs `/verify` internally). Use a descriptive message, e.g.:
```
fix: address PR review feedback
fix: resolve CI lint failures
fix: address review feedback and fix CI failures
```

Then push:
```bash
git push
```

### 8. Summary

Report what was done:
- CI checks: which failed and how they were fixed
- Comments addressed (with thumbs up)
- Comments skipped and why
- Link to the pushed commit
