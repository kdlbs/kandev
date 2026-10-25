---
id: "04-creation-composer-adapters"
title: "Adapt creation composers"
status: done
wave: 2
depends_on: ["01-publish-composer-contract"]
plan: "plan.md"
spec: "../../specs/plugins/requirements/voice-extraction-host.md"
---

# Task 04: Adapt Creation Composers

## Acceptance

- Task creation and new-session creation render their declared plugin action slots on desktop and
  native mobile with live prompt state and relevant identifiers.
- Selection insertion reuses one spacing algorithm and restores focus/caret; plugin submit invokes the
  exact native form handler with existing busy and validation gates.
- Tests cover selected-range replacement, blocked submit, stale dialog handles, successful native
  task/session creation delegation, and cleanup on close.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog-selectors.test.tsx components/task-create-dialog.test.tsx components/task/new-session-dialog.test.tsx
cd apps/web && pnpm run typecheck
```

Follow RED-GREEN-REFACTOR. Do not make Task Create or New Session send a chat message directly.

## Files Likely Touched

- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-selectors.tsx`
- `apps/web/components/task-create-dialog-form-body.tsx`
- `apps/web/components/task-create-dialog.tsx`
- `apps/web/components/task/new-session-dialog.tsx`
- new shared creation-composer plugin action component/helper
- focused tests beside these files

## Mobile Design Contract

Keep actions inside the existing prompt toolbar and full-height responsive dialogs. Preserve the
dialog's scroll owner, safe-area behavior, primary submit hierarchy, and >=44px touch targets.

## Risks

- Programmatic form submission must not bypass native validation or submit while the form is blocked.
- Task Create and New Session share input UI but have different submit ownership; avoid a false shared
  callback that drops either form's gates.
