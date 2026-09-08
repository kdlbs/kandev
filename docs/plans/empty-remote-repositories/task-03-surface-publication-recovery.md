---
id: "03-surface-publication-recovery"
title: "Surface Publication Recovery"
status: done
wave: 3
depends_on:
  - "02-publish-empty-remote-base"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002
acceptance_criteria:
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.5
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.6
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.7
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.8
system_design:
  - ../../specs/workspaces/system-design/empty-remote-repositories.md
---

# Task 03: Surface Publication Recovery

## Summary

Map empty-remote publication error codes to localized recovery messages. Reuse the current desktop and mobile Changes feedback surfaces.

## In scope

- Carry `error_code` through shared Git and change-request result types.
- Map the three bounded empty-remote errors to translated toast text.
- Add required locale keys in all supported catalogs.
- Cover Push and Create PR error mapping with focused frontend tests.

## Out of scope

- Add a new visual surface or responsive composition.
- Change Push, Create PR, or Git graph behavior.
- Add a persistent recovery state.

## Acceptance

- A changed remote tells the user to reconcile remote history before retrying.
- Base and task-branch publication failures state which refs exist and which work remains local.
- Desktop and mobile use the same action state and localized result mapping.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- hooks/use-git-operations.test.ts components/task/changes-panel-hooks.test.ts
pnpm --filter @kandev/web i18n:check
```

## Files likely touched

- `apps/web/hooks/use-git-operations.ts`
- `apps/web/hooks/use-git-operations.test.ts`
- `apps/web/components/task/changes-panel-hooks.ts`
- `apps/web/components/task/changes-panel-hooks.test.ts`
- `apps/web/src/locales/en/common.json`
- `apps/web/src/locales/pt-pt/common.json`
- `apps/web/src/locales/zh-cn/common.json`
- `apps/web/src/locales/zh-hk/common.json`
- `apps/web/src/locales/zh-tw/common.json`

## Dependencies

- Task 02 defines the bounded backend error codes and result semantics.

## Risks

- Raw Git output can bypass translated recovery text if one Changes handler ignores `error_code`.
- Create PR and ordinary Push use related but different response types.

## Parallelism

`sequential`

## Inputs

- `AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.5` through `002.8`
- Existing Changes toast and mobile menu behavior.
- The complete i18n catalog contract.

## Results

Completed. Bounded publication error codes now flow through Git and change-request results into localized desktop and mobile Changes feedback. The focused frontend suite passed (22 tests), all locale checks passed, and web typecheck passed.
