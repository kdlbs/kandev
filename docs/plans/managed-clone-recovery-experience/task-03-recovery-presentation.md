---
id: "03-recovery-presentation"
title: "Present recovery progress and working repositories"
status: completed
wave: 3
depends_on:
  - "01-private-artifacts"
  - "02-durable-progress"
plan: "plan.md"
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-004
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-005
acceptance_criteria:
  - AC-TASKS-MANAGED-CLONE-RELOCATION-003.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-003.2
  - AC-TASKS-MANAGED-CLONE-RELOCATION-004.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-004.2
  - AC-TASKS-MANAGED-CLONE-RELOCATION-004.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-004.4
  - AC-TASKS-MANAGED-CLONE-RELOCATION-004.5
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.2
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.4
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.5
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.6
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.7
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.8
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
  - ../../specs/tasks/system-design/managed-clone-relocation-experience.md
---

# Task 03: Present recovery progress and working repositories

## Summary

Connect shared recovery controls to server progress and show repository display
names in Files. Prove reload, interruption, completion, and canonical file
actions on desktop and phone. Document the verified behavior.

## In scope

- Update the recovery model and shared action hook across active, stopped,
  bootstrap, and message surfaces. Preserve history without duplicate active cards.
- Use local pending state only before acknowledgment. Reconcile lost responses
  through status lookup and disable competing controls while status is unresolved.
- Render repository position, phase, last update, retained copies, and terminal
  outcome. Separate migrated files from agent readiness and actual resume failures.
- Apply exact repository labels without changing paths. Fence tree/search requests
  against inventory changes and refresh them after publication or registry changes.
- Reuse desktop cards/dialogs and focused phone cards/inset drawers. Add accessible
  phase announcements, 44-pixel phone targets, focus return, and long-name behavior.
- Localize copy in all seven supported locales, including plural handling.
- Extend real-Git desktop/mobile E2E suites with an isolated phase barrier.
- Update public Files and executor recovery documentation after rendered verification.
  Reconcile package statuses and results without changing companion historical counts.

## Out of scope

New backup/cancel controls, automatic retry or cleanup, physical directory
renames, progress percentages, and copy optimization.

## Acceptance

1. Reload and disconnect recover server progress on every recovery surface.
   No second transfer occurs, and partial migration never reports agent readiness.
2. Files and searches address canonical active checkouts with clear labels.
   Registered artifacts are excluded, while user lookalikes remain reachable.
3. Desktop and phone pass the plan's browser matrix, geometry, accessibility,
   and localization checks. Public docs describe the verified behavior and limits.

## ASCII UI preview

Use [UI-01 and UI-02 in the combined plan](plan.md#ascii-ui-preview).
The same labels apply here. Copy is illustrative; structure is required.

```text
UI-01 desktop                      UI-01 phone
+--------------------------------+ +-------------------------------+
| Moving workspace files         | | < Task                        |
| Repository 2 of 2: landing      | | Moving workspace files        |
| Verifying saved files          | | Repository 2 of 2             |
| Last update: 13:00              | | landing                       |
| Original files remain saved.   | | Verifying saved files         |
| [Move and resume: disabled]    | | Last update: 13:00             |
| [Technical details v]          | | Original files remain saved.  |
+--------------------------------+ | [Move and resume: disabled]   |
                                   | [Technical details v]         |
                                   +-------------------------------+

UI-02 desktop Files                UI-02 focused phone Files
  .agents/                         < Task                 Files
  .codex/                          Search files
  kdlbs-kandev/                    > kdlbs-kandev
  kdlbs-landing/                   > kdlbs-landing
  notes.txt                        notes.txt
```

Recovery uses the existing single scroll owner. Phone controls have 44-pixel
targets and safe-area spacing; desktop controls retain compact sizes. Existing
dialog/drawer confirmation stays unchanged in purpose. The interrupted action
appears only with current admission proof. Unresolved status disables repair.
After all files move, show resuming separately. Successful readiness retires
the active card; errors retain their existing safe actions.

File labels retain canonical paths. Phone keeps its focused Files navigation.
Unknown artifacts remain visible. Map UI-01 to AC-005.1-.8 and UI-02 to AC-004.1-.5.
Compare actual rendered geometry to these structural choices, not ASCII spacing.

## TDD and regression evidence

Extend `use-session-recovery-actions.test.ts`, its guard tests,
`session-recovery-service.test.ts`, `session-recovery-model.test.ts`, and
recovery card/bootstrap tests before implementation. Cover old error stamps,
late responses, pending hydration, unknown phases, resume failure, and interruption.

Add `file-browser-repository-labels.test.ts` for exact path matching and duplicate
names. Extend path and search-freshness tests for stale inventory responses.
Use the existing real-Git multi-repository helper for every scenario in the plan.
The isolated phase barrier must operate on actual backend recovery, not a mocked
success notification. Release barriers and clean fixture-owned resources on failure.

## Verification

Run from the repository root. Task 02 installs dependencies. The managed runner
rebuilds changed frontend/backend artifacts; run desktop and phone sequentially.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-recovery-actions.test.ts hooks/domains/session/use-session-recovery-actions-guard.test.ts lib/services/session-recovery-service.test.ts components/task/chat/session-recovery-model.test.ts components/task/chat/session-recovery-card.test.tsx components/task/chat/session-bootstrap-recovery-card.test.tsx components/task/file-browser-repository-labels.test.ts components/task/file-browser-path.test.ts components/task/file-browser-search-freshness.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium tests/session/multi-repo-session-resume-recovery.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/session/mobile-multi-repo-session-resume-recovery.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --host --project chromium tests/session/session-resume-recovery.spec.ts --grep 'moves a dirty managed worktree' --retries=0)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/session/mobile-session-resume-recovery.spec.ts --grep 'moves the dirty worktree through touch confirmation' --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

