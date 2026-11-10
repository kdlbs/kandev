---
status: current
system: tasks
requirements:
  - REQ-TASKS-WORKSPACE-COLLABORATION-001
---

# Conversation collaboration boundary

## Identity, authority and storage

Backend composition reads the trusted `mcp/scope.Principal` derived from the
execution's task/session. Task services receive a narrow callback interface and
do not import the MCP transport. Enabled checks run even when native auth is
disabled; real user identity still passes through native workspace authorization.
Browser requests and internal work without a conversation principal remain native.
An execution context lacking its expected principal is an enforcement error.

The task repository owns a typed `workspace_collaboration_grants` table, keyed by
caller task, current source workspace and target workspace. The four independent
booleans default to false. Caller-session IDs are validated on every operation
but are not grant keys. Moving the task does not silently carry authority into a
new source workspace. General task metadata updates cannot change these rows.
Explicit all-false updates revoke authority without deleting the saved record.
The table is initialized after core task migrations. Foreign keys cascade on
caller-task, source-workspace or target-workspace deletion in SQLite and
PostgreSQL, including direct deletes and transactional workspace cascades. This
also covers existing reset deletion paths without separate integration cleanup.
Archival, session replacement and explicitly turning the feature off retain grants.

Human grant management requires native `workspace.manage` in the source workspace
and native read reach of the target. Conversation callers cannot edit grants,
including their own. Each exercised action separately intersects native target
authorization. Existing org, capability and coordinator restrictions remain.

## Action mapping

| Grant | Native operations in this slice | Native permission also required |
| --- | --- | --- |
| read | task/session/workspace reads and lists at the task-domain boundary | workspace.read |
| report | dedicated append-only status report on an existing target session | task.write |
| prompt | ordinary message admission and start/resume prompt authorization | session.prompt, or existing coordinator manager scope |
| control | task/workflow mutations, session stop/control/exec and workspace administration at the same boundary | existing corresponding native scope |

Report entry points accept only target task/session and text. Server code fixes
the status-message type, author and completed-turn behavior. They never call
orchestrator prompt/queue or workflow mutation. Generic message requests still
require prompt permission even if their body claims an agent author.
The report primitive requires a verified conversation context; a plain human
request cannot fabricate an agent-authored report by supplying caller fields.

## Feature lifecycle and API

`features.workspaceCollaboration` uses
`KANDEV_FEATURES_WORKSPACE_COLLABORATION`, is off in prod/dev/e2e and requires
restart. The existing typed runtime registry, persisted overrides and System
Feature Toggles UI own it. Explicit environment > override > profile still holds.
Off closes the new grant/report APIs and preserves existing native APIs. Explicit
administrator off restores native behavior without deleting saved grants.
An enabled store/identity failure returns an error and never takes this off path.

The native API reads and replaces grants under
`/api/v1/tasks/:id/workspace-collaboration`; reports have a separate task/session
entry point. Server route identity determines the grantee; a body cannot replace
the caller task, session, source workspace or user identity. Workspace managers
can open the grant editor from desktop task actions, mobile task navigation or
the integrated-control conversation.

## Settings UI composition

```text
Settings > System > Feature Toggles
  Workspace collaboration                         [ OFF / ON ]
  Restart required. Off restores native permissions; saved grants remain.

Task actions > Workspace collaboration
  Source: [logical conversation] in [current workspace]
  Target workspace        Read  Report  Prompt  Control
  [workspace selector]    [ ]   [ ]     [ ]     [ ]
  Native user permissions still apply.             [Save changes]
  Load/save failure: preserve draft; show error; do not broaden permissions.
```

## Verification

Use existing native authorization and typed flag checks plus disposable SQLite
and dispatcher overlays. Exercise false/default grants, independent actions,
same-workspace/native browser behavior, session replacement, spoofed request
identity, native RBAC denial, store failure, human-only edits, report isolation
and the desktop/mobile editor entry points.
Fresh/replayed schema initialization must preserve grants and existing records.
No new poller, queue, permanent test framework or operating ACL update is needed.

## Transport coverage and original remaining work

