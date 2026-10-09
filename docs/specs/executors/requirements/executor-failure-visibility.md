---
status: active
system: executors
created: 2026-09-30
owners:
  - kandev
---

# Executor failure visibility

## Overview

Users returning to a task must be able to tell whether its agent completed,
was stopped intentionally, lost its executor, or merely lost connectivity.
Executors owns the resource evidence and recovery contract. Tasks owns session
transitions and shared error presentation; this document uses those authorities.

An **episode** is one interruption of an identified execution environment.
A **confirmed loss** requires authoritative resource/process evidence, not a
transport timeout. Workspace retention and provider conversation availability
are separate facts; neither follows from the other.

## Requirements

### REQ-EXECUTORS-FAILURE-VISIBILITY-001: Durable executor interruption reporting

**Intent:** Explain executor interruptions safely and support only recoveries
that the retained resource and conversation evidence actually permit.

#### Acceptance criteria

- **AC-EXECUTORS-FAILURE-VISIBILITY-001.1:** When an owned executor terminates,
  disappears, or unexpectedly restarts, Kandev shall preserve its observed cause,
  occurrence time when available, observation time, and available container exit
  and restart evidence. Kubernetes Pod reason/message and container evidence
  shall remain distinct. Exit 137 alone shall never imply OOM.
- **AC-EXECUTORS-FAILURE-VISIBILITY-001.2:** Agent completion, explicit user
  stop, planned shutdown/maintenance, executor loss, and temporary transport
  disconnection shall remain distinguishable. A disconnect alone shall not
  claim executor failure, dispatch a replacement prompt, or emit completion.
- **AC-EXECUTORS-FAILURE-VISIBILITY-001.3:** Confirmed failures shall remain
  available after reload, reconnect, backend restart, and resource disappearance.
  Active and idle retained environments shall be inspected independently of
  whether they have a tracked agent. Shared impact shall cover only sessions
  attached to the same physical environment generation, including mixed live,
  idle, stopped, and historical session inventories.
- **AC-EXECUTORS-FAILURE-VISIBILITY-001.4:** Confirmed execution loss shall
  retire only the affected current execution and abandon only its observed
  unfinished turn. It shall not publish successful turn completion, advance a
  workflow, invoke automatic agent-error retries, consume queued prompts, cancel
  healthy siblings, or erase transcript, questions, or retained workspace data.
- **AC-EXECUTORS-FAILURE-VISIBILITY-001.5:** Repeated observations and concurrent
  disconnect, polling, resume, and startup reconciliation shall create one episode
  and one historical entry per affected session. Late evidence or recovery for a
  predecessor shall not change a successor execution, turn, resource, or episode.
- **AC-EXECUTORS-FAILURE-VISIBILITY-001.6:** Before control exec or resume against
  a known failed executor, Kandev shall expose the retained actionable cause.
  Failed status checks shall say that current state is unknown and shall preserve
  known historical evidence. Passive Files or Terminal access shall not recreate
  compute while its matching executor incident remains active. Unsupported providers
  shall not gain invented causes.
- **AC-EXECUTORS-FAILURE-VISIBILITY-001.7:** Recovery guidance shall distinguish
  verified retained workspace, unavailable workspace, and unknown retention from
  restored, lost, or unknown provider conversation. A retained PVC shall not imply
  a recoverable conversation or a usable Pod. Destructive reset shall never be
  presented as harmless Resume; existing reset confirmation and permissions remain.
- **AC-EXECUTORS-FAILURE-VISIBILITY-001.8:** Successful correlated recovery shall
  retire matching active failure controls without deleting historical evidence.
  A healthy transport alone shall not claim provider recovery. Recovered transient
  connectivity shall leave no persistent failure notification or duplicate toast.
