---
id: "02-contextual-setup"
title: "Preserve setup context and create keys in place"
status: done
wave: 2
depends_on:
  - "01-nested-navigation"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
  - REQ-EXECUTORS-SSH-CONNECTIONS-005
acceptance_criteria:
  - AC-EXECUTORS-SSH-CONNECTIONS-001.1
  - AC-EXECUTORS-SSH-CONNECTIONS-001.2
  - AC-EXECUTORS-SSH-CONNECTIONS-004.1
  - AC-EXECUTORS-SSH-CONNECTIONS-004.3
  - AC-EXECUTORS-SSH-CONNECTIONS-004.4
  - AC-EXECUTORS-SSH-CONNECTIONS-004.5
  - AC-EXECUTORS-SSH-CONNECTIONS-004.7
  - AC-EXECUTORS-SSH-CONNECTIONS-005.2
  - AC-EXECUTORS-SSH-CONNECTIONS-005.6
system_design:
  - ../../specs/executors/system-design/ssh-connection-manager.md
---

# Task 02: Preserve setup context and create keys in place

## Summary

Let an administrator create a key from the connection identity field and return
to the same connection. Preserve the managed executor draft during the setup
round trip, with explicit selection, testing and saving on return.

## In scope

- Save connection metadata before a contextual identity detour; reuse the
  existing identity form and return on create/cancel. Preserve outer adoption
  and bastion context; consume a returned identity selection only once.
- One in-memory managed-editor continuation in the persistent settings shell,
  outside the pathname-keyed save provider, with a scoped transfer of draft/save ownership. Implement its lifecycle,
  baseline conflict handling, explicit Return/Use actions and focus restoration.
- Desktop/phone flows, keyboard/touch behavior, localization, focused regression
  and browser tests. Update the public setup recipe and draft-lifetime wording.

## Out of scope

New APIs or persisted draft storage, legacy editor redesign, general workflow
continuations, automatic profile save/testing, automatic trust/enrollment,
key export, or live host changes. Do not change backend secret custody.

## Acceptance

1. Create/cancel identity returns to the saved connection without losing target
   fields or outer context. Failure, dismissal and late results neither navigate
   unexpectedly nor replay generation/import. Secret inputs are never recovered.
2. A managed executor draft survives Manage connections and browser Back/Return
   within the flow. Only Use connection changes selection; it requires a ready
   destination. Returning clears executor-check success. Stale server baselines
   require reconciliation, and no return action saves a profile.
3. Desktop/phone checks prove cleanup on save/discard/exit/logout/reload, single
   scroll ownership, accessible focus/actions, localization and updated public
   instructions. Existing identity and enrollment tests continue to pass.

## UI preview

