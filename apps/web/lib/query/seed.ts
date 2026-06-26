import type { QueryClient } from "@tanstack/react-query";
import type { BootPayload, BootRouteData } from "@/src/boot-payload";
import type { AppState } from "@/lib/state/store";
import type {
  Agent,
  AgentDiscovery,
  AvailableAgent,
  CustomPrompt,
  EditorOption,
  Executor,
  NotificationProvider,
  Repository,
  RepositoryScript,
  ToolStatus,
  Workspace,
  Workflow,
  WorkflowSnapshot,
} from "@/lib/types/http";
import type { WorkflowItem } from "@/lib/state/slices";
import type { SecretListItem } from "@/lib/types/http-secrets";
import type { SpritesInstance, SpritesStatus } from "@/lib/types/http-sprites";
import type {
  GitStatusQueryData,
  SessionProcessesQueryData,
} from "./query-options/session-runtime";
import type { AvailableCommand } from "@/lib/state/slices/session-runtime/types";
import type {
  ActivityEntry,
  AgentProfile,
  DashboardData,
  InboxItem,
  OfficeMeta,
  Project,
  Routine,
  Run,
  Skill,
} from "@/lib/state/slices/office/types";
import { qk } from "./keys";

type SeedOptions = {
  sessionId?: string;
};

type OfficeQuerySeedState = Partial<AppState["office"]> & {
  agents?: AgentProfile[];
  projects?: Project[];
  skills?: Skill[];
  routines?: Routine[];
  inboxItems?: InboxItem[];
  inboxCount?: number;
  dashboard?: DashboardData | null;
  activity?: ActivityEntry[];
  runs?: Run[];
  meta?: OfficeMeta | null;
};

export type QuerySeedInitialState = Omit<
  Partial<AppState>,
  "workflows" | "workspaces" | "office"
> & {
  workspaces?: Partial<AppState["workspaces"]> & {
    items?: Workspace[];
  };
  office?: OfficeQuerySeedState;
  workflows?: Partial<AppState["workflows"]> & {
    items?: Array<Workflow | WorkflowItem>;
  };
  workflowSnapshots?: {
    itemsByWorkflowId?: Record<string, WorkflowSnapshot>;
  };
  workflowLists?: {
    itemsByWorkspaceId?: Record<string, Array<Workflow | WorkflowItem>>;
  };
  executors?: { items: Executor[] };
  settingsAgents?: { items: Agent[] };
  agentDiscovery?: { items: AgentDiscovery[]; loading?: boolean; loaded?: boolean };
  availableAgents?: {
    items: AvailableAgent[];
    tools: ToolStatus[];
    loading?: boolean;
    loaded?: boolean;
  };
  editors?: { items: EditorOption[] };
  prompts?: { items: CustomPrompt[] };
  secrets?: { items: SecretListItem[] };
  sprites?: {
    status: SpritesStatus | null;
    instances: SpritesInstance[];
  };
  notificationProviders?: {
    items: NotificationProvider[];
    events: string[];
    appriseAvailable: boolean;
  };
  repositories?: {
    itemsByWorkspaceId?: Record<string, Repository[]>;
  };
  repositoryScripts?: {
    itemsByRepositoryId?: Record<string, RepositoryScript[]>;
  };
  availableCommands?: {
    bySessionId?: Record<string, AvailableCommand[]>;
  };
};

export function seedQueryClientFromBootPayload(client: QueryClient, payload: BootPayload) {
  seedQueryClientFromInitialState(client, payload.initialState ?? {});
  seedQueryClientFromRouteData(client, payload.routeData);
}

export function seedQueryClientFromInitialState(
  client: QueryClient,
  initialState: QuerySeedInitialState,
  options: SeedOptions = {},
) {
  if (Object.keys(initialState).length === 0) return;
  client.setQueryData(qk.boot.initialState(), initialState);
  setIfDefined(client, qk.features(), initialState.features);
  setIfDefined(client, qk.workspaces.all(), initialState.workspaces?.items);
  seedWorkspaceWorkflows(client, initialState);
  seedWorkflowSnapshots(client, initialState);
  seedWorkspaceRepositories(client, initialState);
  setIfDefined(client, qk.settings.user(), initialState.userSettings);
  seedSettingsState(client, initialState);
  seedOfficeState(client, initialState);
  seedRepositoryScripts(client, initialState);
  seedTaskSessions(client, initialState);
  seedSessionMessages(client, initialState, options.sessionId);
  seedSessionTurns(client, initialState, options.sessionId);
  seedSessionRuntime(client, initialState);
}

