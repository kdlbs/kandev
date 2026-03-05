---
name: pr-comments
description: Address PR review comments — fix valid ones, react with thumbs up, resolve threads, and push.
---

# PR Comments

Read and address review comments on a pull request, then push fixes.

## Context

- Current branch: !`git branch --show-current`
- Current PR (if any): !`gh pr view --json number,url,title --jq '"\(.url) - \(.title)"' 2>/dev/null || echo "no PR found"`

## Steps

### 1. Fetch comments

Get the PR number from context or the user. Fetch all review comments:

```bash
gh pr view <number> --json reviews,comments
gh api repos/{owner}/{repo}/pulls/<number>/comments
```

Also check for top-level PR comments and review threads.

### 2. Triage each comment

For each comment, decide:
- **Valid and actionable** — the comment points to a real issue (bug, missing edge case, naming, architecture concern, code quality). Proceed to fix it.
- **Already addressed** — the code already handles what the comment suggests. Skip it.
- **Nitpick or preference** — subjective style preference not covered by linters. Skip unless the reviewer is insistent.
- **Wrong or outdated** — the comment misunderstands the code or refers to old state. Skip it.

### 3. Fix valid comments

For each valid comment:
1. Read the file at the referenced line
2. Implement the fix
3. React with a thumbs up on the comment:
   ```bash
   gh api repos/{owner}/{repo}/pulls/comments/<comment_id>/reactions -f content="+1"
   ```
4. Resolve the review thread:
   ```bash
   gh api graphql -f query='mutation { resolveReviewThread(input: {threadId: "<thread_node_id>"}) { thread { isResolved } } }'
   ```
   Get the `thread_node_id` from the review threads query (use `node_id` field from the thread, not the comment ID).

### 4. Verify

Run `/verify` to ensure formatters, linters, typechecks, and tests all pass. Do NOT push until verify passes clean.

### 5. Commit and push

Run `/commit` to stage and commit the fixes. Use a message like:
```
fix: address PR review feedback
```
or be more specific if fixes are scoped (e.g., `fix: add nil check per review feedback`).

Then push:
```bash
git push
```

### 6. Summary

Report what was done:
- Which comments were addressed (with thumbs up)
- Which comments were skipped and why
- Link to the pushed commit
