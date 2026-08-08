---
name: pr-fixup
description: Wait for CI and automated reviews on a PR, fix valid failures and comments in the primary conversation, verify, and push.
---

# PR Fixup

Use this workflow directly in the user-started primary conversation. Do not
launch a verifier, implementer, or other remediation subagent. A read-only
`pr-poller` is the sole exception: launch it only when the user explicitly asks
to wait for or monitor PR updates. For a cost-controlled workflow, the user may
switch the same conversation to the lower-cost implementation/test model before
starting CI remediation.

Use `gh` by default; auth or transport errors leave state unknown, never clean.
If connector tools are available, use structured PR/check/thread data; avoid
dumping full HTML/diffs. Map GraphQL thread IDs to REST comment IDs before
replies, and refresh current-head state after pushes and review aggregation.

## Pipeline

Create a visible checklist:

1. Gather PR state
2. Fix failing CI checks
3. Triage review comments
4. Address valid comments
5. Commit, rerun affected checks, and push
6. Re-check the new head
7. Report

## 1. Gather PR State

Before the first GitHub helper call, request any runtime network approval that
the environment requires. If access is denied, cancelled, or interrupted, stop
the workflow permanently; retry only transient fetch failures after access is
approved.

Run `scripts/pr-state --summary <PR>` once. Record `checks_head_sha`,
`checks_snapshot_complete`, `failed_checks`, `pending_checks`, and the PR
head delivery fields (`pr.head_repository_owner`, `pr.head_repository_name`,
`pr.head_ref_name`, `pr.head_ref_oid`, and `pr.maintainer_can_modify`). For a
cross-repository PR, those fields are the only authoritative push target; do
not infer it from a local remote named `fork`, `contributor`, or similar.
Linked worktrees share Git configuration and such remotes may belong to an
unrelated task.
Record `unresolved_review_thread_count`, `filtered_review_thread_count`,
`hidden_unresolved_threads`, and
`actionable_issue_comment_count`. Inspect mergeability separately through
`references/merge-conflicts.md`; it is not a `pr-state --summary` field. If a
named reviewer is the semantic evidence source, use `--trusted-reviewer` only
when `review_evidence.trusted_producer` is `"true"`; never use that shortcut
for forks, security, or architecture. Also inspect every non-empty body in
`review_evidence.exact_current_head_reviews[]`, including `COMMENTED`
aggregate-bot reviews: `scripts/pr-resolve list` can be empty while an
exact-head review body still contains actionable findings. Classify each body
as actionable, already addressed, optional, or invalid and record a concrete
skip reason for anything not acted on before declaring reviews clear.
Treat `trusted_producer=true` as qualifying provenance only for the dedicated OpenCode App, never merely because a reviewer name matches.
If `hidden_unresolved_threads` is non-empty, immediately run
`scripts/pr-resolve list <PR>` and triage its output. A zero current-head
unresolved count does not make hidden threads clean.

If the fresh mergeability query reports `mergeable=CONFLICTING` or
`mergeStateStatus=DIRTY`, stop CI and review triage. Load
`references/merge-conflicts.md`, resolve or rebase the conflict, verify the
result, push it, and restart this section with a new PR-state snapshot. Do not
triage comments or checks against a conflicted head.

For pending CI, do not run a rapid polling loop. Wait at a reasonable interval,
then run the same summary again. Stop after about 20 minutes and report the
exact pending checks as "CI in progress." Use two monitoring modes: default
monitoring may return early for a failed check, merge conflict, actionable
finding, or clean terminal state; strict-deadline monitoring applies when the
user says "wait N minutes" or "then fix up." In strict mode, calculate and
pass an absolute deadline, accumulate failures/comments, and do not return
early for findings, pending checks, or a clean snapshot; stop early only when
the PR is merged/closed or access is revoked. Do not use interactive `gh pr
checks --watch` in the primary conversation: its TTY redraws make captured
output unusable. Use saved `scripts/pr-state --summary` snapshots about 60–90
seconds apart, or the read-only `pr-poller` only when the user explicitly asked
to wait or monitor.
Treat a poller's unresolved/pending snapshot as provisional: it can predate a
primary-session push or thread resolution. Re-run `scripts/pr-state --summary
<PR>` at the current head before acting on it or declaring completion.
If a job remains pending beyond the workflow's configured timeout, or its status
conflicts with the GitHub UI/API, query the exact job with
`gh api repos/<owner>/<repo>/actions/jobs/<job_id>` (or inspect the run with
`gh run view <run_id>`) before calling CI hung or changing code. Treat the direct
result as current-head evidence only after its `head_sha` matches
`checks_head_sha`; otherwise report the result as stale or unknown.

