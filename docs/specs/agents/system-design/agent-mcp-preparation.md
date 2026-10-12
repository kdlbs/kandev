---
status: draft
system: agents
requirements:
  - REQ-AGENTS-MCP-PREP-001
  - REQ-AGENTS-MCP-PREP-002
  - REQ-AGENTS-MCP-PREP-003
  - REQ-AGENTS-MCP-PREP-004
  - REQ-AGENTS-MCP-PREP-005
---

# Agent MCP preparation system design

## Ownership and mapping

Agents owns selection and adapter preparation; executors supply trusted local
workspace/environment context, and tasks expose existing preparation progress.
This extends the [Cursor import](cursor-plugin-mcp-import.md) and
[credential bridge](cursor-mcp-oauth-bridge.md). Their filesystem discovery,
policy and ownership guarantees remain; this design supersedes their exclusion
of automatic approval only for final eligible profile-authorized imports.

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-MCP-PREP-001 | Profile and discovery contract; settings UI |
| REQ-AGENTS-MCP-PREP-002 | Adapter; lifecycle and progress; concurrency |
| REQ-AGENTS-MCP-PREP-003 | Recovery; credential continuity |
| REQ-AGENTS-MCP-PREP-004 | Retained native command diagnostics |
| REQ-AGENTS-MCP-PREP-005 | Project-local file ownership and execution generations |

See [the approval boundary decision](../../../decisions/2026-09-28-profile-authorized-mcp-preparation.md).

## Profile and discovery contract

Add `MCPSelectionMode string` / wire `mcp_selection_mode` with values `inherit`
and `selected`, plus `MCPSelectedServers []string` / `mcp_selected_servers`.
SQLite stores TEXT defaults `'inherit'` and `'[]'`; fresh and replay migrations,
table rebuild copies, DTOs, duplicates, catalog generation, frontend boot/action
normalization and runtime resolver all preserve them. Omitted create defaults
to inherit; omitted patch preserves; explicit empty selected list is meaningful.
Reject invalid modes, non-string/empty/duplicate identifiers or excessive lists
at the settings boundary. IDs are exact native emitted server names, scoped by
the profile's agent adapter. Preserve unknown selected IDs as unavailable rows
so discovery outages/removal cannot silently edit the saved profile.

Keep the existing `cursor_plugins_mcp_enabled` and `cursor_mcp_auth_enabled`
switches as compatibility gates. Selection only filters automatic imports.
It cannot override native disable state, policy, reserved names, or user-owned
project/profile entries. New adapters require explicit capabilities; do not
infer support from an executable name or expose Cursor stores to other agents.

Add `GET /api/v1/agents/:id/mcp-discovery` in agent settings handlers. `:id` is
the persisted agent ID; resolve its registered type/strategy on the backend.
The read operation returns `{agent_id,provider_id,status,reason?,servers}` where status
is `ready|unavailable|unsupported` and each server contains only
`{id,name,plugin_name?,source_kind,credentials_available}`. No URLs, commands,
headers, env, auth values, account identities or raw provider errors are returned.
Reuse existing native inventory and manifest parsing with bounded requests.
Credential availability scans existing eligible objects without writing a
master or approving anything. Missing credentials mean unavailable, not an
OAuth failure. Discovery is a host preview; task-specific source disables and
runtime locality are authoritative at launch. Refresh never changes selection.

## Agent adapter and native execution

Use a small agent-neutral contract in the existing `mcpconfig` boundary for
sanitized discovery results, preparation stage/failure codes and native command
execution. The first implementation is Cursor. Do not create a generic plugin
framework or enable another adapter implicitly.

The Cursor adapter invokes the installed CLI directly with argv, never a shell:
`cursor-agent mcp enable <exact-id>` for each final Kandev-owned import, then
`cursor-agent mcp list-tools <exact-id>` to prove readiness. Use the same local
runtime HOME and workspace as the conversational process. Resolve the approved
agent binary using existing runtime executable resolution where applicable.
No model prompt, account login, blanket approval flag or business tool call is
part of automatic preparation. Command stdout/stderr is bounded, classified and
otherwise discarded; raw output never reaches generic logs/session metadata.
Sanitized runner failures use the separate diagnostic contract below.
Use context deadlines and process cleanup, an injected runner for tests and
allowlisted reason codes. Bound the complete preparation, not only each command.

Readiness states distinguish ready, authentication_required, approval_failed,
connection_failed and unavailable. Unknown/nonzero output is connection_failed,
never inferred to be OAuth from mere tool absence. Versioned fixtures cover
native status strings; unsupported output fails visibly rather than succeeding.

