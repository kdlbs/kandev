---
status: active
system: agents
created: 2026-11-07
updated: 2026-11-07
owners:
  - kandev
---

# Incomplete managed npm cache recovery requirements

## Overview

An installed npm package can lack its execution manifest or executable links.
An upgrade can select that incomplete cache and fail before agent initialization.
Kandev must recover eligible tasks and discovery without manual npm maintenance.

The [shared managed runtime recovery contract](managed-npm-runtime-recovery.md)
owns exact runtime selection, shared-cache protection, and the common retry budget.
This document owns the verified incomplete execution-tree condition.
It does not implement the pending missing bundled Codex dependency condition.

## Requirements

### REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006: Incomplete execution tree recovery

**Intent:** An incomplete cache shall not require manual repair after a Kandev
upgrade or restart.

#### Acceptance criteria

- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.1:** Before initialization completes, a registered managed runtime can fail with npm `ENOENT`. When independent inspection confirms a missing execution-tree root manifest for the installed selected package, Kandev shall prepare that runtime privately and retry once. Generic missing-file text alone shall not authorize repair.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.2:** Automatic recovery shall cover task start, saved-session resume, startup agent discovery, profile discovery, and profile model-option resolution. It shall support managed ACP commands and managed native Codex app-server commands. Task executors shall initially include local PC, local Docker, and remote SSH.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.3:** After restart, existing affected installations shall recover through normal discovery and eligible session-open resume. Recovery shall require no terminal command, runtime-update action, new setting, database edit, or fresh conversation. Existing archive, stopped-session, automatic-start, ownership, and capacity restrictions shall remain effective.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.4:** Recovery shall preserve the selected exact package version, registry, authorized environment, workspace, model, permissions, and native conversation identity. It shall not select a global binary, persist another runtime selection, or replay an accepted prompt.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.5:** Preparation shall preserve the incomplete shared tree, unrelated caches, and every healthy sibling process. A healthy sibling shall remain usable while one sibling recovers and another exhausts recovery.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.6:** Before replacement, preparation shall validate the exact installed package and its selected executable. Prepared files shall remain available until all processes that use them stop. Backend detach or adoption shall not release those files.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.7:** All automatic setup recovery categories shall share one replacement budget. Cleanup, preparation, validation, and replacement shall use at most 90 seconds and obey an earlier caller deadline. Cancellation, shutdown, context changes, and failed process cleanup shall prevent replacement.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.8:** Successful recovery shall publish matching capabilities or continue the original conversation without a terminal failure notice. Desktop and phone shall provide the same outcome. Failed preparation or replacement shall retain the existing failure controls and sanitized final cause.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.9:** Authentication, policy, permissions, disk-space, invalid-command, and intentional-termination failures shall retain precedence. Unreadable or ambiguous inventory shall not establish a missing file. Arbitrary modules, external binaries, custom launch wrappers, passthrough, and post-initialization failures shall not qualify.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.10:** Peers without private-preparation support shall fail visibly without deleting shared files. Healthy launches shall retain their normal path. Recovery shall not scan all caches or tasks at startup, change user npm settings, or create an unbounded retry loop.

## Out of scope

- Automatic version rollback, global binary substitution, or runtime selection writes.
- Shared-tree deletion, global cache cleanup, persistent repaired-runtime promotion, and cache-wide startup scans.
- Arbitrary missing files/modules, custom wrappers, passthrough, and failures after initialization.
- Missing bundled Codex platform dependencies under requirement 005 of the shared contract.
- Sprites, remote Docker, Kubernetes, or old peers without validated preparation support.

## Implementation package

- [Incomplete npm cache recovery](../../../plans/managed-npm-cache-recovery/plan.md)
