---
id: turn-changed-files-07
title: Changed-files card and desktop historical diff selection
status: done
wave: 7
depends_on: []
  - turn-changed-files-06
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-001
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-001.6
  - AC-TASKS-TURN-CHANGES-001.7
  - AC-TASKS-TURN-CHANGES-002.5
  - AC-TASKS-TURN-CHANGES-002.6
  - AC-TASKS-TURN-CHANGES-002.8
  - AC-TASKS-TURN-CHANGES-002.9
  - AC-TASKS-TURN-CHANGES-004.1
  - AC-TASKS-TURN-CHANGES-004.7
  - AC-TASKS-TURN-CHANGES-005.1
  - AC-TASKS-TURN-CHANGES-005.2
  - AC-TASKS-TURN-CHANGES-005.3
  - AC-TASKS-TURN-CHANGES-005.4
  - AC-TASKS-TURN-CHANGES-005.5
  - AC-TASKS-TURN-CHANGES-005.6
  - AC-TASKS-TURN-CHANGES-005.7
  - AC-TASKS-TURN-CHANGES-005.8
  - AC-TASKS-TURN-CHANGES-005.9
  - AC-TASKS-TURN-CHANGES-006.1
  - AC-TASKS-TURN-CHANGES-006.2
  - AC-TASKS-TURN-CHANGES-006.3
  - AC-TASKS-TURN-CHANGES-006.4
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Changed-files card and desktop historical diff selection

## Summary

Deliver the transcript card and a complete historical review flow in the existing desktop surface.

## Scope and owned files

- Shared projection helpers near `hooks/use-processed-messages.ts` with durable final-anchor and terminal fallback items.
- New modular card, exact-path tree model, availability rows, and focused tests.
- Shared/native message list and renderer integration outside collapsed tools.
- `lib/state/diff-target-types.ts`, shared navigation/selection persistence, panel action routing, and Dockview content.
- Historical content adapter/domain hook used by `TaskChangesPanel` and existing `FileDiffViewer`.
- Scoped localization keys for all desktop-visible copy.

## Exclusions

No new renderer, standalone diff app, workspace mutation, or historical content fetch from live workspace expansion.
Mobile-specific drawer composition belongs to 08; shared targets must support it now.

## Implementation acceptance

1. Exactly one card/availability row follows the matching final reply or stable terminal fallback, outside tool groups and without reader-position jumps.
2. Exact checkout/path tree aggregation preserves kinds, binary/null counts, rename paths, expansion, selection, keyboard focus, and off/on retained history.
3. Header and file actions select the exact immutable turn in Dockview; whitespace rendering preserves canonical totals and existing source navigation works.

## Verification

Tests exercise two checkouts with the same path, a renamed binary file, mode-only changes, partial/expired capture, late final output, and transcript pagination.

```bash
cd apps/web
pnpm exec vitest run hooks/use-processed-messages.test.ts hooks/turn-changes-projection.test.ts lib/turn-changes/tree.test.ts lib/state/historical-turn-navigation.test.ts components/task/chat/turn-changed-files-card.test.tsx
pnpm e2e:run --project chromium tests/git/turn-changed-files.spec.ts
```

The integrated E2E must capture real Git changes through the new lifecycle seam, then edit/commit again before opening the old card.
Check exact diff content, final reply placement, collapsed tools, reload, off/on visibility, selected scope, and existing live/PR/commit routes.
Compare light/dark rendered screenshots with previews UI-02 through UI-04 and UI-06.

## ASCII UI preview

UI-02: Final reply and ready card. [Combined preview](plan.md#ui-02-desktop-final-reply-and-initially-collapsed-tree).

```text
Assistant: Updated validation and its tests.
+-------------------------------------------------------------+
| 4 changed files +42 -8              [Expand all] [Open diff] |
| > src/                                      +30 -5          |
| > tests/                                    +12 -3          |
| A logo.png              Binary                             |
+-------------------------------------------------------------+
```

UI-04: Existing desktop Changes surface. [Combined preview](plan.md#ui-04-desktop-existing-diff-surface-with-historical-scope).

```text
Changes
[Turn 7, 14:32 v]     4 changed files +42 -8
[ ] Ignore whitespace
app / src / validation.ts
- old validation
+ new validation
```

UI-06: Availability rows. [Combined preview](plan.md#ui-06-availability-and-canceled-turn-fallback).

```text
Preparing changes...
Turn changes unavailable
2 files with available changes +18 -4   [Open diff]
Partial: assets changes unavailable
4 changed files +42 -8  History expired [Open diff: off]
```

Folder controls expand only. File controls open exact checkout/file. Explicit historical scope stays fixed after later turns.
Tree rows participate in transcript scroll; no nested card scroller. Map previews to AC-005 and AC-006.1-.4.

## Dependencies and risks

Depends on 06. Keep state dependency-neutral; never import UI modules into the store.
Late COMPLETE output and paginated history cannot anchor a card under an earlier or unrelated reply.
Renderer context expansion must use historical blobs rather than current files.

## Results

Implemented the final-reply card, collapsed repository tree, explicit historical diff targets, desktop Changes navigation, and historical read-only rendering. Full frontend tests and typecheck passed. The desktop Chromium changed-files and settings E2Es passed (2 tests).

### Review follow-up (2026-10-08)

Archived tasks route historical targets before live-workspace guards. The desktop historical surface has Current, Latest captured, and exact Turn N scopes; Latest pins to a change-set ID. Repository and folder counts identify loaded versus authoritative totals; rows expose change kind/mode and accessible status, and duplicate file paths are disambiguated by checkout. Failure/partial/overlap states are visible. Focused frontend tests passed, and the desktop historical-turn E2E passed after reload and scope switching.
