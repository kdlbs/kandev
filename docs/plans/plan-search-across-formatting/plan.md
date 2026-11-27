---
created: 2026-10-13
status: implemented
requirements:
  - REQ-UI-PLAN-TEXT-SEARCH-001
system_design:
  - ../../specs/ui/system-design/plan-text-search.md
legacy_specs: []
---

# Implementation Plan: Plan Search Across Formatting

## Overview

Plan find now returns phrases across inline formatting through one sequential
matcher correction. ROOT reviewed the four-artifact package and explicitly
released implementation and local checks in the same primary session.

## Evidence and root cause

ROOT qualified the production extension on
`713f9f1b5f071c460440c27caed2869c41236c46`, which is also this checkout's
inspected HEAD. ROOT reports a real ProseMirror Schema/EditorState reproduction
under Node 24: query `deploy service` found one plain occurrence but zero when
`service` was bold, inline code, or contained a mid-word italic boundary.
The command exited 1 with three causal assertion failures. Plain text, separate
paragraphs, and absent-query controls behaved correctly (three controls).
Evidence reference: `/tmp/kandev-root-plan-find-discovery-20261013.json`.
This is reported ROOT evidence, not a command rerun in this design session.
The protected private source was not read, copied, replayed, or cleaned up;
permanent regressions must be independently authored.

Source inspection confirms that `findMatches` runs an escaped global regex
separately on each text node and resets `lastIndex` for each node. Marks split
contiguous visible text into multiple nodes, so a phrase crossing a mark cannot
match. The actual editor registers this extension alongside StarterKit,
Markdown, Link, Highlight, Underline, CommentMark, lists, and tables.

## Requirement reconciliation

UI owns the reusable live-document find/highlight contract; Tasks owns durable
Plan data. The adjacent task-switch requirement protects editor availability
but does not specify phrase matching. Formatting presentation, chat result
ownership, task-plan persistence, and workspace search have distinct contracts.
No discovered active criterion defines matching across marks. Add the smallest
owning vertical [requirement](../../specs/ui/requirements/plan-text-search.md)
and [design](../../specs/ui/system-design/plan-text-search.md) as draft.

Confirmed by the request: literal case-insensitive find, cross-mark phrases,
correct offsets, highlights, wrapping, and recomputation after edits. Verified
in source: marks, existing command/state shape, and shared phone editor.
Agent choice under the autopilot boundary request: non-text inline nodes are
opaque run boundaries, including hard breaks; actual code-block text newlines
remain literal text. No unresolved design blocker remains. A separate ADR is
unnecessary for this local rule captured by the owning pair.

## Scope

### In scope

- Derive matches from consecutive text children within each textblock.
- Preserve document ranges, logical counters, decoration classes, wrapping,
  and the existing edit recomputation/ordinal clamp.
- Independently author plugin regressions and one focused rendered Plan test.
- Keep the four artifact statuses and verification results accurate.

### Out of scope

- Plan persistence, saving, initial load, task switching, accepted-state or
  delayed-result races, and other search companions.
- Layout, controls, copy/catalogs, regex features, search dependencies, cache
  architecture, or a broad editor refactor.
- Native delegates, recursive tasks, model/session changes, protected private
  reproductions, foreign resources, or paused taska6032d95.

## Technical approach

Change `findMatches` in
`apps/web/components/editors/tiptap/search-highlight-extension.ts` to process
textblock runs using each run's absolute document start. Traverse nested blocks
in document order, flush on every non-text inline child, and avoid duplicate
descendant scanning. Retain escaped `gi` matching and UTF-16 positions.
Do not export private matcher helpers solely for tests: register the actual
extension and observe its commands, plugin state, and decorations.

`usePlanFindShortcut`, `TaskPlanPanel`, `PanelSearchBar`, and the editor extension
registration are integration inputs, not intended edit targets. Existing
`apply`, command, decoration, and scroll paths consume the new ranges.

## ASCII UI preview

