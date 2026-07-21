---
name: pr-fixup
description: Wait for CI checks and automated reviews (CodeRabbit, Greptile, Claude, OpenCode, cubic) on a PR, fix failures and address comments, then push.
---

# PR Fixup

Wait for CI and code review to complete on a pull request, fix any failures or valid comments, then push.

## Planner Entry

Load `/planner-orchestration`. The user-started primary session coordinates this
workflow by delegating polling to `pr-poller`, code/comment remediation to an
`implementer`, full checks to `verify`, and commit/push actions to a final
bounded implementer assignment. It does not run GitHub, test, edit, commit, or
push commands directly. If a required worker is unavailable, stop and report
the blocked phase.

Each worker uses only the procedure for its assigned phase: `pr-poller` gathers
state, a remediation implementer handles CI logs/code/comments, `verify` runs
checks and reports failures, and a delivery implementer commits or pushes an
already verified result. Workers never invoke one another.

> **GitHub tool selection:** This skill uses `gh` CLI commands by default. If `gh` is unavailable or fails, use any available GitHub tools in the environment (e.g. MCP GitHub tools) for PR checks, comments, replies, and reviews. Some operations (reactions, resolving threads, fetching CI logs) may not be available in all environments — skip gracefully.
> **Helper scripts location:** `scripts/pr-state`, `scripts/pr-resolve`, and `scripts/run-quiet` are at the worktree root (`<worktree>/scripts/...`), not under `.agents/skills/pr-fixup/scripts/`.

## Available skills and subagents

- **`pr-poller` worker** — Polls CI checks and the 5 review bots until terminal, returns a compact structured report.
- **`verify` worker** — Runs the full verification pipeline and returns a compact pass/fail report before delivery.
- **`/e2e`** — Read for debugging guidance when E2E tests fail in CI. Covers test patterns, run commands, failure triage, and local reproduction.
- **`/commit`** — Use for staging and committing fixes with Conventional Commits format.

The planner uses the registered `pr-poller` and `verify` workers. Do not
substitute generic agents. Any command procedure belongs only to the worker
assigned that section: polling, remediation, verification, or delivery. It is
never a planner fallback.

## Before anything else: create the pipeline

The first thing you do — before fetching PR state, before reading logs, before any fixes — is create a task list for the full pipeline. This is non-negotiable because it keeps you accountable to the process and lets the user see where you are.

Create these tasks immediately in the current session's todo/checklist tool. Do
not create Kandev subtasks or persistent work items unless the user explicitly
requests task tracking:

1. **Gather PR state** — Delegate to `pr-poller`; always include PR mergeability and local conflict state
2. **Fix failing CI checks** — Read failing run logs (via `scripts/run-quiet gh-run -- gh run view ...`), fix issues, run E2E tests locally if needed
3. **Triage review comments** — Classify each comment as valid, already addressed, nitpick, or wrong
4. **Address each comment** — Fix or reply with reasoning, resolve threads
5. **Verify, commit, push** — Delegate verification, then separate bounded commit/push delivery
6. **Re-check** — Delegate to `pr-poller` again; if new failures, loop back to task 2
7. **Summary** — Report what was done

Then start with task 1. Mark each task in_progress when you begin it and completed when you finish it.

---

## Steps

### 1. Gather PR state

Mark task 1 as in_progress.

The planner invokes the registered `pr-poller` with the PR number. The worker:
- Fetches the current CI/bot/comment state once
- Polls (30s cadence, **20 min cap**) until every CI check and every bot (CodeRabbit, Greptile, Claude, OpenCode, cubic) reaches a terminal state
- Counts unresolved review threads and bot issue comments
- Returns a structured report between `=== pr-poller report ===` and `=== end ===` markers

### Merge-conflict gate

Always check for merge conflicts during task 1 before acting on any clean-poller shortcut. A PR can be blocked by conflicts before any check fails.

The planner asks `pr-poller` to include GitHub mergeability and local unmerged
index state. If either reports conflicts, assign a bounded implementer to load
`references/merge-conflicts.md`, resolve them, and return targeted verification
before continuing. The planner does not inspect or resolve conflicts directly.

