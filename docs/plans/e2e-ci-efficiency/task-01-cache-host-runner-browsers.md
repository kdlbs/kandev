---
id: "01-cache-host-runner-browsers"
title: "Cache host-runner Playwright browsers"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/e2e-duration-aware-sharding.md"
---
# Task 01: Cache host-runner Playwright browsers

## Outcome

Remove repeat browser provisioning from warm container-backed E2E jobs while
preserving the current pinned-image path as a correctness-safe fallback.

## Requirements and design

- `REQ-PLATFORM-E2E-DURATION-AWARE-SHARDING-002`
- `AC-PLATFORM-E2E-DURATION-AWARE-SHARDING-002.1`
- `AC-PLATFORM-E2E-DURATION-AWARE-SHARDING-002.2`
- `docs/specs/platform/system-design/e2e-duration-aware-sharding.md`,
  Browser cache contract and Observability sections
- `docs/decisions/2026-08-30-e2e-browser-cache.md`

## Acceptance

- The existing host-runner container jobs resolve the runtime image to an
  immutable digest and restore `/tmp/ms-playwright` using a digest-scoped
  restore prefix plus a run-specific primary key. A verified exact or prefix
  hit skips the Docker pull/copy step.
- A cache miss, stale or unavailable entry, cache action error, or failed
  browser verification runs the digest-pinned image extraction and smoke
  check. A successful fallback uses a new primary key so a rejected cache
  entry is not repeatedly selected. Cache operations cannot fail a healthy
  fallback or change the test manifest, retries, timeout, or worker count.
- Each container job reports cache state, browser verification, and setup
  timing. Workflow contract tests protect the key, conditional fallback, and
  existing container setup.

## Likely files

- `.github/workflows/e2e-tests.yml`
- `.github/scripts/e2e-tests-workflow-contract_test.py`
- `apps/web/e2e/README.md` when the local/CI setup documentation needs the
  cache and fallback behavior recorded

## Verification

```bash
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
git diff --check
```

After the branch workflow is available, inspect one cache-hit and one forced
miss run. The hit must omit the browser pull/copy step. The miss must complete
the existing extraction and E2E smoke path. Record restore, verification,
fallback, and test-start durations in the parent plan.

## Dependencies and parallelism

No implementation dependency. Execute sequentially in the primary session so
the workflow contract and fallback behavior are reviewed as one change.

## Exclusions

Do not change the runtime image's browser source, the normal E2E job, the
matrix sizes, or Docker resource ownership. Do not make cache availability a
required external service.

## Results

The workflow contract, action-pinning tests, targeted workflow security scan,
YAML parse, and diff checks pass locally. PR #3163 passed all container shards;
run 33325045980 restored a digest-scoped prefix cache, verified Chromium, and
skipped image extraction in a reported setup interval of about 9 seconds. The
workflow also retries transient immutable-image metadata lookup failures three
times. A forced-miss fallback run and the post-merge three-run comparison remain
pending for rollout validation.
