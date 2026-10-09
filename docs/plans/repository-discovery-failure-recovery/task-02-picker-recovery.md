---
id: "02-picker-recovery"
title: "Preserve selector recovery without visible failure paths"
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

# Task 02: Preserve selector recovery without visible failure paths

## Summary

Keep failed-root diagnostics in structured backend logs while preserving
available choices and the normal Refresh action in existing selectors.

## In scope

- Add component and E2E coverage for a server response with repositories and
  failed roots, asserting that failed paths are not rendered.
- Keep existing normal Refresh behavior available during partial failure and
  refresh the repository list without a background retry.
- Keep saved desktop Reconnect/Remove controls unchanged.
- Retain `failedRoots` for coordinator retry suppression and structured
  diagnostics; preserve the existing repository-list Refresh in selectors.
- Check all existing consumers: task creation, workspace sources, repository
  settings, automations, and Office project setup.
- Remove warning-only locale keys that no longer have a selector consumer.
- Extend desktop and mobile discovery E2E with partial-failure and selection
  flows.
- Update recovery guidance in configuration and usage documentation.

## Out of scope

New picker layouts, native bridge changes, new discovery endpoints, and
automatic retry changes are outside this work order.

## Acceptance

- Server and phone selectors do not show failed-root warnings or paths when a
  discovery response contains `failed_roots`.
- Available choices remain selectable during failure, and the existing normal
  Refresh action remains reachable and refreshes the repository list without a
  background retry.
- Desktop E2E keeps explicit saved-root Reconnect/Remove management. Phone E2E
  keeps the existing Refresh action at a 44-pixel touch target.

## ASCII UI preview

UI-01: Failed scan, excerpt from [the plan](plan.md#ascii-ui-preview).

```text
Desktop selector                Phone selector
Search + Refresh                Search + Refresh
Available repository choices    Available repository choices
```

Keep the existing selector surface and scroll owner. Do not add a failure
warning, path list, or selector-specific containment for failed roots. During
refresh, preserve the existing action state and available choices. Failed-root
details remain in structured backend logs. These structural requirements
implement AC-003.11.

## Verification

From the repository root, install workspace dependencies if this worktree has none.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/repository-discovery-controls.test.tsx components/repository-discovery-root-controls.test.tsx hooks/domains/workspace/use-repository-discovery.test.ts)
(cd apps/web && pnpm run typecheck)
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
Inspect the rendered phone selector and compare it with UI-01. Keep E2E runs
sequential under the existing worker limits. Use controlled API responses for
partial discovery results. Task 01 supplies deterministic filesystem
regression coverage.

## Files likely touched

- `apps/web/components/repository-discovery-controls.tsx` and its test
- `apps/web/hooks/domains/workspace/use-repository-discovery.test.ts`
- `apps/web/components/repository-discovery-root-controls.test.tsx`
- `apps/web/components/task-create-dialog-pill.tsx`
- `apps/web/e2e/tests/task/repository-discovery-consent.spec.ts`
- `apps/web/e2e/tests/task/mobile-repository-discovery.spec.ts`
- `apps/web/src/locales/` required catalogs
- `docs/public/configuration.md` (reference)
- `docs/public/use-kandev.md` (how-to)

## Dependencies

Task 01 provides the corrected result semantics. Public docs describe the final
implementation, not draft behavior.

## Risks

Some selectors already provide Refresh. Preserve the existing action instead
of adding a second recovery control. Do not offer root mutations for
operator-configured paths. Preserve accessibility and existing desktop controls.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/local-repositories.md), REQ-003.
- [Design](../../specs/workspaces/system-design/local-repositories.md), User interface.
- Existing discovery-controls tests and desktop/mobile consent E2E fixtures.
- Mobile-parity, E2E, TDD, and docs-maintainer skills.

## Results

Kept failed-root details in structured backend logs and removed their selector
warning/path list. Browser and phone selectors retain available repositories
and the normal Refresh action during partial failure. Saved desktop roots retain
Reconnect and Remove controls.

Updated component, hook, and Chromium/mobile-chrome E2E coverage. Removed the
unused warning locale keys and updated public configuration, desktop, and usage
guidance.

Verification passed:

- Correction-focused Vitest run: 29 tests passed across 4 files
- `pnpm run typecheck`
- `pnpm run i18n:check`
- Targeted ESLint for discovery components and workspace repository settings
- `pnpm e2e:run --project chromium tests/task/repository-discovery-consent.spec.ts` (3 passed)
- `pnpm e2e:run --project mobile-chrome tests/task/mobile-repository-discovery.spec.ts` (3 passed)
- Mobile E2E covers failed-root diagnostics staying out of the selector,
  reachable Refresh, and healthy repository selection.
- `make build-web`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check`
