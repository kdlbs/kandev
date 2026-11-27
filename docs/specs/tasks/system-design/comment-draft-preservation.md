---
status: current
system: tasks
requirements:
  - REQ-TASKS-COMMENT-DRAFT-001
---

# Task comment draft preservation system design

## Purpose and boundaries

This design covers acknowledgement-time clearing in the first-party
`TaskChat` comment composer. The [requirement](../requirements/comment-draft-preservation.md)
belongs to Tasks. Existing [Office live updates](../../office/system-design/live-updates-01.md)
continues to own comment publication and timeline behavior, and
[task prompt attachments](prompt-attachments.md#surface-boundary) distinguishes
the body-only comment API from session uploads.

No existing focused requirement owns this send-time draft equality contract.
Plan-comment drafts and agent-message annotations govern different content and
lifecycles. This pair does not migrate or expand those specifications, or adopt
the optimistic-comment redesign described in the Office live-update design.
There is no system boundary change and no new ADR is needed for this local fix.

## Requirement mapping

| Requirement | Design section | Acceptance |
| --- | --- | --- |
| `REQ-TASKS-COMMENT-DRAFT-001` | [Completion rule](#completion-rule) | `.1`, `.2`, `.4` |
| `REQ-TASKS-COMMENT-DRAFT-001` | [Admission and failure](#admission-and-failure) | `.3`, `.5`, `.6` |
| `REQ-TASKS-COMMENT-DRAFT-001` | [Composer isolation and surfaces](#composer-isolation-and-surfaces) | `.7`, `.8` |

## Components and contracts

- `apps/web/components/task/simple/task-chat.tsx`: `ChatInput` owns local
  `input`, `submitting`, `inputValueRef`, and `setInputAndSync`; `handleSubmit`
  captures raw text before awaiting `createComment`.
- `synchronize-input-value.ts`: `synchronizeInputValue` evaluates a functional
  update immediately against `inputValueRef.current`, updates that ref, then
  schedules the rendered state value. This is not a React-deferred updater.
- `useChatInputHandlers` routes asynchronous Markdown insertion through the
  same synchronized setter. Completion protection therefore applies if file
  content has entered the current text before the request acknowledges.
- `hooks/use-prompt-result-delivery.ts`: `usePromptResultDelivery` reads that
  same ref through `getCurrent` and inserts through `apply`. Its generation and
  recovery behavior remain its own contract.
- `lib/api/domains/office-api.ts` re-exports `createComment` from
  `office-extended-api.ts`, which calls `fetchJson` in `lib/api/client.ts`.
  The request remains `POST /api/v1/office/tasks/:taskId/comments` with
  `{ body: capturedRaw.trim(), author_type: "user" }`.
- `TaskChat.onCommentsChanged` is passed to `ChatInput.onSubmitted` and is
  invoked after a successful request regardless of clearing eligibility.

## Completion rule

At admitted submission, retain the existing local `current` raw string as the
request snapshot. Replace only the unconditional success-path empty assignment
with a functional call through `setInputAndSync`:

```typescript
setInputAndSync((latest) => (latest === current ? "" : latest));
```

The helper evaluates equality against the latest ref, rather than the render
closure, and keeps the ref and textarea state synchronized in either outcome.
Comparison uses the exact raw strings; only the wire body is trimmed. Returning
an unchanged `latest` preserves the current text without resurrecting the
submitted snapshot. The success callback remains after this call.

An edit revision counter would reject the explicitly supported
edit-away-and-restore case. Comparing trimmed values would erase whitespace
edits. Directly calling `setInput` would leave prompt delivery observing a stale
ref. A new abstraction or generic draft manager is unnecessary because this
component already has the synchronized functional update boundary.

If asynchronous attachment or prompt insertion changes the current text before
success, it is preserved by this rule. If insertion finishes after clearing, it
still uses the existing functional/scope behavior. No file-read sequencing or
utility-delivery policy changes are part of the correction.

## Admission and failure

Keep the current `!current.trim() || submitting` admission check, send-button
disabled expression, editable textarea, Enter/Shift+Enter handling, and
`setSubmitting(true)`/`finally` settlement. Do not add same-tick locking or a
new submission state machine without independently causal evidence and ROOT
scope extension.

Non-2xx responses are rejected by `fetchJson`; rejected fetches also reach the
existing catch block. Preserve the current localized error fallback and error
message behavior, leave the current text untouched, and clear submitting in
`finally`. No success callback or automatic retry is added on failure. Every
later deliberate send captures its own raw snapshot.

## Composer isolation and surfaces

All snapshots and synchronized state remain local to each mounted `ChatInput`.
No store-wide state, shared pending flag, persistence, or module mutable state is
introduced. Two task composers may send concurrently without affecting each
other's current text or callbacks. Navigation and unmount lifetime semantics
remain outside this design.

The closest actual surface is `ChatActivityTabs` mounting `TaskChat`, used by
`OfficeSimplePane`. The textarea and footer have no viewport-specific send rule.
Desktop and phone use the same completion logic. Existing surrounding
`task-layout.tsx` responsive composition remains untouched.

The mobile-parity pure state/data exception applies: only completion-time value
selection changes. Layout, markup, copy, touch geometry, scroll ownership,
navigation, and breakpoint behavior do not change. Real rendered component
tests exercise the shared textarea and button/keyboard paths against deferred
HTTP transport; no new mobile Playwright test or visual/browser run is needed
for this narrow fix. This exception does not waive checks if implementation
later changes any responsive or touch presentation.

## Verification boundary

Author a new `task-chat.comment-send.test.tsx` independently of the accepted
read-only ROOT proof. Render actual `TaskChat` with actual `StateProvider` and
`createAppStore`, `ToastProvider`, `TooltipProvider`, and
`ActiveSessionRefProvider`. Mock only fetch transport, keeping comment API,
composer, synchronized setter, buttons, textarea, and feedback real. Empty
comments/sessions avoid unrelated timeline requests. Explicitly handle any
required auxiliary fetches, and fail unexpected transport calls.

Deferred responses expose exact pending and completion boundaries without
sleeps. Verify actual URL, POST, parsed trimmed body, attribution, request
counts, callback counts, editable text, pending admission, and settlement.
Use real non-2xx `Response` fixtures and rejected fetches; never mock
`createComment` or replace `ChatInput` with a surrogate. Scope controls to each
rendered root for independent-composer cases. Join/settle every deferred request
and restore transport mocks during cleanup.

The [work order](../../../plans/preserve-next-task-comment/task-01-preserve-comment-draft.md)
maps every acceptance criterion to named rendered regressions, including a
delayed text-file insertion only to prove the common value-update boundary.
Existing TaskChat and prompt-delivery tests remain relevant integration
controls. No production/test changes or product runs are authorized at design.

## Persistence, security, and observability

No storage, migrations, API schemas, authorization changes, retries, metrics,
or logging are required. Existing comment API error feedback and successful
refresh remain the observable boundaries. Do not log draft content.

## Documentation impact

The public-doc audit covered `README.md`, `docs/screenshots.md`, and
`docs/public/**`, including `tasks-and-workflows.md`, `sessions-and-review.md`,
and `developer-tools.md`. Their comment/session/attachment contracts do not
describe this acknowledgement-time reset rule. Restoring unsent text changes
no commands, documented procedure, controls, terminology, screenshots, or
public API. No public-doc edit is needed; this durable pair records the behavior.

## Implementation plan

- [Preserve the next task comment](../../../plans/preserve-next-task-comment/plan.md)
