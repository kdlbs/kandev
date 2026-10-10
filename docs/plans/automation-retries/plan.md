---
created: 2026-09-29
status: complete
requirements:
  - REQ-OFFICE-AUTOMATION-RETRIES-001
system_design:
  - ../../specs/office/system-design/automation-retries.md
legacy_specs: []
---

# Implementation Plan: Durable Automation Retries

## Overview

Deliver a durable retry lifecycle for workspace automations, from persisted policy through attempt scheduling and user-visible history. The implementation is one vertical slice across automation storage, recovery, WebSocket/API projections, the automation editor, and end-to-end coverage.

## Scope

### In scope

- Persist and validate disabled, finite, and infinite retry policies, backoff, and history presentation.
- Preserve immutable policy, trigger, and launch snapshots for every attempt in one retry group.
- Coordinate scheduler claims, task operations, and outbox publication using durable, expiring leases and generation checks.
- Keep stop/disable behavior safe when a live run cannot be stopped.
- Expose complete, bounded retry history and controls without dropping standalone runs or deleting unrelated attempts.

- In the automation editor, hide retry settings and reset the draft to canonical disabled policy for managed-conversation targets; save create/update payloads as disabled and do not restore discarded values when switching back.
- Match retry-mode radio-card selected, unselected, and hover states to the existing Context between runs and Run destination cards, including phone touch targets.
- Preserve legacy webhook interpolation data unless the operator explicitly configures safe JSON pointers.
- Update public automation documentation; add no localized UI copy unless implementation introduces new user-facing text.

### Out of scope

- Retrying workflow-step automations or provider integrations that do not use workspace automation runs.
- Adding retries to unrelated task or plugin operations.
- Automatically changing retry policy based on provider error classification.
- Changing managed-conversation delivery retry mechanics or backend retry admission; the editor saves a managed-target policy as disabled.

## Technical approach

Use the Office automation store as the durable source of ownership and due timestamps. Each original firing owns one retry group; each attempt stores its own run row and exact scheduled time. The lifecycle worker claims due rows transactionally, creates or resumes through a persisted task intent and operation lease, and publishes retry events only while holding the matching outbox token. Recovery preserves active leases and resumes expired operations by idempotent identity. History APIs keep paging bounded; the UI composes groups before status filters and follows due timestamps rather than polling the entire history every second.

For managed targets, the editor resets the retry draft to canonical disabled
values, retains an older persisted policy as the dirty baseline until an
explicit save, and sends disabled policy in managed-target create/update
payloads. `automation-editor.tsx`, `automation-editor-sections.tsx`, and
`automation-payload.ts` own hydration, target transitions, and serialization.
`RetryModeSelector` shares the card-state styles used by Context between runs
and Run destination through `automation-card-styles.ts`. No backend admission
or managed-delivery retry behavior changes.

## ASCII UI preview

### UI-01: Retry policy, target eligibility, and history presentation

**Desktop, retry-capable target**

```text
Run destination
  [●] Create a normal task

Context between runs
  [●] Start a new task for every run

Retry policy
  +--------------------------------------+
  | (●) Do not retry                     | selected: primary border + pale primary fill
  +--------------------------------------+
  +--------------------------------------+
  | ( ) Retry a fixed number of times    | neutral border; faint muted fill on hover
  +--------------------------------------+
  +--------------------------------------+
  | ( ) Retry without a limit            | neutral border; faint muted fill on hover
  +--------------------------------------+
  [Retry fields appear when a mode needs them]

Run history
  ● Retry group: Webhook delivery  attempt 2/4  Scheduled
    Next attempt: 12:05 UTC
    [Stop retries] [Delete history]
  ○ Manual run  Succeeded
```

**Desktop, managed-conversation target**

```text
Run destination
  [●] Send to a managed conversation
  [Managed destination selector]

Context between runs: hidden
Retry policy: hidden; draft disabled. An older persisted policy stays dirty until Save.
```

**Phone**

```text
Run destination
  [full-width target cards]
  [Managed destination selector when managed]

Context between runs: hidden for managed
Retry policy: hidden; draft disabled; an older persisted policy stays dirty until Save.
  Otherwise, full-width retry cards remain at least 44px high;
  the selected border/fill is the touch feedback.
```

Selecting or loading a managed-conversation target replaces the draft policy
with canonical disabled values. When a persisted managed automation has older
enabled values, the editor retains them as its dirty baseline until the
operator explicitly saves. Managed-target create/update payloads contain the
disabled policy, and switching back to a retry-capable target does not restore
discarded settings. The managed delivery retry path is unchanged. Text, group
history, and other retry controls remain as specified below; the sketch is not
a pixel specification.

## Tests

- `apps/web/components/automations/automation-payload.test.ts`: create and update payloads for managed targets contain canonical disabled policy even when the draft contains enabled retry values (AC-OFFICE-AUTOMATION-RETRIES-001.7).

## E2E tests

- `apps/web/e2e/tests/automations-settings.spec.ts` (`chromium`): switching to managed hides retry controls, resets policy, and shows the shared selected/unselected/hover card states (AC-OFFICE-AUTOMATION-RETRIES-001.7, AC-OFFICE-AUTOMATION-RETRIES-001.8).
- `apps/web/e2e/tests/settings/mobile-automations-settings.spec.ts` (`mobile-chrome`): touch selection, managed-mode hiding/reset, 44-pixel cards, and no horizontal overflow (AC-OFFICE-AUTOMATION-RETRIES-001.7, AC-OFFICE-AUTOMATION-RETRIES-001.8).
- `apps/web/e2e/tests/plugins/mobile-managed-automation.spec.ts` (`mobile-chrome`): a managed automation with a persisted enabled policy loads with a disabled draft, remains dirty until save, and persists the reset (AC-OFFICE-AUTOMATION-RETRIES-001.7).

## Work orders

- [x] [Task 01: Deliver the durable retry lifecycle](task-01-durable-retry-lifecycle.md)

## Verification strategy

- Run automation and orchestrator Go tests, including the PostgreSQL retry concurrency contract when a PostgreSQL DSN is available.
- Run frontend retry-history and polling unit tests plus focused lint/build checks.
- Run webhook-alert E2E coverage and verify both required delivery identity and retry payload compatibility.
- Add desktop and phone E2E coverage for managed-mode hiding/reset, retry-card selected/hover styling, and mobile touch geometry.
- Run specification validation, specification lint, and public documentation validation.

## Verification results

AC-OFFICE-AUTOMATION-RETRIES-001.7 and .8 passed: retry payload unit tests (19/19), frontend typecheck and focused ESLint, backend E2E seeder package compile, desktop Chromium E2E (1/1), and mobile E2E (4/4). Public-doc validation passed (62 tests; 47 published pages); specification validation and lint passed (339 decisions, 1287 specifications). E2E ran under Node 22.22.3 although the workspace declares Node 24. Existing retry-lifecycle results remain in Task 01.

## Risks

- Existing managed records with non-disabled retry policies stay persisted until the operator saves; current `admitTriggerLocked` can create generic retry groups from that policy before then. Loading must not silently mutate configuration. If managed retry admission must be blocked immediately, that requires separate backend scope.
- Backend generic retry admission and managed delivery retries are unchanged. The editor's managed-target payload reset is the boundary covered by this work order.
