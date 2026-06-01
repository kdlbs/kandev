import {
  fetchWorkflowSnapshot,
  fetchTask,
  fetchUserSettings,
  listAgents,
  listWorkflows,
  listRepositories,
  listTaskSessionMessages,
  listTaskSessions,
  listWorkspaces,
} from "@/lib/api";
import { toAgentProfileOption } from "@/lib/types/settings";
import { listSessionTurns } from "@/lib/api/domains/session-api";
import { deriveActiveTurnId } from "@/lib/query/query-options/session";
import { fetchTerminals } from "@/lib/api/domains/user-shell-api";
import type {
  ListMessagesResponse,
  Task,
  TaskSession,
  UserSettingsResponse,
  WorkflowSnapshot,
} from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { snapshotToState, taskToState } from "@/lib/ssr/mapper";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { SsrInitialState } from "@/lib/ssr/initial-state";

type BuildSessionPageStateParams = {
  task: Task;
  sessionId: string | null;
  snapshot: Awaited<ReturnType<typeof fetchWorkflowSnapshot>>;
  agents: Awaited<ReturnType<typeof listAgents>>;
  repositories: Awaited<ReturnType<typeof listRepositories>>["repositories"];
  allSessions: TaskSession[];
  workspaces: Awaited<ReturnType<typeof listWorkspaces>>["workspaces"];
  workflows: Awaited<ReturnType<typeof listWorkflows>>["workflows"];
  turns: Awaited<ReturnType<typeof listSessionTurns>>["turns"];
  userSettingsResponse: UserSettingsResponse | null;
  messagesResponse: ListMessagesResponse | null;
};

function buildSessionPageState(p: BuildSessionPageStateParams) {
  const { task, sessionId, snapshot, agents, messagesResponse } = p;
  const messages = messagesResponse?.messages ? [...messagesResponse.messages].reverse() : [];
  const taskState = taskToState(task, sessionId, {
    items: messages,
    hasMore: messagesResponse?.has_more ?? false,
    oldestCursor: messages[0]?.id ?? null,
  });

  return {
    ...snapshotToState(snapshot),
    ...taskState,
    ...buildResourceState(p),
    ...buildSessionState(p),
    // Settings-domain server data: seeded into the TanStack Query cache by
    // StateHydrator (no longer hydrated into a Zustand slice).
    settingsAgents: { items: agents.agents },
    userSettings: mapUserSettingsResponse(p.userSettingsResponse),
  };
}

function buildResourceState(p: BuildSessionPageStateParams) {
  const { task, agents, repositories, workspaces, workflows } = p;
  const repositoryId = task.repositories?.[0]?.repository_id;
  const repository = repositories.find((r) => r.id === repositoryId);
  const scripts = repository?.scripts ?? [];
  return {
    workspaces: {
      items: workspaces.map((w) => ({
        id: w.id,
        name: w.name,
        description: w.description ?? null,
        owner_id: w.owner_id,
        default_executor_id: w.default_executor_id ?? null,
        default_environment_id: w.default_environment_id ?? null,
        default_agent_profile_id: w.default_agent_profile_id ?? null,
        created_at: w.created_at,
        updated_at: w.updated_at,
      })),
      activeId: task.workspace_id,
    },
    // Server data — seeded into the TQ workflows-list cache by the hydrator.
    // No activeId here — null means "All Workflows"; task context comes from the
    // seeded kanban snapshot, not a homepage filter selection.
    workflows: {
      // Full server shape (matches workflowsFromResponse) so the TQ
      // workflows-list seed in StateHydrator isn't under-populated — a missing
      // agent_profile_id silently breaks the create-task workflow agent lock.
      items: workflows.map((w) => ({
        id: w.id as string,
        workspaceId: w.workspace_id as string,
        name: w.name,
        description: w.description,
        sortOrder: w.sort_order ?? 0,
        agent_profile_id: w.agent_profile_id,
        hidden: w.hidden,
        style: w.style,
      })),
    } satisfies SsrInitialState["workflows"],
    repositories: {
      itemsByWorkspaceId: { [task.workspace_id]: repositories },
      loadingByWorkspaceId: { [task.workspace_id]: false },
      loadedByWorkspaceId: { [task.workspace_id]: true },
    },
    repositoryScripts: repositoryId
      ? {
          itemsByRepositoryId: { [repositoryId]: scripts },
          loadingByRepositoryId: { [repositoryId]: false },
          loadedByRepositoryId: { [repositoryId]: true },
        }
      : { itemsByRepositoryId: {}, loadingByRepositoryId: {}, loadedByRepositoryId: {} },
    agentProfiles: {
      items: agents.agents.flatMap((agent) =>
        agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
      ),
      version: 0,
    },
  };
}

