---
created: 2026-11-03
status: implemented
requirements:
  - REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001
  - REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002
system_design:
  - ../../specs/tasks/system-design/clarification-response-reliability.md
legacy_specs: []
---

# Implementation plan: Clarification answer drafts

## Overview

Preserve unfinished question answers when users inspect other tasks and return.
One sequential work order delivers the cache, form integration, regression
coverage, desktop/phone navigation proof, and a small public documentation update.

Source: [issue #5013](https://github.com/kdlbs/kandev/issues/5013), assigned to
`carlosflorencio` on 2026-11-03. The issue is open and has no attachments,
comments, or cross-referenced implementation PR at investigation time.

Authoritative contracts are the
[requirements](../../specs/tasks/requirements/clarification-response-reliability.md),
[response design](../../specs/tasks/system-design/clarification-response-reliability.md#unsubmitted-drafts-across-navigation),
and [ownership decision](../../decisions/2026-11-03-clarification-answer-draft-ownership.md).
The [question-image design](../../specs/tasks/system-design/question-image-previews.md#drafts-across-responsive-layouts)
now refers to that ownership contract.

## Evidence and root cause

Investigation baseline: `f5c03760265415cc4216c01ab9e7449e1633c03e`.
The reporter used backend `v0.98.0`, commit
`2d93502c256780e1f3d9df4348dafbd97b584188`, and reproduced with Claude and Codex.
Local evidence applies to the investigation baseline, not the reporter's database.

`TaskLayout` mounts `ClarificationDraftProvider` around its responsive layout.
The provider owns a React-state map and replaces it whenever `taskId` changes.
Unmounting the provider discards the map entirely. `useClarificationGroup`
also keeps choices in component state. The overlay restores those choices
only through `useClarificationDraft`'s surviving snapshot.

An existing test, `clears task-owned drafts without remounting the layout`,
explicitly expects the reported loss. The former question-image design also
required a task change to replace the draft cache. Its scope covered responsive
remounts rather than navigation. No active acceptance criterion required
navigation retention. New requirement `REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002`
defines that missing outcome and supersedes the old cache-lifetime wording.

Two temporary component reproductions exercised the real provider and hook:

1. Enter custom text, retain a selected option, and set question index 1.
   Render task A, task B, then task A under the same provider.
2. Enter the same state, unmount the task page, then mount task A again.

Both expected the original text, index, and answer array. Both received
`[{},0,null]`. The existing six draft-context tests passed in the same run.
No production changes preceded the reproduction. The temporary file was removed.

Reproduction command from `apps/web`:

```bash
pnpm exec vitest run components/task/chat/issue-5013-repro.test.tsx components/task/chat/clarification-draft-context.test.tsx
```

That temporary path no longer exists. Task 01 recreates the proof as permanent
tests named `retains unsubmitted answers across task navigation` and
`retains unsubmitted answers after task-page unmount`.

## Scope

### In scope

- Retain selected options, exact unfinished text, and question position for
  active pending forms across client task/session navigation and responsive remounts.
- Keep exact source identity, store isolation, auth invalidation, bounded memory,
  current clarification authority, and existing submission behavior.
- Prove desktop and phone outcomes through their shipped task navigation controls.

### Out of scope

- Backend, MCP, database, provider, or response endpoint changes.
- Reload persistence, cross-tab synchronization, durable drafts, and retention
  beyond eight recently used pending bundles.
- New retention behavior for Quick Chat, Inbox, or historical late-answer forms.
- Changes to panel geometry, task selection policy, labels, or translation keys.

## Assumption check

The confirmed intent is the issue's expected behavior: retain unsubmitted
custom text and selected options across task switching. The local reproduction
verifies both state-loss boundaries. The shared overlay establishes provider-independent
ownership. No provider-specific failure is inferred from the reporter's task IDs.

Browser-memory retention follows the existing exclusion of reload persistence.
The design keeps the existing eight-entry bound and makes eviction explicit
across retained tasks. These are package design choices, not claims about
preferences stated by the reporter. No material question blocks this repair.

## Technical approach

Add `apps/web/lib/clarification/answer-draft-cache.ts` as the dependency-neutral
cache owner. Install its authority observer once from `createAppStore` in
`apps/web/lib/state/store.ts`. Use a weak owner reference, exact source keys,
cache generations, and an eight-entry recency bound.

Adapt `ClarificationDraftProvider` and `useClarificationDraft` in
`clarification-draft-context.tsx` to use retained answer state. Keep resize
state local and preserve mounted workbench identity. Restore choices through
the existing overlay and `useClarificationGroup.recordAnswer`.

Avoid empty-mount overwrites, cross-bundle writes, and stale completion writes.
Validate cached answers against current source questions before restoration.
Retire captured entries after definitive settlement. Missing hydration data
does not prove settlement. Current-turn selection and response authority remain
outside the cache.

The shared task form covers Claude and Codex questions after normal
clarification normalization. Native Codex choice-only questions retain
`allow_custom_text: false`. Images remain metadata references. This package
does not add a provider adapter or claim support for another host.

## ASCII UI preview

The form composition stays unchanged. These previews show restored state,
not a new layout. Copy and options are illustrative agent data.

`UI-01: Desktop pending answers`, entered from the task sidebar.
For `AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-002.1`, `.3`, and `.7`:

```text
Before: Task A -> Task B -> Task A       After: Task A -> Task B -> Task A
+--------------+--------------------+   +--------------+--------------------+
| Task A       | Question 1 of 3    |   | Task A       | Question 2 of 3    |
| Task B       | No saved selection |   | Task B       | Q1: PostgreSQL     |
|              | Custom: [         ]|   |              | Custom: [Use Go   ]|
|              | Back / Next       |   |              | Back / Next       |
+--------------+--------------------+   +--------------+--------------------+
```

`UI-02: Phone pending answers`, entered from the task-title picker.
For the same criteria, verified under `mobile-chrome`:

```text
+----------------------------+
| Task A [task picker]        |
| Conversation               |
|----------------------------|
| Question 2 of 3            |
| Custom: [Use Go           ]|
| [Back] [Next]              |
| Existing composer/actions  |
+----------------------------+
```

The phone picker remains an inset drawer. The question panel retains one
internal scroller and existing safe-area and touch behavior. Navigation to
task B and back restores task A's draft without submitting. Q1's selection
remains visible when the user returns to that question. Existing final Submit
remains the primary completion action. Spacing is not a pixel specification.

## Tests

- `answer-draft-cache.test.ts`: store and auth isolation, authority changes
  with no form mounted, same-user return, generation fencing, settlement,
  and eight-entry recency eviction. Covers `.2`, `.4`, `.5`, and `.6` of REQ-002.
- `clarification-draft-context.test.tsx`: both permanent navigation reproductions,
  mixed task/session/bundle identities, provider unmount, responsive remount,
  and unchanged resize behavior. Covers `.1`, `.2`, and `.7` of REQ-002.
- `clarification-input-overlay.test.tsx`: restore real keyed selections and
  unfinished custom text, replace a bundle without remounting, reject invalid
  question/option identities, and prevent empty initial writes. Covers
  `.1` through `.4` of REQ-002.
- Existing custom-input, panel-section, and clarification-group suites preserve
  explicit submission, retry, choice-only, late-answer, and stale-result behavior.
  Covers REQ-001 `.4` and REQ-002 `.3`, `.4`, and `.7`.

## E2E tests

Add a describe group named `question draft navigation` to existing desktop
`clarification.spec.ts` and phone `mobile-clarification.spec.ts`.

- Seed two `clarification-multi` tasks through the existing API helpers before
  entering a draft. Choose Q1, enter raw Q2 text, switch A -> B -> A, and
  verify text, selection, and position. Use sidebar rows on desktop and
  `mobile-task-picker-trigger` on phone. Covers REQ-002 `.1` and `.2`.
- Leave the task route through client navigation and return through browser
  history. Verify that the draft survives a provider remount. Covers `.1` and `.5`.
- Finish and submit the restored bundle. Observe no response request during
  navigation and one response at submission. Verify its persisted answer data
  and retirement after another visit. Covers `.3` and `.4`.
- Preserve the same draft through desktop -> phone -> desktop remounts.
  Keep the existing mobile composition and no horizontal overflow. Covers `.7`.

Do not use `page.goto` or `reload` between entering and inspecting the draft.
The existing seed helper uses `page.goto`, so finish seeding before draft entry.
Use causal route/readiness waits and a live request observer. Apply the existing
`dwell` negative-assertion helper only where no positive event proves absence.

## Work orders

- [x] [Task 01: Retain clarification answer drafts](task-01-retain-answer-drafts.md)

The work order is sequential and has no dependency. It contains exact test,
lint, type, localization, documentation, E2E, and package preflight commands.

## Documentation impact

This design turn updates internal contracts only. Task 01 adds a concise
how-to note to `docs/public/tasks-and-workflows.md#answer-clarification-questions`
after the behavior works. The note explains navigation retention, its eight-bundle
limit, and reload/logout clearing. It does not expose cache implementation details.

## Verification results

Implementation turn, 2026-11-03:

- `apps/web/lib/clarification/answer-draft-cache.ts` owns the store-keyed
  eight-entry cache and its single authority observer, installed by
  `createAppStore`.
- `ClarificationDraftProvider` keeps panel sizing task-local and projects the
  retained answer state into `useClarificationDraft`. Restoration validates
  question and option identities, clamps the question index, ignores the
  previous bundle's live answer map, and fences writes behind the captured
  generation.
- Work-order commands passed: 161 focused Vitest tests across the eight listed
  files, `pnpm run typecheck` (with an 8 GB Node heap for the repository
  toolchain), ESLint with zero warnings, `i18n:check`, and `i18n:ratchet`.
- Full Chromium `clarification.spec.ts`: 30 passed. Full Mobile Chrome
  `mobile-clarification.spec.ts`: 16 passed. Chromium
  `clarification-resize.spec.ts`: 4 passed. The draft scenarios used real
  client navigation and observed zero response requests during navigation.
- Public documentation validation, specification validators, the work-order
  coverage preflight, `git diff --check`, and the final status review passed.
- Review remediation preserved the raw custom draft during restoration and
  restored fresh-edit retention after an authority generation change. Focused
  suites then reported 166 tests. Details are in the work order.

Design turn, 2026-11-03:

- Temporary reproduction: two expected state-loss failures. All six existing
  draft-context tests passed. The temporary test file was removed afterward.
- `python3 scripts/list-docs.py validate`: passed, 426 decisions and
  1,891 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Task 01's `validateCoverage` preflight: `covered`, one work order, no errors.
  The input includes its planned source path to exercise delivery coverage.
- `git diff --check -- docs/plans/clarification-answer-drafts docs/specs docs/decisions`:
  passed. Git status contains the six intended documentation files only.
- Catalog discovery includes the amended requirement/design and the new ADR.
- GitHub readback confirms that issue #5013 remains open and is assigned to
  `carlosflorencio`.

Task 01 implemented the behavior and permanent tests described above in the
same working tree. No commit, push, PR, or additional agent was created.

## Risks

- Initialization effects can overwrite saved answers with an empty answer map.
  The overlay integration regression must prove the real restored form.
- Logout while the task page is absent can expose an old draft on later login
  unless the application owner observes auth transitions independently.
- A stale submit completion can clear another task's draft without captured
  key and generation checks. Keep those boundaries in the regression matrix.
- Retaining answer and resize state together can remount workbenches or change
  sizing behavior. Preserve the existing resize and panel-section tests.
- The cache intentionally loses its oldest draft after a ninth edited bundle.
  Reload persistence remains outside this repair.
