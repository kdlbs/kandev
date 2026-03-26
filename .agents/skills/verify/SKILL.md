---
name: verify
description: Run format, typecheck, test, and lint across the monorepo. Use after implementing changes.
---

# Verify

Run the full verification pipeline for the Kandev monorepo, then fix any issues found.

## Steps

1. **Rebase on main** (ensures CI parity — CI merges main into the PR branch):
   ```bash
   git fetch origin main --quiet
   ```
   - If the current branch is `main`, skip this step.
   - If the branch is stacked on another feature branch (not main), skip this step.
     Detect: `git log --oneline --first-parent origin/main..HEAD | tail -1` — if the first commit's parent is not on `origin/main`, it's stacked.
     Simpler heuristic: check `git merge-base --is-ancestor origin/main HEAD` — if false, the branch diverged from a non-main base; skip rebase.
   - Otherwise, rebase:
     ```bash
     git rebase origin/main
     ```
   - If the rebase has conflicts, resolve them:
     1. For each conflicted file, read the file and understand both sides of the conflict
     2. Resolve by keeping the correct combination of both changes (don't just pick one side)
     3. `git add <resolved-file>` then `git rebase --continue`
     4. Repeat for each conflicting commit until the rebase completes
     5. If a conflict is too ambiguous to resolve confidently, abort (`git rebase --abort`) and ask the user for guidance

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
     - **Lint — file too long**: split the file into smaller, cohesive files grouped by responsibility (e.g., separate types, helpers, constants, or sub-domains into their own files)
     - **Lint — cyclomatic/cognitive complexity**: simplify conditionals, extract early returns, break into smaller functions
     - **Lint — unused imports**: remove them
     - **Lint — duplicate strings**: extract to a constant
     - **Test failures**: read the test, understand the assertion, fix the code (not the test) unless the test is outdated
   - After fixing, re-run only the failed command to confirm the fix

5. **Repeat** steps 3-4 until all commands pass. If a fix introduces new issues, address those too.

6. **Done** when all pass cleanly: rebase, fmt, typecheck, test, lint.