For a cross-repository PR whose current-head snapshot is unexpectedly sparse,
inspect `approval_required_runs`. A current-head workflow with
`conclusion=action_required` is blocked verification, not green or skipped CI.
Only after the user authorizes PR fixup, approve the exact run with
`gh api --method POST repos/<base-owner>/<base-repo>/actions/runs/<run-id>/approve`,
then re-run the summary and require jobs to materialize before polling. `gh run
approve` is not a valid command.

Treat the state as clean only when the current head has no failed or pending
checks, no merge conflict, no actionable review thread or issue comment, and
qualifying exact-head semantic evidence where PR delivery requires it.

## 2. Fix CI Failures

Before changing code, confirm every reported failed check, its `run_id`, and
the parent workflow/job status. A failed job can be visible while its workflow
is still in progress; confirm its conclusion and failing step before treating
it as reproducible code evidence. Use
`scripts/run-quiet gh-run -- gh run view <run-id> --log-failed` so large logs do
not flood the conversation. If it returns only GitHub request/transport lines
or no failure text, treat logs as unavailable and, after terminal state, use
`scripts/pr-state --job-log <job_id>`. For temporary gaps or aggregate-only
logs, use the same fallback; it handles plain-text and ZIP responses and emits
bounded context. Follow `references/ci-troubleshooting.md`. Reproduce the exact
failed command where possible; CI-specific Go lint often needs
`golangci-lint run ./... --new-from-rev=<base> --timeout=5m`.

If CI reports files or commits outside the PR diff, or a stale base SHA, resolve
the authoritative base repository, ref name, and current base SHA from PR
metadata. Fetch that ref from an explicit base remote, verify its tip matches
the reported SHA, and compare `git merge-base HEAD <base-remote>/<base-ref>`
with `git diff <base-remote>/<base-ref>...HEAD`; do not assume `origin/<base>`
when `origin` points to a fork. Inspect the parent workflow/run to determine
whether a newer base commit caused the failure before changing product or docs
code. If the fix is already upstream, update or rebase the branch only when
authorized, rerun affected checks, and invalidate all prior exact-head evidence.

For unfamiliar, infrastructure, or E2E failures, load
`references/ci-troubleshooting.md` before changing code.
Also load it for unexpected zero-duration or no-op manual-review runs: event
and workflow provenance can explain them without a product-code change.

Fix with `/tdd` or `/e2e` as applicable, run focused checks, and keep each
remediation scoped to the reported failure. Do not suppress a failure or mark a
check clean without fresh evidence.

If a reproducible failure is outside the PR diff, compare the failing
assertion with the current implementation and concurrent or sibling PRs before
editing. If it is a stale test expectation, the smallest valid remediation may
be a test-only assertion update: keep it limited to the reported failure, run
the focused test, and call out that scope. Do not change unrelated production
behavior or duplicate a larger sibling change.
When a remediation changes a documented behavior or contract, update the
authoritative spec/guidance, plan, and task file when present; commit docs and
code together, keep regression tests and verification commands aligned, and
re-check the documentation before completion. Record why no update is needed
when the behavior remains internal.

## 3. Triage And Address Reviews

Use `scripts/pr-resolve list <PR>` to obtain unresolved threads. Its previews
can be truncated, so expand each listed review thread with
`scripts/pr-resolve show <PR> <thread_id>` before deciding whether it is valid,
already addressed, a preference, or wrong for this codebase. Use
`scripts/pr-state --comment <comment_id>` only for a flat comment view when a
thread context is not available. Validate against the current head, the spec,
and existing architecture before editing or replying.

