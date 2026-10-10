import { buildSessionModelsState } from "@/lib/state/slices/session-runtime/model-hydration";
import { resolveTaskRoute } from "@/lib/routing/resolve-task-route";
import {
  listWorkflowSteps,
  fetchTask,
  listAgents,
  listWorkflows,
  listRepositories,
  listTaskSessionMessages,
  listTaskSessions,
  listWorkspaces,
} from "@/lib/api";
import { toSelectorProfileOptions } from "@/lib/settings/agent-profile-selector-order";
import { listSessionTurns } from "@/lib/api/domains/session-api";
import { fetchTerminals } from "@/lib/api/domains/user-shell-api";
import type {
  ListMessagesResponse,
  Task,
  TaskSession,
  SidebarTaskPageResponse,
  UserSettingsResponse,
  MessageTurnCoverage,
} from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { taskToState, workflowStepToState } from "@/lib/ssr/mapper";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { prepareResultToSessionState } from "@/lib/state/slices/session-runtime/prepare-result";
import { latestIncompleteTurnId } from "@/lib/state/slices/session/turn-actions";
import type { SessionPrepareState } from "@/lib/state/slices/session-runtime/types";
import type { AppState } from "@/lib/state/store";
import { beginOptionalHydration, type OptionalHydrationResult } from "@/lib/ssr/optional-hydration";
import { mapWorkspaceItem } from "@/lib/routing/route-bootstrap";
import type { TaskNavigationIdentity } from "@/lib/state/task-navigation-reads";
import type { StoreApi } from "zustand";
import {
  getAgentListResourceScope,
  type AgentListResponse,
} from "@/hooks/domains/settings/agent-list-resource";
import { getTaskSessionReads } from "@/lib/state/task-session-reads";
import { toKanbanTask } from "@/lib/kanban/map-task";
import {
  readJourneyRepositories,
  readJourneyWorkspaces,
  readJourneyWorkflows,
  readJourneyUserSettings,
} from "@/hooks/journey-metadata-resources";

import { hydrateSessionTerminals } from "@/lib/ssr/session-terminal-hydration";

export { OPTIONAL_HYDRATION_TIMEOUT_MS } from "@/lib/ssr/optional-hydration";

/** Unwraps a fulfilled optional result; returns undefined when it was unavailable. */
function optionalValue<T>(result: OptionalHydrationResult<T>): T | undefined {
  return result.status === "fulfilled" ? result.value : undefined;
}

/**
 * Builds a Partial<AppState> from an optional value, returning {} when the value
 * is missing so unavailable slices contribute nothing to the initial state.
 */
function optionalState<T>(
  value: T | undefined,
  build: (resolved: T) => Partial<AppState>,
): Partial<AppState> {
  return value === undefined ? {} : build(value);
}

/**
 * Builds the worktrees and sessionWorktreesBySessionId state slices from the
 * sessions that carry a worktree_id.
 */
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
  workflowSteps?: Awaited<ReturnType<typeof listWorkflowSteps>>;
  agents?: Awaited<ReturnType<typeof listAgents>>;
  repositories?: Awaited<ReturnType<typeof listRepositories>>["repositories"];
  allSessions: TaskSession[];
  // Full session payload (with agent_profile_snapshot) for the active sessionId,
  // when available. The list endpoint returns lightweight summaries without the
  // snapshot, which would force the model selector to fall back to the agent's
  // default model on SSR — visible as a brief flash of the wrong model before
  // the WS-driven cached state arrives.
  activeSession: TaskSession | null;
  workspaces?: Awaited<ReturnType<typeof listWorkspaces>>["workspaces"];
  workflows?: Awaited<ReturnType<typeof listWorkflows>>["workflows"];
  turns?: Awaited<ReturnType<typeof listSessionTurns>>["turns"];
  turnCoverage?: MessageTurnCoverage;
  userSettingsResponse?: UserSettingsResponse | null;
  messagesResponse?: ListMessagesResponse | null;
  taskSessionsLoaded?: boolean;
};

