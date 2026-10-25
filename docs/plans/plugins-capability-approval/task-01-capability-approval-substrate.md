---
id: "01-capability-approval-substrate"
title: "Capability approval substrate"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-CAPABILITY-APPROVAL-001
  - REQ-PLUGINS-CAPABILITY-APPROVAL-002
  - REQ-PLUGINS-CAPABILITY-APPROVAL-003
  - REQ-PLUGINS-CAPABILITY-APPROVAL-004
  - REQ-PLUGINS-CAPABILITY-APPROVAL-005
  - REQ-PLUGINS-CAPABILITY-APPROVAL-006
acceptance_criteria:
  - AC-PLUGINS-CAPABILITY-APPROVAL-001.1
  - AC-PLUGINS-CAPABILITY-APPROVAL-001.2
  - AC-PLUGINS-CAPABILITY-APPROVAL-001.3
  - AC-PLUGINS-CAPABILITY-APPROVAL-001.4
  - AC-PLUGINS-CAPABILITY-APPROVAL-001.5
  - AC-PLUGINS-CAPABILITY-APPROVAL-001.6
  - AC-PLUGINS-CAPABILITY-APPROVAL-002.1
  - AC-PLUGINS-CAPABILITY-APPROVAL-002.2
  - AC-PLUGINS-CAPABILITY-APPROVAL-003.1
  - AC-PLUGINS-CAPABILITY-APPROVAL-003.2
  - AC-PLUGINS-CAPABILITY-APPROVAL-003.3
  - AC-PLUGINS-CAPABILITY-APPROVAL-003.4
  - AC-PLUGINS-CAPABILITY-APPROVAL-003.5
  - AC-PLUGINS-CAPABILITY-APPROVAL-003.6
  - AC-PLUGINS-CAPABILITY-APPROVAL-003.7
  - AC-PLUGINS-CAPABILITY-APPROVAL-004.1
  - AC-PLUGINS-CAPABILITY-APPROVAL-004.2
  - AC-PLUGINS-CAPABILITY-APPROVAL-005.1
  - AC-PLUGINS-CAPABILITY-APPROVAL-005.2
  - AC-PLUGINS-CAPABILITY-APPROVAL-006.1
  - AC-PLUGINS-CAPABILITY-APPROVAL-006.2
  - AC-PLUGINS-CAPABILITY-APPROVAL-006.3
system_design:
  - ../../specs/plugins/system-design/capability-approval.md
---

# Task 01: Capability Approval Substrate

## Summary

Deliver the generic plugin capability-approval substrate: host-minted
installation identity, one current approval per `(installation_id,
workspace_id)`, append-only approval events, exact-revision admission, the
fail-closed authorization decision with typed deny reasons, uninstall
tombstones with fresh reinstall identity, the legacy `v1` fence, and the
audit receipt primitives.

## In scope

- `InstallationID` minting and persistence on the plugin store record.
- `CapabilityApproval` ledger with grant/narrow/revoke/upgrade-review events.
- Exact next-revision admission and audit-id idempotency.
- `AuthorizeCapability` fail-closed decision with stable deny vocabulary.
- Tombstones on uninstall; fresh identity on reinstall.
- `host.v2` exact-capability fence for legacy `v1` declarations.
- `ApprovalReceipt` and canonical digest primitives.
- Service and store APIs for list/get/grant/revoke.

## Out of scope

- Approval UI, coordinator surfaces, dashboards.
- Host adapter persistence of receipts.
- Legacy `v1` behavior changes beyond the fence.
- H1-H5 capabilities.

## Acceptance

- Grants replace the current row for the installation/workspace pair and are
  admitted only at the exact next revision with matching manifest digest and
  manifest-declared capabilities.
- Every mutation appends an event with before/after revisions and digests;
  history is never rewritten.
- Authorization denies missing, revoked, tombstoned, stale, human-reserved,
  unavailable, undeclared, and malformed requests with distinct typed reasons;
  foreign-workspace requests alias the missing-approval reason.
- Uninstall tombstones the identity permanently; reinstall mints a new
  identity, so no approval is inherited.
- Legacy `v1` capabilities are rejected from approval lists and denied at
  authorization as unsupported, while retaining their original runtime
  behavior.

## Verification

```bash
cd apps/backend && go build ./...
cd apps/backend && go test ./internal/plugins/...
```

## Files likely touched

- `apps/backend/internal/plugins/approval.go`
- `apps/backend/internal/plugins/approval_api.go`
- `apps/backend/internal/plugins/approval_dto.go`
- `apps/backend/internal/plugins/approval_service.go`
- `apps/backend/internal/plugins/approval_store.go`
- `apps/backend/internal/plugins/registry.go`
- `apps/backend/internal/plugins/service.go`
- `apps/backend/internal/plugins/service_install.go`
- `apps/backend/internal/plugins/service_sync.go`
- `apps/backend/internal/plugins/store/store.go`
- `apps/backend/internal/plugins/service_test.go`
- `apps/backend/internal/plugins/store/store_test.go`

## Dependencies

None. This is the substrate initiative; no prior plugin capability wave is
required.

## Risks

- Cross-workspace denial must not leak existence: the foreign-workspace path
  intentionally reuses the missing-approval deny reason.
- Ledger decode failures must fail closed at lookup rather than default to
  allowing.

## Parallelism

`sequential` — single vertical work order owning the approval files.

## Inputs

- Requirements `REQ-PLUGINS-CAPABILITY-APPROVAL-001` through `-006`.
- System design `docs/specs/plugins/system-design/capability-approval.md`.
- Plan `plan.md`.
- Existing manifest capability derivation (`ManifestCapabilityIDs`) and store
  record patterns in `apps/backend/internal/plugins`.

## Results

Implemented the full substrate on the H6 head: installation id minting in the
install path, the approval ledger with events/tombstones/idempotency, the
fail-closed decision path with the typed deny vocabulary, reinstall identity
mints, the `v1` fence, and receipt primitives. Verified with:

```bash
cd apps/backend && go test ./internal/plugins/...
```

Result: all 10 plugin packages pass, including the approval store, service,
API, DTO, and query tests, on the H6 head and re-verified after the merge with
current main.