function seedWorkspaceRepositories(client: QueryClient, initialState: QuerySeedInitialState) {
  for (const [workspaceId, repositories] of Object.entries(
    initialState.repositories?.itemsByWorkspaceId ?? {},
  )) {
    client.setQueryData(qk.workspaces.repositories(workspaceId), repositories);
  }
}

function seedWorkspaceWorkflows(client: QueryClient, initialState: QuerySeedInitialState) {
  for (const [workspaceId, workflows] of Object.entries(
    initialState.workflowLists?.itemsByWorkspaceId ?? {},
  )) {
    client.setQueryData(qk.workflows.all(workspaceId, { includeHidden: true }), workflows);
  }

  const workflows = initialState.workflows?.items ?? [];
  const byWorkspace = new Map<string, Array<Workflow | WorkflowItem>>();
  for (const workflow of workflows) {
    const workspaceId = "workspaceId" in workflow ? workflow.workspaceId : workflow.workspace_id;
    const items = byWorkspace.get(workspaceId) ?? [];
    items.push(workflow);
    byWorkspace.set(workspaceId, items);
  }
  for (const [workspaceId, items] of byWorkspace) {
    client.setQueryData(qk.workflows.all(workspaceId, { includeHidden: true }), items);
  }
}

function seedWorkflowSnapshots(client: QueryClient, initialState: QuerySeedInitialState) {
  for (const [workflowId, snapshot] of Object.entries(
    initialState.workflowSnapshots?.itemsByWorkflowId ?? {},
  )) {
    client.setQueryData(qk.workflows.snapshot(workflowId), snapshot);
  }
}

function seedQueryClientFromRouteData(client: QueryClient, routeData: BootRouteData | undefined) {
  if (!routeData) return;
  client.setQueryData(qk.boot.routeData(), routeData);
  seedRouteContext(client, routeData.routeContext);
  seedTasksPage(client, routeData.tasksPage);
  seedTaskDetail(client, routeData.taskDetail);
}

function seedRouteContext(
  client: QueryClient,
  routeContext: BootRouteData["routeContext"] | undefined,
) {
  if (!routeContext) return;
  const workspaceId = routeContext.activeWorkspaceId ?? null;
  if (!workspaceId) return;
  setIfDefined(client, qk.workflows.all(workspaceId), routeContext.workflows);
  setIfDefined(client, qk.workspaces.repositories(workspaceId), routeContext.repositories);
}

function seedTasksPage(client: QueryClient, tasksPage: BootRouteData["tasksPage"] | undefined) {
  if (!tasksPage) return;
  const workspaceId = tasksPage.activeWorkspaceId ?? null;
  client.setQueryData(qk.tasks.page(workspaceId), tasksPage);
  if (!workspaceId) return;
  setIfDefined(client, qk.workflows.all(workspaceId), tasksPage.workflows);
  setIfDefined(client, qk.workspaces.repositories(workspaceId), tasksPage.repositories);
}

function seedTaskDetail(client: QueryClient, taskDetail: BootRouteData["taskDetail"] | undefined) {
  if (!taskDetail) return;
  client.setQueryData(qk.tasks.detail(taskDetail.task.id), taskDetail.task);
  seedQueryClientFromInitialState(client, taskDetail.initialState, {
    sessionId: taskDetail.sessionId ?? undefined,
  });
}

function seedOfficeState(client: QueryClient, initialState: QuerySeedInitialState) {
  const office = initialState.office;
  if (!office) return;
  setIfDefined(client, qk.office.meta(), office.meta);

  const workspaceId = initialState.workspaces?.activeId ?? null;
  if (!workspaceId) return;

  seedOfficeEntityLists(client, workspaceId, office);
  seedOfficeInbox(client, workspaceId, office);
  seedOfficeDashboardAndActivity(client, workspaceId, office);
}

function seedOfficeEntityLists(
  client: QueryClient,
  workspaceId: string,
  office: OfficeQuerySeedState,
) {
  if (office.agents) {
    client.setQueryData(qk.office.agents(workspaceId), { agents: office.agents });
  }
  if (office.projects) {
    client.setQueryData(qk.office.projects(workspaceId), { projects: office.projects });
  }
  if (office.skills && office.skills.length > 0) {
    client.setQueryData(qk.office.skills(workspaceId), { skills: office.skills });
  }
  if (office.routines && office.routines.length > 0) {
    client.setQueryData(qk.office.routines(workspaceId), { routines: office.routines });
  }
}