/**
 * Composes the full SSR initial state for a session page: task/message state
 * plus the resource, session, worktree, prepare-progress, agent, and user
 * settings slices, each contributed only when the corresponding data loaded.
 */
function buildSessionPageState(p: BuildSessionPageStateParams) {
  const { task, sessionId, agents, allSessions, messagesResponse } = p;
  const messages = messagesResponse?.messages ? [...messagesResponse.messages].reverse() : [];
  const taskState =
    messagesResponse === undefined
      ? taskToState(task, sessionId)
      : taskToState(task, sessionId, {
          items: messages,
          hasMore: messagesResponse?.has_more ?? false,
          oldestCursor: messages[0]?.id ?? null,
        });

  return {
    ...optionalState(p.workflowSteps, (value) => ({
      kanban: {
        workflowId: task.workflow_id ?? "",
        steps: value.steps.map(workflowStepToState),
        tasks: [toKanbanTask(task)],
        isLoading: false,
        taskCoverage: {
          workspace_id: task.workspace_id,
          workflow_id: task.workflow_id ?? "",
          membership: "active" as const,
          total: 1,
          complete: false,
          ordering_profile: "server_only",
        },
      },
    })),
    ...taskState,
    ...buildResourceState(p),
    ...buildSessionState(p),
    ...buildWorktreeState(allSessions),
    ...buildPrepareProgressState(allSessions),
    ...optionalState(agents, (value) => ({
      settingsAgents: { items: value.agents },
      settingsData: { agentsLoaded: true, executorsLoaded: false },
    })),
    ...optionalState(p.userSettingsResponse, (value) => ({
      userSettings: mapUserSettingsResponse(value),
    })),
  };
}

/** Builds the session-page hydration slice for repositories, agents, and workflow resources. */
function buildResourceState(p: BuildSessionPageStateParams) {
  const { task, agents, repositories, workspaces, workflows } = p;
  const repositoryId = task.repositories?.[0]?.repository_id;
  const repository = repositories?.find((r) => r.id === repositoryId);
  const scripts = repository?.scripts ?? [];
  return {
    workspaces: {
      ...(workspaces ? { items: workspaces.map(mapWorkspaceItem) } : {}),
      activeId: task.workspace_id,
    } as Partial<AppState>["workspaces"],
    // Don't write activeId — null means "All Workflows"; task context lives in kanban.workflowId.
    ...optionalState(workflows, (value) => ({
      workflows: {
        items: value.map((w) => ({
          id: w.id as string,
          workspaceId: w.workspace_id as string,
          name: w.name,
          hidden: w.hidden,
          style: w.style,
        })),
      } as Partial<AppState>["workflows"],
    })),
    ...optionalState(repositories, (value) => ({
      repositories: {
        itemsByWorkspaceId: { [task.workspace_id]: value },
        loadingByWorkspaceId: { [task.workspace_id]: false },
        loadedByWorkspaceId: { [task.workspace_id]: true },
      },
    })),
    ...(repositories
      ? {
          repositoryScripts: repositoryId
            ? {
                itemsByRepositoryId: { [repositoryId]: scripts },
                loadingByRepositoryId: { [repositoryId]: false },
                loadedByRepositoryId: { [repositoryId]: true },
              }
            : { itemsByRepositoryId: {}, loadingByRepositoryId: {}, loadedByRepositoryId: {} },
        }
      : {}),
    ...optionalState(agents, (value) => ({
      agentProfiles: {
        items: toSelectorProfileOptions(value.agents),
        version: 0,
        orderByAgent: {},
      },
    })),
  };
}

