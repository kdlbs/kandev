# PR Fixup Transport Troubleshooting

Load this reference when CI setup fails before tests, or when GraphQL evidence
is unavailable but authenticated REST calls still work.

**Corepack/pnpm bootstrap transport failures:** If `pnpm install --frozen-lockfile`
fails while Corepack downloads pnpm, before repository tests start, and the log
shows a Node/undici transport or parser assertion, classify it as runner or
registry infrastructure. Verify the exact job head and terminal parent run,
then rerun the failed job once with `gh run rerun <run-id> --failed`; recheck
`scripts/pr-state --summary` against that same head. If it repeats, inspect the
runtime image/Corepack toolchain instead of changing application code.

**GraphQL/rate-limit degradation:** If `pr-await` reports
`outcome: blocked-rate-limit`, stop its fixed-cadence loop and preserve the
reported status, `Retry-After`, or `X-RateLimit-Reset` detail. Wait until the
bounded retry time, or use the user's remaining monitoring window, before one
new waiter; never run parallel or manual polling. If `pr-await` or `pr-state`
cannot read GraphQL while REST remains available, query the REST pull request
for state, mergeability, and head SHA, then
`/commits/<head_sha>/check-runs?per_page=100` for checks. Require the REST head
to equal the check snapshot head. REST cannot prove review-thread resolution,
so keep thread evidence unknown until `scripts/pr-resolve` or a connector
succeeds; never call the PR clean from partial REST evidence. A queued job with
`runner_id=0`, no runner name, and no completed steps is hosted-runner backlog,
not a test hang; do not rerun or cancel solely because it is queued. Finish with
a fresh state, thread, and mergeability audit.

When authenticated `gh`/REST calls time out or are rate-limited but the GitHub
connector remains available, use its structured workflow-run and job queries to
inspect the current-head run and exact leaf jobs. Preserve the head-SHA match
and leaf-job evidence, and keep review-thread disposition unknown until the
resolver or connector provides it. Do not replace an unavailable review query
with a green workflow rollup.

When a trusted evaluator publishes its useful result only to
`GITHUB_STEP_SUMMARY`, `gh run view --log-failed` may contain only
`Process completed with exit code 1`. Fetch the exact job log with an
authenticated API request or `scripts/pr-state --job-log <job_id>`, then inspect
the workflow step metadata and summary-producing step. Reproduce the evaluator
locally against the exact PR file set before changing documentation. Treat a
transient API or evaluator failure as incomplete evidence until that
reproduction or a fresh exact-head run explains it.

If the summary-only PR documentation evaluator still has no useful detail,
inspect the trusted workflow revision's `.github/scripts/pr-docs.cjs`. When it
exports `GitHubClient` and `evaluatePullRequest`, verify the evaluation path
uses only read requests, then call them against the exact PR number and head
SHA to separate a coverage defect from a GitHub API failure:

```js
const { GitHubClient, evaluatePullRequest } = require('./.github/scripts/pr-docs.cjs');
const [owner, repo] = process.env.GITHUB_REPOSITORY.split('/');
const client = new GitHubClient({ owner, repo, token: process.env.GITHUB_TOKEN });
evaluatePullRequest({
  client,
  pullNumber: Number(process.env.PR_NUMBER),
  expectedHeadSha: process.env.EXPECTED_HEAD_SHA,
}).then(result => process.stdout.write(`${JSON.stringify(result, null, 2)}\n`));
```

Do not invoke `run()` for diagnosis: it may publish commit statuses and write a
workflow summary. Recheck the exact-head job after any single permitted rerun.

If a terminal workflow job fails only because the workflow's own GitHub API or
code-search request returned HTTP 429, with no product or test assertion and a
matching PR head, classify it as external infrastructure. Preserve the head,
job ID, and run attempt. Honor the precise delay in the job log (such as
`next_delay_ms` or `Retry-After`) plus a small margin; if the log says
`wait-budget-exhausted`, do not rerun before its stated cooldown. Then rerun
only that failed job once with `gh run rerun <run-id> --failed`. Confirm the
new attempt still targets the same head, then use
`scripts/pr-await --mode all-terminal` and `scripts/pr-state --summary` to
verify both the rerun job and aggregate at that head. If the post-cooldown
attempt still gets 429, report external rate limiting rather than editing
product code or retrying in a loop.
