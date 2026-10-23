---
id: "03-composition-and-mobile"
title: "Preserve composition and mobile input"
status: in_progress
wave: 3
depends_on: ["02-native-browser-input"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
  - REQ-UI-MARKDOWN-FILE-EDITING-003
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-001.5
  - AC-UI-MARKDOWN-FILE-EDITING-001.11
  - AC-UI-MARKDOWN-FILE-EDITING-001.12
  - AC-UI-MARKDOWN-FILE-EDITING-002.4
  - AC-UI-MARKDOWN-FILE-EDITING-002.5
  - AC-UI-MARKDOWN-FILE-EDITING-002.8
  - AC-UI-MARKDOWN-FILE-EDITING-002.15
  - AC-UI-MARKDOWN-FILE-EDITING-003.2
  - AC-UI-MARKDOWN-FILE-EDITING-003.3
  - AC-UI-MARKDOWN-FILE-EDITING-003.4
  - AC-UI-MARKDOWN-FILE-EDITING-003.5
  - AC-UI-MARKDOWN-FILE-EDITING-003.7
  - AC-UI-MARKDOWN-FILE-EDITING-003.8
  - AC-UI-MARKDOWN-FILE-EDITING-003.10
  - AC-UI-MARKDOWN-FILE-EDITING-003.11
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 03: Preserve composition and mobile input

## Summary

Make native composition, caret positioning, accessible input, and mobile
keyboard interaction follow the same canonical editing contract. Correct
platform failures through targeted regressions before closing browser parity.

## In scope

- Explicit composition state and settlement before suspension, mode changes,
  remote replacement, and disposal. Keep the same input node while composing.
- Pointer/keyboard selection synchronization, repeated composition, cancellation,
  surrogate pairs, autocorrect replacement, and clipboard/history ownership.
- Caret-to-client positioning after scrolling, table actions, and layout changes.
- Named accessible native input, decorative-subtree exclusion, focus return,
  Find/comment controls, read-only transitions, and navigation protection.
- `markdown-mobile-webkit` coverage and a reproducible native-input checklist
  in `manual-testing.md` for Zen/macOS, Safari/macOS, Android, iOS, NVDA/Firefox,
  and VoiceOver/Safari. Record exact browser/OS/input method and result.

## Out of scope

- Detecting assistive technology by browser name, replacing the phone surface,
  or treating synthetic composition events as OS IME evidence.

## Acceptance

- Composition after selection, sequential composition, mode/blur transitions,
  and outside updates preserve the selected range and exact committed text.
  No composition or clipboard action applies twice or loses the newest draft.
- Phone native input follows the caret, preserves selection and responsive
  drafts, and leaves Save and table/comment controls operable with one scroll
  owner. UI-02 passes at 320px and normal phone widths.
- Screen-reader checks prove current-text reading, selection, editing, and
  focus return. Missing device or accessibility access stays an open result
  with a precise reproduction guide. It cannot become a passing claim.

## UI preview

UI-02: Files opens a focused full-height phone editor. The fixed header retains
Back, file identity, current-mode menu, actions, and Save. One body scrolls.
The virtual keyboard follows the active paragraph or table cell. Targets are
at least 44px and clear safe areas. UI-01 keeps existing desktop density.
Comment and table controls suspend editor input temporarily and return focus
to its selected source range. Runtime failure retains Source recovery.
See [combined preview](plan.md#ui-preview). Criteria: `001.11`, `.12`, `003.2`
through `.5`, `.7`, `.8`, `.10`, `.11`.

## Verification

Commands start at the repository root. Run projects sequentially.

```bash
pnpm --dir apps/web exec vitest run components/editors/markdown/input/markdown-input-context.test.ts components/editors/markdown/input/markdown-textarea-input.test.tsx components/editors/markdown/hybrid-markdown-comments.test.tsx
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web e2e:run --host --shards 1 --project markdown-firefox tests/task/markdown-file-editing.spec.ts -- --grep "native input:"
pnpm --dir apps/web e2e:run --host --shards 1 --project markdown-webkit tests/task/markdown-file-editing.spec.ts -- --grep "native input:"
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-markdown-file-editing.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project markdown-mobile-webkit tests/task/mobile-markdown-file-editing.spec.ts
git diff --check
```

Perform the manual checklist separately. Playwright WebKit does not prove iOS
keyboard/candidate UI. Firefox phone viewport checks do not prove Android
Firefox native input. Do not claim those results from responsive screenshots.

## Files likely touched

- `apps/web/components/editors/markdown/input/` and focused tests.
- `apps/web/components/editors/markdown/hybrid-markdown-editor.tsx`, styles,
  comments, source-position bridge, and their tests.
- `apps/patches/@vscode__markdown-editor@0.0.2-84.patch` if suspension needs correction.
- Existing desktop/mobile Markdown E2E files and `e2e/playwright.config.ts`.
- `apps/patches/monaco-editor@0.55.1.patch`, its pnpm registration/lock, and
  actual Monaco textarea/state regression tests for backward Source input.
- `docs/plans/markdown-browser-parity/manual-testing.md` (new).

## Dependencies

Task 02. Keep this work sequential because it changes the same transport.

## Risks

Settling composition is an asynchronous input boundary. Do not guess final
composition text from a blur event or rewrite native input mid-composition.
Excluding the host from accessibility can also hide its shadow input.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/markdown-file-editing.md).
- [Browser transport design](../../specs/ui/system-design/markdown-file-editing.md#browser-input-transport).
- [Responsive contract](../../specs/ui/system-design/markdown-file-editing.md#responsive-interaction-contract).
- Existing mobile file viewer and comment popover focus conventions.

## Results

Implementation and automated checks are complete. The status remains in
progress for native device, OS input, and assistive-technology acceptance.
The scoped input keeps one native node through composition, defers external
replacement and read-only changes until settlement, and maps native selection
to canonical source without a text/history event. Unit coverage includes two
composition sessions, exact mixed endings, clipboard/history ownership,
read-only/suspended selection, cleanup, and caret geometry.

The final integrated focused run passes 153 tests in 26 files. A controlled
animation-frame regression also exposed and corrected a rapid mode-switch
race: a capture before deferred reveal now retains the requested reading
position. User scrolling and cleanup cancel the pending restoration.
Native Firefox also exposed an upstream backward-selection defect in pristine
Monaco 0.55.1. Its native wrapper collapses reversed bounds, and its input-delta
calculation misclassifies the first replacement as composition. Actual runtime
regressions prove both failures. The maintained package patch keeps native
range direction and normalizes delta calculation endpoints. Exact browser
replacement confirms the corrected first character and saved bytes. Source
restoration also uses immediate scrolling: a new regression proves that a
rapid handoff after editing captures the restored region instead of the old
smooth-scroll viewport. Native Firefox confirms the full handoff, first key,
exact saved text, and return-to-Edit reading position. All 53 distinct desktop
and phone browser scenarios pass on the final artifact.

Native OS IME, Android/iOS keyboard/selection handles, NVDA/Firefox, and
VoiceOver/Safari checks remain pending. The linked native checklist records
these acceptance gaps separately from actual Linux engine automation.
