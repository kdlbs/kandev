# PR Fixup CI Troubleshooting

Load this reference from `/pr-fixup` when a failing CI check is unfamiliar,
looks like infrastructure, or involves E2E.

## Narrow Unfamiliar Failures

If the failure looks unfamiliar or the cause is not obvious from the log, check
CI history on the branch before diving into code:

```bash
gh run list --branch <branch> --workflow "<workflow name>" --limit 10 --json conclusion,headSha,createdAt,databaseId
```

On long-lived PRs that get rebased or squashed, prior SHAs on the same branch
often passed the same workflow. A `passing -> failing` boundary tells you the
regression is isolated to the most recent rework. Diff against the last passing
SHA (`git diff <last-passing-sha>..HEAD`) instead of against `main`.

## Infrastructure Failures

**Failed job in an in-progress workflow:** GitHub can expose a failed job
before the parent workflow is terminal, and `gh run view --log-failed` can
temporarily report its logs as unavailable. Confirm the job's conclusion and
steps before reproducing or changing code, then retrieve the job log directly
when needed:

```bash
gh api repos/<owner>/<repo>/actions/jobs/<job_id> \
  --jq '{status, conclusion, steps: [.steps[] | {name, status, conclusion}]}'
scripts/pr-state --job-log <job_id>
```

Do not wait for the whole workflow only to obtain a log for a terminal job.
Inspect the job endpoint and fetch its job log directly when the aggregate log
command says logs are unavailable.

**Merge queue removals:** If a PR is removed from the merge queue, or its
ordinary checks are green while the queue reports a failure, inspect the issue
timeline for `added_to_merge_queue` and `removed_from_merge_queue` events. Then
query the queue runs directly; ordinary branch runs and `gh pr checks` omit
them:

```bash
gh api graphql \
  -f query='query($owner:String!,$repo:String!,$number:Int!) {
    repository(owner:$owner,name:$repo) {
      pullRequest(number:$number) {
        timelineItems(first:100, itemTypes:[ADDED_TO_MERGE_QUEUE_EVENT,REMOVED_FROM_MERGE_QUEUE_EVENT]) {
          nodes {
            __typename
            ... on AddedToMergeQueueEvent { createdAt beforeCommit { oid } }
            ... on RemovedFromMergeQueueEvent { createdAt reason beforeCommit { oid } }
          }
        }
      }
    }
  }' -f owner=<owner> -f repo=<repo> -F number=<PR> \
  --jq '.data.repository.pullRequest.timelineItems.nodes[] | {type:.__typename, createdAt, reason, before_commit:(.beforeCommit.oid // null)}'
gh run list --event merge_group
gh run list --commit <synthetic-queue-sha>
gh run view <run-id> --json jobs
```

Record each event's `createdAt`, removal `reason`, and `beforeCommit.oid`. Map
that OID to the corresponding `merge_group` run through commit ancestry,
identify the aggregate gate's failed step, and compare cancelled-job timestamps
with the workflow's `timeout-minutes`. Distinguish timeout/cancellation from a
test assertion before changing product code. A green PR-head snapshot does not
prove the merge-group gate passed.

**Synthetic `merge_group` failure:** When a merge-group run fails, record its
event, head SHA, leaf job or shard, and failed assertion. Compare the synthetic
tree and reported files with the PR head and authoritative base, then reproduce
the exact assertion on the PR head and on the parent or last-passing SHA with
retries disabled and repeated runs. Classify it as queue, base, or fixture
infrastructure only after independent evidence supports that conclusion; do not
make speculative source changes or push a stale head. Verify
`checks_head_sha` matches the PR head before acting, and stop if the PR merged
or closed.

**Isolated frontend assertion in an unchanged test:** Reproduce the exact
focused test locally. If it passes and has no relationship to the PR diff,
rerun only the failed workflow jobs once; classify it as transient only when
that rerun passes. A repeatable assertion remains a fixup failure even when the
test file is unchanged.

If a queued PR needs an authorized branch update, do not push blindly into the
queue. Record the PR head/base OIDs and queue event IDs, confirm its queue state,
dequeue it through the repository-approved path before changing the branch, then
verify the exact head and re-enqueue only after the push. Treat the new
`merge_group` SHA and run as fresh evidence; a base race or queue-state change
invalidates prior head checks.

Treat an unavailable log stream as unknown evidence, not a product failure.
If the direct job-log request returns 404 while the job is queued or in progress,
wait for that job to complete before retrying; it is not missing evidence.

