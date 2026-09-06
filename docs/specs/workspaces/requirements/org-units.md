---
status: draft
system: workspaces
created: 2026-08-24
owners:
  - tbd
---

# Organization Unit Requirements

## Overview

An organization needs its own shape inside Kandev. A company has departments,
departments have teams, and a team owns several workspaces. Without that shape,
sharing a workspace means naming every person on it, and the list has to be
maintained again for every new workspace and every joiner.

This capability replaces per-workspace visibility with a tree of organization
units that carries both the workspaces and the people. Reach follows the tree:
being a member of a unit reaches everything beneath it. The workspace system
owns this contract because a unit is where a workspace lives, and placement
determines reach.

The [auth system](../../auth/requirements/roles-and-scopes.md) still owns what a
role may do once reach is established. This document owns only which workspaces
a person reaches.

## Terminology

- **Unit:** A node in an organization's tree. It holds child units, workspaces,
  and members. A department and a team are both units; they differ only in
  depth.
- **Root unit:** The single unit at the top of an organization. Every other unit
  descends from it. It takes members like any other unit; belonging to the
  organization is not membership of it. If it were, every unit beneath it would
  inherit the whole organization and no department could be separate from
  another.
- **Personal unit:** A unit belonging to exactly one user, which takes no
  members.
- **Reach:** Whether a user can see a workspace at all, before any question of
  what they may do in it.
- **Direct grant:** A role recorded against one workspace for one user, rather
  than inherited from a unit.

## Requirements

### REQ-WORKSPACES-ORG-UNITS-001: Organization unit tree

**Intent:** An organization's structure must exist as data, so that access can
follow it instead of being restated on every workspace.

**User story:** As an organization administrator, I want to model my departments
and teams in Kandev, so that access follows the structure I already have.

#### Acceptance criteria

- **AC-WORKSPACES-ORG-UNITS-001.1:** When an organization is created, the system
  shall create exactly one root unit for it.
- **AC-WORKSPACES-ORG-UNITS-001.2:** The system shall allow a unit to contain
  child units to any depth, and shall give every unit except the root exactly
  one parent.
- **AC-WORKSPACES-ORG-UNITS-001.3:** The system shall place every workspace in
  exactly one unit.
- **AC-WORKSPACES-ORG-UNITS-001.4:** When a request would make a unit its own
  ancestor, the system shall refuse the request.
- **AC-WORKSPACES-ORG-UNITS-001.5:** When a request would delete a unit that
  still holds child units or workspaces, the system shall refuse the request and
  name what remains.
- **AC-WORKSPACES-ORG-UNITS-001.6:** When a request would delete or move the
  root unit, the system shall refuse the request.
- **AC-WORKSPACES-ORG-UNITS-001.7:** The system shall keep every unit in exactly
  one organization, and shall refuse a parent in a different organization.
- **AC-WORKSPACES-ORG-UNITS-001.8:** When a caller without `unit.manage` on the
  target unit requests a create, rename, move, or delete, the system shall
  refuse the request.

### REQ-WORKSPACES-ORG-UNITS-002: Unit membership inherits downward

**Intent:** Membership recorded once on a department must reach every workspace
under it, including ones created later, so that joining a team is a single act.

**User story:** As a team lead, I want to add someone to my team once, so that
they reach every board the team owns without further bookkeeping.

#### Acceptance criteria

- **AC-WORKSPACES-ORG-UNITS-002.1:** The system shall record a unit membership
  as a user and a workspace role of `owner`, `collaborator`, or `viewer`.
- **AC-WORKSPACES-ORG-UNITS-002.2:** When a user is a member of a unit, the
  system shall give that user reach at that role to every workspace in that unit
  and in all of its descendants, with no per-workspace record.
- **AC-WORKSPACES-ORG-UNITS-002.3:** When a workspace is created under a unit,
  the system shall give the unit's existing members reach to it immediately.
- **AC-WORKSPACES-ORG-UNITS-002.4:** The system shall treat the root unit as an
  ordinary unit: belonging to an organization grants no membership of it, and
  no reach follows from an organization role alone. Sharing something with
  everyone means adding them to the root, exactly as with any other unit.
- **AC-WORKSPACES-ORG-UNITS-002.5:** When a unit membership is removed, the
  system shall withdraw reach to every workspace the user reached only through
  that membership, and shall disconnect their live subscriptions to those
  workspaces.