The enable operation can clear native disables. Re-read selected source/task
disables and confirm the exact final owned fingerprint immediately before it;
never invoke it on vetoed, user-edited or stale candidates. Capture the final
configuration after policy URL rewriting, not the original manifest.

## Lifecycle and progress

Reuse `PrepareStep`, `PrepareProgressEventPayload`, `newProgressCallback`,
`prepareProgressRecorder`, `SerializePrepareResult` and the existing
`executor.prepare.progress/completed` pipeline. Introduce stable kinds
`agent_mcp_discovery`, `agent_mcp_selection`, `agent_mcp_credentials`,
`agent_mcp_approval` and `agent_mcp_verification`. Per-server rows carry bounded
`mcp_server_id` and `mcp_provider`; `failure_code` is a closed domain code.
UI translates kind/status/failure, not arbitrary backend text.

Integrate shared preparation before ACP/terminal process start, including fresh,
resume and workspace promotion. `launchInternal` currently publishes completed
before MCP materialization; move or aggregate completion so executor steps are
preserved and the final result includes MCP preparation. Preserve persisted
prepare_result on session reload and do not replace it with MCP-only steps on
resume. Progress and completion carry a preparation identity plus an ordered start
marker. Current-attempt progress supersedes prior-attempt state; delayed older
progress/completion cannot replace a successor even if the client never saw
that older attempt. Persist the markers with prepare_result for reload. Keep the executor preparer's
overall outcome when aggregating steps: optional setup-script failures remain
failed rows in a successful preparation result. Fatal environment or MCP
materialization errors still fail the aggregate.

An auth/connection failure for an optional imported MCP is a visible degraded
preparation outcome: retain the task environment and other usable tools, with
failed per-server rows and recovery actions. Do not fabricate a ready state or
abort access to the terminal required for recovery. Configuration integrity,
stale-ownership and unsafe-path failures remain launch-blocking. The UI must
not label a degraded result as all steps successful. Terminal WebSocket callbacks
retain their socket identity: a late close from a replaced socket cannot dispose
the successor terminal's attachment or mark that connection disconnected.
Unix terminal resizing uses guarded syscall access that keeps the PTY descriptor
valid through the resize operation while read completion and close run concurrently.

## Concurrency and isolation

Retain the Cursor canonical-workspace generation fence and containment checks.
External I/O stays outside the global project-write mutex. Serialize native
approval per canonical workspace with cancellation/current-generation checks
and fingerprint revalidation; stale work cannot approve a newer definition.
Avoid leaking lock-map entries. Source repository remains primary RepoSpecs or
trusted persisted metadata, never ordinary attached folders or credential origin.
Gate before host discovery or native commands for unsupported strategies,
remote/container/unknown executors, different HOME and import-disabled profiles.
Credential sharing and import enablement remain independently testable.

## Project-local file ownership and execution generations

Pi retains the project-local contract from [ADR 0020](../../../decisions/0020-pi-project-mcp-config-injection.md):
Kandev writes `<workspace>/.pi/mcp.json`, merges under `mcpServers`, emits the
current execution's Kandev URL with `streamable-http`, and marks generated
servers `eager`. The lifecycle manager owns the shared-file lifetime; the Pi
adapter does not write `~/.pi/agent/mcp.json`.

### Shared resource model

The lifecycle manager treats each canonical project MCP path as one shared
workspace resource. A manager-scoped coordination fence serializes the full
read/merge/write and cleanup decision for that resource. The fence covers the
workspace-relative project file and Kandev-owned temporary files without
changing the existing strategy-specific file format.

The manager mutex is paired with an OS advisory lock derived from the canonical
file path. The lock handle is kept in the current user's private cache
directory, not in the shared temporary directory or workspace, so overlapping
backend processes during restart serialize the filesystem operation without
allowing another operating system user to block it or adding project metadata.

Each participating execution has a non-secret claim record containing only the
normalized path and a SHA-256 fingerprint of the generated file bytes. The
in-memory resource record contains the active claim IDs, the current Kandev
generation fingerprint, and whether Kandev has established ownership. A new
execution claims the resource before the materialization operation is exposed
to cleanup; this is what protects a successor that has not reached
`executionStore.Add` yet.

### Materialization

`launchInternal` and `promoteWorkspaceExecution` continue to materialize
project MCP after the agentctl port is known and before the agent process can
start. Under the resource fence:

1. A new file created with the existing no-follow/O_EXCL path becomes
   Kandev-owned, records its fingerprint, and records the execution claim.
