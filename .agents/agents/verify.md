---
name: verify
description: Run format, typecheck, test, and lint across the monorepo, then fix any issues found. Use before committing code.
tools: Bash, Read, Edit, Write, Grep, Glob
model: sonnet
permissionMode: acceptEdits
---

# Verify

Run the full verification pipeline for the monorepo, then fix any issues found.

## Steps

1. **Rebase on main** (ensures CI parity — CI merges main into the PR branch):
   ```bash
   git fetch origin main --quiet
   ```
   - If the current branch is `main`, skip this step.
   - If the branch is stacked on another feature branch (not main), skip this step.
     Detect: `git rev-parse --abbrev-ref --symbolic-full-name @{upstream} 2>/dev/null` — if the upstream is NOT `origin/main` (e.g., another feature branch), skip rebase.
   - Otherwise, rebase:
     ```bash
     git rebase origin/main
     ```
   - If the rebase has conflicts, resolve them:
     1. For each conflicted file, read the file and understand both sides of the conflict
     2. Resolve by keeping the correct combination of both changes (don't just pick one side)
     3. `git add <resolved-file>` then `git rebase --continue`
     4. Repeat for each conflicting commit until the rebase completes
     5. If a conflict is too ambiguous to resolve confidently, abort (`git rebase --abort`) and report the issue

2. **Format** (prevents formatter-induced lint failures):
   - `make -C apps/backend fmt`
   - `cd apps && pnpm format`

3. **Verify in parallel** where possible:
   - `make -C apps/backend test lint`
   - `cd apps && pnpm --filter @kandev/web typecheck && pnpm --filter @kandev/web lint`

4. **Fix issues** — do NOT just report them:
   - Read each failing file at the reported line number
   - Fix the root cause (don't suppress warnings or add ignores)
   - Common fixes:
     - **Type errors**: fix the type, add a missing import, or correct the function signature
     - **Lint — function too long**: extract a helper function or sub-component
     - **Lint — file too long**: split the file into smaller, cohesive files grouped by responsibility
     - **Lint — cyclomatic/cognitive complexity**: simplify conditionals, extract early returns, break into smaller functions
     - **Lint — unused imports**: remove them
     - **Lint — duplicate strings**: extract to a constant
     - **Test failures**: read the test, understand the assertion, fix the code (not the test) unless the test is outdated
   - After fixing, re-run only the failed command to confirm the fix

5. **Repeat** steps 3-4 until all commands pass. If a fix introduces new issues, address those too.

6. **Done** when all pass cleanly: rebase, fmt, typecheck, test, lint.