function seedOfficeInbox(client: QueryClient, workspaceId: string, office: OfficeQuerySeedState) {
  if (!office.inboxItems && office.inboxCount === undefined) return;
  client.setQueryData(qk.office.inbox(workspaceId), {
    items: office.inboxItems ?? [],
    total_count: office.inboxCount ?? office.inboxItems?.length ?? 0,
  });
}

function seedOfficeDashboardAndActivity(
  client: QueryClient,
  workspaceId: string,
  office: OfficeQuerySeedState,
) {
  setIfDefined(client, qk.office.dashboard(workspaceId), office.dashboard ?? undefined);
  if (office.activity) {
    client.setQueryData(qk.office.activity(workspaceId), { activity: office.activity });
  }
  if (office.runs) {
    client.setQueryData(qk.office.runs(workspaceId), { runs: office.runs });
  }
}

function seedSettingsState(client: QueryClient, initialState: QuerySeedInitialState) {
  if (initialState.executors) {
    client.setQueryData(qk.settings.executors(), {
      executors: initialState.executors.items,
    });
  }
  if (initialState.settingsAgents) {
    client.setQueryData(qk.settings.agents(), {
      agents: initialState.settingsAgents.items,
      total: initialState.settingsAgents.items.length,
    });
  }
  if (initialState.agentDiscovery) {
    client.setQueryData(qk.settings.agentDiscovery(), {
      agents: initialState.agentDiscovery.items,
      total: initialState.agentDiscovery.items.length,
    });
  }
  if (initialState.availableAgents) {
    client.setQueryData(qk.settings.availableAgents(), {
      agents: initialState.availableAgents.items,
      tools: initialState.availableAgents.tools,
      total: initialState.availableAgents.items.length,
    });
  }
  if (initialState.editors) {
    client.setQueryData(qk.settings.editors(), { editors: initialState.editors.items });
  }
  if (initialState.prompts) {
    client.setQueryData(qk.settings.prompts(), { prompts: initialState.prompts.items });
  }
  if (initialState.secrets) {
    client.setQueryData(qk.settings.secrets(), initialState.secrets.items);
  }
  if (initialState.sprites) {
    setIfDefined(client, qk.settings.spritesStatus(), initialState.sprites.status ?? undefined);
    client.setQueryData(qk.settings.spritesInstances(), initialState.sprites.instances);
  }
  if (initialState.notificationProviders) {
    client.setQueryData(qk.settings.notificationProviders(), {
      providers: initialState.notificationProviders.items,
      events: initialState.notificationProviders.events,
      apprise_available: initialState.notificationProviders.appriseAvailable,
    });
  }
}

function seedRepositoryScripts(client: QueryClient, initialState: QuerySeedInitialState) {
  for (const [repositoryId, scripts] of Object.entries(
    initialState.repositoryScripts?.itemsByRepositoryId ?? {},
  )) {
    client.setQueryData(qk.workspaces.repositoryScripts(repositoryId), scripts);
  }
}

function seedTaskSessions(client: QueryClient, initialState: QuerySeedInitialState) {
  const sessionsByTask = initialState.taskSessionsByTask?.itemsByTaskId ?? {};
  for (const [taskId, sessions] of Object.entries(sessionsByTask)) {
    client.setQueryData(qk.taskSession.byTask(taskId), { sessions });
  }
  const sessionsById = initialState.taskSessions?.items ?? {};
  for (const [sessionId, session] of Object.entries(sessionsById)) {
    client.setQueryData(qk.taskSession.byId(sessionId), session);
  }
}

function seedSessionMessages(
  client: QueryClient,
  initialState: QuerySeedInitialState,
  preferredSessionId: string | undefined,
) {
  const messagesBySession = initialState.messages?.bySession ?? {};
  const metaBySession = initialState.messages?.metaBySession ?? {};
  for (const sessionId of sessionIds(messagesBySession, preferredSessionId)) {
    const messages = messagesBySession[sessionId];
    if (!messages) continue;
    const meta = metaBySession[sessionId];
    client.setQueryData(qk.session.messages(sessionId), {
      messages,
      hasMore: meta?.hasMore ?? false,
      oldestCursor: meta?.oldestCursor ?? messages[0]?.id ?? null,
    });
  }
}

