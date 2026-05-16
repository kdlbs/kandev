// Canonical AgentProfile (ADR 0005, Wave E):
// ONE camelCase shape used by both kanban and office consumers. The HTTP
// transport layer normalizes snake_case server payloads at the API client
// boundary (see `lib/api/domains/office-api.ts#normalizeAgent` and
// `lib/api/domains/agent-profile-normalize.ts#normalizeAgentProfile`).
//
// Field categories:
//   - **Identity / CLI**: id, agentId, name, agentDisplayName, model, mode,
//     cliFlags, cliPassthrough, allowIndexing, userModified, createdAt,
//     updatedAt. Always populated by the kanban API (`/api/v1/agents` →
//     `Agent.profiles[]`) and by `/api/v1/agent-profiles/:id`. Optional in
//     the type because office-served rows that have not yet been wired to a
//     CLI client may omit them.
//   - **Office orchestration**: workspaceId, role, status, icon, reportsTo,
//     permissions, budgetMonthlyCents, maxConcurrentSessions, desiredSkills,
//     executorPreference, pauseReason, billingType, utilization,
//     skillIds, agentProfileId. Always populated by the office API;
//     absent on kanban-served rows.

export type CLIFlag = {
  description: string;
  flag: string;
  enabled: boolean;
};

export type BillingType = "api_key" | "subscription";

export type UtilizationWindow = {
  label: string;
  utilization_pct: number;
  reset_at: string;
};

export type ProviderUsage = {
  provider: string;
  windows: UtilizationWindow[];
  fetched_at: string;
};

export type AgentRole =
  | "ceo"
  | "worker"
  | "specialist"
  | "assistant"
  | "security"
  | "qa"
  | "devops";

export type AgentStatus = "idle" | "working" | "paused" | "stopped" | "pending_approval";

export type AgentProfile = {
  // --- Identity ---
  id: string;
  name: string;
  /** ID of the agent (CLI) this profile belongs to. */
  agentId: string;
  /** Human display name for the agent CLI (e.g. "Claude Code"). */
  agentDisplayName: string;

  // --- CLI subprocess config ---
  /** Model ID applied via ACP `session/set_model` at session start. */
  model: string;
  /** Optional ACP session mode applied via `session/set_mode`. */
  mode?: string;
  /** @deprecated Use cliFlags. Retained for legacy clients. */
  allowIndexing: boolean;
  /** User-configurable CLI flags passed to the agent subprocess. */
  cliFlags: CLIFlag[];
  cliPassthrough: boolean;
  userModified?: boolean;

  // --- Office orchestration (always populated by office API,
  //     absent on kanban-served rows; consumers should null-check) ---
  workspaceId?: string;
  /**
   * FK to a kanban-flavour profile when the office row delegates CLI
   * subprocess configuration to a separate profile rather than carrying
   * the CLI fields directly.
   */
  agentProfileId?: string;
  role?: AgentRole;
  icon?: string;
  status?: AgentStatus;
  reportsTo?: string;
  permissions?: Record<string, unknown>;
  budgetMonthlyCents?: number;
  maxConcurrentSessions?: number;
  desiredSkills?: string[];
  executorPreference?: {
    type?: string;
    image?: string;
    resource_limits?: Record<string, unknown>;
    environment_id?: string;
  };
  pauseReason?: string;
  billingType?: BillingType;
  utilization?: ProviderUsage | null;
  skillIds?: string[];

  // --- Timestamps ---
  createdAt: string;
  updatedAt: string;
};

/**
 * Office-served `AgentProfile` view: the canonical shape with
 * orchestration fields narrowed to non-optional. Returned by
 * `/api/v1/office/workspaces/:id/agents` (and friends), which always
 * populates these fields.
 */
export type OfficeAgentProfile = AgentProfile &
  Required<
    Pick<
      AgentProfile,
      "workspaceId" | "role" | "status" | "budgetMonthlyCents" | "maxConcurrentSessions"
    >
  >;

/**
 * Snake_case wire shape for HTTP request bodies sent to `POST/PATCH
 * /api/v1/agents/:id/profiles` and `/api/v1/agent-profiles/:id`. The
 * backend speaks snake_case; this is the legacy shape used by form
 * helpers, server actions, and WS payloads. Read paths normalize this
 * into the canonical `AgentProfile` (camelCase) at the API client
 * boundary.
 */
export type AgentProfilePayload = {
  id: string;
  agent_id: string;
  name: string;
  agent_display_name: string;
  model: string;
  mode?: string;
  allow_indexing: boolean;
  cli_flags: CLIFlag[];
  cli_passthrough: boolean;
  user_modified?: boolean;
  created_at: string;
  updated_at: string;
};
