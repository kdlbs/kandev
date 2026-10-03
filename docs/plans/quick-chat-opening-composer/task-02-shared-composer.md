---
id: "02-shared-composer"
title: "Build the shared desktop and phone composer"
status: done
wave: 2
depends_on:
  - "01-opening-payload"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUICK-CHAT-COMPOSER-001
acceptance_criteria:
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.1
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.2
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.3
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.4
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.5
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.6
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.7
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.8
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.9
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.10
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.11
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.12
system_design:
  - ../../specs/tasks/system-design/quick-chat-opening-composer.md
---

# Task 02: Build the shared desktop and phone composer

## Summary

Replace modal setup branches with the shared opening composer. Keep the draft
stable through profile, mode, tab, and viewport changes.

## In scope

- Integrate Task 01 payload delivery for ordinary and configuration sessions.
- Reuse prompt/attachment primitives and the quick-chat plugin capability.
- Implement profile eligibility/default rules, repository chips, configuration
  disclosure, busy/error states, desktop focus, and touch pickers.
- Preserve per-setup drafts outside remounting branches; discard only on explicit
  setup closure or accepted payload cleanup. Scope persisted descriptors correctly.
- Add all copy in en, pt-pt, zh-cn, zh-hk, zh-tw, ja, and ko. Generate Traditional
  Chinese with the repository command; regenerate the pseudo locale.
- Add focused browser tests for the new composition as part of this work order.
- Review follow-up: keep staged upload completion owned by the persistent setup
  draft across tab unmounts, with readiness, rejection, retry, and discard proof.

## Out of scope

Floating configuration panel redesign, new voice services, new plugin slots,
workflow/executor controls, and configuration repository support.

## Acceptance

1. UI-01 through UI-04 match the approved structure on desktop and phone, with
   immediate editing, valid Send gating, and no separate setup footer.
2. Mode/tab/viewport changes preserve text and ready files. Plugin insertion and
   submission use the current draft and native validation. Config requests omit repos.
3. Targeted unit and desktop/mobile browser tests prove successful first-message
   delivery, recoverable errors, localization, focus, and touch geometry.

## ASCII UI preview

