---
created: 2026-10-22
status: completed
requirements:
  - REQ-UI-BUTTON-STATE-COLORS-001
system_design:
  - ../../specs/ui/system-design/button-state-colors.md
legacy_specs: []
---

# Button State Color Repair

## Diagnosis

The user's dark-mode Markdown Save screenshot shows gray content over a dim
purple fill. Shared Button applies `disabled:opacity-45` to both foreground and
background. Default buttons retain primary colors beneath that opacity.
Three editor Save shortcuts additionally use page-level muted foreground.
The phone Save has the same parent opacity without the separate shortcut.

An AST audit found 286 omitted/static-default Button sites, of which 206 have an
explicit disabled prop. These are potential shared consumers, not 206 separately
verified visual defects. The [inventory](inventory.md) records exact locations,
seven custom-colored consumers, dynamic/spread limits, and adjacent findings.

## Scope and sequence

One sequential work order owns the shared correction and necessary caller
integration. Neutralize unavailable default actions; retain feature-specific
enabled colors. Make editor shortcuts and the Send spinner inherit foreground.
Preserve granted-notification and completed settings-Save success states. Set the sidebar Canvas trigger
to its neutral variant. Retain other button families and action behavior.

No material question remains: the reported defect concerns readability and
state clarity. Use existing theme tokens and the existing phone composition.

## UI preview

**UI-01: Editor Save, desktop and phone.** Entry: an unchanged file, then a
local edit, then Save. Current unavailable state: gray content over faded
purple. Proposed unavailable state: neutral muted surface with readable muted
label/icon. Available state: existing primary surface with one consistent
foreground for icon, label, and desktop shortcut. Busy state: existing progress
label/spinner, using the unavailable action palette when disabled.

Desktop keeps its mode controls, Delete, and compact Save row. Phone keeps
its existing mode menu, overflow, and 44px Save action; it has no shortcut label.
No control is moved. These choices map to `.1`, `.2`, `.3`, and `.6`.

**UI-02: Shared exceptions.** Unavailable custom-colored actions use the same
neutral palette; available actions retain their existing colors. Granted
notification permission and completed settings Save retain readable green status
treatment. The sidebar
Canvas setup action is neutral at rest and on hover; settings creation remains
primary. These choices map to `.4` and `.5`.

## Work orders

- [x] [Task 01: Correct shared action colors](task-01-shared-colors.md).

## Risks and exclusions

Shared styling reaches many consumers. Custom dark colors and explicit child
foregrounds can bypass inheritance, so test the recorded exceptions. Do not
change the meaning of disabled state, rename actions, or neutralize all variants.
Raw green merge buttons have their own styling and are outside this correction.

## Results and delivery

Implementation was explicitly authorized in the next user turn. Eight production
files now implement the shared repair and caller exceptions. The existing branch,
prior Markdown fixes, uncommitted guide, evaluation files, and main instance on
port 9998 remain preserved. No commit or push.

Rendered diagnosis passed in Firefox desktop and WebKit phone, light and dark.
Actual Save opacity is 0.45; the shortcut has an independent muted foreground.
A DOM-only neutral prototype measured 4.35:1 light and 4.05:1 dark text contrast.
These are diagnostic/proposal results; the application build remains unchanged.
All 66 fixture files, database, main PID 1155, and evaluation PID 398656 remain
unchanged. No browser session was left open.

Design validation: catalog check passed (369 decisions, 1496 specifications),
all-specification lint passed, Prettier and whitespace checks passed. The
work-order documentation-coverage preflight passed with the planned shared
Button production path; it does not imply implementation. All five package
files remain untracked and unstaged.

Implementation verification completed on 2026-10-23. Rendered tests reproduced
the Save opacity defect and the custom-green dark cascade defect before their
respective repairs. All 108 targeted unit tests and 43 distinct browser cases
passed across full suites and focused reruns. Typecheck, i18n, changed-file
lint/format, catalog, specification lint, coverage, and whitespace checks passed.
The E2E lint output retains two existing nested-ternary warnings.

The isolated evaluation was refreshed to PID 1126364, then passed eight live
Save checks in native Firefox desktop and WebKit phone, both themes. Two real
unavailable-notification checks passed in WebKit. No file save was attempted.
All 66 fixture hashes, the database, and main PID 1155/start time were preserved.
Disabled Save contrast is 4.35:1 light and 4.05:1 dark. The guide remains untracked
and unstaged. No commit or push was performed.

Evidence: `/tmp/kandev-pr3113-evaluation-4e7mws9y/button-colors-runtime-smoke.json`.
Asset SHA-256: `1871381e94d582889055cd427b88871f2bca8b9689e1378245ee2e82bf386998`.
Final source manifest: `0ef341a1ea3da52289cf32c3ad13b6ee2fbaf93f786ef94deb317a9c91b0d10b`.
