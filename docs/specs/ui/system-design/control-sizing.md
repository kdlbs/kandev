---
status: current
system: ui
requirements:
  - REQ-UI-CONTROL-SIZING-001
---

# Control sizing design

## Purpose and boundaries

The UI system owns shared control geometry. Feature components retain their
state, callbacks, permission checks, and translated labels.
This design centralizes size classes and removes ordinary per-page overrides.
It does not add a backend contract or a density preference.

## Requirement mapping

| Criteria under REQ-UI-CONTROL-SIZING-001 | Design section |
| --- | --- |
| .1, .2, .3, .6 | Shared sizes and caller migration |
| .4, .5, .9 | Responsive contract |
| .7, .8, .10 | Exceptions and interaction preservation |

## Current source

`apps/packages/ui/src/button.tsx` defines default, small, and large heights as
28px, 24px, and 32px. Input and default SelectTrigger use 28px.
SelectTrigger applies height through a data-size variant.
InputGroup also has a 28px shell and a separate multiline composition.

`apps/web/components/settings/settings-control.ts` applies a desktop minimum,
but larger caller heights still win. The settings helper does not reach every
settings control. Combobox and workspace triggers also accept local overrides.

The completed-session banner applies `min-h-11` with `size="sm"` on all
viewports. Layout actions use a desktop minimum of 32px. The workspace heading
picker uses 36px. These choices bypass the standard primitive dimensions.

## Shared sizes and caller migration

Add `apps/packages/ui/src/control-sizing.tsx` as the shared class source.
The existing package wildcard export supports this module without a new package.
It owns the standard and compact height classes, matching icon dimensions,
and adaptive touch classes. It contains no feature state or React hook.

Preserve the existing Button size API and its specialized `xs` and `lg` variants.
Use default Button size for ordinary actions and `sm` for deliberate compact
inline actions. Do not redefine `sm` globally as 28px.
Input retains its native HTML `size` attribute semantics and accepts
`controlSize="none"` when a fixed editor or file-browser field owns its own
geometry.

Reuse the shared size classes in Button, Input, SelectTrigger, and the single-line
InputGroup shell. Apply adaptive touch classes at shared application wrappers
and ordinary control call sites. Do not enlarge every primitive consumer
globally: InputGroup accessories and editor chrome have independent geometry.

Route settings-control helpers through this shared source. Keep typography
separate from dimensions. Migrate ordinary Combobox triggers and workspace
triggers through the same source. Preserve Combobox search, selection, and
portal behavior. Audit its `touchTarget` callers before replacing local dimensions.

Remove conflicting caller `h-*`, `min-h-*`, `size-*`, and data-size height
overrides. Inspect parent flex stretching and attached action widths.
An attached clear or reveal button must not retain a taller absolute hitbox.
Change complete control groups together so fields and buttons remain aligned.

## Responsive contract

Fine-pointer dimensions form the base classes. Adaptive classes apply the touch
minimum below 768px or for `pointer: coarse`. An `any-pointer` match alone does
not select large controls on a mouse-driven hybrid laptop.
The existing `any-pointer: coarse` input font rule remains unchanged.

The 768px condition controls sizing, not overlay composition.
Existing 640px Radix menu treatment remains intact.
CSS adapts geometry without remounting fields or resetting their state.

The Start Task dialog retains its split desktop action and stacked phone actions.
The completed-session banner retains its existing stacked phone composition.
The nearest mobile exemplars are Start Task's footer and settings-control helpers.
Settings actions continue to wrap within their existing page scroll owner.
Workflow navigation retains its Popover and coarse-pointer Drawer split.
The existing drawer owns scrolling and safe-area clearance.

## Exceptions and interaction preservation

The migration classifies each candidate as standard, compact, touch-only,
content-sized, or a documented specialized exception.
Selection cards, full navigation rows, multiline inputs, mobile pills, editor
controls, and embedded third-party surfaces require inspection before any change.
An HTML button tag alone does not identify an ordinary action.

Preserve tooltip wrappers, focus rings, semantic button types, disabled state,
busy labels, translated text, and event handlers.
For long labels, allow horizontal layout or group wrapping before clipping text.
Do not reduce font sizes to fit a mismatched control height.
If a button deliberately supports multiline content, classify it as content-sized.

## Verification

Browser geometry is the authority for the CSS cascade. At a 16px root font,
desktop standard and compact controls use 28px and 24px with a 1px tolerance.
Tests also compare adjacent control heights so wrong equal defaults cannot hide
an absolute-size regression. Touch tests assert minimum active dimensions.

Representative application tests cover Start Task, completed and recovery
banners, Layouts, workspace selectors, workflow actions, repository secrets,
storage fields, and shared integration pickers. Include desktop fine pointers,
phone widths of 390px and 700px, and a coarse-pointer tablet at 900px.
Cover disabled, busy, long-label, and keyboard/touch activation paths.

The initial source inventory is a candidate list, not proof of coverage.
Implementation records a disposition for every candidate file and repeats
searches for named helpers, CSS overrides, padding-only sizes, and size props.
Remaining exceptions need a reason and a source or rendered reference.

## Rationale

The accepted 28px size matches Start Task and existing default primitives.
A global button-height override also resizes cards and specialized chrome.
Changing Button's small variant alters unrelated dense controls.
Shared size classes plus a caller sweep preserve those distinctions.
This design records the rationale without a separate ADR.
