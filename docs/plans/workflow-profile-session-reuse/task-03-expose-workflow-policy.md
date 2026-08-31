---
id: "03-expose-workflow-policy"
title: "Expose workflow policy"
status: done
wave: 3
depends_on:
  - "02-implement-safe-session-parking"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.7
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.8
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 03: Expose Workflow Policy

## Summary

Add the workflow-wide policy to frontend types, draft reconciliation, dirty
tracking, coordinated save, and Workflow details. Explain each choice in plain
language and keep the control usable on desktop and touch viewports.

## In scope

- Frontend API/domain types and workflow draft/save paths.
- Inline policy select, read-only behavior, test IDs, and help copy.
- Five locale catalogs and focused unit/component tests.

## Out of scope

- Runtime session routing.
- Browser E2E and public documentation.

## Acceptance

- An unsaved policy survives store refresh and participates in shared dirty/save
  coordination.
- Saving and reloading preserves the selected canonical value.
- The select is disabled but readable for synced workflows and introduces no
  desktop-only interaction.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/settings/use-workflow-settings.test.ts components/settings/workflow-dirty-state.test.ts components/settings/workflow-profile-session-policy.test.tsx && cd web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/lib/types/http.ts`
- `apps/web/hooks/domains/settings/use-workflow-settings.ts`
- `apps/web/hooks/domains/settings/use-workflow-settings.test.ts`
- `apps/web/components/settings/workflow-dirty-state.ts`
- `apps/web/components/settings/workflow-dirty-state.test.ts`
- `apps/web/components/settings/workflow-card.tsx`
- `apps/web/components/settings/workflow-profile-session-policy.tsx`
- `apps/web/components/settings/workflow-profile-session-policy.test.tsx`
- `apps/web/src/locales/*/workflows.json`

## Dependencies

Task 02.

## Risks

- The workflow draft has separate displayed and saved baselines. Omitting either
  creates lost updates or phantom dirty state.
- Self-explaining copy must remain concise enough for the existing workflow card
  at phone width.

## Parallelism

`sequential`

## Inputs

- System-design Frontend and mobile behavior section.
- Existing default-agent-profile select and mobile workflow settings test.

## Results

Implemented the workflow policy in the web type, boot and WebSocket mappings,
draft and saved baselines, dirty tracking, coordinated save, duplication, and
the Workflow details editor. The editor shows all three choices with localized
descriptions and keeps the value readable but disabled for synchronized
workflows. Desktop and mobile controls use the existing responsive select.

Verification:

- Focused web tests passed: 32 tests across 3 files.
- `pnpm run typecheck` passed.
- `pnpm run i18n:check` passed.
- `pnpm run i18n:ratchet` passed.
