---
id: "03-show-recovery-errors"
title: "Show recovery errors and actions"
status: done
wave: 3
depends_on:
  - "02-persist-branch-recovery-warning"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.6
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.8
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 03: Show Recovery Errors and Actions

## Summary

Stop discarding resume errors. Reuse the existing inline alert and status
message patterns to show causes, read-only outcomes, and the explicit branch
continuation action.

## In scope

- Start with failing tests in all three known error-swallowing paths.
- Add a typed WebSocket request error that retains backend code and details.
- Keep existing callers compatible with the standard `Error` interface.
- Make the shared session recovery helper reject with the typed error.
- Retain the last error in `SessionStoppedBanner` after busy state ends.
- Retain the last error in `RunErrorEntry` after busy state ends.
- Disable the shared Retry control while a recovery request is pending on all
  recovery surfaces.
- Reuse `EnsureSessionErrorBanner` alert anatomy and error description rules.
- Show **Continue on a new branch** only for the typed branch-loss details.
- Offer visible read-only workspace restore after manual resume failure.
- Retain manual resume and read-only restore causes independently, including
  branch details associated with the resume failure.
- Keep automatic read-only fallback and return a visible notice with the first
  failure cause.
- Return both causes when automatic resume and restore fail.
- Clear stale automatic feedback when the shared session state becomes active
  after an external manual recovery.
- Render the hook result in task, preview, and Quick Chat consumers.
- Map `branch_recreated` status metadata to localized honest warning copy.
- Add every new key to all six locale catalogs. Generate Traditional Chinese
  values with `pnpm run i18n:zh-hant`.

## Out of scope

- A new alert component library or recovery page.
- Automatic branch replacement.
- Translation of branch names or other metadata values.
- A new WebSocket event handler for the persisted warning.
- Changes to Start fresh confirmation semantics.

## Acceptance

- A failed manual Resume or Start fresh request shows the backend message near
  the clicked control and leaves it visible after the request finishes.
- A repeated click updates or retains visible feedback and never becomes a
  no-op with no explanation.
- A successful automatic read-only restore states why resume failed and that
  the workspace is read-only.
- A double failure displays the resume cause and the restore cause.
- A manual resume failure followed by a restore failure displays both causes
  and keeps any branch-continuation action tied to the resume failure.
- Only branch-loss details expose **Continue on a new branch**.
- A successful branch replacement clears the blocking error and the persisted
  warning states that conversation history continued while old code did not.
- Desktop and mobile use the same capabilities and existing responsive action
  layout.
- Repeated Retry clicks while a request is pending do not overlap recovery
  requests.

## Verification

Add failing tests first. Then run:

```bash
# From apps/web:
rtk pnpm test -- lib/ws/client.test.ts components/task/chat/session-stopped-banner.test.tsx components/task/simple/components/run-error-entry.test.tsx hooks/domains/session/use-session-resumption.test.ts components/task/chat/messages/status-message.test.tsx
rtk pnpm run typecheck
rtk pnpm run i18n:check
rtk pnpm run i18n:ratchet

# From apps:
rtk pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/lib/ws/client.ts`
- `apps/web/lib/ws/client.test.ts`
- `apps/web/components/task/chat/session-stopped-banner.tsx`
- `apps/web/components/task/chat/session-stopped-banner.test.tsx`
- `apps/web/components/task/simple/components/run-error-entry.tsx`
- `apps/web/components/task/simple/components/run-error-entry.test.tsx`
- `apps/web/hooks/domains/session/use-session-resumption.ts`
- `apps/web/hooks/domains/session/use-session-resumption.test.ts`
- Task, preview, and Quick Chat consumers of `useSessionResumption`
- `apps/web/components/task/ensure-session-error.tsx`
- `apps/web/components/task/chat/messages/status-message.tsx`
- `apps/web/components/task/chat/messages/status-message.test.tsx`
- `apps/web/src/locales/*/task.json`
- `apps/web/src/locales/*/chat.json`

## Dependencies

- Task 02 provides the typed error details and persisted warning metadata.

## Risks

- A broad error-state reset can hide the cause before the user can read it.
  Clear it only on a later success or explicit dismissal.
- Rendering raw details can expose internal fields. Render the backend message
  and only the approved branch metadata.
- A module-scope translation call freezes the boot locale. Translate only
  during render or hook execution.
- A translated confirmation token can break exact input comparisons. Keep any
  persisted token stable and translate only its explanation.
- Locale catalogs fail the build if keys or placeholders differ. Run all
  internationalization gates after `zh-hant` generation.

## Parallelism

`sequential`

## Inputs

- Completed Task 02 protocol and metadata.
- Existing `EnsureSessionErrorBanner`.
- Existing model-selection warning status rendering.
- Existing session recovery component and hook tests.
- Mobile UI language reference for inline alerts and action layout.

## Results

- Implemented typed WebSocket request errors, persistent manual recovery
  feedback, automatic read-only fallback notices, and typed branch-continuation
  actions in the stopped-session, run-error, and automatic-resume surfaces.
- Added localized branch-recreated warning rendering and completed all six
  locale catalogs using the Traditional Chinese generator.
- GREEN: focused frontend recovery suite, 6 files and 73 tests passed.
- GREEN: `rtk pnpm run typecheck`, `rtk pnpm run i18n:check`,
  `rtk pnpm run i18n:ratchet`, and `rtk pnpm --filter @kandev/web lint`.
- GREEN: `rtk pnpm run i18n:zh-hant -- --namespace task`.