**UI-01: Existing Plan find region**, desktop and phone, active query with one
formatted phrase. The bar is the existing top-right panel overlay. Brackets
below denote highlighting, not literal Plan characters; `B(...)` denotes
existing bold formatting. This previews result data, not new product copy.

```text
Before (source-confirmed matcher result):
+--------------------------------------------------+
| [deploy service] [0 / 0] [Prev] [Next] [Close]      |
| deploy B(service)                                |
+--------------------------------------------------+

After (proposed result; shared desktop/phone region):
+--------------------------------------------------+
| [deploy service] [1 / 1] [Prev] [Next] [Close]      |
| [deploy B(service)] <- one current logical match  |
+--------------------------------------------------+
```

Desktop continues to host this region in the Plan workbench panel. Phone
continues to host the same region through `SessionMobileLayout.MobilePlanPanel`
in its Plan destination above existing bottom navigation. The drawing crops
that shared region and does not propose a phone width or new control sizing.
The bar remains over the existing scrolling editor body. Structure, marks,
controls, scroll owner, and safe areas are unchanged; spacing is illustrative.
Preview maps to criteria `.1`, `.3`, and `.6` and the focused rendered check.
Empty/absent results retain `0 / 0`; no new empty/error surface is introduced.

## Tests

New `apps/web/components/editors/tiptap/search-highlight-extension.test.ts`
uses a real Tiptap Editor registering StarterKit and the production extension,
with real ProseMirror state/transactions and cleanup. Add test marks or nodes
only in the test schema when needed. Use independently authored fixtures such
as `release build`, without reading ROOT's private reproduction.

| Criteria | Named test or table-driven group |
| --- | --- |
| `.1`, `.3` | `finds literal phrases across marks`: bold, inline code, link, underline, highlight, comment, and italic mid-word splits; mixed marks; exact ranges and decoration coverage |
| `.1`, `.4` | `keeps plain literal search controls`: plain/absent/empty, mixed case, punctuation such as `a+b.[x]`, exact whitespace, non-overlapping occurrences |
| `.2` | `stops at block and non-text inline boundaries`: paragraphs, empty blocks, hard breaks, opaque test-only inline atoms, and independent hits on both sides |
| `.2`, `.3` | `uses nested textblock document positions`: heading, quoted/list text, code text including real newline, separate code blocks, and cell paragraphs with explicit expected offsets |
| `.3` | `uses UTF-16 match positions`: supplementary character before a marked phrase and inside a marked query; exact range and decoration endpoints |
| `.3`, `.4` | `counts logical matches and wraps navigation`: repeated marked phrases, document order, one current logical range, Next/Previous wrap, zero-result no-op, query reset and clear |
| `.5` | `recomputes after document edits`: insertion before hits, adding/removing occurrences, deleting the selected final hit, removing all hits, formatting-only mark changes, and document replacement |
| `.6` | Same plugin data/commands in both surfaces; retain `tiptap-plan-editor.test.tsx` and `use-plan-find-shortcut.test.tsx` as shared wiring regression checks |

Tests inspect plugin state and `DecorationSet` as well as successful commands;
mocked matcher tests cannot establish this regression. A logical range may
render as several spans. Avoid assertions that count spans as occurrences.

## E2E tests

Add `finds phrases across formatting and wraps navigation` to
`apps/web/e2e/tests/search/plan-search.spec.ts`, project `chromium`.
Use `seedTask`/`planScript` and the shipped Plan panel. Include the existing
`quick brown fox` readiness marker so `openPlanPanel` stays unchanged, followed
by three `release build` occurrences: bold, inline-code, and mid-word italic
Markdown. Assert actual marks, counter `1 / 3`, highlighted text across mark
fragments, and current logical paragraph. Navigate through all three and wrap
forward/backward using the existing search controls/keyboard. An absent query
then gives `0 / 0`. Cover criteria `.1`, `.3`, `.4`.

Run that addition plus the existing basic-highlight and Enter-navigation
scenarios, not a full E2E suite or unrelated task-switch scenario. Managed
E2E rebuilds production web assets. No new mobile spec: the `mobile-parity`
data-only exception applies because both surfaces consume the same matcher
and this correction changes no viewport-dependent interaction. Phone outcome
is covered by the real plugin/shared-editor checks, with no claim of rendered
phone evidence in this design turn.

