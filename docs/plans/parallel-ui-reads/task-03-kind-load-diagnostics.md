---
id: "03-kind-load-diagnostics"
title: "Retain Kind image-load diagnostics"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-E2E-DURATION-AWARE-SHARDING-002
acceptance_criteria:
  - AC-PLATFORM-E2E-DURATION-AWARE-SHARDING-002.3
system_design:
  - ../../specs/platform/system-design/e2e-duration-aware-sharding.md
---

# Task 03: Retain Kind image-load diagnostics

## Scope

CI run 41750009038 failed all four attempts to load the Kubernetes runtime
image into Kind. The fixture discarded command output, so its error and
artifacts showed only the failed command. The cause cannot be recovered from
those artifacts.

Use the existing bounded command helper for runtime and worker image imports.
Keep the command arguments, timeout, resource ownership, and failure behavior.
Record output tails and exit status in quiet-mode failures. Hosted CI must
validate the real image load because the local host has no Docker daemon.

## Files

- `apps/web/e2e/fixtures/kubernetes-tools.ts`
- `apps/web/e2e/fixtures/kubernetes-fixture-policy.ts`
- `apps/web/e2e/fixtures/kubernetes-worker-images.ts`
- `apps/web/e2e/scripts/kubernetes-worker-images.test.ts`
- `apps/web/e2e/scripts/kubernetes-fixture-policy.test.ts`

## Validation and results

The fake Kind transfer emits long output and fails with exit code 17. Its
regression failed before the change because stdout was lost. After the change,
the error retained both output tails and exit status within the size bound.
The existing owned-image cleanup assertions also passed.

```bash
(cd apps/web && pnpm exec vitest run \
  e2e/scripts/kubernetes-worker-images.test.ts \
  e2e/scripts/fixture-command.test.ts \
  e2e/scripts/kubernetes-fixture-policy.test.ts)
```

Initially, all 13 tests across these three suites passed. Hosted CI later
passed the real runtime image load. This change does not claim to fix the
image loader's underlying failure.

CodeRabbit identified two diagnostic edge cases. Setup failure cleanup now
attempts both owned-resource removals and retains the original error plus
cleanup errors in the reported message. Four cases cover successful cleanup,
either removal failing, and both removals failing. The worker-image test
explicitly selects quiet mode so inherited debug settings cannot remove its
captured stream tails. The test failed with `E2E_DEBUG=1` before that correction.
All 17 fixture tests passed with `E2E_DEBUG=1`; changed-file ESLint passed.
