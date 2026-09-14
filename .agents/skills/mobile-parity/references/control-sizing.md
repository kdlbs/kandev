# Control sizing

Use this guide when you add, change, or audit buttons, single-line inputs,
select triggers, combobox triggers, or their shared wrappers.

## Size contract

| Context | Control height | Usage |
| --- | --- | --- |
| Desktop, fine pointer | 28px | Ordinary actions and single-line fields |
| Compact desktop, fine pointer | 24px | Deliberate inline actions in dense chrome |
| Phone or coarse pointer | At least 44px | Action hit areas and single-line fields |

The pixel values assume the standard 16px root font. Preserve root-font scaling.
The desktop Start Task dialog is the reference for ordinary action density.
The default `@kandev/ui` Button, Input, and SelectTrigger already use `h-7`.
An action's importance changes its variant, not its height.

Use the same size for neighboring actions, inputs, and selectors within one row.
Use matching square dimensions for standalone icon controls.
Retain existing specialized controls, such as editor chrome and mobile pills,
only with a named purpose and an adjacent precedent.

Selection cards, navigation rows, multiline fields, and menu options have content
geometry. Do not force their entire surface to the ordinary button height.
Their embedded ordinary buttons still follow the control contract.

## Shared implementation

Use existing shared primitives and sizing helpers before adding local classes.
Inspect the current implementation before referencing a new helper from a plan.
Keep the fine-pointer desktop size as the base value.
Apply larger hit areas through a phone or coarse-pointer condition.
Use the canonical 768px phone boundary for new shared sizing rules.
Preserve existing overlay boundaries unless the task changes their composition.

Do not add a global CSS rule that assigns one height to every HTML button.
Do not change existing compact or large variant meanings to repair individual callers.
Use the standard variant at ordinary action call sites.
Keep single-line fields aligned with their attached reveal, clear, and picker buttons.

Check `height`, `min-height`, `max-height`, padding, line height, and parent stretching together.
An `h-7` button with `min-h-11` still occupies at least 44px.
A helper with `md:min-h-7` does not shrink a caller's explicit `h-11`.
SelectTrigger can also carry a `data-[size=default]:h-*` rule.
Verify the CSS cascade in the browser after changing these combinations.

## Sweep procedure

1. Search shared primitives, application components, route files, and CSS for height overrides.
2. Inspect ordinary Buttons, raw buttons, Inputs, SelectTriggers, combobox wrappers, and inline height styles.
3. Follow named class constants, wrapper helpers, size props, and responsive branches to their callers.
4. Classify each candidate as standard, compact, touch-only, content-sized, or a documented exception.
5. Replace ordinary desktop overrides with the standard size.
6. Preserve touch minimums and existing interaction behavior.
7. Record each retained exception and its reason in the sweep inventory.
8. Repeat the search after migration to find missed wrappers and newly exposed controls.

This search produces candidates, not confirmed defects:

```bash
rg -n '(min-h-|max-h-|h-|size-)(8|9|10|11|12|\[)|size="(sm|lg)"|height\s*:' \
  apps/web/components apps/web/app apps/packages/ui/src \
  -g '*.tsx' -g '*.ts' -g '*.css' -g '!*.test.*'
```

Also inspect controls with padding-only sizing and responsive height resets.
Do not apply a blanket replacement to this search output.

## Rendered verification

Verify representative dialog, task, settings, and shared-picker surfaces.
For each changed family, include normal, disabled, busy, and long-label states.
Compare the actual controls, not only their containers or class strings.

At the standard root font, ordinary desktop controls must measure 28px within 1px.
Compact desktop controls must measure 24px within 1px.
A desktop assertion of only `height >= 28` permits oversized controls to pass.
Touch actions must retain their 44px minimum active dimension.
Standalone touch icons need both height and width checks.

Include desktop fine pointers, phone widths below 768px, and coarse-pointer tablets.
Verify keyboard activation, touch activation, and the absence of overlapping hit areas.
Preserve the existing coarse-pointer input anti-zoom rule.
Check translated labels without reducing their text size to fit a fixed height.

The durable contract is in
[Control sizing requirements](../../../../docs/specs/ui/requirements/control-sizing.md).
