---
created: 2026-11-24
status: complete
requirements:
  - REQ-AGENTS-ACP-SESSION-EVENTS-006
system_design:
  - ../../specs/agents/system-design/acp-session-events.md
legacy_specs: []
---

# Implementation Plan: Copilot interrupted-call notice

## Overview

Show Copilot's recovery warning as a status in the resumed conversation. Start
with the observed ACP frames and carry their classification through the
existing transcript projection. Then verify the status and follow-up on
desktop and phone. [Issue #5476](https://github.com/kdlbs/kandev/issues/5476)

Agents owns this outcome because the provider's frames establish what the
warning means. The [ACP requirement](../../specs/agents/requirements/acp-session-events.md#req-agents-acp-session-events-006-copilot-interrupted-call-recovery)
and [system design](../../specs/agents/system-design/acp-session-events.md#copilot-interrupted-call-recovery)
cover the same contract across transport, storage, and chat.

## Evidence

Verified Copilot CLI 1.0.114 against a local model fixture and HTTP MCP stub.
The tool waited for input while the probe stopped its own Copilot process.
A fresh process loaded the same native session and completed a follow-up
prompt. The release archive matched the published SHA-256 checksum.
[Copilot CLI release](https://github.com/github/copilot-cli/releases/tag/v1.0.114)

| Phase | ACP observation |
| --- | --- |
| `session/load` | A pending `tool_call` replays with its original call ID and the title `kandev-ask_user_question_kandev`. Its kind is `other`. |
| First `session/prompt` | A `tool_call_update` for the same ID has status `failed` and `rawOutput.code: interrupted`. The update omits the tool title. |
| Immediately after the failed update | An `agent_message_chunk` contains the exact warning below. It has no `messageId`. |
| Next assistant text | The model reply arrives in a separate chunk with a `messageId`. The prompt returns `end_turn`. |

```text
Warning: 1 tool call(s) had no recorded result at resume. Files or other state may be partially changed. Completed tool results were retained; unresolved operations were not automatically retried. Verify the current state before continuing.
```

The same sequence occurred for an unrelated MCP call with the native event
subscription off. The optional native stream reported `session.warning` with
`warningType: interrupted_tools` in the clarification probe.

The baseline replay test `TestIssue5476CurrentWarningIsAssistantOutput` passed
using a temporary Go overlay. The test ran the captured frame sequence through
`handleACPUpdate`. It showed that replay suppresses the pending tool call
and that the warning and reply both become ordinary text events.
`convertACPContentNotification` routes the warning to the normal assistant
chunk converter, which loses the distinction in the transcript.

The standalone probe used a stub clarification. The work order checks
Kandev's actual pending question, restart, Skip, and next-prompt flow.

## Scope

- Recognize the observed Copilot ACP recovery sequence with the pinned CLI.
- Render concise clarification or generic tool copy and the original details.
- Keep the warning and reply in chronological order through live delivery
  and reload.
- Cover newly received warnings. The separate standard ACP notice work order
  owns `session.notices` capability support.

## Technical approach

| Provider and transport | Evidence | Behavior |
| --- | --- | --- |
| Copilot 1.0.114 on standard ACP | Exact warning, failed call update, one verified question ID | Clarification status |
| Copilot on standard ACP | Recognized warning with another, several, or uncertain calls | Generic tool status |
| Copilot or other ACP agents | Ordinary text or an unqualified recovery sequence | Regular message rendering |
| Backend or agentctl that ignores the classification marker | Existing text event and original text | Regular message rendering |

Keep one pending question ID during Copilot's load. Use the interrupted tool
update to check it on the first prompt. Mark the original text in
`AgentEvent.Data` and carry that field into the existing message insert.
The insert selects the status type and commits the row with its cursor.
Legacy delivery uses existing idempotent transcript insertion.

The [standard ACP notice work order](../acp-schema-1-28-adoption/task-04-provider-notices.md)
records a crash gap for callback-only status writes. The Copilot work order
uses the existing message insert for this warning.
The platform's
[durability barrier](../../decisions/2026-10-29-coalesced-delivery-durability-barrier.md)
continues to govern journal release.

During implementation, add a short recovery note to
[Sessions and Review](../../public/sessions-and-review.md#answer-clarification-questions).
Place it beside the clarification workflow and describe the Details control
for tool interruptions.

## ASCII UI preview

UI-01: Interrupted-call status in the conversation. After Skip and Send, the
status appears at the provider warning's position in the transcript.

Current desktop and phone

```text
You   Continue with the next step.
Agent Warning: 1 tool call(s) had no recorded result at resume.
      Files or other state may be partially changed. ...
Agent Here is the next step.
```

Desktop with a confirmed question

```text
You   Continue with the next step.
 (i)  The pending question was interrupted. You can continue the conversation.  [Details >]
Agent Here is the next step.
```

Phone with a generic interruption, Details open

```text
You   Continue with the next step.
 (i)  A tool call was interrupted.
      Check its effects before retrying.
      [Details v]
      Warning: 1 tool call(s) had no
      recorded result at resume. ...
Agent Here is the next step.
```

The same disclosure serves both summaries and starts collapsed. On phone,
Details can wrap to the next line and has a touch-sized hit area. The expanded
warning wraps within the existing transcript scroll region. The composer
remains the primary control for continuing. Spacing in the sketch is
illustrative. UI-01 covers `006.1` through `006.5`.

## Tests

| Criteria | Regression evidence |
| --- | --- |
| `006.1`–`006.3`, `006.6` | `adapter_copilot_interruption_test.go::TestCopilotInterruptedToolRecoveryNotice` replays the captured frames and tests classification and isolation. `TestCopilotInterruptedToolRecoveryClassification` keeps evidence across an available-commands update. |
| `006.1`, `006.5`, `006.6` | `agent_delivery_copilot_notice_test.go::TestCopilotInterruptedToolStatusProjection` covers atomic projection, replay, receipt, and retention; `TestCopilotInterruptionWithoutOwnedTurnProjectsAsOrdinaryText` covers unowned fallback. Orchestrator notice tests cover completed execution filtering and legacy text fallback. |
| `006.1`, `006.6` | `manager_events_copilot_notice_test.go::TestCopilotInterruptionNoticeDoesNotRecordAssistantActivity` keeps the notice outside assistant activity and turn completion. Lifecycle identity tests verify prompt-generation ownership and prevent an unowned notice from inheriting a successor turn. |
| `006.1`–`006.4`, `006.6` | `status-message.test.tsx` verifies localized summaries and plain-text disclosure for both classifications. |

## E2E tests

| Flow | Criteria | Spec and project |
| --- | --- | --- |
| Recover the same Copilot session after a restart. Skip its saved question, send a prompt, and check order, collapsed details, reload, and a 44px Details target at 375px with a fine pointer while retaining 24px sizing at 1280px. | `006.1`, `006.2`, `006.4`–`006.6` | `tests/chat/copilot-interrupted-tool-notice.spec.ts`, `chromium` |
| Skip by touch and continue after restart. Open Details on the phone and check row containment, hit area, contrast, and the separate assistant reply. | `006.1`, `006.2`, `006.4`–`006.6` | `tests/chat/mobile-copilot-interrupted-tool-notice.spec.ts`, `mobile-chrome` |

## Work orders

- [x] [Task 01: Project and render Copilot's interruption status](task-01-copilot-recovery-status.md) (done)

## Verification results

- `make -C apps/backend build` passed. Full Go test suites passed for the ACP
  adapter (race enabled), mock agent, lifecycle, SQLite repository, and
  orchestrator packages.
- `pnpm run typecheck`, `pnpm run i18n:check`, and the status message Vitest
  suite passed (21 tests). Desktop Chromium and mobile Chrome E2E passed.
- Public documentation validation passed (63 validator tests and 56 pages).
  `git diff --check` passed.
- SQLite projection and retention coverage passed. The optional PostgreSQL
  projection case skipped because the environment had no configured test DB.
- PR review found that two owned-notice fixtures omitted their seeded turn ID.
  Both assertions failed before the correction; the positive fixtures now carry
  the seeded owner while the unowned fallback stays turnless. The lifecycle and
  SQLite targeted regression commands passed after the correction.
- Desktop Chromium and mobile Chrome E2E passed with reload assertions for the
  warning details. The desktop flow reached Details through keyboard navigation
  from a visible chat control. Fixture cleanup now attempts every resource and
  environment cleanup step before reporting errors.
- PR run `45485003280` exposed the same missing owned-turn IDs in the lifecycle
  and durable projection fixtures, plus one repeated interrupted-status literal
  in ACP lint. The fixtures now carry their seeded owner IDs, and the adapter
  reuses `toolStatusInterrupted`. The changed-package Go lint and focused ACP,
  lifecycle, orchestrator, and SQLite regression commands passed locally.

## Risks

- A CLI release can change the text, order, or identity fields. The existing
  text path keeps an unqualified warning visible.
- Missing replay identity, conflicting IDs, or several calls require the
  generic summary to avoid an unsupported claim about effects.
- Delivery can replay without lifecycle callbacks. The storage test verifies
  the status row and cursor after reopening the repository.
