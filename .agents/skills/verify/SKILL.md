---
name: verify
description: Run format, typecheck, test, and lint across the monorepo. Use after implementing changes.
---

# Verify

Delegate to the **`verify` subagent** to run the full verification pipeline (rebase, format, typecheck, test, lint) and fix any issues it finds. The subagent runs on Sonnet, which is cheaper than the main session and well-suited to the mechanical run-parse-fix loop.

## What to do

Invoke the `verify` subagent in a single call. Wait for it to complete and surface the result.

- If verify passes cleanly: report success.
- If verify cannot fix all failures: surface the remaining errors to the user — do not proceed with downstream actions (commit, push, PR) that depended on a green verify.

Do NOT run the verification commands yourself in the main session — that defeats the cost saving. The subagent's prompt already contains the full procedure (see `.claude/agents/verify.md`).
