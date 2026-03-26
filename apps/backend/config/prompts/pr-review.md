Please review the changes in this pull request.

STEP 1: Determine what to review
Review ONLY the changes in this pull request.
- Use: git diff origin/{{pr.base_branch}}...HEAD to see the PR diff (three-dot = only changes on the PR branch)
- Use: git log --oneline origin/{{pr.base_branch}}..HEAD to list the PR commits
- Do NOT review files outside this diff
- Do NOT diff against origin/main if the base branch is different

STEP 2: Review the changes, then output your findings in EXACTLY 4 sections: BUG, IMPROVEMENT, NITPICK, PERFORMANCE.

Rules:
- Each section is OPTIONAL — only include it if you have findings for that category
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

{{task_prompt}}