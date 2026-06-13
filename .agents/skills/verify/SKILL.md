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

```bash
# If the branch is behind main, rebase first:
git fetch origin main
git rebase origin/main
make fmt
make typecheck
make test
make lint
```

If `make fmt` changes files, review the diff and continue with the remaining commands. If any command fails, fix the issue and re-run the failed command; for formatter-caused changes, re-run any affected checks before reporting success.
