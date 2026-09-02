---
name: pr-fixup
description: Wait for CI and automated reviews on a PR, fix valid failures and comments, disposition every review thread, verify, and push in the primary conversation.
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
Treat the OpenCode App as trusted evidence only when `trusted_producer=true` confirms its dedicated producer provenance.

## Pipeline

Create a visible checklist:

1. Gather PR state
2. Resolve an authorized PR merge conflict
3. Fix failing CI checks
4. Triage every review thread
5. Apply valid fixes and record every disposition
6. Commit, rerun affected checks, and push
7. Re-check the new head
8. Report

## 1. Gather PR State

Before the first GitHub helper call, request any runtime network approval that
the environment requires. If access is denied, cancelled, or interrupted, stop
the workflow permanently; retry only transient fetch failures after access is
approved.

Run `scripts/pr-state --summary <PR>` once. Record the current-head check,
review, and PR-delivery fields described in
`references/review-evidence.md`. For a cross-repository PR, use only the
reported delivery fields as the push target; do not infer it from a local
`fork` or `contributor` remote. Load that reference for exact-head review
classification, authentication fallbacks, and hidden-thread handling. Inspect
mergeability separately through `references/merge-conflicts.md`.

If the fresh mergeability query reports `mergeable=CONFLICTING` or
`mergeStateStatus=DIRTY`, stop CI and review triage. This is an actionable PR
blocker, not a clean or report-only terminal state. Load
`references/merge-conflicts.md`. If the user has explicitly authorized PR
fixup or conflict resolution, resolve the conflict now (prefer a merge of the
fresh base unless the user requests a rebase), verify the result, push it, and
restart this section with a new PR-state snapshot. If authorization is absent,
report the conflict and ask for it before mutating the branch. Do not triage
comments or checks against a conflicted head.

For pending CI, wait with `scripts/pr-await <PR>`. It blocks below the
conversation and prints one report when every check is terminal, so a wait of
any length costs a single round-trip. Do not re-run `scripts/pr-state
--summary` on a timer instead: each snapshot is a separate model turn that
re-reads the whole context, and waiting is only about 6% of what such a turn
actually does.

Default `--mode all-terminal` is the cost-correct choice and returns only when
no check is pending, so every failure arrives in one report and is fixed in one
pass. It is also the correct choice for a reason unrelated to cost: a failed job
can be visible while its parent workflow is still running, so an early return
reports a partial failure set. Use `--mode first-failure` only when the user
wants the first failure interactively; it costs an extra full CI cycle per
failure. Pass `--deadline-min` for a user-stated limit; on exit 2 report the
named pending checks as "CI in progress."

Read its exit code rather than re-deriving state: 0 clean, 1 terminal with
findings, 2 deadline with checks still pending, 3 blocked. Exit 3 covers every
case where a clean answer cannot be trusted: the PR merged or closed, a workflow
needs approval, access was lost, the merge-state query failed, the snapshot
reported `errors` or a null unresolved-thread count, or the host toolchain
cannot run `pr-state` correctly. Never downgrade an exit 3 to "probably clean."

Exit 1 counts blocking reviews, not only threads: a current-head
`CHANGES_REQUESTED` review can exist with no unresolved thread attached.

Every report records the jq and bash versions it ran under. That is provenance,
not a gate: `pr-state` used to fail silently on jq 1.6 and on the bash 3.2 that
macOS ships as `/bin/bash`, both of which made a dirty PR look clean, and both
are fixed at the source. What guards against a degraded `pr-state` now is the
snapshot itself, which is stronger than any version check: a summary reporting
`errors`, one whose unresolved-thread count is null, and one that does not parse
are all treated as unknown rather than clean. A blocked report on an old
toolchain adds a note naming it as a possible cause. A push during the wait
restarts the gate against the new head and is reported.

Do not use interactive `gh pr checks --watch` in the primary conversation: its
TTY redraws make captured output unusable. Use the read-only `pr-poller` only
when the user explicitly asked to wait or monitor and `pr-await` is unavailable.
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
checks, no merge conflict, no blocking review (an active `CHANGES_REQUESTED`
or a review blocked at the exact current head), no unresolved review thread, no
actionable issue comment, and qualifying exact-head semantic evidence where PR
delivery requires it.

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

An explicit request to run `pr fixup` owns every failed required check on the
current head. Never stop by calling a failure "unrelated" only because its
files are outside the PR diff. Reproduce each leaf failure with retries
disabled, inspect its artifacts and shared fixtures/cleanup, and fix every
valid product, test, fixture, cleanup, or CI-contract defect it exposes. A
dependent aggregate failure does not replace its leaf failures: trace the
aggregate to all failed jobs, fix the underlying failures, and wait for the
aggregate checks to rerun. If concrete evidence proves a failure is external
after this investigation, report the exact job/log/reproduction evidence and
keep the PR blocked; do not call it ready while a required check is failed.

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
can be truncated, so expand every listed review thread with
`scripts/pr-resolve show <PR> <thread_id>` before deciding how to handle it.
Use `scripts/pr-state --comment <comment_id>` only for a flat comment view when
a thread context is not available. Validate against the current head, the spec,
and existing architecture before editing or replying.

