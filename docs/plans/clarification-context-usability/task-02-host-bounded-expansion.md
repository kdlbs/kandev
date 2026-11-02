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

- PR review fixup (2026-10-26): the panel ref now registers late mounts so its ResizeObserver measures the rendered height; cached resize preferences are limited to the eight most recent bundles; Expand and Reset retain 44px touch targets on coarse-pointer tablets; the Markdown requirement and design documents now identify the context usability package as implemented.
- Review regression tests: hook and resize-state suites passed (2 files, 18 tests); clarification panel/header suite passed (1 file, 22 tests); coarse-pointer tablet E2E passed at 900px (1 test).
- Fixup checks: web typecheck, focused ESLint, `i18n:check`, `i18n:ratchet`, `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, `node scripts/validate-public-docs.mjs`, and `git diff --check` passed. The managed E2E run built the production Vite assets successfully.

- PR CI remediation (2026-10-26): moved the Markdown prompt ahead of image previews so a height-limited phone panel shows the question before its image; adjusted the plugin action touch-target assertion to tolerate subpixel browser rounding through the existing helper.
- The first PR E2E run had one persistent four-attempt failure in the mobile image-question prompt viewport test. Three other tests failed once but passed on retry (Threads auto-scroll, mobile clarification auto-run queue, and plugin-action focus return).
- Local browser verification with retries disabled passed for the exact phone image-question scenario (1 test), the desktop/mobile clarification and Threads suites (28 tests), and the phone image-question and plugin-action suites. Web typecheck and focused ESLint with `--max-warnings 0` passed.
- The fresh phone image-question capture produced and visually validated three assets (question with preview, answer controls, and image viewer). `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.

### PR CI fixup (2026-10-27)

The first PR run on `aa4dfc5` failed six E2E tests across five shards. The runtime-update rollback assertion rejected harmless subpixel rounding; the profile-deletion fixture assumed the first agent had a profile; the guided-setup option and mobile controls were tapped before their animations or hit targets had settled; and pagination unarchived a task while archive cleanup was still running.

- Share a 0.01px touch-dimension tolerance between the plugin-action and runtime-update specs.
- Select the seeded agent profile by ID in all mobile profile-deletion tests.
- Wait for the guided-setup popover and queued-message panel animations. Scroll the sidebar retry into view and confirm its center owns the hit target before tapping.
- Wait for each archive-cleanup job to finish before the pagination test unarchives its task. The failing backend log identified the cleanup lifecycle race.
- Exact-merge reruns after a frozen workspace install and build passed: profile-deletion (3 tests), guided setup (4), sidebar recovery (1), sidebar pagination (1), mobile Auto-run clarification (1), runtime rollback (1), and plugin action touch targets (1).
- The synthetic-merge backend, E2E fixture packages, and E2E Vite build passed. Web typecheck and focused ESLint passed.
- The subsequent PR run on `2f63492` had three flaky E2E leaf failures (guided setup, pseudo-locale task coverage, and Threads Grid footer auto-scroll) plus the dependent aggregate gate. The two unchanged specs passed 3 and 10 local no-retry repetitions. The guided setup helper now binds each option list to its selector's `aria-controls` popover, waits for it to open and close, and passed 10 local no-retry repetitions. The updated PR head still needs a clean full CI run to confirm the other two failures were transient.

- PR CI follow-up (2026-10-27): the run on `75169c6` completed with 82 checks passed, 12 skipped, and four retry-only E2E failures across three leaf shards plus the dependent aggregate gate. The retry summary showed every affected test passed on its second attempt. Blob audit confirmed exactly four failed attempts and no other unexpected statuses. Exact-order replays of all three failed shard manifests in the CI runtime container passed with retries disabled (shard 2: 142 passed, 1 skipped; shard 6: 134 passed, 2 skipped; shard 15: 150 passed, 2 skipped).
- Fixed the readiness races visible in the failure artifacts: conversation search now waits until the seeded conversation is returned by the backend index, the workflow test waits for the editable idle composer after the step cascade, and the mobile changed-file tests reload finalized history after the backend confirms capture completion.
- Local no-retry verification after these changes passed: mobile conversation search and mobile turn-changed-files (4 tests), plus the full workflow lifecycle (1 test). The managed runs built the production Vite assets. Focused ESLint and web typecheck passed.
- A fresh full PR check run on the updated head is pending; do not treat the preceding retries or local shard replays as a clean PR CI result.

