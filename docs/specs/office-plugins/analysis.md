# Office Plugins: Ecosystem Analysis

## 1. Industry Plugin Ecosystem Inventory

### Official SDK & Tooling (reference implementations)

| Package | Description |
|---------|-------------|
| Plugin SDK | Official TypeScript SDK: worker runtime, UI React hooks, testing harness, bundler presets, dev server |
| Plugin scaffolding CLI | CLI scaffolding tool. Templates: `default`, `connector`, `workspace`, `environment`. Generates manifest, worker, UI bundle, tests |

### First-Party Plugins (by platform maintainers)

| Plugin | Category | Capabilities Used | What It Does |
|--------|----------|-------------------|--------------|
| Discord plugin | connector | events.subscribe, events.emit, webhooks.receive, agent.tools.register, agent.sessions.create/send/close, agents.invoke, plugin.state.read/write, http.outbound, secrets.read-ref, jobs.schedule, activity.log.write, metrics.write | Full bidirectional Discord integration: notifications (task created/done, agent runs, approvals, budget), slash commands, button interactions (approve/reject), multi-agent thread sessions, agent handoffs, community intelligence scanning, custom commands, proactive watch patterns, media pipeline (Whisper transcription), escalation system with HITL buttons, daily digests. 22 capabilities, 5 scheduled jobs, 6 agent tools. |
| GitHub Issues plugin | connector | issues.read, issues.update, issue.comments.read, issue.comments.create, plugin.state.read/write, events.subscribe, http.outbound, secrets.read-ref, webhooks.receive, agent.tools.register, instance.settings.register, ui.detailTab.register | Bidirectional GitHub Issues sync: status mapping (open/closed to backlog/done), comment bridging, webhook ingestion, periodic reconciliation job (15min), 3 agent tools (search/link/unlink), settings page, issue detail tab showing linked GitHub issue. |
| Slack plugin | connector | events.subscribe, http.outbound, secrets.read-ref, plugin.state.read/write | Slack notifications: posts to Slack channels on task creation, completion, and approval requests. |
| Telegram plugin | connector | events.subscribe, http.outbound, secrets.read-ref, plugin.state.read/write | Telegram notifications: same event-to-message pattern as Slack but for Telegram Bot API. |

### Third-Party / Community Plugins

| Plugin | Category | What It Does |
|--------|----------|--------------|
| Linear sync plugin | connector | Bidirectional Linear issue sync: status/priority mapping, comment bridging, project sync, label sync, OAuth flow, full initial import, 4 agent tools, settings page with OAuth connect/disconnect, issue detail tab. 23 capabilities. Most complex community plugin. |
| Chat copilot plugin | ui / automation | Interactive AI chat copilot for managing tasks, agents, and workspaces through a conversational interface. |
| Hindsight plugin | automation | Persistent long-term memory for agents. Recall before every heartbeat, retain after every run. By Vectorize.io. |
| Company wizard plugin | automation | AI-powered company setup assistant with presets for quick onboarding. |
| ACP bridge plugin | connector | ACP runtime that runs Claude Code, Codex, and Gemini CLI from any chat platform (Discord, etc.). Bridges chat platforms to agent CLI processes. |
| WritBase sync plugin | connector | Bidirectional sync between tasks and WritBase tasks with webhook-driven updates and periodic reconciliation. |
| AVP trust plugin | automation | Trust layer using Agent Veil Protocol: DID identity and reputation scoring for agents. |
| Live analytics plugin | analytics | Live visitor map, dashboard widget, and settings page for viewing Agent Analytics. |
| Obsidian integration | ui | Obsidian plugin to browse, comment on, and assign tasks to AI agents. |
| Aperture plugin | ui | Alternative Focus interface: deterministically ranks approvals, task activity, and other human-facing events into now/next/ambient priority tiers. |

### Ecosystem Tooling

| Tool | What It Does |
|------|--------------|
| Plugin bundle | Curated bundle of recommended plugins (meta-package). |
| MCP server | MCP server exposing the platform REST API as tools for Claude Code and Claude Desktop. |
| Discord bot | Standalone Discord bot (not a plugin) for GitHub OAuth contributor roles and daily AI summaries. |

### Capability Usage Patterns

Across the ecosystem, the most-used capabilities are:

1. **events.subscribe** -- every plugin subscribes to at least one event type
2. **plugin.state.read/write** -- every plugin with any persistence uses the KV store
3. **http.outbound** -- every connector plugin calls external APIs
4. **secrets.read-ref** -- every plugin with credentials needs secret resolution
5. **webhooks.receive** -- bidirectional sync plugins need inbound webhooks
6. **agent.tools.register** -- sync plugins expose search/link/unlink tools to agents
7. **jobs.schedule** -- periodic reconciliation is standard for sync plugins
8. **ui.detailTab.register** -- sync plugins show linked entity status on task detail

