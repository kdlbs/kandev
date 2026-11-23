---
id: "03-gate-operator-surfaces"
title: "Gate operator surfaces and document rollout"
status: done
wave: 3
depends_on:
  - "02-settle-disabled-state"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-011
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-011.1
  - AC-TASKS-ASYNC-QUESTIONS-011.2
  - AC-TASKS-ASYNC-QUESTIONS-011.3
  - AC-TASKS-ASYNC-QUESTIONS-011.4
  - AC-TASKS-ASYNC-QUESTIONS-011.5
  - AC-TASKS-ASYNC-QUESTIONS-011.6
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 03: Gate operator surfaces and document rollout

## Summary

Use the effective release flag on desktop and phone. Prove that an administrator
can opt in, that operators keep their blocking workflow while off, and that the
browser honors the new boot after restart.

## In scope

- Gate `AsyncQuestionStrip`, the async request transcript adapter, and the
  async Inbox rows, history, and hidden controls with
  `useFeature("asyncQuestions")`. A disabled child must neither mount nor start a
  read or subscription. Hidden transcript entries must not fall through to a
  generic request card. Ignore retained async store data for display and count
  decisions while off. The server owns shared Inbox totals and snooze expiry.
- Verify the existing Feature Toggles card, environment lock, saved override,
  and restart states. Use the existing reload flow when the backend changes
  boot and the same card layout on each viewport. Apply the existing
  phone answer drawer, scroll owner, safe area and touch targets.
- Enable async questions only in the enabled async suites with
  `backend.useEnv` and restore the baseline in fixture teardown. Keep the
  dedicated default-off test at profile defaults. The existing desktop
  Inbox-off case should override only the Inbox. For the on/off/on test,
  use sequential backend restarts against the same isolated dataset so two
  fixture scopes never request conflicting values for the same key.
  Reset any saved installation override in teardown and restart before
  returning the fixture to another test.
- Add desktop and phone tests in the managed runner. Cover direct navigation,
  a boot with retained state, active or hidden Inbox rows, off-state browser
  network silence, and the enabled chat flow. Keep the existing enabled
  regressions in both projects.
- Document the flag and restart in the public operator guide and qualify
  async availability in tool, coordination and WebSocket reference docs.
  Explain what remains available after re-enable and how to continue a
  previously parked session. Update the PR description with the opt-in and
  actual results after implementation.

## Out of scope

- A new settings screen, modal, flag-specific endpoint, or unrelated card
  redesign.
- Changing enabled question layout, localization conventions, or the Inbox
  route's own flag.

## Acceptance

- Missing or false flag shows no async controls, request cards, loading row,
  or requests to the async routes. An Inbox enabled by itself stays usable
  for blocking questions on desktop and phone.
- An authorized override changes availability after restart and the
  environment lock prevents edits. Async chat works with the Inbox off.
  With both on, the existing Inbox flows still pass on both viewports.
- Public docs and PR instructions state the exact opt-in, restart, disable
  behavior, and rollback limitations and reflect the rendered result.

## ASCII UI preview

