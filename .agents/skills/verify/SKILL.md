---
name: verify
description: Run format, typecheck, test, and lint across the monorepo. Use after implementing changes.
---

# Verify

Run the full verification pipeline for the monorepo, then fix any issues found.

## Steps

**Create a task for each step below and mark them as completed as you go.** This pipeline must run in order — formatting before linting prevents formatter-induced lint failures.

### 1. Rebase on main

Ensures CI parity — CI merges main into the PR branch.

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
  5. If a conflict is too ambiguous to resolve confidently, abort (`git rebase --abort`) and ask the user for guidance

### 2. Format

Run formatters first — they may change line lengths which affects linter results.

- `make -C apps/backend fmt`
- `cd apps && pnpm format`

### 3. Verify in parallel

Run these in parallel where possible:

- `make -C apps/backend test lint`
- `cd apps && pnpm --filter @kandev/web typecheck && pnpm --filter @kandev/web lint`

### 4. Fix issues

Do NOT just report issues — fix them:

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

### 5. Repeat

Repeat steps 3-4 until all commands pass. If a fix introduces new issues, address those too.

### 6. Done

All pass cleanly: rebase, fmt, typecheck, test, lint. Mark all tasks as completed.
