---
created: 2026-10-21
status: in_progress
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
  - REQ-UI-MARKDOWN-FILE-EDITING-003
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
legacy_specs: []
---

# Implementation Plan: Markdown browser parity

## Overview

Make rendered Markdown Edit work across current Chromium, Firefox/Zen, and
Safari browsers. Preserve the existing source-preserving engine, rendered
table editing, comments, history, and canonical draft ownership. Add a scoped
native-input transport instead of treating Source fallback as browser support.

This package follows PR #3113 at `f5cfc7e228390f3e2065af78641a260fd0009464`.
Use its existing `feature/improve-markdown-pre-a36` branch. The previous
[repair package](../markdown-editing-remediation/plan.md) remains historical
evidence for its completed scope. Its browser exclusion is superseded here.

## Evidence and assumption check

- Confirmed: the user requires rendered Edit across browsers and uses Zen.
  Keep `TESTING-PR3113.md` uncommitted in this workspace.
- Verified: Zen reports Firefox 158, no EditContext, and an HTTP origin.
  Chromium loads Edit at the same URL. The pinned package constructs
  EditContext unconditionally. Its view has a bounded input seam.
- Verified: a global polyfill adds selection/focus changes that overlap the
  engine and documents composition gaps. A CodeMirror replacement would also
  require rebuilding rendered-cell editing and exact-byte history.
- Selected engineering approach: scoped input transport, as recorded in the
  [decision](../../decisions/2026-10-21-scoped-markdown-browser-input.md).
  Its implementation evidence and remaining native checks are recorded below.

The UI system owns this reusable editor contract. Existing task/workspace
systems continue to own draft lifetime and file persistence.

## Scope

### In scope

- Reproducible input seam in the exact pinned engine, without parser changes.
- Native textarea input, composition, selection, clipboard, history, focus,
  caret geometry, and accessible editing on the same rendered surface.
- Actual Firefox/WebKit projects and focused CI coverage, plus recorded native
  device and assistive-technology checks.
- Browser-neutral mode controls, public instructions, and evaluation guidance.

### Out of scope

- A global API polyfill, a new document model, or replacing the Markdown engine.
- Hybrid MDX, collaboration, backend file APIs, or persisting table widths.
- Legacy browser versions, resolving the PR's unrelated merge conflict, merge,
  branch creation, or automatic commit/push.
- Resetting the user's fixture edits, modifying the main instance at :9998,
  or changing Tailscale Serve.

## Technical approach

Task 01 adds `EditorViewOptions` input injection via
`apps/patches/@vscode__markdown-editor@0.0.2-84.patch`, registered through
`pnpm.patchedDependencies` in `apps/package.json` and the lockfile. Runtime and
declarations change together. Native behavior must pass existing Chromium
regressions before adding another transport.

Task 02 implements `components/editors/markdown/input/` and selects the
transport in `HybridMarkdownEditor`. Native textarea events use a shadow root
to preserve existing root-target controller checks. The context holds canonical
source. An explicit projection maps normalized native offsets to exact source
ranges. Native selection has an explicit revision-guarded callback into
`model.selection`, including backward/read-only selection. It never emits a
text edit or history entry. Engine history remains authoritative. Mode capability helpers no
longer disable Edit when EditContext is absent. Add focused `markdown-firefox`
and `markdown-webkit` Playwright projects using the existing fixtures.

Task 03 completes composition settlement, keyboard/clipboard ownership,
accessible input, caret geometry, and mobile focus behavior. Match existing
table edge controls and comment popovers. Add `markdown-mobile-webkit` and
native device/assistive-technology evidence. Synthetic composition tests prove
translation contracts only. They do not prove OS IME behavior.

Task 04 adds a focused browser CI job and updates public how-to instructions
only with verified behavior. Keep the workspace guide uncommitted. Refresh the
isolated evaluation runtime only after the checks pass, preserving its current
data and recording the tested SHA. Do not present the current runtime as fixed.

| Input path | Identity and transport | Intended behavior | Required evidence |
| --- | --- | --- | --- |
| Constructible native EditContext | Existing session/repository/path draft; native context | Existing rendered Edit | Chromium regression and native input checks |
| No usable native EditContext | Same draft; scoped textarea and source-offset projection | Full rendered Edit, including active table cells | Actual Firefox and WebKit, native Zen/Safari checks |
| Phone | Same draft and adapter, focused mobile Files surface | Visible Save and native keyboard at active text | Mobile Chromium/WebKit plus real Android/iOS keyboards |
| Genuine transport/engine failure | Same canonical draft; existing Source recovery | Explicit retry with fresh owned instance | Failure/retry and stale-callback tests |

## UI preview

The task's rich-output instruction replaces ASCII drawings. UI-01 and UI-02
are structural requirements. Existing component tokens determine final style.

| View and entry point | Baseline at design checkpoint | Required composition |
| --- | --- | --- |
| UI-01: Desktop Files, Firefox/Zen or Safari | Edit unavailable; Preview/Source | File identity, Preview/Edit/Source, existing actions including Save; rendered document with active source range and editable bordered table cells |
| UI-02: Phone Files, Edit selected | Same capability restriction | Fixed Back/file identity, current-mode menu, actions menu, visible Save; one editor body and native keyboard at the active text |
| UI-01/UI-02: Runtime failure | Draft preserved in Source with retry | Retain that recovery, without classifying missing EditContext as failure |

Reuse `MobileFileViewerPanel` and the mobile UI language's persistent-toolbar
pattern. Phone Files is primary dense content, so retain its focused full-height
surface rather than adding a drawer. Header controls remain fixed, with 44px
phone/coarse-pointer targets and safe-area clearance. Desktop keeps its 28px
controls. One body owns vertical scrolling. Tables scroll horizontally locally.
Mode and actions use existing responsive menus. No browser picker, additional
toolbar, input overlay over cells, or new motion is introduced. Native input
plumbing must not change the visible hierarchy or create another scroll owner.

