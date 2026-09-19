# Waiting for PR checks

Load this reference when checks are pending or a user requests PR monitoring.
Use `scripts/pr-await <PR>` for the wait. It owns the polling loop and returns
one report. Do not run `pr-state` on a timer in the primary conversation.

## Select the wait mode

- Use `--mode all-terminal` by default. It waits for parent workflows and all
  registered checks to finish before reporting the complete failure set.
- For an explicit fixed-duration hold, use
  `--mode strict-deadline --deadline-min <N>`. It continues after terminal CI
  until the deadline, unless the PR closes or evidence becomes blocked.
- For an upper time limit that permits an early result, use
  `--mode all-terminal --deadline-min <N>`.
- Use `--mode first-failure` only when the user requests the first failure.

Distinguish a post-creation hold from check monitoring. If the user asks to
wait N minutes after creating the PR before starting fixup, record the PR
creation time, wait in bounded chunks of 60 seconds or less with progress
updates, and invoke `scripts/pr-await` only after that hold expires. Use
`strict-deadline` when the user asks the waiter to monitor checks during the
interval; do not use it to represent an unmonitored delay.

The default deadline is 45 minutes and the default cadence is 60 seconds.
Pass an explicit deadline for a user-specified limit. Use `--interval-sec <S>`
when the user specifies a cadence. Pending matrix counts can grow as jobs appear.

Before starting the waiter, validate the GitHub credential with `gh auth status`.
If it is stale or invalid, try an explicit `GH_TOKEN="$(gh auth token)"` prefix
for the helper or use the structured connector fallback. Classify REST 401/403
and rate-limit responses as authentication or transport blockage, not CI
failures. A read-only status rollup can show current-head checks while required
policy and review evidence remain unknown; it cannot prove the PR clean.

## Interpret the final result

Read the final tool result's `exit_code`, including for PTY/session commands.

| Exit | Meaning | Action |
| --- | --- | --- |
| 0 | Terminal CI/review counts are clean | Refresh and classify review bodies before delivery. |
| 1 | Terminal findings, review threads, conflicts, or base drift | Triage the reported findings. |
| 2 | Pending checks or an unconfirmed terminal rollup at the deadline | Report the pending work at the user's limit. Otherwise continue waiting. |
| 3 | Closed PR or unavailable/blocked evidence | Read the stated reason. Never infer a clean result. |

If exit 1 reports only base drift, use the advanced-base procedure in
[merge-conflicts.md](merge-conflicts.md). Record its separate validation result.
The helper's exit code remains 1 even when that validation succeeds.
An `all-terminal` exit 1 can therefore show every check green, such as 59
passed with 0 failed and 0 pending, when only the base relationship is stale.
Treat that as a merge-result validation finding, not a CI failure; preserve the
counts and validate against the live base before deciding what to change.

Exit 1 can be a review-only blocker: if `failed_checks` and `pending_checks`
are empty while unresolved review threads remain, proceed to thread disposition
instead of CI remediation. After every waiter result, run both
`scripts/pr-state --summary <PR>` and `scripts/pr-resolve list <PR>`; do not
call the PR clean until the thread list is also clear.

Exit 2 can report zero pending and failed checks when the terminal rollup is
still unconfirmed. Do not classify the wait as clean from those counts. Refresh
`scripts/pr-state --summary <PR>`, `scripts/pr-resolve list <PR>`, and PR
mergeability, then require the exact head, `checks_snapshot_complete=true`,
empty failed and pending lists, no unresolved or hidden review threads, and
`MERGEABLE`/`CLEAN` before calling it clean.

Apply the same rule after `gh run rerun --failed`: a provisional terminal
rollup may show zero failed and pending checks before the rerun evidence is
confirmed. Poll once more through `scripts/pr-await`, then require a fresh
matching-head `scripts/pr-state --summary <PR>` snapshot before triage or
reporting completion.

The structured report can also say `outcome: deadline` with zero failures while
E2E jobs remain pending. Treat that outcome as non-terminal, refresh PR state,
and report the pending checks separately; a deadline is not a green result.

If no user limit prevents further waiting, rerun after exit 2 with a larger
deadline. Do not replace the waiter with timer-driven snapshots.
An `all-terminal` result can arrive before the deadline. Report actual elapsed time.
Strict-deadline returns 0 or 1 for confirmed terminal evidence, 2 for pending or
unconfirmed work, and 3 for blocked evidence.

Exit 3 includes approval-required runs, access errors, unknown mergeability,
incomplete snapshots, and unknown required-check policy. The reported toolchain
versions are diagnostic context, not substitute evidence.
If a transient `pr-state` or GitHub API timeout causes exit 3 or populates
`errors` while checks are still progressing, treat the result as unavailable
evidence. Rerun `scripts/pr-await` after transport recovers and require a
terminal snapshot whose check SHA matches the current PR head before acting.
If the REST fallback is also rate-limited or returns a transient 403/5xx, use
the authenticated GitHub connector for one bounded exact-head snapshot, then
refresh state; keep checks or review evidence blocked while any required data
remains unknown.
When the report says `blocked-required-statuses` or `INCOMPLETE EVIDENCE`, use
that connector to fetch the current PR, exact-head workflow runs, compare or
merge-base, and review threads. Recovered context does not prove clean: keep
the fixup blocked while required-status policy or review evidence is unknown.

## Refresh after waiting

Run `scripts/pr-state --summary <PR>` and `scripts/pr-resolve list <PR>` after
each report. Require the check SHA to match the fresh PR head.
If delayed checks appear, restart the waiter. A sparse early rollup cannot
prove completion. The helper requires two matching terminal snapshots.

Inspect every current-head review body, including aggregate bot reviews.
If earlier snapshots contained top-level findings absent from the new head's
review list, run one `scripts/pr-state --summary --all` audit.
Revalidate those findings against current source.

If a job exceeds its configured timeout or contradicts the rollup, query its
exact job/run before diagnosing a hang. Verify that its SHA matches the PR head.
For fork workflow approval, inspect `approval_required_runs`.
An authorized fixup can approve the exact run through
`gh api --method POST repos/<owner>/<repo>/actions/runs/<run-id>/approve`.
Then refresh state and wait for jobs to appear. `gh run approve` is invalid.

If the PR becomes `MERGED` or `CLOSED`, stop. For a merge, report `mergedAt`
and `mergeCommit.oid`. Do not recreate, update, or re-enqueue its stale branch.
For synthetic merge-group checks, use [merge-queue.md](merge-queue.md).

## Interrupted or unavailable waiters

Retain the session handle until the command ends. An interrupted waiter has no
verdict. If the handle is lost, inspect and stop only its owned process tree
before starting a replacement. Never launch duplicate monitors.

Use the read-only `pr-poller` when the user explicitly requests monitoring and
`pr-await` is unavailable or exits 3 with a transient
`blocked-required-statuses` policy lookup failure. Give it the user's deadline,
or a 20-minute cap when no limit exists. Preserve exact-head matching and treat
the result as provisional or blocked while required-status policy or review
thread evidence is unknown; never report the PR clean from that fallback alone.
Do not use interactive `gh pr checks --watch` in the primary conversation.
