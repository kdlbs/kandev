---
title: "Security and Trust"
description: "Choose a safe Kandev deployment boundary, constrain agent access, protect credentials, and preserve human review."
---

# Security and Trust

Kandev is a developer workbench that runs agents with access to repositories, tools, and credentials. Its local-first model lets an agent use the same Git host, issue tracker, editor, shell, and command-line access available to the Kandev process. That is useful, but it also means the operating-system account, executor, network, and agent profile are the security boundary.

> Kandev does not currently provide a multi-user login, role-based access control, or an authorization boundary for its web UI, HTTP API, WebSocket, or external MCP routes. Treat anyone who can reach the backend as an operator with the potential to read or change developer data.

## Choose a deployment boundary

| Use case | Recommended boundary | Avoid |
|---|---|---|
| One developer on one machine | Desktop or CLI bound to loopback, running as that developer | Publishing the backend port to the LAN |
| One developer on a remote host | Dedicated OS account, private VPN or SSH tunnel, or an authenticated TLS access proxy | A public IP and port with only TLS |
| A trusted team | Dedicated host or service account, identity-aware proxy, private network, scoped credentials, and separate deployments for different trust groups | Treating the Kandev UI as a tenant or role boundary |
| Unattended automation | Dedicated agent and executor profiles, narrow repository credentials, workflow limits, and provider-side branch protection | Reusing a developer's broad personal token or enabling unrestricted approval bypasses |

The default backend host is `0.0.0.0`. Plain `kandev`, `kandev run`, and `npx kandev@latest` commands inherit that all-interface bind unless you override it. For local-only access, set `KANDEV_SERVER_HOST=127.0.0.1` before launch; configure an equivalent protected bind for a managed service. Browser origin and CORS checks reduce accidental cross-site access, but they do not identify or authorize a user. `auth.jwtSecret` is compatibility configuration and does not enable product login.

```bash
KANDEV_SERVER_HOST=127.0.0.1 kandev
```

For remote access, protect the whole origin, including:

- the web application and `/api/v1` routes;
- the `/ws` WebSocket route and terminal or preview tunnels;
- Streamable HTTP MCP at `/mcp`; and
- SSE compatibility at `/mcp/sse` and `/mcp/message`.

Use an authenticated reverse proxy that supports WebSockets and long-lived streaming, or keep the service on a private VPN. Block direct access to the backend so a client cannot bypass the proxy. See [Run Kandev as a service](run-as-a-service.md), [Docker](docker.md), and [Kubernetes](k8s.md) for deployment-specific constraints.

## Understand executor access

An executor decides where the agent process runs. It does not reduce permissions unless its environment is actually isolated and constrained.

| Executor | Primary boundary | Important limit |
|---|---|---|
| Worktree | A separate Git checkout | Isolates file state, not the host account, credentials, processes, ports, or network |
| Local | The selected folder and Kandev host account | The agent can affect the same host resources its process can reach |
| Local Docker | A container plus explicitly mounted paths and credentials | A Docker socket or daemon API can grant host-level control; mounts remain readable in the container |
| SSH | The configured remote account and host | Remote directories and credentials require manual lifecycle review |
| Sprites | A remote sandbox and its injected credentials | Destroying the sandbox can remove unpushed work; network and token scope still matter |

Use separate profiles for different trust levels. Do not give a routine documentation or review task the same environment, secrets, and permission bypasses as a production automation. Review [Executors](executors.md) before changing from the seeded Worktree profile.

## Scope agent profiles

An agent profile combines a CLI, model, mode, flags, environment values, secret references, permissions, and optional MCP servers. Treat it as a reusable authority package.

The configurable ACP command prefix is currently a launch-customization
feature, not an isolation boundary. Kandev does not yet have a separate
authenticated operator session for profile mutations, so an agent that can
reach the main HTTP API may attempt the same profile edits as the UI. Do not
rely on the prefix to confine a hostile agent until the operator-owned settings
boundary in
[ADR-2026-07-24-operator-owned-agent-launcher-settings](../decisions/2026-07-24-operator-owned-agent-launcher-settings.md)
is enforced. The current per-boot token is a deliberately replayable interim
CSRF and accidental-mutation interlock, not authentication: an intentional
agent can fetch and replay it. It does not close this direct-client path, and
an ambient browser login is insufficient while agent-controlled previews can
share the operator origin.

