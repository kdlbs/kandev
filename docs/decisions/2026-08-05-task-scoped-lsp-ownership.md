# ADR-2026-08-05-task-scoped-lsp-ownership: Own Language Servers by Task and Language

**Status:** accepted
**Date:** 2026-08-05
**Area:** backend, frontend, protocol, security

## Context

The first LSP implementation used a browser WebSocket keyed by session and language as both the
JSON-RPC client and the language-server process owner. Editor mounts leased that connection, an
idle timer released it, and closing the browser-to-backend stream stopped the task-host process.
That model duplicates servers across sessions or browser windows, hides task work when the active
file changes, and can repeatedly pay Kotlin's expensive project-import cost even though all
sessions are working in one task environment.

A Kandev task resolves one canonical `TaskEnvironment` for its sessions, while task archive,
deletion, and environment teardown already own runtime cleanup. An inherited-workspace task can
use another task's physical environment, so physical hosting is not proof of product ownership.
LSP policy and runtime therefore need the task boundary without exposing the task environment, an
execution, or a session as product ownership.

## Decision

Kandev owns exactly one logical language-server lifecycle for each `(task_id, language)` pair.
Task policy, desired state, lifecycle evidence, progress projection, capacity admission,
authorization, and user controls use that key. A session ID, task-environment ID, execution ID,
browser attachment ID, editor model, or file tab may be carried inside a transport adapter, but
none of them may become an ownership key or appear in the task LSP product contract.

Ownership is split across two cooperating layers:

- The main backend's `internal/lsp` control plane owns task authorization, durable per-language
  policy and evidence, effective-policy resolution, process-wide capacity, task lifecycle hooks,
  recovery, and task-scoped HTTP/WebSocket projection.
- The canonical task host's `agentctl` instance owns one runtime slot per task and language. That manager
  owns binary discovery or installation, the process tree, the single upstream JSON-RPC peer,
  `initialize`/`initialized`, server work-done progress, capability and diagnostic caches, open
  document arbitration, and downstream browser attachments. Its instance and execution identity
  remain internal runtime details.

The task-host runtime continues after an attachment, browser, or backend watch stream disconnects.
A backend reconnect first adopts an existing task-host generation and opens a non-owning status
watch; it starts a new generation only when the old task-host manager proves that no live
generation exists. Task-host `Start` and `Restart` commands carry a backend-issued monotonic
generation and are idempotent, which closes timeout/retry races. The agentctl instance idle reaper
must treat an active task LSP runtime as owned background work.

Loss of the backend's in-memory task-host handle is not proof that the physical host is absent.
Both read and ensure paths first perform an executor-specific, existing-only reattachment that may
not launch, resume, or provision resources. Only an authoritative not-found result permits a new
task host. That probe uses only stable task/environment identity and physical reconnect data; it
does not resolve launch-time executor profiles, repository credentials, managed-cache state, or
remote-resume preflight. Those inputs remain mandatory for a genuinely new launch, while an
inspection error still fails closed. Backend startup reserves every durable generation that may
still own a process before any fallible task-host inspection and before controls, settings work,
attachments, or discovery can launch competing work. Reconciliation releases a reservation only
after the task host proves that runtime absent; inspection failures and error snapshots remain
capacity-bearing unless their evidence proves process absence.

Process absence is durable generation-scoped evidence, separate from the user-visible phase and
error. A later transport, watch, or reconciliation error cannot erase that evidence and create a
phantom capacity occupant after backend restart. Allocating a new generation clears the prior
absence proof. Conversely, Restart retains the existing task/language reservation if task-host
reattachment fails before the replacement command, because the previous generation may still run.

The durable startup inventory is the capacity ledger's authority. If that inventory cannot be
read, the controller records one sticky startup failure and every task-LSP operation fails closed
with that error until backend restart; it never exposes an empty ledger as ready or launches work
behind a speculative retry. Once inventory succeeds, all possibly-live generations are reserved
before later per-runtime inspection errors can occur. When cleanup records generation-scoped
absence or an inventory row disappears before its serialized reload, reconciliation releases only
the exact stale or proven-absent generation before runtime admission. Task cleanup or deletion
therefore cannot restore a phantom capacity occupant, while newer live evidence remains protected.
A newer accepted queued generation is also authoritative over an older startup inventory generation
for the same task/language: stale adoption cannot delete the queue entry, reactivate the retired
generation, or admit a server above the configured limit.

Startup watch registration uses the durable rows read after reconciliation, never the stale rows
that preceded convergence. A watch failure can replace state only while the current durable phase
still represents a server-bearing generation; `off`, `queued`, `waiting_for_task`, and
`unsupported` remain authoritative without a task-host watch.

A fired recovery owns the `(task_id, language)` command lane for its complete epoch revalidation,
durable-state reload, runtime inspection or task-host recovery, and optional relaunch. Canceling its
recovery identity before lane entry makes it a no-op; an explicit command ordered after lane entry
remains final rather than being stopped by stale recovery intent. Recovery demand arriving while an
attempt is still completing is retained as one pending wakeup and consumes the next bounded backoff
after that attempt; the handoff between command-lane release and recovery bookkeeping cannot lose
the only wakeup for a still-desired server.

