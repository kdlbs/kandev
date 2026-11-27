---
id: "02-default-behavior"
title: "Prove default behavior and update guidance"
status: done
wave: 2
depends_on:
  - "01-retire-flag"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-001
  - REQ-EXECUTORS-PLUGIN-004
  - REQ-EXECUTORS-PLUGIN-007
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-001.1
  - AC-EXECUTORS-PLUGIN-001.3
  - AC-EXECUTORS-PLUGIN-001.8
  - AC-EXECUTORS-PLUGIN-004.1
  - AC-EXECUTORS-PLUGIN-007.1
  - AC-EXECUTORS-PLUGIN-007.2
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 02: Prove default behavior and update guidance

## Summary

Make the existing desktop and phone provider flows prove the default-on behavior. Replace flag-off
browser assertions with plugin absence and unavailability checks, then update public guidance.

## In scope

- Remove `KANDEV_FEATURES_REMOTE_EXECUTOR_PLUGINS` setup and teardown from affected Playwright specs.
- Cover the no-plugin state and an installed but unavailable provider without selecting a different
  executor. Keep desktop and phone profile, status, and retention flows.
- Update `docs/public/executors.md`, `plugins-authoring.md`, and `plugins-manifest.md` to describe
  installation as the entry point; update the ADR's rollout sentence to historical wording.
- Promote the paired requirement to `active`, design to `current`, and plan to `implemented` after all
  work-order checks pass and the final behavior matches them.

## Out of scope

- New provider UI, changes to phone layout, or a production cloud plugin.

## Acceptance

1. Desktop and phone E2E flows pass with shipped defaults and the fixture plugin installed.
2. With no eligible plugin, provider selection is absent or unavailable while built-in executors
   remain usable; retained resources remain identifiable when a provider becomes unavailable.
3. Public instructions and the ADR no longer present administrator flag activation as a current step.

## Verification

```bash
(cd apps/web && pnpm e2e:run --project chromium -- e2e/tests/plugins/remote-executor-plugin.spec.ts e2e/tests/settings/plugin-executor-profiles.spec.ts e2e/tests/session/plugin-executor-status.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- e2e/tests/settings/mobile-plugin-executor-profiles.spec.ts e2e/tests/session/mobile-plugin-executor-status.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
make -C apps/backend lint
git diff --check
```

The E2E runner builds the current web and backend before each invocation. In a fresh worktree, run
`(cd apps && pnpm install --frozen-lockfile)` first. Run the two project commands sequentially.

## Files likely touched

- `apps/web/e2e/tests/plugins/remote-executor-plugin.spec.ts`
- `apps/web/e2e/tests/settings/plugin-executor-profiles.spec.ts`, `mobile-plugin-executor-profiles.spec.ts`
- `apps/web/e2e/tests/session/plugin-executor-status.spec.ts`, `mobile-plugin-executor-status.spec.ts`
- `docs/public/executors.md`, `docs/public/plugins-authoring.md`, `docs/public/plugins-manifest.md`
- `docs/decisions/2026-09-26-remote-executor-plugin-boundary.md`
- `docs/specs/executors/requirements/remote-executor-plugins.md`, `system-design/remote-executor-plugins.md`
- `docs/plans/remote-executor-plugin-flag-graduation/plan.md`

## Dependencies

Task 01.

## Risks

The current E2E helper installs the fixture after changing environment settings; its setup and
cleanup order must remain deterministic once those toggles are removed.

## Parallelism

`sequential`

## Inputs

- [Executor requirements](../../specs/executors/requirements/remote-executor-plugins.md),
  [system design](../../specs/executors/system-design/remote-executor-plugins.md), and
  [original implemented plan](../remote-executor-plugins/plan.md).
- Existing desktop and phone provider Playwright specs and public executor/plugin pages.

## Results

Desktop Playwright passed all 4 selected tests and mobile Playwright passed both selected tests using
the managed E2E runner. Public docs validation passed (62 tests and 47 pages); spec validation and
lint passed (315 decisions, 1,201 specs, and 36 lint tests). The browser profile selectors now target
the profile ID so repeated display names remain unambiguous. `git diff --check` passed.
