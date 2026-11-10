---
created: 2026-10-14
status: implemented
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
legacy_specs: []
---

# Implementation Plan: MCP Customize origin sections

## Overview

Make Customize explain why each server is included through populated origin
sections, then provide equally informative rows for optional session additions.
One focused work order covers the projection, presentation and real selection
journeys. Implementation and targeted verification are complete.

Agents owns the package because it owns additive MCP selections and the data
that explains their contributors. This extends the existing vertical contract,
not a separate UI specification. No new ADR is needed: grouping is reversible
presentation, while additive composition remains governed by the existing
[workspace MCP decision](../../decisions/2026-09-01-workspace-mcp-configuration.md).

## Confirmed intent and presentation choices

- Confirmed: separate task, repository and agent-profile origins; omit empty
  sections; add descriptions and definition metadata to the bottom choices.
- Verified: `useTaskSessionMCPSelection` collects all task repositories, the
  session profile and task selections. `MCPInheritedSelection` already contains
  all contributing origins, collapsed by definition identity.
- Design choice: show each inherited definition once, task before repository
  before profile. Show other contributors under "Also selected by" in plain
  labelled text. This avoids duplicate rows without concealing shared selection.
- Preserve the reviewed plug entry point, application summary, exact pending
  changes, autosave and read-only inheritance. Task-local disabling stays deferred.

## Scope

### In scope

Customize's session branch of the shared picker: populated sections, owner-name
attribution, richer checkbox rows, disabled/pending annotations, translations,
desktop/phone coverage and matching public selection documentation.

### Out of scope

Inherited disabling, changes to runtime union or attachment health, APIs,
storage, polling, marketplace discovery, profile routing, Advanced settings
composition and new Save/Apply controls. Do not reseed the user's preview or
modify the main instance at :9998.

## Technical approach

1. Add a pure `mcp-origin-groups.ts` projection beside the existing origin
   formatter. Use stable definition IDs and contributor tuples; preserve input
   row order within the three fixed groups. Retain all sources. If a row lacks
   recognized contributors, keep it in a populated-only generic task-setup
   fallback, without inventing a task selection. Scope/owner names use
   `formatMCPOriginLabel` and its existing workspace boundary.
2. Extract presentational description/metadata from `McpInheritedServerDetails`
   into a shared definition-details component. Reuse `mcpDefinitionBadgeKeys`
   and the readable neutral badge treatment. Source metadata (Custom, Registry,
   Curated) describes catalog provenance, not the selecting task or repository.
   Legacy import remains unlabelled unless a dedicated truthful key is added.
3. In `MCPSelectionContent`/`InheritedServers`, render semantic origin sections
   only for `sessionPresentation`. Task rows use the current-task section;
   repository/profile rows show named primary contributors as text. Multiple
   repositories remain explicit on the one row. Other contributors use an
   "Also selected by" text block instead of origin chips. Remove redundant
   per-row source headings/badges from this branch.
4. In session `ServerRow`, retain name and alias, add nonblank description and
   shared metadata below them. Align the checkbox to the name, give the complete
   row one selection target, and associate support text accessibly. Keep
   disabled status and pending action annotations separate. Blank descriptions
   need no placeholder. Preserve inherited-ID exclusion and existing name/alias
   search; searching does not hide pending changes above the list.

No change to `useMCPSelectionEditor` ownership, save semantics or application
timing. Retain stable row keys; refreshing application state must not regroup
unchanged choices. Non-session pickers keep their existing compact presentation.

### Mobile design contract

Entry point stays the composer plug, opening Customize in the existing explorer.
The nearest shipped exemplar is the full-height `McpServerExplorer` drawer;
the curated `mobile-picker-sheet.tsx` provides the fixed-header/internal-scroll
pattern. Reuse this full-height surface because origin details plus many choices
are deeper than a short picker. No nested sheet or disclosure.

Phone hierarchy is fixed title/views, then one scrolling body containing status,
pending actions, origin sections and additional choices. Names, descriptions,
badges and source text wrap. Keep dynamic viewport height, safe-area clearance,
44px phone/coarse-pointer row targets and dismissal/focus return. Fine-pointer
desktop retains the bounded dialog and surrounding control density. Desktop and
phone share projection, selection state, filtering and save handlers.

