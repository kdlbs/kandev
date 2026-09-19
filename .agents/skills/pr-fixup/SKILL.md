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

Before the first GitHub call, obtain any network approval required by the
runtime. If the runtime denies access, stop until the user authorizes access.

Run `scripts/pr-state --summary <PR>` and `scripts/pr-resolve list <PR>`.
Load [review-evidence.md](references/review-evidence.md) for snapshot fields,
review classification, hidden threads, and access fallbacks.
For cross-repository PRs, use the snapshot's delivery fields as the push target.

Then run `gh pr view <PR> --json state,baseRefName,headRefOid,mergeable,mergeStateStatus,reviewDecision`.
Require matching head SHAs before triage. Verify the base SHA against the
current base-ref tip. Refresh mismatched metadata before attributing failures.
If the PR is merged or closed, stop. For a merge, report
`mergedAt` and `mergeCommit.oid`.

Treat peer alerts as advisory until their SHA matches the current head.
If captured output is empty, truncated, or invalid, rerun through `rtk proxy`.
Capture stdout and stderr separately and preserve the exit code.
An empty capture cannot prove that the PR is clean.

If GitHub reports a conflict, load
[merge-conflicts.md](references/merge-conflicts.md) before CI or review triage.
An explicit fixup request authorizes conflict resolution within that request.
Otherwise, report the conflict without changing the branch.

If the base advanced without conflicts, use that reference's advanced-base
procedure. Base advancement alone does not require a branch update.
Record the head, base, synthetic merge, and focused test results.
If either input SHA changes, repeat the relevant validation.
Keep the helper's exit code unchanged and report any separately verified base drift.

For related or stacked PRs, use the semantic-conflict procedure in the same
reference. Reconcile changed contracts and stable IDs before delivery.

For pending checks, load [waiting.md](references/waiting.md).
Use `scripts/pr-await` and its final exit code.
For queue membership or enqueue/dequeue requests, load
[merge-queue.md](references/merge-queue.md).
For queue removals or failed synthetic checks, also load
[ci-troubleshooting.md](references/ci-troubleshooting.md).
Ordinary PR-head checks do not validate a merge-group commit.

Directly supplied review findings need current-source validation even without
a GitHub thread. Apply valid fixes within the user's authorization.
Do not invent comment IDs or resolutions for findings without a thread.
External replies and thread resolution still require user authorization.

If `reviewDecision=REVIEW_REQUIRED` and `mergeStateStatus=BLOCKED`, report the
human approval gate. Green checks do not authorize self-approval or merging.

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
If the installed `gh pr view --json` does not expose the base OID, use
`BASE_SHA=$(gh api repos/<owner>/<repo>/pulls/<PR> --jq .base.sha)` alongside
`gh pr view <PR> --json baseRefName,headRefOid` for the ref names. Run this
fallback under `set -euo pipefail`, require a non-empty `BASE_SHA`, verify the
fetched base tip separately, and only then run
`git merge-tree --write-tree <base-remote>/<base-ref> HEAD`.

For unfamiliar, infrastructure, or E2E failures, load
`references/ci-troubleshooting.md` and, for transport or queue evidence,
`references/transport-troubleshooting.md` before changing code.
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
behavior or duplicate a larger sibling change; for request-count/dedup assertions covering multiple hydration or effect continuations, use a deferred response, assert the count while it remains pending, then resolve and drain it before unmount/return so an immediately resolved mock cannot let cleanup timers create a false duplicate.
When a remediation changes a documented behavior or contract, update the
authoritative spec/guidance, plan, and task file when present; refresh each
Verification/Results section with exact post-fixup commands and outcomes; commit
docs and code together, keep regression tests and verification commands aligned,
and re-check the documentation before completion; record why no update is needed when the behavior remains internal.

## 3. Triage And Address Reviews

Use `scripts/pr-resolve list <PR>` to obtain unresolved threads. Before handling multiple
threads, make a thread-to-finding map and one body file per thread. Its previews can be
truncated, so run `scripts/pr-resolve show <PR> <thread_id>` immediately before each
write; verify comment/thread IDs and that the body names that thread's finding, file, and
commit. Use `scripts/pr-state --comment <comment_id>` only for a flat comment view when no
thread context is available. Validate against the current head, spec, and architecture before editing or replying.

Every unresolved thread requires an explicit disposition, regardless of its
author, bot identity, visibility, or apparent severity. Record exactly one of
these dispositions:

