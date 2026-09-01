---
created: 2026-08-31
status: in-progress
requirements:
  - REQ-UI-SURFACE-TEXT-HIERARCHY-001
  - REQ-UI-TASK-CLEANUP-CONFIRMATION-001
system_design:
  - ../../specs/ui/system-design/surface-text-hierarchy.md
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
legacy_specs: []
---

# Implementation Plan: Repair Surface Text Hierarchy

## Overview

Repair the mobile text hierarchy shown by task deletion and remove the same
shared defect from other surfaces. The source audit found that
`AlertDialogDescription` and `AlertDescription` apply
`text-balance md:text-pretty`, so phone prose is balanced while desktop prose is
pretty. The shared AlertDialog rule reaches 24 web files; the Alert rule reaches 35. Seven AlertDialog descriptions contain multiple paragraphs, lists, or
generated groups and need a focused structure pass.

Dialog, Drawer, and Sheet descriptions do not contain the inverted breakpoint
rule. They are not classified as broken, but their primitives join the same
semantic title/body contract so future surfaces do not diverge. The task archive
and delete family also needs a local copy hierarchy: a direct task outcome, a
scannable cleanup group, supporting reassurance, viewport containment, and a
semantic destructive Delete action.

## Scope

### In scope

- Standardize title and description wrapping in shared Alert, AlertDialog,
  Dialog, Drawer, and Sheet primitives.
- Normalize all seven structured AlertDialog descriptions while preserving
  their accessible description boundary and feature behavior.
- Restructure task archive/delete cleanup copy and its executor-specific data
  model across all five locale catalogs.
- Keep long phone content reachable and task cleanup actions touch-safe.
- Add component, localization, mobile, and desktop regression evidence.

### Out of scope

- Rewriting every one-sentence dialog description.
- Changing global typography tokens or responsive breakpoints.
- Applying global action sizing or vertical containment to every dialog.
- Changing task cleanup policy, APIs, state, callbacks, focus, or dismissal.
- Replacing centered alerts with drawers or sheets.

## Technical approach

First change the shared primitive defaults and lock them with one component
contract test. Titles receive balanced heading wrapping; descriptions receive
pretty prose wrapping and word containment at every breakpoint. Remove the
inverted phone/desktop rule from Alert and AlertDialog.

After that foundation, two independent consumer passes normalize structured
descriptions. One owns Quick Chat and agent-profile conflicts. The other owns
task session deletion, detachment, and environment reset. Both add explicit
left alignment, semantic paragraph/list markup, spacing, and local min-width
containment without changing copy or actions.

In parallel, the task-cleanup pass replaces the flat cleanup-line model with
ordered effects and supporting notes. Archive and delete dialogs plus compact
archive surfaces render that common model. It updates all locale catalogs,
bounds long full-dialog bodies to the dynamic viewport, gives phone footer
actions 44px targets, and uses the semantic destructive variant for Delete.

The last work order adds rendered mobile and desktop evidence after the three
implementation branches have landed. It uses long dynamic values and a longer
bundled locale, reads computed wrapping after portal animation settles, and
checks containment, action reachability, and non-destructive Cancel behavior.

## Dependency order

| Wave | Work order                                                                                                 | Dependency       | Parallel-safe ownership                          |
| ---- | ---------------------------------------------------------------------------------------------------------- | ---------------- | ------------------------------------------------ |
| 1    | [Task 01: Standardize surface typography primitives](task-01-standardize-surface-typography-primitives.md) | None             | Shared UI primitives and one new contract test   |
| 2    | [Task 02: Normalize structured app confirmations](task-02-normalize-structured-app-confirmations.md)       | Task 01          | Quick Chat and agent-profile files               |
| 2    | [Task 03: Refine task cleanup confirmations](task-03-refine-task-cleanup-confirmations.md)                 | Task 01          | Task archive/delete, cleanup model, task locales |
| 2    | [Task 04: Normalize structured task confirmations](task-04-normalize-structured-task-confirmations.md)     | Task 01          | Session delete, detach, environment reset        |
| 3    | [Task 05: Add responsive text hierarchy E2E](task-05-add-responsive-text-hierarchy-e2e.md)                 | Tasks 02, 03, 04 | E2E files only                                   |

Tasks 02, 03, and 04 may run concurrently after Task 01 is integrated. Their
production and component-test ownership is disjoint. Task 05 starts only after
all three consumer branches are integrated, so its rendered expectations target
the final composition.

## Acceptance evidence

| Acceptance criterion                                 | Evidence owner                                                   |
| ---------------------------------------------------- | ---------------------------------------------------------------- |
| `AC-UI-SURFACE-TEXT-HIERARCHY-001.1`                 | Task 01 primitive contract test; Task 05 computed-style checks   |
| `AC-UI-SURFACE-TEXT-HIERARCHY-001.2`                 | Tasks 02, 03, and 04 component tests                             |
| `AC-UI-SURFACE-TEXT-HIERARCHY-001.3`                 | Task 01 containment classes; Task 05 320px/393px rendered checks |
| `AC-UI-SURFACE-TEXT-HIERARCHY-001.4`                 | Task 01 class-override regression                                |
| `AC-UI-SURFACE-TEXT-HIERARCHY-001.5`                 | Existing consumer tests plus Tasks 02-05 focused behavior checks |
| `AC-UI-TASK-CLEANUP-CONFIRMATION-001.1` through `.4` | Task 03 data-model, locale, and component tests                  |
| `AC-UI-TASK-CLEANUP-CONFIRMATION-001.5` and `.6`     | Task 03 component tests; Task 05 rendered mobile/desktop checks  |
| `AC-UI-TASK-CLEANUP-CONFIRMATION-001.7`              | Task 03 callback regressions; Task 05 Cancel/task-survival check |

## Work orders

- [x] [Task 01: Standardize surface typography primitives](task-01-standardize-surface-typography-primitives.md) (completed)
- [x] [Task 02: Normalize structured app confirmations](task-02-normalize-structured-app-confirmations.md) (completed)
- [x] [Task 03: Refine task cleanup confirmations](task-03-refine-task-cleanup-confirmations.md) (completed)
- [x] [Task 04: Normalize structured task confirmations](task-04-normalize-structured-task-confirmations.md) (completed)
- [ ] [Task 05: Add responsive text hierarchy E2E](task-05-add-responsive-text-hierarchy-e2e.md)

## Risks

- A shared class change reaches many consumers. Lock primitive semantics first,
  then run representative Alert, AlertDialog, Dialog, Drawer, and Sheet tests.
- `text-wrap: pretty` is progressive enhancement. Keep ordinary wrapping as the
  unsupported-browser fallback and add explicit word containment.
- Radix `asChild` moves primitive classes onto the child. Structured consumer
  tests must assert the rendered description node, not only source literals.
- Cleanup copy is localized and shared by full and compact surfaces. Update the
  structured model, all catalogs, and every renderer atomically.
- Radix Slot combines wrapper and child action classes. Delete must select the
  destructive wrapper variant and remove competing manual semantic colors.
- Browser geometry can be transient during portal animation. E2E measurements
  wait for the surface subtree to finish animating.

## Documentation impact

No public documentation changes. This package updates internal UI requirements,
system design, and delivery records only.