/** Builds the session-page hydration slice (task sessions, turns, models) for the page's task. */
function buildSessionState(p: BuildSessionPageStateParams) {
  const {
    task,
    sessionId,
    allSessions,
    activeSession,
    turns,
    turnCoverage,
    taskSessionsLoaded = true,
  } = p;
  // Prefer the full active session payload (with agent_profile_snapshot) over
  // its summary entry in allSessions so the model selector can resolve the
  // persisted model on first render without flashing the agent default.
  const itemsBySessionId = Object.fromEntries(allSessions.map((s) => [s.id, s]));
  if (activeSession?.id) {
    itemsBySessionId[activeSession.id] = activeSession;
  }
  return {
    taskSessions: { items: itemsBySessionId },
    ...buildSessionModelsState(activeSession),
    ...(taskSessionsLoaded
      ? {
          taskSessionsByTask: {
            itemsByTaskId: { [task.id]: allSessions },
            loadingByTaskId: { [task.id]: false },
            loadedByTaskId: { [task.id]: true },
          },
        }
      : {}),
    ...(turns !== undefined || turnCoverage !== undefined
      ? {
          turns: sessionId
            ? {
                bySession: { [sessionId]: turns ?? [] },
                activeBySession: {
                  [sessionId]:
                    turnCoverage !== undefined
                      ? turnCoverage.active_turn_id
                      : (latestIncompleteTurnId(turns ?? []) ?? null),
                },
                loadedBySession: turnCoverage ? {} : { [sessionId]: true },
                windowCoverageBySession: turnCoverage
                  ? {
                      [sessionId]: {
                        messageIds: turnCoverage.message_ids,
                        activeTurnObserved: true,
                      },
                    }
                  : {},
                reconcileEpochBySession: {},
                settledBoundaryBySession: {},
              }
            : {
                bySession: {},
                activeBySession: {},
                loadedBySession: {},
                windowCoverageBySession: {},
                reconcileEpochBySession: {},
                settledBoundaryBySession: {},
              },
        }
      : {}),
    environmentIdBySessionId: Object.fromEntries(
      allSessions.filter((s) => s.task_environment_id).map((s) => [s.id, s.task_environment_id!]),
    ),
  };
}

/** Builds the prepareProgress slice from each session's prepare metadata; {} when none exists. */
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
  sidebarTaskPage?: SidebarTaskPageResponse;
};

/**
 * SSR entry point for a session route: fetches the full session, its task, and
 * the task's session list, then runs the shared optional-enrichment pipeline to
 * produce the initial page state.
 */
export async function fetchSessionData(sessionId: string): Promise<FetchedSessionData> {
  const { fetchTaskSession } = await import("@/lib/api");
  const sessionResponse = await fetchTaskSession(sessionId, { cache: "no-store" });
  const activeSession = sessionResponse.session ?? null;
  if (!activeSession?.task_id) throw new Error("No task_id found for session");
  const [task, allSessionsResponse] = await Promise.all([
    fetchTask(activeSession.task_id, { cache: "no-store" }),
    listTaskSessions(activeSession.task_id, { cache: "no-store" }),
  ]);

  const optionalHydration = beginOptionalHydration();
  return fetchSessionDataFromTask(task, sessionId, allSessionsResponse, {
    activeSessionResponse: Promise.resolve({
      status: "fulfilled",
      value: { session: activeSession },
    }),
    optionalHydration,
  });
}

/**
 * SSR entry point for a task route: resolves the primary (or first) session,
 * seeding task-only data when no session exists yet and otherwise enriching via
 * the shared optional-hydration pipeline.
 */
export async function fetchSessionDataForTask(
  taskId: string,
  requestedSessionId?: string,
): Promise<FetchedSessionData> {
  const { task, allSessionsResponse, sessionId } = await resolveTaskRoute(
    taskId,
    requestedSessionId,
  );
  if (!sessionId) {
    return fetchTaskDataOnly(task, allSessionsResponse);
  }

  const optionalHydration = beginOptionalHydration();
  const { fetchTaskSession } = await import("@/lib/api");
  const activeSessionResponse = optionalHydration.load("active session snapshot", () =>
    fetchTaskSession(sessionId, { cache: "no-store" }),
  );
  return fetchSessionDataFromTask(task, sessionId, allSessionsResponse, {
    activeSessionResponse,
    optionalHydration,
  });
}