See the [full views in the plan](plan.md#ascii-ui-preview). The sketches show
presence and control order. The existing settings and chat components own
layout, spacing, and translation.

### UI-01: Feature Toggles, administrator view

```text
Desktop: [Async agent questions   Off] [Use default]
Phone:   [Async agent questions]
         [Off]
         [Use default]
Edit -> [Save changes] -> Restart required [Restart if supported]
```

Settings scrolls normally. An environment-controlled switch and reset are
disabled. Coverage: AC-TASKS-ASYNC-QUESTIONS-011.1.

### UI-02: Task chat and Inbox at each feature state

```text
Off: [Transcript and blocking questions] [Composer]
     [Existing Inbox rows and History, if enabled]
On desktop: [Transcript] [Questions (1) v / inline answer] [Composer]
On phone:   [Transcript] [Questions (1) >] [Composer]
             -> [Full-height answer: scroll body, fixed actions]
```

With both flags on, the Inbox also keeps its existing grouped answer flow.
Coverage: AC-TASKS-ASYNC-QUESTIONS-011.6.

## Verification

In a fresh worktree run `(cd apps && pnpm install --frozen-lockfile)` first.
Run these commands from the repo root:

```bash
(cd apps/web && pnpm exec vitest run \
  lib/state/slices/features/features-contract.test.ts \
  lib/state/slices/features/features-slice.test.ts \
  components/settings/system/feature-toggles-settings.test.tsx \
  components/task/chat/async-question-strip.test.tsx \
  components/task/chat/message-list-native-async-questions.test.tsx \
  components/task/chat/message-list-native.test.tsx \
  components/task/chat/message-renderer.test.tsx \
  components/task/chat/messages/agent-question-message.test.tsx \
  hooks/domains/session/use-agent-question-detail.test.tsx \
  hooks/domains/session/use-task-async-questions.test.tsx \
  hooks/domains/needs-you-inbox/use-needs-you-inbox-controller.test.tsx \
  components/needs-you-inbox/needs-you-inbox-hidden-panel.test.tsx \
  app/needs-you-inbox/needs-you-inbox-page-client.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium --retries=0 \
  tests/chat/async-agent-questions-toggle.spec.ts tests/chat/async-agent-questions.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome --retries=0 \
  tests/chat/mobile-async-agent-questions-toggle.spec.ts tests/chat/mobile-async-agent-questions.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Inspect the enabled and disabled phone views during the managed run.
Compare the single answer surface and measured touch targets with UI-02.
Record the actual Playwright counts for both projects and the verified PR
head after delivery.

## Files likely touched

- `apps/web/components/task/task-chat-panel.tsx`,
  `components/task/chat/{async-question-strip,message-renderer}.tsx`
  and `components/task/chat/messages/agent-question-message.tsx`.
- `apps/web/app/needs-you-inbox/needs-you-inbox-page-client.tsx`,
  `components/needs-you-inbox/`, `hooks/domains/{needs-you-inbox,session}/`
  and their tests. Reuse the feature hook and the existing System settings card.
- Existing async E2E files and the two planned toggle specs under
  `apps/web/e2e/tests/chat/`. Reuse `apps/web/e2e/fixtures/backend.ts`.
- `docs/public/{agent-communication,automation-and-mcp,coordination,websocket-api}.md`
  and `docs/public/coverage.json` if the doc mapping grows.

## Dependencies

[Task 02](task-02-settle-disabled-state.md), which depends on task 01.

## Risks

- Hidden markup can still run effects underneath it. Assert that browser
  requests and async-only subscriptions never start while off.
- A scoped test override that survives teardown can mask the default-off
  behavior in unrelated specs. Verify the backend flag after each restore.

## Parallelism

`sequential`

## Inputs

- [Installation release gate](../../specs/tasks/system-design/async-agent-questions.md#installation-release-gate).
- [Original desktop and phone UI previews](../async-agent-questions/plan.md#ascii-ui-preview).
- Existing `FeatureToggleCard`, `RestartRequiredAlert`, and the mobile
  async question answer surface.

## Results

The focused Vitest command passed 255 tests across 13 files in 9.84s.
The disabled-state regressions cover the strip and transcript mount guards,
the native transcript row and unread divider, retained Inbox rows and
history, and async-only refresh subscriptions. Web typecheck, lint, i18n
check, and i18n ratchet passed.

Both managed commands in [Verification](#verification) ran with
`CAPTURE_PR_ASSETS=1` and retries disabled. Chromium passed all 10 tests in
2.8 minutes. The default-off blocking case took 8.7s, and the retained-request
restart case took 22.2s. The restart case verified the live and cleared
pending-action values, a normal prompt while off, and restoration and answer
of the hidden request after re-enable. Mobile Chrome passed all three tests
in 20.2s. The default-off case answered a blocking item through the phone
Inbox in 7.9s. The enabled phone chat flow ran with the Inbox off. The phone
answer surface covered the viewport and the measured controls were at least
44 CSS pixels high.

Public docs validation covered 56 pages, catalog validation covered 462
decisions and 2,093 specifications, and full specification lint passed.
Documentation coverage preflight found 17 covered work orders on the PR.
`git diff --check` passed. Desktop and phone captures show the off setting
and the enabled answer flows. [PR #4980](https://github.com/kdlbs/kandev/pull/4980)
contains the rollout guidance, captures, and CI for the delivered changes.
