---
id: "01-grant-ledger"
title: "Persist and authorize provider grants"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001
acceptance_criteria:
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.1
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.2
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.7
system_design:
  - ../../specs/integrations/system-design/provider-session-access.md
---

# Task 01: Persist and authorize provider grants

## Summary

Add a durable, administrator-managed grant whose generation, expiry and
revocation are enforced for an exact plugin installation and managed session.
Record redacted grant and lease-admission audit without minting a credential.

## In scope

- Replayable SQLite/PostgreSQL schema, grant/lease/audit models and store.
- Authenticated grant create/list/revoke with workspace authorization.
- Exact installed-plugin, H6 approval, managed-conversation session, target
  task/repository/link and provider-connection admission.

## Out of scope

Provider token redemption, provider API mutation, and plugin adapter.

## Acceptance

- A cross-workspace, wrong installation/session, terminal session, stale
  approval or grant generation request is denied without a lease.
- Replacing a grant atomically revokes its predecessor and preserves a
  monotonic generation; same-key replay does not create a second lease.
- Durable rows and audit contain no token, header, or raw idempotency key.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/provideraccess ./internal/plugins ./internal/db)
(cd apps/backend && go test -count=1 ./internal/backendapp)
```

## Files likely touched

- `apps/backend/internal/provideraccess/`
- `apps/backend/internal/db/schema.go`
- `apps/backend/internal/plugins/approval_service.go`
- `apps/backend/internal/backendapp/`

## Dependencies

None.

## Risks

The managed conversation's task metadata and session lookup must be resolved
through service/repository APIs, never trusted from plugin request fields.

## Parallelism

`sequential`

## Inputs

- `docs/specs/integrations/system-design/provider-session-access.md`
- `apps/backend/internal/task/service/agent_conversations.go`
- `apps/backend/internal/plugins/approval_service.go`

## Results

Grant generation replacement and exact-workspace revocation storage are
implemented locally and have focused race-enabled tests. Administrator API,
lease/audit ledger, Host admission and PostgreSQL coverage remain pending.