/** Client navigation leaves optional enrichment to the mounted domain hooks. */
export async function fetchTaskNavigationData(
  taskId: string,
  requestedSessionId?: string,
): Promise<FetchedSessionData> {
  const { task, allSessionsResponse, sessionId } = await resolveTaskRoute(
    taskId,
    requestedSessionId,
  );
  return {
    task,
    sessionId: sessionId ?? null,
    initialState: buildSessionPageState({
      task,
      sessionId: sessionId ?? null,
      allSessions: allSessionsResponse.sessions,
      activeSession: null,
    }),
    initialTerminals: [],
  };
}

function resolveTaskSessionId(
  task: Task,
  allSessionsResponse: Awaited<ReturnType<typeof listTaskSessions>>,
  requestedSessionId?: string,
) {
  const sessions = allSessionsResponse.sessions ?? [];
  const ownedSessions = sessions.filter((session) => session.task_id === task.id);
  const requestedSession = requestedSessionId
    ? ownedSessions.find((session) => session.id === requestedSessionId)
    : undefined;
  const primarySession = task.primary_session_id
    ? ownedSessions.find((session) => session.id === task.primary_session_id)
    : undefined;
  return requestedSession?.id ?? primarySession?.id ?? ownedSessions[0]?.id ?? null;
}

/** Builds task identity and lightweight session state before optional resources settle. */
export function buildTaskNavigationShellData(
  identity: TaskNavigationIdentity,
  requestedSessionId?: string,
): FetchedSessionData {
  const { task, allSessionsResponse } = identity;
  const allSessions = (allSessionsResponse.sessions ?? []).filter(
    (session) => session.task_id === task.id,
  );
  const sessionId = identity.sessionListUnavailable
    ? (requestedSessionId ?? task.primary_session_id ?? null)
    : resolveTaskSessionId(task, allSessionsResponse, requestedSessionId);
  return {
    task,
    sessionId,
    initialState: buildSessionPageState({
      task,
      sessionId,
      allSessions,
      activeSession: null,
      taskSessionsLoaded: !identity.sessionListUnavailable,
    }),
    initialTerminals: [],
  };
}

/** Loads full session and optional convenience state after task identity is visible. */
export async function fetchTaskNavigationEnrichment(
  identity: TaskNavigationIdentity,
  requestedSessionId?: string,
  options: { store?: StoreApi<AppState> } = {},
): Promise<FetchedSessionData> {
  const { task } = identity;
  const allSessionsResponse = identity.sessionListUnavailable
    ? await (options.store
        ? fetchSharedTaskSessions(options.store, task.id)
        : listTaskSessions(task.id, { cache: "no-store" }))
    : identity.allSessionsResponse;
  const sessionId = resolveTaskSessionId(task, allSessionsResponse, requestedSessionId);
  const loadAgentList = options.store
    ? () => getAgentListResourceScope(options.store!).ensure()
    : () => listAgents({ cache: "no-store" });
  if (!sessionId) return fetchTaskDataOnly(task, allSessionsResponse, loadAgentList, options.store);

  const optionalHydration = beginOptionalHydration();
  const { fetchTaskSession } = await import("@/lib/api");
  const activeSessionResponse = optionalHydration.load("active session snapshot", () =>
    fetchTaskSession(sessionId, { cache: "no-store" }),
  );
  return fetchSessionDataFromTask(task, sessionId, allSessionsResponse, {
    activeSessionResponse,
    optionalHydration,
    loadAgentList,
    store: options.store,
  });
}