### Plugin invocation context increment

The SDK binds each outbound plugin RPC to its original server context for that
RPC's lifetime, separately for each plugin Host instance. The Host broker checks
the returned metadata reference before restoring original context values; native
plugin capabilities still apply. Agent tool calls additionally require the
server-verified conversation principal. Event/action/webhook callbacks use their
actual native server context, including any original user identity.

The original tool InvocationID remains correlation data. Its MCP request field
can be repeated, so a fresh server call reference prevents a delayed callback
from acquiring a later invocation with the same ID. No proto field, persistent
ledger, global principal or task identity supplied by a plugin is introduced.
A live opaque reference is a bearer capability for its lifetime. The SDK
removes it from handler-visible metadata and avoids printing it in context
strings, but in-process plugin code that wraps the handler context can still
observe and use a live reference; that is the same invocation authority the
handler already holds, not new authority. No signature or hash binding is added
because plugins are privileged in-process code and the approved scope does not
introduce a new credential scheme. The SDK installs the reference from the
handler's incoming context into a private value and removes it from
handler-visible metadata. Authors must retain that context when calling Host;
dropping it cannot confer background authority. Domain Host calls with missing, expired, canceled or other-plugin
references fail closed while enabled. Plugin-owned capabilities retain their
existing capability rules for startup and detached work: state, config,
secrets, events, capability context, plugin-owned task trees, managed agent
conversations and schedules, managed inputs, executor progress and artifacts,
declared source-issue writeback, utility agents, and profile catalogs. An
unidentified job cannot use conversation domain authority while enabled; an
actual native RPC callback can. The SDK classifies every Host service method
explicitly, an unclassified method defaults to requiring a caller, and a test
fails when the generated service and the classification diverge. Off keeps the
original native path and stored grants.

While enabled, plugin-authored `plugin.<id>.*` events carry a host-installed
envelope containing the emitting plugin and its opaque live call reference.
This is separate from plugin payload and is retained by JSON serialization and
the existing delivery queue/ring buffer. Each attempt resolves the reference
against the issuing RemotePlugin's existing map, including delivery to another
subscriber. The receiver's new RPC inherits the original principal, owner and
cancellation. No additional registry or persisted identity is introduced.
Missing, returned, canceled or restarted origins still permit notification
delivery under an unidentified context; domain Host operations reject that
context. Re-emitting it cannot restore native authority. Existing bounded retries
and buffering remain responsible for delivery outcomes.

This rule concerns events whose payload and publication are directly selected
by plugin code, including an agent tool's call to EmitEvent. Native domain events
such as `task.updated`, emitted after an authorized state transition and consumed
by a registered automation owner, retain that owner's existing independent
authority. A mutation's causal origin does not add a general taint policy to
native automation, human or background work.

Verification for Task 02 must exercise the actual SDK plugin-RPC and broker-Host
path, concurrent conversations/native RPCs, cancellation and late/mismatched
references, with real domain read/report/prompt/control effects. The earlier
API/domain/store checks remain valid and need not be repeated.

The implemented boundary consumes trusted conversation contexts at task-domain
entry points. The native in-session stream installs those contexts through
`agent/runtime/lifecycle/mcp_identity.go` and `mcp/scope/principal.go`; ordinary
human HTTP/WS and internal scheduler calls remain native. The report operation
uses its task/session API and the `message_task_kandev` report delivery mode.

Task 01 recorded the generic Host return path's missing origin as a transport
gap. Task 02 closes that callback boundary with the live SDK context bridge
above, without trusting tool JSON identity or placing one principal on a shared
Host. Domain mutations retain their native control scope: the legacy task
update/move adapter reads only a write-authorized placement/version snapshot
and write-authorized management claim, rather than accidentally requiring a
separate collaboration read grant for its internal CAS preflight. Existing
resource versions and management claim generations still fence the mutation.

Existing generic Host messages remain prompts, including when the caller only
holds report permission. Profile inheritance, delegation, taskless Office
principals and aggregate navigation remain subsequent work. Grant edits do not
change native permissions, install plugins, start a model or derive authority
from watches.
