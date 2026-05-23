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
  // Integrations (health pollers — jira, linear, slack, etc.)
  // -------------------------------------------------------------------------
  integrations: {
    health: (kind: string) => ["integrations", "health", kind] as const,
  },
} as const;