---

## 2. Kandev Features That Could Be Plugins

### Current In-Core Integrations

| Feature | Location | Lines (approx) | Plugin Candidate? | Assessment |
|---------|----------|-----------------|-------------------|------------|
| **GitHub integration** | `internal/github/` | ~40 files, ~3K lines | Strong candidate | PR sync, task watch, review tasks, polling, webhooks. Tightly coupled to event bus and task model but the coupling is through well-defined interfaces (Client, Store, EventBus). Could be extracted to a plugin that receives task/session events and calls GitHub API. The PAT/token management, PR status cache, and search cache would move to plugin state. |
| **Jira integration** | `internal/jira/` | ~14 files, ~1.5K lines | Strong candidate | Ticket browsing, import, config/secret management. Already uses a clean Client interface + ClientFactory pattern. Service depends only on Store (SQLite) and SecretStore. Could become a plugin that syncs Jira tickets to office tasks. |
| **Notification providers** | `internal/notifications/` | ~12 files | Mixed | Three providers: Apprise (shelling out to CLI), System (OS-native notifications), Local (WebSocket). System and Local are core infrastructure. Apprise is the plugin candidate -- it bridges to 80+ notification services (Slack, Telegram, Discord, email, PushBullet, etc.) via a CLI tool. Each Apprise URL target could be a plugin-configured channel. |
| **Sprites integration** | `internal/sprites/` | 2 files | No | Sprites is an executor backend (like Docker or Standalone). Executor backends are deeply integrated into the lifecycle manager. Not a plugin -- plugins don't own execution environments. |
| **Agent adapters** | `internal/agentctl/server/adapter/` | ~15 adapters | No for existing, Yes for new | Existing adapters (ACP, Codex, OpenCode, Copilot, Amp) are transport-level integrations wired into agentctl's subprocess management. They can't be external HTTP services. However, new agent types that run as external services (API-only agents, custom LLM wrappers) could be plugin-provided. |
| **Analytics / cost tracking** | `internal/analytics/` | ~10 files | Partial | The core cost recording (per-session token counts, model pricing) must stay in core for budget enforcement. But cost reporting, external billing integration, and usage dashboards could be plugins that read cost events and produce reports. |

### Office Features That Should Be Plugins

| Feature | Currently Planned As | Why It Should Be a Plugin |
|---------|---------------------|--------------------------|
| **Telegram channel** | Core channel type in office-assistant | Platform-specific webhook ingestion, message formatting, API client, bot management. Different users need different platforms. A Telegram plugin would receive `office.comment.created` events and relay outbound messages, plus expose an inbound webhook for Telegram updates. |
| **Slack channel** | Core channel type in office-assistant | Same reasoning as Telegram. Slack's OAuth, signing secret verification, Block Kit formatting, and app manifest are substantial platform-specific logic. |
| **Discord channel** | Core channel type in office-assistant | Same pattern. Reference Discord plugins in the industry reach 57K+ lines of worker code alone -- this complexity should not live in kandev core. |
| **Email channel** | Core channel type in office-assistant | SMTP/IMAP integration, email parsing, HTML rendering. Specialized enough to be a plugin. |
| **Jira sync** | Existing core integration | Bidirectional Jira-to-office-task sync. The existing `internal/jira/` package is already cleanly separated -- it could become the first reference plugin. |
| **Linear sync** | Not planned | Industry examples show Linear sync is a high-demand integration. As a plugin, it avoids adding Linear-specific code to kandev core. |
| **GitHub Issues sync** | Existing core integration partially covers this | The existing `internal/github/` handles PRs, reviews, and issue watch. Task-level issue sync (status mapping, comment bridging) would be cleaner as a plugin. PR integration might stay in core since it's deeply tied to the development workflow. |
| **Cost reporting / billing** | Core analytics | External billing integration (Stripe, invoice generation), cost dashboards beyond the built-in one, usage alerts to external systems. |
| **Custom notification targets** | Apprise provider | Each notification target (PagerDuty, Opsgenie, ntfy, Gotify, Pushover) could be a plugin instead of routing through Apprise CLI. Gives per-target configuration, rich formatting, and delivery tracking. |
| **Webhook triggers for routines** | Planned in office-routines | External systems (GitHub Actions, CI/CD, monitoring alerts) POST to kandev, which creates tasks. Generic webhook plugins could transform payloads into task creation requests. |
| **Custom agent tools** | Not planned | Plugins could register tools that agents can call during sessions -- database queries, API calls to internal systems, Slack lookups, calendar checks. Existing plugin ecosystems show agent tools are one of the most valuable plugin capabilities. |

