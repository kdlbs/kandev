---
created: 2026-09-07
status: done
requirements:
  - REQ-PLUGINS-CAPABILITY-APPROVAL-001
  - REQ-PLUGINS-CAPABILITY-APPROVAL-002
  - REQ-PLUGINS-CAPABILITY-APPROVAL-003
  - REQ-PLUGINS-CAPABILITY-APPROVAL-004
  - REQ-PLUGINS-CAPABILITY-APPROVAL-005
  - REQ-PLUGINS-CAPABILITY-APPROVAL-006
system_design:
  - ../../specs/plugins/system-design/capability-approval.md
legacy_specs: []
---

# Implementation Plan: Generic Plugin Capability Approval and Audit (H6)

## Overview

Add the generic plugin capability-approval substrate as one vertical work
order: host-minted installation identity, the workspace-scoped approval ledger
with exact-revision admission, the append-only event trail, the fail-closed
authorization decision with typed deny reasons, uninstall tombstones with
reinstall identity, the legacy `v1` fence, and the audit receipt primitives.
Scope is the substrate only: no approval UI, no coordinator surface, and no
host receipt persistence.

## Scope

### In scope

- `installation_id` minting and persistence on the plugin store record.
- One current `CapabilityApproval` per `(installation_id, workspace_id)`.
- Grant, narrow, revoke, and upgrade-review events in an append-only ledger.
- Exact-revision admission and audit-id idempotency.
- Fail-closed `AuthorizeCapability` with stable typed deny reasons.
- Uninstall tombstone and reinstall identity mints.
- The `host.v2` exact-capability fence for legacy `v1` declarations.
- `ApprovalReceipt` and canonical digest primitives.

### Out of scope

- Approval or review UI, coordinator surfaces, dashboards.
- Host adapter persistence of audit or read/write receipts.
- Changes to legacy `v1` runtime behavior beyond fencing it from approval
  authority.
- H1-H5 plugin capabilities; this initiative is substrate only.

### No UI changes

This package does not create or change rendered UI, so no ASCII UI preview is
included.

## Technical approach

The ledger (`approvals.json` under the plugins directory) owns approvals,
events, tombstones, and idempotency inputs. One current approval per
installation/workspace pair is stored keyed by the pair with exact next
revision admission; every mutation appends an event carrying before/after
revisions and digests. `installation_id` is minted at install time and
attached to the store record; reinstall mints fresh identity and tombstones
block the old identity permanently.

The authorization path (`approval_service.go`) is a pure decision function
that evaluates in a fixed order — malformed request, human-reserved, missing
approval (including the foreign-workspace alias), revoked or tombstoned,
stale revision, manifest intersection, approved list — and returns a typed
decision carrying the audit identity and a bounded receipt. Legacy `v1`
declarations keep their behavior but are fenced from approval admission and
authorization.

## Tests

- `approval_test.go`, `approval_store_test.go`, `approval_service_test.go`:
  canonicalization, exact-revision admission, idempotency, tombstone, and
  reinstall-identity coverage.
- `approval_api_test.go`, `approval_query_test.go`: API list/get, grant, and
  revoke flows including conflict semantics.
- `approval_dto_test.go`: DTO shape and receipt bounds.
- Store install/sync tests: `installation_id` minting and persistence.

## E2E tests

No new E2E is required: the substrate exposes no UI surface. Go unit and
integration tests exercise the full ledger and decision matrix; a browser
duplicate of that matrix would add cost without observable UI coverage.

## Work orders

- [x] [Task 01: Capability approval substrate](task-01-capability-approval-substrate.md)

## Verification results

- `cd apps/backend && go build ./...` passed on the H6 head and was
  re-verified on the merged head.
- `cd apps/backend && go test ./internal/plugins/...` passed in 10 packages
  on the H6 head and was re-verified on the merged head.

## Risks

- Ledger file corruption must not grant authority: unknown fields decode
  forward-compatibly and missing maps initialize empty, but any decode failure
  must fail closed at lookup.
- A future policy revision must not silently reinterpret stored approvals:
  the per-row human policy version is the stability point.