Every unresolved thread requires an explicit disposition, regardless of its
author, bot identity, visibility, or apparent severity. Record exactly one of
these dispositions:

- `actionable`: make the valid code or test change, verify it, cite the
  resulting commit in the reply, and resolve the thread when writes are
  authorized.
- `already addressed`: verify the current implementation or an existing commit,
  explain where it is addressed, and resolve the thread when writes are
  authorized.
- `informational` or `optional`: make no code change, acknowledge the information
  or suggestion in a reply, and resolve it when complete cleanup is requested.
- `invalid`: do not make an invalid code change or silently ignore the finding;
  reply with concrete reasoning grounded in the code, spec, or architecture,
  then resolve it only after that pushback is posted and writes are authorized.

A classification is not a disposition. A thread is not complete merely because
it was called optional, informational, already addressed, or invalid. GitHub
replies and thread resolution are external writes. A request such as "complete
cleanup", "clean up all review threads", or "leave no threads unresolved"
explicitly authorizes a concise reply and resolution for every unresolved
thread, including informational, optional, and invalid findings. A request to
address selected comments authorizes replies and resolution only for those
selected threads. A review-only request authorizes no writes. When writes are
not authorized, record every disposition, report the still-unresolved thread,
and do not declare the PR clean.

For code changes, push the fix and pass targeted verification before replying
and resolving. For non-code dispositions, verify the current head first, then
use the atomic helper path
`scripts/pr-resolve reply <PR> <comment_id> <thread_id> --body-file <path>` to
reply, resolve, and react in one operation when the body contains Markdown or
shell metacharacters. For short plain-text bodies, a safely quoted argument is
acceptable. Never interpolate review text into an unquoted shell command or
use backticks in the command itself. After the helper returns, re-fetch the
thread and verify the posted reply body and resolved state before treating the
disposition as complete.
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
skill/reference file, run the shared harness validation in
`.agents/skills/harness-improvement/references/validation.md` before pushing.

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
mandatory hidden-thread gate: expand and disposition each hidden thread, then
run `scripts/pr-resolve list <PR>` again after the refresh and immediately
before reporting.
Require `checks_head_sha` to match that head, report pending checks separately
from failures, and rerun `scripts/pr-resolve list <PR>` before declaring the
PR clean. The final predicate must also require
`hidden_unresolved_threads=[]`; do not require filtered and unresolved counts to
be equal because the filtered count includes resolved threads. Treat prior review
evidence as stale. When the user authorized thread writes, every unresolved
thread still needs an explicit reply and resolution matching its disposition.
A duplicate or stale bot thread needs the same treatment once current source
proves the finding is already fixed, including a thread surfaced only in
`hidden_unresolved_threads`; only current-head actionable threads drive code
changes. Declare the PR clean only when the
exact-current-head review classification reports no unaddressed findings,
`checks_snapshot_complete=true`, `failed_checks=[]`, `pending_checks=[]`,
`approval_required_runs=[]`, `actionable_issue_comment_count=0`,
`unresolved_review_thread_count=0`, `hidden_unresolved_threads=[]`, there is no
merge conflict, and
`scripts/pr-resolve list <PR>` is empty. Within
the user's monitoring limit, continue checking after resolutions until automated
review jobs are terminal; otherwise report the exact pending check names.

If the user explicitly requested a persistent Kandev plan update and the task
has an external Kandev plan, call `get_task_plan_kandev` before fixup and
`update_task_plan_kandev` after fixup with the remediation commit, final
exact-head check counts, resolved-thread state, and mergeability. Without that
authorization, report the plan update as pending and do not invoke Kandev task
or session APIs. Batch plan/task synchronization into the final documentation
commit. Record the prior head's fixup evidence before a plan commit/push: that
push restarts CI and invalidates the snapshot. For tracked `docs/plans/**`
artifacts, keep prose head-agnostic and record remediation scope/local
verification before that commit; then rerun `scripts/pr-state --summary` and
`scripts/pr-resolve list` for the new head and report its pending checks
separately. Mark prior current-head claims historical/superseded when a new
head replaces them; report only the latest head's SHA, CI/review counts, and
mergeability. Do not leave planned verification marked unstarted after it has run.

Before declaring fixup complete, verify `git status --short` is clean,
`git rev-parse HEAD` equals `git rev-parse @{upstream}`, the PR head equals
local `HEAD`, and the fresh mergeability state is not conflicting. Do not call
the PR clean from CI/review counts alone when the worktree or remote tip still
differs.

The phrase "ready to merge" is reserved for a fresh current-head snapshot
with every required check successful or explicitly skipped, no pending or
failed leaf or aggregate check, no unresolved review thread, no merge conflict,
and the local, upstream, and PR head OIDs aligned. A clean local reproduction
does not waive a failed remote check; push the remediation and re-check the
new head first.

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
- Do not silently dismiss an invalid finding; every invalid thread needs a
  concrete pushback disposition, and an authorized complete cleanup must post
  that explanation before resolving it.
- Do not proceed with an unverified PR when mandatory verification is blocked.
