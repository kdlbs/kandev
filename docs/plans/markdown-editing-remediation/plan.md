---
created: 2026-10-20
status: completed
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
  - REQ-UI-MARKDOWN-FILE-EDITING-003
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
legacy_specs: []
---

# Implementation Plan: Markdown Editing Review Remediation

## Overview

Repair the Markdown editor from PR #3113, reviewed at
`b03dce19114c4a63b8a6716894906dbc7f351c95`. Preserve plain Markdown and the
existing workspace save API. Establish draft ownership before changing
presentation and recovery behavior. This package supplements the
[original package](../markdown-file-editing/plan.md).

The review found five defects. Mobile drafts disappear when their layout
unmounts. A same-text external update leaves a stale save baseline. Hybrid
comments omit repository names. Failed retained editors cannot initialize
again. The phone toolbar clips Save when Reload appears.

## Scope

### In scope

- One canonical draft across responsive layouts, guarded navigation, and
  correct save baselines.
- Repository-qualified comments and repeatable hybrid recovery.
- Capability-based Source fallback and reachable phone controls.
- Source-position transfer and tests through the pinned real engine.
- Accurate original delivery records and user documentation.

### Out of scope

- Replacing the engine, adding an EditContext polyfill, or promising hybrid
  editing on browsers without its required capability.
- Backend file APIs, collaborative editing, and disk persistence for drafts.
- Unrelated cancellation, routing, chart, and file-rename changes in the PR.
- Rewriting PR history, resolving every existing CI failure, pushing, or merging.

## Root causes and initial evidence

| Defect | Root cause | Required failing regression |
| --- | --- | --- |
| Draft loss | `useMobilePanelHandlers` owns state below the responsive branch | `retains and saves one draft across the 768px boundary` |
| Stale baseline | `useMobileFileBuffer` accepts a clean update only when text changes | `accepts a remote baseline equal to the local draft` |
| Comment identity | The bridge drops `repo` on lookup and creation. Workspace-prefixed file opens also omit the checkout qualifier. | `keeps same-path comments scoped to their repository` |
| Blank retry | Failure disposes an instance that remains mounted with an empty-dependency initialization effect | `retries Edit after initialization and update failures` |
| Hidden Save | Nonshrinking mode, Reload, and Save controls exceed one clipped row | `keeps Save and Reload reachable at 320px` |

The prior review ran 99 existing focused tests successfully. Two temporary
regression tests failed for baseline and comment identity. Real Chromium
component probes reproduced blank retry and toolbar clipping. Draft loss was
established by tracing responsive ownership. These are diagnosis results,
not implementation acceptance results.

## Technical approach

### Canonical draft lifetime

Task 01 gives the task layout a session-scoped draft owner above its responsive
branches. Mobile and desktop editing read and mutate the same record.
Presentation state and Dockview panel creation remain separate from source
ownership. Restore operations must not clear retained dirty records.
Save, fetch, and remote-update completions carry session, repository, path,
and instance ownership. The owner registers navigation and unload protection.
The system design defines cleanup and baseline reconciliation.

### Comment identity and recovery

Task 02 carries `repo` through `useMarkdownEditorCommentState`, matching
Preview and Source. File opens resolve repository identity from known checkout
roots without changing the initial workspace read request.
Task 03 derives engine capability at the adapter boundary.
Both coordinators unmount a failed instance before explicit retry. Healthy
instances stay mounted. Unsupported capability selects an effective Source
view without changing the saved preference.

| Surface and capability | Identity and transport | Behavior | Evidence |
| --- | --- | --- | --- |
| Desktop, usable EditContext | Session/repository/path, existing workspace save API | Preview, Edit, Source | Real Chromium engine flow |
| Phone, usable EditContext | Same canonical record and save API | Focused editor with visible Save | `mobile-chrome`, rotation and constrained keyboard viewport |
| Browser without EditContext | Same record, no engine allocation | Preview and Source, explained unavailable Edit | Capability-absence browser test, native browser smoke recorded separately |
| `.mdx` | Existing file identity and save API | Preview and Source only | Existing MDX regression |

### Presentation and position

Task 04 reserves phone header space for Save. Current mode and secondary
actions use responsive menu primitives. Task 05 transfers source-based
selection and reading anchors between real editor providers and Preview.
Hidden surfaces do not restore focus or scroll until activated.

### Evidence and documentation

Task 06 exercises the production adapter with the pinned engine and the
preservation corpus. Remove the unused source-edit helper instead of treating
its tests as engine evidence. Reconcile the original work orders only after
their exact checks pass. Update the workbench how-to with supported modes,
browser fallback, Save, Reload, and recovery.

## UI preview

This structured preview replaces ASCII drawings to follow the current Kandev
task's rich-output instruction. UI-01 and UI-02 are structural requirements.
Translated labels and existing component tokens determine final geometry.

