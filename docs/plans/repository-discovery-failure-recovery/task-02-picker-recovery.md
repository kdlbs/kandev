---
id: "02-picker-recovery"
title: "Show discovery failures and recovery"
status: done
wave: 2
depends_on: ["01-scan-recovery"]
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-003
acceptance_criteria:
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.4
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.5
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.7
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.8
  - AC-WORKSPACES-LOCAL-REPOSITORIES-003.11
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
---

# Task 02: Show discovery failures and recovery

## Summary

Show root failures inside existing selectors while preserving available choices.
Provide manual recovery for browser and phone clients through shared discovery state.

## In scope

- Add failing component tests for a server response with repositories and failed roots.
- Add empty-failure, refresh-progress, successful-recovery, and long-path cases.
- Render failures in `RepositoryDiscoveryControls` independently of desktop mode.
- Keep saved desktop Reconnect/Remove controls without duplicate root warnings.
- Reuse `failedRoots` and manual refresh from the existing discovery hook.
- Check all existing consumers: task creation, workspace sources, repository
  settings, automations, and Office project setup.
- Add localized copy in all required catalogs and generate Traditional Chinese.
- Extend desktop and mobile discovery E2E with failure and recovery flows.
- Update recovery guidance in configuration and usage documentation.

## Out of scope

New picker layouts, native bridge changes, new discovery endpoints, and
automatic retry changes are outside this work order.

## Acceptance

- Server and desktop selectors show failed paths and recovery with empty or
  nonempty results. Successful recovery clears the warning without reopening.
- Available choices remain selectable during failure. Shared coordinator tests
  prove manual recovery without new background retries or duplicate scans.
- Desktop and phone E2E prove selection and recovery. Phone paths wrap, actions
  have 44-pixel hit targets, and the document has no horizontal overflow.

## ASCII UI preview

UI-01: Failed scan, excerpt from [the plan](plan.md#ascii-ui-preview).

```text
Desktop selector                Phone selector
Search + Refresh                Search
Warning + failed paths          Warning + wrapped failed paths
Refresh                         Refresh (44px target)
Available repository choices    Available repository choices
```

Keep the existing selector surface and scroll owner. Use the Add Workspace
Sources inline-error pattern. No new overlay is required. During refresh,
show progress and disable the action. On success, remove the warning.
An empty result retains the warning above the existing empty message.
These structural requirements implement AC-003.11. Copy is illustrative and localized.

## Verification

From the repository root, install workspace dependencies if this worktree has none.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/repository-discovery-controls.test.tsx hooks/domains/workspace/use-repository-discovery.test.ts components/task-create-dialog-effects.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm exec eslint components/repository-discovery-controls.tsx)
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --project chromium tests/task/repository-discovery-consent.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-repository-discovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run added component/helper tests explicitly if implementation creates separate files.
Inspect the rendered phone failure state and compare it with UI-01. Keep E2E
runs sequential under the existing worker limits. Use controlled API responses
for UI errors. Task 01 supplies deterministic filesystem regression coverage.

## Files likely touched

- `apps/web/components/repository-discovery-controls.tsx` and its test
- `apps/web/hooks/domains/workspace/use-repository-discovery.test.ts`
- `apps/web/components/task-create-dialog-effects.test.ts`
- `apps/web/e2e/tests/task/repository-discovery-consent.spec.ts`
- `apps/web/e2e/tests/task/mobile-repository-discovery.spec.ts`
- `apps/web/src/locales/` required catalogs
- `docs/public/configuration.md` (reference)
- `docs/public/use-kandev.md` (how-to)

## Dependencies

Task 01 provides the corrected result semantics. Public docs describe the final
implementation, not draft behavior.

## Risks

Some selectors already provide Refresh. Avoid duplicate actions where one
existing action clearly supports the warning. Do not offer root mutations for
operator-configured paths. Preserve accessibility and existing desktop controls.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/local-repositories.md), REQ-003.
- [Design](../../specs/workspaces/system-design/local-repositories.md), User interface.
- Existing discovery-controls tests and desktop/mobile consent E2E fixtures.
- Mobile-parity, E2E, TDD, and docs-maintainer skills.

## Results

Implemented shared failed-root visibility and manual recovery for browser,
phone, desktop, workspace settings, and existing repository-selector
consumers. Available repositories remain selectable during partial failure.
Saved desktop roots retain Reconnect and Remove controls without duplicate
warnings. The localized failure notice wraps long paths in a bounded internal
list, keeps its title and Refresh action reachable, uses a 44-pixel touch
target, disables during refresh, and clears after a successful response.

Added component, hook, and Chromium/mobile-chrome E2E coverage. Updated all
required locale catalogs and public configuration, desktop, and usage guidance.

Verification passed:

- Focused Vitest run: 82 tests passed across 6 files
- `pnpm run typecheck`
- `pnpm run i18n:check`
- Targeted ESLint for discovery components and workspace repository settings
- `pnpm e2e:run --project chromium tests/task/repository-discovery-consent.spec.ts` (3 passed)
- `pnpm e2e:run --project mobile-chrome tests/task/mobile-repository-discovery.spec.ts` (3 passed)
- Mobile E2E covers eight long failed roots, warning-list scrolling, selector
  viewport containment, reachable Refresh, healthy repository selection, and
  successful recovery.
- `make build-web`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check`
