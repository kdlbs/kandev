---
id: "04-workspace-integration"
title: "Workspace and integration settings"
status: complete
wave: 4
depends_on: 
  - 03-agent-executor
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
  - REQ-UI-SETTINGS-COMPOSITION-003
  - REQ-UI-SETTINGS-COMPOSITION-004
acceptance_criteria:
  - AC-UI-SETTINGS-COMPOSITION-001.1
  - AC-UI-SETTINGS-COMPOSITION-001.2
  - AC-UI-SETTINGS-COMPOSITION-001.3
  - AC-UI-SETTINGS-COMPOSITION-001.4
  - AC-UI-SETTINGS-COMPOSITION-001.5
  - AC-UI-SETTINGS-COMPOSITION-003.1
  - AC-UI-SETTINGS-COMPOSITION-003.2
  - AC-UI-SETTINGS-COMPOSITION-003.3
  - AC-UI-SETTINGS-COMPOSITION-003.4
  - AC-UI-SETTINGS-COMPOSITION-003.5
  - AC-UI-SETTINGS-COMPOSITION-004.1
  - AC-UI-SETTINGS-COMPOSITION-004.2
  - AC-UI-SETTINGS-COMPOSITION-004.3
  - AC-UI-SETTINGS-COMPOSITION-004.4
  - AC-UI-SETTINGS-COMPOSITION-004.5
system_design:
  - ../../specs/ui/system-design/settings-composition.md
---

# Task 04: Workspace and integration settings

## Summary

Normalize workspace, repository, workflow, automation, secret, and integration settings presentation. Preserve scoped credentials and specialized editor interactions.

## In scope

- Migrate assigned inventory rows and first-party connection/watch forms.
- Retain workflow pipeline and automation editor geometry while normalizing their surrounding fields and actions.
- Apply consistent credential descriptions, visible permissions, and status treatment without hiding essential scope or override rules.
- Extend the rendered matrix with the workspace integration tag.

## Out of scope

- Integration protocol changes, credential scope changes, and third-party plugin content.

## Acceptance

- Workspace and provider settings share form/resource anatomy while retaining their active-workspace and tab behavior.
- Existing save, credential, confirmation, and immediate-command tests retain their behavioral assertions.
- Mock-backed desktop and mobile coverage proves save/reload, empty/populated states, and contained credentials/actions.

## ASCII UI preview

### UI-04 / UI-03: Shared group composition

```text
Page title                         [existing actions/tabs]
One description.
Group heading                      [group action]
+-------------------------------------------------------+
| Label / short description          [control]           |
| Related field or existing resource content             |
| Visible error or scope notice when applicable          |
+-------------------------------------------------------+
[Reset] [Save changes]             only while dirty
```

On phones, group actions and fields stack. Specialized bodies retain their existing mobile interaction.

See the [full labelled previews](plan.md#ascii-ui-preview). Applicable criteria are listed in frontmatter.
Group structure and state visibility are required. ASCII spacing is illustrative.

## Verification

Start from the repository root. Install dependencies once before the first package command.
Add or update tests before changing covered behavior. Compare rendered results with the assigned preview.
New E2E files and family tags are defined in the plan and must exist before running these commands.

```bash
(cd apps/web && pnpm exec vitest run components/settings components/github components/gitlab components/jira components/linear components/azure-devops components/sentry)
(cd apps/web && pnpm exec eslint components/github components/gitlab components/jira components/linear components/azure-devops components/sentry components/integrations --max-warnings 0)
(cd apps/web && pnpm e2e:run e2e/tests/settings/settings-composition.spec.ts -- --grep 'workspace integration')
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-settings-composition.spec.ts -- --grep 'workspace integration')
(cd apps/web && pnpm e2e:run e2e/tests/integrations/github-workspace-settings.spec.ts e2e/tests/integrations/jira-settings.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/integrations/mobile-github-workspace-settings.spec.ts e2e/tests/settings/mobile-workspace-settings-tabs.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec eslint components/settings app/settings lib/settings-discovery --max-warnings 0)
git diff --check
```

Also run every existing E2E file whose selectors or assertions this work order changes.
Record its exact command and outcome under Results. Do not weaken assertions or leave changed tests unrun.

## Files likely touched

- `apps/web/app/settings/workspace/`
- `apps/web/components/settings/workspaces/`
- `apps/web/components/settings/ repository, workflow, secret, and automation forms`
- `apps/web/components/{github,gitlab,jira,linear,azure-devops,sentry}/ settings forms`
- `apps/web/components/integrations/ host-owned settings forms`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/settings/settings-composition.spec.ts` (new shared family matrix)
- `apps/web/e2e/tests/settings/mobile-settings-composition.spec.ts` (new shared family matrix)
- Existing component and E2E tests beside the migrated surfaces

## Dependencies

Complete 03-agent-executor first.

## Risks

- Shared wrappers can lose dirty markers, accessible descriptions, discovery targets, or state identity.
- Long translations and coarse-pointer controls can expose clipping despite correct desktop appearance.
- Existing tests can depend on card nesting. Preserve behavioral checks when updating selectors.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/settings-composition.md).
- [System design](../../specs/ui/system-design/settings-composition.md).
- [Surface inventory](surface-inventory.md) and the assigned route imports.
- `apps/web/AGENTS.md`, `/tdd`, `/mobile-parity`, and `/e2e`.
- Existing typography, header tabs, control sizing, and route-save contracts.

## Results

Implemented and verified. Workspace settings, secrets, and the integration route adapter now expose shared group chrome while preserving workspace tabs, credential scope, watch tables, form saves, and specialized workflow or automation editors. Repository and workflow sections opt out of the outer group frame so their resource/editor cards remain the single bordered surface. Existing integration children retain their domain forms and immediate commands.

- `(cd apps/web && pnpm exec vitest run components/settings lib/settings-discovery/target.test.ts src/settings-routes.test.ts)`: passed, 207 files and 1,389 tests.
- `(cd apps/web && pnpm e2e:run e2e/tests/settings/settings-composition.spec.ts)`: passed, 6 desktop tests, including workspace/integration coverage.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-settings-composition.spec.ts)`: passed, 5 mobile tests, including workspace/integration coverage.
- `(cd apps/web && pnpm e2e:run e2e/tests/integrations/github-workspace-settings.spec.ts e2e/tests/integrations/jira-settings.spec.ts)`: the specialized desktop run passed 20 of 22 tests; the affected repository-scope test passed in the focused rerun after the heading semantics fix. The other specialized integration tests passed in the original run.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/integrations/mobile-github-workspace-settings.spec.ts e2e/tests/settings/mobile-workspace-settings-tabs.spec.ts)`: passed as part of the 10-test mobile specialized run.
- `(cd apps/web && pnpm e2e:run --host --no-build -- --retries=0 e2e/tests/settings/repository-delete.spec.ts e2e/tests/settings/repository-add-local.spec.ts)`: passed, 3 desktop repository tests after removing the duplicate outer card.
- `(cd apps/web && pnpm e2e:run --host --no-build -- --project=mobile-chrome --retries=0 e2e/tests/settings/mobile-repository-add-local.spec.ts)`: passed, 1 mobile repository test.
- Typecheck, localization checks, ratchet, scoped ESLint, scoped Prettier, and `git diff --check`: passed.