function buildSessionState(p: BuildSessionPageStateParams) {
  const { task, sessionId, allSessions, turns } = p;
  return {
    taskSessions: { items: Object.fromEntries(allSessions.map((s) => [s.id, s])) },
    taskSessionsByTask: {
      itemsByTaskId: { [task.id]: allSessions },
    },
    turns: sessionId
      ? {
          bySession: { [sessionId]: turns },
          activeBySession: {
            [sessionId]: deriveActiveTurnId(turns),
          },
        }
      : { bySession: {}, activeBySession: {} },
    environmentIdBySessionId: Object.fromEntries(
      allSessions.filter((s) => s.task_environment_id).map((s) => [s.id, s.task_environment_id!]),
    ),
  };
}

export type FetchedSessionData = {
  task: Task;
  sessionId: string | null;
  initialState: ReturnType<typeof buildSessionPageState>;
  initialTerminals: Terminal[];
};

export async function fetchSessionData(sessionId: string): Promise<FetchedSessionData> {
  const [allSessionsResponse, task] = await (async () => {
    // We need task + sessions; caller provides sessionId
    const { fetchTaskSession } = await import("@/lib/api");
    const sessionResponse = await fetchTaskSession(sessionId, { cache: "no-store" });
    const session = sessionResponse.session;
    if (!session?.task_id) throw new Error("No task_id found for session");
    const t = await fetchTask(session.task_id, { cache: "no-store" });
    const sessResp = await listTaskSessions(session.task_id, { cache: "no-store" });
    return [sessResp, t] as const;
  })();

  return fetchSessionDataFromTask(task, sessionId, allSessionsResponse);
}

export async function fetchSessionDataForTask(taskId: string): Promise<FetchedSessionData> {
  const task = await fetchTask(taskId, { cache: "no-store" });
  const allSessionsResponse = await listTaskSessions(taskId, { cache: "no-store" });
  const sessions = allSessionsResponse.sessions ?? [];

  const sessionId = task.primary_session_id ?? sessions[0]?.id;
  if (!sessionId) {
    // No sessions yet — fetch task/workspace data so the store is seeded and
    // the auto-start hook can fire immediately without a client-side crash.
    return fetchTaskDataOnly(task, allSessionsResponse);
  }

  return fetchSessionDataFromTask(task, sessionId, allSessionsResponse);
}

async function fetchTaskDataOnly(
  task: Task,
  allSessionsResponse: Awaited<ReturnType<typeof listTaskSessions>>,
): Promise<FetchedSessionData> {
  const [
    snapshot,
    agents,
    repositoriesResponse,
    workspacesResponse,
    workflowsResponse,
    userSettingsResponse,
  ] = await Promise.all([
    task.workflow_id
      ? fetchWorkflowSnapshot(task.workflow_id, { cache: "no-store" })
      : Promise.resolve({ steps: [], tasks: [] } as unknown as WorkflowSnapshot),
    listAgents({ cache: "no-store" }),
    listRepositories(task.workspace_id, { includeScripts: true }, { cache: "no-store" }),
    listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
    listWorkflows(task.workspace_id, { cache: "no-store", includeHidden: true }).catch(() => ({
      workflows: [],
    })),
    fetchUserSettings({ cache: "no-store" }).catch(() => null),
  ]);

  const allSessions = allSessionsResponse.sessions ?? [];
  const repositories = repositoriesResponse.repositories ?? [];
  const workspaces = workspacesResponse.workspaces ?? [];
  const workflows = workflowsResponse.workflows ?? [];

  const initialState = buildSessionPageState({
    task,
    sessionId: null,
    snapshot,
    agents,
    repositories,
    allSessions,
    workspaces,
    workflows,
    turns: [],
    userSettingsResponse,
    messagesResponse: null,
  });

  return { task, sessionId: null, initialState, initialTerminals: [] };
}

type TerminalApiResponse = Awaited<ReturnType<typeof fetchTerminals>>[number];

function shouldHydrateTerminal(t: TerminalApiResponse): boolean {
  const id = t.id ?? t.terminal_id ?? "";
  if (!id || id === "bottom-panel") return false;
  if (t.state === "parked") return false;
  return true;
}

function classifyTerminal(
  t: TerminalApiResponse,
  id: string,
): { isScript: boolean; isOrdinary: boolean } {
  const isScript = t.kind === "script" || id.startsWith("script-");
  const isOrdinary = t.kind === "ordinary" || (!isScript && t.seq !== undefined);
  return { isScript, isOrdinary };
}

