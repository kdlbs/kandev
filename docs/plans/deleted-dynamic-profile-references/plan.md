---
created: 2026-11-19
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
legacy_specs: []
---

# Implementation plan: Deleted dynamic profiles no longer block their candidates

## Overview

A concrete profile that was a candidate of a dynamic profile cannot be deleted
or disabled after that dynamic profile is deleted. The reference query includes
soft-deleted dynamic parents, so the delete returns 409 with `dynamic_profiles`,
but the web conflict dialog does not render that list: the user sees an empty
conflict, or only a toast when disabling. Previously, a delete with a live
dynamic reference also returned early, so other blockers appeared only on the
next attempt. The
disable path has a related gap: when both a live dynamic profile and a utility
agent use the candidate, it reports only the dynamic profile. Confirming the
force action then disables a profile that the utility agent still needs. This
package implements
`AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.32` as described in "Candidate references
from dynamic profiles" in the system design.

## Scope

In scope: live-only reference query, best-effort removal of a deleted
candidate's routes from soft-deleted dynamic parents, aggregated delete and
disable 409 responses, and dynamic profile sections in the delete and disable
conflict dialogs. Both dialogs keep one scrollable body and a visible footer on
phone.

Out of scope: dynamic profile editing, route selection, and routes of live
dynamic profiles.

## ASCII UI preview

UI-01: Delete conflict dialog, agent profile settings, after Delete is
confirmed for a profile that a live dynamic profile routes through. Only the
dynamic profile section is new; the other sections and actions are unchanged.

```text
+------------------------------------------------------+
| Delete agent profile                                 |
| This profile is currently in use. Deleting it will   |
| affect the following:                                |
|                                                      |
| Tasks: ...                     (existing, if any)    |
| Dynamic profiles (will keep stale routes):           |
|   - Balanced selector                                |
|                                                      |
|                        [ Cancel ]  [ Delete Anyway ] |
+------------------------------------------------------+
```

UI-02: Disable conflict dialog, after saving a disabled profile that a live
dynamic profile or utility agent uses.

```text
+------------------------------------------------------+
| Disable profile                                      |
| Dynamic profiles use this profile as a routing       |
| candidate. If it is their only candidate, routing   |
| stops and requires action until replaced or enabled. |
|                                                      |
| Dynamic profiles (will keep stale routes):           |
|   - Balanced selector                                |
| Utility agents:                                       |
|   - Commit message                                    |
|                                                      |
|                      [ Cancel ]  [ Disable profile ] |
+------------------------------------------------------+
```

The warning and both reference lists scroll in one dialog body on phone. The
footer stays visible and the actions remain touch sized. Copy is localized in
every shipped catalog.

## Work orders

- [x] [Task 01: Count only live dynamic references and show them](task-01-live-dynamic-references.md)

One work order: the backend rule and the dialogs form one vertical slice with
one verification boundary.

## Verification strategy

Backend tests against the real SQLite settings store cover mixed disable
references and the unchanged enabled state. Vitest covers conflict parsing and
both reference lists. Playwright covers delete response ordering and the
disable dialog on a short phone viewport.

## Risks

- Route rows of a deleted dynamic profile are removed when their candidate is
  deleted if cleanup succeeds. A failure may leave inert routes in the
  soft-deleted parent. They cannot run, and the dynamic profile stays deleted.
- A live dynamic reference still blocks without force, so routing behavior is
  unchanged for profiles in use.
- Disabling a candidate that is the only route for a live dynamic profile stops
  routing and requires an operator action. The warning names this outcome.

## References

- [Requirement](../../specs/agents/requirements/dynamic-agent-routing.md)
- [System design](../../specs/agents/system-design/dynamic-agent-routing-02.md)
