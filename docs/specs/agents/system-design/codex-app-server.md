---
status: draft
system: agents
requirements:
  - REQ-AGENTS-CODEX-NATIVE-001
  - REQ-AGENTS-CODEX-NATIVE-002
  - REQ-AGENTS-CODEX-NATIVE-003
  - REQ-AGENTS-CODEX-NATIVE-004
  - REQ-AGENTS-CODEX-NATIVE-005
  - REQ-AGENTS-CODEX-NATIVE-006
---

# Codex app-server system design

## Purpose and evidence

The agent system owns native protocol translation and capabilities.
Accounting belongs to the [conversation usage design](../../costs/system-design/conversation-usage.md).
The boundary follows [the native normalization decision](../../../decisions/2026-09-24-native-codex-normalization.md).

On 2026-09-24, local `codex-cli 0.154.0` generated TypeScript and JSON schemas without model execution.
The initial reviewed native runtime targets that version. Live acceptance remains an implementation check, not a completed investigation result.
The schema uses `thread/fork.lastTurnId`, `collabAgentToolCall`, and `subAgentActivity`.
Overview documentation and competitor snapshots differ. Generated versioned schemas define decoding, while fixtures establish runtime behavior.

A local `codexdbg probe` on the same version initialized the app-server and read five models and 140 experimental feature entries.
It created no thread and started no model turn. The initialize response omitted the `jsonrpc` field.
The client accepts that field's absence and rejects any explicit version other than `2.0`; unit tests also cover server requests without the field.
The probe did not exercise prompts, approvals, child activity, usage, or forks.

References:

