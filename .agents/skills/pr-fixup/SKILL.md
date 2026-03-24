---
name: pr-fixup
description: Wait for CI checks and CodeRabbit review on a PR, fix failures and address comments, then push.
---

# PR Fixup

Wait for CI and code review to complete on a pull request, fix any failures or valid comments, then push.

## Context

- Current branch: !`git branch --show-current`
- Current PR: !`gh pr view --json number,url,title`

## Steps

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

### 3. Wait for CodeRabbit review

Check if CodeRabbit has posted or is generating a review. Look at comments from `coderabbitai`:

**Stop waiting immediately if:**
- A comment contains `<!-- rate limited by coderabbit.ai -->` — CodeRabbit is rate-limited and won't review this PR.
- A comment contains `<!-- walkthrough_start -->` — the review is already complete.

**Keep polling if:**
- No `coderabbitai` comment exists AND `gh pr checks` shows a `CodeRabbit` check that is still `pending` — the review is being generated.

Poll every **30 seconds**, cap at **10 minutes**. Fetch comments each poll:
```bash
gh pr view <number> --json comments --jq '.comments[] | select(.author.login == "coderabbitai") | .body'
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

Fetch all review comments — both human reviewers and CodeRabbit:

```bash
gh pr view <number> --json reviews,comments
gh api repos/:owner/:repo/pulls/<number>/comments
```

For each comment, decide:
- **Valid and actionable** — real issue (bug, missing edge case, naming, architecture, code quality). Fix it.
- **Already addressed** — the code already handles what the comment suggests. Skip.
- **Nitpick or preference** — subjective style not covered by linters. Skip unless the reviewer insists.
- **Wrong or outdated** — misunderstands the code or refers to old state. Skip.

### 6. Fix valid comments

For each valid comment:
1. Read the file at the referenced line
2. Implement the fix
3. React with thumbs up:
   ```bash
   gh api repos/:owner/:repo/pulls/comments/<comment_id>/reactions -f content="+1"
   ```
4. Resolve the review thread. First fetch thread node IDs to map comment IDs to threads:
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
   Then resolve the thread using its `id`:
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
