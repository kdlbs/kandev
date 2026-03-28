Please review the changed files in the current git worktree.

STEP 1: Determine what to review
- First, check if there are any uncommitted changes (dirty working directory)
- If there are uncommitted/staged changes: review those files
- If the working directory is clean: review ONLY the commits from this branch
  - Use: git log --oneline $(git merge-base origin/main HEAD)..HEAD to list the branch commits
  - Use: git diff $(git merge-base origin/main HEAD) to see the cumulative changes
  - Do NOT diff directly against origin/main or master - that would include unrelated changes if the branch is outdated
- Read each changed file in full — understand surrounding code, not just the diff
- Navigate callers, interfaces, and tests to understand changes end-to-end
- Check git blame on modified sections to understand why code was written a certain way
- Only REPORT issues on code modified in this changeset, but USE the full codebase for context

If a code review skill is available (e.g. /code-review, /review), invoke it instead of using the fallback below.

STEP 2: Review the changes across these layers (skip layers that don't apply):

SECURITY (blockers if found):
- No secrets, tokens, or credentials in code
- Input validation at system boundaries (user input, API handlers, external data)
- No SQL injection, XSS, command injection, or path traversal
- Auth and authorization checks on new endpoints
- No insecure crypto (MD5/SHA1 for passwords, weak random)

LOGIC & CORRECTNESS:
- Edge cases handled (empty input, nil/null, zero, max values)
- Error paths covered and not silently swallowed
- Race conditions or concurrency issues (unprotected shared state, missing synchronization, goroutine leaks)
- Broken invariants — state that can become invalid

PERFORMANCE:
- No N+1 queries (loop with individual DB calls)
- No memory leaks (unclosed connections, streams, listeners)
- Algorithm complexity appropriate for data scale (O(n^2) where O(n) is possible)
- Unnecessary allocations in loops, regex compilation in hot paths, unbounded resource growth
- Prefer structured concurrency (errgroup, conc) over raw primitives

CODE QUALITY:
- No dead code, unused imports, or commented-out code
- Check for orphaned code: if the change refactored or removed callers, grep for functions/types/exports that lost their last consumer
- No speculative code (unused flags, one-off abstractions with single call site)
- No duplicated logic — extract shared helpers or constants
- Deep nesting (>3 levels) — use early returns

AI SLOP DETECTION:
- Comments that restate code or narrate obvious steps
- Unnecessary try/catch that swallow errors or return silent defaults
- as any / as unknown casts to dodge type errors instead of fixing types
- Redundant validation where inputs are already parsed/typed
- Defensive checks abnormal for the area — compare with surrounding code patterns

STEP 3: Output your findings.

Every finding needs: file:line, what's wrong, why it matters, and how to fix it.
Only report findings you're >=80% confident about.

Use these sections (omit empty ones):

## BLOCKER
Must fix before merge — security holes, data loss risk, broken logic, crashes.

- file:line - Description. Why it matters. How to fix.

## SUGGESTION
Recommended but doesn't block — performance, architecture, missing tests.

- file:line - Description. Why it matters. How to fix.

End with a verdict: Ready to merge / Ready with suggestions / Blocked — fix blockers first

NOT A FINDING (skip these):
- Issues on lines or files the change didn't modify — even if they are real bugs
- Pre-existing code patterns that this change didn't introduce
- Things linters, typecheckers, or CI catch (imports, types, formatting)
- Intentional functionality changes directly related to the task
- Issues explicitly suppressed in code (lint-ignore, nolint comments)
- Pedantic nitpicks a senior engineer wouldn't flag
- General "add more tests" without specifying what logic is untested