Make only valid changes. GitHub replies and thread resolution are external
writes. A direction to "address" valid review comments explicitly authorizes a
concise reply and resolution after the fix is pushed and targeted verification
passes; a review-only request does not. For an invalid comment, reply with
concrete reasoning only when that authorization includes a response. When
writes are not authorized, report valid comments as addressed in code but still
unresolved; do not declare the PR clean solely from the code change. Resolve an
authorized thread only when the change or response genuinely addresses it.
An explicit request to "address them" authorizes replies and resolution only
for the selected current actionable threads. If new comments appear during
remediation, report them or obtain separate confirmation before replying or
resolving them.

After an authorized fix is pushed, use the atomic helper path
`scripts/pr-resolve reply <PR> <comment_id> <thread_id> --body-file <path>` to
reply, resolve, and react in one operation when the body contains Markdown or
shell metacharacters. For short plain-text bodies, a safely quoted argument is
acceptable. Never interpolate review text into an unquoted shell command or
use backticks in the command itself. After the helper returns, re-fetch the
thread and verify the posted reply body before treating the write as complete.
Then rerun
`scripts/pr-resolve list <PR>` and the exact-head `scripts/pr-state --summary
<PR>` check before reporting.

For an ordering or concurrency finding, trace the complete producer → event-bus
transport → gateway/client path. Sequential publishes do not prove delivery
order when a remote bus uses separate subscriptions; consolidate one stream or
add sequence-aware buffering when order is contractual, and cover both the
transport boundary and local emulator.

When feedback says an action must remain reachable, add and run a regression at
the legal minimum width. Verify the actual hit target (for example,
`elementFromPoint()` at the control center) and clickability, not only
`toBeVisible`, before pushing.

## 4. Commit, Verify, Push

Commit through `/commit`, then rerun only the unit, integration, or E2E command
affected by the remediation. Push when that targeted check passes for the exact
current `HEAD`. For a cross-repository PR, push the exact current `HEAD` to the
summary's authoritative head repository and ref only when
`pr.maintainer_can_modify` is true. Re-fetch the PR afterward and require its
`pr.head_ref_oid` to equal local `HEAD`; an upstream remote comparison is not
sufficient. Run broad `/verify` only if the user explicitly requests it or the
PR/CI finding requires it.
After every push, also run a fresh
`gh pr view <PR> --json baseRefName,headRefOid,mergeable,mergeStateStatus` (or
equivalent) at that pushed head. Require `headRefOid` to equal local `HEAD` and
confirm `mergeable` is not `CONFLICTING` and `mergeStateStatus` is not `DIRTY`;
`scripts/pr-state --summary` does not include mergeability.

Immediately before a remediation commit or push—and again after long-running
remediation—refresh PR state. Require the PR to remain open and its head ref to
match the local branch. Before a push, compare the remote head OID with the
local upstream tip; after the push, require the PR head OID to equal local
`HEAD`. If the PR merged or closed, do not recreate its deleted branch with a
stale push: preserve the local fix and ask before creating a clean follow-up.

After any rebase or force-push, fetch the PR base and compare local `HEAD`, the upstream tip, and `pr.head_ref_oid`; rerun affected checks.
A rebase onto a newer base invalidates prior evidence: rerun the exact failed
command and any package-level gate required by whole-file rules (for example
max-lines). Then rerun `scripts/pr-resolve list <PR>` and `scripts/pr-state --summary <PR>`; distinguish stale/current
failures and report pending checks separately. Use `--force-with-lease`, never an unconditional force-push.
If the rebase or conflict resolution touched `AGENTS.md`, `CLAUDE.md`, or a
skill/reference file, also run `python3 scripts/lint-harness-files.test.py` and
`python3 .github/scripts/lint-harness-files.py --all` before pushing. Run
`git diff --check` and inspect `wc -l` for each changed harness file so upstream
additions cannot push a file past its line budget.

