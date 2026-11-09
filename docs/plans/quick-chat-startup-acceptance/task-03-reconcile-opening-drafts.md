---
id: "03-reconcile-opening-drafts"
title: "Reconcile Quick Chat opening drafts"
status: done
wave: 3
depends_on: ["02-recover-keyed-creation"]
plan: "plan.md"
requirements:
  - REQ-TASKS-QUICK-CHAT-COMPOSER-001
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003
acceptance_criteria:
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.8
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.9
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.10
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.11
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.12
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.13
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.14
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.15
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.16
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.1
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.3
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.4
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.5
system_design:
  - ../../specs/tasks/system-design/quick-chat-opening-composer.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 03: Reconcile Quick Chat opening drafts

## Summary

Preserve one pending creation and opening payload across uncertain HTTP results.
Reconcile it into the existing conversation without duplicate work or lost edits.
Prove the same outcome on desktop and phones.

## In scope

- Add typed acceptance/status API support and optional store creation identities.
- Persist an immutable pending submission before POST and reuse its creation/message UUIDs.
- Reconcile HTTP-first and WS-first results, explicit lookup/retry, and same-browser reload.
- Gate passive resumption and opening-message admission while the accepted startup owns the session.
- Add localized unknown/incomplete status controls and retain existing session progress/recovery.
- Preserve newer drafts, auth/workspace generations, dismissal, and deletion tombstones.
- Retire a pending receipt only after an authoritative typed pre-allocation rejection; preserve it for untyped HTTP, transport, or status-lookup outcomes.
- Add browser regressions and update the public recovery how-to when implementation lands.

## Out of scope

Automatic message replay, cross-device drafts, a tab redesign, provider admission changes,
and configuration-chat redesign.

## Acceptance

1. A dropped or malformed creation response resolves by the exact token. It produces one conversation and at most one opening message.
2. Reload, stale results, navigation, and explicit deletion preserve payload and activation ownership. Status reads never send work.
3. Desktop and phone expose pending, accepted, and failed states in existing surfaces. Phone actions meet the preview's reachability and touch contract.

## ASCII UI preview

