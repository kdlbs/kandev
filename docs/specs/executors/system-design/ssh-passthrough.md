---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-SSH-PASSTHROUGH-001
  - REQ-EXECUTORS-SURVIVAL-005
created: 2026-10-14
owners:
  - kandev
---

# SSH passthrough system design

## Boundary

The SSH controller owns the remote CLI and PTY. Lifecycle owns command selection,
prompt admission, and workflow state. Gateway owns browser authorization and the
binary terminal connection. This follows the
[process-ownership decision](../../../decisions/2026-10-14-ssh-passthrough-process-ownership.md).

This is a proposed extension of [SSH execution](ssh-executor.md). It uses existing
SSH connection, fingerprint, helper upload, handshake, and forwarding behavior.
It adds no SSH daemon requirement beyond the existing executor requirements.
Neither Proxmox LXC nor Tailscale requires a separate launch path.

## Requirement mapping

| Criteria | Design boundary |
| --- | --- |
| SSH-PASSTHROUGH-001.1, .2, .5, .14 | Remote preparation and structured launch |
| SSH-PASSTHROUGH-001.3, .4, .13 | Browser terminal and remote streams |
| SSH-PASSTHROUGH-001.6, .8 | Prompt policy and correlated events |
| SSH-PASSTHROUGH-001.7, .9, .10, .11 | Process admission and recovery |
| SSH-PASSTHROUGH-001.12 | Capability and safe failure projection |
| AC-EXECUTORS-SURVIVAL-005.3 | Standalone scope remains separate |

The abbreviated SSH criteria above use the `AC-EXECUTORS-` prefix.

## Existing failure path

`SSHExecutor.buildInstance` publishes `ssh_remote_agentctl_port`, the forward
port, controller identity, and remote workspace. It does not set `StandalonePort`.
`passthroughMCPConfigPort` reads only the standalone port. MCP command preparation
therefore returns `standalone port unavailable for passthrough MCP config`.
`wrapBootstrapFailure` projects the error as `unknown` with the generic bootstrap detail.

After that failure is removed, `Manager.startInteractiveProcess`, resume, and
gateway terminal routing still call `Manager.GetInteractiveRunner`. That method
always chooses the standalone backend. A remote-only directory fails during
local PTY startup. A matching local directory allows local CLI execution.

The non-Docker auth-token selection defect already has a correction in
`resolveLaunchAuthToken` and `TestResolveLaunchAuthToken`. Keep that correction.
A missing real secret remains a failure. The screenshots alone do not establish
why this reporter's resume token was absent.

## Controller runtime

Add one passthrough owner to each `process.Manager`. It uses the existing
`InteractiveRunner`, `ptyexec`, environment sanitization, buffer limits, and
readiness detector. It remains separate from ordinary user shells and ACP adapters.

`Manager.BeginOwnedOperation`, `CloseAdmission`, and `StopForTeardown` govern
passthrough creation and teardown. Teardown closes admission before it waits for
operations, stops the CLI process group, joins runner readers and callbacks, and
then closes streams. It must report a retained cleanup failure rather than
release ownership while descendants remain.

The wrapper serializes start, replace, and stop for the instance's configured
session. It also excludes concurrent ACP launch for that instance. A request's
session must equal `InstanceConfig.SessionID`. Working directory comes from
`InstanceConfig.WorkDir`. The browser never supplies an executable or workdir.

Reuse request fields from `InteractiveStartRequest` in neutral wire DTOs under
`internal/agentctl/types`. Do not expose server-process package types as a new
client contract. The wire request carries structured argv, explicit environment,
strip keys, readiness options, and terminal dimensions. Server logging redacts
secret-bearing arguments independently of the supplied log representation.

## Authenticated controller API

Proposed routes live under `/api/v1/passthrough` on the per-instance router.
They use `instanceAuth`, `instanceIDGuard`, session validation, and stream
credential invalidation. No route joins the unauthenticated MCP exemption list.

| Route | Contract |
| --- | --- |
| `GET /passthrough` | Protocol version 1, controller/session identity, remote paths and MCP URL, current process snapshot, and event cursor |
| `POST /passthrough/prepare` | Materialize emitted MCP artifacts within allowed remote roots and return ownership claims |
| `POST /passthrough/start` | Start by stable operation ID and expected generation, or return the same start result |
| `POST /passthrough/input` | Write bytes to the named process generation, with optional submission ID |
| `POST /passthrough/resize` | Resize the named generation, with validated dimensions |
| `GET /passthrough/idle` | Wait for first idle of the named generation within the caller's cancellation budget |
| `DELETE /passthrough/:process_id` | Stop the named generation |
| `GET /passthrough/stream` | Binary terminal stream for the current session process |
| `GET /passthrough/events` | Ordered JSON events with bounded replay from a cursor |

