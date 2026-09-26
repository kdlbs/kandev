---
id: "06-copilot-wired"
title: "Coordinator copilot wired in"
status: pending
wave: 3
depends_on:
  - "03-session-tool-surface"
  - "04-needs-you-queue-stalls"
  - "05-popover-shell"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COPILOT-002
  - REQ-COORDINATOR-COPILOT-004
  - REQ-COORDINATOR-COPILOT-005
acceptance_criteria:
  - AC-COORDINATOR-COPILOT-002.4
  - AC-COORDINATOR-COPILOT-004.1
  - AC-COORDINATOR-COPILOT-004.2
  - AC-COORDINATOR-COPILOT-004.3
  - AC-COORDINATOR-COPILOT-004.4
  - AC-COORDINATOR-COPILOT-004.5
  - AC-COORDINATOR-COPILOT-004.6
  - AC-COORDINATOR-COPILOT-004.7
  - AC-COORDINATOR-COPILOT-004.8
  - AC-COORDINATOR-COPILOT-005.1
  - AC-COORDINATOR-COPILOT-005.2
  - AC-COORDINATOR-COPILOT-005.4
  - AC-COORDINATOR-COPILOT-005.5
system_design:
  - ../../specs/coordinator/system-design/copilot.md
  - ../../specs/coordinator/system-design/coordinators.md
---

# Task 06: Coordinator Copilot Wired In (WP-4b)

## Summary

Put the copilot on the Coordinator screens: task 05's shell and
`QuickChatSessionView` on the session from task 03's conversation route, with
**Ask about this** and its context chip on task 04's item cards. On the
critical path.

## In scope

- Coordinator controller: launcher for `workspace.manage` only, busy state
  from the session store, 420 by 550 popover titled `Coordinator: <name>`, no
  Expand, full width below 640px. With the flag off no launcher renders.
- The popover builds its `QuickChatSession` value from the conversation
  route's response with `kind: "chat"`, passes `archive_state` as
  `taskArchiveState`, `automaticRecovery={false}` and
  `hideSessionSelectors`.
- The copilot half of `AC-COORDINATOR-COORDINATORS-005.1` to `005.3` (owned
  by task 03): the profile messages in place of the composer, mapped from
  `agent_profile_status` and `executor_profile_status` as the
  [coordinators design](../../specs/coordinator/system-design/coordinators.md#validation)
  tables say: from the coordinator GET without calling the conversation
  route, or from the route's `coordinator_profile_unavailable` 409 body;
  stacked when both are not `ok`, the agent profile message first. A
  component test covers agent `missing`, agent `passthrough` (the passthrough
  message, not the removed one), executor `missing`, both not `ok`, a 409 body
  after an `ok` GET, and an unknown status value (shown as that field's
  `missing` message).
- Empty-conversation intro and one suggestion.
- Copilot store `{open, chip, draft}`, **Ask about this** wiring on task 04's
  item cards, the chip, and the "About <id>: " prefix through task 05's
  `transformOutgoing`, with the hint under the composer.
- Screens leave room for the popover at 1200px.

## Out of scope

- The launcher on other pages, Expand, a Quick Chat tab (phase 3).
- Proposal decisions on the chat card (task 08; the card renders read-only).

## ASCII UI preview

From [plan UI-02](plan.md#ui-02-the-copilot-coordinator-screens):

```text
                                     +---------------------------------+
                                     | * Coordinator: Planner      [x] |
                                     |---------------------------------|
                                     | You: split KAN-418 into two     |
                                     | ( ) list_tasks_kandev           |
                                     | Planner: I proposed it.         |
                                     | ! create_task  Pending Approval |
                                     |---------------------------------|
                                     | [about KAN-418 x]               |
                                     | Ask the coordinator...     [>]  |
                                     | sent as "About KAN-418: ..."    |
                                     +---------------------------------+
                                                                 ( * )
```

## Mockup screenshots and scenarios

Screenshots (visual reference; the acceptance criteria govern):

- [`docs/plans/workspace-coordinator/assets/p1-02-ask-about-this.png`](assets/p1-02-ask-about-this.png)
- [`docs/plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png`](assets/p1-05-chat-create-task-proposal.png)

Mockup scenario specs to port (in the workspace-coordinator analysis
mockup's `mockup/e2e/tests/`, outside this repository; see the plan's [Mockup scenario to repo test](plan.md#mockup-scenario-to-repo-test)):

- `17-ask-the-copilot.spec.ts`: ask, answer, tool call shown.
- `18-v21-copilot-anywhere.spec.ts`, Ask about this: chip, draft, stored prefix and tag.

## Acceptance

- A manager opens the copilot, asks about an item and gets the mock agent's
  answer; the chip, draft, prefix and tag behave as specified.
- Opening, closing and reloading send no prompt, resume or restore request
  and start no turn; after a reload the transcript shows the session's state
  as stored: an idle session shows idle until Send, and a turn that kept
  running across the reload shows running with the launcher busy.
- Readers see no launcher; Escape, Stop, recovery feedback and the 1200px and
  390px layouts pass; no Quick Chat tab appears.

## Verification

```bash
cd apps/web && pnpm test -- hooks/domains/coordinator app/coordinator/copilot
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/copilot.spec.ts tests/config-chat
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/coordinator/copilot.spec.ts
```

`tests/coordinator/copilot.spec.ts` includes a permission-request case: the
mock agent calls one of its own tools, the popover shows Approve and Deny
through `QuickChatSessionView`'s existing permission UI (unchanged by this
work order), and approving lets the tool call complete. This is the test that
pins `AC-COORDINATOR-COPILOT-003.9` (task 03's, enforced by the guard and
auto-approval policy) for a coordinator session.

## Likely files

- `apps/web/app/coordinator/copilot/`
- `apps/web/hooks/domains/coordinator/use-copilot.ts` and test
- `apps/web/src/locales/*/`
- `apps/web/e2e/tests/coordinator/copilot.spec.ts`

## Dependencies

- Task 03 (conversation route, attended session, mock agent tools).
- Task 04 (screens, item cards with **Ask about this**).
- Task 05 (shell, props, transcript tag).
- While G0 is open the branch starts from task 03's branch with the reviewed
  branches of tasks 04 and 05 merged in, and rebases onto main after each
  predecessor merges.

## Risks

- Tasks 03, 04 and 05 land in any order; start only when all three have
  passed Review, so the controller is built on their final interfaces.
