---
id: "02-dirty-recovery"
title: "Recover dirty worktrees"
status: complete
wave: 2
depends_on:
  - "01-clean-relocation"
plan: "plan.md"
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
acceptance_criteria:
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.2
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
  - AC-TASKS-MANAGED-CLONE-RELOCATION-003.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-003.2
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
---

# Task 02: Recover dirty worktrees

## Summary

Add a stamped repair action for a dirty, verified legacy worktree. Reuse the
existing snapshot and claim path. Show one truthful recovery choice on desktop
and phone, then resume the same provider conversation after a successful move.

## In scope

- Add `relocate_and_resume` to session recovery with authorization, stamp
  fencing, and exact selected-environment validation.
- Preserve files and commit history in a sibling replacement. Retain the
  original checkout and snapshot. Report staging limits.
- Add a typed path-free mismatch cause and action eligibility to the existing
  recovery model.
- Add localized desktop and phone confirmation, public Git recovery guidance,
  and focused E2E evidence.

## Out of scope

- Automatic dirty relocation and branch-loss recovery changes.
- Deletion of retained originals and snapshots.

## Acceptance

1. Resume and Start fresh do not modify a dirty mismatched checkout or clear
   its provider token. Only a current explicit action can start transfer.
2. The transfer preserves the specified files, modes, and exact commit. A
   failed or interrupted transfer retains the original and blocks agent start.
3. Desktop and phone show the same eligible action and staging warning. Both
   continue the original task and conversation after success.

## ASCII UI preview

`UI-01` uses the existing recovery surface. See the [full preview](plan.md#ascii-ui-preview).

```text
Desktop: [Workspace needs repair] [Move files and resume] [Technical details]
Phone:   [Workspace needs repair]
         [Move files and resume]
         [Technical details]
Confirm: Original and snapshot remain. Staging choices do not transfer.
         [Cancel] [Move and resume]
```

The phone confirmation uses an inset bottom drawer with one scroll owner and
44-pixel controls. The wording is illustrative and uses localized copy. This
preview covers `AC-TASKS-MANAGED-CLONE-RELOCATION-002.1`, `.002.3`, and `.003.2`.

## Verification

From the repository root:

```bash
(cd apps/backend && go test ./internal/worktree ./internal/orchestrator ./internal/orchestrator/handlers ./internal/task/service -run 'TestManagedCloneRelocation' -count=1)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium tests/session/session-resume-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/session/mobile-session-resume-recovery.spec.ts)
node scripts/validate-public-docs.mjs
git diff --check
```

The guarded E2E runner builds the current frontend and backend. Run the two
projects sequentially. Do not overlap full suites.

## Files likely touched

- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/worktree/` recovery files
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`
- `apps/web/components/task/chat/session-recovery-model.ts`
- `apps/web/components/task/chat/session-bootstrap-recovery-content.tsx`
- `apps/web/e2e/tests/session/session-resume-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-resume-recovery.spec.ts`
- `apps/web/src/locales/` supported catalogs
- `docs/public/git-operations.md`

## Dependencies

Task 01.

## Risks

The snapshot copy does not reconstruct staging choices. The UI must show this
limit before the user requests a move. Source proof and operation stamps must
not leak paths or provider credentials in public errors.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/managed-clone-relocation.md)
- [System design](../../specs/tasks/system-design/managed-clone-relocation.md)
- [Plan preview](plan.md#ascii-ui-preview)
- Existing desktop and phone session recovery E2E files.

## Results

Implemented stamp-authorized dirty relocation, retained snapshots/originals,
and localized desktop and phone confirmation. The dirty worktree E2E fixture
now uses the legacy owner/name source-clone layout. Its explicit recovery path
passed on desktop and phone with retries disabled. Desktop controls assert the
28-pixel default size, while phone controls assert the 44-pixel touch target;
the phone confirmation capture scrolls its action controls into view.
Frontend typecheck and recovery UI tests (79 tests) passed, as did i18n checks,
the earlier full desktop recovery E2E (2 tests), and mobile recovery E2E (5
tests). Public docs validation and tests (62 tests), specification validation,
spec lint, and `git diff --check` passed. In final fixup verification, the
dirty relocation flow passed once on Chromium and once on mobile-chrome with
retries disabled; both tests waited for the resumable runtime row to become
durably `stopped` before offering relocation.
The mobile clarification E2E now displays an update toast during the flow and
checks that it does not cover the submit control. Mobile toast placement was
split by purpose: update notices use a separate top position, while ordinary
action toasts keep their bottom position. Toast surfaces pass pointer
events through to the underlying controls. The mobile clarification, mobile
sidebar archive, and mobile thread-action E2Es passed after this change.