Browsers attach through a task-and-language route after task-owner authorization. An attachment
receives the initialized server capabilities and then exchanges feature requests and document
notifications through the task-host multiplexer. Request IDs are namespaced by attachment and
generation. The task host sends only one upstream `didOpen` for a canonical file URI, orders
accepted changes with one monotonically increasing server version, releases the upstream document
after the last attachment closes, and never stops the language-server process merely because no
documents or attachments remain. Browser-originated file URIs are canonicalized against the
backend-authorized task workspace and repository roots before upstream traffic; traversal, sibling
paths, and symlink escapes are rejected at the task-host boundary. During trusted task-host
configuration, the task host canonicalizes each backend-authorized root, resolves the selected root's
canonical identity from that same pinned handle, and pins that OS root until the workspace projection
changes or the generation closes. It never resolves one pathname identity and then reopens that name
as the trusted root. Browser attachment and document traffic cannot open, replace, or rebind those
roots. The task host first proves lexical root and authority/volume membership without
filesystem access, then performs only handle-relative component inspection. Every symlink or reparse
target is projected back into an authorized canonical root before the target is inspected; a valid
cross-root target switches to that root's pinned handle. These rooted operations fail closed when an
ancestor is replaced during authorization, and an original or redirected untrusted UNC authority
cannot trigger DNS or SMB work during rejection. A missing document tail remains valid after the
first not-found component without further lookup. The language server ultimately consumes a pathname,
not an open file handle, so replacement after authorization and before server use is contained by the
task environment rather than claimed as an atomic file-open guarantee.

`TaskEnvironment` is the sole runtime target for the task-scoped owner. Multi-repository tasks
initialize one server from the task workspace root with the task's ordered repository roots as
workspace folders. Every session of that task uses the same target. When tasks share one physical
environment, the backend first proves membership from durable task/session state, then the task
host keeps independent `(task_id, language)` slots, workspace projections, progress, generations,
and processes. A task ID travels to agentctl only in the authenticated backend transport header;
it is never accepted from a browser or control body. Replacing the environment requires the old
generation to be reaped before a new target may launch; an unproven old process blocks the new
launch rather than risking duplicate imports. Changing the physical task workspace root requires
an explicit language-server restart: `workspace/didChangeWorkspaceFolders` may change repository
membership but cannot rebind the process working directory, initialize-time `rootUri`, or attachment
authorization scope of a live generation.

The backend persists one `task_lsp_languages` row per materialized task/language policy or runtime
record, with a composite primary key and task cascade. The row holds policy, detection summary,
lifecycle phase, generation, revision, timestamps, reason codes, initiator category, and current
error evidence. Active LSP progress, open documents, capabilities, and diagnostics remain
generation-scoped task-host runtime state and are re-adopted when that task host survives.

All human routes call task-owner authorization before reading policy, environment, execution, or
runtime state. The shared controller stamps `user`, `agent`, or `automatic` origin metadata; HTTP
clients cannot choose it. A future in-session MCP adapter must derive the task from its trusted
`AgentExecution`, then invoke the same controller without accepting a task ID, session ID, or
origin from tool arguments. This ADR defines that seam but does not add an MCP tool.

Capacity counts task/language runtimes that are starting, live, or stopping, not browser streams
or editor leases. Unsupported executors are rejected before capacity acquisition or execution
creation. Local PC/Worktree and Local Docker remain supported; Remote Docker, SSH, and Sprites
fail closed. Language-server commands still come only from Kandev's existing registry and managed
cache or an absolute task-host `PATH`; project-controlled binaries remain forbidden.

Releasing capacity atomically reserves the next queued task/language slot, then dispatches that
promotion as controller-lifecycle-owned work. The Stop or cleanup that freed a slot never waits for
another task's installer or process launch. Cancellation or failure before promotion begins returns
the reservation exactly once and considers the next queued entry. Controller shutdown removes
owned promotion work still queued behind detached request commands and joins any promotion already
running.

Every fired recovery and ready-budget-reset timer claims controller-lifecycle ownership before
performing store, task-host, or command-lane work and revalidates its timer epoch. Shutdown cancels
the lifecycle while callback admission is locked, stops pending timers, and joins callbacks that
already fired. Recovery commands keep the lifecycle context instead of detaching from it, so no
timer callback can outlive a successful controller shutdown.

All concurrent and retried shutdown callers wait on one lifecycle-generation completion signal.
A caller timeout does not make later shutdown calls report success while owned work is still
running. Recovery, ready-reset, and discovery timers also capture their immutable registration
object in addition to an epoch, so a delayed callback from a deleted registration cannot consume a
replacement registration whose local epoch restarted at the same number.

