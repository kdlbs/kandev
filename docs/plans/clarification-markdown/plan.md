---
created: 2026-10-24
status: implemented
requirements:
  - REQ-UI-CLARIFICATION-MARKDOWN-001
  - REQ-UI-INBOX-HISTORY-001
system_design:
  - ../../specs/ui/system-design/clarification-markdown.md
  - ../../specs/ui/system-design/inbox-history.md
legacy_specs: []
---

# Implementation plan: Clarification Markdown consistency

## Overview

Render History question content with the existing limited clarification renderer. Check the active-question link path with the same valid input.
One work order delivers this presentation change, its targeted tests, and a short public documentation update.

The user identified question text and Inbox History. The confirmed defect is raw rendering in History.
Standard active-question formatting already exists. Its reported failure remains unconfirmed without the original text or a browser reproduction.

## Scope

- Format question summaries and expanded titles, prompts, and displayed option labels in History.
- Preserve existing link safety, button behavior, History membership, metadata, and answer identifiers.
- Cover active-question and History links on desktop and phone.
- Add a short clarification-formatting section to `docs/public/sessions-and-review.md` during implementation.

Shared context, permissions, custom answers, plugin chat, file links, rich Markdown, and History query changes are excluded.

## Technical approach

Reuse `ClarificationMarkdown` from `apps/web/components/task/chat/clarification-markdown.tsx` in `InboxHistoryQuestionDetail` and the question-derived row summary.
Use block mode for full prompts and inline mode for titles, labels, and summaries.
Use passive link mode inside the row-toggle button. Expanded text remains outside that button and permits existing HTTP/HTTPS links.
Retain a distinction between question-derived summaries and literal context or permission fallbacks.

Keep prompt rendering independent of title presence. Preserve summary truncation when formatted descendants contain code or line breaks.
Keep source values and `inboxHistoryClarificationQuestions` unchanged unless a demonstrated rendering need requires a small local adjustment.

The [original History package](../inbox-history/plan.md) remains a completed delivery record.
This package adds formatting and reference links. Its work order owns the new tests and results without rewriting historical verification.

## ASCII UI preview

### UI-01: Active question, desktop and phone

Entry: pending clarification in task chat. State: unanswered.

```text
Choose storage
Read storage guidance before choosing.
     ^ underlined reference link
[1  Postgres]
[2  SQLite  ]
```

Shared composition: the phone uses the existing question overlay and scroll region. Long prompt text wraps within that surface.
Reference activation opens its destination. A choice still submits its original identifier.
This previews an existing renderer path to verify, not a confirmed active-question code defect.

### UI-02: History row, expanded desktop

```text
[v  Choose storage]   Superseded   Asked time   [Open task] [Copy identifier]
    Choose storage
    Read storage guidance before choosing.
         ^ underlined reference link
    1. Check retention
    2. Compare costs
    Postgres, SQLite
```

The summary text belongs to the disclosure button. Its reference labels do not navigate.
Expanded reference links are outside that button. Collapsed state hides the detail and keeps one truncated summary line.

### UI-03: History row, expanded phone

```text
[v  Choose storage                  ]
    Superseded   Asked time  [Open] [Copy]
    Choose storage
    Read storage guidance before
    choosing.
    1. Check retention
    2. Compare costs
    Postgres, SQLite
```

The existing phone row keeps its two-line header and icon actions. The page remains the scroll owner.
Expanded text wraps. The summary retains one-line truncation. Action hit targets retain their existing 44px minimum.
If title and prompt are absent, show the existing localized fallback in place of the question text.

Structural requirements are separate disclosure/link activation, title-less prompt visibility, phone wrapping, and preserved action placement.
Copy and spacing are illustrative. Use existing tokens and localized labels.
UI-01 maps to AC .1, .3, .5, .7. UI-02 and UI-03 map to AC .1 through .7 of `REQ-UI-CLARIFICATION-MARKDOWN-001`.

## Tests

Extend existing tests rather than duplicating renderer logic:

