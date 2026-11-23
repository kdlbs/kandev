---
requirements:
  - REQ-INTEGRATIONS-DISCORD-001
  - REQ-INTEGRATIONS-DISCORD-002
  - REQ-INTEGRATIONS-DISCORD-003
  - REQ-INTEGRATIONS-DISCORD-004
  - REQ-INTEGRATIONS-DISCORD-005
  - REQ-INTEGRATIONS-DISCORD-006
  - REQ-INTEGRATIONS-DISCORD-007
  - REQ-INTEGRATIONS-DISCORD-008
  - REQ-INTEGRATIONS-DISCORD-009
  - REQ-INTEGRATIONS-DISCORD-010
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-001
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-002
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-003
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-004
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-005
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-006
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-007
  - REQ-PLUGINS-CHAT-BRIDGE-HOST-008
system_design:
  - ../../specs/plugins/system-design/chat-bridge-host-controls.md
  - ../../specs/integrations/system-design/discord.md
legacy_specs: []
created: 2026-11-21
status: draft
---

# Implementation Plan: Discord Task Channels

## Overview

There are two deliverables.

- **(A) Host API changes in `kdlbs/kandev`.** These are additive and were
  decided by the maintainers on
  [#5420](https://github.com/kdlbs/kandev/issues/5420#issuecomment-7083766483)
  (spec "Host API additions"):
  - `CancelTurn`;
  - human responses from linked Kandev users, gated by an admin grant;
  - workflow step approval (`ApproveStep`/`GetStepApproval`);
  - `Message.Source`;
  - docs, including the `raw_content` correction (the payload field stays).
- **(B) The plugin
  [`Fclem/kandev-plugin-discord`](https://github.com/Fclem/kandev-plugin-discord).**
  It is scaffolded from `kdlbs/kandev-plugin-template` on the standalone
  `github.com/kdlbs/kandev/pluginsdk` module.

Most of (B) runs on Host surfaces that already exist on `origin/main`:
provisioning, the channel lifecycle, two-way chat, `/status`, and
`/stop scope:task`. Only `/stop` (turn scope), `/approve`/`/deny`, and echo
suppression wait on (A).

### Baseline correction

The first draft of this plan (earlier on 2026-11-21) was investigated against
a stale task checkout: `a1135aa8`, 2026-08-01, which is 2,503 commits behind.
All host facts below were re-verified against `origin/main` `018808f8`
(2026-11-21). Several gaps from the first draft are already shipped upstream:

- task DTO archive and step fields;
- `Tasks().Move`;
- `Interactions()`;
- exact `StopTaskRun` and `Archive`;
- admin-only plugin management;
- authenticated and admin plugin `actions`;
- the `api_write` docs.

They were removed from (A).

## Investigation findings (`origin/main` 018808f8)

### Plugin architecture and extension points

| Surface | Current behavior (evidence) | Use in this plugin |
|---|---|---|
| SDK and manifest | Standalone module `github.com/kdlbs/kandev/pluginsdk` v0.1.0. Manifest `api_version` 1 or 2; v2 recommended. The template floor is `min_kandev_version: 0.99.0` (`kandev-plugin-template` `AGENTS.md`, `manifest.yaml`). | Scaffold on SDK v0.1.0 and API v2. Raise `min_kandev_version` to the first release that ships (A). |
| Process model | Kandev spawns and supervises the plugin binary. It pings the plugin, restarts it with backoff, and offers no shutdown hook (`apps/backend/internal/plugins/runtime`). | The gateway client lives in the plugin process and starts once Host is injected. Each restart re-identifies. |
| Events | Bounded per-plugin queue with retries; subjects match by segment wildcards (`apps/backend/internal/plugins/delivery/deliverer.go`). | Subscribe to low-volume lifecycle subjects only. Never subscribe to `message.*`, because streaming emits one event per chunk. Events are triggers; content comes from reads. |
| Task reads | `Tasks().List/Get`, plus `Task.ArchivedAt`, `Task.WorkflowStepID`, and `TaskFilter.IncludeArchived` (`pluginsdk/data_types.go:111-132`). | Archive detection and `/status` need no host change. |
| Task writes | `Tasks().Create/Update/Move`. Move uses the board path, which fires `task.moved` and `on_enter` (`docs/public/plugins-authoring.md` "Live Host writes"). Exact `Archive` and `Move` require Host v2 grants. | Not needed; the plugin does not mutate tasks. |
| Message writes | `Messages().Send` (`api_write: messages`) queues behind a running session, otherwise resumes or starts it. It returns `queued`, `sent`, or `started` and stamps `metadata.source=plugin:<id>`. | Inbound Discord → Kandev uses it as-is, which satisfies the queue and running-turn rules. |
| Message reads | `Messages().List` is sanitized. The `Message` DTO has no source or metadata (`pluginsdk/data_types.go:1014-1023`). | Echo suppression needs `Message.Source` (host task-17). |
| Interactions | `Interactions().ListPending/Get` (`api_read: interactions`). Exact `RespondPermission`/`AnswerClarification` need `api_write: interactions`, `host.v2.write:interactions`, and a Host-issued **human response receipt**. Only the authenticated native UI mints receipts (`POST /api/plugins/host/interactions/response-receipts`, user with `session.control`). The v1 response methods return `PermissionDenied`. | Pending prompts come from `Interactions()`. Answering from Discord is blocked by design and needs the delegated authority in host task-15. |
| Execution | `HostExecutionCommands(host)`: `EnsureTaskRun`, `StopTaskRun` (stops an execution by ID; `adapters_plugin_execution.go:84-111` → `orchestrator.StopExecution`), `RecoverSession`, and others, under `api_write: execution` + `host.v2.write:execution`. There is no turn cancel. | `/stop scope:task` uses `StopTaskRun`. `/stop` (turn) needs `CancelTurn` (host task-02). |
| Workflow review approval | The UI Approve button calls `POST /api/v1/sessions/:id/approve` → `task/service.ApproveSession` (`apps/backend/internal/task/service/service_workflow.go:57+`). That resolves the step's configured transition, calls `MoveTask`, and sets `review_status=approved`. There is no plugin equivalent. | Host task-16 adds `ApproveStep`/`GetStepApproval`, gated by task-15's authority. |
| Workspace grants | Host v2 capabilities need a workspace manager's grant in Settings > Plugins > Workspace access, with an approval revision and an audit record (`docs/public/plugins-authoring.md` "Security and capabilities"). | The delegated authority is a new grant in the same ledger. |
| UI ↔ backend | Manifest `actions` (`access: authenticated \| admin`, scopes `workspace \| task \| repository \| notification_provider`) are invoked by `host.api.invokeAction`. The host authenticates and authorizes them before the plugin runs. Plugin config mutations are `RequireAdmin` (`apps/backend/internal/plugins/handlers.go:104-127`). | The quick-setup card saves through the admin config API and calls the admin actions `connection.test`/`connection.status`. No public webhook. |
| Event sanitization | `publishMessageEvent` still adds `raw_content` when hidden system content exists (`apps/backend/internal/task/service/service_events.go:1417-1419`). Plugin delivery forwards payloads unfiltered (`delivery/payload.go`). | Host task-03 fixes it for all plugins. |

### Discord API constraints (verified against `discord/discord-api-docs`, 2026-11)

| Constraint | Value | Consequence |
|---|---|---|
| Guild creation by apps | `POST /guilds` restricted 2025-07-15 and removed; bots can no longer own guilds | Attach only. The operator creates the server and invites the bot. |
| Channels | 500 per guild including categories (error `30013`); 50 per category | Category rollover, retention and deletion, and `capacity_blocked`. |
| Messages | 2000 chars; embeds 10 per message, 6000 chars total | Fence-aware chunker with a 5-chunk cap. |
| Mentions | Regular messages parse everything by default | Always send `allowed_mentions: {parse: []}`. |
| Rate limits | Per-route buckets (`X-RateLimit-Bucket`); global 50 req/s; 10,000 invalid requests per 10 min ban. Renames are commonly 2 per 10 min per channel (undocumented, `[INFERENCE]`) | Header-driven limiter, per-channel senders, a 40 req/s budget, debounces, and a hard stop on 401. |
| Intents | `MESSAGE_CONTENT` is privileged; portal toggle below 100 guilds; close code `4014` if disallowed | README walkthrough; `4004`/`4013`/`4014` are fatal. |
| Gateway | Resume via `resume_gateway_url`; 1000 IDENTIFY per 24 h (exceeding resets the token) | `bwmarrin/discordgo` plus a persisted identify counter. |
| Interactions | 3 s initial response; 15 min token; guild commands update instantly | Defer everything; guild-scoped commands; register only when the hash changes. |
| Gateway vs webhooks | `MESSAGE_CREATE` is gateway-only | Gateway only. |

## Architecture (plugin)

```
kandev events ──OnEvent──▶ dispatcher ──▶ per-task worker ──▶ outbound (Messages.List → chunk → per-channel sender)
                                              │                └▶ channel lifecycle (create/rename/close)
Discord gateway ─MESSAGE_CREATE──▶ inbound (authz → dedupe → Messages.Send → reaction)
               ─INTERACTION_CREATE─▶ commands (/status /stop /approve /deny, buttons, autocomplete)
               ─CHANNEL_DELETE / GUILD_* / READY──▶ reconciler
reconciler (READY + every 5 min): bindings → provisioning → task/channel diff → cursors → Interactions.ListPending → retention
admin actions: connection.test / connection.status ◀── settings card (host.api.invokeAction)
store: Host state
```

Packages under `server/`:

| Package | Responsibility |
|---|---|
| `config` | Parsing and validation |
| `discordapi` | Interface, discordgo implementation, and fake |
| `gateway` | Gateway connection |
| `store` | Host state access |
| `provision` | Guild provisioning |
| `channels` | Channel lifecycle |
| `outbound` | Kandev → Discord mirroring |
| `inbound` | Discord → Kandev delivery |
| `commands` | Slash commands, buttons, autocomplete |
| `reconcile` | Periodic and `READY` reconciliation |
| `authz` | Operator checks |
| `health` | Health cache and admin actions |

## Backend — Kandev host (`kdlbs/kandev`)

| Task | Change | Key files |
|---|---|---|
| [01](task-01-host-control-rpcs-adr.md) | An ADR for all host additions | `docs/decisions/` |
| [02](task-02-host-control-rpcs.md) | `CancelTurn` exact execution command → `orchestrator.Service.CancelAgent` | `pluginsdk/exact_execution*.go`, `pluginsdk/proto/kandev/plugin/v1/plugin.proto`, `apps/backend/internal/plugins/host_exact_execution.go`, `apps/backend/internal/backendapp/adapters_plugin_execution.go` |
| [03](task-03-host-plugin-event-sanitization.md) | Cancelled: `raw_content` stays in `message.*` payloads | — |
| [15](task-15-host-delegated-human-responses.md) | Account linking, `host.v2.write:human_responses` grant, receipts minted for the linked Kandev user | Interaction receipt service, identity links, Host v2 grant ledger, `pluginsdk`, plugin settings UI |
| [16](task-16-host-step-approval-rpcs.md) | `ApproveStep` (reusing `ApproveSession`) and `GetStepApproval` | `apps/backend/internal/task/service/service_workflow.go`, plugin host, `pluginsdk` |
| [17](task-17-host-message-source.md) | `Message.Source` on the read DTO | `pluginsdk/data_types.go`, `apps/backend/internal/plugins/host_data.go` |
| [04](task-04-host-plugin-docs.md) | Public docs for the above | `docs/public/plugins-authoring.md`, `docs/public/plugins-manifest.md` |

## Backend — plugin (`Fclem/kandev-plugin-discord`)

Plugin tasks 05–14 are listed under the waves below. Cross-cutting rules:

- **Idempotency.**
  - An in-memory LRU of event IDs.
  - Every Discord mutation checks the store first.
  - Outbound posting is cursor-driven.
  - Pending prompts are reconciled against `Interactions()`.
- **Concurrency.** One worker per task key serializes writes to Host state,
  which has no compare-and-swap.
- **Logging.** Never log config, the token, or message content at info level.

## Frontend

The Kandev SPA does not change. The plugin ships `ui/bundle.js` with one
`plugin-settings` component above Kandev's schema-driven settings form. The
admin-only form already provides the token (masked), the application ID, and
the `workspace_guilds` fields, and saving it restarts the plugin.

The card adds:

- workspace IDs with a "Copy line" button;
- per-binding invite links;
- **Test connection**: `host.api.invokeAction("connection.test", { workspaceId })`
  per binding, rendering one pass/fail/skip line per check.
- `initialize`/`destroy` must be repeatable.

## Tests

| What (spec scenario) | File | How |
|---|---|---|
| `CancelTurn` fencing, grant gating, delegation to `CancelAgent` | `apps/backend/internal/plugins/host_exact_execution_test.go`, `apps/backend/internal/backendapp/adapters_plugin_execution_test.go` | Table-driven, fake orchestrator |
| Linked receipts: grant, link, `session.control`, single use, revision- and installation-bound, attribution | `apps/backend/internal/plugins/*_test.go` | Real ledger and link store, fake interactions |
| `ApproveStep`/`GetStepApproval`: awaiting rules, `FailedPrecondition`, office `Unimplemented` | `apps/backend/internal/task/service/service_workflow_test.go`, plugin host tests | Real workflow fixtures |
| `Message.Source` projected from `metadata.source` only | `apps/backend/internal/plugins/host_data_test.go`, `pluginsdk/host_data_test.go` | Fake message source; wire round-trip |
| `raw_content` documented as present in `message.*` payloads | `docs/public/plugins-manifest.md` | Docs review |
| Binding parse | `server/config/config_test.go` | Table-driven |
| Naming, rename debounce, rollover, capacity | `server/channels/*_test.go` | Fake `discordapi`, fake clock |
| Provisioning idempotency | `server/provision/provision_test.go` | Fake `discordapi` |
| Reconcile: downtime, archive, pending interactions | `server/reconcile/reconcile_test.go` | Fake Host plus fake Discord |
| Outbound order, cursor, echo suppression, chunking, mentions | `server/outbound/*_test.go` | Fake Host messages |
| Inbound authz, reactions per `Send` outcome, catch-up age | `server/inbound/*_test.go` | Fake `Send` |
| `/status`, `/stop` scopes, `/approve` (permissions, steps, missing grant, stale) | `server/commands/*_test.go` | Fake interactions |
| Admin actions: token, intent, membership, permissions; no secrets in output | `server/health/health_test.go`, `ui/*.test.*` | Fake Discord; template UI harness |
| Fatal close codes; 401 halt | `server/gateway/gateway_test.go` | Fake session |

## E2E Tests

There are no Kandev SPA changes, so there are no Playwright specs in
`kdlbs/kandev`.

- Host changes are covered by Go tests, plus one in-tree fixture plugin
  exercise for `CancelTurn`, `ApproveStep`, and delegated receipts. It follows
  the existing plugin fixture pattern in `apps/backend/internal/plugins/runtime/testdata`.
- Plugin acceptance is an artifact smoke in task-14, run on a disposable dev
  Kandev with a dedicated test bot and guild.

## Verification Results

Pending.

## Phased delivery and estimates (engineer-days)

| Phase | Tasks | Estimate | Outcome |
|---|---|---|---|
| 0. Host additions (after maintainer approval) | 01, 02, 03, 15, 16, 17, 04 | 6–8.5 d | `CancelTurn`, delegated responses, step approval, `Message.Source`, leak fix, docs |
| 1. Plugin foundation | 05–07 | 5–6 d | Scaffold (done), config, gateway, provisioning, channel lifecycle |
| 2. Sync and chat | 08–10 | 5–6 d | Reconcile, mirroring, inbound delivery |
| 3. Commands | 11–12 | 3.5–5 d | `/status`, `/stop`, `/approve`/`/deny` for permissions and steps |
| 4. Ship | 13–14 | 3–4 d | Quick-setup card with connection test, artifact smoke, release |
| **Total** | 17 tasks | **22.5–29.5 d** | Phases 0 and 1–2 run in parallel; phase 3 waits on 0 |

## Implementation Waves And Parallel Candidates

Default execution is sequential. Using subagents requires explicit user
authorization.

```
Wave 1 (parallel candidates — different repositories):
- [x] [task-01-host-control-rpcs-adr](task-01-host-control-rpcs-adr.md)
- [ ] [task-05-plugin-bootstrap-config](task-05-plugin-bootstrap-config.md)

Wave 2:
- [x] [task-02-host-control-rpcs](task-02-host-control-rpcs.md)  (CancelTurn)
- [x] ~~[task-03-host-plugin-event-sanitization](task-03-host-plugin-event-sanitization.md)~~  (cancelled: maintainers keep `raw_content`; task-04 corrects the docs)
- [x] [task-17-host-message-source](task-17-host-message-source.md)  (sequential with 02: shared proto)
- [ ] [task-06-plugin-gateway-provisioning](task-06-plugin-gateway-provisioning.md)

Wave 3:
- [x] [task-15-host-delegated-human-responses](task-15-host-delegated-human-responses.md)
- [ ] [task-07-plugin-channel-lifecycle](task-07-plugin-channel-lifecycle.md)

Wave 4:
- [x] [task-16-host-step-approval-rpcs](task-16-host-step-approval-rpcs.md)
- [ ] [task-08-plugin-reconciler](task-08-plugin-reconciler.md)
- [ ] [task-09-plugin-outbound-mirroring](task-09-plugin-outbound-mirroring.md)

Wave 5:
- [x] [task-04-host-plugin-docs](task-04-host-plugin-docs.md)
- [ ] [task-10-plugin-inbound-delivery](task-10-plugin-inbound-delivery.md)
- [ ] [task-11-plugin-status-stop-commands](task-11-plugin-status-stop-commands.md)

Wave 6:
- [ ] [task-12-plugin-permission-approvals](task-12-plugin-permission-approvals.md)  (permissions + steps; needs 15, 16)
- [ ] [task-13-plugin-settings-card](task-13-plugin-settings-card.md)  (quick setup + connection test)

Wave 7:
- [ ] [task-14-plugin-artifact-smoke-release](task-14-plugin-artifact-smoke-release.md)
```

## Risks

- **Account linking scope.** The maintainers chose linking gated by an admin
  grant (task-15), which adds link storage, a link-code flow, and a settings
  card. Unlinked Discord users get deep links instead of actions.
- **Discord policy.** Further restrictions on bots or privileged intents would
  break inbound chat. Discord sits behind `discordapi`.
- **Channel cap.** Retention, deletion, and rollover mitigate it; forum
  threads were rejected.
- **Authority.** Guild role admins effectively control chat and `/stop`.
  Approvals and answers act only for a linked Kandev user with
  `session.control`, need the Kandev workspace grant, and are audited.
- **Event loss.** Cursor pulls and a 5-minute reconcile mitigate it; the worst
  case is a 5-minute delay.
- **Identify budget.** Config edits and crash loops each re-identify. A
  persisted counter enforces a 900-per-24 h cap.
- **SDK versioning.** (A) needs a pluginsdk release after v0.1.0. The plugin
  pins it and raises `min_kandev_version` together.

## Open Questions

None for the Kandev maintainers; see the spec "Maintainer decisions".
