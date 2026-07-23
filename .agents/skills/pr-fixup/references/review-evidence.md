# Poller Review-Evidence Mechanics

Load this reference only when the `pr-poller` is unavailable, its report is
incomplete or contradictory, or a planner needs to interpret raw
`scripts/pr-state` output while resolving a PR-state incident. It does not
authorize the planner or a remediation worker to replace the registered
`pr-poller` during normal operation.

Request runtime network approval before the first GitHub helper call; denial,
cancellation, or interruption stops the workflow. Retry transient failures from
approved commands only within the bounded polling cadence.

Use `scripts/pr-state --summary <PR>` for CI/review state and
`scripts/pr-resolve list <PR>` for review-thread state. `scripts/pr-state`
accepts flags before or after the PR; when parsing with `jq`, save JSON to a
temp file first so stderr does not corrupt the pipe. Default state is limited to
items after the latest head commit; use `--summary --all` only for a deliberate
historical audit.

The summary fields are:

- `failed_checks` and `pending_checks`: actionable check state and run/job URLs.
- `unresolved_review_thread_count`: all unresolved threads; use
  `hidden_unresolved_threads` as blockers even outside the current-head filter.
- `review_evidence`, `checks_head_sha`, and `checks_snapshot_complete`: exact
  head evidence. A head race or incomplete fetch is unknown, never inferred
  from timestamps.
- `errors`: affected data is unknown; do not reconstruct it from memory.

If head metadata fails but check/thread data is usable, use that data only for
the current poll and retry once at the next cadence. If review-thread state is
unknown, do not call review clean/blocked; retry once, then use
`scripts/pr-resolve list <PR>` before a final status. If total unresolved count
is nonzero while visible threads are empty, fetch the authoritative thread list
and full bodies with `scripts/pr-state --comment <comment_id>` or
`scripts/pr-resolve show <PR> <THREAD_ID_OR_COMMENT_ID>`.

If `branch:"unknown"` or PR-view resolution is transient, retry the explicit
PR-number command once before using direct targeted GitHub fallback. Do not
discard valid check/thread data merely because `since`/repository metadata
failed; however, an unknown review-thread count is never clean evidence.

If `pr-state` hits an argument-list limit, fall back to:

```bash
gh pr checks <PR>
scripts/pr-resolve list <PR>
```

`gh pr checks` covers CI only and can return nonzero while still printing usable
pending/failing rows. Keep only actionable rows and retain `pr-resolve` for
threads:

```bash
gh pr checks <PR> 2>/dev/null | awk -F '\t' '$2 == "pending" || $2 == "fail" {print}'
scripts/pr-resolve list <PR>
```

When `hidden_unresolved_threads` exists or total unresolved count exceeds the
visible list, fetch each body with `scripts/pr-state --comment <comment_id>` or
`scripts/pr-resolve show <PR> <THREAD_ID_OR_COMMENT_ID>`. A listed thread that
is already resolved is stale summary state: re-poll and do not reply again.

Poll at 30-second cadence with a 20-minute cap using bounded one-shot commands;
avoid long inline loops and `gh pr checks --watch`. Stop early on a required
failure. Queued/in-progress jobs are pending, not speculative-fix triggers. On
an explicit wait-through-CI request, repeat bounded checks until failures,
pending checks, and unresolved-thread count are all empty/zero.

For E2E-only pending work, summarize a saved snapshot before printing shards:

```bash
scripts/pr-state --summary <PR> > /tmp/prstate-<PR>.json
jq '{failed_checks, pending_count:(.pending_checks|length), unresolved_review_thread_count, errors}' /tmp/prstate-<PR>.json
jq -r '.pending_checks[] | "\(.status) | \(.name)"' /tmp/prstate-<PR>.json
```

If a manual poll is interrupted, terminate only polling processes you started.
Use raw `scripts/pr-state <PR>` only for an odd-state diagnostic.
