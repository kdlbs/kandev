# Durable sessions across harness generations

- Status: accepted (implemented; PR #3598 under review)
- Date: 2026-09-10
- Owners: kandev
- Scope: agents, platform, executors

## Context

Kandev owns task sessions and messages.
Codex, Claude Code, OpenCode, and other harnesses own their native conversation state.
ACP exposes lifecycle methods but does not define a portable checkpoint format.

Current lifecycle code can create a new native session after a non-transport load error.
Workspace rebind has a separate restore path.
Optional JSONL history does not cover every native-resume harness.

The agentctl stream has no durable replay cursor.
A broken connection can therefore lose output or trigger failure handling without proving whether a prompt finished.

The research compared these gaps with Rivet agentOS.
Its persistence model separates public session identity from native restore and transcript-based continuation.
Its restore implementation also retains the recorded CWD.
This is useful precedent, not proof of universal harness portability.

## Decision

The boundary keeps three distinct stores:

| Owner | Durable state |
| --- | --- |
| Backend SQL | Canonical conversation, continuity attempts, generations, inbox, projection and workflow intent checkpoints |
| Agentctl bbolt journal | Bounded submissions, normalized events, sequence cursors, retained identity tombstones |
| Native harness | Private conversation, compaction, provider-specific state and tools |

Kandev prefers native resume.
Missing or incompatible native state produces a typed outcome.
Starting a replacement conversation from saved history requires an explicit user action.
The existing branch-replacement action remains native-only.

The backend prepares bounded context from canonical messages.
It does not depend on opt-in JSONL files or reconstruct private harness databases.
Both native resume and context continuation use one configuration-preserving coordinator.

Agentctl commits normalized events before publication.
It records submission identity before harness dispatch.
The backend acknowledges only committed inbox records.
Idempotent projection preserves one canonical effect across duplicate delivery.

A dispatching submission without a known outcome never receives an automatic resend.
This rule prefers visible uncertainty over duplicated external actions.

The journal uses bbolt, subject to the targeted build and storage acceptance tests.
Remote agentctl builds disable CGO.
A pure-Go transactional store avoids a custom write-ahead log and preserves those builds.

This functionality ships without a feature flag, as explicitly requested on 2026-09-10.
The draft flag never shipped and needs no runtime retirement.
Completed release milestones and mandatory failure tests replace the proposed toggle rollout.

Continuity ships with explicit autonomous park-and-surface recovery.
Durable transport ships as a complete default path for compatible peers and supported retained storage.
Only genuine protocol or executor incompatibility permits identified legacy delivery.
Journal failures block admission rather than silently selecting legacy behavior.

Current main's queue dispatch claims remain authoritative.
The new submission association extends their identity and startup reconciliation.
Unknown pre-upgrade claims cannot bypass the recovery block.

Office and automation recovery preserve pending work and surface a durable operator action.
Recovery retains run provenance, budget admission, queue ownership, and workflow approval boundaries.
There is no autonomous conversation-replacement policy in this package.

## Release safeguards

There is no instant operator kill switch.
A regression needs a corrected release or a documented compatible rollback after admission stops.
Arbitrary downgrade to pre-protocol binaries is unsafe while unresolved durable work remains.
Protocol negotiation cannot mitigate a bug shared by two compatible peers.

Release evidence therefore includes crash windows, queue restart, storage exhaustion, supported upgrades, and each executor's retained-storage behavior.
Incomplete milestones stay off release branches.
A passing specification linter is not implementation evidence.

## Alternatives considered

### One or two runtime feature flags

A flag permits quick operator rollback but adds configuration states and allows partial subsystem activation.
The user explicitly rejected feature flags for this functionality.
The package instead uses complete release milestones and real compatibility negotiation, with the increased rollback risk stated explicitly.


### Backend-only event persistence

This keeps one store but cannot retain events before a disconnected backend receives them.
It does not satisfy delivery recovery at the harness boundary.

### Another full transcript database in agentctl

This duplicates conversation authority and creates a reconciliation contract.
The proposed journal stores only delivery records with bounded retention.

### Reuse the existing CGO SQLite driver in agentctl

The existing backend driver does not fit CGO-disabled remote builds.
Adding a different SQLite implementation remains possible, but it adds an unnecessary SQL layer for this key-value journal.

### Custom append-only JSONL with checkpoint files

This requires custom crash recovery, synchronization, compaction, locking, and transactional metadata.
A maintained transactional library supplies those primitives with less application code.

### Copy harness homes or rewrite native history

Native formats and directory bindings differ by harness and version.
Copying homes can expose credentials and still fail to resume.
The package records state availability without promising portable private state.

### Replace the native harness with a Kandev model loop

This transfers model/tool ownership to Kandev and changes the product boundary.
It is outside this package.

## Consequences

Kandev can distinguish native resume, context continuation, and uncertain delivery.
The user sees which guarantee applies.
A context continuation cannot restore private reasoning or exact in-flight execution.

The package adds backend schema, a bounded local journal, and compatibility negotiation.
Executor storage lifetime limits the recovery guarantee.
A removed volume still loses its private harness state and journal.

Explicit continuation adds a user action after native-state loss.
This choice preserves existing no-silent-replacement rules.
A future opt-in automatic policy requires a separate product decision.
Snapshot composition retains the 12,000-byte plan budget inside the total 64 KiB recovery cap.
The existing plan reducer remains the owner of plan selection and sanitization.

## Evidence and limitations

Research date: 2026-09-10.
External implementations can change. Pinned harness contract tests remain the release evidence.

- [Source analysis](https://gist.github.com/carlosflorencio/563c03292c3387145098c3a2febac24a)
- [Rivet session persistence](https://rivet.dev/agentos/docs/architecture/sessions-persistence/)
- [Rivet ACP restore implementation](https://github.com/rivet-dev/agent-os/blob/main/crates/agentos-sidecar/src/acp/restore.rs)
- [ACP session setup](https://agentclientprotocol.com/protocol/v1/session-setup)
- [bbolt implementation and durability contract](https://github.com/etcd-io/bbolt)

Local evidence, refreshed against main `407ed4f1a586385f3f0f6d86f9ee928d8fce822f`:

- `apps/backend/internal/orchestrator/queue_dispatch_recovery.go`: startup recovery of existing durable queue claims.
- `apps/backend/internal/orchestrator/messagequeue/repository_dispatch_recovery.go`: attempt identity and accepted-state persistence.
- `apps/backend/internal/office/service/budget_admission.go`: fail-closed admission for unattended runs.
- `apps/backend/internal/agent/planinjection/reduce.go`: shared plan budget and tag sanitization.

- `apps/backend/internal/agent/runtime/lifecycle/session.go`: `createOrLoadSession` and context injection.
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_rebind.go`: separate workspace restore path.
- `apps/backend/internal/agent/runtime/lifecycle/session_history.go`: optional bounded JSONL context.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`: native load with explicit CWD.
- `apps/backend/internal/agent/runtime/agentctl/agent.go`: stream client without durable cursor.
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`: disconnect completion handling.

## Related records

- [Harness continuity requirements](../specs/agents/requirements/harness-session-continuity.md)
- [Harness continuity design](../specs/agents/system-design/harness-session-continuity.md)
- [Durable delivery requirements](../specs/platform/requirements/durable-agent-delivery.md)
- [Durable delivery design](../specs/platform/system-design/durable-agent-delivery.md)
- [Implementation package](../plans/durable-agent-sessions/plan.md)
- [Explicit branch recovery](2026-08-31-explicit-new-branch-session-recovery.md)
- [Configuration-preserving context reset](2026-08-18-context-reset-preserves-runtime-configuration.md)
