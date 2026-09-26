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

## Build decisions

Settled on the Spec step against `main` at `c735b6788` (plus WP-0's docs), so
the builder does not invent them. They refine the copilot design without
changing it.

- **Shell.** `ChatPopoverShell` (`components/config-chat/chat-popover-shell.tsx`)
  owns the Radix `Popover`, the `PopoverContent` classes, sizing and test id,
  the header row (icon, title, `headerActions` slot, Close), and a `beforeBody`
  slot for the floating-actions host. Props: `open`, `onOpenChange`, `trigger`
  (rendered inside the `Popover`, so the caller wraps it in `PopoverTrigger`),
  `icon`, `title`, `closeLabel`, `headerActions?`, `beforeBody?`, `testId`,
  `children`. Escape and focus return stay Radix's; outside clicks do not
  close the shell, as they do not today. `ConfigChatPanel` passes Expand as
  `headerActions` and keeps `config-chat-popover`, every aria-label and every
  class, so its rendered DOM does not change. The snapshot that pins it is
  recorded from the unrefactored panel before the extraction, in a new
  `components/config-chat/config-chat-panel.test.tsx`. It covers three
  states: (1) closed, with the trigger button; (2) open with no config
  session, the setup body branch, with Expand disabled while `isStarting`
  is true and enabled when it is false; (3) open with a config session, the
  session body branch. Each open state is serialized from `document.body`
  after clicking the trigger, because the content renders in a Radix
  portal. The fixture mocks `useAppStore` (the `quickChat.sessions` list,
  empty or holding one `kind: "config"` session for the workspace,
  `openQuickChat` and `setQuickChatInitialPrompt`) and `useConfigChat`,
  and stubs `QuickChatSessionView` and `ConfigChatSetup` with marker
  elements. The snapshot therefore pins the shell, the header, Expand,
  Close and the floating-actions host, and which body branch renders, not
  the bodies' own DOM, which this work order does not change. The same
  file asserts that Expand calls `openQuickChat` with kind `config` and
  closes the popover.
- **`hideSessionSelectors`.** When `true`, the view renders `QuickChatContent`
  with `minimalToolbar`, which has Submit and Stop and no mode or model
  selector; the placeholder stays the chat placeholder. Kind `config` keeps
  its minimal toolbar and placeholder.
- **`taskArchiveState`** has type `TaskArchiveState` (`boolean | null`). Any
  value other than `undefined`, `null` included, replaces
  `resolveTaskArchiveState`.
- **`automaticRecovery`** (default `true`) maps to
  `useSessionResumption(..., { skipAutomaticRecovery: !automaticRecovery })`.
  With the option set, the check-and-resume effect and the remote-status
  retry effect do nothing; `resumeSession`, `retryStatus` and the workspace
  `restore` stay manual actions, and the session row is still read from the
  store.
- **`initialDraft`** is `string | undefined`. `QuickChatSessionView` passes it
  unchanged to `QuickChatContent` as a new `initialDraft` prop, just as it
  passes `initialPrompt` today. `QuickChatContent` owns `chatInputRef`, so it
  applies the draft, and it keeps the last applied value in a ref of its own.
  That ref lives outside `ChatInputContainer`, so the container's remount on
  a clarification key change does not re-apply the draft.
  - **Applying.** A draft is *pending* when it is a non-empty string that
    differs from the last applied value. A pending draft is applied in the
    first commit in which `chatInputRef.current` is non-null and
    `initialPrompt` is empty or `undefined`. Applying calls `clear()`, then
    `insertText(draft, 0, 0)`, then `focusInput()`, and records the value as
    applied. It never submits. The trigger re-checks in every commit and is
    not keyed on the prop alone, so a draft that arrives before the composer
    mounts is held, not dropped. After applying, the composer holds exactly
    the draft, replacing any text the composer restored from its saved
    per-session draft.
  - **Repeats and resets.** The same value on a later render is not applied
    again. `undefined` or `""` leaves the composer as it is and resets
    nothing, so a caller re-applies an identical draft by passing `undefined`
    first.
  - **`initialPrompt` wins.** While `initialPrompt` is non-empty, the draft
    stays pending, and it is applied once the prompt clears (the caller
    clears it through `onInitialPromptAttempted`). The coordinator popover
    passes no `initialPrompt`.
  - **Passthrough sessions.** These render `PassthroughTerminal`, not
    `QuickChatContent`, so they have no composer and the draft has no
    effect. A `QuickChatContent` mounted later (a different session, or a
    session that stops being passthrough) starts with no applied value, so it
    applies the current non-empty draft once under the rule above.
- **`transformOutgoing`** is `(message: string) => string`, passed to
  `useSubmitHandler(panelState, onSend, { transformOutgoing })`. It is applied
  to `payload.message` once per submit, before `buildSubmitMessage`, on both
  the `onSend` and the direct send paths. It never changes the composer
  text. If it throws, the existing catch shows the send-error toast and
  nothing is sent. Without it the submit path is the one on `main` byte for
  byte; the task chat and Quick Chat call sites pass no options.
- **Origin for the tag.** `renderUserMessageBody` gains `taskOrigin?: string`.
  `ChatMessage` reads it from a new `MessageTaskOriginContext` (default
  `undefined`) that task 06 provides around the popover body. The kanban
  store is not consulted: the coordinator task is ephemeral and absent from
  it, and `TaskOrigin` gains `coordinator` only in task 01.
- **Prefix parse.** Only when `taskOrigin === "coordinator"`, the raw view is
  off and the content starts with `About ` (case-sensitive): the id is the
  text between `About ` and the first `: `, and it must be non-empty and hold
  no line break; otherwise the message renders verbatim. The remainder goes
  through `MessageSegments` as today, and the tag follows it, as in the
  mockup's transcript: the body renders the remainder first, then the tag
  on its own line below it, left-aligned inside the same bubble (a block
  wrapper, not inline with the last text line). An empty remainder renders
  the tag alone. The tag is a `Badge` with
  `data-testid="coordinator-about-tag"` and the copy
  `chat:coordinatorAboutTag` (`about {{id}}`) in all six locales. The raw
  view shows the stored text with the prefix.
- **Download and copy keep the stored text.** `MessageSegments` gains an
  optional `downloadSource` prop (default: its `content`, as today). The
  coordinator branch passes the full stored `content`, prefix included, so
  a long message's download matches the stored message. Copy already uses
  `message.content` in `MessageActions` and is unchanged; a test pins that
  it copies the text with the prefix.
- **Tests.** Vitest only for the new props and the tag, because nothing on
  `main` renders them until task 06; the existing Configuration chat
  Playwright specs (`e2e/tests/settings/config-chat-popover.spec.ts`, which
  covers Expand, and the two mobile Configuration chat specs) are the e2e
  regression and must pass unedited.
- **Known limit, for task 06.** An id containing `: ` (a proposal title used
  as the id) splits at its own first `: `, so the tag shows a truncated id.
  The rule is the copilot design's; task 06 owns what goes into the chip.

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
                                     |        why is KAN-418 here? ... |  text,
                                     |        [about KAN-418]          |  then tag
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
cd apps/web && pnpm e2e:run tests/settings/config-chat-popover.spec.ts tests/settings/mobile-config-chat-popover.spec.ts tests/settings/mobile-configuration-chat.spec.ts
```

## Likely files

- `apps/web/components/config-chat/chat-popover-shell.tsx`, `config-chat-panel.tsx`, `config-chat-panel.test.tsx` (new)
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
