---
name: verify
description: Run Kandev format, typecheck, tests, and lint before commit, then fix failures and rerun focused failed commands until clean.
tools: Bash, Read, Edit, Write, Grep, Glob
model: sonnet
permissionMode: acceptEdits
---

# Verify

Run the full verification pipeline for the monorepo, then fix any issues found.

## Steps

1. **Prepare and rebase against the current PR base:**
   - Fresh worktrees share `.git/` but not dependencies. If `apps/node_modules` is missing, run `pnpm install --frozen-lockfile` from `apps/`.
   - Resolve the PR base from GitHub because stacked PRs may not target `main` and can be retargeted after a parent merges:
     ```bash
     PR_BASE="$(gh pr view --json baseRefName --jq .baseRefName 2>/dev/null || true)"
     if [[ -n "$PR_BASE" ]]; then git fetch origin "$PR_BASE" --quiet; fi
     ```
   - If the PR base is unavailable, skip rebasing and report it. Otherwise, if the current branch is not the base branch, rebase onto `origin/$PR_BASE`.
   - Resolve conflicts by preserving the intended behavior from both sides. Stage each resolved file and continue. If a conflict is ambiguous, abort and report it rather than guessing.

2. **Format and generate metadata:**
   ```bash
   scripts/run-quiet format -- make fmt
   git status --short
   node apps/web/scripts/generate-release-notes.mjs
   node apps/web/scripts/generate-changelog.mjs
   ```
   Review formatter changes before continuing.

3. **Run the complete pipeline** through `scripts/run-quiet`:
   ```bash
   scripts/run-quiet typecheck -- make typecheck
   scripts/run-quiet test -- make test
   scripts/run-quiet lint -- make lint
   ```
   - `make test` covers backend, web, CLI, `test-scripts`, and desktop smoke checks. Do not skip a failed subtarget while reporting full verification as green.
   - If a command fails, inspect only targeted ranges of the returned log path; never stream or `cat` the entire log.

4. **Fix issues** - do not just report them:
   - Read each failing file at the reported line number.
   - Fix the root cause; do not suppress warnings or add ignores.
   - For type, lint, or test failures, fix implementation behavior unless the test is demonstrably outdated.
   - For goleak failures after otherwise passing Go tests, loop the affected package with `go test -race -count=10 ./internal/<pkg>/...` and inspect cleanup ownership.
   - If Go tests fail because `httptest.NewServer` cannot bind loopback in a restricted sandbox, rerun the exact command with normal network/loopback escalation before diagnosing code.
   - For `ENOSPC`, read-only cache, or cache-initialization failures, create invocation-specific writable `TMPDIR`, `GOCACHE`, and `GOLANGCI_LINT_CACHE` directories and rerun the exact command. Clean only those directories.
   - For Rust/Tauri changes, compare `rustc --version` with `apps/desktop/src-tauri/Cargo.toml`'s `rust-version`. Activate a matching installed toolchain without replacing `PATH`; report or request installation if unavailable.
   - After fixing, rerun only the failed command through `scripts/run-quiet`.

5. **Repeat** steps 3-4 until all commands pass. If a fix introduces new issues, address those too.

6. **Done** only when rebase, format, metadata generation, typecheck, the complete test target, lint, and any scoped Rust tests all pass.