**Parse the report.** The fields you care about:

- `ci_failed` — list of `{name, run_id, conclusion, url}`. Empty list ⇒ CI is green.
- `ci_pending` — anything still running when the 20-min cap hit. Decide whether to re-invoke `pr-poller` after a short delay, or proceed with what you have and re-check at step 6.
- `bots.<name>` — `done` / `rate_limited` / `pending` / `timeout`. Anything in `done` or `rate_limited` has had its chance; treat the rest as missing data, not a blocker.
- `unresolved_review_threads` and `issue_comments_from_bots` — drive steps 3-4. If both are 0, `ci_failed` is empty, and the merge-conflict gate is clean, skip to step 5 (still run verify + push if you have fixes from earlier).

**E2E CI outlasts the poller.** The pr-poller caps at ~20 minutes. Standard E2E shards plus container shards often run longer. GitHub can expand E2E matrix jobs late, and the shard matrix may appear only after the build job finishes; if pending checks briefly drop near zero and then jump when E2E shards appear, keep treating that as normal pending CI unless a shard reports failure. If the report shows `ci_pending` with only E2E/lint jobs and `ci_failed` is empty, re-invoke pr-poller once those jobs finish — do not spin a manual `gh pr checks` loop in the parent. If the cap hits with E2E still pending, report "CI in progress" to the user instead of blocking, and include the exact pending shard names from `ci_pending`.

**Do not fetch poll output yourself** — that is what burns context. The report is the only thing that enters your context.

#### Polling Worker Reference

The commands below document what the registered `pr-poller` gathers. They are
not a planner or remediation-worker fallback. If `pr-poller` cannot be launched,
stop and report the blocked phase.

```bash
scripts/pr-state --summary <PR>
```

If this command, `scripts/pr-resolve`, or another GitHub helper fails with a transient DNS/network error, retry the parent command with the runtime's required network escalation; treat state as unknown until it succeeds.

`scripts/pr-state` accepts flags before or after the PR (`scripts/pr-state --summary <PR>` and `scripts/pr-state <PR> --summary` both work). When parsing output with `jq`, prefer writing the JSON to a temp file first, then running `jq` against that file; this avoids `set -e` surprises and prevents stderr from corrupting a JSON pipe.

Prefer `scripts/pr-state --summary <PR>` for direct polling and `scripts/pr-resolve list <PR>` for review-thread state over raw `gh pr checks`. `gh pr checks` only reports CI/status checks; review bots can open unresolved threads after CI is green, and a checks-only poll will miss that blocker.

By default, `scripts/pr-state` returns comments, reviews, and review threads created after the latest PR head commit only. This is intentional for fixup passes: it keeps old bot summaries and already-addressed historical threads out of the working set. Use `scripts/pr-state --summary --all <PR>` only when you need to audit the full PR history.

The summary output contains:

- `failed_checks` — actionable non-green checks with `name`, `workflow`, `status`, `conclusion`, `run_id`, `job_id`, `details_url`, and `target_url`
- `pending_checks` — still-running or queued checks with `name`, `workflow`, `status`, `run_id`, `job_id`, `details_url`, and `target_url`
- `unresolved_review_thread_count` — total unresolved thread count on the PR, including older unresolved threads outside the current-head filter
- `filtered_review_thread_count` — informational count for the current-head-filtered review-thread view; it can include resolved or historical comments, so do not treat it as a blocker by itself
- `unresolved_threads` — compact current-head inline review state to triage in this fixup pass
- `hidden_unresolved_threads` — compact unresolved threads outside the current-head filter when historical unresolved threads exist; treat these as actionable blockers, not ignorable history
- `errors` — data-gathering failures; treat affected fields as unknown instead of reconstructing them ad hoc

If `scripts/pr-state --summary <PR>` briefly returns `branch:"unknown"` or reports that `gh pr view` failed while `gh pr view <PR>` works directly, treat it as a transient state-resolution issue. Re-run the explicit PR-number command once and fall back to direct `gh pr view <PR>` / targeted `gh run view` checks for that pass instead of assuming the PR state is unavailable.

