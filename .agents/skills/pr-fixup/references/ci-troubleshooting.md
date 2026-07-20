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

**Cancelled concurrency duplicates:** A required check with
`conclusion=cancelled`, 0s job durations, unexpanded `${{ matrix.* }}` job
names, or a "Canceling since a higher priority waiting request ..." annotation
is usually a superseded GitHub run, not a code failure. Confirm the
non-cancelled run for the same head SHA passed, then trigger one clean run
(rebase onto main + force-push, or `gh run rerun <id>`).

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

If a failed-job rerun reports a different, unrelated package or test, validate
that second failure the same way and allow one additional failed-job rerun
instead of changing unrelated PR code. Stop and fix code when the same failure
reproduces locally or repeats in CI.

## E2E Failures

If any failing check is an E2E test:

1. Read the `/e2e` skill for debugging guidance, test patterns, and commands.
2. Identify the exact failing spec/test from logs before changing code.
3. Fix the root cause; never increase timeouts to hide flakes.
4. Run the exact failed spec/title locally before a full shard. CI logs hide
   in-DOM React render errors that often show up in
   `e2e/test-results/<test>/error-context.md`.

Useful focused commands:

```bash
scripts/run-quiet build -- make build-backend build-web
scripts/run-quiet e2e -- bash -c 'cd apps && pnpm --filter @kandev/web e2e -- tests/path/to/failing.spec.ts'
scripts/run-quiet e2e -- bash -c 'cd apps && pnpm --filter @kandev/web e2e -- tests/path/to/failing.spec.ts -g "exact failing test title"'
```

If the failed E2E spec is unrelated to the PR diff, the exact failing test
passes locally via `pnpm e2e:run`, and there are no unresolved review threads
or other failed checks, rerun the failed GitHub job once and poll until terminal:

```bash
gh run rerun <run-id> --failed
scripts/pr-state --summary <PR>
```

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