- `clarification-markdown.test.tsx`: supported formatting, URL policy, and unsupported elements (AC .1, .4).
- `clarification-input-overlay.test.tsx`: a real prompt hyperlink and unchanged option payload (AC .1, .3, .7).
- `inbox-history-row.test.tsx`: cases named `formats clarification summaries and expanded questions`, `keeps summary links passive and detail links interactive`, `renders title-less prompts as full question content`, and `keeps context-only summaries and permission content literal` (AC .1 through .4, .6, .7).
- Retain `inbox-history-row.mobile.test.tsx` and `inbox-history-row.tablet.test.tsx` as action regressions. Browser tests provide wrapping and truncation evidence (AC .5).

Use TDD for the History defect: observe the new History assertions fail on raw text before wiring the renderer.
Active-question tests may already pass. Record their baseline rather than changing a correct parser.

## E2E tests

- `tests/chat/clarification.spec.ts`, project `chromium`: extend `renders lightweight markdown across active and resolved clarification text` to activate the question reference and check the destination without submitting an answer (AC .1, .3, .7).
- `tests/chat/inbox-history-superseded.spec.ts`, project `chromium`: `renders Markdown question history with independent reference links` seeds a title-less Markdown prompt and formatted option label. It checks passive summary, keyboard-accessible detail reference, lists, existing read-only behavior, and stored text (AC .1 through .4, .6, .7). Component tests cover formatted titles.
- `tests/chat/mobile-clarification.spec.ts`, project `mobile-chrome`: extend `renders lightweight markdown without overflow and submits through formatted content` to tap the prompt reference before selecting an answer (AC .3, .5, .7).
- `tests/chat/mobile-inbox-history-superseded.spec.ts`, project `mobile-chrome`: add `renders Markdown question history without overflow`. Expand by touch, tap the reference, check its destination, long-label containment, summary truncation, and retained action hit targets (AC .1 through .3, .5 through .7).

Use existing fixture APIs and isolated backends. Stub the example destination in the popup so verification does not depend on external networking.
Close created pages and clean owned fixtures. Run desktop and mobile commands sequentially.

## Work orders

- [x] [Task 01: Render consistent clarification Markdown](task-01-render-clarification-markdown.md)

There are no dependencies or authorized subagents. All exact commands are in the work order.

## Verification results

Baseline: workspace dependencies installed with `pnpm install --frozen-lockfile`.
Existing renderer, overlay, transcript, and tool-renderer tests passed: 4 files, 43 tests.
Implementation passed the seven-file component command (65 tests) and all six browser scenarios.
The desktop History spec passed 2 tests, the desktop active-question selection passed 1, the phone History spec passed 2, and the phone active-question selection passed 1.
Final desktop History evidence includes keyboard navigation from the disclosure button through existing row actions to the reference.

Prettier, scoped ESLint, TypeScript, both localization gates, and both public-doc gates passed.
The public-doc validator tests passed all 63 cases and validated all 48 published pages.
TypeScript passed with an 8 GiB heap after its default 4 GiB heap was exhausted.
The [work order](task-01-render-clarification-markdown.md#results) records commands, environment setup, and logs.

Desktop and phone captures were inspected against UI-02 and UI-03, including title-less prompts.
Summary truncation, detail wrapping, independent reference activation, and retained phone actions match the proposed structure.
No actionable UI-polish findings remain in the changed content. No new motion was introduced.

Design-package checks passed:

- `python3 scripts/list-docs.py validate`: 384 decisions and 1618 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs/specs docs/plans`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: the actual documentation diff is exempt. A preflight including the planned History source path is covered by task 01 with no reference errors.

The new requirement is active and its design is current. Both appear in the UI catalog.
All files remain unstaged and uncommitted.

## Risks

- The original active-question link text is unavailable. Passing a standard-link check does not prove that reported case is fixed.
- Inline code and breaks can defeat single-line summary ellipsis. Check rendered styles and bounds, not class names alone.
- History is read-only. Reference links must not introduce response controls or mutate interaction records.
- Public documentation must describe the limited syntax and passive choice/summary links. It must not promise full comment Markdown.
