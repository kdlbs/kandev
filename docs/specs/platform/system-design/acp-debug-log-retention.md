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
mtime. Both caps call `removeFileIfNoOpenWriter`, which checks the process-local
writer registry and removes the file while holding the same mutex used by
`getWriter`. This makes the check and removal atomic with new writer
registration, so a concurrent frame cannot reopen an unmanaged file after the
janitor checks it.

An active segment held by an entry in `acpLogManager.writers` is not a deletion
candidate, and it still counts toward the count limit. The directory can exceed
the limit only by the number of active writers when no other candidates remain.
The age cap uses the same open-writer check; after an idle writer closes, normal
mtime-based retention can remove its file.

The registry protects only active segments held by this agentctl process.
Rotated segments are not associated with their live writer and may be removed by
retention. A second agentctl sharing the directory is not protected. Cross-process
coordination and rotated-segment grouping are outside this design.

## Environment isolation

Agent subprocesses inherit agentctl's environment, so a nested `go test` could
write through the same flag to the same directory. `KANDEV_DEBUG_AGENT_MESSAGES`,
`KANDEV_DEBUG_LOG_DIR`, and the `KANDEV_DEBUG_ACP_*` settings are removed in one
shared predicate used by `CollectAgentEnvWithoutAdditional`,
`collectAgentEnvWithError`, `sanitizeEnvForAgent`, and ACP terminal creation.
The terminal path applies the predicate to both the inherited environment and
explicit overrides. agentctl reads its own process environment, so its logging
is unaffected.

The `acp` test package gains a `TestMain` that points `KANDEV_DEBUG_LOG_DIR` at a
temporary directory, for developers who run tests from a debug shell. If that
directory cannot be created, the test process exits before running tests.

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
