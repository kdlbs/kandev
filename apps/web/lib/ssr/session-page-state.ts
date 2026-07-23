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
import { mapWorkspaceItem } from "@/lib/routing/route-bootstrap";

export const OPTIONAL_HYDRATION_TIMEOUT_MS = 5_000;

type OptionalHydrationResult<T> = { status: "fulfilled"; value: T } | { status: "unavailable" };

function beginOptionalHydration() {
  let deadlineTimer: ReturnType<typeof setTimeout>;
  const deadline = new Promise<void>((resolve) => {
    deadlineTimer = setTimeout(resolve, OPTIONAL_HYDRATION_TIMEOUT_MS);
  });

  return {
    load<T>(label: string, operation: () => Promise<T>): Promise<OptionalHydrationResult<T>> {
      return new Promise((resolve) => {
        let settled = false;
        const settle = (value: OptionalHydrationResult<T>) => {
          if (settled) return;
          settled = true;
          resolve(value);
        };

        void Promise.resolve()
          .then(operation)
          .then(
            (value) => settle({ status: "fulfilled", value }),
            (error) => {
              if (settled) return;
              console.warn(
                `[session-page-state] optional ${label} failed; continuing without it`,
                error,
              );
              settle({ status: "unavailable" });
            },
          );
        void deadline.then(() => {
          if (!settled) {
            console.warn(
              `[session-page-state] optional ${label} timed out after ${OPTIONAL_HYDRATION_TIMEOUT_MS}ms; continuing without it`,
            );
            settle({ status: "unavailable" });
          }
        });
      });
    },
    complete() {
      clearTimeout(deadlineTimer);
    },
  };
}

function optionalValue<T>(result: OptionalHydrationResult<T>): T | undefined {
  return result.status === "fulfilled" ? result.value : undefined;
}

function optionalState<T>(
  value: T | undefined,
  build: (resolved: T) => Partial<AppState>,
): Partial<AppState> {
  return value === undefined ? {} : build(value);
}

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
  snapshot?: Awaited<ReturnType<typeof fetchWorkflowSnapshot>>;
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
  userSettingsResponse?: UserSettingsResponse | null;
  messagesResponse?: ListMessagesResponse | null;
};

function buildSessionPageState(p: BuildSessionPageStateParams) {
  const { task, sessionId, snapshot, agents, allSessions, messagesResponse } = p;
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
    ...optionalState(snapshot, snapshotToState),
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
        items: value.agents.flatMap((agent) =>
          agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
        ),
        version: 0,
      },
    })),
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
    ...(turns !== undefined
      ? {
          turns: sessionId
            ? {
                bySession: { [sessionId]: turns },
                activeBySession: {
                  [sessionId]: turns.filter((t) => !t.completed_at).pop()?.id ?? null,
                },
              }
            : { bySession: {}, activeBySession: {} },
        }
      : {}),
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

  const optionalHydration = beginOptionalHydration();
  return fetchSessionDataFromTask(
    task,
    sessionId,
    allSessionsResponse,
    Promise.resolve({ status: "fulfilled", value: { session: activeSession } }),
    optionalHydration,
  );
}

export async function fetchSessionDataForTask(taskId: string): Promise<FetchedSessionData> {
  const [task, allSessionsResponse] = await Promise.all([
    fetchTask(taskId, { cache: "no-store" }),
    listTaskSessions(taskId, { cache: "no-store" }),
  ]);
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
  // All remaining enrichment shares this deadline so no optional request can
  // extend route loading beyond the configured bound.
  const optionalHydration = beginOptionalHydration();
  const { fetchTaskSession } = await import("@/lib/api");
  const activeSessionResponse = optionalHydration.load("active session snapshot", () =>
    fetchTaskSession(sessionId, { cache: "no-store" }),
  );
  return fetchSessionDataFromTask(
    task,
    sessionId,
    allSessionsResponse,
    activeSessionResponse,
    optionalHydration,
  );
}

