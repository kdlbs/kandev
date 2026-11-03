---
status: draft
system: agents
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
---

# Rich-output discovery and failure disclosure

## Boundary and requirement mapping

Agents owns this extension to [native rich output](agent-rich-output.md).
It covers discovery and inspection of a rejected presentation, including its
desktop and phone surfaces. Shared MCP identity and result selection remain
under the [UI contract](../../ui/system-design/kandev-mcp-tool-results.md).

- Requirement 001, criteria .13 and .21-.23: error selection and responsive disclosure.
- Requirement 003, criteria .11-.14: schema-derived discovery and strict validation.

## Discovery

`apps/web/lib/rich-output/guidance.ts` generates the short description from the
catalog schema. Include the StatusList item status enum in that description.
Derive the values from the schema instead of maintaining another enum list.
Keep the existing valid `passed` example in `examples.ts` and verify it.
Regenerate `rich_output_catalog_v1.guidance.txt` through the existing script.

Retain the 650-rune description limit and the existing task/Office instruction
budgets. Compress repeated policy wording if necessary, preserving routing,
property shapes, workspace-relative CSV guidance and host rendering ownership.
Do not expand the schema, accept `success` as an alias, or promise that guidance
prevents every invalid agent call. Test the generated description as advertised
by the backend, not only its source function.

## Error selection and state

`KandevToolMessage` already supplies the result parsed by `extractMcpResult`.
`RichOutputRenderer` currently discards it for every non-complete status and
returns a non-expandable `KandevRow`. For `error` only, provide an error
disclosure control in place of the existing status icon.

Add an optional status-control slot to `KandevRow`, used only by this renderer.
Permission-pending/rejected indicators must retain precedence. Other Kandev
tools, running/pending calls and successful cards keep their current behavior.
Do not change grouping, provider matching, status normalization or persistence.

A small helper selects a nonempty string result or a string `message`/`error`
field from a parsed object. Unsupported, empty, removed or missing payloads use
a localized unavailable-details fallback. Never stringify an entire object or
fall back to submitted arguments. Render retained diagnostics as plain text.
Keep schema validation's existing redaction contract: do not reconstruct or echo
submitted values. An icon-only trigger has a localized accessible name.

Use the first nonempty diagnostic line, capped at 160 characters, for the
tooltip. The popup contains the complete retained diagnostic, without tooltip
truncation. All host labels use translations in the seven shipped languages;
the diagnostic itself is retained tool output, not a translation key.

Disclosure state belongs to that failed message. Opening it never retries the
tool, reads a file, changes status, or changes another presentation's state.
No retry inference by title, automatic recovered badge, or success-card header
is introduced. Live updates and history use the same renderer.

## Responsive disclosure

The entry point is the existing red cross in task or Office chat. Use the
`RemoteCloudTooltip` touch disclosure as the interaction precedent, with
shared `@kandev/ui` Tooltip, Popover and Drawer primitives.

On fine-pointer desktop, hover/focus shows the short tooltip. Clicking or
pressing Enter/Space opens the error Popover. Suppress the tooltip while the
popup is open. Provide a close action, Escape dismissal and focus return.
Keep the cross visually small and use the existing compact 24px desktop target.

Use `useResponsiveBreakpoint().isMobile || useTouchDrawer()` to select the
Drawer, including narrow fine-pointer phones. Its hierarchy is a fixed title
and close control above one wrapping, internally scrolling diagnostic body.
This is a temporary inspection, so an inset bottom drawer fits better than
navigation to a separate page. Use a dynamic-viewport maximum height and bottom
safe-area padding. Phone/coarse-pointer triggers and dismissal controls have
48px nominal targets, verified at a minimum of 44px after rendering.

Reuse one error selection helper and one disclosure state across pointer modes.
Do not mount both surfaces or reset neighboring chart/file state on resize.
The closed state adds no persistent error text or success decoration.

## Compatibility and verification

Canonical MCP text results, null structured content with text fallback, and
existing parsed string/object results use the same disclosure path. Historic
recognized Kandev identities remain eligible. Foreign or incomplete identities
retain the generic tool path; no new provider support is claimed.

Test an invalid StatusList call followed by a corrected successful call with
distinct call/message IDs. Verify tooltip, popup, drawer, dismissal, missing
details, long plain-text errors, replay and successful-card state preservation.
Keep pending/running, permission and foreign-tool negative controls.

The [fix package](../../../plans/rich-output-failure-disclosure/plan.md) owns
execution and validation. Existing native-output ADR boundaries remain intact;
no migration, new transport, runtime flag, dependency or ADR is needed.
