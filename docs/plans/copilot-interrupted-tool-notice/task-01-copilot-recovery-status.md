---
id: "01-copilot-recovery-status"
title: "Project and render Copilot's interruption status"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-ACP-SESSION-EVENTS-006
acceptance_criteria:
  - AC-AGENTS-ACP-SESSION-EVENTS-006.1
  - AC-AGENTS-ACP-SESSION-EVENTS-006.2
  - AC-AGENTS-ACP-SESSION-EVENTS-006.3
  - AC-AGENTS-ACP-SESSION-EVENTS-006.4
  - AC-AGENTS-ACP-SESSION-EVENTS-006.5
  - AC-AGENTS-ACP-SESSION-EVENTS-006.6
system_design:
  - ../../specs/agents/system-design/acp-session-events.md
---

# Task 01: Project and render Copilot's interruption status

## Summary

Classify the qualified Copilot recovery warning before assistant text
projection. Persist one status per warning in the existing conversation.
Show a short localized summary with the original text available on demand.

## In scope

1. In a new `TestCopilotInterruptedToolRecoveryNotice`, replay the captured
   ACP sequence before changing the adapter. Expect a distinct classification
   on the warning and normal assistant text on the reply. The current adapter
   fails the classification check because both chunks take the same text path.
2. Add Copilot-specific recognition on the existing ACP notification worker.
   Keep one pending question ID for the load and compare it with the interrupted
   call ID on the first prompt. Use the generic result if the IDs are uncertain.
3. Copy the marker into the existing retained and legacy message writes.
   Select the status type in the canonical message insert. Reuse the existing
   message ID builder and cursor transaction.
4. Render the summary and disclosure in the existing status adapter. Translate
   new copy in every supported locale. Add a short recovery note to Sessions
   and Review.
5. Extend the Copilot mock path with the captured replay and prompt frames.
   Use a real saved Kandev clarification for the restart and Skip scenario.

## Out of scope

The work order covers the Copilot ACP warning in the target conversation.
The linked standard ACP work order owns generic notice capability support.
This delivery affects warnings received after the change.

## Acceptance

- The recognized warning has the correct classification. Cover the verified
  question, another tool, missing replay identity, multiple calls, and completed
  replay history. Exercise wrong provider, wrong session, stale generation,
  failed load, normal output quoting the warning, and a changed warning shape.
- Durable and legacy paths produce one status before the reply in the same
  session. After a failed projection, retry and check the message and cursor
  commit together. Test replay with lifecycle callbacks suppressed and check
  the persisted details survive retention. Keep the status outside assistant
  text, foreground activity, and turn-completion accounting.
- Desktop and phone can skip a saved question, send, and receive the reply.
  The muted summary meets the contrast target and the full warning appears
  only after Details opens. Open it with the keyboard and touch, check title
  and details order, and confirm the status survives reload.

## ASCII UI preview

