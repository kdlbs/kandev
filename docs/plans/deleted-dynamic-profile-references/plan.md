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
conflict, or only a toast when disabling, with nothing to resolve. A delete with
a live dynamic reference also returns early, so other blockers appear only on
the next attempt. This package implements
`AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.32` as described in "Candidate references
from dynamic profiles" in the system design.

## Scope

In scope: live-only reference query, removal of a deleted candidate's routes
from soft-deleted dynamic parents, one aggregated delete 409, and the dynamic
profile sections in the delete and disable conflict dialogs.

Out of scope: dynamic profile editing, route selection, the disable path's
blocker order, and routes of live dynamic profiles.

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
dynamic profile routes through.

```text
+------------------------------------------------------+
| Disable profile                                      |
| Dynamic profiles use this profile as a routing       |
| candidate. While disabled, these dynamic profiles    |
| skip it and route to their other candidates ...      |
|                                                      |
| Dynamic profiles (will keep stale routes):           |
|   - Balanced selector                                |
|                                                      |
|                      [ Cancel ]  [ Disable profile ] |
+------------------------------------------------------+
```

Both dialogs keep the existing alert dialog layout on phone; the list scrolls
inside the dialog body. Copy is localized in every shipped catalog.

## Work orders

- [x] [Task 01: Count only live dynamic references and show them](task-01-live-dynamic-references.md)

One work order: the backend rule and the dialogs form one vertical slice with
one verification boundary.

## Verification strategy

Backend tests against the real SQLite settings store, Vitest for the server
action, conflict parsing, and both dialogs, and a Playwright scenario that
deletes a candidate of a deleted dynamic profile and shows a live one in the
conflict dialog.

## Risks

- Route rows of a deleted dynamic profile are removed when their candidate is
  deleted. They could not run, and the dynamic profile itself stays deleted.
- A live dynamic reference still blocks without force, so routing behavior is
  unchanged for profiles in use.

## References

- [Requirement](../../specs/agents/requirements/dynamic-agent-routing.md)
- [System design](../../specs/agents/system-design/dynamic-agent-routing-02.md)
