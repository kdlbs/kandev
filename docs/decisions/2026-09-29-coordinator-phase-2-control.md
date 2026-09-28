# ADR-2026-09-29-coordinator-phase-2-control: Coordinator phase 2, a person approves everything

**Status:** proposed
**Date:** 2026-09-29
**Area:** backend, frontend, protocol, security, workflow

## Context

[ADR-2026-09-26-workspace-coordinator](2026-09-26-workspace-coordinator.md)
(the phase-1 ADR) ships an attended coordinator that reads a Kanban workspace
and proposes tasks. It defines the coordinator permission model (D17) and
hardcodes one policy for every coordinator: six reads `automatic`,
`propose_task_kandev` `requires approval`, everything else `denied`. Its phase
plan puts the following in phase 2 (Control), behind gate G2:

- the D17 settings, and Watches, May do and Standing orders per coordinator;
- the "What it did" log with Undo;
- guided setup;
- a goal, the goal note and baselines;
- resume, message and move as proposals, with Resume on stall cards and
  prompts on pull requests that are ready;
- the copilot launcher on every page and the page context chip.

Gate G2 requires this ADR to record six decisions:

1. how D17's settings are stored and edited;
2. D13 (no `automatic` before the log);
3. the log's storage and retention;
4. D11 (Expand into a Quick Chat tab);
5. D12 (page context);
6. the goal and baseline model.

Phase 2 keeps the phase-1 rule that a person decides every write. Nothing
the coordinator does runs without a manager's decision, and nothing wakes it
unattended; waking is phase 3.

The phase-2 design package is:

- requirements under `docs/specs/coordinator/requirements/`:
  - `permissions.md`
  - `standing-orders.md`
  - `goals.md`
  - `activity-log.md`
  - `proposal-kinds.md`
  - `copilot-everywhere.md`
  - additions to `coordinators.md`
- the matching system designs;
- the plan in `docs/plans/workspace-coordinator-p2/`.

## Decision

### Decisions for phase 2

Decisions D1 to D17 are the phase-1 ADR's. Phase 2 settles D11, D12 and D13
and adds D18 to D25.

| # | Question | Decision | Status |
| --- | --- | --- | --- |
| D11 | Expand opens a Quick Chat tab of kind `"coordinator"`, or is dropped | **Dropped.** The copilot is a resizable right-side panel available on every page of the workspace (D12). A Quick Chat tab would put the same conversation in a second surface with its own lifecycle, and it would need a third `QuickChatSessionKind`. The mockup's "Expand into a Quick Chat tab" (v21-05) is not built. | proposed, G2 |
| D12 | Page context | Ids only. A page offers one chip, `{kind, id}`, with kind `task` or `workflow`. The client composes it into the phase-1 Ask about this prefix format, which carries the reference and a label: the task's display identifier, or for a board the workflow's name (a name the manager gave the board, not task content). Never a task title or any content. No chip is offered until its label has loaded, so a message never carries a placeholder label. The client attaches only ids of the active workspace. The server needs no new check, because every read the coordinator makes with that id passes the workspace and Watches guard: a foreign or unwatched id reads as not found. | proposed, G2 |
| D13 | When a write action may first be `automatic` | Unchanged in substance: no write action is `automatic` in phase 2. The setting's value space includes `automatic` from the first stored policy, the May do UI shows it disabled, and the settings route refuses it with `automatic_not_available`. Phase 3 chooses the first `automatic` action from at least 30 days of log rows (D20) and lifts the refusal for that action only. | proposed, G2 |
| D18 | Storage and editing of D17's settings | A policy per coordinator, shaped like the managed tool policy of ADR-2026-09-25 (#3994): a principal, a revision and an allowlist. **Stored value:** columns on `coordinators`: `policy_json` (a versioned action-to-setting map, NULL meaning the phase-1 policy), `policy_revision` and `watch_scope`, plus `coordinator_watches` rows. **Bound value:** at conversation open the service derives `CoordinatorToolPolicy{coordinator_id, workspace_id, conversation_task_id, policy_revision, tool_names}` and binds it to the conversation task's metadata under `kandev.coordinator_tool_policy`, as the managed tool policy is bound under `kandev.managed_tool_policy`. **Editing:** only through the coordinator settings routes, which require `workspace.manage`; the coordinator's surface has no route to it. A later move to a plugin maps `tool_names` onto the managed policy's `agent_tool_names` mechanically. | proposed, G2 |
| D19 | How registration and the guard read the policy | The phase-1 tool profile becomes the output of the policy. **Registration:** `registerCoordinatorTools` registers the bound `tool_names`. **Auto-approval:** agentctl auto-approves exactly those names. **Guard:** the MCP guard allows an action only when both hold: the action is in the bound list, and the coordinator's current stored policy still allows it. So tightening takes effect on the next call, even in a running session. Loosening takes effect at the next conversation, because a policy change archives the conversation. **Failure:** a missing binding on a conversation opened before phase 2 means the phase-1 profile; an unparsable binding refuses every coordinator action. | proposed, G2 |
| D20 | The "What it did" log | Table `coordinator_activity`, append-only apart from an undo marker and a refusal counter. **Rows:** one per coordinator action and decision: proposed, approved (with whether it was edited), rejected, failed, refused, undone. **Written:** in the same transaction as the state change it records. **Refusals:** coalesce per coordinator, action class and reason for 60 seconds. **Retention:** 400 days, so that phase 3's 30-day window and a year of comparison fit. Rows are deleted with their coordinator and workspace. **Read:** a summary route counts outcomes per action class over 1 to 90 days, used by May do and by phase 3. | proposed, G2 |
| D21 | Goal and baselines | **Goal:** at most one active goal per coordinator: a name, an optional due date and up to 10 exit criteria, with "Mark milestone met". **Baseline:** frozen on the goal row when a new goal is set: open watched tasks now, plus proposals approved and rejected in the 7 days before. The log-derived measures show "No baseline" when the coordinator is younger than 7 days at that moment. **Display:** a measure shows a direction only when it moved by 2 or more; otherwise "No direction yet". No new task history table: the log and live task rows are the sources. | proposed, G2 |
| D22 | Release toggle | Phase 2 ships behind its own `features.coordinatorPhase2` (`KANDEV_FEATURES_COORDINATOR_PHASE2`), effective only when `features.coordinator` is also on, whether or not phase 1 has been promoted first. Off means the phase-1 product exactly; phase-2 data is kept. | proposed, G2 |
| D23 | Proposal kinds, and D15 with a start permission | **Kinds:** a proposal has a `kind`: `create_task`, `resume`, `message` or `move`, all through the phase-1 record, claim, decision routes and card. **Start permission:** `start_agent` has no tool of its own. It governs whether a create or move proposal may target an agent-starting step, one that is not eligible under D15. With `start_agent` `denied` (the default), D15 holds unchanged. With `start_agent` `requires approval`, such a proposal is allowed, and its card says that approving it starts an agent. Approving it is the start decision. **Execution:** resume, message and move run at most once automatically; a claim that goes stale settles `failed` and is never re-run without a new approval. | proposed, G2 |
| D24 | Standing orders, goal and the conversation | Standing orders and the goal are text given in the standing instructions when a conversation starts. Adding, retiring or restoring an order, or changing the goal's name, due date or criteria, archives the conversation, as a context change does, so the next one starts fresh. Orders grant nothing: the guard decides what can run. | proposed, G2 |
| D25 | Stop | The `stop` action is shown in May do and stays `denied` in phase 2. The settings route refuses any other value, because no stop proposal kind exists yet. A later phase adds the kind and lifts the refusal. | proposed, G2 |