UI-02 excerpt; see [full preview and states](plan.md#ui-02-finish-setup-in-context).
Structured preview follows the task's rich-output requirement.

| Step | Desktop | Phone |
| --- | --- | --- |
| Connection | Identity picker; Save draft and create identity | Picker Drawer; creation opens focused page |
| Identity | Existing form with Create / Cancel | Full-page form; Back/Cancel clears secret inputs |
| Return | New key selected in unsaved connection draft | Same destination details, focus at identity picker |
| Executor | Return to executor / Use connection when ready | Separate touch actions; return to preserved editor |
| Final action | Test executor, then shared Save changes | Same order, safe-area and keyboard clearance |

Action order, effect boundaries and preservation are required. Labels are
illustrative. Loading/failure stays on the current step; stale data offers
reload/reconciliation. ACs: 001.1, 001.2, 004.1, 004.3-004.5, 004.7, 005.2, 005.6.

## Verification

Start with failing behavior tests under `/tdd`. New filenames below are proposed;
create and run them, then implement. Run from repository root and execute
browser projects sequentially. Re-run the navigation specs because this order
extends the same route context.

```bash
(cd apps/web && pnpm exec vitest run components/settings/ssh-connections hooks/domains/settings/use-ssh-connections.test.tsx components/settings/settings-save-provider.test.tsx components/settings/settings-save-exclusive.test.tsx lib/settings/ssh-connection-settings-routes.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/ssh-navigation.spec.ts tests/settings/ssh-connections.spec.ts tests/settings/ssh-connections-disabled.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-ssh-navigation.spec.ts tests/settings/mobile-ssh-connections.spec.ts)
(cd apps/web && pnpm e2e:run --project auth tests/auth/ssh-connections-permissions.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

New component tests: `connection-details.test.tsx`, `setup-continuation.test.tsx`
and `managed-binding.test.tsx`, beside their implementations. Cover empty name,
failed save, invalid return IDs, nested detours, concurrent edits, dismissed
creation, duplicate submissions, changed route readiness, both new/existing
editor return, rejected navigation and continuation cleanup. Assert no secret
fields in the continuation shape or browser storage/URL. Existing identity-form
and enrollment tests cover their command/secret contracts.

Browser tests must drive identity creation through the UI against a disposable
backend. Use metadata-only fixtures for the return-selection branch and label
any stubbed executor-test response as UI coverage, not real SSH evidence.
Exercise create/cancel and browser Back; verify saved connection fields after
reload and document that an unsaved executor draft is intentionally lost then.
Include pseudo-locale, narrow breakpoint, coarse-tablet and focus assertions.
No real enrollment is needed for these outcomes.

## Files likely touched

Paths are relative to `apps/web/` unless prefixed with `docs/`.

- `components/settings/ssh-connections/{settings,connection-details,managed-binding,executor-editor,identity-form}.tsx`
- `components/settings/ssh-connections/setup-continuation.tsx` and `.test.tsx` (new)
- New `connection-details.test.tsx`, `managed-binding.test.tsx` beside components
- `components/settings/settings-layout-client.tsx` for the scoped provider
- `components/settings/settings-save-provider.tsx` only if its existing scoped
  coordination API cannot transfer draft ownership; extend narrowly with tests
- `lib/settings/ssh-connection-settings-routes.ts` and `.test.ts`
- `e2e/tests/settings/{ssh-connections,mobile-ssh-connections}.spec.ts`
- `e2e/helpers/ssh-identities.ts` and focused setup helper if shared
- `src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/` affected catalogs
- `docs/public/executors.md`

## Dependencies

Task 01 supplies the canonical route and route-context helper.

## Risks

A general navigation bypass could drop unrelated drafts. A returned identity
candidate could overwrite newer input if replayed. Guard both at the scoped
handoff/consumption boundary. Do not retain secret-bearing forms in the shell.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/ssh-connection-manager.md), ACs above.
- [Settings design](../../specs/executors/system-design/ssh-connection-manager.md#settings-and-mobile-composition).
- Existing `useConnectionDraft`, `IdentityForm` cancellation tests,
  `SettingsSaveProvider` guard/exclusive tests, manager navigation and mobile picker.

## Results

Completed locally on 2026-10-23. Connection metadata is saved before identity
creation; create/cancel returns to that connection and preserves outer context.
The public identity candidate is consumed once without an automatic save.
Browser Back now returns to the saved record instead of an empty create form.
Managed new/existing executor drafts retain name, connection and new-profile
owner in memory. Return/Use require a fresh executor test; a changed baseline
blocks saving. Draft ownership transfer does not bypass unrelated save guards.

RED: save-before-detour, continuation and managed-return cases failed before
implementation. A browser Back regression also failed before the history fix.
GREEN: combined focused suite passed 197 tests; final guard/save cleanup tests
passed in an 8-test binding subset (two additional cases). Coverage includes
empty names, duplicate saves, failed and late saves, invalid identity candidates,
context composition, stale readiness, changed baseline, new/existing return,
rejected navigation and clear-on-save/discard/exit/logout/unmount.

Real disposable-backend browser tests passed create/cancel, Back, identity
selection/focus, preserved metadata after reload and executor draft lifetime on
desktop and phone. Labelled UI fixtures passed explicit selection/fresh-check and
concurrent-editor recovery in both projects. Conflict screenshots were inspected.
No tests enrolled a host. Public setup/draft-lifetime docs and all locales are
updated. See [aggregate verification](plan.md#verification-results) for commands,
counts, environment fixes and evidence limits.