UI-01 and UI-03 excerpts; [full previews](plan.md#ascii-ui-preview). Covers AC 1-12.

```text
Desktop                              Phone
What would you like to discuss?      +-------------------------------+
+--------------------------------+   | < Chats       New Chat        |
| [repo / main v x] [image x]     |   | What would you discuss?       |
| Write a prompt...              |   | +---------------------------+ |
| [+] [Attach] [Config] [Mic] [^] |   | | [repo / main v x]         | |
+--------------------------------+   | | Prompt...                 | |
[Agent      Profile label      v]    | | [+] [Attach] [Config] [^] | |
                                     | +---------------------------+ |
                                     | [Agent     Profile label   v] |
                                     +-------------------------------+
```

Plus opens a repository picker directly. Configuration uses hover/focus help on
desktop and a description with a switch in a phone sheet. Its enabled indicator
appears inside the composer; repository chips return when the mode is disabled.


Desktop centers the bounded composer beneath fixed tabs. Phone uses a full-height
surface with one form scroll owner and inset bottom pickers. Mic is conditional
on plugin availability. Busy and failure states follow UI-04.

## Verification

Use TDD for state and submission logic. Run these commands from the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/quick-chat components/config-chat components/task-create-dialog-selectors.test.tsx components/task-create-dialog.test.tsx lib/state/slices/ui/quick-chat-actions.test.ts lib/state/slices/ui/quick-chat-sync.test.ts lib/local-storage.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/quick-chat components/config-chat components/task-create-dialog-selectors.tsx)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/quick-chat-opening-composer.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-opening-composer.spec.ts)
git diff --check
```

New E2E paths are planned outputs. Assert geometry at 390px phone width and narrow
fine-pointer widths of 767px and 768px. Assert actual control hit areas, bottom
containment at reduced heights, picker scrolling, and no horizontal document overflow.
Capture desktop and phone screenshots and compare them with the previews.

## Files likely touched

- `apps/web/components/quick-chat/quick-chat-setup.tsx`, `quick-chat-modal.tsx`, `use-quick-chat-modal.ts`, and related tests.
- `apps/web/components/quick-chat/configuration-chat-toggle.tsx`.
- A small setup-draft hook and its tests under `components/quick-chat/` (new).
- Shared input extraction from `apps/web/components/task-create-dialog-selectors.tsx`, with its task-create callers and tests if needed.
- `apps/web/components/task/chat/chat-input-plugin-actions.tsx` only if the existing capability needs a local wiring adjustment.
- `apps/web/src/locales/*/chat.json` and `configChat.json`, plus generated pseudo output.
- `apps/web/e2e/tests/chat/quick-chat-opening-composer.spec.ts` (new).
- `apps/web/e2e/tests/chat/mobile-quick-chat-opening-composer.spec.ts` (new).

## Dependencies

Task 01. Reuse `AgentSelector`, `WorkspaceRepoChips`, and the existing mobile picker pattern.

## Risks

Do not use task-create plugin surface identity for a Quick Chat draft. Do not
let controlled-state changes erase attachments or stale closures submit old text.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/quick-chat-opening-composer.md).
- [Design](../../specs/tasks/system-design/quick-chat-opening-composer.md).
- Root and scoped `AGENTS.md`; `/tdd`, `/mobile-parity`, and `/e2e` as applicable.

## Results

Implemented the shared opening composer, scoped setup-draft ownership, profile
and repository controls, configuration mode toggle, attachment actions, and
desktop/mobile presentation. Accepted structured submissions clear only the
matching opening-prompt snapshot, preserving any newer draft. Quick Chat voice
uses the existing task-less plugin capability.

- Focused Vitest suite passed: 28 files, 316 tests.
- `pnpm run typecheck` passed; focused ESLint passed with no warnings.
- The responsive-hook-order regression reproduces the hook-count error when
  touch mode changes and passes with the Send button's hooks unconditional.
- `pnpm run i18n:zh-hant`, `pnpm run i18n:pseudo`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` passed.
- Desktop and mobile opening-composer E2E flows passed. Desktop creation retry
  preserves the draft and does not create a second chat. Installed plugin
  actions insert at the selection and submit the opening payload in desktop
  and mobile Quick Chat fixtures.
- Managed E2E build produced the backend targets and Vite production bundle.
- `git diff --check` passed.

Review follow-up is complete. Upload state is published to the persistent
setup draft as it changes, including completion while the composer is unmounted.
The deferred-upload regression switches to another tab, resolves the upload,
returns to setup, and sends the same staged descriptor. It also verifies that a
rejected upload restores a retryable file and that explicit discard deletes a
late staged upload instead of restoring it.

- The affected frontend suite passed: 31 files, 320 tests.
- `pnpm run typecheck`, focused ESLint, and `pnpm run build:vite` passed.
- The setup discard generation regression confirms delayed callbacks cannot
  repopulate a cleared draft.
- `git diff --check` passed.

Merged-base PR fixup verification also passed:

- The deferred-upload lifecycle test passed: one file becomes ready after the
  setup unmounts and returns; rejection remains retryable; a late staged upload
  is deleted after explicit discard.
- All 23 Quick Chat component test files passed (151 tests), including setup
  draft persistence and deferred opening delivery.
- `pnpm run typecheck`, changed-file ESLint, Prettier, i18n check, and the
  changed-code i18n ratchet passed.

The final reduced-viewport browser checks passed with strict scroll evidence:

- `(cd apps/web && pnpm e2e:run --host --no-build --project chromium e2e/tests/chat/quick-chat-opening-composer.spec.ts)` passed 2 tests. The short desktop case uses a 1440x400 viewport and requires the setup scroll region to overflow.
- `(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/chat/mobile-quick-chat-opening-composer.spec.ts)` passed 1 test. At 390x560, the setup scroll region overflows while Send remains in the viewport; horizontal overflow remains absent.

Final PR review validation also passed:

- `(cd apps/web && pnpm exec vitest run components/quick-chat/use-quick-chat-setup-draft.test.ts lib/local-storage.test.ts components/task-create-dialog-selectors-attachment-lifecycle.test.tsx)`: 3 files, 44 tests passed after extracting stored attachment serialization.
- `pnpm run typecheck`, `pnpm run i18n:check`, `pnpm run i18n:ratchet`, focused ESLint with zero warnings, and `pnpm run build:vite` passed.
- `(cd apps/web && pnpm e2e:run --host --project mobile-chrome e2e/tests/task/mobile-task-create-workflow-step-previews.spec.ts --grep 'keeps long workflow previews contained and touch-usable on a phone' --retries=0)` passed 1 test with a fresh Vite bundle after extracting the touch close control.

## Toolbar refinement

User-approved refinement: direct repository plus picker before Attach, selected
repository chips inside the composer, explanatory configuration icon after Attach,
aligned Send, and one compact selector displaying agent and profile labels.
Focused picker, composer, and responsive regressions cover the revised controls.

Refinement validation: focused Vitest suites passed (3 files, 40 tests), web
typecheck and focused ESLint passed without warnings, and i18n checks passed.
Desktop Playwright passed the opening/retry and existing configuration/repository
flows (4 tests); the final centering rerun passed both opening-composer cases.
Phone entry cases (6), repository startup, and the opening flow passed in focused
runs. The opening flow also proves branch-sheet selection, touch targets,
configuration round trips with repository restoration, reduced viewport scrolling,
and focus return when selecting the final available repository disables Add.
Public docs validation and its 62 tests passed; specification catalog validation,
full specification lint, and diff whitespace checks passed.