async function fetchTaskDataOnly(
  task: Task,
  allSessionsResponse: Awaited<ReturnType<typeof listTaskSessions>>,
): Promise<FetchedSessionData> {
  const optionalHydration = beginOptionalHydration();
  const results = await Promise.all([
    // Only the task and its session list are essential to route recovery. These
    // enrichment requests seed convenience state and must never strand the route.
    optionalHydration.load("workflow snapshot", () =>
      task.workflow_id
        ? fetchWorkflowSnapshot(task.workflow_id, { cache: "no-store" })
        : Promise.resolve({ steps: [], tasks: [] } as unknown as WorkflowSnapshot),
    ),
    optionalHydration.load("agents", () => listAgents({ cache: "no-store" })),
    optionalHydration.load("repositories", () =>
      listRepositories(task.workspace_id, { includeScripts: true }, { cache: "no-store" }),
    ),
    optionalHydration.load("workspaces", () => listWorkspaces({ cache: "no-store" })),
    optionalHydration.load("workflows", () =>
      listWorkflows(task.workspace_id, { cache: "no-store", includeHidden: true }),
    ),
    optionalHydration.load("user settings", () => fetchUserSettings({ cache: "no-store" })),
  ]);
  optionalHydration.complete();
  const [
    snapshot,
    agents,
    repositoriesResponse,
    workspacesResponse,
    workflowsResponse,
    userSettingsResponse,
  ] = results;

  const allSessions = allSessionsResponse.sessions ?? [];
  const snapshotValue = optionalValue(snapshot);
  const agentsValue = optionalValue(agents);
  const repositories = optionalValue(repositoriesResponse)?.repositories;
  const workspaces = optionalValue(workspacesResponse)?.workspaces;
  const workflows = optionalValue(workflowsResponse)?.workflows;

  const initialState = buildSessionPageState({
    task,
    sessionId: null,
    snapshot: snapshotValue,
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
  activeSessionResponse: Promise<OptionalHydrationResult<{ session?: TaskSession | null }>>,
  optionalHydration: ReturnType<typeof beginOptionalHydration>,
): Promise<FetchedSessionData> {
  // User shells are env-scoped — look up this session's task_environment_id
  // from the already-fetched session list. Sessions w/o env (legacy) skip
  // the terminal SSR fetch; the boot-time heal pass + WS-driven user_shell.list
  // will populate it once the env mapping lands.
  const sessionEnvId =
    allSessionsResponse.sessions?.find((s) => s.id === sessionId)?.task_environment_id ?? "";

  const results = await Promise.all([
    // The required task and session list were fetched before this optional fan-out.
    optionalHydration.load("workflow snapshot", () =>
      task.workflow_id
        ? fetchWorkflowSnapshot(task.workflow_id, { cache: "no-store" })
        : Promise.resolve({ steps: [], tasks: [] } as unknown as WorkflowSnapshot),
    ),
    optionalHydration.load("agents", () => listAgents({ cache: "no-store" })),
    optionalHydration.load("repositories", () =>
      listRepositories(task.workspace_id, { includeScripts: true }, { cache: "no-store" }),
    ),
    optionalHydration.load("workspaces", () => listWorkspaces({ cache: "no-store" })),
    optionalHydration.load("workflows", () =>
      listWorkflows(task.workspace_id, { cache: "no-store", includeHidden: true }),
    ),
    optionalHydration.load("session turns", () =>
      listSessionTurns(sessionId, { cache: "no-store" }),
    ),
    optionalHydration.load("user settings", () => fetchUserSettings({ cache: "no-store" })),
    optionalHydration.load("terminals", () =>
      sessionEnvId ? fetchTerminals(task.id, sessionEnvId) : Promise.resolve([]),
    ),
    optionalHydration.load("messages", () =>
      listTaskSessionMessages(sessionId, { limit: 50, sort: "desc" }, { cache: "no-store" }),
    ),
    activeSessionResponse,
  ]);
  optionalHydration.complete();
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
    activeSessionResult,
  ] = results;

  const allSessions = allSessionsResponse.sessions ?? [];
  const snapshotValue = optionalValue(snapshot);
  const agentsValue = optionalValue(agents);
  const repositories = optionalValue(repositoriesResponse)?.repositories;
  const workspaces = optionalValue(workspacesResponse)?.workspaces;
  const workflows = optionalValue(workflowsResponse)?.workflows;
  const turns = optionalValue(turnsResponse)?.turns;
  const terminals = optionalValue(terminalsResponse) ?? [];
  const messages = optionalValue(messagesResponse);
  const activeSession = optionalValue(activeSessionResult)?.session ?? null;

  const initialTerminals: Terminal[] = terminals.filter(shouldHydrateTerminal).map(hydrateTerminal);

  const initialState = buildSessionPageState({
    task,
    sessionId,
    snapshot: snapshotValue,
    agents: agentsValue,
    repositories,
    allSessions,
    activeSession,
    workspaces,
    workflows,
    turns,
    userSettingsResponse: optionalValue(userSettingsResponse),
    messagesResponse: messages,
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