2. An existing `MergeKey` file is compared with the current known Kandev
   generation. If it is the generated file from a related execution, the new
   execution advances the generation, rewrites only the strategy's entries,
   records the new fingerprint, and inherits the shared claim set. If there is
   no matching Kandev ownership evidence, the file is user-owned or unknown:
   Kandev merges without taking deletion ownership.
3. An existing non-merge temporary file keeps its current Kandev temporary-file
   behavior, but its cleanup decision also uses the claim and fingerprint
   guard.

Claim registration, the file write, and execution metadata update are one
fenced lifecycle operation. A failed write rolls back the pending claim.
Launch rollback and duplicate-execution rollback release claims through the
same cleanup path, so an unregistered failed launch cannot strand a generated
project file.

### Cleanup and successor protection

Cleanup first releases only the ending execution's claim. It removes a file
only when all of the following are true: no other claim remains, the resource
is known to be Kandev-owned, the file is still a regular file at the contained
path, and its current fingerprint equals the current generated fingerprint. A
successor claim, a pending launch claim, a user edit, a stale/unknown
ownership record, a symlink, or a malformed file causes cleanup to leave the
path untouched. This makes the old-execution-after-new-materialization order
safe without sleeping or retrying.

Existing user files are never added to the deletion claim set. The existing
`MergeJSONUnderKey` behavior remains authoritative for preserving user MCP
servers and unrelated top-level fields; generated Kandev entries continue to
carry the current port and eager lifecycle. A user-modified file loses
Kandev deletion ownership through the fingerprint check, even if an older
execution still has a historical claim.

### Restart and recovery

Ownership claims are included in the existing `executors_running.metadata`
projection as path/fingerprint records and are marked session-scoped so they
survive same-session resume/recovery but are not inherited by a sibling
session. Recovery hydrates the shared resource record before normal cleanup or
new materialization. If recovery cannot establish that the current bytes match
the recorded Kandev generation, it fails closed and treats the path as
user-owned/unknown. No URL, header, environment value, token, or full file
content is persisted as ownership state.

This scope is deliberately lifecycle-owned rather than a new filesystem
sidecar. It keeps the user workspace free of Kandev metadata, uses the
existing durable execution record for backend restart, and preserves the
project-file contract for Pi.

## Authentication recovery

Expose `POST /api/v1/task-sessions/:sessionId/mcp/authenticate` with JSON
`{server_id}` and response `{terminal_id, task_environment_id, label, reused}`.
Expose `POST /api/v1/task-sessions/:sessionId/mcp/retry` with the same body and
response `{provider_id, server_id, status, reason_code?, tool_count?}`. Status
uses the readiness states above. Server IDs remain JSON values, not URL paths.
These typed actions carry only session ID and native server ID. Resolve the live task execution, selected execution
profile, provider, current owned definition and local eligibility on the backend.
Reject stale/unselected/disabled/unsafe identity and terminal task states.
Build a server-specific native login command from trusted executable/argv and
open it through the existing task user-shell/terminal service; do not accept
an arbitrary client command. Preserve shell environment and cwd parity with
the task, and safely quote only at the existing user-shell command boundary.
The dedicated terminal shows the native browser consent URL and callback flow.
Raw auth URLs/output are confined to the explicit terminal, not preparation
logs or chat. Reuse/focus an existing in-flight login terminal for the same
session/server rather than starting duplicate consent flows. A login attempt is
one-shot: terminal WebSocket reconnection must never execute a completed login
again. Track the attempt explicitly on the backend; browser socket lifetime is
not process lifetime. A later explicit Authenticate action starts a new attempt
after failure or completion, while concurrent/in-flight requests reuse one.

A task-local Authenticate action opens that terminal on desktop and phone.
After consent, a visible Retry connection action rechecks readiness through
Kandev. The backend owns safe same-session reconnect/catalog reload before
returning ready. Cursor requires replacing the ACP child before loading the
exact saved ACP ID: same-process session/load acknowledges success while
retaining a failed MCP client cache. Initialize the replacement and reconnect
its streams, without session/new or prompt replay; the UI must not launch/resume again. Do not create a new
task/session or replay completed prompts. Use existing
session restart/terminal UI plumbing rather than injecting a model message.
Recovery is user initiated; profile selection alone never opens an OAuth browser.
The initial native login-terminal adapter supports POSIX host shells. Native
Windows login-terminal recovery returns unavailable until a shell-aware Windows
command adapter is verified; it must never send POSIX quoting to PowerShell.
Same-conversation automatic reload is initially supported for ACP sessions with
an owned persisted ACP ID. Cursor terminal sessions do not currently persist an
owned native CLI chat ID, so live TUI retry returns unavailable. Never resume
an arbitrary latest chat or treat a Kandev/ACP ID as a terminal chat ID. Native
initial preparation and the task-local login terminal remain supported for TUI.