## ASCII UI preview

UI-01: Current source-backed Customize body.

```text
Application status / changes waiting to apply
FROM TASK SETUP [3]
  Atlas docs
  Description / metadata badges
  Where it comes from
  [Task: ...] [Repository: ...] [Profile: ...]
ADDITIONAL SERVERS
  [Search MCP servers]
  [ ] Issue tracker
      issue-tracker
```

UI-02: Proposed desktop, plug > Customize. Example includes three distinct
servers; the shared documentation server appears once despite two sources.

```text
MCP servers                                           [Close]
Connections | Customize
-------------------------------------------------------------
Application status / changes waiting to apply

SELECTED FOR THIS TASK [1]
  Review notes
  Notes for reviewing changes.
  [Remote endpoint] [Server-sent events] [Custom]

FROM REPOSITORIES [1]
  Atlas documentation
  Shared guides for the infrastructure team.
  [Remote endpoint] [Server-sent events] [Custom]
  Repository: Atlas API
  Also selected by
    Agent profile: Mock - Atlas inherited tools

FROM AGENT PROFILE [1]
  Search tools
  Search project references.
  [Existing executable] [Standard input/output] [Custom]
  Agent profile: Mock - Atlas inherited tools

ADDITIONAL SERVERS
Choose servers from this workspace's catalog for this session.
[Search MCP servers]
  [ ] Issue tracker
      issue-tracker
      Find and update project issues.
      [Remote endpoint] [Streamable HTTP] [Custom]
  [x] Session extras
      session-extras
      Search test artifacts.
      [Existing executable] [Standard input/output] [Custom]
      Add to setup (only while this change is pending)
```

UI-03: Proposed phone. Fixed header/views; everything beneath is the one
existing Customize scroller, not multiple section scrollers.

```text
MCP servers                         [Close]
Connections | Customize
-------------------------------------------
Application status / pending actions

FROM REPOSITORIES [1]
Atlas documentation
Shared guides for the infrastructure team.
[Remote endpoint] [Server-sent events]
[Custom]
Repository: Atlas API
Also selected by
  Agent profile: Mock - Atlas tools

ADDITIONAL SERVERS
[Search MCP servers]
[ ] Issue tracker
    issue-tracker
    Find and update project issues.
    [Remote endpoint] [Streamable HTTP]
    [Custom]
```

UI-04: Changed edge states, shared desktop/phone composition.

```text
No inherited selections: omit all origin sections.
ADDITIONAL SERVERS
[Search MCP servers]
No matching choices: existing localized empty message.

Disabled addition:
[x] Archived docs                        [Disabled]
    atlas-archived
    Optional supplied description
    [Remote endpoint] [Server-sent events] [Custom]
    Existing pending annotation, if any
    Checkbox is unavailable; saved membership stays truthful.

Unattributed inherited row:
FROM TASK SETUP [1]
  Legacy tools
  [Existing executable] [Standard input/output]
  Source unavailable (no invented owner or raw ID)
```

Headings, populated-only groups, one inherited row per definition, visible
contributors, metadata and scroll ownership are structural requirements.
Example names, line breaks and final copy are illustrative. Use existing tokens
and semantic headings; badges remain passive, and no section is collapsible.
Initial loading and load/save failure keep their existing dedicated feedback;
refresh retains loaded rows instead of flashing a new loading screen.

## Tests

| Acceptance | Evidence |
| --- | --- |
| 003.4, 003.8, 006.20 | New `mcp-origin-groups.test.ts`: task-only, repository-only, profile-only, every overlap, multiple repositories, duplicate IDs/contributors, empty/unattributed inputs, disabled membership, stable order |
| 006.21 | Existing `mcp-definition-badges.test.ts` including HTTP alias/legacy mapping; desktop and phone assert descriptions, labels and blank-description omission on selectable rows |
| 003.7, 006.11, 006.16 | Existing editor tests and shared rejection/refresh journeys: inherited exclusion, authoritative writes, retained search/focus |

## E2E tests

