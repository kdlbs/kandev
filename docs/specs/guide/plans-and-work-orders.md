# Plans and Work Orders

## Purpose

Plans and work orders are delivery records. They describe how a change moves
the current system toward its requirements and system design.

They are not living product specifications.

## Plan

`docs/plans/<initiative>/plan.md` is the work-package manifest. It contains the
delivery overview, dependency order, risks, verification strategy, and links to
each work order.

A plan references the applicable requirement documents and system designs. It
does not copy their content.

## Work order

Each `task-<NN>-<slug>.md` file is one work order. A work order contains:

- A short outcome summary.
- In-scope responsibilities.
- Explicit exclusions.
- Applicable `REQ-*` and `AC-*` identifiers.
- Applicable system-design paths.
- One to three implementation acceptance conditions.
- Exact verification commands.
- Likely files and dependencies.
- Results after implementation.

Use a separate work order when work has an independent result or verification
boundary. Do not split work only by backend and frontend layers when a vertical
slice can remain functional.

## Verification

Every work order owns the verification that proves its implementation result.
Use unit, integration, or end-to-end tests at the appropriate boundary.

A user-facing requirement needs end-to-end evidence somewhere in the work
package. A low-level work order does not need an artificial browser test.

Reference acceptance-criterion IDs from tests when that reference improves
traceability.