- PR fixup follow-up (2026-10-27): full CI run `40451012868` on head `62800d1` completed with 82 checks passed, 12 skipped, and four retry-only E2E failures across shards 5, 12, 14, and 18, plus the dependent aggregate gate. The retry summary recorded zero final test failures. The affected cases were new-session submission, mobile executor-profile selection, nested file-tree visibility after a directory move, and the Threads Columns clarification load.
- Added causal readiness checks for backend session creation and dialog closure, the mobile executor drawer's finite animation and closed state, virtualized nested file-tree rows, and the loaded Threads layout/question. The fresh main-base merge is included in the local fixup head.
- Managed no-retry verification passed: the three Chromium specs covering four affected cases (4 tests, 32.4s) and the mobile executor compatibility case (1 test, 10.2s). Both runs built the production Vite assets. This run predates the pushed fixup head and does not replace its fresh PR CI evidence.

- PR fixup follow-up (2026-10-27): full CI run `40455756395` on head `b7413b1` completed with 81 checks passed, 13 skipped, and five failed E2E shards plus the dependent aggregate. The E2E gate rejects retry-only failures. Four tests passed on retry; the sole final failure was the mobile Quick Chat cancellation test's assertion for a disabled cancel button after the session had already settled.
- Tightened the affected browser checks: wait for the canvas task layout, create a genuinely dirty incomplete config-sync draft, center the plugin uninstall anchor before opening its confirmation, match the repository option exactly, and await Quick Chat's backend cancellation-pending state before checking the disabled control.
- Managed no-retry verification passed for the four Chromium cases (4 tests, 23.6s) and the mobile Quick Chat command-palette case (1 test, 23.9s). Both runs built the production Vite assets; focused ESLint, web typecheck, and `git diff --check` passed.
- The subsequent PR run `40459324215` on head `2afd7ad` completed with 85 checks passed, 12 skipped, and one retry-only Quick Chat selector-opening failure plus its dependent aggregate. The same Quick Chat scenario passed locally with retries disabled (1 test, 20.4s, including a managed Vite build). Rerunning only the failed jobs on the same SHA completed cleanly: 87 checks passed, 12 skipped, no failures or pending checks, and no review threads. Latest `main` was then merged without conflicts; the new head requires its own final CI snapshot.

- PR CI follow-up (2026-10-27): run `40463835440` on head `13a3a31` had three retry-only failures across E2E shards 2, 16, and 21. The exact failed-shard rerun cleared shards 2 and 21, but reproduced the file-tree tab-activation timeout on shard 16; that case passed on retry, so the E2E aggregate continued to reject the run.
- The Playwright trace confirmed the Files tab click was delivered while Dockview left the tab inactive. `SessionPage.clickTab` now retries its idempotent selection until the tab's active state is observed. The exact no-retry case passed five consecutive local repetitions; web typecheck and focused ESLint passed.
- This CI rerun does not validate the updated tab helper. The latest PR head requires a fresh CI run.
- Final PR check fixup (2026-10-27): full CI on head `4133ad0` had two retry-only E2E flakes (task route boot budget and mobile office role-tier save). The exact failed-shard rerun passed on the same SHA. Terminal PR evidence: 87 checks passed, 12 skipped, zero failed or pending, and no unresolved review threads; PR state was mergeable and clean.
- Follow-up CI on head `b2965a3` reported three retry-only E2E flakes (canvas setup, Kotlin LSP hover, and Threads Grid scrolling). Its failed-shard rerun cleared those cases but exposed a task archive/unarchive fixture race in the shared-sidebar mobile spec. Backend logs showed unarchive racing the still-running `cascade_archive` cleanup job.
- The shared-sidebar desktop and mobile fixtures now wait for `waitForCascadeArchiveCleanup` before unarchiving. Both specs passed locally with retries disabled; web typecheck and focused ESLint passed. The resulting PR head still requires a fresh full CI run.
