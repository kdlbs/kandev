type OptionalString = string | null | undefined;

export type TaskListFilters = {
  page?: number;
  pageSize?: number;
  query?: string;
  includeArchived?: boolean;
  workflowId?: OptionalString;
  repositoryId?: OptionalString;
};

export type OfficeTaskFilters = {
  status?: string[];
  priority?: string[];
  assignee?: string | string[];
  project?: string | string[];
  sort?: "updated_at" | "created_at" | "priority" | null;
  order?: "asc" | "desc" | null;
  limit?: number;
  includeSystem?: boolean;
};

const NONE = "__none__";

function id(value: OptionalString) {
  return value ?? NONE;
}

function sorted(values: string[] | undefined): string[] {
  return [...(values ?? [])].sort();
}

export function taskListFiltersKey(filters: TaskListFilters = {}) {
  return {
    page: filters.page ?? null,
    pageSize: filters.pageSize ?? null,
    query: filters.query ?? "",
    includeArchived: filters.includeArchived ?? false,
    workflowId: filters.workflowId ?? null,
    repositoryId: filters.repositoryId ?? null,
  } as const;
}

export function officeTaskFiltersKey(filters: OfficeTaskFilters = {}) {
  const sort = filters.sort === undefined ? "updated_at" : filters.sort;
  const order = filters.order === undefined ? "desc" : filters.order;
  return {
    status: sorted(filters.status),
    priority: sorted(filters.priority),
    assignee: Array.isArray(filters.assignee)
      ? sorted(filters.assignee)
      : (filters.assignee ?? null),
    project: Array.isArray(filters.project) ? sorted(filters.project) : (filters.project ?? null),
    sort,
    order,
    limit: filters.limit ?? null,
    includeSystem: filters.includeSystem ?? false,
  } as const;
}

