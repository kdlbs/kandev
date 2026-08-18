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

Treat an unavailable log stream as unknown evidence, not a product failure.
If the direct job-log request returns 404 while the job is queued or in progress,
wait for that job to complete before retrying; it is not missing evidence.
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
with `origin/<baseRefName>` before changing the PR. Update a genuinely stale
manifest or coverage entry, but do not rebase solely to satisfy this check.

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
timeout, and pending-state evidence.

**Manual-review event provenance:** For an unexpected zero-duration or no-op
manual bot-mention run, inspect the workflow at the repository default branch
and the event payload before assuming the PR is behind `main`. Privileged
`issue_comment` and `pull_request_target` runs use the trusted default-branch
workflow definition, not a newly synced PR workflow. Compare the run's
workflow path, event, and head metadata with that definition; land a workflow
fix on the default branch, then reproduce with a fresh comment.

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
`context deadline exceeded`, or `Client.Timeout exceeded while awaiting
headers` are infrastructure/package-registry issues, not app or test failures.
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

Before attributing an E2E failure to product code, compare the failed run SHA
and `checks_head_sha` with local `HEAD`. PR workflows may test GitHub's
synthetic merge commit, so reproduce the exact base-plus-head merge (or checked
merge ref) with CI environment, retries disabled, and the same shard/order;
verify the checkout SHA before interpreting the result.

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