## Credential continuity

Retain whole-object source selection and exclusions. Track private provenance
for the bridge's generated master: selected eligible source identity/fingerprint
and last-projected object fingerprint, never token values in provenance. If the
same eligible source object remains unchanged but Cursor has updated the master,
preserve that native-updated whole object during rebuild. A removed source drops
the entry, including removal of the last eligible source: an existing owned
task link must not keep exposing stale credentials from the old master. Clear
the generated snapshot safely or detach the owned link before launch. A changed
source re-enters normal selection. A corrupt/missing
provenance record does not make the old master an authoritative source.
Use restrictive permissions, containment/regular-file checks, atomic writes and
existing auth locking. Recheck the observed master fingerprint before publication
and abort without replacing an observed concurrent native write. Cursor does not
share Kandev's lock; this is not cross-process atomic compare-and-swap, and an
external update after the final check cannot be serialized by this process. Test refresh preservation, changed/removed sources,
partial failures and invalid provenance. Do not merge token/client objects or
implement provider refresh in Kandev. If native login replaces the task symlink
with a regular file, preserve it under the existing bridge contract and report
its current state; do not overwrite it to force sharing.

## Settings and task UI

Extend `CursorProfilePreferences` through the shared profile draft/save pipeline,
using generic selection fields. Desktop presents grouped rows; phone stacks the
server name, plugin/source and credential-availability text in selectable rows
with 44px hit targets. Keep one page scroll owner and existing save/discard.
In inherit mode show preview, in selected mode allow checkbox selection. Retain
saved unavailable IDs. Localize all copy in the six real locales and generate
Traditional Chinese/pseudo through existing scripts.

Extend the existing task preparation surface rather than chat. Typed per-server
rows show failures and Authenticate/Retry actions; expanded details contain
sanitized status and the typed diagnostic causes defined below. Use the existing mobile terminal navigation, not a new
nested drawer or desktop-only tab. Guard async discovery by profile/agent and
request generation so stale responses never reset current choices.

## Retained native command diagnostics (2026-10-07, draft)

For REQ-AGENTS-MCP-PREP-004, retain errors at the native command boundary instead
of flattening them to readiness alone. Existing readiness and `failure_code`
values remain compatible. A diagnostic describes the command failure, not an
inferred provider outage. This amendment preserves the prohibition on raw
native output; it follows the existing runtime diagnostic sanitization contract.

`ExecNativeMCPCommandRunner` preserves the original Go cause through typed
wrapping, including executable-resolution/start errors currently replaced by
`ErrNativeMCPExecutableUnavailable`. Record stage separately: `resolve`, `start`,
`wait`, `cleanup` or `output`. Inspect `errors.Is`/`errors.As` before sanitizing
to distinguish `exec.ErrWaitDelay`, deadlines, cancellation, exit errors and
other wait failures. Keep an observed process exit code even if waiting for
output or cleanup fails. When both wait and cleanup fail, keep wait as the
primary cause and cleanup as secondary; cleanup must still run. Do not relax
timeouts, process-group cleanup or existing admission/ownership guards.

Add optional safe `NativeMCPDiagnostic` metadata to `NativeMCPCommandResult`
and `NativeMCPReadiness`. Its wire shape, published as `mcp_diagnostic`, is:

```text
operation: enable | list_tools
stage: resolve | start | wait | cleanup | output
kind: executable_unavailable | start_failed | wait_failed |
      output_wait_timeout | timeout | canceled | cleanup_failed |
      exit_status | output_truncated | unrecognized_output
message: sanitized underlying error, or a fixed explanation when no error exists
exit_code?: actual observed process exit code (including zero)
cleanup_message?: sanitized secondary cleanup error
```

Sanitize at the runner boundary with `routingerr.Sanitize`, replace remaining
URL-shaped spans with `[url-redacted]`, strip terminal escape/control sequences,
normalize invalid UTF-8, and cap each message to
1024 UTF-8 bytes after redaction. The existing sanitizer is a pure dependency;
do not copy its credential patterns or broaden this work into sanitizer
relocation. Never populate a message from stdout/stderr. Nonzero commands and
unrecognized/truncated verification output get a fixed technical explanation
plus the exit status, not a raw excerpt. Injected runners must also pass through
the adapter's final sanitization/validation before any diagnostic leaves it.
Successful readiness has no diagnostic. Authentication-required remains a
warning according to the existing severity predicate.

