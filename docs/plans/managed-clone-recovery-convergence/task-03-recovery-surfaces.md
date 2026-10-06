---
id: "03-recovery-surfaces"
title: "Verify recovery on desktop and phone"
status: done
wave: 3
depends_on:
  - "02-error-projection"
plan: "plan.md"
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
acceptance_criteria:
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.2
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.2
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
  - AC-TASKS-MANAGED-CLONE-RELOCATION-003.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-003.2
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
---

# Task 03: Verify recovery on desktop and phone

## Summary

Consume current relocation details from Restore and durable session updates.
Prove that an older multi-repository task exposes one safe action and resumes
after explicit confirmation on desktop and phone.

## In scope

- Handle restore relocation details in `use-session-recovery-actions.ts` with the
  existing operation fence and shared stamp state. Preserve newer responses and errors.
- Reuse the active recovery model, bootstrap card, confirmation, and action test IDs.
  Remove ineffective actions for the current relocation category, including CANCELLED sessions.
- Keep one disabled action group and one existing localized live status while
  admission waits. Keep retry user initiated after a conflict or deadline result.
- Extend shared E2E fixtures for legacy generic errors and two selected repositories.
  Keep the current single-repository relocation tests unchanged in scope.
- Cover reload, restore-first discovery, one Resume request, ignored-file-only dirtiness,
  explicit confirmation, both repository publications, and a later agent response.
- Update the managed-clone recovery paragraph in `docs/public/git-operations.md`.
  Explain the current action, retained originals, staging limit, and busy refusal.

## Out of scope

New phone navigation, automatic retry, alternate recovery surfaces, unrelated
provider failures, hand-authored live database repairs, and full browser suites.

## Acceptance

1. Resume or Restore exposes one relocation card and current stamp. Reload and
   reconnect preserve that action. A late response cannot replace a newer error.
2. Desktop and phone require explicit confirmation with the staging warning.
   Cancellation performs no transfer. Ordinary desktop controls remain 28 pixels.
   Phone controls measure at least 44 pixels and produce no page horizontal overflow.
3. Successful confirmation validates both selected slots, retains originals and
   content, and continues the same task/session and existing provider conversation.
   A later-slot failure preserves completed progress and blocks agent startup.

## ASCII UI preview

`UI-01` reuses the existing card and confirmation. See the
[full before/after and waiting views](plan.md#ascii-ui-preview).

```text
UI-01 desktop
Workspace needs repair
Files remain in the original checkout.
[Move files and resume] [Technical details]
Dialog: Original and snapshot remain. Staging choices do not transfer.
                                      [Cancel] [Move and resume]

UI-01 phone
Workspace needs repair
Files remain in the old checkout.
[Move files and resume]
[Technical details]
Inset drawer: Original and snapshot remain.
             Staging choices do not transfer.
             [Cancel]
             [Move and resume]
```

`UI-02` keeps the source card visible during manual preflight. Its action group
is disabled and its existing live status announces progress. After a deadline,
the source card remains and a user can retry. No automatic submission occurs.

Control order, one recovery surface, and the staging disclosure are required.
Copy and spacing are illustrative. Use the shipped recovery card and
`managed-clone-relocation-confirmation.tsx` as exemplars. The phone drawer has
one internal scroll owner and safe-area clearance. Cancel returns focus to the card.
This preview maps to AC 002.1, 002.3, 003.1, and 003.2.

## Regression first

Add a hook test named `uses restore relocation details without another resume`
to `use-session-recovery-actions.test.ts`. Before the fix, `handleRestore` ignores
the typed relocation details and does not install the current stamp.
Add late-response and changed-session cases beside it.

Extend `active-session-recovery.test.ts` and both recovery card tests with a
CANCELLED session changing from a legacy generic error to the durable typed error.
Assert exact action counts, one control surface, and the same stamp after hydration.

Add desktop and phone scenarios named `recovers a legacy multi-repository workspace`.
Set up disposable Git state, then enter through the visible recovery controls.
Do not seed `managed_clone_relocation_required` as the expected outcome.
Seed a tracked edit in one checkout and an ignored sentinel in the other.
Check original and replacement bytes, exact HEAD, and both current clone identities.
Arm causal response observers before clicks or taps. Use `.tap()` for phone actions.
Use backend barrier tests from Task 01 as the RED gate for contention.

## Verification

Run from the repository root. Install dependencies once if this worktree lacks them:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-recovery-actions.test.ts hooks/domains/session/use-session-recovery-actions-guard.test.ts lib/active-session-recovery.test.ts lib/services/session-recovery-service.test.ts components/task/chat/session-recovery-card.test.tsx components/task/chat/session-bootstrap-recovery-card.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium tests/session/multi-repo-session-resume-recovery.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/session/mobile-multi-repo-session-resume-recovery.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --host --project chromium tests/session/session-resume-recovery.spec.ts --grep 'moves a dirty managed worktree' --retries=0)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/session/mobile-session-resume-recovery.spec.ts --grep 'moves the dirty worktree through touch confirmation' --retries=0)
(cd apps/web && pnpm run lint)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The guarded E2E commands rebuild the current backend and frontend.
Run them sequentially. Record counts and the fresh-build result.
If copy changes, use `t()` and update English plus all six supported translations.
Generate Traditional Chinese through `pnpm run i18n:zh-hant`. Retain pseudo coverage.
Run documentation coverage through the local `validateCoverage` API described in the plan.

