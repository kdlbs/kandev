---
id: "05-durable-bootstrap-publication"
title: "Durable bootstrap continuation and ready publication (R5)"
status: pending
wave: 3
depends_on:
  - "04"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-007
acceptance_criteria:
  - AC-EXECUTORS-NATIVE-WORKER-007.1
  - AC-EXECUTORS-NATIVE-WORKER-007.2
  - AC-EXECUTORS-NATIVE-WORKER-007.3
  - AC-EXECUTORS-NATIVE-WORKER-007.4
  - AC-EXECUTORS-NATIVE-WORKER-007.5
  - AC-EXECUTORS-NATIVE-WORKER-007.6
  - AC-EXECUTORS-NATIVE-WORKER-007.7
  - AC-EXECUTORS-NATIVE-WORKER-007.9
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
---

# Task 05: Durable bootstrap continuation and ready publication (R5)

## Summary

Persist bootstrap nonce and authorization as secret references, resume the handshake after a restart, and add the generic guarded creating-to-ready publication under the recovery claim.

## Size

L: ~500 source, ~900 test lines.

## In scope

- Reuse `MetadataKeyBootstrapNonceSecret` (no new key); write it only for `durable_bootstrap`, keep generic resume and reuse paths from copying it, delete the secret when the record settles; record `handshake_started` before each handshake.
- Persist bootstrap nonce and authorization as secret references, resume the handshake after a restart, and add the generic guarded creating-to-ready publication under the recovery claim.

## Out of scope

- Provider-specific recovery; native-state migration.

## Acceptance

- A restart with a persisted nonce and no authorization resumes under the original operation without a second allocation.
- Publication is one idempotent guarded transaction on both dialects.

## Failing-first tests

- Repeated handshake accepted by agentctl continues to readiness; rejected as consumed ends in `bootstrap_consumed`; the nonce key is not copied onto another session's row.
- Crash before token persist: `bootstrap_consumed` cleanup, no duplicate, no ready (fails: `incomplete_bootstrap` destroy only).
- No provision call before the nonce reference is persisted (ordering test).
- Secrets absent from envelope, checkpoint, progress, logs (seeded-secret scan).
- Publication mismatch publishes nothing; repeat on a ready row is a no-op.
- Archive racing recovery: one claim; SQLite and PostgreSQL.
- Canceled recovery stays `creating` and retryable.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/task/repository/sqlite -run "PluginExecutor|Publish" -count=1)
```

## Likely files and risks

- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin_recovery.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin.go`
- `internal/task/repository/sqlite (new publish method)`
- `remote_recovery.go`
- Risk: concurrency on both dialects; add the PostgreSQL race run before marking done.
