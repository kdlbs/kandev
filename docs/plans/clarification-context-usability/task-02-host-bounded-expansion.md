---
id: "02-host-bounded-expansion"
title: "Expand within measured host bounds"
status: done
wave: 2
depends_on: ["01-context-markdown"]
plan: "plan.md"
requirements:
  - REQ-UI-CLARIFICATION-CONTEXT-001
acceptance_criteria:
  - AC-UI-CLARIFICATION-CONTEXT-001.9
  - AC-UI-CLARIFICATION-CONTEXT-001.10
  - AC-UI-CLARIFICATION-CONTEXT-001.11
system_design:
  - ../../specs/ui/system-design/clarification-context.md
---

# Task 02: Expand within measured host bounds

## Summary

Separate automatic initial caps from user-selected expansion bounds.
Deliver measured host integration, keyboard/touch controls, and regression evidence together.

## In scope

- Rename the initial-cap prop and provide allocation/reservation refs at every caller: Task chat, Quick Chat, Needs You Inbox, Coordinator Needs You, and Threads.
- Observe host and visual viewport changes. Keep CSS and drag clamping on one effective maximum.
- Preserve form, bundle, collapse, reset, and desktop/phone sizing state.
- Add localized Expand/Reset controls in the existing header. Keep the shared mouse grip behavior compatible.
- Preserve the Threads transcript floor and implement the sole-scroll-owner fallback for exhausted host budgets.

## Out of scope

Generic composer resizing, persisted geometry, question lifecycle, and new navigation or drawers.

## Acceptance

1. `expands capped context beyond its initial limit` fails before implementation using actual upward pointer movement and measured browser bounds.
2. All hosts preserve their initial ratios, grow only within measured space, shrink, and clamp on viewport/parent changes. Coordinator Needs You uses its visible question anchor and clipping viewport.
3. Collapse retains answers and desired height. New bundles/reset restore defaults. Keyboard/touch controls work without overwriting desktop state.

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