/**
 * Builds SSR data for a task with no sessions yet: fetches the optional
 * convenience slices (workflow steps, agents, repositories, workspaces, workflows,
 * user settings) and seeds state with sessionId null so the auto-start hook can
 * fire immediately without a client-side crash.
 */
function routeMetadataReads(
  task: Task,
  optionalHydration: ReturnType<typeof beginOptionalHydration>,
  loadAgentList: () => Promise<AgentListResponse>,
  store?: StoreApi<AppState>,
) {
  return [
    optionalHydration.load("workflow steps", (signal) =>
      task.workflow_id
        ? listWorkflowSteps(task.workflow_id, { cache: "no-store", init: { signal } })
        : Promise.resolve({ steps: [], total: 0 }),
    ),
    optionalHydration.load("agents", loadAgentList),
    optionalHydration.load("repositories", (signal) =>
      readJourneyRepositories(store, task.workspace_id, { includeScripts: true, signal }),
    ),
    optionalHydration.load("workspaces", (signal) => readJourneyWorkspaces(store, { signal })),
    optionalHydration.load("workflows", (signal) =>
      readJourneyWorkflows(store, task.workspace_id, { includeHidden: true, signal }),
    ),
    optionalHydration.load("user settings", (signal) => readJourneyUserSettings(store, { signal })),
  ] as const;
}

async function fetchTaskDataOnly(
  task: Task,
  allSessionsResponse: Awaited<ReturnType<typeof listTaskSessions>>,
  loadAgentList: () => Promise<AgentListResponse> = () => listAgents({ cache: "no-store" }),
  store?: StoreApi<AppState>,
): Promise<FetchedSessionData> {
  const optionalHydration = beginOptionalHydration();
  const results = await Promise.all([
    ...routeMetadataReads(task, optionalHydration, loadAgentList, store),
  ]);
  optionalHydration.complete();
  const [
    workflowSteps,
    agents,
    repositoriesResponse,
    workspacesResponse,
    workflowsResponse,
    userSettingsResponse,
  ] = results;

  const allSessions = allSessionsResponse.sessions ?? [];
  const workflowStepsValue = optionalValue(workflowSteps);
  const agentsValue = optionalValue(agents);
  const repositories = optionalValue(repositoriesResponse)?.repositories;
  const workspaces = optionalValue(workspacesResponse)?.workspaces;
  const workflows = optionalValue(workflowsResponse)?.workflows;

  const initialState = buildSessionPageState({
    task,
    sessionId: null,
    workflowSteps: workflowStepsValue,
    agents: agentsValue,
    repositories,
    allSessions,
    activeSession: null,
    workspaces,
    workflows,
    turns: [],
    userSettingsResponse: optionalValue(userSettingsResponse),
    messagesResponse: null,
  });

  return { task, sessionId: null, initialState, initialTerminals: [] };
}

async function fetchSharedTaskSessions(store: StoreApi<AppState>, taskId: string) {
  const reads = getTaskSessionReads(store);
  const release = reads.retain(taskId);
  try {
    return await reads.read(taskId, (signal) =>
      listTaskSessions(taskId, { cache: "no-store", init: { signal } }),
    );
  } finally {
    release();
  }
}

/**
 * Shared SSR pipeline: fans out optional enrichment requests (workflow steps,
 * agents, repositories, workspaces, workflows, user settings, terminals, and
 * a bounded message-turn window under one optional-hydration deadline, then assembles the initial
 * state and the hydrated terminals.
 */