If backend test-only barrier wiring changes in this order, run the targeted
orchestrator/service tests from Task 02 again before browser verification.
Record actual tests, skips, viewport evidence, and localization results. Do not
run overlapping full suites or pass an all-worker override. Rendered assertions
must cover phone scroll ownership, long labels, containment, focus, and touch.

## Files likely touched

- `apps/web/hooks/domains/session/session-recovery-pending.ts`
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`
- `apps/web/lib/services/session-recovery-service.ts`
- `apps/web/components/task/chat/session-recovery-model.ts`
- `apps/web/components/task/chat/session-recovery-card.tsx` and current bootstrap/stopped/message consumers.
- `apps/web/components/task/file-browser.tsx`, `file-browser-data.ts`, `file-browser-hooks.ts`, and header/row helpers.
- `apps/web/lib/types/workspace-files.ts` if display metadata is needed; path semantics stay unchanged.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/` catalogs.
- Test files listed in Verification, plus current workspace-recovery card tests.
- `apps/web/e2e/helpers/multi-repo-managed-clone-recovery.ts`
- `apps/web/e2e/tests/session/multi-repo-session-resume-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-multi-repo-session-resume-recovery.spec.ts`
- Isolated backend E2E fixture hooks for deterministic phase hold and restart.
- `docs/public/developer-tools.md`, `docs/public/executors.md`, and this package's status/results.

## Dependencies

Task 01 supplies artifact exclusions. Task 02 supplies status hydration,
notifications, liveness guards, and monotonic projection merging.

## Risks

Clearing local pending state before status reconciliation can briefly offer
another transfer. Display labels must not become file identifiers. Localization
and long repository names can change wrapping and touch geometry on phones.

## Parallelism

`sequential`

## Inputs

- [Plan evidence, UI previews, and E2E matrix](plan.md).
- Design sections: Shared recovery presentation; Files and workspace search.
- Existing `SessionRecoveryCard` and `ManagedCloneRelocationConfirmation` phone
  card/inset drawer as the closest shipped exemplar.
- Mobile UI language and control sizing references in the mobile-parity skill.
- Existing companion convergence and permission E2E scenarios.

## Results

Implemented durable recovery cards and guards across active, stopped, bootstrap,
and message surfaces, repository display labels with canonical file paths, and
inventory-aware Files/search refresh. Added seven-locale copy and updated the
public Files and executor recovery guidance.

The focused recovery web suites passed 177/177 tests, the changed recovery
projection/card suites passed 94/94, and the six projection tests passed.
Typecheck, full ESLint, `i18n:zh-hant`, `i18n:check`, and `i18n:ratchet` passed.
The production Vite build passed with existing chunk-size and ineffective
dynamic-import warnings. Desktop multi-repository recovery passed 2/2 and phone
multi-repository recovery passed 2/2. Targeted single-repository desktop and
phone cases each passed 1/1. Public documentation tests passed 62/62 and all
47 published pages validated. Spec/catalog validation and `git diff --check`
passed. The PostgreSQL gate remains blocked because its test DSN is unset.
