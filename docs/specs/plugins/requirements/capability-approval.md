---
status: active
system: plugins
created: 2026-09-07
owners:
  - kandev
---

# Plugin Capability Approval and Audit Requirements

## Overview

Installed plugins request host capabilities. Kandev must gate every capability
request behind an explicit, workspace-scoped human approval, record an
append-only audit trail of approval changes, and evaluate requests against the
intersection of what the installed manifest declares, what the human approved,
and what human policy reserves. This document defines that substrate for
`host.v2` capabilities; it does not define approval UI, coordinator surfaces,
or higher-level plugin features built on it.

## Terminology

- **Installation:** One installed plugin release, identified by a host-minted
  opaque `installation_id`. The identity is never derived from the plugin id,
  package digest, workspace, or install path.
- **Approval:** One current capability approval row per
  `(installation_id, workspace_id)` pair, associating an exact manifest digest
  with a canonical list of approved capability ids.
- **Effective authority:** The capability set obtained by intersecting the
  installed manifest declaration, the current approval, and the human deny
  policy. A capability is allowed only when all three admit it.
- **Human deny policy:** The immutable set of human-reserved capabilities
  (merge, deploy, release, history rewrite, cross-workspace access, and
  secret-scope expansion) that no approval can ever grant.
- **Tombstone:** The uninstall record that permanently blocks approvals for an
  `installation_id` so a reinstall mints a new identity.

## Requirements

### REQ-PLUGINS-CAPABILITY-APPROVAL-001: Workspace-scoped capability approvals

**Intent:** Give each workspace explicit, human-controlled authority over the
capabilities an installed plugin may exercise inside that workspace, with one
current approval per installation and workspace.

**User story:** As a workspace operator, I want to approve exactly the
capabilities a plugin may use in my workspace, so that plugin authority is
explicit, auditable, and never broader than what the installed manifest
declares.

#### Acceptance criteria

- **AC-PLUGINS-CAPABILITY-APPROVAL-001.1:** When a plugin installation is
  recorded, the system shall assign it a host-minted opaque `installation_id`
  that is not derived from the plugin id, package digest, workspace, or install
  path.
- **AC-PLUGINS-CAPABILITY-APPROVAL-001.2:** The system shall keep at most one
  current approval per `(installation_id, workspace_id)` pair, and a grant on an
  existing pair shall replace the previous current approval.
- **AC-PLUGINS-CAPABILITY-APPROVAL-001.3:** When an approval is recorded, the
  system shall reject it unless its manifest digest matches the installed
  manifest and every requested capability id is declared by that manifest.
- **AC-PLUGINS-CAPABILITY-APPROVAL-001.4:** When an approval is recorded, the
  system shall reject any capability id that is human-reserved, malformed, or
  not an exact `host.v2` capability.
- **AC-PLUGINS-CAPABILITY-APPROVAL-001.5:** When a revision differs from the
  current approval revision, the system shall reject the write with a revision
  conflict and leave the stored approval unchanged.
- **AC-PLUGINS-CAPABILITY-APPROVAL-001.6:** When a grant reuses an audit id with
  different inputs, the system shall reject it without changing stored state.

### REQ-PLUGINS-CAPABILITY-APPROVAL-002: Append-only approval events

**Intent:** Preserve a complete, ordered audit trail of every approval change
so authority changes can be reviewed after the fact.

**User story:** As an auditor, I want every grant, narrow, revoke, and
upgrade-review decision recorded with before/after revisions and digests, so
that I can reconstruct how plugin authority changed over time.

#### Acceptance criteria

- **AC-PLUGINS-CAPABILITY-APPROVAL-002.1:** When the system changes an
  approval, it shall append an event recording the installation, workspace,
  before and after revisions and digests, actor, reason, event type, and an
  observed timestamp, without rewriting or removing earlier events.
- **AC-PLUGINS-CAPABILITY-APPROVAL-002.2:** The system shall support the event
  types `grant`, `narrow`, `revoke`, and `upgrade_review`.

### REQ-PLUGINS-CAPABILITY-APPROVAL-003: Fail-closed capability authorization

**Intent:** Evaluate every capability request against the manifest ∩ approval
∩ human deny-policy intersection, denying unless all three admit the request.

**User story:** As a workspace operator, I want unauthorized capability requests
to fail closed with a stable machine-readable deny reason, so that stale,
revoked, cross-workspace, or undeclared requests can never succeed and can be
diagnosed reliably.

#### Acceptance criteria

