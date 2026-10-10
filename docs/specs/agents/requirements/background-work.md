---
status: draft
system: agents
created: 2026-09-27
owners:
  - kandev
---

# Agent Background Work

## Purpose and ownership

Users inspect and control agent-owned background commands, jobs, and child agents
through one experience across protocols. The agents system owns provider
capabilities and workload identity. Task storage, runtime transport, and UI are
consumers of that contract. Codex app-server is the first native implementation;
ACP adapters expose only verified observations and operations. Future Claude
native support must use the same contract without a parallel UI or message model.

This replaces the unimplemented Codex-only draft's REQ-AGENTS-CODEX-BG-001,
REQ-AGENTS-CODEX-BG-002, and REQ-AGENTS-CODEX-BG-003. Those identifiers are retired,
not reused. Existing native-Codex, subagent-history, usage, prompt-admission, and
background-liveness requirements remain authoritative in their own scope.

## Terms

- **Workload:** an observed shell command, subagent, or monitor owned by an agent
  session. It is not a Kandev task, workflow step, or independently launched session.
- **Run:** one execution of a workload. A child thread can have successive runs.
- **Capability:** a verified operation or observable data shape supported for
  the connected provider and workload, subject to current state and permissions.
- **Unknown:** prior activity whose current liveness cannot be established.
  Unknown is neither successful completion nor a currently verified running count.

## Requirements

### REQ-AGENTS-BACKGROUND-WORK-001: Shared capabilities and compatibility

**Intent:** Make provider differences explicit without separate product surfaces.

- **AC-AGENTS-BACKGROUND-WORK-001.1:** When supported workloads are reported by different adapters, the system shall show the same kinds, lifecycle meanings, and action semantics without requiring users to select a protocol-specific viewer.
- **AC-AGENTS-BACKGROUND-WORK-001.2:** When a provider cannot supply an operation, output stream, hierarchy, reasoning summary, or usage attribution, the system shall indicate its unavailability and shall not fabricate data or attempt an alternative process-control mechanism.
- **AC-AGENTS-BACKGROUND-WORK-001.3:** When the shared feature is disabled or an older peer omits the new contract, existing chat, inline subagent history, background accounting, and admission behavior shall continue; new controls shall not execute.
- **AC-AGENTS-BACKGROUND-WORK-001.4:** When Codex native support is enabled, its supported capabilities shall be established against the selected protocol version; enabling this feature shall not enable unrelated provider experiments or override an explicit operator restriction.

### REQ-AGENTS-BACKGROUND-WORK-002: Identity and reliable lifecycle

**Intent:** Observe independent work across turns and reconnects without confusing ownership.

- **AC-AGENTS-BACKGROUND-WORK-002.1:** When a root turn finishes while a child or command continues, that workload shall remain visible under its original session and origin turn without producing another root completion or creating a Kandev task/session.
- **AC-AGENTS-BACKGROUND-WORK-002.2:** When duplicate, delayed, or reordered observations arrive, one logical workload/run shall be represented once; observations from an old run shall not mutate a successor run or another session.
- **AC-AGENTS-BACKGROUND-WORK-002.3:** When a browser reconnects or the backend/runtime restarts, retained history shall remain inspectable; unverified live work shall become unknown until the provider establishes current state. Failed or incomplete discovery shall not imply completion.
- **AC-AGENTS-BACKGROUND-WORK-002.4:** When a provider supplies nested parentage, the system shall display it. Missing or ambiguous parentage shall remain explicitly unresolved rather than attaching work to the most recently active turn.
- **AC-AGENTS-BACKGROUND-WORK-002.5:** When multiple source calls reference one child, counts shall count the child run once. Completed runs shall remain discoverable after all active work finishes.

### REQ-AGENTS-BACKGROUND-WORK-003: Scoped workload controls

**Intent:** Control a specific workload without disturbing unrelated work.

- **AC-AGENTS-BACKGROUND-WORK-003.1:** When an authorized user stops a command or interrupts a child with an advertised capability, only the selected workload/run shall be targeted; the root conversation and sibling workloads shall remain unaffected.
- **AC-AGENTS-BACKGROUND-WORK-003.2:** When a workload advertises input support, users shall be able to submit input and, separately when supported, close stdin. Unsupported input controls shall not be actionable.
- **AC-AGENTS-BACKGROUND-WORK-003.3:** When a control is stale, unauthorized, unsupported, or targets disconnected work, the system shall reject it before dispatch. A lost response shall be shown as uncertain and shall not cause automatic input replay or imply termination.
- **AC-AGENTS-BACKGROUND-WORK-003.4:** When a child requests approval or clarification, the existing permission/question flow shall identify that child and preserve cancellation and resolution ownership. Resolving a child request shall not resolve a sibling request.