---

## 3. Plugin Categories

### Connectors
Bidirectional sync with external systems. These are the most common and complex plugins.

- **Issue trackers**: Jira, Linear, GitHub Issues, Asana, Notion, ClickUp, Shortcut
- **Chat platforms**: Slack, Discord, Telegram, Microsoft Teams, email
- **Source control**: GitHub PR sync (enhanced), GitLab MR sync, Bitbucket
- **CI/CD**: GitHub Actions status, CircleCI, Jenkins (trigger builds, report results)

### Automation
Event-driven workflows and scheduled jobs.

- **Memory / knowledge**: Long-term agent memory backends (vector DB, knowledge graph)
- **Approval routing**: Custom approval workflows (escalation policies, auto-approve rules)
- **Task templates**: Template-based task creation from external triggers
- **Webhook transformers**: Parse incoming webhooks from arbitrary systems into task/comment payloads

### Tools
Agent-callable tools that extend what agents can do during sessions.

- **Search**: Search external knowledge bases, documentation, Confluence, Stack Overflow
- **Database**: Query internal databases, run SQL, check data
- **Calendar**: Check/create calendar events, schedule meetings
- **Monitoring**: Query Datadog, Grafana, PagerDuty for system health
- **Deployment**: Trigger deploys, check deploy status, rollback

### Analytics
Read-only plugins that consume events for reporting and insights.

- **Cost dashboards**: External cost reporting, billing integration, invoice generation
- **Velocity metrics**: Sprint velocity, cycle time, throughput tracking
- **Agent performance**: Success rates, retry frequencies, cost efficiency per agent
- **Live analytics**: Real-time activity maps, agent utilization heat maps

### UI Extensions (future, out of scope for v1)
Plugins that extend the kandev web UI.

- **Custom dashboard widgets**: Agent-specific status cards, project health gauges
- **Detail tabs**: Show external system state on task/agent detail pages
- **Settings pages**: Plugin configuration UI

---

## 4. Dependency Analysis: Plugin Capabilities to Kandev APIs

### What Plugins Need to READ

| Data | Kandev API | Used By |
|------|-----------|---------|
| Tasks (list, get, search) | `GET /api/office/tasks` | All connectors (sync), analytics |
| Task comments | `GET /api/office/tasks/:id/comments` | Chat platform connectors |
| Agent instances (list, get, status) | `GET /api/office/agents` | Analytics, monitoring plugins |
| Projects | `GET /api/office/projects` | Connectors mapping external projects |
| Skills | `GET /api/office/skills` | Automation plugins |
| Cost events | `GET /api/office/costs` | Analytics, billing plugins |
| Activity log | `GET /api/office/activity` | Analytics, digest plugins |
| Wakeup queue status | `GET /api/office/wakeups` | Monitoring plugins |
| Approvals | `GET /api/office/approvals` | Notification plugins |
| Plugin's own state | `GET /api/plugins/:id/state/:key` | All plugins with persistence |

### What Plugins Need to WRITE

| Action | Kandev API | Used By |
|--------|-----------|---------|
| Create tasks | `POST /api/office/tasks` | Connectors (import from external), webhook transformers |
| Update task status | `PATCH /api/office/tasks/:id` | Connectors (bidirectional sync) |
| Post comments | `POST /api/office/tasks/:id/comments` | Chat platform connectors, notification relays |
| Create wakeup | `POST /api/office/wakeups` | Webhook transformers, external triggers |
| Write plugin state | `POST /api/plugins/:id/state/:key` | All plugins with persistence |
| Delete plugin state | `DELETE /api/plugins/:id/state/:key` | Cleanup operations |
| Log activity | `POST /api/office/activity` | Connectors logging sync events |
| Record cost event | `POST /api/office/costs` | Cost tracking plugins |

### What Plugins Need from INFRASTRUCTURE

| Capability | Mechanism | Used By |
|-----------|-----------|---------|
| Receive events | Webhook POST from kandev to plugin | All plugins |
| Receive external webhooks | kandev proxies inbound HTTP to plugin | Bidirectional sync plugins |
| Resolve secrets | `GET /api/plugins/:id/secrets/:ref` | All plugins with credentials |
| Register agent tools | Tool manifest in plugin registration | Connectors, search/query plugins |
| Health reporting | `GET` on plugin's health endpoint | Kandev monitoring all plugins |