### What phase 3 reads from phase 2

Phase 3 (card "Coordinator P3 · Autonomy design package") is specified in
parallel and depends on these contracts, which phase 2 defines and does not
change after G2:

1. **The policy.**
   - `coordinator.Service.Policy(ctx, coordinatorID)` returns `{coordinator_id, workspace_id, policy_revision, actions, watch_scope, workflow_ids}`.
   - `actions` maps each of `create_task`, `start_agent`, `message`, `move`, `resume` and `stop` to `denied`, `requires_approval` or `automatic`.
   - Phase 3 lifts the `automatic_not_available` refusal for the action it chooses.
2. **The log.**
   - Table `coordinator_activity`, with fields `action_class`, `outcome`, `edited`, `undone_at`, `proposal_id`, `target_task_id` and `created_at`, kept 400 days.
   - `GET .../coordinators/:cid/activity/summary?days=N` returns per-class counts of `proposed`, `approved`, `approved_with_edits`, `rejected`, `failed`, `refused` and `undone`, plus the earliest row time.
   - Phase 3's "at least 30 days of log rows" is measured from that earliest row time.
3. **The executors.** Each proposal kind has one execute function behind the claim, `Execute(ctx, claim) (Outcome, error)`. An automatic action in phase 3 runs the same function with authorisation `automatic` rather than a second code path.

Phase 3's wake, containment and cost ceiling are not specified here.

### The three rules, restated

The three rules of the phase-1 ADR hold in phase 2 and are now enforced
against stored settings:

- **One approval decides one proposal.** Approving grants nothing further.
- **The runtime never changes its own permissions.** The coordinator's surface has no tool that reads or writes its policy, watches, standing orders or goal.
- **Settings change only through the settings routes.** A manager writes them there, and the guard re-reads them on every call.

### Residual risks carried forward

The phase-1 ADR's residual risks are unchanged:

- the agent's own tools can reach the REST API or `/mcp` when auth is off;
- untrusted board content can steer the agent;
- a failed archive leaves the old session running until restart.

Phase 2 adds one. A policy change archives the conversation, and until that
archive completes the old session keeps its bound tool list. The guard's live
policy read closes that window for tightening. Loosening never needs closing,
because it only adds tools to the next session.

