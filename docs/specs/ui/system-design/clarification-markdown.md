---
status: current
system: ui
requirements:
  - REQ-UI-CLARIFICATION-MARKDOWN-001
---

# Clarification Markdown system design

## Ownership and boundaries

UI owns the limited Markdown presentation shared by active questions, transcripts, tool details, and History rows.
The [Inbox History design](inbox-history.md) retains read eligibility and ordering. The [task clarification design](../../tasks/system-design/clarification-active-lifecycle.md) retains turn ownership and responses.

This is a local extension of an existing renderer. No new dependency, parser, storage contract, or architectural decision is needed.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-UI-CLARIFICATION-MARKDOWN-001.1, .4 | Shared renderer |
| AC-UI-CLARIFICATION-MARKDOWN-001.2, .6 | History integration |
| AC-UI-CLARIFICATION-MARKDOWN-001.3, .7 | Links and state |
| AC-UI-CLARIFICATION-MARKDOWN-001.5 | Responsive behavior |

## Renderer adoption

Commit `469db8bc7d7` added `ClarificationMarkdown` and connected active question titles, prompts, choices, transcripts, and tool details.
`InboxHistoryQuestionDetail` and the collapsed question summary in `inbox-history-row.tsx` use that same renderer.
Question values remain unchanged until rendering. Active and History browser checks use the same HTTP reference destination.
The delivery record and failure reproduction are in the [implementation plan](../../../plans/clarification-markdown/plan.md).

## Shared renderer

Reuse `apps/web/components/task/chat/clarification-markdown.tsx` without widening its element or URL policies.
It uses `react-markdown`, `remark-gfm`, `remark-breaks`, `skipHtml`, `unwrapDisallowed`, and `defaultUrlTransform`.
Its block variant permits paragraphs and lists alongside inline marks. Its inline variant omits block containers.
Keep unsupported code panels as text. Do not introduce the comment normalizer, raw HTML plugins, syntax highlighting, or a second Markdown parser.

## History integration

`apps/web/lib/inbox-history/row-presentation.ts` already provides each question's title, prompt, and option labels through `inboxHistoryClarificationQuestions`.
Keep these values unchanged. Rendering belongs in `apps/web/components/inbox-history/inbox-history-row.tsx`.

For expanded clarification details:

1. Render a non-empty title with the inline variant.
2. Render a non-empty prompt with the block variant, even when the title is absent.
3. Render each existing option label with the inline variant. Keep separators outside individual Markdown values.
4. Preserve literal context and the existing localized empty-content fallback.

Avoid a block renderer inside a paragraph. Expanded content remains outside the row-toggle button.

For collapsed rows, retain the source distinction when deriving `inboxHistoryPrimaryText`.
Only question-title and question-prompt summaries use inline Markdown with `linkBehavior="passive"`.
Permission text, context-only fallbacks, and localized fallbacks remain literal. Do not classify a summary from its text contents.
Preserve single-line ellipsis even when descendant code or break elements would normally wrap. Scope that presentation to the summary.

## Links and state

Expanded question content uses the renderer's existing interactive link mode.
HTTP and HTTPS destinations retain `target="_blank"` and `rel="noopener noreferrer"`.
The renderer's web-link check leaves all other destinations passive after the default URL transform.

Collapsed summaries and active answer buttons use passive links because their enclosing buttons own activation.
Do not nest an anchor inside a button or add a broad row click handler around expanded reference links.
Keep existing link colors and underlines. Reference activation causes no clarification mutation or row-state change.

No HTTP, WebSocket, store, persistence, or metadata changes are required.
Question values remain agent-authored strings until render time. Submission continues to use option identifiers.

## Responsive behavior

This is content formatting inside existing surfaces. The closest mobile exemplars are `mobile-clarification.spec.ts` and `mobile-inbox-history-superseded.spec.ts`.
Keep History as an inline disclosure in the Inbox page, with the existing phone action row beneath the summary.
Use the existing page scroller. Do not add a drawer, nested scroller, or new navigation step.

Expanded content uses the renderer's wrapping behavior. Collapsed content remains one line.
Keep the existing phone and coarse-pointer action targets and fine-pointer desktop density.
Use the existing tokens, icon states, borders, and disclosure feedback. No new motion or surface styling is required.

## Verification and remaining uncertainty

Component tests cover summary/detail formatting, title-less prompts, literal fallbacks, passive versus interactive links, and unchanged answer payloads.
Existing renderer security tests cover unsupported elements and unsafe links. Add URL cases only where the current contract lacks coverage.
Desktop and mobile E2E tests cover actual active question links and History detail links, including popup destination and no overflow.
History tests must retain their existing reason, time, read-only, task-opening, and touch-target assertions.

Desktop and phone browser checks verify standard active-question links, History links, wrapping, and independent answer/disclosure actions.
The exact originally reported active-question input remains unknown. Passing the standard scenario does not establish a reproduction of that input.
No new telemetry is needed for this presentation change.

## Delivery

- [Implementation plan and work order](../../../plans/clarification-markdown/plan.md).
