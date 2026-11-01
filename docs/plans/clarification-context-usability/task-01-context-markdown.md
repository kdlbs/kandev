---
id: "01-context-markdown"
title: "Render safe shared context"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CLARIFICATION-MARKDOWN-001
  - REQ-UI-CLARIFICATION-CONTEXT-001
acceptance_criteria:
  - AC-UI-CLARIFICATION-MARKDOWN-001.4
  - AC-UI-CLARIFICATION-MARKDOWN-001.6
  - AC-UI-CLARIFICATION-MARKDOWN-001.7
  - AC-UI-CLARIFICATION-MARKDOWN-001.8
  - AC-UI-CLARIFICATION-CONTEXT-001.1
  - AC-UI-CLARIFICATION-CONTEXT-001.2
  - AC-UI-CLARIFICATION-CONTEXT-001.3
  - AC-UI-CLARIFICATION-CONTEXT-001.4
  - AC-UI-CLARIFICATION-CONTEXT-001.5
  - AC-UI-CLARIFICATION-CONTEXT-001.6
  - AC-UI-CLARIFICATION-CONTEXT-001.7
  - AC-UI-CLARIFICATION-CONTEXT-001.8
system_design:
  - ../../specs/ui/system-design/clarification-markdown.md
  - ../../specs/ui/system-design/clarification-context.md
---

# Task 01: Render safe shared context

## Summary

Render pending shared context with the existing parser and a context-only table policy.
Preserve storage and answer behavior. Add rendered desktop and phone evidence in the same slice.

## In scope

- Add the context variant and semantic tables with local horizontal overflow.
- Replace the literal shared-context div without changing the context reader or carousel.
- Add synthetic long-context mock scenarios for both input representations.
- Update the clarification-formatting section in `docs/public/sessions-and-review.md`.

## Out of scope

Panel resizing, History context, answer labels, raw HTML, and agent guidance.

## Acceptance

1. `renders shared context Markdown and tables without changing answers` fails on literal rendering before implementation, then passes.
2. Tests cover blank/plain context, paragraphs, long URLs, unsafe schemes, HTML suppression, and unchanged block/inline policies.
3. Desktop and phone browser flows open a safe context link without state mutation, read the table, and complete the question.

## ASCII UI preview

### UI-01: Pending context, desktop

Entry: task chat, Quick Chat, Needs You, or a Threads tile. State: pending bundle.

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

This order delivers only the formatted body in UI-01/UI-02. Expand and Reset arrive in task 02.
See the [combined preview](plan.md#ascii-ui-preview).

## Verification

From the repository root. In a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/clarification-markdown.test.tsx components/task/chat/clarification-input-overlay.test.tsx)
(cd apps/backend && go test ./cmd/mock-agent -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/clarification.spec.ts -- --grep 'context|markdown')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep 'context|markdown')
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/chat/clarification-markdown.tsx components/task/chat/clarification-input-overlay.tsx e2e/tests/chat/clarification.spec.ts e2e/tests/chat/mobile-clarification.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Name new browser tests with `context` so these selections include them.
Capture and inspect desktop and phone screenshots during the browser checks.
Run RED before wiring the renderer, then repeat the commands after implementation.

## Files likely touched

- `apps/web/components/task/chat/clarification-markdown.tsx` and `clarification-markdown.test.tsx`.
- `apps/web/components/task/chat/clarification-input-overlay.tsx` and `clarification-input-overlay.test.tsx`.
- `apps/backend/cmd/mock-agent/scenarios.go` and `scenarios_test.go`.
- `apps/web/e2e/tests/chat/clarification.spec.ts` and `mobile-clarification.spec.ts`.
- `docs/public/sessions-and-review.md`.

## Dependencies

None.

## Risks

A global allowlist change would broaden option and summary formatting. Keep table support context-specific.
Links must not bubble into a host's answer, collapse, or row-toggle behavior.

## Parallelism

`sequential`

## Inputs

- Both referenced designs and their mapped requirements.
- Existing renderer security tests and the intentional literal-context assertion near overlay test line 718.
- Existing mobile clarification and reference-popup helpers.

## Results

Completed. The renderer tests failed against the literal context path before the context variant was added, then passed after implementation.

- `pnpm exec vitest run components/task/chat/clarification-markdown.test.tsx components/task/chat/clarification-input-overlay.test.tsx`: 2 files, 37 tests passed.
- `go test ./cmd/mock-agent -count=1`: passed.
- `pnpm e2e:run --no-build --project chromium tests/chat/clarification.spec.ts -- --grep 'context|markdown'`: 3 tests passed.
- `pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep 'context|markdown'`: 3 tests passed.
- `pnpm run typecheck`: passed with an 8 GiB Node heap after the default heap limit was reached once.
- Focused ESLint, public-doc tests/validation, and `git diff --check`: passed.
- Desktop and Pixel 5 screenshots were captured and inspected. The table stays inside its local horizontal scroller; the bounded body remains the subject of task 02.
