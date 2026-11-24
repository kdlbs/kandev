---
created: 2026-10-26
status: complete
requirements:
  - REQ-UI-CLARIFICATION-MARKDOWN-001
  - REQ-UI-CLARIFICATION-CONTEXT-001
system_design:
  - ../../specs/ui/system-design/clarification-markdown.md
  - ../../specs/ui/system-design/clarification-context.md
legacy_specs: []
---

# Implementation plan: Clarification context usability

## Overview

Issue [#4810](https://github.com/kdlbs/kandev/issues/4810) needs readable context, usable expansion, and concise agent guidance.
Deliver three sequential work orders: context formatting, host-bounded expansion, then authoring guidance.
The first two orders include their desktop and phone browser evidence. The third preserves transport and wait contracts.

## Evidence and root cause

Read-only tracing and existing tests establish the cause:

- `clarification-input-overlay.tsx` renders `sharedContext` in a literal pre-wrap div.
- Its Markdown component test explicitly expects context backticks and no code element.
- `clarification-markdown.tsx` excludes table elements even though it uses GFM.
- `clarification-panel-section.tsx` uses `maxHeightVh` for CSS and the resize hook.
- `use-resizable-clarification-overlay.ts` clamps every drag to that same ratio.
- At an 800px viewport, Quick Chat clamps to 280px. Upward dragging from 280px therefore cannot grow the panel.
- Task chat and Needs You use 50%. Quick Chat uses 35%. Threads additionally reserves an 80px transcript floor.
- The tool and context-field descriptions contain no brevity target.

The four existing focused test files passed all 63 tests on 2026-10-26.
Those tests prove current behavior, including the intentional literal assertion and clamp.
No live pointer gesture, private screenshot, or installed-version reproduction was available.
Additional pointer interception or parent constraints remain possible. Browser RED tests must establish the synthetic geometry failure before implementation.
No temporary runtime, reproduction file, production edit, or permanent test edit was created during investigation.

## Requirement reconciliation

The issue explicitly requests changing the literal-context boundary, not preserving it as an active requirement.
Amend MARKDOWN .4/.6 and add .8. Amend CONTEXT .2/.6 and add .9-.12 in the existing owning requirements.
UI owns this reusable contract across independent clarification hosts. The task system retains lifecycle and barrier ownership.
The draft context design covers UI geometry and agent authoring guidance as one context outcome.
Existing parser and ownership boundaries remain intact, so no new ADR is needed.

Completed companion plans remain historical delivery records:
[rendering](../clarification-context-rendering/plan.md), [paragraphs](../clarification-context-newlines/plan.md),
[Markdown](../clarification-markdown/plan.md), and [Threads scrolling](../threads-grid-clarification-overflow/plan.md).
This package replaces their literal-context and fixed user-resize ceilings prospectively.
It preserves their paragraph storage, answer semantics, and Threads containment regressions.
Do not rewrite their recorded test results as evidence for this package.

## Scope

In scope: context-only Markdown tables, safe links, separate initial/manual sizing, accessible expansion/reset, host remeasurement, and concise guidance across Task chat, Quick Chat, Needs You Inbox, Coordinator Needs You, and Threads.
Out of scope: rich comment rendering, History context formatting, hard word limits, schema migrations, new persistence, and lifecycle changes.

## Technical approach

Use the existing renderer with an explicit context variant. Keep block/inline variants unchanged.
Replace the single ratio with an initial ratio and measured host budget shared by CSS and drag clamping.
Supply explicit host allocation and sibling reservation refs at every clarification caller. Use the PageShell scroll viewport for Needs You Inbox and the Threads tile body for its footer allocation.
Keep desktop and phone desired heights separate from effective clamped geometry.
Use existing header buttons for keyboard/touch Expand and Reset. Retain mouse dragging and double-click reset.
See the [context design](../../specs/ui/system-design/clarification-context.md) for allocation, short-host fallback, and scroll ownership.
Update only `ask_user_question_kandev` descriptions and its conditional system-prompt section for brevity guidance.

## ASCII UI preview

### UI-01: Pending context, desktop

Entry: task chat, Quick Chat, Needs You Inbox, Coordinator Needs You, or a Threads tile. State: pending bundle.

```text
Before: initial cap is also the drag ceiling
[Question header                         Collapse]
[**literal context** and | pipe | table |       ]
[question and choices farther down the scroller ]

After: drag or Expand grows within the host budget
[Question header      Expand  Reset      Collapse]
[formatted context and HTTP(S) reference          ]
[table: horizontal scrolling inside this region  ]
[question                                       ]
[choice A] [choice B] [existing submit/skip flow  ]
```

### UI-02: Pending context, phone

```text
[Question header]
[Expand] [Reset] [Collapse]
[formatted context       ]
[table: local horizontal ]
[question                ]
[choice A                ]
[choice B                ]
[existing answer actions ]
[safe-area clearance     ]
```

The existing header may wrap controls on phones. Context stays once above the active question.
The bounded body has one active vertical scroll owner. Header/actions remain reachable through that owner.
Expand and Reset are touch targets of at least 44px. Desktop controls retain existing density.
The phone uses the existing inline conversation surface, not an added drawer or stacked desktop panels.
Expand uses available space. Reset restores content sizing. Collapse hides the body without clearing answers.
When no headroom exists, Expand is disabled. A new bundle restores default sizing.
Copy and spacing are illustrative. Grouping, containment, safe links, and input-state preservation are required.
UI-01/UI-02 map to MARKDOWN .8 and CONTEXT .1-.6, .9-.11.

## Tests

- Task 01 maps MARKDOWN .4/.6/.7/.8 and CONTEXT .1-.8 to renderer/overlay tests and desktop/mobile context browser flows.
- Task 02 maps CONTEXT .9-.11 to hook/panel tests and actual host geometry, mouse, keyboard, and touch flows.
- Task 03 maps CONTEXT .7/.12 to schema, handler, and conditional system-prompt tests.

## E2E tests

Use synthetic paragraphs, emphasis, inline code, links, a small table, long URLs, and enough text to reach the initial cap.
Exercise both the legacy string and paragraph-array paths through the mock agent.
Task 01 extends `tests/chat/clarification.spec.ts` and `tests/chat/mobile-clarification.spec.ts`.
Task 02 extends `tests/chat/clarification-resize.spec.ts` and creates `tests/chat/clarification-context-hosts.spec.ts`.
It also extends `mobile-clarification.spec.ts` and the desktop/mobile `threads-composer-disclosure.spec.ts` files.
The new host spec covers Quick Chat and Needs You using their existing fixture patterns. The Coordinator Needs You browser test also expands its shared question panel.
Run desktop and mobile commands sequentially with the managed runner and fresh builds.
Stub HTTP references locally. No external website or private data is needed.

## Work orders

- [x] [Task 01: Render safe shared context](task-01-context-markdown.md)
- [x] [Task 02: Expand within measured host bounds](task-02-host-bounded-expansion.md)
- [x] [Task 03: Guide concise decision context](task-03-agent-context-guidance.md)

Order: 01, then 02, then 03. No subagents are authorized.
Exact commands and file ownership are in each work order.

## Verification results

Baseline command from `apps/web`:

```bash
pnpm exec vitest run hooks/use-resizable-clarification-overlay.test.ts components/task/chat/clarification-input-overlay.test.tsx components/task/chat/clarification-panel-section.test.tsx components/task/chat/clarification-markdown.test.tsx
```

Passed: 4 files, 63 tests. Task 01 completed with Markdown, host geometry, and mobile browser evidence. Tasks 02 and 03 are complete.

Implementation verification:

- Task 02: 137 UI unit tests passed; 24 Chromium and 19 mobile Chromium tests passed; typecheck, zero-warning changed-file ESLint, i18n checks/ratchet, public-doc validation, and `git diff --check` passed.
- Final `pnpm run build` from `apps/web`: passed.
- Task 03: MCP question/schema and system-prompt tests passed; changed-Go lint reported 0 issues. Long legacy context stays verbatim, non-empty paragraphs keep precedence, and empty paragraph arrays keep the legacy fallback.
- Browser tests covered measured expansion in Quick Chat, Needs You Inbox, Coordinator, and Threads Grid/Columns, plus Pixel 5, short-phone, and exhausted-host paths. Physical-device keyboard behavior was not tested.
- Final `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: 59 changed paths, `covered`, all three work orders selected, and no errors.
- Final `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, `node scripts/validate-public-docs.mjs`, and `git diff --check`: passed.

Design validation passed on 2026-10-26:

- `python3 scripts/list-docs.py validate`: 403 decisions and 1741 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: all three work orders covered, with no reference errors.
  The preflight included planned renderer, resize-hook, and MCP source changes.
- `git diff --check -- docs/plans/clarification-context-usability docs/specs/ui`: passed.
- All package files remain unstaged and uncommitted. No production or permanent test files changed.

## Risks

- Parent geometry can invalidate a viewport-only fix. Require real host bounds and gesture evidence.
- Threads already has a bounded footer. Preserve its transcript floor and avoid competing scroll owners.
- Markdown rendering changes presentation but must not rewrite stored context or option payloads.
- Keyboard viewport emulation does not prove physical iOS/Android keyboard behavior.
- Guidance cannot guarantee model brevity. The UI must handle longer valid content.
