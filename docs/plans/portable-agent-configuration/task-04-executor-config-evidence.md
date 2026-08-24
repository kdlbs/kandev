---
id: "04-executor-config-evidence"
title: "Prove executor configuration behavior"
status: done
wave: 3
depends_on: ["02-configuration-transfer", "03-executor-config-controls"]
plan: "plan.md"
spec: "../../specs/agents/requirements/portable-agent-configuration.md"
---

# Task 04: Prove executor configuration behavior

## Acceptance

- Desktop and mobile E2E prove selection, warning access, save, reload, touch geometry, and independent authentication.
- Container E2E proves selected-only fresh copy and no overwrite during warm resume.
- Public executor and security documentation explains the copy timing, limits, hook risk, and SSH overwrite risk.

## TDD scenarios

1. RED: Add desktop and mobile settings scenarios before the controls exist.
2. RED: Add a container scenario that fails because no selected bundle reaches the agent home.
3. GREEN: Complete missing selectors, fixtures, and causal waits without weakening assertions.
4. GREEN: Update the executor and security documentation.
5. REFACTOR: Share settings page helpers and restore all changed fixture state.

## Verification

- `cd apps/web && pnpm e2e:run tests/settings/executor-agent-config.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/settings/mobile-executor-agent-config.spec.ts`
- `cd apps/web && pnpm e2e:run --project containers tests/docker/agent-config-copy.spec.ts`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check`

## Files likely touched

- `apps/web/e2e/tests/settings/executor-agent-config.spec.ts`
- `apps/web/e2e/tests/settings/mobile-executor-agent-config.spec.ts`
- `apps/web/e2e/tests/settings/executor-agent-config-helpers.ts`
- `apps/web/e2e/tests/docker/agent-config-copy.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `docs/public/executors.md`
- `docs/public/security.md`

## Dependencies

- Task 02 supplies runtime copy behavior.
- Task 03 supplies the user interface.

## Parallelism

Sequential after Tasks 02 and 03.
The task validates their combined behavior.

## Inputs

- Completed runtime and frontend tasks.
- Existing executor profile E2E fixtures.
- The Playwright container project.
- The public executor and security guides.

## Output contract

Report RED evidence, GREEN evidence, exact E2E commands, public pages, failures, and final package status.

## Risks

- The container suite needs a real Docker daemon.
- Worker-scoped executor profiles require explicit cleanup after each scenario.
- A mobile click does not prove coarse-pointer behavior. The scenario must use `tap()`.

## Results

Desktop settings E2E passed selection, authentication independence, save, and
reload. Mobile E2E passed the bottom drawer, touch targets, and overflow checks.
The real Docker E2E passed fresh provisioning, warm resume preservation, and
environment-reset replacement. Executor and security documentation passed the
public-doc audit.
