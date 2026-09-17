# Coordinator plugin source review

Reviewed 2026-09-17. Source: [yattdev/kandev-plugin-coordinator at 6fb1fdd63c1728258c40a67df037116c5ca9bbb8](https://github.com/yattdev/kandev-plugin-coordinator/tree/6fb1fdd63c1728258c40a67df037116c5ca9bbb8). The code, manifest, UI and relevant tests were read from a local checkout at that exact commit. No installation, plugin execution, or test run was performed. Findings below distinguish code behavior from claims that require a compatible host.

## Recommendation

Take the plugin's reusable lifecycle and reporting ideas, but build the accepted central task view on the current v0.94.0 coordinator foundation. Installing or copying this plugin would not deliver the requested task dashboard and requires host capabilities absent from our target release. A later core/plugin split is still a useful maintainer discussion; no such boundary has been approved upstream.

## What exists in the code

| Area | Observed implementation | Relevance |
| --- | --- | --- |
| Conversation | Stable workspace key `coordinator`; host SDK Ensure/Dispatch wrapper; explicit unavailable/configuration-required results. [Source](https://github.com/yattdev/kandev-plugin-coordinator/blob/6fb1fdd63c1728258c40a67df037116c5ca9bbb8/server/coordinator/conversations.go). | Good lifecycle boundary. Our assignments already own durable identity and chat; retain multiple assignments rather than collapse them to one key. |
| Schedule | Cancellable minute runner, configured timezone/day/window/cadence, stable daily/cycle occurrence keys, separate caller keys for manual runs, cycles armed after successful scheduled standup. [Source](https://github.com/yattdev/kandev-plugin-coordinator/blob/6fb1fdd63c1728258c40a67df037116c5ca9bbb8/server/coordinator/scheduler.go). | Reuse stable occurrence semantics and visible busy/failure handling in existing Automations. Do not add a second heartbeat runner to the coordinator page. |
| Policy selection | Reads monitored workflow steps and per-step prompts from the host on each cycle; stable ordering and no second policy copy. [Source](https://github.com/yattdev/kandev-plugin-coordinator/blob/6fb1fdd63c1728258c40a67df037116c5ca9bbb8/server/coordinator/workflow_policy.go). | Good single-owner principle. The host fields do not exist in v0.94.0, and workflow-specific monitoring policy is beyond the first central-view delivery. |
| Reports | Typed cycle/daily/status artifacts; report plus memory update saved as one workspace document; payload validation and bounded history. [Reports](https://github.com/yattdev/kandev-plugin-coordinator/blob/6fb1fdd63c1728258c40a67df037116c5ca9bbb8/server/coordinator/reports.go), [state](https://github.com/yattdev/kandev-plugin-coordinator/blob/6fb1fdd63c1728258c40a67df037116c5ca9bbb8/server/coordinator/state.go). | Useful follow-up: distinguish durable conclusions from delivery receipts and task state. A typed report feed is not currently implemented in our prototype. |
| Authority | Host-verified workspace for actions; agent tools additionally check the managed task/session. [Actions](https://github.com/yattdev/kandev-plugin-coordinator/blob/6fb1fdd63c1728258c40a67df037116c5ca9bbb8/server/coordinator/actions.go), [tools](https://github.com/yattdev/kandev-plugin-coordinator/blob/6fb1fdd63c1728258c40a67df037116c5ca9bbb8/server/coordinator/tools.go). | Keep server-derived scope and current-session checks. A task ID in a prompt or browser body is not a grant. |
| UI | One Integrations destination, chat/report tabs, manual cycle/standup buttons, settings link, workspace-switch guards and responsive control sizing. [Page](https://github.com/yattdev/kandev-plugin-coordinator/blob/6fb1fdd63c1728258c40a67df037116c5ca9bbb8/ui/src/coordinator-page.ts). | Reuse explicit unavailable/setup states and stale-workspace response guards. There is no task table/grouped workspace overview in this page. |
| Durable library | Separate SQLite current-state/mutation log/snapshot/compaction/replay/fencing package with focused tests. [Library](https://github.com/yattdev/kandev-plugin-coordinator/tree/6fb1fdd63c1728258c40a67df037116c5ca9bbb8/server/durablestate). | No production import of this library appears in `server/coordinator` or `server/main.go` at the pin. Active scheduler state uses Host GetState/SetState. Do not infer that scheduler operations already have the library's full recovery guarantees. |

## Compatibility against the requested release

The plugin's [CI](https://github.com/yattdev/kandev-plugin-coordinator/blob/6fb1fdd63c1728258c40a67df037116c5ca9bbb8/.github/workflows/ci.yml) pins host commit `ff9b8b8ecfd32a7ca00708bbbbff330dc9ccc7a7`. Its [Go module](https://github.com/yattdev/kandev-plugin-coordinator/blob/6fb1fdd63c1728258c40a67df037116c5ca9bbb8/go.mod) replaces the Kandev dependency with a sibling source checkout. This is not proof of compatibility with any released package.

A source search at both `v0.94.0` and `b1cd0d2e` found no corresponding declarations in the SDK, manifest, workflow or host UI implementation for:

- `AgentConversations` / `AgentConversationManager`;
- `capabilities.agent_conversation`;
- `WorkspaceAgentChat`;
- `coordinator_monitored` / `coordinator_prompt`.

The plugin directly references these interfaces and fields. It is therefore not source-compatible with the v0.94.0 SDK as written; its UI also expects the missing host chat export. This conclusion is from source inspection, not a compilation attempt. The README's dated release note is not used as evidence of current release support.

The [manifest](https://github.com/yattdev/kandev-plugin-coordinator/blob/6fb1fdd63c1728258c40a67df037116c5ca9bbb8/manifest.yaml) declares state, managed conversation and workspace/workflow reads, with no minimum Kandev release. Those grants concern the plugin process. They do not by themselves prove that the agent's provider-native shell/MCP tools cannot mutate data. Its prompt restrictions should not be described as enforced read-only execution.

## Concrete ideas to take

| Decision | Idea | How it maps to our scope |
| --- | --- | --- |
| Adopt in the central-view design | Stable conversation identity; native chat reuse; explicit profile/setup failure states; old workspace responses discarded. | Existing coordinator assignment remains the chat owner. Viewing/filtering tasks creates no new worker task or agent run. |
| Adopt in the central-view design | Separate operational state from agent-written reports. | Groups/counts come from canonical task/status projections. Chat prose never decides a task's state, PR completion or pending permissions. |
| Preserve in existing runtime/automation | Verified scope, idempotent occurrence identity, explicit busy/failure/uncertain outcomes. | Keep scoped credentials, run deduplication and `dispatched` semantics already implemented. No silent retry of uncertain external writes. |
| Follow-up, not silently added | Typed reports with bounded retention and explicit occurrence/run links. | Add a separate requirement/design if maintainers want a report feed. Reuse core persistence and permissions; do not turn report memory into an authoritative copy of task state. |
| Follow-up, not silently added | Workflow-owned monitoring selection. | Requires a reviewed workflow contract on v0.94.0 or a later agreed base; avoid copying unsupported host fields into local plugin settings. |
| Defer | Second scheduler, fixed default timezone/cadence, automatic arming policy. | Core Automations already supply opt-in schedules/timezones and manual delivery. Office has its own heartbeat, as the maintainer noted. |
| Do not import wholesale | SQLite state engine, vendored policy contract and bundled coordinator prompts. | Would duplicate storage/policy ownership and substantially widen scope. The central overview needs no new durable state engine. |

## Cautions when adapting the patterns

These are source-level observations and test gaps, not externally reproduced incidents:

- **Report pagination is offset-based.** `reports.go` encodes an integer offset in the cursor while new reports prepend to the list. A publish between pages can repeat or skip an item. A future feed should use an immutable key cursor/snapshot and test concurrent publishing.
- **Receipt publication is not idempotent.** `publishReport` uses a timestamp/type ID and prepends on each call; occurrence identity is metadata, not a uniqueness guard. Retrying a report publication can create duplicates. Use a stable run/occurrence key and content conflict behavior if adopting this feature.
- **Dispatch and plugin state are separate operations.** `dispatchAndRecord` calls host dispatch before updating the workspace document. Correct recovery therefore relies on host occurrence idempotency; it is not a cross-system transaction. Manual/scheduled retries should preserve and surface accepted/unknown outcomes.
- **Standup failure consumes that date.** The scheduler records `LastStandupDate` even for busy/error/duplicate results; only sent/queued/started arms cycles. Tests explicitly cover busy and duplicate not arming. This is a policy choice to review, not an automatic retry behavior to assume.
- **State update locking is process-local.** A workspace mutex surrounds Host GetState/SetState. Do not infer multi-process CAS/fencing from this path or from the separate durable-state library.
- **Fixed prompt invariants are still prompt text.** The code appends them after editable instructions. That is useful guidance, but any security-sensitive capability restriction still belongs at authoritative tool/execution boundaries. The vendored policy validator checks a static contract/default snapshot, not all live agent actions.

Relevant reviewed tests cover stable/DST occurrence keys, busy and duplicate standups, verified workspace/session context, report validation/retention, missing conversation capability and stale-workspace UI updates. They were read, not executed; no plugin smoke-test result is claimed.

## Reuse and attribution

The pinned repository's [LICENSE](https://github.com/yattdev/kandev-plugin-coordinator/blob/6fb1fdd63c1728258c40a67df037116c5ca9bbb8/LICENSE) is MIT. Preserve its notice if copying substantial source. This review adopts design ideas only; no third-party code, policy file or prompt has been copied into the implementation. All future demo prompts remain generic and synthetic.
