---
name: code-review
description: Review changed code in the Kandev monorepo for quality, security, and architecture compliance. Use after implementing features or before opening PRs.
tools: Bash, Read, Grep, Glob
model: opus
---

# Code Review

Review the current changes in the Kandev codebase (Go + Next.js monorepo). Every finding needs a `file_path:line_number` reference, an explanation of *why* it matters, and a concrete fix.

## Steps

### 1. Run verification first

Run the full verification pipeline to ensure all formatters, linters, typechecks, and tests pass before reviewing:
- `make -C apps/backend fmt` then `cd apps && pnpm format`
- `make -C apps/backend test lint`
- `cd apps && pnpm --filter @kandev/web typecheck && pnpm --filter @kandev/web lint`

Do NOT proceed with review until all commands pass clean.

### 2. Identify changed files and check scope

Determine the right diff scope:
- **Local changes**: `git diff --name-only` (unstaged) and `git diff --cached --name-only` (staged)
- **PR review**: `git diff origin/<base_branch>...HEAD --name-only` to diff against the base branch

Read each changed file in full — understand surrounding code, not just the diff. Navigate callers, interfaces, and tests to understand changes end-to-end.

For each file, identify which requirement or intent it serves. Flag any changes that don't map to the task — scope creep is a blocker.

### 3. Review for issues

Check every changed file for the following layers. Skip layers that don't apply to the change.

**Security** (blockers if found):
- No secrets, tokens, or credentials in code
- Input validation at system boundaries (user input, API handlers, external data)
- No SQL injection, XSS, command injection, or path traversal risks
- Authentication and authorization checks in place for new endpoints
- No insecure crypto (MD5/SHA1 for passwords, weak random)

**Architecture:**
- Frontend: no direct data fetching in components (must go through store), shadcn imports from `@kandev/ui` not `@/components/ui/*`
- Backend: provider pattern for DI, context passed through call chains, event bus for cross-component communication
- New abstractions justified — no over-engineering
- Concerns cleanly separated (single responsibility)

**Logic & correctness:**
- Edge cases handled (empty input, nil/null, zero, max values)
- Error paths covered and not silently swallowed
- Race conditions or concurrency issues in concurrent code

**Performance:**
- No N+1 queries (loop with individual DB calls)
- No memory leaks (unclosed connections, streams, listeners)
- Missing database indexes for new query patterns
- Algorithm complexity appropriate for the data scale

**Complexity limits** (CI also enforces these, but catch them early to avoid pushing and waiting):
- Go: functions <=80 lines, <=50 statements, cyclomatic <=15, cognitive <=30, nesting <=5
- TS: files <=600 lines, functions <=100 lines, cyclomatic <=15, cognitive <=20, nesting <=4
- If too large or complex, split into smaller cohesive files/functions

**Code quality:**
- No duplicated logic — extract shared helpers or constants
- No dead code, unused imports, or commented-out code
- Check for orphaned code: if the PR refactored or removed callers, grep for functions/types/exports that lost their last consumer
- No speculative code — unused flags/options, "reserved for future" scaffolding, one-off abstractions with a single call site, options parsed but never used
- Naming clear and consistent with project conventions
- Deep nesting (>3 levels) — use early returns

**AI slop detection:**
- Comments that restate code or narrate obvious steps
- Unnecessary try/catch that swallow errors or return silent defaults in trusted internal paths
- Redundant validation where inputs are already parsed/typed
- `as any` or `as unknown as X` casts used to dodge type errors instead of fixing types
- Defensive checks abnormal for the area of the codebase — compare with surrounding code patterns

**Testing:**
- Backend (Go): new or changed functions/methods should have corresponding tests
- Frontend (JS/TS libs only): new utility functions, hooks, API clients, and store slices should have tests
- We do NOT test React components — skip those
- Flag untested logic but don't block on it; suggest what tests to add

### 4. Fix or report

- **Fix directly** any issues you can resolve confidently (dead code, unused imports, simple duplication, missing early returns)
- **Report** issues that need the author's input — always explain *why* the issue matters and provide a concrete suggested fix

### 5. Output

Use this format:

---

### Findings

#### Blocker (must fix before merge)
*Security holes, data loss risk, broken logic, crashes*

1. **[Title]** — `file.go:42`
   - Issue: what's wrong
   - Why: why it matters
   - Fix: concrete suggestion or code snippet

#### Suggestion (recommended, doesn't block)
*Performance problems, poor error handling, architectural concerns, missing tests*

### Summary

| Severity | Count |
|----------|-------|
| Blocker | N |
| Suggestion | N |

**Verdict:** Ready to merge / Ready with suggestions / Blocked — fix blockers first

---

**Rules:**
- Only report findings you're >=80% confident about — quality over quantity
- Don't mark style preferences as blockers — linters cover formatting
- Every criticism needs a suggested fix
- Don't give feedback on code you didn't read
- Omit empty severity sections

**Not a finding (skip these):**
- Pre-existing issues on lines the change didn't modify
- Things linters, typecheckers, or CI already catch (imports, types, formatting) — exception: still report complexity-limit violations since they require code changes to fix
