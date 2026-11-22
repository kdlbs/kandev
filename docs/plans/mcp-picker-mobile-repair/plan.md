---
created: 2026-10-19
status: implemented
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
legacy_specs: []
---

# Implementation Plan: MCP picker and mobile review repairs

## Overview

Repair three user findings from PR 3293's manual testing guide: duplicated
repository disclosure, sparse repository/task choices, and overlapping phone
Connections rows with an off-center header. Implement picker consistency first,
then phone geometry. Both work orders and their targeted verification are complete.

Agents owns this vertical contract through workspace MCP selection surfaces.
The additive composition ADR and session-owned evidence contract remain intact;
these presentation repairs need no new ADR.

## Confirmed scope and evidence

Confirmed by the user's review: one repository MCP disclosure, informative
repository/task choices, readable phone Connections and centered touch header.
No unresolved product choice requires an interview.

Source evidence at `d30fed2ba6c2c616f366e0027e1c84699e17079c`:

- `RepositoryMCPSelectionSection` wraps `MCPSelectionPicker`, which adds its
  own disclosure/drawer. The screenshot matches that composition.
- `ServerRow` gates `McpDefinitionDetails` and its accessible description on
  `sessionPresentation`. Repository and task-create callers omit that flag.
- `McpServerList` uses a multiline Button with base `h-auto`. The shared Button
  retains `max-md:h-11` and `[@media(pointer:coarse)]:h-11`, so phone rows stay
  44px while their explanatory text overflows.
- `McpExplorerHeader` lays out copy and a trailing close action in a flex row.
  The copy is centered in the remaining width, rather than the drawer width.

Read-only Pixel 5 reproduction on the existing isolated preview at :48595:
open test task 02, activate the MCP plug, select Connections. Row bounds are
44px tall; the content is 68px tall and extends 33px below its row. The next
row starts 48px later, intersecting that text. Drawer midpoint is 196.5px;
title and description midpoint is 168.5px, a 28px offset. No selection writes
or reseeding occurred and the browser was closed.

Local evidence, deliberately outside version control:
`/tmp/kandev-pr3293-test-runtime/review-layout-evidence.json` and
`review-mobile-before.png`. A temporary reproduction script is alongside them.
These are diagnostic observations, not passing regression tests.

Classification: phone overlap violates AC-006.5; the responsive sizing design
was incomplete. Amend AC-006.5 for explicit header alignment, AC-006.7 for a
single repository expansion, and AC-006.21 for metadata in every shared picker.
The original origin package intentionally excluded non-session picker changes;
this follow-up expands that presentation scope without rewriting its completion.

## Scope

### In scope

- Repository single expansion, retaining count and shared save/discard.
- Shared choice description/mode/transport/source in repository, task-create,
  profile and new-agent callers, matching existing Customize metadata.
- Content-sized Connections rows and centered phone/coarse-pointer header.
- Focused desktop/phone E2E, testing-guide correction and isolated asset update.

### Out of scope

Inherited disabling, runtime union or evidence changes, APIs/storage, polling,
new profile routing, marketplace changes, global Button sizing and redesigning
existing task-create controls. Do not modify the main :9998 instance or reseed
the user's preview selections.

## Technical approach

### Picker presentation

In `repository-mcp-selection.tsx`, keep the existing outer Collapsible and
selected count. Own query locally and mount `MCPSelectionContent` directly with
the existing repository description and workspace ID. Keep load/save errors,
baseline IDs, shared save contribution and discard semantics unchanged.

In shared `ServerRow`, reuse `McpDefinitionDetails` for every choice and connect
its support text through `aria-describedby`. Align checkboxes with the name;
wrap long names/descriptions/badges. Preserve aliases, disabled choices, search,
inherited exclusions and session-specific pending annotations. Do not enable
session origin grouping or autosave by passing `sessionPresentation` everywhere.
No fabricated description, transport configuration, endpoint or secret values.
Keep the surrounding compact form control and existing desktop popover/touch
drawer. Shared presentation naturally enriches profile/new-session callers too.

### Connections geometry

Keep the shared Button, focus/hover feedback and current selected state in
`mcp-server-list.tsx`. Override its height with content sizing at the same
phone and coarse-pointer variants as the primitive, retaining touch minimums.
Names, status and explanation occupy the row; dots and trailing icons align at
the top. Do not change shared control variants or add global CSS.

In `mcp-explorer-header.tsx`, give touch copy balanced side gutters, with the
close action in the trailing gutter. Center both title and description on the
surface, wrapping without collision at 320px. Keep the close target at least
44x44px. Preserve desktop left alignment and existing controller/navigation.

### Mobile composition and compatibility

Use the existing `McpServerExplorer` full-height phone drawer; retain its fixed
header/views, dynamic viewport height, safe-area clearance and single active
scroller. Coarse tablets keep their bounded drawer. Repository choices expand
inline in the existing settings form; task creation keeps its temporary picker
sheet. `MobilePickerSheet` is the fixed-header/scroll reference, not a new host
or controller to mount. Selection/capability/save behavior does not vary by
viewport or pointer. English and translated explanations must fit naturally.

## ASCII UI preview

UI-01: Repository settings, expanded (desktop and phone).

