---
status: draft
system: executors
created: 2026-11-08
owners:
  - kandev
---

# Executor runtime selection

## Overview

An executor selection defines where work can run. Users must receive that execution boundary or a refusal before work starts elsewhere.
Executors owns this contract because it owns runtime admission and physical resource identity.

## Terms

- **Required selection:** A concrete executor selected by a caller, profile, saved default, workflow, or retained session binding.
- **Unspecified selection:** No concrete executor remains after the existing selection rules run.
- **Actual runtime:** The backend that created or authenticated an execution. Requested configuration does not establish this fact.
- **Docker runner:** A separately identified worker container. The backend service container cannot satisfy this identity.

## Requirements

### REQ-EXECUTORS-RUNTIME-SELECTION-001: Required runtime admission

**Intent:** Preserve the execution boundary selected for agent and workspace work.

#### Acceptance criteria

- **AC-EXECUTORS-RUNTIME-SELECTION-001.1:** When a required runtime is unavailable, preparation and launch shall refuse it without selecting another runtime.
  This includes a disabled or unregistered Docker backend, an unreachable daemon, and container creation failure.
- **AC-EXECUTORS-RUNTIME-SELECTION-001.2:** A disabled or unregistered required runtime shall be refused before credential resolution, credential seeding, scripts, or worker allocation.
  Later provider failures shall never start a host worker or deliver a prompt outside the selected runtime.
- **AC-EXECUTORS-RUNTIME-SELECTION-001.3:** A required Local Docker launch shall use a separate Docker runner with a verifiable container identity before reporting success.
  Backend container membership, a hostname, or `/.dockerenv` shall not establish runner identity.
- **AC-EXECUTORS-RUNTIME-SELECTION-001.4:** An unspecified selection shall retain the existing standalone default.
  Local, legacy `local_pc`, and Worktree shall retain standalone execution. The mock executor shall retain its explicit test behavior.
  Unknown and unavailable plugin runtimes shall remain refused.
  A failed concrete executor or profile lookup shall not count as an unspecified selection.
- **AC-EXECUTORS-RUNTIME-SELECTION-001.5:** Refusal shall expose a stable reason and safe, actionable guidance through existing launch failure channels.
  An unavailable backend requires operator configuration or a deliberate new selection. A provider failure permits retry on the same runtime.
  Neither outcome shall change saved selection, deployment configuration, or permissions automatically.

### REQ-EXECUTORS-RUNTIME-SELECTION-002: Retained execution compatibility

**Intent:** Prevent caches and recovery from bypassing the selected execution boundary.

#### Acceptance criteria

- **AC-EXECUTORS-RUNTIME-SELECTION-002.1:** Before reusing an execution, cached access, workspace preparation, promotion, resume, and new prompt admission shall compare its actual runtime with the admitted selection.
  A retained standalone execution shall not satisfy Local Docker, including when Docker becomes available later.
- **AC-EXECUTORS-RUNTIME-SELECTION-002.2:** Missing or contradictory retained evidence shall refuse new work until the boundary can be established.
  The refusal shall preserve source files, workspace inventory, conversation history, runtime identity, and unrelated retained work.
  It shall not migrate environments or terminate a worker automatically.
- **AC-EXECUTORS-RUNTIME-SELECTION-002.3:** Authorized operators shall retain execution observation and Stop access for a mismatched retained worker.
  Recovery can authenticate and observe that worker without authorizing new commands, credentials, or prompts on it.
- **AC-EXECUTORS-RUNTIME-SELECTION-002.4:** Both repository-backed and repository-free paths shall enforce the same runtime boundary.
  Concurrent ensure and launch callers shall not use a coalesced result to bypass that boundary.

### REQ-EXECUTORS-RUNTIME-SELECTION-003: Observable execution identity

**Intent:** Let an authorized consumer distinguish selected configuration from observed execution.

#### Acceptance criteria

- **AC-EXECUTORS-RUNTIME-SELECTION-003.1:** Session detail and session summary responses shall expose observed execution runtime separately from selected executor and profile identifiers.
  Unknown execution runtime shall remain unknown. Selection alone shall never populate it.
- **AC-EXECUTORS-RUNTIME-SELECTION-003.2:** Runtime events, persisted execution inventory, session responses, and container identity shall describe the same execution.
  A stopped inventory record describes the last recorded execution, not proof of current liveness.
- **AC-EXECUTORS-RUNTIME-SELECTION-003.3:** Docker success shall expose the worker's daemon-verifiable container ID.
  A missing or conflicting ID shall not be reported as successful Docker execution. Standalone evidence shall remain labelled standalone.
- **AC-EXECUTORS-RUNTIME-SELECTION-003.4:** Identity and refusal diagnostics shall retain current access controls and omit credentials, secret references, and raw environment contents.
  Older clients shall continue to use existing response fields. New identity fields shall be additive and tolerate older servers.

## Existing contracts

This capability does not replace these active requirements:

- [Executor ownership](per-user-ownership.md): `REQ-EXECUTORS-OWNERSHIP-004`, especially criteria `.1` through `.4`, governs actor and compute eligibility.
- [Workspace reuse](../../tasks/requirements/additional-session-workspace-reuse.md): `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001` and `003` govern workspace attachment and executor transitions.
- [Durable agent delivery](../../platform/requirements/durable-agent-delivery.md) governs authenticated delivery and journal ownership during recovery.

A separately authorized environment transition can retire its exact predecessor under existing policy.
A runtime mismatch within one selected binding does not authorize such a transition.

## Exclusions

No new isolation mode, automatic environment migration, executor mapping change, deployment repair, Docker socket installation, or credential workaround.
No new UI controls, indicator redesign, runtime upgrade policy, or worktree cleanup behavior.
Provider-specific configuration, resource ownership, and cleanup contracts remain authoritative.

## Delivery

[Plan and work orders](../../../plans/explicit-executor-runtime-isolation/plan.md).
