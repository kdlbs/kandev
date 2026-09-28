---
id: "05-panel-shell"
title: "Right-side panel shell and chat props"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COPILOT-004
  - REQ-COORDINATOR-COPILOT-005
acceptance_criteria:
  - AC-COORDINATOR-COPILOT-004.9
  - AC-COORDINATOR-COPILOT-004.13
  - AC-COORDINATOR-COPILOT-005.3
system_design:
  - ../../specs/coordinator/system-design/copilot.md
---

# Task 05: Right-Side Panel Shell and Chat Props (WP-4a)

## Summary

A frontend refactor with no coordinator wiring, so it needs no backend: split
the right-side panel out of the board's task preview, add the optional
`QuickChatSessionView` props the copilot needs, and render the "About <id>: "
prefix as a tag in a coordinator transcript. The board preview, Configuration
chat and task chat stay unchanged. Can start as soon as WP-0 has passed Review, in parallel with
everything after it.

## In scope

- `RightSidePanel` extracted from `components/kanban-with-preview.tsx`: the
  inline or floating layout from `useKanbanLayout`, the backdrop and its
  click-to-close, the left-edge `ResizeHandle`, the width clamp
  (`getRenderedPreviewPanelWidth`, `PREVIEW_PANEL` bounds), the Escape
  listener and the mobile full-screen mode, with the width storage supplied by
  the caller. `KanbanWithPreview` renders its preview through it and keeps
  `setKanbanPreviewState`, its key and its maximize action
  ([copilot design](../../specs/coordinator/system-design/copilot.md#panel)).
  `ConfigChatPanel` is not touched.
- `QuickChatSessionView` optional props, each defaulting to today's
  behaviour: `automaticRecovery`, `hideSessionSelectors`, `taskArchiveState`,
  `initialDraft` (inserted once through `chatInputRef.insertText`, not sent)
  and `transformOutgoing` (threaded through `QuickChatContent` into the
  shared `useSubmitHandler`); `useSessionResumption` option
  `skipAutomaticRecovery`
  ([copilot design](../../specs/coordinator/system-design/copilot.md#panel)).
- The coordinator branch in
  `components/task/chat/messages/user-message-body.tsx`: when the task origin
  is `coordinator` and the text starts with `About `, up to the first `: `, it
  renders the remainder plus an `about <id>` tag; the stored text is
  unchanged. The origin is compared as the string `coordinator`, so this work
  order does not need task 01.
- Regression tests: the board preview's layout (inline and floating), resize
  bounds, persisted width, Escape and backdrop close, and maximize unchanged;
  a `RightSidePanel` test of the layout rule and width clamp with a
  caller-supplied storage key; Configuration chat on `/settings` unchanged,
  Expand included; the task chat's submit unchanged with no transform; a Quick Chat
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
this work order provides (panel shell, transcript tag, composer props):

```text
 list column                         |<-> +-----------------------------+
                                     |    | * Coordinator: Planner  [x] |  shell
                                     |    |-----------------------------|
                                     |    | You: [about KAN-418] why .. |  tag
                                     |    |                             |
                                     |    |-----------------------------|
                                     |    | Ask the coordinator...  [>] |  draft, transform
                                     |    +-----------------------------+
                               resize handle; full content height
```

## Mockup screenshots and scenarios

Screenshots (visual reference; the acceptance criteria govern):

- [`docs/plans/workspace-coordinator/assets/p1-02-ask-about-this.png`](assets/p1-02-ask-about-this.png): the header and the transcript tag. The screenshot draws a popover frame; the frame is the right-side panel of `AC-COORDINATOR-COPILOT-004.8`.

Mockup scenario specs to port (in the workspace-coordinator analysis
mockup's `mockup/e2e/tests/`, outside this repository; see the plan's [Mockup scenario to repo test](plan.md#mockup-scenario-to-repo-test)):

- None ported whole here; the stored-prefix-and-tag assertion of `18-v21-copilot-anywhere.spec.ts` (Ask about this) is ported in task 06 on this work order's renderer, which is covered here by Vitest.

## Acceptance

- The panel shell and props exist with the board preview, Configuration chat
  and task chat unchanged; the board preview's and Configuration chat's e2e
  specs pass unchanged.
- A `coordinator`-origin transcript renders "About <id>: " as a tag and keeps
  the stored text; other transcripts render it verbatim.

## Verification

```bash
cd apps/web && pnpm test -- components/right-side-panel components/kanban-with-preview lib/settings/preview-panel-width components/config-chat components/quick-chat components/task/chat/messages/user-message-body.test.tsx hooks/domains/session/use-session-resumption
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/kanban tests/settings/config-chat-popover.spec.ts
```

## Likely files

- `apps/web/components/right-side-panel.tsx` and test, `kanban-with-preview.tsx`
- `apps/web/components/quick-chat/` (`QuickChatSessionView`, `QuickChatContent`)
- `apps/web/hooks/domains/session/use-session-resumption.ts` and test
- `apps/web/components/task/chat/messages/user-message-body.tsx` and test

## Dependencies

- WP-0 has passed Review. No other work order. While G0 is open the branch
  starts from WP-0's branch and rebases onto main when WP-0 merges.

## Risks

- The panel extraction touches the board preview; its existing unit and
  `tests/kanban` e2e specs must pass unchanged.
- The submit transform must not leak into other chat kinds: the default is
  the identity and a regression test pins the task chat's submit.