If `scripts/pr-state --summary <PR>` returns `since: null` with `errors` like `repo` or `since`, but still includes valid `failed_checks`, `pending_checks`, and `unresolved_review_thread_count`, use the check/thread data for that poll and rerun once on the next cadence. Do not discard usable CI state just because head-commit metadata lookup failed.

If `scripts/pr-state --summary <PR>` returns valid `failed_checks` and `pending_checks` but `unresolved_review_thread_count: null` with an error such as `{"source":"review_threads","message":"gh api graphql reviewThreads failed"}`, use the check data for that poll but treat review-thread state as unknown. Do not report review as clean or blocked from that snapshot; rerun `scripts/pr-state --summary <PR>` on the next cadence to recover thread state. Before a final status, retry `scripts/pr-state --summary <PR>` once; if review thread state still errors, run `scripts/pr-resolve list <PR>` directly. Only report "no unresolved review threads" after one of those succeeds.

If `scripts/pr-state --summary <PR>` fails with `jq: Argument list too long`, do not debug the summary script during fixup. Split the fallback checks:

```bash
gh pr checks <PR>
scripts/pr-resolve list <PR>
```

If the checks table is long or mixed with request diagnostics, keep only
actionable rows:

```bash
gh pr checks <PR> 2>/dev/null | awk -F '\t' '$2 == "pending" || $2 == "fail" {print}'
scripts/pr-resolve list <PR>
```

`gh pr checks` gives CI status only; `scripts/pr-resolve list` is still required for unresolved inline review threads. `gh pr checks` can exit nonzero when checks are pending or failed (for example exit code 8) while still printing usable status rows. Treat its table output as valid state, not automatically as a shell/tool failure; parse the rows for `fail`, `pending`, and `pass`, and avoid `set -e` flows unless you intentionally capture or ignore the exit code.

If `unresolved_review_thread_count` is nonzero but `unresolved_threads` is empty, or if `hidden_unresolved_threads` is non-empty, run `scripts/pr-resolve list <PR>` before acting. Fetch each full body with `scripts/pr-state --comment <comment_id>` or `scripts/pr-resolve show <PR> <THREAD_ID_OR_COMMENT_ID>`; hidden threads can contain valid current blockers even when the current-head filtered view is empty. `pr-state` can briefly report total unresolved count from historical state while current-head unresolved threads are empty; `pr-resolve list` is the authoritative actionable thread set for fixup work. If `scripts/pr-state --summary` reports a nonzero unresolved count but `scripts/pr-resolve show <PR> <THREAD_ID>` says the listed thread is `resolved: true`, treat the summary as stale state. Wait briefly, rerun `scripts/pr-state --summary <PR>`, and do not reply again to an already-resolved thread.

Poll at a 30s cadence with a **20 min cap**. Prefer one-shot `scripts/pr-state --summary <PR>` checks, or a bounded command that can finish naturally, over long inline shell loops. In this environment, a running non-TTY loop may not receive Ctrl-C through `write_stdin`, so avoid `while sleep ...; do ...; done` polling in the main session. Run `sleep 30` and then a fresh `scripts/pr-state --summary <PR>` as separate commands; a combined command can capture blank output. Record wall-clock time around requested sleeps: if less elapsed than requested, treat the wait as interrupted, re-poll, and report the shorter time. Rerun a blank summary before interpreting it.

Stop early if any required check fails. A `queued` or `in_progress` job caused by GitHub runner capacity is pending CI, not a failure: when the requested watch window or cap ends with no failures or unresolved comments, do not make speculative code changes. Report each pending job's exact name and status, plus its `details_url` or Actions run URL. If the user explicitly asks to poll until CI is green, continue past the normal pending-E2E stopping point with bounded one-shot `sleep N; scripts/pr-state --summary <PR>` checks; stop immediately on any `failed_checks`, and continue until `failed_checks: []`, `pending_checks: []`, and `unresolved_review_thread_count: 0`. Do not run `gh pr checks --watch` in the main session unless the runtime can keep the watcher isolated and automatically clean it up. If you do use `gh pr checks --watch`, keep watching until the command exits; GitHub can expand matrix jobs after an initial aggregate "Build" check passes, so the first green build/lint/test rows are not necessarily terminal.

