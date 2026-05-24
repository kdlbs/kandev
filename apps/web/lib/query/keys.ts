/**
 * Typed query key factories for the full TanStack Query key taxonomy.
 *
 * All factories return `as const` tuples so that TypeScript can narrow
 * the exact key shape for type-safe cache operations.
 *
 * Convention:
 *  - Prefix arrays serve as invalidation scopes: qc.invalidateQueries({ queryKey: qk.session.prefix(id) })
 *  - Leaf factories are used directly in useQuery / queryOptions.
 *
 * Waves 1–6 workers add queryOptions() and bridge registrars that reference
 * these keys — they extend, not restructure.
 */
export const qk = {
  // -------------------------------------------------------------------------
  // Features
  // -------------------------------------------------------------------------
  features: () => ["features"] as const,

  // -------------------------------------------------------------------------
  // Workspaces
  // -------------------------------------------------------------------------
  workspaces: {
    all: () => ["workspaces"] as const,
    one: (id: string) => ["workspaces", id] as const,
    repos: (id: string) => ["workspaces", id, "repos"] as const,
    branches: (wsId: string, repoId: string) => ["workspaces", wsId, "repos", repoId, "branches"] as const,
  },

  // -------------------------------------------------------------------------
  // Kanban
  // Invalidate together with prefix ["kanban"]
  // -------------------------------------------------------------------------
  kanban: {
    prefix: () => ["kanban"] as const,
    multi: () => ["kanban", "workflows"] as const,
    workflow: (wfId: string) => ["kanban", "workflows", wfId] as const,
    task: (id: string) => ["kanban", "tasks", id] as const,
  },

  // -------------------------------------------------------------------------
  // Session
  // Invalidate together with prefix ["session", id]
  // Note: git keys use envKey = environmentIdBySessionId[sid] ?? sid
  // The mapping stays in Zustand (client-side index, not server state).
  // -------------------------------------------------------------------------
  session: {
    prefix: (id: string) => ["session", id] as const,
    one: (id: string) => ["session", id] as const,
    messages: (id: string) => ["session", id, "messages"] as const,
    shell: (id: string) => ["session", id, "shell"] as const,
    git: (envKey: string) => ["session", "git", envKey] as const,
    commits: (envKey: string) => ["session", "git", envKey, "commits"] as const,
    context: (id: string) => ["session", id, "context"] as const,
    todos: (id: string) => ["session", id, "todos"] as const,
    models: (id: string) => ["session", id, "models"] as const,
  },

  // -------------------------------------------------------------------------
  // Office
  // Invalidate together with prefix ["office", wsId]
  // -------------------------------------------------------------------------
  office: {
    prefix: (wsId: string) => ["office", wsId] as const,
    dashboard: (wsId: string) => ["office", wsId, "dashboard"] as const,
    tasks: (wsId: string, filters?: Record<string, unknown>) =>
      filters !== undefined
        ? (["office", wsId, "tasks", filters] as const)
        : (["office", wsId, "tasks"] as const),
    agents: (wsId: string) => ["office", wsId, "agents"] as const,
    agentRouting: (agentId: string) => ["office", "agents", agentId, "routing"] as const,
    providerHealth: (wsId: string) => ["office", wsId, "providerHealth"] as const,
    runs: (wsId: string) => ["office", wsId, "runs"] as const,
    approvals: (wsId: string) => ["office", wsId, "approvals"] as const,
    activity: (wsId: string) => ["office", wsId, "activity"] as const,
  },

  // -------------------------------------------------------------------------
  // GitHub
  // -------------------------------------------------------------------------
  github: {
    prefix: (wsId: string) => ["github", wsId] as const,
    prs: (wsId: string) => ["github", wsId, "prs"] as const,
    review: (prId: string) => ["github", "prs", prId, "review"] as const,
  },

  // -------------------------------------------------------------------------
  // GitLab
  // -------------------------------------------------------------------------
  gitlab: {
    prefix: (wsId: string) => ["gitlab", wsId] as const,
    mrs: (wsId: string) => ["gitlab", wsId, "mrs"] as const,
    review: (mrId: string) => ["gitlab", "mrs", mrId, "review"] as const,
  },

  // -------------------------------------------------------------------------
  // Jira
  // -------------------------------------------------------------------------
  jira: {
    prefix: (wsId: string) => ["jira", wsId] as const,
    issues: (wsId: string) => ["jira", wsId, "issues"] as const,
  },

  // -------------------------------------------------------------------------
  // Linear
  // -------------------------------------------------------------------------
  linear: {
    prefix: (wsId: string) => ["linear", wsId] as const,
    issues: (wsId: string) => ["linear", wsId, "issues"] as const,
  },

  // -------------------------------------------------------------------------
  // Automations
  // Invalidate together with prefix ["automations", wsId]
  // -------------------------------------------------------------------------
  automations: {
    prefix: (wsId: string) => ["automations", wsId] as const,
    list: (wsId: string) => ["automations", wsId, "list"] as const,
    runs: (automationId: string) => ["automations", "runs", automationId] as const,
  },

  // -------------------------------------------------------------------------
  // Integrations (health pollers — jira, linear, slack, etc.)
  //
  // health(kind)        — HTTP probe result from the 90s backend poller.
  //                       Fetched with refetchInterval: 90_000 to match
  //                       the backend cadence. Kind is e.g. "jira", "linear".
  // availability(kind?) — Combined auth+enabled signal. Optional kind for
  //                       scope (all integrations when omitted).
  // enabled(kind)       — Install-wide on/off toggle. This key is NOT backed
  //                       by a real HTTP endpoint — `useIntegrationEnabled`
  //                       reads localStorage synchronously. Declared here so
  //                       wave 2 workers can invalidate it from mutations
  //                       (setEnabled → qc.setQueryData) if they need to.
  // -------------------------------------------------------------------------
  integrations: {
    health: (kind: string) => ["integrations", "health", kind] as const,
    availability: (kind?: string) =>
      kind !== undefined
        ? (["integrations", "availability", kind] as const)
        : (["integrations", "availability"] as const),
    enabled: (kind: string) => ["integrations", "enabled", kind] as const,
  },

  // -------------------------------------------------------------------------
  // Comments
  //
  // Comments in this domain are client-side diff/plan/file-editor/PR-feedback
  // annotations stored in sessionStorage — they are NOT fetched from the
  // server. These keys are reserved for potential future server persistence,
  // and to satisfy the Wave 1 deliverable contract.
  //
  // The Office domain's task-level user comments (Kanban comments) live under
  // qk.office.* and are a separate concept.
  // -------------------------------------------------------------------------
  comments: {
    /** All comments for a session. */
    bySession: (sessionId: string) => ["comments", "session", sessionId] as const,
    /** All pending (unsent) comments. */
    pending: () => ["comments", "pending"] as const,
  },

  // -------------------------------------------------------------------------
  // Settings
  // Invalidate together with prefix ["settings"]
  // -------------------------------------------------------------------------
  settings: {
    prefix: () => ["settings"] as const,
    executors: () => ["settings", "executors"] as const,
    agents: () => ["settings", "agents"] as const,
    agentProfiles: () => ["settings", "agentProfiles"] as const,
    agentDiscovery: () => ["settings", "agentDiscovery"] as const,
    availableAgents: () => ["settings", "availableAgents"] as const,
    editors: () => ["settings", "editors"] as const,
    prompts: () => ["settings", "prompts"] as const,
    secrets: () => ["settings", "secrets"] as const,
    sprites: (secretId?: string) =>
      secretId !== undefined
        ? (["settings", "sprites", secretId] as const)
        : (["settings", "sprites"] as const),
    notificationProviders: () => ["settings", "notificationProviders"] as const,
    userSettings: () => ["settings", "userSettings"] as const,
    installJobs: (id?: string) =>
      id !== undefined
        ? (["settings", "installJobs", id] as const)
        : (["settings", "installJobs"] as const),
    systemHealth: () => ["settings", "systemHealth"] as const,
    remoteAuthSpecs: () => ["settings", "remoteAuthSpecs"] as const,
    dynamicModels: (agentName: string) =>
      ["settings", "dynamicModels", agentName] as const,
  },
} as const;
