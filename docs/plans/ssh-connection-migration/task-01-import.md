---
id: "01-import"
title: "Import saved SSH executors"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-EXECUTORS-SSH-MIGRATION-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-MIGRATION-001.1
  - AC-EXECUTORS-SSH-MIGRATION-001.2
  - AC-EXECUTORS-SSH-MIGRATION-001.3
  - AC-EXECUTORS-SSH-MIGRATION-001.4
  - AC-EXECUTORS-SSH-MIGRATION-001.7
system_design:
  - ../../specs/executors/system-design/ssh-connection-migration.md
---

# Import saved SSH executors

Reuse the SSH resolver and transactional storage in `apps/backend/internal/sshconnections`. Replace manual adoption drafts with server-owned import records. Wire offline conversion into backend upgrade before SSH admission. Implement the import-only binding transaction for complete and incomplete routes. Preserve default-agent semantics and destination and bastion trust provenance. Avoid raw connection config in logs. No network calls or enrollment in this work order.

1. A database with several profiles under one executor creates one bound graph. Another executor with the same host gets a distinct import. An existing managed binding is unchanged.
2. Reopen an interrupted import and race two importers on SQLite and PostgreSQL. Assert stable record IDs and a single commit per executor. Simulate credentials and config changing under the service account.
3. Missing agent socket, missing alias, unsupported jump and malformed mixed config each produce a safe reason. An explicitly disabled installation converts local data without dialing or accessing private key material. Offline hosts do not block conversion. Prove that ordinary create/update callers cannot use the import-only binding path.

## Verification

Run each line from the repository root, under the systemd limit in the [plan](plan.md). Add the named migration tests during implementation.

```bash
cd apps/backend && go test ./internal/sshconnections -run "Migrat|Adopt"
```

## Results

The migration tests passed against SQLite and a disposable PostgreSQL 17 server with the Go race detector. The upgrade and rollback fixture preserved both profile IDs and bound both profiles to one connection. The fixture also verified restart with the feature explicitly disabled.
