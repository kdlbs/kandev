---
id: "03-recovery-ui"
title: "Show actionable desktop and phone recovery"
status: done
wave: 3
depends_on:
  - "01-source-diagnostics"
  - "02-runtime-projection"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.4
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.5
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.1
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.2
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.4
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.5
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.8
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.10
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.11
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.13
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.1
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.2
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.3
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.4
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.5
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.6
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.7
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.8
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.9
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.10
system_design:
  - ../../specs/platform/system-design/delivery-storage-diagnostics.md
---

# Task 03: Show actionable desktop and phone recovery

## Summary

Show the specific storage problem and its eligible recovery action in task chat and Quick Chat.
Preserve the difference between current capacity and an historical session failure.

## In scope

- Typed diagnostic decoding in session errors, HTTP/boot records, and live session events.
- One shared view model for local, remote, unknown, stale, current, and historical states.
- A compact 90% advisory row directly above the composer, a distinct write-pressure notice, and historical recovery copy in the shared card.
- Collapsed extra guidance, executor-local source labels, and independent local/remote/Sprites warning states.
- Existing local Storage settings navigation, safe technical details, and copied diagnostics.
- Existing recovery eligibility, focus, draft, pending state, and failure stamps.
- Seven-language catalogs, locale formatting, and phone/coarse-pointer actions.
- Desktop and phone rendered flows and a public how-to section in `sessions-and-review.md`.

## Out of scope

New cleanup controls, automatic policy changes, browser polling, unrelated recovery redesign, or changes to public session admission.

## Acceptance

1. The rendered notice names the supported cause, distinguishes failure-time and current measurements, and retains a neutral old-peer fallback after reload.
2. Local users can open Storage settings without cleanup or context loss. Remote storage receives executor guidance, and normal recovery remains authoritative.
3. Desktop and phone pass current/cleared/stale/history flows, locale changes, safe details, 44px touch targets, and focus/overflow checks.

## ASCII UI preview

Full views and annotations: [UI-01 through UI-03](plan.md#ascii-ui-preview).
These excerpts retain the same labels and map to requirement 003's acceptance criteria.

UI-01: Compact advisory, shared chat region:

```text
Desktop:
  Chat transcript ...
  -------------------------------------------------------------
    ! Disk <usage>% used | <free> free     [Storage] [Details v]
  -------------------------------------------------------------
  [ Write a message ...                                       ]

Phone:
  Chat transcript ...
  --------------------------------------
    ! Disk <usage>% used [Storage] [v]
  --------------------------------------
  [ Write a message ...                ]
```

UI-02: Desktop historical failure:

```text
Session interrupted by low disk space
Reported: <failure time>
[Open storage settings] [Resume session]
[Technical details v]
```

UI-03: Phone historical failure:

```text
Session interrupted by
low disk space
Reported: <failure time>
[ Open storage settings ]
[ Resume session       ]
[ Technical details  v ]
```

Use the current recovery region as the single bounded scroll owner.
Only historical recovery actions stack on phones. Advisory controls remain in one padded row with at least 44px touch targets.
Place that row below the transcript and immediately above the composer, with 8px top and bottom padding.
Extra measurements and guidance remain behind Details. Technical details wrap inside the same region.
Remote variants identify the executor and show Details instead of the local settings action, including for a Sprite workspace.
The advisory leaves tasks running. It clears below 85% and provides no Resume action without a separate recovery cause.

## Verification

Install workspace dependencies once in a fresh worktree before frontend commands.
Run these commands from the repository root, sequentially:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/session-delivery-storage.test.ts lib/session-last-agent-error.test.ts lib/active-session-recovery.test.ts lib/ws/handlers/session-delivery-storage.test.ts components/task/chat/session-delivery-storage-notice.test.tsx components/task/chat/session-recovery-model.test.ts components/task/chat/session-recovery-card-details.test.tsx)
(cd apps/web && pnpm exec eslint lib/session-delivery-storage.ts lib/session-last-agent-error.ts lib/active-session-recovery.ts lib/ws/handlers/session-delivery-storage.ts components/task/chat/session-delivery-storage-notice.tsx components/task/chat/session-recovery-model.ts components/task/chat/session-recovery-card.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/session/delivery-storage-diagnostics.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-delivery-storage-diagnostics.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Use `/tdd`, `/mobile-parity`, and `/e2e`.
Managed E2E builds fresh artifacts. Do not overlap desktop and phone runs or override worker budgets.
Verify test discovery, then record the visible cause and eligible action before dismissal and after reload.

## Files likely touched

- New `apps/web/lib/session-delivery-storage.ts` and its tests.
- `apps/web/lib/types/http.ts`, `lib/types/task-status-summary.ts`, and `lib/session-last-agent-error.ts`.
- `apps/web/lib/active-session-recovery.ts` and session merge/hydration helpers.
- New `apps/web/lib/ws/handlers/session-delivery-storage.ts` and registered handler tests.
- `apps/web/components/task/chat/session-recovery-model.ts`, `session-recovery-card.tsx`, and shared chat composition.
- `apps/web/components/task/chat/composer-status-notices.tsx` for the advisory row directly above the input region, preserving `chat-input-body.tsx` ownership.
- New `session-delivery-storage-notice.tsx` beside the shared recovery card, with tests.
- `apps/web/components/task/session-error-details.tsx` and copied-detail formatting where necessary.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/` and the pseudo-locale path when applicable.
- New desktop/phone specs and a shared helper beside `apps/web/e2e/helpers/session-error-recovery-ui.ts`.
- `docs/public/sessions-and-review.md`.

## Dependencies

[Task 01](task-01-source-diagnostics.md), then [Task 02](task-02-runtime-projection.md).

## Risks

- Historical low-space evidence must not imply the disk remains full after cleanup.
- A raw text fallback cannot establish a precise storage cause.
- A settings link must preserve navigation drafts and existing settings permissions.
- Phone details must not push all recovery actions outside the reachable region.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/delivery-storage-diagnostics.md), all three requirements.
- [Design](../../specs/platform/system-design/delivery-storage-diagnostics.md), Shared presentation through Verification.
- [Plan previews](plan.md#ascii-ui-preview).
- Existing recovery card, error history, and `session-error-recovery-ui.ts` fixture patterns.

## Results

Typed decoding, the shared view model, compact advisory row, historical recovery copy and Storage action, remote wording, seven-locale catalogs, public recovery guidance, and desktop/phone E2E specs are implemented. Typecheck, ESLint, i18n, docs validation, and focused Vitest suites pass. The managed desktop and phone E2E specs pass in `chromium` and `mobile-chrome`.
