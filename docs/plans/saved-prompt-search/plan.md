---
created: 2026-10-28
status: implemented
requirements:
  - REQ-UI-SAVED-PROMPT-SEARCH-001
system_design:
  - ../../specs/ui/system-design/saved-prompt-search.md
legacy_specs: []
---

# Implementation Plan: Saved Prompt Search

## Overview

Add a local filter to Settings > Prompts and verify the desktop and phone
interaction as one sequential vertical slice. The work order starts with a
mounted failing consumer test, then adds the presentation and focused browser
coverage. Implementation awaits a later explicit execution and exclusive heavy
release after ROOT reads all four artifacts.

## Scope

### In scope

- Search saved names and raw content of the existing loaded built-in and custom list.
- Protect existing settings forms and mutation actions; preserve loading and empty views.
- Add desktop and mobile focused E2E and one concise Saved prompts guide instruction.

### Out of scope

- Data ownership, mutation/action migrations, remote search, persistence,
  regex, reference expansion, counts, new settings controls, new translations,
  global keyboard/CSS changes and asynchronous lifecycle hardening.

## Technical approach

`apps/web/components/settings/prompts-settings.tsx` owns the local query and
projects `prompts` into matching rows without sorting or mutating data. Preserve
`PromptListContent`'s loading and actual-empty precedence, adding a separate
no-match branch. Disable input for `editingId`, `showCreate`, `deleteTarget` and
`isBusy`; use saved content for matching, not form drafts. Keep
`getPromptDraftMeta`, mutation hooks/actions, `SettingsPageTemplate` and
`PromptAgentPermission` contracts intact. The settings `Input` is already
imported. Extract only a small presentation component and its test if necessary
for local modularity limits, not a generic abstraction. See
[ADR 0046](../../decisions/0046-settings-route-save-coordinator.md) and
[saved prompt delivery](../../specs/tasks/requirements/saved-prompt-delivery.md).

Phone uses the existing direct settings route and scrolling owner, inline search
above rows and existing explicit row actions/drawer. Native entry is the primary
interaction; no fixed region, extra overlay, or new safe-area owner is needed.
Phone or coarse input is at least 44px high, contained, and preserves the
coarse input anti-zoom rule; fine desktop keeps 28px density. The current mobile
prompt settings is the nearby route precedent; mobile UI language's direct
navigation and contained touch controls guide the composition.

## ASCII UI preview

UI-01: Settings > Prompts, loaded list, desktop and phone share this order
(AC .1-.3, .7). The settings content scrolls; nothing shown is fixed. Labels
and spacing are illustrative, while order, native input and existing actions are
structural requirements.

```text
Custom prompts                            [+ Add]
[ Search...                               ]
[ Review                        existing actions ]
[ Deploy                        existing actions ]
```

On phone the same region is a single contained column; the search hit area is
at least 44px and existing row actions use their existing phone behavior.

UI-02: nonmatching query; same desktop/phone scroll region (AC .4).

```text
Custom prompts                            [+ Add]
[ missing                                 ]
  No results.
```

UI-03: a matched entry being edited; search is disabled, row and draft remain
mounted (AC .5-.6). Create, delete confirmation and busy likewise disable search.

```text
[ review                          disabled ]
[ Review: existing editor and actions       ]
```

## Tests

Extend `apps/web/components/settings/prompts-settings.test.tsx` without replacing
fixtures or existing assertions. Mounted RED then GREEN covers name/body beyond
preview, case, literal and whitespace matching, reset/order/builtin, actual
empty versus no results, immutable saved objects (AC .1-.4), search disabled in
edit/create/delete/busy, edited draft preserved, existing actions (AC .5), and
no mutation or settings dirty/navigation guard from search (AC .6). Optional
extracted presentation component gets a focused test only if needed. Existing
consumer regressions remain in the focused run.

## E2E tests

- `apps/web/e2e/tests/settings/saved-prompt-search.spec.ts`, chromium: disposable
  uniquely named real prompts, keyboard query by name/body, matches, no results,
  clear/reset and existing order (AC .1-.4, .6). Clean up only created IDs.
- `apps/web/e2e/tests/settings/mobile-saved-prompt-search.spec.ts`, mobile-chrome:
  disposable real prompts, touch focus and text input, rendered matches, no
  results and clear/reset, input/ancestor geometry and zero document overflow
  (AC .1-.4, .7). Inspect rendered phone view against UI-01/02. Include a
  narrow fine-pointer geometry regression for the phone-size contract.

Use isolated fixtures; never touch live operator prompts. Preserve any owned
synthetic screenshot originals, manifests and hashes externally before another
managed run clears them. Test projects run separately with one worker and zero
retries after matching same-worktree builds.

## Work orders

- [x] [Task 01: Search saved prompts](task-01-search-saved-prompts.md)

## Verification results

Mounted RED failed on the missing searchbox; the final focused consumer suite passed 19 tests. Typecheck, focused lint, i18n checks and staged ratchet, E2E sleep ratchet, public-doc validators, spec validators and diff checks passed. Matching backend, web and fixture builds passed. Desktop chromium and mobile-chrome E2E each passed one test, including phone, narrow fine-pointer and coarse-pointer geometry. Screenshot originals and manifests were preserved externally with SHA-256 hashes before the subsequent managed run.

Corrective verification also passed: both causal saved-refresh regressions, the final 23-test mounted suite, lint, typecheck, spec checks, matching web rebuild, and the desktop/phone browser cases. Edited and delete-target rows retain their current list positions only while their IDs remain loaded.

## Risks

- A filtered array mistaken for the underlying collection would replace the true
  empty state or hide a draft. Maintain the two meanings and test the mounted consumer.
- Sibling guide edit and PR 1512 touch adjacent work; preserve all unrelated bytes
  and refresh main/source gap and fully paginated open-PR inventory for every
  actual owned path before publication. Reassess changed overlaps.
- A managed browser run can clear assets; retain owned original evidence first.
