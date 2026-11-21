---
id: "03-answer-card"
title: "Answer card, settings label and locales"
status: done
wave: 2
depends_on: ["01-kind-policy-flag"]
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-ANSWER-004
  - REQ-COORDINATOR-ANSWER-005
acceptance_criteria:
  - AC-COORDINATOR-ANSWER-005.1
  - AC-COORDINATOR-ANSWER-005.2
  - AC-COORDINATOR-ANSWER-005.3
  - AC-COORDINATOR-ANSWER-005.4
  - AC-COORDINATOR-ANSWER-005.5
  - AC-COORDINATOR-ANSWER-005.6
  - AC-COORDINATOR-ANSWER-004.4
system_design:
  - ../../specs/coordinator/system-design/answer-proposals.md
---

# Task 03: Answer card, settings label and locales

## Summary

Render answer proposals on Needs you with Approve, Edit, Reject and reply,
the Needs-your-decision badge, source rendering, the stale notice, the May do
label for `answer`, and the read-only variant when the flag is off.

## Scope

`apps/web/app/coordinator/components/` (card, source list helper, the new draft-only `AnswerDraftEditor`; `question-answer.tsx` and `permission-answer.tsx` are not reused), the May do `answer` row (shown only when the settings read includes an `answer` entry), the coordinator store and
API client types, locale files in all seven languages, one Playwright spec
`tests/coordinator/answer-proposals.spec.ts` at desktop and phone widths. The
card renders the question and options from the proposal's `snapshot`, shows
**Refused (stale)** and **Refused (task unavailable)** from the 409 `code`, keeps
entered text on validation, conflict and stale responses, offers Retry only for the failure codes `resolution_in_progress` and `not_delivered`, and shows Reject only when the flag is off.

## Exclusions

Any change to the in-place answer controls' behaviour; the Edit view uses new draft-only controls (`AnswerDraftEditor`) and sends the top-level `{"draft": ...}` edit map.

## Acceptance

- Source `ref` links only for `http` and `https`; `javascript:` renders as text.
- The stale approval shows **Refused (stale)** and the entered edit stays until
  an outcome.
- `pnpm run i18n:check` passes; no em dash in copy.

## Verification

```bash
cd apps/web && pnpm test -- app/coordinator && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/answer-proposals.spec.ts
```