Carry the optional object through `PrepareStep`, `PrepareProgressEventPayload`,
the launch progress publisher, `SerializePrepareResult` and same-session retry
results/HTTP DTOs. The failed command's row owns the diagnostic; skipped
verification must not claim that it ran or repeat the approval error. Persist
within existing session `metadata.prepare_result`, without a database migration.
Emit one structured `native MCP preparation failed` log per failed command,
with existing task/session/preparation/server correlation, operation, stage,
kind, safe messages and exit code. Do not log argv, environment or raw streams.

The frontend explicitly drops existing MCP `error`/`output` fields in
`executor-prepare.ts` and `prepare-result.ts`. Continue dropping those legacy
fields; accept only the new bounded typed diagnostic. Extend
`executor-payloads.ts` and `PrepareStepInfo`, with the same normalization for
progress, completed results and reload hydration. Unknown/invalid diagnostics
fall back to the old generic presentation. Preparation generation fences remain
authoritative, so older failure events cannot overwrite current diagnostics.

`AgentMcpPrepareActions` renders a localized approval/verification explanation
and the safe diagnostic as selectable plain text within the existing expanded
preparation row, ahead of recovery controls. Runner approval failures use an
approval-command explanation rather than a provider connection claim. Technical
error text remains runtime diagnostic data; labels and explanations use `t()`.
Phone entry remains the task conversation's preparation details, using inline
wrapping text and vertically stacked actions with at least 44px targets,
including narrow fine-pointer viewports. Desktop retains ordinary 28px actions.
Reuse the nearest shipped surface, `AgentMcpPrepareActions` and task mobile
conversation; add no overlay or nested scroll owner.

Recovery errors currently use `{error, reason_code}`, while `ApiError.errorCode`
reads `error_code`. Add `error_code` to the Cursor recovery error envelope and
retain `error` for compatibility. The existing busy-error predicate then shows
the localized session-busy explanation. Retry command failures return the same
safe diagnostic shape and also persist it through the preparation recorder;
busy guards leave the original failure untouched. No automatic retry, process
restart or prompt replay is added.

Regression evidence includes injected runner failures, an owned Unix child
inheriting stdout after its successful parent exits (`exec.ErrWaitDelay`),
primary wait plus secondary cleanup failure, and secret-bearing error strings.
These are diagnostic regressions, not proof of the original incident's cause.
Desktop/phone tests cover live updates, reload, legacy unsafe fields, busy retry
feedback, successful recovery and stale-attempt rejection. See the
[delivery package](../../../plans/native-mcp-failure-diagnostics/plan.md).

## Verification

Unit/integration tests inject credential/native-command boundaries. Browser E2E
runs in isolated managed fixtures with sanitized discovery/status responses,
real profile persistence and preparation event/reload behavior. Real Cursor
smoke uses an isolated HOME/workspace, existing Cursor account only in memory,
a local authenticated fixture and no personal provider credentials. The test
must invoke Kandev preparation and then call the fixture without an out-of-band
manual enable/login. Test both ACP and terminal and preserve tool permissions.

## Warning presentation amendment (2026-10-02, draft)

For AC-AGENTS-MCP-PREP-002.6/002.7 and AC-AGENTS-MCP-PREP-003.5,
separate connection readiness from presentation severity. Keep the existing
persisted failed verification status and `authentication_required` reason code:
verification did not succeed. Do not mark unavailable tools ready to obtain an
amber icon. Add a shared frontend severity predicate constrained to the agent
MCP verification kind and exact reason code. Use it for the preparation row,
aggregate failed/warning counts, and `AgentMcpPrepareActions` message tone.
An overall failed preparation remains failed; any other failed row takes
precedence over warning-only completion. Other MCP failure codes retain their
current error presentation. No wire schema or data migration is needed.

`prepare-progress.tsx` must count these rows as warnings, render the amber
triangle, and retain visible explanation and recovery controls. The existing
`completed_with_warnings` summary is reused. `agent-mcp-prepare-actions.tsx`
continues to accept the failed readiness row and invoke the existing guarded
login/retry actions. Typed classification also works on older hydrated rows.
Current-attempt reconciliation and recovery fences remain authoritative.

Use the existing responsive preparation surface: actions wrap on narrow screens,
retain at least 44px touch targets, and require no hover. Cover warning-only,
mixed warning/error, fatal overall failure, hydrated legacy rows, and successful
retry through the same classifier. See [delivery package](../../../plans/setup-recovery-ux/plan.md).