For E2E-only pending phases, summarize counts before dumping long shard lists into context:

```bash
scripts/pr-state --summary <PR> > /tmp/prstate-<PR>.json
jq '{failed_checks, pending_count:(.pending_checks|length), pending_by_workflow:(.pending_checks|group_by(.workflow)|map({workflow:.[0].workflow,count:length,statuses:(map(.status)|unique)})), unresolved_review_thread_count, errors}' /tmp/prstate-<PR>.json
```

When stopping at the cap, print the exact pending names from the saved snapshot:

```bash
jq -r '.pending_checks[] | "\(.status) | \(.name)"' /tmp/prstate-<PR>.json
```

If a user interrupts a long manual poll loop (`sleep`, `gh pr checks`, or `scripts/pr-state`), check for leftover polling processes before switching tasks and terminate only the processes you started.

Use raw mode only when debugging an odd GitHub state:

```bash
scripts/pr-state <PR>
```

Mark task 1 as completed.

### 2. Fix failing CI checks

Mark task 2 as in_progress.

The planner assigns this section to a remediation implementer with the poller
report, failed run IDs, owned files, and acceptance criteria. The implementer
does not invoke other workers.

**Sanity-check the poller's `ci_failed` before fixing anything.** Confirm each reported check `name` actually appears in `gh pr checks <PR>` output and its `run_id` resolves (`gh run view <run_id>` must not 404). If the report cites checks the repo doesn't have, discard it and re-gather state directly before touching code.

Prefer `scripts/pr-state <PR>` for this cross-check before falling back to raw `gh` output. It already gives you raw checks with normalized names plus extracted `run_id`s in one JSON snapshot.

When local verification passes but CI fails, reproduce the exact command from the failed job log before changing code. Backend lint can run stricter CI-only modes such as:

```bash
golangci-lint run ./... --new-from-rev="${BASE_SHA}" --timeout=5m
```

Do not assume plain `make lint` covers that failure; `--new-from-rev` can surface new-code-only lint failures such as complexity or cyclomatic threshold issues. After fixing backend lint, rerun the exact CI-shaped command from `apps/backend` before pushing.

If `Run Backend Tests` fails and the visible error is a report step such as `Generate test report` failing with `ENOENT: no such file or directory, open 'apps/backend/test-results.json'`, treat that as a downstream symptom. Search earlier in the saved CI log for `Run golangci-lint`, `.go:<line>:<col>:`, `1 issues:`, or `Process completed with exit code 1`, then reproduce with the CI-shaped `golangci-lint ... --new-from-rev=<base> --timeout=5m` command instead of debugging the missing report file.

For each entry in the report's `ci_failed:` list:

