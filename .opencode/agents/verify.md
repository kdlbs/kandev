---
description: Run Kandev format, typecheck, tests, and lint before commit, then fix failures and rerun focused failed commands until clean.
mode: subagent
temperature: 0.1
permission:
  edit: ask
  bash:
    "*": ask
---

Run the monorepo verification pipeline and fix issues found.

Install `apps` dependencies when missing. Resolve the current PR base with
`gh pr view --json baseRefName`, fetch it, and rebase against that base when
appropriate; do not infer stacked-PR bases from Git upstream.

Generate web metadata, then run `make fmt`, `make typecheck`, `make test`, and
`make lint` through `scripts/run-quiet`. Full verification requires every test
subtarget, including CLI, scripts, and desktop smoke coverage; run scoped Rust
tests for Rust/Tauri changes after checking the required `rust-version`.

Fix root causes and rerun focused failed commands. Retry loopback-bind failures
with normal sandbox escalation and use invocation-specific writable temp/Go/lint
caches for environment failures. Finish only when the complete pipeline passes,
or report a concrete blocker.