**Malformed or truncated GitHub API responses:** If a workflow fails while
parsing `gh api` JSON, reports an unexpected end of input, or receives a
successful response with missing required fields, preserve the raw response and
HTTP status in a temporary file. Treat the result as unknown rather than as an
empty or clean payload. Inspect the workflow's expected response with a focused
contract test. For workflow-owned idempotent GET/PATCH requests, use bounded
short-backoff retries, validate that the GET body is complete JSON before
parsing, and silence PATCH output when the next step expects no response body.
Rerun the failed job once, and stop with the transport failure if the same
response problem repeats; do not add unbounded retries or change product code to
accommodate a truncated response.

### Static artifact byte mismatches

If an object upload and public GET both succeed but a strict `cmp` fails, save
and compare the origin artifact with the public body before changing the
publisher. A public HTML body containing `__cf_email__`, `/cdn-cgi/`, or
`email-decode.min.js` indicates Cloudflare edge transformation rather than a
failed upload. Preserve the byte-for-byte comparison and fix origin metadata
with `Cache-Control: no-transform`, or disable Email Obfuscation for the
dedicated hostname; verify a fresh upload and public body match exactly.

When the aggregate command exposes only a merge/report job, obtain every failed
shard `job_id` from `scripts/pr-state --summary <PR>` and inspect at least one
failure from each shard before changing code:

```bash
scripts/pr-state --job-log <job_id>
```

If that summary still names only the aggregate gate, enumerate the failed jobs
from the workflow and inspect the concrete shard (then load its test-results
artifact as described in `/e2e`):

```bash
gh run view <run-id> --json jobs \
  --jq '.jobs[] | select(.conclusion == "failure") | {name, databaseId}'
```

Verify the output names the actual failing spec or assertion, rather than only
the merge-report exit code.

**CodeQL code-scanning upload failure:** If CodeQL completes extraction and
query evaluation, then fails immediately after `Uploading code scanning
results` without a source finding or actionable log error, treat it as GitHub
code-scanning upload infrastructure. On the current head, rerun the failed job
once and re-check the PR state:

```bash
gh run rerun <run-id> --failed
scripts/pr-state --summary <PR>
```

If a newer head has already been pushed, rely on its newly triggered CodeQL
run. Report an unchanged upload failure rather than changing product code.

**Go module-proxy transport failure:** If a Go test job fails during package
setup while downloading from `proxy.golang.org` (for example, `stream error:
stream ID ... INTERNAL_ERROR`), and has no test assertion or compile failure,
treat it as proxy transport infrastructure. Rerun the failed job once, then
re-check the current head:

```bash
gh run rerun <run-id> --failed
scripts/pr-state --summary <PR>
```

Only investigate product code when the rerun exposes a repeatable test or build
failure.

**Third-party deployment fetch failure:** For a deployment action that cannot
fetch its third-party resource, compare adjacent successful runs for the same
branch/head and rerun the failed job once before changing workflow or product
code. Treat a one-off fetch failure as infrastructure unless it repeats with
an actionable configuration error.

**Merge-ref validation drift:** GitHub can run a PR check against the synthetic
merge ref, where a current-base deletion makes a cited path or coverage entry
appear missing even though it exists on the PR head. Compare the failing path
with `origin/<baseRefName>` before changing the PR. Whole-file limits can also
fail only on the synthetic merge when incoming base content makes the merged
file larger than the PR head. Compare the exact PR-head file with the
`refs/pull/<PR>/merge` version (for example through the Contents API), then
rerun the size/line check on a fresh merge result after any base merge. Update
a genuinely stale manifest or coverage entry, but do not rebase solely to
satisfy this check.

**Cancelled concurrency duplicates:** A required check with
`conclusion=cancelled`, 0s job durations, unexpanded `${{ matrix.* }}` job
names, or a "Canceling since a higher priority waiting request ..." annotation
is usually a superseded GitHub run, not a code failure. Confirm the
non-cancelled run for the same head SHA passed, then trigger one clean run
(rebase onto main + force-push, or `gh run rerun <id>`).

**Required job timeout cancellation:** If a required job is cancelled at its
configured `timeout-minutes`, inspect its concrete steps and logs. When the
race/test step has no assertion failure and one rerun repeats the same timeout,
classify it as CI infrastructure, stop rerunning, and report the exact check,
timeout, and pending-state evidence. If every E2E matrix shard's test step
remains `in_progress` past the configured timeout without assertion or log
progress, inspect the exact jobs, classify the matrix stall as runner
infrastructure, and cancel the run once. If GitHub rejects a rerun or leaves no
clean run, create a fresh pull-request event with a harmless CI-only commit
only when authorized; verify the new head and run ID, and do not loop reruns.

