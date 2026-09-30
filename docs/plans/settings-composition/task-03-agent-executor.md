---
id: "03-agent-executor"
title: "Agents and execution settings"
status: complete
wave: 3
depends_on:
  - 02-preferences
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

# Task 03: Agents and execution settings

## Summary

Normalize agent, executor, utility-agent, MCP, and prompt settings chrome. Preserve their forms, editor content, technical details, and immediate commands.

## In scope

- Migrate all assigned inventory rows, including profile detail and create pages.
- Reuse shared card headers, fields, helpers, descriptions, and actions in nested settings forms and dialogs.
- Retain SSH/Kubernetes diagnostic bodies and technical editors. Record exact exceptions.
- Extend the rendered matrix with the agent executor tag.

## Out of scope

- Executor lifecycle changes, real-container scenarios, model resolution changes, and provider API contracts.

## Acceptance

- Assigned pages and their dialogs use shared presentation without new navigation.
- Profile edits preserve Save/Reset and reload behavior, while immediate test/install/connect actions retain their contracts.
- Desktop and phone evidence covers long names, disabled states, and field/action geometry.

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
(cd apps/web && pnpm exec vitest run components/settings)
(cd apps/web && pnpm e2e:run e2e/tests/settings/settings-composition.spec.ts -- --grep 'agent executor')
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-settings-composition.spec.ts -- --grep 'agent executor')
(cd apps/web && pnpm e2e:run e2e/tests/settings/agent-profile-layout.spec.ts e2e/tests/settings/executor-profile-spacing.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-agent-profile-layout.spec.ts e2e/tests/settings/mobile-executor-profile-spacing.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec eslint components/settings app/settings lib/settings-discovery --max-warnings 0)
git diff --check
```

Also run every existing E2E file whose selectors or assertions this work order changes.
Record its exact command and outcome under Results. Do not weaken assertions or leave changed tests unrun.

## Files likely touched

- `apps/web/app/settings/{agents,executor,executors,utility-agents,external-mcp}/`
- `apps/web/components/settings/{profile-edit,agent-profile-page.tsx,prompts-settings.tsx}`
- `apps/web/components/settings/ SSH, Kubernetes, agent, MCP, and executor forms`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/settings/settings-composition.spec.ts` (new shared family matrix)
- `apps/web/e2e/tests/settings/mobile-settings-composition.spec.ts` (new shared family matrix)
- Existing component and E2E tests beside the migrated surfaces

## Dependencies

Complete 02-preferences first.

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

Implemented and verified. Agents, executors, utility-agent, MCP, prompt, and create/profile surfaces now use shared resource or form group chrome where they own the surrounding presentation. Specialized SSH/Kubernetes diagnostics, prompt/editor bodies, and immediate install/test/connect actions remain intact.

- `(cd apps/web && pnpm exec vitest run components/settings lib/settings-discovery/target.test.ts src/settings-routes.test.ts)`: passed, 207 files and 1,389 tests.
- `(cd apps/web && pnpm e2e:run e2e/tests/settings/settings-composition.spec.ts)`: passed, 6 desktop tests, including agent/executor coverage.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-settings-composition.spec.ts)`: passed, 5 mobile tests, including agent/executor coverage.
- `(cd apps/web && pnpm e2e:run e2e/tests/settings/agent-profile-layout.spec.ts e2e/tests/settings/executor-profile-spacing.spec.ts)`: passed in the 22-test desktop specialized run.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-agent-profile-layout.spec.ts e2e/tests/settings/mobile-executor-profile-spacing.spec.ts)`: passed in the 10-test mobile specialized run.
- Typecheck, localization checks, ratchet, scoped ESLint, scoped Prettier, and `git diff --check`: passed.