1. Use the `run_id` from the report (the poller already extracted these — don't re-run `gh pr checks`).
2. Fetch the failed logs via `scripts/run-quiet` — `gh run view --log-failed` dumps thousands of lines and will blow your context if it goes straight to stdout. The wrapper redirects to `/tmp/kandev-run.gh-run.<random>.log` and auto-greps for the relevant error lines:
   ```bash
   scripts/run-quiet gh-run -- gh run view <run-id> --log-failed
   ```
   If the printed summary is enough, stop. Only `Read` specific line ranges from the printed log path if you need surrounding context.
   If `gh run view --log-failed` returns only GitHub request metadata and no failure output, immediately fetch the failed job log directly and search the saved file for failing specs/errors:
   ```bash
   job_id="<failed job id from scripts/pr-state or gh run view --json jobs>"
   gh api repos/:owner/:repo/actions/jobs/"$job_id"/logs > /tmp/kandev-job.log
   rg -n "(Error:|FAIL|Failed|Timed out|Timeout|\\.spec\\.ts|ghcr\\.io|docker/login-action)" /tmp/kandev-job.log
   ```
   The job-log endpoint may return plain text rather than a zip; inspect the saved file directly before assuming it needs `unzip`. Then inspect targeted line ranges or downloaded artifacts rather than streaming the full log into context.
   If fetching logs fails with a transient GitHub or DNS error such as `lookup api.github.com ... i/o timeout` or `error connecting to api.github.com`, wait one short cadence and retry once before treating the logs as unavailable or debugging from stale local output.
   If `gh run view --log-failed` says logs are unavailable because the workflow is still running, wait for the workflow/report job to finish before retrying.
3. Read the relevant source files at the failing lines (use `Read` with `offset`/`limit`, not `cat`)
4. Fix the issues (lint errors, test failures, type errors, etc.)

If the failure is unfamiliar, looks like infrastructure, or involves E2E, load
`references/ci-troubleshooting.md` before changing code. It covers branch
history checks, cancelled concurrency duplicates, semantic-title transport
failures, Vitest runner crashes, E2E container setup failures, and local E2E
reproduction.

Mark task 2 as completed.

### 3. Triage review comments

Mark task 3 as in_progress.

The planner assigns this section to a remediation implementer with the compact
thread list and relevant diff scope.

Use the report's `unresolved_review_threads` and `issue_comments_from_bots` counts to know whether there's anything to triage. If both are 0, mark this step completed and move on.

Otherwise, fetch the actual comment bodies on demand — one bot or one set at a time, not all at once:

```bash
# Inline review threads (humans, Greptile, Claude same-repo, OpenCode, cubic):
gh api repos/:owner/:repo/pulls/<number>/comments
# Issue comments (CodeRabbit walkthrough, Claude fork findings):
gh pr view <number> --json comments
```

When `scripts/pr-resolve list <PR>` returns a `comment_id`, prefer fetching the exact review comment over bulk-fetching all PR comments/reviews if only one or two unresolved threads are actionable:

```bash
scripts/pr-state --comment <comment_id>
# Fallback if the helper is unavailable:
gh api repos/:owner/:repo/pulls/comments/<comment_id> --jq .body
```

This keeps context small and avoids mixing current unresolved review comments with old bot summaries.

When piping `gh api` or `gh api graphql` to `jq`, never use `2>&1`. `gh` writes diagnostics to stderr, and merging them corrupts the JSON stream (`jq: parse error: Invalid numeric literal`). Use `2>/dev/null` or `gh`'s built-in `--jq`.

Bot issue comments such as CodeRabbit walkthroughs, Claude summaries, Greptile summaries, and historical "actionable comments posted" notices are informational once `scripts/pr-resolve list <PR>` is empty and the latest bot check is passing. Do not reply to old top-level summaries unless they contain a current unresolved request.

To verify whether OpenCode posted a no-findings or diagnostic PR-visible comment, search issue comments for its stable markers, not only review threads:

```bash
gh api repos/:owner/:repo/issues/<PR>/comments \
  --jq '.[] | select(.body | contains("opencode-review")) | {user:.user.login, created_at, updated_at, body}'
```

OpenCode valid-empty output is represented by `<!-- opencode-review:no-findings -->`; absence of inline review comments does not mean OpenCode skipped review.

If an automated review check fails with a tool diagnostic instead of actionable code findings, classify it separately from review feedback. Fetch the failed job log and any diagnostic comment, but do not invent code changes for infrastructure output such as `OpenCode did not produce parseable findings`. Keep polling or rerun as appropriate until the required check is no longer failed.

When CodeQL opens an inline security review thread, resolving the thread is only half the work. The PR is not clean until the corresponding CodeQL check reruns successfully on the new head and `failed_checks` is empty.

**Verify before implementing.** Do not blindly accept review feedback — evaluate each comment technically:

For each comment:
1. Restate the requirement in your own words — if you can't, ask for clarification
2. Check against the codebase: is the suggestion correct for THIS code?
3. Check if it breaks existing functionality or conflicts with architectural decisions
4. YAGNI check: if the suggestion adds unused features ("implement properly"), grep for actual usage first

Then classify:
- **Valid and actionable** — real issue (bug, missing edge case, naming, architecture, code quality). Fix it.
- **Already addressed** — the code already handles what the comment suggests. Skip.
- **Nitpick or preference** — subjective style not covered by linters. Skip unless the reviewer insists.
- **Wrong or outdated** — misunderstands the code, refers to old state, or is technically incorrect. Push back with reasoning.

For sanitizer-schema feedback, inspect the installed package's `defaultSchema`
and the production renderer before changing code. If the claimed element or
attribute is already allowed, add or retain a regression that proves the
behavior and reply with that evidence rather than duplicating a schema change.

For review comments about CI/workflow changes, account for `pull_request_target` trust boundaries. Source agent instructions from the base commit or trusted workflow content, not PR-head harness/config files; keep `GH_TOKEN` out of direct model execution when a trusted wrapper can post comments afterward; and validate model-produced comment targets against the computed diff scope before posting.

**Push back when:**
- The suggestion breaks existing functionality
- The reviewer lacks full context (explain what they're missing)
- It violates YAGNI (the feature is unused)
- It's technically incorrect for this stack
- It conflicts with architectural decisions

Mark task 3 as completed.

### 4. Address each comment

Mark task 4 as in_progress.

The same bounded remediation implementer addresses only the assigned threads
and reports its edits, replies, and targeted checks.

Every comment must get a response — either a fix or a reply explaining why it was skipped.

**Per-thread engagement is mandatory. Do not take shortcuts:**

- **Never post a single summary issue comment in place of individual thread replies.** A top-level summary comment leaves every inline thread unresolved and unanswered; reviewers have to hunt for your response across the diff. The only acceptable use of a summary comment is as an *addition* to per-thread replies, not a substitute.
- **Every unresolved review thread on the PR must receive a direct reply and be resolved**, even if that means 20+ thread interactions. Looping over threads programmatically is fine (and expected); batching into one summary is not.
- **Reply to the comment that started the thread**, not a random later one. Get the first-comment ID from the GraphQL `reviewThreads(first: 100) { nodes { comments(first: 1) { nodes { databaseId } } } }` query.
- **Do not mark task 4 completed until every previously-unresolved review thread is either resolved or has an explicit reason documented in a reply.** If you finish the pass and the `isResolved == false` set is still non-empty, you are not done.

**Important: issue comments vs review comments use different APIs:**
- **Review comments** (inline, from `gh api repos/:owner/:repo/pulls/<number>/comments`) — use `scripts/pr-resolve` (below).
- **Issue comments** (conversation timeline, from `gh pr view --json comments` — e.g., CodeRabbit walkthrough) — reply by posting a new comment via `gh pr comment <number> --body "..."`, react via `gh api repos/:owner/:repo/issues/comments/<comment_id>/reactions -f content="+1"`. There's no "resolve" concept for issue comments.

### Review comments: `scripts/pr-resolve`

Use the script for every review thread, not just batches — it collapses reply + resolve + +1 reaction into a single call so you don't re-derive the graphql mutation each session.

```bash
# Dump every unresolved thread, TAB-separated (tid, cid, author, path, body_first_120):
scripts/pr-resolve list <PR>

# Show full thread/comment details before deciding how to respond:
scripts/pr-resolve show <PR> <THREAD_ID>
scripts/pr-resolve show <PR> <COMMENT_ID>

# Reply syntax is:
#   scripts/pr-resolve reply <PR> <comment_id> <thread_id> <body>
#   scripts/pr-resolve reply <PR> <comment_id> <thread_id> --body-file <path>
# Note the order: list prints <thread_id> then <comment_id>, but reply takes
# <comment_id> before <thread_id>. If reply returns "Parent comment not found",
# first check whether those two IDs were swapped.
#
# Reply + resolve + +1 (same call whether you're agreeing or pushing back —
# the body text conveys which):
scripts/pr-resolve reply <PR> <COMMENT_ID> <THREAD_ID> 'Fixed via monotonic counter in useRef. See commit abc1234.'
printf '%s\n' 'Acknowledged; the strict source check was relaxed for E2E. Tracking as a follow-up.' > /tmp/pr-reply.txt
scripts/pr-resolve reply <PR> <COMMENT_ID> <THREAD_ID> --body-file /tmp/pr-reply.txt
```

When calling `scripts/pr-resolve reply`, avoid double-quoted bodies that
contain Markdown backticks, `$...`, or shell metacharacters; the shell will
execute or expand them before the script sees the text. Prefer single quotes
for simple bodies, `--body-file` for multi-sentence replies, or plain text
without Markdown code formatting. If quoting is awkward, shorten the reply and
reference the commit SHA.

If `scripts/pr-resolve reply` fails mid-flight, first rerun `scripts/pr-resolve list <PR>` (with network escalation after a transient DNS/network error), then run `scripts/pr-resolve show <PR> <THREAD_ID>` before retrying. Confirm whether the reply landed and only resolve/reaction failed, so you do not post a duplicate reply. When replying to several threads, check every command result; retry individual failures caused by transient GitHub, DNS, or API errors before treating that thread as still actionable.

`scripts/pr-resolve list` only prints previews. Before deciding whether a thread is valid, stale, duplicate, or already fixed, fetch the full comment/thread body:

```bash
scripts/pr-resolve show <PR> <THREAD_ID_OR_COMMENT_ID>

# Repo helper for a single review comment:
scripts/pr-state --comment <comment_id>

# Raw fallback for a single review comment:
gh api repos/:owner/:repo/pulls/comments/<comment_id> --jq '{body,path,line,commit_id,html_url}'

# Raw fallback for all PR review comments when the helper only supports list/reply:
gh api repos/:owner/:repo/pulls/<PR>/comments --paginate

# Use GraphQL reviewThreads when you need thread IDs, resolution state,
# or all comments in a review thread.
```

Bots may post duplicate or stale review threads against the previous head immediately after a fix push. Check `commit_id` on the full comment. If the current branch already contains the fix, reply and resolve with wording like "Fixed in <new commit>."

For valid comments: read the file, implement the fix, then call `pr-resolve reply` with a body that names the commit or the file:line of the fix.

If the valid fix is PR-description prose rather than code, edit the PR body as part of the fixup before replying. If `gh pr edit <PR> --body-file <file>` fails with the GitHub Projects classic deprecation / `repository.pullRequest.projectCards` GraphQL error, do not retry the same wrapper. Use the REST body fallback documented in `/pr`, or fetch the PR node ID (`gh pr view <PR> --json id --jq .id`) and update the body through GraphQL `updatePullRequest`.

If the fix moves, splits, or deletes tests or source files, check both the original and destination paths before committing. Use `rg` for symbols or imports that were removed from the original file, and run targeted lint/typecheck commands that cover every touched file. File splits can leave missing imports or stale references in the file that stayed behind even when the moved test passes.

For skipped comments (already addressed, nitpick, wrong, outdated): call `pr-resolve reply` with a body that explains why. Examples:
- "This is already handled by X on line Y."
- "This is a style preference not enforced by our linters — keeping as-is."
- "Refers to code that was changed in a later commit."

For dozens of threads grouped by topic, declare a bash associative array mapping thread IDs → category, then a `reply_for` case that returns the right body per category. Avoids retyping the same explanation across duplicate threads from multiple bots:

```bash
declare -A CAT=(
  [PRRT_xxx1]=fixed_counter
  [PRRT_xxx2]=fixed_counter
  [PRRT_xxx3]=skipped_source_guard
)
declare -A COMMENT_ID=(
  [PRRT_xxx1]=3253164429
  [PRRT_xxx2]=3253168996
  [PRRT_xxx3]=3253164669
)
reply_for() {
  case "$1" in
    fixed_counter) echo "Fixed — monotonic counter via useRef. See commit abc1234." ;;
    skipped_source_guard) echo "Acknowledged; the strict source check was relaxed for E2E. Tracking as a follow-up." ;;
  esac
}
for THREAD_ID in "${!CAT[@]}"; do
  scripts/pr-resolve reply <PR> "${COMMENT_ID[$THREAD_ID]}" "$THREAD_ID" "$(reply_for "${CAT[$THREAD_ID]}")"
done
```

### Verify resolution before moving on

Run `scripts/pr-resolve list <PR>` — output must be empty. Run it again after pushing fixes and resolving threads; automated reviewers may add duplicate or new threads on the latest pushed SHA.

Informational threads (acknowledged, no code change) still need `scripts/pr-resolve reply` + resolve — skipping them leaves the PR blocked.

Mark task 4 as completed.

### 5. Verify, commit, and push

Mark task 5 as in_progress.

1. The planner launches the registered `verify` worker. It reports failures
   without changing source/test logic.
2. If verification fails, the planner creates a new bounded remediation packet,
   then launches a fresh verification assignment. Do not deliver until green.
3. After a green report, the planner delegates commit and push as bounded
   delivery assignments using `/commit` and `/push`. Supply the verification
   result and exact intended paths. Workers do not invoke one another.

Mark task 5 as completed after the delivery worker reports the pushed commit.

### 6. Re-check PR state

Mark task 6 as in_progress.

After the push, CI restarts and bots may re-review. The planner launches
`pr-poller` again with the same contract and 20-minute cap. If it cannot be
launched, stop and report the blocked phase. Parse the returned report:

- Require the new poller report to include newly opened review threads before
  waiting on CI. Review bots may add threads after earlier ones were resolved.
- Treat the PR as clean only when `ci_failed:` is absent, `ci_pending: none`,
  `unresolved_review_threads: 0`, `issue_comments_from_bots: 0`, and every bot
  row is terminal (`done`, `rate_limited`, or `timeout`).
- Treat a clean intermediate state as provisional while `ci_pending` is not
  `none` or any bot row is `pending`/`unknown`. Reviewer services may add new
  threads after CI is mostly green.
- Do not report the PR as review-clean while harness lint or bot review checks
  are queued or in progress unless the bounded poll cap is reached. Report the
  pending checks exactly from the poller result.
- If only long-running checks remain in `ci_pending` after full local
  verification is green, `ci_failed:` is absent, and
  `unresolved_review_threads: 0`, stop at the bounded cap and report the exact
  pending checks. Continue immediately if a check fails or a new thread appears.
- If a pushed fix restarts CI while previous checks were in progress, launch a
  fresh poller packet. If checks remain pending at the cap, report their exact
  names from `ci_pending`.
- If new CI failures appeared from the latest commit → loop back to task 2 and reset task 2-5 to `in_progress` as needed.
- If new review comments appeared after the push → loop back to task 3.
- If the poller hit its cap (`recommendation:` mentions "timed out") → surface the remaining pending items to the user and stop.

Before declaring the PR complete, launch one final `pr-poller` assignment that
includes head SHA, mergeability, and local unmerged-index state. If conflicts
appear, assign conflict resolution, verification, delivery, and re-polling as
new bounded packets.

Cap re-check loops at **3 iterations** to prevent runaway sessions. After 3, surface the remaining state to the user and stop.

Mark task 6 as completed.

### Multi-round bot reviews

**Expect new threads after every push.** CodeRabbit, Greptile, Claude, OpenCode,
and cubic often re-review the latest commit. On cross-cutting changes, plan for
2-3 fixup rounds. After each push, launch `pr-poller` again and require its
report to include both summary state and actionable unresolved threads; do not
rely on the prior round.

**Stop when green unless the next comment is clearly worth another cycle.** Tiny review-only commits restart the full CI and bot-review stack. Once the PR is green with no unresolved threads, avoid nonessential cleanup that would trigger another round unless the comment is blocking, clearly valid, or requested by the user.

### 7. Summary

Mark task 7 as in_progress.

Report what was done:
- CI checks: which failed and how they were fixed
- Comments addressed (with thumbs up)
- Comments skipped and why
- Link to the pushed commit
- Re-check iteration count

Mark task 7 as completed.
