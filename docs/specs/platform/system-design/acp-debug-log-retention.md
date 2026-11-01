---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-ACP-DEBUG-LOG-RETENTION-001
  - REQ-PLATFORM-ACP-DEBUG-LOG-RETENTION-002
---

# ACP Debug Log Retention System Design

## Purpose and boundaries

`agentctl/server/adapter/transport/shared/acplog.go` owns log writers and
retention. `agentctl/server/config` and `agentctl/server/utility` own the child
process environment. The diagnostic bundle reader in `internal/debug` is a consumer.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-ACP-DEBUG-LOG-RETENTION-001` | [Retention](#retention) |
| `REQ-PLATFORM-ACP-DEBUG-LOG-RETENTION-002` | [Environment isolation](#environment-isolation) |

## Retention

`acpLogManager.prune` applies the age cap and then the count cap, oldest first by
mtime. The count cap currently ignores open writers, and `removeFile` closes the
writer before deleting the file. A burst of new files can therefore delete a live
session's log. Later frames then start a new file without the earlier history.

The count cap partitions candidates first. A file held by an entry in
`acpLogManager.writers` is never a deletion candidate, and it still counts toward
the limit. The directory can exceed the limit only by the number of open writers,
which live sessions bound. The age cap keeps its existing `writerActiveSince` rule.

Only the writers of the pruning process are visible. A second agentctl sharing the
directory is not protected, and environment isolation removes the known source of
that pressure.

## Environment isolation

Agent subprocesses inherit agentctl's environment, so a nested `go test` writes
through the same flag to the same directory. `KANDEV_DEBUG_AGENT_MESSAGES`,
`KANDEV_DEBUG_LOG_DIR`, and the `KANDEV_DEBUG_ACP_*` settings are removed in one
shared predicate used by `CollectAgentEnvWithoutAdditional`,
`collectAgentEnvWithError`, and `sanitizeEnvForAgent`. agentctl reads its own
process environment, so its logging is unaffected.

The `acp` test package gains a `TestMain` that points `KANDEV_DEBUG_LOG_DIR` at a
temporary directory, for developers who run tests from a debug shell.

## Failure and recovery

Logging stays best effort. A failed removal is ignored as it is today, and the
next janitor tick retries.

## Persistence

Logs remain plain files with the existing naming contract. No migration applies.

## Security

Frames can contain secrets. These changes do not widen who can read them or where
they are written.

## Observability

No new metric. Retention behavior is verified by tests.
