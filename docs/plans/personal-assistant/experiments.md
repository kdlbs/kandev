# Existing orchestration: read-only review and experiments

Date: 2026-09-16. This is a characterization report, not evidence that the proposed personal assistant is implemented.

## Boundary

Inspected the pre-rebase orchestration prototype and preserved existing changes. Production was not restarted, reconfigured or sent experimental messages. No Bitwarden items or secret values were retrieved. Browser fixtures used their own temporary database, repositories and mock agent. Build outputs were refreshed; production source and permanent tests were not edited during this review.

Temporary characterization tests were created, run and removed. They asserted the current deficient behavior, not the desired behavior. Their passing result means a gap was reproduced, not fixed.

## What the existing build already does

Global roles plus workspace assignments; pinned execution profiles; normal Kanban delegation/adoption; persistent conversation/comments; bounded memory; scoped run/session credentials; task-state callbacks; explicit retry; scheduled delivery to the same conversation; independent Orchestration and Office flags; desktop/mobile navigation.

The standalone browser flow exercised role creation and update, two orchestrator assignments, chat submission and streamed mock reply, profile preservation, mobile/desktop navigation, task backlink/callback and feature/Office isolation. The scheduled browser flow exercised a manual run of a configured automation and its linked conversation reply. Neither test demonstrates autonomous reasoning or real external integration use: the mock agent responds without deciding how to inspect a real account.

## Reproduced gaps

| Observation | Evidence | Consequence |
| --- | --- | --- |
| Worker waiting/permission/turn events did not queue a coordinator run while its task remained IN_PROGRESS. A REVIEW task-state event did queue one. | Temporary TestAssistantReviewProbeWorkerInputDoesNotWakeConversation; runtime/events.go | A question can wait unseen without a board movement. |
| An older confirmed-style user preference disappeared from the initial prompt after eight newer task memories, while remaining stored. | Temporary TestAssistantReviewProbeNewMemoryDisplacesUserPreference; sqlite/memory.go | Persistent storage alone does not make important knowledge reliably available. |
| A queued message's exact original instructions disappeared after 100 newer comments. | Temporary TestAssistantReviewProbeQueuedMessageFallsOutOfContext; runtime/service.go | The claimed run can lack its own original request and restrictions. |
| Automation then workspace-chat tests in one worker left a third orchestrator where the latter expected two. Fresh standalone workspace-chat passed. | Combined browser run: 1 passed / 1 failed; standalone: 1 passed | Fixture reset is incomplete/order-sensitive; increasing retries would mask it. |

Additional source findings (not experimentally established end-to-end):

- Runtime create-task input omits workflow_step_id even though WorkspaceTaskSpec and the canonical adapter support it. Explicit lightweight execution routing needs an end-to-end contract, not guessed workflow labels.
- WorkspaceCatalog exposes workflows/repositories, and runtime appends execution profiles; it is not a unified integrations/plugin/MCP capability directory.
- Plugin tool definitions support kanban-task and office-task surfaces, while the native MCP profile also has conversation. Extending applicability must be explicit and preserve per-call authorization.
- Worker result inspection is bounded by recent messages and latest-session selection. This is insufficient to enumerate all outstanding structured requests in multi-session tasks.
- Coordinator memory is not automatically assembled into a shared versioned worker context packet.
- Human comment persistence and run enqueue are separate; original-comment lookup is a bounded scan. Durable intake/idempotency and exact lookup need first-class handling.
- Existing e2e_reset.go clears run/routing state but not orchestrator registrations; it also overwrites workspace-profile routing settings with Office-style inheritance. Scope the fix to fixture-owned records and preserve ordinary seeded execution profiles.
- Repository policy itself mandates a separate design-package turn for non-trivial changes. Avoiding the workflow's Requirements stage alone will not remove that policy. Changing such policy is separate authorized work.
- Permission failures need structured evidence to distinguish classifier defects, provider policy, missing attachment and authentication; no classifier repair is established by this review.

## Historical checks and outcomes

The orchestration package suite, three temporary characterization probes and
selected runtime/task-adapter regressions passed. The probes demonstrated gaps;
they were removed afterward and are not permanent regression tests. Some selected
packages reported no matching tests, which was not counted as additional coverage.

The combined coordinator/Automation browser run originally produced one pass and
one fixture-isolation failure; the standalone coordinator flow passed. Fresh
backend/mock-provider/web builds were used. The reset gap was subsequently fixed
in the v0.94.0 integration: both specs passed together without retries. See the
[rebase validation](../../review/orchestration/rebase.md) for the later checkpoint.

The initial design handoff validated all eleven work-order dependencies/links,
Office-independent runtime imports and whitespace. Fixture processes were stopped
and production remained unchanged. Raw commands, temporary paths, process IDs,
failure screenshots and local toolchain details are intentionally not published.
Use each current work order's commands for reproducible continuation.

## Product comparison

OpenClaw's main-session model provides a continuous personal conversation to which delegated results can return; work references do not by themselves grant access. That is a useful interaction pattern for this assistant, not a reason to replace Kandev's task engine. [Official main-session documentation](https://docs.openclaw.ai/concepts/main-session).

OpenClaw also documents periodic and targeted wakes with bounded/no-op handling. Kandev should combine event-driven wakeups with deterministic reconciliation of its own pending requests, session state and receipts, so unchanged work does not repeatedly invoke a model. [Official heartbeat documentation](https://docs.openclaw.ai/gateway/heartbeat).

The distinction from Office is product accountability: Office models an organization of agents and its own work relationships. The personal assistant owns continuity with a person and their objectives, and uses existing profiles, tasks, integrations and approvals as tools. It must run with Office disabled and retain no Office runtime dependency.

## Next experiments, after implementation

Use synthetic read-only fixtures before enabling a live assistant:

1. Ask which tasks need the user; validate every answer against canonical request IDs, including idle sessions with no question.
2. Ask for a repository/PR summary; verify scoped evidence and zero delivery tasks/plan documents or external mutations.
3. Delegate the same credential-dependent lookup twice; use a fake credential resolver and verify both tasks receive only the scoped descriptor.
4. Answer a worker question in the central chat; verify exactly one native resolution and no new task.
5. Drop a send acknowledgement, replay an event, restart the backend, and revoke a tool/grant; verify deduplication and current state.
6. Try a write through native, plugin, external MCP and provider-native paths while read-only; require denial at every available dispatch boundary.
7. Inject three synthetic permission incidents; verify one improvement candidate and no policy change without the maintenance grant.
8. Only after enforceable read-only capability checks pass, perform a real-provider trial against an isolated fixture. Record actual tool receipts and baseline diffs; a plausible answer alone is not acceptance.

No real-provider capability/credential trial or permission-classifier repair was completed in this design turn.
