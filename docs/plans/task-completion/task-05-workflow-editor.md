---
id: "05-workflow-editor"
title: "Expose workflow completion settings"
status: done
wave: 5
depends_on: 
  - 04-session-resume
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-001
acceptance_criteria:
  - AC-TASKS-COMPLETION-001.1
  - AC-TASKS-COMPLETION-001.2
  - AC-TASKS-COMPLETION-001.3
  - AC-TASKS-COMPLETION-001.4
  - AC-TASKS-COMPLETION-001.7
  - AC-TASKS-COMPLETION-001.13
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 05: Expose workflow completion settings

## Summary

Let authors configure task completion in the existing workflow step editor on desktop and phone. Preserve the choice through drafts, state delivery, creation, and duplication.

## In scope

- Carry the boolean through backend/http frontend types, workflow API, boot/WS merges, workspace actions, draft cloning/save/discard, workflow creation and duplication. Partial updates must not discard it.
- Show the localized checkbox only in the final step's entry/general settings, including when it has no turn-complete actions. Put its explanation behind an adjacent info icon with desktop hover/focus and mobile tap support. Respect read-only sync.
- Follow the design's mobile contract: use the existing focused step navigation, a wrapping 44 px tappable label, touch-accessible info help, shared Save changes, and no document overflow. Opening help must not toggle the checkbox.
- Update workflow public docs and portable/sync reference pages for the new setting and version-2 format. Keep version-1 compatibility explicit.

## Out of scope

The completed-chat banner is owned by Task 06. No redesign of the workflow settings page.

## Acceptance

- True/false survive save, reload, discard, duplication, workflow creation, boot and WebSocket update; unrelated step settings remain unchanged.
- Only the final step exposes the checkbox and info icon on desktop and mobile. The new final step uses its own saved value after reorder; earlier steps cannot complete work. Read-only workflows remain protected.
- Focused browser tests demonstrate actual completion and successful follow-up on an unchecked final step; all five language catalogs pass.

## TDD entry

Add draft/API/WS regressions first, including omission after a saved true and reorder/save/discard visibility. Write workflow-task-completion.spec.ts and mobile-workflow-task-completion.spec.ts before the checkbox; run each RED on the missing control, then implement and rerun. Assert no checkbox or help icon on non-final steps. Cover info disclosure by hover, keyboard focus, and tap without toggling the value.

## Verification

Use the worktree dependency setup recorded below.

```bash
rtk pnpm install --frozen-lockfile
rtk pnpm --filter @kandev/web exec vitest run components/settings/workflow-card-actions.test.ts app/settings/workspace/workflow-duplication.test.ts app/settings/workspace/use-workflow-creation.test.ts app/actions/workspaces.test.ts lib/api/domains/workflow-api.test.ts lib/ws/handlers/workflows.test.ts
rtk pnpm --filter @kandev/web typecheck
rtk pnpm --filter @kandev/web i18n:zh-hant
rtk pnpm --filter @kandev/web i18n:check
```

Run the commands above from apps/; install once before the first pnpm command in this worktree. Add workflows.test.ts if no existing test owns partial workflow-step merges.

From apps/web/, run sequentially:

```bash
rtk pnpm e2e:run --project chromium tests/workflow/workflow-task-completion.spec.ts tests/workflow/workflow-duplication.spec.ts tests/workflow/workflow-import-export.spec.ts tests/workflow/workflow-children-completed.spec.ts -- --retries=0
rtk pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-task-completion.spec.ts tests/workflow/mobile-workflow-settings.spec.ts tests/workflow/mobile-workflow-duplication.spec.ts -- --retries=0
```

From the repository root:

```bash
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
```


## Files likely touched

- `apps/web/lib/types/backend.ts`
- `apps/web/lib/types/http.ts`
- `apps/web/lib/api/domains/workflow-api.ts`
- `apps/web/lib/ws/handlers/workflows.ts`
- `apps/web/components/settings/workflow-card-actions.ts`
- `apps/web/components/settings/workflow-pipeline-editor-step-actions.tsx`
- `apps/web/app/actions/workspaces.ts`
- `apps/web/app/settings/workspace/use-workflow-creation.ts`
- `apps/web/app/settings/workspace/workflow-duplication.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/workflows.json`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/pages/workflow-settings-page.ts`
- `apps/web/e2e/tests/workflow/workflow-task-completion.spec.ts (new)`
- `apps/web/e2e/tests/workflow/mobile-workflow-task-completion.spec.ts (new)`
- `docs/public/workflow-tips.md`
- `docs/public/workflow-import-export.md`
- `docs/public/workflow-sync.md`

## Dependencies

Complete 04-session-resume first.

## Risks

Do not hide the checkbox under transitionType !== none. Set explicit completion values in test seeds that relied on Done naming. Keep shared dirty/save behavior and avoid broadening desktop touch dimensions. Do not rewrite public docs as though opening a completed task resumes it.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-completion.md), criteria in frontmatter.
- [System design](../../specs/tasks/system-design/task-completion.md).
- [Plan](plan.md), confirmed source trace and existing reproduction tests.
- Nearby workflow settings and session recovery tests supply fixture conventions.
- Follow `/tdd`, `/mobile-parity`, and `/e2e` in the primary session.

## Results

The workflow editor now exposes `Complete task when entering this step` only on
the final step, with an adjacent localized info disclosure. The value remains
per-step across reorder, save, reload, discard, creation, duplication, boot,
WebSocket updates, and version-2 export/import. Non-final values remain stored
but inactive, and read-only synced workflows stay protected.

Final verification passed:

```text
workflow/API/WS unit gate: 55 tests in 6 files
workflow editor focused unit gate: included in 75 frontend tests
web typecheck: passed
i18n catalog and pseudo-locale gate: passed
desktop workflow E2E: 12 tests
mobile workflow E2E: 8 tests
public-doc validator tests: 61 tests
published-doc validation: 46 pages
```

The public workflow tips, import/export, sync, and MCP documentation describe
the final-step rule, version-2 explicit booleans, version-1 compatibility, and
completed-conversation follow-ups.
