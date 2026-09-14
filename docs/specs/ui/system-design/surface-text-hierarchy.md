---
status: current
system: ui
requirements:
  - REQ-UI-SURFACE-TEXT-HIERARCHY-001
---

# Surface Text Hierarchy System Design

## Purpose and boundaries

This design defines semantic wrapping defaults for the reusable Alert,
AlertDialog, Dialog, Drawer, and Sheet title and description primitives. It
also defines the local treatment required when an AlertDialog description
contains paragraphs, lists, or generated groups.

The change is presentation-only. Radix or Vaul ownership of portals, accessible
names, focus, dismissal, and modal behavior does not change. Feature components
retain their state and business actions.

## Requirement mapping

| Requirement                         | Design sections                                                                                                                                                       |
| ----------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-SURFACE-TEXT-HIERARCHY-001` | [Shared primitive contract](#shared-primitive-contract), [Structured descriptions](#structured-descriptions), and [Responsive verification](#responsive-verification) |

## Current-state evidence

`AlertDialogDescription` and `AlertDescription` currently apply
`text-balance md:text-pretty`. That gives prose balanced line lengths below
768px and pretty wrapping only on larger viewports. The rule reaches every one
of the 24 web files using AlertDialog descriptions and 35 files using Alert
descriptions. Seven AlertDialog descriptions use structured child content, so
the same inherited rule affects every nested paragraph and list item.

Dialog, Drawer, and Sheet descriptions do not contain the inverted breakpoint
rule. They use ordinary wrapping, so they are a consistency gap rather than the
cause of the reported defect. Standardizing them in the shared package prevents
new surface families from diverging by accident.

## Shared primitive contract

The five title primitives add `min-w-0`, `text-balance`, and
`wrap-anywhere`. The five description primitives add `min-w-0`, `text-pretty`,
and `wrap-anywhere`. The zero minimum lets grid and flex items shrink, while
`overflow-wrap: anywhere` also lowers the text's min-content contribution and
contains arbitrary unbroken values. `overflow-wrap: break-word` is not
sufficient for this contract because it preserves the unbroken value's
min-content width. The two broken description primitives remove both
`text-balance` and the `md:text-pretty` switch. These classes live on the
semantic primitive rather than in every consumer:

- `AlertTitle` and `AlertDescription` in `apps/packages/ui/src/alert.tsx`;
- `AlertDialogTitle` and `AlertDialogDescription` in
  `apps/packages/ui/src/alert-dialog.tsx`;
- `DialogTitle` and `DialogDescription` in
  `apps/packages/ui/src/dialog.tsx`;
- `DrawerTitle` and `DrawerDescription` in
  `apps/packages/ui/src/drawer.tsx`;
- `SheetTitle` and `SheetDescription` in
  `apps/packages/ui/src/sheet.tsx`.

The existing `cn` merge remains the override boundary. A consumer can supply a
later Tailwind wrapping or alignment class for an intentional exception. No new
component property or package export is introduced. Browsers without advanced
text-wrap support retain normal wrapping as progressive fallback.

## Structured descriptions

Short one-sentence AlertDialog descriptions may continue to inherit the
header's centered phone alignment. Structured descriptions explicitly use
left-aligned prose and semantic HTML. Existing `asChild` consumers keep one
Radix description node while their child supplies paragraphs, lists, spacing,
and local overflow containment.

The consumer pass covers Quick Chat deletion, agent-profile deletion conflicts,
task archive and delete, session deletion, task detachment, and environment
reset. It does not mechanically rewrite every one-sentence confirmation. Task
archive and delete have their separate consequence hierarchy in the task
confirmation design.

Dynamic labels use the description primitive's zero-minimum and word
containment contract. Nested consumer list items also apply `min-w-0` where a
grid or flex item would otherwise use its min-content width. This contains
unusually long task names, profile names, paths, and identifiers without
breaking normal words unnecessarily.

## Responsive behavior

Desktop and phone keep the same modal or drawer surface already selected by
each feature. This design changes text wrapping, not navigation or presentation
mode. On phones, structured description content remains left aligned inside a
centered confirmation and actions retain their existing order. Any consumer
that grows beyond the viewport continues to use its local containment contract.

## Failure and recovery

There is no asynchronous failure path. If `text-wrap: pretty` is unsupported,
the browser falls back to ordinary wrapping. Long unbroken values use the word
containment fallback. If a consumer intentionally overrides wrapping, its
focused regression owns that exception.

## Persistence and security

None. No stored data, permissions, or trust boundary changes.

## Responsive verification

A component contract test renders all five primitive families and asserts title
and description class semantics, including the absence of responsive balanced
body copy. Consumer tests verify that structured descriptions keep semantic
paragraph/list markup and explicit left alignment.

Mobile Chromium coverage opens representative short and structured surfaces at
320px and 393px. It inspects computed `text-wrap`, exercises a longer bundled
locale and a long dynamic value, and asserts no document horizontal overflow.
A desktop representative check guards the same title/body hierarchy without
changing action density or surface placement.

## Related decisions

None. This standardizes established semantic typography through existing
`className` composition and creates no new public API or ownership boundary.
