---
name: code-review
description: Review changed code in the Kandev monorepo for quality, security, and architecture compliance. Use after implementing features or before opening PRs.
---

# Code Review

Review the current changes in the Kandev codebase (Go + Next.js monorepo).

## Steps

### 1. Run verification first

Invoke the `/verify` skill to ensure all formatters, linters, typechecks, and tests pass. Do NOT proceed with review until verify passes clean.

### 2. Identify changed files

Run `git diff --name-only` (and `git diff --cached --name-only` for staged changes) to get the list of modified files. Read each changed file.

### 3. Review for issues

Check every changed file for the following. Report issues with `file_path:line_number` references.

**Complexity limits** (these are enforced by CI — catch them early):
- Go: functions ≤80 lines, ≤50 statements, cyclomatic ≤15, cognitive ≤30, nesting ≤5
- TS: files ≤600 lines, functions ≤100 lines, cyclomatic ≤15, cognitive ≤20, nesting ≤4
- If a file is too large or a function too complex, split it into smaller cohesive files/functions grouped by responsibility

**Duplicated code:**
- Look for copy-pasted logic across changed files and their surrounding code
- Extract shared helpers, constants, or utility functions when duplication is found

**Dead code:**
- Unused functions, variables, imports, types, or unreachable branches introduced by the changes
- Remove them — don't leave commented-out code

**Security:**
- No secrets, tokens, or credentials in code
- Proper input validation at system boundaries (user input, API handlers, external data)
- No SQL injection, XSS, command injection, or path traversal risks

**Architecture violations:**
- Frontend: no direct data fetching in components (must go through store), shadcn imports from `@kandev/ui` not `@/components/ui/*`
- Backend: provider pattern for DI, context passed through call chains, event bus for cross-component communication

**Missing unit tests:**
- Backend (Go): new or changed functions/methods should have corresponding tests
- Frontend (JS/TS libs only): new utility functions, hooks, API clients, and store slices should have tests
- We do NOT test React components — skip those
- Flag untested logic but don't block on it; suggest what tests to add

**Improvements:**
- Suggest simplifications, better naming, or more idiomatic patterns — but only if the improvement is clear and worth the churn
- Don't nitpick formatting or style that linters already cover

### 4. Fix or report

- **Fix directly** any issues you can resolve confidently (dead code, unused imports, simple duplication, missing early returns)
- **Report with explanation** any issues that need the author's input (architectural decisions, ambiguous test coverage, design trade-offs)

### 5. Summary

Provide a brief summary:
- Number of issues found and fixed
- Any remaining items that need author attention
- Overall assessment: ready to merge, needs minor fixes, or needs rework