UI-01: Interrupted-call status in the conversation.
[Full preview and criteria map](plan.md#ascii-ui-preview).

```text
Desktop
 (i)  The pending question was interrupted. You can continue the conversation. [Details >]
Agent Here is the next step.

Phone, Details open
 (i)  A tool call was interrupted.
      Check its effects before retrying.
      [Details v]
      Warning: 1 tool call(s) had no
      recorded result at resume. ...
Agent Here is the next step.
```

The transcript owns scrolling. Use one shared inline disclosure with a compact
24px desktop Details control and a hit area of at least 44px on phone or coarse
pointer. Check narrow phone width with both coarse and fine pointers. Check
details containment, contrast in light and dark themes, and keyboard focus.

## Verification

Run the new regression checks first and record the expected failures. Then
run them after the change from the repository root. Run the browser projects
sequentially with fresh builds. The work order introduces the named tests.

```bash
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agentctl/server/adapter/transport/acp -run '^TestCopilotInterruptedToolRecovery')
(cd apps/backend && go test -trimpath -count=1 ./internal/agent/runtime/lifecycle -run 'CopilotInterrupt')
(cd apps/backend && go test -trimpath -count=1 ./internal/orchestrator -run '^TestCopilotInterruptionNotice')
(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/task/repository/sqlite -run '^TestCopilotInterrupt')
pnpm --dir apps install --frozen-lockfile
pnpm --dir apps/web exec vitest run components/task/chat/messages/status-message.test.tsx
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/copilot-interrupted-tool-notice.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-copilot-interrupted-tool-notice.spec.ts
git diff --check
```

Use the SQLite and PostgreSQL repository test harnesses in
`agent_delivery_combined_projection_test.go`. Record coverage for each
driver in Results. Install the Chromium fixture only if the host lacks it.

For the browser fixture, enable `KANDEV_E2E_MOCK_COPILOT` with
`backend.useEnv`. Use the isolated restart pattern in `session-resume.spec.ts`
and verify that a fresh ACP load occurred. Disable retained process adoption
for this fixture if necessary to exercise the native restart sequence.
Wait for the saved pending question before restart. After reload, click
or tap Skip and send a new prompt. Assert persisted message type and order
in addition to the visible status and reply.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/` files
  `dialect_copilot.go`, `adapter_updates.go`, `adapter_session.go`,
  `adapter_prompt.go`, and new `adapter_copilot_interruption_test.go`.
- `apps/backend/internal/agentctl/types/streams/agent.go` for the shared
  classification reader and marker literals.
- `apps/backend/internal/agent/runtime/lifecycle/` files
  `durable_delivery_stream.go` and `manager_events.go` for identity and
  publication. Existing stream publication carries the marker.
- `apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go`
  and new `agent_delivery_copilot_notice_test.go`.
- `apps/backend/internal/orchestrator/event_handlers_streaming.go` and new
  `event_handlers_copilot_notice_test.go`.
- `apps/backend/cmd/mock-agent/main.go` and a focused Copilot fixture. Follow
  the session-scoped marker pattern in `interruption_continuation.go`.
- `apps/web/components/task/chat/messages/status-message.tsx`,
  `status-message.test.tsx`, and the supported `task.json` locale files.
- `apps/web/e2e/tests/chat/copilot-interrupted-tool-notice.spec.ts` and
  `mobile-copilot-interrupted-tool-notice.spec.ts`.
- `docs/public/sessions-and-review.md`.

## Dependencies

None.

## Risks

Reusing the assistant event needs a distinct canonical row key. A regression
must prove that a reply with a missing provider message ID also stays separate.
If a provider changes the warning frame, use the regular text path until a
capture qualifies the new shape.

## Parallelism

`sequential`

## Inputs

- [Qualified wire evidence](plan.md#evidence) and
  [projection design](../../specs/agents/system-design/acp-session-events.md#project-the-status-before-the-reply).
- [Clarification lifecycle](../../specs/tasks/requirements/clarification-active-lifecycle.md)
  for the existing Skip and saved question rules.
- `ContextCompactionMessage` for the inline disclosure and
  `copilot-native-compaction-helpers.ts` for the mock Copilot profile.

## Results

Implemented the qualified Copilot recovery notice from ACP notification
through durable and legacy transcript delivery. The status keeps a distinct
message identity, does not affect assistant activity accounting, and renders
as a localized summary with the original warning behind Details. The mock
fixture replays a saved Core question across a fresh process and emits the
captured interrupted-call warning and separate assistant reply.

Verification passed:

- Backend build and full Go tests for ACP adapter (with race detection),
  mock agent, lifecycle, SQLite repository, and orchestrator.
- Web typecheck, i18n checks, 21 status-message tests, and desktop/mobile E2E.
- Public documentation validation and `git diff --check`.

SQLite storage, replay, cursor, and retention coverage passed. The optional
PostgreSQL projection test skipped because no test database was configured.

Review remediation keeps warning ownership tied to the prompt generation and
does not project unowned warnings as status rows. Late events from completed
executions are dropped, and unowned legacy warnings use ordinary text delivery.
The ACP recovery evidence is bounded to one failed call ID plus interruption
and ambiguity flags; unrelated session metadata updates preserve it until a
new tool start, assistant content, or prompt end.

The review regressions passed for the metadata-update gap, successor-turn
ownership, completed-execution delivery, and unowned durable and legacy
fallbacks. The Chromium E2E also verified a 44px Details hit area at 375px with
a fine pointer and a 24px control at 1280px with a fine pointer. Mobile Chrome
E2E, web typecheck, i18n checks, status-message tests, spec validation, and
`git diff --check` passed after the remediation.

The owned-notice regression fixtures include the seeded `TurnID`; the unowned
fallback remains turnless. Before the fixture correction, the lifecycle and
SQLite regression tests failed on the status classification assertions. After
the correction, the following targeted commands passed:

```bash
(cd apps/backend && go test -trimpath -count=1 ./internal/agent/runtime/lifecycle -run 'CopilotInterrupt')
(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/task/repository/sqlite -run '^TestCopilotInterrupt')
pnpm --dir apps/web e2e:run --no-build --host --shards 1 --project chromium tests/chat/copilot-interrupted-tool-notice.spec.ts
pnpm --dir apps/web e2e:run --no-build --host --shards 1 --project mobile-chrome tests/chat/mobile-copilot-interrupted-tool-notice.spec.ts
```

Both browser commands passed after the review assertions were added. The desktop
test reached Details through keyboard navigation from a visible chat action and
reopened the disclosure after reload. The mobile test reopened the disclosure
after reload by touch and checked the persisted warning text. Shared cleanup
now attempts task, profile, trace, and environment cleanup independently.

The first PR CI run also reproduced the owned-turn fixture failures in the
lifecycle and durable projection suites and found a repeated interrupted-status
literal in ACP lint. After adding the seeded turn IDs and reusing
`toolStatusInterrupted`, changed-package Go lint and the focused ACP,
lifecycle, orchestrator, and SQLite regression commands passed.