| View | Current evidence | Proposed composition |
| --- | --- | --- |
| UI-01: Phone, dirty file with remote update, 320px | Preview, Edit, Source, and Reload fill one row. Save is clipped. | Fixed identity row with Back and dirty/remote cues. Fixed action row with current-mode trigger, actions trigger, and visible Save. Actions menu contains Reload. One scrolling editor body. |
| UI-01: Desktop | Inline mode controls and file actions | Retain the inline controls and normal desktop density. |
| UI-02: Hybrid failure | Source fallback, then empty Edit on retry | Source shows preserved draft and localized failure status. Explicit Edit retries with a fresh instance. |
| UI-02: Capability unavailable | Engine initialization fails | Edit is unavailable with an accessible explanation. Preview and Source remain usable. |

Phone controls have at least 44px hit areas, including narrow fine-pointer
viewports. Desktop fine-pointer controls retain the 28px convention.
Menus use existing phone bottom-sheet treatment and return focus to their
trigger. The fixed header clears safe areas. The editor owns vertical
scrolling. Mode changes have no new movement animation.

UI-01 maps to `AC-UI-MARKDOWN-FILE-EDITING-003.2`, `.3`, `.4`, `.5`, and `.10`.
UI-02 maps to `AC-UI-MARKDOWN-FILE-EDITING-001.10`, `002.8`, and `002.15`.

## Tests

| Contract | Targeted evidence |
| --- | --- |
| `002.3` through `.5`, `.14`, `003.8`, `.9` | Shared draft integration, mobile buffer, delayed save, workspace sync, and route cancellation tests in Task 01 |
| `002.6`, `.13` | Same-path repositories in Task 02 |
| `001.4`, `.10`, `002.8`, `.15` | Missing capability, initialization failure, update failure, retry, and single disposal in Task 03 |
| `003.2` through `.5`, `.10` | Measured phone controls and remote-update menu in Task 04 |
| `001.5`, `002.4` | Source offsets, direction, nearest line, hidden activation, and provider transfer in Task 05 |
| `001.3`, `.6`, `.8`, `.9`, `002.9` through `.12`, `003.6`, `.7` | Real-engine corpus and existing table interaction flows in Task 06 |

## E2E tests

Use `apps/web/e2e/tests/task/markdown-file-editing.spec.ts` on `chromium`
and `apps/web/e2e/tests/task/mobile-markdown-file-editing.spec.ts` on
`mobile-chrome`. Each work order adds named scenarios to these existing files
and runs its own filter. Task 06 runs the completed files serially.

Required scenarios include rotation in both directions, a save completing
after unmount, same-text external updates, duplicate repository paths,
failure followed by retry, unsupported capability, toolbar states at 320px
and 393px, and a long-document mode transfer.

## Work orders

- [x] [Task 01: Retain one draft across responsive layouts](task-01-shared-draft.md)
- [x] [Task 02: Preserve comment repository identity](task-02-comment-identity.md)
- [x] [Task 03: Recover failed and unsupported hybrid editors](task-03-hybrid-recovery.md)
- [x] [Task 04: Keep phone file actions reachable](task-04-phone-actions.md)
- [x] [Task 05: Transfer source positions between modes](task-05-position-handoff.md)
- [x] [Task 06: Prove real-engine behavior and reconcile records](task-06-engine-evidence.md)

Integrate the work orders sequentially because coordinators, phone code, and
E2E files overlap. Adapter, comment bridge, and desktop Markdown repairs can
start while Task 01 finishes draft ownership. Their worker must wait for an
explicit file-ownership handoff before changing the mobile panel or its tests.
The primary session owns all E2E edits and runs.
Source-position contracts and Monaco, CodeMirror, and Preview provider ports
can also proceed independently. Their worker must wait for the recovery and
phone-toolbar owners before changing the hybrid adapter or mode coordinators.
Each work order is a compact handoff. The user authorized native subagents,
but that authorization does not make overlapping edits parallel-safe.

## Verification results

Completed on 2026-10-20 on the existing PR branch
`feature/improve-markdown-pre-a36`, based on
`b03dce19114c4a63b8a6716894906dbc7f351c95`. Changes remain uncommitted.

- 311 targeted tests passed across 31 files.
- All 10 desktop Markdown browser scenarios were verified. Nine passed in the
  complete run. The repository-isolation scenario passed separately after the
  final Files panel qualifier fix.
- All seven phone Markdown browser scenarios passed. Both existing phone/tablet
  file-confirmation scenarios also passed.
- TypeScript, changed-file ESLint with zero warnings, translations, production
  builds, and whitespace checks passed.
- Real-engine LF and CRLF corpus tests saved exact bytes after edits, undo/redo,
  and mode changes. Desktop and phone table interactions passed.