## 5. Re-check

After every push, re-fetch current-head state and run
`scripts/pr-resolve list <PR>` (and `show` for any thread you may answer), then
`scripts/pr-state --summary <PR>` again for the new head. For explicit
consolidation, verify the target head before commenting/closing the superseded
PR; preserve its branch unless deletion is requested. Automated reviewers may
resolve or replace threads; do not reply to or resolve a thread that fresh state
reports as resolved. Before replying to a pre-push thread, run
`scripts/pr-resolve show <PR> <thread_id>`; if it reports `resolved: true` (often
with an `Addressed in commit ...` marker), record the thread as auto-resolved and
do not post a duplicate reply. Continue replying/resolving only for `resolved: false` threads, including hidden unresolved threads.
Treat each fresh summary as a new review-evidence snapshot: inspect every
non-empty body in `review_evidence.exact_current_head_reviews[]`, even when
`unresolved_review_thread_count=0` and `scripts/pr-resolve list` is empty.
Classify current-head review bodies and top-level bot/issue comments before
declaring the PR clean; empty thread and issue-comment counts are insufficient.
Treat a non-empty `hidden_unresolved_threads` value in that fresh snapshot as a
mandatory hidden-thread gate: run `scripts/pr-resolve list <PR>` again after
the refresh and immediately before reporting.
Require `checks_head_sha` to match that head, report pending checks separately
from failures, and rerun `scripts/pr-resolve list <PR>` before declaring the
PR clean. The final predicate must also require
`hidden_unresolved_threads=[]`; do not require filtered and unresolved counts to
be equal because the filtered count includes resolved threads. Treat prior review
evidence as stale. When the user authorized thread
writes, a duplicate or stale bot thread still needs an explicit reply and
resolution once current source proves the finding is already fixed, including a
thread surfaced only in `hidden_unresolved_threads`; only current-head
actionable threads drive code changes. Declare the PR clean only when the
exact-current-head review classification reports no unaddressed findings,
`checks_snapshot_complete=true`, `failed_checks=[]`, `pending_checks=[]`,
`approval_required_runs=[]`, `actionable_issue_comment_count=0`,
`hidden_unresolved_threads=[]`, there is no merge conflict, and
`scripts/pr-resolve list <PR>` is empty. Within
the user's monitoring limit, continue checking after resolutions until automated
review jobs are terminal; otherwise report the exact pending check names.

If the user explicitly requested a persistent Kandev plan update and the task
has an external Kandev plan, update it after fixup with the remediation commit,
final exact-head check counts, resolved-thread state, and mergeability. Without
that authorization, report the plan update as pending and do not invoke Kandev
task or session APIs. Batch plan/task synchronization into the final documentation
commit. For tracked `docs/plans/**` artifacts, keep prose head-agnostic and record
remediation scope/local verification before that commit; a later metadata commit
creates a new head and requires a fresh PR-state check. Mark prior current-head
claims historical/superseded when a new head replaces them; report only the latest head's SHA, CI/review counts, and mergeability. Do not leave planned verification marked unstarted after it has run.

Before declaring fixup complete, verify `git status --short` is clean,
`git rev-parse HEAD` equals `git rev-parse @{upstream}`, the PR head equals
local `HEAD`, and the fresh mergeability state is not conflicting. Do not call
the PR clean from CI/review counts alone when the worktree or remote tip still
differs.

## 6. User-Requested Merge

Merge only after the user explicitly asks and the current-head state is clean.
From a linked worktree, run `gh pr merge <PR> --squash` without
`--delete-branch`: that flag can attempt a local checkout of the base branch
and fail when another worktree owns it, even after the remote merge succeeds.
Report the remote merge separately. Delete a remote or local branch only when
requested and through a worktree-safe cleanup flow.

## Guardrails

- Do not create Kandev subtasks unless the user explicitly asks for task
  tracking.
- Do not use native delegation or a full-history context fork to poll CI.
- Do not push, post comments, or resolve threads when the user asked for review
  only.
- Do not proceed with an unverified PR when mandatory verification is blocked.
