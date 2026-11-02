---
id: "03-ci-manifest-lifecycle"
title: "Wire manifests into the CI workflow"
status: completed
wave: 3
depends_on:
  - "01-timing-profile"
  - "02-shard-planner"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-E2E-DURATION-AWARE-SHARDING-001
  - REQ-PLATFORM-E2E-DURATION-AWARE-SHARDING-002
acceptance_criteria:
  - AC-PLATFORM-E2E-DURATION-AWARE-SHARDING-001.1
  - AC-PLATFORM-E2E-DURATION-AWARE-SHARDING-001.2
  - AC-PLATFORM-E2E-DURATION-AWARE-SHARDING-002.3
system_design:
  - ../../specs/platform/system-design/e2e-duration-aware-sharding.md
---

# Task 03: Wire manifests into the CI workflow

## Acceptance

- The build/planning job obtains the latest successful `main` profile when
  available, falls back visibly when it is not, validates both cohorts, and
  uploads run-scoped normal/container manifests.
- Normal and container matrix jobs download their own manifest and execute
  explicit selections through one shared runner while preserving current
  project filters, retries, timeouts, blob reporting, environment, output
  isolation, and `workers: 1`; they do not silently use ordinal `--shard`.
- The report job emits a profile candidate, retry summary, predicted-versus-
  actual timing summary, and validation/fallback counters, while only
  successful `main` artifacts can become the next baseline.

## Verification

```sh
cd apps
pnpm --filter @kandev/web test -- e2e/scripts/e2e-timings.test.ts e2e/scripts/plan-shards.test.ts
pnpm --filter @kandev/web typecheck
cd ..
git diff --check
```

The workflow review must confirm the permission needed to read another run's
artifact, stable artifact names, failure behavior for missing manifests, and
that every matrix command receives a shard-specific explicit file list.

## Files likely touched

- `.github/workflows/e2e-tests.yml`
- `apps/web/e2e/scripts/run-planned-shard.sh`
- `apps/web/e2e/scripts/e2e-timings.ts`
- `apps/web/e2e/scripts/plan-shards.ts`

## Dependencies

Depends on Tasks 01 and 02. Do not duplicate profile parsing or bin packing in
the workflow.

## Parallelism

Sequential. This task changes the single CI execution path and must land only
after the manifest and fallback unit contracts are tested.

## Inputs

- Current build, normal matrix, container matrix, and report jobs in
  `.github/workflows/e2e-tests.yml`.
- GitHub Actions artifact download permissions and run selection behavior.
- Existing `apps/web/e2e/scripts/run-e2e.sh` environment and output
  conventions.

## Output contract

Report the exact job/data flow, artifact names, permission changes, fallback
behavior, command-line invocation, files changed, and local verification
results. Include a dry-run or fixture-based proof that a shard cannot run
without its validated manifest.

## Implementation result

The workflow now fetches the latest successful `main` profile when available,
creates and uploads run-scoped `normal/<n>.json` and `containers/<n>.json`
manifests, runs both matrices through the shared validated runner, and uploads
profile/retry/diagnostic artifacts. A missing-manifest invocation failed closed
with the expected error. Backend/web builds and the focused type/lint checks
passed.


### PR #3680 CI diagnostic retention (2026-10-10)

The exact-head normal shards 5 and 12 reached their 45-minute job limit. Their
blob reporters had not finalized, so the blob uploads failed. The results
uploads used `failure()` and were skipped for the cancelled jobs, losing the
available trace and screenshot diagnostics. Normal, container and Kubernetes result
uploads now admit cancellation as well as failure. This preserves diagnosis
without changing test selection, worker count, retries, timeout or gate status.
The legacy plan/work-order metadata now uses the existing requirements,
acceptance criteria and design fields accepted by the coverage evaluator.

Validation: all 16 existing workflow contract tests passed. Catalog and full
specification validation passed, and the trusted documentation evaluator
accepted all 13 changed work orders in the candidate diff.
