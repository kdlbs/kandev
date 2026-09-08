# PR Review-Evidence Mechanics

Load this reference when `scripts/pr-state --summary` is incomplete or
contradictory, or when the primary conversation needs to interpret raw
`scripts/pr-state` output while resolving a PR-state incident.

Use `scripts/pr-state --summary <PR>` for CI/review state and
`scripts/pr-resolve list <PR>` for review-thread state. `scripts/pr-state`
accepts flags before or after the PR; when parsing with `jq`, save JSON to a
temp file first so stderr does not corrupt the pipe. Default state is limited to
items after the latest head commit; use `--summary --all` only for a deliberate
historical audit.

The summary fields are:

- `failed_checks` and `pending_checks`: actionable check state and run/job URLs.
- `unresolved_review_thread_count`: all unresolved threads, regardless of
  classification; every one is a completion gate. Use
  `hidden_unresolved_threads` as blockers even outside the current-head filter.
- `review_evidence`, `checks_head_sha`, and `checks_snapshot_complete`: exact
  head evidence. A head race or incomplete fetch is unknown, never inferred
  from timestamps.
- `errors`: affected data is unknown; do not reconstruct it from memory.

Record `checks_head_sha`, `checks_snapshot_complete`, `failed_checks`,
`pending_checks`, review counts, and the PR delivery fields
(`pr.head_repository_owner`, `pr.head_repository_name`, `pr.head_ref_name`,
`pr.head_ref_oid`, and `pr.maintainer_can_modify`). For a cross-repository PR, those
delivery fields are the only authoritative push target. A named-reviewer
shortcut requires `review_evidence.trusted_producer="true"` and is not valid
for forks, security, or architecture.

Inspect every non-empty body in
`review_evidence.exact_current_head_reviews[]`, including `COMMENTED`
aggregate-bot reviews, and classify it as actionable, already addressed,
informational, optional, or invalid before declaring reviews clear.
`trusted_producer=true` qualifies only for the dedicated OpenCode App, never
merely because a reviewer name matches.

For review threads, classification and disposition are separate. Expand every
unresolved thread with `scripts/pr-resolve show <PR> <THREAD_ID>` and record one
of these outcomes: implement and verify an actionable finding; explain where an
already-addressed, duplicate, or stale finding is handled; acknowledge an
informational or optional suggestion; or give concrete code/spec/architecture
reasoning for an invalid finding. Do not treat a label, internal note, or lack
of code change as a completed disposition.

When the user requests complete cleanup, including wording such as "clean up
all review threads" or "leave no threads unresolved", reply to and resolve
every unresolved thread after its disposition. This includes informational and
optional threads, which need an acknowledgement, and invalid threads, which
need the concrete pushback reply before resolution. A request limited to
selected actionable comments does not authorize writes to other threads. If
thread writes are not authorized, report each disposition and keep the thread
unresolved; never report the review state as clean. An invalid finding must
never be silently ignored.

If `gh`, `scripts/pr-state`, or `scripts/pr-resolve` fails with an
authentication or transport error (including a broker 401), do not treat empty
or unknown output as clean. If GitHub connector tools are available, use their
equivalents of `github_fetch_pr`,
`github_list_pull_request_review_threads` (including `is_resolved` and
`is_outdated`), `github_fetch_pr_comments`,
`github_fetch_commit_workflow_runs`, and
`github_get_commit_combined_status`. Gather PR metadata, review threads and
discussion comments, and commit workflow/status evidence. Keep SSH Git
operations available for fetch, rebase, and push; after a push, require the
connector-reported PR head OID to equal local `HEAD`. If the fallback cannot
provide CI or review evidence, report it as unknown or pending, never clean.

For connector-backed review writes, prefer structured workflow results and
`github_list_pull_request_review_threads`; do not request full PR HTML or diffs.
Map a GraphQL thread ID to its REST numeric top-level comment ID with
`github_fetch_pr_comments` before replying, but resolve using the GraphQL thread
ID. After every push and after automated review aggregation, refresh current-head
checks and threads.

If head metadata fails but check/thread data is usable, use that data only for
the current poll and retry once at the next cadence. If review-thread state is
unknown, do not call review clean/blocked; retry once, then use
`scripts/pr-resolve list <PR>` before a final status. If total unresolved count
is nonzero while visible threads are empty, fetch the authoritative thread list
and full bodies with `scripts/pr-resolve show <PR> <THREAD_ID>`; use
`scripts/pr-state --comment <comment_id>` only when a flat comment view is all
that is available.

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
checks_file=/tmp/pr-checks-<PR>.txt
if gh pr checks <PR> >"$checks_file" 2>"${checks_file}.err"; then status=0; else status=$?; fi
printf 'gh pr checks exit=%s\n' "$status"
cat "$checks_file"
test -s "${checks_file}.err" && cat "${checks_file}.err" >&2
awk -F '\t' '$2 == "pending" || $2 == "fail" {print}' "$checks_file"
scripts/pr-resolve list <PR>
```

Transport/collection failure leaves checks unknown. Parseable pending/failing
rows remain usable when `gh pr checks` exits 8; never hide diagnostics in a pipe.

When `hidden_unresolved_threads` is non-empty, fetch and disposition each thread
body with `scripts/pr-resolve show <PR> <THREAD_ID>`; use the flat comment command
only for a comment without thread context. A listed thread that is already
resolved is stale summary state: re-poll and do not reply again.

Poll at 30-second cadence with a 20-minute cap using bounded one-shot commands;
avoid long inline loops and `gh pr checks --watch`. In default monitoring,
stop early on a required failure. For an explicit fixed-duration request, use
strict-deadline mode: accumulate failures and comments until the absolute
deadline, stopping early only if the PR is merged/closed or access is revoked.
Queued/in-progress jobs are pending, not speculative-fix triggers. On an
explicit wait-through-CI request without a fixed deadline, use the same
20-minute absolute cap: repeat bounded checks until failures, pending checks,
and unresolved-thread count are all empty/zero, or stop at the deadline and
report remaining pending checks or unresolved threads. A nonzero unresolved
count is a blocker even when every remaining thread is informational, optional,
or invalid. Preserve early stopping
for failures and merged/closed or access-revoked conditions.

For E2E-only pending work, summarize a saved snapshot before printing shards:

```bash
scripts/pr-state --summary <PR> > /tmp/prstate-<PR>.json
jq '{failed_checks, pending_count:(.pending_checks|length), unresolved_review_thread_count, errors}' /tmp/prstate-<PR>.json
jq -r '.pending_checks[] | "\(.status) | \(.name)"' /tmp/prstate-<PR>.json
```

If a manual poll is interrupted, terminate only polling processes you started.
Use raw `scripts/pr-state <PR>` only for an odd-state diagnostic.