- English and Portuguese 320px phone screenshots were visually inspected.
- Document catalog and specification lint passed. Public documentation checks
  passed with 63 tests and 47 published pages. Reference coverage passed for
  all nine changed work orders.

[Task 06](task-06-engine-evidence.md#results) records the environment adjustments,
run composition, and evidence limits. Browser suites ran sequentially with one
worker. Native Firefox and Safari were unavailable and remain unverified.
Design validation previously passed the document catalog, specification lint,
36 linter tests, reference coverage, relative links, and whitespace checks.
Those design results remain separate from implementation acceptance.

No commit, push, merge, or GitHub comment was requested or performed.

## Risks

- The PR conflicts with current main. This package targets its reviewed head.
  After integration, rerun affected work-order checks against the actual result.
- Shared draft adoption must preserve existing editor instance and session
  guards. A copied cache with two writers would retain the original defect.
- Engine retry resets failed-instance history. Canonical source must survive.
- A simulated missing capability does not prove native IME or touch behavior.
- The original package records historical checks. Do not present those counts
  as current-head results or mark pending table work complete from code alone.

## Evaluation delivery

On 2026-10-21 the user authorized committing and pushing this package on the
existing PR branch. The user also requested an isolated Tailscale evaluation
instance and a [manual testing guide](manual-testing.md). The earlier delivery
statements describe the implementation checkpoint before that authorization.

## Browser compatibility correction (2026-10-21)

The completed results above describe the earlier capability-limited package.
The user requires rendered Edit across browsers. Source fallback does not meet
that requirement. The [browser parity package](../markdown-browser-parity/plan.md)
supersedes the browser exclusion and capability-only acceptance. Earlier test
counts remain historical Chromium evidence, not cross-browser acceptance.

## PR fixup (2026-10-23)

The user authorized CI and review remediation on the existing PR branch.
The integration includes the browser input, walkthrough, source-position, and
button-state packages above. It merges main at
`d3fab6f5e564dc1d0f09391d2d4e41c5c54b7620`.

The review audit repaired tablet mode defaults and persistence, stabilized
empty adapter props, corrected test mocks, and repaired a design reference.
CI remediation updates phone tests for the mode menu and preserves typed
workspace recovery guidance after a failed manual resume. The recovery test
explicitly expects Source after its injected editor initialization failure.

Current local checks passed: 188 targeted tests in 20 changed files, frontend
typecheck, translation checks, documentation catalog and specification lint,
and 204 harness tests. The four original phone CI scenarios passed against
the rebuilt app. All 15 desktop Markdown cases and all nine phone Markdown
cases passed across full runs and focused reruns. The multi-repository test
no longer collapses an already expanded root. Four configuration-chat DOM
snapshots were updated for the approved shared button classes. Earlier counts
in this plan remain historical evidence.

The user-requested `TESTING-PR3113.md` remains outside the commit. Native device
IME and assistive-technology checks remain pending in the browser parity plan.

The first new CI run exposed textual workflow-contract assertions for Node
version quoting and the report dependency list. The workflow retains main's
format for those fields while keeping all three native browser jobs required.
All 27 E2E workflow-contract tests, five external-runner contract tests, eight
blob-download tests, three Markdown browser-coverage tests, and action-pin
validation pass locally. The timeout test used the host's Nix `sleep` path in
memory because this machine has no `/bin/sleep`; its timeout assertion remains
unchanged.

The same CI run exposed Firefox's root/home ownership mismatch and two test
fixture problems. The native-browser job aligns its home directory owner
with its container user before browser launch. Repository navigation uses
single clicks, checks document identity, and restores collapsed roots after
walkthrough reloads. The PR action test seeds merge permission explicitly.
The workflow-change test waits for its successful move response before
checking dialog closure.

The repository navigation failure reproduced twice in WebKit. After repair,
it passed three consecutive WebKit runs. The three affected Chromium cases
passed all nine executions across three fresh-fixture repetitions. All runs
used one worker and disabled retries. Browser coverage contract tests and
changed-file formatting/lint also pass. Application source is unchanged in
this follow-up, so the published UI captures still describe its rendered UI.

The next CI run isolated two native-browser test failures. The Firefox corpus
check used an absolute page click while mode restoration could move the editor.
It now clicks the trailing half of the final glyph relative to its paragraph
and verifies the native caret before insertion. The exact saved-source checks
remain unchanged.

WebKit reproduced a missing newline before editor recovery began. Per-command
model checks and a captured visible suggestion widget showed that Enter accepted
a Monaco completion instead of inserting a newline. The source typing helper
now dismisses suggestions with Escape, verifies dismissal, and checks the exact
model after each command. Five fresh WebKit recovery runs passed with retries
disabled. No application source changed in this follow-up.
The final test repairs passed all 15 Firefox and all 15 WebKit Markdown cases,
with one worker and retries disabled. Changed-file ESLint and formatting passed.
