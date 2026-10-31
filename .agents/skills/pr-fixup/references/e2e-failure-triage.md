# PR Fixup E2E Failure Triage

Load this reference when an E2E check fails during PR fixup.

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

A cancelled job is not selected by `gh run rerun --failed`. Get its job
`databaseId`, then rerun it without a positional run ID:

```bash
gh run view <run-id> --json jobs --jq '.jobs[] | {name, databaseId}'
gh run rerun --job <database-id>
```

Do not use the browser URL's job number. Verify the new attempt uses the same
head, then let the aggregate report job rerun.

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

For an explicit E2E quality requirement, download the exact-head retry summary
and matching blob reports:

```bash
gh run download <run-id> --pattern 'blob-report-*' --dir <tmp>
gh run download <run-id> --name e2e-retry-summary --dir <tmp>
python3 scripts/playwright-blob-audit <tmp>
jq '{counts, flake, flaky_or_retried: [.tests[] | select(.outcome == "flaky" or .attempts > 1) | {key, attempts, statuses, outcome}]}' \
  <tmp>/retry-summary.json
```

The blob audit reports retries, errors, parse failures, and unexpected statuses.
Use `flake.flaky` and per-test `outcome` for flaky verdicts; `attempts > 1` alone
does not prove a flake. For “no flakes,” require zero flaky verdicts; for “no
retries,” require one attempt per test. Missing or invalid required artifacts
are incomplete evidence. Ordinary PR fixup needs no extra audit unless the user
sets one of these requirements. Report any comparable baseline delta separately.
