---
id: "02-browser-evidence"
title: "Prove desktop and phone behavior"
status: complete
wave: 2
depends_on:
  - "01-permission-presentation"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.4
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.11
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.12
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.13
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.14
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.15
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.16
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.17
system_design:
  - ../../specs/agents/system-design/automatic-permission-presentation.md
---

# Task 02: Prove Desktop and Phone Behavior

## Summary

Add browser coverage that observes automatic approval before it completes.
Prove the manual path still works and update the public explanation of
automatic approval.

## In scope

- Add shared request-scoped browser observation and delivery gating helpers.
- Add desktop and phone cases for live automatic approval, hydration/reload,
  fallback presentation, unavailable feedback, and manual completion.
- Preserve existing manual, offered-choice, and unattended Git scenarios.
- Update only the affected permission explanation in public agent documentation.
- Record exact results and completed work-order status after targeted checks.

## Out of scope

- Broad QA, unrelated permission policy, or new production fault-injection APIs.
- A new mobile sheet, navigation, transcript layout, or audit history UI.
- Claiming that seeded message states reproduce a backend delivery failure.

## Acceptance

1. An observer active before scenario submission records zero approval prompts,
   action rows, or amber input states through automatic completion. Audit and
   the resulting Git commit remain present after reload.
2. Desktop and phone show the same fallback and unavailable states. A real
   attended request completes after a click or tap. Touch choices remain reachable.
3. Tests use isolated fixtures and restore changed shared state. Public text
   describes automatic decisions, interactive fallback, and durable audit accurately.

## UI preview

These views match [the combined preview](plan.md#ui-preview).

| View | Desktop and phone observation |
| --- | --- |
| UI-01: Automatic approval | Ordinary tool row remains. No input fragment appears at any observed update. |
| UI-02: Interactive fallback | Existing offered choices appear. Keyboard or touch can select a choice. |
| UI-03: Unavailable decision | Existing feedback remains readable, with no second-answer controls. |

Use the existing inline phone permission exemplar. Preserve transcript scrolling,
composer safe-area clearance, and focus during the automatic interval.
No separate phone composition is needed for this state correction.

## Verification

Use the component RED from task 01 as the primary regression gate. Add the live
browser observer before `SessionPage.sendMessage` submits the scenario.
Do not create the permission-requesting turn before opening the page.

Scope any WebSocket gate by task/session/request identity. Gate matching
message-list hydration while completion frames are held, so a refresh cannot
skip the observed interval. Do not block provider delivery or unrelated events.
Record when the candidate reaches the browser, inspect UI-01 while held, and
release the gate before completion assertions. An after-idle absence check alone
does not pass. Use the repository's causal wait helpers for negative observations.

Use `apiClient.seedSessionMessage` for persistent fallback/unavailable snapshots.
Use a request-scoped browser frame fixture for the automatic-to-interactive
render transition. Backend tests from task 01 prove actual failure classification.
Keep that evidence distinction in the results.

Run from the repository root, sequentially. The managed runner rebuilds both
product artifacts and tears down its isolated processes:

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/chat/automatic-permission-presentation.spec.ts tests/chat/permission-approval.spec.ts tests/chat/agent-permission-unattended.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-automatic-permission-presentation.spec.ts tests/chat/mobile-permission-offered-choices.spec.ts)
(cd apps/web && pnpm exec vitest run e2e/helpers/automatic-permission-presentation.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 e2e/helpers/automatic-permission-presentation.ts e2e/helpers/automatic-permission-presentation.test.ts e2e/tests/chat/automatic-permission-presentation.spec.ts e2e/tests/chat/mobile-automatic-permission-presentation.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The helper test must prove that the gate matches only the target request and
releases held messages on success and failure. Record discovered test counts
for both Playwright projects. Do not overlap E2E commands or change worker limits.
Capture a rendered phone screenshot and compare its structure with UI-01/02.

## Files likely touched

- New `apps/web/e2e/tests/chat/automatic-permission-presentation.spec.ts`.
- New `apps/web/e2e/tests/chat/mobile-automatic-permission-presentation.spec.ts`.
- New `apps/web/e2e/helpers/automatic-permission-presentation.ts` and its test.
- `apps/web/e2e/tests/chat/agent-permission-unattended.spec.ts` if shared
  setup can replace its after-idle-only assertion without losing commit evidence.
- `apps/web/e2e/pages/session-page.ts` only for shared stable selectors.
- `docs/public/agents-and-profiles.md`, permissions and unattended work section.
- This plan and both work-order Results sections.

## Dependencies

Task 01. Its final metadata and fallback contract must exist before browser fixtures use it.

## Risks

A held WebSocket frame is a browser presentation fixture, not a provider fault.
Snapshot reads can bypass the gate. Test both channels without changing unrelated
traffic. Restore any worker-scoped approval setting and delete disposable profiles.

## Parallelism

`sequential`

## Inputs

- [Requirement 003](../../specs/agents/requirements/permission-control-integrity.md#req-agents-permission-control-integrity-003-auto-approve-approves-or-prompts-and-never-denies).
- [Presentation design](../../specs/agents/system-design/automatic-permission-presentation.md).
- Existing `agent-permission-unattended.spec.ts`,
  `mobile-permission-offered-choices.spec.ts`, and
  `e2e/helpers/native-resume-projection.ts` patterns.
- The `/e2e`, `/mobile-parity`, and `/docs-maintainer` workflows.

## Results

Completed on 2026-10-30, with static-review remediation.

- Added shared request-scoped browser helpers that install a mutation recorder
  before prompt submission, hold only the target session's permission row
  completion frames and `message.list` snapshot responses, and gate matching
  HTTP hydration until release. The pure frame gate has unit coverage for
  session/row matching, snapshot response correlation, and single release.
- The automatic browser case triggers a real foreground history refresh while
  completion is held, proves the snapshot response is queued, and confirms the
  commit completion stays out of the browser until release.
- Added a desktop spec with four cases: live automatic approval through held
  completion, seeded interactive fallback, seeded unavailable feedback on an
  attached tool row, and a real attended request completed by a click.
- Added a phone spec with the same four cases. The attended request completes by
  tap, fallback choices meet the 44px touch target, and automatic/fallback
  screenshots are captured through `prCapture`.
- The live automatic cases observe zero permission rows, approval buttons,
  permission messages, or amber input states while completion is held. They
  assert the durable automatic audit, the resulting commit, and the same state
  after reload.
- Updated the public auto-approval explanation to describe the allow-or-prompt
  contract, durable audit, ordinary tool progress, safe interactive fallback,
  and non-actionable uncertain outcomes.

Verification passed:

- `chromium` desktop permission group: 9 tests passed (new spec plus
  `permission-approval` and `agent-permission-unattended`).
- `mobile-chrome` phone permission group: 5 tests passed (new spec plus
  `mobile-permission-offered-choices`).
- Browser-gate unit tests cover target-session snapshots and unrelated traffic.
- Public docs validation: 54 pages passed; docs tests: 63 passed.
- ESLint, `tsc --noEmit`, and `git diff --check` passed.
