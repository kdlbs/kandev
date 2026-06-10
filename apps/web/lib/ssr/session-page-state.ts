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
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import { listSessionTurns } from "@/lib/api/domains/session-api";
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
import { prepareResultToSessionState } from "@/lib/state/slices/session-runtime/prepare-result";
import type { SessionPrepareState } from "@/lib/state/slices/session-runtime/types";
import type { AppState } from "@/lib/state/store";

function buildWorktreeState(allSessions: TaskSession[]) {
  const sessionsWithWorktrees = allSessions.filter((s) => s.worktree_id);
  return {
    worktrees: {
      items: Object.fromEntries(
        sessionsWithWorktrees.map((s) => [
          s.worktree_id,
          {
            id: s.worktree_id!,
            sessionId: s.id,
            repositoryId: s.repository_id ?? undefined,
            path: s.worktree_path ?? undefined,
            branch: s.worktree_branch ?? undefined,
          },
        ]),
      ),
    },
    sessionWorktreesBySessionId: {
      itemsBySessionId: Object.fromEntries(
        sessionsWithWorktrees.map((s) => [s.id, [s.worktree_id!]]),
      ),
    },
  };
}

type BuildSessionPageStateParams = {
  task: Task;
  sessionId: string | null;
  snapshot: Awaited<ReturnType<typeof fetchWorkflowSnapshot>>;
  agents: Awaited<ReturnType<typeof listAgents>>;
  repositories: Awaited<ReturnType<typeof listRepositories>>["repositories"];
  allSessions: TaskSession[];
  // Full session payload (with agent_profile_snapshot) for the active sessionId,
  // when available. The list endpoint returns lightweight summaries without the
  // snapshot, which would force the model selector to fall back to the agent's
  // default model on SSR — visible as a brief flash of the wrong model before
  // the WS-driven cached state arrives.
  activeSession: TaskSession | null;
  workspaces: Awaited<ReturnType<typeof listWorkspaces>>["workspaces"];
  workflows: Awaited<ReturnType<typeof listWorkflows>>["workflows"];
  turns: Awaited<ReturnType<typeof listSessionTurns>>["turns"];
  userSettingsResponse: UserSettingsResponse | null;
  messagesResponse: ListMessagesResponse | null;
};

function buildSessionPageState(p: BuildSessionPageStateParams) {
  const { task, sessionId, snapshot, agents, allSessions, messagesResponse } = p;
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
    ...buildWorktreeState(allSessions),
    ...buildPrepareProgressState(allSessions),
    settingsAgents: { items: agents.agents },
    settingsData: { agentsLoaded: true, executorsLoaded: false },
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
    // Don't write activeId — null means "All Workflows"; task context lives in kanban.workflowId.
    workflows: {
      items: workflows.map((w) => ({
        id: w.id as string,
        workspaceId: w.workspace_id as string,
        name: w.name,
        hidden: w.hidden,
        style: w.style,
      })),
    } as Partial<AppState>["workflows"],
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
  const { task, sessionId, allSessions, activeSession, turns } = p;
  // Prefer the full active session payload (with agent_profile_snapshot) over
  // its summary entry in allSessions so the model selector can resolve the
  // persisted model on first render without flashing the agent default.
  const itemsBySessionId = Object.fromEntries(allSessions.map((s) => [s.id, s]));
  if (activeSession?.id) {
    itemsBySessionId[activeSession.id] = activeSession;
  }
  return {
    taskSessions: { items: itemsBySessionId },
    taskSessionsByTask: {
      itemsByTaskId: { [task.id]: allSessions },
      loadingByTaskId: { [task.id]: false },
      loadedByTaskId: { [task.id]: true },
    },
    turns: sessionId
      ? {
          bySession: { [sessionId]: turns },
          activeBySession: {
            [sessionId]: turns.filter((t) => !t.completed_at).pop()?.id ?? null,
          },
        }
      : { bySession: {}, activeBySession: {} },
    environmentIdBySessionId: Object.fromEntries(
      allSessions.filter((s) => s.task_environment_id).map((s) => [s.id, s.task_environment_id!]),
    ),
  };
}

function buildPrepareProgressState(allSessions: TaskSession[]) {
  const bySessionId: Record<string, SessionPrepareState> = {};

  for (const session of allSessions) {
    const prepareState = prepareResultToSessionState(session.id, session.metadata);
    if (prepareState) bySessionId[session.id] = prepareState;
  }

  if (Object.keys(bySessionId).length === 0) return {};
  return { prepareProgress: { bySessionId } };
}

export type FetchedSessionData = {
  task: Task;
  sessionId: string | null;
  initialState: ReturnType<typeof taskToState>;
  initialTerminals: Terminal[];
};

export async function fetchSessionData(sessionId: string): Promise<FetchedSessionData> {
  const { fetchTaskSession } = await import("@/lib/api");
  const sessionResponse = await fetchTaskSession(sessionId, { cache: "no-store" });
  const activeSession = sessionResponse.session ?? null;
  if (!activeSession?.task_id) throw new Error("No task_id found for session");
  const [task, allSessionsResponse] = await Promise.all([
    fetchTask(activeSession.task_id, { cache: "no-store" }),
    listTaskSessions(activeSession.task_id, { cache: "no-store" }),
  ]);

  return fetchSessionDataFromTask(task, sessionId, allSessionsResponse, activeSession);
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

  // Refetch the active session via the single-session endpoint to get
  // agent_profile_snapshot, which the list endpoint strips. See
  // BuildSessionPageStateParams.activeSession for the SSR-flicker rationale.
  // A failure here (auth, 5xx, timeout) degrades gracefully to the original
  // initial-flash behaviour but should surface in server logs so silent
  // 401/500s are debuggable.
  const { fetchTaskSession } = await import("@/lib/api");
  const sessionResponse = await fetchTaskSession(sessionId, { cache: "no-store" }).catch((e) => {
    console.warn(
      "[session-page-state] failed to fetch active session snapshot; SSR will fall back to summary entry",
      e,
    );
    return null;
  });
  return fetchSessionDataFromTask(
    task,
    sessionId,
    allSessionsResponse,
    sessionResponse?.session ?? null,
  );
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
    activeSession: null,
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
  activeSession: TaskSession | null,
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
    activeSession,
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
