---
status: active
system: system-page
created: 2026-10-02
updated: 2026-10-02
owners:
  - cfl
---

# Go cache reuse and reclamation

## Overview

Operators need shared Go cache reuse and direct deletion of reproducible build data.
An always-busy installation can prefer build retries over disk exhaustion.
System-page owns the storage policy and operator feedback.

This requirement preserves optional launch fallback in
[Storage maintenance](storage-maintenance.md), requirement 006.
The user explicitly accepts possible build failures when busy cleanup is enabled.
No cache generations or consumer tracking are required.

## Requirements

### REQ-SYSTEM-PAGE-GO-CACHE-001: Shared cache reuse

#### Acceptance criteria

- **AC-SYSTEM-PAGE-GO-CACHE-001.1:** With management enabled, host-local task executions shall share the selected cache path across worktrees.
- **AC-SYSTEM-PAGE-GO-CACHE-001.2:** Preparation, agent, shell, test, build, and associated scripts shall retain the existing consistent cache selection for each execution.
- **AC-SYSTEM-PAGE-GO-CACHE-001.3:** Repository-maintained Kandev build and test entry points shall use `-trimpath` consistently. Matching package inputs shall reuse compiled artifacts across absolute worktree paths.
- **AC-SYSTEM-PAGE-GO-CACHE-001.4:** Disabled management, optional-cache fallback, and executor-local caches shall retain their existing contracts. Unrelated repositories shall not receive forced build flags.

### REQ-SYSTEM-PAGE-GO-CACHE-004: Direct deletion with optional busy cleanup

#### Acceptance criteria

- **AC-SYSTEM-PAGE-GO-CACHE-004.1:** Eligible Go build-cache cleanup shall delete data directly without creating a quarantine entry. Workspace and temporary-artifact quarantine shall remain unchanged.
- **AC-SYSTEM-PAGE-GO-CACHE-004.2:** A persisted Go-specific setting shall allow cleanup while tasks run. It shall default to disabled on fresh installations and upgrades.
- **AC-SYSTEM-PAGE-GO-CACHE-004.3:** With the setting disabled, automatic cleanup shall retain the existing global idle requirement. Manual cleanup shall retain existing activity admission and explicit force behavior.
- **AC-SYSTEM-PAGE-GO-CACHE-004.4:** With the setting enabled, due automatic Go cleanup shall bypass task activity and quiet-period checks. Existing tasks and newly starting tasks shall not prevent its progress.
- **AC-SYSTEM-PAGE-GO-CACHE-004.5:** Automatic deletion shall still require scheduled maintenance, enabled Go-cache cleanup, and cache size above the configured threshold. The new setting alone shall not enable scheduling or cache management.
- **AC-SYSTEM-PAGE-GO-CACHE-004.6:** Explicit Go cleanup shall remain available with scheduling or management disabled. The busy setting shall apply without requiring repeated force confirmation.
- **AC-SYSTEM-PAGE-GO-CACHE-004.7:** Busy cleanup shall not bypass ownership, adoption, containment, symlink, mount, or administrator restrictions. It shall not authorize busy cleanup of another resource.
- **AC-SYSTEM-PAGE-GO-CACHE-004.8:** Cleanup shall serialize cache mutations and remain bounded under concurrent writes. Partial failure and cancellation shall preserve unrelated data and permit another cleanup attempt.
- **AC-SYSTEM-PAGE-GO-CACHE-004.9:** Existing Go quarantine entries shall retain recorded retention, restore, and deletion behavior. They shall not block new direct cleanup.
- **AC-SYSTEM-PAGE-GO-CACHE-004.10:** Busy cleanup shall neither terminate tasks nor promise transparent retries. Builds can fail; subsequent build attempts shall reuse the same cache path.
- **AC-SYSTEM-PAGE-GO-CACHE-004.11:** Threshold discovery and deletion shall make bounded progress across repeated calls. A partial prefix shall not authorize deletion unless accumulated measurements establish that the configured threshold is exceeded. A process restart or root/settings identity change may discard in-memory progress and start a new scan.
- **AC-SYSTEM-PAGE-GO-CACHE-004.12:** Cleanup shall identify nested mount boundaries independently of device identity on supported platforms. If mount identity cannot be established, cleanup shall fail closed and preserve the affected data.

### REQ-SYSTEM-PAGE-GO-CACHE-005: Policy and result feedback

#### Acceptance criteria

- **AC-SYSTEM-PAGE-GO-CACHE-005.1:** Desktop and phone users shall see a Go-specific busy-cleanup switch beside its threshold, with a visible build-failure warning.
- **AC-SYSTEM-PAGE-GO-CACHE-005.2:** The switch shall persist after reload. Its explanation shall state that it bypasses idle checks only for Go cleanup.
- **AC-SYSTEM-PAGE-GO-CACHE-005.3:** The interface shall explain direct deletion without restore, the size threshold, and possible cache growth between maintenance runs.
- **AC-SYSTEM-PAGE-GO-CACHE-005.4:** Cleanup results shall distinguish removed bytes, remaining or unknown bytes, partial failure, and skipped resources. Busy cleanup shall never claim an empty cache or exact disk recovery without evidence.
- **AC-SYSTEM-PAGE-GO-CACHE-005.5:** New copy shall use the selected language. Phone users shall access the same controls and results without horizontal page scrolling.

## Retired draft identities

`REQ-SYSTEM-PAGE-GO-CACHE-002` and `REQ-SYSTEM-PAGE-GO-CACHE-003`, including their
acceptance IDs, are retired from the earlier unimplemented generation proposal.
Do not reuse them. Requirements 004 and 005 replace that proposal.

## Exclusions

Generations, consumer leases, hard disk quotas, per-file LRU, external cache
services, automatic task retries, module-cache cleanup, and host-wide deletion
are outside scope. The existing shared cache remains opt-in.

## Design and implementation

- [System design](../system-design/go-cache-reclamation.md)
- [Implementation package](../../../plans/go-cache-reclamation/plan.md)