1. Create a profile for one purpose, such as local implementation, read-focused review, or unattended maintenance.
2. Select the least-privileged Git, provider, and cloud credentials that purpose needs.
3. Leave approval or sandbox bypasses disabled unless the executor is disposable and the task is trusted.
4. Allow only required MCP transports and servers in the executor policy.
5. Test the profile on a disposable repository before enabling workflow auto-start or scheduled automation.

Agent CLIs can also discover authentication from their normal home-directory files, environment, keychain, or provider CLI. Removing a Kandev secret does not revoke a token stored elsewhere. Revoke credentials at the provider and remove retained executor copies when access should end.

See [Agents and profiles](agents-and-profiles.md) for exact profile fields and [Automation and MCP](automation-and-mcp.md) for unattended and external-client boundaries.

## Protect stored secrets

Secrets created through Kandev are encrypted in the database with the AES-256 master key at `<home>/data/master.key`. Protect both files:

- a copied database can contain encrypted provider and profile credentials;
- a database plus its matching master key can recover those values;
- a database restored without its matching key cannot decrypt them; and
- filesystem permissions and backup access remain part of the security boundary.

Back up the master key separately with owner-only access when encrypted settings must survive recovery. Do not commit secrets to `config.yaml`, repository instructions, workflow prompts, task descriptions, capture artifacts, or shell history. Environment variables are visible to the process and may be visible to child agents.

Webhook secrets are a separate case. A workspace automation stores its webhook secret with the automation, and a user with settings access can reveal it. Use TLS, keep it out of URLs and logs, and replace the automation when rotation is required.

See [Configuration](configuration.md) for storage and environment fields and [Operations](operations.md) for backup, restore, logs, and reset behavior.

## Keep a human in the loop

Kandev can automate planning, implementation, review preparation, and pull-request operations without making the agent the final authority.

For a human-gated workflow:

1. Use a dedicated Review or Approval step.
2. Set **On Turn Complete** to **Do nothing (wait for user)**.
3. Do not auto-start the next privileged step.
4. Inspect the conversation, diff, tests, walkthrough, checks, and provider review state.
5. Let a person move the task or send the next instruction after approval.

`step_complete_kandev` proves that an agent emitted the configured completion signal; it is not human approval. Kandev also does not bypass Git host permissions, required checks, review rules, or branch protection. Keep those controls authoritative for merges and deployments.

For a coordinator pattern, split work into bounded sessions or subtasks, constrain each profile, and keep a human gate before merge or release. See [Tasks and workflows](tasks-and-workflows.md), [Coordinate work](coordination.md), and [Sessions and review](sessions-and-review.md).

## Treat input and output as untrusted

Repository content, task attachments, issue and pull-request text, Slack messages, webhook payloads, MCP client prompts, agent output, generated commands, and URLs can all influence an agent.

- Do not let external text select credentials, shell commands, deployment targets, or unrestricted profiles without validation.
- Review generated commands before running them in a privileged terminal.
- Keep tool approval enabled for agents handling untrusted content.
- Treat shared session snapshots, logs, traces, screenshots, and videos as potentially sensitive.
- Test automation templates with missing and adversarial payload fields.

Task MCP is scoped to an active Kandev agent session, but it can still create or mutate tasks and coordinate other sessions. External MCP exposes configuration and task-management tools without Kandev authentication. Review every client's live tool list and approval policy before connecting it.

## Operational checklist

Before shared or remote use, confirm:

- the backend is not directly reachable from an untrusted network;
- an authenticated access layer protects HTTP, WebSocket, and MCP traffic;
- the Kandev process runs as a dedicated, non-root account where practical;
- repository, provider, agent, Docker, SSH, and cloud credentials are narrowly scoped;
- Docker daemon access is absent unless a task requires it;
- workflow auto-start, turn-completion transitions, and schedules have been tested with human gates;
- branch protection and required checks are enforced at the Git provider;
- database, `master.key`, logs, traces, captures, and backups have controlled access and retention; and
- restore and credential-revocation procedures have been tested.

If the backend may have been exposed, restrict network access first. Then rotate provider, agent, Git, webhook, executor, and proxy credentials that the process or its tasks could access; inspect task/session history and logs; preserve evidence according to your policy; and rebuild disposable executor environments. Kandev does not currently provide a complete security audit log, so rely on host, proxy, provider, and infrastructure logs for incident investigation.

Related: [Get started](use-kandev.md), [Feature status](feature-status.md), [Executors](executors.md), [Automation and MCP](automation-and-mcp.md), and [Operations](operations.md).
