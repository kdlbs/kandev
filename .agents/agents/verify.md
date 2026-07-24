---
name: verify
description: Verify the committed Kandev artifact before push, reusing proven hook coverage and reporting a compact pass/fail result.
tools: Bash, Read, Grep, Glob
model: sonnet
effort: low
permissionMode: acceptEdits
---

# Verify

Run changed-scope verification by default and report mode, paths, commands, and
coverage limits. Read
`.agents/skills/verify/references/impact-matrix.md` and, when supplied,
`hook-evidence.md`; use full mode only for explicit broad/ambiguous triggers.
Scoped success is `changed-scope PASS`.
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

2. **Resolve verification scope and hook evidence:**
   - Prefer the supplied last verified SHA as scope base only when it is an
     ancestor of `HEAD`; otherwise use `origin/$PR_BASE`.
   - Validate any supplied `/commit` receipt against current `HEAD`, `HEAD^`,
     the scope base, hook results, bypass state, and a clean worktree exactly as
     required by
     `.agents/skills/verify/references/hook-evidence.md`. Never infer a receipt.
   - Collect the scope delta and any dirty paths:
   ```bash
   git diff --name-only "$SCOPE_BASE"...HEAD
   git diff --name-only
   git diff --cached --name-only
   git ls-files --others --exclude-standard
   ```
   Categorize the union with
   `.agents/skills/verify/references/impact-matrix.md`. Use
   `mode=changed` unless that matrix requires `mode=full`. If the base or diff
   cannot be resolved, fail closed to full mode.
   Build the omitted-check list before executing commands. Once a receipt is
   eligible, running a covered duplicate is a procedure failure; do not rerun
   it for reassurance.

3. **Run only the selected commands** through `scripts/run-quiet`.
   - In changed mode, run uncovered matrix commands for impacted categories.
     Report every omitted command and its exact covering hook.
     Generate web metadata only when web/shared TypeScript is impacted. Run
     only the applicable formatter and review any formatter changes.
   - In full mode, ignore hook omissions and run:
   ```bash
   scripts/run-quiet format -- make fmt
   git status --short
   node apps/web/scripts/generate-release-notes.mjs
   node apps/web/scripts/generate-changelog.mjs
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
     explain that mandatory verification is preventing push and PR delivery,
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
   handles a small remediation directly or delegates a larger one, then
   launches a fresh verification run.

6. **Done** only when PR/scope bases, ancestry, mode, receipt eligibility,
   omissions, changed paths/categories, exact commands, and coverage limits are
   reported and every selected check passes. If formatting changed the
   post-commit checkout, invalidate the pass until a new commit and verification
   run. Report `changed-scope PASS` or `full PASS` accurately.

Do not spawn subagents. Report pass/fail state, blockers, and any required user
action to the planner.
