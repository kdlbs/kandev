import { StateHydrator } from "@/components/state-hydrator";
import { readLayoutDefaults } from "@/lib/layout/read-layout-defaults";
import {
  fetchWorkflowSnapshot,
  fetchTaskSession,
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
} from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { snapshotToState, taskToState } from "@/lib/ssr/mapper";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { TaskPageContent } from "@/components/task/task-page-content";

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
  sessionId: string;
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
  const {
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
  } = p;

  const repositoryId = task.repositories?.[0]?.repository_id;
  const repository = repositories.find((r) => r.id === repositoryId);
  const scripts = repository?.scripts ?? [];
  const messages = messagesResponse?.messages ? [...messagesResponse.messages].reverse() : [];

  const taskState = taskToState(task, sessionId, {
    items: messages,
    hasMore: messagesResponse?.has_more ?? false,
    oldestCursor: messages[0]?.id ?? null,
  });

  const repositoryScripts = repositoryId
    ? {
        itemsByRepositoryId: { [repositoryId]: scripts },
        loadingByRepositoryId: { [repositoryId]: false },
        loadedByRepositoryId: { [repositoryId]: true },
      }
    : {
        itemsByRepositoryId: {},
        loadingByRepositoryId: {},
        loadedByRepositoryId: {},
      };

  return {
    ...snapshotToState(snapshot),
    ...taskState,
    workspaces: {
      items: workspaces.map((workspace) => ({
        id: workspace.id,
        name: workspace.name,
        description: workspace.description ?? null,
        owner_id: workspace.owner_id,
        default_executor_id: workspace.default_executor_id ?? null,
        default_environment_id: workspace.default_environment_id ?? null,
        default_agent_profile_id: workspace.default_agent_profile_id ?? null,
        created_at: workspace.created_at,
        updated_at: workspace.updated_at,
      })),
      activeId: task.workspace_id,
    },
    workflows: {
      items: workflows.map((workflow) => ({
        id: workflow.id,
        workspaceId: workflow.workspace_id,
        name: workflow.name,
      })),
      activeId: task.workflow_id,
    },
    repositories: {
      itemsByWorkspaceId: { [task.workspace_id]: repositories },
      loadingByWorkspaceId: { [task.workspace_id]: false },
      loadedByWorkspaceId: { [task.workspace_id]: true },
    },
    repositoryScripts,
    agentProfiles: {
      items: agents.agents.flatMap((agent) =>
        agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
      ),
      version: 0,
    },
    taskSessions: {
      items: Object.fromEntries(allSessions.map((s) => [s.id, s])),
    },
    taskSessionsByTask: {
      itemsByTaskId: { [task.id]: allSessions },
      loadingByTaskId: { [task.id]: false },
      loadedByTaskId: { [task.id]: true },
    },
    turns: {
      bySession: { [sessionId]: turns },
      activeBySession: {
        [sessionId]: turns.filter((t) => !t.completed_at).pop()?.id ?? null,
      },
    },
    ...buildWorktreeState(allSessions),
    settingsAgents: { items: agents.agents },
    settingsData: {
      agentsLoaded: true,
      executorsLoaded: false,
    },
    userSettings: mapUserSettingsResponse(userSettingsResponse),
  };
}

type FetchedSessionData = {
  task: Task;
  sessionId: string;
  initialState: ReturnType<typeof taskToState>;
  initialTerminals: Terminal[];
};

async function fetchSessionData(paramSessionId: string): Promise<FetchedSessionData> {
  const sessionResponse = await fetchTaskSession(paramSessionId, { cache: "no-store" });
  const session = sessionResponse.session;
  if (!session?.task_id) {
    throw new Error("No task_id found for session");
  }

  const task = await fetchTask(session.task_id, { cache: "no-store" });
  const [
    snapshot,
    agents,
    repositoriesResponse,
    allSessionsResponse,
    workspacesResponse,
    workflowsResponse,
    turnsResponse,
    userSettingsResponse,
    terminalsResponse,
    messagesResponse,
  ] = await Promise.all([
    fetchWorkflowSnapshot(task.workflow_id, { cache: "no-store" }),
    listAgents({ cache: "no-store" }),
    listRepositories(task.workspace_id, { includeScripts: true }, { cache: "no-store" }),
    listTaskSessions(session.task_id, { cache: "no-store" }),
    listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
    listWorkflows(task.workspace_id, { cache: "no-store" }).catch(() => ({ workflows: [] })),
    listSessionTurns(paramSessionId, { cache: "no-store" }).catch(() => ({ turns: [], total: 0 })),
    fetchUserSettings({ cache: "no-store" }).catch(() => null),
    fetchTerminals(paramSessionId).catch(() => []),
    listTaskSessionMessages(
      paramSessionId,
      { limit: 50, sort: "desc" },
      { cache: "no-store" },
    ).catch(() => null as ListMessagesResponse | null),
  ]);

  const repositories = repositoriesResponse.repositories ?? [];
  const allSessions = allSessionsResponse.sessions ?? [session];
  const workspaces = workspacesResponse.workspaces ?? [];
  const workflows = workflowsResponse.workflows ?? [];
  const turns = turnsResponse.turns ?? [];

  const initialTerminals: Terminal[] = terminalsResponse.map((t) => ({
    id: t.terminal_id,
    type: t.initial_command ? ("script" as const) : ("shell" as const),
    label: t.label,
    closable: t.closable,
  }));

  const initialState = buildSessionPageState({
    task,
    sessionId: paramSessionId,
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

  return { task, sessionId: paramSessionId, initialState, initialTerminals };
}

function extractInitialRepositories(
  initialState: FetchedSessionData["initialState"] | null,
  task: Task | null,
) {
  return initialState?.repositories?.itemsByWorkspaceId?.[task?.workspace_id ?? ""] ?? [];
}

function extractInitialScripts(
  initialState: FetchedSessionData["initialState"] | null,
  task: Task | null,
) {
  const repoId = task?.repositories?.[0]?.repository_id ?? "";
  return initialState?.repositoryScripts?.itemsByRepositoryId?.[repoId] ?? [];
}

export default async function SessionPage({
  params,
  searchParams,
}: {
  params: Promise<{ sessionId: string }>;
  searchParams: Promise<{ layout?: string }>;
}) {
  let fetchedData: FetchedSessionData | null = null;
  const defaultLayouts = await readLayoutDefaults();
  const { layout: initialLayout } = await searchParams;

  try {
    const { sessionId: paramSessionId } = await params;
    fetchedData = await fetchSessionData(paramSessionId);
  } catch (error) {
    console.warn(
      "Could not SSR session (client will load via WebSocket):",
      error instanceof Error ? error.message : String(error),
    );
  }

  const { task, sessionId, initialState, initialTerminals } = fetchedData ?? {
    task: null,
    sessionId: null,
    initialState: null,
    initialTerminals: [],
  };

  return (
    <>
      {initialState ? (
        <StateHydrator initialState={initialState} sessionId={sessionId ?? undefined} />
      ) : null}
      <TaskPageContent
        task={task}
        sessionId={sessionId}
        initialRepositories={extractInitialRepositories(initialState, task)}
        initialScripts={extractInitialScripts(initialState, task)}
        initialTerminals={initialTerminals}
        defaultLayouts={defaultLayouts}
        initialLayout={initialLayout}
      />
    </>
  );
}
