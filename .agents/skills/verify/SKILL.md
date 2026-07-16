---
name: verify
description: Run format, typecheck, test, and lint across the monorepo. Use after implementing changes.
---

# Verify

Delegate to the **registered `verify` subagent** to run the full verification pipeline (rebase, format, typecheck, test, lint) and fix any issues it finds when the runtime supports delegated helpers. The subagent runs on Sonnet, which is cheaper than the main session and well-suited to the mechanical run-parse-fix loop. Do not substitute a generic agent: it may lack the required GitHub network access or shared-worktree write permissions.

If runtime policy forbids delegated helpers/subagents unless the user explicitly requested them, the named helper is not registered, or the helper fails to start, initialize, or access the required Git/network/worktree resources, treat delegation as unavailable and use the direct-command fallback below. Do not stop at a partial check just because delegation is unavailable.

## What to do

Invoke the `verify` subagent in a single call when available. Wait for it to complete and surface the result.

- If verify passes cleanly: report success.
- If verify cannot fix all failures: surface the remaining errors to the user — do not proceed with downstream actions (commit, push, PR) that depended on a green verify.

Do NOT run the verification commands yourself in the main session when the helper is available — that defeats the cost saving. The subagent's prompt already contains the full procedure (see `.claude/agents/verify.md`).

## Direct-command fallback

Use this only when the runtime does not permit delegated helpers/subagents. Run the full pipeline directly from the repository root and report the exact commands and results:

```bash
# Fresh worktrees share .git/ but not apps/node_modules.
if [ ! -d apps/node_modules ]; then
  (cd apps && pnpm install --frozen-lockfile)
fi

# If the branch is behind main, rebase first.
git fetch origin main
git merge-base --is-ancestor origin/main HEAD || echo "branch is behind origin/main"
git rebase origin/main

# make typecheck uses the top-level Makefile path and can bypass package
# pretypecheck hooks, so generate web metadata before typecheck.
make fmt
node apps/web/scripts/generate-release-notes.mjs
node apps/web/scripts/generate-changelog.mjs
make typecheck
make test
make lint
```

Before rebasing, check whether `origin/main` is already an ancestor of `HEAD`.
If tracked files for the intended change are dirty, stash only those pathspecs
before `git rebase origin/main`, then pop the stash before running
`make fmt/typecheck/test/lint`. Do not use a broad `git stash` that could hide
unrelated user changes. If you miss this and `git rebase origin/main` fails
because of unstaged tracked changes, apply the same pathspec-only stash flow,
then rerun the rebase. Resolve conflicts before continuing verification.

If `make fmt` changes files, review the diff and continue with the remaining commands. If any command fails, fix the issue and re-run the failed command; for formatter-caused changes, re-run any affected checks before reporting success.

If `make typecheck` still fails because `apps/web/generated/changelog.json` or
`apps/web/generated/release-notes.json` is missing, regenerate them and rerun
`make typecheck`:

```bash
(cd apps/web && node scripts/generate-release-notes.mjs)
(cd apps/web && node scripts/generate-changelog.mjs)
```

When verifying the web package directly, prefer:

```bash
(cd apps/web && pnpm run typecheck)
```

That package script runs `pretypecheck` and regenerates
`generated/changelog.json` / `generated/release-notes.json`. If troubleshooting
the web package directly, prefer the package-local script over workspace-filter
forms so TypeScript runs in the intended package context.

If the aggregate `make lint` wrapper stalls or does not provide useful progress, run the backend and frontend lint checks directly instead and record the substitution in your result:

```bash
make lint-backend
cd apps && pnpm --filter @kandev/web lint
```