- [Official app-server overview](https://learn.chatgpt.com/docs/app-server).
- [Versioned upstream protocol](https://github.com/openai/codex/blob/rust-v0.154.0/codex-rs/app-server-protocol/src/protocol/common.rs).
- [Paseo provider](https://github.com/getpaseo/paseo/blob/7fa78244fda1e3ff03cc0754a61f091fc325d68c/packages/server/src/server/agent/providers/codex-app-server-agent.ts).
- [Orca background tracking](https://github.com/stablyai/orca/blob/6847390c0a4ff47f3ca4984953d650b973dc506e/src/main/codex/codex-background-task-tracker.ts).

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-AGENTS-CODEX-NATIVE-001 | Registration and gates |
| REQ-AGENTS-CODEX-NATIVE-002 | Client, normalization, execution |
| REQ-AGENTS-CODEX-NATIVE-003 | Children and background work |
| REQ-AGENTS-CODEX-NATIVE-004 | Forks |
| REQ-AGENTS-CODEX-NATIVE-005 | Inspection CLI and skill |
| REQ-AGENTS-CODEX-NATIVE-006 | Presentation |

## Registration and gates

Add `agents.CodexAppServer` with ID `codex-app-server` and display name `Codex app server`.
Keep `CodexACP` unchanged. Add `agent.ProtocolCodexAppServer` and one factory case in `server/adapter/factory.go`.
Use the existing managed runtime catalogue for `@openai/codex@0.154.0`, with `app-server` as its command argument.
Generalize the ACP-named managed command helpers where necessary without changing ACP command output.
Do not silently launch an arbitrary PATH version instead of the effective selected version.
Discovery, login, and runtime installation must report which binary/version the native session uses.

Flag identity:

- Key: `features.codexAppServer`.
- Environment: `KANDEV_FEATURES_CODEX_APP_SERVER`.
- Config field and JSON key: `CodexAppServer` and `codexAppServer`.
- Registry: experimental release toggle, restart required, all shipped profiles false.

The effective startup value controls backend availability.
Gate agent discovery, profile creation/reassignment, task start/resume/fork, dynamic selection, utility inference, and Office dispatch.
Direct HTTP, WebSocket, and MCP callers receive the same unavailable error.
Retain descriptors for historical display and stored-profile validation without advertising executable capability.
Disabled profiles remain readable and deletable. They do not heal or migrate into ACP profiles.
Before restart, existing sessions retain the previous effective value.
After a disabled restart, reject native process adoption and new turns, and terminate owned native processes through existing lifecycle cleanup.
Keep stored history. No independent goroutine bypasses the gate.

## Client and execution

Proposed `apps/backend/pkg/codexappserver` owns typed requests, responses, notifications, and framing.
It accepts caller-owned stdin/stdout and a context. It does not own Kandev settings, processes, UI, or credentials.
Generate a reviewed schema fixture and a small typed Go surface from the selected binary.
Keep generation reproducible and record the CLI version and schema digest.

Production process ownership remains in agentctl's `process.Manager`.
The new `server/adapter/transport/codexappserver` implements `AgentAdapter`.
The existing executor selects cwd, isolated home, environment, and process lifetime.
Use stdio initially, including Docker, SSH, and Kubernetes executors. Do not expose a new network listener.

The client has one ordered reader and one serialized writer.
Responses complete pending requests by ID. Server requests and notifications go to separate handlers.
Preserve string and numeric request IDs without collisions.
Bound frames and queues. EOF, malformed frames, deadlines, and close must release pending calls.
Reject unsupported mandatory methods. Ignore unknown optional notifications with a bounded diagnostic counter.
Request deadlines are independent from the full turn deadline.
Never automatically retry `turn/start` or `thread/fork` after an ambiguous response loss.

Admission for server-request handlers is bounded. Each admitted request has one terminal reply owner; user answers, `serverRequest/resolved`, cancellation, and close cannot produce a second reply.
Resolving a request cancels its handler without blocking ordered notification dispatch. Overload returns an explicit JSON-RPC error.
Approval options preserve the exact offered provider decisions, including structured values, and only the selected offered value is returned.
The pinned v0.154.0 server-request method inventory is recorded separately from the CLI-generated v2 schema because that generated schema omits the server-to-client request union.
Every inventory entry is classified as supported or deliberately rejected by the adapter and debugger.

Initialize with Kandev client identity, then send `initialized`.
Map `thread/start`, `thread/resume`, `turn/start`, `turn/interrupt`, and `turn/steer` to adapter operations.
Expose steering and forks through optional capability interfaces, not agent-name checks in the UI.
Serialize session transitions and scope prompt completion to the matching root thread, turn, and prompt generation.

Reuse Codex credential-copy and session-home policies from `codex_acp.go`.
Share read-only credentials where existing executor rules allow, but do not share live thread ownership between transports.
Use a per-process configuration overlay for Kandev MCP servers and permissions. Do not overwrite global user configuration.
Native approvals follow Kandev's effective permission policy, including injected MCP approval requirements.
ACP gateway authentication does not apply to the native client. Translate an explicitly selected provider endpoint through native configuration only when tested.
Otherwise reject the unsupported gateway configuration before launch. Never fall back to the user's default account.
Native model probes and utility inference must use the same client and selected version as chat.

## Normalization

Keep raw protocol parsing in the native adapter and client.
Extend `streams.AgentEvent`, existing shared payloads, and corresponding TypeScript types additively.
The UI reads normalized capabilities, identities, and states only.

| Native event or operation | Normalized result |
| --- | --- |
| Agent message delta/final item | Message chunks with stable protocol item identity and completion reconciliation |
| Reasoning summary | Existing reasoning presentation, without exposing unavailable private reasoning |
| Command, file change, MCP tool | Existing tool call/update/result kinds, output, and diffs |
| Plan and diff updates | Existing plan and diff models |
| Server approval/question | Permission or clarification request with request/thread/turn ownership; direct native questions use the Kandev session ID for clarification persistence while native IDs remain provider correlation only |
| `serverRequest/resolved` | Close the corresponding pending request |
| Child activity and child turn events | Subagent tool detail plus independent normalized execution state |
| Background command lifecycle | Independent normalized background-work record |
| Usage notifications | Scoped observations defined by the costs design |
| Root `turn/completed` | Exactly one root completion, never a child completion |

Use `(threadId, turnId, itemId)` for item state and deduplication.
Do not append both streamed text and the authoritative final text as separate messages.
Keep bounded diagnostics for unsupported item kinds. Do not render raw JSON as a fallback chat message.
Persist provider item and turn identity so history and streamed events reconcile after reconnect.
Extend context provenance with `app_server`; do not label native context reports as ACP.

## Children and background work

Reuse `task_session_subagents` for the observable child record and `ChildSessionID` for its native thread ID.
That field is not a foreign key to a persistent Kandev child session.
Preserve existing `(task_session_id, agent_execution_id, tool_call_id)` identity rules.
Add a provider-thread binding that records the root Kandev session, original Kandev turn, native parent, and source call.
Reuse the original binding across process resume. A new execution must not create a second logical child from history replay.
Persist native-only bindings separately where the existing subagent row cannot represent a root or an unannounced child.

Activity items can arrive at start and completion. They establish identity, not proof that the child finished.
Child turn state owns running/completed/failed status.
Buffer a bounded set of early child events until parentage is known. Unknown ownership never attaches to the active unrelated turn.
Represent background commands separately from child agents, even when a command belongs to a child.
Parent completion leaves the event reader alive and preserves active background state.
Do not emit a workflow completion or another root prompt completion for child events.

Reconnect reads available thread/history/background state and reconciles stable IDs.
Mark unresolved work unknown after transport loss, rather than completed.
Foreground interrupt targets the root turn. Individual stop controls appear only for proven native operations.
Background-terminal methods require explicit experimental opt-in.
Full session termination owns process-tree cleanup and marks remaining activities interrupted only after termination evidence.

## Forks

Initial product scope: fork through a selected completed root turn into a new session of the same task and executor.
This is a conversation fork with shared files. It is not a new task or worktree.
Require a quiescent source, including no known active children, for the initial implementation.
Use `thread/fork` with the selected native `lastTurnId`; never simulate a fork by concatenating rendered messages.

Add optional `ForkableSession` capability and a task-owned fork service operation.
Check task access, feature availability, native profile, executor compatibility, turn identity, and source quiescence before RPC.
Create an idempotency record before RPC with pending/succeeded/uncertain/failed state.
On success, atomically persist the destination session, native thread ID, source session, and source turn.
If persistence fails, clean up only the newly created native fork when its identity is known.
If the response is lost, mark the request uncertain and require reconciliation instead of creating another fork.
Do not expose a selectable destination before persistence succeeds.

The destination inherits the source profile and effective configuration.
Hydrate inherited messages with original provider identity and a history-only marker.
Initialize usage at the fork baseline so copied history produces no ledger events.
Use existing session concurrency and workspace ownership rules when the user later starts either session.

## Inspection CLI and skill

Proposed binary: `apps/backend/bin/codexdbg`, built by `make -C apps/backend build-codexdbg`.
Packages: `cmd/codexdbg`, `internal/agent/codexdbg`, shared `pkg/codexappserver`.
Proposed skill: `.agents/skills/codex-app-server-debug/SKILL.md`.
Reuse acpdbg's capture concepts, not its ACP initialization or canned request replies.

Commands:

```text
codexdbg probe --out DIR
codexdbg prompt --prompt TEXT --workdir DIR --out DIR
codexdbg thread-read --thread-id ID --out DIR
codexdbg thread-resume --thread-id ID --prompt TEXT --out DIR
codexdbg thread-fork --thread-id ID --through-turn ID --out DIR
codexdbg interrupt --prompt TEXT --after 2s --out DIR
codexdbg mcp-probe --out DIR
codexdbg inspect --file CAPTURE.jsonl --thread-id ID --turn-id ID
```

Shared options: executable path plus argument array, timeout, output path, workdir, and explicit stderr capture.
`prompt` can linger for a bounded interval after root completion to capture child/background events.
`probe` performs initialization and model/capability reads without a prompt or thread creation.
`inspect` is offline and can show usage scopes and request/response correlation.
MCP probing uses the existing sentinel pattern and distinguishes configured attachment from observed MCP traffic.

Default to a fresh temporary workdir and developer-owned subprocess.
Approvals default to decline; questions default to explicit cancellation. Native secret questions are rejected because the existing clarification flow persists answers in chat. A provider resolution cancels its pending clarification through the session timeout action.
An answer file can supply method-specific responses for an intentionally requested reproduction.
Unknown server requests receive a protocol error, not approval.
Never resume an arbitrary live thread as part of a generic probe.

Capture raw JSONL frames with version, schema digest, monotonically increasing sequence, timestamp, and direction.
Create capture files exclusively with owner-only permissions. Do not overwrite existing paths or follow symlinks.
Separate redacted summaries from raw capture. Do not copy raw prompts, credentials, or stderr into routine application logs.
Bound capture size and process time. Mark truncation and stop rather than silently lose evidence.
Always record close/timeout/error reason when the file remains writable, and clean up owned subprocesses.

The skill documents operation selection, build, schema inspection, frame queries, usage interpretation, and cleanup.
It names evidence limitations and uses existing authorization for the requested reproduction.
Only actions outside that request, such as mutating an unrelated live thread, require additional input.

## Presentation

Reuse existing agent profile forms and normalized conversation components.
Use child rows for available native activity and a distinct background-work summary after root completion.
A visible action opens child history. Phone history uses a full-height surface with one scroll owner.
Fork appears in completed-turn actions, with a short shared-files explanation before execution.

Reuse `MobilePickerSheet` for short choices and `useTouchDrawer` for temporary disclosures.
Keep 28px desktop controls and at least 44px coarse-pointer targets.
Use the existing responsive breakpoint, safe-area handling, localized strings, and focus-return behavior.
The [plan previews](../../../plans/codex-app-server/plan.md#ascii-ui-preview) define the relevant structure.

## Validation and rollout

Start with fake-server protocol tests, then normalized replay fixtures and targeted executor tests.
Capture real 0.154.0 events in a disposable workdir before marking optional capabilities available.
Keep actual authentication-dependent checks explicit in results. Mocks do not establish upstream availability.
Retain the flag until chat, children, accounting, forks, resume, and disabled paths have evidence.
No automatic ACP migration is part of this plan.