export const qk = {
  boot: {
    initialState: () => ["boot", "initialState"] as const,
    routeData: () => ["boot", "routeData"] as const,
  },
  features: () => ["features"] as const,
  workspaces: {
    all: () => ["workspaces"] as const,
    detail: (workspaceId: string) => ["workspaces", workspaceId] as const,
    repositories: (workspaceId: string, params?: { includeScripts?: boolean }) =>
      [
        "workspaces",
        workspaceId,
        "repositories",
        { includeScripts: params?.includeScripts ?? false },
      ] as const,
    branches: (workspaceId: string, source: { repositoryId?: string; path?: string }) =>
      [
        "workspaces",
        workspaceId,
        "branches",
        { repositoryId: source.repositoryId ?? null, path: source.path ?? null },
      ] as const,
    repositoryBranches: (repositoryId: string) =>
      ["repositories", repositoryId, "branches"] as const,
    repositoryScripts: (repositoryId: string) => ["repositories", repositoryId, "scripts"] as const,
    quickChatSessions: (workspaceId: string) =>
      ["workspaces", workspaceId, "quickChatSessions"] as const,
  },
  workflows: {
    all: (workspaceId: string, params?: { includeHidden?: boolean }) =>
      ["workflows", workspaceId, { includeHidden: params?.includeHidden ?? false }] as const,
    snapshot: (workflowId: string) => ["workflows", workflowId, "snapshot"] as const,
    steps: (workflowId: string) => ["workflows", workflowId, "steps"] as const,
  },
  tasks: {
    detail: (taskId: string) => ["tasks", taskId] as const,
    page: (workspaceId: OptionalString, filters?: TaskListFilters) =>
      ["tasks", "page", id(workspaceId), taskListFiltersKey(filters)] as const,
    infinite: (workspaceId: string, filters?: TaskListFilters) =>
      ["tasks", "infinite", workspaceId, taskListFiltersKey(filters)] as const,
    subtaskCount: (taskId: string) => ["tasks", taskId, "subtaskCount"] as const,
  },
  taskSession: {
    byTask: (taskId: string) => ["session", "byTask", taskId] as const,
    byId: (sessionId: string) => ["session", "byId", sessionId] as const,
  },
  taskPlan: {
    detail: (taskId: string) => ["taskPlan", taskId] as const,
    revisions: (taskId: string) => ["taskPlan", taskId, "revisions"] as const,
    revision: (taskId: string, revisionId: string) =>
      ["taskPlan", taskId, "revisions", revisionId] as const,
  },
  session: {
    messages: (sessionId: string) => ["session", sessionId, "messages"] as const,
    messagesPage: (
      sessionId: string,
      params?: { limit?: number; before?: string; after?: string; sort?: "asc" | "desc" },
    ) =>
      [
        "session",
        sessionId,
        "messagesPage",
        {
          limit: params?.limit ?? null,
          sort: params?.sort ?? "asc",
          before: params?.before ?? null,
          after: params?.after ?? null,
        },
      ] as const,
    messagesInfinite: (sessionId: string, params?: { limit?: number; sort?: "asc" | "desc" }) =>
      [
        "session",
        sessionId,
        "messagesInfinite",
        { limit: params?.limit ?? null, sort: params?.sort ?? "asc" },
      ] as const,
    turns: (sessionId: string) => ["session", sessionId, "turns"] as const,
    search: (sessionId: string, query: string, limit = 50) =>
      ["session", sessionId, "search", { query, limit }] as const,
    queue: (sessionId: string) => ["session", sessionId, "queue"] as const,
  },
  sessionRuntime: {
    gitStatus: (environmentId: string) =>
      ["sessionRuntime", "environment", id(environmentId), "gitStatus"] as const,
    commits: (environmentId: string) =>
      ["sessionRuntime", "environment", id(environmentId), "commits"] as const,
    userShells: (environmentId: string, taskId?: OptionalString) =>
      [
        "sessionRuntime",
        "environment",
        id(environmentId),
        "userShells",
        { taskId: taskId ?? null },
      ] as const,
    processes: (sessionId: string) =>
      ["sessionRuntime", "session", id(sessionId), "processes"] as const,
    prepare: (sessionId: string) =>
      ["sessionRuntime", "session", id(sessionId), "prepare"] as const,
    contextWindow: (sessionId: string) =>
      ["sessionRuntime", "session", id(sessionId), "contextWindow"] as const,
    availableCommands: (sessionId: string) =>
      ["sessionRuntime", "session", id(sessionId), "availableCommands"] as const,
    mode: (sessionId: string) => ["sessionRuntime", "session", id(sessionId), "mode"] as const,
    agentCapabilities: (sessionId: string) =>
      ["sessionRuntime", "session", id(sessionId), "agentCapabilities"] as const,
    models: (sessionId: string) => ["sessionRuntime", "session", id(sessionId), "models"] as const,
    promptUsage: (sessionId: string) =>
      ["sessionRuntime", "session", id(sessionId), "promptUsage"] as const,
    todos: (sessionId: string) => ["sessionRuntime", "session", id(sessionId), "todos"] as const,
    pollMode: (sessionId: string) =>
      ["sessionRuntime", "session", id(sessionId), "pollMode"] as const,
    agentctl: (sessionId: string) =>
      ["sessionRuntime", "session", id(sessionId), "agentctl"] as const,
    worktrees: (sessionId: string) =>
      ["sessionRuntime", "session", id(sessionId), "worktrees"] as const,
  },
  settings: {
    user: () => ["settings", "userSettings"] as const,
    systemMetrics: () => ["settings", "systemMetrics"] as const,
    executors: () => ["settings", "executors"] as const,
    executor: (executorId: string) => ["settings", "executors", executorId] as const,
    executorProfiles: (executorId: string) =>
      ["settings", "executors", executorId, "profiles"] as const,
    allExecutorProfiles: () => ["settings", "executorProfiles"] as const,
    scriptPlaceholders: () => ["settings", "scriptPlaceholders"] as const,
    defaultScripts: (executorType: string) => ["settings", "defaultScripts", executorType] as const,
    agents: () => ["settings", "agents"] as const,
    agentDiscovery: () => ["settings", "agents", "discovery"] as const,
    availableAgents: () => ["settings", "agents", "available"] as const,
    agentMcpConfig: (profileId: string) =>
      ["settings", "agentProfiles", profileId, "mcpConfig"] as const,
    installJobs: () => ["settings", "agentInstallJobs"] as const,
    installJob: (jobId: string) => ["settings", "agentInstallJobs", jobId] as const,
    dynamicModels: (agentName: string) => ["settings", "agentModels", agentName] as const,
    editors: () => ["settings", "editors"] as const,
    prompts: () => ["settings", "prompts"] as const,
    notificationProviders: () => ["settings", "notificationProviders"] as const,
    secrets: () => ["settings", "secrets"] as const,
    spritesStatus: (secretId?: OptionalString) =>
      ["settings", "sprites", "status", id(secretId)] as const,
    spritesInstances: (secretId?: OptionalString) =>
      ["settings", "sprites", "instances", id(secretId)] as const,
    systemHealth: () => ["settings", "systemHealth"] as const,
    runtimeFlags: () => ["settings", "runtimeFlags"] as const,
  },
  office: {
    meta: () => ["office", "meta"] as const,
    dashboard: (workspaceId: string) => ["office", "workspaces", workspaceId, "dashboard"] as const,
    tasks: (workspaceId: string, filters?: OfficeTaskFilters) =>
      ["office", "workspaces", workspaceId, "tasks", officeTaskFiltersKey(filters)] as const,
    task: (workspaceId: string, taskId: string) =>
      ["office", "workspaces", workspaceId, "tasks", taskId] as const,
    taskComments: (taskId: string) => ["office", "tasks", taskId, "comments"] as const,
    taskActivity: (workspaceId: string, taskId: string) =>
      ["office", "workspaces", workspaceId, "tasks", taskId, "activity"] as const,
    taskSearch: (workspaceId: string, query: string, limit = 50) =>
      ["office", "workspaces", workspaceId, "tasks", "search", { query, limit }] as const,
    agents: (workspaceId: string) => ["office", "workspaces", workspaceId, "agents"] as const,
    projects: (workspaceId: string) => ["office", "workspaces", workspaceId, "projects"] as const,
    project: (projectId: string) => ["office", "projects", projectId] as const,
    inbox: (workspaceId: string) => ["office", "workspaces", workspaceId, "inbox"] as const,
    activity: (workspaceId: string, filterType = "all") =>
      ["office", "workspaces", workspaceId, "activity", { filterType }] as const,
    runs: (workspaceId: string) => ["office", "workspaces", workspaceId, "runs"] as const,
    routing: (workspaceId: string) => ["office", "workspaces", workspaceId, "routing"] as const,
    providerHealth: (workspaceId: string) =>
      ["office", "workspaces", workspaceId, "providerHealth"] as const,
    routingPreview: (workspaceId: string) =>
      ["office", "workspaces", workspaceId, "routingPreview"] as const,
    agentRoute: (agentId: string) => ["office", "agents", agentId, "route"] as const,
    agentSummary: (agentId: string, days?: number) =>
      ["office", "agents", agentId, "summary", { days: days ?? null }] as const,
    agentRuns: (agentId: string, params?: { limit?: number }) =>
      ["office", "agents", agentId, "runs", { limit: params?.limit ?? null }] as const,
    runDetail: (agentId: string, runId: string) =>
      ["office", "agents", agentId, "runs", runId] as const,
    runAttempts: (runId: string) => ["office", "runs", runId, "attempts"] as const,
    costs: (workspaceId: string) => ["office", "workspaces", workspaceId, "costs"] as const,
    costBreakdown: (workspaceId: string) =>
      ["office", "workspaces", workspaceId, "costBreakdown"] as const,
    budgets: (workspaceId: string) => ["office", "workspaces", workspaceId, "budgets"] as const,
    routines: (workspaceId: string) => ["office", "workspaces", workspaceId, "routines"] as const,
    routineRuns: (workspaceId: string) =>
      ["office", "workspaces", workspaceId, "routineRuns"] as const,
    routineTriggers: (routineId: string) => ["office", "routines", routineId, "triggers"] as const,
    skills: (workspaceId: string) => ["office", "workspaces", workspaceId, "skills"] as const,
  },
  integrations: {
    github: {
      status: () => ["integrations", "github", "status"] as const,
      prs: (workspaceId: OptionalString) =>
        ["integrations", "github", "prs", id(workspaceId)] as const,
      issues: (workspaceId: OptionalString) =>
        ["integrations", "github", "issues", id(workspaceId)] as const,
      prWatches: () => ["integrations", "github", "prWatches"] as const,
      reviewWatches: (workspaceId?: OptionalString) =>
        ["integrations", "github", "reviewWatches", id(workspaceId)] as const,
      issueWatches: (workspaceId?: OptionalString) =>
        ["integrations", "github", "issueWatches", id(workspaceId)] as const,
      taskPr: (taskId: string) => ["integrations", "github", "taskPr", id(taskId)] as const,
      taskCiOptions: (taskId: string) =>
        ["integrations", "github", "taskCiOptions", id(taskId)] as const,
      actionPresets: (workspaceId: OptionalString) =>
        ["integrations", "github", "actionPresets", id(workspaceId)] as const,
      rateLimit: () => ["integrations", "github", "rateLimit"] as const,
    },
    gitlab: {
      status: () => ["integrations", "gitlab", "status"] as const,
      stats: () => ["integrations", "gitlab", "stats"] as const,
      mrs: (workspaceId: OptionalString) =>
        ["integrations", "gitlab", "mrs", id(workspaceId)] as const,
      taskMr: (taskId: string) => ["integrations", "gitlab", "taskMr", id(taskId)] as const,
      reviewWatches: (workspaceId?: OptionalString) =>
        ["integrations", "gitlab", "reviewWatches", id(workspaceId)] as const,
      issueWatches: (workspaceId?: OptionalString) =>
        ["integrations", "gitlab", "issueWatches", id(workspaceId)] as const,
      actionPresets: (workspaceId: OptionalString) =>
        ["integrations", "gitlab", "actionPresets", id(workspaceId)] as const,
      projects: () => ["integrations", "gitlab", "projects"] as const,
    },
    jira: {
      config: () => ["integrations", "jira", "config"] as const,
      projects: () => ["integrations", "jira", "projects"] as const,
      issueWatches: (workspaceId?: OptionalString) =>
        ["integrations", "jira", "issueWatches", id(workspaceId)] as const,
    },
    linear: {
      config: () => ["integrations", "linear", "config"] as const,
      teams: () => ["integrations", "linear", "teams"] as const,
      issueWatches: (workspaceId?: OptionalString) =>
        ["integrations", "linear", "issueWatches", id(workspaceId)] as const,
    },
    slack: {
      config: () => ["integrations", "slack", "config"] as const,
    },
    sentry: {
      config: () => ["integrations", "sentry", "config"] as const,
      issueWatches: (workspaceId?: OptionalString) =>
        ["integrations", "sentry", "issueWatches", id(workspaceId)] as const,
    },
  },
  system: {
    info: () => ["system", "info"] as const,
    diskUsage: () => ["system", "diskUsage"] as const,
    database: () => ["system", "database"] as const,
    backups: () => ["system", "backups"] as const,
    logFiles: () => ["system", "logs"] as const,
    logTail: (n = 1000) => ["system", "logs", "tail", { n }] as const,
    jobs: () => ["system", "jobs"] as const,
    job: (jobId: string) => ["system", "jobs", jobId] as const,
    metrics: () => ["system", "metrics"] as const,
    updates: () => ["system", "updates"] as const,
    restartCapability: () => ["system", "restartCapability"] as const,
  },
  automations: {
    list: (workspaceId: OptionalString) => ["automations", id(workspaceId)] as const,
    runs: (automationId: OptionalString) => ["automations", id(automationId), "runs"] as const,
  },
};
