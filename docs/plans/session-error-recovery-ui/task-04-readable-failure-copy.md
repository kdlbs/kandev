---
id: "04-readable-failure-copy"
title: "Keep failure explanations readable"
status: done
wave: 4
depends_on: ["03-uniform-recovery"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.6
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.7
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.8
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.9
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 04: Keep failure explanations readable

## Outcome and scope

Restore the existing chronological error contract: each transcript error keeps
its summary and safe technical details, even while a recovery card owns the
actions. The previous active-owner branch replaced that evidence with a pointer
to a card "above", although the card can be below the transcript or absent.

The accompanying copy audit replaces vague status and error text with concrete
conditions and existing next steps. It covers authentication fallbacks, inbox
load failures, session recovery and delivery status, diagnostic settings, model
selection help, and auto-fix help. Supply all shipped languages and pseudo.

No recovery eligibility, correlation, retry, authentication, diagnostic
sanitization, or backend behavior changes are included. Task 03 is the completed
prerequisite. This record describes the implemented correction, not a new design
approval checkpoint.

## Acceptance and presentation

1. The transcript shows its own summary and expandable safe details with or
   without a mounted recovery card. Current recovery actions appear only once.
2. Desktop and phone retain chronological history, scroll behavior, readable
   details, and reachable recovery controls after reload and resolution.
3. Copy identifies the condition and available action without positional
   references or unsupported promises. Existing limits and retry semantics remain.

The affected region follows the plan's recovery presentation: a summary followed
by collapsed Technical details in the scrolling transcript; the existing composer
card owns buttons. Phone keeps stacked actions and touch targets of at least
44 pixels. Fresh desktop and phone captures accompany the PR.

## Files

- `apps/web/components/task/chat/messages/action-message-recovery-history.tsx`
  and `action-message.tsx`, with history/card regression tests.
- `apps/web/app/auth/{login-page,invite-page,setup-wizard}.tsx`.
- `apps/web/components/github/pr-ci-automation-rows.tsx` and its existing tests.
- `apps/web/src/locales/*/{auth,failedInbox,github,inboxHistory,needsYouInbox,office,settings,task}.json`.
- Existing session hook and stopped-banner copy assertions.
- Desktop and phone E2E under `apps/web/e2e/tests/{task,session,pr}`.
- `docs/public/sessions-and-review.md` (how-to guidance).

## Verification and results

Completed October 20. The missing-summary regressions failed before the fix.
All 113 tests across these seven files passed afterward:

```bash
pnpm --dir apps/web exec vitest run components/task/chat/messages/action-message.test.tsx components/task/chat/session-recovery-card.test.tsx components/task/chat/session-stopped-banner.test.tsx components/github/pr-status-chip.auto-fix-rounds.test.tsx hooks/domains/session/use-session-launch.test.ts hooks/domains/session/use-session-recovery-actions-guard.test.ts hooks/domains/session/use-session-resumption-guard-refusal.test.ts
NODE_OPTIONS=--max-old-space-size=8192 pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web run i18n:ratchet
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Changed-file ESLint passed. Public-doc validation passed all 63 tests and 48 pages.
The managed runner passed three desktop and three phone scenarios with zero
retries. Run these sequentially with the host's Chromium libraries available:

```bash
KANDEV_SERVER_HOST=127.0.0.1 pnpm --dir apps/web e2e:run --host --project chromium tests/task/launch-failure-recovery.spec.ts tests/session/durable-stream-recovery.spec.ts tests/pr/ci-automation-options.spec.ts -- --grep 'retains session failure|surfaces uncertain delivery|keeps uncertain delivery|merge conflict' --retries=0
KANDEV_SERVER_HOST=127.0.0.1 pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/task/mobile-launch-failure-recovery.spec.ts tests/session/mobile-durable-stream-recovery.spec.ts tests/pr/mobile-ci-automation-options.spec.ts -- --grep 'retains session failure|surfaces uncertain delivery|keeps uncertain delivery|merge conflict' --retries=0
```

The final capture run repeats the recovery scenario on both viewports with
`CAPTURE_PR_ASSETS=1`. Screenshots show the error summary in chat, one set of
current recovery actions, and no horizontal overflow.

### Review follow-up

Removed the unused `activeOwner` return field from `resolveRecoveryHistoryState`.
The helper now returns `true | null`; its ownership and history selection rules
are unchanged. The existing cleanup-history assertion failed before the change,
then all 75 tests across cleanup history, action messages, and the recovery card
passed. This internal return-shape cleanup does not change rendered UI or public
behavior, so the existing screenshots and public guidance remain accurate.

```bash
pnpm --dir apps/web exec vitest run components/task/chat/messages/action-message-cleanup-history.test.ts components/task/chat/messages/action-message.test.tsx components/task/chat/session-recovery-card.test.tsx
NODE_OPTIONS=--max-old-space-size=8192 pnpm --dir apps/web run typecheck
```

### CI follow-up

E2E shard 17 still selected the previous diagnostic ZIP description. The failure
reproduced locally with retries disabled. Updated that expectation, two shared
recovery helpers, and the launch-queue translation mock to match the new copy.
The Logs page keeps its privacy-note and action-alignment assertions.

All five affected desktop scenarios, both phone recovery scenarios, and seven
launch-queue unit tests passed. Changed-file ESLint and Prettier checks passed.
These test-only corrections do not change the rendered UI.

```bash
pnpm --dir apps/web e2e:run --host --no-build --project chromium e2e/tests/system/logs-page.spec.ts e2e/tests/session/delivery-retention.spec.ts e2e/tests/session/session-resume-settings-recovery.spec.ts -- --grep 'System Logs page|preserves saved history|explains one-attempt' --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/session/mobile-delivery-retention.spec.ts e2e/tests/session/mobile-session-resume-settings-recovery.spec.ts -- --retries=0
pnpm --dir apps/web exec vitest run components/task/launch-queue-status.test.tsx
```