async function fetchSessionDataFromTask(
  task: Task,
  sessionId: string,
  allSessionsResponse: Awaited<ReturnType<typeof listTaskSessions>>,
  options: {
    activeSessionResponse: Promise<OptionalHydrationResult<{ session?: TaskSession | null }>>;
    optionalHydration: ReturnType<typeof beginOptionalHydration>;
    loadAgentList?: () => Promise<AgentListResponse>;
    store?: StoreApi<AppState>;
  },
): Promise<FetchedSessionData> {
  const { activeSessionResponse, optionalHydration, store } = options;
  const loadAgentList = options.loadAgentList ?? (() => listAgents({ cache: "no-store" }));
  // User shells are env-scoped — look up this session's task_environment_id
  // from the already-fetched session list. Sessions w/o env (legacy) skip
  // the terminal SSR fetch; the boot-time heal pass + WS-driven user_shell.list
  // will populate it once the env mapping lands.
  const sessionEnvId =
    allSessionsResponse.sessions?.find((s) => s.id === sessionId)?.task_environment_id ?? "";

  const results = await Promise.all([
    ...routeMetadataReads(task, optionalHydration, loadAgentList, store),
    optionalHydration.load("terminals", () =>
      sessionEnvId ? fetchTerminals(task.id, sessionEnvId) : Promise.resolve([]),
    ),
    optionalHydration.load("messages", () =>
      listTaskSessionMessages(
        sessionId,
        { limit: 50, sort: "desc", include_turns: true },
        { cache: "no-store" },
      ),
    ),
    activeSessionResponse,
  ]);
  const [
    workflowSteps,
    agents,
    repositoriesResponse,
    workspacesResponse,
    workflowsResponse,
    userSettingsResponse,
    terminalsResponse,
    messagesResponse,
    activeSessionResult,
  ] = results;

  const allSessions = allSessionsResponse.sessions ?? [];
  const workflowStepsValue = optionalValue(workflowSteps);
  const agentsValue = optionalValue(agents);
  const repositories = optionalValue(repositoriesResponse)?.repositories;
  const workspaces = optionalValue(workspacesResponse)?.workspaces;
  const workflows = optionalValue(workflowsResponse)?.workflows;
  const terminals = optionalValue(terminalsResponse) ?? [];
  const messages = optionalValue(messagesResponse);
  const activeSession = optionalValue(activeSessionResult)?.session ?? null;
  const { turns, turnCoverage } = await resolveMessageTurnWindow(
    sessionId,
    messages,
    optionalHydration,
  );
  optionalHydration.complete();

  const initialTerminals: Terminal[] = hydrateSessionTerminals(terminals);

  const initialState = buildSessionPageState({
    task,
    sessionId,
    workflowSteps: workflowStepsValue,
    agents: agentsValue,
    repositories,
    allSessions,
    activeSession,
    workspaces,
    workflows,
    turns,
    turnCoverage,
    userSettingsResponse: optionalValue(userSettingsResponse),
    messagesResponse: messages,
  });

  return { task, sessionId, initialState, initialTerminals };
}

async function resolveMessageTurnWindow(
  sessionId: string,
  messages: ListMessagesResponse | undefined,
  optionalHydration: ReturnType<typeof beginOptionalHydration>,
): Promise<{
  turns?: ListMessagesResponse["turns"];
  turnCoverage?: MessageTurnCoverage;
}> {
  if (messages?.turn_coverage) {
    return { turns: messages.turns, turnCoverage: messages.turn_coverage };
  }
  const turnsResponse = await optionalHydration.load("session turns", () =>
    listSessionTurns(sessionId, { cache: "no-store" }),
  );
  return { turns: optionalValue(turnsResponse)?.turns };
}

/** Returns the repositories seeded for a task's workspace, or [] when absent. */
export function extractInitialRepositories(
  initialState: FetchedSessionData["initialState"] | null,
  task: Task | null,
) {
  return initialState?.repositories?.itemsByWorkspaceId?.[task?.workspace_id ?? ""] ?? [];
}

/** Returns the scripts seeded for a task's first repository, or [] when absent. */
export function extractInitialScripts(
  initialState: FetchedSessionData["initialState"] | null,
  task: Task | null,
) {
  const repoId = task?.repositories?.[0]?.repository_id ?? "";
  return initialState?.repositoryScripts?.itemsByRepositoryId?.[repoId] ?? [];
}
