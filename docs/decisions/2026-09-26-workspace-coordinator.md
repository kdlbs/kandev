# ADR-2026-09-26-workspace-coordinator: Workspace coordinator as a core page

**Status:** proposed
**Date:** 2026-09-26
**Area:** backend, frontend, protocol, security, workflow

## Context

Kandev users who run several agents in one workspace need a single place that
says what needs them, why, and what clears it, plus a conversation that can
explain the board and suggest work. Kandev main has five pieces that point at
this without composing it: Office's coordinator role, the Needs-you Inbox, the
task-level `task.stalled` event, Configuration chat, and
ADR-2026-08-31's plan for a Coordinator plugin.

[ADR-2026-08-31-generic-plugin-host-boundary](2026-08-31-generic-plugin-host-boundary.md)
placed the Coordinator product in a separately released plugin and says core
must not gain a Coordinator table, field, profile, role, principal, grant,
setting, tool or audit vocabulary. The plugin needs Host contracts that are not
yet built. The feature described here needs none of them: it is a projection
of task facts Kandev already publishes, one ordinary Kandev session per
coordinator, and one write (a task proposal a person decides).

[ADR-2026-09-25-plugin-coordination-platform](2026-09-25-plugin-coordination-platform.md),
accepted after this ADR was first drafted, gives plugins the roles, policy,
memory, proposals and product composition of coordination, and gives core
the generic execution, managed conversation and restricted tool policy
contracts. Asked directly whether the workspace coordinator should be core or
a plugin, the kdlbs maintainer answered on 2026-09-28: "sounds good to be in
the core, we can move to a plugin later if needed. just have it behind a
feature flag so we can test it a bit". This ADR records that direction; the
[relationship section](#relationship-to-adr-2026-09-25-plugin-coordination-platform)
says what it does and does not change.

This decision places the workspace coordinator in core for Kanban workspaces,
behind `features.coordinator`, and records the phase plan. The phase 1
requirements and system designs live under
[`docs/specs/coordinator/`](../specs/coordinator/README.md); the delivery plan
is [`docs/plans/workspace-coordinator/plan.md`](../plans/workspace-coordinator/plan.md).

The phase 1 Needs you screen from the mockup (visual reference; the
requirements govern):

![Phase 1 Needs you: count strip, question, stall and error items, sidebar entry with its open-proposal badge](../plans/workspace-coordinator/assets/p1-01-needs-you.png)

## Decision

### Decisions for phase 1

| # | Decision | Phase 1 answer | Status |
| --- | --- | --- | --- |
| D1 | Where the product lives | A core Coordinator page for Kanban workspaces, behind `features.coordinator` so it can be tested before it is promoted. It may move to a plugin on the managed coordination platform later if needed. | agreed with the maintainer, 2026-09-28 |
| D2 | The runner | An ordinary Kandev session on an ephemeral task, started like Configuration chat, one per coordinator. It needs no automation, run queue or plugin, and has no wake. The wake is decided at G3. | proposed, G0 |
| D3 | Durable coordinator state | Its conversation task, its proposals and the stall records. No checkpoint, because nothing resumes it unattended. Revisited at G3. | proposed, G0 |
| D4 | May a coordinator move or archive cards? | Not in phase 1. Moving, archiving, deleting, stopping and resuming tasks are `denied` actions under D17, not banned in every phase: a later phase may allow moving, resuming or stopping under an explicit per-coordinator permission. Merging and moving a task to Done are not D17 actions; allowing either would be its own decision. This is a rule about the coordinator actor only; people, workflows and other actors keep their contracts. `product-constraints.md` records the rule. | proposed, G0 |
| D5 | How it asks a person | Through proposals, in its chat and on Needs you. `ask_user_question_kandev` is not on its surface. | proposed, G0 |
| D6 | ADR 0004's coordination-task pattern | Not used, and left unchanged. | proposed, G0 |
| D7 | The plugin's interim path | The v1 fence stays as ADR-2026-08-31 defines it. #3994 added the `managed-conversation` MCP surface and mode and the adapter-enforced managed tool policy; the `coordinator` surface, mode and origin coexist with them, and the agentctl permission handler checks the coordinator allowlist, then the managed tool policy, then the default. Other open PRs that touch `internal/mcp` are left to their own review: phase 1 needs none and blocks none, and whichever lands second rebases. Before adding a name, the implementation checks main for a name already taken. | proposed, G0 |
| D8 | Scope and cardinality | Workspace scope. Several coordinators per workspace, each reading the whole workspace until Watches narrows it in phase 2. Each proposal belongs to one coordinator. | proposed, G0 |
| D9 | Autonomy and approval | Phase 1 is attended: every turn starts from a manager's message. The coordinator's Kandev surface can only read and propose, enforced by the MCP guard and by auto-approving exactly those tools. The agent CLI's own tools are governed by its profile and settings, as in any Kandev chat; see [Residual risk](#residual-risk-the-agents-own-tools). Enforced containment of those tools is a condition of G3, before any unattended turn. | proposed, G0 |
| D10 | External intake (Jira, Linear, Sentry) and tracker write-back | Phase 4. | proposed, G4 |
| P | The mockup's phone "Teammate" persona | Not planned. `features.auth` is off in every shipped profile and clarifications have no addressee. | proposed, G0 |
| D14 | Kandev's Inbox | Keeps its row, count and tabs in every phase. The coordinator keeps its own count. | proposed, G0 |
| D15 | Approval never starts an agent | A proposal may target only an eligible step: one without an `auto_start_agent` on-enter action, and not a feeder (directly or transitively) of a step that has one, so an automatic queue/WIP promotion after the create cannot reach an auto-starting step either. Creating a task and starting its agent are separate actions under D17: approving a create never authorizes a start, including a start that the target step's on-enter actions or a queue or WIP promotion would trigger. | proposed, G0 |
| D17 | Coordinator permissions | Defined now, built in a later phase: each coordinator has a scope and a setting per action, each action `denied`, `requires approval` or `automatic`. Phase 1 hardcodes one policy for every coordinator: the six reads `automatic`, `propose_task_kandev` `requires approval`, everything else `denied`. See [Coordinator permission model](#coordinator-permission-model). Settles D16 in favour of per-coordinator settings. | proposed, G0 (implementation G2) |
| F | Flag | `features.coordinator`: `prod` and `dev` `"false"`, `e2e` `"true"`. Dogfooding uses the runtime override. Restart required. | proposed, G0 |
| N | Name | UI "Coordinator". Specifications say "workspace coordinator", and the Office glossary distinguishes it from Office's coordinator role. | proposed, G0 |

### Decisions for later phases

| # | Decision | Status |
| --- | --- | --- |
| D11 | Expand opens a Quick Chat tab of kind `"coordinator"`, or is dropped now that the copilot is a panel. | open, G2 |
| D12 | Page context is ids only (`{kind, id}`), validated to the workspace. | proposed, G2 |
| D13 | No `automatic` write class before the phase 2 "What it did" log; the first is chosen in phase 3 from recorded approvals. | proposed, G2 |
| D16 | Per-coordinator action tiers, or one policy per workspace. | superseded by D17 (per coordinator) |

### Supersession of ADR-2026-08-31's placement clause

This ADR supersedes the product-placement clause of
[ADR-2026-08-31-generic-plugin-host-boundary](2026-08-31-generic-plugin-host-boundary.md):

- lines 9 to 12, up to "Kandev state." (the Coordinator product moving into a
  separately released plugin, with its state, tools and UI belonging there);
  the rest of line 12, "The current Host contract is too small for that use
  case...", starts a new sentence about the pre-existing Host contract and is
  not part of what this ADR supersedes;
- lines 32 to 34, up to "remain plugin-owned." (Coordinator identity, policy,
  scheduling, state, prompts, tools and UI remaining plugin-owned). The next
  sentence, lines 34 to 36 ("Kandev owns only generic Host contracts..."), is
  not superseded: it is the generic-Host-contract definition this ADR
  restates as unchanged below and in Consequences;
- lines 56 to 57, the sentence "Core must not gain a Coordinator table,
  field, profile, role, principal, grant, setting, tool, or audit vocabulary."
  (the only sentence of that paragraph this ADR supersedes; the plugin
  integration ban before it on lines 54 to 56 stands).

Core now owns the workspace coordinator: the `coordinator` store and routes,
the `coordinator` task origin, MCP surface and mode, the
`coordinator.propose_task` action, the settings tab and the Coordinator
screens.

The generic Host half of ADR-2026-08-31 stands unchanged: the sanctioned
plugin call chain, the capability approval ledger, exact writers, transition
guards, DTOs and result vocabulary. A coordinator plugin built on those
contracts remains possible as an optional extension. This ADR does not move
the plugin's v1 fence.

### Relationship to ADR-2026-09-25-plugin-coordination-platform

ADR-2026-09-25 stands unchanged for everything it decides about the generic
Host: execution, durable input admission, canonical task observations, task
claims, completion gates, managed conversations, the restricted agent tool
policy and reusable chat components. This ADR adds no Host contract and
changes none of those.

What differs is where one product lives. ADR-2026-09-25 expects coordination
products to be plugins; this ADR places the workspace coordinator in core as
a flagged, reversible experiment, on the maintainer's direction of 2026-09-28.
It is not a general coordinator framework: it is one page, one attended
conversation per coordinator, and one write that a person decides.

Phase 1 does not run on the managed conversation surface because that surface
is plugin-bound today: its tool policy names a plugin, an installation and a
manifest digest, and it registers only that plugin's tools, so the
coordinator's core read tools and `propose_task_kandev` cannot appear on it.
If the coordinator later moves to a plugin, its conversation would run on the
managed conversation surface with its tools declared by the plugin, and the
core page, store and routes would be removed or reduced to what the plugin
cannot provide.

### Coordinator permission model

A coordinator's authority is a permission model, not a fixed tool list. Each
coordinator has a scope (phase 1: its whole workspace; Watches narrows it in
phase 2) and a setting per action, and each action is one of three values:
`denied` (not on its surface; a call is refused), `requires approval` (the
coordinator proposes; nothing happens until a manager approves) or
`automatic` (it runs without asking). Phase 1 builds no settings and hardcodes
one policy for every coordinator: the six read tools are `automatic`,
`propose_task_kandev` is `requires approval`, and every other action is
`denied`. The seven tools are therefore the phase-1 tool profile, not a
permanent contract.

Actions are fine-grained. Creating a task and starting its agent are two
actions, so a policy can allow proposals while denying starts, and approving
a create never authorizes a start, including a start that a workflow's
on-enter actions or a queue or WIP promotion would trigger (D15). Moving,
resuming and stopping tasks are actions too, `denied` in phase 1 (D4). The
settings take the shape of the restricted agent tool policy of
[ADR-2026-09-25](2026-09-25-plugin-coordination-platform.md): a list of
allowed actions per principal, enforced where the tool call is checked. If the
coordinator later moves to a plugin, its policy maps onto that contract
instead of being migrated from a second model.

A proposal is phase 1's only `requires approval` action. Later actions that
require approval reuse the same record, decision routes and Needs you card
rather than adding a queue per action.

Three rules hold in every phase. Approving one proposal grants no permission
for any later action; each approval decides one proposal. A coordinator's
runtime session can never change or raise its own permissions: the settings
are written only by a manager through the settings routes, never through the
coordinator's surface. Decision D13 governs when a write action may first be
`automatic`. Storing and editing the settings is phase 2 work (gate G2).

### Residual risk: the agent's own tools

Kandev enforces the coordinator's Kandev surface:

- the session registers only the read tools and `propose_task_kandev`;
- the backend guard refuses every other action from a coordinator principal
  and every id outside its workspace;
- Kandev auto-approves only those exact tool names, and forces the profile and
  environment auto-approve settings off for coordinator sessions;
- CLI-passthrough profiles, which skip Kandev's MCP wrapping, are refused at
  create, edit and session start.

Kandev does not control the agent CLI's own tools and settings. An agent may
have a shell on its executor, its own MCP servers, and its own permission
rules, for example a mode that approves commands without asking. With
`features.auth` off, Kandev's REST API and external `/mcp` endpoint accept any
local caller, so an agent with a shell could call them directly: create a task
on an auto-starting step without going through `propose_task_kandev` at all,
or call the approve route on its own pending proposal. Neither bypass needs
an unattended turn; both are reachable from inside an attended one, since
"attended" means a person started the turn, not that a person reviews every
tool call the agent's own CLI makes inside it. In phase 1 a coordinator is
therefore exactly as capable as any Kandev chat on the same profile, and no
more; its guarded MCP surface adds a proposal path with a person in the loop,
but does not remove the agent's pre-existing, larger capability on the same
host.

Phase 1 enforces the part it can. The approve and reject decisions are not
on the coordinator's MCP surface, and the backend guard refuses them from a
coordinator principal, failing closed when the principal cannot be resolved.
The REST routes cannot tell a person's browser from the agent's shell while
`features.auth` is off, so phase 1 does not claim they can: no user-facing
text says that only a person can approve. The copilot's intro and the
settings copy say that the coordinator proposes through Kandev and that a
proposal waits for a manager's decision in Kandev, which the guard does
enforce.

The rest of this residual is **accepted for phase 1**: nothing in this
design or in phase 1's containment stops a coordinator's agent from calling
the Kandev API directly with a shell, and no phase-1 gate closes it. This is
distinct from gate G3's requirement, which is about a *different* axis:
before any turn can start **unattended** (no person present at all), G3 must
record enforced containment: an executor without a shell or network path to
the host, or `features.auth` on with a coordinator-scoped token, or an
equivalent decided there. G3's containment, once it lands, will also close
this attended-turn residual as a side effect, but until then this risk is
carried, not deferred to a gate that names it. The settings page says that
the profile's auto-approve is ignored for coordinators; that setting narrows
what the *Kandev-mediated* surface will auto-run, not what the agent's own
CLI can do outside it.

### Residual risk: untrusted board content read through the allowed tools

The residual above is about tools the coordinator is not supposed to have. A
second, distinct residual is about content reaching the tools it *is* supposed
to have: the coordinator's read tools return task titles, descriptions and
conversation text that any workspace member can write, so a title or message
aimed at the coordinator's agent rather than at a person could steer its
proposal or how it describes one, and nothing here distinguishes such content
from a person's own, though every effect still routes through the same
phase-1 tool profile and the same manager approval. This residual is
**accepted, unmitigated, for phase 1**, and is not closed by G3, since G3 is
about what an unattended turn's tools can reach, not whether the content
those tools read is trustworthy. Mitigating it, for example by surfacing the
source task link next to the coordinator's paraphrase so a manager can check
the claim, is not built in phase 1 and is left for a later phase to decide.

### Residual risk: an archive failure leaves the old session running until restart

A coordinator PATCH that changes its context or profile clears
`conversation_task_id` and then archives the old conversation task through
`ArchiveTask`, which stops its running turn
([coordinators design](../specs/coordinator/system-design/coordinators.md#routes)).
If that archive call itself fails, the PATCH still succeeds: the old task is
already unreferenced, so it resolves to no coordinator, and the next startup
pass archives it, but nothing revisits it before then. Between the failed
archive and the next process restart, the previous agent can keep running
under the context and profile the manager just replaced. This residual is
**accepted, unmitigated, for phase 1**: no in-process retry or supervisor
sweep closes the window, since a manager can still stop the stray session by
hand from the task, and a full mitigation is left for a later phase.

## Phase plan

Each phase ships a subset of the final design; no phase redraws what an
earlier phase built. Every phase ships behind `features.coordinator` while it
is `prod: "false"`. If phase 1 is promoted before a later phase lands, that
phase adds its own release toggle (`features.coordinatorPhase<N>`) and ships
behind it.

| Phase | Ships | Work packages | Gate |
| --- | --- | --- | --- |
| 1. Core flow | Coordinators in workspace settings; a sidebar entry per coordinator beside the Inbox; Needs you and Queue with the count strip; item cards for stalls, questions and permissions, errors, and task proposals (Approve, Edit, Reject); Ask about this; the copilot chat panel on the right side of the Coordinator screens, which reads the workspace and proposes tasks | WP-0 to WP-5b (WP-0, WP-1, WP-1b, WP-2, WP-3, WP-4a, WP-4b, WP-4c, WP-4d, WP-4e, WP-5a, WP-5b) | G0 |
| 2. Control (a person approves everything) | The permission settings of D17; the "What it did" log with undo; Watches, May do and Standing orders per coordinator; guided setup; a goal, the goal note and baselines; resume, message and move as proposals (Resume on stall cards, PR-ready prompts); the launcher on every page and the page context chip | WP-6 to WP-10 | G2 |
| 3. Autonomy (it may act on its own) | Wake on its tasks' events with a level-triggered backstop; enforced containment for unattended turns; questions and permissions answered in place; cost against a ceiling; improvement proposals; the first `automatic` action | WP-11, WP-12 | G3 |
| 4. Integrations | Came in from Jira, Linear and Sentry; tracker write-back as a proposal | WP-13, WP-14 | G4 |

Few phases, one theme each (owner, 2026-09-29): control with a person approving,
then autonomy, then external integrations. Autonomy cannot fold into control:
its first `automatic` action is chosen from at least 30 days of phase 2's log,
and unattended turns need containment and a cost ceiling first.

Gate N (G2 to G4) is met when all four hold:

1. Every work package of phase N-1 is merged with its exit met, and the repo
   ports of the mockup's phase N-1 view pass in CI.
2. Phase N's work packages are written to phase 1's depth and reviewed.
3. Phase N's requirements and ADR are posted on
   [#3752](https://github.com/kdlbs/kandev/issues/3752) with screenshots, and a
   kdlbs maintainer replies without objecting, or 10 working days pass with no
   reply and that ADR records "proceeding without maintainer reply" with the
   date.
4. That ADR records the gate's decision:

| Gate | Opens | Decision to record |
| --- | --- | --- |
| G2 | WP-6 to WP-10 | Storage and editing of D17's settings; D13; the log's storage and retention; D11; D12; the goal and baseline model |
| G3 | WP-11, WP-12 | The wake design (a level-triggered backstop, no replacement fork); enforced containment for unattended turns; the cost-ceiling model; the first `automatic` class, from at least 30 days of log rows |
| G4 | WP-13, WP-14 | D10: which trackers, the de-duplication key, write-back as a proposal |

## G0 Status

**Status:** pending.

G0 gates merging phase 1, not building it. Phase 1's work packages are built
and reviewed on their branches once WP-0 has passed Review and ship together
with this design package as one pull request
([#3981](https://github.com/kdlbs/kandev/pull/3981)), which does not merge
until G0 is met. The phase 1
requirements, this ADR, the plan's ASCII previews and the mockup's phase 1
screenshots are posted on [#3752](https://github.com/kdlbs/kandev/issues/3752).
G0 is met when a kdlbs maintainer replies without objecting to D1, D2 and D9,
or when 10 working days pass with no maintainer reply; in the second case this
section records "proceeding without maintainer reply" with the date. An
objection re-plans the affected work packages before their upstream PRs open.

2026-09-28: the maintainer agreed to D1 (core, behind a feature flag, may move
to a plugin later). G0 stays pending until the maintainer has reviewed this
design package.
2026-09-29: the owner chose to ship phase 1 as one pull request, #3981,
carrying this design package and all of its code, instead of one PR per work
package.

## Prior art

- **Conductor loop (author's notes, `concepts/conductor-loop.md`, updated
  2026-09-06).** Separates admission, progress, learning and economy; a stall
  counter makes a missing inner loop visible to a person; warns against an
  outer loop without an inner one and against confusing the attention gate with
  the merge gate; deterministic gates hold authority while models draft and
  explain. Adopted: `task.stalled` is the progress signal, Needs you is an
  attention gate only (merging stays on the PR), and proposals are decided by
  a person.
- **Paperclip approvals and Inbox.** Strategy and hire approvals that do not
  expire, with "request revision"; an Inbox and a Blocked inbox of items
  needing a person; agents run on heartbeats. Adopted: approvals that stay
  until decided, and a list of blocked items with a reason. Differs: no
  heartbeat or wake in phase 1 (attended only, no idle spend), and no request
  revision; the manager edits instead.
- **Devin managed sessions.** A coordinating session proposes child sessions
  for approval before launching them. Adopted: propose before create. Differs:
  approval creates a task and never starts an agent (D15).
- What Kandev adds: approval is idempotent through a reserved external id, the
  coordinator count is separate from the Inbox count (D14), and the attention
  list is a projection of facts Kandev already publishes rather than a queue
  the agent maintains.

## Consequences

- Core gains a small coordinator vocabulary (store, origin, surface, mode,
  action, flag). It is reachable only when `features.coordinator` is on, and
  its data survives the flag being off.
- A coordinator plugin, if built, must coexist with this page; the generic
  Host contracts it would use are unaffected.
- The placement is reversible: because every coordinator surface sits behind
  `features.coordinator`, moving the product to a plugin later removes core
  code without a data migration for users who never turned the flag on.
- The MCP guard's allowlist is the single place new coordinator abilities
  join, one decision at a time.
- The residual risk of the agent's own tools is accepted for attended turns
  only, and blocks unattended turns until G3.
- The Inbox, Office and Configuration chat keep their behaviour.

## Alternatives considered

- **Keep the product in the plugin (ADR-2026-08-31).** Blocks the feature on
  Host contracts it does not need, and makes the attention list depend on a
  separately released component. Rejected for phase 1; the plugin remains an
  option.
- **Build it now as a plugin on the managed coordination platform
  (ADR-2026-09-25).** Its managed conversation is plugin-bound, so the
  coordinator's tools would have to be plugin tools and the page a plugin
  surface. Deferred on the maintainer's direction to test the product in core
  first; it remains the path if the product moves.
- **Build on Office.** Office's coordinator role routes autonomous agents with
  budgets and heartbeats; Kanban workspaces would inherit an autonomy model
  they did not ask for. Rejected.
- **Run the coordinator on the automation runner.** Adds a wake, run queue and
  unattended turns before containment exists. Deferred to the G3 wake design.
- **Let the coordinator create tasks directly.** Removes the person from the
  only write. Rejected; D13 governs any future automatic class.