UI-01 maps to `001.2`, `001.10` through `.12`, `002.9` through `.12` and
`003.5`. UI-02 also maps to `003.2` through `.4`, `.7`, `.8`, `.10`, `.11`.
These suffixes refer to the full acceptance IDs in the linked requirements.

## Tests

| Criteria | Named evidence |
| --- | --- |
| `001.3`, `.5`, `.6`, `.11`, `002.4`, `.5`, `.14` | `markdown-input-context.test.ts`: exact source projection, CRLF/LF/mixed endings/BOM, surrogate pairs, stale updates and exact undo |
| `001.10`, `002.8`, `.15` | `hybrid-markdown-editor.test.tsx`, coordinator tests: no EditContext keeps Edit, failure/retry still preserves draft |
| `001.11`, `.12`, `003.3`, `.5` | `markdown-textarea-input.test.tsx`: native command ownership, composition state, focus, suspension and accessible control |
| `002.6`, `.9` through `.13` | Existing table and comment tests through production adapter, now using native textarea input |
| `001.9`, `003.8`, `.9`, `.10` | Retain syntax, responsive draft, navigation, and narrow-header regressions |

## Native acceptance guide

[Native browser, device, and screen-reader checklist](manual-testing.md).
These results remain separate from automated Linux engine evidence.

## E2E tests

Extend `apps/web/e2e/tests/task/markdown-file-editing.spec.ts` and
`mobile-markdown-file-editing.spec.ts`. Keep existing guarded fixtures and one
worker. New projects select only Markdown tests, not every suite. Selectors must
target rendered content and active cells, never silently type into Source.

Required `browser parity:` scenarios cover paragraph and rendered-cell edits,
pointer/keyboard selection, native selection without keyboard events,
Enter/Tab, clipboard, undo/redo, comments, mode
handoff, save/reload, mixed-ending preservation, two-editor isolation, and
initialization failure/retry. Include missing native API on Chromium as a
transport-selection test alongside actual Firefox/WebKit runs.

Required `native input:` scenarios cover composition after selection, sequential
composition, blur/mode transitions, comment and table-control focus, keyboard
clearance, safe-area/scroll containment, and responsive draft retention. Record
whether composition is synthetic or uses a real OS input method.

## Work orders

- [x] [Task 01: Isolate the engine input seam](task-01-input-seam.md).
- [x] [Task 02: Enable scoped browser input](task-02-native-browser-input.md).
- [ ] [Task 03: Preserve composition and mobile input](task-03-composition-and-mobile.md). Implementation and automated checks complete; native acceptance remains open.
- [x] [Task 04: Enforce and document browser coverage](task-04-browser-coverage.md).

The order is sequential. These work orders do not authorize delegation.
The user previously authorized GPT-6.1 Sol subagents; the primary session owns
integration, durable artifacts, and communication.

## Verification results

Implementation is on the existing branch and remains uncommitted. Native
textarea input enables rendered Edit without EditContext. Maintained pnpm
patches cover the hybrid transport seam and Monaco 0.55.1 backward native
selection/input deltas. Cross-mode restoration retains pending hybrid intent
and uses immediate Source scrolling.

The final focused run passes 153 tests in 26 files, including the actual
patched package runtimes and CI contracts. Typecheck, changed-file ESLint,
i18n, formatting, frozen installation, and pristine patch application pass.
Native desktop Firefox and WebKit each pass 13 scenarios. Chromium has all
13 distinct scenarios passing (11 in the full run plus two public-selection
assertion corrections in a focused rerun). Phone Chromium and phone WebKit
each pass all seven scenarios. This covers 53 distinct browser scenarios.

CI is configured to install and execute the three focused Firefox/WebKit projects
with one worker and includes them in the report and required gate. Both
workflow contract suites pass; the Nix host needs a temporary fixture adapter
for an unchanged hardcoded `/bin/sleep` test path.

At browser-parity delivery, the isolated evaluation runtime served asset fingerprint
`af406b0332755f284e75c0953116faa890e4334d9691f99908e0f60f57146984`, based on
`f5cfc7e228390f3e2065af78641a260fd0009464` plus uncommitted corrections. The
refresh preserved SQLite, all 66 fixture files, and fixture Git states. Four
read-only live checks passed: desktop Chromium, Firefox, WebKit, and phone
WebKit. Firefox/WebKit opened Edit at the direct HTTP origin without EditContext.
The main :9998 listener remained PID 1155. Its stop helper ownership check passes.
`TESTING-PR3113.md` remains untracked and contains the link and shutdown command.

Native Zen/macOS, Safari/macOS, Android/iOS keyboard/selection, OS IME, NVDA,
and VoiceOver acceptance remains open in the linked manual checklist. The
implementation and automated checks are complete; the plan stays in progress
for this native acceptance. Linux engines and synthetic composition do not
close those results.

The later [Markdown walkthrough package](../markdown-walkthrough-ranges/plan.md)
retains this browser input work and records the current evaluation build. Its
additional browser checks do not close the native acceptance items above.

## Risks

- Retain per-engine regression coverage on dependency upgrades. Native
  shadow retargeting, clipboard defaults, and focus can differ between engines.
- Native textareas normalize line endings. Missing offset mapping can corrupt
  source selections even when simple typing appears correct.
- IME, autocorrect, virtual keyboard, and screen-reader behavior need native
  checks. Missing access remains an explicit acceptance gap.
- The experimental dependency may change its input contract. Keep the patch
  narrow and upgrade only with the full contract matrix.