## Prior art

- **Author's wiki (`wiki-query @henry`).** The configuration resolved to
  `OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki` with QMD collection
  `wiki`. QMD was unavailable: no `qmd` CLI and no QMD MCP tool. The grep
  fallback was refused by the sandbox ("Operation not permitted" on the vault
  path), so this pass read nothing new. The phase-1 ADR's reading of
  `concepts/conductor-loop.md` still applies: deterministic gates hold
  authority while models draft and explain.
- **Devin permissions.**
  - What it does: deny, then ask, then allow rules; modes from Normal to Autonomous; organisation deny rules that no mode overrides; "allow once, for the session, for the project".
  - Adopted: denied beats everything, and a person's decision covers one action.
  - Differs: no "allow for the session" from an approval. Approval never widens the policy, so a grant is always a settings change.
- **Warp agent profiles.**
  - What it does: a setting per action type (`always_ask`, `always_allow`, `agent_decides`), with a command denylist.
  - Adopted: a setting per action, not per tool name.
  - Differs: Warp lets the agent edit its own profile file on request. Here the coordinator can never read or write its settings (the second rule).
- **Factory.ai autonomy levels.**
  - What it does: tiered autonomy with an organisation-managed maximum.
  - Adopted: a maximum above the per-coordinator setting. In phase 2 that maximum is `requires approval` for every write (D13).
- **OpenHands confirmation policy.**
  - What it does: confirm always, never, or when an analyser judges an action risky.
  - Differs: no model judges safety. The stored setting and the guard decide.
- **Augment Code tool permissions.**
  - What it does: allow, ask and deny rules per tool, plus workspace rules and guidelines.
  - Adopted: workspace-level guidance text next to, but separate from, permissions. Standing orders never grant anything.
- **Paperclip activity log and board approvals.**
  - What it does: every mutation writes a permanent record of actor, action, entity, details and time. Approvals carry a decision note and never expire. The agent's spend gate is written in its instructions.
  - Adopted: an actor-and-action record per decision, and rejection reasons kept.
  - Differs: rows are kept 400 days rather than forever, and the gate is enforced by the guard, not by the agent's instructions. Paperclip's own guide warns that an instruction-level gate can be bypassed.
- **Multica autopilots.**
  - What it does: a runbook of goal, constraints and steps read on every run; run history with the reason for each skip or failure; creator and admin-only edits.
  - Adopted: a goal and standing rules given at the start of every conversation, and manager-only edits.
  - Differs: no schedule. Waking is phase 3.
- **What Kandev adds:**
  - the policy is bound to the session and re-read on every call;
  - a log row is written in the same transaction as the decision it records;
  - Undo is itself a logged write;
  - the first `automatic` action is chosen from the log (D13), not from a default.

## Consequences

- **Store:** the coordinator store gains four tables (`coordinator_watches`,
  `coordinator_standing_orders`, `coordinator_activity`, `coordinator_goals`)
  and additive columns on `coordinators` and `coordinator_proposals`. All are
  registered in `requiredstores` with an upgrade test from the phase-1 schema.
- **Tool profile:** it stops being a constant. The guard's table test covers
  every registered Kandev MCP action against every allowed combination of
  settings.
- **Starts:** creating a task and starting its agent stay separate. A start
  happens only on a card that says it will.
- **Merging:** merging a pull request and moving a task to Done stay human in
  every phase. The May do table shows both as "Always human".
- **Copilot:** its store moves from the Coordinator route to the workspace
  shell. The board's task preview and the copilot share the right side, one
  at a time.

## Alternatives considered

- **One policy per workspace.** Rejected by D17 in phase 1: two coordinators
  of one workspace may need different reach.
- **A row per action setting (`coordinator_action_settings`).** Six rows per
  coordinator, with no benefit over one versioned value that is bound and
  compared as a unit. A single value also maps onto the managed tool policy
  directly. Rejected.
- **Rebuild the MCP server when a policy changes mid-session.** The tool list
  of a running agent cannot be changed reliably across agent CLIs. The live
  policy read in the guard gives the same safety for tightening. Rejected.
- **Log in the task event stream instead of a coordinator table.** Task
  events have no retention guarantee and do not record refusals or rejected
  proposals, which never touch a task. Rejected.
- **A daily sampler for baselines.** A timer, a table and a gap-filling policy,
  all to draw a trend line that phase 2 does not show. Rejected in favour of a
  baseline frozen when the goal is set.
- **Keep Expand (D11).** It duplicates the panel. Rejected.
- **Manager Resume and Send it back as proposals.** A manager's own action
  needs no approval. They call the task system's routes as the manager and
  are not coordinator actions, so they write no log row.

## G2 Status

**Status:** pending. The G2 conditions are those of the phase-1 ADR's phase
plan. When G2 is met, this section records the maintainer reply on
[#3752](https://github.com/kdlbs/kandev/issues/3752), or "proceeding without
maintainer reply" with the date.
