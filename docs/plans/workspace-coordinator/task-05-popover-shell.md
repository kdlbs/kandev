---
id: "05-popover-shell"
title: "Popover shell and chat props"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COPILOT-004
  - REQ-COORDINATOR-COPILOT-005
acceptance_criteria:
  - AC-COORDINATOR-COPILOT-004.9
  - AC-COORDINATOR-COPILOT-005.3
system_design:
  - ../../specs/coordinator/system-design/copilot.md
---

# Task 05: Popover Shell and Chat Props (WP-4a)

## Summary

A frontend refactor with no coordinator wiring, so it needs no backend: split
the popover shell out of Configuration chat, add the optional
`QuickChatSessionView` props the copilot needs, and render the "About <id>: "
prefix as a tag in a coordinator transcript. Configuration chat and task chat
stay unchanged. Can start as soon as WP-0 has passed Review, in parallel with
everything after it.

## In scope

- `ChatPopoverShell` extracted from `ConfigChatPanel` (position, size, header,
  close, Escape handling, focus return); `ConfigChatPanel` keeps its Expand
  and behaviour, pinned by a snapshot test.
- `QuickChatSessionView` optional props, each defaulting to today's
  behaviour: `automaticRecovery`, `hideSessionSelectors`, `taskArchiveState`,
  `initialDraft` (inserted once through `chatInputRef.insertText`, not sent)
  and `transformOutgoing` (threaded through `QuickChatContent` into the
  shared `useSubmitHandler`); `useSessionResumption` option
  `skipAutomaticRecovery`
  ([copilot design](../../specs/coordinator/system-design/copilot.md#popover)).
- The coordinator branch in
  `components/task/chat/messages/user-message-body.tsx`: when the task origin
  is `coordinator` and the text starts with `About `, up to the first `: `, it
  renders the remainder plus an `about <id>` tag; the stored text is
  unchanged. The origin is compared as the string `coordinator`, so this work
  order does not need task 01.
- Regression tests: Configuration chat on `/settings` unchanged, Expand
  included; the task chat's submit unchanged with no transform; a Quick Chat
  tab's restore on mount unchanged; the draft inserted once and not sent; the
  prefix rendered as a tag only for a `coordinator`-origin task.

`QuickChatSessionKind` stays `"chat" | "config"`; this work order adds no
`"coordinator"` kind and no kind-specific toolbar. The source analysis plan
(`implementation-plan.md` revision 10, section 7, WP-4a, outside this
repository) widens the kind; the copilot design keeps it narrow so the Quick
Chat tab list, selection and `serverIdsByKind` types are untouched, and hides
the selectors through `hideSessionSelectors` instead. The design governs.

## Out of scope

- The coordinator controller, launcher, chip and store (task 06).
- Any backend change.

## ASCII UI preview

From [plan UI-02](plan.md#ui-02-the-copilot-coordinator-screens), the parts
this work order provides (shell, transcript tag, composer props):

```text
                                     +---------------------------------+
                                     | * Coordinator: Planner      [x] |  shell
                                     |---------------------------------|
                                     | You: [about KAN-418] why ...    |  tag
                                     |---------------------------------|
                                     | Ask the coordinator...     [>]  |  draft, transform
                                     +---------------------------------+
```

## Mockup screenshots and scenarios

Screenshots (visual reference; the acceptance criteria govern):

- [`docs/plans/workspace-coordinator/assets/p1-02-ask-about-this.png`](assets/p1-02-ask-about-this.png): the popover shell and the transcript tag.

Mockup scenario specs to port (in the workspace-coordinator analysis
mockup's `mockup/e2e/tests/`, outside this repository; see the plan's [Mockup scenario to repo test](plan.md#mockup-scenario-to-repo-test)):

- None ported whole here; the stored-prefix-and-tag assertion of `18-v21-copilot-anywhere.spec.ts` (Ask about this) is ported in task 06 on this work order's renderer, which is covered here by Vitest.

## Acceptance

- The shell and props exist with Configuration chat and task chat unchanged;
  the Configuration chat e2e specs pass unchanged.
- A `coordinator`-origin transcript renders "About <id>: " as a tag and keeps
  the stored text; other transcripts render it verbatim.

## Verification

```bash
cd apps/web && pnpm test -- components/config-chat components/quick-chat components/task/chat/messages/user-message-body.test.tsx hooks/domains/session/use-session-resumption
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/config-chat
```

## Likely files

- `apps/web/components/config-chat/chat-popover-shell.tsx`, `config-chat-panel.tsx`
- `apps/web/components/quick-chat/` (`QuickChatSessionView`, `QuickChatContent`)
- `apps/web/hooks/domains/session/use-session-resumption.ts` and test
- `apps/web/components/task/chat/messages/user-message-body.tsx` and test

## Dependencies

- WP-0 has passed Review. No other work order. While G0 is open the branch
  starts from WP-0's branch and rebases onto main when WP-0 merges.

## Risks

- The shell extraction touches Configuration chat; its existing e2e specs must
  pass unchanged.
- The submit transform must not leak into other chat kinds: the default is
  the identity and a regression test pins the task chat's submit.
