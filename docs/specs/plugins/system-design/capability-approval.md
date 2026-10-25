---
status: current
system: plugins
requirements:
  - REQ-PLUGINS-CAPABILITY-APPROVAL-001
  - REQ-PLUGINS-CAPABILITY-APPROVAL-002
  - REQ-PLUGINS-CAPABILITY-APPROVAL-003
  - REQ-PLUGINS-CAPABILITY-APPROVAL-004
  - REQ-PLUGINS-CAPABILITY-APPROVAL-005
  - REQ-PLUGINS-CAPABILITY-APPROVAL-006
created: 2026-09-07
owners:
  - kandev
---

# Plugin Capability Approval and Audit System Design

## Purpose and boundaries

This design describes the generic plugin capability-approval substrate in
`apps/backend/internal/plugins`: the host-minted installation identity, the
workspace-scoped approval ledger, the append-only event trail, the
fail-closed authorization decision, uninstall tombstones, the legacy `v1`
fence, and the audit receipt primitives. Higher-level surfaces (approval UI,
coordinator flows, host receipt persistence) consume this substrate without
being part of it.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-CAPABILITY-APPROVAL-001` | [Identity and approval ledger](#identity-and-approval-ledger) |
| `REQ-PLUGINS-CAPABILITY-APPROVAL-002` | [Append-only event trail](#append-only-event-trail) |
| `REQ-PLUGINS-CAPABILITY-APPROVAL-003` | [Fail-closed authorization](#fail-closed-authorization) |
| `REQ-PLUGINS-CAPABILITY-APPROVAL-004` | [Tombstones and reinstall identity](#tombstones-and-reinstall-identity) |
| `REQ-PLUGINS-CAPABILITY-APPROVAL-005` | [Legacy v1 fence](#legacy-v1-fence) |
| `REQ-PLUGINS-CAPABILITY-APPROVAL-006` | [Audit receipts](#audit-receipts) |

## Identity and approval ledger

`store.Record.InstallationID` (`apps/backend/internal/plugins/store/store.go`)
is the host-minted opaque principal for an installed plugin release. It is
minted at install time and persisted with the record; it is never derived from
the plugin id, package digest, workspace, or install path, so a reinstall
necessarily produces a different identity.

`CapabilityApproval` (`apps/backend/internal/plugins/approval_store.go`) is the
current approval row: `(installation_id, workspace_id)` is the ledger key, and
at most one current row exists per pair. A grant replaces the previous
current row for its pair. The row stores the exact `manifest_digest` the
approval was granted against, the canonically sorted and deduplicated
`capability_ids`, the `active`/`revoked` state, the human actor, the immutable
human policy version, and the audit revision. `HumanPolicyVersionImmutable`
records that all current approvals are granted under the one unversioned
human deny policy.

Approval writes are admitted only at the exact next revision. A write naming
any other revision fails with `ErrApprovalRevisionConflict` and leaves stored
state unchanged (`revokeIfRevision` in `approval_store.go`). Reused audit ids
are idempotent only for byte-identical inputs; a different input on the same
audit id fails without changing stored state.

`CanonicalCapabilityList` (`apps/backend/internal/plugins/approval.go`) is the
single canonicalizer for capability lists: trim, bounded length, no NUL,
deduplicate, sort. It rejects human-reserved capabilities and any id that is
not an exact `host.v2` capability. `ManifestCapabilityIDs` derives the exact
`host.v2.read:<resource>` / `host.v2.write:<resource>` capability set from an
installed manifest, and `ManifestCapabilityDigest` derives its canonical
digest. `Service.validateApprovalManifest` rejects a grant whose manifest
digest does not match the installed manifest or that names capabilities the
manifest does not declare: the approval can never be broader than the
manifest.

Ledger state persists as `approvals.json` under the plugins directory through
the `approvalLedger` type (load/migrate semantics in `approval_store.go`).

## Append-only event trail

`CapabilityApprovalEvent` records every approval change after commit:
installation, workspace, before/after revisions and digests, actor, reason, the
`CapabilityApprovalEventType` (`grant`, `narrow`, `revoke`,
`upgrade_review`), and the observed timestamp. Events are appended to the
ledger; grants, narrows, revokes, and upgrade reviews never rewrite or remove
earlier events. The API surface (`apps/backend/internal/plugins/approval_api.go`)
exposes list/get so future audit consumers can replay authority history.

## Fail-closed authorization

`Service.authorizePluginCapability` (`approval_service.go`) is the single
decision path for a capability request. It evaluates in a fixed order and
returns a typed `ApprovalDecision` with a stable `ApprovalDenyReason`:

1. Structural validation (`malformedAuthorizationRequestReason`): bounded
   identifiers and digests, NUL-free, and an exact capability form. A
   malformed request denies as `malformed_request` (or
   `unsupported_capability` for a non-`host.v2` form) before any lookup.
2. Human-reserved capability check denies as `human_reserved`.
3. Ledger lookup. A missing row denies as `missing_capability_approval`; an
   unavailable ledger denies as `unavailable_capability`. A foreign workspace
   intentionally reuses the missing-approval reason: a more specific
   cross-workspace reason would disclose that the installation exists in
   another workspace.
4. Revoked state or a non-nil tombstone denies as `capability_revoked`.
5. A requested revision different from the current row denies as
   `stale_capability_revision`.
6. Manifest intersection (`manifestIntersectionDenyReason`): the installed
   manifest must still declare the capability under the digest the approval
   was granted against, otherwise `unavailable_capability`.
7. The capability must appear in the approved list, otherwise
   `undeclared_capability`. Otherwise the decision is allowed and carries the
   audit identity.

Every denial is side-effect free: no Host operation runs inside the approval
layer. The deny vocabulary is typed (`ApprovalDenyReason` constants in
`approval.go`) so callers can match reasons machine-readably.

## Tombstones and reinstall identity

`Service.approvalTombstoneInstallation` writes a tombstone for an
installation identity at uninstall time. A tombstoned identity denies all
later approvals and requests (`ErrApprovalInstallationTombstoned`,
`capability_revoked`), permanently. Because the reinstall mints a new
`installation_id`, no prior approval can attach to the new installation:
authority cannot be silently inherited across an uninstall/reinstall cycle.

## Legacy v1 fence

`isExactHostV2Capability` gates both grant admission and authorization. A
capability id that is not an exact `host.v2` capability is rejected from
approval lists (`CanonicalCapabilityList`) and denied at authorization time as
`unsupported_capability`. Legacy `v1` manifest declarations keep their original
runtime behavior but cannot be granted or exercised through this substrate.

## Audit receipts

`ApprovalReceipt` (`approval.go`) is the bounded, deterministic metadata a
future host adapter can persist on read or write receipts without carrying
authority: installation, workspace, revision, capability id, request and
method digests, audit id, result, and observed time. `CanonicalApprovalDigest`
builds stable digest identities from trimmed, NUL-joined inputs. Receipt
fields pass through the same safe-bounds helpers as the decision inputs, so a
hostile identifier is truncated or redacted rather than propagated: the
receipt is always bounded and cannot be coerced into authority.

## Persistence and compatibility

The approval ledger file carries approvals, events, tombstones, and idempotency
inputs with forward-compatible JSON decoding (missing maps initialize empty).
Optional fields on `store.Record` (installation id) decode on older records
without migration; a missing installation identity simply has no approvals.
The substrate admits only `HumanPolicyVersionImmutable` today; the stored
per-row policy version is the stable extension point for a future policy
revision.

## Observability

Decisions are pure typed results; the audit id and receipt give future
adapters the exact metadata to persist. Events themselves are the observable
authority history; there is no separate metric emission in this substrate.