function seedSessionTurns(
  client: QueryClient,
  initialState: QuerySeedInitialState,
  preferredSessionId: string | undefined,
) {
  const turnsBySession = initialState.turns?.bySession ?? {};
  const activeBySession = initialState.turns?.activeBySession ?? {};
  for (const sessionId of sessionIds(turnsBySession, preferredSessionId)) {
    const turns = turnsBySession[sessionId];
    if (!turns) continue;
    client.setQueryData(qk.session.turns(sessionId), {
      turns,
      activeTurnId: activeBySession[sessionId] ?? null,
    });
  }
}

function seedSessionRuntime(client: QueryClient, initialState: QuerySeedInitialState) {
  seedGitStatus(client, initialState);
  seedSessionCommits(client, initialState);
  seedSessionProcesses(client, initialState);
  seedEnvScopedRuntime(client, initialState);
  seedPerSessionRuntime(client, initialState);
}

function seedGitStatus(client: QueryClient, initialState: QuerySeedInitialState) {
  const latestByEnv = initialState.gitStatus?.byEnvironmentId ?? {};
  const byRepoByEnv = initialState.gitStatus?.byEnvironmentRepo ?? {};
  for (const envKey of new Set([...Object.keys(latestByEnv), ...Object.keys(byRepoByEnv)])) {
    const latest = latestByEnv[envKey];
    const byRepo =
      byRepoByEnv[envKey] ?? (latest ? { [latest.repository_name ?? ""]: latest } : {});
    const data: GitStatusQueryData = { latest, byRepo };
    client.setQueryData(qk.sessionRuntime.gitStatus(envKey), data);
  }
}

function seedSessionCommits(client: QueryClient, initialState: QuerySeedInitialState) {
  const commitsByEnv = initialState.sessionCommits?.byEnvironmentId ?? {};
  for (const [envKey, commits] of Object.entries(commitsByEnv)) {
    client.setQueryData(qk.sessionRuntime.commits(envKey), commits);
  }
}

function seedSessionProcesses(client: QueryClient, initialState: QuerySeedInitialState) {
  const processIdsBySession = initialState.processes?.processIdsBySessionId ?? {};
  const processesById = initialState.processes?.processesById ?? {};
  const activeBySession = initialState.processes?.activeProcessBySessionId ?? {};
  const devBySession = initialState.processes?.devProcessBySessionId ?? {};
  for (const [sessionId, processIds] of Object.entries(processIdsBySession)) {
    const data: SessionProcessesQueryData = {
      processesById: Object.fromEntries(
        processIds
          .map((processId) => [processId, processesById[processId]] as const)
          .filter(([, process]) => Boolean(process)),
      ),
      processIds,
      activeProcessId: activeBySession[sessionId],
      devProcessId: devBySession[sessionId],
    };
    client.setQueryData(qk.sessionRuntime.processes(sessionId), data);
  }
}

function seedEnvScopedRuntime(client: QueryClient, initialState: QuerySeedInitialState) {
  const userShells = initialState.userShells?.byEnvironmentId ?? {};
  for (const [envKey, shells] of Object.entries(userShells)) {
    client.setQueryData(qk.sessionRuntime.userShells(envKey), shells);
  }
}

function seedPerSessionRuntime(client: QueryClient, initialState: QuerySeedInitialState) {
  setBySession(client, qk.sessionRuntime.prepare, initialState.prepareProgress?.bySessionId);
  setBySession(client, qk.sessionRuntime.contextWindow, initialState.contextWindow?.bySessionId);
  setBySession(
    client,
    qk.sessionRuntime.availableCommands,
    initialState.availableCommands?.bySessionId,
  );
  setBySession(client, qk.sessionRuntime.models, initialState.sessionModels?.bySessionId);
  setBySession(client, qk.sessionRuntime.agentctl, initialState.sessionAgentctl?.itemsBySessionId);
}

function sessionIds<T>(bySession: Record<string, T>, preferredSessionId: string | undefined) {
  const ids = new Set(Object.keys(bySession));
  if (preferredSessionId) ids.add(preferredSessionId);
  return ids;
}

function setBySession<T>(
  client: QueryClient,
  keyForSession: (sessionId: string) => readonly unknown[],
  bySession: Record<string, T> | undefined,
) {
  for (const [sessionId, value] of Object.entries(bySession ?? {})) {
    client.setQueryData(keyForSession(sessionId), value);
  }
}

function setIfDefined<T>(client: QueryClient, key: readonly unknown[], value: T | undefined) {
  if (value !== undefined) {
    client.setQueryData(key, value);
  }
}