## Implementation files

- `apps/web/components/confirmation/mobile-action-confirmation.tsx`
- `apps/web/components/task/chat/session-recovery-card.test.tsx`
- `apps/web/components/task/chat/session-bootstrap-recovery-card.test.tsx`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/helpers/multi-repo-managed-clone-recovery.ts`
- `apps/web/e2e/tests/session/multi-repo-session-resume-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-multi-repo-session-resume-recovery.spec.ts`
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`
- `apps/web/hooks/domains/session/use-session-recovery-actions.test.ts`
- `apps/web/lib/active-session-recovery.test.ts`
- `docs/public/git-operations.md` and `docs/public/coverage.json`

## Dependencies

Task 02, including durable error publication and structured restore responses.

## Risks

Existing E2E setup modifies shared repository rows. Restore each baseline and
remove disposable sentinels in failure-safe cleanup. A cached mock response does
not prove continuation. Count a new response after recovery and a later prompt.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/managed-clone-relocation.md), criteria listed in frontmatter.
- [Design](../../specs/tasks/system-design/managed-clone-relocation.md#durable-workspace-recovery-error-projection).
- [Plan previews](plan.md#ascii-ui-preview).
- `.agents/skills/mobile-parity/SKILL.md` and `.agents/skills/e2e/SKILL.md`.
- Existing desktop/phone dirty-relocation cases and `seedManagedCloneRelocationFixture`.

## Results

Initial implementation validation: The Restore-response hook regression and
CANCELLED legacy-to-typed recovery model tests passed. All six focused Vitest files
passed (100 tests); typecheck, full web lint, `i18n:check`, and `i18n:ratchet`
passed.

Both new multi-repository browser scenarios passed: desktop Resume and phone
Restore-first, one test each. The existing single-repository dirty-relocation
scenarios also passed on desktop and phone, one test each. Each guarded run
rebuilt the host backend and Vite assets. Phone assertions covered touch actions,
drawer viewport fit, and no horizontal overflow.

Public documentation tests passed (62 tests) and all 47 published pages validated.
Specification catalog validation and lint passed (339 decisions, 1,283
specifications, and 36 linter tests). The PR documentation coverage preflight and
`git diff --check` passed. The only validation limitation is unavailable
PostgreSQL configuration for the Task 02 connection-gated cases.

Review follow-up validation: The rebased focused frontend set passed 95 tests across
six files, including the fixture regression that preserves stopped resumable
executor rows and refuses seeding while a live process is recorded. Typecheck,
full web lint, `i18n:check`, and `i18n:ratchet` passed. After a fresh host build,
the mobile multi-repository Restore-first scenario and desktop multi-repository
recovery scenario each passed (one test each). The existing single-repository
desktop and phone cases had passed earlier in the implementation run. PostgreSQL
execution remains unavailable because `KANDEV_TEST_POSTGRES_DSN` is unset.

The rebased specification catalog validated 343 decisions and 1,309
specifications; all 36 specification-linter tests passed. Public documentation
tests passed (62 tests) and all 47 published pages validated.
