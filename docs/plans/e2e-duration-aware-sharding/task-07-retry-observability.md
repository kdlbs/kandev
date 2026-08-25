---
id: "07-retry-observability"
title: "Surface retry and timing evidence"
status: completed
wave: 4
depends_on:
  - "03-ci-manifest-lifecycle"
plan: "plan.md"
spec: "../../specs/platform/requirements/e2e-duration-aware-sharding.md"
---

# Task 07: Surface retry and timing evidence

## Acceptance

- Every merged run publishes a retry summary with stable test key, attempts,
  final status, error category, and available trace/screenshot/video links.
- The report compares predicted and actual shard durations and records unknown,
  warm, fallback, and passed-after-retry counts.
- A scheduled or explicit diagnostic mode can enforce `failOnFlakyTests: true`
  without changing the normal PR retry gate during initial rollout.

## Verification

```sh
cd apps
pnpm --filter @kandev/web test -- e2e/scripts/retry-summary.test.ts
pnpm --filter @kandev/web typecheck
cd ..
git diff --check
```

Add fixture-based tests for first-attempt pass, pass-after-retry, final
failure, timeout, and missing artifact links.

## Files likely touched

- `.github/workflows/e2e-tests.yml`
- `apps/web/e2e/scripts/retry-summary.ts`
- `apps/web/e2e/scripts/retry-summary.test.ts`
- `apps/web/e2e/playwright.config.ts` or a diagnostic config override

## Dependencies

Depends on Task 03's report artifact flow and the timing identity from Task 01.

## Parallelism

Sequential after CI manifest integration. It shares the report job and must
not make the timing collector accept retry samples.

## Inputs

- Current blob report merge job and Playwright retry fields.
- The seven PR #2471 retry groups and the persistent failures observed in the
  adjacent PR run.
- Existing trace, screenshot, video, and job-summary artifact paths.

## Output contract

Report the summary schema, job-summary presentation, diagnostic-mode trigger,
normal PR policy, files changed, and exact focused test results.

## Implementation result

Implemented retry grouping with final status, error categories, attachments,
and predicted-versus-actual shard deltas. The report job uploads the retry and
timing diagnostics and supports `fail_on_flaky=true` without changing normal
PR retries. The focused retry-summary tests passed as part of the 15-test
tooling run.
Review remediation copies blob-internal retry resources into the retained
diagnostics artifact, records the artifact name and run URL, and renders
per-test retry/failure links in the GitHub job summary.
