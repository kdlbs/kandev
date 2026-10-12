---
id: "07-answer-in-place"
title: "Questions and permissions answered on the Needs you card"
status: in_progress
wave: 2
depends_on:
  - "01-flag-schema-settings"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-RELAY-001
  - REQ-COORDINATOR-RELAY-002
acceptance_criteria:
  - AC-COORDINATOR-RELAY-001.1
  - AC-COORDINATOR-RELAY-001.2
  - AC-COORDINATOR-RELAY-001.3
  - AC-COORDINATOR-RELAY-001.4
  - AC-COORDINATOR-RELAY-001.5
  - AC-COORDINATOR-RELAY-001.6
  - AC-COORDINATOR-RELAY-002.1
  - AC-COORDINATOR-RELAY-002.2
  - AC-COORDINATOR-RELAY-002.3
  - AC-COORDINATOR-RELAY-002.4
  - AC-COORDINATOR-RELAY-002.5
system_design:
  - ../../specs/coordinator/system-design/relay.md
---

# Task 07: Questions And Permissions Answered On The Needs You Card (WP-11)

## Summary

Adds the relay read route and **Answer here** on question and permission
items, reusing the Inbox's clarification component and resolver and the task
chat's permission response. No answer contract changes.

## In scope

- Backend: `GET .../coordinators/:cid/relay/:taskId`
  ([Relay read](../../specs/coordinator/system-design/relay.md#relay-read)),
  reading the bundle through `clarification_bundle_query.go` directly (not the
  flag-gated Inbox handler), with one optional `SessionID` predicate added to
  its options and one exported hydration function extracted from the Inbox method in `internal/clarification`
  (Inbox behaviour unchanged); the permission through
  `ListPendingInteractions` for the primary session.
- Web: extract the `permission.respond` builder and stale test from
  `components/task/chat/messages/use-permission-handlers.ts` into
  `apps/web/lib/permissions/respond.ts`, chat behaviour pinned by its existing
  tests; `app/coordinator/components/question-answer.tsx` and
  `permission-answer.tsx`; **Answer here** in the item actions for managers
  while phase 3 is effective; phase 1's text and **Open task** otherwise
  (amended `AC-COORDINATOR-NEEDS-YOU-002.5`).
- Copy in six locales.

## Out of scope

- Any change to Inbox behaviour, `ClarificationPanelSection`, the resolver, or
  the permission WebSocket handler (the additive query predicate and the
  hydration wrapper above change none of them).
- Recording a clarification answerer (ADR residual).

## ASCII UI preview

See [plan UI-03](plan.md#ascii-ui-previews).

```text
| KAN-418  Build  [Decide now]  12m                              |
| The agent is waiting for your answer                           |
| [Answer here] [Open task]                                      |
```

## Acceptance

- A manager answers a question in place and the outcomes recorded, lost,
  no longer active and failed behave as the design's table; a missing or
  unreadable bundle shows the phase 1 text; readers see no **Answer here**.
- A manager answers a permission in place through the same WebSocket request
  the chat sends, with the audit source `web`; a stale response collapses with
  the notice, other errors keep the card with Try again.
- Answering in place writes no Inbox dismiss or snooze state; answering a
  permission leaves the Inbox rows and count unchanged, and answering a
  question leaves the Inbox as answering it in the chat does
  (`AC-COORDINATOR-RELAY-002.4`, D14).
- The relay read is 404 off phase 3 and for a task or coordinator outside the
  workspace, 200 with nulls when nothing is answerable, and the card shows
  **Answer here** only after it resolves with an answerable item
  (`AC-COORDINATOR-RELAY-001.6`).
- The permission card sends the chat's `rejected`/`cancelled` fields for the
  chosen option (`AC-COORDINATOR-RELAY-002.5`).

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Relay' -count=1
cd apps/web && pnpm test -- lib/permissions components/task/chat/messages app/coordinator/components
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/answer-in-place.spec.ts
```


## Current integration verification

Hosted E2E on PR #4044 head a31096c9 failed the phone collapse regression
because its test-id locator included both the card's expand button and the
hidden overlay's collapse button. The regression now targets the overlay's
accessible Collapse control and the card's accessible Expand control separately.
It retains the 44-pixel geometry, hidden overlay, collapsed/expanded ARIA state,
waiting session and reopen assertions. The 90-second test limit is unchanged.
Actual updated-head browser verification remains pending; no local E2E ran.

The desktop Escape regression reported the same ambiguous collapsed-control
locator in the later shard result. It now targets the uniquely accessible Expand
button while preserving waiting-state, reopen, answer/resume and Inbox-clear
assertions. This is a test-only follow-up to the real hosted failure.

Source implementation is present in the current-main integration for PR #4044.
Review remediation and complete exact-head hosted verification are still in
progress. Historical contributor CI and focused dependency projections do not
complete this work order. Merge and completion remain blocked until the full
integrated suites and every review finding are resolved.

Review verification: desktop Escape and native phone disclosure regressions leave the real clarification session waiting, reopen it, and retain the existing answer/resume assertions. Full hosted execution is pending.