Task stop, archive, and delete cancel recovery and stop every language namespace owned by that
task. Cleanup of a borrowing task cannot terminate another task's host or language slots. When a
departing task owns a physical environment that another live task still uses, ownership transfers
to one live borrower before the task mutation and the shared task host remains alive; only the
departing task's slots stop. Once those processes are proved stopped, the surviving host purges the
departing task's slots, snapshots, subscriptions, diagnostics, capabilities, and workspace
projection without touching another task's namespace. When no live borrower remains, cleanup reaps
the full task-host process tree. Task-environment teardown remains the final unconditional
full-process-tree owner. Policy
survives temporary task stop and archive so task resume can reconcile it; task deletion cascades
the rows. Backend shutdown drops watches without making a browser or watch stream the stop owner.
If the task host does not survive, recovery launches at most one new generation after the old
runtime is known dead.

Explicit Stop, Disabled policy, and terminal task cleanup are interrupting task-level transitions:
they cancel an accepted Start/install before waiting for its language lane or task admission lock.
Terminal task mutation publishes exclusive writer intent before cancellation so no new runtime
reader can slip between cancellation and cleanup admission. Cleanup then remains the final
process-tree proof. Non-terminal commands retain ordered per-language serialization. Automatic
task and settings reconciliation reloads current durable policy only after entering that language
lane, preventing an earlier inventory snapshot from reversing a newer accepted user transition.

Browser connection generations do not own attachment intent. The frontend manager retains a stable
task/language/session lease registry and rebuilds a replacement transport from every live lease, so
one session cannot disappear when another session wins reconnect. Open-document references retain
their originating session, and the final lease release drains that session's document references
before removing its routing membership; another live session therefore cannot retain an upstream
document that nobody owns. Task event subscriptions expose their server acknowledgement; the task
LSP view performs an authoritative refresh after that ACK, closing the gap between initial HTTP
hydration and live event registration.

Workspace configuration publication and live application are one serialized per-task transition.
No earlier refresh may apply its folders, attachment roots, or snapshot after a later desired
configuration has committed.

Terminal task mutation reserves its durable cleanup admission barrier before reading sessions,
worktrees, executions, or environment inventory. Every runtime-producing path, including queued
promotion and workspace-source refresh, holds the shared side of that same task/environment
barrier. An abandoned prepared reservation is cancelled deterministically after its bounded
transition window, including when a repository failure prevented its diagnostic marker from being
written. Inside agentctl, purging a task retires its slots before removing their map entries, so a
caller that already obtained a slot cannot launch an untracked process after purge.

A task-host execution becomes observable to the LSP control plane only after agentctl readiness and
runtime-credential persistence both succeed. Registration may happen earlier solely so rollback
and recovery retain the physical cleanup handle. Docker credential-handshake state follows the
physical container lifetime: warm stop retains it, confirmed removal evicts it, uncertain removal
retains it, and executor shutdown clears it.

## Consequences

File switches, panel switches, editor unmounts, session switches, multiple browsers, navigation,
and reload no longer restart a task's server or repeat a project import. Initialization and
server-reported work remain observable even when no editor exists, so task-level status can be
stable and honest. Explicit Start can warm a task before a matching file is opened, while explicit
Stop becomes a durable task policy rather than a browser-local override.

The task host becomes a small LSP client and multiplexer instead of a byte-for-byte, connection-
owned stdio bridge. It must arbitrate shared documents, remap concurrent request IDs, cache
generation-scoped evidence, and expose idempotent control and watch contracts. The backend must
reconcile durable desired state with task-host reality and integrate with task cleanup. These are
more moving parts, but they are required to make persistence and deduplication true rather than a
UI illusion.

A surviving task-host process can preserve an import across a backend restart. A full task-host or
environment restart cannot preserve in-memory LSP protocol state and creates one new generation;
the UI reports that recovery rather than claiming reattachment. Concurrent edits of the same file
are ordered for analysis but do not turn Kandev into a collaborative editor.

## Alternatives Considered

- **Keep browser/session ownership and lengthen or remove the idle timer.** Rejected because a
  reload, second session, or second browser still duplicates or kills the process, and lifecycle
  truth remains invisible without a mounted editor.
- **Persist browser-owned status in the backend while leaving JSON-RPC in the browser.** Rejected
  because persisted labels cannot reattach to the initialized protocol peer, preserve work
  progress, or prevent a browser disconnect from destroying the real owner.
- **Put the whole control plane only in the main backend.** Rejected because the task host already
  owns process-tree safety and can preserve the JSON-RPC peer across an ordinary backend restart;
  keeping runtime multiplexing beside the process also avoids routing every ownership decision
  through a transient browser stream.
- **Use one server per session or browser and aggregate only the UI.** Rejected because it hides
  duplicate resource use and repeated project imports rather than preventing them.
- **Own servers by physical workspace, environment, or repository.** Rejected because task
  environments can carry task-specific worktrees, branches, source sets, and executor policy.
  A physical host may be reused, but sharing one language-server lifecycle across tasks would cross
  filesystem and authorization boundaries.
- **Let a future MCP tool start its own hidden server.** Rejected because it would create a second
  lifecycle and let agent-supplied identity bypass task ownership. MCP must use the shared
  controller when that product surface is designed.