See [UI-01, UI-02, and UI-03](plan.md#ascii-ui-preview) for the full context.
The following excerpt uses the same view labels.

```text
UI-01 Desktop accepted: [Existing | Conversation* | +]
                       Preparing repository / setup progress
                       Existing conversation composer

UI-02 Unknown desktop: Draft and files -> status -> [Check status]
UI-02 Unknown phone:   Fixed Quick Chat navigation
                       Draft and files / status / [Check status]
                       One internal scroll body / safe-area inset

UI-03 Accepted phone:  Fixed Quick Chat navigation
                       Preparation rows / conversation scroll body
                       Existing composer / safe-area inset
```

Status lookup is the primary unknown-state action.
Phone controls measure at least 44px. Desktop controls retain compact sizing.
Copy is illustrative. Preserve all locale and accessible-name contracts.

## Verification

Write the dropped non-JSON 502 regression before changing the hook.
Include malformed 200, delayed found/unsettled state, lookup errors, same-token
retry, different-owner candidates, newer edits, and deletion races.
Keep the existing `clientMessageId` through initial delivery and explicit retry.
After reload, assert no automatic POST or attempted-message replay.
An untyped creation 404 and a status-lookup 404 remain uncertain. Only the
typed pre-allocation rejection retires the receipt while retaining the editable
draft and allowing corrected selections.

Add the two startup-acceptance browser files named in the plan.
Use real POST allocation before response replacement, causal waits, disposable
fixtures, and touch input on the phone project. Keep existing companion suites.
Install workspace dependencies first if this checkout has no `apps/node_modules`.

Run these commands from the repository root, sequentially:

```bash
(cd apps/web && pnpm exec vitest run components/quick-chat lib/api/domains/workspace-api-retention.test.ts lib/state/slices/ui/quick-chat-actions.test.ts lib/state/slices/ui/quick-chat-sync.test.ts lib/ws/handlers/tasks-quick-chat.test.ts hooks/use-quick-chat-resync.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/quick-chat-startup-acceptance.spec.ts tests/chat/quick-chat-opening-composer.spec.ts tests/chat/setup-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-startup-acceptance.spec.ts tests/chat/mobile-quick-chat-opening-composer.spec.ts tests/chat/mobile-setup-recovery.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add a new dedicated API test file to the Vitest command if its coverage is
not included by an existing listed file. Add every changed store test to that
command before recording completion. Managed E2E builds the current source.
Do not overlap suites or bypass worker limits.

## Files likely touched

- `apps/web/lib/api/domains/workspace-api.ts` and retention/creation tests
- `apps/web/components/quick-chat/use-quick-chat-modal.ts` and tests
- `apps/web/components/quick-chat/use-quick-chat-setup-draft.ts` and tests
- `apps/web/components/quick-chat/quick-chat-setup.tsx` and tests
- `apps/web/components/quick-chat/quick-chat-modal.tsx` and tests
- Existing Quick Chat initial-prompt and recovery hooks, only where ownership requires changes
- `apps/web/components/quick-chat/quick-chat-session-view.tsx` and recovery-gate tests
- `apps/web/lib/state/slices/ui/types.ts`, opening-draft helpers, actions, and sync tests
- `apps/web/lib/ws/handlers/quick-chat.ts` and task lifecycle tests
- New `apps/web/e2e/tests/chat/quick-chat-startup-acceptance.spec.ts`
- New `apps/web/e2e/tests/chat/mobile-quick-chat-startup-acceptance.spec.ts`
- Existing opening-composer and setup-recovery browser scenarios as compatibility requires
- All seven locale catalogs for new status/action copy
- `docs/public/tasks-and-workflows.md` recovery how-to

## Dependencies

Tasks 01 and 02. The API must return exact acceptance identity before UI recovery uses it.

## Risks

A session event can arrive before terminal opening-payload ownership is accepted.
Do not clear its draft from that event alone.
No matching candidate is safer than a guessed chat.
Confirmed deletion retires the creation token and requires attachment revalidation.

## Parallelism

`sequential`

## Inputs

- [Browser reconciliation](../../specs/tasks/system-design/quick-chat-opening-composer.md#browser-reconciliation-and-payload-ownership)
- [Responsive preview](plan.md#ascii-ui-preview)
- Existing opening-composer, setup-recovery, initial-prompt, and generation tests

## Results

Implemented persistent keyed submission recovery, exact identity reconciliation,
opening-message ownership, startup admission guards, localized recovery states,
and desktop/phone presentation. The public tasks and workflows guide now
documents status checking, same-token retry, startup progress, and reload
behavior.

Verification passed:

- Current-base frontend Vitest: 35 files and 285 tests across Quick Chat,
  creation and retention APIs, changed UI stores, task lifecycle handlers, and
  Quick Chat resync.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`.
- Desktop Chromium managed E2E: 8 tests passed, including startup acceptance, opening composer, and setup recovery.
- Phone `mobile-chrome` managed E2E: 4 tests passed, including startup acceptance, opening composer, and setup recovery.
- Both managed E2E commands built the current production Vite assets and backend fixture.
- `node scripts/validate-public-docs.mjs`: 54 published documentation pages validated.
- `python3 scripts/list-docs.py validate`: 404 decisions and 1,742 specifications validated.
- `python3 scripts/lint-spec-files.py --all` and `git diff --check` passed.

Final revalidation on 2026-10-27 also passed the changed-file frontend ESLint run
with zero errors, the Prettier check, and the changed-file Go linter with zero
issues. A rendered phone recovery screenshot was inspected; the temporary
screenshot probe and file were removed.

PR review fixup revalidation on 2026-10-27:

- `pnpm exec vitest run components/quick-chat lib/api/domains/workspace-api-retention.test.ts lib/state/slices/ui/quick-chat-actions.test.ts lib/state/slices/ui/quick-chat-sync.test.ts lib/ws/handlers/tasks-quick-chat.test.ts hooks/use-quick-chat-resync.test.ts`: 34 files and 290 tests passed.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- From `apps/web`, `pnpm exec eslint components/quick-chat/quick-chat-setup.test.tsx components/quick-chat/quick-chat-startup-state.ts components/quick-chat/use-agent-selection-retention.test.ts components/quick-chat/use-quick-chat-agent-selection.ts components/quick-chat/use-quick-chat-creation.test.tsx components/quick-chat/use-quick-chat-creation.ts components/quick-chat/use-quick-chat-modal.ts e2e/tests/chat/quick-chat-startup-acceptance.spec.ts` and the matching `pnpm exec prettier --check` command passed. `pnpm run build` succeeded.
- `(cd apps/web && pnpm e2e:run --project chromium tests/chat/quick-chat-startup-acceptance.spec.ts tests/chat/quick-chat-opening-composer.spec.ts tests/chat/setup-recovery.spec.ts)`: 8 tests passed.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-startup-acceptance.spec.ts tests/chat/mobile-quick-chat-opening-composer.spec.ts tests/chat/mobile-setup-recovery.spec.ts)`: 4 tests passed. Both commands built the current backend fixture and production web assets.
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed. `node scripts/validate-public-docs.mjs`: 54 pages validated.
- `python3 scripts/list-docs.py validate`: 404 decisions and 1,746 specifications validated. `python3 scripts/lint-spec-files.py --all` and `git diff --check` passed.
- A mobile Configuration Chat regression exposed the shared view applying ordinary Quick Chat startup blocking to configuration sessions. The startup gate now applies only to ordinary chat tabs. Its component regression passed, and `pnpm e2e:run --project mobile-chrome tests/settings/mobile-configuration-chat.spec.ts` passed both tests.