Extend `e2e/tests/task/task-mcp-selection.spec.ts` and
`mobile-task-mcp-selection.spec.ts`, with shared helpers. Real fixtures include
task-only, repository-only, profile-only, a server in all three scopes, one
server contributed by two repositories, disabled and optional session choices.
Assert each ID has one inherited row, all contributor names remain visible,
empty group headings disappear, inherited choices have no checkbox, and rich
choices can be selected and persist after reload.

Adapt `mcp-inheritance-badges.ts` and hierarchy assertions to section attribution
instead of origin chips. Existing status, deferred/rejected-save and pending
comparison journeys must continue to pass. Phone verifies one contained drawer,
internal scrolling to the final choice, wrapped long names, 44px selectable
targets and no horizontal overflow. Include a narrow fine-pointer phone check.
Inspect light/dark desktop and phone captures after implementation.

## Work orders

- [x] [Task 01: Group origins and enrich session choices](task-01-origin-sections.md)

## Verification results

Completed on 2026-10-14. Task 01 is done; application/controller behavior matches
the paired active requirement and current design. No delegation or PR mutation.

| Check | Result |
| --- | --- |
| Targeted five-suite Vitest command | 54 passed; 13 grouping tests include every overlap, multiple repositories, duplicate identities, disabled membership and fallback. Group suite rerun after its type annotation correction: 13 passed. |
| Typecheck, scoped ESLint, final Prettier | Passed, no scoped lint warnings |
| i18n check and new-code ratchet | Passed across seven shipped languages and pseudo; nine messages added/updated. Traditional Chinese generated; unrelated chat-key conversion drift restored. |
| Native, production web and E2E plugin fixture builds | Passed; task-private Go cache used |
| Desktop task/pending/status E2E command | 13 passed (3.8m), including narrow fine-pointer and coarse-pointer tablet journeys |
| Phone task/pending/status E2E command | 10 passed (3.0m); whole-row tap saves and persists after reload |
| Public-doc validators | 62 tests and 47 pages passed; existing explanation page updated |
| Catalog, spec lint and planned-code PR-doc references | 349 decisions / 1337 specs valid, all-spec lint passed, package references covered |
| Isolated preview rendered checks | Six modes passed: desktop dark/light, phone dark/light, 320px phone and 767px fine-pointer phone. Unique origin rows, passive metadata, one scroll owner, final-row reachability and no horizontal overflow. Screenshots inspected. |

RED evidence: new unit assertion failed before grouping; browser assertion
failed on missing origin headings before presentation changes. Browser checks
then exposed assumptions in the test helper: a named fixture was not necessarily
the final catalog row, and a fine-pointer context cannot tap. Assertions now
measure the actual final choice and choose input by touch capability. A first
suite was interrupted with exit 143; the final full runs above completed with
exit 0. No production scrolling/save change was made to accommodate these
fixture corrections. Stale fixture package was rebuilt after locale generation.

Fresh frontend assets published to :48595, retaining old hashed assets for open
tabs. Saved selections and main/preview process start identities were preserved;
no backend restart, reseed or :9998 mutation. Evidence lives under
`/tmp/kandev-pr3293-test-runtime/`: `origin-unit-tests.log`,
`origin-desktop-e2e-verified.log`, `origin-mobile-e2e.log`,
`origin-sections-visual.json`, `origin-sections-deployment.json`, and
`origin-sections-{top,choices}-*.png`. Exact commands remain in the work order.

## Risks

- Grouping priority could be mistaken for override priority: secondary sources
  must remain explicit, and documentation must explain the additive union.
- Richer rows increase content height: do not regress the repaired scroller.
- Shared picker changes can leak into task creation; use its session branch.
- Empty or unavailable names must never reveal identifiers or foreign names.

## Companion records

The delivered [status clarity](../mcp-status-clarity/plan.md),
[pending changes](../mcp-pending-changes/plan.md),
[composer customization](../composer-mcp-customization/plan.md) and
[workspace package](../workspace-mcp-configuration/plan.md) stay historical
delivery records. Their origin-chip previews describe earlier iterations;
UI-02/UI-03 here specify the delivered composition. Existing application
state, controller and Advanced-settings work remain in force.
