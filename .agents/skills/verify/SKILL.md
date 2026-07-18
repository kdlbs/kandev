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

# Resolve the current PR base; stacked PRs may not target main.
PR_BASE="$(gh pr view --json baseRefName --jq .baseRefName 2>/dev/null || true)"
if [ -n "$PR_BASE" ]; then
  git fetch origin "$PR_BASE"
  git merge-base --is-ancestor "origin/$PR_BASE" HEAD || echo "branch is behind origin/$PR_BASE"
  git rebase "origin/$PR_BASE"
else
  echo "No PR base resolved; skipping rebase to avoid rewriting a stacked branch."
fi

# Keep verbose output out of the main agent context. The helper prints the log
# path and extracts targeted failure lines when a command fails.
scripts/run-quiet format -- make fmt
git status --short

# make typecheck uses the top-level Makefile path and can bypass package
# pretypecheck hooks, so generate web metadata before typecheck.
node apps/web/scripts/generate-release-notes.mjs
node apps/web/scripts/generate-changelog.mjs
scripts/run-quiet typecheck -- make typecheck
scripts/run-quiet test -- make test
scripts/run-quiet lint -- make lint
```

After quiet formatting, inspect the intended diff because formatter changes
still require review. When a quiet command fails, use its returned log path for
targeted inspection instead of rerunning the command with streamed output.

### Disk-constrained runners

If format, typecheck, tests, lint, or E2E reports `ENOSPC`, cache
initialization/lock errors, or an apparently unrelated secondary failure,
inspect free space on the temp and cache filesystems before changing code:

```bash
df -h /tmp /var/tmp "$PWD"
```

Choose one writable filesystem with space, create one agent-owned root there,
and use it consistently for the remaining verification commands. For example,
replace `/var/tmp` below if a different filesystem has the available space:

```bash
VERIFY_CACHE_ROOT="$(mktemp -d /var/tmp/kandev-verify.XXXXXXXX)"
mkdir -p "$VERIFY_CACHE_ROOT/tmp" "$VERIFY_CACHE_ROOT/go-cache" "$VERIFY_CACHE_ROOT/golangci-cache"
export TMPDIR="$VERIFY_CACHE_ROOT/tmp"
export GOCACHE="$VERIFY_CACHE_ROOT/go-cache"
export GOLANGCI_LINT_CACHE="$VERIFY_CACHE_ROOT/golangci-cache"
```

In a managed sandbox, request the normal filesystem escalation when the chosen
root is outside the writable roots; do not work around sandbox permissions.
`scripts/run-quiet` honors `KANDEV_RUN_QUIET_DIR`, then `TMPDIR`, so its logs
move to the same filesystem. Re-run the original failing command before
diagnosing source code and verify that the disk/cache errors are gone. After all
verification finishes, remove only `$VERIFY_CACHE_ROOT`; do not clear shared
caches or unrelated temp files.

### Restricted remote-environment failures

If Go tests fail from `httptest.NewServer` with an error such as
`listen tcp6 [::1]:0: socket: operation not permitted`, treat the first result
as a sandbox limitation. Rerun the exact command with the runtime's normal
network or loopback escalation. Diagnose test code only if the escalated rerun
still fails.

For desktop Rust changes, compare `rustc --version` with the `rust-version` in
`apps/desktop/src-tauri/Cargo.toml` before running the Rust suite. Activate an
installed matching rustup toolchain, extending `PATH` rather than replacing it
and losing Node/pnpm. If no matching toolchain is installed, report the exact
requirement or request installation instead of silently skipping Rust tests.

When a PR base was resolved, check whether `origin/$PR_BASE` is already an
ancestor of `HEAD` before rebasing.
If tracked files for the intended change are dirty, stash only those pathspecs
before `git rebase "origin/$PR_BASE"`, then pop the stash before running
`make fmt/typecheck/test/lint`. Do not use a broad `git stash` that could hide
unrelated user changes. If you miss this and `git rebase "origin/$PR_BASE"` fails
because of unstaged tracked changes, apply the same pathspec-only stash flow,
then rerun the rebase. Resolve conflicts before continuing verification.

If `make fmt` changes files, review the diff and continue with the remaining commands. If any command fails, fix the issue and re-run the failed command; for formatter-caused changes, re-run any affected checks before reporting success.

`make test` includes backend, web, CLI, and `test-scripts`; do not silently skip
`test-scripts` or its desktop smoke coverage while reporting full verification
as green. Claim full verification only after the complete format, typecheck,
test, and lint targets pass, plus the scoped Rust suite when Rust/Tauri code
changed.

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