```text
Before: MCP servers [0] v
          MCP servers [0] v
After:  MCP servers [0] v
        Choose MCP servers for tasks using this repository.
        [Search MCP servers]
        [ ] Atlas documentation
            atlas-docs
            Shared project guides.
            [Remote endpoint] [Server-sent events] [Custom]
        ...existing shared Save changes / Discard...
```

UI-02: New Task > Advanced settings, open picker.

```text
MCP servers (info) [0 selected v]  (compact form control unchanged)
Picker:
  [Search MCP servers]
  [ ] Atlas documentation
      atlas-docs
      Shared project guides.
      [Remote endpoint] [Server-sent events] [Custom]
```

UI-01/02 share row composition. Phone wraps badges and description, retains
44px selection targets and reaches the final choice through one existing
scroll owner. No nested repository picker sheet. Initial loading, local errors,
no-match search and disabled choices retain their existing meanings.

UI-03: Composer plug > Connections, phone/coarse pointer.

```text
[space]            MCP servers                 [Close]
            Review MCP connections, tools,
                  and session additions.
Connections | Customize                 (fixed)
------------------------------------------------------
CURRENT AGENT                           (scrolls)
  o atlas-docs                       [evidence icon]
    Delivered, connection unverified
    Configuration sent to the agent. The connection
    has not been verified.

  o issue-tracker                    [evidence icon]
    Not available
    This agent does not support HTTP MCP servers.
```

Unlike the current fixed 44px row, the next row starts after all explanatory
text. Copy is centered on the drawer, not remaining space beside Close.
Desktop retains its left-aligned header and server/tool split. Structural
requirements are containment, centering, disclosure count and shared metadata;
ASCII spacing and example server data are illustrative. Reuse localized copy.

## Tests

Reuse the existing badge mapping unit suite and description component. No new
pure logic is planned; markup/geometry is verified through the actual browser.
AC references below abbreviate `AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-`.

## E2E tests

| Outcome | AC | Playwright evidence |
| --- | --- | --- |
| One repository expansion; save/discard/reload | 006.7, 006.4 | `settings/workspace-mcp-configuration.spec.ts`, phone counterpart: `opens repository MCP choices through one disclosure` |
| Description and passive badges in repo/task choices | 006.21 | Same settings specs plus task Advanced settings journeys in `task/task-mcp-selection.spec.ts` and its phone counterpart, using `expectMCPChoiceDetails` |
| Long and short connection rows do not overlap | 006.5, 006.4 | `chat/mobile-mcp-status.spec.ts`: `contains multiline MCP connection rows and centers the touch header at 320px/393px/767px`; desktop counterpart guards navigation and touch tablet |
| Header center/close containment, final row reachable | 006.5, 006.19 | Same chat specs; 320/393/767px touch phones and 900px touch tablet. Deployed preview adds 767px fine-pointer phone and 1440px desktop |

Check actual row and content bounding boxes, not just minimum height/class
strings: every text/icon lies within its row and following rows begin below it.
Check title/description center and computed text alignment, plus contained
close action. Include mixed short Active, long Delivered, Not available and
unknown/translated explanations. Use controlled fixture evidence, not network
endpoint availability. Exercise keyboard selection and phone taps, retained
search, disabled entries, and final-choice reachability. Preserve configured
single-worker runner limits; run suites sequentially.

## Work orders

- [x] [Task 01: Unify picker details and repository disclosure](task-01-picker-consistency.md)
- [x] [Task 02: Repair touch Connections geometry](task-02-connections-geometry.md)

## Verification results

Implementation complete in the primary session, without delegation. RED gates
reproduced both picker defects and phone row overlap before their corrections.

- Task 01: 25 unit tests, 14 desktop E2E and 10 phone E2E passed.
- Task 02/final reruns: 5 desktop scenarios passed (3 chat/tablet, 2 picker
  reruns); 4 phone chat scenarios and 2 final phone picker reruns passed.
  Across the package this is 31 distinct browser scenarios, with 4 picker
  reruns after the last production edit. Final unit rerun: 25 passed.
- Production build, typecheck, full lint, changed-file formatting, public docs
  validation (47 pages), specification catalog (349 decisions/1337 specs),
  all-spec lint and diff whitespace checks passed.
- Repository PR-documentation coverage validator on the actual changed paths:
  `covered`, zero errors, both work orders linked to the owning requirement and
  design. No synthetic production change is needed in the final preflight.
- Deployed web assets to the owned :48595 preview. All four selection tables
  compare equal before/after, with unchanged backend/proxy/main process IDs and
  start times. No backend restart, reseeding, main-instance change or push.
- Seven deployed viewport/theme/language checks passed, including Portuguese at
  320px and a 900px touch tablet. No row overlap/overflow; touch title and
  description offset is 0px. Final Customize choices remain reachable.

Exact commands and detailed results are in the two work orders. The manual
sweep now explicitly covers sections 3, 4 and 8; public selection guidance
explains the shared metadata. The global-profile catalog editor remains the
previously recorded separate gap, not resolved by this presentation repair.

## Risks

Richer choices increase list height, so scroll/final-choice checks are required.
Responsive Button overrides can win over base sizing; assert actual geometry.
The isolated preview contains user edits: asset-only deployment must prove
selection rows unchanged and guard owned PID identities before copying assets.