- **AC-PLUGINS-CAPABILITY-APPROVAL-003.1:** When a capability request arrives
  whose approval is missing, revoked, or tombstoned, the system shall deny it
  with the corresponding stable deny reason and no side effects.
- **AC-PLUGINS-CAPABILITY-APPROVAL-003.2:** When a capability request names a
  revision different from the current approval revision, the system shall deny
  it as stale.
- **AC-PLUGINS-CAPABILITY-APPROVAL-003.3:** When a capability request comes
  from an installation with no approval in the requested workspace, the system
  shall deny it with the same deny reason used for missing approvals, without
  disclosing that the installation exists in another workspace.
- **AC-PLUGINS-CAPABILITY-APPROVAL-003.4:** When a capability id is
  human-reserved, the system shall deny it with the human-reserved deny reason.
- **AC-PLUGINS-CAPABILITY-APPROVAL-003.5:** When the installed manifest no
  longer declares the requested capability, or the approval digest no longer
  matches the installed manifest, the system shall deny the request as
  unavailable.
- **AC-PLUGINS-CAPABILITY-APPROVAL-003.6:** When a request is structurally
  malformed (unbounded or NUL-bearing identifiers, invalid digests, or an
  unsupported capability form), the system shall deny it as malformed before
  any approval lookup.
- **AC-PLUGINS-CAPABILITY-APPROVAL-003.7:** When a request passes every gate,
  the system shall allow it and attach the audit identity and a receipt
  recording the bounded decision metadata.

### REQ-PLUGINS-CAPABILITY-APPROVAL-004: Uninstall tombstones and reinstall identity

**Intent:** Make uninstalled plugin authority permanently inert and force
reinstalls to start from a fresh installation identity and fresh approvals.

**User story:** As an operator, I want an uninstalled plugin's approvals to be
permanently blocked and a reinstalled copy to require new approvals, so that
stale authority can never be silently inherited.

#### Acceptance criteria

- **AC-PLUGINS-CAPABILITY-APPROVAL-004.1:** When a plugin is uninstalled, the
  system shall tombstone the installation identity and deny every later
  capability request for it.
- **AC-PLUGINS-CAPABILITY-APPROVAL-004.2:** When a plugin is reinstalled, the
  system shall mint a new `installation_id` so no prior approval can attach to
  the new installation.

### REQ-PLUGINS-CAPABILITY-APPROVAL-005: Legacy declaration fence

**Intent:** Keep legacy `v1` capability declarations from being granted or
exercised through the H6 approval substrate while preserving their original
runtime behavior.

**User story:** As a maintainer, I want legacy v1 capabilities fenced out of
the approval substrate, so that only explicit `host.v2` capabilities can be
approved and evaluated.

#### Acceptance criteria

- **AC-PLUGINS-CAPABILITY-APPROVAL-005.1:** When a capability id is not an
  exact `host.v2` capability, the system shall reject it from approval lists
  and deny it at authorization time as unsupported.
- **AC-PLUGINS-CAPABILITY-APPROVAL-005.2:** Legacy `v1` declarations shall
  retain their original runtime behavior without gaining H6 approval authority.

### REQ-PLUGINS-CAPABILITY-APPROVAL-006: Generic audit receipts

**Intent:** Give every authorization decision a bounded, deterministic receipt
metadata shape that a future host adapter can persist without carrying
authority.

**User story:** As a host adapter author, I want a typed, bounded receipt on
every decision, so that audit persistence can be added without redefining the
decision contract.

#### Acceptance criteria

- **AC-PLUGINS-CAPABILITY-APPROVAL-006.1:** When the system produces a
  decision, the decision shall carry a receipt with bounded installation,
  workspace, revision, capability, request and method digests, audit id, and
  result values.
- **AC-PLUGINS-CAPABILITY-APPROVAL-006.2:** The receipt digests shall be
  deterministic canonical digests of their normalized inputs.
- **AC-PLUGINS-CAPABILITY-APPROVAL-006.3:** When any receipt field would
  exceed its bound, the system shall truncate or redact it so the receipt
  remains bounded.

## Exclusions

- No approval or review UI (settings pages, coordinator surfaces, or
  dashboards) is in scope for this substrate.
- No host adapter persistence of receipts is required; the receipt shape only
  defines the metadata contract.
- No changes to legacy `v1` runtime behavior are made beyond fencing it from
  approval authority.
- No cross-workspace disclosure surface exists: foreign-workspace requests are
  indistinguishable from missing approvals.