See the [combined preview](plan.md#ascii-ui-preview). This order delivers the sizing controls and all geometry states.

## Regression matrix

- Hook tests: measured start, separate automatic/manual caps, downward drag, host shrink/grow, missing measurement, cleanup, and sub-120px budget.
- Panel tests: collapsed form remains mounted, bundle reset, explicit reset, no-headroom button, responsive height separation, and unchanged answer payloads.
- `clarification-resize.spec.ts`: long context at the initial cap, short context auto-sizing, upward/downward gestures, keyboard Expand/Reset, and viewport contraction.
- New `clarification-context-hosts.spec.ts`: Quick Chat and Needs You real host bounds and reachable answers, including a short viewport.
- `coordinator/needs-you.spec.ts`: the Coordinator question card uses the shared measured expansion and reset controls.
- `mobile-clarification.spec.ts`: touch Expand/Reset, long context/table, scroll to answers, no document overflow, and desktop-phone-desktop draft preservation.
- Threads desktop/mobile specs: Grid and Columns, short tile, 80px transcript floor, footer fallback, and reachable final options/Submit.
- Include 767px and 768px width checks plus canonical Pixel 5 and narrow fine-pointer controls. Measure hit targets, not class strings.
- Simulate visual viewport contraction and keep input/Submit reachable. Record physical keyboard testing as unverified unless actually performed.

Do not replace the old 50vh assertions with an arbitrary larger constant.
Retain their initial-sizing coverage and assert manual bounds against the measured host.
Use actual mouse/touch input for reachability. Auto-scrolling locator clicks do not establish scroll correctness.

## Verification

From the repository root, after task 01. Managed browser runs build current assets and run sequentially.

```bash
(cd apps/web && pnpm exec vitest run hooks/use-resizable-clarification-overlay.test.ts components/task/chat/clarification-panel-section.test.tsx components/task/chat/clarification-input-overlay.test.tsx components/task/chat/composer-disclosure.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/clarification-resize.spec.ts tests/chat/clarification-context-hosts.spec.ts tests/coordinator/needs-you.spec.ts tests/task/threads-composer-disclosure.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-clarification.spec.ts tests/task/mobile-threads-composer-disclosure.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/use-resizable-clarification-overlay.ts components/task/chat/clarification-panel-section.tsx components/task/chat/clarification-input-overlay.tsx components/task/chat/clarification-overlay-header.tsx components/task/chat/composer-disclosure.tsx components/task/task-chat-panel.tsx components/quick-chat/quick-chat-content.tsx components/needs-you-inbox/needs-you-inbox-row.tsx app/needs-you-inbox/needs-you-inbox-page-client.tsx components/page-shell.tsx app/coordinator/components/question-answer.tsx components/threads/thread-column.tsx e2e/tests/chat/clarification-resize.spec.ts e2e/tests/chat/clarification-context-hosts.spec.ts e2e/tests/chat/mobile-clarification.spec.ts e2e/tests/coordinator/needs-you.spec.ts e2e/tests/task/threads-composer-disclosure.spec.ts e2e/tests/task/mobile-threads-composer-disclosure.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
node scripts/validate-public-docs.mjs
git diff --check
```

Inspect captured desktop/phone views against UI-01/UI-02. Record actual geometry and any unsupported hardware checks.

## Files likely touched

- `apps/web/hooks/use-resizable-clarification-overlay.ts` and its test file.
- `apps/web/components/task/chat/clarification-panel-section.tsx` and its test file.
- `apps/web/components/task/chat/clarification-input-overlay.tsx` and its test file.
- `apps/web/components/task/chat/composer-disclosure.tsx` and its test file.
- `apps/web/components/task/task-chat-panel.tsx`.
- `apps/web/components/quick-chat/quick-chat-content.tsx`.
- `apps/web/components/needs-you-inbox/needs-you-inbox-row.tsx`, `apps/web/app/needs-you-inbox/needs-you-inbox-page-client.tsx`, and `apps/web/components/page-shell.tsx`.
- `apps/web/app/coordinator/components/question-answer.tsx` and `apps/web/app/coordinator/components/needs-you-item-relay.test.tsx`.
- `apps/web/components/threads/thread-column.tsx` and `apps/web/components/task/chat/composer-disclosure.tsx`.
- The six browser specs named in Verification and their existing host helpers.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/chat.json` and generated pseudo catalog.
- `docs/public/sessions-and-review.md` for expansion/reset instructions.

Add copy in all seven languages. Use `pnpm run i18n:zh-hant` from `apps/web` for Traditional Chinese.
If extracted geometry helpers need separate files, add those exact paths and tests to this work order before implementation.

## Dependencies

Task 01 supplies the synthetic long-context scenario and renderer.

## Risks

Self-measurement can cause observer loops. Measure the host allocation and siblings, never the growing content as its maximum.
Preserve the Threads outer boundary and avoid introducing two active vertical scroll owners.

## Parallelism

`sequential`

## Inputs

- Context design: Host geometry and height state, Accessible desktop and phone interaction.
- Existing resize browser test and `threads-clarification-helpers.ts`.
- Existing Quick Chat and Needs You fixture patterns.

## Results

Completed. Desired phone and desktop heights are cached by task and clarification bundle, so responsive remounts preserve the current bundle's size and drafts without reusing state for a different task or bundle. Expansion controls and the mouse grip share measured host bounds; the short Threads fallback keeps its footer as the only vertical scroll owner. The mobile Submit action has enough width to keep Skip and Submit hit targets separate when the header wraps.

- Focused unit suite: 9 files, 137 tests passed. After extracting the drag event lifecycle to satisfy the function-complexity limit, the resize hook, panel, and cache suite passed again (3 files, 31 tests).
- `pnpm e2e:run --project chromium tests/chat/clarification-resize.spec.ts tests/chat/clarification-context-hosts.spec.ts tests/coordinator/needs-you.spec.ts tests/task/threads-composer-disclosure.spec.ts`: 24 tests passed. Measured expansion passed for Quick Chat, Needs You Inbox, Coordinator, and Threads Grid/Columns.
- `pnpm e2e:run --project mobile-chrome tests/chat/mobile-clarification.spec.ts tests/task/mobile-threads-composer-disclosure.spec.ts`: 19 tests passed, including Pixel 5, short-phone, exhausted-host, and responsive draft/size preservation flows.
- `pnpm run typecheck`, focused ESLint with `--max-warnings 0`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`: passed.
- `node scripts/validate-public-docs.mjs` and `git diff --check`: passed.
- Browser evidence covers emulated keyboard viewport contraction. Physical-device keyboard behavior was not tested.

### Review remediation (2026-10-26)

Fixed the four host-resizing review findings. Expand availability now compares the measured host budget with the panel's rendered height, with a 2px geometry tolerance; short auto-sized content retains Expand when it has headroom. Viewport height changes are tracked even when the host budget stays fixed. The rendered minimum follows the host budget, including zero and unavailable measurements. Threads footer fallback clears the rendered manual height while preserving the desired height for restoration.

- Focused hook and panel suite: 2 files, 33 tests passed. Coverage includes partial drag then Expand, host shrink/growth, short auto-sized content, viewport-only height changes, host-aware minimums, and manual-height fallback transitions.
- `pnpm e2e:run --project chromium e2e/tests/chat/clarification-resize.spec.ts e2e/tests/chat/clarification-context-hosts.spec.ts`: 7 tests passed, including Quick Chat's real rendered bounds below a 120px host and partial-drag then Expand.
- `pnpm e2e:run --project mobile-chrome e2e/tests/task/mobile-threads-composer-disclosure.spec.ts -- --grep "drops expanded height in the footer fallback"`: 1 test passed, including shrink to footer fallback, final-option reachability, height restoration, and composer bounds.
- `pnpm run typecheck` and focused ESLint with `--max-warnings 0`: passed.

- Post-rebase verification (2026-10-26): the focused Vitest set passed (7 files, 101 tests); full web ESLint, typecheck, Vite production build, `i18n:check`, `i18n:ratchet`, public-doc validation, changed-file Go lint, backend tests for the changed packages, and `make -C apps/backend build` passed. Desktop clarification coverage passed 53 tests; mobile clarification and Threads coverage passed 20 tests with retries disabled. Desktop and phone PR captures were generated and visually inspected.
