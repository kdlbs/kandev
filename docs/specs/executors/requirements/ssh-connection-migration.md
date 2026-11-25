---
status: active
system: executors
created: 2026-11-24
owners:
  - kandev
---

# Migrate inline SSH settings to shared connections

## Overview

An upgrade should preserve an administrator's existing SSH executors and remove the choice between two SSH setup flows. SSH settings belong to the executor. Its profiles inherit those settings and keep their own names, scripts and policies. Import once per executor, regardless of profile count.

### REQ-EXECUTORS-SSH-MIGRATION-001: One SSH setup flow after upgrade

- **AC-EXECUTORS-SSH-MIGRATION-001.1:** New SSH executors shall use a shared connection. Existing executor and profile IDs, display names, task associations and unrelated settings shall survive conversion. An already managed executor shall remain untouched.
- **AC-EXECUTORS-SSH-MIGRATION-001.2:** The backend shall automatically convert old executors to shared connection references on upgrade, including when hosts are offline. Restart, concurrent requests and retry shall not duplicate identities, connections or bindings. Matching host names alone shall never combine records.
- **AC-EXECUTORS-SSH-MIGRATION-001.3:** A complete saved route shall continue to work without an administrator action. Migration shall preserve the resolved endpoint, login, exact saved host pin and file or agent reference. Import shall carry forward recorded trust. The first authorized use shall automatically perform any required fresh checks before starting work. Migration shall never copy private keys, install keys remotely, invent trust, or treat an earlier successful session as proof of a new route.
- **AC-EXECUTORS-SSH-MIGRATION-001.4:** Missing or changed trust, an unavailable credential, an unresolved alias and an unsupported jump chain shall identify the affected hop and a next action. Administrators shall resolve exceptions in the shared connection editor. A deliberately different route requires explicit replacement and fresh verification. Other executors can finish independently.
- **AC-EXECUTORS-SSH-MIGRATION-001.5:** An incomplete route shall block only its own new launches and identify the missing fact. Transient unavailability shall remain retryable through normal use. A previously running or recoverable session shall retain its original owner and route evidence. Migration shall neither stop it nor redirect it through a newly selected connection. An unsafe reconnect shall require recovery.
- **AC-EXECUTORS-SSH-MIGRATION-001.6:** Desktop and phone shall show imported connections in the normal collection. Only a connection needing repair shall offer a contextual action in Access. Only authorized administrators may view sensitive configuration references, change trust, verify or bind. Other users shall see a safe reason for blocked launch.
- **AC-EXECUTORS-SSH-MIGRATION-001.7:** One PR shall deliver the automatic data migration and cutover for UI, APIs, tasks, probes and services. An explicit disabled SSH-manager setting shall never enable inline setup or background SSH activity. Skipped-version upgrades shall still import old records after the old creation and launch paths have been removed.

## Related contracts

These requirements govern the single-release cutover described in [SSH connection manager, section 005](ssh-connection-manager.md#req-executors-ssh-connections-005-compatibility-and-rollout). The [delivery plan](../../../plans/ssh-connection-migration/plan.md) owns the transition.