Controller APIs use an instance-local process generation and an opaque process
ID. A successful snapshot returns status, start time, exit code, first-idle
state, and last event sequence. It excludes raw argv, environment, and config
contents. Every event identifies controller, session, process, generation, and
sequence. Events include started, exited, input submitted, and raw readiness signals.

The remote owner records events before publishing them. Start returns a cursor
from before that process starts so early exit or idle cannot disappear before
the backend registers the process. A reader subscribes and captures its replay
snapshot under one lock. Sequence checks suppress duplicate delivery. An exited or canceled observer
releases its target. Observer cleanup must match its observer generation so an
old goroutine cannot clear a replacement that follows the same process.
Retention is bounded by the existing per-instance event limit. Each replay page
contains at most 256 events and reports the last included sequence. The next
page starts after that sequence, within the backend reader limit. A replay gap
requires a fresh snapshot. Apply a verified non-running process status after
the cursor save succeeds. An idle snapshot cannot attribute a lost completion
to the active turn, so it must not synthesize a turn-completion event.

Start admission is idempotent by operation ID. The same ID with different
parameters is a conflict. A different ID while the prior CLI is live is a
conflict unless the caller names a verified replacement generation and stops it.
An uncertain response is resolved by querying that operation's result before
any second start. Retain the current generation's start receipt. A replacement
retires earlier start and input receipts; expected-generation checks reject
old start requests. Stop is idempotent for the specified generation.

Automatic prompt writes use a submission ID. The owner records admission and
result before allowing a duplicate to write. A partial or interrupted write is
`uncertain`, and cannot be replayed automatically. This deduplication is scoped
to the retained controller. It is not an ACP delivery journal.

Identified input uses canonical UUIDv7 submission IDs. Retain at most 4096
receipts for the current process. Evict only completed receipts in ID order and
advance an expiry floor. Reject an unknown ID at or below that floor, so an
expired retry cannot write again. Pending receipts remain retained. Raw browser
keystrokes do not request receipts and do not consume their capacity.

Each input receipt includes the event sequence after the write and its submitted
event. Persist the final initial-submission boundary before clearing the startup
prompt guard. An idle event at or before that boundary cannot complete the turn,
even when its WebSocket delivery follows the HTTP write response.

Explicit passthrough Resume does not require an ACP conversation or retire ACP
delivery records. If prior SQL delivery history remains unresolved, Resume
stays blocked. Passthrough prompt receipts retain their own recovery boundary.
Conversation resume intent travels separately from the previous runtime owner:
ordinary Stop removes the operational runtime record.

Browser attachment waits for an admitted SSH execution to register. If none
exists, it reports that the workspace is not ready and leaves creation to the
launch or Resume operation. It must not create a second remote controller from
workspace information while that operation prepares the target.

The binary stream keeps the existing protocol: raw input and output, plus
`0x01` followed by resize JSON. If an output subscriber overflows, close the
browser stream so it can reconnect and replay the retained buffer. Normal PTY
output completion keeps the session stream open for deliberate replacement. The remote owner uses process-specific PTY
access. Session-level stream attachment follows a confirmed replacement and
resends the last size. Browser disconnect removes only its subscription.
An old stream's cleanup must not clear a successor connection's writer.

## Remote command and MCP preparation

The capability snapshot provides remote paths using the target platform's path
rules. Session configuration belongs beneath
`<remote-workspace>/.kandev/sessions/<session-id>/passthrough/`.
The snapshot advertises `http://127.0.0.1:<instance-port>/mcp`, where the port
belongs to the remote instance. The local SSH forward port is never placed in
remote CLI configuration.

Lifecycle continues to use the selected agent's `PassthroughMCPStrategy`.
`passthroughMCPServers`, `passthroughMCPPaths`, and `applyPassthroughMCPWithPreparation`
receive the selected transport's address and paths. Claude file arguments,
OpenCode file environment, Codex overrides, and project-local strategies all
refer to remote resources. External MCP entries follow existing remote transport
filtering. Host-only stdio commands do not become remotely available by rewriting
an address. No reverse SSH tunnel is introduced.

