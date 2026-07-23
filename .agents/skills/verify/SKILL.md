---
name: verify
description: Verify the committed artifact before push, reusing proven pre-commit hook coverage and escalating to full checks when impact requires it.
---

# Verify

In the user-started primary session, delegate verification to the registered
Spark `verify` worker after commit and before push, with `mode=changed` by
default. Supply the `/commit` hook receipt and last successfully verified SHA
when available. Do not substitute
a generic agent: it may lack the required GitHub network access or
shared-worktree write permissions. If the worker cannot be launched or access
the required resources, stop and report that verification is blocked.

An explicitly assigned `verify` worker follows the worker procedure below,
reports verification failures without fixing source or test logic, and does not
spawn workers. The planner handles a small remediation directly or delegates a
larger one, then launches a fresh `verify` assignment.

## What to do

Invoke the `verify` worker in a single call. Wait for it to complete and surface the result.

- If verify passes cleanly: report success.
- If verify fails: create a bounded remediation assignment from its report and
  do not proceed with downstream actions that depend on green verification.
- If verify reports that required sandbox capabilities could not be authorized,
  stop before push or PR delivery and surface its required user action.
  On Codex, tell the user exactly: "Switch the mode selector to
  `Agent (full access)`, then retry verification." Explain that push and PR
  delivery are waiting on mandatory verification; do not imply that Codex or GitHub cannot
  create PRs, ask whether to proceed unverified, or launch downstream workers.

Do not run verification commands in the planner session. The worker's prompt
contains the full procedure in `.agents/agents/verify.md`.

## Worker Procedure

Resolve the PR base and verification scope base, then collect
`scope-base...HEAD`, staged, unstaged, and untracked paths. The supplied last
verified SHA may be the scope base only when it is an ancestor of `HEAD`;
otherwise use the PR base. Report PR base/head, scope base, paths/categories,
hook-receipt eligibility and omissions, exact commands, and coverage limits.
If base/diff is unavailable or impact is ambiguous, use `mode=full`; use full
mode for explicit requests, releases, shared build or toolchain changes, and
unusually broad work. PR CI is the authoritative full matrix. Read
[impact-matrix.md](references/impact-matrix.md) and, when a receipt is
supplied, [hook-evidence.md](references/hook-evidence.md) before commands. A
scoped pass is `changed-scope PASS`, never `full PASS`.

In `mode=full`, run the pipeline below and ignore hook omissions. In
`mode=changed`, run only uncovered matrix commands for impacted categories; do
not run unrelated suites or repeat eligible hook-covered formatting/lint.

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

Keep reusable caches shared. In particular, preserve an existing absolute
`GOCACHE` injected by Kandev's managed Go-cache provider, and preserve an
existing `GOLANGCI_LINT_CACHE`. Create an invocation-owned directory only for
scratch files and command logs. For example, replace `/var/tmp` below if a
different filesystem has the available space:

```bash
VERIFY_SCRATCH_ROOT="$(mktemp -d /var/tmp/kandev-verify.XXXXXXXX)"
mkdir -p "$VERIFY_SCRATCH_ROOT/tmp" "$VERIFY_SCRATCH_ROOT/logs"
export TMPDIR="$VERIFY_SCRATCH_ROOT/tmp"
export KANDEV_RUN_QUIET_DIR="$VERIFY_SCRATCH_ROOT/logs"
```

In a managed sandbox, request the normal filesystem escalation when the chosen
root is outside the writable roots; do not work around sandbox permissions.
If the cache filesystem itself is full or unwritable, relocate only the affected
cache to an explicit persistent, agent-owned path outside every worktree and
reuse that path on later verification runs. Never fall back to `.verify-cache`,
`.tmp`, or another directory inside the repository. Re-run the original failing
command before diagnosing source code. After verification, remove only
`$VERIFY_SCRATCH_ROOT`; do not clear shared caches or unrelated temp files.

### Restricted remote-environment failures

If Go tests fail from `httptest.NewServer` with an error such as
`listen tcp6 [::1]:0: socket: operation not permitted`, treat the first result
as a sandbox limitation. Rerun the exact command with the runtime's normal
network or loopback escalation. Diagnose test code only if the escalated rerun
still fails.

If that escalation is unavailable, denied, cancelled, or interrupted, stop and
return a blocked verification report with a **Required user action** section.
State that mandatory verification must pass before push and PR delivery can
continue. On Codex, the action must say exactly: "Switch the mode selector to
`Agent (full access)`, then retry verification." On other runtimes, tell the
user to enable the runtime's full filesystem, network, or loopback access as
needed, then retry verification. Do not offer to proceed with an unverified PR
or describe the blocker as an inability of Codex or the repository host to
create one. Recommend full access only after normal escalation could not
authorize the required capability in the current mode.

For desktop Rust changes, compare `rustc --version` with the `rust-version` in
`apps/desktop/src-tauri/Cargo.toml` before running the Rust suite. Activate an
installed matching rustup toolchain, extending `PATH` rather than replacing it
and losing Node/pnpm. If no matching toolchain is installed, report the exact
requirement or request installation instead of silently skipping Rust tests.

When a PR base was resolved, report whether `origin/$PR_BASE` is already an
ancestor of `HEAD`. Do not rebase, stash, or resolve conflicts in this role;
those require a bounded implementer assignment before verification continues.

For source, test, type, or lint failures, stop after capturing targeted failure
evidence. Report the command, quiet-log path and relevant lines, likely owned
files, and a concise remediation recommendation. Do not edit logic or rerun the
selected checks until the planner has integrated a fix.

If formatting changes files after commit, review and report the formatter diff,
invalidate the hook receipt and verified-commit state, then continue only to
collect useful evidence. The planner must commit the formatter result and
launch fresh verification before push. If a later command fails, capture
targeted evidence and stop for planner-assigned remediation.

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