Before cancelling or rerunning a stale pending report, re-fetch the PR head,
parent run, and named job. Require each `head_sha` to match the current
`checks_head_sha`; if the job is already terminal-success, do nothing. When a
shard succeeds while its parent remains active, enumerate nonterminal jobs and
distinguish report/final-gate cleanup from a hung shard. Because cancellation is
run-scoped, cancel only after confirming the matching-head run is still stalled
and no active job or gate would be collateral damage. Verify with fresh
`scripts/pr-state --summary`, `scripts/pr-resolve list`, and mergeability.

**Rerun attempt semantics:** `gh run list` can retain a prior failed conclusion
while a later `run_attempt` of the same workflow is still in progress. Query the
REST run's `run_attempt`, `status`, `conclusion`, `head_sha`, and current jobs;
classify only the current attempt after its parent workflow is terminal.

**Privileged workflow provenance and bootstrap:** For an unexpected no-op or
old-behavior result in an `issue_comment` or `pull_request_target` workflow,
including a PR-produced cross-job URL or SHA that is normalized or truncated,
inspect the exact job log, event, workflow SHA, and base SHA. Compare the
producer output with the consumer environment and with the workflow at the
default branch, for example:

```bash
git show <workflow-sha>:.github/workflows/<file>
```

These privileged runs use the trusted default-branch workflow definition, so a
fix committed only on the PR head cannot affect the current run. Validate the
change with a focused contract test, rerun the failed job once, and if the log
still proves default-branch provenance, report the base-controlled blocker and
land the workflow fix through the authorized path before triggering a fresh
event. Do not keep patching or rerunning the PR branch to exercise a workflow
definition GitHub is not using.

**Semantic PR title transport failures:** If `pr-title` /
`amannn/action-semantic-pull-request` fails with transport or response parsing
errors such as `invalid json response body ... Unexpected end of JSON input`,
treat it as infrastructure. Confirm the PR title is valid Conventional
Commits, rerun once, then re-check:

```bash
gh run rerun <run-id> --failed
scripts/pr-state --summary <PR>
```

**Vitest runner/runtime crashes after passing suites:** If `Run Frontend Lint,
Tests, and Build` or another Vitest-based job logs all test suites passing and
then exits from a Node/V8 fatal crash such as `FATAL ERROR: v8::ToLocalChecked
Empty MaybeLocal` or `node::cjs_lexer::Parse`, rerun the failed job once:

```bash
gh run rerun <run-id> --failed
scripts/pr-state --summary <PR>
```

Only debug code if the rerun fails with an actual lint, test, or build error.

**E2E container setup failures:** If an E2E Containers shard fails during setup
before tests run, check for dependency or registry failures. Patterns such as
`packages.microsoft.com ... 403 Forbidden`, `docker/login-action@v3`,
`Error response from daemon: Get "https://ghcr.io/v2/"`, `ghcr.io/token`,
`docker buildx imagetools inspect`, `Could not resolve an immutable digest`,
`ghcr.io/kdlbs/kandev-ci:runtime-latest`, `context deadline exceeded`, or
`Client.Timeout exceeded while awaiting headers` are infrastructure/package-
registry issues, not app or test failures.
If GitHub rejects `gh run rerun <run-id> --failed` while the workflow is still
active, wait for the workflow/report job to finish and retry.

**Third-party action pnpm auto-install failures:** If an action detects pnpm
and fails with `ERR_PNPM_ADDING_TO_ROOT`, inspect the pinned action bundle and
its supported inputs before changing the repository package manager. Do not
switch a pnpm workspace with `workspace:*` dependencies to npm. Either
preinstall the action's pinned tool with
`pnpm add --workspace-root --save-dev --ignore-scripts <tool>@<version>`, or
run the action from an isolated npm working directory when the action supports
one. Reproduce the action's exact version-detection command locally before
pushing the workflow fix.

## Go Race-Suite Flakes

For a backend race-suite failure, extract the named failure from the saved log:

```bash
rg -n '"Action":"fail"|--- FAIL:|goleak:' /tmp/kandev-job.log
```

Reproduce that exact failure first, then exercise the affected package for
suite interaction or leak-cleanup timing:

```bash
go test -race ./path/to/package -run '^TestName$' -count=20
go test -race ./path/to/package -count=3
```