function deriveHydratedLabel(
  t: TerminalApiResponse,
  isScript: boolean,
  isOrdinary: boolean,
): string {
  if (t.display_name) return t.display_name;
  if (t.custom_name && t.custom_name !== "") return t.custom_name;
  if (t.label) return t.label;
  if (isOrdinary && t.seq) return `Terminal ${t.seq}`;
  return isScript ? "Script" : "Terminal";
}

function pickTerminalKind(
  isOrdinary: boolean,
  isScript: boolean,
): "ordinary" | "script" | undefined {
  if (isOrdinary) return "ordinary";
  if (isScript) return "script";
  return undefined;
}

function hydrateTerminal(t: TerminalApiResponse): Terminal {
  const id = (t.id ?? t.terminal_id ?? "") as string;
  const { isScript, isOrdinary } = classifyTerminal(t, id);
  const kind = pickTerminalKind(isOrdinary, isScript);
  return {
    id,
    type: isScript ? ("script" as const) : ("shell" as const),
    label: deriveHydratedLabel(t, isScript, isOrdinary),
    closable: t.closable ?? true,
    kind,
    seq: t.seq,
    customName: t.custom_name ?? undefined,
    state: t.state,
    ptyStatus: t.pty_status,
  };
}

async function fetchSessionDataFromTask(
  task: Task,
  sessionId: string,
  allSessionsResponse: Awaited<ReturnType<typeof listTaskSessions>>,
): Promise<FetchedSessionData> {
  // User shells are env-scoped — look up this session's task_environment_id
  // from the already-fetched session list. Sessions w/o env (legacy) skip
  // the terminal SSR fetch; the boot-time heal pass + WS-driven user_shell.list
  // will populate it once the env mapping lands.
  const sessionEnvId =
    allSessionsResponse.sessions?.find((s) => s.id === sessionId)?.task_environment_id ?? "";

  const [
    snapshot,
    agents,
    repositoriesResponse,
    workspacesResponse,
    workflowsResponse,
    turnsResponse,
    userSettingsResponse,
    terminalsResponse,
    messagesResponse,
  ] = await Promise.all([
    task.workflow_id
      ? fetchWorkflowSnapshot(task.workflow_id, { cache: "no-store" })
      : Promise.resolve({ steps: [], tasks: [] } as unknown as WorkflowSnapshot),
    listAgents({ cache: "no-store" }),
    listRepositories(task.workspace_id, { includeScripts: true }, { cache: "no-store" }),
    listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
    listWorkflows(task.workspace_id, { cache: "no-store", includeHidden: true }).catch(() => ({
      workflows: [],
    })),
    listSessionTurns(sessionId, { cache: "no-store" }).catch(() => ({ turns: [], total: 0 })),
    fetchUserSettings({ cache: "no-store" }).catch(() => null),
    sessionEnvId ? fetchTerminals(task.id, sessionEnvId).catch(() => []) : Promise.resolve([]),
    listTaskSessionMessages(sessionId, { limit: 50, sort: "desc" }, { cache: "no-store" }).catch(
      () => null as ListMessagesResponse | null,
    ),
  ]);

  const allSessions = allSessionsResponse.sessions ?? [];
  const repositories = repositoriesResponse.repositories ?? [];
  const workspaces = workspacesResponse.workspaces ?? [];
  const workflows = workflowsResponse.workflows ?? [];
  const turns = turnsResponse.turns ?? [];

  const initialTerminals: Terminal[] = terminalsResponse
    .filter(shouldHydrateTerminal)
    .map(hydrateTerminal);

  const initialState = buildSessionPageState({
    task,
    sessionId,
    snapshot,
    agents,
    repositories,
    allSessions,
    workspaces,
    workflows,
    turns,
    userSettingsResponse,
    messagesResponse,
  });

  return { task, sessionId, initialState, initialTerminals };
}

export function extractInitialRepositories(
  initialState: FetchedSessionData["initialState"] | null,
  task: Task | null,
) {
  return initialState?.repositories?.itemsByWorkspaceId?.[task?.workspace_id ?? ""] ?? [];
}

export function extractInitialScripts(
  initialState: FetchedSessionData["initialState"] | null,
  task: Task | null,
) {
  const repoId = task?.repositories?.[0]?.repository_id ?? "";
  return initialState?.repositoryScripts?.itemsByRepositoryId?.[repoId] ?? [];
}
