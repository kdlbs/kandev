Please review the changed files in the current git worktree.

STEP 1: Determine what to review
- First, check if there are any uncommitted changes (dirty working directory)
- If there are uncommitted/staged changes: review those files
- If the working directory is clean: review ONLY the commits from this branch
  - Use: git log --oneline $(git merge-base origin/main HEAD)..HEAD to list the branch commits
  - Use: git diff $(git merge-base origin/main HEAD) to see the cumulative changes
  - Do NOT diff directly against origin/main or master — that would include unrelated changes if the branch is outdated

STEP 2: Review the changes, then output your findings in EXACTLY 4 sections: BUG, IMPROVEMENT, NITPICK, PERFORMANCE.

Rules:
- Each section is OPTIONAL - only include it if you have findings for that category
- If a section has no findings, omit it entirely
- Format each finding as: filename:line_number - Description
- Be specific and reference exact line numbers
- Keep descriptions concise but actionable
- Sort findings by severity within each section
- Focus on logic and design issues, NOT formatting or style that automated tools handle

Section definitions:

BUG: Critical issues that will cause runtime errors, crashes, incorrect behavior, data corruption, or logic errors
- Examples: null/nil dereference, race conditions, incorrect algorithms, type mismatches, resource leaks, deadlocks

IMPROVEMENT: Code quality, architecture, security, or maintainability concerns
- Examples: missing error handling, incorrect access modifiers (public/private/exported), SQL injection vulnerabilities, hardcoded credentials, tight coupling, missing validation, incorrect concurrency patterns

NITPICK: Significant readability or maintainability issues that impact code understanding
- Examples: misleading variable/function names, overly complex logic that should be refactored, missing critical comments for complex algorithms, inconsistent error handling patterns
- EXCLUDE: formatting, whitespace, import ordering, trivial naming preferences, style issues handled by linters/formatters

PERFORMANCE: Algorithmic or resource usage problems with measurable impact
- Examples: O(n²) where O(n) or O(1) is possible, unnecessary allocations in loops, missing indexes for database queries, blocking I/O in hot paths, regex compilation in loops, unbounded resource growth
- Concurrency-specific: unprotected shared state, missing synchronization, improper use of locks, goroutine leaks, missing context cancellation
- Prefer structured concurrency libraries (e.g., errgroup, conc) over raw primitives for better error handling and panic recovery

Example format:
## BUG
- src/handler.go:45 - Dereferencing pointer without nil check will panic when user is not found
- lib/parser.rs:123 - Loop condition uses <= instead of < causing out-of-bounds access

## IMPROVEMENT
- api/db.go:67 - Database query error ignored, will silently fail and return stale data
- services/auth.py:34 - Password comparison vulnerable to timing attacks, use constant-time comparison
- internal/user.go:15 - Type exported but only used internally, should be unexported

## NITPICK
- components/processor.ts:12 - Function name 'doStuff' doesn't describe what it actually does (transforms user input to API format)
- utils/cache.go:89 - Error wrapped multiple times making original cause hard to trace

## PERFORMANCE
- src/repository.go:156 - Linear search through slice on every request, use map for O(1) lookup
- handlers/api.py:45 - Compiling regex inside handler function, compile once at module level
- workers/processor.go:78 - Launching unbounded goroutines without limit, use worker pool or semaphore pattern
- db/queries.go:34 - N+1 query pattern, fetch all related records in single query with join

Now review the changes.