## Public documentation assessment

Internal docs updated: this requirement/design pair and delivery package.
`docs/public/tasks-and-workflows.md` already describes the shipped Plan surface;
searches of public docs, root README, and screenshot catalog found no published
cross-mark or structural search policy to amend. This speculative design adds
no public command, copy, route, screenshot, API, or workflow. No public docs
change is needed in this design turn. Reassess any implementation divergence
before finishing; do not introduce public-page scope without cause.

## Work orders

- [x] [Task 01: Match Plan text across inline formatting](task-01-match-inline-text.md)

One sequential work order; no dependencies or delegation.

## Verification results

Design checkpoint, 2026-10-13:

- `python3 scripts/list-docs.py validate`: passed, 379 decisions and 1580
  specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- UI catalog query for `plan-text-search`: discovers both new owning files.
- Local `.github/scripts/pr-docs.cjs::validateCoverage` preflight: passed,
  `ok: true`, `status: covered`, no errors, and the owning design/requirement
  accepted for Task 01. Input included the four new documents and the intended
  production matcher path, without pretending that production code changed.
  Used `/home/jcfs/.nvm/versions/node/v24.18.0/bin/node`; bare `node` is absent
  from this shell's PATH. No runtime install or dependency changes occurred.
- ROOT reports reading all four actual files in full and passing scope,
  boundary, UTF-16, decoration, navigation, edit, and narrow rendered-test design
  review. Implementation/local-heavy release remains pending a later interrupt.
- Documents remain draft, Task 01 pending, and all four artifacts uncommitted.
- Diff whitespace checks include the new untracked documents via
  `git diff --no-index --check`; no whitespace errors were reported.

Implementation completed after ROOT's later release:

- One frozen install from `apps/`: passed with pnpm 9.15.9, 935 reused packages,
  zero downloads. Node v24.18.0 and Go go1.26.0 verified; `GOTOOLCHAIN=local`.
- Independently authored causal RED against the unchanged production extension:
  18 assertion failures and 12 passing controls in 30 plugin tests. No import
  or compilation errors, and no access to the protected private source.
- Final task-defined Vitest command: 40 passed across the new plugin suite and
  existing editor/hook suites. Scoped ESLint with zero warnings, typecheck,
  staged i18n ratchet, catalog/spec lint, 36 linter tests, and diff checks passed.
- Managed E2E: exactly 3 passed in 27.1s, Chromium, one worker, one shard, no
  retries, fresh backend/web/plugin builds. Used the work-order command with
  `--host` and `CAPTURE_PR_ASSETS=1`; no Docker resources were touched.
  Native session 70588 joined exit 0; log:
  `/tmp/child117-84f121d2-delivery/kandev-run.e2e.8tGhUwZz.log`.
- Fresh `plan-search--plan-formatted-phrase.png` verified against the one-entry
  capture manifest and visually inspected using synthetic E2E data. No new
  phone test or capture: this shared matcher change has no viewport-dependent
  rendering or interaction contract, as documented in the owning design.
- Requirement is active, design current, plan implemented, Task 01 done.
  Hosted CI/review and publication receipts are tracked in the external task
  plan without optional follow-up documentation pushes.

The historical ROOT reproduction remains discovery evidence, separate from
the independently authored permanent regressions and their results above.

## Risks

- Whole-document concatenation can fabricate cross-paragraph or cross-atom hits.
- Code-point counting or lowercasing can shift UTF-16 offsets and highlights.
- Duplicate descendant scanning can inflate totals; fragmented DOM decorations
  can be mistaken for multiple logical matches.
- Match state, current decoration, and scroll targets must share the same range
  list after edits; retain the current ordinal clamp instead of new selection
  restoration policy.
- Planned checks require existing managed dependencies and ROOT's local-heavy
  lease. Do not replace caches, install, or start builds during this checkpoint.
