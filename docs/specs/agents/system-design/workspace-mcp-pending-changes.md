---
status: current
system: agents
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
created: 2026-10-10
owners:
  - Kandev
---

# Workspace MCP Pending Changes System Design

## Boundary and grounding

This capability-specific continuation of
[workspace MCP configuration](workspace-mcp-configuration.md) owns comparison
of saved session choices with the last accepted agent setup. Agents owns that
desired/applied configuration contract, including its Customize presentation.
[Platform observability](../../platform/system-design/mcp-session-observability.md)
continues to own connection and tool evidence. No second health model is added.

Before this capability, `SessionMCPSelectionState` stored revision counters, an application
state and attachment attempt ID, but not the accepted selection set. Frontend attachment rows lacked definition
identity/revision. Names and observed connection rows cannot provide an exact
membership comparison.
An initial-open browser baseline would lose changes on reload and would make
failed reconnects look applied. Neither is a valid fallback.

| Criteria | Design |
| --- | --- |
| AC-006.15/.16 | Safe comparison and presentation |
| AC-006.17/.18, AC-005.5/.7 | Accepted snapshot and lifecycle consistency |
| AC-006.19 | Responsive presentation and verification |

## Accepted snapshot and lifecycle consistency

Extend the existing `mcp_task_session_apply_state` persistence with a nullable,
versioned accepted-setup snapshot. Do not create a historical configuration log
or store resolved configuration. Migration leaves existing records unknown;
an empty authoritative set is explicitly known, not null.

Capture descriptors from the exact effective selection resolution used for
the outbound request. Include workspace definition IDs, definition revisions,
enabled state and bounded display/runtime names, including selected definitions
excluded by policy. These descriptors represent accepted selection intent;
filtered definitions are not connected servers. Collapse duplicate scope
references by definition ID. Built-in Kandev and unidentifiable legacy/imported
servers are outside selectable catalog comparison, and remain in Connections.

Freeze session selection IDs and their desired revision together before
resolving the request, and read inherited choices/catalog descriptors
consistently. Do not hold a database transaction across provider I/O. Capture
and reuse the same resolved set for provider delivery and the safe snapshot;
never reconstruct the accepted snapshot from current selections afterward.

At successful initial start, resume/load reconfiguration, and supported normal
restart, persist that snapshot with its applied revision and attachment attempt
through the existing state update/CAS path. A failed provider call preserves
the previous accepted snapshot even if newer attachment evidence was emitted.
If R+1 was saved while R was accepted, store R's accepted snapshot and leave
R+1 pending. A stale result must not regress a newer applied baseline. Unknown
producer paths retain an unavailable comparison until they capture a successful
setup; they must not silently use an empty baseline.

No new lifecycle timing, automatic restart, reset, retry semantics, or inherited
exclusion is introduced. Preserve model/mode/runtime settings across reconnect.

## Safe comparison projection

Extend the existing session `GET/PUT .../mcp-selections` response with an optional
versioned `comparison` field within `mcp_state`. Other selection scopes retain
their existing response shape. Authenticate workspace, task and session through
the existing guards before composing any projection.

The projection identifies its saved and applied revisions and accepted attempt,
and has `status: known | unavailable`. A known comparison contains `changes`,
each with a definition ID, action (`add`, `remove`, `update`), and safe before/
after descriptors as needed. An unavailable comparison contains a stable reason
such as missing baseline or inconsistent snapshot, with no definitive actions.

Build desired descriptors from the complete saved profile/repository/task/
session union, minus [saved session exclusions](session-mcp-exclusions.md), without
revealing secrets, materializing packages, or probing endpoints. Return a comparison only when its selection revision and descriptors
were read consistently. Compare against committed positive and excluded IDs, not an unsaved addition
draft. An obsolete comparison cannot decorate newer saved state. Failed projection reads do not fail an otherwise successful saved write;
they yield unavailable comparison and retain existing application authority.

Diff by definition ID: desired-only is add; accepted-only is remove; same ID
with a changed definition revision or enabled state is update. An origin-only
change does not become a server update; removing one contributor while another
retains the server is not removal. Keep stored safe names for deleted/renamed
baseline entries, so removed rows do not expose UUIDs or disappear. A known
empty difference is legitimate even while lifecycle revisions await acknowledgment.

Bound serialized snapshots/projections to 1,024 descriptors and 256 KiB. On
overflow, retain explicit unavailable comparison rather than a partial set or
false exact count. These limits constrain explanation, not existing selections
or provider delivery. Exclude endpoints, arguments, environment/header values,
secret bindings/values and configuration hashes from this API/persistence.
Comparison is selection/definition intent, not proof of secret rotation,
executor availability, loaded tools or a successful third-party connection.

## Presentation

Extend `MCPSessionSelectionBody` using a small shared comparison presenter.
Keep the application headline and timing sentence, then render the named
changes immediately beneath them, ahead of From task setup and Additional
servers. The section is independent of picker search. Show a count only for a
known comparison, and retain all removed entries even if no picker row exists.
Render a deterministic list of compact server rows with action words/icons;
use the existing amber/blue/failure application accent, not health-green rows.

Annotate matching picker rows with Add to setup, Remove from setup, or Update
setup. Checkboxes express latest saved choice; an unchecked pending-removal
row explicitly remains part of the last accepted setup. Do not duplicate
connection badges in Customize. A failed application keeps the exact change
list and existing Retry; a failed save reverts to committed state and has a
separate save error. Saving/polling does not reorder choices or steal focus.

Applied state keeps the compact Setup up to date notice and removes change
annotations. Known zero difference while pending says that the saved choices
match the last accepted setup, with acknowledgment timing preserved. Missing
baseline says Saved choices are shown; exact changes cannot be compared yet.
No add/remove counts or per-row claims appear in that state. Loading/refresh
must preserve the existing committed selection; never show a fabricated empty
change list. All new copy uses translation keys in every shipped locale.

## Responsive contract

Entry remains the composer plug and Customize tab. Desktop retains the shipped
MCP dialog; phone/coarse pointer retains its existing drawer. The shipped MCP
explorer is the layout exemplar. The fixed header/view selector remains outside
one bounded body scroller containing timing, change list, inherited rows and
choices. This fits an occasional multi-server configuration task and keeps
summary and controls together without adding another overlay or conversation
bar. No hover dependency, horizontal overflow, nested scroll region or new
bottom action. Existing automatic saving and 44px touch targets remain.

## Verification and delivery

Store/resolver/lifecycle tests prove known empty versus unknown, accepted
snapshot durability, exact outbound revision, failed apply, overlapping saves,
scoped inheritance and bounded safe projection. Frontend tests reject stale or
malformed comparisons without losing selection state. Desktop/phone journeys
cover add/remove, reload, failure/retry, revert-to-baseline, session switch,
search hiding an available choice while changes remain visible, and long-list
scrolling. Presentation fixtures must not be claimed as live connectivity.

[Implementation plan](../../../plans/mcp-pending-changes/plan.md) separates the
authoritative projection from the visible composition. Preview delivery must
use the owned sandbox, preserve all saved choices and main :9998, and verify
the new migration before swapping only owned processes.