- **AC-EXECUTORS-FAILURE-VISIBILITY-001.9:** Desktop and phone shall expose the
  same durable failure in the regular recovery surface above the session composer,
  including task-owned environment failures. Session-only losses shall remain in
  their owning conversation. Tasks without sessions retain an inline recovery
  card at the bottom of the workbench. Recovery history shall visually distinguish a fresh
  provider conversation from restored continuity; cleared failures must not remain
  active errors. Task navigation shall distinguish
  executor failure from ordinary idle/completion without relying on hover or color.
- **AC-EXECUTORS-FAILURE-VISIBILITY-001.10:** User-visible details shall be
  bounded and sanitized before persistence, logging, events, copying, and display.
  Credentials, raw workload metadata, environment values, private paths, and
  secret references shall not leak. All new UI copy shall be localized; phone
  actions shall have at least 44px targets, keyboard access, and contained scrolling.
- **AC-EXECUTORS-FAILURE-VISIBILITY-001.11:** Under a responsive provider API,
  an idle retained environment failure shall be observed by the next completed
  one-minute reconciliation pass. Unavailable APIs shall use bounded inspection
  and retry without resource mutation or blocking startup indefinitely. Legacy
  rows without trustworthy identity shall expose uncertainty rather than adoption.

- **AC-EXECUTORS-FAILURE-VISIBILITY-001.12:** A Running Kubernetes Pod with an
  unavailable required agent container shall not be reported as a healthy executor.
  Repeated container startup failure shall retain readiness, waiting reason, last
  termination and restart evidence even when a Docker sidecar recovered. Correlated
  node reboot evidence, when available, shall remain distinct from inferred cause.
- **AC-EXECUTORS-FAILURE-VISIBILITY-001.13:** When stale-execution cleanup or
  recovery also fails, the durable explanation shall preserve the primary executor
  cause and separately identify cleanup timeout, authentication failure, or other
  secondary blockers. A duplicate-execution wrapper shall not replace that cause
  or falsely claim a live agent. Unknown liveness shall remain explicit.
- **AC-EXECUTORS-FAILURE-VISIBILITY-001.14:** Recovery shall distinguish executor
  availability, workspace retention and provider conversation continuity in durable
  readback. When existing authorized fallback creates a fresh provider conversation,
  users shall see that outcome after reload on desktop and phone, even if Resume
  succeeds and the session has no active error. Kandev transcript retention shall
  not be described as native conversation restoration. This requirement does not
  expand or change when conversation replacement is allowed.
  Historical volume retention shall retain its observation time and shall not
  establish original workspace continuity after deletion or replacement. A rebuild
  from published commits or reconstructed edits is separate from original data
  survival; successful Resume and retained Kandev history prove neither.

## Exclusions

Storage capacity/provisioning, worker protocol upgrades, eviction prevention,
automatic destructive repair, prompt replay, new provider retry policy, and
recovery of the reported colors task are excluded. No production resources are
used for verification. Existing resource ownership and recovery admission remain.

## Related contracts

- [Task error scope and history](../../tasks/requirements/task-launch-failure-recovery.md)
- [Interrupted task warning](../../tasks/requirements/interrupted-task-indicator.md)
- [Retained Kubernetes recovery](kubernetes-failure-recovery.md)
- [System design](../system-design/executor-failure-visibility.md)
- [Implementation plan](../../../plans/executor-failure-visibility/plan.md)

## Worker connectivity evidence

AC .2/.6/.13 include a responsive Kubernetes API returning stale Running and
container Ready=true status while Pod readiness/disruption evidence indicates
worker unavailability. This must not become a healthy/live-agent claim. Stable
explicit worker evidence is a durable uncertain warning, distinct from confirmed
process death and from a temporary API disconnect without resource evidence.
The warning survives reload, preserves primary evidence alongside cleanup blockers,
and keeps execution, turn and recovery ownership without completion or replay.

Desktop and phone show connection loss, unverifiable agent status and connectivity
repair guidance. Bound PVC/PV inventory is distinct from accessible workspace,
verified data integrity and recovery on another worker. Inspection must work
without mandatory node-level RBAC. No hardware or OOM cause is inferred.