Extract shared filesystem rules from `materializePassthroughFileLocked`, the
merge helpers, and ownership helpers into proposed
`internal/common/passthroughconfig`. Both local lifecycle and remote agentctl
use this implementation. Preserve local behavior with its existing tests.
The common package owns filesystem operations, not provider strategy selection.

Remote preparation validates every path against the remote workspace or the
session config root. It refuses symlink traversal, uses private directory and
file permissions, and writes atomically. Project-file updates retain the current
merge policy and unknown-user-file cleanup exclusion.

For shared project files, store claims under a private task-level directory and
serialize them with a cross-process file lock. Each claim names controller,
session, generation, and fingerprint. Separate remote controllers must see all
claims. A last-owner cleanup deletes only an unchanged Kandev-owned generation.
An unreadable claim registry preserves the file and reports cleanup failure.
A preparation must not change a file while another owner claims its contents.
Identical JSON may share a claim. A conflicting controller URL fails before
writing files or adding claims. The claims contain no tokens or configuration
bytes. Unproven stale claims
preserve files instead of enabling deletion.

Preparation accepts only the next process generation. Under the controller's
startup lock, it releases the stopped current generation's claims before
materializing replacement artifacts. It preserves pending next-generation
claims and all other controllers' claims. A live generation retains its claims.
An agent reset resolves profile credentials before Stop, then prepares remote
MCP files only after Stop succeeds. Failed Stop preserves the recorded process
identity and files. Preparation failure after Stop marks the execution failed.

The CLI uses the existing agent passthrough command and remote environment
snapshot. Native executable resolution occurs on the target. A missing remote
CLI is a startup failure, not a reason to invoke its host installation.
Credential selection and copying remain in the existing SSH credential path.
Do not introduce implicit installation or credential transfer.

Open the existing MCP stream before the CLI can make a tool call. It remains
independent of the terminal stream and uses the existing task/session MCP gates.
Configuration is refreshed on a confirmed new launch. Reconnecting to a retained
CLI preserves its existing configuration and does not repeat preparation.

## Lifecycle transport selection

Add an internal passthrough transport seam in lifecycle with the operations
actually used by launch, status, input, resize, buffer, first idle, stop, and
terminal attachment. Its local implementation wraps the existing runner.
Its SSH implementation uses the authenticated `agentctl.Client` held by the
execution. Select by executor identity, not by whether the local runner exists.

Replace direct runner use in fresh launch, `EnsurePassthroughExecution`, resume,
automatic prompt injection, fallback, model relaunch, reset, stdin, resize,
buffer reads, and `stopPassthroughProcess`. Keep local user-shell APIs separate.
`GetInteractiveRunner` remains available for local user-shell consumers.

The remote transport acquires the current client lease for each operation.
Gateway resolves the authorized execution before selecting its terminal path.
For SSH it dials the passthrough binary stream with controller auth, then
bridges the existing browser route. Both pump directions have one owner that
closes both sockets and joins both pumps on cancellation or failure.

The remote owner emits an input-submitted event only after a PTY write succeeds
and contains the submit sequence. Lifecycle claims running for that event's
current process under its existing identity lock. A successful WebSocket send
alone does not prove PTY acceptance. Programmatic input uses the current
lifecycle admission and prompt boundary. Ctrl+C, resize, and reconnect do not
create extra turns.

Remote events enter lifecycle with the execution captured by stream registration.
Check that execution, controller, process, and generation remain current before
calling `handlePassthroughStatus` or `handlePassthroughTurnComplete`.
Event projection and cursor persistence do not hold the agentctl source guard.
Event state writes and cursor saves share the runtime teardown guard. Recheck
the active execution and process before writing. Publish captured lifecycle
events after releasing the guard so subscribers can re-enter Stop or Resume.
Keep the pending initial-prompt marker in lifecycle as specified by
[ADR-2026-09-01](../../../decisions/2026-09-01-passthrough-initial-prompt-turn-boundary.md).
The remote runner emits raw readiness and does not decide task completion.

## Recovery and persistence

Persist only allowlisted controller/process identity, generation, start operation
IDs, and automatic-input submission IDs in existing execution metadata. Record
the submission identity before its first write. SSH auth remains a secret
handle through `persistAuthToken` and `resolveLaunchAuthToken`. Do not store
credentials, PTY output, or MCP bytes in metadata. No SQL migration is needed.

