---
id: "03-container-recovery-evidence"
title: "Prove container executor recovery"
status: done
wave: 3
depends_on:
  - "02-lifecycle-executor-recovery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.2
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 03: Prove container executor recovery

## Summary

Add real container-backed evidence for local Docker and remote SSH. Use a
test-only npx wrapper and no public registry access.

## In scope

- Add one Docker executor recovery spec.
- Add one SSH executor recovery spec.
- Add the smallest shared fixture helper for the deterministic npx wrapper.
- Assert original-session completion, exact retry arguments, and no failure card.

## Out of scope

- New UI controls or copy.
- Live npm registry tests.
- Mobile layout changes.

## Acceptance

- The offline attempt emits the strict ETARGET signature inside each target.
- The online attempt starts the mock ACP agent in the same executor and session.
- Each test proves the exact tree is gone and an unrelated tree remains.

## Verification

```bash
KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/docker/managed-runtime-npm-recovery.spec.ts
KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/ssh/managed-runtime-npm-recovery.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/docker/managed-runtime-npm-recovery.spec.ts`
- `apps/web/e2e/tests/ssh/managed-runtime-npm-recovery.spec.ts`
- `apps/web/e2e/fixtures/docker-probe.ts`
- `apps/web/e2e/fixtures/managed-runtime-npx.sh`
- `apps/web/e2e/fixtures/ssh-image.ts`
- `apps/web/e2e/helpers/managed-runtime-recovery.ts`

## Dependencies

- Task 02 completes the production recovery flow.

## Risks

- The fixture must keep the wrapper inside the target executor.
- The fixture must not enable unrelated real provider probes.

## Parallelism

`sequential`

## Inputs

- Docker and SSH E2E fixture contracts.
- Existing managed runtime recovery presentation specs.
- `/usr/local/bin/mock-agent` in both E2E target images.

## Results

- Docker recovery passed in the host-backed `containers` project with
  `KANDEV_E2E_CONTAINERS=1 pnpm exec playwright test --config
  e2e/playwright.config.ts --project=containers
  tests/docker/managed-runtime-npm-recovery.spec.ts --workers=1`.
- SSH recovery passed with the corresponding `tests/ssh/managed-runtime-npm-recovery.spec.ts`
  command.
- Both specs proved that the original session completed, the stale marker was
  removed, a fresh marker was recreated, the sibling tree remained, exactly one
  online invocation was recorded, and no recovery card was shown.
- The nested `e2e:run` wrapper could not start in this environment because its
  runtime had no Docker daemon; the equivalent host-backed containers project
  supplied the real Docker and SSH evidence.