- **AC-WORKSPACES-ORG-UNITS-002.6:** When a caller without `member.manage` on
  the unit requests a membership change, the system shall refuse the request.

### REQ-WORKSPACES-ORG-UNITS-003: Personal units

**Intent:** Work that belongs to one person needs somewhere to live that no
inherited membership can reach, without a per-workspace privacy flag.

#### Acceptance criteria

- **AC-WORKSPACES-ORG-UNITS-003.1:** When a user account is created, the system
  shall create a personal unit owned by that user.
- **AC-WORKSPACES-ORG-UNITS-003.2:** The system shall refuse any request to add
  a member to a personal unit.
- **AC-WORKSPACES-ORG-UNITS-003.3:** When a workspace is in a personal unit, the
  system shall give reach only to the unit's owner and to any user holding a
  direct grant on that workspace.
- **AC-WORKSPACES-ORG-UNITS-003.4:** The system shall not give an organization
  administrator or an instance operator reach into a personal unit by role
  alone.
- **AC-WORKSPACES-ORG-UNITS-003.5:** The system shall refuse a request to move
  or delete a personal unit while its owner's account exists.
- **AC-WORKSPACES-ORG-UNITS-003.6:** When no unit is named at workspace
  creation, the system shall place the workspace in the caller's personal unit.

### REQ-WORKSPACES-ORG-UNITS-004: Effective role by union

**Intent:** With roles arriving from several places at once, the rule that
combines them has to be one sentence a person can hold in their head.

#### Acceptance criteria

- **AC-WORKSPACES-ORG-UNITS-004.1:** The system shall resolve a user's effective
  role on a workspace as the highest of the roles granted by the root unit, by
  every ancestor unit of the workspace, and by any direct grant on the
  workspace.
- **AC-WORKSPACES-ORG-UNITS-004.2:** The system shall provide no mechanism that
  lowers a role inherited from an ancestor unit.
- **AC-WORKSPACES-ORG-UNITS-004.3:** When a user's organization differs from the
  workspace's organization, the system shall give no reach, and shall decide
  this before evaluating any role.
- **AC-WORKSPACES-ORG-UNITS-004.4:** When a user account is disabled, the system
  shall give that account no reach and no role, while retaining its membership
  records.
- **AC-WORKSPACES-ORG-UNITS-004.5:** When a user has no reach to a workspace,
  the system shall answer requests naming that workspace as not found rather
  than as forbidden.

### REQ-WORKSPACES-ORG-UNITS-005: Workspace placement

**Intent:** Moving a workspace between teams is a normal event, and it is the
only way to narrow or widen who reaches it.

#### Acceptance criteria

- **AC-WORKSPACES-ORG-UNITS-005.1:** When a caller creates a workspace and names
  a unit, the system shall place it there if the caller holds `unit.manage` on
  that unit, and shall otherwise refuse.
- **AC-WORKSPACES-ORG-UNITS-005.2:** When a caller moves a workspace, the system
  shall require `workspace.manage` on the workspace and `unit.manage` on the
  destination unit.
- **AC-WORKSPACES-ORG-UNITS-005.3:** When a workspace moves, the system shall
  apply the destination's reach immediately, and shall disconnect live
  subscriptions held by users who no longer reach it.
- **AC-WORKSPACES-ORG-UNITS-005.4:** The system shall refuse a move whose
  destination unit is in another organization.
- **AC-WORKSPACES-ORG-UNITS-005.5:** When a workspace moves, the system shall
  retain its direct grants.

## Out of scope

- **Standalone groups.** A set of people that ignores the tree, such as every
  site reliability engineer across three departments, is not modelled. A unit
  already answers who is in a team.
- **Negative grants.** There is no deny rule and no way to subtract a role.
  Narrowing is achieved by placement.
- **Custom roles.** The role vocabulary and the scopes each role holds stay with
  the [auth system](../../auth/requirements/roles-and-scopes.md), which fixes
  them.
- **Inherited configuration.** Executors, secrets, agent profiles, and default
  workflows are not inherited through the tree by this capability.
- **External directory sync.** Units and memberships are not populated from
  LDAP, SCIM, or an identity provider.
- **Cross-organization sharing.** The tenant boundary is owned by
  [multi-tenancy](../../multi-tenancy/spec.md) and remains absolute.
