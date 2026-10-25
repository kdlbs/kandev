---
created: 2026-09-22
status: implemented
requirements:
  - REQ-UI-MARKDOWN-MATH-001
  - REQ-UI-MARKDOWN-MATH-002
system_design:
  - ../../specs/ui/system-design/markdown-math.md
legacy_specs: []
---

# Implementation Plan: Markdown Math

## Overview

One sequential work order delivers shared math rendering and its regression evidence.
The shared parser, renderer wiring, and dependency lockfile belong in the same implementation pass.
The implementation is complete after the explicit implementation request.

## Scope

### In scope

- [Formula and compatibility requirements](../../specs/ui/requirements/markdown-math.md).
- Shared KaTeX dependencies, stylesheet, parser, sanitizer schemas, and renderer integration.
- Desktop and phone tests, plus a short public syntax reference.

### Out of scope

- Formula editing, standalone TeX compilation, backend changes, and new preferences.
- Specialized pipelines identified in the paired design.

## Technical approach

Use the [system design](../../specs/ui/system-design/markdown-math.md) as the renderer inventory and pipeline authority.
Start with failing tests for the requested examples before integrating dependencies and plugins.
Use parsed-node compatibility only where installed plugin behavior fails the required syntax cases.
Retain source offsets, sanitizer ordering, and existing motion behavior.
Apply math rendering to every consumer of the shared remark list in the same work order.

## ASCII UI preview

### UI-01: Formula reading, desktop and phone

Entry: an existing chat message, document body, comment, or file preview.
State: a complete formula inside surrounding prose.

```text
+------------------------------------+
| Energy: E = mc^2                    |
|                                    |
|                  a                 |
|                 ---                |
|                  b                 |
|                                    |
| Cost: $100 and $200                 |
+------------------------------------+
```

The sketch uses ASCII approximations for typeset math. Formula spacing is illustrative.
Required structure: inline formulas stay in prose, display formulas use a separate region, and currency stays literal.
On phones, wide display formulas scroll horizontally within that region.
The existing chat or preview owns vertical scrolling. Navigation and controls do not change.
An incomplete expression remains readable until it closes. Invalid TeX shows a nonfatal fallback.
This view maps to AC-UI-MARKDOWN-MATH-001.1 through 001.6 and AC-UI-MARKDOWN-MATH-002.1.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| 001.1, 001.2, 002.1, 002.2 | `markdown-components.test.tsx`: inline, standalone display, multiline display, currency, escapes, and code |
| 001.3, 002.4 | New `markdown-math-renderers.test.tsx`: four primary renderer integrations and sanitizer safety |
| 001.6, 002.5 | `chat-markdown-motion.test.tsx`: appended formulas and prose with animation enabled and disabled |
| 002.3, 002.5 | Existing markdown and preview suites, plus focused formula/source-position regressions |

All abbreviated criteria use the prefix `AC-UI-MARKDOWN-MATH-`.
New compatibility logic gets unit coverage in `lib/markdown/remark-math-compat.test.ts` if required.
Tests must use real plugins for rendering assertions. Existing mocked memoization tests remain distinct.

## E2E tests

Add `e2e/tests/chat/markdown-math.spec.ts` for the `chromium` project.
Add `e2e/tests/chat/mobile-markdown-math.spec.ts` for the `mobile-chrome` project.
Use the mock-agent message fixture and existing file-preview helper patterns.

- Render inline and display formulas in chat and file previews, including the supplied nested SPC fraction.
- Verify light and dark themes, loaded KaTeX fonts, and visible fractions.
- Verify currency and malformed input beside valid formulas.
- On phones, verify local formula scrolling, reachable formula edges, and no document overflow.
- Preserve existing Markdown preview HTML-safety and comment-selection tests.

These flows cover 001.1 through 001.6, 002.1, 002.4, and 002.5.
Component tests cover comments and tool document bodies without adding artificial browser routes.

## Work orders

- [x] [Task 01: Render math across shared Markdown](task-01-render-math.md) (`done`, wave 1, no dependencies)

## Verification results

Design validation passed on 2026-09-22:

- Catalog validation: 299 decisions and 1110 specifications.
- Specification linter tests: 36 passed.
- Full specification lint: passed.
- Whitespace validation: passed.

Implementation validation passed on 2026-09-22:

- TDD RED was recorded before plugin wiring; the final focused suite passed 10 files and 63 tests, including preview source/comment mapping, raw delimiter whitespace, and sanitizer marker regressions.
- Typecheck, lint, and the production Vite build passed, with KaTeX font assets bundled by Vite.
- Chromium browser coverage passed 11 tests, including light and dark chat themes, KaTeX font loading, sanitized file preview, and existing Markdown preview flows.
- Mobile Chromium coverage passed 2 tests for local display-formula overflow in chat and the native file preview viewer.
- Specification catalog validation, full specification lint, public docs tests, public docs validation, and whitespace checks passed.

## Risks

- Default math parsing can misread currency or classify single-line double-dollar formulas as inline math.
- Sanitization can remove display markers, and custom code components can intercept unconverted math nodes.
- Typography styles and centered wide formulas can clip content on phones.
- A missed shared-plugin consumer can parse formulas without rendering them.
- Preview source positions and chat motion offsets can drift after careless string preprocessing.