If a failed-job rerun reports a different package or test, validate that failure
with the same rigor even when it is outside the PR diff. Do not dismiss or rerun
past a valid race, leak, or product defect because it appears unrelated; fix it
and exercise the affected package without retry masking. Only an evidenced
external dependency or infrastructure fault is exempt from code remediation.

If a Backend Tests shard log names only `fail: github.com/.../package` without
the test name, download its matching `backend-test-results-<shard>` artifact:

```bash
gh run download <run-id> --name backend-test-results-<shard> --dir <temp>
rg -n '"Action":"fail"' <temp>
```

The artifact JSONL may start with `go: downloading ...` or other non-JSON
preamble lines; skip those lines when parsing so the recorded test name can be
reproduced exactly.

## E2E Failures

If any failing check is an E2E test:

1. Read the `/e2e` skill for debugging guidance, test patterns, and commands.
2. Identify the exact failing spec/test from logs before changing code.
3. Fix the root cause; never increase timeouts to hide flakes.
4. Run the exact failed spec/title locally before a full shard. CI logs hide
   in-DOM React render errors that often show up in
   `e2e/test-results/<test>/error-context.md`.

If a failed E2E action leaves the browser on a detail route, inspect the
rendered confirmation dialog and recent interaction-contract changes before
changing router code or increasing route waits. A new intentional confirmation
dialog is a navigation gate: update the test to assert and confirm the dialog
instead of altering navigation to bypass it.

Before attributing an E2E failure to product code, compare the failed run SHA
and `checks_head_sha` with local `HEAD`. PR workflows may test GitHub's
synthetic merge commit, so reproduce the exact base-plus-head merge (or checked
merge ref) with CI environment, retries disabled, and the same shard/order;
verify the checkout SHA before interpreting the result.

If a focused reproduction reports `No tests found`, a title mismatch, or a 404
for a workflow or API path, first verify the checked-out test title and compare
the PR with the authoritative current base: record `base_head_oid`, the merge
base, and `git diff <base>...HEAD`. An old branch can make an apparent product
failure disappear because the fix and updated test already landed on the base.
If the fix is already upstream, update the branch only with authorization and
invalidate all prior exact-head CI evidence.

When a shard is cancelled and aggregate E2E/report gates fail, treat the
aggregate failures as cascading until the concrete shard is inspected. Identify
the cancelled job and timeout, run its exact manifest-selected shard without
retry masking, then rerun the failed workflow jobs once. Classify it as
infrastructure only if the timeout repeats without an assertion failure.

Useful focused commands:

```bash
scripts/run-quiet build -- make build-backend build-web
scripts/run-quiet e2e -- bash -c 'cd apps && pnpm --filter @kandev/web e2e:raw -- tests/path/to/failing.spec.ts'
scripts/run-quiet e2e -- bash -c 'cd apps && pnpm --filter @kandev/web e2e:raw -- tests/path/to/failing.spec.ts -g "exact failing test title"'
```

An isolated pass does not invalidate a CI failure, and a spec outside the PR
diff remains part of fixup scope. Re-run it with retries disabled under CI
resource limits and preserve the failed shard's ordering. If that exposes a
race, stale state, or interaction leak, fix it and stress the smallest
reproducing sequence before running the full affected shard. Never use a
failed-job rerun as a substitute for explaining and fixing a valid test failure.

When a UI copy rename is intentional, search E2E specs for old visible text
before debugging deeper. Prefer updating assertions to the new label while
keeping stable routes unchanged when route compatibility is intentional.

For repeated failures, do not dismiss them as flaky. Compare per-shard runtime
against recent `main` runs and reproduce the exact failing spec locally. A
shard that is much slower on the PR than on `main`, or cancelled exactly at the
job timeout boundary, usually indicates real test failures plus retries.

For pending E2E matrix shards, inspect the workflow once for a compact list
instead of repeatedly dumping the full checks table:

```bash
gh run view <run-id> --json status,conclusion,jobs \
  --jq '{status, conclusion, remaining: [.jobs[] | select(.status != "completed" or .conclusion != "success") | {name, status, conclusion}]}'
```

For an explicit user requirement that an E2E run contain no flakes or retries,
run the deterministic blob audit after the exact-head E2E merge report
completes:

```bash
python3 scripts/playwright-blob-audit <downloaded-blob-report-dir>
```

It recursively reads `report-*.zip` and `report.jsonl` artifacts, reports
attempts, retry attempts, `onTestEnd` status counts, and results with errors,
and exits nonzero for retries, errors, parse failures, or unexpected statuses.
Do not make this extra artifact audit part of ordinary PR fixup; a normal green
aggregate check is sufficient unless the no-flakes requirement is explicit.
