---
id: "01-shared-draft"
title: "Retain one draft across responsive layouts"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-002
  - REQ-UI-MARKDOWN-FILE-EDITING-003
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-002.3
  - AC-UI-MARKDOWN-FILE-EDITING-002.4
  - AC-UI-MARKDOWN-FILE-EDITING-002.5
  - AC-UI-MARKDOWN-FILE-EDITING-002.14
  - AC-UI-MARKDOWN-FILE-EDITING-003.8
  - AC-UI-MARKDOWN-FILE-EDITING-003.9
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 01: Retain one draft across responsive layouts

## Summary

Give phone and desktop surfaces one canonical draft above the responsive
boundary. Repair baseline publication and protect navigation from draft loss.

## In scope

- Add a session/repository/path registry through the existing file-state
  mutation boundary. Keep `openFiles` as an active-session projection that
  references canonical records.
- Retain its session controller above `ResponsiveTaskLayout`. Mobile stores
  the selected key and uses canonical state instead of a private editable copy.
- Make restoration reuse live records. Separate projection clearing from
  explicit draft retirement.
- Publish accepted baseline/hash changes even when the text is unchanged.
- Guard routing, mobile switchers, and browser unload from the retained owner.
  Preserve current confirmation behavior through the existing dialog.

## Out of scope

- Draft disk persistence or backend save changes.
- Toolbar redesign, comment creation, hybrid recovery, and position handoff.
- Rewriting unrelated Dockview state.

## Acceptance

- `retains and saves one draft across the 768px boundary` first reproduces
  draft loss. After correction, mobile, tablet, and desktop edit and save the
  same unsaved bytes. The same path in another repository or session is isolated.
- `accepts a remote baseline equal to the local draft` first reproduces stale
  state. For A → local B → remote B/hash-B → local C, the buffer becomes clean
  at B and the next save uses B → C with hash-B.
- Controlled delayed responses prove that rotation preserves valid save
  acknowledgements, newer typing remains dirty, and discarded or replaced
  instances reject stale results. Cancelled navigation preserves the draft.

## UI preview

Draft transfer changes state, not toolbar composition. A dirty indicator
remains attached to the same file after rotation. Cancelling the existing
discard dialog leaves the file and destination unchanged. Confirmed discard
retires only the owned draft. UI-01's phone composition is implemented later
in Task 04. See the [combined preview](plan.md#ui-preview).

## Verification

Add `components/task/task-layout-file-drafts.test.tsx`. Mount the real retained
owner with responsive consumers. Use deferred promises for saved snapshots,
outside reads, replacement instances, and A → B → A session changes.
Test shared record identity and explicit retirement in the file-state suite.

Add the browser scenario prefix `review regression: responsive draft`.
Exercise 767px and 768px in both directions, open the same desktop file after
mobile edits, and save through the active layout. Include internal Markdown
links, task navigation, browser Back cancellation, and unload registration.
Run the regression first, then the complete block from the repository root:

```bash
(cd apps/web && pnpm exec vitest run lib/state/dockview-file-state.test.ts hooks/use-file-editors.mutation-ownership.test.tsx hooks/use-file-editors.test.tsx hooks/use-file-save-delete.test.ts hooks/file-editors-sync.ownership.test.tsx components/task/task-layout-file-drafts.test.tsx components/task/mobile/session-mobile-layout.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx components/task/mobile/mobile-navigation-guard.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project=mobile-chrome tests/task/mobile-markdown-file-editing.spec.ts --grep "review regression: responsive draft")
(cd apps/web && pnpm e2e:run --project=mobile-chrome tests/task/mobile-file-confirmations.spec.ts)
```

## Files likely touched

- `apps/web/components/task/task-layout.tsx`
- The tablet `TaskCenterPanel` tab controller and its affected tests.
- `apps/web/components/task/task-layout-file-drafts.test.tsx` (new)
- `apps/web/components/task/file-draft-session-provider.tsx` (proposed)
- `apps/web/lib/state/dockview-store.ts`
- `apps/web/lib/state/dockview-file-state.ts`
- `apps/web/lib/state/dockview-file-state.test.ts`
- `apps/web/hooks/use-file-editors.ts`
- `apps/web/hooks/use-file-editor-buffer.ts`
- `apps/web/hooks/use-file-save-delete.ts`
- `apps/web/hooks/file-editors-sync.ts`
- `apps/web/hooks/use-file-editors.mutation-ownership.test.tsx`
- `apps/web/hooks/use-file-editors.test.tsx`
- `apps/web/hooks/use-file-save-delete.test.ts`
- `apps/web/hooks/file-editors-sync.ownership.test.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/task/mobile/mobile-selected-file-state.ts`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`
- `apps/web/components/task/mobile/mobile-navigation-guard.tsx`
- The corresponding mobile test files in the verification command.
- `apps/web/e2e/tests/task/mobile-markdown-file-editing.spec.ts`

## Dependencies

None. Read the existing mutation-ownership and workspace-sync tests before
changing the shared file-state actions.

## Risks

The desktop restore path currently clears all active file states. Copying
mobile state into that map is insufficient. Do not synchronize two editable
copies through effects.

Preserve draft instance, owner epoch, and operation generation checks.
Separate canonical acknowledgement from Dockview panel effects. A valid save
can complete after a responsive unmount, but not after explicit retirement.
Keep unrelated settings navigation guards intact.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/markdown-file-editing.md).
- [Responsive draft ownership](../../specs/ui/system-design/markdown-file-editing.md#responsive-draft-ownership).
- Existing `FileEditorState` and its instance/visit ownership tests.
- `apps/web/lib/routing/navigation-guard.ts`.

## Results

Implemented one session-owned draft registry above responsive layouts. Desktop,
phone, and tablet use the same content and baseline. Save completion retains
newer typing and rejects retired instances, replaced records, and prior session
visits. Clean drafts revalidate on session activation and reconnect. Dirty
records survive tab restoration and responsive unmounts.

The targeted aggregate passed 255 tests across 26 files. It includes draft
ownership, mobile save, navigation guards, workspace synchronization, tab
restoration, and stale-operation regressions. The phone browser test passed for 767px to 768px transfer, saving in the new
layout, and returning to a clean phone draft. The existing discard and
confirmation flows also passed.
