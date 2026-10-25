---
status: current
system: ui
requirements:
  - REQ-UI-MARKDOWN-MATH-001
  - REQ-UI-MARKDOWN-MATH-002
---

# Markdown Math System Design

## Purpose and boundaries

The shared Markdown pipeline owns formula presentation. This change adds no backend API or persistence contract.
The [comment Markdown contract](../requirements/comment-markdown.md) continues to own existing formatting and link behavior.
The [chat motion design](chat-motion.md) continues to own animation eligibility and source offsets.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-MARKDOWN-MATH-001 | Shared pipeline, Renderer integration, Presentation and mobile |
| REQ-UI-MARKDOWN-MATH-002 | Syntax compatibility, Safety and failure behavior, Verification |

## Shared pipeline

Add `remark-math`, `rehype-katex`, and `katex` as web runtime dependencies.
Add `@types/katex` as a development dependency. Update `apps/pnpm-lock.yaml` through pnpm.
Import `katex/dist/katex.min.css` from the Vite entry at `apps/web/src/main.tsx`
before the application stylesheet. This lets Vite bundle the referenced font
assets. Fonts remain bundled assets, without a CDN dependency.

`components/shared/markdown-components.tsx` exports both plugin lists.
The remark list retains GFM, breaks, and emoji, then adds math and any required syntax compatibility transform.
The rehype list contains KaTeX with trust disabled and the standard HTML/MathML output.
Use `PluggableList` where available instead of adding another untyped array.

## Syntax compatibility

The requested examples define the contract. Default plugin behavior alone is insufficient evidence for currency or single-line display support.
First, reproduce those cases with the installed library versions.
A small proposed `lib/markdown/remark-math-compat.ts` transform addresses confirmed parser gaps after `remarkMath`.
It operates on parsed math nodes and their original source positions, without a whole-document regular-expression rewrite.

For single-dollar inline nodes, inspect the original delimiter-adjacent characters.
If either inner character is whitespace, restore the exact source slice as a text node.
This preserves `$100 and $200`, whose apparent closing dollar has preceding whitespace.
Keep ordinary `$E = mc^2$`, `$2x$`, and `$100$` valid.
Escaped delimiters and code nodes remain outside this transform.
Ambiguous unescaped dollar pairs beyond this rule retain standard math parsing. Literal dollars can use backslash escapes.

A paragraph containing only a double-dollar expression becomes a display math node if the parser produced inline math.
Use the source slice to distinguish double-dollar delimiters from single-dollar syntax.
Preserve source positions and create the standard math HAST markers through the library's node shape.
Multiline display math remains native parser behavior.
An expression embedded within a larger sentence retains native inline behavior.

## Renderer integration

All paths in this section are relative to `apps/web/`.

| Consumer | Rehype order |
| --- | --- |
| `components/shared/memoized-markdown.tsx` | Shared KaTeX plugins, then motion plugins |
| `components/task/markdown-preview-content.tsx` | Raw HTML, sanitizer with math markers, shared KaTeX plugins |
| `components/task/simple/markdown-comment.tsx` | Existing comment sanitizer with math markers, shared KaTeX plugins |
| `components/task/chat/messages/kandev/document-renderers.tsx` | Shared KaTeX plugins |

`MemoizedMarkdown` memoizes the combined plugin array and retains the normalized source for motion offsets.
KaTeX-generated nodes have no source mapping suitable for text animation.
Regression tests must prove that no motion spans enter generated HTML or MathML.
If necessary, explicitly skip KaTeX subtrees in `lib/markdown/chat-text-motion-plugin.ts`.

The same shared-plugin wiring also applies to these existing consumers:

- `components/github/pr-shared.tsx`
- `components/azure-devops/azure-devops-work-item-detail.tsx`
- `components/release-notes/release-notes-dialog.tsx`
- `components/diff/walkthrough-step-card.tsx`
- `components/diff/review-finding-card.tsx`
- `components/task/chat/queued-ghost-message.tsx`
- `components/settings/changelog-list.tsx`

Specialized clarification, share-snapshot, and authentication-hint pipelines stay outside this change.
Their plugin lists do not consume the shared math parser.

## Safety and failure behavior

Raw HTML remains enabled only on the existing preview surface.
`rehypeRaw` remains immediately followed by `rehypeSanitize`.
Extend each existing schema's code class allowlist with exact `math-inline` and `math-display` values.
Retain the existing language-class rule and all other attributes and protocol rules.
A small shared schema helper or constant can prevent the two schemas from drifting.
Do not permit arbitrary styles, HTML attributes, or MathML from raw input.
KaTeX runs after sanitization and supplies its own generated markup.

This follows the upstream [sanitizer math example](https://github.com/rehypejs/rehype-sanitize#example-math).
The [KaTeX plugin documentation](https://github.com/remarkjs/remark-math/tree/main/packages/rehype-katex) describes math markers and rendering behavior.
Trust remains false. Do not share mutable macro definitions across messages.
Invalid TeX uses the plugin's nonfatal fallback. Surrounding Markdown remains usable.
No global exception handler, new telemetry, or new localized UI copy is required.

## Presentation and mobile

Inline math participates in existing prose. Display math occupies a separate reading region.
The shared stylesheet supplies bounded horizontal scrolling for wide display formulas.
Ensure the left edge remains reachable when KaTeX centers an oversized expression.
Scope overflow styles to math containers rather than every generated span.
Preserve MathML accessibility and the visual HTML tree's existing accessibility attributes.

The phone entry point remains Task Chat or the current file preview.
The nearest exemplar is `e2e/tests/chat/mobile-markdown-wrap.spec.ts`, which proves local overflow containment.
The existing full-height chat or viewer remains the vertical scroll owner.
Formula overflow adds only a local horizontal reading region, with no new toolbar, drawer, or navigation.
Existing safe areas and composer placement remain unchanged.
The desktop and phone compositions are identical at the formula level.

## Verification

Component tests use the real shared plugins and component overrides.
They cover both sanitized pipelines, motion composition, currency, code, links, and invalid TeX.
Preview tests retain source-line comment coverage and existing raw-HTML safety assertions.
Browser tests prove bundled fonts, theme rendering, and phone overflow containment.
The [work order](../../../plans/markdown-math/task-01-render-math.md) owns exact commands and evidence.

## Documentation

During implementation, add a short Markdown math section to `docs/public/sessions-and-review.md`.
Describe inline math, display math, literal dollar escaping, and the supported reading surfaces.
This is reference content. Do not claim TipTap or share exports support formulas.
