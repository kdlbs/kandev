---
id: "09-rollout-documentation"
title: "Document rollout and operating checks"
status: completed
wave: 5
depends_on:
  - "03-ci-manifest-lifecycle"
  - "06-container-isolation"
  - "07-retry-observability"
plan: "plan.md"
spec: "../../specs/platform/requirements/e2e-duration-aware-sharding.md"
---

# Task 09: Document rollout and operating checks

## Acceptance

- `apps/web/e2e/README.md` documents profile ownership, artifact names,
  manifest generation, new/changed-test fallback behavior, local dry runs,
  retry summaries, and container ownership requirements.
- Workflow comments and CI job summaries explain how to diagnose a stale or
  unavailable profile, an invalid manifest, a passed-after-retry result, and a
  shard predicted/actual mismatch.
- The rollout checklist records the three-successful-main-run balance check,
  the no-hidden-flake check, and the default `workers: 1`/matrix decisions.

## Verification

```sh
cd apps/web
pnpm typecheck
cd ../..
git diff --check
```

Review all paths in the linked spec, plan, and ADR after the implementation
changes so command names, artifact names, and status claims match the actual
workflow.

## Files likely touched

- `apps/web/e2e/README.md`
- `.github/workflows/e2e-tests.yml`
- `docs/specs/platform/requirements/e2e-duration-aware-sharding.md` (status or contract sync)
- `docs/decisions/2026-08-10-duration-aware-e2e-sharding.md` (consequence or
  implementation-status sync if needed)

## Dependencies

Depends on the final CI, container, and retry contracts from Tasks 03, 06, and 07. It should not invent a second source of truth for those contracts.

## Parallelism

Sequential finalization after behavior stabilizes.

## Inputs

- The generated workflow behavior and report examples.
- Existing E2E README commands and container prerequisites.
- The spec success criteria and proposed ADR.

## Output contract

Report documentation paths changed, command examples verified, rollout status,
and any remaining discrepancies between artifacts and implementation.

## Implementation result

Updated the E2E README, workflow comments/job summary, spec, plan, ADR, and
task statuses to describe the final artifact names, fallback behavior, scoped
Docker ownership, retry diagnostics, and rollout checks. The README commands
were reviewed against the generated runner and workflow paths; typecheck and
diff checks passed.