Browser reconnect verifies the live process snapshot and binds the existing
stream. It never reruns initial-prompt injection. Transport disconnect marks
availability as unknown and suspends replacement. Reopen the authenticated SSH
forward and reconcile the same controller/process snapshot before admitting input.

After a backend crash, explicit Resume uses the preserved SSH controller identity
and token. It queries the remote passthrough owner before the ACP-only status path
can treat an unconfigured adapter as an absent agent. If a matching CLI is live,
register its snapshot and attach without starting or loading a provider session.
Initial-prompt writes with uncertain results remain blocked for user recovery.

If the CLI is confirmed absent while the controller is alive, start using the
existing provider resume flags. If the controller is confirmed absent, retain
the workspace and use normal SSH controller creation before the same CLI resume.
Missing tokens, mismatched identities, ambiguous process counts, and failed
liveness probes do not permit replacement. Existing explicit reset/recovery
actions must confirm termination before they admit a new process.

Graceful backend shutdown follows current passthrough Stop behavior and preserves
the conversation, rather than promising continued work across graceful shutdown.
Local standalone passthrough remains excluded from automatic survival recovery.
The local `features.agentSurvival` setting does not govern SSH retained-process
inspection or explicit Resume. This scopes `AC-EXECUTORS-SURVIVAL-005.3` to the
standalone capability described by its owning specification.

## Compatibility and visible behavior

| Shape | Selected transport | Failure behavior |
| --- | --- | --- |
| Local/worktree passthrough | Existing local runner | Existing local behavior |
| SSH plus protocol version 1 | Remote controller | Safe remote startup or recovery error |
| SSH plus old/unknown helper | None | Explicit capability error, no local fallback |
| Other remote executor | No new support in this package | Must not inherit the SSH adapter or use a local fallback |

The desktop terminal and phone Chat terminal keep the existing
`PassthroughTerminal` binary protocol. Phone behavior follows
`MobileTerminalKeybar`, manual input routing, touch scrolling, canvas rendering,
and visual-viewport positioning. Existing translated recovery controls display
safe typed causes. If additional product copy is needed, it must enter all
required locale catalogs through the existing i18n contract.

Phone Chat shows the existing keybar for agent passthrough and reserves its
44-pixel height above task navigation. Its existing visual-viewport and safe-area
positioning remain authoritative. Terminal output owns the only content scroll.
Agent senders are bound to a session and socket. Disconnect or replacement clears
only that sender. Agent keys cannot fall back to the workspace shell. Desktop
retains its composition. Keyboard input remains immediate. E2E coverage must prove actual remote output and
accepted input on both viewports, not only terminal visibility.

## Verification

Permanent regressions first prove the missing standalone-port failure and local
execution bug through real lifecycle entry points. Do not seed a fake standalone
port in SSH tests. Then prove remote configuration addresses, file ownership,
authenticated start and stream, prompt deduplication, and process-group cleanup.

Use existing real-SSH container fixtures for end-to-end startup with no prepare
script. The remote mock CLI records a bounded test-only marker containing its
PID, cwd, and received prompt IDs. Assert remote files and remote PID alongside
visible terminal results. A host marker must remain absent.

Use desktop and Pixel 5 contexts in the guarded `containers` project. SSH phone
specs retain the `mobile-` filename but are excluded from `mobile-chrome`, because
every test requiring a Docker-backed SSH target belongs to `containers`.
Ensure `ssh-test-base` forwards device options when it creates its context.
Validate actual phone viewport, touch mode, keybar input, and document overflow.

## Delivery

See the [implementation package](../../../plans/ssh-passthrough/plan.md).

### Ended session recovery surface

A failed, cancelled, or completed passthrough session uses the existing Chat
recovery surface. This exposes the current Resume, workspace restore, and
new-session actions on desktop and phone. Resume restores terminal interaction
when the session starts. Displaying this recovery surface does not change the
session passthrough snapshot or select ACP execution. The phone agent keybar
is hidden while the session has ended.


## Preparation failure recovery

Hold the ownership-registry lock through file preparation and the registry save.
If either fails, reverse the completed writes before releasing the lock. Remove
new files and restore prior file contents only while the written file identity,
size, and modification time still match. Preserve an external replacement or
edit. Return rollback errors with the preparation error.
