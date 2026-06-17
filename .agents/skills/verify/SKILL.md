---
name: verify
description: Run format, typecheck, test, and lint across the monorepo. Use after implementing changes.
---

# Verify

Delegate to the **`verify` subagent** to run the full verification pipeline (rebase, format, typecheck, test, lint) and fix any issues it finds when the runtime supports delegated helpers. The subagent runs on Sonnet, which is cheaper than the main session and well-suited to the mechanical run-parse-fix loop.

If runtime policy forbids delegated helpers/subagents unless the user explicitly requested them, treat delegation as unavailable and use the direct-command fallback below. Do not stop at a partial check just because delegation is unavailable.

## What to do

Invoke the `verify` subagent in a single call when available. Wait for it to complete and surface the result.

- If verify passes cleanly: report success.
- If verify cannot fix all failures: surface the remaining errors to the user — do not proceed with downstream actions (commit, push, PR) that depended on a green verify.

Do NOT run the verification commands yourself in the main session when the helper is available — that defeats the cost saving. The subagent's prompt already contains the full procedure (see `.claude/agents/verify.md`).

## Direct-command fallback

Use this only when the runtime does not permit delegated helpers/subagents. Run the full pipeline directly from the repository root and report the exact commands and results:

Fresh git worktrees share `.git/` but not `apps/node_modules/`. Before running
the pipeline, if `apps/node_modules` is missing, run:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

```bash
# If the branch is behind main, rebase first:
git fetch origin main
git rebase origin/main
make fmt
make typecheck
make test
make lint
```

Before rebasing, check whether `origin/main` is already an ancestor of `HEAD`:

```bash
git merge-base --is-ancestor origin/main HEAD
```

If the branch is behind and the worktree is dirty, stash the current patch,
rebase, then pop the stash before running `make fmt/typecheck/test/lint`.
Resolve conflicts before continuing verification.

If `make fmt` changes files, review the diff and continue with the remaining commands. If any command fails, fix the issue and re-run the failed command; for formatter-caused changes, re-run any affected checks before reporting success.

If `make typecheck` fails because `apps/web/generated/changelog.json` or
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
`generated/changelog.json` / `generated/release-notes.json`. Avoid relying on
`pnpm --filter @kandev/web typecheck` under RTK while troubleshooting; RTK can
mis-handle the filter and run TypeScript in the wrong context, producing
unrelated workspace-wide alias errors.

If the aggregate `make lint` wrapper stalls or does not provide useful progress, run the backend and frontend lint checks directly instead and record the substitution in your result:

```bash
make lint-backend
cd apps && pnpm --filter @kandev/web lint
```
