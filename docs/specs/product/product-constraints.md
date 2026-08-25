# Product constraints

**Status:** Proposed baseline for product review.

These constraints apply across Kandev systems. A system requirement may be more
specific, but it must not silently contradict these boundaries. A deliberate
exception belongs in an ADR and in the owning system documentation.

## Product and support boundary

- The regular workspace, Kanban, task, workflow, session, changes, and review
  path is the supported product contract.
- A feature is not supported only because its schema, route, mock, flag, or
  internal plan exists. Use the documented feature-status categories.
- Office autonomy, routines, budgets, coordinator behavior, and quorum flows
  remain feature-flagged and in progress until explicitly graduated.
- Provider, agent, executor, platform, credential, and install-channel
  dependencies must be visible to users and operators.

## Ownership and authority

- Every requirement has one owning system. Product documents describe context;
  they do not duplicate system behavior.
- The Go backend and persistence layer own durable Kandev state. Frontend stores,
  WebSocket events, desktop state, CLI output, and MCP responses are surfaces
  over that authority.
- Tasks own work intent, workflow state, sessions, and task runtime state.
  Workspaces own repository and worktree context. Agents own profiles and
  provider configuration. Executors own execution environments.
- Integrations own external provider identity and actions. A remote provider is
  not silently treated as the source of truth for Kandev task state.

## Trust, permissions, and credentials

- Validate identity, scope, and authorization at the backend boundary. A
  frontend-only check is not a security boundary.
- Treat repository content, agent output, provider responses, URLs, archives,
  paths, and command arguments as untrusted.
- Scope credentials and environment values to the smallest practical profile,
  repository, workspace, and executor boundary.
- An executor changes where work runs. It does not automatically reduce the
  permissions granted to the agent profile.
- Local and worktree execution do not isolate host processes, host files, or
  credentials. Container and remote execution add boundaries but do not make
  explicitly delivered credentials safe by default.
- Keep human approval before merge, release, deployment, deletion, or another
  irreversible operation.

## Runtime and data safety

- Persist stable identifiers and explicit states before publishing events.
  Consumers must tolerate duplicate, missed, stale, and reordered delivery.
- Restart reconciliation and durable cleanup must preserve task-owned files and
  expose failures instead of silently deleting or abandoning work.
- Session deletion is not task-workspace deletion. Physical cleanup belongs to
  the task or workspace lifecycle that owns the resource.
- Provider and executor failures should fail closed where continued execution
  could cause an unsafe or misleading result.
- SQLite and PostgreSQL are supported persistence paths where the relevant
  system contract says so. Data migrations and startup recovery must preserve
  the documented user state.

## Protocol and extension boundaries

- ACP is the structured agent-session protocol. REST and WebSocket are Kandev
  client and control surfaces. MCP supplies tools. These protocols are not
  interchangeable ownership layers.
- Plugins and integrations use explicit host contracts. They must not bypass
  authentication, task ownership, persistence, or security boundaries.
- External MCP routes and agent-controlled previews require an appropriate
  deployment boundary, proxy protection, and scoped credentials.

## Surface and release constraints

- Browser, Tauri, CLI, API, and MCP surfaces share product authority and must
  not create conflicting state.
- Mobile surfaces must preserve the supported core capability with native
  touch interaction and no avoidable horizontal overflow.
- New UI copy must use the localization system and pass locale checks.
- Release artifacts, native runtime packages, desktop artifacts, package
  managers, and container channels must agree on the published version and
  compatibility contract.
- Public documentation describes the current main branch unless it explicitly
  identifies a release or historical version. Released builds may lag current
  docs.

## Product-context constraints

- Product docs may define purpose, actors, relationships, principles, measures,
  and cross-system constraints.
- Product docs must not become a second home for API details, feature behavior,
  implementation steps, roadmaps, or work status.
- System-specific requirements and designs remain authoritative for behavior
  and technical contracts.

## Open product decisions

- Whether and when Kandev adds multi-user identity, roles, and shared-workspace
  authorization.
- Which Office capabilities are required for graduation into the supported
  product boundary.
- Which metrics and targets are approved for release and product decisions.
- Which provider, executor, integration, and plugin capabilities are formal
  compatibility commitments.
- What data retention, telemetry, and privacy policy should govern product
  measures beyond the current local evidence model.