### REQ-AGENTS-BACKGROUND-WORK-004: Output, history, and usage

**Intent:** Inspect available evidence without confusing logs with an interactive terminal or costs with estimates.

- **AC-AGENTS-BACKGROUND-WORK-004.1:** When output is available, users shall see retained command output or child transcript content and any newly supported streamed output, with explicit unavailable, gap, and truncation states. Reconnect shall not duplicate already displayed content.
- **AC-AGENTS-BACKGROUND-WORK-004.2:** When inspecting command output, users shall have ANSI-color-safe text rendering, search within retained output, pause/resume auto-scroll, and local clear. Clear shall not stop the process or delete durable conversation history.
- **AC-AGENTS-BACKGROUND-WORK-004.3:** When child summaries, tool activity, or reasoning summaries are exposed by the provider, the shared viewer shall reuse their normalized transcript representation; unavailable private reasoning shall not be inferred or promised.
- **AC-AGENTS-BACKGROUND-WORK-004.4:** When usage can be attributed to a child, the viewer shall use the existing accounting source, distinguish reported/estimated/unavailable values, and avoid adding the same measurement to parent and child totals twice.

### REQ-AGENTS-BACKGROUND-WORK-005: Desktop and phone experience

**Intent:** Keep background work accessible without changing how prompts are admitted.

- **AC-AGENTS-BACKGROUND-WORK-005.1:** When running, waiting, or unknown work exists, a small, content-width pill directly above the chat input shall show the total as a localized label such as "2 Background Jobs". Clicking, keyboard-activating, or tapping it shall open a summary with separate running, waiting, and unknown counts. A history entry shall remain reachable when only completed work exists.
- **AC-AGENTS-BACKGROUND-WORK-005.2:** When background state changes, typing and the user's draft shall be preserved. Send, queue, steering, and root-cancel behavior shall retain existing session-admission rules; background visibility alone shall not enable concurrent turns. Explicit same-turn input is governed separately by the [Platform steering contract](../../platform/requirements/explicit-turn-steering.md).
- **AC-AGENTS-BACKGROUND-WORK-005.3:** When opening details, selecting a job or agent on desktop shall open its own named detail tab in the central Dockview group. Different workloads shall remain open independently, selecting an already-open workload shall activate its existing tab, and closing a tab shall not stop work. View all shall open a separate Background Work overview; phone shall offer a focused list and full-height detail surface with Back navigation, visible actions, safe-area and keyboard clearance, and no document horizontal overflow.
- **AC-AGENTS-BACKGROUND-WORK-005.4:** When loading, empty, disconnected, unsupported, or failed states occur, each shall be distinguishable and accessible without color alone. Phone/coarse-pointer actions shall have at least 44px hit targets, and each content view shall have one vertical scroll owner with focus returned on dismissal.
- **AC-AGENTS-BACKGROUND-WORK-005.5:** When changing locale, new labels, action/error feedback, counts, and accessibility names shall use the existing translation system with all supported catalogs and correct pluralization.

### REQ-AGENTS-BACKGROUND-WORK-006: Delivery and compatibility evidence

**Intent:** Prevent a provider-specific success from being mistaken for portable support.

- **AC-AGENTS-BACKGROUND-WORK-006.1:** When enabling the feature for Codex, contract tests shall prove discovery, targeted termination, isolated child interruption, output fallback, and reconnection against the pinned schema, with real-provider evidence distinguished from deterministic fake-server coverage.
- **AC-AGENTS-BACKGROUND-WORK-006.2:** When an ACP adapter supplies existing normalized observations, it shall use the shared viewer with unsupported controls absent. A provider-neutral conformance fixture shall exercise supported and unsupported capabilities without protocol-name branches in the UI.
- **AC-AGENTS-BACKGROUND-WORK-006.3:** When running through local, Docker, SSH, or Kubernetes executors, controls and observations shall use the owning runtime; a remote process identifier shall never be treated as a host OS process identifier.

## Exclusions and related contracts

No new Claude-native adapter, invented ACP extension, distributed scheduler,
new task/session creation, arbitrary host PID control, process restart/re-run,
child prompting/steering, or replacement of ordinary user terminals is included.
Input is part of the shared contract, but Codex background stdin is unsupported
unless independently proven for thread-owned commands in the pinned version.

- [Design](../system-design/background-work.md)
- [Native Codex requirements](codex-app-server.md)
- [Subagent invocation history](subagent-context-persistence.md)
- [Existing subagent presentation](../../ui/requirements/subagent-observability.md)
- [Usage](../../costs/requirements/conversation-usage.md)
- [Prompt admission](../../tasks/requirements/queue-admission.md)
- [Session activity projection](../../platform/requirements/background-work-liveness.md)