When an automated review identifies a security-boundary risk and the user
explicitly chooses the behavior, record the accepted risk, trust/revocation
responsibility, and targeted regression coverage in the final response, PR body,
and authoritative spec. Do not silently encode the compromise.

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
After every push, run fresh `gh pr view <PR> --json baseRefName,headRefOid,mergeable,mergeStateStatus`
(or equivalent) at the pushed head; if GitHub briefly returns an older head, retry
the query and exact-head `pr-state` with short bounded backoff before triaging or reporting.
Require local `HEAD`, `headRefOid`, and `checks_head_sha` to match; confirm
`mergeable` is not `CONFLICTING` and `mergeStateStatus` is not `DIRTY`; `scripts/pr-state --summary` does not include mergeability.

If a remediation changes rendered UI, invalidate screenshots captured before
fixup and recapture and re-publish every affected viewport after the final
commit. Never leave pre-fixup screenshots in the PR.

Immediately before a remediation commit or push—and again after long-running
remediation—refresh PR state. Require the PR to remain open and its head ref to
match the local branch. Before a push, compare the remote head OID with the
local upstream tip; after the push, require the PR head OID to equal local
`HEAD`. If the PR merged or closed, do not recreate its deleted branch with a
stale push: preserve the local fix and ask before creating a clean follow-up.

After any rebase or force-push, fetch the PR base and compare local `HEAD`, the upstream tip, and `pr.head_ref_oid`; rerun affected checks.
A merge-commit head requires `--rebase-merges` or a verified merge-only delta;
after long hooks/tests, compare the latest authoritative base with the rebase
base and reconcile if changed; otherwise a rebase invalidates prior evidence:
rerun affected checks, `scripts/pr-resolve list <PR>`, and `scripts/pr-state --summary <PR>`; use `--force-with-lease`, never an unconditional force-push.
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
reports as resolved unless an authorized later code/policy change invalidates
the rationale for that resolution. In that exception, re-fetch/show the thread,
explicitly reopen it with `scripts/pr-resolve reopen <PR> <thread_id>`, then use
the reply helper to reply/resolve/react and verify the final thread. Before an
ordinary reply to a pre-push thread, run `scripts/pr-resolve show <PR>
<thread_id>`; if it reports `resolved: true` (often with an `Addressed in commit
...` marker), record the thread as auto-resolved and do not post a duplicate
reply. Continue replying/resolving only for `resolved: false` threads, including
hidden unresolved threads.
If a finding cites an obsolete commit, inspect the cited lines at the current PR
head before editing. Apply only the missing portion, and do not duplicate an
assertion that is already present or reply to a resolved or stale thread.
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
`unresolved_review_thread_count=0`, `hidden_unresolved_threads=[]`,
there is no merge conflict, and `scripts/pr-resolve list <PR>` is empty.
Require either `base_advanced_since_head=false` or recorded merge-result
validation for the current head and current base, as defined in
`references/merge-conflicts.md`. An unknown base relationship is not sufficient.
Within
the user's monitoring limit, continue checking after resolutions until automated
review jobs are terminal; otherwise report the exact pending check names.

When remediation changes tests or validation, reconcile any validation commands
or counts claimed in the live PR description with the final verification before
declaring fixup complete. Reuse `/pr`'s live-body preservation and REST-fallback
procedure, preserve intervening bot or maintainer text, and re-fetch exact-head
state afterward; a body PATCH triggers workflows, so restart `pr-state`/`pr-await`
before treating pre-PATCH CI as current.

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
`scripts/pr-resolve list` for the new head and report pending checks separately; keep
exact-head verification/queue work externally pending, not complete, until pending
checks are empty. Mark prior current-head claims historical/superseded when a new
head replaces them; report only the latest head's SHA, CI/review counts, and mergeability; do not leave planned verification marked unstarted after it has run.

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

When branch protection requires a merge queue, after the exact current head is
clean and all checks pass, run `gh pr merge <PR> --auto
--match-head-commit <SHA>` without a merge strategy. In this mode the command
requests queue admission, not a direct merge. Verify the
`added_to_merge_queue` timeline event and
`pullRequest.mergeQueueEntry { id state position estimatedTimeToMerge headCommit { oid } }` with GraphQL; treat "already queued" as verification, not a retry.
Compare the expected SHA with `pullRequest.headRefOid`, not the queue commit; keep the PR open while queue checks run. Do not rely on a null `autoMergeRequest` or
`mergeStateStatus=UNKNOWN` to determine queue status.

For "monitor until merge", poll queue/PR state with bounded cadence; require
`MERGED`, `mergedAt`, `mergeCommit.oid`, and `isInMergeQueue=false`.
`QUEUED`/`AWAITING_CHECKS` and queued `CLEAN` remain pending; inspect
`references/merge-queue.md` for synthetic checks before reporting a blocker.

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
