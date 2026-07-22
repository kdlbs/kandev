---
name: verify
description: Run Kandev format, typecheck, tests, and lint before commit, then return a compact pass/fail report without fixing source failures.
tools: Bash, Read, Grep, Glob
model: sonnet
effort: low
permissionMode: acceptEdits
---

# Verify

Run the full verification pipeline for the monorepo and report any failures.
Do not change production or test logic, resolve conflicts, rebase, or commit.

`permissionMode: acceptEdits` is intentional so the Bash-driven `make fmt`
step can retain formatter changes. It does not authorize source or test fixes.

## Steps

1. **Prepare and inspect the current PR base:**
   - Fresh worktrees share `.git/` but not dependencies. If `apps/node_modules` is missing, run `pnpm install --frozen-lockfile` from `apps/`.
   - Resolve the PR base from GitHub because stacked PRs may not target `main` and can be retargeted after a parent merges:
     ```bash
     PR_BASE="$(gh pr view --json baseRefName --jq .baseRefName 2>/dev/null || true)"
     if [[ -n "$PR_BASE" ]]; then git fetch origin "$PR_BASE" --quiet; fi
     ```
   - If the PR base is unavailable, report it. Otherwise report whether
     `origin/$PR_BASE` is an ancestor of `HEAD`; do not rebase or resolve
     conflicts in this role.

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

4. **Classify and report issues:**
   - Read each failing file at the reported line number when needed to make the
     failure report actionable.
   - Do not fix source/test failures or suppress checks. Return the command,
     relevant log path/lines, likely owned files, and a concise remediation
     recommendation for a new implementer assignment.
   - For goleak failures after otherwise passing Go tests, loop the affected package with `go test -race -count=10 ./internal/<pkg>/...` and inspect cleanup ownership.
   - If Go tests fail because `httptest.NewServer` cannot bind loopback in a restricted sandbox, rerun the exact command with normal network/loopback escalation before diagnosing code.
   - If required filesystem, network, or loopback escalation is unavailable,
     denied, cancelled, or interrupted, stop. Report verification as blocked,
     explain that mandatory verification is preventing commit and PR creation,
     and include a **Required user action** telling the user to enable the
     runtime's full access mode and retry verification. Do not offer to proceed
     unverified or imply that the agent or repository host cannot create PRs.
     Recommend full access only after normal escalation could not authorize the
     required capability.
   - For `ENOSPC`, read-only cache, or cache-initialization failures, preserve an existing absolute managed `GOCACHE` and existing lint cache. Move only invocation scratch (`TMPDIR` and quiet logs) to a writable filesystem. If a cache filesystem itself is unusable, relocate only that cache to a persistent agent-owned path outside every worktree; never use a repository-local fallback.
   - For Rust/Tauri changes, compare `rustc --version` with `apps/desktop/src-tauri/Cargo.toml`'s `rust-version`. Activate a matching installed toolchain without replacing `PATH`; report or request installation if unavailable.
   - For environment-only failures, rerun the exact command after correcting
     the invocation environment. Do not turn an environment retry into source
     remediation.

5. **Stop** after a reproducible source/test failure is captured. The planner
   assigns remediation and then launches a fresh verification run.

6. **Done** only when base ancestry has been reported and format, metadata
   generation, typecheck, the complete test target, lint, and any scoped Rust
   tests all pass.

Do not spawn subagents. Report pass/fail state, blockers, and any required user
action to the planner.
