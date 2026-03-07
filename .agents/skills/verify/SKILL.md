---
name: verify
description: Run format, typecheck, test, and lint across the monorepo. Use after implementing changes.
---

# Verify

Run the full verification pipeline for the Kandev monorepo, then fix any issues found.

## Steps

1. **Format first** (prevents formatter-induced lint failures):
   - `make -C apps/backend fmt`
   - `cd apps && pnpm format`

2. **Verify in parallel** where possible:
   - `make -C apps/backend test lint`
   - `cd apps && pnpm --filter @kandev/web typecheck && pnpm --filter @kandev/web lint`

3. **Fix issues** — do NOT just report them:
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

4. **Repeat** steps 2-3 until all commands pass. If a fix introduces new issues, address those too.

5. **Done** when all four pass cleanly: fmt, typecheck, test, lint.
