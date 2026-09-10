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

The default deadline is 45 minutes and the default cadence is 60 seconds.
Pass an explicit deadline for a user-specified limit. Use `--interval-sec <S>`
when the user specifies a cadence. Pending matrix counts can grow as jobs appear.

## Interpret the final result

Read the final tool result's `exit_code`, including for PTY/session commands.

| Exit | Meaning | Action |
| --- | --- | --- |
| 0 | Terminal CI/review counts are clean | Refresh and classify review bodies before delivery. |
| 1 | Terminal findings, conflicts, or base drift | Triage the reported findings. |
| 2 | Pending checks or an unconfirmed terminal rollup at the deadline | Report the pending work at the user's limit. Otherwise continue waiting. |
| 3 | Closed PR or unavailable/blocked evidence | Read the stated reason. Never infer a clean result. |

If exit 1 reports only base drift, use the advanced-base procedure in
[merge-conflicts.md](merge-conflicts.md). Record its separate validation result.
The helper's exit code remains 1 even when that validation succeeds.

If no user limit prevents further waiting, rerun after exit 2 with a larger
deadline. Do not replace the waiter with timer-driven snapshots.
An `all-terminal` result can arrive before the deadline. Report actual elapsed time.
Strict-deadline returns 0 or 1 for confirmed terminal evidence, 2 for pending or
unconfirmed work, and 3 for blocked evidence.

Exit 3 includes approval-required runs, access errors, unknown mergeability,
incomplete snapshots, and unknown required-check policy. The reported toolchain
versions are diagnostic context, not substitute evidence.

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

Use the read-only `pr-poller` only when the user explicitly requests monitoring
and `pr-await` is unavailable. Give it the user's deadline, or a 20-minute cap
when no limit exists. Treat its output as provisional and refresh before acting.
Do not use interactive `gh pr checks --watch` in the primary conversation.
